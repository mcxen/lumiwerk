package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBreadcrumbClickOpensAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "pix")
	os.MkdirAll(sub, 0o755)
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 128, Cursor: -1, SidebarWidth: sidebarDefaultWidth}
	a.openFolder(sub)
	parentName := filepath.Base(root)
	tt := ui.NewTester(func(c *ui.Context) { a.toolbarView(c) }, 900, 48)
	tt.Frame()
	if err := tt.Click(parentName); err != nil {
		t.Fatalf("click crumb %q: %v texts=%v", parentName, err, tt.Texts())
	}
	tt.Frame()
	if a.Root != root {
		t.Fatalf("root=%q want %q", a.Root, root)
	}
}
