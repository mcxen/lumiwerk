package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSchemeAHeaderAndWindowsTopRightControls(t *testing.T) {
	oldControls := showControls
	showControls = true
	t.Cleanup(func() { showControls = oldControls })

	a := polishTestApp(t, 0, 4)
	dir := t.TempDir()
	a.Root = dir
	a.SidebarShown = true
	a.SidebarWidth = 220
	a.treeCache = map[string]treeEntry{}

	const winW, winH = 1200, 720
	te := ui.NewTester(a.view, winW, winH)
	te.Frame()

	sb, ok := te.Find("显示/隐藏侧栏")
	if !ok {
		t.Fatalf("missing sidebar toggle button: %v", te.Texts())
	}
	if sb.X >= a.SidebarWidth {
		t.Fatalf("sidebar toggle x=%v should sit inside expanded sidebar (< %v)", sb.X, a.SidebarWidth)
	}
	fav, ok := te.Find("收藏夹")
	if !ok {
		t.Fatalf("missing 收藏夹: %v", te.Texts())
	}
	if fav.Y < 52 {
		t.Fatalf("收藏夹 Y=%v, want >= 52 aligned below seamless header", fav.Y)
	}

	assertControlsPinnedTopRight := func(stage string) {
		t.Helper()
		minBtn, ok1 := te.Find("最小化")
		maxBtn, ok2 := te.Find("最大化")
		closeBtn, ok3 := te.Find("关闭")
		if !ok1 || !ok2 || !ok3 {
			t.Fatalf("%s: window controls missing (%v %v %v): %v", stage, ok1, ok2, ok3, te.Texts())
		}
		if math.Abs(float64(closeBtn.Y)) > 1 || math.Abs(float64(maxBtn.Y)) > 1 || math.Abs(float64(minBtn.Y)) > 1 {
			t.Fatalf("%s: window controls not flush at top edge (min.Y=%v max.Y=%v close.Y=%v)",
				stage, minBtn.Y, maxBtn.Y, closeBtn.Y)
		}
		if math.Abs(float64(closeBtn.X+closeBtn.W-winW)) > 1 {
			t.Fatalf("%s: close button right=%v, want %v", stage, closeBtn.X+closeBtn.W, winW)
		}
		if math.Abs(float64(closeBtn.W-controlW)) > 1 || math.Abs(float64(closeBtn.H-controlH)) > 1 {
			t.Fatalf("%s: close size=%vx%v, want %dx%d", stage, closeBtn.W, closeBtn.H, controlW, controlH)
		}
	}

	assertControlsPinnedTopRight("single-tab browse")

	a.Tabs = tabState{
		Tabs:   []tab{{Root: dir, Name: "Folder1", Cursor: -1}, {Root: dir, Name: "Folder2", Cursor: -1}},
		Active: 0,
	}
	te.Frame()
	assertControlsPinnedTopRight("multi-tab browse")

	a.Gallery = true
	a.InLargeView = true
	a.LargeIndex = 0
	te.Frame()
	assertControlsPinnedTopRight("gallery immersive")

	a.Gallery = false
	a.InLargeView = false
	a.Tabs = tabState{}
	te.Frame()
	if err := te.Click("显示/隐藏侧栏"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if a.SidebarShown {
		t.Fatal("clicking sidebar header toggle did not collapse sidebar")
	}
	if _, ok := te.Find("显示/隐藏侧栏"); !ok {
		t.Fatalf("collapsed toolbar did not show sidebar reopen button: texts=%v", te.Texts())
	}
	assertControlsPinnedTopRight("collapsed sidebar")
}

func TestFavoritesQuickPinUnpinDisambiguateAndReorder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, sub := range []string{"Downloads", "Pictures"} {
		if err := os.MkdirAll(filepath.Join(home, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	seeded := &appState{}
	seeded.seedDefaultFavorites()
	if len(seeded.Profiles.Favorites) != 2 {
		t.Fatalf("seedDefaultFavorites = %v, want Downloads and Pictures", seeded.Profiles.Favorites)
	}

	driveA := filepath.Join(home, "driveA", "photos")
	driveB := filepath.Join(home, "driveB", "photos")
	for _, p := range []string{driveA, driveB} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	a := &appState{
		SidebarShown: true,
		SidebarWidth: 230,
		Thumb:        128,
		Root:         driveA,
		treeCache:    map[string]treeEntry{},
		Profiles:     prefsFile{View: defaultViewPreferences()},
	}
	te := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 230, 560)
	te.Frame()

	if _, ok := te.Find("收藏当前目录"); !ok {
		t.Fatalf("empty favorites missing quick-pin button: %v", te.Texts())
	}
	if err := te.Click("收藏当前目录"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if !favoriteContains(a.Profiles.Favorites, driveA) || !a.Profiles.FavoritesCustom {
		t.Fatalf("after pin: favs=%v custom=%v", a.Profiles.Favorites, a.Profiles.FavoritesCustom)
	}
	// Clicking empty-state / header quick action toggles favorite off and stays cleared.
	if err := te.Click("取消收藏当前目录"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if len(a.Profiles.Favorites) != 0 {
		t.Fatalf("after unpin: favs=%v, want empty", a.Profiles.Favorites)
	}
	a.seedDefaultFavorites()
	if len(a.Profiles.Favorites) != 0 {
		t.Fatalf("seedDefaultFavorites overrode custom empty list: %v", a.Profiles.Favorites)
	}

	// Duplicate basenames disambiguate by parent directory and moveFavorite reorders cleanly.
	a.Profiles.Favorites = []string{driveA, driveB}
	te.Frame()
	texts := te.Texts()
	if !hasText(texts, "driveA") || !hasText(texts, "driveB") {
		t.Fatalf("duplicate basenames missing parent disambiguation: %v", texts)
	}
	a.moveFavorite(driveB, -1)
	if len(a.Profiles.Favorites) != 2 || pathKey(a.Profiles.Favorites[0]) != pathKey(driveB) {
		t.Fatalf("after moveFavorite(-1): %v", a.Profiles.Favorites)
	}
}
