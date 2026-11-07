package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRecurseDepthKeepsOnlyTheChosenLevels(t *testing.T) {
	root := t.TempDir()
	mustFile(t, filepath.Join(root, "a.jpg"))
	mustFile(t, filepath.Join(root, "sub", "b.jpg"))
	mustFile(t, filepath.Join(root, "sub", "nested", "c.jpg"))
	mustFile(t, filepath.Join(root, "sub", "nested", "deep", "d.jpg"))
	view := defaultViewPreferences()

	names := func(maxDepth int) []string {
		items := scanTreeParallel(root, func(entry scanEntry) scanEntryResult {
			return acceptScanEntry(root, entry, view, true, maxDepth)
		})
		out := make([]string, len(items))
		for i, item := range items {
			rel, err := filepath.Rel(root, item.Path)
			if err != nil {
				t.Fatal(err)
			}
			out[i] = filepath.ToSlash(rel)
		}
		return out
	}
	if got := names(1); !sameNames(got, []string{"a.jpg", "sub/b.jpg"}) {
		t.Fatalf("1 layer: %v", got)
	}
	if got := names(2); !sameNames(got, []string{"a.jpg", "sub/b.jpg", "sub/nested/c.jpg"}) {
		t.Fatalf("2 layers: %v", got)
	}
	if got := names(0); !sameNames(got, []string{"a.jpg", "sub/b.jpg", "sub/nested/c.jpg", "sub/nested/deep/d.jpg"}) {
		t.Fatalf("all layers: %v", got)
	}
}

func TestRecurseScopeSummaryAndFolderLabel(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "album")
	items := []mediaItem{
		{Path: filepath.Join(root, "RAW 原片", "太平山等日出", "a.jpg"), Ext: "jpg"},
		{Path: filepath.Join(root, "clip.mp4"), Ext: "mp4", IsVideo: true},
		{Path: filepath.Join(root, "day", "b.jpg"), Ext: "jpg"},
	}
	folders, images, videos := recurseScopeStats(root, items, []int{0, 1, 2})
	if folders != 3 || images != 2 || videos != 1 {
		t.Fatalf("stats folders=%d images=%d videos=%d", folders, images, videos)
	}
	want := "已展开 3 个子文件夹，平铺显示全部 3 项（2 图片 · 1 视频）"
	if got := recurseSummaryText(folders, images, videos); got != want {
		t.Fatalf("summary %q", got)
	}
	if got := sourceFolderLabel(root, items[0].Path); got != "RAW 原片/太平山等日出" {
		t.Fatalf("label %q", got)
	}
	if got := sourceFolderLabel(root, items[1].Path); got != "" {
		t.Fatalf("current folder labeled %q", got)
	}
	if formatClock(83) != "1:23" || formatClock(3661) != "1:01:01" {
		t.Fatalf("clock %q %q", formatClock(83), formatClock(3661))
	}
	a := &appState{Root: root, Recursive: true, Profiles: prefsFile{View: viewPreferences{RecurseDepth: 1}}}
	if !a.folderInRecurseScope(filepath.Join(root, "day")) {
		t.Fatal("child folder should be in scope")
	}
	if a.folderInRecurseScope(filepath.Join(root, "day", "more")) {
		t.Fatal("grandchild should be out of a 1-layer scope")
	}
	a.Recursive = false
	if a.folderInRecurseScope(filepath.Join(root, "day")) {
		t.Fatal("scope line without recursion")
	}
}

func TestRecurseToolbarSummaryAndWaterfallMarks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := filepath.Join(string(filepath.Separator), "album")
	a := &appState{
		Chosen: -1, LargeIndex: -1, Thumb: 160, LayoutMode: 2,
		Root: root, Recursive: true,
		Profiles: prefsFile{View: defaultViewPreferences()},
		Items: []mediaItem{
			{Path: filepath.Join(root, "RAW 原片", "太平山等日出", "a.jpg"), Name: "a.jpg", Ext: "jpg", Decoded: true, InfoLoaded: true, Info: mediaInfo{Width: 400, Height: 300}},
			{Path: filepath.Join(root, "clip.mp4"), Name: "clip.mp4", Ext: "mp4", IsVideo: true, Decoded: true, InfoLoaded: true, Info: mediaInfo{Width: 320, Height: 180, DurationSec: 83}},
		},
		Visible: []int{0, 1},
	}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			a.toolbarView(c)
			a.waterfallView(c)
			a.statusBar(c)
		})
	}, 1800, 720)
	te.Frame()
	te.Frame()
	texts := te.Texts()
	for _, want := range []string{
		"递归",
		"全部",
		"图片",
		"视频",
		"全部层级",
		"1 层",
		"2 层",
		"已展开 2 个子文件夹，平铺显示全部 2 项（1 图片 · 1 视频）",
		"RAW 原片/太平山等日出",
		"1:23",
	} {
		if !hasText(texts, want) {
			t.Fatalf("missing %q in %v", want, texts)
		}
	}
	if err := te.Click("2 层"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if a.Profiles.View.RecurseDepth != 2 {
		t.Fatalf("depth %d", a.Profiles.View.RecurseDepth)
	}
	if err := te.Click("图片"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if a.FilterMode != 1 {
		t.Fatalf("filter %d", a.FilterMode)
	}
	if err := te.Click("排序"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if !hasText(te.Menu(), "文件名") {
		t.Fatalf("sort menu %v", te.Menu())
	}
}

func mustFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sameNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, name := range got {
		seen[name]++
	}
	for _, name := range want {
		seen[name]--
		if seen[name] < 0 {
			return false
		}
	}
	return true
}

func hasText(texts []string, want string) bool {
	for _, text := range texts {
		if text == want {
			return true
		}
	}
	return false
}
