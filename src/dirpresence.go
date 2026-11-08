package main

import (
	"sync"
	"time"
)

// dirPresence answers "is this folder still there?" for views without
// touching the file system on the main thread. The sidebar asked os.Stat for
// every favorite on every frame; a favorite on a sleeping disk, an unmounted
// share or a slow network volume then held each frame for as long as the
// stat took, which made the whole home screen, its text included, stutter.
//
// A path is assumed present until a background check says otherwise, and is
// checked again at most every dirPresenceTTL while it is on screen.
type dirPresence struct {
	mu      sync.Mutex
	entries map[string]*dirPresenceEntry
}

type dirPresenceEntry struct {
	ok       bool
	at       time.Time
	checking bool
}

const dirPresenceTTL = 3 * time.Second

// statDirForPresence is swapped by tests to simulate a slow volume.
var statDirForPresence = isDir

// folderPresent reports the last known state of path and refreshes it in
// the background when it is unknown or stale. Safe on the main thread.
func (a *appState) folderPresent(path string) bool {
	if path == "" {
		return false
	}
	p := &a.dirSeen
	p.mu.Lock()
	if p.entries == nil {
		p.entries = map[string]*dirPresenceEntry{}
	}
	e := p.entries[path]
	if e == nil {
		e = &dirPresenceEntry{ok: true}
		p.entries[path] = e
	}
	ok := e.ok
	stale := !e.checking && (e.at.IsZero() || time.Since(e.at) > dirPresenceTTL)
	if stale {
		e.checking = true
	}
	p.mu.Unlock()
	if stale {
		safeGo("dirpresence.check", func() {
			present := statDirForPresence(path)
			p.mu.Lock()
			changed := e.ok != present
			e.ok, e.at, e.checking = present, time.Now(), false
			p.mu.Unlock()
			if changed {
				a.update(func() {})
			}
		})
	}
	return ok
}
