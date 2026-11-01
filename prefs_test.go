package main

import (
	"encoding/json"
	"os"
	"testing"
)

// TestLoadPrefsFillsProfilesWithoutFile checks that a first launch (no prefs
// file yet) still ends up with the nine configuration slots, so the next save
// cannot write "profiles": null and drop a folder or a slot.
func TestLoadPrefsFillsProfilesWithoutFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	p := loadPrefs()
	if len(p.Profiles) != 9 {
		t.Fatalf("profiles = %d, want 9", len(p.Profiles))
	}
	if p.View.Theme == "" || p.QuickRename == "" {
		t.Fatalf("defaults missing: theme=%q quickRename=%q", p.View.Theme, p.QuickRename)
	}
}

// TestSavePrefsNeverWritesNullProfiles guards the round trip: saving defaults
// must write nine slots, and reading that file back must keep them.
func TestSavePrefsNeverWritesNullProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	savePrefs(prefsFile{})

	data, err := os.ReadFile(prefsPath())
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Profiles []profile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Profiles) != 9 {
		t.Fatalf("saved profiles = %d, want 9 (file: %s)", len(raw.Profiles), data)
	}

	p := loadPrefs()
	if len(p.Profiles) != 9 {
		t.Fatalf("reloaded profiles = %d, want 9", len(p.Profiles))
	}
}

// TestLoadPrefsKeepsConfiguredProfile checks that normalization keeps a root
// the user configured in the startup slot.
func TestLoadPrefsKeepsConfiguredProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	savePrefs(prefsFile{Profiles: []profile{{Root: "/tmp/example"}}})

	p := loadPrefs()
	if len(p.Profiles) != 9 {
		t.Fatalf("profiles = %d, want 9", len(p.Profiles))
	}
	if p.Profiles[0].Root != "/tmp/example" {
		t.Fatalf("root = %q, want /tmp/example", p.Profiles[0].Root)
	}
}
