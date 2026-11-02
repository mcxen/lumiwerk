// Settings pages in the GodGo group/row layout.
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *appState) settingsGeneralPage(c *ui.Context, pal *settingsPalette) {
	settingsGroup(c, pal, "浏览", "", func() {
		settingsRow(c, pal, "thumb", "默认缩略图", "新窗口打开时的缩略图大小", func() {
			ui.Slider(c, &a.Settings.Thumb, 64, 512).Width(180).Label("默认缩略图")
		})
		settingsRow(c, pal, "sort", "排序", "", func() {
			settingsPopup(c, "sort-popup", &a.Settings.Sort,
				[]string{"name", "date", "size"},
				func(v string) string {
					switch v {
					case "date":
						return "日期"
					case "size":
						return "大小"
					default:
						return "名称"
					}
				}, 140, pal)
		})
	})
	if ui.Button(c.Key("settings-save"), "应用").Clicked() {
		a.Thumb = a.Settings.Thumb
		a.Sort = a.Settings.Sort
		if a.Window != nil {
			a.update(func() { a.refresh() })
		}
	}
	settingsGroup(c, pal, "启动", "", func() {
		settingsRow(c, pal, "restore", "记住上次浏览位置", "再次打开 Lumiwerk 时回到上次浏览的目录；侧栏的历史记录也会跨启动保留。", func() {
			on := !a.Profiles.RestoreOff
			settingsToggle(c, "restore-toggle", &on, pal)
			if on == a.Profiles.RestoreOff {
				a.Profiles.RestoreOff = !on
				a.Profiles.RestoreSession = on
				savePrefs(a.Profiles)
			}
		})
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
