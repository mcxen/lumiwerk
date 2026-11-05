package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"time"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// gestureState tracks a right-button drag for FlowVision-style folder navigation.
type gestureState struct {
	active bool
	x, y   float32
}

const gestureThreshold = 48.0

func (a *appState) nextMediaFolder(delta int) {
	if a.Root == "" {
		return
	}
	parent := filepath.Dir(a.Root)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(parent, e.Name())
		if hasMedia(p) {
			dirs = append(dirs, p)
		}
	}
	for i, p := range dirs {
		if p == a.Root {
			j := i + delta
			if j >= 0 && j < len(dirs) {
				a.openFolder(dirs[j])
			}
			return
		}
	}
}

func hasMedia(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if isMediaFile(ext) {
			return true
		}
	}
	return false
}

const zoomSettle = 400 * time.Millisecond

func (a *appState) nudgeThumb(factor float32) {
	if factor <= 0 || factor == 1 || a.InLargeView {
		return
	}
	before := thumbBand(thumbEdge(a.Thumb))
	next := a.Thumb * float64(factor)
	if next < 64 {
		next = 64
	}
	if next > 512 {
		next = 512
	}
	a.Thumb = next
	a.HUD = fmt.Sprintf("缩略图 %d", int(a.Thumb+0.5))
	a.HUDUntil = time.Now().Add(800 * time.Millisecond)
	a.noteThumbBand(before)
}

func (a *appState) noteThumbBand(before int) {
	if thumbBand(thumbEdge(a.Thumb)) == before {
		return
	}
	// Keep the current pictures on screen. A sharper band is decoded after
	// the pinch stops, instead of blanking the grid mid-gesture.
	a.thumbDue = time.Now().Add(zoomSettle)
}

func (a *appState) settleThumbs(c *ui.Context) {
	if a.thumbDue.IsZero() {
		return
	}
	wait := time.Until(a.thumbDue)
	if wait > 0 {
		c.After(wait)
		return
	}
	a.applyThumbSettle(time.Now())
}

// applyThumbSettle reloads thumbnails that are coarser than the settled
// band. A finer thumbnail is kept when the grid shrinks.
func (a *appState) applyThumbSettle(now time.Time) bool {
	if a.thumbDue.IsZero() || now.Before(a.thumbDue) {
		return false
	}
	a.thumbDue = time.Time{}
	band := thumbBand(thumbEdge(a.Thumb))
	for i := range a.Items {
		item := &a.Items[i]
		if item.IsDir {
			// An empty folder has no bitmap to sharpen. A stack that is
			// already at this band stays; a coarser one is built again.
			if item.Bitmap == nil || item.ThumbEdge >= band {
				continue
			}
		} else if item.Decoded && item.ThumbEdge >= band {
			continue
		}
		item.LoadGen++
		item.Decoded = false
		item.Failed = false
		item.Loading = false
	}
	return true
}

func (a *appState) handleGesture(ev ui.InputEvent) bool {
	if !a.InLargeView && ev.Kind == ui.InputZoom && ev.Scale > 0 && ev.Scale != 1 {
		a.zoomBrowse(ev.Scale, a.columnX+ev.X, a.columnY+ev.Y, true)
		return true
	}
	if !a.InLargeView && ev.Kind == ui.InputScroll && ev.Mods&(ui.Cmd|ui.Ctrl) != 0 {
		delta := ev.DY
		if delta == 0 {
			delta = ev.DX
		}
		factor := wheelZoomFactor(delta, ev.Precise, ev.Momentum)
		if factor != 1 {
			a.zoomBrowse(factor, a.columnX+ev.X, a.columnY+ev.Y, true)
			return true
		}
	}
	// Plain scrolls drive the browse offset directly: MyGo eases a wheel's
	// notches over 150ms, which visibly lags the hand, and precise trackpad
	// deltas follow the fingers best applied untouched.
	if !a.InLargeView && !a.modalOpen() && ev.Kind == ui.InputScroll && a.LayoutMode != 1 &&
		ev.Mods&(ui.Cmd|ui.Ctrl) == 0 && ev.DY != 0 {
		a.scrollBrowseBy(ev.DY)
		return true
	}
	if ev.Button != 1 {
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		a.Gesture = gestureState{active: true, x: ev.X, y: ev.Y}
		return true
	case ui.InputPointerMove:
		if a.Gesture.active {
			return true
		}
	case ui.InputPointerUp:
		if !a.Gesture.active {
			return false
		}
		dx, dy := ev.X-a.Gesture.x, ev.Y-a.Gesture.y
		a.Gesture = gestureState{}
		if abs32(dx) < gestureThreshold && abs32(dy) < gestureThreshold {
			return true
		}
		if abs32(dy) > abs32(dx) {
			if dy < 0 {
				a.goUp()
			} else {
				a.goBack()
			}
		} else {
			if dx > 0 {
				a.nextMediaFolder(1)
			} else {
				a.nextMediaFolder(-1)
			}
		}
		return true
	}
	return false
}

// scrollBrowseBy advances the browse scroll offset by dy in one write, so
// the frame that follows paints the content exactly where the hand left it.
//
// The offset stays on whole DIPs (whole device pixels at 1x and 2x) and
// the fraction carries to the next event: MyGo rounds picture edges to
// device pixels but floors text pens and baselines, so at a fractional
// offset a caption could sit a pixel off its thumbnail and flicker
// between the two as the fraction changed, lagging behind the picture.
func (a *appState) scrollBrowseBy(dy float32) {
	want := a.browseScroll.Y + a.scrollCarry + dy
	y := float32(math.Round(float64(want)))
	a.scrollCarry = want - y
	if y < 0 {
		y = 0
	}
	// Before the first layout MaxY is still 0: leave the clamp to the scroll
	// container then rather than pinning everything to the top.
	if max := a.browseScroll.MaxY; max > 0 && y > max {
		y = max
	}
	a.browseScroll.Y = y
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// browseBox is the scroll surface of the current layout, in the column that
// receives the pinch, plus the grid geometry of the last frame.
type browseBox struct {
	X, Y, W, H float32
	ContentW   float32
	Gap        float32
	Cols       int
	Stride     float32
	Items      int
}

// browseAnchor is a grid scroll to apply on the next frame, in content DIPs.
type browseAnchor struct {
	set    bool
	y      float64
	cols   int
	stride float64
}

func gridColumnCount(viewW, cell, gap float32) int {
	if cell < 1 {
		cell = 1
	}
	if gap < 0 {
		gap = 0
	}
	if viewW < 1 {
		return 1
	}
	n := int(math.Floor(float64((viewW + gap) / (cell + gap))))
	if n < 1 {
		return 1
	}
	return n
}

func gridStride(thumb float64, gap float32) float32 {
	row := float32(int(thumb+gridCellPad) + 30)
	if gap > 0 {
		row += gap
	}
	if row < 1 {
		return 1
	}
	return row
}

// anchoredScroll keeps the content point under the pointer while a uniform
// layout scales from oldSize to newSize.
func anchoredScroll(scroll, pointer, oldSize, newSize float32) float32 {
	if oldSize <= 0 || newSize <= 0 {
		return scroll
	}
	y := (scroll+pointer)*(newSize/oldSize) - pointer
	if y < 0 {
		return 0
	}
	return y
}

// anchoredGridScroll keeps the item under the pointer in place when the
// cell size and the column count both change.
func anchoredGridScroll(scrollY, pointerX, pointerY, viewW, oldStride, newStride float32, oldCols, newCols, n int) float32 {
	if n < 1 || oldStride <= 0 || newStride <= 0 {
		return scrollY
	}
	if oldCols < 1 {
		oldCols = 1
	}
	if newCols < 1 {
		newCols = 1
	}
	if viewW < 1 {
		viewW = 1
	}
	colW := viewW / float32(oldCols)
	if colW < 1 {
		colW = 1
	}
	col := int(pointerX / colW)
	if col < 0 {
		col = 0
	}
	if col >= oldCols {
		col = oldCols - 1
	}
	rowF := (scrollY + pointerY) / oldStride
	row := int(math.Floor(float64(rowF)))
	if row < 0 {
		row = 0
	}
	frac := rowF - float32(row)
	item := row*oldCols + col
	if item >= n {
		item = n - 1
	}
	if item < 0 {
		item = 0
	}
	newRow := item / newCols
	y := float32(newRow)*newStride + frac*newStride - pointerY
	if y < 0 {
		return 0
	}
	return y
}

func (a *appState) zoomBrowse(factor, x, y float32, hasPoint bool) {
	old := a.Thumb
	box := a.browseBox
	a.nudgeThumb(factor)
	a.anchorBrowse(old, a.Thumb, x, y, hasPoint, box)
}

func (a *appState) anchorBrowse(oldThumb, newThumb float64, x, y float32, hasPoint bool, box browseBox) {
	if oldThumb == newThumb || oldThumb <= 0 || box.W < 2 || box.H < 2 {
		return
	}
	localX, localY := box.W/2, box.H/2
	if hasPoint {
		localX = x - box.X
		localY = y - box.Y
	}
	if localX < 0 {
		localX = 0
	}
	if localY < 0 {
		localY = 0
	}
	if localX > box.W {
		localX = box.W
	}
	if localY > box.H {
		localY = box.H
	}
	switch a.LayoutMode {
	case 0:
		gap := box.Gap
		if gap <= 0 {
			gap = 8
		}
		width := box.ContentW
		if width <= 0 {
			width = box.W
		}
		cell := float32(int(newThumb + gridCellPad + 8))
		cols := gridColumnCount(width, cell, gap)
		oldStride := box.Stride
		if oldStride <= 0 {
			oldStride = gridStride(oldThumb, gap)
		}
		yScroll := anchoredGridScroll(a.browseScroll.Y, localX, localY, width, oldStride, gridStride(newThumb, gap), box.Cols, cols, box.Items)
		a.browseAnchor = browseAnchor{set: true, y: float64(yScroll), cols: cols, stride: float64(gridStride(newThumb, gap))}
	case 2:
		aspects := a.visibleAspects()
		oldBoxes := waterfallFrames(aspects, box.W, float32(oldThumb))
		newBoxes := waterfallFrames(aspects, box.W, float32(newThumb))
		if y, ok := anchoredFlowScroll(oldBoxes, newBoxes, a.browseScroll.X, a.browseScroll.Y, localX, localY); ok {
			a.browseScroll.Y = y
		}
	case 3:
		aspects := a.visibleAspects()
		oldBoxes := justifiedFrames(aspects, box.W, float32(oldThumb))
		newBoxes := justifiedFrames(aspects, box.W, float32(newThumb))
		if y, ok := anchoredFlowScroll(oldBoxes, newBoxes, a.browseScroll.X, a.browseScroll.Y, localX, localY); ok {
			a.browseScroll.Y = y
		}
	}
}

// anchoredFlowScroll keeps the card under the pointer. A uniform scale is
// wrong once a waterfall or justified row reflows into a different count.
func anchoredFlowScroll(oldBoxes, newBoxes []flowBox, scrollX, scrollY, localX, localY float32) (float32, bool) {
	hit, ok := flowHit(oldBoxes, scrollX+localX, scrollY+localY)
	if !ok || hit.H <= 0 {
		return scrollY, false
	}
	var nxt flowBox
	found := false
	for _, b := range newBoxes {
		if b.Index == hit.Index {
			nxt, found = b, true
			break
		}
	}
	if !found || nxt.H <= 0 {
		return scrollY, false
	}
	frac := (scrollY + localY - hit.Y) / hit.H
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	y := nxt.Y + frac*nxt.H - localY
	if y < 0 {
		return 0, true
	}
	return y, true
}

func (a *appState) noteBrowseBox(b ui.Rect, cols int, stride, gap, contentW float32, items int) {
	if b.W < 2 || b.H < 2 {
		return
	}
	a.browseBox = browseBox{X: b.X, Y: b.Y, W: b.W, H: b.H, ContentW: contentW, Gap: gap, Cols: cols, Stride: stride, Items: items}
}

func (a *appState) applyGridAnchor() {
	if !a.browseAnchor.set {
		return
	}
	anchor := a.browseAnchor
	a.browseAnchor = browseAnchor{}
	setGridScrollRequest(&a.Grid, anchor.y, anchor.cols, anchor.stride)
}

func gridField(v reflect.Value, name string) reflect.Value {
	f := v.FieldByName(name)
	if !f.IsValid() {
		panic("lumiwerk: missing grid field " + name)
	}
	return f
}

func setIntField(f reflect.Value, n int) {
	*(*int)(unsafe.Pointer(f.UnsafeAddr())) = n
}

func setBoolField(f reflect.Value, on bool) {
	*(*bool)(unsafe.Pointer(f.UnsafeAddr())) = on
}

func setFloatField(f reflect.Value, n float64) {
	*(*float64)(unsafe.Pointer(f.UnsafeAddr())) = n
}

func nilSliceField(f reflect.Value) {
	type sliceHeader struct {
		data unsafe.Pointer
		len  int
		cap  int
	}
	*(*sliceHeader)(unsafe.Pointer(f.UnsafeAddr())) = sliceHeader{}
}

func gridRowWidth(grid *ui.GridState) float32 {
	rows := gridField(reflect.ValueOf(grid).Elem(), "rows")
	return *(*float32)(unsafe.Pointer(gridField(rows, "width").UnsafeAddr()))
}

// setGridScrollRequest asks the grid's list to land on y and stops it from
// pinning the old first row when the column count changes. The measured row
// heights belong to the previous cell size, so they are dropped.
func setGridScrollRequest(grid *ui.GridState, y float64, cols int, stride float64) {
	root := reflect.ValueOf(grid).Elem()
	setIntField(gridField(root, "cols"), cols)
	rows := gridField(root, "rows")
	req := gridField(rows, "req")
	base := unsafe.Pointer(req.UnsafeAddr())
	*(*bool)(base) = true
	*(*bool)(unsafe.Add(base, 1)) = false
	*(*bool)(unsafe.Add(base, 2)) = false
	*(*bool)(unsafe.Add(base, 3)) = true
	*(*bool)(unsafe.Add(base, 4)) = false
	*(*int)(unsafe.Add(base, 8)) = 0
	*(*float64)(unsafe.Add(base, 24)) = y
	setBoolField(gridField(rows, "scrolled"), true)
	heights := gridField(rows, "heights")
	setFloatField(gridField(heights, "def"), stride)
	setBoolField(gridField(heights, "dirty"), true)
	nilSliceField(gridField(heights, "ids"))
	nilSliceField(gridField(heights, "blocks"))
}
