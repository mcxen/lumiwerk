// Lumiwerk — FlowVision-aligned state, views, and file/media engine.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	appName                     = "Lumiwerk"
	appVersion                  = "0.2.0"
	rootName                    = "Macintosh HD"
	sidebarDefaultWidth float32 = 279
)

var (
	dirIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`))
	imageIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m21 16-5-5-7 7"/></svg>`))
	videoIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="5" width="12" height="14" rx="2"/><path d="m21 8-6 4 6 4z"/></svg>`))
	starIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3l2.9 5.9 6.1.9-4.5 4.4 1.1 6.3-5.6-3-5.6 3 1.1-6.3L3 9.8l6.1-.9z"/></svg>`))
	clockIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/></svg>`))
)

type mediaItem struct {
	Path      string
	Name      string
	Ext       string
	Size      int64
	ModTime   int64
	Bitmap    *ui.Bitmap
	Decoded   bool
	Failed    bool
	Loading   bool
	IsVideo   bool
	ThumbDone bool
}

// videoView owns a real MyGo player for the video currently in large view.
type videoView struct {
	player *ui.VideoPlayer
	path   string
	err    error
}

type appState struct {
	Window *mygo.Window
	mu     sync.Mutex

	Root       string
	Items      []mediaItem
	Visible    []int
	Recursive  bool
	scanSeq    int
	watchCtx   context.Context
	watchStop  context.CancelFunc
	refreshing bool

	History []string
	Cursor  int

	Grid      ui.GridState
	ListState ui.ListState
	Chosen    int

	Outline      ui.OutlineState[string]
	TreeChosen   int
	SidebarWidth float32
	SidebarShown bool
	Breadcrumb   int

	Query      string
	Sort       string
	LayoutMode int
	Thumb      float64

	InLargeView bool
	LargeIndex  int
	Zoom        float32
	Rotation    float32
	Mirror      bool
	Video       *videoView
}

func newAppMenu(a *appState) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开目录…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { go a.pickFolder() }},
			{Label: "上级目录", Accelerator: "CmdOrCtrl+Up", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.goUp() }) }},
			mygo.Separator(),
			{Label: "递归浏览", Type: mygo.MenuItemCheckbox, Click: func(item *mygo.MenuItem, _ *mygo.Window) {
				a.Window.Update(func() {
					a.Recursive = item.IsChecked()
					a.refresh()
				})
			}},
		}},
		{Label: "显示", Submenu: []*mygo.MenuItem{
			{Label: "隐藏/显示侧栏", Accelerator: "CmdOrCtrl+B", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.SidebarShown = !a.SidebarShown }) }},
		}},
	})
}

// ============================
// File System Engine
// ============================

func isImageFile(ext string) bool {
	switch strings.ToLower(ext) {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "tiff", "tif", "ico":
		return true
	}
	return false
}

func isVideoFile(ext string) bool {
	switch strings.ToLower(ext) {
	case "mp4", "mov", "mkv", "avi", "webm", "m4v", "ts", "mts":
		return true
	}
	return false
}

func isMediaFile(ext string) bool { return isImageFile(ext) || isVideoFile(ext) }

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathName(path string) string {
	if path == "/" {
		return rootName
	}
	return filepath.Base(path)
}

func (a *appState) openFolder(path string) {
	path = filepath.Clean(path)
	if path == "" || !isDir(path) || path == a.Root {
		return
	}
	a.stopWatcher()
	a.mu.Lock()
	a.Root = path
	a.Query = ""
	a.Items = nil
	a.Visible = nil
	a.Chosen = -1
	a.InLargeView = false
	a.LargeIndex = -1
	a.scanSeq++
	if a.Cursor >= 0 && a.Cursor < len(a.History) && a.History[a.Cursor] == path {
		// already positioned.
	} else if a.Cursor < 0 || a.Cursor >= len(a.History) {
		a.History = append(a.History, path)
		a.Cursor = len(a.History) - 1
	} else {
		a.History = append(a.History[:a.Cursor+1], path)
		a.Cursor = len(a.History) - 1
	}
	a.mu.Unlock()
	a.Window.SetTitle(fmt.Sprintf("%s — %s", appName, pathName(path)))
	a.refresh()
	a.startWatcher()
}

func (a *appState) refresh() {
	a.mu.Lock()
	root := a.Root
	seq := a.scanSeq
	a.mu.Unlock()
	if root == "" {
		return
	}
	go a.scanFolder(root, seq)
}

func (a *appState) scanFolder(root string, seq int) {
	var files []mediaItem
	if a.Recursive {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			if !isMediaFile(ext) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			files = append(files, mediaItem{Path: path, Name: filepath.Base(path), Ext: strings.ToLower(ext), Size: info.Size(), ModTime: info.ModTime().Unix(), IsVideo: isVideoFile(ext)})
			return nil
		})
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.TrimPrefix(filepath.Ext(e.Name()), ".")
			if !isMediaFile(ext) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, mediaItem{Path: filepath.Join(root, e.Name()), Name: e.Name(), Ext: strings.ToLower(ext), Size: info.Size(), ModTime: info.ModTime().Unix(), IsVideo: isVideoFile(ext)})
		}
	}
	slices.SortStableFunc(files, func(x, y mediaItem) int {
		switch a.Sort {
		case "date":
			return int(y.ModTime - x.ModTime)
		case "size":
			return int(y.Size - x.Size)
		default:
			return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		}
	})
	a.Window.Update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.Root != root || a.scanSeq != seq {
			return
		}
		a.Items = files
		a.applyFilterLocked()
	})
}

func (a *appState) applyFilterLocked() {
	a.Visible = a.Visible[:0]
	q := strings.ToLower(strings.TrimSpace(a.Query))
	for i := range a.Items {
		if q == "" || strings.Contains(strings.ToLower(a.Items[i].Name), q) {
			a.Visible = append(a.Visible, i)
		}
	}
	if a.Chosen >= len(a.Visible) {
		a.Chosen = -1
	}
}

func (a *appState) startWatcher() {
	if a.Root == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.watchCtx = ctx
	a.watchStop = cancel
	root := a.Root
	recursive := a.Recursive
	a.mu.Unlock()

	go func() {
		w, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{Recursive: recursive, Debounce: 100 * time.Millisecond})
		if err != nil {
			return
		}
		a.mu.Lock()
		if a.Root != root || a.watchCtx != ctx {
			_ = w.Close()
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()

		for {
			events, err := w.Next(ctx)
			if err != nil {
				_ = w.Close()
				return
			}
			if len(events) > 0 {
				a.Window.Update(func() { a.refresh() })
			}
		}
	}()
}

func (a *appState) stopWatcher() {
	a.mu.Lock()
	cancel := a.watchStop
	a.watchCtx = nil
	a.watchStop = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *appState) goUp() {
	if a.Root == "" {
		return
	}
	parent := filepath.Dir(a.Root)
	if parent != a.Root {
		a.openFolder(parent)
	}
}

func (a *appState) goBack() {
	a.mu.Lock()
	if a.Cursor <= 0 {
		a.mu.Unlock()
		return
	}
	a.Cursor--
	path := a.History[a.Cursor]
	a.mu.Unlock()
	a.openFolder(path)
}

func (a *appState) goForward() {
	a.mu.Lock()
	if a.Cursor+1 >= len(a.History) {
		a.mu.Unlock()
		return
	}
	a.Cursor++
	path := a.History[a.Cursor]
	a.mu.Unlock()
	a.openFolder(path)
}

func (a *appState) treeChildren(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

func (a *appState) pickFolder() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: "打开媒体目录", Directory: true})
	if err != nil || len(paths) == 0 {
		return
	}
	a.Window.Update(func() { a.openFolder(paths[0]) })
}

func (a *appState) loadThumb(root string, index int) {
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) {
		a.mu.Unlock()
		return
	}
	item := &a.Items[index]
	if item.Decoded || item.Failed || item.Loading {
		a.mu.Unlock()
		return
	}
	item.Loading = true
	path := item.Path
	video := item.IsVideo
	a.mu.Unlock()

	go func() {
		var data []byte
		var err error
		if video {
			data, err = videoPosterPNG(path)
		} else {
			data, err = os.ReadFile(path)
		}
		var bitmap *ui.Bitmap
		if err == nil {
			bitmap, err = ui.DecodeBitmap(data)
		}
		a.Window.Update(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.Root != root || index >= len(a.Items) || a.Items[index].Path != path {
				return
			}
			current := &a.Items[index]
			if err != nil {
				current.Failed = true
			} else {
				current.Bitmap = bitmap
				current.Decoded = true
			}
			current.Loading = false
		})
	}()
}

// ============================
// Views
// ============================

func (a *appState) view(c *ui.Context) {
	t := c.Theme()
	c.Root().Background(t.Background)

	// Global shortcuts.
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		go a.pickFolder()
	}
	if c.Shortcut(ui.Cmd, ui.KeyUp) {
		a.goUp()
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.SidebarShown = !a.SidebarShown
	}
	if c.Shortcut(ui.Cmd, ui.KeyLeft) || (a.InLargeView && c.Shortcut(0, ui.KeyLeft)) {
		if a.InLargeView {
			a.prevImage()
		} else {
			a.goBack()
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyRight) || (a.InLargeView && c.Shortcut(0, ui.KeyRight)) {
		if a.InLargeView {
			a.nextImage()
		} else {
			a.goForward()
		}
	}
	if c.Shortcut(0, ui.KeyEscape) && a.InLargeView {
		a.closeLargeImage()
	}
	if c.Shortcut(0, ui.KeySpace) && !a.InLargeView && a.Chosen >= 0 {
		a.openLargeImage(a.Chosen)
	}

	if a.SidebarWidth == 0 {
		a.SidebarWidth = sidebarDefaultWidth
	}
	if !a.SidebarShown {
		a.SidebarShown = true
	}

	if a.SidebarShown {
		ui.Split(c, &a.SidebarWidth, func() { a.sidebarView(c) }, func() { a.mainView(c) }).Fill()
	} else {
		a.mainView(c)
	}
}

func (a *appState) sidebarView(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Fill().Background(t.Surface)
	pane.Children(func() {
		ui.Row(c).Padding(8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, starIcon).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "收藏夹").FontSize(11).Bold().TextColor(t.TextMuted)
		})
		ui.Divider(c)
		if a.Root == "" {
			ui.Text(c, "打开目录后显示目录树").TextColor(t.TextMuted).Padding(12)
			return
		}
		a.Outline.List.Selected = &a.TreeChosen
		a.Outline.List.Key = func(row int) any { return a.Outline.Item(row) }
		roots := []string{"/", a.Root}
		roots = slices.Compact(slices.Sorted(slices.Values(roots)))
		ui.Outline(c, &a.Outline, roots, a.treeChildren, func(path string) { a.treeRow(c, path) }).Grow(1)
	})
}

func (a *appState) treeRow(c *ui.Context, path string) {
	t := c.Theme()
	row := ui.Row(c).Padding(4, 8).Gap(6).AlignItems(ui.Center).Radius(4)
	selected := a.Root == path
	if selected {
		row.Background(t.Accent).TextColor(t.AccentText)
	}
	row.Children(func() {
		ui.Icon(c, dirIcon).FontSize(14)
		ui.Text(c, pathName(path)).FontSize(12).SingleLine()
	})
	row.OnClick(func() { a.openFolder(path) })
}

func (a *appState) mainView(c *ui.Context) {
	t := c.Theme()
	col := ui.Column(c).Fill().Background(t.Background)
	col.Children(func() {
		a.toolbarView(c)
		ui.Divider(c)
		if a.InLargeView {
			a.largeImageView(c)
		} else if a.Root == "" {
			a.emptyView(c)
		} else if len(a.Visible) == 0 {
			a.emptyFolderView(c)
		} else if a.LayoutMode == 1 {
			a.detailView(c)
		} else {
			a.gridView(c)
		}
		a.statusBar(c)
	})
}

func (a *appState) toolbarView(c *ui.Context) {
	t := c.Theme()
	bar := ui.Toolbar(c, func() {
		if ui.Button(c.Key("sidebar"), "").Label("侧栏").Tooltip("显示/隐藏侧栏").Clicked() {
			a.SidebarShown = !a.SidebarShown
		}
		if ui.Button(c.Key("back"), "").Label("后退").Tooltip("后退").Clicked() {
			a.goBack()
		}
		if ui.Button(c.Key("forward"), "").Label("前进").Tooltip("前进").Clicked() {
			a.goForward()
		}
		if a.InLargeView {
			item := a.currentItem()
			title := "无媒体"
			if item != nil {
				title = fmt.Sprintf("%s (%d/%d)", item.Name, a.LargeIndex+1, len(a.Visible))
			}
			ui.Text(c, title).FontSize(12).Bold().Grow(1).MaxLines(1)
		} else {
			items := strings.Split(strings.ReplaceAll(a.breadcrumbPath(), " › ", "/"), "/")
			chosen := len(items) - 1
			ui.Breadcrumbs(c, items, &chosen).Label("路径").Grow(1)
			ui.Textf(c, "(%d 图像)", len(a.Visible)).FontSize(11).TextColor(t.TextMuted)
		}
		ui.SearchField(c, &a.Query).Width(180).Changed()
		ui.Segmented(c, &a.LayoutMode, "网格", "详情").Label("视图")
		ui.Slider(c, &a.Thumb, 192, 4096).Width(90)
		ui.MenuButton(c, "排序", func(m *ui.Menu) {
			for _, item := range []struct{ label, value string }{
				{"名称", "name"}, {"日期", "date"}, {"大小", "size"},
			} {
				if m.Item(item.label).Radio(a.Sort == item.value).Chosen() {
					a.Sort = item.value
					a.refresh()
				}
			}
		})
	}).Padding(6, 10).Background(t.Surface)
	_ = bar
}

func (a *appState) breadcrumbPath() string {
	if a.Root == "" {
		return "未打开目录"
	}
	parts := strings.Split(strings.Trim(a.Root, "/"), "/")
	out := []string{rootName}
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " › ")
}

func (a *appState) currentItem() *mediaItem {
	if !a.InLargeView || a.LargeIndex < 0 || a.LargeIndex >= len(a.Visible) {
		return nil
	}
	return &a.Items[a.Visible[a.LargeIndex]]
}

func (a *appState) emptyView(c *ui.Context) {
	t := c.Theme()
	zone := ui.Column(c).Fill().Center().AlignItems(ui.Center).Gap(8)
	if files := zone.DroppedFiles(); len(files) > 0 {
		for _, f := range files {
			if isDir(f) {
				a.openFolder(f)
				break
			}
		}
	}
	if zone.FileDragOver() {
		zone.Border(2, t.Accent).Radius(8)
	}
	zone.Children(func() {
		ui.Icon(c, imageIcon).FontSize(48).TextColor(t.TextMuted)
		ui.Text(c, "拖入目录开始浏览").FontSize(16).Bold()
		ui.Text(c, "或 文件 → 打开目录… (⌘O)").TextColor(t.TextMuted)
	})
}

func (a *appState) emptyFolderView(c *ui.Context) {
	zone := ui.Column(c).Fill().Center().AlignItems(ui.Center).Gap(6)
	zone.Children(func() {
		ui.Text(c, "没有媒体文件").FontSize(16).Bold()
		ui.Text(c, "打开其他目录或启用递归浏览").TextColor(c.Theme().TextMuted)
	})
}

func (a *appState) gridView(c *ui.Context) {
	t := c.Theme()
	root := a.Root
	a.Grid.Key = func(i int) any { return a.Items[a.Visible[i]].Path }
	a.Grid.Selected = &a.Chosen
	a.Grid.Label = func(i int) string { return a.Items[a.Visible[i]].Name }
	cell := int(a.Thumb/2 + 96)
	grid := ui.GridView(c, &a.Grid, len(a.Visible), float32(cell), float32(cell+28), func(i int) {
		item := &a.Items[a.Visible[i]]
		card := ui.Column(c).Grow(1).Margin(4).Padding(4).Gap(4).Radius(6).Background(t.Surface)
		if a.Chosen == i {
			card.Border(2, t.Accent)
		}
		card.Children(func() {
			thumb := ui.Box(c).Grow(1).Radius(4).Background(t.Background).Justify(ui.Center).AlignItems(ui.Center)
			thumb.Children(func() {
				if item.Bitmap != nil {
					ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1)
				} else if item.Failed {
					ui.Icon(c, videoIcon).FontSize(24).TextColor(t.TextMuted)
				} else {
					ui.Spinner(c)
				}
			})
			ui.Text(c, item.Name).FontSize(11).MaxLines(1).SingleLine().TextAlign(ui.Start)
		})
		card.Tooltip(fmt.Sprintf("%s\n%.1f MB", item.Name, float64(item.Size)/(1024*1024)))
		if !item.Decoded && !item.Failed && !item.Loading {
			a.loadThumb(root, a.Visible[i])
		}
	})
	grid.Grow(1)
	if grid.Submitted() && a.Chosen >= 0 && a.Chosen < len(a.Visible) {
		a.openLargeImage(a.Chosen)
	}
}

func (a *appState) statusBar(c *ui.Context) {
	t := c.Theme()
	bar := ui.Row(c).Padding(4, 12).Background(t.Surface)
	bar.Children(func() {
		label := "未打开目录"
		if a.Root != "" {
			label = fmt.Sprintf("%s (%d 图像)", a.Root, len(a.Visible))
		}
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *appState) largeImageView(c *ui.Context) {
	t := c.Theme()
	item := a.currentItem()
	if item == nil {
		ui.Text(c, "没有图像").TextColor(t.TextMuted)
		return
	}
	box := ui.Box(c).Fill().Background(t.Background).Justify(ui.Center).AlignItems(ui.Center)
	if ev := box.HandleInput; false {
		_ = ev
	}
	box.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputZoom:
			a.Zoom = clampZoom(a.Zoom * ev.Scale)
			return true
		case ui.InputScroll:
			if ev.Mods&ui.Ctrl != 0 {
				delta := ev.DY
				if delta == 0 {
					delta = ev.DX
				}
				a.Zoom = clampZoom(a.Zoom * (1 - delta/500))
				return true
			}
			return false
		case ui.InputKeyDown:
			switch ev.Key {
			case ui.KeyE:
				a.Rotation += 90
				return true
			case ui.KeyQ:
				a.Rotation -= 90
				return true
			case ui.KeyM:
				a.Mirror = !a.Mirror
				return true
			case ui.Key0:
				a.Zoom, a.Rotation, a.Mirror = 1, 0, false
				return true
			case ui.KeyEqual:
				a.Zoom = clampZoom(a.Zoom * 1.25)
				return true
			case ui.KeyMinus:
				a.Zoom = clampZoom(a.Zoom / 1.25)
				return true
			}
		}
		return false
	})
	box.Children(func() {
		if item.IsVideo {
			a.ensureVideo(item.Path)
			if a.Video.err != nil {
				ui.Textf(c, "视频解码失败: %v", a.Video.err).TextColor(t.Danger)
			} else if a.Video.player != nil {
				ui.Video(c, a.Video.player).Fit(ui.Contain).Grow(1).Transform(transformFor(a.Zoom, a.Rotation, a.Mirror))
			} else {
				ui.Spinner(c)
			}
		} else {
			if item.Bitmap == nil {
				ui.Spinner(c)
			} else {
				ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1).Transform(transformFor(a.Zoom, a.Rotation, a.Mirror))
			}
		}
		// Overlay controls like FlowVision's zoom and rotate buttons.
		controls := ui.Row(c).Attach(ui.AnchorBottom, ui.AnchorCenter).Gap(8).Padding(8).Background(t.Surface).Radius(8)
		controls.Children(func() {
			if ui.Button(c.Key("zoom-out"), "−").Tooltip("缩小").Clicked() {
				a.Zoom = clampZoom(a.Zoom / 1.25)
			}
			if ui.Button(c.Key("fit"), "1:1").Tooltip("100%").Clicked() {
				a.Zoom = 1
			}
			if ui.Button(c.Key("zoom-in"), "+").Tooltip("放大").Clicked() {
				a.Zoom = clampZoom(a.Zoom * 1.25)
			}
			if ui.Button(c.Key("rotate"), "⟳").Tooltip("旋转").Clicked() {
				a.Rotation += 90
			}
			if ui.Button(c.Key("mirror"), "⇔").Tooltip("镜像").Clicked() {
				a.Mirror = !a.Mirror
			}
		})
	})
}

func clampZoom(z float32) float32 {
	if z < 0.1 {
		return 0.1
	}
	if z > 8 {
		return 8
	}
	return z
}

func transformFor(zoom float32, rotation float32, mirror bool) ui.Transform {
	t := ui.Scaling(zoom, zoom).Then(ui.Rotation(rotation))
	if mirror {
		t = t.Then(ui.Scaling(-1, 1))
	}
	return t
}

func (a *appState) ensureVideo(path string) {
	if a.Video != nil && a.Video.path == path && a.Video.err == nil {
		return
	}
	if a.Video != nil && a.Video.player != nil {
		a.Video.player.Close()
	}
	dec := newFFmpegDecoder(path)
	player, err := ui.NewVideoPlayer(dec)
	a.Video = &videoView{player: player, path: path, err: err}
}

func (a *appState) openLargeImage(index int) {
	if index < 0 || index >= len(a.Visible) {
		return
	}
	a.InLargeView = true
	a.LargeIndex = index
	a.Zoom, a.Rotation, a.Mirror = 1, 0, false
	if a.Video != nil && a.Video.player != nil {
		a.Video.player.Close()
	}
	a.Video = nil
	item := &a.Items[a.Visible[index]]
	a.Window.SetTitle(fmt.Sprintf("%s (%d/%d)", item.Name, index+1, len(a.Visible)))
	if !item.Decoded && !item.Failed && !item.Loading {
		a.loadThumb(a.Root, a.Visible[index])
	}
}

func (a *appState) closeLargeImage() {
	a.InLargeView = false
	a.LargeIndex = -1
	if a.Video != nil && a.Video.player != nil {
		a.Video.player.Close()
	}
	a.Video = nil
	if a.Root != "" {
		a.Window.SetTitle(fmt.Sprintf("%s — %s", appName, pathName(a.Root)))
	}
}

func (a *appState) nextImage() {
	if a.LargeIndex+1 < len(a.Visible) {
		a.openLargeImage(a.LargeIndex + 1)
	}
}

func (a *appState) prevImage() {
	if a.LargeIndex > 0 {
		a.openLargeImage(a.LargeIndex - 1)
	}
}

func (a *appState) applyFilter() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyFilterLocked()
}

func (a *appState) detailView(c *ui.Context) {
	t := c.Theme()
	a.Grid.Key = func(i int) any { return a.Items[a.Visible[i]].Path }
	a.Grid.Selected = &a.Chosen
	a.Grid.Label = func(i int) string { return a.Items[a.Visible[i]].Name }
	list := ui.List(c, &a.ListState, len(a.Visible), func(i int) {
		item := &a.Items[a.Visible[i]]
		row := ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center)
		if a.Chosen == i {
			row.Background(t.Accent).TextColor(t.AccentText)
		}
		row.Children(func() {
			icon := imageIcon
			if isVideoFile(item.Ext) {
				icon = videoIcon
			}
			ui.Icon(c, icon).FontSize(16)
			ui.Text(c, item.Name).Grow(1).FontSize(12).SingleLine()
			ui.Textf(c, "%.1f MB", float64(item.Size)/(1024*1024)).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, time.Unix(item.ModTime, 0).Format("2006-01-02 15:04:05")).FontSize(11).TextColor(t.TextMuted)
		})
		row.OnClick(func() { a.Chosen = i })
		if row.DoubleClicked() {
			a.openLargeImage(i)
		}
	})
	list.Grow(1)
}
