package main

import (
	"testing"
)

// Esc out of the large view must not lay the browser out at a transient
// width: the sidebar Split stays in the tree, so the grid gets its full
// width on the first frames instead of a zero-width frame and a jump.
func TestLargeViewExitKeepsBrowseWidth(t *testing.T) {
	a, te, _ := navApp(t, navTree(t))
	a.SidebarShown, a.SidebarWidth, a.LayoutMode = true, sidebarDefaultWidth, 0
	for i := 0; i < 3; i++ {
		te.Frame()
	}
	want := a.browseBox
	if want.W < 2 {
		t.Fatalf("no grid box before large view: %+v", want)
	}
	vi := a.visibleIndexOf(a.Root + "/b.png")
	a.Chosen = vi
	a.openLargeImage(vi)
	for i := 0; i < 3; i++ {
		te.Frame()
	}
	if a.SidebarWidth != sidebarDefaultWidth {
		t.Fatalf("large view changed sidebar width to %v", a.SidebarWidth)
	}
	a.closeLargeImage()
	a.browseBox = browseBox{}
	// LayoutBounds reads the previous frame's layout. The old tree swap
	// rebuilt the browser as new elements, so the first frame after Esc
	// had no layout at all (zero width) and the grid came in narrow.
	te.Frame()
	if a.browseBox.W < 2 {
		t.Fatalf("first frame after exit has no grid layout: %+v (browser rebuilt)", a.browseBox)
	}
	for i := 1; i < 4; i++ {
		te.Frame()
		if a.browseBox.W != want.W || a.browseBox.X != want.X || a.browseBox.Cols != want.Cols {
			t.Fatalf("frame %d after exit: grid %+v, want %+v", i, a.browseBox, want)
		}
	}
	if a.Chosen != vi {
		t.Fatalf("chosen %d after exit, want %d", a.Chosen, vi)
	}
}
