package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// browseKeyState builds an app browsing n synthetic images in the layout.
func browseKeyState(layout, n int) *appState {
	a := &appState{Chosen: 0, LargeIndex: -1, keyPivot: 0, Thumb: 128, Root: "/probe", LayoutMode: layout}
	a.Items = make([]mediaItem, n)
	a.Visible = make([]int, n)
	for i := range a.Items {
		a.Items[i] = mediaItem{
			Path:       fmt.Sprintf("/probe/frame-%03d.jpg", i),
			Name:       fmt.Sprintf("frame-%03d.jpg", i),
			Ext:        "jpg",
			InfoLoaded: true,
			Info:       mediaInfo{Width: 800, Height: 600},
			Decoded:    true, // skip thumbnail loading; selection is the point
		}
		a.Visible[i] = i
	}
	return a
}

// TestArrowKeysMoveSelectionEveryLayout covers the keyboard navigation the
// slideshow had only in the grid: arrows choose and scroll in every layout,
// Shift extends, Home/End jump to the ends.
func TestArrowKeysMoveSelectionEveryLayout(t *testing.T) {
	for _, layout := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("layout-%d", layout), func(t *testing.T) {
			a := browseKeyState(layout, 40)
			te := ui.NewTester(a.view, 1100, 760)
			te.Frame()
			te.Frame()
			te.Key(0, ui.KeyRight)
			te.Frame()
			if a.Chosen != 1 {
				t.Fatalf("right chose %d, want 1", a.Chosen)
			}
			if it := a.visibleItem(1); it == nil || !a.Selected.Has(it.Path) {
				t.Fatalf("selection after right = %#v", a.Selected.All())
			}
			te.Key(0, ui.KeyDown)
			te.Frame()
			if a.Chosen <= 1 || a.Chosen >= len(a.Visible) {
				t.Fatalf("down chose %d, want inside (1, 40)", a.Chosen)
			}
			before := a.Chosen
			te.Key(ui.Shift, ui.KeyDown)
			te.Frame()
			if a.Chosen == before || a.Selected.Len() < 2 {
				t.Fatalf("shift-down: chosen %d selected %d", a.Chosen, a.Selected.Len())
			}
			if !a.Selected.Has(a.visibleItem(before).Path) || !a.Selected.Has(a.visibleItem(a.Chosen).Path) {
				t.Fatalf("shift selection misses the range %#v", a.Selected.All())
			}
			te.Key(0, ui.KeyEnd)
			te.Frame()
			if a.Chosen != len(a.Visible)-1 {
				t.Fatalf("end chose %d, want %d", a.Chosen, len(a.Visible)-1)
			}
			te.Key(0, ui.KeyHome)
			te.Frame()
			if a.Chosen != 0 {
				t.Fatalf("home chose %d, want 0", a.Chosen)
			}
			// Every frame renders quiet chrome: no crash, cards still show.
			if !te.HasText("frame-000.jpg") {
				t.Fatalf("first card text missing: %v", te.Texts())
			}
		})
	}
}

// TestDetailRowsChooseLikeCards keeps the list layout on the cards' click
// contract after the widget's own row selection was unwired: click chooses
// only, Shift-click extends, double click opens the large view.
func TestDetailRowsChooseLikeCards(t *testing.T) {
	a := browseKeyState(1, 40)
	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()
	te.Frame()
	r, ok := te.Find("frame-005.jpg")
	if !ok {
		t.Fatalf("row not shown: %v", te.Texts())
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	te.ClickAt(x, y)
	te.Frame()
	if a.Chosen != 5 || a.Selected.Len() != 1 {
		t.Fatalf("click chose %d selected %d", a.Chosen, a.Selected.Len())
	}
	te.ClickAtWith(ui.Shift, x, y+40)
	te.Frame()
	if a.Selected.Len() < 2 {
		t.Fatalf("shift-click selected %d", a.Selected.Len())
	}
	te.ClickAt(x, y)
	te.ClickAt(x, y)
	te.Frame()
	if !a.InLargeView {
		t.Fatalf("double click did not open the large view, chosen %d", a.Chosen)
	}
	if it := a.currentItem(); it == nil || it.Name != "frame-005.jpg" {
		t.Fatalf("large view opened %v", a.currentItem())
	}
}

// TestWheelScrollsTheBrowseInstantly pins the 跟手 fix: the offset moves
// with the wheel event itself, not after MyGo's 150ms ease.
func TestWheelScrollsTheBrowseInstantly(t *testing.T) {
	for _, layout := range []int{0, 2, 3} {
		t.Run(fmt.Sprintf("layout-%d", layout), func(t *testing.T) {
			a := browseKeyState(layout, 400)
			te := ui.NewTester(a.view, 1100, 760)
			te.Frame()
			te.Frame()
			if a.browseBox.H < 10 {
				t.Fatalf("browse box = %+v", a.browseBox)
			}
			te.Wheel(a.browseBox.X+40, a.browseBox.Y+80, 0, 240)
			if a.browseScroll.Y <= 0 {
				t.Fatalf("wheel did not move the offset at once: %.1f", a.browseScroll.Y)
			}
			moved := a.browseScroll.Y
			te.Frame()
			if a.browseScroll.Y < moved {
				t.Fatalf("scroll snapped back on frame: %.1f -> %.1f", moved, a.browseScroll.Y)
			}
		})
	}
}
