package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

const sampleImage = "/Users/mcx/Pictures/Qxpicture/qxpicture-mwm-landscape-1784973357023.png"

// TestRenderLargeView renders the large-image layout headless for visual alignment.
func TestRenderLargeView(t *testing.T) {
	data, err := os.ReadFile(sampleImage)
	if err != nil {
		t.Skip("sample image unavailable")
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: 0, LargeIndex: 0, Thumb: 128, InLargeView: true}
	a.Root = "/Users/mcx/Pictures/Qxpicture"
	a.Items = []mediaItem{{
		Path: sampleImage, Name: "qxpicture-mwm-landscape-1784973357023.png", Ext: "png",
		Bitmap: bm, Decoded: true, InfoLoaded: true,
		Info: mediaInfo{Width: 1024, Height: 1024},
		Tags: []string{"红色", "评分4"},
	}}
	a.Visible = []int{0}
	a.Zoom = 1.6
	a.PanX, a.PanY = 40, -20
	a.HUD = "缩放: 160%"
	a.HUDUntil = time.Now().Add(time.Hour)
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	img := te.Image()
	f, err := os.Create("/tmp/lw-large-render.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestRenderGrid renders the grid layout headless for visual alignment.
func TestRenderGrid(t *testing.T) {
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 128}
	a.Root = "/Users/mcx/Pictures/Qxpicture"
	a.Items = []mediaItem{
		{Path: "/a.png", Name: "a.png", Ext: "png", Tags: []string{"红色"}},
	}
	a.Visible = []int{0}
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	img := te.Image()
	f, err := os.Create("/tmp/lw-grid-render.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
