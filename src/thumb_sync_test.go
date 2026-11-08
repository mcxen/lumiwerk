package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testThumbBitmap(t *testing.T) *ui.Bitmap {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	bm, err := ui.DecodeBitmap(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return bm
}

// TestScrolledCardsShowCachedThumbInSameFrame scrolls the grid, waterfall and
// justified layouts onto cards whose thumbnails sit in the memory cache.
// Their names are drawn in the frame the card is built, so the picture must
// be too: not a frame later through a worker and Window.Update.
func TestScrolledCardsShowCachedThumbInSameFrame(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bm := testThumbBitmap(t)
	for _, tc := range []struct {
		name   string
		layout int
	}{{"grid", 0}, {"waterfall", 2}, {"justified", 3}} {
		t.Run(tc.name, func(t *testing.T) {
			const n = 600
			a := waterfalTestState(n)
			a.LayoutMode = tc.layout
			edge := thumbBand(thumbEdge(a.Thumb))
			for i := range a.Items {
				a.Items[i].Decoded = false
				key := thumbCacheKey(a.Items[i].Path, edge)
				thumbLRU.Put(key, bm, edge)
				defer thumbLRU.Delete(key)
			}
			te := ui.NewTester(a.view, 1280, 820)
			te.Frame()
			a.browseScroll.Y = a.browseScroll.MaxY / 2
			built := 0
			defer thumbCardProbeSwap(func() { built++ })()
			te.Frame()
			if built == 0 {
				t.Fatal("no cards built after scroll")
			}
			bare := 0
			for i := range a.Items {
				if it := &a.Items[i]; it.Seen != 0 && it.Bitmap == nil {
					bare++
				}
			}
			if bare != 0 {
				t.Fatalf("%d built cards drew their name without the cached thumbnail", bare)
			}
		})
	}
}
