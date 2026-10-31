package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	minSegmentSeconds = 0.08
	editorHandleHit   = float32(14)
	editorMinCrop     = float32(24)
	editorFilmCount   = 12
)

// editSegment is one inclusive export range on the source timeline.
type editSegment struct {
	Start float64
	End   float64
}

// normRect is a crop in fractions of the source frame, origin at the top left.
type normRect struct {
	X, Y, W, H float32
}

type editorBox struct {
	X, Y, W, H float32
}

type editorDrag struct {
	Kind             string
	Active           bool
	OriginX, OriginY float32
	Segment          editSegment
	Crop             normRect
}

// editorState is the large-view trim and crop editor, matching FlowVision's
// timeline plus a pixel crop projected onto the fitted preview.
type editorState struct {
	Open           bool
	Mode           string
	Path           string
	Duration       float64
	FrameW, FrameH int
	Segments       []editSegment
	Selected       int
	Playhead       float64
	CropOn         bool
	Crop           normRect
	Drag           editorDrag
	ViewW, ViewH   float32
	TimelineW      float32
	LastSeek       time.Time
	Thumbs         []*ui.Bitmap
	ThumbsStarted  bool
	Token          int
}

func (a *appState) openVideoEditor(mode, path string) bool {
	if path == "" {
		if item := a.currentItem(); item != nil {
			path = item.Path
		}
	}
	if path == "" || !isVideoFile(extOf(path)) {
		return false
	}
	if !a.InLargeView || a.currentItem() == nil || a.currentItem().Path != path {
		index := -1
		for i, visible := range a.Visible {
			if visible >= 0 && visible < len(a.Items) && a.Items[visible].Path == path {
				index = i
				break
			}
		}
		if index < 0 {
			return false
		}
		a.openLargeImage(index)
	}
	item := a.currentItem()
	if item == nil || !item.IsVideo {
		return false
	}
	if !item.InfoLoaded || item.Info.Width == 0 || item.Info.DurationSec <= 0 {
		item.Info = probeVideo(item.Path)
		item.InfoLoaded = true
	}
	duration := item.Info.DurationSec
	if duration <= minSegmentSeconds {
		return false
	}
	a.editorSeq++
	a.Editor = editorState{
		Open:     true,
		Mode:     mode,
		Path:     item.Path,
		Duration: duration,
		FrameW:   item.Info.Width,
		FrameH:   item.Info.Height,
		Segments: []editSegment{{End: duration}},
		CropOn:   mode == "crop" && item.Info.Width > 1 && item.Info.Height > 1,
		Crop:     normRect{W: 1, H: 1},
		Token:    a.editorSeq,
	}
	a.holdVideo(false)
	a.seekVideo(0)
	a.Editor.Playhead = 0
	return true
}

func (a *appState) closeEditor() {
	a.Editor.Open = false
	a.Editor.Drag.Active = false
}

func (a *appState) editorView(c *ui.Context) {
	t := c.Theme()
	a.tickEditorPlayback()
	a.loadEditorThumbs()
	root := ui.Column(c).Fill().Background(ui.RGB(20, 20, 22))
	root.Children(func() {
		preview := ui.Box(c).Grow(1).Clip().Background(ui.RGB(8, 8, 10))
		if bounds := preview.LayoutBounds(); bounds.W > 1 && bounds.H > 1 {
			a.Editor.ViewW, a.Editor.ViewH = bounds.W, bounds.H
		}
		preview.HandleInput(a.editorPreviewInput)
		preview.Children(func() {
			a.editorPreview(c)
			if a.Editor.CropOn && a.Editor.ViewW > 1 && a.Editor.ViewH > 1 {
				a.editorCropChrome(c)
			}
		})
		a.editorTimeline(c)
		a.editorBar(c, t)
	})
}

func (a *appState) editorPreview(c *ui.Context) {
	a.ensureVideo(a.Editor.Path)
	a.videoLayer(c, false)
}

func (a *appState) editorCropChrome(c *ui.Context) {
	canvas := containBox(a.Editor.ViewW, a.Editor.ViewH, float32(a.Editor.FrameW), float32(a.Editor.FrameH))
	crop := cropDisplay(canvas, a.Editor.Crop)
	shade := ui.RGB(0, 0, 0).Alpha(0.55)
	for _, piece := range shadeBoxes(canvas, crop) {
		if piece.W < 1 || piece.H < 1 {
			continue
		}
		ui.Box(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
			Left(piece.X).Top(piece.Y).Size(piece.W, piece.H).Background(shade)
	}
	ui.Box(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
		Left(crop.X).Top(crop.Y).Size(crop.W, crop.H).Border(2, ui.RGB(255, 255, 255))
	for _, handle := range cropHandles(crop) {
		ui.Box(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
			Left(handle.X).Top(handle.Y).Size(handle.W, handle.H).
			Radius(5).Background(ui.RGB(255, 255, 255)).Border(1, ui.RGB(0, 0, 0))
	}
	pw, ph := cropPixels(a.Editor.FrameW, a.Editor.FrameH, a.Editor.Crop)
	label := ui.Row(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
		Left(crop.X).Top(crop.Y+crop.H+6).Padding(3, 8).Radius(6).Background(ui.RGB(0, 0, 0).Alpha(0.78))
	label.Children(func() {
		ui.Textf(c, "%d×%d", pw, ph).FontSize(11).TextColor(ui.RGB(255, 255, 255))
	})
}

func (a *appState) editorTimeline(c *ui.Context) {
	bar := ui.Box(c).Height(64).FillWidth().Background(ui.RGB(28, 28, 32)).Clip()
	if bounds := bar.LayoutBounds(); bounds.W > 1 {
		a.Editor.TimelineW = bounds.W
	}
	bar.HandleInput(a.editorTimelineInput)
	bar.Children(func() {
		row := ui.Row(c).PassThrough().Fill()
		row.Children(func() {
			for i := 0; i < editorFilmCount; i++ {
				cell := ui.Box(c).PassThrough().Grow(1).Fill()
				var thumb *ui.Bitmap
				if i < len(a.Editor.Thumbs) {
					thumb = a.Editor.Thumbs[i]
				}
				if thumb != nil {
					cell.Children(func() {
						ui.Image(c, thumb).PassThrough().Fit(ui.Cover).Grow(1)
					})
				} else {
					cell.Background(ui.RGB(42, 42, 48))
				}
			}
		})
		width := a.Editor.TimelineW
		if width < 2 || a.Editor.Duration <= 0 {
			return
		}
		for i, seg := range a.Editor.Segments {
			x := timelineX(seg.Start, a.Editor.Duration, width)
			w := timelineX(seg.End, a.Editor.Duration, width) - x
			if w < 2 {
				w = 2
			}
			color := ui.RGB(250, 204, 21).Alpha(0.28)
			if i == a.Editor.Selected {
				color = ui.RGB(250, 204, 21).Alpha(0.45)
			}
			ui.Box(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
				Left(x).Top(0).Size(w, 64).Background(color).Border(2, ui.RGB(250, 204, 21))
		}
		if a.Editor.Selected >= 0 && a.Editor.Selected < len(a.Editor.Segments) {
			seg := a.Editor.Segments[a.Editor.Selected]
			for _, x := range []float32{
				timelineX(seg.Start, a.Editor.Duration, width),
				timelineX(seg.End, a.Editor.Duration, width),
			} {
				ui.Box(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
					Left(x-5).Top(8).Size(10, 48).Radius(4).Background(ui.RGB(250, 204, 21))
			}
		}
		head := timelineX(a.Editor.Playhead, a.Editor.Duration, width)
		ui.Box(c).PassThrough().Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).
			Left(head-1).Top(0).Size(2, 64).Background(ui.RGB(255, 255, 255))
	})
}

func (a *appState) editorBar(c *ui.Context, t *ui.Theme) {
	bar := ui.Row(c).Height(44).Padding(6, 10).Gap(6).AlignItems(ui.Center).Background(t.Surface)
	bar.Children(func() {
		label := "播放"
		playing := a.videoPlaying()
		if playing {
			label = "暂停"
		}
		if ui.Button(c.Key("editor-play"), label).Clicked() {
			a.toggleEditorPlayback()
		}
		seg := a.selectedSegment()
		ui.Textf(c, "%s – %s / %s", shortTime(seg.Start), shortTime(seg.End), shortTime(a.Editor.Duration)).
			FontSize(12).TextColor(t.TextMuted)
		if ui.Button(c.Key("editor-add"), "添加片段").Clicked() {
			a.Editor.Segments, a.Editor.Selected = splitEditSegment(a.Editor.Segments, a.Editor.Selected, a.Editor.Duration)
		}
		if ui.Button(c.Key("editor-del"), "删除").Disabled(len(a.Editor.Segments) < 2).Clicked() {
			a.Editor.Segments, a.Editor.Selected = deleteEditSegment(a.Editor.Segments, a.Editor.Selected)
		}
		a.editorAspects(c)
		ui.Box(c).Grow(1)
		if ui.Button(c.Key("editor-cancel"), "取消").Clicked() {
			a.closeEditor()
		}
		if ui.Button(c.Key("editor-export"), "导出").Clicked() {
			a.exportEditor()
		}
	})
}

func (a *appState) editorAspects(c *ui.Context) {
	options := []struct {
		label string
		ratio float32
		off   bool
	}{
		{label: "不裁剪", off: true},
		{label: "自由", ratio: 0},
		{label: "16:9", ratio: 16.0 / 9},
		{label: "4:3", ratio: 4.0 / 3},
		{label: "1:1", ratio: 1},
		{label: "9:16", ratio: 9.0 / 16},
	}
	for _, option := range options {
		option := option
		button := ui.Button(c.Key("aspect-"+option.label), option.label)
		if button.Clicked() {
			if option.off || a.Editor.FrameW < 2 || a.Editor.FrameH < 2 {
				a.Editor.CropOn = false
				a.Editor.Crop = normRect{W: 1, H: 1}
				continue
			}
			a.Editor.CropOn = true
			if option.ratio == 0 {
				a.Editor.Crop = normRect{W: 1, H: 1}
				continue
			}
			a.Editor.Crop = centeredAspect(a.Editor.FrameW, a.Editor.FrameH, option.ratio)
		}
	}
}

func (a *appState) editorPreviewInput(ev ui.InputEvent) bool {
	if !a.Editor.CropOn || a.Editor.ViewW < 2 || a.Editor.ViewH < 2 {
		return false
	}
	canvas := containBox(a.Editor.ViewW, a.Editor.ViewH, float32(a.Editor.FrameW), float32(a.Editor.FrameH))
	if canvas.W < 2 || canvas.H < 2 {
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 || !canvas.contains(ev.X, ev.Y) {
			return false
		}
		shown := cropDisplay(canvas, a.Editor.Crop)
		kind := cropHit(ev.X, ev.Y, shown, editorHandleHit)
		a.Editor.Drag = editorDrag{
			Kind: kind, Active: true, OriginX: ev.X, OriginY: ev.Y, Crop: a.Editor.Crop,
		}
		return true
	case ui.InputPointerMove:
		drag := a.Editor.Drag
		if !drag.Active {
			return false
		}
		if drag.Kind == "create" {
			a.Editor.Crop = cropFromPoints(drag.OriginX, drag.OriginY, ev.X, ev.Y, canvas)
			return true
		}
		a.Editor.Crop = applyCropDrag(drag.Crop, drag.Kind, ev.X-drag.OriginX, ev.Y-drag.OriginY, canvas.W, canvas.H)
		return true
	case ui.InputPointerUp:
		if !a.Editor.Drag.Active {
			return false
		}
		a.Editor.Drag.Active = false
		return true
	default:
		return false
	}
}

func (a *appState) editorTimelineInput(ev ui.InputEvent) bool {
	width := a.Editor.TimelineW
	if width < 2 || a.Editor.Duration <= 0 {
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		kind, index := a.timelineHit(ev.X)
		if index >= 0 {
			a.Editor.Selected = index
		}
		seg := a.selectedSegment()
		a.Editor.Drag = editorDrag{Kind: kind, Active: true, OriginX: ev.X, Segment: seg}
		at := seg.Start
		if kind == "end" {
			at = seg.End
		} else if kind == "seek" {
			at = timelineTime(ev.X, width, a.Editor.Duration)
		}
		a.editorSeek(at, true)
		return true
	case ui.InputPointerMove:
		drag := a.Editor.Drag
		if !drag.Active || a.Editor.Selected < 0 || a.Editor.Selected >= len(a.Editor.Segments) {
			return false
		}
		delta := float64((ev.X-drag.OriginX)/width) * a.Editor.Duration
		switch drag.Kind {
		case "start", "end":
			a.Editor.Segments[a.Editor.Selected] = dragEdge(drag.Segment, drag.Kind, delta, a.Editor.Duration)
			edge := a.Editor.Segments[a.Editor.Selected].Start
			if drag.Kind == "end" {
				edge = a.Editor.Segments[a.Editor.Selected].End
			}
			a.editorSeek(edge, false)
		case "seek":
			a.editorSeek(timelineTime(ev.X, width, a.Editor.Duration), false)
		}
		return true
	case ui.InputPointerUp:
		if !a.Editor.Drag.Active {
			return false
		}
		a.Editor.Drag.Active = false
		a.editorSeek(a.Editor.Playhead, true)
		return true
	default:
		return false
	}
}

func (a *appState) timelineHit(x float32) (string, int) {
	width := a.Editor.TimelineW
	if a.Editor.Selected >= 0 && a.Editor.Selected < len(a.Editor.Segments) {
		seg := a.Editor.Segments[a.Editor.Selected]
		startX := timelineX(seg.Start, a.Editor.Duration, width)
		endX := timelineX(seg.End, a.Editor.Duration, width)
		if abs32(x-startX) <= editorHandleHit {
			return "start", a.Editor.Selected
		}
		if abs32(x-endX) <= editorHandleHit {
			return "end", a.Editor.Selected
		}
	}
	seconds := timelineTime(x, width, a.Editor.Duration)
	for i := len(a.Editor.Segments) - 1; i >= 0; i-- {
		seg := a.Editor.Segments[i]
		if seconds >= seg.Start && seconds <= seg.End {
			return "seek", i
		}
	}
	return "seek", a.Editor.Selected
}

func (a *appState) selectedSegment() editSegment {
	if a.Editor.Selected < 0 || a.Editor.Selected >= len(a.Editor.Segments) {
		return editSegment{End: a.Editor.Duration}
	}
	return a.Editor.Segments[a.Editor.Selected]
}

func (a *appState) editorSeek(at float64, force bool) {
	if at < 0 {
		at = 0
	}
	if at > a.Editor.Duration {
		at = a.Editor.Duration
	}
	a.Editor.Playhead = at
	if !force && time.Since(a.Editor.LastSeek) < 160*time.Millisecond {
		return
	}
	a.Editor.LastSeek = time.Now()
	if a.videoPlaying() {
		a.holdVideo(false)
	}
	a.seekVideo(at)
}

func (a *appState) toggleEditorPlayback() {
	if a.Video == nil {
		return
	}
	if a.videoPlaying() {
		a.holdVideo(false)
		a.Editor.Playhead = a.Video.Position
		return
	}
	seg := a.selectedSegment()
	if a.Editor.Playhead < seg.Start || a.Editor.Playhead >= seg.End-0.05 {
		a.editorSeek(seg.Start, true)
	}
	a.playVideo()
}

func (a *appState) tickEditorPlayback() {
	if !a.videoPlaying() {
		return
	}
	a.Editor.Playhead = min(a.Editor.Duration, a.playhead())
	seg := a.selectedSegment()
	if a.Editor.Playhead >= seg.End-0.03 {
		a.holdVideo(false)
		a.Editor.Playhead = seg.End
		if a.Video != nil {
			a.Video.clock.seek(seg.End, false, time.Now())
			a.Video.Position = seg.End
		}
	}
}

func (a *appState) loadEditorThumbs() {
	if a.Editor.ThumbsStarted || a.Editor.Path == "" || a.Window == nil {
		return
	}
	a.Editor.ThumbsStarted = true
	path := a.Editor.Path
	duration := a.Editor.Duration
	token := a.Editor.Token
	go func() {
		thumbs := make([]*ui.Bitmap, editorFilmCount)
		for i := 0; i < editorFilmCount; i++ {
			at := 0.05
			if duration > 0 {
				at = duration * (float64(i) + 0.5) / float64(editorFilmCount)
			}
			if bmp, err := videoThumbAt(path, at); err == nil {
				thumbs[i] = bmp
			}
		}
		if a.Window == nil {
			return
		}
		a.Window.Update(func() {
			if !a.Editor.Open || a.Editor.Token != token || a.Editor.Path != path {
				return
			}
			a.Editor.Thumbs = thumbs
		})
	}()
}

func (a *appState) exportEditor() {
	segments := validSegments(a.Editor.Segments, a.Editor.Duration)
	if len(segments) == 0 {
		a.toast("没有可导出的片段。")
		return
	}
	filter, cropped := editorCropFilter(a.Editor.FrameW, a.Editor.FrameH, a.Editor.Crop, a.Editor.CropOn)
	if len(segments) == 1 && segments[0].Start <= 0.05 && a.Editor.Duration-segments[0].End <= 0.05 && !cropped {
		a.toast("片段和画面都没有改动。")
		return
	}
	src := a.Editor.Path
	a.closeEditor()
	a.toast("正在导出…")
	go func() {
		var created []string
		failed := 0
		for i, seg := range segments {
			dest := flowUnique(filepath.Join(filepath.Dir(src), trimOutputName(src, seg.Start, seg.End, i)), false, created)
			if err := exportVideoEdit(src, dest, filter, seg.Start, seg.End); err != nil {
				failed++
				continue
			}
			created = append(created, dest)
		}
		if a.Window == nil {
			return
		}
		a.Window.Update(func() {
			if len(created) == 0 {
				a.toast("视频导出失败。")
				return
			}
			applied := fileOperation{op: opCopy, created: created}
			if inverse, ok := inverseOf(applied); ok {
				a.Undo.record(applied, inverse)
			}
			if failed > 0 {
				a.toast(fmt.Sprintf("已导出 %d 段，%d 段失败", len(created), failed))
			} else {
				a.toast(fmt.Sprintf("已导出 %d 段", len(created)))
			}
			a.KeepLarge = true
			a.PendingLarge = src
			a.refresh()
		})
	}()
}

func containBox(viewW, viewH, frameW, frameH float32) editorBox {
	if viewW <= 1 || viewH <= 1 || frameW <= 1 || frameH <= 1 {
		return editorBox{}
	}
	scale := viewW / frameW
	if viewH/frameH < scale {
		scale = viewH / frameH
	}
	w, h := frameW*scale, frameH*scale
	return editorBox{X: (viewW - w) / 2, Y: (viewH - h) / 2, W: w, H: h}
}

func (b editorBox) contains(x, y float32) bool {
	return x >= b.X && y >= b.Y && x < b.X+b.W && y < b.Y+b.H
}

func cropDisplay(canvas editorBox, crop normRect) editorBox {
	return editorBox{
		X: canvas.X + crop.X*canvas.W,
		Y: canvas.Y + crop.Y*canvas.H,
		W: crop.W * canvas.W,
		H: crop.H * canvas.H,
	}
}

func shadeBoxes(canvas, crop editorBox) []editorBox {
	return []editorBox{
		{X: canvas.X, Y: canvas.Y, W: canvas.W, H: max(0, crop.Y-canvas.Y)},
		{X: canvas.X, Y: crop.Y + crop.H, W: canvas.W, H: max(0, canvas.Y+canvas.H-(crop.Y+crop.H))},
		{X: canvas.X, Y: crop.Y, W: max(0, crop.X-canvas.X), H: crop.H},
		{X: crop.X + crop.W, Y: crop.Y, W: max(0, canvas.X+canvas.W-(crop.X+crop.W)), H: crop.H},
	}
}

func cropHandles(crop editorBox) []editorBox {
	const s = float32(10)
	points := [][2]float32{
		{crop.X, crop.Y}, {crop.X + crop.W/2, crop.Y}, {crop.X + crop.W, crop.Y},
		{crop.X, crop.Y + crop.H/2}, {crop.X + crop.W, crop.Y + crop.H/2},
		{crop.X, crop.Y + crop.H}, {crop.X + crop.W/2, crop.Y + crop.H}, {crop.X + crop.W, crop.Y + crop.H},
	}
	out := make([]editorBox, len(points))
	for i, point := range points {
		out[i] = editorBox{X: point[0] - s/2, Y: point[1] - s/2, W: s, H: s}
	}
	return out
}

func cropHit(x, y float32, rect editorBox, tol float32) string {
	nearLeft := abs32(x-rect.X) <= tol
	nearRight := abs32(x-(rect.X+rect.W)) <= tol
	nearTop := abs32(y-rect.Y) <= tol
	nearBottom := abs32(y-(rect.Y+rect.H)) <= tol
	switch {
	case nearLeft && nearTop:
		return "tl"
	case nearRight && nearTop:
		return "tr"
	case nearLeft && nearBottom:
		return "bl"
	case nearRight && nearBottom:
		return "br"
	case nearLeft:
		return "left"
	case nearRight:
		return "right"
	case nearTop:
		return "top"
	case nearBottom:
		return "bottom"
	case x >= rect.X && y >= rect.Y && x <= rect.X+rect.W && y <= rect.Y+rect.H:
		return "move"
	default:
		return "create"
	}
}

func applyCropDrag(start normRect, kind string, dx, dy, canvasW, canvasH float32) normRect {
	if canvasW < 1 || canvasH < 1 {
		return start
	}
	nx, ny := dx/canvasW, dy/canvasH
	minW, minH := editorMinCrop/canvasW, editorMinCrop/canvasH
	if minW > 0.9 {
		minW = 0.9
	}
	if minH > 0.9 {
		minH = 0.9
	}
	rect := start
	switch kind {
	case "move":
		rect.X = clampFloat32(start.X+nx, 0, 1-start.W)
		rect.Y = clampFloat32(start.Y+ny, 0, 1-start.H)
	case "left", "tl", "bl":
		right := start.X + start.W
		rect.X = clampFloat32(start.X+nx, 0, right-minW)
		rect.W = right - rect.X
	case "right", "tr", "br":
		rect.W = clampFloat32(start.W+nx, minW, 1-start.X)
	}
	switch kind {
	case "top", "tl", "tr":
		bottom := start.Y + start.H
		rect.Y = clampFloat32(start.Y+ny, 0, bottom-minH)
		rect.H = bottom - rect.Y
	case "bottom", "bl", "br":
		rect.H = clampFloat32(start.H+ny, minH, 1-start.Y)
	}
	return rect
}

func cropFromPoints(x0, y0, x1, y1 float32, canvas editorBox) normRect {
	left, right := min(x0, x1), max(x0, x1)
	top, bottom := min(y0, y1), max(y0, y1)
	left = clampFloat32(left, canvas.X, canvas.X+canvas.W)
	right = clampFloat32(right, canvas.X, canvas.X+canvas.W)
	top = clampFloat32(top, canvas.Y, canvas.Y+canvas.H)
	bottom = clampFloat32(bottom, canvas.Y, canvas.Y+canvas.H)
	if right-left < editorMinCrop {
		right = min(canvas.X+canvas.W, left+editorMinCrop)
	}
	if bottom-top < editorMinCrop {
		bottom = min(canvas.Y+canvas.H, top+editorMinCrop)
	}
	return normRect{
		X: (left - canvas.X) / canvas.W,
		Y: (top - canvas.Y) / canvas.H,
		W: (right - left) / canvas.W,
		H: (bottom - top) / canvas.H,
	}
}

func centeredAspect(frameW, frameH int, ratio float32) normRect {
	if frameW < 2 || frameH < 2 || ratio <= 0 {
		return normRect{W: 1, H: 1}
	}
	fw, fh := float32(frameW), float32(frameH)
	pw, ph := fw, fh
	if pw/ph > ratio {
		pw = ph * ratio
	} else {
		ph = pw / ratio
	}
	return normRect{X: (fw - pw) / 2 / fw, Y: (fh - ph) / 2 / fh, W: pw / fw, H: ph / fh}
}

func cropPixels(frameW, frameH int, crop normRect) (int, int) {
	if frameW < 2 || frameH < 2 {
		return 0, 0
	}
	filter, ok := editorCropFilter(frameW, frameH, crop, true)
	if !ok {
		return frameW, frameH
	}
	var w, h, x, y int
	if _, err := fmt.Sscanf(filter, "crop=%d:%d:%d:%d,setsar=1", &w, &h, &x, &y); err != nil {
		return frameW, frameH
	}
	return w, h
}

func editorCropFilter(frameW, frameH int, crop normRect, enabled bool) (string, bool) {
	if !enabled || frameW < 2 || frameH < 2 || crop.W <= 0 || crop.H <= 0 {
		return "", false
	}
	x := int(crop.X*float32(frameW) + 0.5)
	y := int(crop.Y*float32(frameH) + 0.5)
	w := int(crop.W*float32(frameW) + 0.5)
	h := int(crop.H*float32(frameH) + 0.5)
	if x <= 1 && y <= 1 && w >= frameW-2 && h >= frameH-2 {
		return "", false
	}
	filter, err := rectCropFilter(frameW, frameH, x, y, w, h)
	if err != nil {
		return "", false
	}
	return filter, true
}

func timelineTime(x, width float32, duration float64) float64 {
	if width <= 1 || duration <= 0 {
		return 0
	}
	fraction := float64(x / width)
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	return fraction * duration
}

func timelineX(seconds, duration float64, width float32) float32 {
	if duration <= 0 {
		return 0
	}
	fraction := seconds / duration
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	return float32(fraction) * width
}

func dragEdge(seg editSegment, edge string, delta, duration float64) editSegment {
	switch edge {
	case "start":
		seg.Start = min(max(0, seg.Start+delta), seg.End-minSegmentSeconds)
	case "end":
		seg.End = max(min(duration, seg.End+delta), seg.Start+minSegmentSeconds)
	}
	return seg
}

func splitEditSegment(segs []editSegment, index int, duration float64) ([]editSegment, int) {
	if index < 0 || index >= len(segs) {
		return segs, index
	}
	seg := segs[index]
	if seg.End-seg.Start < minSegmentSeconds*2 {
		return segs, index
	}
	mid := (seg.Start + seg.End) / 2
	segs[index].End = mid
	next := editSegment{Start: mid, End: seg.End}
	out := append([]editSegment{}, segs[:index+1]...)
	out = append(out, next)
	out = append(out, segs[index+1:]...)
	_ = duration
	return out, index + 1
}

func deleteEditSegment(segs []editSegment, index int) ([]editSegment, int) {
	if len(segs) < 2 || index < 0 || index >= len(segs) {
		return segs, index
	}
	out := append([]editSegment{}, segs[:index]...)
	out = append(out, segs[index+1:]...)
	if index >= len(out) {
		index = len(out) - 1
	}
	return out, index
}

func validSegments(segs []editSegment, duration float64) []editSegment {
	var out []editSegment
	for _, seg := range segs {
		if seg.Start < 0 {
			seg.Start = 0
		}
		if seg.End > duration {
			seg.End = duration
		}
		if seg.End-seg.Start > 0.05 {
			out = append(out, seg)
		}
	}
	return out
}

func videoThumbAt(path string, at float64) (*ui.Bitmap, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, err
	}
	if at < 0 {
		at = 0
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.3f", at), "-i", path,
		"-frames:v", "1", "-vf", "scale=160:90:force_original_aspect_ratio=increase,crop=160:90",
		"-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "6", "-")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(out)
}
