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
	// A test temp dir on macOS sits under /var/folders/… and the crumb
	// row is wider than this toolbar, so the bar overflows it into More
	// and the click never sees the name. A short path stays on the bar.
	root, err := os.MkdirTemp("/tmp", "lwb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	sub := filepath.Join(root, "pix")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
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
