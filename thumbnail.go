package main

import (
	"container/list"

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
