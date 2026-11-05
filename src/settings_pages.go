// Settings pages, each a column of settingsSection groups. Controls write
// straight through to appState and the preferences, so a change takes
// effect at once instead of waiting for a button.
package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *appState) settingsGeneralPage(c *ui.Context, pal *settingsPalette) {
	setGroup(c, "配置", func() {
		setRow(c, "配置槽", func() { a.profileSlots(c, pal) })
	})
	setGroup(c, "外观", func() {
		if setChoice(c, "theme", "主题", &a.Profiles.View.Theme,
			[]string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"}) {
			a.setTheme(a.Profiles.View.Theme)
		}
	})
	setGroup(c, "启动", func() {
		if setText(c, "start-folder", "启动目录", &a.Settings.StartFolder, "上次浏览的目录", "选择…", false) {
			safeGo("dialog.pickStartFolder", a.pickStartFolder)
		}
		if setToggle(c, "restore-session", "恢复上次会话", &a.Profiles.RestoreSession) {
			a.Profiles.RestoreOff = !a.Profiles.RestoreSession
			savePrefs(a.Profiles)
		}
	})
	setGroup(c, "窗口", func() {
		if setChoice(c, "close", "关闭窗口时", &a.Profiles.CloseAction,
			[]string{closeToTray, closeQuit}, []string{"收进菜单栏", "直接退出"}) {
			if a.Profiles.CloseAction != closeQuit {
				a.Profiles.CloseAction = closeToTray
			}
			savePrefs(a.Profiles)
		}
	})
}

// profileSlots draws the nine configuration slots: the active one is filled
// with the accent, and it carries an asterisk while the live view has
// drifted from what it stored.
func (a *appState) profileSlots(c *ui.Context, pal *settingsPalette) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		for i := range a.Profiles.Profiles {
			i := i
			active := a.Profiles.Active == i
			label := strconv.Itoa(i + 1)
			if active && a.profileDirty(i) {
				label += "*"
			}
			slot := ui.Row(c.Key("profile-slot-"+strconv.Itoa(i))).Size(28, 28).Radius(6).
				Justify(ui.Center).AlignItems(ui.Center).
				Label(fmt.Sprintf("配置 %d", i+1)).Tooltip(fmt.Sprintf("配置 %d", i+1))
			bg := ui.Transparent
			switch {
			case active:
				bg = pal.accent
			case slot.Hovered():
				bg = pal.surfaceHover
			}
			slot.Background(slot.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
			if slot.Clicked() {
				a.applyProfile(i)
				c.Toast(fmt.Sprintf("已切换到配置 %d", i+1))
			}
			slot.Children(func() {
				color := pal.text
				if active {
					color = t.AccentText
				}
				ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(color)
			})
		}
		if ui.Button(c.Key("profile-save"), "保存到当前配置").Clicked() {
			a.saveProfile(a.Profiles.Active)
			c.Toast(fmt.Sprintf("已保存到配置 %d", a.Profiles.Active+1))
		}
	})
}

// setViewFilter stores the type filter the user just changed, saves it and
// rescans so the browser shows the new set at once.
func (a *appState) setViewFilter() {
	savePrefs(a.Profiles)
	a.refreshView()
}

// setThumbSize moves the live browser to a new default thumbnail size,
// keeping the current pictures on screen and settling the sharper band
// after the drag stops.
func (a *appState) setThumbSize(size float64) {
	if size < 64 {
		size = 64
	}
	if size > 512 {
		size = 512
	}
	if size == a.Thumb {
		return
	}
	old := a.Thumb
	before := thumbBand(thumbEdge(a.Thumb))
	a.Thumb = size
	a.noteThumbBand(before)
	a.anchorBrowse(old, a.Thumb, 0, 0, false, a.browseBox)
}

// layoutOptions are the four layouts a new window can open in, in the
// spec's order: justified, waterfall, grid, detail.
var layoutOptions = []struct {
	mode  int
	label string
}{
	{3, "两端对齐"},
	{2, "瀑布流"},
	{0, "等大网格"},
	{1, "详情列表"},
}

func layoutName(mode int) string {
	for _, option := range layoutOptions {
		if option.mode == mode {
			return option.label
		}
	}
	return layoutOptions[0].label
}

func (a *appState) settingsViewPage(c *ui.Context, pal *settingsPalette) {
	view := &a.Profiles.View
	setGroup(c, "类型过滤", func() {
		if setChecks(c, "显示", []setCheck{
			{"filter-images", "图像文件", &view.Images},
			{"filter-raw", "RAW 文件", &view.RAW},
			{"filter-videos", "视频文件", &view.Videos},
			{"filter-archives", "压缩文件", &view.Archives},
			{"filter-all", "全部类型", &view.AllTypes},
			{"filter-hidden", "隐藏文件", &view.Hidden},
			{"filter-folders", "包含文件夹", &view.RecursiveFolders},
		}) {
			a.setViewFilter()
		}
	})
	setGroup(c, "布局", func() {
		modes := make([]int, 0, len(layoutOptions))
		for _, option := range layoutOptions {
			modes = append(modes, option.mode)
		}
		if setSelect(c, "layout-popup", "默认布局", &a.Settings.Layout, modes, layoutName) {
			a.LayoutMode = a.Settings.Layout
			a.refresh()
		}
		if setSlider(c, "settings-thumb", "默认缩略图", &a.Settings.Thumb, 64, 512, " px") {
			a.setThumbSize(a.Settings.Thumb)
		}
	})
	setGroup(c, "排序", func() {
		if setToggle(c, "folder-first", "文件夹优先", &a.FolderFirst) {
			a.refresh()
		}
		if setToggle(c, "full-path-sort", "递归时全路径排序", &a.FullPathSort) {
			a.refresh()
		}
		if setToggle(c, "media-unit", "媒体单元模式", &a.Profiles.View.MediaUnit) {
			a.setMediaUnit(a.Profiles.View.MediaUnit)
		}
		if setToggle(c, "autoplay-selected", "自动播放选中视频", &a.Profiles.View.AutoPlaySelected) {
			savePrefs(a.Profiles)
		}
	})
}

func (a *appState) settingsAdvancedPage(c *ui.Context, pal *settingsPalette) {
	setGroup(c, "性能", func() {
		if setSlider(c, "cache", "缩略图缓存数量", &a.Settings.CacheCap, 64, 1024, "") {
			thumbLRU.SetLimit(int(a.Settings.CacheCap))
		}
		setSlider(c, "workers", "后台解码并发", &a.Settings.Workers, 1, 16, "")
	})
	setGroup(c, "清理缓存", func() {
		setRow(c, "已用", func() {
			ui.Text(c, fmt.Sprintf("内存 %.0f / %.0f MiB · 磁盘 %.0f MiB",
				float64(thumbLRU.Size())/(1024*1024), float64(thumbLRU.Limit())/(1024*1024), float64(diskThumbs.Size())/(1024*1024))).
				FontSize(13).TextColor(c.Theme().Text)
		})
		if setButton(c, "settings-clear-caches", "缩略图与临时文件", "清理") {
			thumbLRU.Clear()
			diskThumbs.Clear()
			sweepTempLeftovers()
			logInfof("settings: caches cleared by the user")
		}
	})
	setGroup(c, "诊断", func() {
		if setButton(c, "settings-open-diagnostics", "运行诊断", "打开诊断") {
			a.showDiagnostics()
		}
	})
}
