package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarTreeStartsAtCurrentFolderByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	outer := t.TempDir()
	parent := filepath.Join(outer, "tree-parent")
	opened := filepath.Join(parent, "tree-opened")
	child := filepath.Join(opened, "tree-child")
	grandchild := filepath.Join(child, "tree-grandchild")
	sibling := filepath.Join(opened, "tree-sibling")
	for _, d := range []string{grandchild, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	photo := filepath.Join(opened, "photo.jpg")
	video := filepath.Join(opened, "clip.mp4")
	note := filepath.Join(opened, "notes.txt")
	for _, f := range []string{photo, video, note} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := &appState{
		Chosen: -1, LargeIndex: -1, Thumb: 128, SidebarShown: true, SidebarWidth: 240,
		treeCache: map[string]treeEntry{},
		Profiles:  prefsFile{View: defaultViewPreferences()},
	}
	a.openRootFolder(opened)

	te := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 240, 560)
	te.Frame()

	for _, want := range []string{"tree-opened", "tree-child", "tree-sibling", "photo.jpg", "clip.mp4"} {
		if !te.HasText(want) {
			t.Fatalf("sidebar missing %q in current-folder mode; texts=%v", want, te.Texts())
		}
	}
	if te.HasText("notes.txt") {
		t.Fatalf("non-media file should not be listed in sidebar tree; texts=%v", te.Texts())
	}
	if got := a.treeChildren(photo); got != nil {
		t.Fatalf("media file should have nil treeChildren, got %v", got)
	}
	if te.HasText("tree-parent") || te.HasText(rootName) {
		t.Fatalf("current-folder tree should not display ancestors; texts=%v", te.Texts())
	}
	if _, ok := a.treeCache["/"]; ok {
		t.Fatal("current-folder mode must not read / into treeCache")
	}
	if _, ok := a.treeCache[parent]; ok {
		t.Fatalf("current-folder mode must not read parent %q into treeCache", parent)
	}

	// Media filter hides non-matching files in the tree too.
	a.FilterMode = filterImages
	te.Frame()
	if !te.HasText("photo.jpg") || te.HasText("clip.mp4") {
		t.Fatalf("filterImages should keep only images; texts=%v", te.Texts())
	}
	a.FilterMode = filterVideos
	te.Frame()
	if te.HasText("photo.jpg") || !te.HasText("clip.mp4") {
		t.Fatalf("filterVideos should keep only videos; texts=%v", te.Texts())
	}
	a.FilterMode = filterAll

	// Opting out hides media files and keeps folders only.
	a.Profiles.SidebarTreeFoldersOnly = true
	te.Frame()
	if te.HasText("photo.jpg") || te.HasText("clip.mp4") {
		t.Fatalf("folders-only mode should hide media files; texts=%v", te.Texts())
	}
	a.Profiles.SidebarTreeFoldersOnly = false
	te.Frame()

	// Clicking a media file in the tree selects it in the active folder.
	a.Items = []mediaItem{
		{Path: photo, Name: "photo.jpg", Ext: "jpg"},
		{Path: video, Name: "clip.mp4", Ext: "mp4", IsVideo: true},
	}
	a.Visible = []int{0, 1}
	if err := te.Click("photo.jpg"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if a.Chosen != 0 || !a.Selected.Has(photo) {
		t.Fatalf("clicking photo.jpg in tree did not select it: chosen=%d selected=%v", a.Chosen, a.Selected.Has(photo))
	}

	// Entering a child keeps the opened folder as the tree root so siblings remain visible.
	a.openFolder(child)
	te.Frame()
	if !te.HasText("tree-opened") || !te.HasText("tree-sibling") {
		t.Fatalf("entering child dropped opened root or sibling; texts=%v", te.Texts())
	}

	// Navigating deeper into grandchild expands child automatically.
	a.openFolder(grandchild)
	te.Frame()
	if !te.HasText("tree-grandchild") {
		t.Fatalf("entering grandchild did not expand child; texts=%v", te.Texts())
	}

	// Navigating above the base root re-anchors the tree at the new folder.
	a.openFolder(parent)
	te.Frame()
	if !te.HasText("tree-parent") || a.treeBaseRoot != parent {
		t.Fatalf("going above base root did not re-anchor: base=%q texts=%v", a.treeBaseRoot, te.Texts())
	}

	// Switching to system root restores the Macintosh HD / root tree.
	a.Profiles.SidebarTreeRoot = treeRootSystem
	a.treeOpenRoot = ""
	a.treeBaseRoot = ""
	te.Frame()
	if !te.HasText(rootName) {
		t.Fatalf("system mode missing %q; texts=%v", rootName, te.Texts())
	}
}
