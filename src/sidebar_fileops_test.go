package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarFolderMenuCopiesAndOffersFileOps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	parent := t.TempDir()
	dir := filepath.Join(parent, "sidepix")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := &appState{
		Chosen: -1, LargeIndex: -1, Thumb: 128, SidebarShown: true,
		Root: parent, treeCache: map[string]treeEntry{},
		Profiles: prefsFile{View: defaultViewPreferences(), Favorites: []string{dir}},
	}
	te := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 240, 420)
	te.Frame()
	if err := te.RightClick("sidepix"); err != nil {
		t.Fatalf("right click: %v texts=%v", err, te.Texts())
	}
	te.Frame()
	labels := te.Menu()
	for _, want := range []string{"打开", "重命名…", "复制", "移到废纸篓"} {
		if !hasText(labels, want) {
			t.Fatalf("menu missing %q in %v", want, labels)
		}
	}
	te.CloseMenu()
	a.menuPath = dir
	if got := a.selectedPaths(); len(got) != 1 || got[0] != dir {
		t.Fatalf("menu path %v", got)
	}
	a.menuPath = ""
}

func TestFolderAndFileMenusBothOfferFileOps(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "album")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(dir, "a.jpg")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{
		Chosen: -1, LargeIndex: -1, Root: dir,
		Profiles: prefsFile{View: defaultViewPreferences()},
		Items: []mediaItem{
			{Path: folder, Name: "album", IsDir: true},
			{Path: img, Name: "a.jpg", Ext: "jpg"},
		},
		Visible: []int{0, 1},
	}
	for _, index := range []int{0, 1} {
		te := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, index) })
		}, 400, 300)
		te.Frame()
		te.RightClickAt(200, 150)
		te.Frame()
		if !hasText(te.Menu(), "移到废纸篓") || !hasText(te.Menu(), "复制") {
			t.Fatalf("index %d menu %v", index, te.Menu())
		}
	}
}
