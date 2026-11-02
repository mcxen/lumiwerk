package main

import (
	"image"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestJustifiedVirtualization builds only the rows near the scroll window and
// keeps the full content height.
func TestJustifiedVirtualization(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const n = 5000
	a := waterfalTestState(n)
	a.LayoutMode = 3
	cards := 0
	restore := thumbCardProbeSwap(func() { cards++ })
	defer restore()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	if cards == 0 || cards > 400 {
		t.Fatalf("built %d cards for %d items", cards, n)
	}
	if a.browseScroll.MaxY < 50_000 {
		t.Fatalf("MaxY=%.0f, content collapsed", a.browseScroll.MaxY)
	}
	a.browseScroll.Y = a.browseScroll.MaxY
	cards = 0
	te.Frame()
	if cards == 0 || cards > 400 {
		t.Fatalf("at the end built %d cards", cards)
	}
	if !te.HasText(a.Items[n-1].Name) {
		t.Fatal("last item not shown at the end")
	}
}

func TestSweepReleasesOffscreenUncachedThumbs(t *testing.T) {
	a := waterfalTestState(4)
	bm := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	now := time.Now()
	old := now.Add(-time.Minute).UnixNano()
	for i := range a.Items {
		a.Items[i].Bitmap = bm
		a.Items[i].ThumbEdge = 384
	}
	a.Items[0].Seen = old            // off screen, not cached: released
	a.Items[1].Seen = now.UnixNano() // on screen: kept
	a.Items[2].Seen = 0              // never drawn by a view: kept
	a.Items[3].Seen = old            // off screen but cached: kept
	key := thumbCacheKey(a.Items[3].Path, 384)
	thumbLRU.Put(key, bm, 384)
	defer thumbLRU.Delete(key)
	if got := a.sweepThumbsAt(now); got != 1 {
		t.Fatalf("released %d", got)
	}
	if a.Items[0].Bitmap != nil || a.Items[0].Decoded {
		t.Fatal("off-screen thumbnail kept")
	}
	for i := 1; i < 4; i++ {
		if a.Items[i].Bitmap == nil {
			t.Fatalf("item %d released", i)
		}
	}
}

func TestRequestInfoStartsOneProbe(t *testing.T) {
	a := waterfalTestState(1)
	a.Items[0].InfoLoaded = false
	a.Items[0].Path = "/nonexistent/lumiwerk-probe.png"
	for range 50 {
		a.requestInfo(0)
	}
	if !a.Items[0].InfoLoading {
		t.Fatal("probe not marked")
	}
}

func TestQueuedDecodeAbandonedOffscreen(t *testing.T) {
	a := waterfalTestState(2)
	frame := time.Now().UnixNano()
	atomic.StoreInt64(&a.frameNow, frame)
	a.Items[0].Seen = frame     // built in the latest frame
	a.Items[1].Seen = frame - 1 // scrolled away
	if a.thumbAbandoned(0, a.Items[0].Path, 0) {
		t.Fatal("on-screen decode abandoned")
	}
	if !a.thumbAbandoned(1, a.Items[1].Path, 0) {
		t.Fatal("off-screen decode kept")
	}
	a.Items[1].Seen = 0
	if a.thumbAbandoned(1, a.Items[1].Path, 0) {
		t.Fatal("decode for an item no view drew (large view) abandoned")
	}
}

func TestTreeChildrenCached(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &appState{}
	if got := a.treeChildren(dir); len(got) != 1 {
		t.Fatalf("children %v", got)
	}
	// Within the TTL the listing is served from the cache.
	if err := os.Mkdir(filepath.Join(dir, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := a.treeChildren(dir); len(got) != 1 {
		t.Fatalf("listing re-read on the next frame: %v", got)
	}
}

func TestFlowLayoutCachedAcrossFrames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, layout := range []int{2, 3} {
		a := waterfalTestState(2000)
		a.LayoutMode = layout
		te := ui.NewTester(a.view, 1280, 820)
		te.Frame()
		te.Frame()
		base := a.flowCache.builds
		for s := 1; s <= 20; s++ {
			a.browseScroll.Y = float32(s) * 300
			te.Frame()
		}
		if got := a.flowCache.builds - base; got != 0 {
			t.Fatalf("layout %d: scrolling repacked %d times", layout, got)
		}
		a.Thumb = 200
		te.Frame()
		if a.flowCache.builds == base {
			t.Fatalf("layout %d: thumbnail size change kept the old packing", layout)
		}
		base = a.flowCache.builds
		a.Items[0].Info.Width = 3000
		a.noteLayoutChanged()
		te.Frame()
		if a.flowCache.builds == base {
			t.Fatalf("layout %d: aspect change kept the old packing", layout)
		}
		base = a.flowCache.builds
		te.SetSize(1000, 820)
		te.Frame()
		te.Frame()
		if a.flowCache.builds == base {
			t.Fatalf("layout %d: resize kept the old packing", layout)
		}
	}
}
