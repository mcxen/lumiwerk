//go:build !android && !ios && mpvbackend

package main

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	mpv "github.com/gen2brain/go-mpv"
)

// mpvBackend drives libmpv through gen2brain/go-mpv.
//
// The UI thread never waits for libmpv on the hot path:
//
//   - A render goroutine, locked to one OS thread, owns the software render
//     context. mpv's update callback wakes it when a new frame is due; it
//     renders straight into a pooled *image.RGBA (rows 4×width bytes, the
//     layout ui.Bitmap.Update takes without a copy) and publishes it.
//   - An event goroutine drains the client queue and keeps every property the
//     view reads (time-pos, duration, cache, size, tracks, eof) in a cache fed
//     by observe_property, so Position/Duration/Buffered/Stalled/tracks are
//     plain memory reads instead of synchronous mpv_get_property calls.
//   - Frame returns the newest published frame and never renders itself.
//   - Seeks, volume and speed go out asynchronously; only pause stays
//     synchronous so the session's paused picture matches the core.
type mpvBackend struct {
	// life guards core against destruction while a caller is inside libmpv.
	life   sync.RWMutex
	core   *mpv.Mpv
	render *mpv.RenderContext

	wake       chan struct{}
	closed     chan struct{}
	eventsDone chan struct{}
	renderDone chan struct{}
	stop       sync.Once

	st mpvState

	frames mpvFramePool
	// notify is called (coalesced) after a new frame is published.
	notify        atomic.Pointer[func()]
	notifyPending atomic.Bool
}

// mpvState is the property cache the event loop fills. Every field is read
// and written under mu, which is never held across a libmpv call.
type mpvState struct {
	mu          sync.Mutex
	accepting   bool // false between loadfile/stop and the next start-file
	position    float64
	duration    float64
	cacheDur    float64
	cacheTime   float64
	stalled     bool
	eof         bool
	width       int // dwidth/dheight: display size after aspect correction
	height      int
	audioTracks []playbackTrack
	videoTracks []playbackTrack
	// redraw asks the render loop to render even without a frame update,
	// e.g. when the size became known after mpv announced the first frame.
	redraw bool
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

// mpvRequiredOptions must all apply or the backend is unusable.
var mpvRequiredOptions = [][2]string{
	{"config", "no"}, {"input-default-bindings", "no"}, {"input-vo-keyboard", "no"},
	{"terminal", "no"}, {"vo", "libmpv"}, {"keep-open", "yes"}, {"idle", "yes"},
	{"video-timing-offset", "0"}, {"osd-level", "0"},
}

// mpvTunedOptions are best effort: an older libmpv may not know a value.
// The software render API has no hardware interop, so plain "auto-safe"
// silently decodes on the CPU; the copy-back variants keep VideoToolbox
// decoding and only copy the decoded planes back.
var mpvTunedOptions = [][]string{
	{"hwdec", "auto-copy-safe", "auto-copy", "auto-safe"},
	{"vd-lavc-threads", "0"},
	{"sws-fast", "yes"},
	{"zimg-fast", "yes"},
	{"hr-seek-framedrop", "yes"},
	{"demuxer-readahead-secs", "2"},
}

func newMPVBackend() (playbackBackend, error) {
	core := mpv.New()
	if core == nil {
		return nil, fmt.Errorf("libmpv: %w", mpv.LoadError())
	}
	for _, kv := range mpvRequiredOptions {
		if err := core.SetOptionString(kv[0], kv[1]); err != nil {
			core.TerminateDestroy()
			return nil, fmt.Errorf("mpv option %s: %w", kv[0], err)
		}
	}
	for _, opt := range mpvTunedOptions {
		for _, value := range opt[1:] {
			if err := core.SetOptionString(opt[0], value); err == nil {
				break
			} else {
				logDebugf("video.mpv.option: %s=%s err=%v", opt[0], value, err)
			}
		}
	}
	if err := core.Initialize(); err != nil {
		core.TerminateDestroy()
		return nil, fmt.Errorf("mpv_initialize: %w", err)
	}
	for _, p := range []struct {
		name   string
		format mpv.Format
	}{
		{"time-pos", mpv.FormatDouble}, {"duration", mpv.FormatDouble},
		{"demuxer-cache-duration", mpv.FormatDouble}, {"demuxer-cache-time", mpv.FormatDouble},
		{"paused-for-cache", mpv.FormatFlag}, {"eof-reached", mpv.FormatFlag},
		{"dwidth", mpv.FormatInt64}, {"dheight", mpv.FormatInt64},
		{"track-list", mpv.FormatNode},
	} {
		if err := core.ObserveProperty(0, p.name, p.format); err != nil {
			logDebugf("video.mpv.observe: property=%s err=%v", p.name, err)
		}
	}
	b := &mpvBackend{
		core: core, wake: make(chan struct{}, 1),
		closed: make(chan struct{}), eventsDone: make(chan struct{}),
		renderDone: make(chan struct{}),
	}
	ready := make(chan error, 1)
	go b.renderLoop(ready)
	if err := <-ready; err != nil {
		core.TerminateDestroy()
		return nil, err
	}
	return b, nil
}

// withCore runs fn with the live core, or returns false once it is gone.
func (b *mpvBackend) withCore(fn func(core *mpv.Mpv)) bool {
	b.life.RLock()
	defer b.life.RUnlock()
	if b.core == nil {
		return false
	}
	fn(b.core)
	return true
}

func (b *mpvBackend) signal() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// renderLoop owns the render context on one locked OS thread.
func (b *mpvBackend) renderLoop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(b.renderDone)
	render, err := b.core.NewRenderContextSW()
	if err != nil {
		ready <- fmt.Errorf("mpv software renderer: %w", err)
		return
	}
	b.render = render
	// The callback runs on an mpv thread: it may only signal.
	render.SetUpdateCallback(b.signal)
	go b.eventLoop()
	ready <- nil
	for {
		select {
		case <-b.wake:
		case <-b.closed:
			b.shutdownRender(render)
			return
		}
		flags := render.Update()
		b.st.mu.Lock()
		redraw := b.st.redraw
		b.st.redraw = false
		width, height := b.st.width, b.st.height
		b.st.mu.Unlock()
		if flags&mpv.RenderUpdateFrame == 0 && !redraw {
			continue
		}
		width, height = mpvRenderSize(width, height)
		if width < 2 || height < 2 {
			// mpv announced a frame before its size: render it once the
			// dwidth/dheight event arrives.
			b.st.mu.Lock()
			b.st.redraw = true
			b.st.mu.Unlock()
			continue
		}
		b.renderFrame(render, width, height)
	}
}

func (b *mpvBackend) renderFrame(render *mpv.RenderContext, width, height int) {
	frame := b.frames.acquire(width, height)
	if err := render.RenderSW(width, height, frame.Stride, "rgb0", frame.Pix); err != nil {
		b.frames.release(frame)
		logWarnf("video.mpv.render: %v", err)
		return
	}
	render.ReportSwap()
	opaque(frame.Pix)
	b.frames.publish(frame)
	if b.notifyPending.CompareAndSwap(false, true) {
		if fn := b.notify.Load(); fn != nil && *fn != nil {
			(*fn)()
		}
	}
}

// eventLoop is the one goroutine that calls WaitEvent. Draining the queue
// also keeps mpv_set_property("pause") effective: a full queue stops it.
func (b *mpvBackend) eventLoop() {
	defer close(b.eventsDone)
	core := b.core
	for {
		select {
		case <-b.closed:
			return
		default:
		}
		ev := core.WaitEvent(1)
		if ev == nil {
			continue
		}
		switch ev.EventID {
		case mpv.EventShutdown:
			return
		case mpv.EventStart:
			b.st.mu.Lock()
			b.st.resetLocked()
			b.st.accepting = true
			b.st.mu.Unlock()
		case mpv.EventPropertyChange:
			b.applyProperty(ev.Property())
		case mpv.EventVideoReconfig:
			b.st.mu.Lock()
			b.st.redraw = true
			b.st.mu.Unlock()
			b.signal()
		}
	}
}

func (s *mpvState) resetLocked() {
	s.position, s.duration, s.cacheDur, s.cacheTime = 0, 0, 0, 0
	s.stalled, s.eof = false, false
	s.width, s.height = 0, 0
	s.audioTracks, s.videoTracks = nil, nil
	s.redraw = false
}

func (b *mpvBackend) applyProperty(p mpv.EventProperty) {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	if !b.st.accepting {
		return
	}
	switch p.Name {
	case "time-pos":
		b.st.position = mpvFloat(p.Data)
	case "duration":
		b.st.duration = mpvFloat(p.Data)
	case "demuxer-cache-duration":
		b.st.cacheDur = mpvFloat(p.Data)
	case "demuxer-cache-time":
		b.st.cacheTime = mpvFloat(p.Data)
	case "paused-for-cache":
		b.st.stalled = mpvBool(p.Data)
	case "eof-reached":
		b.st.eof = mpvBool(p.Data)
	case "dwidth", "dheight":
		n, _ := mpvInt(p.Data)
		if p.Name == "dwidth" {
			b.st.width = n
		} else {
			b.st.height = n
		}
		if b.st.width > 1 && b.st.height > 1 {
			b.st.redraw = true
			b.signal()
		}
	case "track-list":
		list, _ := p.Data.([]any)
		b.st.audioTracks, b.st.videoTracks = mpvParseTracks(list)
	}
}

func mpvFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return n
	case int64:
		return float64(n)
	}
	return 0
}

func mpvBool(v any) bool {
	switch f := v.(type) {
	case bool:
		return f
	case int:
		return f != 0
	case int32:
		return f != 0
	case int64:
		return f != 0
	}
	return false
}

// shutdownRender runs on the render thread. The event loop must leave
// before the core is destroyed; Wakeup unblocks WaitEvent.
func (b *mpvBackend) shutdownRender(render *mpv.RenderContext) {
	b.core.Wakeup()
	<-b.eventsDone
	render.Free()
	b.life.Lock()
	core := b.core
	b.core = nil
	b.render = nil
	b.life.Unlock()
	if core != nil {
		core.TerminateDestroy()
	}
}

func (b *mpvBackend) Name() string { return "libmpv" }

// SetFrameNotify registers fn to run, off the main thread, when a new frame is
// ready. Calls are coalesced until the next Frame.
func (b *mpvBackend) SetFrameNotify(fn func()) {
	b.notify.Store(&fn)
	b.notifyPending.Store(false)
}

// beginLoad drops the cached state of the previous file and ignores its late
// property events until mpv reports the next start-file.
func (b *mpvBackend) beginLoad() {
	b.st.mu.Lock()
	b.st.resetLocked()
	b.st.accepting = false
	b.st.mu.Unlock()
	b.frames.clear()
}

func (b *mpvBackend) Open(path string, at float64, playing bool) error {
	b.SetPlaying(playing)
	b.beginLoad()
	command := "loadfile " + strconv.Quote(path) + " replace -1"
	if at > 0 {
		command += " start=" + strconv.FormatFloat(at, 'f', 3, 64)
	}
	var err error
	if !b.withCore(func(core *mpv.Mpv) { err = core.CommandString(command) }) {
		return fmt.Errorf("mpv closed")
	}
	if err != nil {
		return fmt.Errorf("mpv loadfile: %w", err)
	}
	// loadfile can clear a pause that was set before the file was current.
	if !playing {
		b.SetPlaying(false)
	}
	return nil
}

func (b *mpvBackend) Close() error {
	b.stop.Do(func() {
		close(b.closed)
		b.withCore(func(core *mpv.Mpv) { core.Wakeup() })
	})
	return nil
}

// Park unloads the file but keeps the libmpv core and software renderer.
func (b *mpvBackend) Park() {
	b.beginLoad()
	b.withCore(func(core *mpv.Mpv) { _ = core.CommandString("stop") })
}

// Frame returns the newest rendered frame. It never blocks on libmpv.
func (b *mpvBackend) Frame() image.Image {
	b.notifyPending.Store(false)
	if frame := b.frames.take(); frame != nil {
		return frame
	}
	return nil // not a typed nil: the session tests frame != nil
}

func (b *mpvBackend) setAsync(name string, value float64) {
	b.withCore(func(core *mpv.Mpv) {
		if err := core.SetPropertyAsync(name, 0, mpv.FormatDouble, value); err != nil {
			logDebugf("video.mpv.set: property=%s err=%v", name, err)
		}
	})
}

// Position is the cached playhead. At the end of a keep-open file mpv parks
// time-pos on the last frame, a frame short of the duration; report the
// duration so the session sees the end.
func (b *mpvBackend) Position() float64 {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	if b.st.eof && b.st.duration > 0 {
		return b.st.duration
	}
	return b.st.position
}

func (b *mpvBackend) Duration() float64 {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	return b.st.duration
}

func (b *mpvBackend) SetPlaying(playing bool) {
	// String form, not FormatFlag: the flag pointer used to be a stack slot
	// that a later call could overwrite before libmpv read it, so pause was
	// stored as false and the file kept playing.
	value := "yes"
	if playing {
		value = "no"
	}
	b.withCore(func(core *mpv.Mpv) {
		if err := core.SetPropertyString("pause", value); err != nil {
			logWarnf("video.mpv.pause: playing=%t err=%v", playing, err)
		}
	})
}

// Seek is asynchronous: mpv coalesces queued seeks, so a scrub never waits
// for the previous seek to finish on the UI thread.
func (b *mpvBackend) Seek(seconds float64) {
	b.st.mu.Lock()
	at := clampPlaybackTime(seconds, b.st.duration)
	b.st.eof = false
	b.st.mu.Unlock()
	b.setAsync("time-pos", at)
}

func (b *mpvBackend) SetVolume(volume float64) { b.setAsync("volume", volume*100) }

func (b *mpvBackend) SetSpeed(speed float64) {
	b.setAsync("speed", normalizedPlaybackSpeed(speed))
}

// FrameStep steps one frame and waits briefly for its picture and time, so
// the paused session takes the stepped frame instead of the previous one.
func (b *mpvBackend) FrameStep(direction int) (float64, error) {
	b.SetPlaying(false)
	command := "frame-step"
	if direction < 0 {
		command = "frame-back-step"
	}
	gen := b.frames.generation()
	before := b.Position()
	var err error
	if !b.withCore(func(core *mpv.Mpv) { err = core.CommandString(command) }) {
		return before, fmt.Errorf("mpv closed")
	}
	if err != nil {
		return before, fmt.Errorf("mpv %s: %w", command, err)
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if b.frames.generation() != gen && b.Position() != before {
			break
		}
		time.Sleep(4 * time.Millisecond)
	}
	return b.Position(), nil
}

func (b *mpvBackend) Buffered() float64 {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	pos := b.st.position
	buf := pos + b.st.cacheDur
	if b.st.cacheTime > buf {
		buf = b.st.cacheTime
	}
	if dur := b.st.duration; dur > 0 && buf > dur {
		buf = dur
	}
	if buf < pos {
		buf = pos
	}
	return buf
}

func (b *mpvBackend) Stalled() bool {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	return b.st.stalled
}

func (b *mpvBackend) AudioTracks() []playbackTrack {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	return append([]playbackTrack(nil), b.st.audioTracks...)
}

func (b *mpvBackend) VideoTracks() []playbackTrack {
	b.st.mu.Lock()
	defer b.st.mu.Unlock()
	return append([]playbackTrack(nil), b.st.videoTracks...)
}

func (b *mpvBackend) SelectAudio(id int) error { return b.selectMPVTrack("aid", id, true) }

func (b *mpvBackend) SelectVideo(id int) error { return b.selectMPVTrack("vid", id, false) }

func (b *mpvBackend) selectMPVTrack(prop string, id int, audio bool) error {
	var err error
	if !b.withCore(func(core *mpv.Mpv) { err = core.SetProperty(prop, mpv.FormatInt64, int64(id)) }) {
		return fmt.Errorf("mpv closed")
	}
	if err != nil {
		return err
	}
	// Show the choice now; the track-list event confirms it shortly.
	b.st.mu.Lock()
	tracks := b.st.videoTracks
	if audio {
		tracks = b.st.audioTracks
	}
	for i := range tracks {
		tracks[i].Selected = tracks[i].ID == id
	}
	b.st.mu.Unlock()
	return nil
}

func mpvParseTracks(list []any) (audio, video []playbackTrack) {
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
		selected := mpvBool(item["selected"])
		switch kind {
		case "audio":
			audioN++
			audio = append(audio, playbackTrack{ID: id, Label: mpvTrackLabel("音轨", audioN, item), Selected: selected})
		case "video":
			videoN++
			video = append(video, playbackTrack{ID: id, Label: mpvTrackLabel("画质", videoN, item), Selected: selected})
		}
	}
	return audio, video
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
