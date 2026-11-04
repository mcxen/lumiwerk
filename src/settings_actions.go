// Settings pages for actions, in the GodGo group/row layout.
package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *appState) settingsActionsPage(c *ui.Context, pal *settingsPalette) {
	settingsGroup(c, pal, "Enter 键：", "", func() {
		settingsRow(c, pal, "enter", "按 Enter 时", "", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Radio(c, &a.Settings.EnterOpens, false, "重命名")
				ui.Radio(c, &a.Settings.EnterOpens, true, "打开 / 关闭")
			})
		})
	})
	settingsGroup(c, pal, "文件操作：", "", func() {
		settingsRow(c, pal, "quickrule", "快速重命名", "在当前文件夹内快速重命名的规则", func() {
			ui.TextInput(c, &a.Settings.QuickRule).Grow(1).MinWidth(160)
			if ui.Button(c.Key("settings-apply"), "应用").Clicked() {
				a.applyActionSettings()
			}
		})
		a.destinationSetting(c, "图片文件：", &a.Settings.Dest, "image")
		settingsRow(c, pal, "imgshortcut", "图片复制快捷键", "", func() {
			ui.MenuButton(c, a.Settings.ImageShortcut, func(m *ui.Menu) {
				a.shortcutMenu(m, &a.Settings.ImageShortcut)
			})
		})
		a.destinationSetting(c, "视频文件：", &a.Settings.VideoDest, "video")
		settingsRow(c, pal, "vidshortcut", "视频复制快捷键", "", func() {
			ui.MenuButton(c, a.Settings.VideoShortcut, func(m *ui.Menu) {
				a.shortcutMenu(m, &a.Settings.VideoShortcut)
			})
		})
		settingsRow(c, pal, "vidend", "播放结束", "停在末帧、播放下一个或循环", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Radio(c, &a.Settings.PlaybackEnd, playbackEndStop, "停在末帧")
				ui.Radio(c, &a.Settings.PlaybackEnd, playbackEndNext, "下一个")
				ui.Radio(c, &a.Settings.PlaybackEnd, playbackEndLoop, "循环")
			})
			if normalizePlaybackEnd(a.Settings.PlaybackEnd) != normalizePlaybackEnd(a.Profiles.PlaybackEnd) {
				a.applyActionSettings()
			}
		})
	})
	settingsGroup(c, pal, "ZIP 默认密码：", "", func() {
		settingsRow(c, pal, "zip", "密码", "可选；用于加密压缩", func() {
			ui.TextInput(c, &a.Settings.ZipPassword).Password().Tooltip("可选").Width(160)
		})
		settingsRow(c, pal, "zipuse", "默认加密", "默认使用上面的密码进行加密压缩", func() {
			settingsToggle(c, "zipuse", &a.Settings.ZipUsePassword, pal)
		})
	})
	settingsGroup(c, pal, "操作指引：", "", func() {
		settingsRow(c, pal, "guide", "说明与快捷键", "点击按钮后会以弹窗显示说明，按 Esc 可先关闭弹窗，再按 Esc 关闭设置窗口。", func() {
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c.Key("operation-guide"), "操作说明").Clicked() {
					safeGo("dialog.operationGuide", func() { a.showOperationGuide(false) })
				}
				if ui.Button(c.Key("shortcut-guide"), "快捷键对照").Clicked() {
					safeGo("dialog.shortcutGuide", func() { a.showOperationGuide(true) })
				}
			})
		})
	})
	if warning := shortcutConflict(a.Settings.ImageShortcut, a.Settings.VideoShortcut); warning != "" {
		ui.Text(c, warning).FontSize(11).TextColor(pal.warn)
	}
}

func (a *appState) destinationSetting(c *ui.Context, label string, value *string, kind string) {
	pal := settingsPaletteFor(c)
	settingsRow(c, pal, "dest-"+kind, strings.TrimSuffix(label, "："), "复制到配置目录", func() {
		ui.TextInput(c, value).Tooltip(*value).Grow(1).MinWidth(160)
		if ui.Button(c.Key("settings-pick-"+kind), "选择文件夹").Clicked() {
			safeGo("dialog.pickActionDestination", func() { a.pickActionDestination(kind) })
		}
	})
}

var _ = fmt.Sprintf
