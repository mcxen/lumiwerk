// Playback position memory: resume a video where it left off.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxPlayProgressEntries = 2000
	playProgressMinSec     = 5.0
	playProgressEndFrac    = 0.05
	playProgressEndSec     = 5.0
	playProgressAutosave   = 2 * time.Second
)

// playProgressEntry is one remembered playhead, keyed by path+size+mtime
// so a replaced file does not resume at the old offset.
type playProgressEntry struct {
	Path    string  `json:"path"`
	Size    int64   `json:"size"`
	MTime   int64   `json:"mtime"`
	At      float64 `json:"at"`
	Updated int64   `json:"updated"`
}

type playProgressFile struct {
	Entries []playProgressEntry `json:"entries"`
}

type playProgressStore struct {
	mu        sync.Mutex
	entries   []playProgressEntry
	loaded    bool
	dirty     bool
	lastFlush time.Time
}

var playProgress = &playProgressStore{}

func playProgressPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "lumiwerk", "play-progress.json")
}

func (s *playProgressStore) ensureLoaded() {
	if s.loaded {
		return
	}
	s.loaded = true
	data, err := os.ReadFile(playProgressPath())
	if err != nil {
		return
	}
	var file playProgressFile
	if err := json.Unmarshal(data, &file); err != nil {
		logWarnf("play progress %s is not valid JSON (%v); starting empty", playProgressPath(), err)
		return
	}
	s.entries = file.Entries
}

// fileIdentity is the on-disk size and mtime used as the resume key.
func fileIdentity(path string) (size, mtime int64, ok bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return 0, 0, false
	}
	return info.Size(), info.ModTime().Unix(), true
}

// nearPlayEnd reports whether at is close enough to the end that the
// next open should start over instead of resuming.
func nearPlayEnd(at, duration float64) bool {
	if duration <= 0 || at < 0 {
		return false
	}
	if duration-at <= playProgressEndSec {
		return true
	}
	return at >= duration*(1-playProgressEndFrac)
}

// Lookup returns a remembered position for path when the file still
// matches the stored size and mtime.
func (s *playProgressStore) Lookup(path string) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoaded()
	size, mtime, ok := fileIdentity(path)
	if !ok {
		return 0, false
	}
	key := pathKey(path)
	for _, e := range s.entries {
		if pathKey(e.Path) == key && e.Size == size && e.MTime == mtime {
			if e.At < playProgressMinSec {
				return 0, false
			}
			return e.At, true
		}
	}
	return 0, false
}

// Remember stores at for path, or clears it when the playhead is too
// early or too close to the end. duration 0 only clears early positions.
func (s *playProgressStore) Remember(path string, at, duration float64) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoaded()
	size, mtime, ok := fileIdentity(path)
	if !ok {
		return
	}
	if at < playProgressMinSec || nearPlayEnd(at, duration) {
		s.removeLocked(path, size, mtime)
		s.dirty = true
		return
	}
	key := pathKey(path)
	now := time.Now().Unix()
	for i := range s.entries {
		e := &s.entries[i]
		if pathKey(e.Path) == key && e.Size == size && e.MTime == mtime {
			e.At = at
			e.Updated = now
			s.dirty = true
			return
		}
	}
	s.entries = append(s.entries, playProgressEntry{
		Path: path, Size: size, MTime: mtime, At: at, Updated: now,
	})
	s.trimLocked()
	s.dirty = true
}

// Clear drops any entry for path regardless of size/mtime.
func (s *playProgressStore) Clear(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoaded()
	key := pathKey(path)
	kept := s.entries[:0]
	for _, e := range s.entries {
		if pathKey(e.Path) != key {
			kept = append(kept, e)
		}
	}
	if len(kept) != len(s.entries) {
		s.entries = kept
		s.dirty = true
	}
}

func (s *playProgressStore) removeLocked(path string, size, mtime int64) {
	key := pathKey(path)
	kept := s.entries[:0]
	for _, e := range s.entries {
		if pathKey(e.Path) == key && e.Size == size && e.MTime == mtime {
			continue
		}
		kept = append(kept, e)
	}
	s.entries = kept
}

func (s *playProgressStore) trimLocked() {
	for len(s.entries) > maxPlayProgressEntries {
		oldest := 0
		for i := 1; i < len(s.entries); i++ {
			if s.entries[i].Updated < s.entries[oldest].Updated {
				oldest = i
			}
		}
		s.entries = append(s.entries[:oldest], s.entries[oldest+1:]...)
	}
}

// Flush writes dirty entries to disk. safe to call often; no-ops when clean.
func (s *playProgressStore) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return
	}
	s.ensureLoaded()
	data, err := json.MarshalIndent(playProgressFile{Entries: s.entries}, "", "  ")
	if err != nil {
		logErrorf("could not encode play progress: %v", err)
		return
	}
	path := playProgressPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logErrorf("could not create play progress directory: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		logErrorf("could not write play progress %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		logErrorf("could not install play progress %s: %v", path, err)
		return
	}
	s.dirty = false
	s.lastFlush = time.Now()
}

// FlushSoon writes when dirty and the last flush was a while ago.
func (s *playProgressStore) FlushSoon() {
	s.mu.Lock()
	dirty := s.dirty
	due := time.Since(s.lastFlush) >= playProgressAutosave
	s.mu.Unlock()
	if dirty && due {
		s.Flush()
	}
}
