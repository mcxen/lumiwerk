package main

import (
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
)

// Thumbnail lifetime and background work bounds.
//
//   - markSeen stamps an item each frame a view builds its thumbnail.
//   - thumbAbandoned lets a queued decode give up when, by the time it gets
//     a decode slot, the latest frame no longer built the item: fast
//     scrolling no longer leaves a queue of off-screen decodes in front of
//     the cards now on screen. The item is reset so it loads again when it
//     shows.
//   - sweepThumbs drops the bitmap of an item that has been off screen for
//     a while and that the byte-budgeted thumbCache has evicted. Before, every
//     mediaItem kept its bitmap for the life of the folder, so the cache
//     budget bounded nothing; now it bounds the thumbnails held.
//   - requestInfo starts at most one metadata probe per item; views that are
//     drawn every frame used to start one goroutine per item per frame
//     until the first probe answered.

const (
	thumbSweepEvery             = 2 * time.Second
	thumbOffscreen              = 5 * time.Second
	thumbPrefetchMargin float32 = 240
	thumbPrefetchExtra          = 12
)

// beginFrame starts a build. The stamp stays private until finishFrame so
// thumbAbandoned, which compares against the published frame, does not
// treat a still-visible card as scrolled away in the gap before markSeen.
func (a *appState) beginFrame() {
	atomic.StoreInt64(&a.frameStamp, time.Now().UnixNano())
}

// finishFrame publishes the build that just finished.
func (a *appState) finishFrame() {
	stamp := atomic.LoadInt64(&a.frameStamp)
	if stamp == 0 {
		return
	}
	atomic.StoreInt64(&a.frameNow, stamp)
}

func (a *appState) markSeen(item *mediaItem) {
	now := atomic.LoadInt64(&a.frameStamp)
	if now == 0 {
		now = atomic.LoadInt64(&a.frameNow)
	}
	if now == 0 {
		now = time.Now().UnixNano()
	}
	atomic.StoreInt64(&item.Seen, now)
}

func (a *appState) thumbAbandoned(index int, path string, gen int) bool {
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) || a.Items[index].Path != path {
		a.mu.Unlock()
		return true
	}
	seen := atomic.LoadInt64(&a.Items[index].Seen)
	frame := atomic.LoadInt64(&a.frameNow)
	a.mu.Unlock()
	if seen == 0 || frame == 0 || seen >= frame {
		return false // not drawn by a view yet (large view), or on screen
	}
	a.update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if index < len(a.Items) && a.Items[index].Path == path && a.Items[index].LoadGen == gen {
			a.Items[index].Loading = false
		}
	})
	return true
}

func (a *appState) sweepThumbs(c *ui.Context) {
	now := time.Now()
	if now.Before(a.nextSweep) {
		return
	}
	a.nextSweep = now.Add(thumbSweepEvery)
	a.sweepThumbsAt(now)
}

// sweepThumbsAt returns how many bitmaps it released.
func (a *appState) sweepThumbsAt(now time.Time) int {
	cut := now.Add(-thumbOffscreen).UnixNano()
	released := 0
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.Items {
		item := &a.Items[i]
		if item.Bitmap == nil || item.Loading {
			continue
		}
		seen := atomic.LoadInt64(&item.Seen)
		if seen == 0 || seen > cut || item.Path == a.Hero.path {
			continue
		}
		key := thumbCacheKey(item.Path, item.ThumbEdge)
		if item.IsDir {
			key = item.CoverKey
		}
		if key != "" && thumbLRU.Has(key) {
			continue
		}
		item.Bitmap = nil
		item.Decoded = false
		item.LoadGen++
		if item.IsDir {
			item.CoverKey = ""
		}
		released++
	}
	return released
}

func (a *appState) noteWindowHidden(c *ui.Context) {
	a.windowHidden = c.WindowHidden()
}

// maybeRequestThumb loads a card that is on screen or within the prefetch
// margin. Hidden windows start no new decode.
func (a *appState) maybeRequestThumb(el ui.Element, root string, visIndex int) {
	if a.windowHidden {
		return
	}
	item := a.visibleItem(visIndex)
	if item == nil || item.Decoded || item.Failed || item.Loading {
		return
	}
	if el != (ui.Element{}) && !el.InView() && !el.InViewMargin(thumbPrefetchMargin) {
		return
	}
	a.requestThumb(root, a.Visible[visIndex])
}

func (a *appState) visibleIndexRange() (first, last int) {
	n := len(a.Visible)
	box := a.browseBox
	if n == 0 || box.H < 1 || box.Stride < 1 {
		return 0, -1
	}
	cols := box.Cols
	if cols < 1 {
		cols = 1
	}
	firstRow := int(a.browseScroll.Y / box.Stride)
	if firstRow < 0 {
		firstRow = 0
	}
	rows := int(box.H/box.Stride) + 2
	first = firstRow * cols
	last = first + rows*cols - 1
	if first >= n {
		return 0, -1
	}
	if last >= n {
		last = n - 1
	}
	return first, last
}

// prefetchAround asks for the visible range first, then a few neighbors.
func (a *appState) prefetchAround(root string, first, last, n, extra int) {
	if a.windowHidden || n <= 0 {
		return
	}
	if first < 0 {
		first = 0
	}
	if last >= n {
		last = n - 1
	}
	if last < first {
		return
	}
	for i := first; i <= last; i++ {
		a.maybeRequestThumb(ui.Element{}, root, i)
	}
	end := last + extra
	if end >= n {
		end = n - 1
	}
	for i := last + 1; i <= end; i++ {
		a.maybeRequestThumb(ui.Element{}, root, i)
	}
	start := first - extra
	if start < 0 {
		start = 0
	}
	for i := first - 1; i >= start; i-- {
		a.maybeRequestThumb(ui.Element{}, root, i)
	}
}

func (a *appState) requestInfo(index int) {
	if a.windowHidden {
		return
	}
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) || a.Items[index].InfoLoaded || a.Items[index].InfoLoading {
		a.mu.Unlock()
		return
	}
	a.Items[index].InfoLoading = true
	a.mu.Unlock()
	safeGo("loadInfo", func() { a.loadInfo(index) })
}
