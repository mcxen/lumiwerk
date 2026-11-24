package main

import (
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Exiting large view to the thumbnail grid must park the playback backend for
// reuse on the next video rather than destroying and recreating libmpv.
func TestCloseLargeImageParksBackendForReuse(t *testing.T) {
	a, te, _ := navApp(t, navTree(t))
	stub := &stubBackend{name: "stub", path: a.Root + "/clip.mp4", playing: true}
	kernel := newPlaybackSession()
	kernel.backend = stub
	kernel.path = stub.path
	a.Video = &videoView{kernel: kernel, path: stub.path}
	a.InLargeView = true
	a.LargeIndex = 0
	te.Frame()

	a.closeLargeImage()
	if !stub.parked {
		t.Fatal("expected closeLargeImage to park playback backend instead of closing it")
	}
	if a.Video == nil || a.Video.kernel == nil || a.Video.kernel.backend != stub {
		t.Fatal("expected Video kernel and backend preserved after closeLargeImage")
	}

	// Reopening playback reuses the parked backend instance without probing a new one.
	origProbe := playbackProbe
	probeCalls := 0
	playbackProbe = func() (playbackBackend, error) {
		probeCalls++
		return &stubBackend{name: "cold"}, nil
	}
	t.Cleanup(func() { playbackProbe = origProbe })

	a.openPlayback(a.Root+"/next.mp4", 0, true)
	if probeCalls != 0 {
		t.Fatalf("playbackProbe called %d times, want 0 (reused parked backend)", probeCalls)
	}
	if stub.parked || stub.path != a.Root+"/next.mp4" {
		t.Fatalf("parked backend not reused: parked=%v path=%q", stub.parked, stub.path)
	}
}

func TestWarmPlaybackBackendAvoidsColdInitOnFirstOpen(t *testing.T) {
	origProbe := playbackProbe
	var probeCalls atomic.Int32
	stub := &stubBackend{name: "warmed"}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	playbackProbe = func() (playbackBackend, error) {
		probeCalls.Add(1)
		once.Do(func() { close(started) })
		<-release
		return stub, nil
	}
	t.Cleanup(func() { playbackProbe = origProbe })

	a := &appState{Chosen: -1, LargeIndex: -1}
	t.Cleanup(a.closeWarmedVideo)

	a.warmPlaybackBackend()
	<-started
	// Duplicate call while warming is in flight must not spawn a second probe.
	a.warmPlaybackBackend()
	close(release)

	a.openPlayback("/tmp/clip.mp4", 0, false)
	if got := probeCalls.Load(); got != 1 {
		t.Fatalf("playbackProbe calls = %d, want 1", got)
	}
	if a.Video == nil || a.Video.kernel == nil || a.Video.kernel.backend != stub {
		t.Fatal("first openPlayback did not take the pre-warmed backend")
	}
	if stub.path != "/tmp/clip.mp4" {
		t.Fatalf("warmed backend path = %q, want /tmp/clip.mp4", stub.path)
	}
}

func TestFFmpegOpenPopulatesFirstFrameAndResetsDuration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	twoSec := filepath.Join(dir, "two.mp4")
	oneSec := filepath.Join(dir, "one.mp4")
	for _, tc := range []struct {
		path string
		dur  string
		rate string
	}{
		{twoSec, "2", "10"},
		{oneSec, "1", "25"},
	} {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "testsrc=duration="+tc.dur+":size=96x64:rate="+tc.rate,
			"-c:v", "mpeg4", "-pix_fmt", "yuv420p", tc.path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create %s: %v\n%s", tc.path, err, out)
		}
	}

	// Verify pure-Go frame duration probing matches the authored container rates.
	if dur, ok := probeFrameDurationNative(oneSec); !ok || dur < 35*time.Millisecond || dur > 45*time.Millisecond {
		t.Fatalf("probeFrameDurationNative(%s) = %v, %v (want ~40ms)", oneSec, dur, ok)
	}

	b, err := newFFmpegBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if err := b.Open(twoSec, 0, false); err != nil {
		t.Fatal(err)
	}
	// Open decodes frame 0 synchronously on a single ffmpeg process.
	if b.Frame() == nil {
		t.Fatal("expected first frame populated immediately after Open")
	}
	if d := b.Duration(); d < 1.8 || d > 2.2 {
		t.Fatalf("two.mp4 duration = %v, want ~2s", d)
	}

	// Reopening a shorter file on the parked backend must reset duration.
	b.Park()
	if err := b.Open(oneSec, 0, false); err != nil {
		t.Fatal(err)
	}
	if b.Frame() == nil {
		t.Fatal("expected first frame populated after Park+Open")
	}
	if d := b.Duration(); d < 0.8 || d > 1.2 {
		t.Fatalf("one.mp4 duration after reuse = %v, want ~1s (stale duration kept?)", d)
	}
}
