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

	a := &appState{
		Chosen: -1, LargeIndex: -1, Thumb: 128, SidebarShown: true, SidebarWidth: 240,
		treeCache: map[string]treeEntry{},
		Profiles:  prefsFile{View: defaultViewPreferences()},
	}
	a.openRootFolder(opened)

	te := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 240, 560)
	te.Frame()

	for _, want := range []string{"tree-opened", "tree-child", "tree-sibling"} {
		if !te.HasText(want) {
			t.Fatalf("sidebar missing %q in current-folder mode; texts=%v", want, te.Texts())
		}
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
