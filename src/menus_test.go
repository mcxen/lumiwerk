package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// menuApp lists a mixed folder (picture, clip, archive, folder, text) so the
// scope of a menu can be told by what is selected.
func menuApp(t *testing.T) *appState {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := "/probe"
	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: 128, Root: root, LayoutMode: 0}
	a.Profiles = prefsFile{View: defaultViewPreferences(), ViewSet: true}
	a.Items = []mediaItem{
		{Path: root + "/a.jpg", Name: "a.jpg", Ext: "jpg", InfoLoaded: true},
		{Path: root + "/b.png", Name: "b.png", Ext: "png", InfoLoaded: true},
		{Path: root + "/c.mp4", Name: "c.mp4", Ext: "mp4", IsVideo: true, InfoLoaded: true},
		{Path: root + "/d.mov", Name: "d.mov", Ext: "mov", IsVideo: true, InfoLoaded: true},
		{Path: root + "/e.zip", Name: "e.zip", Ext: "zip", InfoLoaded: true},
		{Path: root + "/f", Name: "f", IsDir: true, InfoLoaded: true},
		{Path: root + "/g", Name: "g", IsDir: true, InfoLoaded: true},
		{Path: root + "/h.txt", Name: "h.txt", Ext: "txt", InfoLoaded: true},
	}
	for i := range a.Items {
		a.Visible = append(a.Visible, i)
	}
	return a
}

func (a *appState) pick(names ...string) {
	a.Selected.Clear()
	for i, it := range a.Items {
		if slices.Contains(names, it.Name) {
			a.Selected.Add(it.Path)
			a.Chosen = i
		}
	}
}

func idsFor(a *appState, surface menuSurface) menuModel {
	return modelFor(menuEntries(), a.menuScopeFor(surface))
}

func TestMenuRegistryIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range menuEntries() {
		if e.ID == "" {
			continue
		}
		if seen[e.ID] {
			t.Fatalf("menu entry %q registered twice", e.ID)
		}
		seen[e.ID] = true
		if !e.Sep && e.Custom == nil && e.Run == nil && e.ID != "count" {
			t.Fatalf("menu entry %q does nothing", e.ID)
		}
	}
}

func TestMenuItemsFollowTheSelectionKind(t *testing.T) {
	type want struct {
		has, not []string
	}
	cases := []struct {
		name   string
		pick   []string
		expect want
	}{
		{"one picture", []string{"a.jpg"}, want{
			has: []string{"open", "reveal", "copy", "rename", "rename-folder", "rot-right", "zip", "trash", "open-with", "share"},
			not: []string{"count", "rename-batch", "collect", "video-crop", "extract", "open-tab", "favorite", "open-player"}}},
		{"several pictures", []string{"a.jpg", "b.png"}, want{
			has: []string{"count", "rename-batch", "rot-right", "zip", "trash", "copy-to", "move-to", "new-folder-with"},
			not: []string{"rename", "collect", "video-crop"}}},
		{"one video", []string{"c.mp4"}, want{
			has: []string{"open-player", "video-crop", "zip"},
			not: []string{"rot-right", "rot-180", "extract", "video-frame", "video-trim"}}},
		{"two videos", []string{"c.mp4", "d.mov"}, want{
			has: []string{"count", "video-crop", "rename-batch"},
			not: []string{"rot-right", "open-player"}}},
		{"picture and video", []string{"a.jpg", "c.mp4"}, want{
			has: []string{"count", "zip", "rename-batch"},
			not: []string{"rot-right", "video-crop"}}},
		{"archive", []string{"e.zip"}, want{
			has: []string{"extract", "extract-drop", "zip"},
			not: []string{"rot-right", "video-crop"}}},
		{"one folder", []string{"f"}, want{
			has: []string{"open", "open-tab", "favorite", "zip", "trash", "rename"},
			not: []string{"rot-right", "rename-folder", "open-with", "collect", "count"}}},
		{"two folders", []string{"f", "g"}, want{
			has: []string{"count", "collect", "rename-batch"},
			not: []string{"open-tab", "favorite", "rot-right", "rename-folder"}}},
		{"picture and folder", []string{"a.jpg", "f"}, want{
			has: []string{"count", "rot-right", "rename-folder", "open-with"},
			not: []string{"collect", "open-tab"}}},
		{"text file", []string{"h.txt"}, want{
			has: []string{"rename", "zip", "copy"},
			not: []string{"rot-right", "video-crop", "extract"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := menuApp(t)
			a.pick(tc.pick...)
			m := idsFor(a, surfItem)
			for _, id := range tc.expect.has {
				if !m.Has(id) {
					t.Errorf("%v: %q missing from %v", tc.pick, id, m.IDs())
				}
			}
			for _, id := range tc.expect.not {
				if m.Has(id) {
					t.Errorf("%v: %q should not show: %v", tc.pick, id, m.IDs())
				}
			}
		})
	}
}

func TestMenuEmptySpaceShowsFolderCommandsOnly(t *testing.T) {
	a := menuApp(t)
	a.pick("a.jpg", "b.png") // a selection does not turn empty space into an item menu
	m := idsFor(a, surfBlank)
	for _, id := range []string{"blank-paste", "blank-paste-move", "new-folder", "new-text", "select-all", "refresh", "terminal", "fav-here"} {
		if !m.Has(id) {
			t.Errorf("empty-space menu lacks %q: %v", id, m.IDs())
		}
	}
	for _, id := range []string{"copy", "trash", "rename", "zip", "count", "open"} {
		if m.Has(id) {
			t.Errorf("empty-space menu has item command %q", id)
		}
	}
	// No listing, no "select all".
	b := menuApp(t)
	b.Visible = nil
	if idsFor(b, surfBlank).Has("select-all") {
		t.Error("select all offered over an empty folder")
	}
}

func TestMenuLargeViewHasZoomAndSaveForVideos(t *testing.T) {
	a := menuApp(t)
	a.pick("c.mp4")
	a.InLargeView = true
	a.LargeIndex = 2
	m := idsFor(a, surfLarge)
	for _, id := range []string{"zoom-fit", "zoom-actual", "save-video", "video-frame", "video-trim", "video-crop", "trash"} {
		if !m.Has(id) {
			t.Errorf("large video menu lacks %q: %v", id, m.IDs())
		}
	}
	if m.Has("open") || m.Has("count") {
		t.Errorf("large view menu shows %v", m.IDs())
	}
	a.LargeIndex = 0
	a.pick("a.jpg")
	m = idsFor(a, surfLarge)
	if m.Has("save-video") || !m.Has("rot-right") {
		t.Errorf("large picture menu: %v", m.IDs())
	}
}

func TestMenuNeverLeadsTrailsOrDoublesSeparators(t *testing.T) {
	a := menuApp(t)
	picks := [][]string{nil, {"a.jpg"}, {"a.jpg", "b.png"}, {"c.mp4"}, {"e.zip"}, {"f"}, {"f", "g"}, {"h.txt"}, {"a.jpg", "c.mp4", "f"}}
	for _, surface := range []menuSurface{surfItem, surfLarge, surfBlank} {
		for _, p := range picks {
			a.pick(p...)
			var check func(lines []menuLine, where string)
			check = func(lines []menuLine, where string) {
				for i, l := range lines {
					if l.Sep && (i == 0 || i == len(lines)-1 || lines[i-1].Sep) {
						t.Fatalf("surface %d pick %v: stray separator at %d in %s", surface, p, i, where)
					}
					if l.Group != "" {
						if len(l.Sub) == 0 {
							t.Fatalf("surface %d pick %v: empty submenu %q", surface, p, l.Group)
						}
						check(l.Sub, l.Group)
					}
				}
			}
			check(modelFor(menuEntries(), a.menuScopeFor(surface)).Top, "top")
		}
	}
}

// The drawn menu is the registry's: what the Tester sees is what modelFor says.
func TestMenusDrawnFromTheRegistry(t *testing.T) {
	a := menuApp(t)
	a.pick("a.jpg", "b.png", "e.zip")
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().ContextMenu(func(m *ui.Menu) { a.fileMenu(m) })
	}, 600, 400)
	te.Frame()
	te.RightClickAt(300, 200)
	te.Frame()
	labels := te.Menu()
	for _, want := range []string{"已选 3 项", "批量重命名…", "复制与移动", "压缩", "移到废纸篓"} {
		if !slices.Contains(labels, want) {
			t.Errorf("menu lacks %q: %q", want, labels)
		}
	}
	if slices.Contains(labels, "重命名…") {
		t.Errorf("single rename offered for 3 items: %q", labels)
	}
	if err := te.ChooseMenuItem("压缩", "解压到当前目录"); err != nil {
		t.Errorf("extract entry for an archive in the selection: %v", err)
	}
}

func TestBlankSpaceRightClickOpensFolderMenuInEveryLayout(t *testing.T) {
	for _, layout := range []int{0, 1, 2, 3} {
		a := marqueeApp(layout, 3)
		te := marqueeTester(t, a)
		box := a.marqueeBox
		// The lower part of the view is empty with three items.
		te.RightClickAt(box.X+box.W/2, box.Y+box.H-40)
		te.Frame()
		labels := te.Menu()
		if !slices.Contains(labels, "新建文件夹") || !slices.Contains(labels, "全选") {
			t.Errorf("layout %d: empty-space menu is %q", layout, labels)
		}
		if slices.Contains(labels, "移到废纸篓") {
			t.Errorf("layout %d: empty-space menu offers item commands: %q", layout, labels)
		}
		te.CloseMenu()
	}
}

func TestRightClickKeepsABandSelectionAndReplacesAStrayOne(t *testing.T) {
	a := marqueeApp(0, 12)
	te := marqueeTester(t, a)
	x1, y1 := cardCenter(a, 1)
	dragBand(a, te, 0, a.marqueeBox.X+1, a.marqueeBox.Y+1, x1, y1)
	if got := selectedIndexes(a); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("band selected %v", got)
	}
	x0, y0 := cardCenter(a, 0)
	te.RightClickAt(x0, y0)
	te.Frame()
	if got := selectedIndexes(a); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("right-click inside the band selection changed it to %v", got)
	}
	if !slices.Contains(te.Menu(), "已选 2 项") || !slices.Contains(te.Menu(), "批量重命名…") {
		t.Fatalf("multi menu: %q", te.Menu())
	}
	te.CloseMenu()
	x5, y5 := cardCenter(a, 5)
	te.RightClickAt(x5, y5)
	te.Frame()
	if got := selectedIndexes(a); !slices.Equal(got, []int{5}) {
		t.Fatalf("right-click outside the selection left %v", got)
	}
	if slices.Contains(te.Menu(), "已选 2 项") {
		t.Fatalf("stale count: %q", te.Menu())
	}
}

// ---- Recursive listings ------------------------------------------------

// recursiveApp is a recursive listing over real files: a folder row, the
// files it expanded into, a deeper folder, and files of the top folder.
func recursiveApp(t *testing.T) (*appState, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	files := []string{"top1.jpg", "top2.jpg", "album/a1.jpg", "album/a2.jpg", "album/deep/d1.jpg", "other/o1.jpg"}
	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: 128, Root: root, LayoutMode: 0, Recursive: true}
	a.Profiles = prefsFile{View: defaultViewPreferences(), ViewSet: true}
	for _, dir := range []string{"album", "album/deep", "other"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		a.Items = append(a.Items, mediaItem{Path: filepath.Join(root, dir), Name: filepath.Base(dir), IsDir: true, InfoLoaded: true})
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.WriteFile(p, []byte("x"+f), 0o644); err != nil {
			t.Fatal(err)
		}
		a.Items = append(a.Items, mediaItem{Path: p, Name: filepath.Base(f), Ext: "jpg", InfoLoaded: true, Decoded: true})
	}
	for i := range a.Items {
		a.Visible = append(a.Visible, i)
	}
	return a, root
}

func (a *appState) selectPaths(root string, rel ...string) {
	a.Selected.Clear()
	for _, r := range rel {
		a.Selected.Add(filepath.Join(root, r))
	}
	a.Chosen = 0
}

func TestFolderAndItsExpandedFilesAreOneTarget(t *testing.T) {
	a, root := recursiveApp(t)
	a.selectPaths(root, "album", "album/a1.jpg", "album/deep/d1.jpg", "top1.jpg")
	got := a.operationPaths()
	want := []string{filepath.Join(root, "album"), filepath.Join(root, "top1.jpg")}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("targets %v, want %v", got, want)
	}
	// A file next to a folder with the same name prefix is not inside it.
	if err := os.WriteFile(filepath.Join(root, "album2.jpg"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := collapseNested([]string{filepath.Join(root, "album"), filepath.Join(root, "album2.jpg")}); len(got) != 2 {
		t.Fatalf("album2.jpg collapsed into album: %v", got)
	}
}

func TestExpandedFilesCountAsSelectedWithTheirFolder(t *testing.T) {
	a, root := recursiveApp(t)
	a.selectPaths(root, "album")
	for _, rel := range []string{"album/a1.jpg", "album/a2.jpg", "album/deep/d1.jpg"} {
		if !a.impliedPicked(filepath.Join(root, rel)) {
			t.Errorf("%s is inside the selected folder but not picked", rel)
		}
	}
	for _, rel := range []string{"top1.jpg", "other/o1.jpg"} {
		if a.impliedPicked(filepath.Join(root, rel)) {
			t.Errorf("%s is outside the selected folder but picked", rel)
		}
	}
	a.Recursive = false
	if a.impliedPicked(filepath.Join(root, "album/a1.jpg")) {
		t.Error("implied selection outside recursive browsing")
	}
}

// A band over cards of several subfolders selects them all, and the batch
// commands then act on every one of them.
func TestBandSelectedRecursiveFilesTakeBatchOperations(t *testing.T) {
	a, root := recursiveApp(t)
	te := ui.NewTester(a.view, 1000, 700)
	te.Frame()
	te.Frame()
	te.Frame()
	box := a.marqueeBox
	dragBand(a, te, 0, box.X+1, box.Y+1, box.X+box.W-20, box.Y+box.H-20)
	if got := selectedIndexes(a); len(got) != len(a.Visible) {
		t.Fatalf("band over the whole listing selected %d of %d", len(got), len(a.Visible))
	}
	// The folders took their files with them (deep sits inside album): the
	// targets are album, other and the two top files.
	targets := a.operationPaths()
	if len(targets) != 4 {
		t.Fatalf("targets %v", targets)
	}

	// Pick just the files, from three folders.
	a.selectPaths(root, "top1.jpg", "album/a1.jpg", "album/deep/d1.jpg", "other/o1.jpg")
	files := a.operationPaths()
	if len(files) != 4 {
		t.Fatalf("file targets %v", files)
	}

	// Batch rename: each keeps its own folder.
	a.startBatchRename()
	a.Batch.Rule.Prefix = "x_"
	a.updateBatchPreview()
	a.commitBatchRename()
	for _, rel := range []string{"x_top1.jpg", "album/x_a1.jpg", "album/deep/x_d1.jpg", "other/x_o1.jpg"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("batch rename: %s missing: %v", rel, err)
		}
	}

	// Compress: one archive above them all, with their folders inside.
	renamed := []string{"x_top1.jpg", "album/x_a1.jpg", "album/deep/x_d1.jpg", "other/x_o1.jpg"}
	a.selectPaths(root, renamed...)
	a.compressPlain(false)
	zips, _ := filepath.Glob(filepath.Join(root, "Archive_*.zip"))
	if len(zips) != 1 {
		t.Fatalf("expected one archive in %s, got %v", root, zips)
	}
	zr, err := zip.OpenReader(zips[0])
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	zr.Close()
	for _, rel := range renamed {
		if !slices.Contains(names, filepath.ToSlash(rel)) {
			t.Errorf("archive lacks %s: %v", rel, names)
		}
	}

	// Move to one folder: all four land there; same-name files would be
	// renamed, not overwritten.
	dest := filepath.Join(root, "moved")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	a.selectPaths(root, renamed...)
	a.beginTransfer(fileOperation{op: opMove, paths: a.operationPaths(), dest: dest})
	for _, name := range []string{"x_top1.jpg", "x_a1.jpg", "x_d1.jpg", "x_o1.jpg"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("move: %s missing: %v", name, err)
		}
	}

	// Delete the moved ones.
	a.Selected.Clear()
	for _, name := range []string{"x_top1.jpg", "x_a1.jpg", "x_d1.jpg", "x_o1.jpg"} {
		a.Selected.Add(filepath.Join(dest, name))
	}
	a.Chosen = 0
	a.runOperation(fileOperation{op: opDelete, paths: a.operationPaths(), permanent: true})
	left, _ := os.ReadDir(dest)
	if len(left) != 0 {
		t.Fatalf("delete left %d files", len(left))
	}
}

// Moving a folder together with the files it expanded into is one move, not
// a failing second one for files that are already gone.
func TestMovingAFolderAndItsExpandedFilesOnce(t *testing.T) {
	a, root := recursiveApp(t)
	dest := filepath.Join(root, "moved")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	a.selectPaths(root, "album", "album/a1.jpg", "album/deep/d1.jpg")
	a.beginTransfer(fileOperation{op: opMove, paths: a.operationPaths(), dest: dest})
	for _, rel := range []string{"album/a1.jpg", "album/a2.jpg", "album/deep/d1.jpg"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Fatalf("%s did not move with its folder: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "album")); !os.IsNotExist(err) {
		t.Fatal("the folder is still in place")
	}
}

func TestBatchTagsReachEveryBandSelectedFile(t *testing.T) {
	if !tagsSupported {
		t.Skip("no file tags on this platform")
	}
	a, root := recursiveApp(t)
	a.selectPaths(root, "top1.jpg", "album/a1.jpg", "other/o1.jpg")
	paths := a.operationPaths()
	name := finderLabelNames()[0]
	a.toggleTag(paths, name)
	for _, p := range paths {
		tags, err := urlTags(p)
		if err != nil || !containsString(tags, name) {
			t.Fatalf("%s: tags %v err %v after tagging %q", strings.TrimPrefix(p, root), tags, err, name)
		}
	}
}

// The right-drag gesture and the context menu share the right button: the
// menu opens on the press, and the gesture state is still fed by it.
func TestRightPressOpensMenuAndStartsGesture(t *testing.T) {
	a := marqueeApp(0, 6)
	te := marqueeTester(t, a)
	x, y := cardCenter(a, 1)
	te.PressButton(1, x, y)
	te.Frame()
	if !a.Gesture.active {
		t.Fatal("right press did not start the gesture")
	}
	if len(te.Menu()) == 0 {
		t.Fatal("right press opened no context menu in the browse view")
	}
}
