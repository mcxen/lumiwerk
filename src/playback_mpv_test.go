//go:build mpvbackend

package main

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	mpv "github.com/gen2brain/go-mpv"
)

// mpvTestClip makes a short clip and points go-mpv at the bundled libmpv,
// or skips when either is unavailable.
func mpvTestClip(t *testing.T, rate int) string {
	t.Helper()
	dylib := filepath.Join("..", "resources", "darwin-universal")
	if _, err := os.Stat(filepath.Join(dylib, "libmpv.2.dylib")); err != nil {
		t.Skip("bundled libmpv not available")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	video := filepath.Join(t.TempDir(), "clip.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=320x180:rate=%d:duration=3", rate),
		"-pix_fmt", "yuv420p", "-c:v", "libx264", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	mpv.SetSearchPaths(func() []string { return []string{dylib} })
	return video
}

func TestMPVPauseStopsTheClock(t *testing.T) {
	video := mpvTestClip(t, 30)
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

// Frames are pushed by mpv's update callback and rendered off the caller's
// thread: Frame and the property getters must return at once, and the
// pushed frames must keep the video's own rate.
func TestMPVPushesFramesWithoutBlockingTheCaller(t *testing.T) {
	video := mpvTestClip(t, 60)
	backend, err := newMPVBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	mb := backend.(*mpvBackend)
	var pushed atomic.Int64
	mb.SetFrameNotify(func() { pushed.Add(1); mb.notifyPending.Store(false) })
	if err := backend.Open(video, 0, true); err != nil {
		t.Fatal(err)
	}
	var slowest time.Duration
	var frames int
	var last image.Image
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		start := time.Now()
		frame := backend.Frame()
		_ = backend.Position()
		_ = backend.Duration()
		_ = mb.Buffered()
		_ = mb.Stalled()
		_ = mb.AudioTracks()
		slowest = max(slowest, time.Since(start))
		if frame != nil && frame != last {
			frames++
			last = frame
			rgba, ok := frame.(*image.RGBA)
			if !ok || rgba.Stride != rgba.Rect.Dx()*4 {
				t.Fatalf("frame %T is not a tight RGBA; the view would copy it", frame)
			}
			if a := rgba.Pix[3]; a != 255 {
				t.Fatalf("alpha=%d, want opaque", a)
			}
		}
		time.Sleep(4 * time.Millisecond)
	}
	if slowest > 5*time.Millisecond {
		t.Fatalf("a getter call took %v; the UI thread must not wait for libmpv", slowest)
	}
	if n := pushed.Load(); n < 40 {
		t.Fatalf("only %d frames pushed in 1.5 s of 60 fps video", n)
	}
	if frames < 30 {
		t.Fatalf("caller saw only %d distinct frames", frames)
	}
	if d := backend.Duration(); d < 2.9 || d > 3.1 {
		t.Fatalf("cached duration=%.3f", d)
	}
	backend.SetPlaying(false)
	time.Sleep(100 * time.Millisecond)
	before := backend.Frame()
	at := backend.Position()
	stepped, err := backend.FrameStep(1)
	if err != nil {
		t.Fatal(err)
	}
	if stepped <= at {
		t.Fatalf("frame step position %.3f did not pass %.3f", stepped, at)
	}
	if after := backend.Frame(); after == before {
		t.Fatal("frame step did not publish the stepped frame before returning")
	}
}

// Reopening must not report the previous file's position or end.
func TestMPVOpenDropsThePreviousFilesState(t *testing.T) {
	video := mpvTestClip(t, 30)
	backend, err := newMPVBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.Open(video, 2.9, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && backend.Position() < 2.95 {
		time.Sleep(10 * time.Millisecond)
	}
	if err := backend.Open(video, 0, false); err != nil {
		t.Fatal(err)
	}
	if pos := backend.Position(); pos > 0.5 {
		t.Fatalf("position right after reopen=%.3f, still the previous file's", pos)
	}
}
