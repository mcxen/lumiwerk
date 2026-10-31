package main

import (
	"bufio"
	"fmt"
	"image"
	"image/png"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// ffmpegDecoder streams real video frames from ffmpeg into MyGo's VideoPlayer.
type ffmpegDecoder struct {
	path     string
	cmd      *exec.Cmd
	stdout   *bufio.Reader
	stderr   *strings.Builder
	mu       sync.Mutex
	started  bool
	closed   bool
	frameDur time.Duration
}

func newFFmpegDecoder(path string) *ffmpegDecoder {
	return &ffmpegDecoder{path: path, frameDur: 100 * time.Millisecond}
}

func (d *ffmpegDecoder) start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	d.stderr = &strings.Builder{}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", d.path, "-f", "image2pipe", "-vcodec", "png", "-")
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

// decodePNG reads one PNG stream object from ffmpeg's image2pipe output.
func decodePNG(r *bufio.Reader) (image.Image, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (d *ffmpegDecoder) Next() (ui.VideoFrame, error) {
	if err := d.start(); err != nil {
		return ui.VideoFrame{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ui.VideoFrame{}, io.EOF
	}
	img, err := decodePNG(d.stdout)
	if err != nil {
		return ui.VideoFrame{}, io.EOF
	}
	return ui.VideoFrame{Image: img, Duration: d.frameDur}, nil
}

func (d *ffmpegDecoder) Rewind() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
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
