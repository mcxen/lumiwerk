//go:build darwin

package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	mpvFormatFlag   = 3
	mpvFormatDouble = 5
)

type mpvRenderParam struct {
	kind int32
	data unsafe.Pointer
}

type mpvAPI struct {
	handle       uintptr
	create       func() uintptr
	initialize   func(uintptr) int32
	destroy      func(uintptr)
	setOption    func(uintptr, string, string) int32
	command      func(uintptr, string) int32
	setProperty  func(uintptr, string, int32, unsafe.Pointer) int32
	getProperty  func(uintptr, string, int32, unsafe.Pointer) int32
	renderCreate func(*uintptr, uintptr, *mpvRenderParam) int32
	render       func(uintptr, *mpvRenderParam) int32
	renderFree   func(uintptr)
}

type mpvRenderRequest struct {
	width, height int
	result        chan mpvRenderResult
}

type mpvRenderResult struct {
	frame *image.NRGBA
	err   error
}

type mpvBackend struct {
	mu       sync.Mutex
	api      *mpvAPI
	core     uintptr
	render   uintptr
	requests chan mpvRenderRequest
	closed   chan struct{}
	pixels   []byte
	frame    image.Image
	width    int
	height   int
}

func loadMPVAPI() (*mpvAPI, error) {
	var candidates []string
	if configured := os.Getenv("LUMIWERK_MPV"); configured != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "..", "Frameworks", "libmpv.dylib"))
	}
	candidates = append(candidates, "libmpv.2.dylib", "libmpv.dylib",
		"/opt/homebrew/lib/libmpv.2.dylib", "/usr/local/lib/libmpv.2.dylib")
	var lastErr error
	for _, candidate := range candidates {
		handle, err := purego.Dlopen(candidate, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			lastErr = err
			continue
		}
		api := &mpvAPI{handle: handle}
		purego.RegisterLibFunc(&api.create, handle, "mpv_create")
		purego.RegisterLibFunc(&api.initialize, handle, "mpv_initialize")
		purego.RegisterLibFunc(&api.destroy, handle, "mpv_destroy")
		purego.RegisterLibFunc(&api.setOption, handle, "mpv_set_option_string")
		purego.RegisterLibFunc(&api.command, handle, "mpv_command_string")
		purego.RegisterLibFunc(&api.setProperty, handle, "mpv_set_property")
		purego.RegisterLibFunc(&api.getProperty, handle, "mpv_get_property")
		purego.RegisterLibFunc(&api.renderCreate, handle, "mpv_render_context_create")
		purego.RegisterLibFunc(&api.render, handle, "mpv_render_context_render")
		purego.RegisterLibFunc(&api.renderFree, handle, "mpv_render_context_free")
		return api, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no libmpv candidates")
	}
	return nil, lastErr
}

func newMPVBackend() (*mpvBackend, error) {
	api, err := loadMPVAPI()
	if err != nil {
		return nil, err
	}
	core := api.create()
	if core == 0 {
		return nil, fmt.Errorf("mpv_create returned nil")
	}
	for key, value := range map[string]string{
		"config": "no", "input-default-bindings": "no", "input-vo-keyboard": "no",
		"terminal": "no", "vo": "libmpv", "keep-open": "yes", "idle": "yes",
		"hwdec": "auto-safe", "video-timing-offset": "0", "osd-level": "0",
	} {
		if code := api.setOption(core, key, value); code < 0 {
			api.destroy(core)
			return nil, fmt.Errorf("mpv option %s: code %d", key, code)
		}
	}
	if code := api.initialize(core); code < 0 {
		api.destroy(core)
		return nil, fmt.Errorf("mpv_initialize: code %d", code)
	}
	backend := &mpvBackend{api: api, core: core, requests: make(chan mpvRenderRequest), closed: make(chan struct{})}
	ready := make(chan error, 1)
	go backend.renderLoop(ready)
	if err := <-ready; err != nil {
		api.destroy(core)
		return nil, err
	}
	return backend, nil
}

// All render calls stay on one locked OS thread. Client/property calls may be
// made concurrently by the UI thread as allowed by the libmpv API.
func (b *mpvBackend) renderLoop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	apiName := []byte("sw\x00")
	params := []mpvRenderParam{{kind: 1, data: unsafe.Pointer(&apiName[0])}, {}}
	if code := b.api.renderCreate(&b.render, b.core, &params[0]); code < 0 {
		ready <- fmt.Errorf("mpv software renderer: code %d", code)
		return
	}
	ready <- nil
	for {
		select {
		case request := <-b.requests:
			request.result <- b.renderFrame(request.width, request.height)
		case <-b.closed:
			b.api.renderFree(b.render)
			b.render = 0
			b.api.destroy(b.core)
			b.core = 0
			return
		}
	}
}

func (b *mpvBackend) renderFrame(width, height int) (result mpvRenderResult) {
	if width < 2 || height < 2 {
		return result
	}
	stride := (width*4 + 63) &^ 63
	needed := stride * height
	if cap(b.pixels) < needed {
		b.pixels = make([]byte, needed)
	}
	pixels := b.pixels[:needed]
	format := []byte("rgb0\x00")
	size := []int32{int32(width), int32(height)}
	strideValue := uintptr(stride)
	block := int32(1)
	params := []mpvRenderParam{
		{kind: 17, data: unsafe.Pointer(&size[0])},
		{kind: 18, data: unsafe.Pointer(&format[0])},
		{kind: 19, data: unsafe.Pointer(&strideValue)},
		{kind: 20, data: unsafe.Pointer(&pixels[0])},
		{kind: 12, data: unsafe.Pointer(&block)},
		{},
	}
	if code := b.api.render(b.render, &params[0]); code < 0 {
		result.err = fmt.Errorf("mpv_render_context_render: code %d", code)
		return result
	}
	frame := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		row := pixels[y*stride : y*stride+width*4]
		for x := 3; x < len(row); x += 4 {
			row[x] = 255
		}
		copy(frame.Pix[y*frame.Stride:(y+1)*frame.Stride], row)
	}
	result.frame = frame
	return result
}

func (b *mpvBackend) Name() string { return "libmpv" }

func (b *mpvBackend) Open(path string, at float64, playing bool) error {
	b.SetPlaying(playing)
	options := ""
	if at > 0 {
		options = " start=" + strconv.FormatFloat(at, 'f', 3, 64)
	}
	if code := b.api.command(b.core, "loadfile "+strconv.Quote(path)+" replace -1"+options); code < 0 {
		return fmt.Errorf("mpv loadfile: code %d", code)
	}
	return nil
}

func (b *mpvBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.core == 0 {
		return nil
	}
	close(b.closed)
	b.core = 0
	return nil
}

func (b *mpvBackend) Frame() image.Image {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.core == 0 {
		return nil
	}
	width, height := int(b.double("video-out-params/dw")), int(b.double("video-out-params/dh"))
	if width < 2 || height < 2 {
		return b.frame
	}
	const maxEdge = 2560
	if max(width, height) > maxEdge {
		scale := float64(maxEdge) / float64(max(width, height))
		width, height = int(float64(width)*scale), int(float64(height)*scale)
	}
	request := mpvRenderRequest{width: width, height: height, result: make(chan mpvRenderResult, 1)}
	select {
	case b.requests <- request:
	case <-b.closed:
		return nil
	}
	select {
	case result := <-request.result:
		if result.err != nil {
			logWarnf("video.mpv.render: %v", result.err)
			return b.frame
		}
		if result.frame != nil {
			b.frame = result.frame
		}
	case <-b.closed:
		return nil
	}
	return b.frame
}

func (b *mpvBackend) double(name string) float64 {
	value := float64(0)
	if b.api.getProperty(b.core, name, mpvFormatDouble, unsafe.Pointer(&value)) < 0 {
		return 0
	}
	return value
}

func (b *mpvBackend) setDouble(name string, value float64) {
	if code := b.api.setProperty(b.core, name, mpvFormatDouble, unsafe.Pointer(&value)); code < 0 {
		logDebugf("video.mpv.set: property=%s code=%d", name, code)
	}
}

func (b *mpvBackend) Position() float64 { return b.double("time-pos") }
func (b *mpvBackend) Duration() float64 { return b.double("duration") }

func (b *mpvBackend) SetPlaying(playing bool) {
	value := int32(0)
	if !playing {
		value = 1
	}
	_ = b.api.setProperty(b.core, "pause", mpvFormatFlag, unsafe.Pointer(&value))
}

func (b *mpvBackend) Seek(seconds float64) {
	b.setDouble("time-pos", clampPlaybackTime(seconds, b.Duration()))
}

func (b *mpvBackend) SetVolume(volume float64) {
	b.setDouble("volume", volume*100)
}

func (b *mpvBackend) SetSpeed(speed float64) { b.setDouble("speed", normalizedPlaybackSpeed(speed)) }

func (b *mpvBackend) FrameStep(direction int) (float64, error) {
	b.SetPlaying(false)
	command := "frame-step"
	if direction < 0 {
		command = "frame-back-step"
	}
	if code := b.api.command(b.core, command); code < 0 {
		return b.Position(), fmt.Errorf("mpv %s: code %d", command, code)
	}
	return b.Position(), nil
}
