package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// tagStore keeps file tags for
// platforms without Finder-style file metadata. It is a path-keyed JSON file
// in the user config dir. macOS does not use it: tags live in the file's own
// xattr there. Renames and moves made by the app migrate the entries; changes
// made outside the app are not followed.
type tagStore struct {
	mu     sync.Mutex
	path   string
	loaded bool
	data   map[string][]string
}

var fileTagStore = &tagStore{}

func tagStorePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "lumiwerk", "filetags.json")
}

// tagKey normalizes a path; Windows paths compare without case.
func tagKey(path string) string {
	key := filepath.Clean(path)
	if abs, err := filepath.Abs(key); err == nil {
		key = abs
	}
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

// load reads the file once per config path; the caller holds the lock.
func (s *tagStore) load() {
	want := tagStorePath()
	if s.loaded && s.path == want {
		return
	}
	s.path, s.loaded, s.data = want, true, map[string][]string{}
	raw, err := os.ReadFile(want)
	if err != nil {
		if !os.IsNotExist(err) {
			logWarnf("could not read file tags %s: %v", want, err)
		}
		return
	}
	if err := json.Unmarshal(raw, &s.data); err != nil || s.data == nil {
		logWarnf("ignoring unreadable file tags %s: %v", want, err)
		s.data = map[string][]string{}
	}
}

// save writes atomically; the caller holds the lock.
func (s *tagStore) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *tagStore) get(path string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	return append([]string(nil), s.data[tagKey(path)]...)
}

func (s *tagStore) set(path string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	key := tagKey(path)
	tags = nonEmpty(tags)
	if len(tags) == 0 {
		if _, ok := s.data[key]; !ok {
			return nil
		}
		delete(s.data, key)
	} else {
		s.data[key] = append([]string(nil), tags...)
	}
	return s.save()
}

// move carries the tags of src (and anything beneath it) over to dst.
func (s *tagStore) move(src, dst string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	from, to := tagKey(src), tagKey(dst)
	if from == to {
		return
	}
	sep := string(filepath.Separator)
	moved := map[string][]string{}
	for key, tags := range s.data {
		switch {
		case key == from:
			moved[to] = tags
		case strings.HasPrefix(key, from+sep):
			moved[to+key[len(from):]] = tags
		default:
			continue
		}
		delete(s.data, key)
	}
	if len(moved) == 0 {
		return
	}
	for key, tags := range moved {
		s.data[key] = tags
	}
	if err := s.save(); err != nil {
		logWarnf("could not save file tags: %v", err)
	}
}

// tagStoreMoved is called after the app moved or renamed a path.
func tagStoreMoved(src, dst string) { fileTagStore.move(src, dst) }

func nonEmpty(list []string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
