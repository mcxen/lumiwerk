package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func newLargeState() *appState {
	a := &appState{}
	a.Window = nil
	a.Root = "/tmp"
	a.Items = []mediaItem{{Path: "/tmp/a.png", Name: "a.png", InfoLoaded: true, Info: mediaInfo{Width: 1000, Height: 1000}}}
	a.Visible = []int{0}
	a.InLargeView = true
	a.LargeIndex = 0
	return a
}

func TestZoomAtKeepsPointerAnchor(t *testing.T) {
	a := newLargeState()
	a.Zoom = 1
	a.PanX, a.PanY = 0, 0
	// The content point under (700, 300) must stay there after zooming.
	cx, cy := float32(700), float32(300)
	w, h := a.largeSize()
	lx, ly := cx-w/2, cy-h/2
	contentX := (lx - a.PanX) / a.Zoom
	contentY := (ly - a.PanY) / a.Zoom
	a.zoomAt(cx, cy, 2)
	newLX := contentX*a.Zoom + a.PanX
	newLY := contentY*a.Zoom + a.PanY
	if abs32(newLX-lx) > 0.01 || abs32(newLY-ly) > 0.01 {
		t.Fatalf("anchor moved: got (%v,%v), want (%v,%v)", newLX, newLY, lx, ly)
	}
}

func TestFitScaleAndZoomPercent(t *testing.T) {
	a := newLargeState()
	a.Items[0].Info.Width = 1000
	a.Items[0].Info.Height = 1000
	fit := a.fitScale(&a.Items[0])
	if fit <= 0 {
		t.Fatalf("fit scale = %v", fit)
	}
	a.Zoom = 1
	if got := a.zoomPercent(&a.Items[0]); got <= 0 {
		t.Fatalf("zoom percent = %d", got)
	}
}

func TestResetZoomClearsPan(t *testing.T) {
	a := newLargeState()
	a.Zoom = 3
	a.PanX, a.PanY = 40, -20
	a.resetZoom()
	if a.Zoom != 1 || a.PanX != 0 || a.PanY != 0 {
		t.Fatalf("reset = zoom %v pan (%v,%v)", a.Zoom, a.PanX, a.PanY)
	}
}

func TestZoomLimitsMatchOriginalWidth(t *testing.T) {
	a := newLargeState()
	fit := a.fitScale(&a.Items[0])
	if got := a.clampZoom(0); abs32(got-0.5/fit) > 0.001 {
		t.Fatalf("minimum zoom = %v, want %v", got, 0.5/fit)
	}
	if got := a.clampZoom(100); abs32(got-10/fit) > 0.001 {
		t.Fatalf("maximum zoom = %v, want %v", got, 10/fit)
	}
}

func TestLargeViewZoomControls(t *testing.T) {
	a := newLargeState()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	if err := te.Click("1:1"); err != nil {
		t.Fatalf("click 1:1: %v", err)
	}
	if got := a.zoomPercent(&a.Items[0]); got != 100 {
		t.Fatalf("1:1 zoom = %d%%, state zoom %v, HUD %q, controls %v", got, a.Zoom, a.HUD, te.Texts())
	}
	if a.HUD != "缩放: 100%" {
		t.Fatalf("HUD = %q, want 100%%", a.HUD)
	}
	if err := te.Click("适应"); err != nil {
		t.Fatalf("click fit: %v", err)
	}
	if a.Zoom != 1 || a.PanX != 0 || a.PanY != 0 {
		t.Fatalf("fit state = zoom %v pan (%v,%v)", a.Zoom, a.PanX, a.PanY)
	}
	if err := te.Click("放大"); err != nil {
		t.Fatalf("click zoom in: %v", err)
	}
	if a.Zoom != 1.25 {
		t.Fatalf("zoom in = %v, want 1.25", a.Zoom)
	}
	for _, label := range []string{"缩小", "顺时针旋转", "逆时针旋转", "镜像"} {
		if !te.HasText(label) {
			t.Fatalf("Lucide control is missing accessible label %q: %v", label, te.Texts())
		}
	}
	if err := te.Click("缩小"); err != nil {
		t.Fatalf("click zoom out: %v", err)
	}
	if a.Zoom != 1 {
		t.Fatalf("zoom out = %v, want 1", a.Zoom)
	}
	if err := te.Click("顺时针旋转"); err != nil {
		t.Fatal(err)
	}
	if a.Rotation != 90 {
		t.Fatalf("rotation = %v, want 90", a.Rotation)
	}
	if err := te.Click("逆时针旋转"); err != nil {
		t.Fatal(err)
	}
	if a.Rotation != 0 {
		t.Fatalf("counterclockwise rotation = %v, want 0", a.Rotation)
	}
	if err := te.Click("镜像"); err != nil {
		t.Fatal(err)
	}
	if !a.Mirror {
		t.Fatal("mirror button did not change the view")
	}
}

func TestTagColors(t *testing.T) {
	if got := tagColor("红色"); got == tagColor("蓝色") {
		t.Fatal("tag colors should differ")
	}
}
