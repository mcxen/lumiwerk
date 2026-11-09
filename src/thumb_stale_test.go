package main

import (
	"testing"
)

// A file rewritten under the same name must not show its old thumbnail from
// memory: the rescan sees a new mtime/size, so the old entry must not match.
func TestRewrittenFileMissesOldMemoryThumb(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bm := testThumbBitmap(t)
	a := waterfalTestState(1)
	a.Items[0].Decoded = false
	a.Items[0].ModTime, a.Items[0].Size = 100, 5000
	edge := thumbBand(thumbEdge(a.Thumb))
	old := itemThumbKey(&a.Items[0], edge)
	thumbLRU.Put(old, bm, edge)
	defer thumbLRU.Delete(old)

	a.adoptCachedThumb(&a.Items[0])
	if a.Items[0].Bitmap == nil {
		t.Fatal("unchanged file did not adopt its cached thumbnail")
	}

	// The external rewrite: same path, new time and size, state not carried.
	fresh := []mediaItem{{Path: a.Items[0].Path, Name: "x", Size: 6000, ModTime: 200}}
	carryItemState(a.Items, fresh)
	if fresh[0].Bitmap != nil || fresh[0].Decoded {
		t.Fatal("changed file kept its old thumbnail")
	}
	a.adoptCachedThumb(&fresh[0])
	if fresh[0].Bitmap != nil {
		t.Fatal("changed file adopted the stale memory thumbnail")
	}
	a.mu.Lock()
	a.Items = fresh
	a.Visible = []int{0}
	a.mu.Unlock()
	a.loadThumb(a.Root, 0)
	if fresh[0].Bitmap != nil && fresh[0].Decoded {
		t.Fatal("loadThumb adopted the stale memory thumbnail")
	}
}

func TestForgetThumbDropsEveryBand(t *testing.T) {
	bm := testThumbBitmap(t)
	path := "/probe/forget.png"
	var keys []string
	// Every band thumbBand can return, found by probing instead of listing.
	seen := map[int]bool{}
	for edge := 1; edge <= 2000; edge++ {
		band := thumbBand(edge)
		if seen[band] {
			continue
		}
		seen[band] = true
		k := thumbCacheKey(path, band, 1, 2)
		keys = append(keys, k)
		thumbLRU.Put(k, bm, band)
		defer thumbLRU.Delete(k)
	}
	other := thumbCacheKey("/probe/other.png", 256, 1, 2)
	thumbLRU.Put(other, bm, 256)
	defer thumbLRU.Delete(other)
	if len(keys) < 5 {
		t.Fatalf("only %d bands probed", len(keys))
	}
	forgetThumb(path)
	for _, k := range keys {
		if thumbLRU.Has(k) {
			t.Fatalf("%s survived forgetThumb", k)
		}
	}
	if !thumbLRU.Has(other) {
		t.Fatal("forgetThumb dropped another file's thumbnail")
	}
}

func TestCarryItemStateKeepsCoverKey(t *testing.T) {
	old := []mediaItem{{Path: "/p/d", IsDir: true, Size: 1, ModTime: 2, Decoded: true, CoverKey: "k"}}
	fresh := []mediaItem{{Path: "/p/d", IsDir: true, Size: 1, ModTime: 2}}
	carryItemState(old, fresh)
	if fresh[0].CoverKey != "k" {
		t.Fatalf("CoverKey = %q, want k", fresh[0].CoverKey)
	}
}
