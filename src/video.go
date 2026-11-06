package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// ffmpegDecoder streams real video frames from ffmpeg into MyGo's VideoPlayer.
type ffmpegDecoder struct {
	path        string
	cmd         *exec.Cmd
	stdout      *bufio.Reader
	stderr      *strings.Builder
	mu          sync.Mutex
	started     bool
	closed      bool
	frameDur    time.Duration
	sourceFrame time.Duration
	startAt     float64
	plain       bool
	edge        int
	audioMap    int
	videoMap    int
	quit        chan struct{}
}

// audio playback is owned by the view, not the frame decoder.

func newFFmpegDecoder(path string) *ffmpegDecoder {
	return newFFmpegDecoderAt(path, 0)
}

func newFFmpegDecoderAt(path string, startAt float64) *ffmpegDecoder {
	return &ffmpegDecoder{path: path, startAt: startAt, edge: 2560, audioMap: -1, videoMap: -1, quit: make(chan struct{})}
}

func (d *ffmpegDecoder) start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.startLocked()
}

func (d *ffmpegDecoder) startLocked() error {
	if d.closed {
		return io.EOF
	}
	if d.started {
		return nil
	}
	if _, err := lookTool("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	if d.sourceFrame <= 0 {
		d.sourceFrame = probeFrameDuration(d.path)
	}
	d.frameDur = d.sourceFrame
	if d.frameDur > 0 && d.frameDur < time.Second/30 {
		d.frameDur = time.Second / 30
	}
	d.stderr = &strings.Builder{}
	args := playbackArgsMapped(d.path, d.startAt, d.sourceFrame, d.plain, d.edge, d.audioMap, d.videoMap)
	cmd := exec.Command(toolPath("ffmpeg"), args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = d.stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	d.cmd = cmd
	d.stdout = bufio.NewReaderSize(stdout, 1<<20)
	d.started = true
	return nil
}

func probeFrameDuration(path string) time.Duration {
	const fallback = 100 * time.Millisecond
	out, err := exec.Command(toolPath("ffprobe"), "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=avg_frame_rate,r_frame_rate",
		"-of", "default=noprint_wrappers=1", path).Output()
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		var fps float64
		if numerator, denominator, found := strings.Cut(value, "/"); found {
			n, nErr := strconv.ParseFloat(numerator, 64)
			d, dErr := strconv.ParseFloat(denominator, 64)
			if nErr != nil || dErr != nil || d == 0 {
				continue
			}
			fps = n / d
		} else {
			fps, err = strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
		}
		if key == "avg_frame_rate" && fps > 0 && fps <= 240 {
			return time.Duration(float64(time.Second) / fps)
		}
		if key == "r_frame_rate" && fps > 0 && fps <= 240 {
			return time.Duration(float64(time.Second) / fps)
		}
	}
	return fallback
}

// playbackEdge raises the decode size as the user zooms past fit. Fit stays
// at 2560; deeper zoom gets a sharper frame instead of stretching that JPEG.
func playbackEdge(zoom float32) int {
	if zoom < 1 {
		zoom = 1
	}
	switch {
	case zoom <= 1.25:
		return 2560
	case zoom <= 2.5:
		return 3840
	default:
		return 5120
	}
}

// playbackArgs decodes through VideoToolbox into a JPEG stream. Lanczos keeps
// zoomed playback sharper than a fast bilinear scale of a tiny frame.
func playbackArgs(path string, startAt float64, frameDur time.Duration, plain bool, edge int) []string {
	if edge <= 0 {
		edge = 2560
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-threads", "0"}
	if !plain {
		args = append(args, "-hwaccel", "videotoolbox")
	}
	if startAt > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", startAt))
	}
	vf := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:flags=lanczos", edge, edge)
	if frameDur > 0 && frameDur < time.Second/30 {
		vf = "fps=30," + vf
	}
	args = append(args, "-i", path, "-an", "-vf", vf, "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "2", "-")
	return args
}

// playbackArgsMapped is playbackArgs plus optional stream picks. A negative
// map leaves that stream on ffmpeg's default, so the unmapped command line
// stays byte-for-byte the same.
func playbackArgsMapped(path string, startAt float64, frameDur time.Duration, plain bool, edge, audioMap, videoMap int) []string {
	return insertStreamMaps(playbackArgs(path, startAt, frameDur, plain, edge), path, audioMap, videoMap)
}

// insertStreamMaps places -map after the matching -i. Ordinals are among
// streams of that type (0:v:N, 0:a:N), and the audio map is optional.
func insertStreamMaps(args []string, path string, audioMap, videoMap int) []string {
	if audioMap < 0 && videoMap < 0 {
		return args
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-i" || args[i+1] != path {
			continue
		}
		var maps []string
		if videoMap >= 0 {
			maps = append(maps, "-map", fmt.Sprintf("0:v:%d", videoMap))
		}
		if audioMap >= 0 {
			maps = append(maps, "-map", fmt.Sprintf("0:a:%d?", audioMap))
		}
		out := append([]string{}, args[:i+2]...)
		out = append(out, maps...)
		return append(out, args[i+2:]...)
	}
	return args
}

func decodeFrame(r *bufio.Reader) (image.Image, error) {
	img, err := jpeg.Decode(r)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// grabVideoStill extracts one high-quality frame at the paused timestamp.
func grabVideoStill(path string, at float64) (image.Image, error) {
	if at < 0 {
		at = 0
	}
	cmd := exec.Command(toolPath("ffmpeg"), "-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.3f", at), "-i", path,
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "2", "-")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return jpeg.Decode(bytes.NewReader(out))
}

func (d *ffmpegDecoder) Next() (ui.VideoFrame, error) {
	if err := d.start(); err != nil {
		return ui.VideoFrame{}, err
	}
	img, err := d.readFrame()
	if err == nil {
		return ui.VideoFrame{Image: img, Duration: d.frameDuration()}, nil
	}
	// VideoToolbox rejects some files. Retry once with a software decoder.
	if !d.plain {
		d.mu.Lock()
		d.plain = true
		at := d.startAt
		closed := d.closed
		d.mu.Unlock()
		if !closed {
			if seekErr := d.SeekTo(at); seekErr == nil {
				if img, err = d.readFrame(); err == nil {
					return ui.VideoFrame{Image: img, Duration: d.frameDuration()}, nil
				}
			}
		}
	}
	return ui.VideoFrame{}, io.EOF
}

func (d *ffmpegDecoder) frameDuration() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.frameDur <= 0 {
		return 100 * time.Millisecond
	}
	return d.frameDur
}

func (d *ffmpegDecoder) readFrame() (image.Image, error) {
	d.mu.Lock()
	if d.closed || d.stdout == nil {
		d.mu.Unlock()
		return nil, io.EOF
	}
	stdout := d.stdout
	d.mu.Unlock()
	// Decode outside the lock so Close can kill ffmpeg while a frame is in flight.
	return decodeFrame(stdout)
}

func (d *ffmpegDecoder) Rewind() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("decoder closed")
	}
	if d.cmd != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	d.started = false
	d.cmd = nil
	d.stdout = nil
	d.stderr = nil
	d.mu.Unlock()
	return d.start()
}

// SeekTo restarts decoding at a timestamp so playback follows real media time.
func (d *ffmpegDecoder) SeekTo(seconds float64) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("decoder closed")
	}
	if seconds < 0 {
		seconds = 0
	}
	if d.cmd != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	d.startAt = seconds
	d.started = false
	d.cmd = nil
	d.stdout = nil
	d.stderr = nil
	d.mu.Unlock()
	return d.start()
}

func (d *ffmpegDecoder) signalLocked() {
	if d.quit == nil {
		return
	}
	select {
	case <-d.quit:
	default:
		close(d.quit)
	}
}

func (d *ffmpegDecoder) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.signalLocked()
	cmd := d.cmd
	d.started = false
	d.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return nil
}

const soundtrackRate = 48000

// mediaClock is the only playhead. The picture shows the frame due at this
// time, and the soundtrack is started at the same origin. It is not a second
// clock, and nothing seeks the sound back onto the picture later.
type mediaClock struct {
	origin float64
	wall   time.Time
}

func (c mediaClock) at(now time.Time) float64 {
	if c.wall.IsZero() {
		return c.origin
	}
	return c.origin + now.Sub(c.wall).Seconds()
}

func (c *mediaClock) play(now time.Time) {
	if c.wall.IsZero() {
		c.wall = now
	}
}

func (c *mediaClock) pause(now time.Time) {
	c.origin = c.at(now)
	c.wall = time.Time{}
}

func (c *mediaClock) seek(at float64, playing bool, now time.Time) {
	if at < 0 {
		at = 0
	}
	c.origin = at
	if playing {
		c.wall = now
		return
	}
	c.wall = time.Time{}
}

// soundtrackAt is the media time a new soundtrack must join. It is the clock
// origin, never a corrected audio position.
func soundtrackAt(c mediaClock) float64 { return c.origin }

// timedFrame is one decoded picture and the media time it belongs to.
type timedFrame struct {
	img image.Image
	at  float64
	err error
}

// shownFrame keeps the newest frame at or before now and the first frame
// that is still ahead of the clock.
func shownFrame(frames []timedFrame, now float64) (show timedFrame, ok bool, ahead timedFrame, hasAhead bool) {
	const lead = 0.0005
	for _, frame := range frames {
		if frame.err != nil {
			if ok {
				return show, true, timedFrame{}, false
			}
			return frame, true, timedFrame{}, false
		}
		if frame.at <= now+lead {
			show = frame
			ok = true
			continue
		}
		return show, ok, frame, true
	}
	return show, ok, timedFrame{}, false
}

func startFramePump(d *ffmpegDecoder) chan timedFrame {
	frames := make(chan timedFrame, 6)
	safeGo("video.framePump", func() { d.runPump(frames) })
	return frames
}

func (d *ffmpegDecoder) runPump(frames chan<- timedFrame) {
	at := d.startAt
	if at < 0 {
		at = 0
	}
	for {
		select {
		case <-d.quit:
			return
		default:
		}
		frame, err := d.Next()
		if err != nil {
			select {
			case frames <- timedFrame{err: err}:
			case <-d.quit:
			}
			return
		}
		dur := frame.Duration.Seconds()
		if dur <= 0 {
			dur = 0.033
		}
		select {
		case frames <- timedFrame{img: frame.Image, at: at}:
		case <-d.quit:
			return
		}
		at += dur
	}
}

// soundtrackArgs extracts float32 stereo from the same timestamp as the picture.
func soundtrackArgs(path string, at float64) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if at > 0.05 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", at))
	}
	return append(args, "-i", path, "-vn", "-ac", "2", "-ar", strconv.Itoa(soundtrackRate), "-f", "f32le", "-")
}

func soundtrackArgsMapped(path string, at float64, audioMap, videoMap int) []string {
	return insertStreamMaps(soundtrackArgs(path, at), path, audioMap, videoMap)
}

// soundtrack streams decoded audio from the shared playhead. The player
// consumes it at that rate; playback does not seek it to catch the picture.
type soundtrack struct {
	path     string
	base     float64
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	pending  []byte
	speed    float64
	volume   float64
	audioMap int
	videoMap int
	mu       sync.Mutex
	closed   bool
}

var errSoundtrackLate = errors.New("soundtrack late")

func newSoundtrackRate(path string, at, speed float64) (*soundtrack, error) {
	if _, err := lookTool("ffmpeg"); err != nil {
		return nil, err
	}
	if at < 0 {
		at = 0
	}
	return &soundtrack{path: path, base: at, speed: normalizedPlaybackSpeed(speed), volume: 1, audioMap: -1, videoMap: -1}, nil
}

func newSoundtrack(path string, at float64) (*soundtrack, error) {
	return newSoundtrackRate(path, at, 1)
}

func (d *soundtrack) SetVolume(volume float64) {
	d.mu.Lock()
	d.volume = normalizedPlaybackVolume(volume)
	d.mu.Unlock()
}

func (d *soundtrack) SampleRate() int { return soundtrackRate }
func (d *soundtrack) Channels() int   { return 2 }
func (d *soundtrack) Len() int64      { return -1 }

func (d *soundtrack) startLocked(at float64) error {
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	if d.stdout != nil {
		_ = d.stdout.Close()
		d.stdout = nil
	}
	d.pending = nil
	cmd := exec.Command(toolPath("ffmpeg"), soundtrackArgsMapped(d.path, at, d.audioMap, d.videoMap)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	d.cmd = cmd
	d.stdout = stdout
	return nil
}

// prime waits until the first samples exist, so the playhead and the sound
// can start together instead of the picture running ahead of ffmpeg.
func (d *soundtrack) prime() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return io.EOF
	}
	if err := d.startLocked(d.base); err != nil {
		d.mu.Unlock()
		return err
	}
	stdout := d.stdout
	d.mu.Unlock()
	buf := make([]byte, 8192)
	type got struct {
		n   int
		err error
	}
	done := make(chan got, 1)
	safeGo("video.go:495", func() {
		n, err := io.ReadFull(stdout, buf)
		done <- got{n, err}
	})
	select {
	case res := <-done:
		if res.n < 8 {
			if res.err == nil {
				res.err = io.ErrUnexpectedEOF
			}
			return res.err
		}
		res.n -= res.n % 8
		d.mu.Lock()
		d.pending = append([]byte(nil), buf[:res.n]...)
		d.mu.Unlock()
		return nil
	case <-time.After(800 * time.Millisecond):
		return errSoundtrackLate
	}
}

func decodeSamples(buf []float32, raw []byte) int {
	n := len(raw) - len(raw)%8
	samples := n / 4
	if samples > len(buf) {
		samples = len(buf) - len(buf)%2
		n = samples * 4
	}
	if samples < 2 {
		return 0
	}
	for i := 0; i < samples; i++ {
		buf[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4 : i*4+4]))
	}
	return samples
}

func (d *soundtrack) Read(buf []float32) (int, error) {
	if len(buf) < 2 {
		return 0, nil
	}
	need := (len(buf) / 2) * 2
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return 0, io.EOF
	}
	volume := d.volume
	if len(d.pending) >= 8 {
		n := len(d.pending)
		if n > need*4 {
			n = need * 4
		}
		n -= n % 8
		raw := append([]byte(nil), d.pending[:n]...)
		d.pending = d.pending[n:]
		d.mu.Unlock()
		return scaleSamples(buf, decodeSamples(buf, raw), volume), nil
	}
	if d.stdout == nil {
		if err := d.startLocked(d.base); err != nil {
			d.mu.Unlock()
			return 0, err
		}
	}
	stdout := d.stdout
	d.mu.Unlock()
	raw := make([]byte, need*4)
	n, err := io.ReadFull(stdout, raw)
	n -= n % 8
	if n >= 8 {
		return scaleSamples(buf, decodeSamples(buf, raw[:n]), volume), nil
	}
	if err == nil || err == io.EOF || err == io.ErrUnexpectedEOF {
		return 0, io.EOF
	}
	return 0, err
}

func scaleSamples(buf []float32, n int, volume float64) int {
	if volume != 1 {
		for i := 0; i < n; i++ {
			buf[i] = float32(float64(buf[i]) * volume)
		}
	}
	return n
}

func (d *soundtrack) SetPosition(frame int64) error {
	if frame < 0 {
		frame = 0
	}
	at := d.base + float64(frame)/float64(soundtrackRate)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return io.EOF
	}
	return d.startLocked(at)
}

func (d *soundtrack) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	cmd := d.cmd
	stdout := d.stdout
	d.stdout = nil
	d.pending = nil
	d.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	if stdout != nil {
		_ = stdout.Close()
	}
	return nil
}

func (a *appState) shutdownVideo() {
	if a.Video != nil && a.Video.kernel != nil {
		a.Video.kernel.Close()
	}
}

func (a *appState) closeVideo() {
	a.flushPlayProgress()
	a.shutdownVideo()
	a.Video = nil
	a.playerEndSeen = false
}

func (a *appState) videoSnapshot() playbackSnapshot {
	if a.Video == nil || a.Video.kernel == nil {
		return playbackSnapshot{}
	}
	return a.Video.kernel.Snapshot()
}

func (a *appState) videoPlaying() bool {
	return a.Video != nil && a.Video.kernel != nil && a.videoSnapshot().Playing
}

func (a *appState) playhead() float64 {
	return a.videoSnapshot().Position
}

func (a *appState) holdVideo(sharp bool) {
	if a.Video == nil || a.Video.kernel == nil {
		return
	}
	a.Video.kernel.SetPlaying(false)
	if sharp {
		a.requestSharpFrame()
	}
}

func (a *appState) playVideo() {
	if a.Video == nil || a.Video.kernel == nil {
		return
	}
	a.Video.Still = nil
	a.Video.err = nil
	a.Video.kernel.SetPlaying(true)
}

func (a *appState) stepVideo(direction int) {
	if a.Video == nil || a.Video.kernel == nil {
		return
	}
	a.Video.Still = nil
	if err := a.Video.kernel.Step(direction); err != nil {
		a.toast("逐帧播放不可用")
	}
}

func shouldRestartVideoEdge(decodedEdge int, zoom float32, due, now time.Time) bool {
	if due.IsZero() || now.Before(due) {
		return false
	}
	return decodedEdge != playbackEdge(zoom)
}

func (a *appState) noteVideoZoom() {
	if a.Video == nil || a.Video.kernel == nil || !a.videoPlaying() {
		return
	}
	backend, ok := a.Video.kernel.backend.(*ffmpegBackend)
	if !ok {
		return
	}
	backend.mu.Lock()
	decodedEdge := backend.edge
	backend.mu.Unlock()
	if decodedEdge == playbackEdge(a.Zoom) {
		a.videoEdgeDue = time.Time{}
		return
	}
	a.videoEdgeDue = time.Now().Add(zoomSettle)
}


// flushPlayProgress writes the current playhead for the open video, then
// persists when dirty. Safe when nothing is playing.
func (a *appState) flushPlayProgress() {
	if a.Video == nil || a.Video.path == "" || a.Video.kernel == nil {
		return
	}
	if !a.Profiles.rememberPlay() {
		return
	}
	snap := a.videoSnapshot()
	playProgress.Remember(a.Video.path, snap.Position, snap.Duration)
	playProgress.Flush()
}

// maybeAutosavePlayProgress records the playhead every few seconds while
// a video is on screen, so a crash still keeps recent progress.
func (a *appState) maybeAutosavePlayProgress() {
	if a.Video == nil || a.Video.path == "" || a.Video.kernel == nil {
		return
	}
	if !a.Profiles.rememberPlay() {
		return
	}
	snap := a.videoSnapshot()
	if snap.Duration <= 0 {
		return
	}
	playProgress.Remember(a.Video.path, snap.Position, snap.Duration)
	playProgress.FlushSoon()
}

// resumePlayAt is the stored offset for path when resume is enabled.
func (a *appState) resumePlayAt(path string) float64 {
	if path == "" || !a.Profiles.rememberPlay() {
		return 0
	}
	at, ok := playProgress.Lookup(path)
	if !ok {
		return 0
	}
	return at
}

func (a *appState) openPlayback(path string, at float64, playing bool) {
	if path == "" {
		return
	}
	// Save the previous clip before retargeting the backend.
	if a.Video != nil && a.Video.path != "" && pathKey(a.Video.path) != pathKey(path) {
		a.flushPlayProgress()
	}
	if at == 0 {
		at = a.resumePlayAt(path)
	}
	a.videoEdgeDue = time.Time{}
	a.playerMoved = time.Now()
	a.playerPtrSeen = false
	a.playerEndSeen = false
	a.playerSkipClick = false
	volume := a.VideoVolume
	if volume < 0 {
		volume = 0
	}
	// Keep one libmpv/FFmpeg backend across image↔video and video↔video
	// switches: tearing it down and recreating the render thread is what
	// made stepping through a mixed folder feel janky.
	if a.Video != nil && a.Video.kernel != nil {
		a.Video.path = path
		a.Video.frame = nil
		a.Video.Still = nil
		a.Video.err = nil
		a.Video.kernel.SetVolume(volume)
		if err := a.Video.kernel.Open(path, at, playing); err != nil {
			a.Video.err = err
			logErrorf("video.open: path=%s err=%v", path, err)
			return
		}
		if edgeBackend, ok := a.Video.kernel.backend.(*ffmpegBackend); ok {
			edgeBackend.SetRenderEdge(playbackEdge(a.Zoom))
			edgeBackend.SetNotify(func() { a.update(func() {}) })
		}
		if !playing {
			a.requestSharpFrame()
		}
		return
	}
	kernel := newPlaybackSession()
	kernel.SetVolume(volume)
	v := &videoView{kernel: kernel, path: path}
	a.Video = v
	if err := kernel.Open(path, at, playing); err != nil {
		v.err = err
		logErrorf("video.open: path=%s err=%v", path, err)
		return
	}
	if edgeBackend, ok := kernel.backend.(*ffmpegBackend); ok {
		edgeBackend.SetRenderEdge(playbackEdge(a.Zoom))
		edgeBackend.SetNotify(func() {
			a.update(func() {})
		})
	}
	if !playing {
		a.requestSharpFrame()
	}
}

// parkVideo pauses and unloads the current file but keeps the playback
// backend alive for the next video. Used when the large view shows an image.
func (a *appState) parkVideo() {
	if a.Video == nil || a.Video.kernel == nil {
		return
	}
	a.flushPlayProgress()
	a.Video.kernel.Park()
	a.Video.path = ""
	a.Video.frame = nil
	a.Video.Still = nil
	a.Video.err = nil
	a.playerEndSeen = false
}

func (a *appState) presentVideo(c *ui.Context, snapshot playbackSnapshot) *ui.Bitmap {
	v := a.Video
	if v == nil {
		return nil
	}
	a.maybeAutosavePlayProgress()
	v.err = snapshot.Err
	switch snapshot.Phase {
	case playLoading, playPlaying, playBuffering:
		c.After(33 * time.Millisecond)
	default:
		if snapshot.Seeking || snapshot.Scrubbing {
			c.After(33 * time.Millisecond)
		}
	}
	if snapshot.Frame == nil {
		return v.frame
	}
	if v.frame == nil {
		v.frame = ui.NewBitmap(snapshot.Frame)
	} else {
		v.frame.Update(snapshot.Frame)
	}
	return v.frame
}

func (a *appState) videoLayer(c *ui.Context, transformed bool) {
	if a.Video == nil {
		ui.Spinner(c)
		return
	}
	snapshot := a.videoSnapshot()
	var pic *ui.Bitmap
	frozen := snapshot.Phase == playPaused && !snapshot.Scrubbing && !snapshot.Seeking
	if a.Video.Still != nil && frozen {
		pic = a.Video.Still
	} else {
		pic = a.presentVideo(c, snapshot)
	}
	if pic == nil {
		if snapshot.Err != nil || snapshot.Phase == playError {
			if snapshot.Err != nil {
				ui.Textf(c, "视频解码失败: %v", snapshot.Err).TextColor(c.Theme().Danger)
				return
			}
			ui.Text(c, "视频解码失败").TextColor(c.Theme().Danger)
			return
		}
		// Show the grid thumbnail immediately while libmpv warms up the
		// first frame — avoids a blank stage on image→video switches.
		if item := a.currentItem(); item != nil && item.Bitmap != nil {
			img := ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1)
			if transformed {
				img.Transform(a.transform())
			}
			ui.Row(c).Attach(ui.AnchorCenter, ui.AnchorBottom).Gap(8).
				Padding(8, 12).Radius(8).Background(ui.RGBA(0, 0, 0, 0.55)).AlignItems(ui.Center).
				Children(func() {
					ui.Spinner(c)
					label := "正在加载"
					if snapshot.Phase == playBuffering {
						label = "正在缓冲"
					}
					ui.Text(c, label).FontSize(12).TextColor(ui.RGB(255, 255, 255))
				})
			return
		}
		ui.Column(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Spinner(c)
			label := "正在加载"
			if snapshot.Phase == playBuffering {
				label = "正在缓冲"
			}
			ui.Text(c, label).FontSize(13).TextColor(c.Theme().TextMuted)
		})
		return
	}
	ui.Box(c).Grow(1).Children(func() {
		image := ui.Image(c, pic).Fit(ui.Contain).Grow(1)
		if transformed {
			image.Transform(a.transform())
		}
		if label := playbackStatusLabel(snapshot); label != "" {
			badge := ui.Row(c).Attach(ui.AnchorCenter, ui.AnchorCenter).Gap(8).
				Padding(8, 12).Radius(8).Background(ui.RGBA(0, 0, 0, 0.62)).AlignItems(ui.Center)
			badge.Children(func() {
				ui.Spinner(c)
				ui.Text(c, label).FontSize(13).TextColor(ui.RGB(245, 245, 247))
			})
		}
	})
}
