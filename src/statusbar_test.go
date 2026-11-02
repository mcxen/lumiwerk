package main

import (
	"math"
	"strings"
	"testing"
)

func TestJustifiedWidthsFollowAspectAndFillRow(t *testing.T) {
	aspects := []float32{2, 1, 0.5}
	w := justifiedWidths(aspects, []int{0, 1, 2}, 700, 100, 10, false)
	if !(w[0] > w[1] && w[1] > w[2]) {
		t.Fatalf("widths %v not ordered by aspect", w)
	}
	if sum := w[0] + w[1] + w[2] + 20; math.Abs(float64(sum-700)) > 0.5 {
		t.Fatalf("full row width %v, want 700", sum)
	}
	last := justifiedWidths(aspects, []int{1}, 700, 100, 10, true)
	if last[0] != 100 {
		t.Fatalf("short last row stretched to %v", last[0])
	}
}

func TestStatusTextShowsSelectionNotFolder(t *testing.T) {
	a := &appState{Root: "/tmp/p", Chosen: -1}
	a.Items = []mediaItem{
		{Path: "/tmp/p/a.jpg", Name: "a.jpg", Ext: ".jpg", Size: 2048, ModTime: 1, InfoLoaded: true, Info: mediaInfo{Width: 4000, Height: 3000}},
		{Path: "/tmp/p/b.jpg", Name: "b.jpg", Ext: ".jpg", Size: 1024, InfoLoaded: true},
	}
	a.Visible = []int{0, 1}
	if got := a.statusText(); !strings.HasSuffix(got, " 个媒体") {
		t.Fatalf("idle status %q", got)
	}
	a.Selected.Add("/tmp/p/a.jpg")
	got := a.statusText()
	if !strings.HasPrefix(got, "a.jpg · ") || !strings.Contains(got, "4000×3000") || strings.Contains(got, "/tmp/p") {
		t.Fatalf("selection status %q", got)
	}
}
