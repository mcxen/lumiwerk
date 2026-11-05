package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The item menu keeps a short top level; the rest lives in submenus.
func TestImageContextMenuTopLevelIsShort(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.jpg")
	os.WriteFile(img, []byte("x"), 0o644)
	a := &appState{Chosen: 0, LargeIndex: -1, Cursor: -1, Root: dir, Profiles: prefsFile{View: defaultViewPreferences()}}
	a.Items = []mediaItem{{Path: img, Name: "a.jpg", Ext: "jpg"}}
	a.Visible = []int{0}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, 0) })
	}, 600, 400)
	te.Frame()
	te.RightClickAt(300, 200)
	te.Frame()
	items := 0
	for _, l := range te.Menu() {
		if l != "-" {
			items++
		}
	}
	t.Logf("menu: %q", te.Menu())
	if items > 14 {
		t.Fatalf("top level has %d items: %q", items, te.Menu())
	}
	if err := te.ChooseMenuItem("旋转", "向右旋转 90°"); err != nil {
		t.Fatalf("rotate submenu: %v", err)
	}
}
