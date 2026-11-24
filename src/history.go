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
const historyDwell = 3 * time.Second

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

// visitedAt returns the recorded Unix timestamp for path, matching exact
// keys first and falling back to normalized pathKey.
func visitedAt(visited map[string]int64, path string) int64 {
	if len(visited) == 0 || path == "" {
		return 0
	}
	if at := visited[path]; at > 0 {
		return at
	}
	want := pathKey(path)
	for k, at := range visited {
		if at > 0 && pathKey(k) == want {
			return at
		}
	}
	return 0
}

// pruneVisited keeps only visit timestamps whose normalized path is still
// referenced by recent or history, preventing unbounded growth in history.json.
func pruneVisited(visited map[string]int64, recent, history []string) map[string]int64 {
	if len(visited) == 0 {
		return nil
	}
	keep := make(map[string]bool, len(recent)+len(history))
	for _, p := range recent {
		if p != "" {
			keep[pathKey(p)] = true
		}
	}
	for _, p := range history {
		if p != "" {
			keep[pathKey(p)] = true
		}
	}
	out := make(map[string]int64, len(keep))
	for k, v := range visited {
		if v > 0 && keep[pathKey(k)] {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// noteDwell records path in the recent list once it has stayed open for
// historyDwell; navigating away first cancels the previous timer.
func (a *appState) noteDwell(path string, seq int) {
	a.mu.Lock()
	a.noteDwellLocked(path, seq)
	a.mu.Unlock()
}

func (a *appState) noteDwellLocked(path string, seq int) {
	if a.dwellTimer != nil {
		a.dwellTimer.Stop()
	}
	a.dwellTimer = time.AfterFunc(historyDwell, func() {
		a.update(func() { a.recordVisit(path, seq) })
	})
}

func (a *appState) recordVisit(path string, seq int) {
	a.mu.Lock()
	if seq != a.dwellSeq || pathKey(a.Root) != pathKey(path) {
		a.mu.Unlock()
		return
	}
	if a.Visited == nil {
		a.Visited = map[string]int64{}
	}
	a.Visited[path] = time.Now().Unix()
	a.Recent = dedupeRecent(append([]string{path}, a.Recent...), nil)
	a.Visited = pruneVisited(a.Visited, a.Recent, a.History)
	a.mu.Unlock()
	// Sidebar recent still waits for historyDwell; once recorded, flush so
	// a crash after the dwell keeps the list without waiting for quit.
	a.persistBrowsingHistory()
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
	h.Visited = pruneVisited(h.Visited, h.Recent, h.History)
	return h
}

// CursorPath is the folder the saved trail points at, or "".
func (h historyFile) CursorPath() string {
	if h.Cursor >= 0 && h.Cursor < len(h.History) {
		return h.History[h.Cursor]
	}
	return ""
}

// saveHistory writes the trail atomically with a .bak backup, as savePrefs does.
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
	h.Visited = pruneVisited(h.Visited, h.Recent, h.History)
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
	if _, err := os.Stat(path); err == nil {
		_ = os.Rename(path, path+".bak")
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

// persistBrowsingHistory writes the navigation trail so a relaunch can
// reopen the current folder. History/Cursor are the session-restore
// source and must be flushed without waiting for historyDwell; Recent
// and Visited are written only once the sidebar has already recorded
// them. A missing trail is left untouched so a first launch stays empty.
func (a *appState) persistBrowsingHistory() {
	a.mu.Lock()
	h := historyFile{
		History:   append([]string(nil), a.History...),
		Cursor:    a.Cursor,
		Visited:   cloneVisited(a.Visited),
		Recent:    append([]string(nil), a.Recent...),
		LastImage: a.lastLargePath(),
	}
	cleared := a.historyCleared
	a.mu.Unlock()
	if len(h.History) == 0 && len(h.Recent) == 0 && !cleared {
		return
	}
	saveHistory(h)
}

func cloneVisited(m map[string]int64) map[string]int64 {
	if m == nil {
		return nil
	}
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// clearHistory empties the trail and its visit times and saves at once.
func (a *appState) clearHistory() {
	a.mu.Lock()
	a.History, a.Cursor, a.Visited, a.Recent = nil, -1, nil, nil
	a.historyCleared = true
	a.mu.Unlock()
	saveHistory(historyFile{Cursor: -1})
}

// removeHistoryEntry drops path from the sidebar Recent list and Visited times
// without disturbing the back/forward navigation trail.
func (a *appState) removeHistoryEntry(path string) {
	if path == "" {
		return
	}
	target := pathKey(path)
	a.mu.Lock()
	kept := make([]string, 0, len(a.Recent))
	for _, r := range a.Recent {
		if pathKey(r) != target {
			kept = append(kept, r)
		}
	}
	a.Recent = kept
	for k := range a.Visited {
		if pathKey(k) == target {
			delete(a.Visited, k)
		}
	}
	if len(a.Recent) == 0 && len(a.History) == 0 {
		a.historyCleared = true
	}
	a.mu.Unlock()
	a.persistBrowsingHistory()
}

// remapHistoryPaths updates Recent, History, and Visited after folders move or rename.
func (a *appState) remapHistoryPaths(mappings []pathPair) {
	if len(mappings) == 0 {
		return
	}
	a.mu.Lock()
	changed := false
	for i, p := range a.History {
		if next := remapPath(p, mappings); next != p {
			a.History[i] = next
			changed = true
		}
	}
	remappedRecent := make([]string, len(a.Recent))
	for i, p := range a.Recent {
		next := remapPath(p, mappings)
		remappedRecent[i] = next
		if next != p {
			changed = true
		}
	}
	if changed {
		a.Recent = dedupeRecent(remappedRecent, nil)
	}
	if len(a.Visited) > 0 {
		nextVisited := make(map[string]int64, len(a.Visited))
		for k, v := range a.Visited {
			nk := remapPath(k, mappings)
			if nk != k {
				changed = true
			}
			if v > nextVisited[nk] {
				nextVisited[nk] = v
			}
		}
		a.Visited = nextVisited
	}
	a.mu.Unlock()
	if changed {
		a.persistBrowsingHistory()
	}
}

func pathMatchesAny(p string, targets []string) bool {
	pk := pathKey(p)
	for _, t := range targets {
		if t == "" {
			continue
		}
		if pk == pathKey(t) || pathWithin(t, p) {
			return true
		}
	}
	return false
}

// pruneHistoryPaths drops deleted or trashed folders (and their subfolders)
// from Recent, History, and Visited, keeping Cursor aligned to survivors.
func (a *appState) pruneHistoryPaths(removed []string) {
	if len(removed) == 0 {
		return
	}
	a.mu.Lock()
	changed := false
	keptRecent := make([]string, 0, len(a.Recent))
	for _, p := range a.Recent {
		if pathMatchesAny(p, removed) {
			changed = true
			continue
		}
		keptRecent = append(keptRecent, p)
	}
	if changed {
		a.Recent = keptRecent
	}

	keptHist := make([]string, 0, len(a.History))
	keptCursor := -1
	histChanged := false
	for i, p := range a.History {
		if pathMatchesAny(p, removed) {
			histChanged = true
			continue
		}
		if i == a.Cursor {
			keptCursor = len(keptHist)
		}
		keptHist = append(keptHist, p)
	}
	if histChanged {
		a.History = keptHist
		if len(a.History) > 0 && keptCursor < 0 {
			keptCursor = len(a.History) - 1
		}
		a.Cursor = keptCursor
		changed = true
	}

	for k := range a.Visited {
		if pathMatchesAny(k, removed) {
			delete(a.Visited, k)
			changed = true
		}
	}
	if changed && len(a.Recent) == 0 && len(a.History) == 0 {
		a.historyCleared = true
	}
	a.mu.Unlock()
	if changed {
		a.persistBrowsingHistory()
	}
}
