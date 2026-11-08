package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func twoFrameGIF(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.RGBA{200, 10, 10, 255}, color.RGBA{10, 10, 200, 255}}
	g := &gif.GIF{LoopCount: 0}
	for i := 0; i < 2; i++ {
		img := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.SetColorIndex(x, y, uint8(i))
			}
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGIFDecoderReturnsFrames(t *testing.T) {
	dec, err := ui.NewGIFDecoder(twoFrameGIF(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dec.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err := dec.Next(); err != nil {
		t.Fatal(err)
	}
}

func TestLargeViewGIFUsesPlayerNotMPV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "loop.gif")
	if err := os.WriteFile(path, twoFrameGIF(t), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: 0, LargeIndex: 0, Thumb: 128, InLargeView: true, Root: dir}
	a.Items = []mediaItem{{Path: path, Name: "loop.gif", Ext: "gif", InfoLoaded: true, Info: mediaInfo{Width: 8, Height: 8}}}
	a.Visible = []int{0}
	te := ui.NewTester(a.view, 640, 480)
	te.Frame()
	te.Frame()
	if a.gifPlayer == nil || a.gifPath != path {
		t.Fatal("large view did not build a GIF player")
	}
	if a.Video != nil {
		t.Fatal("GIF used the mpv video view")
	}
}

func TestGridGIFStaysStill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "loop.gif")
	if err := os.WriteFile(path, twoFrameGIF(t), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appState{Chosen: 0, LargeIndex: -1, Thumb: 128, Root: dir, LayoutMode: 0}
	a.Items = []mediaItem{{Path: path, Name: "loop.gif", Ext: "gif", InfoLoaded: true, Info: mediaInfo{Width: 8, Height: 8}}}
	a.Visible = []int{0}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(96, 96).Children(func() {
			paintMediaThumb(c, &a.Items[0], 96, ui.Cover)
		})
	}, 96, 96)
	te.Frame()
	if a.gifPlayer != nil {
		t.Fatal("grid card started a GIF player")
	}
}

func TestVideoFileDoesNotUseGIFPlayer(t *testing.T) {
	a := &appState{Chosen: 0, LargeIndex: 0, InLargeView: true}
	item := &mediaItem{Path: "/media/clip.mp4", Name: "clip.mp4", Ext: "mp4", IsVideo: true}
	if a.ensureGIF(item) || a.gifPlayer != nil {
		t.Fatal("mp4 took the GIF path")
	}
}
