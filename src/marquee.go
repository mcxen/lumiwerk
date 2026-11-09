package main

import (
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
)

// Rubber-band (marquee) selection.
//
// A left press on empty space of the browse area, dragged, draws a band;
// every card or row it touches is selected, as in Explorer. Shift adds to
// the selection the press found, Cmd flips the touched items against
// it. Near the top or bottom edge the view scrolls on its own. A press on a
// card is not ours: the card keeps its click, its file drag and the
// Finder-style selection chrome.
//
// Where the pointer is taken. The main column's HandleInput (handleGesture)
// is the innermost input handler over the browse area, so it sees a press
// before the card under it; marqueeInput answers true only for a press that
// lands on blank space, so a press on a card falls through untouched.
//
// What "touches" means. Cards are laid out by mygo (the grid and the table
// are widgets of its own), so each frame the cards it built report their
// layout bounds and noteCardRect keeps them in content coordinates (x from
// the browse box, y from the top of the scrolled content). Cards that
// scrolled away keep their rect until the layout changes, so a band taller
// than the window still reaches them. The band is a stable-key overlay
// (marquee-band) above the layout that never takes the pointer.

const (
	marqueeThreshold = 4                      // DIPs before a press becomes a band
	marqueeEdge      = 36                     // auto-scroll zone at the top and bottom
	marqueeMaxSpeed  = 1400                   // DIPs per second at the very edge
	marqueeHold      = 350 * time.Millisecond // touch: hold this long, then drag, to band
	marqueeTick      = time.Second / 60
)

type marqueeState struct {
	down   bool // a left press on blank space is in progress
	active bool // it moved far enough: the band shows and selects
	scroll bool // a touch that moved too early: it scrolls instead
	mods   ui.Modifiers
	downAt time.Time
	// sx, sy is the press point: x from the browse box, y in content.
	sx, sy float32
	// px, py is the pointer from the browse box's corner, in the viewport.
	px, py float32
	lastY  float32 // touch scroll: pointer y at the last move
	base   []string
	hits   []int
	tick   time.Time
}

// cardRectKey identifies a layout; cached rects are dropped when it changes.
type cardRectKey struct {
	gen   uint64
	mode  int
	n     int
	thumb float64
	w     float32
	label float32
}

type cardRects struct {
	key   cardRectKey
	rects map[int]ui.Rect
	// seq counts browse builds. The list's rects are relative to its view,
	// which scrolls on its own, so they live one build.
	seq, at int
}

func (a *appState) cardRectKeyNow() cardRectKey {
	return cardRectKey{a.layoutGen, a.LayoutMode, len(a.Visible), a.Thumb, a.marqueeBox.W, flowLabel}
}

// browseOffset is how far the scroll container is scrolled, 0 for the list:
// its table keeps its own scroll state, not a.browseScroll.
func (a *appState) browseOffset() float32 {
	if a.LayoutMode == 1 {
		return 0
	}
	return a.browseScroll.Y
}

// noteMarqueeBox records the scroll surface of the browse view, in window
// coordinates. The four layouts call it with their scroll container's bounds.
func (a *appState) noteMarqueeBox(b ui.Rect) {
	if b.W < 2 || b.H < 2 {
		return
	}
	a.marqueeBox = b
}

// noteCardRect records where card i sits in content coordinates. A list row
// is made of several cells: it spans the box, so only its y range is kept.
func (a *appState) noteCardRect(i int, el ui.Element) {
	b := el.LayoutBounds()
	box := a.marqueeBox
	if b.W < 1 || b.H < 1 || box.W < 2 {
		return
	}
	if key := a.cardRectKeyNow(); a.cardRects.rects == nil || a.cardRects.key != key ||
		(a.LayoutMode == 1 && a.cardRects.at != a.cardRects.seq) {
		a.cardRects = cardRects{key: key, rects: make(map[int]ui.Rect, 64), seq: a.cardRects.seq, at: a.cardRects.seq}
	}
	r := ui.Rect{X: b.X - box.X, Y: b.Y - box.Y + a.laidScrollY, W: b.W, H: b.H}
	if a.LayoutMode == 1 {
		old, seen := a.cardRects.rects[i]
		top, bottom := r.Y, r.Y+r.H
		if seen {
			top, bottom = min(top, old.Y), max(bottom, old.Y+old.H)
		}
		r = ui.Rect{X: 0, Y: top, W: box.W, H: bottom - top}
	}
	a.cardRects.rects[i] = r
}

// marqueeBlankAt reports whether the window point (x, y) is empty space of
// the browse area: inside the box, over no card. In the list the rows span
// the full width, so only the space under the last row is blank (the header
// and the rows keep their own clicks).
func (a *appState) marqueeBlankAt(x, y float32) bool {
	box := a.marqueeBox
	if box.W < 2 || box.H < 2 || len(a.Visible) == 0 {
		return false
	}
	if x < box.X || y < box.Y || x >= box.X+box.W || y >= box.Y+box.H {
		return false
	}
	if a.cardRects.rects == nil || a.cardRects.key != a.cardRectKeyNow() {
		return false
	}
	cx, cy := x-box.X, y-box.Y+a.browseOffset()
	if a.LayoutMode == 1 {
		last, ok := a.cardRects.rects[len(a.Visible)-1]
		return ok && cy >= last.Y+last.H
	}
	for _, r := range a.cardRects.rects {
		if cx >= r.X && cx < r.X+r.W && cy >= r.Y && cy < r.Y+r.H {
			return false
		}
	}
	return true
}

// marqueeBand is the band in content coordinates, normalized.
func (a *appState) marqueeBand() ui.Rect {
	m := &a.marquee
	ex, ey := m.px, m.py+a.browseOffset()
	x0, x1 := min(m.sx, ex), max(m.sx, ex)
	y0, y1 := min(m.sy, ey), max(m.sy, ey)
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// marqueeHits are the visible indexes whose card the band touches, in order.
func (a *appState) marqueeHits(band ui.Rect) []int {
	var hits []int
	for i, r := range a.cardRects.rects {
		if i < 0 || i >= len(a.Visible) {
			continue
		}
		if r.X < band.X+band.W && r.X+r.W > band.X && r.Y < band.Y+band.H && r.Y+r.H > band.Y {
			hits = append(hits, i)
		}
	}
	sort.Ints(hits)
	return hits
}

// marqueeSelect is the selection a band makes: the touched paths alone, or,
// with Shift, added to base, or, with Cmd, flipped against it.
func marqueeSelect(base, touched []string, mods ui.Modifiers) []string {
	switch {
	case mods&ui.Cmd != 0:
		set := make(map[string]bool, len(base))
		for _, p := range base {
			set[p] = true
		}
		for _, p := range touched {
			if set[p] {
				delete(set, p)
			} else {
				set[p] = true
			}
		}
		out := make([]string, 0, len(set))
		for _, p := range base {
			if set[p] {
				out = append(out, p)
				delete(set, p)
			}
		}
		for _, p := range touched {
			if set[p] {
				out = append(out, p)
				delete(set, p)
			}
		}
		return out
	case mods&ui.Shift != 0:
		out := slices.Clone(base)
		for _, p := range touched {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
		return out
	}
	return touched
}

// marqueeApply makes the selection follow the band.
func (a *appState) marqueeApply() {
	m := &a.marquee
	hits := a.marqueeHits(a.marqueeBand())
	if slices.Equal(hits, m.hits) {
		return
	}
	m.hits = hits
	touched := make([]string, 0, len(hits))
	for _, i := range hits {
		if item := a.visibleItem(i); item != nil {
			touched = append(touched, item.Path)
		}
	}
	a.Selected.Clear()
	for _, p := range marqueeSelect(m.base, touched, m.mods) {
		a.Selected.Add(p)
	}
	a.PendingSelect = nil
	if m.mods&(ui.Cmd|ui.Shift) != 0 && a.Chosen >= 0 {
		return // keep the chosen item the press found
	}
	// The chosen item (keyboard anchor) is the touched card nearest the pointer.
	best, bestD := -1, float32(0)
	ex, ey := m.px, m.py+a.browseOffset()
	for _, i := range hits {
		r := a.cardRects.rects[i]
		dx, dy := r.X+r.W/2-ex, r.Y+r.H/2-ey
		if d := dx*dx + dy*dy; best < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	a.Chosen, a.keyPivot = best, best
}

func (a *appState) marqueeCancel() {
	m := &a.marquee
	if m.active {
		a.Selected.Clear()
		for _, p := range m.base {
			a.Selected.Add(p)
		}
	}
	*m = marqueeState{}
}

// marqueeInput is the left-button half of the main column's input handler.
// It reports whether it took the event.
func (a *appState) marqueeInput(ev ui.InputEvent) bool {
	m := &a.marquee
	switch ev.Kind {
	case ui.InputKeyDown:
		if ev.Key == ui.KeyEscape && m.down {
			a.marqueeCancel()
			return true
		}
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		*m = marqueeState{}
		// Ctrl-click is the context menu on macOS: not a band.
		if a.InLargeView || a.galleryActive() || a.modalOpen() || a.Renaming || a.Root == "" || ev.Mods&ui.Ctrl != 0 {
			return false
		}
		wx, wy := a.columnX+ev.X, a.columnY+ev.Y
		if !a.marqueeBlankAt(wx, wy) {
			return false
		}
		box := a.marqueeBox
		m.down = true
		m.mods = ev.Mods
		m.downAt = ev.Time
		m.px, m.py = wx-box.X, wy-box.Y
		m.sx, m.sy = m.px, m.py+a.browseOffset()
		m.lastY = m.py
		m.tick = time.Time{}
		for p := range a.Selected.All() {
			m.base = append(m.base, p)
		}
		if ev.Pointer != ui.PointerTouch {
			// Mouse and pen: drag at once. Touch waits for a hold.
			m.downAt = time.Time{}
		}
		return true
	case ui.InputPointerMove:
		if !m.down || ev.Button != 0 {
			return false
		}
		box := a.marqueeBox
		m.px, m.py = a.columnX+ev.X-box.X, a.columnY+ev.Y-box.Y
		if ev.Mods != m.mods {
			// Shift or Cmd pressed or let go during the drag: the band
			// follows the keys as held now.
			m.mods = ev.Mods
			m.hits = nil
		}
		if m.scroll {
			a.scrollBrowseBy(m.lastY - m.py)
			m.lastY = m.py
			return true
		}
		if !m.active {
			if abs32(m.px-m.sx) < marqueeThreshold && abs32(m.py+a.browseOffset()-m.sy) < marqueeThreshold {
				return true
			}
			if !m.downAt.IsZero() && ev.Time.Sub(m.downAt) < marqueeHold {
				// A finger that moves before the hold scrolls the view.
				m.scroll = true
				return true
			}
			m.active = true
		}
		a.marqueeApply()
		return true
	case ui.InputPointerUp:
		if !m.down || ev.Button != 0 {
			return false
		}
		if ev.Canceled {
			a.marqueeCancel()
			return true
		}
		wasActive, mods := m.active, ev.Mods
		*m = marqueeState{}
		if !wasActive && mods&(ui.Cmd|ui.Shift) == 0 {
			// A click on empty space clears the selection, as in Finder.
			a.PendingSelect = nil
			a.Selected.Clear()
			a.Chosen, a.keyPivot = -1, -1
		}
		return true
	}
	return false
}

// marqueeFrame runs at the end of every browse build: it scrolls the view
// while the pointer rests near an edge, keeps the layout bookkeeping the
// rects need and draws the band. c.After keeps frames coming while the
// pointer is held still at an edge.
func (a *appState) marqueeFrame(c *ui.Context) {
	m := &a.marquee
	a.laidScrollY = a.browseOffset()
	a.cardRects.seq++
	if !m.active || a.marqueeBox.W < 2 {
		return
	}
	box := a.marqueeBox
	band := a.marqueeBand()
	top := band.Y - a.browseOffset()
	left, right := max(band.X, 0), min(band.X+band.W, box.W)
	top, bottom := max(top, 0), min(top+band.H, box.H)
	if right <= left || bottom <= top {
		return
	}
	t := c.Theme()
	ui.Box(c.Key("marquee-band")).Absolute().
		Left(box.X-a.columnX+left).Top(box.Y-a.columnY+top).
		Width(right-left).Height(bottom-top).
		Background(t.Accent.Alpha(0.16)).Border(1, t.Accent.Alpha(0.75)).Radius(2).
		HitTest(func(float32, float32, ui.Rect) bool { return false })
}

func (a *appState) marqueeAutoScroll(c *ui.Context) {
	m := &a.marquee
	if !m.active || a.InLargeView {
		return
	}
	now := time.Now()
	dt := float32(0.016)
	if !m.tick.IsZero() {
		dt = float32(now.Sub(m.tick).Seconds())
	}
	m.tick = now
	dt = min(max(dt, 0.004), 0.05)
	box := a.marqueeBox
	var speed float32
	switch {
	case m.py < marqueeEdge:
		speed = -marqueeMaxSpeed * min((marqueeEdge-m.py)/marqueeEdge, 1)
	case m.py > box.H-marqueeEdge:
		speed = marqueeMaxSpeed * min((m.py-(box.H-marqueeEdge))/marqueeEdge, 1)
	}
	c.After(marqueeTick)
	if speed == 0 || a.LayoutMode == 1 {
		return // the table scrolls on its own state: no offset to drive
	}
	y := a.browseScroll.Y + speed*dt
	if y < 0 {
		y = 0
	}
	if max := a.browseScroll.MaxY; max > 0 && y > max {
		y = max
	}
	if y == a.browseScroll.Y {
		return
	}
	a.browseScroll.Y = y
	a.marqueeApply()
}

// impliedPicked reports that path lies under a selected folder while the
// listing is recursive: the files a folder expanded into count as selected
// with it. Operations take the folder alone (collapseNested).
func (a *appState) impliedPicked(path string) bool {
	if !a.Recursive || a.Selected.Len() == 0 || a.Root == "" {
		return false
	}
	root := filepath.Clean(a.Root)
	for dir := filepath.Dir(path); len(dir) > len(root) && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if a.Selected.Has(dir) {
			return true
		}
	}
	return false
}

// collapseNested drops every path that lies inside another path of the list:
// a folder and the files it expanded into are one target, not two (moving
// both would move the file twice). Order is kept.
func collapseNested(paths []string) []string {
	if len(paths) < 2 {
		return paths
	}
	set := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		set[filepath.Clean(p)] = struct{}{}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		covered := false
		for dir := filepath.Dir(filepath.Clean(p)); ; {
			if _, ok := set[dir]; ok {
				covered = true
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		if !covered {
			out = append(out, p)
		}
	}
	return out
}
