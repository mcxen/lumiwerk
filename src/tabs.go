package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

type tab struct {
	Root string
	Name string
	// History and Cursor are the tab's back/forward trail while another
	// tab is active; the active tab's live in appState.
	History []string
	Cursor  int
}

// enterTab makes tab i active, keeping the back/forward trail of the tab
// it leaves and taking up i's.
func (a *appState) enterTab(i int) {
	if from := a.Tabs.Active; from >= 0 && from < len(a.Tabs.Tabs) && from != i {
		a.Tabs.Tabs[from].History, a.Tabs.Tabs[from].Cursor = a.History, a.Cursor
	}
	a.Tabs.Active = i
	if i >= 0 && i < len(a.Tabs.Tabs) {
		a.mu.Lock()
		a.History, a.Cursor = a.Tabs.Tabs[i].History, a.Tabs.Tabs[i].Cursor
		a.mu.Unlock()
	}
}

type tabState struct {
	Tabs   []tab
	Active int
}

func (a *appState) openTab(path string) {
	a.Tabs.Tabs = append(a.Tabs.Tabs, tab{Root: path, Name: pathName(path), Cursor: -1})
	a.enterTab(len(a.Tabs.Tabs) - 1)
	a.openFolder(path)
}

// openInNewTab keeps the current folder as a tab, then opens path beside it.
func (a *appState) openInNewTab(path string) {
	if path == "" || !isDir(path) {
		return
	}
	if len(a.Tabs.Tabs) == 0 && a.Root != "" {
		a.Tabs.Tabs = append(a.Tabs.Tabs, tab{Root: a.Root, Name: pathName(a.Root), Cursor: -1})
		a.Tabs.Active = 0
	}
	a.openTab(path)
}

func (a *appState) noteTabRoot(path string) {
	if len(a.Tabs.Tabs) == 0 {
		return
	}
	if a.Tabs.Active < 0 || a.Tabs.Active >= len(a.Tabs.Tabs) {
		a.Tabs.Active = 0
	}
	a.Tabs.Tabs[a.Tabs.Active].Root = path
	a.Tabs.Tabs[a.Tabs.Active].Name = pathName(path)
}

func (a *appState) closeTab(i int) {
	if i < 0 || i >= len(a.Tabs.Tabs) {
		return
	}
	active := a.Tabs.Active
	if active >= 0 && active < len(a.Tabs.Tabs) && active != i {
		a.Tabs.Tabs[active].History, a.Tabs.Tabs[active].Cursor = a.History, a.Cursor
	}
	a.Tabs.Tabs = append(a.Tabs.Tabs[:i], a.Tabs.Tabs[i+1:]...)
	next := active
	if active > i {
		next = active - 1
	} else if next >= len(a.Tabs.Tabs) {
		next = len(a.Tabs.Tabs) - 1
	}
	a.Tabs.Active = -1
	if next >= 0 {
		a.enterTab(next)
		a.openFolder(a.Tabs.Tabs[next].Root)
	}
}

func (a *appState) switchTab(i int) {
	if i < 0 || i >= len(a.Tabs.Tabs) {
		return
	}
	a.enterTab(i)
	a.openFolder(a.Tabs.Tabs[i].Root)
}

func (a *appState) tabsBar(c *ui.Context) {
	if len(a.Tabs.Tabs) == 0 {
		return
	}
	t := c.Theme()
	leftPad := float32(10)
	if tb := c.TitleBar(); tb.Left > 0 && (!a.SidebarShown || a.InLargeView) {
		if pad := tb.Left - cardGap + 8; pad > leftPad {
			leftPad = pad
		}
	}
	rightPad := float32(10)
	if a.showWindowControls() {
		rightPad = float32(controlsWidth - cardGap + 8)
	}
	bar := ui.ScrollHorizontal(c.Key("tabs-bar")).FillWidth().Height(34).Gap(4).
		Padding(4, rightPad, 4, leftPad).Background(t.Background).AlignItems(ui.Center).DragWindow()
	bar.Children(func() {
		for i, tb := range a.Tabs.Tabs {
			i := i
			active := i == a.Tabs.Active
			tab := ui.Row(c.Key(fmt.Sprintf("tab-%d", i))).Height(26).Gap(6).Padding(0, 6, 0, 10).Radius(6).
				AlignItems(ui.Center).Shrink(0).Cursor(ui.CursorPointer).Tooltip(tb.Root)
			if active {
				tab.Background(t.Accent.Alpha(0.14)).TextColor(t.Accent)
			} else if tab.Hovered() {
				tab.Background(t.SurfaceHover)
			}
			closed := false
			tab.Children(func() {
				txt := ui.Text(c, tb.Name).FontSize(11.5).SingleLine().MaxLines(1).MaxWidth(160).Ellipsis("…")
				if active {
					txt.Bold().TextColor(t.Accent)
				}
				closeBtn := ui.ButtonBase(c.Key(fmt.Sprintf("tab-close-%d", i))).Size(18, 18).Radius(4).Center().
					Cursor(ui.CursorPointer).Label("关闭标签页").Tooltip("关闭标签页")
				if active {
					closeBtn.TextColor(t.Accent)
					if closeBtn.Hovered() || closeBtn.Pressed() {
						closeBtn.Background(t.Accent.Alpha(0.16))
					}
				} else {
					closeBtn.TextColor(t.TextMuted)
					if closeBtn.Hovered() || closeBtn.Pressed() {
						closeBtn.Background(t.SurfacePressed).TextColor(t.Text)
					}
				}
				closeBtn.Children(func() { ui.Icon(c, lucideClose).Size(11, 11) })
				if closeBtn.Clicked() {
					closed = true
					a.closeTab(i)
				}
			})
			if !closed {
				tab.OnClick(func() { a.switchTab(i) })
			}
		}
	})
	ui.Divider(c)
}
