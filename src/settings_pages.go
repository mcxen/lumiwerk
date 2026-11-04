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

// settingsField is one label/control line: a 140px label column, then the
// control, on the spec's 32px standard row and 12px form rhythm.
func settingsField(c *ui.Context, label string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinHeight(32).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(140).FontSize(13).TextColor(t.Text).SingleLine().Shrink(0)
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

// settingsNote is the 11px helper line under a field or a group.
func settingsNote(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(11).TextColor(c.Theme().TextMuted).MinWidth(0)
}

// settingsOption is a checkbox line; it reports whether the user flipped it
// on this frame so the caller can persist the new value.
func settingsOption(c *ui.Context, key string, value *bool, label string) bool {
	changed := false
	ui.Row(c).FillWidth().MinHeight(32).AlignItems(ui.Center).Children(func() {
		if ui.Checkbox(c.Key(key), value, label).Changed() {
			changed = true
		}
	})
	return changed
}

// settingsSectionNote pairs a group title's helper text with the group body.
func settingsBlock(c *ui.Context, title, note string, body func()) {
	settingsSection(c, title, func() {
		body()
		if note != "" {
			settingsNote(c, note)
		}
	})
}

func (a *appState) settingsGeneralPage(c *ui.Context, pal *settingsPalette) {
	settingsBlock(c, "配置槽", "Opt+1–9 切换配置，Cmd+Opt+1–9 把当前视图存到该配置。每个配置记录类型过滤、布局、缩略图尺寸、侧栏与排序。", func() {
		a.profileSlots(c, pal)
	})
	settingsSection(c, "外观", func() {
		before := a.Profiles.View.Theme
		settingsField(c, "主题", func() {
			ui.Radio(c.Key("theme-system"), &a.Profiles.View.Theme, "system", "跟随系统")
			ui.Radio(c.Key("theme-light"), &a.Profiles.View.Theme, "light", "浅色")
			ui.Radio(c.Key("theme-dark"), &a.Profiles.View.Theme, "dark", "深色")
		})
		if a.Profiles.View.Theme != before {
			a.setTheme(a.Profiles.View.Theme)
		}
	})
	settingsSection(c, "启动", func() {
		settingsField(c, "启动目录", func() {
			ui.TextInput(c.Key("start-folder"), &a.Settings.StartFolder).Grow(1).MinWidth(160).
				Placeholder("未设置，回到上次浏览的目录").Tooltip(a.Settings.StartFolder)
			if ui.Button(c.Key("pick-start-folder"), "选择…").Clicked() {
				safeGo("dialog.pickStartFolder", a.pickStartFolder)
			}
		})
		if settingsOption(c, "restore-session", &a.Profiles.RestoreSession, "恢复上次会话") {
			a.Profiles.RestoreOff = !a.Profiles.RestoreSession
			savePrefs(a.Profiles)
		}
	})
	settingsSection(c, "窗口", func() {
		before := a.Profiles.CloseAction
		settingsField(c, "关闭窗口时", func() {
			ui.Radio(c.Key("close-tray"), &a.Profiles.CloseAction, closeToTray, "收进菜单栏")
			ui.Radio(c.Key("close-quit"), &a.Profiles.CloseAction, closeQuit, "直接退出")
		})
		if a.Profiles.CloseAction != before {
			if a.Profiles.CloseAction != closeQuit {
				a.Profiles.CloseAction = closeToTray
			}
			savePrefs(a.Profiles)
		}
		settingsNote(c, "收进菜单栏后应用继续在后台运行，点菜单栏图标回到窗口。Cmd+Q 或托盘里的「退出」才会结束进程。")
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
	settingsBlock(c, "类型过滤", "关闭某一类后，下次扫描不再把它列入媒体区。", func() {
		changed := false
		for _, option := range []struct {
			key   string
			label string
			value *bool
		}{
			{"filter-hidden", "隐藏文件", &view.Hidden},
			{"filter-images", "图像文件", &view.Images},
			{"filter-raw", "RAW 文件", &view.RAW},
			{"filter-videos", "视频文件", &view.Videos},
			{"filter-archives", "压缩文件", &view.Archives},
			{"filter-all", "全部类型", &view.AllTypes},
			{"filter-folders", "包含文件夹", &view.RecursiveFolders},
		} {
			if settingsOption(c, option.key, option.value, option.label) {
				changed = true
			}
		}
		if changed {
			a.setViewFilter()
		}
	})
	settingsSection(c, "布局", func() {
		before := a.Settings.Layout
		settingsField(c, "默认布局", func() {
			modes := make([]int, 0, len(layoutOptions))
			for _, option := range layoutOptions {
				modes = append(modes, option.mode)
			}
			settingsPopup(c, "layout-popup", &a.Settings.Layout, modes, layoutName, 140, pal)
		})
		if a.Settings.Layout != before {
			a.LayoutMode = a.Settings.Layout
			a.refresh()
		}
		beforeThumb := a.Settings.Thumb
		settingsField(c, "默认缩略图", func() {
			ui.Slider(c.Key("settings-thumb"), &a.Settings.Thumb, 64, 512).Width(180).Label("默认缩略图")
			ui.Text(c, fmt.Sprintf("%d px", int(a.Settings.Thumb))).FontSize(11).
				TextColor(c.Theme().TextMuted).Width(52).TextAlign(ui.End)
		})
		if a.Settings.Thumb != beforeThumb {
			a.setThumbSize(a.Settings.Thumb)
		}
	})
	settingsSection(c, "显示", func() {
		if settingsOption(c, "folder-first", &a.FolderFirst, "文件夹优先排序") {
			a.refresh()
		}
		if settingsOption(c, "full-path-sort", &a.FullPathSort, "全路径排序（递归模式）") {
			a.refresh()
		}
		if settingsOption(c, "media-unit", &a.Profiles.View.MediaUnit, "媒体单元模式") {
			a.setMediaUnit(a.Profiles.View.MediaUnit)
		}
	})
}

func (a *appState) settingsTagsPage(c *ui.Context, pal *settingsPalette) {
	settingsGroup(c, pal, "标签与评分", "", func() {
		settingsRow(c, pal, "tags-info", "Finder 标签和评分", "在文件的右键菜单中设置 Finder 标签和评分。", func() {})
	})
}

func (a *appState) settingsAdvancedPage(c *ui.Context, pal *settingsPalette) {
	settingsGroup(c, pal, "性能", "", func() {
		settingsRow(c, pal, "cache", "缩略图缓存数量", "", func() {
			ui.Slider(c, &a.Settings.CacheCap, 64, 1024).Width(180).Label("缩略图缓存数量")
		})
		settingsRow(c, pal, "workers", "后台解码并发", "", func() {
			ui.Slider(c, &a.Settings.Workers, 1, 16).Width(180).Label("后台解码并发")
		})
	})
	settingsGroup(c, pal, "清理缓存：", "", func() {
		settingsRow(c, pal, "clear", "缩略图与临时文件",
			fmt.Sprintf("已用 %.0f MiB（上限 %.0f MiB）。清理会释放它们和上次的临时文件；缩略图会在浏览时重新生成。",
				float64(thumbLRU.Size())/(1024*1024), float64(thumbLRU.Limit())/(1024*1024)),
			func() {
				if ui.Button(c.Key("settings-clear-caches"), "清理").Clicked() {
					thumbLRU.Clear()
					sweepTempLeftovers()
					logInfof("settings: caches cleared by the user")
				}
			})
	})
	settingsGroup(c, pal, "诊断：", "", func() {
		settingsRow(c, pal, "diagnostics", "运行诊断", "查看最近警告、错误、运行指标和日志位置。", func() {
			if ui.Button(c.Key("settings-open-diagnostics"), "打开诊断").Clicked() {
				a.showDiagnostics()
			}
		})
	})
	if ui.Button(c.Key("settings-save"), "应用").Clicked() {
		thumbLRU.SetLimit(int(a.Settings.CacheCap))
	}
}
