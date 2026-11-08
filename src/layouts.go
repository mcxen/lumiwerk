package main

import (
	"container/heap"

	"github.com/egoist/mygo/ui"
	"slices"
)

// waterfallColumns sizes columns like Finder's icon view: a thumbnail-sized
// tile plus room for its caption, not the whole window divided by the thumb.
func waterfallColumns(viewW, thumb float32) int {
	if thumb < 64 {
		thumb = 64
	}
	if thumb > 512 {
		thumb = 512
	}
	single := thumb + 64
	if single < 160 {
		single = 160
	}
	if viewW <= flowPad*2 {
		return 1
	}
	inner := viewW - flowPad*2
	n := int((inner + flowGap) / (single + flowGap))
	if n < 1 {
		return 1
	}
	return n
}

// justifiedRows packs aspects into rows of about rowH, wrapping at viewW.
func justifiedRows(aspects []float64, viewW, rowH, gap float64) [][]int {
	if rowH < 48 {
		rowH = 48
	}
	if viewW < 100 {
		viewW = 100
	}
	if gap < 0 {
		gap = 0
	}
	var rows [][]int
	var row []int
	var used float64
	for i, ar := range aspects {
		if ar <= 0 {
			ar = 1
		}
		w := ar * rowH
		if len(row) > 0 && used+gap+w > viewW {
			rows = append(rows, row)
			row = nil
			used = 0
		}
		if len(row) > 0 {
			used += gap
		}
		row = append(row, i)
		used += w
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// flowBox is one thumbnail card in content coordinates, before scrolling.
type flowBox struct {
	Index      int
	X, Y, W, H float32
}

const (
	flowPad    float32 = 8
	flowGap    float32 = 6
	flowMargin float32 = 0
)

// flowLabel is the caption band under a card; 0 in 无字模式, where the
// pictures take the room (syncCaption).
var flowLabel float32 = 30

// syncCaption applies 无字模式 to the card geometry, once per change.
func (a *appState) syncCaption() {
	want := float32(30)
	if a.Profiles.View.NoCaption {
		want = 0
	}
	if flowLabel != want {
		flowLabel = want
		a.layoutGen++
	}
}

func (a *appState) toggleCaption() {
	a.Profiles.View.NoCaption = !a.Profiles.View.NoCaption
	savePrefs(a.Profiles)
	a.syncCaption()
}

func (a *appState) visibleAspects() []float32 {
	out := make([]float32, len(a.Visible))
	for i, idx := range a.Visible {
		out[i] = 1
		if idx < 0 || idx >= len(a.Items) {
			continue
		}
		item := &a.Items[idx]
		if item.Info.Width > 0 && item.Info.Height > 0 {
			if ar := float32(item.Info.Width) / float32(item.Info.Height); ar > 0 {
				out[i] = ar
			}
		}
	}
	return out
}

// columnHeightHeap keeps the shortest column at the root. Ties go to the
// lowest column index, the same order a linear scan used to pick.
type columnHeightHeap struct {
	cols    []int
	heights []float32
}

func (h columnHeightHeap) Len() int { return len(h.cols) }
func (h columnHeightHeap) Less(i, j int) bool {
	a, b := h.cols[i], h.cols[j]
	if h.heights[a] != h.heights[b] {
		return h.heights[a] < h.heights[b]
	}
	return a < b
}
func (h columnHeightHeap) Swap(i, j int) { h.cols[i], h.cols[j] = h.cols[j], h.cols[i] }
func (h *columnHeightHeap) Push(x any)   { h.cols = append(h.cols, x.(int)) }
func (h *columnHeightHeap) Pop() any {
	old := h.cols
	n := len(old)
	x := old[n-1]
	h.cols = old[:n-1]
	return x
}

// waterfallGroups appends each item to the current shortest column.
// The score is 1/aspect, the same order the view paints.
func waterfallGroups(aspects []float32, cols int) [][]int {
	if cols < 1 {
		cols = 1
	}
	groups := make([][]int, cols)
	heights := make([]float32, cols)
	h := columnHeightHeap{cols: make([]int, cols), heights: heights}
	for i := range h.cols {
		h.cols[i] = i
	}
	heap.Init(&h)
	for i, ar := range aspects {
		if ar <= 0 {
			ar = 1
		}
		best := h.cols[0]
		groups[best] = append(groups[best], i)
		heights[best] += 1 / ar
		heap.Fix(&h, 0)
	}
	return groups
}

func waterfallColumnWidth(viewW float32, cols int) (flex, heightBasis float32) {
	if cols < 1 {
		cols = 1
	}
	inner := viewW - flowPad*2
	if inner < 48 {
		inner = 48
	}
	flex = (inner - flowGap*float32(cols-1)) / float32(cols)
	heightBasis = flex
	if heightBasis < 48 {
		heightBasis = 48
	}
	return flex, heightBasis
}

// waterfallFrames is the card geometry waterfallView paints.
func waterfallFrames(aspects []float32, viewW, thumb float32) []flowBox {
	cols := waterfallColumns(viewW, thumb)
	flex, basis := waterfallColumnWidth(viewW, cols)
	groups := waterfallGroups(aspects, cols)
	boxes := make([]flowBox, len(aspects))
	cardW := flex
	if cardW < 1 {
		cardW = 1
	}
	for ci, group := range groups {
		y := flowPad
		x := flowPad + float32(ci)*(flex+flowGap)
		for _, i := range group {
			ar := aspects[i]
			if ar <= 0 {
				ar = 1
			}
			if ar < 0.2 {
				ar = 0.2
			}
			h := basis/ar + flowLabel
			boxes[i] = flowBox{Index: i, X: x, Y: y, W: cardW, H: h}
			y += h + flowGap
		}
	}
	return boxes
}

// justifiedFrames is the card geometry justifiedView paints. Rows are packed
// by aspect, then each row is a fixed thumb+label strip with equal cards.
func justifiedFrames(aspects []float32, viewW, thumb float32) []flowBox {
	raw := make([]float64, len(aspects))
	for i, ar := range aspects {
		raw[i] = float64(ar)
		if raw[i] <= 0 {
			raw[i] = 1
		}
	}
	rows := justifiedRows(raw, float64(viewW)-float64(flowPad*2), float64(thumb), float64(flowGap))
	boxes := make([]flowBox, len(aspects))
	rowH := thumb + flowLabel
	cardH := rowH - flowMargin*2
	if cardH < 1 {
		cardH = 1
	}
	inner := viewW - flowPad*2
	y := flowPad
	for _, row := range rows {
		n := len(row)
		if n == 0 {
			continue
		}
		cardW := (inner - flowGap*float32(n-1) - flowMargin*2*float32(n)) / float32(n)
		if cardW < 1 {
			cardW = 1
		}
		for i, idx := range row {
			x := flowPad + flowMargin + float32(i)*(cardW+flowMargin*2+flowGap)
			boxes[idx] = flowBox{Index: idx, X: x, Y: y + flowMargin, W: cardW, H: cardH}
		}
		y += rowH + flowGap
	}
	return boxes
}

func flowHit(boxes []flowBox, x, y float32) (flowBox, bool) {
	best := flowBox{}
	bestD := float32(1e9)
	found := false
	for _, b := range boxes {
		if b.W <= 0 || b.H <= 0 {
			continue
		}
		inside := x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H
		dx, dy := float32(0), float32(0)
		if x < b.X {
			dx = b.X - x
		} else if x > b.X+b.W {
			dx = x - (b.X + b.W)
		}
		if y < b.Y {
			dy = b.Y - y
		} else if y > b.Y+b.H {
			dy = y - (b.Y + b.H)
		}
		d := dx*dx + dy*dy
		if inside {
			d = -1
		}
		if !found || d < bestD {
			best, bestD, found = b, d, true
		}
		if inside {
			return b, true
		}
	}
	return best, found
}

// thumbCardProbe lets tests count cards the flow views build. Tests swap it
// and restore; production never reads it.
var thumbCardProbe = func() {}

// selectionChrome marks a media card chosen or selected. The old loud accent
// frame is gone: a quiet neutral fill, rounded like the thumbnail it wraps,
// keeps the selection legible in both themes without drawing a box.
func selectionChrome(c *ui.Context, el ui.Element, on bool) {
	if !on {
		return
	}
	el.Radius(heroThumbRadius).Background(c.Theme().SurfacePressed)
}

// waterfallView packs items into N columns, each appended to the shortest.
// The canvas is one fixed-height spacer; cards are absolutely positioned
// children built only for the visible window, so a folder of 100k photos
// builds tens of widgets a frame instead of all of them.
// flowOverscan is how many screens the flow layouts build past each edge.
const flowOverscan = 0.5

func (a *appState) waterfallView(c *ui.Context) {
	root := a.Root
	scroll := ui.Scroll(c).Fill().TrackScroll(&a.browseScroll)
	width := float32(1280)
	height := float32(800)
	if bounds := scroll.LayoutBounds(); bounds.W > 1 {
		width = bounds.W
		height = bounds.H
		a.noteBrowseBox(bounds, 0, 0, 0, bounds.W, len(a.Visible))
	}
	boxes := a.cachedWaterfall(width)
	contentH := float32(0)
	for _, b := range boxes {
		if bottom := b.Y + b.H + flowPad; bottom > contentH {
			contentH = bottom
		}
	}
	if contentH < height {
		contentH = height
	}
	a.revealFlow(boxes, height)
	// The scroll window the cards must cover, grown half a screen each
	// way so fast scrolling shows cards instead of gaps; a full screen
	// built and painted three screens of cards every frame.
	top := a.browseScroll.Y - height*flowOverscan
	if top < 0 {
		top = 0
	}
	bottom := a.browseScroll.Y + height*(1+flowOverscan)
	if bottom > contentH {
		bottom = contentH
	}
	scroll.Children(func() {
		ui.Box(c).Width(width).Height(contentH)
		for _, b := range boxes {
			if b.Y+b.H < top || b.Y > bottom {
				continue
			}
			i := b.Index
			item := a.visibleItem(i)
			if item == nil {
				continue
			}
			if !item.InfoLoaded {
				a.requestInfo(a.Visible[i])
			}
			card := ui.Box(c).Absolute().Left(b.X).Top(b.Y).Width(b.W).Height(b.H)
			card.Children(func() {
				a.thumbCard(c, root, i, b.H)
			})
		}
	})
}

// justifiedView packs items into rows, each row stretched to fill the width.
func (a *appState) justifiedView(c *ui.Context) {
	root := a.Root
	scroll := ui.Scroll(c).Fill().TrackScroll(&a.browseScroll)
	width, height := float32(1280), float32(800)
	if bounds := scroll.LayoutBounds(); bounds.W > 1 {
		width, height = bounds.W, bounds.H
		a.noteBrowseBox(bounds, 0, 0, 0, bounds.W, len(a.Visible))
	}
	const gap = float64(flowGap)
	rows := a.cachedJustified(width)
	aspects := a.visibleAspects()
	rowH := float32(a.Thumb) + flowLabel
	stride := rowH + float32(gap)
	if a.Chosen >= 0 && a.revealFrames > 0 {
		for r, row := range rows {
			if slices.Contains(row, a.Chosen) {
				a.revealFlow([]flowBox{{Index: a.Chosen, Y: flowPad + float32(r)*stride, H: rowH}}, height)
				break
			}
		}
	}
	// Rows have one height, so the visible ones follow from the scroll
	// offset: build those plus one screen each way, and stand the rest in
	// with two spacers, as waterfallView does with its canvas.
	first := int((a.browseScroll.Y - height*flowOverscan - flowPad) / stride)
	last := int((a.browseScroll.Y+height*(1+flowOverscan)-flowPad)/stride) + 1
	first = max(0, min(first, len(rows)))
	last = max(first, min(last, len(rows)))
	scroll.Children(func() {
		col := ui.Column(c).FillWidth().Gap(float32(gap)).Padding(flowPad)
		col.Children(func() {
			if first > 0 {
				ui.Box(c).Key("lw-justified-above").Height(float32(first)*stride - float32(gap))
			}
			for _, r := range rows[first:last] {
				widths := justifiedWidths(aspects, r, float64(width-flowPad*2), float64(a.Thumb), gap, r[len(r)-1] == len(a.Visible)-1)
				rr := ui.Row(c).Key(a.itemKey(r[0])).Gap(flowGap).Height(rowH)
				rr.Children(func() {
					for k, i := range r {
						if item := a.visibleItem(i); item != nil && !item.InfoLoaded {
							a.requestInfo(a.Visible[i])
						}
						a.thumbCardWidth(c, root, i, rowH, widths[k])
					}
				})
			}
			if last < len(rows) {
				ui.Box(c).Key("lw-justified-below").Height(float32(len(rows)-last)*stride - float32(gap))
			}
		})
	})
}

func (a *appState) thumbCard(c *ui.Context, root string, i int, height float32) {
	a.thumbCardWidth(c, root, i, height, 0)
}

// justifiedWidths sizes the cards of one row by their aspect ratio, scaled
// so full rows fill viewW exactly; the last row keeps the nominal height.
func justifiedWidths(aspects []float32, row []int, viewW, rowH, gap float64, last bool) []float32 {
	out := make([]float32, len(row))
	var sum float64
	for k, i := range row {
		ar := 1.0
		if i >= 0 && i < len(aspects) && aspects[i] > 0 {
			ar = float64(aspects[i])
		}
		w := ar * rowH
		out[k] = float32(w)
		sum += w
	}
	free := viewW - gap*float64(len(row)-1)
	if sum <= 0 || free <= 0 || (last && sum < free) {
		return out
	}
	scale := free / sum
	for k := range out {
		out[k] = float32(float64(out[k]) * scale)
	}
	return out
}

func (a *appState) thumbCardWidth(c *ui.Context, root string, i int, height, width float32) {
	t := c.Theme()
	item := a.visibleItem(i)
	if item == nil {
		return
	}
	thumbCardProbe()
	a.markSeen(item)
	card := ui.Column(c).Gap(4)
	if width > 0 {
		card.Width(width).Shrink(0)
	} else {
		card.Grow(1)
	}
	if height > 0 {
		card.Height(height)
	}
	if a.isCut(item.Path) {
		card.Opacity(0.4)
	}
	selectionChrome(c, card, a.Chosen == i || a.Selected.Has(item.Path))
	card.Children(func() {
		thumb := ui.Box(c).Grow(1).Background(t.Background).Clip().Justify(ui.Center).AlignItems(ui.Center)
		a.thumbHero(thumb, item.Path)
		thumb.Children(func() {
			paintMediaThumb(c, item, width, ui.Cover)
			a.paintThumbBadges(c, item, a.LayoutMode == 2)
		})
		if flowLabel > 0 {
			ui.Text(c, item.Name).FontSize(11).MaxLines(2).TextAlign(ui.Center)
		}
	})
	a.fileDragSource(card, item.Path)
	a.cardPointer(card, i)
	card.ContextMenu(func(m *ui.Menu) { a.mediaContextMenu(m, i) })
	a.maybeRequestThumb(card, root, i)
}

// paintMediaThumb draws one card's picture. Folder covers use the same
// stacked bitmap in every layout; until that exists the card keeps the
// plain folder icon. Photos keep the layout's own fit.
func paintMediaThumb(c *ui.Context, item *mediaItem, width float32, fit ui.Fit) {
	t := c.Theme()
	if item.IsDir {
		if item.Bitmap != nil {
			img := ui.Image(c, item.Bitmap).Fit(ui.Contain).Grow(1)
			if width > 0 {
				img.Width(width) // the centred box would keep the bitmap's own width
			}
			releaseShownPixels(item)
			return
		}
		ui.Icon(c, dirIcon).FontSize(28).TextColor(t.TextMuted)
		return
	}
	if item.Bitmap != nil {
		// Rounded on the picture itself, so a letterboxed or contained
		// cover (videos are often 16:9 in a square cell) keeps the corners
		// the card's clip gives a picture that fills it.
		img := ui.Image(c, item.Bitmap).Fit(fit).Grow(1).Radius(heroThumbRadius)
		if width > 0 {
			img.Width(width)
		}
		releaseShownPixels(item)
		return
	}
	if item.Failed {
		ui.Icon(c, videoIcon).FontSize(24).TextColor(t.TextMuted)
		return
	}
	ui.Spinner(c)
}
