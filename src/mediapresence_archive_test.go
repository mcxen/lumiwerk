package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexSubtreeBottomUpFlags(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	photos := filepath.Join(root, "photos")
	clips := filepath.Join(root, "clips")
	empty := filepath.Join(root, "empty")
	deep := filepath.Join(photos, "2024")
	for _, d := range []string{photos, clips, empty, deep} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(deep, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clips, "b.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	folderMedia.indexSubtree(root, false, nil)

	check := func(path string, wantImg, wantVid bool) {
		t.Helper()
		mtime := dirMtime(path)
		folderMedia.mu.Lock()
		rec, ok := folderMedia.lookupArchiveLocked(path, false, mtime)
		folderMedia.mu.Unlock()
		if !ok {
			t.Fatalf("%s: missing archive", path)
		}
		if rec.Image != wantImg || rec.Video != wantVid {
			t.Fatalf("%s: image=%v video=%v want %v/%v", path, rec.Image, rec.Video, wantImg, wantVid)
		}
	}
	check(deep, true, false)
	check(photos, true, false)
	check(clips, false, true)
	check(empty, false, false)
	check(root, true, true)

	if res, ok := folderMedia.Lookup(empty, mediaKindImage, false); !ok || res != presenceNo {
		t.Fatalf("empty images: res=%v ok=%v", res, ok)
	}
	if res, ok := folderMedia.Lookup(photos, mediaKindImage, false); !ok || res != presenceYes {
		t.Fatalf("photos images: res=%v ok=%v", res, ok)
	}
	if res, ok := folderMedia.Lookup(photos, mediaKindVideo, false); !ok || res != presenceNo {
		t.Fatalf("photos videos: res=%v ok=%v", res, ok)
	}
}

func TestNoteChangeBubblesAncestors(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	sub := filepath.Join(root, "album")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	folderMedia.indexSubtree(root, false, nil)
	mtime := dirMtime(root)
	folderMedia.mu.Lock()
	rec, ok := folderMedia.lookupArchiveLocked(root, false, mtime)
	folderMedia.mu.Unlock()
	if !ok || rec.Image {
		t.Fatalf("root should start without images: ok=%v rec=%v", ok, rec)
	}

	if err := os.WriteFile(filepath.Join(sub, "new.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	folderMedia.forgetArchive(sub)
	folderMedia.indexSubtree(sub, false, nil)
	folderMedia.bubbleAncestors(sub, false, nil)

	mtime = dirMtime(root)
	folderMedia.mu.Lock()
	rec, ok = folderMedia.lookupArchiveLocked(root, false, mtime)
	folderMedia.mu.Unlock()
	if !ok || !rec.Image {
		t.Fatalf("root should gain hasImage after bubble: ok=%v rec=%+v", ok, rec)
	}
}

func TestArchiveRoundTripPersist(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	dir := t.TempDir()
	root := filepath.Join(dir, "tree")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	folderMedia.indexSubtree(root, false, nil)
	folderMedia.mu.Lock()
	dirs := make([]archiveDir, 0, len(folderMedia.archive))
	for _, r := range folderMedia.archive {
		dirs = append(dirs, r)
	}
	folderMedia.mu.Unlock()
	raw, err := json.Marshal(presenceArchiveFile{Version: archiveVersion, Dirs: dirs})
	if err != nil {
		t.Fatal(err)
	}

	resetPresenceCacheForTest()
	folderMedia.mu.Lock()
	var file presenceArchiveFile
	if err := json.Unmarshal(raw, &file); err != nil {
		folderMedia.mu.Unlock()
		t.Fatal(err)
	}
	folderMedia.ensureArchiveLocked()
	for _, r := range file.Dirs {
		folderMedia.archive[archiveKey{path: r.Path, hidden: r.Hidden}] = r
	}
	folderMedia.diskOnce.Do(func() {})
	folderMedia.mu.Unlock()

	res, ok := folderMedia.Lookup(root, mediaKindImage, false)
	if !ok || res != presenceYes {
		t.Fatalf("reloaded archive miss: res=%v ok=%v", res, ok)
	}
}

func TestPrewarmFillsLookupBeforeFilter(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	with := filepath.Join(root, "pics")
	for _, d := range []string{empty, with} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(with, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	folderMedia.indexSubtree(root, false, nil)

	a := &appState{
		Items: []mediaItem{
			{Path: empty, Name: "empty", IsDir: true},
			{Path: with, Name: "pics", IsDir: true},
		},
		FilterMode: 1,
	}
	a.applyFilterLocked()
	if len(a.Visible) != 1 || a.Items[a.Visible[0]].Name != "pics" {
		names := make([]string, 0, len(a.Visible))
		for _, i := range a.Visible {
			names = append(names, a.Items[i].Name)
		}
		t.Fatalf("visible=%v", names)
	}
}

func TestIndexSubtreeSkipsSymlinkDirs(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skip(err)
	}
	done := make(chan struct{})
	go func() {
		folderMedia.indexSubtree(root, false, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("symlink loop did not terminate")
	}
	res, ok := folderMedia.Lookup(root, mediaKindImage, false)
	if !ok || res != presenceYes {
		t.Fatalf("res=%v ok=%v", res, ok)
	}
}

func TestIndexSubtreeArchiveAndInstallerFlags(t *testing.T) {
	resetPresenceCacheForTest()
	t.Cleanup(resetPresenceCacheForTest)

	root := t.TempDir()
	pkgs := filepath.Join(root, "安装包")
	zips := filepath.Join(root, "压缩")
	for _, d := range []string{pkgs, zips} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pkgs, "Tool.dmg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zips, "a.7z"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(pkgs, "Demo.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}

	folderMedia.indexSubtree(root, false, nil)

	mtime := dirMtime(pkgs)
	folderMedia.mu.Lock()
	rec, ok := folderMedia.lookupArchiveLocked(pkgs, false, mtime)
	folderMedia.mu.Unlock()
	if !ok || !rec.Installer || rec.Image {
		t.Fatalf("pkgs archive: ok=%v rec=%+v", ok, rec)
	}
	mtime = dirMtime(zips)
	folderMedia.mu.Lock()
	rec, ok = folderMedia.lookupArchiveLocked(zips, false, mtime)
	folderMedia.mu.Unlock()
	if !ok || !rec.Archive || rec.Installer {
		t.Fatalf("zips archive: ok=%v rec=%+v", ok, rec)
	}
	if res, ok := folderMedia.Lookup(pkgs, mediaKindInstaller, false); !ok || res != presenceYes {
		t.Fatalf("installer lookup: res=%v ok=%v", res, ok)
	}
	if archiveVersion != 3 {
		t.Fatalf("archiveVersion=%d want 3", archiveVersion)
	}
}
