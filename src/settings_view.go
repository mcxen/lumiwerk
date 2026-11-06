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
	{"keys", "快捷键", lucideKeyboard},
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
		Hidden:      true,
	})
	w.OnReadyToShow(w.Show)
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
						case "keys":
							a.settingsKeysPage(c, pal)
						default:
							a.settingsAdvancedPage(c, pal)
						}
					})
				})
			})
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
		ui.Text(c, p.title).FontSize(13.5).TextColor(pal.text).SingleLine().Grow(1)
		if p.id == "advanced" && availableUpdate() != "" {
			updateBadge(c, "新", pal)
		}
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
		{"general", "关闭窗口时", "close window tray quit 关闭 窗口 托盘 退出 菜单栏"},
		{"view", "隐藏文件", "hidden dotfile 隐藏"},
		{"view", "图像", "image jpg png 图像 文件"},
		{"view", "RAW", "raw cr2 nef 相机"},
		{"view", "视频", "video mp4 mov 视频 文件"},
		{"view", "压缩包", "archive zip 压缩"},
		{"view", "全部", "all types 全部 类型"},
		{"view", "包含文件夹", "folder recursive 文件夹"},
		{"view", "默认布局", "layout justified waterfall grid detail 布局"},
		{"view", "默认缩略图", "thumbnail size 缩略图"},
		{"view", "文件夹优先", "folder first sort 排序"},
		{"view", "全路径排序", "full path sort 递归 排序"},
		{"view", "媒体单元", "media unit 媒体 单元"},
		{"view", "选中即播", "autoplay video 自动 播放 选中"},
		{"keys", "快捷键", "shortcut keyboard hotkey 快捷键 按键"},
		{"actions", "按 Enter 时", "enter rename open 回车 重命名"},
		{"actions", "图片复制键", "shortcut copy image 快捷键 复制"},
		{"actions", "视频复制键", "shortcut copy video 快捷键 复制"},
		{"actions", "重命名规则", "quick rename rule 重命名"},
		{"actions", "图片复制到", "copy destination 复制 目标"},
		{"actions", "视频复制到", "copy destination 复制 目标"},
		{"actions", "播放结束", "playback end loop next 播放 循环"},
		{"actions", "记住播放进度", "remember play progress resume 播放 进度 续播"},
		{"actions", "默认密码", "zip password archive 压缩 密码"},
		{"advanced", "缩略图缓存", "cache limit 缓存 MiB"},
		{"advanced", "更新源", "update source mirror github tvv"},
		{"advanced", "缓存", "clear cache 清理 缓存"},
		{"advanced", "诊断", "diagnostics 诊断 日志"},
	}
}
