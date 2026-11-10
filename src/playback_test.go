package main

import (
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type fakePlaybackBackend struct {
	path       string
	position   float64
	duration   float64
	playing    bool
	volume     float64
	speed      float64
	frame      image.Image
	opens      int
	closes     int
	seeks      []float64
	steps      []int
	playStates []bool
}

func (b *fakePlaybackBackend) Name() string { return "test" }
func (b *fakePlaybackBackend) Open(path string, at float64, playing bool) error {
	b.path, b.position, b.playing = path, at, playing
	b.opens++
	return nil
}
func (b *fakePlaybackBackend) Close() error { b.closes++; return nil }
func (b *fakePlaybackBackend) Frame() image.Image {
	if b.frame == nil {
		b.frame = image.NewNRGBA(image.Rect(0, 0, 16, 9))
	}
	return b.frame
}
func (b *fakePlaybackBackend) Position() float64 { return b.position }
func (b *fakePlaybackBackend) Duration() float64 { return b.duration }
func (b *fakePlaybackBackend) SetPlaying(playing bool) {
	b.playing = playing
	b.playStates = append(b.playStates, playing)
}
func (b *fakePlaybackBackend) Seek(at float64) {
	b.position = at
	b.seeks = append(b.seeks, at)
}
func (b *fakePlaybackBackend) SetVolume(volume float64) { b.volume = volume }
func (b *fakePlaybackBackend) SetSpeed(speed float64)   { b.speed = speed }
func (b *fakePlaybackBackend) FrameStep(direction int) (float64, error) {
	b.steps = append(b.steps, direction)
	b.position = clampPlaybackTime(b.position+float64(direction)/30, b.duration)
	return b.position, nil
}

func fakeVideoState(backend playbackBackend) *appState {
	kernel := newPlaybackSession()
	kernel.backend = backend
	kernel.path = "/tmp/movie.mp4"
	kernel.playing = backend.(*fakePlaybackBackend).playing
	kernel.duration = 120
	kernel.SetVolume(0.7)
	return &appState{
		Video: &videoView{kernel: kernel, path: "/tmp/movie.mp4"},
		Items: []mediaItem{
			{Path: "/tmp/previous.jpg", Name: "previous.jpg", IsVideo: false, Decoded: true},
			{Path: "/tmp/movie.mp4", Name: "movie.mp4", Ext: "mp4", IsVideo: true, Decoded: true,
				Info: mediaInfo{Width: 1920, Height: 1080, DurationSec: 120}, InfoLoaded: true},
			{Path: "/tmp/next.jpg", Name: "next.jpg", IsVideo: false, Decoded: true},
		},
		Visible: []int{0, 1, 2}, LargeIndex: 1, InLargeView: true, Zoom: 1,
		VideoVolume: 0.7, PrevVolume: 0.7,
	}
}

func TestPlaybackSessionControlsOneBackend(t *testing.T) {
	backend := &fakePlaybackBackend{duration: 120}
	kernel := newPlaybackSession()
	kernel.backend = backend
	if err := kernel.Open("/tmp/movie.mp4", 0, true); err != nil {
		t.Fatal(err)
	}
	if backend.volume != 1 || backend.speed != 1 {
		t.Fatalf("initial volume=%v speed=%v", backend.volume, backend.speed)
	}
	kernel.SetVolume(0.4)
	kernel.SetSpeed(1.5)
	kernel.Seek(12)
	if backend.opens != 1 || len(backend.seeks) != 1 || backend.seeks[0] != 12 {
		t.Fatalf("seek rebuilt backend: opens=%d seeks=%v", backend.opens, backend.seeks)
	}
	if err := kernel.Step(1); err != nil {
		t.Fatal(err)
	}
	if backend.playing || len(backend.steps) != 1 || backend.steps[0] != 1 {
		t.Fatalf("step did not pause and advance: playing=%v steps=%v", backend.playing, backend.steps)
	}
	kernel.Seek(120)
	if snapshot := kernel.Snapshot(); !snapshot.Ended {
		t.Fatal("seeking to the end should expose ended state")
	}
	kernel.SetPlaying(true)
	if backend.opens != 2 || backend.position != 0 || !backend.playing {
		t.Fatalf("replay did not restart the source: opens=%d position=%v playing=%v", backend.opens, backend.position, backend.playing)
	}
	kernel.Close()
	kernel.Close()
	if backend.closes != 1 {
		t.Fatalf("backend closed %d times", backend.closes)
	}
}

func TestPausedSnapshotKeepsItsFrameAndClearsAFalseEnd(t *testing.T) {
	red := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	red.Pix[0], red.Pix[3] = 255, 255
	blue := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	blue.Pix[2], blue.Pix[3] = 255, 255
	backend := &fakePlaybackBackend{duration: 10, position: 1, playing: true, frame: red}
	kernel := newPlaybackSession()
	kernel.backend = backend
	kernel.path = "/tmp/movie.mp4"
	kernel.playing = true
	kernel.duration = 10
	kernel.ready = true
	if snap := kernel.Snapshot(); snap.Frame != red || !snap.Playing {
		t.Fatalf("playing snapshot frame=%v playing=%v", snap.Frame != nil, snap.Playing)
	}
	backend.frame = blue
	kernel.SetPlaying(false)
	snap := kernel.Snapshot()
	if snap.Playing || snap.Frame != red {
		t.Fatalf("pause pulled a new frame or stayed playing: playing=%v same=%v", snap.Playing, snap.Frame == red)
	}
	if backend.playing {
		t.Fatal("pause did not reach the backend")
	}
	backend.position = 10
	snap = kernel.Snapshot()
	if !snap.Ended || backend.playing {
		t.Fatalf("end left the backend running: ended=%v playing=%v", snap.Ended, backend.playing)
	}
	backend.position = 1
	snap = kernel.Snapshot()
	if snap.Ended {
		t.Fatal("a playhead well before the end stayed ended, so the next play click would reopen")
	}
	kernel.SetPlaying(true)
	if backend.opens != 0 || !backend.playing {
		t.Fatalf("resume reopened or did not play: opens=%d playing=%v", backend.opens, backend.playing)
	}
}

func TestPlaybackSessionReportsBackendFailure(t *testing.T) {
	original := playbackProbe
	playbackProbe = func() (playbackBackend, error) { return nil, errors.New("unavailable") }
	t.Cleanup(func() { playbackProbe = original })
	kernel := newPlaybackSession()
	if err := kernel.Open("/tmp/movie.mp4", 0, true); err == nil {
		t.Fatal("an unavailable backend must not report a playing session")
	}
	if snapshot := kernel.Snapshot(); snapshot.Playing || snapshot.Err == nil {
		t.Fatalf("failed open snapshot=%+v", snapshot)
	}
}

func TestVideoVolumePersistsMuteAndRestoresPreviousLevel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120}
	a := fakeVideoState(backend)
	a.Profiles = prefsFile{View: defaultViewPreferences(), ViewSet: true}
	a.toggleVideoVolume()
	if a.VideoVolume != 0 || backend.volume != 0 {
		t.Fatalf("mute volume=%v backend=%v", a.VideoVolume, backend.volume)
	}
	persisted := loadPrefs()
	if persisted.Volume != 0 || persisted.PreviousVolume != 0.7 {
		t.Fatalf("persisted volume=%v previous=%v", persisted.Volume, persisted.PreviousVolume)
	}
	a.toggleVideoVolume()
	if a.VideoVolume != 0.7 || backend.volume != 0.7 {
		t.Fatalf("unmute volume=%v backend=%v", a.VideoVolume, backend.volume)
	}
	if persisted := loadPrefs(); persisted.Volume != 0.7 {
		t.Fatalf("restored volume=%v", persisted.Volume)
	}
}

func TestVideoControlsNativeInteraction(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 20, playing: true}
	a := fakeVideoState(backend)
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 1000, 620)
	te.Frame()
	for _, label := range []string{"播放进度", "上一个媒体", "下一个媒体", "音量", "更多播放操作", "00:20", "02:00"} {
		if !te.HasText(label) {
			t.Fatalf("player chrome is missing %q: %q", label, te.Texts())
		}
	}
	if err := te.Click("暂停"); err != nil {
		t.Fatal(err)
	}
	if backend.playing {
		t.Fatal("pause button did not reach the playback kernel")
	}
	if err := te.Click("更多播放操作"); err != nil {
		t.Fatal(err)
	}
	if err := te.Click("下一帧"); err != nil {
		t.Fatal(err)
	}
	if len(backend.steps) != 1 || backend.steps[0] != 1 {
		t.Fatalf("frame button steps=%v", backend.steps)
	}
	if err := te.Click("静音"); err != nil {
		t.Fatal(err)
	}
	if a.VideoVolume != 0 || backend.volume != 0 {
		t.Fatalf("mute volume=%v backend=%v", a.VideoVolume, backend.volume)
	}
	if err := te.Click("静音"); err != nil {
		t.Fatal(err)
	}
	if a.VideoVolume != 0.7 || backend.volume != 0.7 {
		t.Fatalf("unmute volume=%v backend=%v", a.VideoVolume, backend.volume)
	}
	progress, ok := te.Find("播放进度")
	if !ok {
		t.Fatal("missing native timeline")
	}
	te.ClickAt(progress.X+progress.W*0.75, progress.Y+progress.H/2)
	if len(backend.seeks) == 0 || backend.seeks[len(backend.seeks)-1] < 85 {
		t.Fatalf("timeline did not seek: %v", backend.seeks)
	}
	if err := te.Click("更多播放操作"); err != nil {
		t.Fatal(err)
	}
	if !te.HasText("倍速 2×") || !te.HasText("上一帧") || !te.HasText("实际大小") {
		t.Fatalf("more menu is incomplete: %q", te.Texts())
	}
	if err := te.Click("倍速 2×"); err != nil {
		t.Fatal(err)
	}
	if backend.speed != 2 {
		t.Fatalf("speed menu did not reach the backend: %v", backend.speed)
	}
}

// A scrub callback runs after the build pass, so it must not touch the
// timeline Element. This drags the playhead and resizes the window to prove
// the committed width stays usable and current between frames.
func TestVideoTimelineScrubDragAndResize(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 20, playing: true}
	a := fakeVideoState(backend)
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 1000, 620)
	te.Frame()
	progress, ok := te.Find("播放进度")
	if !ok {
		t.Fatal("missing native timeline")
	}
	y := progress.Y + progress.H/2
	te.Press(progress.X+progress.W*0.25, y)
	te.Move(progress.X+progress.W*0.6, y)
	te.Release(progress.X+progress.W*0.7, y)
	if len(backend.seeks) == 0 {
		t.Fatal("drag did not commit a seek")
	}
	if at := backend.seeks[len(backend.seeks)-1]; at < 60 || at > 105 {
		t.Fatalf("drag seek out of range: %v", backend.seeks)
	}
	te.Frame()
	if !te.HasText("播放进度") || !te.HasText("更多播放操作") {
		t.Fatalf("controls stopped rendering after scrub: %q", te.Texts())
	}

	te.SetSize(1400, 620)
	te.Frame()
	wide, ok := te.Find("播放进度")
	if !ok {
		t.Fatal("missing timeline after resize")
	}
	before := len(backend.seeks)
	te.ClickAt(wide.X+wide.W*0.5, wide.Y+wide.H/2)
	if len(backend.seeks) <= before {
		t.Fatal("resize scrub did not commit a seek")
	}
	if at := backend.seeks[len(backend.seeks)-1]; at < 50 || at > 70 {
		t.Fatalf("resize used a stale timeline width: %v", backend.seeks)
	}
	if err := te.Click("暂停"); err != nil {
		t.Fatalf("controls not interactive after scrub: %v", err)
	}
}

func TestVideoLargeViewRendersController(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 20, playing: true}
	a := fakeVideoState(backend)
	a.Items[0].InfoLoaded = true
	a.Items[2].InfoLoaded = true
	te := ui.NewTester(a.largeImageView, 1200, 800)
	te.Frame()
	if !te.HasText("播放进度") || !te.HasText("00:20") || !te.HasText("更多播放操作") {
		t.Fatalf("full player controls missing: %q", te.Texts())
	}
	progress, ok := te.Find("播放进度")
	if !ok || progress.Y+progress.H > 800 || progress.W < 200 {
		t.Fatalf("timeline is too short for a seek bar: %+v", progress)
	}
	// A floating capsule, not a full-width bar: centered, narrower than the
	// window, and clear of both sides.
	first, okFirst := te.Find("暂停")
	last, okLast := te.Find("更多播放操作")
	if !okFirst || !okLast {
		t.Fatal("capsule ends missing")
	}
	span := last.X + last.W - first.X
	if span > barMaxWidth || span > 1200-2*barSideMargin {
		t.Fatalf("control bar spans %.0fpt, want a capsule of at most %d", span, barMaxWidth)
	}
	if centre := (first.X + last.X + last.W) / 2; absf(centre-600) > 12 {
		t.Fatalf("capsule is not centered: middle at %.0f of 1200", centre)
	}
	// One compact row: the play button sits on the timeline's line and
	// the whole bar (8pt margin + 6pt padding) stays within ~56pt.
	play, ok := te.Find("暂停")
	if !ok {
		t.Fatal("missing play button")
	}
	if mid := func(r ui.Rect) float32 { return r.Y + r.H/2 }; absf(mid(play)-mid(progress)) > 2 {
		t.Fatalf("timeline %+v is not on the transport row %+v", progress, play)
	}
	if top := progress.Y + progress.H/2 - barHeight/2; top < 800-barBottom-barHeight-2 {
		t.Fatalf("control bar starts %.0fpt above the bottom, want a single row", 800-top)
	}
	if !te.HasText("00:20") || !te.HasText("02:00") {
		t.Fatalf("clock missing: %q", te.Texts())
	}
	if dir := os.Getenv("LW_CARD_SHOTS"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "video-capsule.png"))
		_ = png.Encode(f, te.Image())
		f.Close()
	}
	if te.HasText("另存视频…") {
		t.Fatal("the save action belongs in the more menu, not on the bar")
	}
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestVideoControlsSwitchMedia(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120}
	a := fakeVideoState(backend)
	a.Root = "/tmp"
	a.Chosen = 1
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, a.currentItem()) })
	}, 1000, 180)
	if err := te.Click("下一个媒体"); err != nil {
		t.Fatal(err)
	}
	if a.LargeIndex != 2 {
		t.Fatalf("next media index=%d", a.LargeIndex)
	}
	a.LargeIndex = 1
	backend = &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120}
	a = fakeVideoState(backend)
	a.Root = "/tmp"
	te = ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, a.currentItem()) })
	}, 1000, 180)
	if err := te.Click("上一个媒体"); err != nil {
		t.Fatal(err)
	}
	if a.LargeIndex != 0 {
		t.Fatalf("previous media index=%d", a.LargeIndex)
	}
}

func TestVideoControlsOpenCropEditor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	makeSample := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=96x64:rate=10",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", path)
	if out, err := makeSample.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize video: %v %s", err, out)
	}
	backend := &fakePlaybackBackend{path: path, duration: 2}
	a := fakeVideoState(backend)
	a.Root = dir
	a.Items[1].Path = path
	a.Video.path = path
	a.Video.kernel.path = path
	a.Profiles.View.AutoPlaySelected = false
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 1000, 720)
	if err := te.Click("更多播放操作"); err != nil {
		t.Fatal(err)
	}
	if err := te.Click("裁剪画面"); err != nil {
		t.Fatal(err)
	}
	if !a.Editor.Open || a.Editor.Mode != "crop" {
		t.Fatalf("crop editor=%+v", a.Editor)
	}
}

func TestVideoControlsScreenshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	makeSample := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=96x64:rate=10",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", path)
	if out, err := makeSample.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize video: %v %s", err, out)
	}
	backend := &fakePlaybackBackend{path: path, duration: 1}
	a := fakeVideoState(backend)
	a.Root = dir
	a.Items[1].Path = path
	a.Video.path = path
	a.Video.kernel.path = path
	a.Window = nil // captureCurrentFrame uses this only for its completion toast.
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 1000, 720)
	if err := te.Click("更多播放操作"); err != nil {
		t.Fatal(err)
	}
	if err := te.Click("截存当前画面"); err != nil {
		t.Fatal(err)
	}
	var matches []string
	waitPlayback(t, func() bool {
		matches, _ = filepath.Glob(filepath.Join(dir, "*_frame_*.png"))
		return len(matches) == 1
	})
}

func TestFFmpegBackendPlaysSeeksAndSteps(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "playback.mp4")
	makeSample := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=96x64:rate=10",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", path)
	if out, err := makeSample.CombinedOutput(); err != nil {
		t.Fatalf("create sample video: %v\n%s", err, out)
	}
	backend, err := newFFmpegBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.Open(path, 0, false); err != nil {
		t.Fatal(err)
	}
	waitPlayback(t, func() bool { return backend.Frame() != nil })
	backend.Seek(0.5)
	if backend.Position() != 0.5 {
		t.Fatalf("seek position=%v", backend.Position())
	}
	at, err := backend.FrameStep(1)
	if err != nil || at <= 0.5 {
		t.Fatalf("step at=%v err=%v", at, err)
	}
	waitPlayback(t, func() bool { return backend.Frame() != nil })
	backend.SetPlaying(true)
	waitPlayback(t, func() bool { return backend.Position() > 0.5 })
	backend.SetPlaying(false)
	backend.Close()
}

func waitPlayback(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the playback backend")
}

func TestTimeAndVolumeNormalization(t *testing.T) {
	if clampPlaybackTime(-1, 100) != 0 || clampPlaybackTime(200, 100) != 100 {
		t.Fatal("media time was not clamped")
	}
	if normalizedPlaybackVolume(-1) != 0 || normalizedPlaybackVolume(3) != 2 {
		t.Fatal("volume was not clamped")
	}
	if got := volumeButtonIcon(0); got != lucideMuted {
		t.Fatal("muted volume should use the Lucide volume-x icon")
	}
	if got := volumeButtonIcon(1); got != lucideVolume {
		t.Fatal("audible volume should use the Lucide volume-2 icon")
	}
	if got := playbackButtonIcon(true); got != lucidePause {
		t.Fatal("playing media should use the Lucide pause icon")
	}
	if got := playbackButtonLabel(true); got != "暂停" {
		t.Fatalf("pause label=%q", got)
	}
}

// In the gallery the progress bar and thumbnail strip hold the bottom edge,
// so the video capsule has to ride above them instead of covering them.
func TestVideoCapsuleClearsGalleryChrome(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 20, playing: true}
	a := fakeVideoState(backend)
	a.Items[0].InfoLoaded = true
	a.Items[2].InfoLoaded = true
	a.Gallery = true
	a.galleryPeek = true // the strip is up
	te := ui.NewTester(a.view, 1200, 800)
	for i := 0; i < 12; i++ { // let the capsule glide up
		te.Frame()
	}
	play, ok := te.Find("暂停")
	if !ok {
		t.Fatalf("no capsule in the gallery: %q", te.Texts())
	}
	bottom := play.Y + play.H/2 + barHeight/2
	limit := float32(800 - galleryProgressH - galleryStripH)
	if bottom > limit {
		t.Fatalf("capsule bottom at %.0f covers the gallery strip, want it above %.0f", bottom, limit)
	}
}
