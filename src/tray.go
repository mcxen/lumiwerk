package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The tray icon is resident for the whole run. A visible window still
// leaves it in the menu bar (the usual macOS choice: the icon is how you
// reach the app, not a badge that appears only while it is hidden). Quit
// removes it.

func (a *appState) installTray() {
	icon, err := loadTrayIcon()
	if err != nil {
		logErrorf("tray icon: %v", err)
		return
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "lumiwerk",
		Icon:           icon,
		IconIsTemplate: true,
		ToolTip:        appName,
	})
	if err != nil {
		logErrorf("tray: %v", err)
		return
	}
	a.Tray = tray
	// No menu: a menu would swallow the primary click. The click opens the
	// panel, or brings the window back when it is hidden.
	//   - window hidden: left click shows, focuses and restores it
	//   - window visible: left click toggles the status panel
	//   - right click or Control-click: always toggles the panel, so a
	//     shelved app can be quit or inspected without raising the window
	tray.OnClick(func() {
		if !a.mainWindowVisible() {
			a.showMainWindow()
			return
		}
		a.toggleTrayPanel()
	})
	tray.OnRightClick(func() { a.toggleTrayPanel() })
	a.syncTrayTip()
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.flyout != nil && !a.flyout.IsDestroyed() {
			a.flyout.Destroy()
			a.flyout = nil
		}
		if a.Tray != nil {
			a.Tray.Destroy()
			a.Tray = nil
		}
	})
}

func (a *appState) mainWindowVisible() bool {
	return a.Window != nil && a.Window.IsVisible()
}

func (a *appState) syncTrayTip() {
	if a.Tray == nil {
		return
	}
	tip := appName
	if a.Root != "" {
		tip = appName + " — " + pathName(a.Root)
	}
	if !a.mainWindowVisible() {
		tip += "（已收起）"
	}
	a.Tray.SetToolTip(tip)
}

// toggleTrayPanel opens or hides the status panel. MyGo leaves a press on
// the tray icon to this click (it does not also dismiss the popover), so
// hiding here is what closes it.
func (a *appState) toggleTrayPanel() {
	if a.trayPanelVisible() {
		a.dismissTrayPanel()
		return
	}
	a.openTrayPanel()
}

func (a *appState) trayPanelVisible() bool {
	return a.flyout != nil && !a.flyout.IsDestroyed() && a.flyout.IsVisible()
}

func (a *appState) openTrayPanel() {
	if a.Tray == nil {
		return
	}
	if a.flyout != nil && !a.flyout.IsDestroyed() {
		a.flyout.Show()
		return
	}
	w := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.Tray,
		Placement: mygo.PlacementBottom,
		Width:     300,
		Height:    292,
		Focusable: true,
		Popover:   true,
		Content:   ui.View(a.trayPanel),
	})
	a.flyout = w
	w.OnClosed(func() {
		if a.flyout == w {
			a.flyout = nil
		}
	})
}

// dismissTrayPanel hides the panel after the click that asked for it has
// finished building the frame.
func (a *appState) dismissTrayPanel() {
	w := a.flyout
	if w == nil || w.IsDestroyed() {
		return
	}
	safeGo("tray.dismiss", func() { w.Hide() })
}

func (a *appState) trayShowWindow() {
	a.dismissTrayPanel()
	a.showMainWindow()
}

func (a *appState) trayOpenSettings() {
	a.dismissTrayPanel()
	a.openSettings()
	a.settingsWindow()
}

func (a *appState) traySummary() string {
	if a.Root == "" {
		return "尚未打开目录"
	}
	summary := fmt.Sprintf("%d 个媒体", a.mediaCount())
	if n := len(a.Tabs.Tabs); n > 1 {
		summary += fmt.Sprintf(" · %d 个标签页", n)
	}
	if n := len(a.selectedPaths()); n > 0 {
		summary += fmt.Sprintf(" · 已选 %d", n)
	}
	return summary
}

func (a *appState) trayWindowStatus() string {
	if a.mainWindowVisible() {
		return "主窗口已打开"
	}
	return "主窗口已收起"
}

// trayPanel is the menu-bar popover: where the browser is, how much it is
// showing, and the actions that used to require the window to be open.
func (a *appState) trayPanel(c *ui.Context) {
	t := c.Theme()
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(t.Background)
	}
	ui.Column(c).Fill().Padding(16, 16, 14, 16).Gap(12).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, appName).FontSize(12).FontWeight(600).TextColor(t.Text).Grow(1)
			ui.Text(c, a.trayWindowStatus()).FontSize(11).TextColor(t.TextMuted)
		})

		head := ui.Column(c).FillWidth().Gap(2).Padding(8, 8).Radius(8)
		if a.Root != "" && head.Hovered() {
			head.Background(t.SurfaceHover)
		}
		head.Tooltip("在主窗口中打开此目录")
		head.OnClick(func() { a.trayShowWindow() })
		head.Children(func() {
			name := "未打开目录"
			if a.Root != "" {
				name = pathName(a.Root)
			}
			ui.Text(c, name).FontSize(16).FontWeight(600).TextColor(t.Text).SingleLine().Ellipsis("…").FillWidth()
			if a.Root != "" {
				ui.Text(c, a.Root).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…").FillWidth()
			} else {
				ui.Text(c, "打开主窗口后可以选择目录").FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
			ui.Text(c, a.traySummary()).FontSize(12).TextColor(t.Text).SingleLine().Ellipsis("…").FillWidth()
		})

		ui.Divider(c).FillWidth()

		ui.Column(c).FillWidth().Gap(8).Children(func() {
			if ui.PrimaryButton(c.Key("tray-show"), "显示主窗口").FillWidth().Clicked() {
				a.trayShowWindow()
			}
			if ui.Button(c.Key("tray-settings"), "偏好设置").FillWidth().Clicked() {
				a.trayOpenSettings()
			}
			if ui.Button(c.Key("tray-quit"), "退出 Lumiwerk").FillWidth().Clicked() {
				mygo.App.Quit()
			}
		})
	})
}

// loadTrayIcon reads the template PNG shipped in resources/. A bundled app
// finds it in Contents/Resources; go run finds the project's resources
// directory, and a run started from src/ still checks the parent.
func loadTrayIcon() ([]byte, error) {
	var candidates []string
	if dir, err := mygo.App.Path(mygo.PathResources); err == nil && dir != "" {
		candidates = append(candidates, filepath.Join(dir, "tray-template.png"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "resources", "tray-template.png"),
			filepath.Join(wd, "..", "resources", "tray-template.png"),
		)
	}
	var last error
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		last = err
	}
	if last == nil {
		last = os.ErrNotExist
	}
	return nil, fmt.Errorf("tray-template.png: %w", last)
}
