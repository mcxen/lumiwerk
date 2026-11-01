package main

import (
	"github.com/egoist/mygo/ui"
)

func safeView(scope string, c *ui.Context, body func()) {
	defer logRecover(scope)
	body()
}

// mediaListState is the single selection contract for every media collection.
// Grid and list keys are always strings, matching Selection[string].
type mediaListState struct {
	Grid *ui.GridState
	List *ui.ListState
}

func (a *appState) mediaListState() mediaListState {
	a.Grid.Key = a.itemKey
	a.Grid.Selected = &a.Chosen
	a.Grid.Selection = &a.Selected
	a.Grid.Label = a.itemLabel
	a.ListState.Key = a.itemKey
	a.ListState.Selected = &a.Chosen
	a.ListState.Selection = &a.Selected
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

func settingsSection(c *ui.Context, title string, body func()) {
	ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Text(c, title).Width(100).FontSize(13).Bold().Padding(5, 0)
		ui.Column(c).Grow(1).Gap(12).Children(body)
	})
}
