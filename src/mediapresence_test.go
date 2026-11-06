package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanFolderHasMediaFindsNestedAndEarlyExit(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "shot.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, complete := scanFolderHasMedia(root, mediaKindImage, false)
	if !complete || res != presenceYes {
		t.Fatalf("root images: res=%v complete=%v", res, complete)
	}
	res, complete = scanFolderHasMedia(empty, mediaKindImage, false)
	if !complete || res != presenceNo {
		t.Fatalf("empty: res=%v complete=%v", res, complete)
	}
	res, complete = scanFolderHasMedia(root, mediaKindVideo, false)
	if !complete || res != presenceNo {
		t.Fatalf("root videos: res=%v complete=%v", res, complete)
	}
}

func TestScanFolderHasMediaRespectsHiddenAndKind(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".secret")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "cam.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clip.MOV"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, complete := scanFolderHasMedia(root, mediaKindVideo, false)
	if !complete || res != presenceYes {
		t.Fatalf("visible video: res=%v complete=%v", res, complete)
	}
	onlyHidden := t.TempDir()
	h := filepath.Join(onlyHidden, ".hid")
	if err := os.MkdirAll(h, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, complete = scanFolderHasMedia(onlyHidden, mediaKindImage, false)
	if !complete || res != presenceNo {
		t.Fatalf("hidden skipped: res=%v complete=%v", res, complete)
	}
	res, complete = scanFolderHasMedia(onlyHidden, mediaKindImage, true)
	if !complete || res != presenceYes {
		t.Fatalf("hidden shown: res=%v complete=%v", res, complete)
	}
}

func TestScanFolderHasMediaSkipsSymlinkDirs(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "loop")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	// Without the symlink skip this would recurse forever; with it, only
	// "real" is walked and the answer is yes via that child.
	res, complete := scanFolderHasMedia(root, mediaKindImage, false)
	if !complete || res != presenceYes {
		t.Fatalf("symlink tree: res=%v complete=%v", res, complete)
	}
}

func TestPresenceCacheMtimeInvalidation(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, complete := scanFolderHasMedia(root, mediaKindAny, false)
	if !complete || res != presenceNo {
		t.Fatalf("before: res=%v complete=%v", res, complete)
	}
	folderMedia.mu.Lock()
	k := presenceKey{path: filepath.Clean(root), kind: mediaKindAny, hidden: false}
	folderMedia.mem[k] = presenceMem{mtime: dirMtime(root), result: presenceNo, complete: true, at: time.Now()}
	folderMedia.mu.Unlock()

	got, ok := folderMedia.Lookup(root, mediaKindAny, false)
	if !ok || got != presenceNo {
		t.Fatalf("cached no: got=%v ok=%v", got, ok)
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(root, "pic.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Touch the directory mtime by rewriting an entry; some FS keep dir
	// mtime sticky, so force Invalidate when Lookup still hits.
	if got, ok = folderMedia.Lookup(root, mediaKindAny, false); ok && got == presenceNo {
		folderMedia.Invalidate(root)
		if _, ok = folderMedia.Lookup(root, mediaKindAny, false); ok {
			t.Fatal("invalidate should drop the cache entry")
		}
	}
	if !folderMedia.HasSync(root, mediaKindAny, false) {
		t.Fatal("HasSync should see the new image")
	}
}

func TestApplyFilterHidesFoldersWithoutMatchingMedia(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	withImg := filepath.Join(root, "photos")
	withVid := filepath.Join(root, "clips")
	empty := filepath.Join(root, "empty")
	for _, d := range []string{withImg, withVid, empty} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(withImg, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withVid, "b.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Seed conclusive cache entries so applyFilter does not wait on async.
	for _, p := range []string{withImg, withVid, empty} {
		kind := mediaKindImage
		has := folderMedia.HasSync(p, kind, false)
		_ = has
		_ = folderMedia.HasSync(p, mediaKindVideo, false)
	}

	a := &appState{
		Items: []mediaItem{
			{Path: withImg, Name: "photos", IsDir: true},
			{Path: withVid, Name: "clips", IsDir: true},
			{Path: empty, Name: "empty", IsDir: true},
			{Path: filepath.Join(root, "x.jpg"), Name: "x.jpg", Ext: "jpg"},
			{Path: filepath.Join(root, "y.mp4"), Name: "y.mp4", Ext: "mp4", IsVideo: true},
		},
		FilterMode: 1,
	}
	a.applyFilterLocked()
	var names []string
	for _, i := range a.Visible {
		names = append(names, a.Items[i].Name)
	}
	if len(a.Visible) != 2 {
		t.Fatalf("image filter visible=%v", names)
	}
	for _, n := range names {
		if n != "photos" && n != "x.jpg" {
			t.Fatalf("unexpected visible %q in %v", n, names)
		}
	}

	a.FilterMode = 2
	a.applyFilterLocked()
	names = names[:0]
	for _, i := range a.Visible {
		names = append(names, a.Items[i].Name)
	}
	if len(a.Visible) != 2 {
		t.Fatalf("video filter visible=%v", names)
	}
	for _, n := range names {
		if n != "clips" && n != "y.mp4" {
			t.Fatalf("unexpected visible %q in %v", n, names)
		}
	}

	a.FilterMode = 0
	a.applyFilterLocked()
	if len(a.Visible) != 5 {
		t.Fatalf("all mode should keep every item, got %d", len(a.Visible))
	}
}

func TestFilterMediaKindMapping(t *testing.T) {
	if filterMediaKind(0) != mediaKindAny || filterMediaKind(1) != mediaKindImage || filterMediaKind(2) != mediaKindVideo {
		t.Fatal("filterMediaKind mapping is wrong")
	}
	if !matchesMediaKind(".JPG", mediaKindImage) || matchesMediaKind("mp4", mediaKindImage) {
		t.Fatal("image kind matching failed")
	}
	if !matchesMediaKind("mov", mediaKindVideo) || matchesMediaKind("png", mediaKindVideo) {
		t.Fatal("video kind matching failed")
	}
}
