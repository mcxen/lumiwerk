package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// noFrameBackend is a backend still loading its first frame.
type noFrameBackend struct{ fakePlaybackBackend }

func (b *noFrameBackend) Frame() image.Image { return nil }

// redBounds returns the bounding box of strongly red pixels.
func redBounds(img image.Image) image.Rectangle {
	var box image.Rectangle
	if img == nil {
		return box
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r > 0xc000 && g < 0x4000 && bl < 0x4000 {
				box = box.Union(image.Rect(x, y, x+2, y+2))
			}
		}
	}
	return box
}

// While the first frame loads, the grid thumbnail stands in for the video.
// It used to be laid out at its own small size in the middle of the stage;
// it must fill the stage like the frame that replaces it.
func TestVideoPosterFillsTheStageWhileLoading(t *testing.T) {
	backend := &noFrameBackend{fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, playing: true}}
	a := fakeVideoState(&backend.fakePlaybackBackend)
	a.Video.kernel.backend = backend
	thumb := image.NewNRGBA(image.Rect(0, 0, 160, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			thumb.SetNRGBA(x, y, color.NRGBA{R: 230, G: 10, B: 10, A: 255})
		}
	}
	a.Items[1].Bitmap = ui.NewBitmap(thumb)
	const w, h = 1200, 800
	a.Items[0].InfoLoaded = true
	a.Items[2].InfoLoaded = true
	te := ui.NewTester(a.largeImageView, w, h)
	te.Frame()
	te.Frame()
	if snap := a.videoSnapshot(); snap.Phase != playLoading {
		t.Fatalf("phase = %v, want loading", snap.Phase)
	}
	if !te.HasText("正在加载") {
		t.Fatalf("loading badge missing: %q", te.Texts())
	}
	box := redBounds(te.Image())
	scale := float64(te.Image().Bounds().Dx()) / w
	if got := float64(box.Dx()) / scale; got < w*0.8 {
		t.Fatalf("poster is %.0fpt wide in a %dpt stage (box %v)", got, w, box)
	}
}
