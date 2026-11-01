package main

import (
	"image"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestPerfMeasure prints frame, card and memory figures for a synthetic
// folder. It runs only with LUMIWERK_PERF=1; the numbers are reported, not
// asserted, so it stays stable on slow machines.
func TestPerfMeasure(t *testing.T) {
	if os.Getenv("LUMIWERK_PERF") == "" {
		t.Skip("set LUMIWERK_PERF=1")
	}
	const n = 3000
	for _, layout := range []int{0, 1, 2, 3} {
		t.Setenv("HOME", t.TempDir())
		a := waterfalTestState(n)
		a.LayoutMode = layout
		for i := range a.Items {
			img := image.NewRGBA(image.Rect(0, 0, 256, 384))
			for p := range img.Pix {
				img.Pix[p] = byte(i + p)
			}
			a.Items[i].Bitmap = ui.NewBitmap(img)
			a.Items[i].InfoLoaded = i%2 == 0 // half still need a probe
		}
		cards := 0
		restore := thumbCardProbeSwap(func() { cards++ })
		var ms0 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms0)
		g0 := runtime.NumGoroutine()
		te := ui.NewTester(a.view, 1280, 820)
		te.Frame()
		first := cards
		maxG := runtime.NumGoroutine()
		start := time.Now()
		const steps = 60
		for s := 1; s <= steps; s++ {
			a.browseScroll.Y = float32(s) * 400
			te.Frame()
			maxG = max(maxG, runtime.NumGoroutine())
		}
		per := time.Since(start) / steps
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		restore()
		retained := 0
		for i := range a.Items {
			if a.Items[i].Bitmap != nil {
				retained++
			}
		}
		t.Logf("layout=%d items=%d firstFrameCards=%d avgScrollFrame=%v extraGoroutinesPeak=%d heapGrowth=%.1fMiB retainedThumbs=%d",
			layout, n, first, per, maxG-g0, (float64(ms.HeapInuse)-float64(ms0.HeapInuse))/(1<<20), retained)
		runtime.KeepAlive(te)
	}
}
