package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVisibleSharpRegionFitsAndZooms(t *testing.T) {
	region, ok := visibleSharpRegion(4000, 3000, 800, 600, 1, 0, 0)
	if !ok {
		t.Fatal("fit region missing")
	}
	if region.SrcX != 0 || region.SrcY != 0 || region.SrcW != 4000 || region.SrcH != 3000 {
		t.Fatalf("fit source = %d,%d %dx%d", region.SrcX, region.SrcY, region.SrcW, region.SrcH)
	}
	if region.OutW != 1600 || region.OutH != 1200 {
		t.Fatalf("fit output = %dx%d, want 1600x1200", region.OutW, region.OutH)
	}
	if !sharpRegionNeeded(4000, 3000, region) {
		t.Fatal("a fitted large photo should resample")
	}
	if abs32(region.PlaceX) > 0.5 || abs32(region.PlaceY) > 0.5 || abs32(region.PlaceW-800) > 1 || abs32(region.PlaceH-600) > 1 {
		t.Fatalf("fit place = %v,%v %vx%v", region.PlaceX, region.PlaceY, region.PlaceW, region.PlaceH)
	}

	zoomed, ok := visibleSharpRegion(4000, 3000, 800, 600, 1.5, 0, 0)
	if !ok {
		t.Fatal("zoom region missing")
	}
	if zoomed.SrcW >= 4000 || zoomed.SrcH >= 3000 {
		t.Fatalf("zoom should crop, got %dx%d", zoomed.SrcW, zoomed.SrcH)
	}
	if zoomed.PlaceX > 2 || zoomed.PlaceY > 2 || zoomed.PlaceX+zoomed.PlaceW < 798 || zoomed.PlaceY+zoomed.PlaceH < 598 {
		t.Fatalf("zoom place = %v,%v %vx%v", zoomed.PlaceX, zoomed.PlaceY, zoomed.PlaceW, zoomed.PlaceH)
	}
	if !sharpRegionNeeded(4000, 3000, zoomed) {
		t.Fatal("a still-minified zoom should resample")
	}
}

func TestSharpRegionSkipsActualPixels(t *testing.T) {
	region, ok := visibleSharpRegion(4000, 3000, 800, 600, 5, 0, 0)
	if !ok {
		t.Fatal("1:1 region missing")
	}
	if sharpRegionNeeded(4000, 3000, region) {
		t.Fatalf("1:1 should keep the original, region out %dx%d src %dx%d", region.OutW, region.OutH, region.SrcW, region.SrcH)
	}
	if !sharpRegionNeeded(20000, 15000, region) {
		t.Fatal("a capped original should resample the visible window")
	}
}

func TestDecodeSharpRegionColor(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "half.png")
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x < 32 {
				img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	region := sharpRegion{SrcX: 48, SrcY: 16, SrcW: 8, SrcH: 8, OutW: 8, OutH: 8}
	data, err := sharpRegionJPEG(path, -1, region)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := got.At(4, 4).RGBA()
	if r > 0x2000 || b < 0xc000 {
		t.Fatalf("center = r %x b %x, want blue", r, b)
	}
}

func TestQuarterTurnMapsBackToSource(t *testing.T) {
	// 100x60, clockwise 90. Oriented (10,20) comes from source (20,49):
	// the left of the file is the top after the turn.
	sx, sy := sourcePoint(100, 60, 90, false, 10, 20)
	if sx != 20 || sy != 49 {
		t.Fatalf("point = %d,%d", sx, sy)
	}
	x, y, w, h, ok := sourceRect(100, 60, 90, false, 10, 20, 30, 40)
	if !ok || x != 20 || y != 20 || w != 40 || h != 30 {
		t.Fatalf("rect ok=%v %d,%d %dx%d", ok, x, y, w, h)
	}
	// 270 is the other quarter. (2,2) on a 32x16 frame is source (29,2).
	rx, ry := sourcePoint(32, 16, 270, false, 2, 2)
	if rx != 29 || ry != 2 {
		t.Fatalf("270 = %d,%d", rx, ry)
	}
	// Mirror flips the file horizontally before the turn.
	mx, my := sourcePoint(100, 60, 0, true, 0, 5)
	if mx != 99 || my != 5 {
		t.Fatalf("mirror = %d,%d", mx, my)
	}
	// hflip then clockwise 90. Oriented (2,2) is source (29,13) on 32x16.
	fx, fy := sourcePoint(32, 16, 90, true, 2, 2)
	if fx != 29 || fy != 13 {
		t.Fatalf("mirror+90 = %d,%d", fx, fy)
	}
}

func TestRotatedFitRefitsTheWindow(t *testing.T) {
	a := &appState{StageW: 800, StageH: 600, Zoom: 1, Rotation: 90}
	item := &mediaItem{InfoLoaded: true, Info: mediaInfo{Width: 4000, Height: 2000}}
	a.Items = []mediaItem{*item}
	a.Visible = []int{0}
	a.InLargeView = true
	fit := a.fitScale(item)
	// Oriented 2000x4000 into 800x600 is limited by height: 600/4000.
	if abs32(fit-600.0/4000.0) > 0.0001 {
		t.Fatalf("oriented fit = %v", fit)
	}
	scale := a.contentScale(item)
	base := containScale(4000, 2000, 800, 600)
	if abs32(scale-fit/base) > 0.0001 {
		t.Fatalf("content scale = %v, want %v", scale, fit/base)
	}
}

func TestSharpRegionFollowsRotation(t *testing.T) {
	region, ok := sharpRegionFor(4000, 2000, 90, false, 800, 600, 1, 0, 0)
	if !ok {
		t.Fatal("missing rotated region")
	}
	if region.Filter == "" || region.OriW < 2 || region.SrcW < 2 {
		t.Fatalf("region = %+v", region)
	}
	if region.PlaceW > 801 || region.PlaceH > 601 {
		t.Fatalf("place %v x %v", region.PlaceW, region.PlaceH)
	}
}

func TestRotatedSharpJPEGTurnsClockwise(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "half.png")
	// 32x16 keeps each half well clear of JPEG chroma samples.
	// Left is red, right is blue. Clockwise 90 puts red on top.
	const iw, ih = 32, 16
	img := image.NewRGBA(image.Rect(0, 0, iw, ih))
	for y := 0; y < ih; y++ {
		for x := 0; x < iw; x++ {
			if x < iw/2 {
				img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Oriented (0,0)-(8,16) is the bottom of the left half: source
	// (0,8) 16x8. A full-frame crop would hide a reversed mapping.
	sx, sy, sw, sh, ok := sourceRect(iw, ih, 90, false, 0, 0, 8, 16)
	if !ok || sx != 0 || sy != 8 || sw != 16 || sh != 8 {
		t.Fatalf("crop ok=%v %d,%d %dx%d", ok, sx, sy, sw, sh)
	}
	turned := sharpRegion{SrcX: sx, SrcY: sy, SrcW: sw, SrcH: sh, OutW: 8, OutH: 16, Filter: orientFilter(90, false)}
	assertJPEGRed(t, path, turned, 4, 8)

	// The whole turned frame: top red, bottom blue.
	region, ok := sharpRegionFor(iw, ih, 90, false, 16, 32, 1, 0, 0)
	if !ok {
		t.Fatal("region")
	}
	region.OutW, region.OutH = 16, 32
	assertJPEGRed(t, path, region, 8, 4)
	data, err := sharpRegionJPEG(path, -1, region)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	_, _, b, _ := got.At(8, 24).RGBA()
	if b < 0xc000 {
		t.Fatalf("bottom blue %x, region %+v", b, region)
	}

	// 270 takes the top-left from the right side of the file.
	rx, ry, rw, rh, ok := sourceRect(iw, ih, 270, false, 0, 0, 8, 16)
	if !ok || rx != 16 || ry != 0 || rw != 16 || rh != 8 {
		t.Fatalf("270 crop ok=%v %d,%d %dx%d", ok, rx, ry, rw, rh)
	}
	ccw := sharpRegion{SrcX: rx, SrcY: ry, SrcW: rw, SrcH: rh, OutW: 8, OutH: 16, Filter: orientFilter(270, false)}
	data, err = sharpRegionJPEG(path, -1, ccw)
	if err != nil {
		t.Fatal(err)
	}
	got, err = jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	_, _, b, _ = got.At(4, 8).RGBA()
	r, _, _, _ := got.At(4, 8).RGBA()
	if b < 0xc000 || r > 0x2000 {
		t.Fatalf("270 top-left r %x b %x, crop %d,%d %dx%d", r, b, rx, ry, rw, rh)
	}
}

func assertJPEGRed(t *testing.T, path string, region sharpRegion, x, y int) {
	t.Helper()
	data, err := sharpRegionJPEG(path, -1, region)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := got.At(x, y).RGBA()
	if r < 0xc000 || b > 0x2000 {
		t.Fatalf("pixel %d,%d r %x b %x, want red, region %+v size %v", x, y, r, b, region, got.Bounds())
	}
}
