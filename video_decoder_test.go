package main

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFFmpegDecoderRewindAndSeekRestartStreams(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "sample.mp4")
	makeSample := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=64x48:rate=10",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", path)
	if out, err := makeSample.CombinedOutput(); err != nil {
		t.Fatalf("create sample video: %v\n%s", err, out)
	}

	dec := newFFmpegDecoder(path)
	defer dec.Close()
	if _, err := dec.Next(); err != nil {
		t.Fatalf("decode first frame: %v", err)
	}

	restart := make(chan error, 1)
	go func() { restart <- dec.Rewind() }()
	select {
	case err := <-restart:
		if err != nil {
			t.Fatalf("rewind decoder: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("rewind decoder deadlocked")
	}
	if _, err := dec.Next(); err != nil {
		t.Fatalf("decode after rewind: %v", err)
	}

	go func() { restart <- dec.SeekTo(0.5) }()
	select {
	case err := <-restart:
		if err != nil {
			t.Fatalf("seek decoder: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("seek decoder deadlocked")
	}
	if _, err := dec.Next(); err != nil {
		t.Fatalf("decode after seek: %v", err)
	}
}
