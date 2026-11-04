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
	thumbSweepEvery = 2 * time.Second
	thumbOffscreen  = 5 * time.Second
)

func (a *appState) markSeen(item *mediaItem) {
	now := atomic.LoadInt64(&a.frameNow)
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

func (a *appState) requestInfo(index int) {
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) || a.Items[index].InfoLoaded || a.Items[index].InfoLoading {
		a.mu.Unlock()
		return
	}
	a.Items[index].InfoLoading = true
	a.mu.Unlock()
	safeGo("loadInfo", func() { a.loadInfo(index) })
}
