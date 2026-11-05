// Settings page for actions.
package main

import (
	"github.com/egoist/mygo/ui"
)

func (a *appState) settingsActionsPage(c *ui.Context, pal *settingsPalette) {
	setGroup(c, "键盘", func() {
		setChoice(c, "enter", "按 Enter 时", &a.Settings.EnterOpens, []bool{false, true}, []string{"重命名", "打开/关闭"})
		if setSelect(c, "imgshortcut", "图片复制键", &a.Settings.ImageShortcut, shortcutChoices, shortcutName) {
			a.applyActionSettings()
		}
		if setSelect(c, "vidshortcut", "视频复制键", &a.Settings.VideoShortcut, shortcutChoices, shortcutName) {
			a.applyActionSettings()
		}
		if warning := shortcutConflict(a.Settings.ImageShortcut, a.Settings.VideoShortcut); warning != "" {
			setRow(c, "", func() { ui.Text(c, warning).FontSize(12).TextColor(pal.warn) })
		}
	})
	setGroup(c, "文件操作", func() {
		if setText(c, "quickrule", "重命名规则", &a.Settings.QuickRule, "", "应用", false) {
			a.applyActionSettings()
		}
		if setText(c, "dest-image", "图片复制到", &a.Settings.Dest, "", "选择…", false) {
			safeGo("dialog.pickActionDestination", func() { a.pickActionDestination("image") })
		}
		if setText(c, "dest-video", "视频复制到", &a.Settings.VideoDest, "", "选择…", false) {
			safeGo("dialog.pickActionDestination", func() { a.pickActionDestination("video") })
		}
	})
	setGroup(c, "播放", func() {
		if setChoice(c, "vidend", "播放结束", &a.Settings.PlaybackEnd,
			[]string{playbackEndStop, playbackEndNext, playbackEndLoop}, []string{"停在末帧", "下一个", "循环"}) {
			a.applyActionSettings()
		}
	})
	setGroup(c, "ZIP 加密", func() {
		setText(c, "zip", "默认密码", &a.Settings.ZipPassword, "未设置", "", true)
		setToggle(c, "zipuse", "默认加密", &a.Settings.ZipUsePassword)
	})
}

func shortcutName(key string) string {
	if key == "" {
		return "无"
	}
	return key
}
