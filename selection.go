package main

// Selection stability.
//
// a.Chosen is an index into a.Visible, and a.Selected holds paths. A rescan
// (after rename, delete, move, undo, an editor save or a watcher event)
// replaces Items and rebuilds Visible, so the raw index came to point at a
// different file, or past the end, which reset it to -1; Selected kept the
// path of a deleted file, so nothing showed as selected. The anchor below
// records the chosen path and its index before Visible is rebuilt and maps
// the selection back by path afterwards.

type selectionAnchor struct {
	set   bool
	path  string
	index int
}

// noteSelectionAnchorLocked records the chosen item before Items is
// replaced, while a.Visible still indexes the old Items.
func (a *appState) noteSelectionAnchorLocked() {
	a.selAnchor = selectionAnchor{set: true, path: "", index: a.Chosen}
	if it := a.visibleItem(a.Chosen); it != nil {
		a.selAnchor.path = it.Path
	}
}

// takeSelectionAnchorLocked returns the anchor noted before a rescan, or the
// current chosen item when Visible is rebuilt over the same Items (a filter
// or search change).
func (a *appState) takeSelectionAnchorLocked() selectionAnchor {
	if a.selAnchor.set {
		an := a.selAnchor
		a.selAnchor = selectionAnchor{}
		return an
	}
	an := selectionAnchor{set: true, index: a.Chosen}
	if it := a.visibleItem(a.Chosen); it != nil {
		an.path = it.Path
	}
	return an
}

// reconcileSelectionLocked maps the selection onto the rebuilt Visible:
// selected paths that are gone are dropped; the chosen item keeps its path
// at its new index; a chosen item that is gone hands over to the item now
// at its place (the next one), or the last one when it was last.
func (a *appState) reconcileSelectionLocked(an selectionAnchor) {
	where := make(map[string]int, len(a.Visible))
	for vi, ii := range a.Visible {
		where[a.Items[ii].Path] = vi
	}
	var gone []string
	for p := range a.Selected.All() {
		if _, ok := where[p]; !ok {
			gone = append(gone, p)
		}
	}
	for _, p := range gone {
		a.Selected.Remove(p)
	}
	switch {
	case len(a.Visible) == 0:
		a.Chosen = -1
		return
	case an.path != "":
		if vi, ok := where[an.path]; ok {
			a.Chosen = vi
		} else if an.index >= 0 {
			a.Chosen = min(an.index, len(a.Visible)-1)
			if a.Selected.Len() == 0 {
				a.revealFrames = 2
			}
		} else {
			a.Chosen = -1
		}
	case a.Chosen >= len(a.Visible):
		a.Chosen = -1
	}
	if a.Chosen >= 0 && a.Selected.Len() == 0 {
		a.Selected.Add(a.Items[a.Visible[a.Chosen]].Path)
	}
}

// selectAfterLoad chooses path once the folder being loaded lists it, as
// the folder just left after going up or back.
func (a *appState) selectAfterLoad(path string) {
	if path == "" {
		return
	}
	a.mu.Lock()
	a.PendingSelect = []string{path}
	a.mu.Unlock()
}

// takeReveal reports whether the view should scroll the chosen item into
// view in this frame.
func (a *appState) takeReveal() bool {
	if a.revealFrames <= 0 || a.Chosen < 0 {
		return false
	}
	a.revealFrames--
	return true
}

// revealFlow scrolls a flow layout so the chosen box shows.
func (a *appState) revealFlow(boxes []flowBox, viewH float32) {
	if !a.takeReveal() || viewH <= 1 {
		return
	}
	for _, b := range boxes {
		if b.Index != a.Chosen {
			continue
		}
		if b.Y < a.browseScroll.Y {
			a.browseScroll.Y = max(0, b.Y-flowPad)
		} else if b.Y+b.H > a.browseScroll.Y+viewH {
			a.browseScroll.Y = b.Y + b.H + flowPad - viewH
		}
		return
	}
}
