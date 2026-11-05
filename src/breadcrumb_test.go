package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenBreadcrumbNavigatesAncestors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix path breadcrumbs")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1, SidebarWidth: sidebarDefaultWidth}
	a.openFolder(sub)
	if a.Root != sub {
		t.Fatalf("root %q", a.Root)
	}
	items := a.breadcrumbItems()
	// last is current; pick the parent "a" level = len-2
	parentIdx := len(items) - 2
	a.openBreadcrumb(parentIdx)
	want := filepath.Join(root, "a")
	if a.Root != want {
		t.Fatalf("after crumb %d: root=%q want %q items=%v", parentIdx, a.Root, want, items)
	}
	a.openBreadcrumb(0)
	if a.Root != "/" {
		t.Fatalf("root crumb: %q", a.Root)
	}
}
