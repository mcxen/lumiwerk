package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func isEnabledFile(path, ext string, view viewPreferences) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	name := path
	if base := filepath.Base(path); base != "" {
		name = base
	}
	return (view.Images && isImageFile(ext)) || (view.RAW && isRawFile(ext)) ||
		(view.Videos && isVideoFile(ext)) || (view.Archives && isArchiveName(name)) ||
		(view.Installers && isInstallerName(name))
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
	hidden := a.Profiles.View.Hidden
	a.mu.Unlock()
	folderMedia.SetPreferHidden(hidden)
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
	case "installers":
		view.Installers = !view.Installers
	case "folders":
		view.RecursiveFolders = !view.RecursiveFolders
	}
	savePrefs(a.Profiles)
	a.refreshView()
}

func (a *appState) setRecursive(enabled bool) {
	a.Recursive = enabled
	a.treeRecurseOpen = ""
	a.Profiles.View.MediaUnit = false
	a.MediaUnitRoot = ""
	savePrefs(a.Profiles)
	a.refreshView()
}

// setRecurseDepth stores how many folder levels 递归浏览 includes.
// 0 is every level, 1 is child folders, 2 is one level below those.
func (a *appState) setRecurseDepth(depth int) {
	if depth < 0 || depth > 2 {
		depth = 0
	}
	if a.Profiles.View.RecurseDepth == depth {
		return
	}
	a.Profiles.View.RecurseDepth = depth
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
	item := a.visibleItem(index)
	if item == nil {
		return
	}
	if !item.IsDir && isBrowsableArchive(item.Name) {
		a.openFolder(item.Path)
		return
	}
	if !item.IsDir && !item.IsVideo && !isImageFile(item.Ext) && !isRawFile(item.Ext) {
		_ = openWithDefault(item.Path)
		return
	}
	// Videos always open in the built-in player (libmpv / FFmpeg). External
	// players are available from the context menu only.
	a.openLargeImage(index)
}

// viewMenu is the toolbar's 显示与浏览 menu: frequent toggles only.
// Toolbar controls (sidebar, 无字模式, media mode, recursion and its depth,
// layout, sort, gallery) and
// set-and-forget preferences (file types, theme) are not repeated here.
func (a *appState) viewMenu(m *ui.Menu) {
	view := a.Profiles.View
	if m.Item("显示隐藏文件").Checked(view.Hidden).Shortcut(a.menuKey(a.binding("hidden"))).Chosen() {
		a.toggleViewOption("hidden")
	}
	if m.Item("媒体单元模式").Checked(a.MediaUnitRoot != "").Chosen() {
		a.setMediaUnit(a.MediaUnitRoot == "")
	}
	if a.toolbarCompact {
		// The window is too narrow for these on the toolbar.
		if a.Recursive {
			m.Submenu("递归深度", func(sub *ui.Menu) {
				for i, name := range []string{"全部层级", "1 层", "2 层"} {
					if sub.Item(name).Checked(view.RecurseDepth == i).Chosen() {
						a.setRecurseDepth(i)
					}
				}
			})
			if m.Item("包含文件夹").Checked(view.RecursiveFolders).Chosen() {
				a.toggleViewOption("folders")
			}
		}
		if a.FilterMode != filterAll {
			images := a.FilterMode == filterImages || a.FilterMode == filterMedia
			videos := a.FilterMode == filterVideos || a.FilterMode == filterMedia
			if m.Item("媒体：图片").Checked(images).Chosen() && !(images && !videos) {
				a.setFilterMode(toFilterMode(!images, videos))
			}
			if m.Item("媒体：视频").Checked(videos).Chosen() && !(videos && !images) {
				a.setFilterMode(toFilterMode(images, !videos))
			}
		}
	}
	m.Separator()
	if m.Item("自动滚动").Checked(a.AutoScroll).Disabled(a.InLargeView).Chosen() {
		a.askAutoConfig("scroll")
	}
	if m.Item("自动播放").Checked(a.AutoPlay).Disabled(!a.InLargeView).Chosen() {
		a.askAutoConfig("play")
	}
	m.Submenu("窗口", func(sub *ui.Menu) {
		if sub.Item("窗口置顶").Checked(a.Pinned).Shortcut(a.menuKey(a.binding("pin"))).Chosen() {
			a.togglePinned()
		}
		if sub.Item("最大化窗口").Shortcut(a.menuKey(a.binding("maximize"))).Chosen() {
			a.Window.ToggleMaximize()
		}
		if sub.Item("合适窗口").Shortcut(a.menuKey(a.binding("fitwindow"))).Chosen() {
			a.Window.SetSize(1280, 820)
			a.Window.Center()
		}
		if sub.Item("调整至图片实际大小").Disabled(!a.InLargeView).Shortcut(a.menuKey(keyBinding{0, ui.Key3})).Chosen() {
			a.fitWindowToImage(false)
		}
		if sub.Item("调整至图片当前大小").Disabled(!a.InLargeView).Shortcut(a.menuKey(keyBinding{0, ui.Key4})).Chosen() {
			a.fitWindowToImage(true)
		}
		if sub.Item("窗口居中").Shortcut(a.menuKey(a.binding("center"))).Chosen() {
			a.Window.Center()
		}
	})
	m.Separator()
	if m.Item("设置…").Shortcut(a.menuKey(a.binding("settings"))).Chosen() {
		a.openSettings()
		a.settingsWindow()
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
	open := true
	title, label := "自动滚动", "滚动速度（像素/秒）"
	if a.AutoConfig == "play" {
		title, label = "自动播放", "播放间隔（秒）"
	}
	m := ui.Modal(c, &open, func() {
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
	if m.Dismissed() {
		a.AutoConfig, a.AutoValue = "", ""
	}
}

func (a *appState) fitWindowToImage(current bool) {
	item := a.currentItem()
	if item == nil {
		return
	}
	if !item.InfoLoaded {
		item.Info = loadMediaInfo(item.Path, item.IsVideo)
		item.InfoLoaded = true
		a.noteLayoutChanged()
	}
	w, h := float64(item.Info.Width), float64(item.Info.Height)
	if w <= 0 || h <= 0 {
		pixelW, pixelH := probeImageSize(item.Path)
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

func (a *appState) viewStatus() string {
	if a.MediaUnitRoot != "" {
		return fmt.Sprintf("媒体单元 · %s", pathName(a.MediaUnitRoot))
	}
	return ""
}
