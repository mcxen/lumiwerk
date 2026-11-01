package main

import (
	"container/list"
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"
)

const thumbnailCacheLimit = 512

// lruCache stores decoded thumbnails keyed by source path.
type lruCache struct {
	limit int
	ll    *list.List
	items map[string]*list.Element
}

type lruEntry struct {
	key    string
	bitmap *ui.Bitmap
}

func newLRUCache(limit int) *lruCache {
	if limit <= 0 {
		limit = thumbnailCacheLimit
	}
	return &lruCache{limit: limit, ll: list.New(), items: make(map[string]*list.Element, limit)}
}

func (c *lruCache) Get(key string) *ui.Bitmap {
	el, ok := c.items[key]
	if !ok {
		return nil
	}
	c.ll.MoveToFront(el)
	return el.Value.(lruEntry).bitmap
}

func (c *lruCache) Put(key string, bitmap *ui.Bitmap) {
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value = lruEntry{key: key, bitmap: bitmap}
		return
	}
	c.items[key] = c.ll.PushFront(lruEntry{key: key, bitmap: bitmap})
	for c.ll.Len() > c.limit {
		el := c.ll.Back()
		if el == nil {
			return
		}
		delete(c.items, el.Value.(lruEntry).key)
		c.ll.Remove(el)
	}
}

func (c *lruCache) Delete(key string) {
	el, ok := c.items[key]
	if !ok {
		return
	}
	delete(c.items, key)
	c.ll.Remove(el)
}

// thumbEdge is the longest side of a grid thumbnail. It tracks the slider
// but stays small enough that a folder of photos does not decode originals.
func thumbEdge(slider float64) int {
	edge := int(slider * 2)
	if edge < 384 {
		edge = 384
	}
	if edge > 1024 {
		edge = 1024
	}
	return edge
}

// thumbBand keeps nearby slider positions on one decode size so pinch-zoom
// does not rebuild every thumbnail for a one-pixel change.
func thumbBand(edge int) int {
	switch {
	case edge < 448:
		return 384
	case edge < 640:
		return 512
	case edge < 896:
		return 768
	default:
		return 1024
	}
}

func thumbCacheKey(path string, edge int) string {
	return fmt.Sprintf("%s#thumb#%d", path, thumbBand(edge))
}

// thumbBytes builds a JPEG thumbnail. Images go through the vfs pipeline
// (sips on macOS, in-process Go elsewhere); videos take one scaled frame.
// Callers fall back to the original bytes if this fails.
func thumbBytes(path string, video bool, edge int) ([]byte, error) {
	return mediaThumbJPEG(path, video, edge)
}

// decodeLargeBitmap decodes the original for 1:1 zoom. Images larger than
// 12000px on a side are resampled so one photo cannot exhaust memory.
func decodeLargeBitmap(path string) (*ui.Bitmap, error) {
	data, err := decodeLargeImage(path)
	if err != nil {
		return nil, err
	}
	if bmp, err := ui.DecodeBitmap(data); err == nil {
		return bmp, nil
	}
	data, err = imageResampleJPEG(path, 12000, "82")
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(data)
}

// _ keeps os available for tests that embed this file's helpers.
var _ = os.Getpid
