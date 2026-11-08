package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The image strip is built from the same shell and buttons as the video
// transport, so both must expose the shared controls and stay interactive.
func TestImageControlBarSharedComponents(t *testing.T) {
	a := newLargeState()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	for _, label := range []string{"上一张", "下一张", "缩小", "放大", "适应", "1:1", "顺时针旋转", "逆时针旋转", "镜像", "全屏"} {
		if !te.HasText(label) {
			t.Fatalf("image control bar missing %q: %v", label, te.Texts())
		}
	}
	if err := te.Click("适应"); err != nil {
		t.Fatal(err)
	}
	if err := te.Click("放大"); err != nil {
		t.Fatal(err)
	}
	if a.Zoom != 1.25 {
		t.Fatalf("zoom in = %v, want 1.25", a.Zoom)
	}
	if err := te.Click("镜像"); err != nil {
		t.Fatal(err)
	}
	if !a.Mirror {
		t.Fatal("mirror toggle did not change state")
	}
	// Still renders and responds after the toggle (accent state).
	te.Frame()
	if err := te.Click("顺时针旋转"); err != nil {
		t.Fatal(err)
	}
	if a.Rotation != 90 {
		t.Fatalf("rotation = %v, want 90", a.Rotation)
	}
}
