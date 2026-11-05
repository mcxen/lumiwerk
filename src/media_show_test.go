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

func writeRedPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 220, G: 20, B: 20, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func redSamples(img image.Image) int {
	if img == nil {
		return 0
	}
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r > 0x8000 && g < 0x6000 && bl < 0x6000 {
				n++
			}
		}
	}
	return n
}

func TestJustifiedAndLargeViewPaintAPhoto(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "red.png")
	writeRedPNG(t, path, 320, 200)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		t.Fatal(err)
	}
	item := mediaItem{
		Path: path, Name: "red.png", Ext: "png", Bitmap: bm, Decoded: true,
		InfoLoaded: true, Info: mediaInfo{Width: 320, Height: 200},
	}
	grid := &appState{Chosen: 0, LargeIndex: -1, Thumb: 160, LayoutMode: 3, Root: dir}
	grid.Items = []mediaItem{item}
	grid.Visible = []int{0}
	te := ui.NewTester(grid.view, 800, 600)
	te.Frame()
	te.Frame()
	if got := redSamples(te.Image()); got < 80 {
		t.Fatalf("justified thumbnail is blank, red=%d", got)
	}

	largeItem := item
	largeItem.Large = bm
	large := &appState{
		Chosen: 0, LargeIndex: 0, Thumb: 160, InLargeView: true, Zoom: 1, Root: dir,
		Items: []mediaItem{largeItem}, Visible: []int{0},
	}
	lte := ui.NewTester(large.view, 800, 600)
	lte.Frame()
	lte.Frame()
	if got := redSamples(lte.Image()); got < 200 {
		t.Fatalf("large view is blank, red=%d", got)
	}
}

func TestVisiblePhotoDecodesOntoTheCard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hook := func(fn func()) { fn() }
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })

	dir := t.TempDir()
	path := filepath.Join(dir, "red.png")
	writeRedPNG(t, path, 96, 64)
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 160, LayoutMode: 3, Root: dir}
	a.Items = []mediaItem{{Path: path, Name: "red.png", Ext: "png"}}
	a.Visible = []int{0}
	te := ui.NewTester(a.view, 800, 600)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		te.Frame()
		a.mu.Lock()
		decoded, failed := a.Items[0].Decoded, a.Items[0].Failed
		a.mu.Unlock()
		if failed {
			t.Fatal("thumbnail decode failed")
		}
		if decoded {
			if got := redSamples(te.Image()); got < 40 {
				t.Fatalf("decoded thumbnail did not paint, red=%d", got)
			}
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("visible photo was never decoded")
}
