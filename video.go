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

	"github.com/egoist/mygo/audio"
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
	quit        chan struct{}
}

// audio playback is owned by the view, not the frame decoder.

func newFFmpegDecoder(path string) *ffmpegDecoder {
	return newFFmpegDecoderAt(path, 0)
}

func newFFmpegDecoderAt(path string, startAt float64) *ffmpegDecoder {
	return &ffmpegDecoder{path: path, startAt: startAt, edge: 2560, quit: make(chan struct{})}
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
	if _, err := exec.LookPath("ffmpeg"); err != nil {
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
	args := playbackArgs(d.path, d.startAt, d.sourceFrame, d.plain, d.edge)
	cmd := exec.Command("ffmpeg", args...)
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
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
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
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
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

// soundtrack streams decoded audio from the shared playhead. The player
// consumes it at that rate; playback does not seek it to catch the picture.
type soundtrack struct {
	path    string
	base    float64
	cmd     *exec.Cmd
	stdout  io.ReadCloser
	pending []byte
	mu      sync.Mutex
	closed  bool
}

var errSoundtrackLate = errors.New("soundtrack late")

func newSoundtrack(path string, at float64) (*soundtrack, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, err
	}
	if at < 0 {
		at = 0
	}
	return &soundtrack{path: path, base: at}, nil
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
	cmd := exec.Command("ffmpeg", soundtrackArgs(d.path, at)...)
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
	if len(d.pending) >= 8 {
		n := len(d.pending)
		if n > need*4 {
			n = need * 4
		}
		n -= n % 8
		raw := append([]byte(nil), d.pending[:n]...)
		d.pending = d.pending[n:]
		d.mu.Unlock()
		return decodeSamples(buf, raw), nil
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
		return decodeSamples(buf, raw[:n]), nil
	}
	if err == nil || err == io.EOF || err == io.ErrUnexpectedEOF {
		return 0, io.EOF
	}
	return 0, err
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

func (a *appState) stopAudio() {
	if a.Video == nil || a.Video.sound == nil {
		return
	}
	a.Video.sound.Pause()
	a.Video.sound.Close()
	a.Video.sound = nil
}

func (a *appState) shutdownVideo() {
	a.audioGen++
	if a.Video == nil {
		return
	}
	a.stopAudio()
	if a.Video.dec != nil {
		_ = a.Video.dec.Close()
		a.Video.dec = nil
	}
	a.Video.frames = nil
	a.Video.held = nil
}

func (a *appState) closeVideo() {
	a.shutdownVideo()
	a.Video = nil
}

// syncAudio starts the soundtrack at the shared playhead, or stops it.
// The clock itself stays put until the first samples are ready.
func (a *appState) syncAudio(playing bool) {
	a.audioGen++
	gen := a.audioGen
	a.stopAudio()
	if !playing || a.Video == nil || a.Video.path == "" {
		return
	}
	a.armSoundtrack(gen)
}

func (a *appState) armSoundtrack(gen int) {
	if a.Video == nil {
		return
	}
	path := a.Video.path
	at := soundtrackAt(a.Video.clock)
	safeGo("video.go:655", func() {
		dec, err := newSoundtrack(path, at)
		if err == nil {
			err = dec.prime()
		}
		if err != nil && dec != nil {
			_ = dec.Close()
			dec = nil
		}
		publish := func() {
			if a.audioGen != gen || a.Video == nil || !a.Video.playing {
				if dec != nil {
					_ = dec.Close()
				}
				return
			}
			now := time.Now()
			if dec == nil {
				a.Video.clock.play(now)
				return
			}
			player := audio.NewPlayer(dec)
			player.Play()
			a.Video.sound = player
			a.Video.clock.origin = at
			a.Video.clock.wall = time.Time{}
			a.Video.clock.play(now)
		}
		if a.Window == nil {
			publish()
			return
		}
		a.update(publish)
	})
}

func (a *appState) videoPlaying() bool {
	return a.Video != nil && a.Video.playing
}

func (a *appState) playhead() float64 {
	if a.Video == nil {
		return 0
	}
	t := a.Video.clock.at(time.Now())
	if limit, ok := a.knownDuration(); ok && t > limit {
		t = limit
	}
	if t < 0 {
		t = 0
	}
	a.Video.Position = t
	return t
}

func (a *appState) holdVideo(sharp bool) {
	if a.Video == nil || !a.Video.playing {
		return
	}
	a.Video.clock.pause(time.Now())
	a.Video.playing = false
	a.Video.Position = a.Video.clock.origin
	a.syncAudio(false)
	if sharp {
		a.requestSharpFrame()
	}
}

func (a *appState) playVideo() {
	if a.Video == nil || a.Video.playing {
		return
	}
	if a.Video.dec != nil && a.Video.dec.edge != playbackEdge(a.Zoom) {
		at := a.Video.clock.origin
		path := a.Video.path
		a.openPlayback(path, at, true)
		return
	}
	a.Video.Still = nil
	a.Video.playing = true
	a.syncAudio(true)
}

func shouldRestartVideoEdge(decodedEdge int, zoom float32, due, now time.Time) bool {
	if due.IsZero() || now.Before(due) {
		return false
	}
	return decodedEdge != playbackEdge(zoom)
}

func (a *appState) noteVideoZoom() {
	if a.Video == nil || a.Video.dec == nil || !a.Video.playing {
		return
	}
	if a.Video.dec.edge == playbackEdge(a.Zoom) {
		a.videoEdgeDue = time.Time{}
		return
	}
	a.videoEdgeDue = time.Now().Add(zoomSettle)
}

func (a *appState) openPlayback(path string, at float64, playing bool) {
	if path == "" {
		return
	}
	a.videoEdgeDue = time.Time{}
	if at < 0 {
		at = 0
	}
	a.shutdownVideo()
	dec := newFFmpegDecoderAt(path, at)
	dec.edge = playbackEdge(a.Zoom)
	a.Video = &videoView{
		dec:      dec,
		path:     path,
		frames:   startFramePump(dec),
		playing:  playing,
		clock:    mediaClock{origin: at},
		Position: at,
	}
	if playing {
		a.syncAudio(true)
		return
	}
	a.requestSharpFrame()
}

func (a *appState) takeDueFrames(now float64) {
	v := a.Video
	if v == nil {
		return
	}
	var batch []timedFrame
	if v.held != nil {
		batch = append(batch, *v.held)
		v.held = nil
	}
	if v.frames != nil {
		for {
			select {
			case frame := <-v.frames:
				batch = append(batch, frame)
			default:
				goto picked
			}
		}
	}
picked:
	show, ok, ahead, hasAhead := shownFrame(batch, now)
	if hasAhead {
		held := ahead
		v.held = &held
	}
	if !ok {
		return
	}
	if show.err != nil {
		if v.frame == nil && !errors.Is(show.err, io.EOF) {
			v.err = show.err
			logWarnf("video decode failed for %s: %v", v.path, show.err)
		}
		if v.playing {
			a.holdVideo(v.frame != nil)
		}
		return
	}
	if show.img == nil {
		return
	}
	if v.frame == nil {
		v.frame = ui.NewBitmap(show.img)
	} else {
		v.frame.Update(show.img)
	}
	v.frameAt = show.at
}

// presentVideo shows the frame the shared clock says is due.
func (a *appState) presentVideo(c *ui.Context) *ui.Bitmap {
	v := a.Video
	if v == nil {
		return nil
	}
	now := time.Now()
	t := v.clock.at(now)
	if limit, ok := a.knownDuration(); ok && v.playing && t >= limit-0.01 {
		v.clock.seek(limit, false, now)
		v.playing = false
		v.Position = limit
		a.syncAudio(false)
		a.requestSharpFrame()
		t = limit
	} else {
		v.Position = t
	}
	a.takeDueFrames(t)
	if v.playing {
		wait := 16 * time.Millisecond
		if v.held != nil {
			due := time.Duration((v.held.at - t) * float64(time.Second))
			if due > time.Millisecond && due < wait {
				wait = due
			}
		}
		c.After(wait)
	}
	return v.frame
}

func (a *appState) videoLayer(c *ui.Context, transformed bool) {
	if a.Video == nil {
		ui.Spinner(c)
		return
	}
	var pic *ui.Bitmap
	if a.Video.Still != nil && !a.Video.playing {
		pic = a.Video.Still
	} else {
		pic = a.presentVideo(c)
	}
	if pic == nil {
		if a.Video.err != nil {
			ui.Textf(c, "视频解码失败: %v", a.Video.err).TextColor(c.Theme().Danger)
			return
		}
		ui.Spinner(c)
		return
	}
	image := ui.Image(c, pic).Fit(ui.Contain).Grow(1)
	if transformed {
		image.Transform(a.transform())
	}
}
