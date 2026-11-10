package main

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// hasColorNear reports whether a pixel close to rgb appears in the region.
func hasColorNear(img *image.RGBA, r image.Rectangle, want [3]uint8, tol int) bool {
	b := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y && y < b.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X && x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			dr := int(c.R) - int(want[0])
			dg := int(c.G) - int(want[1])
			db := int(c.B) - int(want[2])
			if dr < 0 {
				dr = -dr
			}
			if dg < 0 {
				dg = -dg
			}
			if db < 0 {
				db = -db
			}
			if dr <= tol && dg <= tol && db <= tol {
				return true
			}
		}
	}
	return false
}

// TestLargeViewOverlays checks that tag dots draw and that rating tags,
// which the app no longer shows, never become dots.
func TestLargeViewOverlays(t *testing.T) {
	a := &appState{Chosen: 0, LargeIndex: 0, Thumb: 128, InLargeView: true}
	a.Root = "/tmp"
	a.Items = []mediaItem{{
		Path: "/tmp/a.png", Name: "a.png", Ext: "png",
		InfoLoaded: true, Info: mediaInfo{Width: 100, Height: 100},
		Tags: []string{"红色", "绿色", "评分4"},
	}}
	a.Visible = []int{0}
	a.HUD = "缩放: 160%"
	a.HUDUntil = time.Now().Add(time.Hour)
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	img := te.Image()
	if !hasColorNear(img, image.Rect(900, 0, 1280, 120), [3]uint8{230, 69, 55}, 24) {
		t.Error("red Finder tag dot not found in top-right overlay")
	}
	if !hasColorNear(img, image.Rect(900, 0, 1280, 120), [3]uint8{60, 180, 74}, 24) {
		t.Error("green Finder tag dot not found in top-right overlay")
	}
	if te.HasText("评级") {
		t.Error("the star rating control must be gone")
	}
}
