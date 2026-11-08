// Settings window: a slim left nav and, on the right, the active page as a
// stack of rounded cards on a quiet background (see settings_form.go). The
// pages only hold preferences that exist nowhere else; anything the toolbar
// or the View menu already changes live is not repeated here.
package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingsPage struct {
	id    string
	title string
	icon  *ui.SVG
}

var settingsPages = []settingsPage{
	{"general", "通用", lucideSliders},
	{"browse", "浏览与播放", lucideImage},
	{"files", "文件", lucideFolder},
	{"keys", "快捷键", lucideKeyboard},
	{"advanced", "高级", lucideTerminal},
}

func (a *appState) settingsWindow() {
	if a.Settings.Window != nil {
		a.Settings.Window.Show()
		a.Settings.Window.Focus()
		return
	}
	a.openSettings()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:       "设置",
		Width:       780,
		Height:      600,
		MinWidth:    680,
		MinHeight:   460,
		StateKey:    "settings",
		Content:     ui.View(a.settingsView),
		AlwaysOnTop: true,
		Hidden:      true,
	})
	w.OnReadyToShow(w.Show)
	a.Settings.Window = w
	w.OnClosed(func() {
		a.Settings.Window = nil
		a.Settings.Open = false
	})
}

func (a *appState) settingsView(c *ui.Context) {
	safeView("settings", c, func() { a.settingsBody(c) })
}

func (a *appState) settingsPageID() string {
	if a.Settings.Pane < 0 || a.Settings.Pane >= len(settingsPages) {
		return "general"
	}
	return settingsPages[a.Settings.Pane].id
}

func (a *appState) setPage(id string) {
	for i, p := range settingsPages {
		if p.id == id {
			a.Settings.Pane = i
			return
		}
	}
}

// settingsWindowColor is the page background: cards sit on it.
func settingsWindowColor(pal *settingsPalette) ui.Color {
	if pal.dark {
		return pal.background
	}
	return pal.surface
}

func (a *appState) settingsBody(c *ui.Context) {
	pal := settingsPaletteFor(c)
	c.SetTheme(settingsTheme(c, pal))
	page := settingsPages[0]
	for _, p := range settingsPages {
		if p.id == a.settingsPageID() {
			page = p
		}
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(settingsWindowColor(pal)).Children(func() {
		ui.Column(c).Width(184).Shrink(0).Padding(14, 10, 12, 10).Gap(2).Children(func() {
			for _, p := range settingsPages {
				settingsNavRow(c, a, p, pal)
			}
			ui.Spacer(c)
			ui.Text(c, "Lumiwerk "+appVersion).FontSize(11.5).TextColor(pal.textFaint).Padding(8, 10)
		})
		ui.Scroll(c.Key("page-"+page.id)).Grow(1).MinWidth(0).Padding(18, 24, 28, 14).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(580).Gap(22).Children(func() {
				ui.Text(c, page.title).FontSize(20).FontWeight(600).TextColor(pal.text)
				switch page.id {
				case "general":
					a.settingsGeneralPage(c, pal)
				case "browse":
					a.settingsBrowsePage(c, pal)
				case "files":
					a.settingsFilesPage(c, pal)
				case "keys":
					a.settingsKeysPage(c, pal)
				default:
					a.settingsAdvancedPage(c, pal)
				}
			})
		})
	})
}

func settingsNavRow(c *ui.Context, a *appState, p settingsPage, pal *settingsPalette) {
	sel := a.settingsPageID() == p.id
	item := ui.Row(c.Key(p.id)).Height(34).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8).Label(p.title)
	bg := ui.Transparent
	switch {
	case sel:
		bg = pal.accent.Alpha(0.16)
	case item.Hovered():
		bg = pal.rowHover
	}
	item.Background(item.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if (item.Pressed() || item.Clicked()) && !sel {
		a.setPage(p.id)
	}
	item.Children(func() {
		ic := ui.Icon(c, p.icon).Size(16, 16).TextColor(pal.textMuted)
		if sel {
			ic.TextColor(pal.accent)
		}
		ui.Text(c, p.title).FontSize(13.5).FontWeight(sel2w(sel)).TextColor(pal.text).SingleLine().Grow(1)
		if p.id == "advanced" && availableUpdate() != "" {
			updateBadge(c, "新", pal)
		}
	})
}

func sel2w(sel bool) int {
	if sel {
		return 600
	}
	return 400
}
