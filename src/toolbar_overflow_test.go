package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestToolbarOverflowLayoutLabels(t *testing.T) {
	a := &appState{
		Thumb: 128, SidebarShown: true, SidebarWidth: sidebarDefaultWidth,
		LayoutMode: 0, FilterMode: 0, Sort: "name",
		Profiles: prefsFile{View: defaultViewPreferences()},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.toolbarView(c) }, 280, 48)
	tt.Frame()
	if err := tt.Click("More"); err != nil {
		t.Fatalf("open overflow: %v texts=%v", err, tt.Texts())
	}
	labels := tt.Menu()
	t.Log("overflow:", labels)
	for _, want := range []string{"网格", "详情", "瀑布", "紧凑"} {
		found := false
		for _, l := range labels {
			if l == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("overflow missing %q in %v", want, labels)
		}
	}
	// Separators show as "" in Tester.Menu(); real blank controls would
	// add extra empties beyond those group breaks.
	for _, l := range labels {
		if strings.TrimSpace(l) == "" {
			t.Fatalf("blank overflow item in %v", labels)
		}
	}
}

// TestToolbarKeepsRightClusterAndFoldsDepth checks the toolbar at the width
// of a typical window (990pt) and a wide one: the display controls on the
// right always stay on the bar; the depth menu shows only while recursion is
// on and there is room, and otherwise moves into the 显示与浏览 menu.
func TestToolbarKeepsRightClusterAndFoldsDepth(t *testing.T) {
	for _, tc := range []struct {
		width     int
		recursive bool
		depthBar  bool
	}{
		{990, false, false},
		{990, true, false},
		{1700, false, false},
		{1700, true, true},
	} {
		a := &appState{
			Thumb: 128, SidebarShown: true, SidebarWidth: sidebarDefaultWidth,
			Root: "/Users/mcx/Downloads", Recursive: tc.recursive, Sort: "name", FilterMode: filterMedia,
			Profiles: prefsFile{View: defaultViewPreferences()},
		}
		tt := ui.NewTester(func(c *ui.Context) { a.toolbarView(c) }, tc.width, 48)
		tt.Frame()
		tt.Frame()
		texts := tt.Texts()
		if has := hasText(texts, "全部层级"); has != tc.depthBar {
			t.Fatalf("%+v: depth on the bar = %v: %v", tc, has, texts)
		}
		for _, want := range []string{"排序", "显示与浏览", "媒体模式：只看图片和视频"} {
			if _, ok := tt.Find(want); !ok {
				t.Fatalf("%+v: %q fell off the toolbar: %v", tc, want, texts)
			}
		}
		if tc.width > 1000 && tc.recursive {
			if err := tt.Click("显示与浏览"); err != nil {
				t.Fatal(err)
			}
			for _, dup := range []string{"递归浏览", "包含文件夹", "递归深度"} {
				if hasText(tt.Menu(), dup) {
					t.Fatalf("%q repeats a toolbar control in the menu: %v", dup, tt.Menu())
				}
			}
		}
		if tc.recursive && !tc.depthBar {
			if err := tt.Click("显示与浏览"); err != nil {
				t.Fatal(err)
			}
			if !hasText(tt.Menu(), "递归深度") || !hasText(tt.Menu(), "媒体：图片") {
				t.Fatalf("%+v: folded controls missing from the menu: %v", tc, tt.Menu())
			}
		}
	}
}
