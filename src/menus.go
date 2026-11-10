package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Context menu registry.
//
// Every right-click menu of the browser is built from menuEntries, one list.
// An entry says where it appears (surface), under which submenu, when it is
// shown and when it is disabled, as functions of the menuScope — what the
// menu was opened over: nothing (empty space), one item, several, folders,
// videos, pictures, archives. Nothing else writes menu items, so a command
// added here shows up on every surface that lists it and a new selection
// kind needs one predicate, not a copy of the menu.
//
// Surfaces:
//
//	surfItem   a card or row of the grid/list, or a sidebar folder
//	surfLarge  the large view's stage (the current picture or video)
//	surfBlank  empty space of a folder
//
// Commands read the selection through a.operationPaths (selectedPaths with
// nested paths collapsed), the same list the menu's scope was made from, so
// what a menu offers is what the command then acts on — a band selection of
// files from several subfolders of a recursive listing included.

type menuSurface uint8

const (
	surfItem menuSurface = 1 << iota
	surfLarge
	surfBlank

	surfFiles = surfItem | surfLarge // the item commands
)

// menuScope is what a menu was opened over.
type menuScope struct {
	Surface menuSurface
	// Paths are the targets, nested paths collapsed. Empty for empty space.
	Paths []string
	// Counts of Paths by kind. Others are files that are none of the rest.
	Dirs, Videos, Images, Archives int
	// Sidebar marks a menu opened on a sidebar folder (a.menuPath).
	Sidebar bool
	// Folder is where Paste and the new-item commands act; Root is the open
	// folder.
	Folder string
	Root   string
	// Video is the current clip of the large view.
	Video *mediaItem
	// Favorite: the single target folder (or, for empty space, the open
	// folder) is already a favorite.
	Favorite bool
	// ZipPassword: a default ZIP password is configured.
	ZipPassword bool
	// Listed is how many items the folder lists.
	Listed int
}

func (s *menuScope) N() int         { return len(s.Paths) }
func (s *menuScope) None() bool     { return len(s.Paths) == 0 }
func (s *menuScope) One() bool      { return len(s.Paths) == 1 }
func (s *menuScope) Many() bool     { return len(s.Paths) > 1 }
func (s *menuScope) OnlyDirs() bool { return s.N() > 0 && s.Dirs == s.N() }
func (s *menuScope) Files() int     { return s.N() - s.Dirs }

// AllVideos is true when every target is a video.
func (s *menuScope) AllVideos() bool { return s.N() > 0 && s.Videos == s.N() }

// menuEntry is one line of a menu. Exactly one of Sep, Custom or a label
// (Text or Title) says what it draws.
type menuEntry struct {
	ID       string
	Surfaces menuSurface
	Group    string // submenu it lives in; "" is the top level
	Text     string
	Title    func(s *menuScope) string // label that depends on the scope
	Sep      bool
	Show     func(s *menuScope) bool // nil: always
	Off      func(s *menuScope) bool // nil: enabled
	Check    func(s *menuScope) bool
	Run      func(a *appState, s *menuScope)
	// Custom draws its own items (the system's dynamic submenus).
	Custom func(a *appState, m *ui.Menu, s *menuScope)
}

func (e *menuEntry) label(s *menuScope) string {
	if e.Title != nil {
		return e.Title(s)
	}
	return e.Text
}

func sep(surfaces menuSurface, group string) menuEntry {
	return menuEntry{Surfaces: surfaces, Group: group, Sep: true}
}

func menuItem(id string, surfaces menuSurface, group, text string, run func(a *appState, s *menuScope)) menuEntry {
	return menuEntry{ID: id, Surfaces: surfaces, Group: group, Text: text, Run: run}
}

func (e menuEntry) show(f func(s *menuScope) bool) menuEntry { e.Show = f; return e }
func (e menuEntry) off(f func(s *menuScope) bool) menuEntry  { e.Off = f; return e }
func (e menuEntry) title(f func(s *menuScope) string) menuEntry {
	e.Title = f
	return e
}

var (
	needsTargets = func(s *menuScope) bool { return s.None() }
	noFolder     = func(s *menuScope) bool { return s.Folder == "" }
)

// menuEntries is the registry, in menu order. It is a function so entries
// close over nothing at package initialisation.
func menuEntries() []menuEntry {
	const (
		copyMove = "复制与移动"
		rotate   = "旋转"
		video    = "视频"
		zip      = "压缩"
	)
	pasteOff := func(s *menuScope) bool { return s.Folder == "" }
	return []menuEntry{
		// ---- Large view only -------------------------------------------
		{ID: "zoom-fit", Surfaces: surfLarge, Group: "缩放", Text: "适应窗口", Run: func(a *appState, _ *menuScope) { a.resetZoom() }},
		{ID: "zoom-actual", Surfaces: surfLarge, Group: "缩放", Text: "实际大小", Run: func(a *appState, _ *menuScope) { a.zoom100() }},
		menuItem("save-video", surfLarge, "", "另存视频…", func(a *appState, s *menuScope) { a.chooseVideoSave(s.Video) }).
			show(func(s *menuScope) bool { return s.Video != nil }),
		sep(surfLarge, ""),

		// ---- Items ----------------------------------------------------
		menuItem("count", surfItem, "", "", nil).
			title(func(s *menuScope) string { return fmt.Sprintf("已选 %d 项", s.N()) }).
			show(func(s *menuScope) bool { return s.Many() }).
			off(func(*menuScope) bool { return true }),
		menuItem("open", surfItem, "", "打开", func(a *appState, s *menuScope) {
			if s.Sidebar && isDir(a.menuPath) {
				a.openFolder(a.menuPath)
			} else {
				a.openItem(a.Chosen)
			}
		}).off(func(s *menuScope) bool { return !s.One() }),
		menuItem("open-tab", surfItem, "", "在新标签页中打开", func(a *appState, s *menuScope) { a.openInNewTab(s.Paths[0]) }).
			show(func(s *menuScope) bool { return s.One() && s.Dirs == 1 }),
		menuItem("open-player", surfItem, "", "用系统播放器打开", func(a *appState, s *menuScope) { _ = openWithDefault(s.Paths[0]) }).
			show(func(s *menuScope) bool { return s.One() && s.Videos == 1 }),
		{ID: "open-with", Surfaces: surfFiles, Custom: func(a *appState, m *ui.Menu, s *menuScope) { a.openWithMenu(m, s.Paths) },
			Show: func(s *menuScope) bool { return !s.None() && !s.OnlyDirs() }},
		menuItem("reveal", surfFiles, "", revealLabel, func(a *appState, _ *menuScope) { a.revealInFinder() }).off(needsTargets),
		menuItem("info", surfFiles, "", "文件简介", func(a *appState, _ *menuScope) { a.showInfo() }).off(needsTargets),
		sep(surfFiles, ""),
		menuItem("copy", surfFiles, "", "复制", func(a *appState, _ *menuScope) { a.doCopy() }).off(needsTargets),
		menuItem("paste", surfFiles, "", "粘贴", func(a *appState, _ *menuScope) { a.doPaste(false) }).off(pasteOff),
		menuItem("rename-batch", surfFiles, "", "批量重命名…", func(a *appState, _ *menuScope) { a.startBatchRename() }).
			show(func(s *menuScope) bool { return s.Many() }),
		menuItem("rename", surfFiles, "", "重命名…", func(a *appState, _ *menuScope) { a.startRename() }).
			show(func(s *menuScope) bool { return !s.Many() }).off(needsTargets),
		menuItem("rename-folder", surfFiles, "", "按文件夹名重命名", func(a *appState, _ *menuScope) { a.renameByFolderName() }).
			show(func(s *menuScope) bool { return s.Files() > 0 }),
		sep(surfFiles, ""),

		menuItem("cut", surfFiles, copyMove, "剪切", func(a *appState, _ *menuScope) { a.doCut() }).off(needsTargets),
		menuItem("copy-path", surfFiles, copyMove, "复制路径", func(a *appState, _ *menuScope) { a.copyPath() }).off(needsTargets),
		menuItem("duplicate", surfFiles, copyMove, "制作副本", func(a *appState, _ *menuScope) { a.duplicateSelection() }).off(needsTargets),
		sep(surfFiles, copyMove),
		menuItem("copy-to", surfFiles, copyMove, "复制到…", func(a *appState, _ *menuScope) { a.doCopyTo() }).off(needsTargets),
		menuItem("move-to", surfFiles, copyMove, "移动到…", func(a *appState, _ *menuScope) { a.doMove() }).off(needsTargets),
		menuItem("copy-target", surfFiles, copyMove, "复制到目标", func(a *appState, _ *menuScope) { a.copyToConfigured() }).off(needsTargets),
		menuItem("copy-downloads", surfFiles, copyMove, "复制到下载", func(a *appState, _ *menuScope) { a.transferToDownloads(opCopy) }).off(needsTargets),
		menuItem("move-downloads", surfFiles, copyMove, "移动到下载", func(a *appState, _ *menuScope) { a.transferToDownloads(opMove) }).off(needsTargets),
		sep(surfFiles, copyMove),
		menuItem("new-folder-with", surfFiles, copyMove, "用所选项目新建文件夹", func(a *appState, _ *menuScope) { a.newFolderWithSelection() }).off(needsTargets),
		menuItem("collect", surfFiles, copyMove, "提取子文件夹文件并归集", func(a *appState, _ *menuScope) { a.collectSelection() }).
			show(func(s *menuScope) bool { return s.Many() && s.OnlyDirs() }),

		// Pictures only: rotating text files and clips is not offered.
		menuItem("rot-right", surfFiles, rotate, "向右旋转 90°", func(a *appState, _ *menuScope) { a.rotateSelection(90) }).
			show(func(s *menuScope) bool { return s.Images > 0 && s.Videos == 0 }),
		menuItem("rot-left", surfFiles, rotate, "向左旋转 90°", func(a *appState, _ *menuScope) { a.rotateSelection(270) }).
			show(func(s *menuScope) bool { return s.Images > 0 && s.Videos == 0 }),
		menuItem("rot-180", surfFiles, rotate, "旋转 180°", func(a *appState, _ *menuScope) { a.rotateSelection(180) }).
			show(func(s *menuScope) bool { return s.Images > 0 && s.Videos == 0 }),

		menuItem("video-frame", surfFiles, video, "截存当前画面", func(a *appState, _ *menuScope) { a.captureCurrentFrame() }).
			show(func(s *menuScope) bool { return s.AllVideos() && s.Video != nil }),
		menuItem("video-trim", surfFiles, video, "无损裁剪片段…", func(a *appState, _ *menuScope) { a.promptTrim() }).
			show(func(s *menuScope) bool { return s.AllVideos() && s.Video != nil }),
		menuItem("video-crop", surfFiles, video, "裁剪视频尺寸…", func(a *appState, s *menuScope) { a.promptCrop(s.Paths) }).
			show(func(s *menuScope) bool { return s.AllVideos() }),

		menuItem("extract", surfFiles, zip, "解压到当前目录", func(a *appState, _ *menuScope) { a.extractSelection(false) }).
			show(func(s *menuScope) bool { return s.Archives > 0 }),
		menuItem("extract-drop", surfFiles, zip, "解压并删除压缩包", func(a *appState, _ *menuScope) { a.extractSelection(true) }).
			show(func(s *menuScope) bool { return s.Archives > 0 }),
		sep(surfFiles, zip).show(func(s *menuScope) bool { return s.Archives > 0 }),
		menuItem("zip", surfFiles, zip, "压缩为 ZIP", func(a *appState, _ *menuScope) { a.quickCompress() }),
		menuItem("zip-encrypt", surfFiles, zip, "加密压缩…", func(a *appState, _ *menuScope) { a.encryptCompress(false) }),
		menuItem("zip-default", surfFiles, zip, "使用默认密码加密压缩", func(a *appState, _ *menuScope) { a.compressWithPassword(false) }).
			show(func(s *menuScope) bool { return s.ZipPassword }),
		menuItem("zip-drop", surfFiles, zip, "压缩并删除源文件", func(a *appState, _ *menuScope) { a.compressPlain(true) }),

		{ID: "tags", Surfaces: surfFiles, Custom: func(a *appState, m *ui.Menu, s *menuScope) { a.tagMenu(m, s.Paths) },
			Show: func(s *menuScope) bool { return tagsSupported && !s.None() }},
		{ID: "share", Surfaces: surfFiles, Custom: func(a *appState, m *ui.Menu, s *menuScope) { a.shareMenu(m, s.Paths) },
			Show: func(s *menuScope) bool { return !s.None() }},
		menuItem("favorite", surfFiles, "", "", func(a *appState, s *menuScope) { a.toggleFavorite(s.Paths[0]) }).
			title(func(s *menuScope) string {
				if s.Favorite {
					return "取消收藏"
				}
				return "添加到收藏"
			}).show(func(s *menuScope) bool { return s.One() && s.Dirs == 1 }),
		menuItem("open-parent", surfFiles, "", openParentLabel, func(a *appState, _ *menuScope) { a.showOriginalFolder() }).
			show(func(s *menuScope) bool { return s.One() && filepath.Dir(s.Paths[0]) != s.Root }),
		sep(surfFiles, ""),
		menuItem("trash", surfFiles, "", "移到废纸篓", func(a *appState, _ *menuScope) { a.doTrash() }).off(needsTargets),

		// ---- Empty space ---------------------------------------------
		menuItem("blank-paste", surfBlank, "", "粘贴", func(a *appState, _ *menuScope) { a.doPaste(false) }).
			off(func(s *menuScope) bool { return s.Folder == "" }),
		menuItem("blank-paste-move", surfBlank, "", "移动到此", func(a *appState, _ *menuScope) { a.doPaste(true) }).
			off(func(s *menuScope) bool { return s.Folder == "" }),
		sep(surfBlank, ""),
		menuItem("new-folder", surfBlank, "", "新建文件夹", func(a *appState, _ *menuScope) { a.newFolder() }).off(noFolder),
		menuItem("new-text", surfBlank, "", "新建文本文件", func(a *appState, _ *menuScope) { a.newTextFile() }).off(noFolder),
		sep(surfBlank, ""),
		menuItem("select-all", surfBlank, "", "全选", func(a *appState, _ *menuScope) { a.selectAllVisible() }).
			show(func(s *menuScope) bool { return s.Listed > 0 }),
		menuItem("refresh", surfBlank, "", "刷新", func(a *appState, _ *menuScope) { a.refresh() }).off(noFolder),
		menuItem("terminal", surfBlank, "", "在终端中打开", func(a *appState, _ *menuScope) { a.openInTerminal() }).off(noFolder),
		menuItem("fav-here", surfBlank, "", "", func(a *appState, s *menuScope) { a.toggleFavorite(s.Folder) }).
			title(func(s *menuScope) string {
				if s.Favorite {
					return "取消收藏当前目录"
				}
				return "收藏当前目录"
			}).off(noFolder),
	}
}

// entryVisible reports whether e shows on the scope's surface.
func entryVisible(e *menuEntry, s *menuScope) bool {
	if e.Surfaces&s.Surface == 0 {
		return false
	}
	return e.Show == nil || e.Show(s)
}

// menuModel is the visible part of the registry for a scope: top-level
// entries and submenus in order, separators collapsed so none leads, trails
// or doubles. Tests read it; the live menu is drawn from it.
type menuModel struct {
	Top []menuLine
}

type menuLine struct {
	Entry *menuEntry
	Group string // submenu title when this line is a submenu
	Sub   []menuLine
	Sep   bool
}

func (m menuModel) IDs() []string {
	var out []string
	var walk func(lines []menuLine)
	walk = func(lines []menuLine) {
		for _, l := range lines {
			if l.Entry != nil && l.Entry.ID != "" {
				out = append(out, l.Entry.ID)
			}
			walk(l.Sub)
		}
	}
	walk(m.Top)
	return out
}

func (m menuModel) Has(id string) bool {
	for _, got := range m.IDs() {
		if got == id {
			return true
		}
	}
	return false
}

// modelFor filters the registry through the scope.
func modelFor(entries []menuEntry, s *menuScope) menuModel {
	var lines []menuLine
	groups := map[string]int{} // submenu title -> index in lines
	for i := range entries {
		e := &entries[i]
		if !entryVisible(e, s) {
			continue
		}
		if e.Group == "" {
			lines = append(lines, menuLine{Entry: e, Sep: e.Sep})
			continue
		}
		at, ok := groups[e.Group]
		if !ok {
			lines = append(lines, menuLine{Group: e.Group})
			at = len(lines) - 1
			groups[e.Group] = at
		}
		lines[at].Sub = append(lines[at].Sub, menuLine{Entry: e, Sep: e.Sep})
	}
	for i := range lines {
		lines[i].Sub = tidySeparators(lines[i].Sub)
	}
	// A submenu with nothing but separators is no submenu.
	kept := lines[:0]
	for _, l := range lines {
		if l.Group != "" && len(l.Sub) == 0 {
			continue
		}
		kept = append(kept, l)
	}
	return menuModel{Top: tidySeparators(kept)}
}

// tidySeparators drops leading, trailing and repeated separators.
func tidySeparators(lines []menuLine) []menuLine {
	var out []menuLine
	for _, l := range lines {
		if l.Sep {
			if len(out) == 0 || out[len(out)-1].Sep {
				continue
			}
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1].Sep {
		out = out[:len(out)-1]
	}
	return out
}

// menuScopeFor describes what a menu opened on surface acts on.
func (a *appState) menuScopeFor(surface menuSurface) *menuScope {
	s := &menuScope{Surface: surface, Folder: a.Root, Root: a.Root, Listed: len(a.Visible)}
	_, s.ZipPassword = configuredZipPassword(a.Profiles)
	if surface == surfBlank {
		s.Favorite = favoriteContains(a.Profiles.Favorites, a.Root)
		return s
	}
	if a.menuPath != "" {
		s.Sidebar = true
		if isDir(a.menuPath) {
			s.Folder = a.menuPath
		}
	}
	s.Paths = a.operationPaths()
	a.classifyPaths(s)
	if len(s.Paths) == 1 {
		s.Favorite = favoriteContains(a.Profiles.Favorites, s.Paths[0])
	}
	if a.InLargeView {
		if item := a.currentItem(); item != nil && item.IsVideo {
			s.Video = item
		}
	}
	return s
}

// classifyPaths counts the scope's targets by kind from the listing, which
// already knows folders from files, and from the name for paths it does not
// list (a sidebar folder outside the current listing).
func (a *appState) classifyPaths(s *menuScope) {
	if len(s.Paths) == 0 {
		return
	}
	want := make(map[string]struct{}, len(s.Paths))
	for _, p := range s.Paths {
		want[p] = struct{}{}
	}
	known := make(map[string]bool, len(s.Paths))
	count := func(path string, dir bool) {
		known[path] = true
		name := filepath.Base(path)
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		switch {
		case dir:
			s.Dirs++
		case isVideoFile(ext):
			s.Videos++
		case isImageFile(ext):
			s.Images++
		case isArchiveName(name):
			s.Archives++
		}
	}
	for i := range a.Items {
		it := &a.Items[i]
		if _, ok := want[it.Path]; ok && !known[it.Path] {
			count(it.Path, it.IsDir)
		}
	}
	for _, p := range s.Paths {
		if !known[p] {
			count(p, isDir(p))
		}
	}
}

// buildMenu draws the registry's menu for surface into m.
func (a *appState) buildMenu(m *ui.Menu, surface menuSurface) {
	a.buildMenuFor(m, surface, nil)
}

// buildMenuFor is buildMenu with the clip a large-view menu is about, when
// the caller knows it (the stage hands over the item it shows).
func (a *appState) buildMenuFor(m *ui.Menu, surface menuSurface, video *mediaItem) {
	s := a.menuScopeFor(surface)
	if video != nil && video.IsVideo {
		s.Video = video
	}
	a.drawMenu(m, s, modelFor(menuEntries(), s).Top)
}

func (a *appState) drawMenu(m *ui.Menu, s *menuScope, lines []menuLine) {
	for _, l := range lines {
		e := l.Entry
		switch {
		case l.Sep:
			m.Separator()
		case l.Group != "":
			sub := l.Sub
			m.Submenu(l.Group, func(sm *ui.Menu) { a.drawMenu(sm, s, sub) })
		case e.Custom != nil:
			e.Custom(a, m, s)
		default:
			item := m.Item(e.label(s))
			if e.Off != nil {
				item = item.Disabled(e.Off(s))
			}
			if e.Check != nil {
				item = item.Checked(e.Check(s))
			}
			if item.Chosen() && e.Run != nil {
				e.Run(a, s)
			}
		}
	}
}

// selectAllVisible selects every listed item.
func (a *appState) selectAllVisible() {
	a.Selected.Clear()
	for _, index := range a.Visible {
		if index >= 0 && index < len(a.Items) {
			a.Selected.Add(a.Items[index].Path)
		}
	}
	if a.Chosen < 0 && len(a.Visible) > 0 {
		a.Chosen = 0
	}
}
