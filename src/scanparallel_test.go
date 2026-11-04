package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sequentialTree is the callback scanFolder used before the bounded parallel
// walk, kept here as the reference for ordering and filtering.
func sequentialTree(root string, hidden, recursiveFolders bool) []mediaItem {
	var files []mediaItem
	_ = walkDirMedia(root, func(entry scanEntry) error {
		path := entry.Path
		if entry.IsDir {
			if path != root && !hidden && entry.Hidden {
				return filepath.SkipDir
			}
			if recursiveFolders && path != root && (!hidden || !entry.Hidden) {
				files = append(files, mediaItem{Path: path, Name: entry.Name, ModTime: entry.ModTime.Unix(), IsDir: true, InfoLoaded: true})
			}
			return nil
		}
		if !hidden && isHiddenPath(root, path) {
			return nil
		}
		if entry.Info == nil {
			return nil
		}
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		files = append(files, mediaItem{Path: path, Name: entry.Name, Ext: strings.ToLower(ext), Size: entry.Size, ModTime: entry.ModTime.Unix(), IsVideo: isVideoFile(ext)})
		return nil
	})
	return files
}

func parallelTree(root string, hidden, recursiveFolders bool) []mediaItem {
	return scanTreeParallel(root, func(entry scanEntry) scanEntryResult {
		path := entry.Path
		if entry.IsDir {
			if path != root && !hidden && entry.Hidden {
				return scanEntryResult{skipDir: true}
			}
			if recursiveFolders && path != root && (!hidden || !entry.Hidden) {
				return scanEntryResult{keep: true, item: mediaItem{Path: path, Name: entry.Name, ModTime: entry.ModTime.Unix(), IsDir: true, InfoLoaded: true}}
			}
			return scanEntryResult{}
		}
		if !hidden && isHiddenPath(root, path) {
			return scanEntryResult{}
		}
		if entry.Info == nil {
			return scanEntryResult{}
		}
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		return scanEntryResult{keep: true, item: mediaItem{Path: path, Name: entry.Name, Ext: strings.ToLower(ext), Size: entry.Size, ModTime: entry.ModTime.Unix(), IsVideo: isVideoFile(ext)}}
	})
}

func makeScanTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dirs := []string{"sub1", "sub1/deep", "sub2", ".hidden", "empty", "bulk"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{
		"a.jpg", "b.png", "notes.txt", ".hidden.jpg",
		"sub1/c.jpg", "sub1/deep/d.jpg", "sub2/e.mp4",
		".hidden/secret.jpg",
	}
	for i := 0; i < 120; i++ {
		files = append(files, fmt.Sprintf("bulk/f%03d.jpg", i))
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScanTreeParallelMatchesSequentialWalk(t *testing.T) {
	root := makeScanTree(t)
	for _, hidden := range []bool{false, true} {
		for _, recursiveFolders := range []bool{false, true} {
			want := sequentialTree(root, hidden, recursiveFolders)
			got := parallelTree(root, hidden, recursiveFolders)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("hidden=%v folders=%v: got %d items, want %d\ngot:  %v\nwant: %v",
					hidden, recursiveFolders, len(got), len(want), pathsOf(got), pathsOf(want))
			}
			if len(got) < 100 {
				t.Fatalf("hidden=%v folders=%v: only %d items", hidden, recursiveFolders, len(got))
			}
		}
	}
}

func TestScanTreeParallelMissingAndEmptyFolders(t *testing.T) {
	if got := scanTreeParallel(filepath.Join(t.TempDir(), "missing"), func(scanEntry) scanEntryResult {
		return scanEntryResult{keep: true}
	}); len(got) != 0 {
		t.Fatalf("missing folder returned %d items", len(got))
	}
	if got := scanTreeParallel(t.TempDir(), func(scanEntry) scanEntryResult {
		return scanEntryResult{keep: true}
	}); len(got) != 0 {
		t.Fatalf("empty folder returned %d items", len(got))
	}
	if n := maxScanWorkers(); n < 1 || n > 16 {
		t.Fatalf("worker cap out of range: %d", n)
	}
}

func pathsOf(items []mediaItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Path)
	}
	return out
}
