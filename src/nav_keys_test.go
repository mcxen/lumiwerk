package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func navDirs(t *testing.T, names ...string) []string {
	root := t.TempDir()
	var out []string
	for _, n := range names {
		p := filepath.Join(root, n)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// TestTabsKeepTheirOwnHistory: going back in one tab walks that tab's
// folders only, and switching back finds the other tab's trail.
func TestTabsKeepTheirOwnHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := navDirs(t, "a", "b", "x", "y")
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1}
	a.openFolder(d[0])
	a.openFolder(d[1])
	a.openInNewTab(d[2])
	a.openFolder(d[3])
	if !slices.Equal(a.History, []string{d[2], d[3]}) || a.Cursor != 1 {
		t.Fatalf("new tab history %v@%d", a.History, a.Cursor)
	}
	a.switchTab(0)
	if a.Root != d[1] || !slices.Equal(a.History, []string{d[0], d[1]}) {
		t.Fatalf("tab 0 history %v@%d root %s", a.History, a.Cursor, a.Root)
	}
	a.goBack()
	if a.Root != d[0] {
		t.Fatalf("back in tab 0 went to %s", a.Root)
	}
	a.switchTab(1)
	a.goBack()
	if a.Root != d[2] {
		t.Fatalf("back in tab 1 went to %s", a.Root)
	}
	a.closeTab(1)
	if a.Tabs.Active != 0 || a.Root != d[0] || !a.canGoForward() {
		t.Fatalf("after close: active %d root %s history %v@%d", a.Tabs.Active, a.Root, a.History, a.Cursor)
	}
}

// TestMouseBackForwardKeys: the mouse's side buttons go back and forward.
func TestMouseBackForwardKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := navDirs(t, "a", "b")
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1}
	a.openFolder(d[0])
	a.openFolder(d[1])
	te := ui.NewTester(a.view, 900, 600)
	te.Frame()
	te.Key(0, ui.KeyBack)
	te.Frame()
	if a.Root != d[0] {
		t.Fatalf("mouse back: root %s", a.Root)
	}
	te.Key(0, ui.KeyForward)
	te.Frame()
	if a.Root != d[1] {
		t.Fatalf("mouse forward: root %s", a.Root)
	}
}
