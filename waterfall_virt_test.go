package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// waterfalTestState builds an appState with n synthetic items and no disk IO.
func waterfalTestState(n int) *appState {
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, LayoutMode: 2}
	a.Root = "/probe"
	a.Items = make([]mediaItem, n)
	a.Visible = make([]int, n)
	for i := range a.Items {
		a.Items[i] = mediaItem{
			Path:       "/probe/" + string(rune('a'+i%26)) + itoa(i) + ".png",
			Name:       "img" + itoa(i) + ".png",
			Ext:        "png",
			InfoLoaded: true,
			Info:       mediaInfo{Width: 800, Height: 1200},
			Decoded:    true, // skip thumbnail loading; view only
		}
		a.Visible[i] = i
	}
	return a
}

// thumbCardProbeSwap swaps the probe and returns the restore func.
func thumbCardProbeSwap(fn func()) func() {
	old := thumbCardProbe
	thumbCardProbe = fn
	return func() { thumbCardProbe = old }
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// TestWaterfallVirtualization builds only visible cards: a 5000-item folder
// must not build 5000 card widgets in one frame.
func TestWaterfallVirtualization(t *testing.T) {
	const n = 5000
	a := waterfalTestState(n)
	cards := 0
	restore := thumbCardProbeSwap(func() { cards++ })
	defer restore()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	if cards == 0 {
		t.Fatal("no cards built")
	}
	if cards > 300 {
		t.Fatalf("built %d cards for %d items in one frame; virtualization failed", cards, n)
	}
	t.Logf("built %d cards for %d items", cards, n)
}

// TestWaterfallContentHeight keeps the whole waterfall scrollable: MaxY must
// reach the packing height of all items, not just the visible window.
func TestWaterfallContentHeight(t *testing.T) {
	const n = 2000
	a := waterfalTestState(n)
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	if a.browseScroll.MaxY < 100_000 {
		t.Fatalf("MaxY=%.0f too small for %d items; content collapsed", a.browseScroll.MaxY, n)
	}
	// Scroll to the end: the window must follow and still build few cards.
	a.browseScroll.Y = a.browseScroll.MaxY
	cards := 0
	restore := thumbCardProbeSwap(func() { cards++ })
	defer restore()
	te.Frame()
	if cards == 0 || cards > 300 {
		t.Fatalf("at end built %d cards; window did not follow scroll", cards)
	}
}

// TestWaterfallFramesCoversAllItems checks the packing puts every item in
// exactly one box at sane coordinates.
func TestWaterfallFramesCoversAllItems(t *testing.T) {
	aspects := make([]float32, 50)
	for i := range aspects {
		aspects[i] = 0.5 + float32(i%7)*0.25
	}
	boxes := waterfallFrames(aspects, 1200, 128)
	if len(boxes) != len(aspects) {
		t.Fatalf("boxes = %d, aspects = %d", len(boxes), len(aspects))
	}
	seen := map[int]bool{}
	for _, b := range boxes {
		if seen[b.Index] {
			t.Fatalf("item %d packed twice", b.Index)
		}
		seen[b.Index] = true
		if b.W <= 0 || b.H <= 0 || b.X < 0 || b.Y < 0 {
			t.Fatalf("bad box %+v", b)
		}
	}
	for i := range aspects {
		if !seen[i] {
			t.Fatalf("item %d not packed", i)
		}
	}
}
