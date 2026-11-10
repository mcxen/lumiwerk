package main

import (
	"github.com/egoist/mygo/ui"
)

func safeView(scope string, c *ui.Context, body func()) {
	defer logRecover(scope)
	body()
}

// mediaListState returns the grid and list states for a media collection.
// Keys are always strings, matching Selection[string], and keep one element
// identity per media path through re-sorts. The widgets' own selection
// (Selected/Selection) is deliberately not wired: their chrome is the big
// accent box, and selection stays a single model in appState, drawn by the
// views as a quiet neutral fill and driven by browseKeys.
type mediaListState struct {
	Grid *ui.GridState
	List *ui.ListState
}

func (a *appState) mediaListState() mediaListState {
	a.Grid.Key = a.itemKey
	a.Grid.Label = a.itemLabel
	a.ListState.Key = a.itemKey
	a.ListState.Label = a.itemLabel
	a.ListState.DragRows = a.listDragRows
	return mediaListState{Grid: &a.Grid, List: &a.ListState}
}

func (a *appState) listDragRows(rows []int) any {
	if len(rows) == 0 {
		return nil
	}
	item := a.visibleItem(rows[0])
	if item == nil {
		return nil
	}
	paths := a.dragPaths(item.Path)
	if len(paths) == 0 {
		return nil
	}
	return paths
}

func (a *appState) itemLabel(i int) string {
	if item := a.visibleItem(i); item != nil {
		return item.Name
	}
	return ""
}

func (a *appState) browserShell(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.mainView(c)
		a.renameDialog(c)
		a.folderDialog(c)
		a.batchDialog(c)
		a.zipDialog(c)
		a.cropDialog(c)
		a.trimDialog(c)
		a.infoDialog(c)
		a.autoConfigDialog(c)
	})
	a.heroFlyer(c)
}

func (a *appState) browserLayout(c *ui.Context) {
	if a.SidebarWidth == 0 {
		a.SidebarWidth = sidebarDefaultWidth
	}
	pal := shellPaletteFor(c)
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	a.paintBackdrop(c, row)
	row.Children(func() {
		if !a.SidebarShown {
			a.contentCard(c, pal)
			return
		}
		// The large view hides the sidebar by folding its pane to zero inside
		// the same Split instead of dropping the Split: rebuilding the tree on
		// Esc made browserShell a new element whose first frame had no layout
		// width, so the grid laid out narrow for a frame or two, then jumped
		// back. A throwaway size keeps the user's sidebar width untouched.
		size := &a.SidebarWidth
		if a.InLargeView {
			a.largeSidebarFold = 0
			size = &a.largeSidebarFold
		}
		ui.Split(c, size, func() {
			if !a.InLargeView {
				a.sidebarView(c)
			}
		}, func() { a.contentCard(c, pal) }).Fill()
	})
}

// contentCard floats the browser in one rounded card over the wash. The gap
// around it moves the window like the bars do.
func (a *appState) contentCard(c *ui.Context, pal *shellPalette) {
	ui.Column(c).Fill().Padding(cardGap).DragWindow().Children(func() {
		cardSurface(ui.Column(c).Grow(1).MinHeight(0).MinWidth(0), pal).Children(func() {
			a.browserShell(c)
		})
	})
}
