// Persisted browsing history: the folder trail survives restarts, so
// reopening the app lands where the last session left off, and the
// sidebar's History section and 文件 -> 最近打开 menu show past sessions.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// maxPersistedHistory bounds the trail written to disk. The sidebar and
// menu show at most a handful; the rest is buffer for goBack/goForward.
const maxPersistedHistory = 60

// historyFile is the on-disk shape of the browsing trail. Cursor indexes
// History, the same invariant appState keeps in memory.
type historyFile struct {
	History   []string  `json:"history"`
	Cursor    int       `json:"cursor"`
	SavedAt   time.Time `json:"savedAt,omitempty"`
	LastImage string    `json:"lastImage,omitempty"`
	// Visited is each folder's last visit (Unix seconds); older files
	// have none and show no time until the folder is visited again.
	Visited map[string]int64 `json:"visited,omitempty"`
	// Recent is the sidebar's history, newest first, one entry per folder
	// and only folders the user stayed in for historyDwell.
	Recent []string `json:"recent,omitempty"`
}

// historyDwell is how long a folder must stay open before it is recorded.
const historyDwell = 20 * time.Second

// dedupeRecent keeps the first (newest) entry per normalized path and drops
// folders that no longer exist.
func dedupeRecent(paths []string, alive func(string) bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = filepath.Clean(p)
		k := pathKey(p)
		if p == "" || seen[k] || (alive != nil && !alive(p)) {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	if len(out) > maxPersistedHistory {
		out = out[:maxPersistedHistory]
	}
	return out
}

// recentFromTrail rebuilds the sidebar list for files written before
// Recent existed: visited folders, newest visit first, duplicates merged.
func recentFromTrail(trail []string, visited map[string]int64) []string {
	var out []string
	for i := len(trail) - 1; i >= 0; i-- {
		if visited[trail[i]] > 0 {
			out = append(out, trail[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return visited[out[i]] > visited[out[j]] })
	return out
}

// noteDwell records path in the recent list once it has stayed open for
// historyDwell; navigating away first (a newer seq) cancels it.
func (a *appState) noteDwell(path string, seq int) {
	time.AfterFunc(historyDwell, func() {
		a.update(func() { a.recordVisit(path, seq) })
	})
}

func (a *appState) recordVisit(path string, seq int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if seq != a.dwellSeq || pathKey(a.Root) != pathKey(path) {
		return
	}
	if a.Visited == nil {
		a.Visited = map[string]int64{}
	}
	a.Visited[path] = time.Now().Unix()
	a.Recent = dedupeRecent(append([]string{path}, a.Recent...), nil)
}

func historyPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "lumiwerk", "history.json")
}

// loadHistory reads the saved trail, dropping entries that no longer
// exist so the menu and sidebar stay openable. It never fails: a missing
// or broken file is an empty trail.
func loadHistory() historyFile {
	var h historyFile
	data, err := os.ReadFile(historyPath())
	if err == nil {
		if err := json.Unmarshal(data, &h); err != nil {
			logWarnf("history %s is not valid JSON (%v); starting empty", historyPath(), err)
			h = historyFile{}
		}
	}
	if h.Recent == nil {
		h.Recent = recentFromTrail(h.History, h.Visited)
	}
	h.Recent = dedupeRecent(h.Recent, isDir)
	if h.Cursor < -1 || h.Cursor >= len(h.History) {
		h.Cursor = len(h.History) - 1
	}
	kept := make([]string, 0, len(h.History))
	// A dead folder cannot be reopened, so the trail drops it and the
	// cursor follows the pruned order (the entry it pointed at moves to
	// the same position among the survivors, or the trail is empty).
	keptCursor := -1
	for i, p := range h.History {
		if isDir(p) {
			if i == h.Cursor {
				keptCursor = len(kept)
			}
			kept = append(kept, p)
		}
	}
	if len(kept) != len(h.History) {
		h.History = kept
		h.Cursor = keptCursor
	}
	// A trail that still has folders should restore one of them: fall back
	// to the newest survivor when the cursor's entry was pruned.
	if len(h.History) > 0 && h.Cursor < 0 {
		h.Cursor = len(h.History) - 1
	}
	if h.Cursor < -1 || h.Cursor >= len(h.History) {
		h.Cursor = len(h.History) - 1
	}
	if len(h.History) > maxPersistedHistory {
		// Keep the newest entries around the cursor: drop the oldest tail
		// first, then the oldest head beyond the bound.
		h.History = h.History[len(h.History)-maxPersistedHistory:]
		h.Cursor = len(h.History) - 1
	}
	return h
}

// CursorPath is the folder the saved trail points at, or "".
func (h historyFile) CursorPath() string {
	if h.Cursor >= 0 && h.Cursor < len(h.History) {
		return h.History[h.Cursor]
	}
	return ""
}

// saveHistory writes the trail atomically, as savePrefs does.
func saveHistory(h historyFile) {
	h.SavedAt = time.Now()
	if len(h.History) > maxPersistedHistory {
		keep := h.History[len(h.History)-maxPersistedHistory:]
		if h.Cursor >= len(h.History)-len(keep) {
			h.Cursor -= len(h.History) - len(keep)
		} else {
			h.Cursor = -1
		}
		h.History = keep
	}
	if h.Cursor < -1 {
		h.Cursor = -1
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		logErrorf("could not encode history: %v", err)
		return
	}
	path := historyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logErrorf("could not create history directory: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		logErrorf("could not write history %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		logErrorf("could not install history %s: %v", path, err)
	}
}

// visitLabel is a history row's last visit: HH:mm today, else MM-dd HH:mm.
func visitLabel(unix int64, now time.Time) string {
	t := time.Unix(unix, 0)
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

// clearHistory empties the trail and its visit times and saves at once.
func (a *appState) clearHistory() {
	a.mu.Lock()
	a.History, a.Cursor, a.Visited, a.Recent = nil, -1, nil, nil
	a.historyCleared = true
	a.mu.Unlock()
	saveHistory(historyFile{Cursor: -1})
}
