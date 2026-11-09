package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The tray icon is resident for the whole run. Left click opens the status
// panel (or brings a hidden window back); right click or Control-click
// pops up a native menu. A menu is never attached permanently: it would
// swallow the primary click. The icon shows a mark while a task runs or
// media plays, and can carry a short status title.

// trayRuntime is the tray's live state. Main thread only.
type trayRuntime struct {
	baseIcon []byte
	kind     trayIconKind
	title    string
	hotkey   string
	stop     chan struct{}
	// tab is the panel's quick-access tab; empty means the saved one.
	tab string
}

const (
	trayPanelW      = 320
	trayPanelH      = 400
	trayPanelRows   = 5
	trayStatusEvery = time.Second
)

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
	a.trayRT.baseIcon = icon
	a.trayRT.stop = make(chan struct{})
	//   - window hidden: left click shows, focuses and restores it
	//   - window visible: left click toggles the status panel
	//   - right click or Control-click: the native menu
	tray.OnClick(func() {
		if !a.mainWindowVisible() {
			a.showMainWindow()
			return
		}
		a.toggleTrayPanel()
	})
	tray.OnRightClick(func() { a.popTrayMenu() })
	a.syncTrayTip()
	a.applyTrayPrefs()
	safeGo("tray.status", func() {
		tick := time.NewTicker(trayStatusEvery)
		defer tick.Stop()
		for {
			select {
			case <-a.trayRT.stop:
				return
			case <-tick.C:
				a.update(a.refreshTray)
			}
		}
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.trayRT.stop != nil {
			close(a.trayRT.stop)
			a.trayRT.stop = nil
		}
		if a.trayRT.hotkey != "" {
			mygo.GlobalShortcut.Unregister(a.trayRT.hotkey)
			a.trayRT.hotkey = ""
		}
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

// refreshTray applies the current status to the icon mark, the title and
// the tooltip, and redraws an open panel. Main thread.
func (a *appState) refreshTray() {
	if a.Tray == nil {
		return
	}
	st := a.trayStatusNow()
	if kind := st.Icon(); kind != a.trayRT.kind {
		a.trayRT.kind = kind
		if err := a.Tray.SetIcon(trayIconVariant(a.trayRT.baseIcon, kind), true); err != nil {
			logWarnf("tray icon: %v", err)
		}
		a.syncTrayTip()
	}
	title := ""
	if a.Profiles.TrayTitle {
		title = st.Title()
	}
	if title != a.trayRT.title {
		a.trayRT.title = title
		a.Tray.SetTitle(title)
	}
	if a.trayPanelVisible() && (st.HasTask() || st.HasMedia()) {
		a.flyout.Invalidate()
	}
}

// applyTrayPrefs (re)registers the global shortcut and the Dock policy
// from the preferences. Call after they change.
func (a *appState) applyTrayPrefs() {
	want := a.Profiles.trayHotkey()
	if want != a.trayRT.hotkey {
		if a.trayRT.hotkey != "" {
			mygo.GlobalShortcut.Unregister(a.trayRT.hotkey)
			a.trayRT.hotkey = ""
		}
		if want != "" {
			if err := mygo.GlobalShortcut.Register(want, func() { a.toggleMainWindow() }); err != nil {
				logWarnf("global shortcut %s: %v", want, err)
			} else {
				a.trayRT.hotkey = want
			}
		}
	}
	a.applyDockPolicy()
	a.refreshTray()
}

// applyDockPolicy hides the Dock icon while the window is shelved, when
// the user asked for a menu-bar-only app.
func (a *appState) applyDockPolicy() {
	if mygo.App.Dock == nil {
		return
	}
	if a.Profiles.HideDockWhenShelved && !a.mainWindowVisible() {
		mygo.App.Dock.Hide()
		return
	}
	mygo.App.Dock.Show()
}

// toggleMainWindow is the global shortcut and the menu's first item.
func (a *appState) toggleMainWindow() {
	if a.mainWindowVisible() {
		a.dismissTrayPanel()
		a.shelfMainWindow()
		return
	}
	a.showMainWindow()
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
	st := a.trayStatusNow()
	if st.HasTask() {
		tip += "\n" + st.Task
		if p := st.TaskProgress(); p >= 0 {
			tip += fmt.Sprintf(" %d%%", int(p*100))
		}
	}
	if st.Playing {
		tip += "\n▶ " + st.Media
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
		Width:     trayPanelW,
		Height:    trayPanelH,
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

// trayOpenFolder opens path in the main window, bringing it back first.
func (a *appState) trayOpenFolder(path string) {
	a.dismissTrayPanel()
	a.showMainWindow()
	a.openFolder(path)
}

// trayPickFolder asks for a folder in the (shown) main window.
func (a *appState) trayPickFolder() {
	a.dismissTrayPanel()
	a.showMainWindow()
	safeGo("tray.pickFolder", func() { a.pickFolder() })
}

func (a *appState) trayRevealRoot() {
	if a.Root == "" {
		return
	}
	a.dismissTrayPanel()
	a.revealInFinder()
}

// trayDroppedFolder is the folder to open for files dropped on the
// panel: the folder itself, or the folder holding a dropped file.
func trayDroppedFolder(paths []string) string {
	for _, p := range paths {
		if isDir(p) {
			return p
		}
	}
	for _, p := range paths {
		if dir := filepath.Dir(p); p != "" && isDir(dir) {
			return dir
		}
	}
	return ""
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

func (a *appState) trayActiveTab() string {
	if a.trayRT.tab != "" {
		return a.trayRT.tab
	}
	return a.Profiles.TrayTab
}

func (a *appState) traySeek(delta float64) {
	v := a.Video
	if v == nil || v.kernel == nil {
		return
	}
	snap := v.kernel.Snapshot()
	at := max(snap.Position+delta, 0)
	if snap.Duration > 0 {
		at = min(at, snap.Duration)
	}
	v.kernel.Seek(at)
}

// trayPanel is the menu-bar popover: where the browser is, what is
// running, quick access to folders, and the actions that used to need the
// window. Files dropped on it open their folder.
func (a *appState) trayPanel(c *ui.Context) {
	safeView("tray", c, func() { a.trayPanelBody(c) })
}

func (a *appState) trayPanelBody(c *ui.Context) {
	t := c.Theme()
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(t.Background)
	}
	st := a.trayStatusNow()
	root := ui.Column(c).Fill().Padding(14, 14, 12, 14).Gap(10)
	if dropped := root.DroppedFiles(); len(dropped) > 0 {
		if dir := trayDroppedFolder(dropped); dir != "" {
			a.trayOpenFolder(dir)
		}
	}
	root.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, appName).FontSize(12).FontWeight(600).TextColor(t.Text)
			ui.Text(c, a.trayWindowStatus()).FontSize(11).TextColor(t.TextMuted).Grow(1)
			if trayLink(c, "tray-show-icon", "显示窗口", false) {
				a.trayShowWindow()
			}
		})

		a.trayFolderCard(c)
		a.trayStatusBlock(c, st)
		a.trayQuickAccess(c)

		ui.Spacer(c).Grow(1)
		ui.Divider(c).FillWidth()
		ui.Row(c).FillWidth().Gap(2).AlignItems(ui.Center).Children(func() {
			if trayLink(c, "tray-open", "打开文件夹…", false) {
				a.trayPickFolder()
			}
			if trayLink(c, "tray-settings", "偏好设置", false) {
				a.trayOpenSettings()
			}
			if trayLink(c, "tray-diagnostics", "诊断", false) {
				a.dismissTrayPanel()
				a.showDiagnostics()
			}
			ui.Spacer(c).Grow(1)
			if trayLink(c, "tray-quit", "退出", true) {
				mygo.App.Quit()
			}
		})
	})
}

// trayLink is a compact, borderless action: text with a hover fill. A
// full-size button is too heavy for a popover footer.
func trayLink(c *ui.Context, key, label string, danger bool) bool {
	t := c.Theme()
	clicked := false
	row := ui.Row(c.Key(key)).Padding(4, 8).Radius(6).Cursor(ui.CursorPointer).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.OnClick(func() { clicked = true })
	row.Children(func() {
		txt := ui.Text(c, label).FontSize(12).SingleLine().TextColor(t.Text)
		if danger {
			txt.TextColor(t.TextMuted)
		}
	})
	return clicked
}

func (a *appState) trayFolderCard(c *ui.Context) {
	t := c.Theme()
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
			ui.Text(c, "打开主窗口后可以选择目录，也可把文件夹拖到这里").FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
		ui.Text(c, a.traySummary()).FontSize(12).TextColor(t.Text).SingleLine().Ellipsis("…").FillWidth()
	})
}

// trayStatusBlock shows the running task and the loaded video; absent
// when neither exists.
func (a *appState) trayStatusBlock(c *ui.Context, st trayStatus) {
	if !st.HasTask() && !st.HasMedia() {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(6).Padding(8, 8).Radius(8).Background(t.Surface).Children(func() {
		if st.HasTask() {
			label := st.Task
			if st.TaskTotal > 1 {
				label += fmt.Sprintf(" %d/%d", min(st.TaskDone, st.TaskTotal), st.TaskTotal)
			}
			ui.Text(c, label).FontSize(12).TextColor(t.Text).SingleLine().Ellipsis("…").FillWidth()
			ui.Progress(c, st.TaskProgress()).FillWidth().Label(st.Task)
		}
		if st.HasMedia() {
			ui.Text(c, st.Media).FontSize(12).FontWeight(500).TextColor(t.Text).SingleLine().Ellipsis("…").FillWidth()
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				if ui.Button(c.Key("tray-back"), "−10 秒").Clicked() {
					a.traySeek(-10)
				}
				label := "播放"
				if st.Playing {
					label = "暂停"
				}
				if ui.PrimaryButton(c.Key("tray-play"), label).Clicked() {
					a.toggleVideoPlayback()
				}
				if ui.Button(c.Key("tray-fwd"), "+10 秒").Clicked() {
					a.traySeek(10)
				}
				ui.Text(c, formatClock(st.Position)+" / "+formatClock(st.Duration)).
					FontSize(11).TextColor(t.TextMuted).Grow(1).TextAlign(ui.End)
			})
		}
	})
}

// trayQuickAccess is the tabbed list: recent folders, favorites, and the
// open tabs.
func (a *appState) trayQuickAccess(c *ui.Context) {
	t := c.Theme()
	active := a.trayActiveTab()
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		for _, tb := range []struct{ id, label string }{
			{trayTabRecent, "最近"}, {trayTabFavorites, "收藏夹"}, {trayTabTabs, "标签页"},
		} {
			tb := tb
			chip := ui.Row(c.Key("tray-tab-"+tb.id)).Padding(3, 10).Radius(6).Cursor(ui.CursorPointer)
			if tb.id == active {
				chip.Background(t.SurfaceHover)
			}
			chip.OnClick(func() { a.trayRT.tab = tb.id; a.Profiles.TrayTab = tb.id; savePrefs(a.Profiles) })
			chip.Children(func() {
				w := 400
				if tb.id == active {
					w = 600
				}
				ui.Text(c, tb.label).FontSize(12).FontWeight(w).TextColor(t.Text)
			})
		}
	})
	type row struct{ path, label, hint string }
	var rows []row
	empty := "还没有记录"
	switch active {
	case trayTabFavorites:
		empty = "收藏夹为空"
		for _, p := range a.trayFavoriteFolders() {
			rows = append(rows, row{p, pathName(p), ""})
		}
	case trayTabTabs:
		empty = "没有打开的标签页"
		for i, tb := range a.Tabs.Tabs {
			if tb.Root != "" {
				rows = append(rows, row{tb.Root, pathName(tb.Root), tabHint(i == a.Tabs.Active)})
			}
		}
	default:
		now := time.Now()
		for _, p := range a.trayRecentFolders(a.Profiles.trayRecentCount()) {
			hint := ""
			if at := a.Visited[p]; at > 0 {
				hint = visitLabel(at, now)
			}
			rows = append(rows, row{p, pathName(p), hint})
		}
	}
	if len(rows) == 0 {
		ui.Text(c, empty).FontSize(12).TextColor(t.TextMuted).Padding(6, 4)
		return
	}
	ui.Column(c.Key("tray-rows-" + active)).FillWidth().Gap(1).Children(func() {
		for i, r := range rows {
			if i >= trayPanelRows {
				ui.Text(c, fmt.Sprintf("另有 %d 项，右键菜单可查看全部", len(rows)-i)).
					FontSize(11).TextColor(t.TextMuted).Padding(4, 8)
				break
			}
			r := r
			line := ui.Row(c.Key(fmt.Sprintf("tray-row-%s-%d", active, i))).FillWidth().
				Padding(5, 8).Radius(6).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if line.Hovered() {
				line.Background(t.SurfaceHover)
			}
			line.Tooltip(r.path)
			line.OnClick(func() { a.trayOpenFolder(r.path) })
			line.Children(func() {
				ui.Text(c, r.label).FontSize(12).TextColor(t.Text).SingleLine().Ellipsis("…").Grow(1)
				if r.hint != "" {
					ui.Text(c, r.hint).FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			})
		}
	})
}

func tabHint(active bool) string {
	if active {
		return "当前"
	}
	return ""
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
