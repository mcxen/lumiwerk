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
	thumbLRU  = newLRUCache(512)
	thumbGate = make(chan struct{}, 8)

	dirIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`))
	imageIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m21 16-5-5-7 7"/></svg>`))
	videoIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="5" width="12" height="14" rx="2"/><path d="m21 8-6 4 6 4z"/></svg>`))
	starIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3l2.9 5.9 6.1.9-4.5 4.4 1.1 6.3-5.6-3-5.6 3 1.1-6.3L3 9.8l6.1-.9z"/></svg>`))
	clockIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/></svg>`))
	arrowLeftIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 5l-7 7 7 7"/></svg>`))
	arrowRightIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 5l7 7-7 7"/></svg>`))
	sidebarIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg>`))
)

type mediaItem struct {
	Path       string
	Name       string
	Ext        string
	Size       int64
	ModTime    int64
	Bitmap     *ui.Bitmap
	Decoded    bool
	Failed     bool
	Loading    bool
	IsVideo    bool
	ThumbDone  bool
	Info       mediaInfo
	InfoLoaded bool
	Tags       []string
}

// videoView owns a real MyGo player for the video currently in large view.
type videoView struct {
	player     *ui.VideoPlayer
	dec        *ffmpegDecoder
	path       string
	err        error
	Position   float64
	StartedAt  time.Time
	Still      *ui.Bitmap
	stillToken int
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

	Grid       ui.GridState
	ListState  ui.ListState
	Chosen     int
	Selected   ui.Selection[string]
	FilterMode int

	Outline      ui.OutlineState[string]
	TreeChosen   int
	SidebarWidth float32
	SidebarShown bool
	Breadcrumb   int

	Query      string
	Sort       string
	LayoutMode int
	Thumb      float64

	InLargeView   bool
	LargeIndex    int
	Zoom          float32
	Rotation      float32
	Mirror        bool
	Video         *videoView
	Info          infoState
	Gesture       gestureState
	Undo          undoStack
	TagFilter     string
	Pinned        bool
	PanX, PanY    float32
	RightDown     bool
	LargeDrag     dragState
	HUD           string
	HUDUntil      time.Time
	Settings      settingsState
	Tabs          tabState
	Profiles      prefsFile
	Batch         batchState
	Renaming      bool
	FocusSearch   bool
	LastQuery     string
	RenamePath    string
	RenameVal     string
	Clip          []string
	Cut           map[string]bool
	PendingSelect []string
	PendingLarge  string
	KeepLarge     bool
	NewFolder     bool
	FolderName    string
	FolderTake    []string
	SeekFrom      float64
	ResumePlay    bool
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
			{Label: "设置复制目录…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { go a.setCopyDestination() }},
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.Window.Update(func() { a.openSettings(); a.settingsWindow() })
			}},
			{Label: "复制到配置目录", Accelerator: "CmdOrCtrl+Shift+C", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { go a.copyToConfigured() }},
			{Label: "快速重命名当前目录", Accelerator: "CmdOrCtrl+Shift+R", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.Window.Update(func() { a.quickRenameFolder() })
			}},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Label: "复制", Accelerator: "CmdOrCtrl+C", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.doCopy() }) }},
			{Label: "剪切", Accelerator: "CmdOrCtrl+X", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.doCut() }) }},
			{Label: "粘贴", Accelerator: "CmdOrCtrl+V", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.doPaste(false) }) }},
			{Label: "移动到此", Accelerator: "CmdOrCtrl+Alt+V", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.doPaste(true) }) }},
			mygo.Separator(),
			{Label: "重命名…", Accelerator: "F2", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.startRename() }) }},
			{Label: "移到废纸篓", Accelerator: "Delete", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.doTrash() }) }},
			{Label: "在 Finder 中显示", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Update(func() { a.revealInFinder() }) }},
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
		a.restorePendingSelection()
		go a.loadTags()
	})
}

func (a *appState) restorePendingSelection() {
	if len(a.PendingSelect) > 0 {
		want := map[string]struct{}{}
		for _, path := range a.PendingSelect {
			want[path] = struct{}{}
		}
		a.Selected.Clear()
		first := -1
		for visibleIndex, itemIndex := range a.Visible {
			path := a.Items[itemIndex].Path
			if _, ok := want[path]; !ok {
				continue
			}
			a.Selected.Add(path)
			if first < 0 {
				first = visibleIndex
			}
		}
		if first >= 0 {
			a.Chosen = first
		}
		a.PendingSelect = nil
	}
	if !a.KeepLarge {
		if a.InLargeView && a.LargeIndex >= 0 && a.LargeIndex < len(a.Visible) {
			item := a.Items[a.Visible[a.LargeIndex]]
			if a.Window != nil {
				a.Window.SetTitle(fmt.Sprintf("%s (%d/%d)", item.Name, a.LargeIndex+1, len(a.Visible)))
			}
		}
		return
	}
	found := -1
	if a.PendingLarge != "" {
		for visibleIndex, itemIndex := range a.Visible {
			if a.Items[itemIndex].Path == a.PendingLarge {
				found = visibleIndex
				break
			}
		}
	}
	a.KeepLarge = false
	a.PendingLarge = ""
	if found < 0 {
		a.closeLargeImage()
		return
	}
	a.InLargeView = true
	a.LargeIndex = found
	item := &a.Items[a.Visible[found]]
	if a.Video != nil && item.IsVideo {
		a.Video.path = item.Path
		if a.Video.dec != nil {
			a.Video.dec.path = item.Path
		}
	}
	if a.Window != nil {
		a.Window.SetTitle(fmt.Sprintf("%s (%d/%d)", item.Name, found+1, len(a.Visible)))
	}
}

func (a *appState) applyFilterLocked() {
	a.Visible = a.Visible[:0]
	q := strings.ToLower(strings.TrimSpace(a.Query))
	for i := range a.Items {
		if q != "" && !pinyinContains(a.Items[i].Name, q) {
			continue
		}
		if (a.FilterMode == 1 && a.Items[i].IsVideo) || (a.FilterMode == 2 && !a.Items[i].IsVideo) {
			continue
		}
		if a.TagFilter != "" && !containsString(a.Items[i].Tags, a.TagFilter) {
			continue
		}
		a.Visible = append(a.Visible, i)
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
		if bitmap := thumbLRU.Get(path); bitmap != nil {
			a.Window.Update(func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if a.Root != root || index >= len(a.Items) || a.Items[index].Path != path {
					return
				}
				current := &a.Items[index]
				current.Bitmap = bitmap
				current.Decoded = true
				current.Loading = false
			})
			return
		}
		thumbGate <- struct{}{}
		defer func() { <-thumbGate }()
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
				thumbLRU.Put(path, bitmap)
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
	if c.Shortcut(0, ui.Key1) && !a.InLargeView {
		a.Window.ToggleMaximize()
	}
	if c.Shortcut(0, ui.Key2) && !a.InLargeView {
		a.Window.SetSize(1280, 820)
		a.Window.Center()
	}
	if c.Shortcut(0, ui.Key5) && !a.InLargeView {
		a.Window.Center()
	}
	if c.Shortcut(0, ui.KeyT) && !a.InLargeView {
		a.Pinned = !a.Pinned
		a.Window.SetAlwaysOnTop(a.Pinned)
	}
	if !a.InLargeView {
		switch {
		case c.Shortcut(0, ui.KeyW):
			a.goUp()
		case c.Shortcut(0, ui.KeyA):
			a.nextMediaFolder(-1)
		case c.Shortcut(0, ui.KeyS):
			a.goBack()
		case c.Shortcut(0, ui.KeyD):
			a.nextMediaFolder(1)
		}
	}
	if c.Shortcut(0, ui.KeyEscape) && a.InLargeView {
		a.closeLargeImage()
	}
	if c.Shortcut(0, ui.KeySpace) && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		if a.InLargeView {
			if item := a.currentItem(); item != nil && item.IsVideo {
				a.toggleVideoPlayback()
			}
		} else if a.Chosen >= 0 {
			a.openLargeImage(a.Chosen)
		}
	}
	if c.Shortcut(0, ui.KeyI) && a.InLargeView {
		a.Info.EXIF = !a.Info.EXIF
	}
	if c.Shortcut(0, ui.KeyU) && a.InLargeView {
		a.Info.File = !a.Info.File
	}
	if c.Shortcut(0, ui.KeyDelete) || c.Shortcut(0, ui.KeyBackspace) {
		if !a.Renaming && !a.NewFolder && !a.Batch.Open {
			a.doTrash()
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) && !a.InLargeView && !a.Renaming && !a.NewFolder {
		a.newFolder()
	}
	if c.Shortcut(0, ui.KeyF2) && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		a.startRename()
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) && !a.InLargeView {
		a.FocusSearch = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyZ) && !a.InLargeView {
		a.undo()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyZ) && !a.InLargeView {
		a.redo()
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		a.doCopy()
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) && !a.InLargeView && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		a.doPaste(false)
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyV) && !a.InLargeView && !a.Renaming && !a.NewFolder {
		a.doPaste(true)
	}
	if c.Shortcut(ui.Cmd, ui.KeyA) && !a.InLargeView && !a.Renaming && !a.NewFolder {
		a.Selected.Clear()
		for _, index := range a.Visible {
			a.Selected.Add(a.Items[index].Path)
		}
		if a.Chosen < 0 && len(a.Visible) > 0 {
			a.Chosen = 0
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		a.refresh()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyR) && !a.InLargeView && !a.Renaming && !a.NewFolder {
		a.quickRenameFolder()
	}
	for i := 0; i < 9; i++ {
		key := ui.Key1 + ui.Key(i)
		if c.Shortcut(ui.Alt, key) {
			a.applyProfile(i)
		}
		if c.Shortcut(ui.Cmd|ui.Alt, key) {
			a.saveProfile(i)
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyX) && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		a.doCut()
	}

	if a.SidebarWidth == 0 {
		a.SidebarWidth = sidebarDefaultWidth
	}
	if !a.SidebarShown && !a.InLargeView {
		a.SidebarShown = true
	}

	if a.SidebarShown && !a.InLargeView {
		ui.Split(c, &a.SidebarWidth, func() { a.sidebarView(c) }, func() { a.mainView(c) }).Fill()
	} else {
		a.mainView(c)
	}
	a.renameDialog(c)
	a.folderDialog(c)
	a.batchDialog(c)
}

func (a *appState) sidebarView(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Fill().Background(t.Surface).Gap(0)
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
		ui.Divider(c)
		// History section: recent folders visited in this session.
		ui.Row(c).Padding(8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, clockIcon).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "历史记录").FontSize(11).Bold().TextColor(t.TextMuted)
		})
		if len(a.History) == 0 {
			ui.Text(c, "无").FontSize(11).TextColor(t.TextMuted).Padding(4, 12)
		}
		for i, h := range a.History {
			hist := h
			row := ui.Row(c).Padding(4, 12).Gap(6).AlignItems(ui.Center).Radius(4)
			if i == a.Cursor {
				row.Background(t.Accent).TextColor(t.AccentText)
			}
			row.Children(func() {
				ui.Icon(c, dirIcon).FontSize(12)
				ui.Text(c, pathName(hist)).FontSize(11).SingleLine().MaxLines(1)
			})
			row.OnClick(func() { a.openFolder(hist) })
		}
		ui.Divider(c)
		// Finder tags section (placeholder groups matching FlowVision sidebar).
		ui.Row(c).Padding(8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, starIcon).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "Finder标签").FontSize(11).Bold().TextColor(t.TextMuted)
		})
		for _, tag := range []string{"红色", "橙色", "黄色", "绿色", "蓝色", "灰色"} {
			name := tag
			item := ui.Text(c, name).FontSize(11).TextColor(t.TextMuted).Padding(4, 12)
			if a.TagFilter == name {
				item.TextColor(t.Accent).Bold()
			}
			item.OnClick(func() {
				if a.TagFilter == name {
					a.TagFilter = ""
				} else {
					a.TagFilter = name
					go a.loadTags()
				}
				a.applyFilter()
			})
		}
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
	col.HandleInput(a.handleGesture)
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
		} else if a.LayoutMode == 2 {
			a.waterfallView(c)
		} else if a.LayoutMode == 3 {
			a.justifiedView(c)
		} else {
			a.gridView(c)
		}
		a.statusBar(c)
		a.toastOverlay(c)
	})
}

func (a *appState) toolbarView(c *ui.Context) {
	t := c.Theme()
	bar := ui.Toolbar(c, func() {
		if ui.Button(c.Key("sidebar"), "").Children(func() { ui.Icon(c, sidebarIcon).FontSize(14) }).Tooltip("显示/隐藏侧栏").Clicked() {
			a.SidebarShown = !a.SidebarShown
		}
		if ui.Button(c.Key("back"), "").Children(func() { ui.Icon(c, arrowLeftIcon).FontSize(14) }).Tooltip("后退").Clicked() {
			a.goBack()
		}
		if ui.Button(c.Key("forward"), "").Children(func() { ui.Icon(c, arrowRightIcon).FontSize(14) }).Tooltip("前进").Clicked() {
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
			ui.Breadcrumbs(c, items, &chosen).Label("路径").Grow(1).MinWidth(160)
			ui.Textf(c, "(%d 图像)", len(a.Visible)).FontSize(11).TextColor(t.TextMuted)
		}
		if !a.InLargeView {
			prevQuery := a.LastQuery
			search := ui.SearchField(c, &a.Query).Key("query").Width(150)
			if a.FocusSearch {
				search.Focus()
				a.FocusSearch = false
			}
			if a.Query != prevQuery {
				a.LastQuery = a.Query
				a.mu.Lock()
				a.applyFilterLocked()
				a.mu.Unlock()
			}
			ui.MenuButton(c, "类型", func(m *ui.Menu) {
				for i, name := range []string{"全部", "图片", "视频"} {
					idx := i
					if m.Item(name).Radio(a.FilterMode == idx).Chosen() {
						a.FilterMode = idx
						a.applyFilter()
					}
				}
			})
		}
		if !a.InLargeView {
			ui.MenuButton(c, "布局", func(m *ui.Menu) {
				for i, name := range []string{"网格", "详情", "瀑布", "紧凑"} {
					idx := i
					if m.Item(name).Radio(a.LayoutMode == idx).Chosen() {
						a.LayoutMode = idx
					}
				}
			})
			ui.Slider(c, &a.Thumb, 64, 512).Width(80)
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
		}
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
	zone.ContextMenu(func(m *ui.Menu) { a.fileContextMenu(m) })
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
	a.Grid.Selection = &a.Selected
	a.Grid.Label = func(i int) string { return a.Items[a.Visible[i]].Name }
	cell := int(a.Thumb + 64)
	grid := ui.GridView(c, &a.Grid, len(a.Visible), float32(cell), float32(cell+24), func(i int) {
		item := &a.Items[a.Visible[i]]
		card := ui.Column(c).Grow(1).Margin(4).Padding(4).Gap(4).Radius(6).Background(t.Surface)
		if a.isCut(item.Path) {
			card.Opacity(0.4)
		}
		if a.Chosen == i {
			card.Border(2, t.Accent)
		}
		card.Children(func() {
			thumb := ui.Box(c).Grow(1).Radius(4).Background(t.Background).Justify(ui.Center).AlignItems(ui.Center)
			thumb.Children(func() {
				if len(item.Tags) > 0 {
					tags := ui.Row(c).Attach(ui.AnchorTopEnd, ui.AnchorTopEnd).Margin(4, 4).Gap(2)
					tags.Children(func() {
						for range item.Tags {
							ui.Box(c).Width(5).Height(5).Radius(2).Background(t.Accent)
						}
					})
				}
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
		card.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
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
	a.ensureMediaInfo(item)
	box := ui.Box(c).Fill().Background(t.Background)
	a.tickLongPress(c, item)
	box.Children(func() {
		// The gesture surface sits behind the controls so buttons still receive clicks.
		stage := ui.Box(c).Fill().Justify(ui.Center).AlignItems(ui.Center)
		stage.HandleInput(func(ev ui.InputEvent) bool { return a.largeInput(ev, item) })
		stage.ContextMenu(func(m *ui.Menu) { a.largeContextMenu(m, item) })
		stage.Children(func() {
			if item.IsVideo {
				a.ensureVideo(item.Path)
				if a.Video.err != nil {
					ui.Textf(c, "视频解码失败: %v", a.Video.err).TextColor(t.Danger)
				} else if a.Video.Still != nil && a.Video.player != nil && !a.Video.player.Playing() {
					ui.Image(c, a.Video.Still).Fit(ui.Contain).Grow(1).Transform(a.transform())
				} else if a.Video.player != nil {
					ui.Video(c, a.Video.player).Fit(ui.Contain).Grow(1).Transform(a.transform())
				} else {
					ui.Spinner(c)
				}
			} else if item.Bitmap == nil {
				ui.Spinner(c)
			} else {
				ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1).Transform(a.transform())
			}
		})
		a.mediaOverlay(c, item)
		a.zoomHUD(c)
		a.edgeArrows(c)
		a.largeControls(c, item)
	})
}

// largeInput handles zoom, pan, edge clicks and double-click close.
func (a *appState) largeInput(ev ui.InputEvent, item *mediaItem) bool {
	switch ev.Kind {
	case ui.InputZoom:
		a.zoomAt(ev.X, ev.Y, ev.Scale)
		return true
	case ui.InputScroll:
		delta := ev.DY
		if delta == 0 {
			delta = ev.DX
		}
		factor := wheelZoomFactor(delta, ev.Precise, ev.Momentum)
		if factor == 1 {
			return true
		}
		a.zoomAt(ev.X, ev.Y, factor)
		return true
	case ui.InputPointerDown:
		// Images keep right-click for the context menu. Video captures it to seek.
		if ev.Button == 1 && !item.IsVideo {
			return false
		}
		if ev.Button == 1 {
			a.RightDown = true
		}
		a.LargeDrag = dragState{active: true, startX: ev.X, startY: ev.Y, lastX: ev.X, lastY: ev.Y, moved: false, button: ev.Button, time: ev.Time}
		if item.IsVideo && ev.Button == 1 && a.Video != nil {
			a.SeekFrom = a.Video.Position
			a.ResumePlay = a.Video.player != nil && a.Video.player.Playing()
			if a.ResumePlay {
				a.Video.player.Pause()
			}
		}
		if ev.Button == 0 && ev.Clicks >= 2 {
			a.closeLargeImage()
			return true
		}
		return true
	case ui.InputPointerMove:
		if !a.LargeDrag.active {
			return false
		}
		dx, dy := ev.X-a.LargeDrag.lastX, ev.Y-a.LargeDrag.lastY
		a.LargeDrag.lastX, a.LargeDrag.lastY = ev.X, ev.Y
		if abs32(ev.X-a.LargeDrag.startX) > 3 || abs32(ev.Y-a.LargeDrag.startY) > 3 {
			a.LargeDrag.moved = true
		}
		if !a.LargeDrag.moved {
			return true
		}
		if a.LargeDrag.button == 1 && item.IsVideo {
			preview := seekPreview(a.SeekFrom, float64(ev.X-a.LargeDrag.startX), float64(a.largeWidth()), a.videoDuration())
			a.HUD = "跳转到 " + shortTime(preview)
			a.HUDUntil = time.Now().Add(800 * time.Millisecond)
			return true
		}
		if a.LargeDrag.button == 0 {
			a.PanX += dx
			a.PanY += dy
			a.clampPan()
		}
		return true
	case ui.InputPointerUp:
		button := ev.Button
		moved := a.LargeDrag.moved
		fired := a.LargeDrag.fired
		startX := a.LargeDrag.startX
		a.LargeDrag.active = false
		if button == 1 {
			a.RightDown = false
			if item.IsVideo && moved {
				preview := seekPreview(a.SeekFrom, float64(ev.X-startX), float64(a.largeWidth()), a.videoDuration())
				a.seekVideo(preview)
				if a.ResumePlay && a.Video != nil && a.Video.player != nil {
					a.Video.player.Play()
					a.Video.StartedAt = time.Now()
				}
				a.ResumePlay = false
				return true
			}
		}
		if fired || moved {
			return true
		}
		if button != 0 {
			return false
		}
		left, right := edgeZones(a.largeWidth())
		if ev.X <= left {
			a.prevImage()
			return true
		}
		if ev.X >= right {
			a.nextImage()
			return true
		}
		if item.IsVideo {
			a.toggleVideoPlayback()
			return true
		}
		return false
	case ui.InputKeyDown:
		switch ev.Key {
		case ui.KeyEqual, ui.KeyMinus, ui.Key0:
			switch ev.Key {
			case ui.KeyEqual:
				a.zoomStep(1.25)
			case ui.KeyMinus:
				a.zoomStep(1 / 1.25)
			case ui.Key0:
				a.resetZoom()
			}
			return true
		case ui.KeyE:
			a.Rotation += 90
			return true
		case ui.KeyQ:
			a.Rotation -= 90
			return true
		case ui.KeyM, ui.KeyF:
			a.Mirror = !a.Mirror
			return true
		case ui.KeySpace:
			if item.IsVideo {
				a.toggleVideoPlayback()
				return true
			}
		}
	}
	return false
}

// mediaOverlay stacks Finder tag dots, rating stars and the info panel over the media.
func (a *appState) mediaOverlay(c *ui.Context, item *mediaItem) {
	t := c.Theme()
	dots := 0
	for _, tag := range item.Tags {
		if ratingFor(&mediaItem{Tags: []string{tag}}) > 0 {
			continue
		}
		dots++
	}
	if dots > 0 {
		tags := ui.Row(c).Attach(ui.AnchorTopEnd, ui.AnchorTopEnd).Margin(10, 10).Gap(4)
		tags.Children(func() {
			for _, tag := range item.Tags {
				if ratingFor(&mediaItem{Tags: []string{tag}}) > 0 {
					continue
				}
				ui.Box(c).Width(10).Height(10).Radius(5).Background(tagColor(tag))
			}
		})
	}
	if stars := ratingFor(item); stars > 0 {
		panel := ui.Row(c).Attach(ui.AnchorBottomStart, ui.AnchorBottomStart).Margin(14, 14).Padding(4, 8).Radius(6).Background(t.Surface)
		panel.Children(func() {
			for i := 0; i < 5; i++ {
				col := t.TextMuted
				if i < stars {
					col = ui.RGB(255, 199, 0)
				}
				ui.Icon(c, starIcon).FontSize(16).TextColor(col)
			}
		})
	}
	a.infoOverlay(c, item)
}

// zoomHUD shows the transient zoom percentage.
func (a *appState) zoomHUD(c *ui.Context) {
	if a.HUD == "" {
		return
	}
	if time.Now().After(a.HUDUntil) {
		a.HUD = ""
		return
	}
	if d := time.Until(a.HUDUntil); d > 0 {
		c.After(d)
	}
	t := c.Theme()
	hud := ui.Row(c).Attach(ui.AnchorTop, ui.AnchorTop).Margin(24, 0).Padding(6, 14).Radius(6).Background(t.Surface)
	hud.Children(func() {
		ui.Text(c, a.HUD).FontSize(12).Bold()
	})
}

// largeControls is the floating bottom control strip.
func (a *appState) largeControls(c *ui.Context, item *mediaItem) {
	if item.IsVideo {
		a.videoControlsBar(c, item)
		return
	}
	t := c.Theme()
	controls := ui.Row(c).Attach(ui.AnchorBottom, ui.AnchorBottom).Margin(16, 0).Gap(6).Padding(6).Background(t.Surface).Radius(8)
	controls.Children(func() {
		if ui.Button(c.Key("zoom-out"), "−").Tooltip("缩小").Clicked() {
			a.zoomStep(1 / 1.25)
		}
		if ui.Button(c.Key("fit"), "适应").Tooltip("适应窗口").Clicked() {
			a.resetZoom()
		}
		if ui.Button(c.Key("zoom-in"), "+").Tooltip("放大").Clicked() {
			a.zoomStep(1.25)
		}
		if ui.Button(c.Key("zoom-100"), "1:1").Tooltip("实际大小").Clicked() {
			a.zoom100()
		}
		if ui.Button(c.Key("rotate"), "⟳").Tooltip("顺时针旋转").Clicked() {
			a.Rotation += 90
		}
		if ui.Button(c.Key("rotate-ccw"), "⟲").Tooltip("逆时针旋转").Clicked() {
			a.Rotation -= 90
		}
		if ui.Button(c.Key("mirror"), "⇔").Tooltip("镜像").Clicked() {
			a.Mirror = !a.Mirror
		}
	})
}

// videoControlsBar is the floating 520px playback strip above the video.
func (a *appState) videoControlsBar(c *ui.Context, item *mediaItem) {
	if a.Video == nil || a.Video.player == nil {
		return
	}
	t := c.Theme()
	duration := item.Info.DurationSec
	if duration <= 0 {
		duration = 1
	}
	if a.Video.player.Playing() {
		a.Video.Position = min(duration, a.Video.Position+time.Since(a.Video.StartedAt).Seconds())
		a.Video.StartedAt = time.Now()
	}
	bar := ui.Row(c).Attach(ui.AnchorBottom, ui.AnchorBottom).Width(520).Height(67).Margin(14, 0).Padding(8, 12).
		Gap(8).AlignItems(ui.Center).Background(t.Surface).Radius(8).Border(1, t.Border)
	bar.Children(func() {
		label := "播放"
		if a.Video.player.Playing() {
			label = "暂停"
		}
		if ui.Button(c.Key("video-play"), label).Clicked() {
			a.toggleVideoPlayback()
		}
		pos := a.Video.Position
		if ui.Slider(c, &pos, 0, duration).Width(260).Tooltip("进度").Changed() {
			a.seekVideo(pos)
		}
		ui.Textf(c, "%s / %s", shortTime(a.Video.Position), shortTime(duration)).FontSize(11).TextColor(t.TextMuted)
	})
}

// edgeArrows mirrors FlowVision's left/right media switching hints.
func (a *appState) edgeArrows(c *ui.Context) {
	t := c.Theme()
	left := ui.Row(c).Attach(ui.AnchorLeft, ui.AnchorLeft).Margin(12, 0).Padding(8, 10).Radius(20).Background(t.Surface)
	left.Children(func() {
		ui.Icon(c, arrowLeftIcon).FontSize(22).TextColor(t.TextMuted)
	})
	if left.Clicked() {
		a.prevImage()
	}
	right := ui.Row(c).Attach(ui.AnchorRight, ui.AnchorRight).Margin(12, 0).Padding(8, 10).Radius(20).Background(t.Surface)
	right.Children(func() {
		ui.Icon(c, arrowRightIcon).FontSize(22).TextColor(t.TextMuted)
	})
	if right.Clicked() {
		a.nextImage()
	}
}

func shortTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int(seconds)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// ensureMediaInfo loads EXIF dimensions for ratio math.
func (a *appState) ensureMediaInfo(item *mediaItem) {
	if item.InfoLoaded && item.Info.Width > 0 {
		return
	}
	go a.loadInfo(a.Visible[a.LargeIndex])
}

// largeWidth is the content area width, used for edge-click zones.
func (a *appState) largeWidth() float32 {
	if a.Window == nil {
		return 1280
	}
	w, _ := a.Window.ContentSize()
	if w <= 0 {
		return 1280
	}
	return float32(w)
}

func clampFloat32(value, min, max float32) float32 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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
	a.Video = &videoView{player: player, dec: dec, path: path, err: err, StartedAt: time.Now()}
}

// seekVideo restarts the real ffmpeg stream at seconds so the control bar
// moves through the source media instead of a synthetic timeline.
func (a *appState) seekVideo(seconds float64) {
	if a.Video == nil || a.Video.path == "" {
		return
	}
	if seconds < 0 {
		seconds = 0
	}
	playing := a.Video.player != nil && a.Video.player.Playing()
	if a.Video.player != nil {
		a.Video.player.Close()
	}
	dec := newFFmpegDecoderAt(a.Video.path, seconds)
	player, err := ui.NewVideoPlayer(dec)
	a.Video.player = player
	a.Video.dec = dec
	a.Video.err = err
	a.Video.Still = nil
	a.Video.Position = seconds
	a.Video.StartedAt = time.Now()
	if player == nil {
		return
	}
	if playing {
		player.Play()
		return
	}
	player.Pause()
	a.requestSharpFrame()
}

func (a *appState) openLargeImage(index int) {
	if index < 0 || index >= len(a.Visible) {
		return
	}
	a.InLargeView = true
	a.LargeIndex = index
	a.Zoom, a.Rotation, a.Mirror = 1, 0, false
	a.PanX, a.PanY = 0, 0
	a.HUD, a.HUDUntil = "", time.Time{}
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
	a.Grid.Selection = &a.Selected
	a.Grid.Label = func(i int) string { return a.Items[a.Visible[i]].Name }
	a.ListState.Selection = &a.Selected
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
		row.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
		if a.isCut(item.Path) {
			row.Opacity(0.4)
		}
		if row.DoubleClicked() {
			a.openLargeImage(i)
		}
	})
	list.Grow(1)
}

func (a *appState) currentProfile() profile {
	return profile{
		Root:        a.Root,
		Sidebar:     a.SidebarShown,
		SidebarW:    a.SidebarWidth,
		LayoutMode:  a.LayoutMode,
		Thumb:       a.Thumb,
		Sort:        a.Sort,
		Recursive:   a.Recursive,
		Destination: a.currentDestination(),
	}
}

func (a *appState) applyProfile(i int) {
	if i < 0 || i >= len(a.Profiles.Profiles) {
		return
	}
	p := a.Profiles.Profiles[i]
	a.Profiles.Active = i
	a.SidebarShown = p.Sidebar
	if p.SidebarW > 0 {
		a.SidebarWidth = p.SidebarW
	}
	a.LayoutMode = p.LayoutMode
	a.Thumb = p.Thumb
	a.Sort = p.Sort
	a.Recursive = p.Recursive
	savePrefs(a.Profiles)
	if p.Root != "" && isDir(p.Root) {
		a.openFolder(p.Root)
	}
	a.refresh()
}

func (a *appState) saveProfile(i int) {
	if i < 0 || i >= len(a.Profiles.Profiles) {
		return
	}
	a.Profiles.Profiles[i] = a.currentProfile()
	a.Profiles.Active = i
	savePrefs(a.Profiles)
}
