package main

import "github.com/egoist/mygo/ui"

// cardPointer gives a media card the grid's click contract. The card is a
// file drag source, and a drag source takes the press, so the grid cell
// under it never sees the click or the double click: clicking did not
// select and double-clicking a folder did not enter it. A plain click
// chooses the item, Cmd toggles it, Shift extends from the chosen item,
// and a double click opens it (a folder is entered).
func (a *appState) cardPointer(card ui.Element, i int) {
	item := a.visibleItem(i)
	if item == nil {
		return
	}
	if card.DoubleClicked() {
		a.chooseOnly(i)
		a.openItem(i)
		return
	}
	if !card.Clicked() {
		return
	}
	mods := card.ClickModifiers()
	switch {
	case mods&ui.Cmd != 0:
		if a.Selected.Has(item.Path) {
			a.Selected.Remove(item.Path)
		} else {
			a.Selected.Add(item.Path)
		}
		a.Chosen = i
	case mods&ui.Shift != 0 && a.Chosen >= 0:
		lo, hi := min(a.Chosen, i), max(a.Chosen, i)
		a.Selected.Clear()
		for j := lo; j <= hi; j++ {
			if it := a.visibleItem(j); it != nil {
				a.Selected.Add(it.Path)
			}
		}
		a.Chosen = i
	default:
		a.chooseOnly(i)
	}
}

// chooseOnly makes item i the chosen and only selected item.
func (a *appState) chooseOnly(i int) {
	item := a.visibleItem(i)
	if item == nil {
		return
	}
	a.Chosen = i
	a.Selected.Clear()
	a.Selected.Add(item.Path)
}
