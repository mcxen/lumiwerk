package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// mediaCardState holds one photo and one video in a subfolder, so every
// layout's card has a name, a duration badge and (in the waterfall) a
// source-folder badge to draw.
func mediaCardState(layout int) *appState {
	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: 128, Root: "/probe", LayoutMode: layout}
	a.Items = []mediaItem{
		{Path: "/probe/a.jpg", Name: "a.jpg", Ext: "jpg", InfoLoaded: true, Info: mediaInfo{Width: 800, Height: 600}, Decoded: true},
		{Path: "/probe/sub/b.mp4", Name: "b.mp4", Ext: "mp4", IsVideo: true, InfoLoaded: true, Info: mediaInfo{Width: 640, Height: 360, DurationSec: 65}, Decoded: true},
	}
	a.Visible = []int{0, 1}
	return a
}

// TestMediaCardsShareChromeAcrossLayouts covers the card shared by the grid
// and the justified/waterfall layouts: names and badges show, a click
// chooses the card, and only the waterfall shows the source folder.
func TestMediaCardsShareChromeAcrossLayouts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct {
		name       string
		layout     int
		folderChip bool
	}{
		{"grid", 0, false},
		{"waterfall", 2, true},
		{"justified", 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mediaCardState(tc.layout)
			te := ui.NewTester(a.view, 1000, 700)
			te.Frame()
			te.Frame()
			for _, want := range []string{"a.jpg", "b.mp4", formatClock(65)} {
				if !te.HasText(want) {
					t.Fatalf("missing %q: %v", want, te.Texts())
				}
			}
			if got := te.HasText("sub"); got != tc.folderChip {
				t.Fatalf("folder chip shown = %v, want %v: %v", got, tc.folderChip, te.Texts())
			}
			// The grid card's tooltip carries the name, so Find may
			// return the whole card; its center is on the card either way.
			r, ok := te.Find("b.mp4")
			if !ok {
				t.Fatalf("card name not found: %v", te.Texts())
			}
			te.ClickAt(r.X+r.W/2, r.Y+r.H/2)
			te.Frame()
			if a.Chosen != 1 || !a.Selected.Has("/probe/sub/b.mp4") {
				t.Fatalf("click chose %d selected %d", a.Chosen, a.Selected.Len())
			}
		})
	}
}

// TestOnlyGridCardArmsSlowRename checks the grid-only rename option does not
// leak into the justified card: a click on the chosen card's name arms a
// slow rename only in the grid (the justified layout renames by dialog).
func TestOnlyGridCardArmsSlowRename(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct {
		name   string
		layout int
		grid   bool
	}{
		{"grid", 0, true},
		{"justified", 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mediaCardState(tc.layout)
			a.Chosen = 0
			a.Selected.Add("/probe/a.jpg")
			te := ui.NewTester(a.view, 1000, 700)
			te.Frame()
			te.Frame()
			r, ok := te.Find("a.jpg")
			if !ok {
				t.Fatalf("name not shown: %v", te.Texts())
			}
			// Find may return the whole grid card (its tooltip has the
			// name); the label sits along the card's bottom edge.
			te.ClickAt(r.X+r.W/2, r.Y+r.H-6)
			te.Frame()
			if armed := a.renameArm == "/probe/a.jpg"; armed != tc.grid {
				t.Fatalf("slow rename armed = %v (%q), want %v", armed, a.renameArm, tc.grid)
			}
		})
	}
}

// A file with no picture (a .dmg, a .html…) draws its type icon; on a large
// grid card it must grow with the card instead of staying a 28-DIP speck.
func TestOtherTypeGlyphGrowsWithLargeGridCards(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	inkHeight := func(thumb float64) int {
		a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: thumb, Root: "/probe"}
		a.Items = []mediaItem{{Path: "/probe/x.dmg", Name: "x.dmg", Ext: "dmg", InfoLoaded: true}}
		a.Visible = []int{0}
		side := float32(thumb)
		te := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Size(side, side+flowLabel).Children(func() {
				a.mediaCard(c, a.Root, 0, mediaCardOptions{fit: ui.Contain, tagDots: true, inlineRename: true, tooltip: true})
			})
		}, int(side), int(side+flowLabel))
		te.Frame()
		te.Frame()
		img := te.Image()
		name, ok := te.Find("x.dmg")
		if !ok {
			t.Fatalf("card name missing: %v", te.Texts())
		}
		bg := img.At(1, 1)
		br, bgG, bb, _ := bg.RGBA()
		top, bottom := -1, -1
		b := img.Bounds()
		for y := b.Min.Y; y < int(name.Y)-2; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				d := absDiff(r, br) + absDiff(g, bgG) + absDiff(bl, bb)
				if d > 3*0x2800 {
					if top < 0 {
						top = y
					}
					bottom = y
					break
				}
			}
		}
		if top < 0 {
			t.Fatalf("thumb %v: no glyph drawn", thumb)
		}
		return bottom - top + 1
	}
	small, large := inkHeight(96), inkHeight(320)
	if large < small*2 {
		t.Fatalf("glyph ink height small=%d large=%d; the icon must scale with the card", small, large)
	}
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
