package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// scenery paints a soft procedural landscape: a sky gradient, a sun and two
// hill layers. Each palette gives one picture its own mood.
func scenery(w, h int, sky0, sky1, sun, hill0, hill1 color.RGBA, seed float64) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	mix := func(a, b color.RGBA, t float64) color.RGBA {
		l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
		return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
	}
	sx, sy, sr := float64(w)*(0.25+0.5*math.Mod(seed*0.37, 1)), float64(h)*0.34, float64(h)*0.11
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			c := mix(sky0, sky1, fy/float64(h))
			if d := math.Hypot(fx-sx, fy-sy); d < sr*3 {
				t := math.Max(0, 1-d/(sr*3))
				c = mix(c, sun, t*t*0.8)
				if d < sr {
					c = sun
				}
			}
			ridge0 := float64(h)*0.62 + math.Sin(fx/float64(w)*5+seed)*float64(h)*0.06
			ridge1 := float64(h)*0.76 + math.Sin(fx/float64(w)*7+seed*2)*float64(h)*0.05
			if fy > ridge0 {
				c = hill0
			}
			if fy > ridge1 {
				c = hill1
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

type shotScene struct {
	name                    string
	w, h                    int
	sky0, sky1, sun, h0, h1 color.RGBA
}

var shotScenes = []shotScene{
	{"alps.jpg", 1600, 1067, color.RGBA{96, 156, 232, 255}, color.RGBA{214, 232, 250, 255}, color.RGBA{255, 244, 214, 255}, color.RGBA{86, 124, 150, 255}, color.RGBA{38, 66, 82, 255}},
	{"dusk.jpg", 1200, 1600, color.RGBA{58, 52, 120, 255}, color.RGBA{250, 160, 120, 255}, color.RGBA{255, 226, 170, 255}, color.RGBA{86, 54, 96, 255}, color.RGBA{36, 28, 56, 255}},
	{"meadow.jpg", 1600, 1000, color.RGBA{132, 200, 232, 255}, color.RGBA{236, 246, 226, 255}, color.RGBA{255, 250, 220, 255}, color.RGBA{120, 176, 110, 255}, color.RGBA{64, 120, 74, 255}},
	{"harbor.jpg", 1600, 1200, color.RGBA{236, 170, 140, 255}, color.RGBA{252, 226, 190, 255}, color.RGBA{255, 250, 236, 255}, color.RGBA{96, 128, 160, 255}, color.RGBA{54, 82, 112, 255}},
	{"dunes.jpg", 1500, 1000, color.RGBA{250, 206, 150, 255}, color.RGBA{254, 240, 214, 255}, color.RGBA{255, 255, 240, 255}, color.RGBA{214, 150, 96, 255}, color.RGBA{168, 104, 66, 255}},
	{"fjord.jpg", 1000, 1500, color.RGBA{150, 190, 214, 255}, color.RGBA{226, 238, 244, 255}, color.RGBA{250, 252, 255, 255}, color.RGBA{70, 110, 120, 255}, color.RGBA{30, 58, 70, 255}},
	{"lagoon.jpg", 1600, 1067, color.RGBA{70, 190, 200, 255}, color.RGBA{214, 244, 236, 255}, color.RGBA{255, 252, 224, 255}, color.RGBA{60, 150, 150, 255}, color.RGBA{28, 96, 110, 255}},
	{"night.jpg", 1600, 1000, color.RGBA{16, 22, 56, 255}, color.RGBA{66, 82, 140, 255}, color.RGBA{236, 238, 255, 255}, color.RGBA{30, 40, 80, 255}, color.RGBA{12, 16, 38, 255}},
	{"orchard.jpg", 1200, 1200, color.RGBA{170, 214, 150, 255}, color.RGBA{250, 244, 196, 255}, color.RGBA{255, 252, 214, 255}, color.RGBA{138, 180, 90, 255}, color.RGBA{86, 130, 60, 255}},
	{"glacier.jpg", 1600, 1067, color.RGBA{120, 170, 230, 255}, color.RGBA{232, 244, 255, 255}, color.RGBA{255, 255, 255, 255}, color.RGBA{190, 214, 236, 255}, color.RGBA{110, 150, 190, 255}},
	{"canyon.jpg", 1100, 1500, color.RGBA{236, 130, 96, 255}, color.RGBA{252, 206, 150, 255}, color.RGBA{255, 240, 200, 255}, color.RGBA{176, 78, 58, 255}, color.RGBA{112, 46, 40, 255}},
	{"bay.jpg", 1600, 1000, color.RGBA{84, 150, 214, 255}, color.RGBA{226, 238, 250, 255}, color.RGBA{255, 246, 214, 255}, color.RGBA{64, 138, 160, 255}, color.RGBA{30, 84, 112, 255}},
}

func shotApp() *appState {
	a := &appState{Chosen: -1, LargeIndex: -1, Cursor: -1, Thumb: 200, Root: "/Users/me/Pictures/Trips", LayoutMode: 3}
	a.Profiles = prefsFile{View: defaultViewPreferences(), ViewSet: true}
	for i, s := range shotScenes {
		sc := scenery(560, int(560*float64(s.h)/float64(s.w)), s.sky0, s.sky1, s.sun, s.h0, s.h1, float64(i)+1)
		b := sc.Bounds()
		it := mediaItem{
			Path: filepath.Join(a.Root, s.name), Name: s.name, Ext: "jpg", Size: 2_400_000 + int64(i)*91_000,
			InfoLoaded: true, Info: mediaInfo{Width: s.w, Height: s.h}, Decoded: true, ModTime: int64(1_700_000_000 + i*86400),
			Bitmap: ui.NewBitmap(sc),
		}
		_ = b
		a.Items = append(a.Items, it)
		a.Visible = append(a.Visible, i)
	}
	return a
}

func saveShot(t *testing.T, dir, name string, img image.Image) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestReadmeScreenshots renders the pictures in the README from the real
// views. It only runs when LW_README_SHOTS names an output folder:
//
//	LW_README_SHOTS=$PWD/docs/screenshots go test -run TestReadmeScreenshots ./src
func TestReadmeScreenshots(t *testing.T) {
	dir := os.Getenv("LW_README_SHOTS")
	if dir == "" {
		t.Skip("set LW_README_SHOTS to render the README screenshots")
	}
	t.Setenv("HOME", t.TempDir())
	frames := func(te *ui.Tester) {
		for i := 0; i < 4; i++ {
			te.Frame()
		}
	}

	// Browser: justified rows, with a few files selected.
	a := shotApp()
	te := ui.NewTester(a.view, 1280, 800)
	frames(te)
	for _, i := range []int{1, 2, 5} {
		a.Selected.Add(a.Items[i].Path)
	}
	frames(te)
	saveShot(t, dir, "browser.png", te.Image())

	// Browser in dark mode.
	te.SetDark(true)
	frames(te)
	saveShot(t, dir, "browser-dark.png", te.Image())

	// Large view with the floating control capsule.
	b := shotApp()
	b.InLargeView, b.LargeIndex, b.Chosen, b.Zoom = true, 0, 0, 1
	big := shotScenes[0]
	pic := scenery(1600, int(1600*float64(big.h)/float64(big.w)), big.sky0, big.sky1, big.sun, big.h0, big.h1, 1)
	b.Items[0].Large = ui.NewBitmap(pic)
	tl := ui.NewTester(b.view, 1280, 800)
	frames(tl)
	saveShot(t, dir, "viewer.png", tl.Image())

	// Settings: the browse page with the control bar choice.
	s := newSettingsTestState(t)
	s.setPage("browse")
	ts := ui.NewTester(func(c *ui.Context) { s.settingsBody(c) }, 780, 1000)
	frames(ts)
	saveShot(t, dir, "settings.png", ts.Image())
	fmt.Println("README screenshots written to", dir)
}
