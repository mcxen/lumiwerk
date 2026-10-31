package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestRenderLargeView renders the large-image layout headless for visual alignment.
func TestRenderLargeView(t *testing.T) {
	a := &appState{Chosen: 0, LargeIndex: 0, Thumb: 128, InLargeView: true}
	a.Root = "/Users/mcx/Pictures/Qxpicture"
	a.Items = []mediaItem{
		{Path: "/Users/mcx/Pictures/Qxpicture/a.png", Name: "a.png", Ext: "png", IsVideo: false},
	}
	a.Visible = []int{0}
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
