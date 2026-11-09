package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// marqueeApp is a browse window over n synthetic files in one folder, laid
// out as layout (0 grid, 1 list, 2 waterfall, 3 justified).
func marqueeApp(layout, n int) *appState {
	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: 128, Root: "/probe", LayoutMode: layout}
	a.Profiles = prefsFile{View: defaultViewPreferences(), ViewSet: true}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("p%03d.jpg", i)
		a.Items = append(a.Items, mediaItem{
			Path: filepath.Join("/probe", name), Name: name, Ext: "jpg", Size: 1000 + int64(i),
			InfoLoaded: true, Info: mediaInfo{Width: 800, Height: 600}, Decoded: true, ModTime: 1,
		})
		a.Visible = append(a.Visible, i)
	}
	return a
}

func marqueeTester(t *testing.T, a *appState) *ui.Tester {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	te := ui.NewTester(a.view, 1000, 700)
	te.Frame()
	te.Frame()
	te.Frame()
	return te
}

// cardCenter is the window point of the center of card i, from the rect the
// layout reported.
func cardCenter(a *appState, i int) (float32, float32) {
	r := a.cardRects.rects[i]
	return a.marqueeBox.X + r.X + r.W/2, a.marqueeBox.Y + r.Y + r.H/2 - a.browseScroll.Y
}

func selectedIndexes(a *appState) []int {
	var out []int
	for i := range a.Visible {
		if it := a.visibleItem(i); it != nil && a.Selected.Has(it.Path) {
			out = append(out, i)
		}
	}
	return out
}

func TestMarqueeRectsMatchLayout(t *testing.T) {
	for _, layout := range []int{0, 1, 2, 3} {
		a := marqueeApp(layout, 60)
		te := marqueeTester(t, a)
		if len(a.cardRects.rects) == 0 {
			t.Fatalf("layout %d: no card rects recorded", layout)
		}
		// The center of the first card sits on its name label.
		x, y := cardCenter(a, 0)
		name, ok := te.Find("p000.jpg")
		if !ok {
			t.Fatalf("layout %d: no first card: %v", layout, te.Texts())
		}
		t.Logf("layout %d: box %+v card0 center (%.0f,%.0f) name %+v rects %d", layout, a.marqueeBox, x, y, name, len(a.cardRects.rects))
		if layout != 1 && (x < name.X-name.W || x > name.X+2*name.W) {
			t.Fatalf("layout %d: card center x %.0f is off the card name %+v", layout, x, name)
		}
		if a.marqueeBlankAt(x, y) {
			t.Fatalf("layout %d: the middle of a card is blank", layout)
		}
	}
}

func TestMarqueeSelectsCardsUnderTheBand(t *testing.T) {
	for _, layout := range []int{0, 2, 3} {
		a := marqueeApp(layout, 40)
		te := marqueeTester(t, a)
		// Find blank space: just right of the last card of the first row.
		x0, y0 := cardCenter(a, 0)
		x1, y1 := cardCenter(a, 2)
		// Start in the gap above-left of card 0 so the press is blank.
		sx, sy := a.marqueeBox.X+1, a.marqueeBox.Y+1
		if !a.marqueeBlankAt(sx, sy) {
			t.Fatalf("layout %d: corner (%.0f,%.0f) is not blank", layout, sx, sy)
		}
		te.Press(sx, sy)
		te.Move(sx+10, sy+10)
		te.Move(x1, y1)
		te.Frame()
		got := selectedIndexes(a)
		t.Logf("layout %d: centers (%.0f,%.0f)..(%.0f,%.0f) selected %v", layout, x0, y0, x1, y1, got)
		if !slices.Contains(got, 0) || !slices.Contains(got, 2) {
			t.Fatalf("layout %d: band over cards 0..2 selected %v", layout, got)
		}
		if !a.marquee.active {
			t.Fatalf("layout %d: band not active", layout)
		}
		te.Release(x1, y1)
		te.Frame()
		if a.marquee.down || a.marquee.active {
			t.Fatalf("layout %d: band stuck after release", layout)
		}
		if len(selectedIndexes(a)) != len(got) {
			t.Fatalf("layout %d: release changed the selection", layout)
		}
	}
}

// gapRightOf is a blank window point just right of card i, in the gap
// between it and its neighbour.
func gapRightOf(t *testing.T, a *appState, i int) (float32, float32) {
	t.Helper()
	r := a.cardRects.rects[i]
	x, y := a.marqueeBox.X+r.X+r.W+3, a.marqueeBox.Y+r.Y+r.H/2-a.browseScroll.Y
	if !a.marqueeBlankAt(x, y) {
		t.Fatalf("(%.0f,%.0f) right of card %d is not blank (rect %+v)", x, y, i, r)
	}
	return x, y
}

// dragBand drags a band from (sx, sy) to (ex, ey) in window coordinates.
// The Tester's pointer events carry no modifier keys, so a band with Shift
// or Cmd is fed to the handler as the platform delivers it: events with Mods.
func dragBand(a *appState, te *ui.Tester, mods ui.Modifiers, sx, sy, ex, ey float32) {
	ev := func(kind ui.InputKind, x, y float32) ui.InputEvent {
		return ui.InputEvent{Kind: kind, Button: 0, Mods: mods, X: x - a.columnX, Y: y - a.columnY}
	}
	a.marqueeInput(ev(ui.InputPointerDown, sx, sy))
	a.marqueeInput(ev(ui.InputPointerMove, sx+(ex-sx)/10, sy+(ey-sy)/10+6))
	a.marqueeInput(ev(ui.InputPointerMove, ex, ey))
	te.Frame()
	a.marqueeInput(ev(ui.InputPointerUp, ex, ey))
	te.Frame()
}

func TestMarqueeModifiersAddAndFlip(t *testing.T) {
	a := marqueeApp(0, 40)
	te := marqueeTester(t, a)
	x1, y1 := cardCenter(a, 1)
	dragBand(a, te, 0, a.marqueeBox.X+1, a.marqueeBox.Y+1, x1, y1)
	if got := selectedIndexes(a); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("plain band selected %v", got)
	}
	// Shift adds to the selection the press found.
	gx3, gy3 := gapRightOf(t, a, 3)
	x4, y4 := cardCenter(a, 4)
	dragBand(a, te, ui.Shift, gx3, gy3, x4, y4)
	if got := selectedIndexes(a); !slices.Equal(got, []int{0, 1, 4}) {
		t.Fatalf("shift band selected %v, want 0 1 4", got)
	}
	// Cmd flips what the band touches: 1 goes off, 2 and 3 come on.
	dragBand(a, te, ui.Cmd, gx3, gy3, x1, y1)
	if got := selectedIndexes(a); !slices.Equal(got, []int{0, 2, 3, 4}) {
		t.Fatalf("cmd band selected %v, want 0 2 3 4", got)
	}
}

func TestMarqueeClickOnBlankClearsAndCardKeepsItsClick(t *testing.T) {
	a := marqueeApp(0, 12)
	te := marqueeTester(t, a)
	x0, y0 := cardCenter(a, 0)
	// A press on a card is not the band's: the card chooses itself.
	te.Press(x0, y0)
	if a.marquee.down {
		t.Fatal("a press on a card started a band")
	}
	te.Release(x0, y0)
	te.Frame()
	if got := selectedIndexes(a); !slices.Equal(got, []int{0}) || a.Chosen != 0 {
		t.Fatalf("card click selected %v chosen %d", got, a.Chosen)
	}
	// A click on empty space clears the selection.
	sx, sy := a.marqueeBox.X+1, a.marqueeBox.Y+1
	te.Press(sx, sy)
	if !a.marquee.down {
		t.Fatalf("press on blank space (%.0f,%.0f) not taken", sx, sy)
	}
	te.Release(sx, sy)
	te.Frame()
	if n := len(selectedIndexes(a)); n != 0 || a.Chosen != -1 {
		t.Fatalf("blank click left %d selected, chosen %d", n, a.Chosen)
	}
}

// A drag that starts on a card is the card's (the file drag that moves it):
// no band, and the card keeps the press.
func TestMarqueeDoesNotTakeADragFromACard(t *testing.T) {
	a := marqueeApp(0, 12)
	te := marqueeTester(t, a)
	x2, y2 := cardCenter(a, 2)
	te.Press(x2, y2)
	te.Move(x2+30, y2+30)
	te.Move(x2+60, y2+60)
	if a.marquee.down || a.marquee.active {
		t.Fatal("dragging a card started a band")
	}
	if n := len(selectedIndexes(a)); n != 0 {
		t.Fatalf("dragging a card selected %d cards", n)
	}
}

func TestMarqueeCancelRestoresSelection(t *testing.T) {
	a := marqueeApp(0, 20)
	te := marqueeTester(t, a)
	a.chooseOnly(5)
	x1, y1 := cardCenter(a, 1)
	sx, sy := a.marqueeBox.X+1, a.marqueeBox.Y+1
	te.Press(sx, sy)
	te.Move(sx+10, sy+10)
	te.Move(x1, y1)
	te.Frame()
	if got := selectedIndexes(a); slices.Contains(got, 5) || !slices.Contains(got, 1) {
		t.Fatalf("band selected %v", got)
	}
	te.CancelPress()
	te.Frame()
	if got := selectedIndexes(a); !slices.Equal(got, []int{5}) {
		t.Fatalf("a canceled band left %v, want the old selection 5", got)
	}
	if a.marquee.down {
		t.Fatal("canceled band still down")
	}
}

func TestMarqueeAutoScrollsAtTheEdgeAndSelectsFurther(t *testing.T) {
	a := marqueeApp(0, 300)
	te := marqueeTester(t, a)
	box := a.marqueeBox
	sx, sy := box.X+1, box.Y+1
	te.Press(sx, sy)
	te.Move(sx+10, sy+10)
	te.Move(box.X+300, box.Y+box.H-4) // inside the bottom edge zone
	for i := 0; i < 80; i++ {
		te.Advance(16 * 1000 * 1000)
		te.Frame()
	}
	if a.browseScroll.Y < 200 {
		t.Fatalf("the view did not scroll while the band rested at the edge: y=%.0f", a.browseScroll.Y)
	}
	te.Release(box.X+300, box.Y+box.H-4)
	te.Frame()
	got := selectedIndexes(a)
	if len(got) < 20 {
		t.Fatalf("auto-scrolled band selected only %d cards", len(got))
	}
	// Cards the band scrolled past are in the selection, not only the
	// ones on screen now.
	if got[0] != 0 {
		t.Fatalf("first selected card is %d, want 0", got[0])
	}
}

func TestMarqueeListSelectsRowsFromBlankSpaceBelow(t *testing.T) {
	a := marqueeApp(1, 5)
	te := marqueeTester(t, a)
	last := a.cardRects.rects[4]
	box := a.marqueeBox
	sx, sy := box.X+200, box.Y+last.Y+last.H+30
	if !a.marqueeBlankAt(sx, sy) {
		t.Fatalf("(%.0f,%.0f) under the last row is not blank", sx, sy)
	}
	// The rows and the header are not blank: they keep their clicks.
	if a.marqueeBlankAt(box.X+200, box.Y+a.cardRects.rects[1].Y+4) || a.marqueeBlankAt(box.X+200, box.Y+4) {
		t.Fatal("a row or the header counts as blank")
	}
	r2 := a.cardRects.rects[2]
	te.Press(sx, sy)
	te.Move(sx+5, sy-10)
	te.Move(sx, box.Y+r2.Y+r2.H/2)
	te.Frame()
	te.Release(sx, box.Y+r2.Y+r2.H/2)
	te.Frame()
	if got := selectedIndexes(a); !slices.Equal(got, []int{2, 3, 4}) {
		t.Fatalf("list band selected %v, want rows 2-4", got)
	}
}

// The band is an overlay that never takes the pointer, with a stable key.
func TestMarqueeBandIsDrawnAndReleasesCleanly(t *testing.T) {
	a := marqueeApp(0, 20)
	te := marqueeTester(t, a)
	x1, y1 := cardCenter(a, 1)
	sx, sy := a.marqueeBox.X+1, a.marqueeBox.Y+1
	before := te.Image().At(int(sx+30), int(sy+30))
	te.Press(sx, sy)
	te.Move(sx+10, sy+10)
	te.Move(x1, y1)
	te.Frame()
	te.Frame()
	if !a.marquee.active {
		t.Fatal("band not active")
	}
	var accent ui.Color
	probe := ui.NewTester(func(c *ui.Context) { accent = c.Theme().Accent }, 10, 10)
	probe.Frame()
	_ = accent
	if after := te.Image().At(int(sx+30), int(sy+30)); after == before {
		t.Fatalf("no band painted at (%.0f,%.0f): %v", sx+30, sy+30, after)
	}
	te.Release(x1, y1)
	te.Frame()
	te.Frame()
	if after := te.Image().At(int(sx+30), int(sy+30)); after == before && len(selectedIndexes(a)) == 0 {
		t.Fatal("nothing selected after the band")
	}
	if a.marquee.active {
		t.Fatal("band active after release")
	}
}

func TestMarqueeSelectPureRules(t *testing.T) {
	base := []string{"a", "b"}
	if got := marqueeSelect(base, []string{"b", "c"}, 0); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("plain %v", got)
	}
	if got := marqueeSelect(base, []string{"b", "c"}, ui.Shift); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("shift %v", got)
	}
	if got := marqueeSelect(base, []string{"b", "c"}, ui.Cmd); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("cmd %v", got)
	}
}
