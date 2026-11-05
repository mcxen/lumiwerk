//go:build !android && !ios && mpvbackend

package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	mpv "github.com/gen2brain/go-mpv"
)

// mpvBackend drives libmpv through gen2brain/go-mpv. It renders software
// frames off one locked OS thread, mirroring the raw-FFI backend it
// replaces, and works on every desktop platform libmpv supports.
type mpvBackend struct {
	mu          sync.Mutex
	core        *mpv.Mpv
	render      *mpv.RenderContext
	requests    chan mpvRenderRequest
	closed      chan struct{}
	eventsDone  chan struct{}
	stop        sync.Once
	pixels      []byte
	frame       image.Image
	audioTracks []playbackTrack
	videoTracks []playbackTrack
	probedAt    time.Time
}

type mpvRenderRequest struct {
	width, height int
	result        chan mpvRenderResult
}

type mpvRenderResult struct {
	frame *image.NRGBA
	err   error
}

func init() {
	// The bundled dylibs ship among the app's resources (Contents/Resources
	// in a macOS bundle, the executable's directory elsewhere). The loader
	// tries those before the bare system names; a dev run without the
	// bundle falls back to a system libmpv if one is installed.
	mpv.SetSearchPaths(func() []string {
		var dirs []string
		if dir, err := mygo.App.Path(mygo.PathResources); err == nil && dir != "" {
			dirs = append(dirs, dir)
		}
		if exe, err := os.Executable(); err == nil {
			dirs = append(dirs, filepath.Dir(exe))
		}
		return dirs
	})
}

func newMPVBackend() (playbackBackend, error) {
	core := mpv.New()
	if core == nil {
		return nil, fmt.Errorf("libmpv: %w", mpv.LoadError())
	}
	for key, value := range map[string]string{
		"config": "no", "input-default-bindings": "no", "input-vo-keyboard": "no",
		"terminal": "no", "vo": "libmpv", "keep-open": "yes", "idle": "yes",
		"hwdec": "auto-safe", "video-timing-offset": "0", "osd-level": "0",
	} {
		if err := core.SetOptionString(key, value); err != nil {
			core.TerminateDestroy()
			return nil, fmt.Errorf("mpv option %s: %w", key, err)
		}
	}
	if err := core.Initialize(); err != nil {
		core.TerminateDestroy()
		return nil, fmt.Errorf("mpv_initialize: %w", err)
	}
	backend := &mpvBackend{
		core: core, requests: make(chan mpvRenderRequest),
		closed: make(chan struct{}), eventsDone: make(chan struct{}),
	}
	ready := make(chan error, 1)
	go backend.renderLoop(ready)
	if err := <-ready; err != nil {
		core.TerminateDestroy()
		return nil, err
	}
	return backend, nil
}

// All render calls stay on one locked OS thread. Client/property calls may be
// made concurrently by the UI thread as allowed by the libmpv API.
//
// libmpv stops accepting pause (and other property writes) once its event
// queue fills, and the picture clock does not follow pause unless each
// presented frame is reported. The event loop drains that queue; renderFrame
// reports the swap.
func (b *mpvBackend) renderLoop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	render, err := b.core.NewRenderContextSW()
	if err != nil {
		ready <- fmt.Errorf("mpv software renderer: %w", err)
		return
	}
	b.render = render
	go b.eventLoop()
	ready <- nil
	for {
		select {
		case request := <-b.requests:
			request.result <- b.renderFrame(request.width, request.height)
		case <-b.closed:
			b.shutdownRender(render)
			return
		}
	}
}

// eventLoop is the one thread allowed to call WaitEvent. Without it the
// client queue fills and mpv_set_property("pause") stops taking effect
// while the file keeps playing.
func (b *mpvBackend) eventLoop() {
	defer close(b.eventsDone)
	// Copy the core once. Taking b.mu on every poll would stall this loop
	// behind Frame, which holds the lock while rendering, and a render that
	// needs the event queue drained would then deadlock.
	b.mu.Lock()
	core := b.core
	b.mu.Unlock()
	if core == nil {
		return
	}
	for {
		select {
		case <-b.closed:
			return
		default:
		}
		ev := core.WaitEvent(0.1)
		if ev != nil && ev.EventID == mpv.EventShutdown {
			return
		}
	}
}

// shutdownRender runs on the render thread. The event loop must leave
// before the core is destroyed; Wakeup unblocks WaitEvent.
func (b *mpvBackend) shutdownRender(render *mpv.RenderContext) {
	b.mu.Lock()
	core := b.core
	b.mu.Unlock()
	if core != nil {
		core.Wakeup()
	}
	<-b.eventsDone
	render.Free()
	b.mu.Lock()
	core = b.core
	b.core = nil
	b.render = nil
	b.mu.Unlock()
	if core != nil {
		core.TerminateDestroy()
	}
}

func (b *mpvBackend) renderFrame(width, height int) (result mpvRenderResult) {
	if width < 2 || height < 2 {
		return result
	}
	if b.render != nil {
		b.render.Update()
	}
	stride := (width*4 + 63) &^ 63
	needed := stride * height
	if cap(b.pixels) < needed {
		b.pixels = make([]byte, needed)
	}
	pixels := b.pixels[:needed]
	if err := b.render.RenderSW(width, height, stride, "rgb0", pixels); err != nil {
		result.err = fmt.Errorf("mpv render: %w", err)
		return result
	}
	b.render.ReportSwap()
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
	if err := b.core.CommandString("loadfile " + strconv.Quote(path) + " replace -1" + options); err != nil {
		return fmt.Errorf("mpv loadfile: %w", err)
	}
	// loadfile can clear a pause that was set before the file was current.
	if !playing {
		b.SetPlaying(false)
	}
	return nil
}

func (b *mpvBackend) Close() error {
	b.mu.Lock()
	core := b.core
	b.mu.Unlock()
	if core == nil {
		return nil
	}
	b.stop.Do(func() {
		close(b.closed)
		core.Wakeup()
	})
	return nil
}

// Park unloads the file but keeps the libmpv core and software renderer.
func (b *mpvBackend) Park() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frame = nil
	if b.core == nil {
		return
	}
	_ = b.core.CommandString("stop")
}

func (b *mpvBackend) Frame() image.Image {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.core == nil {
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
	value, err := b.core.GetProperty(name, mpv.FormatDouble)
	if err != nil {
		return 0
	}
	d, _ := value.(float64)
	return d
}

func (b *mpvBackend) setDouble(name string, value float64) {
	if err := b.core.SetProperty(name, mpv.FormatDouble, value); err != nil {
		logDebugf("video.mpv.set: property=%s err=%v", name, err)
	}
}

func (b *mpvBackend) Position() float64 { return b.double("time-pos") }
func (b *mpvBackend) Duration() float64 { return b.double("duration") }

func (b *mpvBackend) SetPlaying(playing bool) {
	b.mu.Lock()
	core := b.core
	b.mu.Unlock()
	if core == nil {
		return
	}
	// String form, not FormatFlag: the flag pointer used to be a stack slot
	// that a later call could overwrite before libmpv read it, so pause was
	// stored as false and the file kept playing.
	value := "yes"
	if playing {
		value = "no"
	}
	if err := core.SetPropertyString("pause", value); err != nil {
		logWarnf("video.mpv.pause: playing=%t err=%v", playing, err)
	}
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
	if err := b.core.CommandString(command); err != nil {
		return b.Position(), fmt.Errorf("mpv %s: %w", command, err)
	}
	return b.Position(), nil
}

func (b *mpvBackend) Buffered() float64 {
	if b == nil || b.core == nil {
		return 0
	}
	pos := b.double("time-pos")
	buf := pos + b.double("demuxer-cache-duration")
	if end := b.double("demuxer-cache-time"); end > buf {
		buf = end
	}
	if dur := b.Duration(); dur > 0 && buf > dur {
		buf = dur
	}
	if buf < pos {
		buf = pos
	}
	return buf
}

func (b *mpvBackend) Stalled() bool {
	if b == nil || b.core == nil {
		return false
	}
	value, err := b.core.GetProperty("paused-for-cache", mpv.FormatFlag)
	if err != nil {
		return false
	}
	flag, _ := value.(bool)
	return flag
}

func (b *mpvBackend) AudioTracks() []playbackTrack {
	b.refreshTracks()
	return append([]playbackTrack(nil), b.audioTracks...)
}

func (b *mpvBackend) VideoTracks() []playbackTrack {
	b.refreshTracks()
	return append([]playbackTrack(nil), b.videoTracks...)
}

func (b *mpvBackend) SelectAudio(id int) error {
	return b.selectMPVTrack("aid", id, true)
}

func (b *mpvBackend) SelectVideo(id int) error {
	return b.selectMPVTrack("vid", id, false)
}

func (b *mpvBackend) selectMPVTrack(prop string, id int, audio bool) error {
	if b == nil || b.core == nil {
		return fmt.Errorf("mpv closed")
	}
	if err := b.core.SetProperty(prop, mpv.FormatInt64, int64(id)); err != nil {
		return err
	}
	tracks := b.videoTracks
	if audio {
		tracks = b.audioTracks
	}
	for i := range tracks {
		tracks[i].Selected = tracks[i].ID == id
	}
	b.probedAt = time.Time{}
	return nil
}

func (b *mpvBackend) refreshTracks() {
	if b == nil || b.core == nil {
		return
	}
	if !b.probedAt.IsZero() && time.Since(b.probedAt) < 500*time.Millisecond {
		return
	}
	b.probedAt = time.Now()
	value, err := b.core.GetProperty("track-list", mpv.FormatNode)
	if err != nil {
		return
	}
	list, ok := value.([]any)
	if !ok {
		return
	}
	var audio, video []playbackTrack
	audioN, videoN := 0, 0
	for _, raw := range list {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := item["type"].(string)
		id, ok := mpvInt(item["id"])
		if !ok {
			continue
		}
		selected, _ := item["selected"].(bool)
		switch kind {
		case "audio":
			audioN++
			audio = append(audio, playbackTrack{ID: id, Label: mpvTrackLabel("音轨", audioN, item), Selected: selected})
		case "video":
			videoN++
			video = append(video, playbackTrack{ID: id, Label: mpvTrackLabel("画质", videoN, item), Selected: selected})
		}
	}
	b.audioTracks, b.videoTracks = audio, video
}

func mpvInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func mpvTrackLabel(fallback string, n int, item map[string]any) string {
	title, _ := item["title"].(string)
	lang, _ := item["lang"].(string)
	codec, _ := item["codec"].(string)
	label := strings.TrimSpace(title)
	if label == "" {
		label = strings.TrimSpace(lang)
	}
	if label == "" {
		label = strings.TrimSpace(codec)
	}
	if w, ok := mpvInt(item["demux-w"]); ok && w > 0 {
		if h, ok := mpvInt(item["demux-h"]); ok && h > 0 {
			size := fmt.Sprintf("%d×%d", w, h)
			if label == "" {
				label = size
			} else {
				label += " " + size
			}
		}
	}
	if label == "" {
		return fmt.Sprintf("%s %d", fallback, n)
	}
	return label
}
