package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarClearsTitleBar(t *testing.T) {
	a := &appState{SidebarShown: true, SidebarWidth: 220, Thumb: 128,
		Profiles: prefsFile{View: defaultViewPreferences(), Favorites: []string{"/tmp"}},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 220, 400)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 72})
	tt.Frame()
	r, ok := tt.Find("收藏夹")
	if !ok {
		t.Fatal("no 收藏夹", tt.Texts())
	}
	if r.Y < 28 {
		t.Fatalf("收藏夹 y=%v overlaps title bar height 28", r.Y)
	}
}
