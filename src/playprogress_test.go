package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPlayProgressRememberAndLookup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := &playProgressStore{}
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte("video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	store.Remember(path, 12.5, 120)
	at, ok := store.Lookup(path)
	if !ok || at != 12.5 {
		t.Fatalf("lookup = %v %v, want 12.5 true", at, ok)
	}
	store.Flush()
	reloaded := &playProgressStore{}
	at, ok = reloaded.Lookup(path)
	if !ok || at != 12.5 {
		t.Fatalf("reloaded = %v %v", at, ok)
	}
}

func TestPlayProgressSkipsEarlyAndClearsNearEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := &playProgressStore{}
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte("video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	store.Remember(path, 3, 100)
	if _, ok := store.Lookup(path); ok {
		t.Fatal("early position should not resume")
	}
	store.Remember(path, 40, 100)
	if _, ok := store.Lookup(path); !ok {
		t.Fatal("mid position should resume")
	}
	store.Remember(path, 96, 100)
	if _, ok := store.Lookup(path); ok {
		t.Fatal("near-end should clear")
	}
	store.Remember(path, 40, 100)
	store.Remember(path, 97, 100)
	if _, ok := store.Lookup(path); ok {
		t.Fatal("last-5s should clear")
	}
}

func TestPlayProgressKeyUsesSizeAndMtime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := &playProgressStore{}
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	store.Remember(path, 20, 100)
	if err := os.WriteFile(path, []byte("brand-new-contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, newer, newer); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Lookup(path); ok {
		t.Fatal("replaced file must not resume old offset")
	}
}

func TestPlayProgressLRUCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := &playProgressStore{}
	dir := t.TempDir()
	for i := 0; i < maxPlayProgressEntries+5; i++ {
		path := filepath.Join(dir, "f"+strconv.Itoa(i)+".mp4")
		if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		store.Remember(path, 10, 100)
		store.mu.Lock()
		if n := len(store.entries); n > 0 {
			store.entries[n-1].Updated = int64(i + 1)
		}
		store.mu.Unlock()
	}
	store.mu.Lock()
	n := len(store.entries)
	store.mu.Unlock()
	if n > maxPlayProgressEntries {
		t.Fatalf("entries = %d, want <= %d", n, maxPlayProgressEntries)
	}
}

func TestNearPlayEnd(t *testing.T) {
	if !nearPlayEnd(96, 100) {
		t.Fatal("95%+ should be near end")
	}
	if !nearPlayEnd(97, 100) {
		t.Fatal("last 5s should be near end")
	}
	if nearPlayEnd(50, 100) || nearPlayEnd(4, 100) {
		t.Fatal("mid/early should not be near end")
	}
}
