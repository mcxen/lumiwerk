package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The status panel names the open folder and how much media it holds,
// and offers the actions that have to work while the window is hidden.
func TestTrayPanelShowsFolderAndActions(t *testing.T) {
	a := &appState{
		Chosen:     -1,
		LargeIndex: -1,
		Root:       "/Users/test/Photos",
		Items: []mediaItem{
			{Path: "/Users/test/Photos/a.jpg", Name: "a.jpg", Ext: "jpg"},
			{Path: "/Users/test/Photos/b.jpg", Name: "b.jpg", Ext: "jpg"},
			{Path: "/Users/test/Photos/notes.txt", Name: "notes.txt", Ext: "txt"},
		},
		Visible: []int{0, 1, 2},
		Tabs:    tabState{Tabs: []tab{{Root: "/Users/test/Photos"}, {Root: "/Users/test/Other"}}, Active: 0},
	}
	te := ui.NewTester(a.trayPanel, 300, 292)
	te.Frame()
	for _, want := range []string{"Lumiwerk", "Photos", "/Users/test/Photos", "2 个媒体", "2 个标签页", "主窗口已收起", "显示主窗口", "偏好设置", "退出 Lumiwerk"} {
		if !te.HasText(want) {
			t.Fatalf("panel missing %q in %q", want, te.Texts())
		}
	}
}

func TestTrayPanelEmptyFolder(t *testing.T) {
	a := &appState{Chosen: -1, LargeIndex: -1}
	te := ui.NewTester(a.trayPanel, 300, 292)
	te.Frame()
	if !te.HasText("未打开目录") || !te.HasText("尚未打开目录") || !te.HasText("打开主窗口后可以选择目录") || !te.HasText("显示主窗口") {
		t.Fatalf("empty panel: %q", te.Texts())
	}
}
