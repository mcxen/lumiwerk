package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingsState struct {
	Open           bool
	Pane           int
	Thumb          float64
	Sort           string
	Dest           string
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
		Dest:           a.currentDestination(),
		CacheCap:       float64(thumbLRU.limit),
		Workers:        float64(cap(thumbGate)),
		QuickRule:      a.Profiles.QuickRename,
		ZipPassword:    a.Profiles.ZipPassword,
		ZipUsePassword: a.Profiles.ZipUsePassword,
	}

	a.Settings.Open = true
}

func (a *appState) settingsWindow() {
	if !a.Settings.Open {
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:       "设置",
		Width:       620,
		Height:      420,
		MinWidth:    520,
		MinHeight:   360,
		Content:     ui.View(a.settingsView),
		AlwaysOnTop: true,
	})
	_ = w
}

func (a *appState) settingsView(c *ui.Context) {
	t := c.Theme()
	row := ui.Row(c).Fill().Gap(0)
	row.Children(func() {
		list := ui.Column(c).Width(180).Background(t.Surface).Gap(2).Padding(8)
		list.Children(func() {
			for i, name := range []string{"常规", "自定义", "高级"} {
				i := i
				item := ui.Text(c, name).Padding(8, 12).Radius(4)
				if a.Settings.Pane == i {
					item.TextColor(t.Accent).Bold()
				}
				item.OnClick(func() { a.Settings.Pane = i })
			}
		})
		ui.Divider(c)
		pane := ui.Column(c).Grow(1).Padding(16).Gap(12)
		pane.Children(func() {
			switch a.Settings.Pane {
			case 0:
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
			case 1:
				ui.Text(c, "自定义").FontSize(16).Bold()
				ui.Text(c, "复制目录").FontSize(12)
				if a.Settings.Dest == "" {
					ui.Text(c, "未设置").TextColor(t.TextMuted)
				} else {
					ui.Text(c, a.Settings.Dest).MaxLines(1)
				}
				if ui.Button(c.Key("settings-pick"), "选择目录…").Clicked() {
					go a.setCopyDestination()
				}
				ui.TextInput(c, &a.Settings.QuickRule).Label("快速重命名规则").Width(360)
				ui.Text(c, "默认 {folder}_{index}，也可用 {index:03} {name} {ext}").FontSize(11).TextColor(t.TextMuted)
				ui.Checkbox(c, &a.Settings.ZipUsePassword, "快速压缩使用默认密码")
				ui.TextInput(c, &a.Settings.ZipPassword).Label("默认压缩密码").Password().Width(280)
			default:
				ui.Text(c, "高级").FontSize(16).Bold()
				ui.Slider(c, &a.Settings.CacheCap, 64, 1024).Label("缩略图缓存数量")
				ui.Slider(c, &a.Settings.Workers, 1, 16).Label("后台解码并发")
			}
			if ui.Button(c.Key("settings-save"), "保存").Clicked() {
				a.Thumb = a.Settings.Thumb
				a.Sort = a.Settings.Sort
				if rule := strings.TrimSpace(a.Settings.QuickRule); rule != "" {
					a.Profiles.QuickRename = rule
				}
				a.Profiles.ZipPassword = a.Settings.ZipPassword
				a.Profiles.ZipUsePassword = a.Settings.ZipUsePassword
				savePrefs(a.Profiles)
				thumbLRU.limit = int(a.Settings.CacheCap)
				a.Settings.Open = false
				if a.Window != nil {
					a.Window.Update(func() { a.refresh() })
				}
			}
		})
	})
}
