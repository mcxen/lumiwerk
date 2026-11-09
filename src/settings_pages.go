// Settings pages, each a column of setGroup cards. Controls write straight
// through to appState and the preferences, so a change takes effect at once
// instead of waiting for a button.
package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

func (a *appState) settingsGeneralPage(c *ui.Context, pal *settingsPalette) {
	setGroup(c, "外观", func() {
		if setChoice(c, "theme", "主题", &a.Profiles.View.Theme,
			[]string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"}) {
			a.setTheme(a.Profiles.View.Theme)
		}
		figureTheme(c, pal, a.Profiles.View.Theme)
	})
	setGroup(c, "启动", func() {
		if setToggleHint(c, "restore-session", "恢复上次会话", "重新打开上次的文件夹和标签页", &a.Profiles.RestoreSession) {
			a.Profiles.RestoreOff = !a.Profiles.RestoreSession
			savePrefs(a.Profiles)
		}
		if setTextHint(c, "start-folder", "启动目录", "不恢复会话时打开这里", &a.Settings.StartFolder, "上次浏览的目录", "选择…", false) {
			safeGo("dialog.pickStartFolder", a.pickStartFolder)
		}
	})
	setGroup(c, "窗口", func() {
		if setChoice(c, "close", "关闭窗口时", &a.Profiles.CloseAction,
			[]string{closeToTray, closeQuit}, []string{"隐藏到托盘", "直接退出"}) {
			if a.Profiles.CloseAction != closeQuit {
				a.Profiles.CloseAction = closeToTray
			}
			savePrefs(a.Profiles)
		}
		figureClose(c, pal, a.Profiles.CloseAction)
	})
}

// setViewFilter stores the type filter the user just changed, saves it and
// rescans so the browser shows the new set at once.
func (a *appState) setViewFilter() {
	savePrefs(a.Profiles)
	a.refreshView()
}

// anyCustomTypeEnabled reports whether at least one concrete type filter is on.
func anyCustomTypeEnabled(view viewPreferences) bool {
	return view.Images || view.RAW || view.Videos || view.Archives || view.Installers
}

// settingsBrowsePage holds what is not on the toolbar or the View menu:
// which file types appear, and how Enter, selection and playback behave.
// Layout, thumbnail size, sorting and hidden files are changed live there.
func (a *appState) settingsBrowsePage(c *ui.Context, pal *settingsPalette) {
	view := &a.Profiles.View
	setGroup(c, "显示的文件类型", func() {
		mode := 0
		if !view.AllTypes {
			mode = 1
		}
		if setSegmented(c, "type-mode", "类型", &mode, "全部", "自定义") {
			view.AllTypes = mode == 0
			if mode == 1 && !anyCustomTypeEnabled(*view) {
				view.Images, view.Videos = true, true
				view.RAW, view.Archives, view.Installers = false, false, false
			}
			a.setViewFilter()
		}
		if mode == 1 {
			if setChecks(c, "", []setCheck{
				{"filter-images", "图像", &view.Images},
				{"filter-raw", "RAW", &view.RAW},
				{"filter-videos", "视频", &view.Videos},
				{"filter-archives", "压缩包", &view.Archives},
				{"filter-installers", "安装包", &view.Installers},
			}) {
				a.setViewFilter()
			}
		}
		figureTypes(c, pal, *view)
	})
	setGroup(c, "浏览", func() {
		if setChoiceHint(c, "enter", "按 Enter 时", "对选中的文件", &a.Settings.EnterOpens, []bool{false, true}, []string{"重命名", "打开/关闭"}) {
			a.applyActionSettings()
		}
		figureEnter(c, pal, a.Settings.EnterOpens)
		if setToggleHint(c, "autoplay-selected", "选中即播", "在浏览器里选中视频就开始预览", &a.Profiles.View.AutoPlaySelected) {
			savePrefs(a.Profiles)
		}
	})
	setGroup(c, "视频播放", func() {
		if setChoice(c, "vidend", "播放结束后", &a.Settings.PlaybackEnd,
			[]string{playbackEndStop, playbackEndNext, playbackEndLoop}, []string{"停住", "下一个", "循环"}) {
			a.applyActionSettings()
		}
		figurePlayEnd(c, pal, a.Settings.PlaybackEnd)
		if setToggleHint(c, "remember-play", "记住播放进度", "下次从上次看到的位置继续", &a.Profiles.RememberPlayProgress) {
			a.Profiles.RememberPlayOff = !a.Profiles.RememberPlayProgress
			savePrefs(a.Profiles)
		}
	})
}

func (a *appState) settingsAdvancedPage(c *ui.Context, pal *settingsPalette) {
	setGroup(c, "缓存", func() {
		if setSlider(c, "cache", "缩略图缓存", &a.Settings.CacheCap, 64, 1024, " MiB") {
			a.Profiles.ThumbCacheMiB = normalizeThumbCacheMiB(int(a.Settings.CacheCap))
			thumbLRU.SetLimit(a.Profiles.ThumbCacheMiB << 20)
			savePrefs(a.Profiles)
		}
		usage := fmt.Sprintf("内存 %.0f / %.0f MiB · 磁盘 %.0f MiB",
			float64(thumbLRU.Size())/(1024*1024), float64(thumbLRU.Limit())/(1024*1024), float64(diskThumbs.Size())/(1024*1024))
		if setButtonHint(c, "settings-clear-caches", "清理缓存", usage, "清理") {
			thumbLRU.Clear()
			diskThumbs.Clear()
			sweepTempLeftovers()
			logInfof("settings: caches cleared by the user")
		}
	})
	setGroup(c, "更新", func() {
		if setSelect(c, "update-source", "更新源", &a.Settings.UpdateSource, updateSourceChoices, updateSourceName) {
			a.Profiles.UpdateSource = a.Settings.UpdateSource
			savePrefs(a.Profiles)
			applyUpdateSource(a.Profiles.UpdateSource)
		}
		if v := availableUpdate(); v != "" {
			setRow(c, "新版本可用", func() {
				updateBadge(c, "v"+strings.TrimPrefix(v, "v"), pal)
			})
		}
	})
	setGroup(c, "诊断", func() {
		if setButtonHint(c, "settings-open-diagnostics", "诊断", "查看最近的警告、错误和运行状态（⇧⌘D）", "打开") {
			a.showDiagnostics()
		}
	})
}

// availableUpdate is updater.AvailableVersion, or "" where the updater
// plugin is not in use (tests).
func availableUpdate() (v string) {
	defer func() {
		if recover() != nil {
			v = ""
		}
	}()
	return updateProbe()
}

// updateProbe reads the version the updater found; tests replace it.
var updateProbe = updater.AvailableVersion

// updateBadge is a mygo Badge in the accent color, for a newer version.
func updateBadge(c *ui.Context, text string, pal *settingsPalette) ui.Element {
	return ui.Badge(c, text).Background(pal.accent).TextColor(ui.Hex("#ffffff"))
}
