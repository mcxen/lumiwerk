package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// heroSample is a solid red photo that is easy to find in a rendered frame.
func heroSample(t *testing.T, w, h int) *ui.Bitmap {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{230, 30, 40, 255})
		}
	}
	return ui.NewBitmap(img)
}

// redBox returns the bounds of the red sample in a frame.
func redBox(img *image.RGBA) image.Rectangle {
	b := img.Bounds()
	box := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := img.RGBAAt(x, y)
			if p.R > 200 && p.G < 70 && p.B < 80 {
				box.Min.X, box.Min.Y = min(box.Min.X, x), min(box.Min.Y, y)
				box.Max.X, box.Max.Y = max(box.Max.X, x+1), max(box.Max.Y, y+1)
			}
		}
	}
	return box
}

func heroShot(t *testing.T, te *ui.Tester, name string) {
	dir := os.Getenv("LUMIWERK_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, te.Image()); err != nil {
		t.Fatal(err)
	}
}

func heroApp(t *testing.T, layout int) *appState {
	t.Setenv("HOME", t.TempDir())
	bm := heroSample(t, 1600, 900)
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 128, LayoutMode: layout}
	a.Root = "/hero"
	a.Items = []mediaItem{{
		Path: "/hero/a.png", Name: "a.png", Ext: "png", Bitmap: bm, Large: bm,
		Decoded: true, InfoLoaded: true, Info: mediaInfo{Width: 1600, Height: 900},
	}}
	grey := image.NewRGBA(image.Rect(0, 0, 900, 900))
	for i := range grey.Pix {
		grey.Pix[i] = 140
		if i%4 == 3 {
			grey.Pix[i] = 255
		}
	}
	gb := ui.NewBitmap(grey)
	for _, n := range []string{"b.png", "c.png", "d.png"} {
		a.Items = append(a.Items, mediaItem{Path: "/hero/" + n, Name: n, Ext: "png", Bitmap: gb, Large: gb,
			Decoded: true, InfoLoaded: true, Info: mediaInfo{Width: 900, Height: 900}})
	}
	a.Visible = []int{0, 1, 2, 3}
	return a
}

// TestSharedElementOpenAndClose: the thumbnail grows into the fitted image
// and shrinks back, with intermediate frames between the two boxes.
func TestSharedElementOpenAndClose(t *testing.T) {
	for _, layout := range []int{0, 2, 3} {
		a := heroApp(t, layout)
		te := ui.NewTester(a.view, 1100, 760)
		te.Frame()
		te.Advance(time.Second)
		thumb := redBox(te.Image())
		if thumb.Empty() || thumb.Dx() > 400 {
			t.Fatalf("layout %d: thumbnail box %v", layout, thumb)
		}
		heroShot(t, te, "layout"+string(rune('0'+layout))+"-0-grid")

		a.openLargeImage(0)
		te.Frame()
		start := redBox(te.Image())
		te.Advance(90 * time.Millisecond)
		mid := redBox(te.Image())
		heroShot(t, te, "layout"+string(rune('0'+layout))+"-1-open-mid")
		te.Advance(2 * time.Second)
		end := redBox(te.Image())
		heroShot(t, te, "layout"+string(rune('0'+layout))+"-2-open-end")
		if abs32(float32(start.Dx()-thumb.Dx())) > 6 {
			t.Errorf("layout %d: open starts at %v, thumbnail was %v", layout, start, thumb)
		}
		if !(mid.Dx() > thumb.Dx()+10 && mid.Dx() < end.Dx()-10) {
			t.Errorf("layout %d: open midway %v, from %v to %v", layout, mid, thumb, end)
		}
		if end.Dx() < 900 {
			t.Errorf("layout %d: open ends at %v, not fitted", layout, end)
		}
		if a.Hero.phase != heroNone {
			t.Errorf("layout %d: hero phase %d after settling", layout, a.Hero.phase)
		}

		// Close, then reverse mid-flight: the open starts where the
		// shrinking image is, not from the thumbnail.
		a.closeLargeImage()
		te.Frame()
		te.Advance(90 * time.Millisecond)
		closing := redBox(te.Image())
		heroShot(t, te, "layout"+string(rune('0'+layout))+"-3-close-mid")
		if !(closing.Dx() < end.Dx()-10 && closing.Dx() > thumb.Dx()+10) {
			t.Errorf("layout %d: close midway %v", layout, closing)
		}
		a.openLargeImage(0)
		te.Frame()
		if b := redBox(te.Image()); abs32(float32(b.Dx()-closing.Dx())) > 40 {
			t.Errorf("layout %d: reversal jumped from %v to %v", layout, closing, b)
		}
		te.Advance(2 * time.Second)
		a.closeLargeImage()
		te.Frame()
		te.Advance(2 * time.Second)
		if b := redBox(te.Image()); abs32(float32(b.Dx()-thumb.Dx())) > 6 {
			t.Errorf("layout %d: close ends at %v, thumbnail was %v", layout, b, thumb)
		}
		heroShot(t, te, "layout"+string(rune('0'+layout))+"-4-closed")
	}
}

// TestSharedElementReduceMotion: with less motion, the image is fitted at once.
func TestSharedElementReduceMotion(t *testing.T) {
	a := heroApp(t, 0)
	te := ui.NewTester(a.view, 1100, 760)
	te.SetPreferences(ui.Preferences{ReduceMotion: true})
	te.Frame()
	a.openLargeImage(0)
	te.Frame()
	if b := redBox(te.Image()); b.Dx() < 900 {
		t.Errorf("without motion, the image opens at %v", b)
	}
}
