//go:build mpvbackend

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mpv "github.com/gen2brain/go-mpv"
)

func TestMPVPauseStopsTheClock(t *testing.T) {
	dylib := filepath.Join("..", "resources", "darwin-universal")
	if _, err := os.Stat(filepath.Join(dylib, "libmpv.2.dylib")); err != nil {
		t.Skip("bundled libmpv not available")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=30:duration=3",
		"-pix_fmt", "yuv420p", "-c:v", "libx264", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	mpv.SetSearchPaths(func() []string { return []string{dylib} })

	backend, err := newMPVBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.Open(video, 0, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var pos float64
	for time.Now().Before(deadline) {
		_ = backend.Frame()
		pos = backend.Position()
		if pos > 0.2 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if pos <= 0.2 {
		t.Fatalf("playback did not start, pos=%.3f", pos)
	}
	backend.SetPlaying(false)
	time.Sleep(350 * time.Millisecond)
	held := backend.Position()
	_ = backend.Frame()
	time.Sleep(350 * time.Millisecond)
	later := backend.Position()
	if later > held+0.12 {
		t.Fatalf("pause did not stop the clock: %.3f -> %.3f (started at %.3f)", held, later, pos)
	}
}
