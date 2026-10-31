package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

type tab struct {
	Root string
	Name string
}

type tabState struct {
	Tabs   []tab
	Active int
}

func (a *appState) openTab(path string) {
	a.Tabs.Tabs = append(a.Tabs.Tabs, tab{Root: path, Name: pathName(path)})
	a.Tabs.Active = len(a.Tabs.Tabs) - 1
	a.openFolder(path)
}

// openInNewTab keeps the current folder as a tab, then opens path beside it.
func (a *appState) openInNewTab(path string) {
	if path == "" || !isDir(path) {
		return
	}
	if len(a.Tabs.Tabs) == 0 && a.Root != "" {
		a.Tabs.Tabs = append(a.Tabs.Tabs, tab{Root: a.Root, Name: pathName(a.Root)})
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
	a.Tabs.Tabs = append(a.Tabs.Tabs[:i], a.Tabs.Tabs[i+1:]...)
	if a.Tabs.Active >= len(a.Tabs.Tabs) {
		a.Tabs.Active = len(a.Tabs.Tabs) - 1
	}
	if a.Tabs.Active >= 0 {
		a.openFolder(a.Tabs.Tabs[a.Tabs.Active].Root)
	}
}

func (a *appState) switchTab(i int) {
	if i < 0 || i >= len(a.Tabs.Tabs) {
		return
	}
	a.Tabs.Active = i
	a.openFolder(a.Tabs.Tabs[i].Root)
}

func (a *appState) tabsBar(c *ui.Context) {
	if len(a.Tabs.Tabs) == 0 {
		return
	}
	t := c.Theme()
	bar := ui.Row(c).Gap(4).Padding(4, 6).Background(t.Surface)
	bar.Children(func() {
		for i, tb := range a.Tabs.Tabs {
			i := i
			tab := ui.Row(c).Gap(4).Padding(4, 10).Radius(4)
			if i == a.Tabs.Active {
				tab.Background(t.Accent).TextColor(t.AccentText)
			}
			tab.Children(func() {
				ui.Text(c, tb.Name).FontSize(11).MaxLines(1)
				if ui.Button(c.Key(fmt.Sprintf("tab-close-%d", i)), "×").Clicked() {
					a.closeTab(i)
				}
			})
			tab.OnClick(func() { a.switchTab(i) })
		}
	})
}
