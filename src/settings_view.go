// Settings window in the Kero/GodGo layout: a left nav with search, and a
// raised card on the right holding the active page as grouped rows. Layout
// adapted from github.com/egoist/kero (MIT) via GodGo.
package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingsPage struct {
	id    string
	title string
	icon  *ui.SVG
}

var settingsPages = visibleSettingsPages([]settingsPage{
	{"general", "通用", lucideSliders},
	{"view", "查看", lucideImage},
	{"actions", "操作", lucideMouse},
	{"tags", "标签", lucideTag},
	{"advanced", "高级", lucideTerminal},
})

// visibleSettingsPages drops the tags page where the OS has no file tags.
func visibleSettingsPages(all []settingsPage) []settingsPage {
	if tagsSupported {
		return all
	}
	out := all[:0:0]
	for _, p := range all {
		if p.id != "tags" {
			out = append(out, p)
		}
	}
	return out
}

type settingsSearch struct {
	query string
	focus ui.Handle
}

// settingsExtra holds state beyond the persisted settingsState.
type settingsExtra struct {
	search settingsSearch
	hits   map[string]bool
	reveal string
}

var settingsUI = struct {
	extra map[*mygo.Window]*settingsExtra
}{extra: map[*mygo.Window]*settingsExtra{}}

func settingsExtraFor(w *mygo.Window) *settingsExtra {
	e, ok := settingsUI.extra[w]
	if !ok {
		e = &settingsExtra{}
		settingsUI.extra[w] = e
	}
	return e
}

func (a *appState) settingsWindow() {
	if a.Settings.Window != nil {
		a.Settings.Window.Show()
		a.Settings.Window.Focus()
		return
	}
	a.openSettings()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:       "设置",
		Width:       860,
		Height:      640,
		MinWidth:    720,
		MinHeight:   480,
		StateKey:    "settings",
		Content:     ui.View(a.settingsView),
		AlwaysOnTop: true,
	})
	a.Settings.Window = w
	w.OnClosed(func() {
		delete(settingsUI.extra, w)
		a.Settings.Window = nil
		a.Settings.Open = false
	})
}

func (a *appState) settingsView(c *ui.Context) {
	safeView("settings", c, func() { a.settingsBody(c) })
}

func (a *appState) settingsPageID() string {
	if a.Settings.Pane < 0 || a.Settings.Pane >= len(settingsPages) {
		return "general"
	}
	return settingsPages[a.Settings.Pane].id
}

func (a *appState) setPage(id string) {
	for i, p := range settingsPages {
		if p.id == id {
			a.Settings.Pane = i
			return
		}
	}
}

func (a *appState) settingsBody(c *ui.Context) {
	pal := settingsPaletteFor(c)
	c.SetTheme(settingsTheme(c, pal))
	e := settingsExtraFor(a.Settings.Window)
	found := searchSettingsPages(e, a.Settings.Pane)

	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	row.Children(func() {
		// Left nav: search, pages, version footer.
		ui.Column(c).Width(212).Shrink(0).Padding(12, 10, 12, 10).Gap(2).Children(func() {
			settingsSearchField(c, e, pal)
			for _, p := range settingsPages {
				if found == nil || found[p.id] != nil {
					settingsNavRow(c, a, p, pal)
				}
			}
			if found != nil && len(found) == 0 {
				ui.Text(c, "没有匹配的设置").FontSize(13).TextColor(pal.textFaint).Padding(8, 10)
			}
			ui.Spacer(c)
			ui.Text(c, "Lumiwerk "+appVersion).FontSize(11.5).TextColor(pal.textFaint).Padding(8, 10)
		})
		// Right card: header + scrolling page.
		ui.Column(c).Grow(1).MinWidth(0).Padding(8, 8, 8, 0).Children(func() {
			card := raised(ui.Column(c).Grow(1).MinHeight(0).Radius(14).Background(pal.card), pal).Clip()
			card.Children(func() {
				page := settingsPages[0]
				for _, p := range settingsPages {
					if p.id == a.settingsPageID() {
						page = p
					}
				}
				ui.Scroll(c.Key("page-"+page.id)).Fill().Padding(24, 24, 28, 24).Children(func() {
					ui.Column(c).Gap(24).MaxWidth(620).Children(func() {
						ui.Text(c, page.title).FontSize(16).FontWeight(600).TextColor(pal.text)
						switch page.id {
						case "general":
							a.settingsGeneralPage(c, pal)
						case "view":
							a.settingsViewPage(c, pal)
						case "actions":
							a.settingsActionsPage(c, pal)
						case "tags":
							a.settingsTagsPage(c, pal)
						default:
							a.settingsAdvancedPage(c, pal)
						}
					})
				})
			})
		})
	})
}

// settingsGroup is GodGo's group: a muted title, a rounded chip container
// of rows, and an optional note under it.
func settingsGroup(c *ui.Context, pal *settingsPalette, title, note string, fn func()) {
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(pal.textMuted).Padding(0, 4)
		}
		rows := ui.Column(c).Radius(12).Background(pal.chip).Padding(4, 16).Children(fn)
		_ = rows
		if note != "" {
			ui.Text(c, note).FontSize(12).TextColor(pal.textFaint).Padding(0, 4)
		}
	})
}

// settingsRow is GodGo's row: label and optional detail on the left,
// control on the right, hairline separators between rows.
func settingsRow(c *ui.Context, pal *settingsPalette, key, label, detail string, control func()) {
	ui.Column(c).Gap(0).Children(func() {
		row := ui.Row(c.Key("row-"+key)).Padding(11, 0).Gap(16).AlignItems(ui.Center)
		if detail != "" {
			row.AlignItems(ui.Center)
		}
		row.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, label).FontSize(13.5).TextColor(pal.text)
				if detail != "" {
					ui.Text(c, detail).FontSize(12).TextColor(pal.textMuted).MinWidth(0)
				}
			})
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
		})
	})
}

func settingsNavRow(c *ui.Context, a *appState, p settingsPage, pal *settingsPalette) {
	sel := a.settingsPageID() == p.id
	item := ui.Row(c.Key(p.id)).Height(32).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8).Label(p.title)
	bg := ui.Transparent
	switch {
	case sel:
		bg = pal.rowSelected
	case item.Hovered():
		bg = pal.rowHover
	}
	item.Background(item.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if (item.Pressed() || item.Clicked()) && !sel {
		a.setPage(p.id)
	}
	item.Children(func() {
		ic := ui.Icon(c, p.icon).Size(15, 15).TextColor(pal.textMuted)
		if sel {
			ic.TextColor(pal.accent)
		}
		ui.Text(c, p.title).FontSize(13.5).TextColor(pal.text).SingleLine()
	})
}

func settingsSearchField(c *ui.Context, e *settingsExtra, pal *settingsPalette) {
	var in ui.Element
	frame := ui.Row(c.Key("search")).Height(28).Padding(0, 8).Gap(6).AlignItems(ui.Center).
		Radius(8).Margin(0, 0, 10, 0).Background(pal.chip).Cursor(ui.CursorText)
	frame.OnClick(func() {})
	frame.Children(func() {
		ui.Icon(c, lucideSearch).Size(13, 13).TextColor(pal.textFaint).Shrink(0)
		in = ui.TextInputBase(c.Key("in"), &e.search.query).Bind(&e.search.focus).Grow(1).MinWidth(0).FontSize(13).
			Placeholder("搜索设置").Label("搜索设置")
		in.OnShortcut(0, ui.KeyEscape, func() { e.search.query = "" })
	})
	if in.Focused() {
		frame.Border(1.5, pal.accent.Alpha(0.55))
	}
}

func searchSettingsPages(e *settingsExtra, active int) map[string][]string {
	terms := strings.Fields(strings.ToLower(e.search.query))
	if len(terms) == 0 {
		e.hits = nil
		return nil
	}
	match := func(s string) bool {
		s = strings.ToLower(s)
		for _, term := range terms {
			if !strings.Contains(s, term) {
				return false
			}
		}
		return true
	}
	found := map[string][]string{}
	for _, p := range settingsPages {
		if match(p.title) {
			found[p.id] = []string{}
		}
	}
	for _, entry := range settingsSearchEntries() {
		if match(entry.label + " " + entry.words) {
			found[entry.page] = append(found[entry.page], entry.label)
		}
	}
	if found[settingsPages[active].id] == nil {
		for _, p := range settingsPages {
			if found[p.id] != nil {
				settingsPages[active] = p // not enough; page switch handled by caller below
				break
			}
		}
	}
	e.hits = map[string]bool{}
	for _, label := range found[settingsPages[active].id] {
		e.hits[label] = true
	}
	return found
}

type settingsSearchEntry struct {
	page, label, words string
}

func settingsSearchEntries() []settingsSearchEntry {
	return []settingsSearchEntry{
		{"general", "配置槽", "profile slot 配置 槽"},
		{"general", "主题", "theme light dark 主题 外观"},
		{"general", "启动目录", "startup folder 启动 目录"},
		{"general", "恢复上次会话", "restore session 恢复 会话"},
		{"general", "关闭窗口时", "close window tray quit menu bar 关闭 窗口 菜单栏 退出"},
		{"view", "隐藏文件", "hidden dotfile 隐藏"},
		{"view", "图像文件", "image jpg png 图像"},
		{"view", "RAW 文件", "raw cr2 nef 相机"},
		{"view", "视频文件", "video mp4 mov 视频"},
		{"view", "压缩文件", "archive zip 压缩"},
		{"view", "全部类型", "all types 全部 类型"},
		{"view", "包含文件夹", "folder recursive 文件夹"},
		{"view", "默认布局", "layout justified waterfall grid detail 布局"},
		{"view", "默认缩略图", "thumbnail size 缩略图"},
		{"view", "文件夹优先排序", "folder first sort 排序"},
		{"view", "全路径排序", "full path sort 递归 排序"},
		{"view", "媒体单元模式", "media unit 媒体 单元"},
		{"actions", "复制目标目录", "copy destination 复制 目标"},
		{"actions", "删除前确认", "confirm delete 删除 确认"},
		{"actions", "使用外部播放器", "external player 外部 播放器"},
		{"actions", "快捷操作", "quick action shortcut 快捷 操作"},
		{"actions", "快速重命名", "quick rename rule 重命名"},
		{"actions", "ZIP 默认密码", "zip password archive 压缩 密码"},
		{"tags", tagsLabel, "finder tag 标签"},
		{"tags", "过滤标签", "filter tag 过滤 标签"},
		{"tags", "默认评分", "rating star 评分"},
		{"advanced", "缩略图缓存大小", "cache limit mb 缓存"},
		{"advanced", "并发解码线程数", "workers decode 并发 解码 线程"},
		{"advanced", "日志级别", "log level debug 日志 级别"},
		{"advanced", "打开日志目录", "open log folder 日志 目录"},
		{"advanced", "打开诊断窗口", "diagnostics window 诊断"},
		{"advanced", "清除缩略图缓存", "clear cache 清理 缓存"},
		{"advanced", "重置偏好设置", "reset preferences 重置 偏好"},
		{"advanced", "导出诊断报告", "export report 导出 报告"},
	}
}
