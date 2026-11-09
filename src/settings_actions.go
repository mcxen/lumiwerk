// The 文件 settings page: where copies go, how quick rename names files and
// the default ZIP encryption.
package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *appState) settingsFilesPage(c *ui.Context, pal *settingsPalette) {
	// The ZIP fields are plain inputs with no change callback, so compare
	// them with the saved preferences after the page is drawn and save as
	// soon as one differs.
	defer func() {
		p := a.Profiles
		if a.Settings.ZipPassword != p.ZipPassword || a.Settings.ZipUsePassword != p.ZipUsePassword ||
			strings.Join(mergePasswords(splitLines(a.Settings.ZipBook)), "\n") != strings.Join(p.ZipPasswords, "\n") {
			a.applyActionSettings()
		}
	}()
	setGroup(c, "复制到文件夹", func() {
		if setText(c, "dest-image", "图片复制到", &a.Settings.Dest, "未设置", "选择…", false) {
			safeGo("dialog.pickActionDestination", func() { a.pickActionDestination("image") })
		}
		if setSelect(c, "imgshortcut", "图片复制键", &a.Settings.ImageShortcut, shortcutChoices, shortcutName) {
			a.applyActionSettings()
		}
		if setText(c, "dest-video", "视频复制到", &a.Settings.VideoDest, "未设置", "选择…", false) {
			safeGo("dialog.pickActionDestination", func() { a.pickActionDestination("video") })
		}
		if setSelect(c, "vidshortcut", "视频复制键", &a.Settings.VideoShortcut, shortcutChoices, shortcutName) {
			a.applyActionSettings()
		}
		if warning := shortcutConflict(a.Settings.ImageShortcut, a.Settings.VideoShortcut); warning != "" {
			setNote(c, warning, true)
		}
	})
	setGroup(c, "快速重命名", func() {
		if setTextHint(c, "quickrule", "重命名规则", "例如 {folder}_{index:03}", &a.Settings.QuickRule, "", "应用", false) {
			a.applyActionSettings()
		}
	})
	setGroup(c, "ZIP 加密", func() {
		setTextHint(c, "zip", "默认密码", "只保存在本机偏好中", &a.Settings.ZipPassword, "未设置", "", true)
		setToggle(c, "zipuse", "默认加密", &a.Settings.ZipUsePassword)
	})
	setGroup(c, "解压密码本", func() {
		setBook(c, "zipbook", "备用密码", "每行一个。解压加密压缩包时按顺序自动尝试，默认密码排在最前", &a.Settings.ZipBook)
		figureZipBook(c, pal, len(zipPasswordCandidates(prefsFile{
			ZipPassword:  a.Settings.ZipPassword,
			ZipPasswords: splitLines(a.Settings.ZipBook),
		})))
	})
}

func shortcutName(key string) string {
	if key == "" {
		return "无"
	}
	return key
}

// setBook is a row with a multi-line field, for a list kept one item a line.
func setBook(c *ui.Context, key, label, hint string, value *string) {
	setRowHint(c, label, hint, func() {
		ui.TextArea(c.Key(key), value).Width(setTextW).Height(76).Placeholder("每行一个密码")
	})
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}
