package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
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
}

func newFFmpegDecoder(path string) *ffmpegDecoder {
	return &ffmpegDecoder{path: path}
}

func newFFmpegDecoderAt(path string, startAt float64) *ffmpegDecoder {
	return &ffmpegDecoder{path: path, startAt: startAt}
}

func (d *ffmpegDecoder) start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.startLocked()
}

func (d *ffmpegDecoder) startLocked() error {
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
	args := playbackArgs(d.path, d.startAt, d.sourceFrame, d.plain)
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

// playbackArgs decodes through VideoToolbox into a capped JPEG stream.
// PNG frames were the browsing bottleneck; 30fps JPEG at the longer edge of
// 2560 keeps pan and zoom responsive without giving up a sharp pause frame.
func playbackArgs(path string, startAt float64, frameDur time.Duration, plain bool) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-threads", "0"}
	if !plain {
		args = append(args, "-hwaccel", "videotoolbox")
	}
	if startAt > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", startAt))
	}
	vf := "scale=w='min(2560,iw)':h='min(2560,ih)':force_original_aspect_ratio=decrease:flags=fast_bilinear"
	if frameDur > 0 && frameDur < time.Second/30 {
		vf = "fps=30," + vf
	}
	args = append(args, "-i", path, "-an", "-vf", vf, "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "3", "-")
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
	defer d.mu.Unlock()
	if d.closed || d.stdout == nil {
		return nil, io.EOF
	}
	return decodeFrame(d.stdout)
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

func (d *ffmpegDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	d.started = false
	return nil
}
