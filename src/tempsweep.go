package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Temporaries the app creates (sips and ffmpeg intermediates, sharp crops)
// are named with this prefix in the system temp directory and removed by
// defer as soon as their step ends. A crash, a force quit or a kill -9
// leaves them behind, so launch sweeps what an earlier run left: files
// older than an hour, so a concurrent run's files are never touched.
const tempPrefix = "lumiwerk-"

// osTempDir is os.TempDir, a variable so tests can point the sweep at their
// own directory.
var osTempDir = os.TempDir

func sweepTempLeftovers() {
	dir := osTempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), tempPrefix) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if e.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			logDebugf("temp sweep: %s: %v", path, err)
		}
	}
}
