package main

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A favorite on a slow volume must not hold the sidebar's frames: the old
// sidebar stat'ed every favorite on the main thread each frame.
func TestSidebarFavoritesNeverStatOnTheMainThread(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	slow := filepath.Join(t.TempDir(), "slow-share")
	gone := filepath.Join(t.TempDir(), "unplugged")
	release := make(chan struct{})
	var stats atomic.Int64
	orig := statDirForPresence
	statDirForPresence = func(path string) bool {
		stats.Add(1)
		if path == slow {
			<-release
			return true
		}
		return path != gone
	}
	t.Cleanup(func() { close(release); statDirForPresence = orig })
	hook := func(fn func()) { fn() }
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })

	a := &appState{SidebarShown: true, SidebarWidth: 220, Thumb: 128,
		Profiles:  prefsFile{View: defaultViewPreferences(), Favorites: []string{slow, gone}},
		treeCache: map[string]treeEntry{},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.sidebarView(c) }, 220, 500)
	start := time.Now()
	for i := 0; i < 20; i++ {
		tt.Frame()
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("20 sidebar frames took %v behind a stuck favorite", d)
	}
	if !tt.HasText(pathName(slow)) {
		t.Fatalf("an unchecked favorite must stay listed, texts %v", tt.Texts())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && tt.HasText(pathName(gone)) {
		time.Sleep(10 * time.Millisecond)
		tt.Frame()
	}
	if tt.HasText(pathName(gone)) {
		t.Fatal("a favorite whose folder is gone stays listed after the background check")
	}
	if n := stats.Load(); n > 4 {
		t.Fatalf("%d stats for 2 favorites across many frames; they must be rate limited", n)
	}
}
