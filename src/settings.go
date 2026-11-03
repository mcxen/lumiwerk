// Settings state, persistence bridge and shared helpers. The window and
// page layout live in settings_view.go and the pages in settings_pages.go
// and settings_actions.go, in the GodGo group/row layout.
package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingsState struct {
	Open           bool
	Window         *mygo.Window
	Pane           int
	Thumb          float64
	Sort           string
	Dest           string
	VideoDest      string
	ImageShortcut  string
	VideoShortcut  string
	EnterOpens     bool
	VideoShift     bool
	ShowArchives   bool
	CacheCap       float64
	Workers        float64
	QuickRule      string
	ZipPassword    string
	ZipUsePassword bool
	// StartFolder is the folder the next launch opens, kept apart from the
	// live Root so choosing one does not move the browser.
	StartFolder string
	// Layout is the default layout chosen on the view pane.
	Layout int
}

func (a *appState) openSettings() {
	a.Settings = settingsState{
		Thumb:          a.Thumb,
		Sort:           a.Sort,
		Dest:           firstNonEmpty(a.Profiles.ImageDestination, a.currentDestination()),
		VideoDest:      a.Profiles.VideoDestination,
		ImageShortcut:  a.Profiles.ImageCopyShortcut,
		VideoShortcut:  a.Profiles.VideoCopyShortcut,
		EnterOpens:     a.Profiles.EnterOpens,
		VideoShift:     a.Profiles.VideoShiftArrow,
		ShowArchives:   a.Profiles.View.Archives,
		CacheCap:       float64(thumbLRU.limit),
		Workers:        float64(cap(thumbGate)),
		QuickRule:      a.Profiles.QuickRename,
		ZipPassword:    a.Profiles.ZipPassword,
		ZipUsePassword: a.Profiles.ZipUsePassword,
		Layout:         a.LayoutMode,
	}
	if active := a.Profiles.Active; active >= 0 && active < len(a.Profiles.Profiles) {
		a.Settings.StartFolder = a.Profiles.Profiles[active].Root
	}
	a.Settings.Open = true
}

// pickStartFolder asks for the folder Lumiwerk opens on the next launch and
// stores it in the configuration slot the settings window is editing.
func (a *appState) pickStartFolder() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Settings.Window, Title: "选择启动目录", Directory: true})
	if err != nil || len(paths) == 0 {
		return
	}
	a.update(func() { a.setStartFolder(paths[0]) })
}

func (a *appState) setStartFolder(path string) {
	a.Settings.StartFolder = path
	if i := a.Profiles.Active; i >= 0 && i < len(a.Profiles.Profiles) {
		a.Profiles.Profiles[i].Root = path
		savePrefs(a.Profiles)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var shortcutChoices = []string{
	"A", "B", "C", "D", "E", "F", "G", "H", "J", "K", "L", "N", "O", "P", "V", "Y",
	"0", "6", "7", "8", "9", "=", "-", ",", ".", "[", "]",
	"F1", "F4", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
}

func (a *appState) shortcutMenu(m *ui.Menu, selected *string) {
	for _, key := range shortcutChoices {
		key := key
		if m.Item(key).Radio(*selected == key).Chosen() {
			*selected = key
			a.applyActionSettings()
		}
	}
}

func shortcutConflict(image, video string) string {
	var warnings []string
	if image != "" && image == video {
		warnings = append(warnings, fmt.Sprintf("图片与视频复制使用了同一个快捷键：%s", image))
	}
	reserved := map[string]string{
		"A": "上一项", "D": "下一项", "W": "放大", "S": "缩小",
		"Q": "左旋", "E": "右旋", "F": "镜像", "T": "窗口置顶",
		"Z": "缩放 100%", "X": "缩放适合", "M": "移动到下载",
		"U": "文件简介", "I": "信息/EXIF", "O": "OCR", "P": "二维码",
		"0": "重置缩略图", "=": "缩略图放大", "-": "缩略图缩小",
		"1": "最大化窗口", "2": "合适窗口", "5": "居中",
	}
	for _, key := range []string{image, video} {
		if note, ok := reserved[strings.ToUpper(key)]; ok {
			warnings = append(warnings, fmt.Sprintf("快捷键 %s 会覆盖内置功能：%s", key, note))
		}
	}
	return strings.Join(warnings, "\n")
}

func (a *appState) applyActionSettings() {
	if rule := strings.TrimSpace(a.Settings.QuickRule); rule != "" {
		a.Profiles.QuickRename = rule
	}
	a.Profiles.EnterOpens = a.Settings.EnterOpens
	a.Profiles.VideoShiftArrow = a.Settings.VideoShift
	a.Profiles.ImageCopyShortcut = a.Settings.ImageShortcut
	a.Profiles.VideoCopyShortcut = a.Settings.VideoShortcut
	a.Profiles.ImageDestination = strings.TrimSpace(a.Settings.Dest)
	a.Profiles.VideoDestination = strings.TrimSpace(a.Settings.VideoDest)
	a.Profiles.ZipPassword = a.Settings.ZipPassword
	a.Profiles.ZipUsePassword = a.Settings.ZipUsePassword
	a.Profiles.View.Archives = a.Settings.ShowArchives
	a.Profiles.ViewSet = true
	if a.Profiles.Active >= 0 && a.Profiles.Active < len(a.Profiles.Profiles) {
		a.Profiles.Profiles[a.Profiles.Active].Destination = a.Profiles.ImageDestination
	}
	archives := a.Settings.ShowArchives
	savePrefs(a.Profiles)
	if a.Profiles.View.Archives != archives {
		a.Profiles.View.Archives = archives
		a.refresh()
	}
}

func (a *appState) pickActionDestination(kind string) {
	title := "选择图片文件夹"
	if kind == "video" {
		title = "选择视频文件夹"
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Settings.Window, Title: title, Directory: true})
	if err != nil || len(paths) == 0 {
		return
	}
	a.update(func() {
		if kind == "video" {
			a.Settings.VideoDest = paths[0]
		} else {
			a.Settings.Dest = paths[0]
		}
		a.applyActionSettings()
	})
}

func (a *appState) showOperationGuide(shortcuts bool) {
	message := "单击选择，双击打开；空格打开或播放/暂停。左右方向键切换媒体，滚轮或双指缩放，Esc 退出大图。"
	if shortcuts {
		message = fmt.Sprintf(`快捷键对照：

图片复制：%s    视频复制：%s
空格：打开/播放暂停    Esc：退出大图
左右：切换媒体（视频时左右快进/退 5 秒）
Cmd+左右：视频逐帧    Shift+左右：视频切换上/下文件
Q/E：左旋/右旋    F：镜像    Z：缩放 100%%    X：缩放适合
W/S：放大/缩小    =/-：缩略图放大/缩小    0：重置缩略图
1：最大化    2：合适窗口    5：居中    3/4：窗口适配图片
T：窗口置顶    M：移动到下载    Cmd+Down：打开选中项
I：信息/EXIF    U：文件简介    F2：重命名    F3：搜索    F5：刷新
Delete：移到废纸篓    Cmd+1~9：Finder 标签    Ctrl+1~5：评级
Alt+Enter：全屏    Opt+1~9：切换配置    Cmd+Opt+1~9：保存配置`,
			a.Settings.ImageShortcut, a.Settings.VideoShortcut)
	}
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.Settings.Window, Title: "操作指引", Message: message, Buttons: []string{"好"}})
}
