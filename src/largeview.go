package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// dragState tracks an in-progress pointer drag in the large view.
type dragState struct {
	active         bool
	startX, startY float32
	lastX, lastY   float32
	moved          bool
	fired          bool
	button         int
	time           time.Time
}

// transform composes pan, zoom, rotation and mirror for the large view media.
// fitTransform keeps rotation and mirror, but does not enlarge a thumbnail.
func (a *appState) fitTransform() ui.Transform {
	var t ui.Transform
	if a.Mirror {
		t = ui.Scaling(-1, 1)
	}
	return t.Then(ui.Rotation(a.Rotation))
}

func (a *appState) transform() ui.Transform {
	// Mirror and rotation happen before zoom. Scale, then pan, so the
	// content under the pointer stays put. Translation-before-scale does not.
	scale := a.contentScale(a.currentItem())
	return a.fitTransform().Then(ui.Scaling(scale, scale)).Then(ui.Translation(a.PanX, a.PanY))
}

// contentScale refits after a quarter turn. Fit(Contain) sizes the unrotated
// bitmap; FlowVision swaps the sides first, so a wide photo turned on its
// side fits the window again instead of sticking out of it.
func (a *appState) contentScale(item *mediaItem) float32 {
	if item == nil || a.Zoom <= 0 {
		return a.Zoom
	}
	bw, bh := a.largeSize()
	base := containScale(item.Info.Width, item.Info.Height, bw, bh)
	if base <= 0 {
		return a.Zoom
	}
	ow, oh := orientedSize(item.Info.Width, item.Info.Height, quarterTurns(a.Rotation))
	want := containScale(ow, oh, bw, bh)
	if want <= 0 {
		return a.Zoom
	}
	return a.Zoom * want / base
}

func (a *appState) largeSize() (float32, float32) {
	if a.StageW > 1 && a.StageH > 1 {
		return a.StageW, a.StageH
	}
	if a.Window == nil {
		return 1280, 760
	}
	w, h := a.Window.ContentSize()
	if w <= 0 || h <= 0 {
		return 1280, 760
	}
	// The media box sits below the tab/toolbar rows and above the status bar.
	return float32(w), float32(h) - 90
}

// zoomAt scales around a cursor position, keeping the content under the cursor.
func (a *appState) zoomAt(cx, cy, factor float32) {
	w, h := a.largeSize()
	localX := cx - w/2
	localY := cy - h/2
	next := a.clampZoom(a.Zoom * factor)
	if next == a.Zoom {
		return
	}
	ratio := next / a.Zoom
	a.PanX = localX - ratio*(localX-a.PanX)
	a.PanY = localY - ratio*(localY-a.PanY)
	a.Zoom = next
	a.clampPan()
	a.showZoomHUD()
	a.noteVideoZoom()
}

func (a *appState) zoomStep(factor float32) {
	w, h := a.largeSize()
	a.zoomAt(w/2, h/2, factor)
}

// resetZoom fits the media to the window.
func (a *appState) resetZoom() {
	a.Zoom, a.PanX, a.PanY = 1, 0, 0
	a.showZoomHUD()
	a.noteVideoZoom()
}

// zoom100 shows the media at its original pixel scale, centered.
func (a *appState) zoom100() {
	w, h := a.largeSize()
	a.zoom100At(w/2, h/2)
}

// zoom100At keeps the point under the cursor while reaching original pixels.
func (a *appState) zoom100At(cx, cy float32) {
	item := a.currentItem()
	if item == nil {
		return
	}
	if a.Zoom <= 0 {
		a.Zoom = 1
	}
	fit := a.fitScale(item)
	if fit <= 0 {
		return
	}
	a.zoomAt(cx, cy, a.clampZoom(1/fit)/a.Zoom)
}

func (a *appState) clampPan() {
	w, h := a.largeSize()
	limitX := w * max(a.Zoom, 1)
	limitY := h * max(a.Zoom, 1)
	a.PanX = clampFloat32(a.PanX, -limitX, limitX)
	a.PanY = clampFloat32(a.PanY, -limitY, limitY)
}

// tickLongPress applies FlowVision's half-second hold: left reaches 100%
// under the pointer, right fits the window.
func (a *appState) tickLongPress(c *ui.Context, item *mediaItem) {
	if !a.LargeDrag.active || a.LargeDrag.moved || a.LargeDrag.fired {
		return
	}
	wait := 500*time.Millisecond - time.Since(a.LargeDrag.time)
	if wait > 0 {
		c.After(wait)
		return
	}
	a.LargeDrag.fired = true
	if a.LargeDrag.button == 1 {
		a.resetZoom()
		return
	}
	a.zoom100At(a.LargeDrag.startX, a.LargeDrag.startY)
	_ = item
}

// clampZoom keeps the rendered media between one half and ten times its
// original width, matching FlowVision's large-view zoom limits.
func (a *appState) clampZoom(z float32) float32 {
	item := a.currentItem()
	if item == nil {
		return clampFloat32(z, 0.1, 8)
	}
	fit := a.fitScale(item)
	if fit <= 0 {
		return clampFloat32(z, 0.1, 8)
	}
	return clampFloat32(z, 0.5/fit, 10/fit)
}

// fitScale is the scale Fit(Contain) applies to the original pixels.
func (a *appState) fitScale(item *mediaItem) float32 {
	if item == nil {
		return 0
	}
	bw, bh := a.largeSize()
	ow, oh := orientedSize(item.Info.Width, item.Info.Height, quarterTurns(a.Rotation))
	return containScale(ow, oh, bw, bh)
}

func (a *appState) zoomPercent(item *mediaItem) int {
	fit := a.fitScale(item)
	if fit <= 0 {
		return int(a.Zoom*100 + 0.5)
	}
	return int(a.Zoom*fit*100 + 0.5)
}

func (a *appState) showZoomHUD() {
	item := a.currentItem()
	if item == nil {
		a.HUD = ""
		return
	}
	a.HUD = fmt.Sprintf("缩放: %d%%", a.zoomPercent(item))
	a.HUDUntil = time.Now().Add(1200 * time.Millisecond)
}

func (a *appState) toggleVideoPlayback() {
	if a.videoPlaying() {
		a.holdVideo(true)
		return
	}
	a.playVideo()
}

func (a *appState) requestSharpFrame() {
	if a.Video == nil || a.Video.path == "" {
		return
	}
	path := a.Video.path
	at := a.playhead()
	a.Video.stillToken++
	token := a.Video.stillToken
	safeGo("largeview.go:212", func() {
		img, err := grabVideoStill(path, at)
		if err != nil || a.Window == nil {
			return
		}
		bitmap := ui.NewBitmap(img)
		a.update(func() {
			if a.Video == nil || a.Video.stillToken != token || a.Video.path != path {
				return
			}
			if a.videoPlaying() {
				return
			}
			a.Video.Still = bitmap
		})
	})
}

func (a *appState) videoDuration() float64 {
	if secs, ok := a.knownDuration(); ok {
		return secs
	}
	return 1
}

func (a *appState) knownDuration() (float64, bool) {
	item := a.currentItem()
	if item == nil || item.Info.DurationSec <= 0 {
		return 0, false
	}
	return item.Info.DurationSec, true
}

// ratingFor reads a 1-5 rating from Finder tags such as "评分3" or "3 星".
func ratingFor(item *mediaItem) int {
	if item == nil {
		return 0
	}
	for _, tag := range item.Tags {
		if !isRatingTag(tag) {
			continue
		}
		for _, r := range tag {
			if r >= '1' && r <= '5' {
				return int(r - '0')
			}
		}
	}
	return 0
}

// tagColor maps Finder tag names to their system colors.
func tagColor(tag string) ui.Color {
	switch {
	case strings.Contains(tag, "红"):
		return ui.RGB(230, 69, 55)
	case strings.Contains(tag, "橙"):
		return ui.RGB(245, 166, 35)
	case strings.Contains(tag, "黄"):
		return ui.RGB(253, 216, 53)
	case strings.Contains(tag, "绿"):
		return ui.RGB(60, 180, 74)
	case strings.Contains(tag, "蓝"):
		return ui.RGB(41, 121, 255)
	case strings.Contains(tag, "紫"):
		return ui.RGB(150, 90, 200)
	case strings.Contains(tag, "灰"):
		return ui.RGB(142, 142, 147)
	}
	return ui.RGB(0, 122, 255)
}
