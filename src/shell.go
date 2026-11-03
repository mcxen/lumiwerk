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
	return mediaListState{Grid: &a.Grid, List: &a.ListState}
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
	if a.SidebarShown && !a.InLargeView {
		ui.Split(c, &a.SidebarWidth, func() { a.sidebarView(c) }, func() { a.browserShell(c) }).Fill()
	} else {
		a.browserShell(c)
	}
}

// settingsSection is the label/content group every settings pane is built
// from: a 14px bold title in a 120px column, then the fields, 12px apart as
// the spec's form rhythm (the pane itself carries the 24px padding).
func settingsSection(c *ui.Context, title string, body func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Text(c, title).Width(120).FontSize(14).FontWeight(600).TextColor(t.Text).Padding(4, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(body)
	})
}
