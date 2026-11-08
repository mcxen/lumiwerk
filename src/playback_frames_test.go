package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The render thread must never write into a buffer the view was handed
// (the one on screen or the one before it) or the frame waiting to be taken.
func TestMPVFramePoolNeverReusesShownBuffers(t *testing.T) {
	var p mpvFramePool
	var shown []*image.RGBA
	allocs := map[*byte]bool{}
	for i := 0; i < 200; i++ {
		frame := p.acquire(64, 36)
		allocs[&frame.Pix[0]] = true
		for k := len(shown) - 1; k >= 0 && k >= len(shown)-2; k-- {
			if sameBuffer(frame.Pix, shown[k].Pix) {
				t.Fatalf("frame %d renders into the buffer shown %d frames ago", i, len(shown)-k)
			}
		}
		frame.Pix[0] = byte(i)
		p.publish(frame)
		// The view takes most frames; some are dropped as it lags.
		if i%3 != 2 {
			got := p.take()
			if got != frame {
				t.Fatalf("take = %p, want newest %p", got, frame)
			}
			shown = append(shown, got)
		}
	}
	if len(allocs) > 5 {
		t.Fatalf("steady playback allocated %d buffers, want at most 5", len(allocs))
	}
	if again := p.take(); again != shown[len(shown)-1] {
		t.Fatal("take without a new frame must return the frame on screen")
	}
}

func TestMPVFramePoolHeadersDifferPerPublish(t *testing.T) {
	var p mpvFramePool
	seen := map[*image.RGBA]bool{}
	for i := 0; i < 20; i++ {
		f := p.acquire(32, 16)
		p.publish(f)
		got := p.take()
		if seen[got] {
			t.Fatal("a new picture reused an old image pointer; the view would skip it")
		}
		seen[got] = true
		if got.Stride != 32*4 || got.Rect != image.Rect(0, 0, 32, 16) {
			t.Fatalf("frame layout stride=%d rect=%v; ui.Bitmap.Update copies anything else", got.Stride, got.Rect)
		}
	}
	if p.generation() != 20 {
		t.Fatalf("generation=%d", p.generation())
	}
	p.clear()
	if p.take() != nil {
		t.Fatal("clear must drop the previous file's frame")
	}
}

func TestMPVRenderSize(t *testing.T) {
	for _, tc := range []struct{ w, h, wantW, wantH int }{
		{0, 0, 0, 0},
		{1280, 720, 1280, 720},
		{1918, 1080, 1904, 1072},
		{3840, 2160, 1920, 1080},
		{1080, 1920, 1072, 1906},
	} {
		w, h := mpvRenderSize(tc.w, tc.h)
		if w != tc.wantW || h != tc.wantH {
			t.Errorf("mpvRenderSize(%d,%d) = %d×%d, want %d×%d", tc.w, tc.h, w, h, tc.wantW, tc.wantH)
		}
		if w > 0 && (w%16 != 0 || w*h > mpvMaxPixels) {
			t.Errorf("mpvRenderSize(%d,%d) = %d×%d breaks alignment or the pixel cap", tc.w, tc.h, w, h)
		}
	}
}

func TestOpaqueFillsPaddingByte(t *testing.T) {
	pix := []byte{1, 2, 3, 0, 4, 5, 6, 9}
	opaque(pix)
	want := []byte{1, 2, 3, 255, 4, 5, 6, 255}
	for i := range pix {
		if pix[i] != want[i] {
			t.Fatalf("opaque = %v, want %v", pix, want)
		}
	}
}

// pushingBackend announces frames like the libmpv backend does.
type pushingBackend struct {
	fakePlaybackBackend
	notify func()
}

func (b *pushingBackend) SetFrameNotify(fn func()) { b.notify = fn }

// A redraw that sees the same frame must not convert and upload it again,
// and a frame-pushing backend must not be polled at a fixed 30 Hz.
func TestPresentVideoUploadsEachFrameOnce(t *testing.T) {
	backend := &pushingBackend{fakePlaybackBackend: fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, playing: true}}
	kernel := newPlaybackSession()
	kernel.backend = backend
	kernel.path = "/tmp/movie.mp4"
	kernel.playing = true
	a := &appState{Video: &videoView{kernel: kernel, path: "/tmp/movie.mp4"}}
	a.wirePlaybackBackend(kernel)
	if backend.notify == nil {
		t.Fatal("frame-pushing backend was not wired to redraw the window")
	}
	first := image.NewRGBA(image.Rect(0, 0, 16, 9))
	backend.frame = first
	var bitmaps []*ui.Bitmap
	te := ui.NewTester(func(c *ui.Context) {
		snap := a.videoSnapshot()
		if !snap.FramePushed {
			t.Error("snapshot does not report a frame-pushing backend")
		}
		bitmaps = append(bitmaps, a.presentVideo(c, snap))
	}, 320, 200)
	te.Frame()
	bitmap := a.Video.frame
	if bitmap == nil || a.Video.shown != image.Image(first) {
		t.Fatalf("first frame not shown: bitmap=%v shown=%v", bitmap, a.Video.shown)
	}
	te.Frame()
	if a.Video.frame != bitmap || a.Video.shown != image.Image(first) {
		t.Fatal("the same frame replaced the bitmap")
	}
	second := image.NewRGBA(image.Rect(0, 0, 16, 9))
	backend.frame = second
	te.Frame()
	if a.Video.frame != bitmap || a.Video.shown != image.Image(second) {
		t.Fatal("a new frame must update the existing bitmap in place")
	}
	for _, b := range bitmaps {
		if b != bitmap {
			t.Fatal("presentVideo returned a different bitmap")
		}
	}
}
