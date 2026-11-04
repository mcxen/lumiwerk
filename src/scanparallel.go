package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// maxScanWorkers bounds concurrent directory walks. Walks are I/O bound, so
// more workers than cores only adds contention; 16 keeps large machines sane.
func maxScanWorkers() int {
	n := runtime.NumCPU()
	if n > 16 {
		n = 16
	}
	if n < 1 {
		n = 1
	}
	return n
}

// scanEntryResult is one visited item plus a signal to skip a whole subtree,
// matching filepath.SkipDir in the sequential walk.
type scanEntryResult struct {
	item    mediaItem
	keep    bool
	skipDir bool
}

// scanTreeParallel walks root's immediate children concurrently. Each worker
// owns one subtree and writes into its own slot, so the merged result keeps
// the exact lexical order filepath.WalkDir produced. The root directory
// itself is not visited, just as the sequential callback ignored it.
func scanTreeParallel(root string, visit func(scanEntry) scanEntryResult) []mediaItem {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	parts := make([][]mediaItem, len(entries))
	sem := make(chan struct{}, maxScanWorkers())
	var wg sync.WaitGroup
	for i, e := range entries {
		entry := scanEntry{Path: filepath.Join(root, e.Name()), Name: e.Name(), IsDir: e.IsDir()}
		if info, infoErr := e.Info(); infoErr == nil {
			entry.Info = info
			entry.Size = info.Size()
			entry.ModTime = info.ModTime()
		}
		entry.Hidden = entryHidden(entry.Name, entry.Info)
		i, entry := i, entry
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if !entry.IsDir {
				if r := visit(entry); r.keep {
					parts[i] = append(parts[i], r.item)
				}
				return
			}
			_ = walkDirMedia(entry.Path, func(child scanEntry) error {
				r := visit(child)
				if r.keep {
					parts[i] = append(parts[i], r.item)
				}
				if r.skipDir {
					return filepath.SkipDir
				}
				return nil
			})
		}()
	}
	wg.Wait()
	var files []mediaItem
	for _, part := range parts {
		files = append(files, part...)
	}
	return files
}
