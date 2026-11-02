package main

import (
	"container/list"
	"sync"
	"time"
	"weak"

	"github.com/egoist/mygo/ui"
)

// thumbBudget is the default bytes of decoded thumbnails the cache keeps.
// A 384px thumbnail decodes to ~0.4 MB, so 128 MiB is hundreds of them;
// the point of a byte budget is that 1024px thumbs cost 8x more and a
// count-based cache hides that.
const thumbBudget = 128 << 20

// cachedThumb is one decoded thumbnail.
type cachedThumb struct {
	key    string
	bitmap *ui.Bitmap
	edge   int
	used   time.Time
	bytes  int
	el     *list.Element
}

// thumbCache keeps decoded thumbnails, up to limit bytes. Entries evicted
// wait in gone as weak pointers while something else still holds them, so
// scrolling back shows thumbnails as they were instead of decoding them
// again. Every method is safe for concurrent use; bitmap access itself
// stays on the main thread as drawing requires.
type thumbCache struct {
	mu    sync.Mutex
	art   map[string]*cachedThumb
	gone  map[string]weak.Pointer[cachedThumb]
	busy  map[string]bool
	ll    *list.List // front = most recently used
	limit int
	size  int
	now   func() time.Time
}

func newThumbCache(limit int) *thumbCache {
	if limit <= 0 {
		limit = thumbBudget
	}
	return &thumbCache{
		art:   map[string]*cachedThumb{},
		gone:  map[string]weak.Pointer[cachedThumb]{},
		busy:  map[string]bool{},
		ll:    list.New(),
		limit: limit,
		now:   time.Now,
	}
}

// Get returns the cached bitmap for key, or nil.
func (c *thumbCache) Get(key string) *ui.Bitmap {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.art[key]; ok {
		t.used = c.now()
		c.ll.MoveToFront(t.el)
		return t.bitmap
	}
	if p, ok := c.gone[key]; ok {
		t := p.Value()
		delete(c.gone, key)
		if t != nil {
			t.used = c.now()
			c.admit(t)
			c.evict()
			return t.bitmap
		}
	}
	return nil
}

// Put stores a decoded thumbnail and evicts to the budget.
func (c *thumbCache) Put(key string, bitmap *ui.Bitmap, edge int) {
	if bitmap == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w, h := bitmap.Size()
	t := &cachedThumb{
		key:    key,
		bitmap: bitmap,
		edge:   edge,
		used:   c.now(),
		bytes:  w * h * 4,
	}
	if old, ok := c.art[key]; ok {
		c.ll.Remove(old.el)
		c.size -= old.bytes
	}
	c.admit(t)
	c.evict()
}

// admit links a fresh entry without evicting; the caller holds mu.
func (c *thumbCache) admit(t *cachedThumb) {
	t.el = c.ll.PushFront(t)
	c.art[t.key] = t
	c.size += t.bytes
}

// evict drops least-recently-used entries until the cache fits the budget,
// down to three quarters of it so it sorts seldom. Evicted entries stay as
// weak pointers while a card still shows them.
func (c *thumbCache) evict() {
	for c.size > c.limit {
		el := c.ll.Back()
		if el == nil {
			break
		}
		t := el.Value.(*cachedThumb)
		c.ll.Remove(el)
		delete(c.art, t.key)
		c.size -= t.bytes
		if t.bitmap != nil {
			c.gone[t.key] = weak.Make(t)
		}
	}
	if len(c.gone) > 1024 {
		for k, p := range c.gone {
			if p.Value() == nil {
				delete(c.gone, k)
			}
		}
	}
}

// Delete forgets one key: the file changed or went away.
func (c *thumbCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.art[key]; ok {
		c.ll.Remove(t.el)
		delete(c.art, t.key)
		c.size -= t.bytes
	}
	delete(c.gone, key)
}

// SetLimit resizes the budget and evicts to it.
func (c *thumbCache) SetLimit(limit int) {
	if limit <= 0 {
		limit = thumbBudget
	}
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	c.evict()
}

// Len reports how many live entries the cache holds.
func (c *thumbCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.art)
}

// Size reports the bytes the live entries take.
func (c *thumbCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.size
}

// Limit reports the byte budget the cache keeps.
func (c *thumbCache) Limit() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limit
}

// Clear drops every live and weak entry; bitmaps still referenced by grid
// items stay alive until the items change, and re-enter nothing here.
func (c *thumbCache) Clear() {
	c.mu.Lock()
	c.art = map[string]*cachedThumb{}
	c.gone = map[string]weak.Pointer[cachedThumb]{}
	c.ll = list.New()
	c.size = 0
	c.mu.Unlock()
}

// Busy claims a decode slot for key so concurrent folder walks decode each
// thumbnail once; it reports whether the claim was won.
func (c *thumbCache) Busy(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy[key] {
		return false
	}
	c.busy[key] = true
	return true
}

// Done releases the decode claim.
func (c *thumbCache) Done(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.busy, key)
}

// releasedTracks remembers which bitmaps already drew once, so
// releaseShownPixels frees their CPU copy exactly once, after the GPU
// renderer (or the CPU renderer's last draw) has taken the pixels.
var releasedTracks = map[*ui.Bitmap]bool{}

// releaseShownPixels drops the CPU pixels of bitmaps that have shown, on
// the main thread. MyGo re-decodes from the bytes DecodeBitmap kept when a
// renderer needs the pixels again, so this only saves memory.
func releaseShownPixels(item *mediaItem) {
	if item == nil || item.Bitmap == nil || releasedTracks[item.Bitmap] {
		return
	}
	releasedTracks[item.Bitmap] = true
	if len(releasedTracks) > 4096 {
		releasedTracks = map[*ui.Bitmap]bool{}
	}
	item.Bitmap.ReleasePixels()
}

// Has reports whether key is cached, without touching its recency.
func (c *thumbCache) Has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.art[key]; ok {
		return true
	}
	if p, ok := c.gone[key]; ok && p.Value() != nil {
		return true
	}
	return false
}
