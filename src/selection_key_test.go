package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestItemKeyIsAlwaysString guards the grid/list selection contract:
// a.Selected is a Selection[string], so every row key must be a string. A row
// keyed by its index panics in mygo with "a row's key is a int, not the string
// of its Selection", which took the whole app down while laying out the list.
func TestItemKeyIsAlwaysStringForSelection(t *testing.T) {
	a := &appState{}

	if key, ok := a.itemKey(0).(string); !ok || key == "" {
		t.Fatalf("missing row key = %#v, want a non-empty string", a.itemKey(0))
	}

	a.Items = []mediaItem{{Path: "/tmp/one.jpg"}, {Path: "/tmp/two.mp4"}}
	a.Visible = []int{1, 0}
	if key := a.itemKey(0); key != "/tmp/two.mp4" {
		t.Fatalf("visible row 0 key = %#v, want /tmp/two.mp4", key)
	}
	if key := a.itemKey(1); key != "/tmp/one.jpg" {
		t.Fatalf("visible row 1 key = %#v, want /tmp/one.jpg", key)
	}
	if _, ok := a.itemKey(2).(string); !ok {
		t.Fatalf("out-of-range row key = %#v, want a string", a.itemKey(2))
	}
}

// TestDetailListRendersWithPathSelection exercises the same mygo list layout
// that crashed when ListState kept its default integer row keys.
func TestDetailListRendersWithPathSelection(t *testing.T) {
	a := &appState{Chosen: 0, Thumb: 192}
	a.Items = []mediaItem{
		{Path: "/tmp/one.jpg", Name: "one.jpg", Ext: "jpg"},
		{Path: "/tmp/two.mp4", Name: "two.mp4", Ext: "mp4", IsVideo: true},
	}
	a.Visible = []int{0, 1}
	a.Selected.Add("/tmp/one.jpg")

	te := ui.NewTester(a.detailView, 1000, 600)
	te.Frame()
	te.Frame()
	if !te.HasText("one.jpg") || !te.HasText("two.mp4") {
		t.Fatalf("detail rows missing: %q", te.Texts())
	}
}
