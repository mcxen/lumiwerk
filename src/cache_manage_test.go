package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDiskThumbStoreClearReclaimsFileAndSetLimitEvicts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dataDir := t.TempDir()
	f1 := filepath.Join(dataDir, "a.jpg")
	f2 := filepath.Join(dataDir, "b.jpg")
	f3 := filepath.Join(dataDir, "c.jpg")
	for _, p := range []string{f1, f2, f3} {
		if err := os.WriteFile(p, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := &thumbStore{limit: 10 << 20}
	defer s.Close()

	blob := make([]byte, 4096)
	s.Put(f1, 192, blob)
	time.Sleep(5 * time.Millisecond)
	s.Put(f2, 192, blob)
	time.Sleep(5 * time.Millisecond)
	s.Put(f3, 192, blob)

	wantSize := int64(3 * (len(blob) + 8))
	if got := s.Size(); got != wantSize {
		t.Fatalf("size = %d, want %d", got, wantSize)
	}
	if _, ok := s.Get(f1, 192); !ok {
		t.Fatal("expected f1 in store before limit shrink")
	}

	// Shrinking limit below current size evicts oldest entries down to 80%.
	const limit = int64(6000)
	s.SetLimit(limit)
	if s.Limit() != limit {
		t.Fatalf("limit = %d", s.Limit())
	}
	if got := s.Size(); got > limit {
		t.Fatalf("size after SetLimit = %d, want <= %d", got, limit)
	}
	if _, ok := s.Get(f1, 192); ok {
		t.Fatal("expected oldest entry f1 evicted after limit shrink")
	}
	if _, ok := s.Get(f3, 192); !ok {
		t.Fatal("expected newest entry f3 kept after limit shrink")
	}
	if s.FileSize() <= 0 {
		t.Fatalf("expected non-empty db file before Clear, got %d", s.FileSize())
	}

	// Clear deletes and closes the db file, reclaiming disk space immediately.
	s.Clear()
	if _, err := os.Stat(thumbStorePath()); !os.IsNotExist(err) {
		t.Fatalf("expected db file removed on Clear, stat err = %v", err)
	}
	if got := s.FileSize(); got != 0 {
		t.Fatalf("FileSize after Clear = %d, want 0", got)
	}
	if s.Size() != 0 {
		t.Fatalf("size after Clear = %d, want 0", s.Size())
	}
	// Store stays usable after Clear.
	s.Put(f1, 192, blob[:128])
	if got, ok := s.Get(f1, 192); !ok || len(got) != 128 {
		t.Fatalf("Put/Get after Clear failed: ok=%v len=%d", ok, len(got))
	}
}

func TestArchiveCacheSizeLimitAndClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "archives")
	origRoot := archiveCacheRoot
	origLimit := archiveCacheLimitBytes()
	t.Cleanup(func() {
		archiveCacheRoot = origRoot
		setArchiveCacheLimit(origLimit)
	})
	archiveCacheRoot = func() string { return root }

	oldDir := filepath.Join(root, "old-key")
	newDir := filepath.Join(root, "new-key")
	for _, d := range []string{oldDir, newDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "entry.bin"), make([]byte, 2048), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	if got := archiveCacheSize(); got != 4096 {
		t.Fatalf("archiveCacheSize = %d, want 4096", got)
	}

	setArchiveCacheLimit(3000)
	enforceArchiveCacheLimit()

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected oldest archive cache dir evicted, stat err = %v", err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("expected newest archive cache dir kept, stat err = %v", err)
	}
	if got := archiveCacheSize(); got != 2048 {
		t.Fatalf("archiveCacheSize after enforce = %d, want 2048", got)
	}

	clearArchiveCache()
	if got := archiveCacheSize(); got != 0 {
		t.Fatalf("archiveCacheSize after Clear = %d, want 0", got)
	}
}

func TestFolderMediaClearDropsDiskAndMemory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pic.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !folderMedia.HasSync(dir, mediaKindImage, false) {
		t.Fatal("expected image presence detected")
	}
	folderMedia.flushDisk()
	if folderMedia.DiskSize() <= 0 {
		t.Fatalf("expected non-empty presence disk cache, got %d", folderMedia.DiskSize())
	}

	folderMedia.Clear()
	if folderMedia.DiskSize() != 0 {
		t.Fatalf("presence disk size after Clear = %d, want 0", folderMedia.DiskSize())
	}
	if _, ok := folderMedia.Lookup(dir, mediaKindImage, false); ok {
		t.Fatal("expected in-memory presence entry cleared")
	}
}

func TestThumbCacheSetLimitConcurrent(t *testing.T) {
	c := newThumbCache(4 << 20)
	bmp := testBitmap(128, 128)
	var wg sync.WaitGroup
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				k := filepath.Join("k", string(rune('a'+g)), string(rune('0'+(i%10))))
				c.Put(k, bmp, 128)
				c.Get(k)
				if i%5 == 0 {
					c.SetLimit((1 + (g+i)%4) << 18)
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Size() > c.Limit() {
		t.Fatalf("size %d exceeds final limit %d", c.Size(), c.Limit())
	}
}
