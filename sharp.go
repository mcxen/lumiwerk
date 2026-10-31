package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const sharpSettle = 400 * time.Millisecond

// sharpPixelScale is FlowVision's extra sample: resample at twice the
// on-screen point size, then stop at the source pixels.
const sharpPixelScale float32 = 2

// sharpRegion is the source crop that currently covers the large view,
// the bitmap to decode for it, and where that bitmap sits in the view.
type sharpRegion struct {
	SrcX, SrcY, SrcW, SrcH int
	OutW, OutH             int
	PlaceX, PlaceY         float32
	PlaceW, PlaceH         float32
	// OriW and OriH are the crop in the orientation on screen. Src is that
	// crop mapped back to the file, which swaps axes after a quarter turn.
	OriW, OriH int
	Filter     string
}

type sharpPlace struct {
	X, Y, W, H float32
}

func (r sharpRegion) place() sharpPlace {
	return sharpPlace{X: r.PlaceX, Y: r.PlaceY, W: r.PlaceW, H: r.PlaceH}
}

func (r sharpRegion) key(path string) string {
	return fmt.Sprintf("%s@%d,%d %dx%d -> %dx%d %s", path, r.SrcX, r.SrcY, r.SrcW, r.SrcH, r.OutW, r.OutH, r.Filter)
}

func containScale(iw, ih int, viewW, viewH float32) float32 {
	if iw <= 0 || ih <= 0 || viewW <= 0 || viewH <= 0 {
		return 0
	}
	fw := viewW / float32(iw)
	fh := viewH / float32(ih)
	if fw < fh {
		return fw
	}
	return fh
}

func evenSize(n int) int {
	if n < 2 {
		return 0
	}
	return n - n%2
}

// visibleSharpRegion maps the fitted, zoomed image back to source pixels.
// Pan and zoom use the same center-origin model as zoomAt.
func visibleSharpRegion(iw, ih int, viewW, viewH, zoom, panX, panY float32) (sharpRegion, bool) {
	if iw < 2 || ih < 2 || viewW < 2 || viewH < 2 || zoom <= 0 {
		return sharpRegion{}, false
	}
	scale := containScale(iw, ih, viewW, viewH) * zoom
	if scale <= 0 {
		return sharpRegion{}, false
	}
	dispW := float32(iw) * scale
	dispH := float32(ih) * scale
	left := (viewW-dispW)/2 + panX
	top := (viewH-dispH)/2 + panY
	x0 := max(float32(0), left)
	y0 := max(float32(0), top)
	x1 := min(viewW, left+dispW)
	y1 := min(viewH, top+dispH)
	if x1-x0 < 2 || y1-y0 < 2 {
		return sharpRegion{}, false
	}
	sx := (x0 - left) / scale
	sy := (y0 - top) / scale
	sw := (x1 - x0) / scale
	sh := (y1 - y0) / scale
	srcX := int(math.Floor(float64(sx)))
	srcY := int(math.Floor(float64(sy)))
	srcR := int(math.Ceil(float64(sx + sw)))
	srcB := int(math.Ceil(float64(sy + sh)))
	if srcX < 0 {
		srcX = 0
	}
	if srcY < 0 {
		srcY = 0
	}
	if srcR > iw {
		srcR = iw
	}
	if srcB > ih {
		srcB = ih
	}
	if srcX%2 == 1 {
		srcX--
	}
	if srcY%2 == 1 {
		srcY--
	}
	if srcR%2 == 1 {
		if srcR < iw {
			srcR++
		} else if srcR > srcX {
			srcR--
		}
	}
	if srcB%2 == 1 {
		if srcB < ih {
			srcB++
		} else if srcB > srcY {
			srcB--
		}
	}
	srcW := srcR - srcX
	srcH := srcB - srcY
	srcW, srcH = evenSize(srcW), evenSize(srcH)
	if srcW < 2 || srcH < 2 || srcX+srcW > iw || srcY+srcH > ih {
		return sharpRegion{}, false
	}
	outW := int(math.Round(float64(float32(srcW) * scale * sharpPixelScale)))
	outH := int(math.Round(float64(float32(srcH) * scale * sharpPixelScale)))
	if outW > srcW {
		outW = srcW
	}
	if outH > srcH {
		outH = srcH
	}
	outW, outH = evenSize(outW), evenSize(outH)
	if outW < 2 || outH < 2 {
		return sharpRegion{}, false
	}
	return sharpRegion{
		SrcX: srcX, SrcY: srcY, SrcW: srcW, SrcH: srcH,
		OutW: outW, OutH: outH,
		PlaceX: left + float32(srcX)*scale,
		PlaceY: top + float32(srcY)*scale,
		PlaceW: float32(srcW) * scale,
		PlaceH: float32(srcH) * scale,
	}, true
}

// sharpRegionNeeded reports whether a settled view should resample. FlowVision
// regenerates when the on-screen size is still below the original. A file
// that was capped at 12000px also resamples a zoomed-in window from the source.
func sharpRegionNeeded(iw, ih int, region sharpRegion) bool {
	fromW, fromH := region.OriW, region.OriH
	if fromW < 2 || fromH < 2 {
		fromW, fromH = region.SrcW, region.SrcH
	}
	if fromW < 2 || fromH < 2 || region.OutW < 2 || region.OutH < 2 {
		return false
	}
	if region.OutW < fromW || region.OutH < fromH {
		return true
	}
	capped := iw > 12000 || ih > 12000
	partial := region.SrcX > 1 || region.SrcY > 1 || region.SrcX+region.SrcW < iw-1 || region.SrcY+region.SrcH < ih-1
	return capped && partial
}

// quarterTurns snaps a large-view rotation to the clockwise quarter FlowVision stores.
func quarterTurns(deg float32) int {
	n := int(math.Round(float64(deg))) % 360
	if n < 0 {
		n += 360
	}
	n = ((n + 45) / 90) * 90
	return n % 360
}

func orientedSize(iw, ih, deg int) (int, int) {
	if deg == 90 || deg == 270 {
		return ih, iw
	}
	return iw, ih
}

// orientFilter is mirror, then clockwise rotation, applied to a source crop.
func orientFilter(deg int, mirror bool) string {
	var parts []string
	if mirror {
		parts = append(parts, "hflip")
	}
	switch deg {
	case 90:
		parts = append(parts, "transpose=1")
	case 180:
		parts = append(parts, "hflip", "vflip")
	case 270:
		parts = append(parts, "transpose=2")
	}
	return strings.Join(parts, ",")
}

// sourcePoint maps one oriented pixel back to the file. The picture is
// mirrored horizontally and then turned clockwise, matching orientFilter
// and ui.Rotation. The inverse undoes the turn first.
func sourcePoint(iw, ih, deg int, mirror bool, ox, oy int) (int, int) {
	var x1, y1 int
	switch deg {
	case 90: // clockwise: the left of the file becomes the top
		x1 = oy
		y1 = ih - 1 - ox
	case 180:
		x1 = iw - 1 - ox
		y1 = ih - 1 - oy
	case 270:
		x1 = iw - 1 - oy
		y1 = ox
	default:
		x1, y1 = ox, oy
	}
	if mirror {
		x1 = iw - 1 - x1
	}
	return x1, y1
}

// sourceRect is the file crop that becomes the oriented rectangle.
func sourceRect(iw, ih, deg int, mirror bool, ox, oy, ow, oh int) (int, int, int, int, bool) {
	if iw < 2 || ih < 2 || ow < 1 || oh < 1 || ox < 0 || oy < 0 {
		return 0, 0, 0, 0, false
	}
	x0, y0 := sourcePoint(iw, ih, deg, mirror, ox, oy)
	x1, y1 := sourcePoint(iw, ih, deg, mirror, ox+ow-1, oy+oh-1)
	sx, sy := x0, y0
	if x1 < sx {
		sx = x1
	}
	if y1 < sy {
		sy = y1
	}
	sw, sh := x0-x1, y0-y1
	if sw < 0 {
		sw = -sw
	}
	if sh < 0 {
		sh = -sh
	}
	sw++
	sh++
	if sx < 0 || sy < 0 || sx+sw > iw || sy+sh > ih || sw < 1 || sh < 1 {
		return 0, 0, 0, 0, false
	}
	return sx, sy, sw, sh, true
}

// sharpRegionFor is the visible crop after the large view's mirror and rotation.
// Upright images stay on the unrotated path.
func sharpRegionFor(iw, ih, deg int, mirror bool, viewW, viewH, zoom, panX, panY float32) (sharpRegion, bool) {
	ow, oh := orientedSize(iw, ih, deg)
	region, ok := visibleSharpRegion(ow, oh, viewW, viewH, zoom, panX, panY)
	if !ok {
		return sharpRegion{}, false
	}
	if deg == 0 && !mirror {
		return region, true
	}
	sx, sy, sw, sh, ok := sourceRect(iw, ih, deg, mirror, region.SrcX, region.SrcY, region.SrcW, region.SrcH)
	if !ok {
		return sharpRegion{}, false
	}
	region.OriW, region.OriH = region.SrcW, region.SrcH
	region.SrcX, region.SrcY, region.SrcW, region.SrcH = sx, sy, sw, sh
	region.Filter = orientFilter(deg, mirror)
	return region, true
}

func upright(deg float32) bool {
	n := int(math.Round(float64(deg))) % 360
	if n < 0 {
		n += 360
	}
	return n == 0
}

func (a *appState) clearSharp() {
	a.sharpToken++
	a.Sharp = nil
	a.SharpKey = ""
	a.SharpPlace = sharpPlace{}
	a.sharpDue = time.Time{}
	a.sharpDueKey = ""
	a.sharpBusy = false
	a.sharpBusyKey = ""
	a.sharpFailKey = ""
}

// prepareSharp waits until zoom and pan have settled, then swaps in a
// Lanczos crop of the visible pixels. Until it arrives the view keeps the
// GPU-scaled bitmap, which is what FlowVision does during the wheel gesture.
func (a *appState) prepareSharp(c *ui.Context, item *mediaItem) bool {
	if item == nil || item.IsDir || (item.IsVideo && a.videoPlaying()) {
		if a.Sharp != nil || a.sharpDueKey != "" || a.sharpBusy {
			a.clearSharp()
		}
		return false
	}
	iw, ih := item.Info.Width, item.Info.Height
	vw, vh := a.largeSize()
	region, ok := sharpRegionFor(iw, ih, quarterTurns(a.Rotation), a.Mirror, vw, vh, a.Zoom, a.PanX, a.PanY)
	if !ok || !sharpRegionNeeded(iw, ih, region) {
		if a.Sharp != nil || a.sharpDueKey != "" || a.sharpBusy {
			a.clearSharp()
		}
		return false
	}
	key := region.key(item.Path)
	if item.IsVideo {
		key = fmt.Sprintf("%s t%.2f", key, a.playhead())
	}
	if a.Sharp != nil && a.SharpKey == key {
		a.SharpPlace = region.place()
		return a.SharpPlace.W >= 2 && a.SharpPlace.H >= 2
	}
	if a.sharpFailKey == key || a.LargeDrag.active {
		return false
	}
	if a.sharpDueKey != key {
		a.sharpToken++
		a.Sharp = nil
		a.SharpKey = ""
		a.sharpDue = time.Now().Add(sharpSettle)
		a.sharpDueKey = key
		a.sharpBusy = false
	}
	if wait := time.Until(a.sharpDue); wait > 0 {
		c.After(wait)
		return false
	}
	if a.Window == nil || (a.sharpBusy && a.sharpBusyKey == key) {
		return false
	}
	a.sharpBusy = true
	a.sharpBusyKey = key
	a.sharpToken++
	token := a.sharpToken
	path := item.Path
	at := -1.0
	if item.IsVideo {
		at = a.playhead()
	}
	go func() {
		bmp, err := decodeSharpRegion(path, at, region)
		if a.Window == nil {
			return
		}
		a.Window.Update(func() {
			if token != a.sharpToken || a.sharpDueKey != key {
				return
			}
			a.sharpBusy = false
			if err != nil || bmp == nil {
				a.sharpFailKey = key
				return
			}
			a.Sharp = bmp
			a.SharpKey = key
			a.SharpPlace = region.place()
		})
	}()
	return false
}

func decodeSharpRegion(path string, at float64, region sharpRegion) (*ui.Bitmap, error) {
	data, err := sharpRegionJPEG(path, at, region)
	if err != nil && region.Filter == "" && at < 0 {
		data, err = sharpRegionSips(path, region)
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(data)
}

func sharpRegionJPEG(path string, at float64, region sharpRegion) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, err
	}
	vf := fmt.Sprintf("crop=%d:%d:%d:%d", region.SrcW, region.SrcH, region.SrcX, region.SrcY)
	if region.Filter != "" {
		vf += "," + region.Filter
	}
	vf += fmt.Sprintf(",scale=%d:%d:flags=lanczos", region.OutW, region.OutH)
	args := []string{"-hide_banner", "-loglevel", "error"}
	if at >= 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", at))
	}
	args = append(args, "-i", path, "-vf", vf, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "2", "-")
	cmd := exec.Command("ffmpeg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("empty sharp region")
	}
	return stdout.Bytes(), nil
}

// sharpRegionSips is the fallback for images ffmpeg cannot decode. Crop and
// resample are separate sips passes; one invocation applies -z too early.
func sharpRegionSips(path string, region sharpRegion) ([]byte, error) {
	dir, err := os.MkdirTemp("", "lumiwerk-sharp-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	cropped := dir + "/crop.png"
	cmd := exec.Command("sips",
		"--cropOffset", strconv.Itoa(region.SrcY), strconv.Itoa(region.SrcX),
		"--cropToHeightWidth", strconv.Itoa(region.SrcH), strconv.Itoa(region.SrcW),
		"-s", "format", "png", path, "--out", cropped)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	scaled := cropped
	if region.OutW != region.SrcW || region.OutH != region.SrcH {
		scaled = dir + "/out.jpg"
		cmd = exec.Command("sips", "-z", strconv.Itoa(region.OutH), strconv.Itoa(region.OutW),
			"-s", "format", "jpeg", "-s", "formatOptions", "92", cropped, "--out", scaled)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
		}
	}
	return os.ReadFile(scaled)
}
