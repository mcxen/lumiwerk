package main

import (
	"github.com/egoist/mygo/ui"
)

// Keyboard navigation of the browser.
//
// The flow views never had keyboard navigation and the grid's came from the
// widget, which also painted the big accent box. Selection now moves in the
// app, the same in every layout: arrows move the chosen item, plain
// Left/Right step one place in flat order, Up/Down follow the visual
// column of the current layout, Shift extends from the last plain choice,
// and Home/End jump to the ends. Text fields keep the keys they consume, so
// typing in the search box still sees Left/Right for its caret.

// browseKeys reads this frame's navigation keys and moves the selection.
// It runs in every frame, also the ones with no keys, so the window keeps
// claiming them ahead of the scroll containers' line scrolling.
func (a *appState) browseKeys(c *ui.Context) {
	if a.Root == "" || a.InLargeView || a.modalOpen() || a.Renaming || a.NewFolder || a.Batch.Open {
		return
	}
	n := len(a.Visible)
	if n == 0 {
		return
	}
	key, extend, ok := browseArrow(c)
	if !ok {
		return
	}
	at := a.Chosen
	if at < 0 {
		at = 0 // first key lands on the first item
	}
	to := at
	switch key {
	case ui.KeyHome:
		to = 0
	case ui.KeyEnd:
		to = n - 1
	default:
		to = a.visualStep(at, key, n)
	}
	if to != at {
		a.revealFrames = 2
	}
	if extend {
		a.extendSelectionTo(to)
	} else {
		a.chooseOnly(to)
	}
}

// browseArrow reports which navigation key was pressed this frame, if any,
// and whether Shift was down for it.
func browseArrow(c *ui.Context) (ui.Key, bool, bool) {
	for _, k := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown} {
		if c.Shortcut(ui.Shift, k) {
			return k, true, true
		}
		if c.Shortcut(0, k) {
			return k, false, true
		}
	}
	if c.Shortcut(0, ui.KeyHome) {
		return ui.KeyHome, false, true
	}
	if c.Shortcut(0, ui.KeyEnd) {
		return ui.KeyEnd, false, true
	}
	return 0, false, false
}

// visualStep is where a directional key lands from the item at: Left/Right
// step in flat order everywhere; Up/Down follow the layout's visual shape —
// grid rows, list rows, the masonry column of a waterfall or the slot of a
// justified row.
func (a *appState) visualStep(at int, key ui.Key, n int) int {
	clamp := func(to int) int {
		return min(max(to, 0), n-1)
	}
	switch key {
	case ui.KeyLeft:
		return clamp(at - 1)
	case ui.KeyRight:
		return clamp(at + 1)
	}
	dir := 1
	if key == ui.KeyUp {
		dir = -1
	}
	switch a.LayoutMode {
	case 0:
		cols := a.browseBox.Cols
		if cols < 1 {
			cols = 1
		}
		return clamp(at + dir*cols)
	case 1:
		return clamp(at + dir)
	case 2, 3:
		if at >= 0 && at < n {
			if to, ok := a.flowColumnNeighbor(at, dir); ok {
				return to
			}
		}
		return clamp(at + dir)
	}
	return clamp(at + dir)
}

// flowColumnNeighbor finds the item in the column above (dir < 0) or below
// (dir > 0) the item at, nearest in horizontal order, in the waterfall and
// justified layouts.
func (a *appState) flowColumnNeighbor(at, dir int) (int, bool) {
	w := a.browseBox.W
	if w < 2 {
		return 0, false
	}
	var boxes []flowBox
	if a.LayoutMode == 2 {
		boxes = a.cachedWaterfall(w)
	} else {
		boxes = justifiedFrames(a.visibleAspects(), w, float32(a.Thumb))
	}
	if at < 0 || at >= len(boxes) {
		return 0, false
	}
	cur := boxes[at]
	if cur.Index != at {
		return 0, false
	}
	cx := cur.X + cur.W/2
	best := -1
	bestScore := float32(0)
	for _, b := range boxes {
		if b.Index == at || b.W <= 0 || b.H <= 0 {
			continue
		}
		var gap float32
		if dir > 0 {
			gap = b.Y - (cur.Y + cur.H)
		} else {
			gap = cur.Y - (b.Y + b.H)
		}
		if gap <= 0 {
			continue
		}
		// The column above or below wins by scale: horizontal distance
		// first, vertical distance breaks the tie.
		score := abs32(b.X+b.W/2-cx)*1000 + gap
		if best < 0 || score < bestScore {
			best, bestScore = b.Index, score
		}
	}
	return best, best >= 0
}

// extendSelectionTo makes the items between the last plain choice (the
// pivot, as a Shift-click's anchor) and to the selection, like mygo's
// Shift+arrow handling did.
func (a *appState) extendSelectionTo(to int) {
	n := len(a.Visible)
	if n == 0 {
		return
	}
	anchor := a.keyPivot
	if anchor < 0 || anchor >= n {
		anchor = a.Chosen
	}
	if anchor < 0 || anchor >= n {
		anchor = to
	}
	a.keyPivot = anchor
	a.Selected.Clear()
	for j := min(anchor, to); j <= max(anchor, to); j++ {
		if it := a.visibleItem(j); it != nil {
			a.Selected.Add(it.Path)
		}
	}
	a.Chosen = to
}
