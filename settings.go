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
	}
	a.Settings.Open = true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (a *appState) settingsWindow() {
	if a.Settings.Window != nil {
		a.Settings.Window.Focus()
		return
	}
	a.openSettings()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:       "操作",
		Width:       910,
		Height:      760,
		MinWidth:    780,
		MinHeight:   640,
		Content:     ui.View(a.settingsView),
		AlwaysOnTop: true,
	})
	a.Settings.Window = w
	w.OnClosed(func() {
		a.Settings.Window = nil
		a.Settings.Open = false
	})
}

func (a *appState) settingsView(c *ui.Context) {
	safeView("settings", c, func() { a.settingsBody(c) })
}

func (a *appState) settingsBody(c *ui.Context) {
	t := c.Theme()
	root := ui.Column(c).Fill().Gap(0)
	root.Children(func() {
		tabs := ui.Row(c).FillWidth().Justify(ui.Center).Background(t.Surface).Gap(8).Padding(14, 12)
		tabs.Children(func() {
			icons := []*ui.SVG{sidebarIcon, imageIcon, videoIcon, starIcon, clockIcon}
			for i, name := range []string{"通用", "查看", "操作", "标签", "高级"} {
				i := i
				tab := ui.Column(c).Width(96).Padding(10, 12).Radius(6).AlignItems(ui.Center).Gap(5)
				if a.Settings.Pane == i {
					tab.Background(t.SurfaceHover).TextColor(t.Accent)
				}
				tab.Children(func() {
					ui.Icon(c, icons[i]).FontSize(23)
					ui.Text(c, name).FontSize(12).Bold()
				})
				tab.OnClick(func() { a.Settings.Pane = i })
			}
		})
		ui.Divider(c)
		pane := ui.Column(c).Grow(1).Padding(22, 30).Gap(18)
		pane.Children(func() {
			switch a.Settings.Pane {
			case 0:
				a.generalSettings(c)
			case 1:
				a.viewSettings(c)
			case 2:
				a.actionSettings(c)
			case 3:
				a.tagSettings(c)
			default:
				a.advancedSettings(c)
			}
		})
	})
}

func (a *appState) generalSettings(c *ui.Context) {
	ui.Text(c, "常规").FontSize(16).Bold()
	ui.Slider(c, &a.Settings.Thumb, 64, 512).Label("默认缩略图")
	ui.MenuButton(c, "排序", func(m *ui.Menu) {
		for _, item := range []struct{ label, value string }{
			{"名称", "name"}, {"日期", "date"}, {"大小", "size"},
		} {
			if m.Item(item.label).Radio(a.Settings.Sort == item.value).Chosen() {
				a.Settings.Sort = item.value
			}
		}
	})
	if ui.Button(c.Key("settings-save"), "应用").Clicked() {
		a.Thumb = a.Settings.Thumb
		a.Sort = a.Settings.Sort
		if a.Window != nil {
			a.update(func() { a.refresh() })
		}
	}
}

func (a *appState) viewSettings(c *ui.Context) {
	ui.Text(c, "查看").FontSize(16).Bold()
	ui.Text(c, "方向键、滚轮和 W/A/S/D 手势用于浏览媒体。").TextColor(c.Theme().TextMuted)
}

func (a *appState) tagSettings(c *ui.Context) {
	ui.Text(c, "标签").FontSize(16).Bold()
	ui.Text(c, "在文件的右键菜单中设置 Finder 标签和评分。").TextColor(c.Theme().TextMuted)
}

func (a *appState) advancedSettings(c *ui.Context) {
	ui.Text(c, "高级").FontSize(16).Bold()
	ui.Slider(c, &a.Settings.CacheCap, 64, 1024).Label("缩略图缓存数量")
	ui.Slider(c, &a.Settings.Workers, 1, 16).Label("后台解码并发")
	if ui.Button(c.Key("settings-save"), "应用").Clicked() {
		thumbLRU.limit = int(a.Settings.CacheCap)
	}
}

func (a *appState) actionSettings(c *ui.Context) {
	t := c.Theme()
	section := func(title string, body func()) { settingsSection(c, title, body) }

	section("Enter 键：", func() {
		ui.Row(c).Gap(28).Children(func() {
			ui.Radio(c, &a.Settings.EnterOpens, false, "重命名")
			ui.Radio(c, &a.Settings.EnterOpens, true, "打开 / 关闭")
		})
	})
	ui.Divider(c)
	section("操作指引：", func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("operation-guide"), "操作说明").Clicked() {
				safeGo("dialog.operationGuide", func() { a.showOperationGuide(false) })
			}
			if ui.Button(c.Key("shortcut-guide"), "快捷键对照").Clicked() {
				safeGo("dialog.shortcutGuide", func() { a.showOperationGuide(true) })
			}
		})
		ui.Text(c, "点击按钮后会以弹窗显示说明，按 Esc 可先关闭弹窗，再按 Esc 关闭设置窗口。").FontSize(11).TextColor(t.TextMuted)
	})
	ui.Divider(c)
	section("文件操作：", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "快速重命名：").FontSize(12)
			ui.TextInput(c, &a.Settings.QuickRule).Grow(1).MinWidth(160)
			if ui.Button(c.Key("settings-apply"), "应用").Clicked() {
				a.applyActionSettings()
			}
		})
		a.destinationSetting(c, "图片文件：", &a.Settings.Dest, "image")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "复制快捷键：").FontSize(12)
			ui.MenuButton(c, a.Settings.ImageShortcut, func(m *ui.Menu) {
				a.shortcutMenu(m, &a.Settings.ImageShortcut)
			})
		})
		a.destinationSetting(c, "视频文件：", &a.Settings.VideoDest, "video")
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "视频复制快捷键：").FontSize(12)
			ui.MenuButton(c, a.Settings.VideoShortcut, func(m *ui.Menu) {
				a.shortcutMenu(m, &a.Settings.VideoShortcut)
			})
		})
		ui.Checkbox(c, &a.Settings.VideoShift, "视频 Shift+左右 切换上/下文件")
		ui.Checkbox(c, &a.Settings.ShowArchives, "显示压缩文件")
	})
	section("ZIP 默认密码：", func() {
		ui.TextInput(c, &a.Settings.ZipPassword).Password().Tooltip("可选").Grow(1)
		ui.Checkbox(c, &a.Settings.ZipUsePassword, "默认使用上面的密码进行加密压缩")
	})
	if warning := shortcutConflict(a.Settings.ImageShortcut, a.Settings.VideoShortcut); warning != "" {
		ui.Text(c, warning).FontSize(11).TextColor(t.Warning)
	}
}

func (a *appState) destinationSetting(c *ui.Context, label string, value *string, kind string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12)
		ui.TextInput(c, value).Tooltip(*value).Grow(1).MinWidth(160)
		if ui.Button(c.Key("settings-pick-"+kind), "选择文件夹").Clicked() {
			safeGo("dialog.pickActionDestination", func() { a.pickActionDestination(kind) })
		}
	})
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
