package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Bottom-up media-presence archive (空间换时间).
//
// There is no separate disk search index in this repo (the 665b67e hint does
// not exist here), so presence itself is the archive: each directory stores
// hasImage/hasVideo as the OR of its own files and every descendant. Opening
// a folder prewarms the subtree at low priority; fs changes reindex the
// touched directory and bubble flags up the ancestor chain. Lookup still
// falls back to the on-demand early-exit scan when nothing is archived.

const (
	archiveVersion     = 3
	archiveDiskCap     = 100_000
	prewarmThrottleN   = 48
	prewarmThrottleGap = 2 * time.Millisecond
	prewarmStartDelay  = 40 * time.Millisecond
)

type archiveKey struct {
	path   string
	hidden bool
}

type archiveDir struct {
	Path      string `json:"p"`
	Hidden    bool   `json:"h,omitempty"`
	Mtime     int64  `json:"m"`
	Image     bool   `json:"i,omitempty"`
	Video     bool   `json:"v,omitempty"`
	Archive   bool   `json:"a,omitempty"`
	Installer bool   `json:"n,omitempty"`
}

type presenceArchiveFile struct {
	Version int          `json:"v"`
	Dirs    []archiveDir `json:"dirs"`
}

type prewarmState struct {
	gen    atomic.Uint64
	active atomic.Bool
}

// SetPreferHidden remembers which hidden-items setting the next Invalidate
// should reindex with (watcher refreshes do not carry the preference).
func (c *presenceCache) SetPreferHidden(showHidden bool) {
	c.mu.Lock()
	c.lastHidden = showHidden
	c.mu.Unlock()
}

// Prewarm schedules a low-priority bottom-up index of root. A newer Prewarm
// cancels the previous job via generation counter so navigation stays light.
func (c *presenceCache) Prewarm(root string, showHidden bool) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return
	}
	c.mu.Lock()
	c.lastHidden = showHidden
	c.ensureArchiveLocked()
	c.mu.Unlock()

	gen := c.prewarm.gen.Add(1)
	safeGo("mediaPresence.prewarm", func() {
		time.Sleep(prewarmStartDelay)
		if c.prewarm.gen.Load() != gen {
			return
		}
		if !c.prewarm.active.CompareAndSwap(false, true) {
			for i := 0; i < 40 && c.prewarm.active.Load(); i++ {
				if c.prewarm.gen.Load() != gen {
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
			if !c.prewarm.active.CompareAndSwap(false, true) {
				return
			}
		}
		defer c.prewarm.active.Store(false)
		if c.prewarm.gen.Load() != gen {
			return
		}
		stop := func() bool { return c.prewarm.gen.Load() != gen }
		c.indexSubtree(root, showHidden, stop)
		if stop() {
			return
		}
		c.bubbleAncestors(root, showHidden, stop)
		c.mu.Lock()
		c.scheduleSaveLocked()
		c.mu.Unlock()
	})
}

// NoteChange updates the archive after a filesystem event: reindex the
// containing directory's subtree and refresh ancestors.
func (c *presenceCache) NoteChange(changed string, showHidden bool) {
	changed = filepath.Clean(changed)
	if changed == "" || changed == "." {
		return
	}
	dir := changed
	if fi, err := os.Lstat(changed); err == nil {
		if !fi.IsDir() {
			dir = filepath.Dir(changed)
		}
	} else {
		dir = filepath.Dir(changed)
	}
	dir = filepath.Clean(dir)
	c.dropMemUnder(dir)
	c.forgetArchive(dir)
	c.Prewarm(dir, showHidden)
}

// forgetArchive removes the archived row for path (both hidden modes) so the
// next index pass cannot reuse a same-mtime stale answer.
func (c *presenceCache) forgetArchive(path string) {
	path = filepath.Clean(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureArchiveLocked()
	for _, hidden := range []bool{false, true} {
		k := archiveKey{path: path, hidden: hidden}
		if _, ok := c.archive[k]; ok {
			delete(c.archive, k)
			c.dirty = true
		}
	}
	if c.dirty {
		c.scheduleSaveLocked()
	}
}

func (c *presenceCache) dropMemUnder(path string) {
	path = filepath.Clean(path)
	prefix := path + string(filepath.Separator)
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.mem {
		if k.path == path || strings.HasPrefix(k.path, prefix) {
			delete(c.mem, k)
		}
	}
}

func (c *presenceCache) ensureArchiveLocked() {
	if c.archive == nil {
		c.archive = map[archiveKey]archiveDir{}
	}
}

func (c *presenceCache) lookupArchiveLocked(path string, showHidden bool, mtime int64) (archiveDir, bool) {
	c.ensureArchiveLocked()
	rec, ok := c.archive[archiveKey{path: path, hidden: showHidden}]
	if !ok || rec.Mtime != mtime {
		return archiveDir{}, false
	}
	return rec, true
}

func (c *presenceCache) storeArchiveLocked(rec archiveDir) {
	c.ensureArchiveLocked()
	c.archive[archiveKey{path: rec.Path, hidden: rec.Hidden}] = rec
	mtime := rec.Mtime
	now := time.Now()
	put := func(kind mediaKind, has bool) {
		k := presenceKey{path: rec.Path, kind: kind, hidden: rec.Hidden}
		c.mem[k] = presenceMem{mtime: mtime, result: boolResult(has), complete: true, at: now}
	}
	put(mediaKindImage, rec.Image)
	put(mediaKindVideo, rec.Video)
	put(mediaKindArchive, rec.Archive)
	put(mediaKindInstaller, rec.Installer)
	put(mediaKindAny, rec.Image || rec.Video)
	c.dirty = true
}

func boolResult(has bool) presenceResult {
	if has {
		return presenceYes
	}
	return presenceNo
}

type indexCounter struct {
	n int
}

func (c *presenceCache) indexSubtree(root string, showHidden bool, stop func() bool) {
	ctr := &indexCounter{}
	c.indexDir(root, showHidden, map[string]bool{}, ctr, stop)
}

func (c *presenceCache) indexDir(path string, showHidden bool, seen map[string]bool, ctr *indexCounter, stop func() bool) (img, vid, arc, inst bool) {
	path = filepath.Clean(path)
	if stop != nil && stop() {
		return
	}
	if seen[path] {
		return
	}
	seen[path] = true

	ctr.n++
	if ctr.n%prewarmThrottleN == 0 {
		time.Sleep(prewarmThrottleGap)
		if stop != nil && stop() {
			return
		}
	}

	mtime := dirMtime(path)
	c.mu.Lock()
	if rec, ok := c.lookupArchiveLocked(path, showHidden, mtime); ok {
		c.mu.Unlock()
		return rec.Image, rec.Video, rec.Archive, rec.Installer
	}
	c.mu.Unlock()

	entries, err := os.ReadDir(path)
	if err != nil {
		c.mu.Lock()
		c.storeArchiveLocked(archiveDir{Path: path, Hidden: showHidden, Mtime: mtime})
		c.mu.Unlock()
		return
	}

	var subs []string
	for _, entry := range entries {
		if stop != nil && stop() {
			return
		}
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		child := filepath.Join(path, name)
		mode := entry.Type()
		if mode&os.ModeSymlink != 0 {
			if entry.IsDir() {
				continue
			}
		}
		if entry.IsDir() {
			if isInstallerBundle(name) {
				if matchesPresenceName(name, mediaKindInstaller) {
					inst = true
				}
				if matchesPresenceName(name, mediaKindArchive) {
					arc = true
				}
				continue
			}
			if fi, infoErr := entry.Info(); infoErr == nil && !showHidden && entryHidden(name, fi) {
				continue
			}
			subs = append(subs, child)
			continue
		}
		if matchesPresenceName(name, mediaKindImage) {
			img = true
		}
		if matchesPresenceName(name, mediaKindVideo) {
			vid = true
		}
		if matchesPresenceName(name, mediaKindArchive) {
			arc = true
		}
		if matchesPresenceName(name, mediaKindInstaller) {
			inst = true
		}
	}
	for _, sub := range subs {
		ci, cv, ca, cn := c.indexDir(sub, showHidden, seen, ctr, stop)
		img = img || ci
		vid = vid || cv
		arc = arc || ca
		inst = inst || cn
	}

	c.mu.Lock()
	c.storeArchiveLocked(archiveDir{
		Path: path, Hidden: showHidden, Mtime: dirMtime(path),
		Image: img, Video: vid, Archive: arc, Installer: inst,
	})
	c.mu.Unlock()
	return img, vid, arc, inst
}

func (c *presenceCache) bubbleAncestors(start string, showHidden bool, stop func() bool) {
	cur := filepath.Clean(start)
	for i := 0; i < presenceMaxDepth; i++ {
		if stop != nil && stop() {
			return
		}
		parent := filepath.Dir(cur)
		if parent == cur || parent == "" || parent == "." {
			return
		}
		_, _, changed := c.recomputeOneLevel(parent, showHidden, stop)
		if !changed {
			return
		}
		cur = parent
	}
}

func (c *presenceCache) recomputeOneLevel(path string, showHidden bool, stop func() bool) (img, vid bool, changed bool) {
	path = filepath.Clean(path)
	mtime := dirMtime(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, false, false
	}
	var arc, inst bool
	for _, entry := range entries {
		if stop != nil && stop() {
			return img, vid, false
		}
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		child := filepath.Join(path, name)
		mode := entry.Type()
		if mode&os.ModeSymlink != 0 && entry.IsDir() {
			continue
		}
		if entry.IsDir() {
			if isInstallerBundle(name) {
				if matchesPresenceName(name, mediaKindInstaller) {
					inst = true
				}
				continue
			}
			if fi, infoErr := entry.Info(); infoErr == nil && !showHidden && entryHidden(name, fi) {
				continue
			}
			cm := dirMtime(child)
			c.mu.Lock()
			rec, ok := c.lookupArchiveLocked(child, showHidden, cm)
			c.mu.Unlock()
			if ok {
				img = img || rec.Image
				vid = vid || rec.Video
				arc = arc || rec.Archive
				inst = inst || rec.Installer
				continue
			}
			ir, ic := scanFolderHasMedia(child, mediaKindImage, showHidden)
			vr, vc := scanFolderHasMedia(child, mediaKindVideo, showHidden)
			ar, ac := scanFolderHasMedia(child, mediaKindArchive, showHidden)
			nr, nc := scanFolderHasMedia(child, mediaKindInstaller, showHidden)
			ci, cv := ic && ir == presenceYes, vc && vr == presenceYes
			ca, cn := ac && ar == presenceYes, nc && nr == presenceYes
			if ic || vc || ac || nc {
				c.mu.Lock()
				c.storeArchiveLocked(archiveDir{
					Path: child, Hidden: showHidden, Mtime: cm,
					Image: ci, Video: cv, Archive: ca, Installer: cn,
				})
				c.mu.Unlock()
			}
			img = img || ci
			vid = vid || cv
			arc = arc || ca
			inst = inst || cn
			continue
		}
		if matchesPresenceName(name, mediaKindImage) {
			img = true
		}
		if matchesPresenceName(name, mediaKindVideo) {
			vid = true
		}
		if matchesPresenceName(name, mediaKindArchive) {
			arc = true
		}
		if matchesPresenceName(name, mediaKindInstaller) {
			inst = true
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	prev, had := c.lookupArchiveLocked(path, showHidden, mtime)
	c.storeArchiveLocked(archiveDir{
		Path: path, Hidden: showHidden, Mtime: mtime,
		Image: img, Video: vid, Archive: arc, Installer: inst,
	})
	if !had {
		return img, vid, true
	}
	changed = prev.Image != img || prev.Video != vid || prev.Archive != arc || prev.Installer != inst
	return img, vid, changed
}
