package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Folder media presence for 仅媒体 (toolbar 图片/视频) filtering.
//
// When FilterMode is 图片 or 视频, the grid and sidebar hide folders with no
// matching media underneath. Answers come from, in order: (1) a persistent
// bottom-up archive of hasImage/hasVideo per directory (空间换时间, filled by
// Prewarm when a folder opens and by NoteChange on watcher events), (2) an
// in-memory kind cache, (3) an on-demand early-exit scan. Unknown stays
// visible until resolved. See mediapresence_archive.go for the archive.

type mediaKind int

const (
	mediaKindAny mediaKind = iota
	mediaKindImage
	mediaKindVideo
)

type presenceResult int

const (
	presenceUnknown presenceResult = iota
	presenceYes
	presenceNo
)

const (
	presenceMaxDepth  = 128
	presenceMaxVisits = 100_000
	presenceMemCap    = 8192
	presenceDiskCap   = archiveDiskCap
)

type presenceKey struct {
	path   string
	kind   mediaKind
	hidden bool
}

type presenceMem struct {
	mtime    int64
	result   presenceResult
	complete bool
	at       time.Time
}

type presenceDiskRec struct {
	Path     string `json:"path"`
	Kind     int    `json:"kind"`
	Hidden   bool   `json:"hidden"`
	Mtime    int64  `json:"mtime"`
	Has      bool   `json:"has"`
	Complete bool   `json:"complete"`
}

type presenceCache struct {
	mu         sync.Mutex
	mem        map[presenceKey]presenceMem
	inflight   map[presenceKey]bool
	disk       map[presenceKey]presenceDiskRec // legacy kind rows; kept for migration
	archive    map[archiveKey]archiveDir
	diskOnce   sync.Once
	dirty      bool
	saveDue    time.Time
	lastHidden bool
	prewarm    prewarmState
}

var folderMedia = &presenceCache{
	mem:      map[presenceKey]presenceMem{},
	inflight: map[presenceKey]bool{},
	disk:     map[presenceKey]presenceDiskRec{},
	archive:  map[archiveKey]archiveDir{},
}

func filterMediaKind(mode int) mediaKind {
	switch mode {
	case 1:
		return mediaKindImage
	case 2:
		return mediaKindVideo
	default:
		return mediaKindAny
	}
}

func matchesMediaKind(ext string, kind mediaKind) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch kind {
	case mediaKindImage:
		return isImageFile(ext) || isRawFile(ext)
	case mediaKindVideo:
		return isVideoFile(ext)
	default:
		return isMediaFile(ext)
	}
}

func presenceCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "lumiwerk", "media-presence.json")
}

func dirMtime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

func (c *presenceCache) loadDisk() {
	c.diskOnce.Do(func() {
		c.ensureArchiveLocked()
		data, err := os.ReadFile(presenceCachePath())
		if err != nil || len(data) == 0 {
			return
		}
		var file presenceArchiveFile
		if json.Unmarshal(data, &file) == nil && file.Version >= archiveVersion && file.Dirs != nil {
			for _, r := range file.Dirs {
				if r.Path == "" {
					continue
				}
				c.archive[archiveKey{path: r.Path, hidden: r.Hidden}] = r
			}
			return
		}
		var recs []presenceDiskRec
		if json.Unmarshal(data, &recs) != nil {
			return
		}
		for _, r := range recs {
			if !r.Complete || r.Path == "" {
				continue
			}
			k := presenceKey{path: r.Path, kind: mediaKind(r.Kind), hidden: r.Hidden}
			c.disk[k] = r
		}
	})
}

func (c *presenceCache) scheduleSaveLocked() {
	c.dirty = true
	if c.saveDue.IsZero() {
		c.saveDue = time.Now().Add(2 * time.Second)
		safeGo("mediaPresence.save", func() {
			time.Sleep(2 * time.Second)
			c.flushDisk()
		})
	}
}

func (c *presenceCache) flushDisk() {
	c.mu.Lock()
	if !c.dirty {
		c.mu.Unlock()
		return
	}
	c.loadDisk()
	c.ensureArchiveLocked()
	dirs := make([]archiveDir, 0, len(c.archive))
	for _, r := range c.archive {
		dirs = append(dirs, r)
	}
	if len(dirs) > presenceDiskCap {
		dirs = dirs[len(dirs)-presenceDiskCap:]
	}
	file := presenceArchiveFile{Version: archiveVersion, Dirs: dirs}
	c.dirty = false
	c.saveDue = time.Time{}
	c.mu.Unlock()

	data, err := json.Marshal(file)
	if err != nil {
		return
	}
	path := presenceCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// Lookup returns a cached presence for path. ok is false when nothing is
// known yet (caller should Request a scan and treat the folder as visible).
func (c *presenceCache) Lookup(path string, kind mediaKind, showHidden bool) (presenceResult, bool) {
	path = filepath.Clean(path)
	k := presenceKey{path: path, kind: kind, hidden: showHidden}
	mtime := dirMtime(path)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadDisk()

	if e, ok := c.mem[k]; ok {
		if e.mtime == mtime && e.complete {
			e.at = time.Now()
			c.mem[k] = e
			return e.result, true
		}
		if e.mtime == mtime && !e.complete && e.result == presenceYes {
			// Partial scan already found media; that answer is final.
			e.at = time.Now()
			c.mem[k] = e
			return presenceYes, true
		}
	}
	if rec, ok := c.lookupArchiveLocked(path, showHidden, mtime); ok {
		has := false
		switch kind {
		case mediaKindImage:
			has = rec.Image
		case mediaKindVideo:
			has = rec.Video
		default:
			has = rec.Image || rec.Video
		}
		res := boolResult(has)
		c.mem[k] = presenceMem{mtime: mtime, result: res, complete: true, at: time.Now()}
		return res, true
	}
	if r, ok := c.disk[k]; ok && r.Mtime == mtime && r.Complete {
		res := presenceNo
		if r.Has {
			res = presenceYes
		}
		c.mem[k] = presenceMem{mtime: mtime, result: res, complete: true, at: time.Now()}
		return res, true
	}
	return presenceUnknown, false
}

// Request starts a background scan when none is in flight. onDone runs on
// the caller's update path after the result is stored (may be nil).
func (c *presenceCache) Request(path string, kind mediaKind, showHidden bool, onDone func()) {
	path = filepath.Clean(path)
	k := presenceKey{path: path, kind: kind, hidden: showHidden}

	c.mu.Lock()
	c.loadDisk()
	if c.inflight[k] {
		c.mu.Unlock()
		return
	}
	if e, ok := c.mem[k]; ok && e.complete && e.mtime == dirMtime(path) {
		c.mu.Unlock()
		return
	}
	c.inflight[k] = true
	c.mu.Unlock()

	safeGo("mediaPresence.scan", func() {
		res, complete := scanFolderHasMedia(path, kind, showHidden)
		mtime := dirMtime(path)
		c.mu.Lock()
		delete(c.inflight, k)
		if len(c.mem) > presenceMemCap {
			c.evictLocked()
		}
		c.mem[k] = presenceMem{mtime: mtime, result: res, complete: complete, at: time.Now()}
		if complete && (res == presenceYes || res == presenceNo) {
			c.disk[k] = presenceDiskRec{
				Path: path, Kind: int(kind), Hidden: showHidden,
				Mtime: mtime, Has: res == presenceYes, Complete: true,
			}
			c.scheduleSaveLocked()
		}
		c.mu.Unlock()
		if onDone != nil {
			onDone()
		}
	})
}

func (c *presenceCache) evictLocked() {
	var oldest presenceKey
	var oldestAt time.Time
	first := true
	for k, e := range c.mem {
		if first || e.at.Before(oldestAt) {
			oldest, oldestAt, first = k, e.at, false
		}
	}
	if !first {
		delete(c.mem, oldest)
	}
}

// Invalidate drops in-memory answers under path and schedules a low-priority
// reindex. The on-disk archive is kept: Lookup ignores rows whose mtime no
// longer matches. Watcher events should also call NoteChange so flags bubble
// when a nested edit does not bump every ancestor mtime.
func (c *presenceCache) Invalidate(path string) {
	path = filepath.Clean(path)
	prefix := path + string(filepath.Separator)
	c.mu.Lock()
	c.loadDisk()
	for k := range c.mem {
		if k.path == path || strings.HasPrefix(k.path, prefix) {
			delete(c.mem, k)
		}
	}
	hidden := c.lastHidden
	c.mu.Unlock()
	if path != "" && path != "." {
		c.Prewarm(path, hidden)
	}
}

// HasSync answers presence for navigation helpers. It uses the cache when
// possible and otherwise runs a budgeted scan inline (early exit on hit).
func (c *presenceCache) HasSync(path string, kind mediaKind, showHidden bool) bool {
	if res, ok := c.Lookup(path, kind, showHidden); ok {
		return res == presenceYes
	}
	res, complete := scanFolderHasMedia(path, kind, showHidden)
	mtime := dirMtime(path)
	k := presenceKey{path: filepath.Clean(path), kind: kind, hidden: showHidden}
	c.mu.Lock()
	c.mem[k] = presenceMem{mtime: mtime, result: res, complete: complete, at: time.Now()}
	if complete {
		c.disk[k] = presenceDiskRec{
			Path: k.path, Kind: int(kind), Hidden: showHidden,
			Mtime: mtime, Has: res == presenceYes, Complete: true,
		}
		c.scheduleSaveLocked()
	}
	c.mu.Unlock()
	return res == presenceYes
}

type presenceNode struct {
	path  string
	depth int
}

// scanFolderHasMedia walks path looking for a file matching kind.
// complete is false when visit/depth budgets stop the walk before the
// tree is exhausted; callers must not treat that as a conclusive "no".
func scanFolderHasMedia(root string, kind mediaKind, showHidden bool) (presenceResult, bool) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return presenceNo, true
	}

	seen := map[string]bool{root: true}
	queue := []presenceNode{{root, 0}}
	visits := 0

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth > presenceMaxDepth {
			return presenceUnknown, false
		}
		entries, err := os.ReadDir(cur.path)
		if err != nil {
			if cur.path == root {
				return presenceNo, true
			}
			continue
		}
		var subs []string
		for _, entry := range entries {
			visits++
			if visits > presenceMaxVisits {
				return presenceUnknown, false
			}
			name := entry.Name()
			if !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(cur.path, name)
			mode := entry.Type()
			if mode&os.ModeSymlink != 0 {
				// Skip symlink directories to avoid cycles; a symlink
				// file still counts if its name looks like media.
				if entry.IsDir() {
					continue
				}
			}
			if entry.IsDir() {
				if fi, infoErr := entry.Info(); infoErr == nil && !showHidden && entryHidden(name, fi) {
					continue
				}
				subs = append(subs, path)
				continue
			}
			ext := strings.TrimPrefix(filepath.Ext(name), ".")
			if matchesMediaKind(ext, kind) {
				return presenceYes, true
			}
		}
		for _, sub := range subs {
			clean := filepath.Clean(sub)
			if seen[clean] {
				continue
			}
			seen[clean] = true
			queue = append(queue, presenceNode{clean, cur.depth + 1})
		}
	}
	return presenceNo, true
}

// resetPresenceCacheForTest clears memory and disk maps; tests only.
func resetPresenceCacheForTest() {
	folderMedia.prewarm.gen.Add(1)
	folderMedia.mu.Lock()
	defer folderMedia.mu.Unlock()
	folderMedia.mem = map[presenceKey]presenceMem{}
	folderMedia.inflight = map[presenceKey]bool{}
	folderMedia.disk = map[presenceKey]presenceDiskRec{}
	folderMedia.archive = map[archiveKey]archiveDir{}
	folderMedia.dirty = false
	folderMedia.saveDue = time.Time{}
	folderMedia.lastHidden = false
	folderMedia.diskOnce = sync.Once{}
}
