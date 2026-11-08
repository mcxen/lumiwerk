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

func TestGlyphSizeScalesWithCard(t *testing.T) {
	if got := glyphSize(40, 28); got != 28 {
		t.Fatalf("small card glyph = %v, want floor 28", got)
	}
	if got := glyphSize(200, 28); got != 140 {
		t.Fatalf("200pt card glyph = %v, want 140", got)
	}
	if got := glyphSize(1000, 28); got != 160 {
		t.Fatalf("huge card glyph = %v, want cap 160", got)
	}
}

// TestCardNamesKeepSizeWhileChromeScales checks the scale follows the
// slider but the caption band, which holds file and folder names, stays fixed.
func TestCardNamesKeepSizeWhileChromeScales(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if cardScale(150) != 1 || cardScale(64) != 0.9 || cardScale(512) != 1.8 {
		t.Fatalf("scale 64/150/512 = %v/%v/%v", cardScale(64), cardScale(150), cardScale(512))
	}
	for _, thumb := range []float64{64, 150, 512} {
		a := waterfalTestState(10)
		a.Thumb = thumb
		a.syncCaption()
		if flowLabel != 30 {
			t.Fatalf("caption band = %v at thumb %v, want fixed 30", flowLabel, thumb)
		}
		te := ui.NewTester(a.view, 1280, 820)
		te.Frame()
		if !te.HasText("img0.png") {
			t.Fatalf("thumb %v: names missing: %v", thumb, te.Texts())
		}
	}
}
