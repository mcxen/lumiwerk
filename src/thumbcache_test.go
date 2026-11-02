package main

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
)

func testBitmap(w, h int) *ui.Bitmap {
	return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, w, h)))
}

// TestThumbCacheEvictsByBytes keeps the cache within its budget: small
// thumbs stay, an oversized run of big ones evicts the oldest first.
func TestThumbCacheEvictsByBytes(t *testing.T) {
	c := newThumbCache(16 << 20) // 16 MiB
	big := testBitmap(1024, 1024)
	w, _ := big.Size()
	if w != 1024 {
		t.Fatalf("bitmap size %d", w)
	}
	// 16 MiB / 4 MiB per 1024px RGBA = 4 entries fit.
	for i := 0; i < 10; i++ {
		c.Put(fmt.Sprintf("k%d", i), big, 1024)
	}
	if c.Size() > c.limit {
		t.Fatalf("size %d over limit %d", c.Size(), c.limit)
	}
	if c.Len() > 5 {
		t.Fatalf("kept %d entries of 4 MiB in a 16 MiB cache", c.Len())
	}
	if c.Get("k9") == nil {
		t.Fatal("newest entry evicted despite budget")
	}
	// Evicted entries may resurrect from gone until the GC runs; after a
	// GC nothing holds them and they are gone for real.
	runtime.GC()
	if c.Get("k0") != nil {
		t.Fatal("oldest entry resurrected after GC")
	}
}

// TestThumbCacheGoneWeakPointer evicted entries come back while something
// still holds the cached entry, and disappear once nothing does.
func TestThumbCacheGoneWeakPointer(t *testing.T) {
	c := newThumbCache(1 << 20)
	c.Put("a", testBitmap(512, 512), 512)
	c.mu.Lock()
	held := c.art["a"]
	c.mu.Unlock()
	// Push it out with newer, bigger entries.
	for i := 0; i < 8; i++ {
		c.Put(fmt.Sprintf("b%d", i), testBitmap(512, 512), 512)
	}
	runtime.GC()
	if got := c.Get("a"); got == nil {
		t.Fatal("held entry did not come back from gone")
	}
	runtime.KeepAlive(held)
	// Once only the cache referenced the entry, eviction drops it for real.
	c2 := newThumbCache(1 << 20)
	c2.Put("x", testBitmap(512, 512), 512)
	for i := 0; i < 8; i++ {
		c2.Put(fmt.Sprintf("y%d", i), testBitmap(512, 512), 512)
	}
	runtime.GC()
	if got := c2.Get("x"); got != nil {
		t.Fatal("unheld entry resurrected after GC")
	}
}

// TestThumbCacheBusyDedupes serializes decodes of one key.
func TestThumbCacheBusyDedupes(t *testing.T) {
	c := newThumbCache(0)
	if !c.Busy("p") {
		t.Fatal("first claim refused")
	}
	if c.Busy("p") {
		t.Fatal("second claim won")
	}
	c.Done("p")
	if !c.Busy("p") {
		t.Fatal("claim not released")
	}
}

// TestThumbCacheConcurrentDecode hammers Get/Put/Delete/Busy from many
// goroutines; -race must stay quiet.
func TestThumbCacheConcurrentDecode(t *testing.T) {
	c := newThumbCache(4 << 20)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("k%d", i%20)
				if c.Get(key) == nil {
					c.Put(key, testBitmap(64, 64), 384)
				}
				if i%10 == 0 {
					c.Delete(key)
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Size() > c.limit {
		t.Fatalf("size %d over limit", c.Size())
	}
}
