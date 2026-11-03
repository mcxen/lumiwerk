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

func (a *appState) settingsViewPage(c *ui.Context, pal *settingsPalette) {
	settingsGroup(c, pal, "浏览", "", func() {
		settingsRow(c, pal, "gesture", "浏览手势", "方向键、滚轮和 W/A/S/D 手势用于浏览媒体。", func() {})
		settingsRow(c, pal, "archives", "显示压缩文件", "把 ZIP 等压缩包当作媒体显示", func() {
			settingsToggle(c, "archives", &a.Settings.ShowArchives, pal)
		})
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
