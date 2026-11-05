package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSidebarSectionsFoldAndPersist folds 收藏夹 and 历史 with a click on
// their titles and keeps the folded state in the preferences.
func TestSidebarSectionsFoldAndPersist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	a := &appState{SidebarShown: true, SidebarWidth: 220, Thumb: 128, Root: dir,
		Profiles: prefsFile{View: defaultViewPreferences(), Favorites: []string{dir}},
		Recent:   []string{dir}, Visited: map[string]int64{dir: 1}, treeCache: map[string]treeEntry{},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 220, 500)
	tt.Frame()
	name := pathName(dir)
	if n := countText(tt, name); n < 2 {
		t.Fatalf("want favorite and history rows for %q, texts %v", name, tt.Texts())
	}
	tt.Click("收藏夹")
	tt.Frame()
	if !a.Profiles.FavoritesClosed {
		t.Fatal("click on 收藏夹 did not fold it")
	}
	if !loadPrefs().FavoritesClosed {
		t.Fatal("folded 收藏夹 not saved")
	}
	tt.Click("历史")
	tt.Frame()
	if !a.Profiles.HistoryClosed {
		t.Fatal("click on 历史 did not fold it")
	}
	tt.Click("收藏夹")
	tt.Frame()
	if a.Profiles.FavoritesClosed || loadPrefs().FavoritesClosed {
		t.Fatal("second click did not unfold 收藏夹")
	}
}

func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, x := range tt.Texts() {
		if x == s {
			n++
		}
	}
	return n
}
