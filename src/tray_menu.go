package main

import (
	"fmt"

	"github.com/egoist/mygo"
)

// The right-click menu. It is built fresh for every pop-up so recents,
// favorites and the running task are current, then detached again: a
// permanently attached menu would swallow the primary click.

func (a *appState) popTrayMenu() {
	if a.Tray == nil {
		return
	}
	a.dismissTrayPanel()
	a.Tray.SetMenu(a.buildTrayMenu())
	a.Tray.PopUpMenu()
	a.Tray.SetMenu(nil)
}

func trayFolderItems(paths []string, open func(string)) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(paths))
	for _, p := range paths {
		p := p
		items = append(items, &mygo.MenuItem{
			Label:   pathName(p),
			ToolTip: p,
			Click:   func(*mygo.MenuItem, *mygo.Window) { open(p) },
		})
	}
	return items
}

func (a *appState) buildTrayMenu() *mygo.Menu {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}
	st := a.trayStatusNow()
	var items []*mygo.MenuItem

	toggle := "显示 " + appName
	if a.mainWindowVisible() {
		toggle = "隐藏 " + appName
	}
	items = append(items, &mygo.MenuItem{Label: toggle, Accelerator: a.Profiles.trayHotkey(), Click: click(a.toggleMainWindow)})
	items = append(items, mygo.Separator())

	items = append(items, &mygo.MenuItem{Label: "打开文件夹…", Accelerator: "CmdOrCtrl+O", Click: click(a.trayPickFolder)})
	recent := trayFolderItems(a.trayRecentFolders(a.Profiles.trayRecentCount()), a.trayOpenFolder)
	if len(recent) > 0 {
		recent = append(recent, mygo.Separator(), &mygo.MenuItem{Label: "清除历史记录", Click: click(a.clearHistory)})
	} else {
		recent = append(recent, &mygo.MenuItem{Label: "无记录", Disabled: true})
	}
	items = append(items, &mygo.MenuItem{Label: "最近打开", Submenu: recent})
	favs := trayFolderItems(a.trayFavoriteFolders(), a.trayOpenFolder)
	if len(favs) == 0 {
		favs = append(favs, &mygo.MenuItem{Label: "收藏夹为空", Disabled: true})
	}
	items = append(items, &mygo.MenuItem{Label: "收藏夹", Submenu: favs})

	if st.HasTask() || st.HasMedia() {
		items = append(items, mygo.Separator())
	}
	if st.HasTask() {
		label := "任务：" + st.Task
		if p := st.TaskProgress(); p >= 0 {
			label += fmt.Sprintf(" %d%%", int(p*100))
		}
		items = append(items, &mygo.MenuItem{Label: label, Disabled: true})
	}
	if st.HasMedia() {
		play := "播放"
		if st.Playing {
			play = "暂停"
		}
		items = append(items, &mygo.MenuItem{
			Label: "视频：" + play + " " + st.Media, Click: click(a.toggleVideoPlayback),
		}, &mygo.MenuItem{Label: "后退 10 秒", Click: click(func() { a.traySeek(-10) })},
			&mygo.MenuItem{Label: "前进 10 秒", Click: click(func() { a.traySeek(10) })})
	}
	items = append(items, mygo.Separator())

	items = append(items,
		&mygo.MenuItem{Label: "在 " + revealLabel + " 中显示当前目录", Disabled: a.Root == "", Click: click(a.trayRevealRoot)},
		&mygo.MenuItem{Label: "偏好设置…", Accelerator: "CmdOrCtrl+,", Click: click(a.trayOpenSettings)},
		&mygo.MenuItem{Label: "运行诊断", Accelerator: "CmdOrCtrl+Shift+D", Click: click(a.showDiagnostics)},
		mygo.Separator(),
	)

	toTray := a.Profiles.CloseAction != closeQuit
	items = append(items, &mygo.MenuItem{
		Label: "关闭窗口时隐藏到托盘", Type: mygo.MenuItemCheckbox, Checked: toTray,
		Click: click(func() { a.setCloseToTray(!toTray) }),
	})
	items = append(items, &mygo.MenuItem{
		Label: "登录时启动", Type: mygo.MenuItemCheckbox, Checked: openAtLogin(),
		Click: click(a.toggleOpenAtLogin),
	})
	items = append(items, mygo.Separator(),
		&mygo.MenuItem{Label: "退出 " + appName, Accelerator: "CmdOrCtrl+Q", Click: click(mygo.App.Quit)})
	return mygo.NewMenu(items)
}

// openAtLogin reads the login item, which has to ask the main thread: before
// the app is ready (and in tests) there is none to ask.
func openAtLogin() bool {
	return mygo.App.IsReady() && mygo.App.OpenAtLogin()
}

func (a *appState) setCloseToTray(on bool) {
	if on {
		a.Profiles.CloseAction = closeToTray
	} else {
		a.Profiles.CloseAction = closeQuit
	}
	savePrefs(a.Profiles)
}

func (a *appState) toggleOpenAtLogin() {
	if err := mygo.App.SetOpenAtLogin(!openAtLogin()); err != nil {
		logWarnf("open at login: %v", err)
		a.toast("无法修改登录时启动")
	}
}
