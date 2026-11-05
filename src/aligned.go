// Lumiwerk — FlowVision-aligned state, views, and file/media engine.
package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

const (
	appName                     = "Lumiwerk"
	appVersion                  = "0.3.11"
	sidebarDefaultWidth float32 = 279
)

var (
	thumbLRU  = newThumbCache(thumbBudget)
	thumbGate chan struct{}

	dirIcon        = lucideFolder
	imageIcon      = lucideImage
	videoIcon      = lucideVideo
	starIcon       = lucideStar
	clockIcon      = lucideClock
	arrowLeftIcon  = lucideChevronLeft
	arrowRightIcon = lucideChevronRight
	sidebarIcon    = lucideSidebar

	// Layout switcher glyphs mirror Finder's view control: icon, list,
	// gallery-style columns and a mosaic.
	gridIcon      = lucideGrid
	detailIcon    = lucideList
	waterfallIcon = lucideColumns
	justifiedIcon = lucideGallery
)

func init() {
	// thumbGate 并发数根据 CPU 核心数动态设置，最多 16
	concurrency := runtime.NumCPU()
	if concurrency > 16 {
		concurrency = 16
	}
	if concurrency < 1 {
		concurrency = 1
	}
	thumbGate = make(chan struct{}, concurrency)
}

type mediaItem struct {
	Path         string
	Name         string
	Ext          string
	Size         int64
	ModTime      int64
	Bitmap       *ui.Bitmap
	Decoded      bool
	Failed       bool
	Loading      bool
	Large        *ui.Bitmap
	LargeLoading bool
	IsVideo      bool
	ThumbDone    bool
	Info         mediaInfo
	InfoLoaded   bool
	Tags         []string
	IsDir        bool
	ThumbEdge    int
	LoadGen      int
	// CoverKey is the thumbCache key of a folder stack, path plus content
	// digest. Empty for files and for folders that still show the plain icon.
	CoverKey string
	// InfoLoading is set while a metadata probe for the item runs, so views
	// drawn every frame do not start one probe per frame.
	InfoLoading bool
	// Seen is when a view last built the item's thumbnail (UnixNano,
	// atomic): queued decodes for items scrolled away are skipped and
	// thumbnails long off screen give their bitmap back to the cache.
	Seen int64
}

// videoView binds the native bitmap cache to one Go playback kernel.
type videoView struct {
	kernel     *playbackSession
	path       string
	frame      *ui.Bitmap
	Still      *ui.Bitmap
	stillToken int
	err        error
}

type appState struct {
	Window *mygo.Window
	// quitting is set on the main thread from OnBeforeQuit so the main
	// window can tell a real quit from the close button.
	quitting bool
	// shelved is true while the main window is hidden and background work
	// is paused. Thumbnail goroutines read it under mu.
	shelved bool
	// resumeVideo, shelvedScroll and shelvedPlay remember what suspend
	// paused, so showing the window continues them. Main thread only.
	resumeVideo   bool
	shelvedScroll bool
	shelvedPlay   bool
	// Tray is the menu-bar icon, created once the app is ready and removed
	// on quit. flyout is its status panel; nil when it has not been opened
	// or AppKit has dismissed it.
	Tray   *mygo.Tray
	flyout *mygo.Window
	mu     sync.Mutex

	Root          string
	Items         []mediaItem
	Visible       []int
	Recursive     bool
	MediaUnitRoot string
	scanSeq       int
	watchCtx      context.Context
	watchStop     context.CancelFunc
	refreshing    bool
	decodeCtx     context.Context
	decodeCancel  context.CancelFunc

	History []string
	Cursor  int

	Grid       ui.GridState
	ListState  ui.ListState
	Chosen     int
	Selected   ui.Selection[string]
	FilterMode int

	Outline      ui.OutlineState[string]
	treeOpenRoot string
	SidebarWidth float32
	SidebarShown bool
	Breadcrumb   int

	Query        string
	Sort         string
	FolderFirst  bool
	FullPathSort bool
	sortSalt     int64
	LayoutMode   int
	Thumb        float64
	thumbDue     time.Time
	browseScroll ui.ScrollState
	browseBox    browseBox
	browseAnchor browseAnchor
	columnX      float32
	columnY      float32

	InLargeView    bool
	Visited        map[string]int64
	Recent         []string
	dwellSeq       int
	historyCleared bool
	scrollCarry    float32
	Gallery        bool
	galleryPeek    bool
	Hero           heroState
	LargeIndex     int
	Zoom           float32
	Rotation       float32
	Mirror         bool
	Video          *videoView
	videoEdgeDue   time.Time
	Info           infoState
	Gesture        gestureState
	Undo           undoStack
	Pinned         bool
	AutoScroll     bool
	AutoPlay       bool
	autoTick       time.Time
	AutoConfig     string
	AutoValue      string
	PanX, PanY     float32
	RightDown      bool
	LargeDrag      dragState
	HUD            string
	HUDUntil       time.Time
	Settings       settingsState
	Diagnostics    diagnosticsState
	Tabs           tabState
	Profiles       prefsFile
	Batch          batchState
	Renaming       bool
	FocusSearch    bool
	LastQuery      string
	RenamePath     string
	RenameVal      string
	Clip           []string
	Cut            map[string]bool
	PendingSelect  []string
	layoutGen      uint64
	flowCache      flowLayoutCache
	opBusy         bool
	treeCache      map[string]treeEntry
	selAnchor      selectionAnchor
	keyPivot       int
	revealFrames   int
	lastUp         time.Time
	// frameStamp is the current build's time. frameNow publishes it only
	// after the build finishes, so a decode running at the start of the
	// next frame still sees the cards the previous frame showed.
	frameStamp      int64
	frameNow        int64
	nextSweep       time.Time
	PendingLarge    string
	KeepLarge       bool
	NewFolder       bool
	FolderName      string
	FolderTake      []string
	ZipAsk          bool
	ZipPass         string
	ZipFor          string
	ZipPaths        []string
	ZipDrop         bool
	CropAsk         bool
	CropW           string
	CropH           string
	CropX           string
	CropY           string
	CropPaths       []string
	TrimAsk         bool
	TrimStart       string
	TrimEnd         string
	Editor          editorState
	editorSeq       int
	InfoAsk         bool
	InfoText        string
	ShareHold       []shareService
	SeekFrom        float64
	ResumePlay      bool
	VideoVolume     float64
	PrevVolume      float64
	VideoSave       saveVideoState
	playerMoved     time.Time
	playerPtrX      float32
	playerPtrY      float32
	playerPtrSeen   bool
	playerTimelineW float32
	playerMenu      bool
	playerEndSeen   bool
	playerSkipClick bool
	StageW          float32
	StageH          float32
	Sharp           *ui.Bitmap
	SharpKey        string
	SharpPlace      sharpPlace
	sharpToken      int
	sharpDue        time.Time
	sharpDueKey     string
	sharpBusy       bool
	sharpBusyKey    string
	sharpFailKey    string
}

func newAppMenu(a *appState) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Lumiwerk", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			updater.MenuItem(),
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleQuit},
		}},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开目录…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { safeGo("dialog.pickFolder", a.pickFolder) }},
			// RoleOpenRecent fills from mygo's persisted recent documents,
			// which openFolder records; entries open through OnOpenFile.
			{Role: mygo.RoleOpenRecent},
			{Label: "上级目录", Accelerator: "CmdOrCtrl+Up", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.goUp() }) }},
			mygo.Separator(),
			{Label: "递归浏览", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.setRecursive(!a.Recursive) }) }},
			{Label: "设置复制目录…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { safeGo("dialog.setCopyDestination", a.setCopyDestination) }},
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.update(func() { a.openSettings(); a.settingsWindow() })
			}},
			{Label: "复制到配置目录", Accelerator: "CmdOrCtrl+Shift+C", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.copyToConfigured() }) }},
			{Label: "复制到下载", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.transferToDownloads(opCopy) }) }},
			{Label: "移动到下载", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.transferToDownloads(opMove) }) }},
			{Label: "新建文本文件", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.newTextFile() }) }},
			{Label: "快速重命名当前目录", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.update(func() { a.quickRenameFolder() })
			}},
			mygo.Separator(),
			{Label: "快速压缩", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.quickCompress() }) }},
			{Label: "压缩为 ZIP", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.compressPlain(false) }) }},
			{Label: "压缩并删除源文件", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.compressPlain(true) }) }},
			{Label: "加密压缩…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.encryptCompress(false) }) }},
			{Label: "解压到当前目录", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.extractSelection(false) }) }},
			{Label: "解压并删除压缩包", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.extractSelection(true) }) }},
			mygo.Separator(),
			{Label: "截存当前画面", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.captureCurrentFrame() }) }},
			{Label: "旋转文件 90°", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.rotateSelection(90) }) }},
			{Label: "旋转文件 270°", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.rotateSelection(270) }) }},
			{Label: "收藏当前目录", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleFavorite(a.Root) }) }},
			{Label: "提取子文件夹文件并归集", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.collectSelection() }) }},
			{Label: "裁剪视频尺寸…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.promptCrop(a.operationPaths()) }) }},
			{Label: "无损裁剪片段…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.promptTrim() }) }},
			mygo.Separator(),
			// RoleClose sends Cmd+W to the key window. The main window's
			// OnClose decides whether that hides or quits; a settings or
			// diagnostics window in front closes on its own.
			{Label: "关闭窗口", Accelerator: "CmdOrCtrl+W", Role: mygo.RoleClose},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Label: "复制", Accelerator: "CmdOrCtrl+C", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.doCopy() }) }},
			{Label: "剪切", Accelerator: "CmdOrCtrl+X", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.doCut() }) }},
			{Label: "粘贴", Accelerator: "CmdOrCtrl+V", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.doPaste(false) }) }},
			{Label: "移动到此", Accelerator: "CmdOrCtrl+Alt+V", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.doPaste(true) }) }},
			mygo.Separator(),
			{Label: "重命名…", Accelerator: "F2", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.startRename() }) }},
			{Label: "移到废纸篓", Accelerator: "Delete", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.doTrash() }) }},
			{Label: revealLabel, Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.revealInFinder() }) }},
			{Label: "显示简介", Accelerator: "CmdOrCtrl+I", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.showInfo() }) }},
			{Label: "制作副本", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.duplicateSelection() }) }},
			{Label: openParentLabel, Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.showOriginalFolder() }) }},
		}},
		{Label: "显示", Submenu: []*mygo.MenuItem{
			{Label: "运行诊断…", Accelerator: "CmdOrCtrl+Shift+D", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.update(a.showDiagnostics)
			}},
			{Label: "置顶", Accelerator: "T", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.togglePinned() }) }},
			{Label: "自定义布局样式…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.update(func() { a.openSettings(); a.Settings.Pane = 0; a.settingsWindow() })
			}},
			{Label: "显示隐藏文件", Accelerator: "CmdOrCtrl+Shift+.", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("hidden") }) }},
			{Label: "显示所有类型文件", Accelerator: "CmdOrCtrl+Shift+,", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("all") }) }},
			{Label: "显示图像文件", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("images") }) }},
			{Label: "显示相机 RAW 文件", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("raw") }) }},
			{Label: "显示视频文件", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("videos") }) }},
			{Label: "显示压缩文件", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("archives") }) }},
			{Label: "返回上一级目录", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.goUp() }) }},
			{Label: "递归浏览模式", Accelerator: "CmdOrCtrl+Shift+R", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.setRecursive(!a.Recursive) }) }},
			{Label: "包含文件夹", Accelerator: "CmdOrCtrl+Shift+F", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.toggleViewOption("folders") }) }},
			{Label: "媒体单元模式", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				a.update(func() { a.setMediaUnit(!a.Profiles.View.MediaUnit) })
			}},
			{Label: "启用自动滚动", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.askAutoConfig("scroll") }) }},
			{Label: "启用自动播放", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.askAutoConfig("play") }) }},
			{Label: "最大化窗口", Accelerator: "1", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.ToggleMaximize() }},
			{Label: "合适窗口大小", Accelerator: "2", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.SetSize(1280, 820); a.Window.Center() }},
			{Label: "将窗口居中", Accelerator: "5", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.Window.Center() }},
			{Label: "跟随系统主题", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.setTheme("system") }) }},
			{Label: "浅色模式", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.setTheme("light") }) }},
			{Label: "黑暗模式", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.setTheme("dark") }) }},
			{Label: "隐藏/显示侧栏", Accelerator: "CmdOrCtrl+B", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.update(func() { a.SidebarShown = !a.SidebarShown }) }},
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

func isRawFile(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "arw", "cr2", "cr3", "crw", "dng", "erf", "kdc", "mrw", "nef", "nrw", "orf", "pef", "raf", "raw", "rw2", "sr2", "srf", "x3f", "heic", "heif", "avif":
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

func isMediaFile(ext string) bool { return isImageFile(ext) || isRawFile(ext) || isVideoFile(ext) }

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

// lastLargePath is the path of the image the large view last showed, for
// the session record; "" when none was opened.
func (a *appState) lastLargePath() string {
	if !a.InLargeView || a.LargeIndex < 0 || a.LargeIndex >= len(a.Visible) {
		return ""
	}
	item := a.visibleItem(a.LargeIndex)
	if item == nil || item.IsDir {
		return ""
	}
	return item.Path
}

// noteRecentFolder records a browsed folder with mygo's recent documents
// and with the system (the Dock menu on macOS). Best-effort: a failure
// only means the folder is missing from the system list.
func (a *appState) noteRecentFolder(path string) {
	if a.Window == nil || path == "" {
		return
	}
	if err := mygo.App.AddRecentDocument(path); err != nil {
		logDebugf("recent document %s: %v", path, err)
	}
}

func (a *appState) openFolder(path string) {
	path = filepath.Clean(path)
	if path == "" || !isDir(path) || path == a.Root {
		return
	}

	// 取消旧目录的解码任务
	if a.decodeCancel != nil {
		a.decodeCancel()
	}
	a.decodeCtx, a.decodeCancel = context.WithCancel(context.Background())

	a.stopWatcher()
	a.stopAutomation()
	a.mu.Lock()
	a.Root = path
	if a.MediaUnitRoot != "" && path != a.MediaUnitRoot && filepath.Dir(path) != a.MediaUnitRoot {
		a.MediaUnitRoot = ""
	}
	a.Query = ""
	a.Items = nil
	a.Visible = nil
	a.Chosen = -1
	a.InLargeView = false
	a.LargeIndex = -1
	a.scanSeq++
	a.dwellSeq++
	a.noteDwell(path, a.dwellSeq)
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
	a.noteTabRoot(path)
	a.noteRecentFolder(path)
	a.setTitle(fmt.Sprintf("%s — %s", appName, pathName(path)))
	a.syncTrayTip()
	a.refreshView()
}

func (a *appState) refresh() {
	a.mu.Lock()
	root := a.Root
	// Each scan supersedes the ones before it: a slower, older listing that
	// lands later must not overwrite a newer one (and its pending selection).
	a.scanSeq++
	seq := a.scanSeq
	a.mu.Unlock()
	if root == "" {
		return
	}
	safeGo("scanFolder", func() { a.scanFolder(root, seq) })
}

func (a *appState) scanFolder(root string, seq int) {
	a.mu.Lock()
	view := a.Profiles.View
	recursive := a.Recursive
	mediaUnitRoot := a.MediaUnitRoot
	a.mu.Unlock()
	var files []mediaItem
	flat := recursive || mediaUnitRoot == root
	includeFile := func(path, ext string) bool {
		if !view.Hidden && isHiddenPath(root, path) {
			return false
		}
		return view.AllTypes || isEnabledFile(path, ext, view)
	}
	if flat {
		files = scanTreeParallel(root, func(entry scanEntry) scanEntryResult {
			path := entry.Path
			if entry.IsDir {
				if path != root && !view.Hidden && entry.Hidden {
					return scanEntryResult{skipDir: true}
				}
				if recursive && view.RecursiveFolders && path != root && (!view.Hidden || !entry.Hidden) {
					return scanEntryResult{keep: true, item: mediaItem{Path: path, Name: entry.Name, ModTime: entry.ModTime.Unix(), IsDir: true, InfoLoaded: true}}
				}
				return scanEntryResult{}
			}
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			if !includeFile(path, ext) {
				return scanEntryResult{}
			}
			if entry.Info == nil {
				return scanEntryResult{}
			}
			return scanEntryResult{keep: true, item: mediaItem{Path: path, Name: entry.Name, Ext: strings.ToLower(ext), Size: entry.Size, ModTime: entry.ModTime.Unix(), IsVideo: isVideoFile(ext)}}
		})
	} else {
		entries, err := scanDir(root)
		if err != nil {
			logErrorf("could not read folder %s: %v", root, err)
			return
		}
		for _, e := range entries {
			if !view.Hidden && e.Hidden {
				continue
			}
			if e.IsDir {
				files = append(files, mediaItem{Path: e.Path, Name: e.Name, ModTime: e.ModTime.Unix(), IsDir: true, InfoLoaded: true})
				continue
			}
			ext := strings.TrimPrefix(filepath.Ext(e.Name), ".")
			if !includeFile(e.Path, ext) {
				continue
			}
			if e.Info == nil {
				continue
			}
			files = append(files, mediaItem{Path: e.Path, Name: e.Name, Ext: strings.ToLower(ext), Size: e.Size, ModTime: e.ModTime.Unix(), IsVideo: isVideoFile(ext)})
		}
	}
	slices.SortStableFunc(files, func(x, y mediaItem) int { return a.compareMedia(x, y) })
	a.update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.Root != root || a.scanSeq != seq {
			return
		}
		a.noteSelectionAnchorLocked()
		carryItemState(a.Items, files)
		a.Items = files
		a.applyFilterLocked()
		a.restorePendingSelection()
		if a.InLargeView && (a.LargeIndex < 0 || a.LargeIndex >= len(a.Visible)) {
			a.closeLargeImage()
		}
		safeGo("loadTags", a.loadTags)
		safeGo("probeAspects", a.probeVisibleAspects)
	})
}

func (a *appState) compareMedia(x, y mediaItem) int {
	if a.FolderFirst && x.IsDir != y.IsDir {
		if x.IsDir {
			return -1
		}
		return 1
	}
	key := a.Sort
	desc := strings.HasSuffix(key, "-desc")
	key = strings.TrimSuffix(key, "-desc")

	cmp := 0
	switch key {
	case "size":
		cmp = compareInt64(x.Size, y.Size)
	case "type":
		cmp = strings.Compare(strings.ToLower(x.Ext), strings.ToLower(y.Ext))
	case "date", "created", "added":
		cmp = compareInt64(x.ModTime, y.ModTime)
	case "tag":
		cmp = strings.Compare(firstTag(x.Tags), firstTag(y.Tags))
	case "random":
		cmp = compareUint64(randomSortKey(x.Path, a.sortSalt), randomSortKey(y.Path, a.sortSalt))
	case "exif":
		cmp = strings.Compare(x.Info.Created, y.Info.Created)
		if cmp == 0 {
			cmp = compareInt64(x.ModTime, y.ModTime)
		}
	default:
		xName, yName := x.Name, y.Name
		if a.FullPathSort {
			xName, yName = x.Path, y.Path
		}
		cmp = strings.Compare(strings.ToLower(xName), strings.ToLower(yName))
	}
	if cmp == 0 {
		cmp = strings.Compare(strings.ToLower(x.Path), strings.ToLower(y.Path))
	}
	if desc {
		return -cmp
	}
	return cmp
}

func firstTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(tags[0]))
}

func compareInt64(x, y int64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	default:
		return 0
	}
}

func compareUint64(x, y uint64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	default:
		return 0
	}
}

func randomSortKey(path string, salt int64) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d:%s", salt, path)
	return h.Sum64()
}

func ancestorRoots(path string) []string {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return []string{"/"}
	}
	roots := []string{"/"}
	current := ""
	for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
		if part == "" {
			continue
		}
		current += "/" + part
		roots = append(roots, current)
	}
	return roots
}

func (a *appState) filterLabel() string {
	switch a.FilterMode {
	case 1:
		return "图片"
	case 2:
		return "视频"
	default:
		return "全部"
	}
}

func (a *appState) layoutLabel() string {
	switch a.LayoutMode {
	case 1:
		return "详情"
	case 2:
		return "瀑布"
	case 3:
		return "紧凑"
	default:
		return "网格"
	}
}

// layoutSwitcher draws Finder's view control: four icon segments that pick
// the media layout (icon view, list, waterfall and justified mosaic).
func (a *appState) layoutSwitcher(c *ui.Context) {
	t := c.Theme()
	layouts := []struct {
		icon  *ui.SVG
		label string
	}{
		{gridIcon, "网格"},
		{detailIcon, "详情"},
		{waterfallIcon, "瀑布"},
		{justifiedIcon, "紧凑"},
	}
	seg := ui.SegmentedBase(c, &a.LayoutMode, len(layouts))
	seg.Track.AlignItems(ui.Stretch).Padding(2).Gap(2).Radius(6).Background(t.Surface).Children(func() {
		for i, layout := range layouts {
			active := a.LayoutMode == i
			s := seg.Segment(i).Justify(ui.Center).AlignItems(ui.Center).Padding(3, 6).Radius(4)
			if active {
				s.Background(t.Accent)
			}
			s.Tooltip(layout.label).Label(layout.label)
			s.Children(func() {
				color := t.Text
				if active {
					color = t.AccentText
				}
				ui.Icon(c, layout.icon).FontSize(13).TextColor(color)
			})
		}
	})
}

func (a *appState) sortLabel() string {
	base := strings.TrimSuffix(a.Sort, "-desc")
	names := map[string]string{
		"name":    "名称",
		"size":    "文件大小",
		"type":    "文件类型",
		"created": "创建日期",
		"date":    "修改日期",
		"added":   "添加日期",
		"tag":     tagsLabel,
		"random":  "随机",
		"exif":    "Exif 日期",
	}
	label := names[base]
	if label == "" {
		label = "名称"
	}
	return label
}

func (a *appState) finderSortMenu(m *ui.Menu) {
	if m.Item("文件夹优先排序").Checked(a.FolderFirst).Chosen() {
		a.FolderFirst = !a.FolderFirst
		a.refresh()
	}
	if m.Item("集合模式下使用完整路径排序").Checked(a.FullPathSort).Chosen() {
		a.FullPathSort = !a.FullPathSort
		a.refresh()
	}
	m.Item("说明…").Disabled(true)
	m.Separator()
	for _, item := range []struct{ label, value string }{
		{"文件名", "name"},
		{"文件名 (降序)", "name-desc"},
		{"文件大小", "size"},
		{"文件大小 (降序)", "size-desc"},
		{"文件类型", "type"},
		{"文件类型 (降序)", "type-desc"},
		{"创建日期", "created"},
		{"创建日期 (降序)", "created-desc"},
		{"修改日期", "date"},
		{"修改日期 (降序)", "date-desc"},
		{"添加日期", "added"},
		{"添加日期 (降序)", "added-desc"},
		{"Finder 标签", "tag"},
		{"Finder 标签 (倒序)", "tag-desc"},
		{"随机", "random"},
	} {
		if !tagsSupported && strings.HasPrefix(item.value, "tag") {
			continue
		}
		if m.Item(item.label).Radio(a.Sort == item.value).Chosen() {
			a.Sort = item.value
			if item.value == "random" {
				a.sortSalt = time.Now().UnixNano()
			}
			a.refresh()
		}
	}
	m.Submenu("根据 Exif 信息排序", func(sm *ui.Menu) {
		for _, item := range []struct{ label, value string }{
			{"拍摄日期", "exif"},
			{"拍摄日期 (降序)", "exif-desc"},
		} {
			if sm.Item(item.label).Radio(a.Sort == item.value).Chosen() {
				a.Sort = item.value
				a.refresh()
			}
		}
	})
}

func (a *appState) restorePendingSelection() {
	if len(a.PendingSelect) > 0 {
		want := map[string]struct{}{}
		for _, path := range a.PendingSelect {
			want[path] = struct{}{}
		}
		present := false
		for _, itemIndex := range a.Visible {
			if _, ok := want[a.Items[itemIndex].Path]; ok {
				present = true
				break
			}
		}
		if present {
			// Items moved elsewhere keep the reconciled selection instead.
			a.Selected.Clear()
		}
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
			a.revealFrames = 2
		}
		a.PendingSelect = nil
	}
	if !a.KeepLarge {
		if a.InLargeView && a.LargeIndex >= 0 && a.LargeIndex < len(a.Visible) {
			item := a.Items[a.Visible[a.LargeIndex]]
			if a.Window != nil {
				a.setTitle(fmt.Sprintf("%s (%d/%d)", item.Name, a.LargeIndex+1, len(a.Visible)))
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
		if a.Video.kernel != nil {
			a.Video.kernel.retarget(item.Path)
		}
	}
	if a.Window != nil {
		a.setTitle(fmt.Sprintf("%s (%d/%d)", item.Name, found+1, len(a.Visible)))
	}
}

func (a *appState) applyFilterLocked() {
	anchor := a.takeSelectionAnchorLocked()
	a.Visible = a.Visible[:0]
	q := strings.ToLower(strings.TrimSpace(a.Query))
	for i := range a.Items {
		if q != "" && !pinyinContains(a.Items[i].Name, q) {
			continue
		}
		if !a.Items[i].IsDir {
			ext := a.Items[i].Ext
			if (a.FilterMode == 1 && (!isImageFile(ext) && !isRawFile(ext))) || (a.FilterMode == 2 && !isVideoFile(ext)) {
				continue
			}
		}
		a.Visible = append(a.Visible, i)
	}
	a.noteLayoutChanged()
	a.reconcileSelectionLocked(anchor)
}

func (a *appState) startWatcher() {
	// A shelved window does not watch the disk. refreshView still scans
	// when a folder is opened, then startWatcher returns here until show.
	if a.isShelved() || a.Root == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.watchCtx = ctx
	a.watchStop = cancel
	root := a.Root
	recursive := a.Recursive || a.MediaUnitRoot == root
	a.mu.Unlock()

	safeGo("watchFiles", func() {
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
				a.update(func() { a.refresh() })
			}
		}
	})
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
	// Cmd+Up reaches both the menu accelerator and the view shortcut on
	// some desktops; one key press goes up one level only.
	if now := time.Now(); now.Sub(a.lastUp) < 150*time.Millisecond {
		return
	} else {
		a.lastUp = now
	}
	parent := filepath.Dir(a.Root)
	if parent != a.Root {
		from := a.Root
		a.openFolder(parent)
		a.selectAfterLoad(from)
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
	from := a.Root
	a.openFolder(path)
	a.selectAfterLoad(from)
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
	from := a.Root
	a.openFolder(path)
	a.selectAfterLoad(from)
}

// treeChildren answers the sidebar outline, which asks for every expanded
// folder on every frame. Listing a directory on the main thread each frame
// stalls scrolling and animation on slow or network volumes, so listings are
// cached: the first one is read at once, later ones are refreshed in the
// background once they are older than treeTTL.
func (a *appState) treeChildren(path string) []string {
	now := time.Now()
	if e, ok := a.treeCache[path]; ok {
		if now.Sub(e.at) > treeTTL && !e.refreshing && a.Window != nil {
			e.refreshing = true
			a.treeCache[path] = e
			safeGo("treeChildren", func() {
				dirs := listSubdirs(path)
				a.update(func() { a.treeCache[path] = treeEntry{dirs: dirs, at: time.Now()} })
			})
		}
		return e.dirs
	}
	if a.treeCache == nil || len(a.treeCache) > 4096 {
		a.treeCache = map[string]treeEntry{}
	}
	dirs := listSubdirs(path)
	a.treeCache[path] = treeEntry{dirs: dirs, at: now}
	return dirs
}

const treeTTL = 3 * time.Second

type treeEntry struct {
	dirs       []string
	at         time.Time
	refreshing bool
}

func listSubdirs(path string) []string {
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
		if err != nil {
			logErrorf("open folder dialog failed: %v", err)
		}
		return
	}
	a.update(func() { a.openFolder(paths[0]) })
}

func (a *appState) loadThumb(root string, index int) {
	a.mu.Lock()
	if a.shelved || index < 0 || index >= len(a.Items) {
		a.mu.Unlock()
		return
	}
	item := &a.Items[index]
	if item.Decoded || item.Failed || item.Loading {
		a.mu.Unlock()
		return
	}
	item.LoadGen++
	gen := item.LoadGen
	item.Loading = true
	path := item.Path
	video := item.IsVideo
	edge := thumbBand(thumbEdge(a.Thumb))
	a.mu.Unlock()

	safeGo("loadThumb", func() {
		if a.thumbClaimStale(index, path, gen) {
			a.clearThumbClaim(index, path, gen)
			return
		}
		key := thumbCacheKey(path, edge)
		if bitmap := thumbLRU.Get(key); bitmap != nil {
			a.update(func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if a.Root != root || index >= len(a.Items) || a.Items[index].Path != path {
					return
				}
				current := &a.Items[index]
				if current.LoadGen != gen {
					return
				}
				current.Bitmap = bitmap
				current.Decoded = true
				current.ThumbEdge = edge
				current.Loading = false
			})
			return
		}
		thumbGate <- struct{}{}
		defer func() { <-thumbGate }()
		// A claim that was queued before the window was shelved wakes up
		// here and leaves without decoding.
		if a.thumbClaimStale(index, path, gen) {
			a.clearThumbClaim(index, path, gen)
			return
		}
		if a.thumbAbandoned(index, path, gen) {
			return
		}
		data, fromDisk := diskThumbs.Get(path, edge)
		var err error
		if !fromDisk {
			data, err = thumbBytes(path, video, edge)
			if err == nil {
				diskThumbs.Put(path, edge, data)
			}
		}
		if err != nil && !video {
			data, err = os.ReadFile(path)
		}
		var bitmap *ui.Bitmap
		if err == nil {
			bitmap, err = ui.DecodeBitmap(data)
		}
		a.update(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.Root != root || index >= len(a.Items) || a.Items[index].Path != path {
				return
			}
			current := &a.Items[index]
			if current.LoadGen != gen {
				return
			}
			if err != nil {
				current.Failed = true
				logDebugf("thumbnail decode failed for %s: %v", path, err)
			} else {
				current.Bitmap = bitmap
				current.Decoded = true
				current.ThumbEdge = edge
				thumbLRU.Put(key, bitmap, edge)
			}
			current.Loading = false
		})
	})
}

// ============================
// Views
// ============================

// tests set it, to run background results on the test goroutine.
// headlessUpdate receives update callbacks when there is no window. Tests
// install one; access is atomic because background jobs call update while a
// test may still be installing or clearing the hook.
var headlessUpdate atomic.Pointer[func(fn func())]

// update runs fn on the window's main thread when a window exists. Background
// work that touches the interface goes through it, so headless runs (tests or
// an early startup failure) do nothing instead of panicking on a nil Window.
func (a *appState) update(fn func()) {
	if a.Window == nil {
		if hook := headlessUpdate.Load(); hook != nil {
			(*hook)(fn)
		}
		return
	}
	a.Window.Update(fn)
}

// visibleItem returns the item at visible index i, or nil when either index is
// out of range. Visible and Items are rewritten by background scans, so a view
// that runs while a scan swaps them can see a stale index; this keeps that from
// panicking with an out-of-range slice access.
func (a *appState) visibleItem(i int) *mediaItem {
	if i < 0 || i >= len(a.Visible) {
		return nil
	}
	itemIndex := a.Visible[i]
	if itemIndex < 0 || itemIndex >= len(a.Items) {
		return nil
	}
	return &a.Items[itemIndex]
}

// itemKey is the identity a media row carries in selection state. Grid and list
// rows must use the same string keys as a.Selected (a Selection[string]); a
// default row key is the row index, and mixing those panics in mygo with
// "a row's key is a int, not the string of its Selection".
func (a *appState) itemKey(i int) any {
	if item := a.visibleItem(i); item != nil {
		return item.Path
	}
	return fmt.Sprintf("!missing:%d", i)
}

func (a *appState) view(c *ui.Context) {
	safeView("view", c, func() { a.rootView(c) })
}

func (a *appState) rootView(c *ui.Context) {
	a.syncCaption()
	t := c.Theme()
	c.Root().Background(t.Background)
	a.tickAutomation(c)
	a.beginFrame()
	defer a.finishFrame()
	if !a.InLargeView {
		a.settleThumbs(c)
		a.sweepThumbs(c)
	}

	// Global shortcuts.
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		safeGo("dialog.pickFolder", a.pickFolder)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		a.showDiagnostics()
	}
	if c.Shortcut(ui.Cmd, ui.KeyUp) {
		a.goUp()
	}
	if !a.InLargeView && !a.modalOpen() && c.Shortcut(ui.Cmd, ui.KeyBracketLeft) {
		a.goBack()
	}
	if !a.InLargeView && !a.modalOpen() && c.Shortcut(ui.Cmd, ui.KeyBracketRight) {
		a.goForward()
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		a.SidebarShown = !a.SidebarShown
	}
	// Picture arrows change the file. Video arrows belong to the playhead,
	// so this query skips a video or it would consume the key first.
	if !a.Editor.Open && !a.videoLargeView() && (c.Shortcut(ui.Cmd, ui.KeyLeft) || (a.InLargeView && c.Shortcut(0, ui.KeyLeft))) {
		if a.InLargeView {
			a.prevImage()
		} else {
			a.goBack()
		}
	}
	if !a.Editor.Open && !a.videoLargeView() && (c.Shortcut(ui.Cmd, ui.KeyRight) || (a.InLargeView && c.Shortcut(0, ui.KeyRight))) {
		if a.InLargeView {
			a.nextImage()
		} else {
			a.goForward()
		}
	}
	// Up/Down step through the folder in the large view and the gallery,
	// pictures and videos alike; video volume moved to Shift+Up/Down.
	if a.InLargeView && !a.Editor.Open && !a.modalOpen() {
		if c.Shortcut(0, ui.KeyUp) {
			a.prevImage()
		} else if c.Shortcut(0, ui.KeyDown) {
			a.nextImage()
		}
	}
	a.videoPlayerKeys(c)
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
		a.togglePinned()
	}
	// Large-view single-key shortcuts matching FlowVision.
	if a.InLargeView && !a.Editor.Open && !a.modalOpen() {
		if c.Shortcut(0, ui.KeyZ) {
			a.zoom100()
		}
		if c.Shortcut(0, ui.KeyX) {
			a.resetZoom()
		}
		if !a.videoLargeView() && c.Shortcut(0, ui.KeyM) {
			a.transferToDownloads(opMove)
		}
		if !a.videoLargeView() && c.Shortcut(0, ui.KeyHome) {
			a.openLargeImage(0)
		}
		if !a.videoLargeView() && c.Shortcut(0, ui.KeyEnd) {
			a.openLargeImage(len(a.Visible) - 1)
		}
		if c.Shortcut(0, ui.KeyW) {
			a.zoomStep(1.25)
		}
		if c.Shortcut(0, ui.KeyS) {
			a.zoomStep(1 / 1.25)
		}
	}
	// F3 = search, F5 = refresh.
	if c.Shortcut(0, ui.KeyF3) && !a.InLargeView {
		a.FocusSearch = true
	}
	if c.Shortcut(0, ui.KeyF5) {
		a.refresh()
	}
	// Cmd+Down opens selected item or enters folder.
	if c.Shortcut(ui.Cmd, ui.KeyDown) && !a.InLargeView && !a.modalOpen() {
		if a.Chosen >= 0 && a.Chosen < len(a.Visible) {
			a.openItem(a.Chosen)
		}
	}
	// Alt+Enter toggles fullscreen.
	if c.Shortcut(ui.Alt, ui.KeyEnter) {
		a.Window.ToggleFullScreen()
	}
	// Cmd+1~9 toggles Finder tags on selection.
	if !a.InLargeView && !a.modalOpen() {
		for i := 0; i < 9; i++ {
			key := ui.Key1 + ui.Key(i)
			if c.Shortcut(ui.Cmd, key) {
				names := finderLabelNames()
				if i < len(names) {
					paths := a.operationPaths()
					if len(paths) > 0 {
						a.toggleTag(paths, names[i])
					}
				}
			}
		}
	}
	// Ctrl+1~5 sets rating, Ctrl+0 clears.
	if !a.InLargeView && !a.modalOpen() {
		for i := 0; i <= 5; i++ {
			var key ui.Key
			if i == 0 {
				key = ui.Key0
			} else {
				key = ui.Key1 + ui.Key(i-1)
			}
			if c.Shortcut(ui.Ctrl, key) {
				paths := a.operationPaths()
				if len(paths) > 0 {
					a.setRating(paths, i)
				}
			}
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyPeriod) && !a.modalOpen() {
		a.toggleViewOption("hidden")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyComma) && !a.modalOpen() {
		a.toggleViewOption("all")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyR) && !a.modalOpen() && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		a.setRecursive(!a.Recursive)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) && !a.modalOpen() && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		a.toggleViewOption("folders")
	}
	if c.Shortcut(0, ui.Key3) && a.InLargeView && !a.videoLargeView() {
		a.fitWindowToImage(false)
	}
	if c.Shortcut(0, ui.Key4) && a.InLargeView && !a.videoLargeView() {
		a.fitWindowToImage(true)
	}
	if !a.InLargeView && !a.modalOpen() && !a.Renaming && !a.NewFolder && !a.Batch.Open {
		switch {
		case c.Shortcut(0, ui.KeyEqual):
			a.zoomBrowse(1.15, 0, 0, false)
		case c.Shortcut(0, ui.KeyMinus):
			a.zoomBrowse(1/1.15, 0, 0, false)
		case c.Shortcut(0, ui.Key0):
			if a.Thumb > 0 {
				a.zoomBrowse(float32(128/a.Thumb), 0, 0, false)
			}
		}
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
	// MyGo's Shortcut consumes the delivered key on the first query, so
	// the old modal/large if-else chain — which asked twice — let the
	// modal query eat every Escape and the large view could never close.
	// Query once, then branch on the guards.
	switch c.Shortcut(0, ui.KeyEscape) {
	case true:
		switch {
		case a.modalOpen():
			a.AutoConfig = ""
			a.AutoValue = ""
			a.ZipAsk = false
			a.ZipPass = ""
			a.CropAsk = false
			a.TrimAsk = false
			a.InfoAsk = false
			a.closeEditor()
		case a.InLargeView:
			if a.videoLargeView() && a.Window != nil && a.Window.IsFullScreen() {
				a.Window.ToggleFullScreen()
			} else {
				a.closeLargeImage()
			}
		}
	}
	if c.Shortcut(0, ui.KeySpace) && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		if a.InLargeView {
			if item := a.currentItem(); item != nil && item.IsVideo {
				a.toggleVideoPlayback()
			}
		} else if a.Chosen >= 0 {
			a.Gallery = false // Space is the plain quick look; G opens the gallery
			a.openItem(a.Chosen)
		}
	}
	if c.Shortcut(0, ui.KeyG) && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() && !a.Editor.Open {
		a.toggleGallery()
	}
	if c.Shortcut(0, ui.KeyH) && a.galleryActive() {
		a.galleryPeek = !a.galleryPeek
	}
	if c.Shortcut(0, ui.KeyI) && a.InLargeView {
		a.Info.EXIF = !a.Info.EXIF
	}
	if c.Shortcut(0, ui.KeyU) && a.InLargeView {
		a.Info.File = !a.Info.File
	}
	if !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		switch {
		case c.Shortcut(ui.Shift, ui.KeyDelete), c.Shortcut(ui.Shift, ui.KeyBackspace):
			a.doPermanentDelete()
		case c.Shortcut(0, ui.KeyDelete), c.Shortcut(0, ui.KeyBackspace):
			a.doTrash()
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) && !a.InLargeView && !a.Renaming && !a.NewFolder && !a.modalOpen() {
		a.newFolder()
	}
	if c.Shortcut(0, ui.KeyF2) && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
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
	if c.Shortcut(ui.Cmd, ui.KeyC) && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		a.doCopy()
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) && !a.InLargeView && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		a.doPaste(false)
	}
	if c.Shortcut(ui.Cmd|ui.Alt, ui.KeyV) && !a.InLargeView && !a.Renaming && !a.NewFolder && !a.modalOpen() {
		a.doPaste(true)
	}
	if c.Shortcut(ui.Cmd, ui.KeyA) && !a.InLargeView && !a.Renaming && !a.NewFolder && !a.modalOpen() {
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
	for i := 0; i < 9; i++ {
		key := ui.Key1 + ui.Key(i)
		if c.Shortcut(ui.Alt, key) {
			a.applyProfile(i)
		}
		if c.Shortcut(ui.Cmd|ui.Alt, key) {
			a.saveProfile(i)
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyX) && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		a.doCut()
	}
	a.browseKeys(c)

	// Configured copy shortcuts: image folder and video folder.
	if !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		if key, ok := shortcutKey(a.Profiles.ImageCopyShortcut); ok && c.Shortcut(0, key) {
			a.copyToImageFolder()
		}
		if key, ok := shortcutKey(a.Profiles.VideoCopyShortcut); ok && c.Shortcut(0, key) {
			a.copyToVideoFolder()
		}
	}

	// Enter key: rename or open/close depending on preference.
	if c.Shortcut(0, ui.KeyEnter) && !a.Renaming && !a.NewFolder && !a.Batch.Open && !a.modalOpen() {
		if a.InLargeView {
			a.closeLargeImage()
		} else if a.Chosen >= 0 {
			if a.Profiles.EnterOpens {
				a.openItem(a.Chosen)
			} else {
				a.startRename()
			}
		}
	}

	a.browserLayout(c)
}

func (a *appState) sidebarView(c *ui.Context) {
	t := c.Theme()
	pane := ui.Column(c).Fill().Background(t.Surface).Gap(0)
	pane.Children(func() {
		// TitleBarHiddenInset puts the traffic lights over the sidebar's
		// top-left; keep a drag strip under them so "收藏夹" starts below.
		if tb := c.TitleBar(); tb.Left > 0 {
			h := tb.Height
			if h < 28 {
				h = 28
			}
			ui.Box(c).Height(h).FillWidth().DragWindow()
		}
		ui.Row(c).Padding(8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, starIcon).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "收藏夹").FontSize(11).Bold().TextColor(t.TextMuted)
		})
		for _, fav := range a.Profiles.Favorites {
			fav := fav
			if !isDir(fav) {
				continue
			}
			row := ui.Row(c).Padding(4, 12).Gap(6).AlignItems(ui.Center).Radius(4)
			if pathKey(a.Root) == pathKey(fav) {
				row.Background(t.SurfacePressed)
			}
			row.Children(func() {
				ui.Icon(c, dirIcon).FontSize(12)
				ui.Text(c, pathName(fav)).FontSize(11).SingleLine().MaxLines(1)
			})
			a.folderDropTarget(c, row, fav)
			row.OnClick(func() { a.openFolder(fav) })
		}
		ui.Divider(c)
		if a.Root == "" {
			ui.Text(c, "打开目录后显示目录树").TextColor(t.TextMuted).Padding(12)
			return
		}
		// Selection stays app-side (see mediaListState): rows paint the
		// current folder themselves, quietly, in treeRow.
		a.Outline.List.Key = func(row int) any { return a.Outline.Item(row) }
		roots := []string{"/"}
		if a.treeOpenRoot != a.Root {
			for _, ancestor := range ancestorRoots(a.Root) {
				if ancestor != filepath.Clean(a.Root) {
					a.Outline.Open.Add(ancestor)
				}
			}
			a.treeOpenRoot = a.Root
		}
		ui.Outline(c, &a.Outline, roots, a.treeChildren, func(path string) { a.treeRow(c, path) }).Grow(1)
		ui.Divider(c)
		// History section: recent folders visited in this session.
		ui.Row(c).Padding(8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, clockIcon).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, "历史").FontSize(11).Bold().TextColor(t.TextMuted).Grow(1)
			if len(a.Recent) > 0 {
				x := ui.Box(c).Padding(2).Radius(3).Tooltip("清除历史记录")
				x.Children(func() { ui.Icon(c, lucideClose).FontSize(10).TextColor(t.TextMuted) })
				if x.Clicked() {
					a.clearHistory()
				}
			}
		})
		for _, h := range a.Recent {
			hist := h
			row := ui.Row(c).Padding(4, 12).Gap(6).AlignItems(ui.Center).Radius(4)
			if pathKey(hist) == pathKey(a.Root) {
				row.Background(t.SurfacePressed)
			}
			row.Children(func() {
				ui.Icon(c, dirIcon).FontSize(12)
				ui.Text(c, pathName(hist)).FontSize(11).SingleLine().MaxLines(1).Grow(1).MinWidth(0)
				if at := a.Visited[hist]; at > 0 {
					ui.Text(c, visitLabel(at, time.Now())).FontSize(10).TextColor(t.TextMuted).SingleLine().Shrink(0)
				}
			})
			row.ContextMenu(func(m *ui.Menu) {
				if m.Item("清除历史记录").Chosen() {
					a.clearHistory()
				}
			})
			a.folderDropTarget(c, row, hist)
			row.OnClick(func() { a.openFolder(hist) })
		}
	})
}

func (a *appState) treeRow(c *ui.Context, path string) {
	t := c.Theme()
	row := ui.Row(c).Padding(4, 8).Gap(6).AlignItems(ui.Center).Radius(4)
	selected := a.Root == path
	if selected {
		row.Background(t.SurfacePressed)
	}
	row.Children(func() {
		ui.Icon(c, dirIcon).FontSize(14)
		ui.Text(c, pathName(path)).FontSize(12).SingleLine()
	})
	a.folderDropTarget(c, row, path)
	row.OnClick(func() { a.openFolder(path) })
}

func (a *appState) mainView(c *ui.Context) {
	t := c.Theme()
	col := ui.Column(c).Fill().Background(t.Background)
	col.HandleInput(a.handleGesture)
	if bounds := col.LayoutBounds(); bounds.W > 1 {
		a.columnX, a.columnY = bounds.X, bounds.Y
	}
	if a.Root != "" && !a.InLargeView {
		if col.FileDragOver() {
			col.Border(2, t.Accent)
		}
		if files := col.DroppedFiles(); len(files) > 0 {
			a.importDropped(files)
		}
		a.folderDropTarget(c, col, a.Root)
	}
	col.Children(func() {
		if !a.galleryActive() {
			a.tabsBar(c)
			a.toolbarView(c)
			ui.Divider(c)
		}
		if a.galleryActive() {
			a.galleryView(c)
		} else if a.InLargeView {
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
		if !a.galleryActive() {
			a.statusBar(c)
		}
		if !a.InLargeView {
			a.zoomHUD(c)
		}
		a.toastOverlay(c)
	})
}

func (a *appState) toolbarView(c *ui.Context) {
	t := c.Theme()
	bar := ui.Toolbar(c, func() {
		// 左侧：导航控件组
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if lucideIconButton(c, "sidebar", "显示/隐藏侧栏", sidebarIcon).Clicked() {
				a.SidebarShown = !a.SidebarShown
			}
			ui.Box(c).Width(1).Height(18).Background(t.Border)
			if lucideIconButton(c, "back", "后退", arrowLeftIcon).Clicked() {
				a.goBack()
			}
			if lucideIconButton(c, "forward", "前进", arrowRightIcon).Clicked() {
				a.goForward()
			}
		})

		ui.Box(c).Width(12) // 间距

		// 中间：标题或面包屑
		if a.InLargeView {
			item := a.currentItem()
			title := "无媒体"
			if item != nil {
				title = fmt.Sprintf("%s (%d/%d)", item.Name, a.LargeIndex+1, len(a.Visible))
			}
			ui.Text(c, title).FontSize(12).Bold().Grow(1).MaxLines(1)
		} else {
			items := strings.Split(a.breadcrumbPath(), " › ")
			chosen := len(items) - 1
			bc := ui.Breadcrumbs(c, items, &chosen).Label("路径").Grow(1).MinWidth(160)
			if bc.Changed() {
				a.openBreadcrumb(chosen)
			}
		}

		ui.Box(c).Width(8) // 间距

		// 右侧：浏览控件组
		if !a.InLargeView {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				prevQuery := a.LastQuery
				search := ui.SearchField(c.Key("query"), &a.Query).Width(150).Label("搜索")
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
				ui.MenuButton(c, a.filterLabel(), func(m *ui.Menu) {
					for i, name := range []string{"全部", "图片", "视频"} {
						idx := i
						if m.Item(name).Radio(a.FilterMode == idx).Chosen() {
							a.FilterMode = idx
							a.applyFilter()
						}
					}
				})
				ui.Box(c).Width(1).Height(18).Background(t.Border)
				a.layoutSwitcher(c)
				nc := lucideIconButton(c, "nocaption", "无字模式 (⇧⌘H)", captionIcon)
				if a.Profiles.View.NoCaption {
					nc.Background(t.Accent)
				}
				if nc.Clicked() {
					a.toggleCaption()
				}
				g := lucideIconButton(c, "gallery", "画廊 (G)", galleryIcon)
				if a.Gallery {
					g.Background(t.Accent)
				}
				if g.Clicked() {
					a.toggleGallery()
				}
				oldThumb := a.Thumb
				beforeBand := thumbBand(thumbEdge(a.Thumb))
				box := a.browseBox
				if ui.Slider(c, &a.Thumb, 64, 512).Width(80).Changed() {
					a.noteThumbBand(beforeBand)
					a.anchorBrowse(oldThumb, a.Thumb, 0, 0, false, box)
				}
				ui.MenuButton(c, a.sortLabel(), a.finderSortMenu)
			})
		}
		lucideMenuButton(c, "显示与浏览", a.viewMenu)
	}).Padding(6, 10).Background(t.Surface)
	// Under a hidden macOS title bar the top row drags the window. The
	// traffic lights sit over the sidebar when it is open, otherwise over
	// this toolbar — only then pad past them.
	if tb := c.TitleBar(); tb.Left > 0 && len(a.Tabs.Tabs) == 0 {
		s := t.Space(1)
		if a.SidebarShown && !a.InLargeView {
			bar.Padding(s).DragWindow()
		} else {
			bar.Padding(s, s, s, tb.Left+s).DragWindow()
		}
	}
}

func (a *appState) breadcrumbPath() string {
	return strings.Join(a.breadcrumbItems(), " › ")
}

func (a *appState) breadcrumbItems() []string {
	if a.Root == "" {
		return []string{"未打开目录"}
	}
	parts := strings.Split(strings.Trim(a.Root, string(filepath.Separator)), string(filepath.Separator))
	out := []string{rootName}
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// openBreadcrumb opens the folder for breadcrumbs index (0 = volume root).
func (a *appState) openBreadcrumb(index int) {
	if a.Root == "" || index < 0 {
		return
	}
	items := a.breadcrumbItems()
	if index >= len(items)-1 {
		return // already there (last crumb is not a link)
	}
	if index == 0 {
		if runtime.GOOS == "windows" {
			return
		}
		a.openFolder("/")
		return
	}
	sep := string(filepath.Separator)
	parts := strings.Split(strings.Trim(a.Root, sep), sep)
	var cleaned []string
	for _, p := range parts {
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if index > len(cleaned) {
		return
	}
	path := strings.Join(cleaned[:index], sep)
	if runtime.GOOS != "windows" {
		path = "/" + path
	}
	a.openFolder(path)
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
	state := a.mediaListState()
	cell := int(a.Thumb + gridCellPad)
	gap := c.Theme().Space(2)
	a.applyGridAnchor()
	if a.takeReveal() {
		a.Grid.ScrollIntoView(a.Chosen)
	}
	grid := ui.GridView(c, state.Grid, len(a.Visible), float32(cell), float32(cell)+flowLabel, func(i int) {
		item := a.visibleItem(i)
		if item == nil {
			return
		}
		thumbCardProbe()
		a.markSeen(item)
		card := ui.Column(c).Grow(1).Gap(4)
		if a.isCut(item.Path) {
			card.Opacity(0.4)
		}
		selectionChrome(c, card, a.Chosen == i || a.Selected.Has(item.Path))
		card.Children(func() {
			thumb := ui.Box(c).Grow(1).Background(t.Background).Clip().Justify(ui.Center).AlignItems(ui.Center)
			a.thumbHero(thumb, item.Path)
			thumb.Children(func() {
				if dots := tagDots(item.Tags); len(dots) > 0 {
					tags := ui.Row(c).Attach(ui.AnchorTopEnd, ui.AnchorTopEnd).Margin(4, 4).Gap(2)
					tags.Children(func() {
						for _, tag := range dots {
							ui.Box(c).Width(5).Height(5).Radius(2).Background(tagColor(tag))
						}
					})
				}
				paintMediaThumb(c, item, 0, ui.Contain)
			})
			if flowLabel > 0 {
				ui.Text(c, item.Name).FontSize(11).MaxLines(2).TextAlign(ui.Center)
			}
		})
		card.Tooltip(fmt.Sprintf("%s\n%.1f MB", item.Name, float64(item.Size)/(1024*1024)))
		a.fileDragSource(card, item.Path)
		a.cardPointer(card, i)
		card.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
		if !item.Decoded && !item.Failed && !item.Loading {
			a.requestThumb(root, a.Visible[i])
		}
	})
	grid.Grow(1).TrackScroll(&a.browseScroll)
	if bounds := grid.LayoutBounds(); bounds.W > 1 {
		contentW := gridRowWidth(&a.Grid)
		if contentW <= 0 {
			contentW = bounds.W
		}
		cols := gridColumnCount(contentW, float32(cell), gap)
		a.noteBrowseBox(bounds, cols, gridStride(a.Thumb, gap), gap, contentW, len(a.Visible))
	}
	// Opening on Enter/Space is handled by the global shortcuts; the grid
	// widget's own submit/selection path is not wired (see mediaListState).
}

func (a *appState) statusBar(c *ui.Context) {
	t := c.Theme()
	bar := ui.Row(c).Padding(4, 12).Background(t.Surface).AlignItems(ui.Center)
	bar.Children(func() {
		// 左侧：选中项信息
		ui.Text(c, a.statusText()).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1)

		// 右侧：性能指标（可选）
		if len(a.Visible) > 0 {
			ui.Box(c).Width(1).Height(14).Background(t.Border)
			ui.Box(c).Width(8) // 间距
			info := fmt.Sprintf("%d 可见", len(a.Visible))
			if a.Chosen >= 0 {
				info += fmt.Sprintf(" · %d 选中", a.Chosen+1)
			}
			ui.Text(c, info).FontSize(10).TextColor(t.TextMuted).SingleLine()
		}
	})
}

func (a *appState) largeImageView(c *ui.Context) {
	if a.Editor.Open {
		a.editorView(c)
		return
	}
	t := c.Theme()
	item := a.currentItem()
	if item != nil && item.IsVideo {
		a.tickPlaybackEnd()
		item = a.currentItem()
	}
	if item == nil {
		ui.Text(c, "没有图像").TextColor(t.TextMuted)
		return
	}
	a.ensureMediaInfo(item)
	bg := t.Background
	if a.galleryActive() {
		bg = ui.RGB(0, 0, 0)
	}
	box := ui.Box(c).Fill().Background(bg)
	a.tickLongPress(c, item)
	box.Children(func() {
		// The gesture surface sits behind the controls so buttons still receive clicks.
		stage := ui.Box(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Clip()
		if bounds := stage.LayoutBounds(); bounds.W > 1 && bounds.H > 1 {
			a.StageW, a.StageH = bounds.W, bounds.H
		}
		if !item.IsVideo {
			a.ensureLarge(a.LargeIndex)
		}
		showSharp := a.prepareSharp(c, item)
		stage.HandleInput(func(ev ui.InputEvent) bool { return a.largeInput(ev, item) })
		if item.IsVideo {
			if x, y, over := stage.PointerPosition(); over {
				a.notePlayerPointer(x, y)
			}
		}
		stage.ContextMenu(func(m *ui.Menu) { a.largeContextMenu(m, item) })
		stage.Children(func() {
			if showSharp {
				ui.Image(c, a.Sharp).Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
					Left(a.SharpPlace.X).Top(a.SharpPlace.Y).
					Size(a.SharpPlace.W, a.SharpPlace.H).Fit(ui.FillBox)
			} else if item.IsVideo {
				a.ensureVideo(item.Path)
				a.settleVideoEdge(c)
				if !a.largeHero(c, item, nil, func() { a.videoLayer(c, true) }) {
					a.videoLayer(c, true)
				}
			} else {
				pic := item.Large
				if pic == nil {
					pic = item.Bitmap
				}
				if pic == nil {
					ui.Spinner(c)
				} else {
					tr := a.transform()
					if !sharpReady(item, a.zoomPercent(item)) {
						// Keep the zoomed size on the thumbnail while the
						// original loads; snapping back to fit read as a jump.
						a.HUD = "原图加载中"
						a.HUDUntil = time.Now().Add(1200 * time.Millisecond)
					}
					if !a.largeHero(c, item, pic, func() { ui.Image(c, pic).Fit(ui.Cover).Grow(1).Transform(tr) }) {
						ui.Image(c, pic).Fit(ui.Contain).Grow(1).Transform(tr)
					}
				}
			}
		})
		a.mediaOverlay(c, item)
		a.zoomHUD(c)
		a.edgeArrows(c)
		if !a.galleryActive() || item.IsVideo {
			a.largeControls(c, item)
		}
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
		if item.IsVideo && ev.Button == 1 {
			a.SeekFrom = a.playhead()
		}
		if ev.Button == 0 && ev.Clicks >= 2 {
			if item.IsVideo {
				a.toggleFullscreen()
				a.playerSkipClick = true
				return true
			}
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
		if a.LargeDrag.button == 1 && item.IsVideo && a.Video != nil && a.Video.kernel != nil {
			if !a.Video.kernel.Scrubbing() {
				a.Video.kernel.BeginScrub()
			}
			preview := seekPreview(a.SeekFrom, float64(ev.X-a.LargeDrag.startX), float64(a.largeWidth()), a.playheadDuration())
			a.Video.kernel.ScrubTo(preview)
			a.HUD = "跳转到 " + formatPlaybackTime(preview)
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
		if a.playerSkipClick {
			a.playerSkipClick = false
			if button == 1 {
				a.RightDown = false
			}
			return true
		}
		if button == 1 {
			a.RightDown = false
			if item.IsVideo && moved && a.Video != nil && a.Video.kernel != nil && a.Video.kernel.Scrubbing() {
				preview := seekPreview(a.SeekFrom, float64(ev.X-startX), float64(a.largeWidth()), a.playheadDuration())
				a.Video.kernel.ScrubTo(preview)
				a.Video.kernel.EndScrub()
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
		case ui.KeyF:
			if item.IsVideo {
				a.toggleFullscreen()
				return true
			}
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
	// 半透明深色背景 + 投影 + 圆角，类似 macOS 系统 HUD
	bgColor := ui.RGBA(0, 0, 0, 0.75)
	hud := ui.Row(c).Attach(ui.AnchorTop, ui.AnchorTop).Margin(24, 0).Padding(8, 16).Radius(8).Background(bgColor)
	hud.Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.3))
	hud.Children(func() {
		ui.Text(c, a.HUD).FontSize(13).Bold().TextColor(ui.RGB(255, 255, 255))
	})
}

// largeControls is the floating bottom control strip.
func (a *appState) largeControls(c *ui.Context, item *mediaItem) {
	if item.IsVideo {
		a.videoControlsBar(c, item)
		return
	}
	// 半透明背景 + 投影，与 HUD 风格一致
	bgColor := ui.RGBA(0, 0, 0, 0.7)
	controls := ui.Row(c).Attach(ui.AnchorBottom, ui.AnchorBottom).Margin(16, 0).Gap(6).Padding(8).Background(bgColor).Radius(8)
	controls.Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.3))
	controls.Children(func() {
		if lucideIconButton(c, "zoom-out", "缩小", lucideMinus).Clicked() {
			a.zoomStep(1 / 1.25)
		}
		if ui.Button(c.Key("fit"), "适应").Tooltip("适应窗口").Clicked() {
			a.resetZoom()
		}
		if lucideIconButton(c, "zoom-in", "放大", lucidePlus).Clicked() {
			a.zoomStep(1.25)
		}
		if ui.Button(c.Key("zoom-100"), "1:1").Tooltip("实际大小").Clicked() {
			a.zoom100()
		}
		if lucideIconButton(c, "rotate", "顺时针旋转", lucideRotateCW).Clicked() {
			a.Rotation += 90
		}
		if lucideIconButton(c, "rotate-ccw", "逆时针旋转", lucideRotateCCW).Clicked() {
			a.Rotation -= 90
		}
		if lucideIconButton(c, "mirror", "镜像", lucideMirror).Clicked() {
			a.Mirror = !a.Mirror
		}
	})
}

func (a *appState) toggleVideoVolume() {
	if a.Video == nil {
		return
	}
	if a.VideoVolume > 0 {
		a.PrevVolume = a.VideoVolume
		a.setVideoVolume(0)
		return
	}
	volume := a.PrevVolume
	if volume <= 0 || volume > 2 {
		volume = 1
	}
	a.setVideoVolume(volume)
}

func (a *appState) setVideoVolume(volume float64) {
	a.VideoVolume = normalizedPlaybackVolume(volume)
	if a.VideoVolume > 0 {
		a.PrevVolume = a.VideoVolume
	}
	if a.Video != nil && a.Video.kernel != nil {
		a.Video.kernel.SetVolume(a.VideoVolume)
	}
	a.Profiles.Volume = a.VideoVolume
	a.Profiles.PreviousVolume = a.PrevVolume
	savePrefs(a.Profiles)
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

// videoLargeView reports whether the current large-view item is a video.
func (a *appState) videoLargeView() bool {
	item := a.currentItem()
	return item != nil && item.IsVideo
}

func shortTime(seconds float64) string {
	return formatPlaybackTime(seconds)
}

// shortcutKey maps a configured shortcut string to a ui.Key.
func shortcutKey(name string) (ui.Key, bool) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "A":
		return ui.KeyA, true
	case "B":
		return ui.KeyB, true
	case "C":
		return ui.KeyC, true
	case "D":
		return ui.KeyD, true
	case "E":
		return ui.KeyE, true
	case "F":
		return ui.KeyF, true
	case "G":
		return ui.KeyG, true
	case "H":
		return ui.KeyH, true
	case "J":
		return ui.KeyJ, true
	case "K":
		return ui.KeyK, true
	case "L":
		return ui.KeyL, true
	case "N":
		return ui.KeyN, true
	case "O":
		return ui.KeyO, true
	case "P":
		return ui.KeyP, true
	case "V":
		return ui.KeyV, true
	case "Y":
		return ui.KeyY, true
	case "0":
		return ui.Key0, true
	case "6":
		return ui.Key6, true
	case "7":
		return ui.Key7, true
	case "8":
		return ui.Key8, true
	case "9":
		return ui.Key9, true
	case "=":
		return ui.KeyEqual, true
	case "-":
		return ui.KeyMinus, true
	case ",":
		return ui.KeyComma, true
	case ".":
		return ui.KeyPeriod, true
	case "[":
		return ui.KeyBracketLeft, true
	case "]":
		return ui.KeyBracketRight, true
	case "F1":
		return ui.KeyF1, true
	case "F4":
		return ui.KeyF4, true
	case "F6":
		return ui.KeyF6, true
	case "F7":
		return ui.KeyF7, true
	case "F8":
		return ui.KeyF8, true
	case "F9":
		return ui.KeyF9, true
	case "F10":
		return ui.KeyF10, true
	case "F11":
		return ui.KeyF11, true
	case "F12":
		return ui.KeyF12, true
	}
	return 0, false
}

func (a *appState) copyToImageFolder() {
	dest := strings.TrimSpace(a.Profiles.ImageDestination)
	if dest == "" || !isDir(dest) {
		a.toast("请先在设置中配置图片文件夹")
		return
	}
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	a.beginTransfer(fileOperation{op: opCopy, paths: paths, dest: dest})
}

func (a *appState) copyToVideoFolder() {
	dest := strings.TrimSpace(a.Profiles.VideoDestination)
	if dest == "" || !isDir(dest) {
		a.toast("请先在设置中配置视频文件夹")
		return
	}
	var paths []string
	if a.InLargeView {
		if item := a.currentItem(); item != nil && item.IsVideo {
			paths = []string{item.Path}
		}
	} else {
		for _, path := range a.selectedPaths() {
			if isVideoFile(strings.TrimPrefix(filepath.Ext(path), ".")) {
				paths = append(paths, path)
			}
		}
	}
	if len(paths) == 0 {
		a.toast("请先选择至少一个视频")
		return
	}
	a.beginTransfer(fileOperation{op: opCopy, paths: paths, dest: dest})
}

// ensureMediaInfo loads EXIF dimensions for ratio math.
func (a *appState) ensureMediaInfo(item *mediaItem) {
	if item.InfoLoaded && item.Info.Width > 0 {
		return
	}
	index := a.LargeIndex
	if index < 0 || index >= len(a.Visible) {
		return
	}
	a.requestInfo(a.Visible[index])
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
	if path == "" || (a.Video != nil && a.Video.path == path) {
		return
	}
	a.openPlayback(path, 0, a.Profiles.View.AutoPlaySelected)
}

// seekVideo restarts the real ffmpeg stream at seconds so the control bar
// moves through the source media instead of a synthetic timeline.
func (a *appState) seekVideo(seconds float64) {
	if a.Video == nil || a.Video.path == "" {
		return
	}
	a.Video.kernel.Seek(seconds)
	a.Video.Still = nil
}

// restartVideo rebuilds the decoder at the zoom-sized edge.
func (a *appState) restartVideo(seconds float64, playing bool) {
	if a.Video == nil || a.Video.path == "" {
		return
	}
	if a.videoPlaying() != playing && a.Video.kernel != nil {
		a.Video.kernel.SetPlaying(playing)
	}
	a.seekVideo(seconds)
}

// settleVideoEdge waits until a zoom gesture stops before restarting the
// decoder. The current frame stays on screen, scaled by the view transform.
func (a *appState) settleVideoEdge(c *ui.Context) {
	if a.Video == nil || a.Video.kernel == nil || !a.videoPlaying() {
		a.videoEdgeDue = time.Time{}
		return
	}
	backend, ok := a.Video.kernel.backend.(*ffmpegBackend)
	if !ok {
		a.videoEdgeDue = time.Time{}
		return
	}
	backend.mu.Lock()
	decodedEdge := backend.edge
	backend.mu.Unlock()
	if !shouldRestartVideoEdge(decodedEdge, a.Zoom, a.videoEdgeDue, time.Now()) {
		if decodedEdge == playbackEdge(a.Zoom) {
			a.videoEdgeDue = time.Time{}
		} else if !a.videoEdgeDue.IsZero() {
			c.After(time.Until(a.videoEdgeDue))
		}
		return
	}
	a.videoEdgeDue = time.Time{}
	a.restartVideo(a.playhead(), true)
}

func (a *appState) dropLargeBitmaps(keep string) {
	for i := range a.Items {
		if a.Items[i].Path == keep {
			continue
		}
		a.Items[i].Large = nil
		a.Items[i].LargeLoading = false
	}
}

// ensureLarge decodes the original beside the small thumbnail so 1:1 zoom
// is sharp. The grid keeps using the thumbnail. edge is the decode edge
// (see largeStageEdge); the sharp resample covers beyond it.
func (a *appState) ensureLarge(index int) {
	if a.isShelved() || a.Window == nil || index < 0 || index >= len(a.Visible) {
		return
	}
	itemIndex := a.Visible[index]
	if itemIndex < 0 || itemIndex >= len(a.Items) {
		return
	}
	item := &a.Items[itemIndex]
	if item.IsVideo || item.Large != nil || item.LargeLoading {
		return
	}
	item.LargeLoading = true
	path := item.Path
	root := a.Root
	edge := largeStageEdge(max(a.largeSize()))
	safeGo("loadLarge", func() {
		bmp, err := decodeLargeBitmap(path, edge)
		if a.Window == nil {
			return
		}
		a.update(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.Root != root || itemIndex >= len(a.Items) || a.Items[itemIndex].Path != path {
				return
			}
			current := &a.Items[itemIndex]
			current.LargeLoading = false
			if err != nil || !a.InLargeView || a.shelved {
				return
			}
			if a.LargeIndex < 0 || a.LargeIndex >= len(a.Visible) || a.Visible[a.LargeIndex] != itemIndex {
				return
			}
			current.Large = bmp
		})
	})
}

func (a *appState) openLargeImage(index int) {
	if index < 0 || index >= len(a.Visible) {
		return
	}
	if a.Items[a.Visible[index]].IsDir {
		a.openFolder(a.Items[a.Visible[index]].Path)
		return
	}
	a.clearSharp()
	a.heroBeginOpen(a.Items[a.Visible[index]].Path, !a.InLargeView)
	a.InLargeView = true
	a.LargeIndex = index
	a.Zoom, a.Rotation, a.Mirror = 1, 0, false
	a.PanX, a.PanY = 0, 0
	a.HUD, a.HUDUntil = "", time.Time{}
	item := &a.Items[a.Visible[index]]
	// Keep the libmpv/FFmpeg backend across switches: park on image, reuse
	// on the next video. Full teardown only happens when leaving large view.
	if item.IsVideo {
		// ensureVideo / openPlayback will loadfile-replace on the live core.
	} else {
		a.parkVideo()
	}
	a.setTitle(fmt.Sprintf("%s (%d/%d)", item.Name, index+1, len(a.Visible)))
	if !item.Decoded && !item.Failed && !item.Loading {
		a.loadThumb(a.Root, a.Visible[index])
	}
	a.dropLargeBitmaps(item.Path)
	if !item.IsVideo {
		a.ensureLarge(index)
	}
	// The neighbors are the likeliest next opens: decode them while the
	// current picture shows, so stepping through a folder feels instant.
	a.ensureNeighborLarge(index - 1)
	a.ensureNeighborLarge(index + 1)
}

// ensureNeighborLarge prefetches one adjacent picture's large bitmap
// without disturbing the visible item. Skips videos and anything already
// loaded or loading; a keep-flag protects prefetches from the drop when
// the view moves.
func (a *appState) ensureNeighborLarge(index int) {
	if a.isShelved() || index < 0 || index >= len(a.Visible) {
		return
	}
	itemIndex := a.Visible[index]
	if itemIndex < 0 || itemIndex >= len(a.Items) {
		return
	}
	item := &a.Items[itemIndex]
	if item.IsVideo || item.IsDir || item.Large != nil || item.LargeLoading {
		return
	}
	item.LargeLoading = true
	path := item.Path
	root := a.Root
	edge := largeStageEdge(max(a.largeSize()))
	safeGo("prefetchLarge", func() {
		bmp, err := decodeLargeBitmap(path, edge)
		if a.Window == nil {
			return
		}
		a.update(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.Root != root || itemIndex >= len(a.Items) || a.Items[itemIndex].Path != path {
				return
			}
			current := &a.Items[itemIndex]
			current.LargeLoading = false
			// Only keep the pixels while the prefetch is still adjacent;
			// dropLargeBitmaps clears the rest when the view moves.
			if err != nil || !a.InLargeView || a.shelved {
				return
			}
			if !a.largeNeighborWanted(itemIndex) {
				return
			}
			current.Large = bmp
		})
	})
}

// largeNeighborWanted reports whether itemIndex neighbors the item the
// large view currently shows (or is that item).
func (a *appState) largeNeighborWanted(itemIndex int) bool {
	if a.LargeIndex < 0 || a.LargeIndex >= len(a.Visible) {
		return false
	}
	current := a.Visible[a.LargeIndex]
	if current == itemIndex {
		return true
	}
	for _, d := range []int{-1, 1} {
		n := a.LargeIndex + d
		if n >= 0 && n < len(a.Visible) && a.Visible[n] == itemIndex {
			return true
		}
	}
	return false
}

func (a *appState) closeLargeImage() {
	a.AutoPlay = false
	a.autoTick = time.Time{}
	a.closeEditor()
	a.clearSharp()
	if item := a.currentItem(); a.InLargeView && item != nil {
		a.heroBeginClose(item.Path)
	}
	a.InLargeView = false
	a.LargeIndex = -1
	a.dropLargeBitmaps("")
	a.closeVideo()
	if a.Root != "" {
		a.setTitle(fmt.Sprintf("%s — %s", appName, pathName(a.Root)))
	}
}

func (a *appState) nextImage() {
	for i := a.LargeIndex + 1; i < len(a.Visible); i++ {
		if !a.Items[a.Visible[i]].IsDir {
			a.openLargeImage(i)
			return
		}
	}
}

func (a *appState) prevImage() {
	for i := a.LargeIndex - 1; i >= 0; i-- {
		if !a.Items[a.Visible[i]].IsDir {
			a.openLargeImage(i)
			return
		}
	}
}

func (a *appState) mediaCount() int {
	n := 0
	for _, index := range a.Visible {
		if index >= 0 && index < len(a.Items) && isMediaFile(a.Items[index].Ext) {
			n++
		}
	}
	return n
}

func (a *appState) modalOpen() bool {
	return a.ZipAsk || a.CropAsk || a.TrimAsk || a.InfoAsk || a.Editor.Open || a.AutoConfig != ""
}

func (a *appState) applyFilter() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyFilterLocked()
}

func (a *appState) detailView(c *ui.Context) {
	t := c.Theme()
	state := a.mediaListState()
	if a.takeReveal() {
		a.ListState.ScrollIntoView(a.Chosen)
	}
	list := ui.List(c, state.List, len(a.Visible), func(i int) {
		item := a.visibleItem(i)
		if item == nil {
			return
		}
		row := ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center)
		if a.Chosen == i || a.Selected.Has(item.Path) {
			row.Background(t.SurfacePressed)
		}
		row.Children(func() {
			icon := imageIcon
			if item.IsDir {
				icon = dirIcon
			} else if isVideoFile(item.Ext) {
				icon = videoIcon
			}
			ui.Icon(c, icon).FontSize(16)
			ui.Text(c, item.Name).Grow(1).FontSize(12).SingleLine()
			ui.Textf(c, "%.1f MB", float64(item.Size)/(1024*1024)).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, time.Unix(item.ModTime, 0).Format("2006-01-02 15:04:05")).FontSize(11).TextColor(t.TextMuted)
		})
		a.cardPointer(row, i)
		a.fileDragSource(row, item.Path)
		row.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
		if a.isCut(item.Path) {
			row.Opacity(0.4)
		}
	})
	list.Grow(1).TrackScroll(&a.browseScroll)
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
		Filters:     a.Profiles.View,
	}
}

// profileDirty reports whether the live view has drifted from what slot i
// stored, which the settings window marks with an asterisk.
func (a *appState) profileDirty(i int) bool {
	if i < 0 || i >= len(a.Profiles.Profiles) {
		return false
	}
	p := a.Profiles.Profiles[i]
	return p.Root != a.Root || p.LayoutMode != a.LayoutMode || p.Thumb != a.Thumb ||
		p.Sort != a.Sort || p.Sidebar != a.SidebarShown || p.Recursive != a.Recursive ||
		p.Filters != a.Profiles.View
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
	if p.Thumb > 0 {
		a.Thumb = p.Thumb
	}
	a.Sort = p.Sort
	a.Recursive = p.Recursive
	if p.Filters != (viewPreferences{}) {
		a.Profiles.View = p.Filters
	}
	a.MediaUnitRoot = ""
	savePrefs(a.Profiles)
	if p.Root != "" && isDir(p.Root) {
		a.openFolder(p.Root)
	}
	a.refreshView()
}

func (a *appState) saveProfile(i int) {
	if i < 0 || i >= len(a.Profiles.Profiles) {
		return
	}
	a.Profiles.Profiles[i] = a.currentProfile()
	a.Profiles.Active = i
	savePrefs(a.Profiles)
}

// carryItemState keeps what a rescan of the same folder already knows:
// unchanged files (same path, size and time) keep their thumbnail, info
// and tags, so a watcher event only loads what was added or changed and
// the grid does not flash every card back to a spinner.
func carryItemState(old, fresh []mediaItem) {
	if len(old) == 0 {
		return
	}
	byPath := make(map[string]*mediaItem, len(old))
	for i := range old {
		byPath[old[i].Path] = &old[i]
	}
	for i := range fresh {
		o := byPath[fresh[i].Path]
		if o == nil || o.Size != fresh[i].Size || o.ModTime != fresh[i].ModTime || o.Loading {
			continue
		}
		f := &fresh[i]
		f.Bitmap, f.Decoded, f.Failed, f.ThumbEdge = o.Bitmap, o.Decoded, o.Failed, o.ThumbEdge
		f.Info, f.InfoLoaded, f.Tags = o.Info, o.InfoLoaded, o.Tags
	}
}
