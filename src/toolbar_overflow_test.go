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
