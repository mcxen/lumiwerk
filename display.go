package main

import (
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func isEnabledFile(path, ext string, view viewPreferences) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	return (view.Images && isImageFile(ext)) || (view.RAW && isRawFile(ext)) ||
		(view.Videos && isVideoFile(ext)) || (view.Archives && isArchiveName(path))
}

func isHiddenPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part != "." && strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func (a *appState) stopAutomation() {
	a.AutoScroll, a.AutoPlay = false, false
	a.autoTick = time.Time{}
	a.AutoConfig, a.AutoValue = "", ""
}

func (a *appState) refreshView() {
	a.stopAutomation()
	a.stopWatcher()
	a.mu.Lock()
	a.scanSeq++
	a.mu.Unlock()
	a.refresh()
	a.startWatcher()
}

func (a *appState) toggleViewOption(option string) {
	view := &a.Profiles.View
	switch option {
	case "hidden":
		view.Hidden = !view.Hidden
	case "all":
		view.AllTypes = !view.AllTypes
	case "images":
		view.Images = !view.Images
	case "raw":
		view.RAW = !view.RAW
	case "videos":
		view.Videos = !view.Videos
	case "archives":
		view.Archives = !view.Archives
	case "folders":
		view.RecursiveFolders = !view.RecursiveFolders
	}
	savePrefs(a.Profiles)
	a.refreshView()
}

func (a *appState) setRecursive(enabled bool) {
	a.Recursive = enabled
	a.Profiles.View.MediaUnit = false
	a.MediaUnitRoot = ""
	savePrefs(a.Profiles)
	a.refreshView()
}

func (a *appState) setMediaUnit(enabled bool) {
	a.Profiles.View.MediaUnit = enabled
	a.Recursive = false
	a.MediaUnitRoot = ""
	if enabled && a.Root != "" {
		a.MediaUnitRoot = a.Root
	}
	savePrefs(a.Profiles)
	a.refreshView()
}

func (a *appState) setTheme(theme string) {
	a.Profiles.View.Theme = theme
	mygo.Theme.SetSource(mygo.ThemeSource(theme))
	savePrefs(a.Profiles)
}

func (a *appState) togglePinned() {
	a.Pinned = !a.Pinned
	a.Window.SetAlwaysOnTop(a.Pinned)
}

func (a *appState) openItem(index int) {
	if index < 0 || index >= len(a.Visible) {
		return
	}
	item := a.Items[a.Visible[index]]
	if !item.IsDir && !item.IsVideo && !isImageFile(item.Ext) && !isRawFile(item.Ext) {
		_ = exec.Command("open", item.Path).Start()
		return
	}
	if item.IsVideo && !a.Profiles.View.InternalPlayer {
		_ = exec.Command("open", item.Path).Start()
		return
	}
	a.openLargeImage(index)
}

func (a *appState) viewMenu(m *ui.Menu) {
	view := a.Profiles.View
	if m.Item("置顶").Checked(a.Pinned).Shortcut(0, ui.KeyT).Chosen() {
		a.togglePinned()
	}
	if m.Item("设置…").Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
		a.openSettings()
		a.settingsWindow()
	}
	if m.Item("自定义布局样式…").Chosen() {
		a.openSettings()
		a.Settings.Pane = 0
		a.settingsWindow()
	}
	m.Separator()
	for _, item := range []struct {
		label, option string
		on            bool
	}{
		{"显示隐藏文件", "hidden", view.Hidden},
		{"显示所有类型文件", "all", view.AllTypes},
		{"显示图像文件", "images", view.Images},
		{"显示相机 RAW 文件", "raw", view.RAW},
		{"显示视频文件", "videos", view.Videos},
		{"显示压缩文件", "archives", view.Archives},
	} {
		if m.Item(item.label).Checked(item.on).Chosen() {
			a.toggleViewOption(item.option)
		}
	}
	m.Separator()
	if m.Item("返回上一级目录").Disabled(a.Root == "" || a.Root == "/").Shortcut(ui.Cmd, ui.KeyUp).Chosen() {
		a.goUp()
	}
	m.Separator()
	if m.Item("自动播放选中视频").Checked(view.AutoPlaySelected).Chosen() {
		a.Profiles.View.AutoPlaySelected = !view.AutoPlaySelected
		savePrefs(a.Profiles)
	}
	if m.Item("使用内置视频播放器").Checked(view.InternalPlayer).Chosen() {
		a.Profiles.View.InternalPlayer = !view.InternalPlayer
		savePrefs(a.Profiles)
	}
	m.Item("说明…").Disabled(true)
	m.Separator()
	if m.Item("递归浏览模式").Checked(a.Recursive).Shortcut(ui.Cmd|ui.Shift, ui.KeyR).Chosen() {
		a.setRecursive(!a.Recursive)
	}
	if m.Item("包含文件夹").Checked(view.RecursiveFolders).Disabled(!a.Recursive).Shortcut(ui.Cmd|ui.Shift, ui.KeyF).Chosen() {
		a.toggleViewOption("folders")
	}
	m.Item("说明…").Disabled(true)
	if m.Item("媒体单元模式").Checked(a.MediaUnitRoot != "").Chosen() {
		a.setMediaUnit(a.MediaUnitRoot == "")
	}
	m.Separator()
	if m.Item("启用自动滚动").Checked(a.AutoScroll).Disabled(a.InLargeView).Chosen() {
		a.askAutoConfig("scroll")
	}
	if m.Item("启用自动播放").Checked(a.AutoPlay).Disabled(!a.InLargeView).Chosen() {
		a.askAutoConfig("play")
	}
	m.Separator()
	if m.Item("最大化窗口").Shortcut(0, ui.Key1).Chosen() {
		a.Window.ToggleMaximize()
	}
	if m.Item("合适窗口大小").Shortcut(0, ui.Key2).Chosen() {
		a.Window.SetSize(1280, 820)
		a.Window.Center()
	}
	if m.Item("调整窗口至图片实际大小").Disabled(!a.InLargeView).Shortcut(0, ui.Key3).Chosen() {
		a.fitWindowToImage(false)
	}
	if m.Item("调整窗口至图片当前大小").Disabled(!a.InLargeView).Shortcut(0, ui.Key4).Chosen() {
		a.fitWindowToImage(true)
	}
	if m.Item("将窗口居中").Shortcut(0, ui.Key5).Chosen() {
		a.Window.Center()
	}
	m.Separator()
	for _, item := range []struct{ label, theme string }{
		{"跟随系统主题", "system"}, {"浅色模式", "light"}, {"黑暗模式", "dark"},
	} {
		if m.Item(item.label).Radio(view.Theme == item.theme).Chosen() {
			a.setTheme(item.theme)
		}
	}
}

func (a *appState) askAutoConfig(kind string) {
	if kind == "scroll" && a.AutoScroll {
		a.AutoScroll = false
		return
	}
	if kind == "play" && a.AutoPlay {
		a.AutoPlay = false
		return
	}
	a.AutoConfig = kind
	if kind == "play" {
		a.AutoValue = strconv.FormatFloat(a.Profiles.View.AutoPlayInterval, 'f', -1, 64)
	} else {
		a.AutoValue = strconv.FormatFloat(a.Profiles.View.AutoScrollSpeed, 'f', -1, 64)
	}
}

func (a *appState) commitAutoConfig() {
	value, err := strconv.ParseFloat(strings.TrimSpace(a.AutoValue), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		a.toast("请输入大于 0 的数字")
		return
	}
	if a.AutoConfig == "play" {
		a.Profiles.View.AutoPlayInterval = value
		a.AutoPlay = true
	} else {
		a.Profiles.View.AutoScrollSpeed = value
		a.AutoScroll = true
	}
	savePrefs(a.Profiles)
	a.AutoConfig = ""
	a.AutoValue = ""
	a.autoTick = time.Time{}
}

func nextAutoTick(now, last time.Time, interval float64) (time.Time, bool) {
	if last.IsZero() {
		return now.Add(time.Duration(interval * float64(time.Second))), false
	}
	if now.Before(last) {
		return last, false
	}
	return now.Add(time.Duration(interval * float64(time.Second))), true
}

func (a *appState) tickAutomation(c *ui.Context) {
	if !a.AutoScroll && !a.AutoPlay {
		return
	}
	if a.modalOpen() || a.Renaming || a.NewFolder || a.Batch.Open {
		c.After(time.Second / 4)
		return
	}
	now := time.Now()
	if a.autoTick.IsZero() {
		a.autoTick = now
	}
	if a.AutoScroll && !a.InLargeView {
		due, advance := nextAutoTick(now, a.autoTick, 1.0/60.0)
		a.autoTick = due
		if advance {
			a.browseScroll.Y += float32(a.Profiles.View.AutoScrollSpeed / 60)
			if a.browseScroll.MaxY > 0 && a.browseScroll.Y >= a.browseScroll.MaxY {
				a.browseScroll.Y = a.browseScroll.MaxY
				a.AutoScroll = false
			}
		}
		c.After(time.Second / 60)
	} else if a.AutoPlay && a.InLargeView {
		due, advance := nextAutoTick(now, a.autoTick, a.Profiles.View.AutoPlayInterval)
		a.autoTick = due
		if advance {
			if a.LargeIndex >= 0 {
				last := true
				for i := a.LargeIndex + 1; i < len(a.Visible); i++ {
					if !a.Items[a.Visible[i]].IsDir {
						last = false
						break
					}
				}
				if last {
					a.AutoPlay = false
					return
				}
			}
			a.nextImage()
		}
		c.After(time.Second / 20)
	}
}

func (a *appState) autoConfigDialog(c *ui.Context) {
	if a.AutoConfig == "" {
		return
	}
	t := c.Theme()
	title, label := "自动滚动", "滚动速度（像素/秒）"
	if a.AutoConfig == "play" {
		title, label = "自动播放", "播放间隔（秒）"
	}
	panel := ui.Column(c).Attach(ui.AnchorCenter, ui.AnchorCenter).Width(340).Padding(16).Gap(10).
		Background(t.Surface).Border(1, t.Border).Radius(8)
	panel.Children(func() {
		ui.Text(c, title).Bold().FontSize(14)
		field := ui.TextInput(c, &a.AutoValue).Label(label).Width(300)
		if field.Submitted() {
			a.commitAutoConfig()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("auto-cancel"), "取消").Clicked() {
				a.AutoConfig, a.AutoValue = "", ""
			}
			if ui.Button(c.Key("auto-ok"), "开始").Clicked() {
				a.commitAutoConfig()
			}
		})
	})
}

func (a *appState) fitWindowToImage(current bool) {
	item := a.currentItem()
	if item == nil {
		return
	}
	if !item.InfoLoaded {
		item.Info = loadMediaInfo(item.Path, item.IsVideo)
		item.InfoLoaded = true
	}
	w, h := float64(item.Info.Width), float64(item.Info.Height)
	if w <= 0 || h <= 0 {
		pixelW, pixelH := pixelSize(item.Path)
		w, h = float64(pixelW), float64(pixelH)
	}
	if w <= 0 || h <= 0 {
		a.toast("无法读取图片尺寸")
		return
	}
	if int(a.Rotation)%180 != 0 {
		w, h = h, w
	}
	scale := 1.0
	if current {
		scale = float64(a.Zoom) * float64(a.fitScale(item))
		if scale <= 0 {
			scale = float64(a.fitScale(item))
		}
	}
	// Leave room for the toolbar and a modest desktop margin.
	a.Window.SetSize(int(w*scale)+32, int(h*scale)+112)
	a.Window.Center()
}
