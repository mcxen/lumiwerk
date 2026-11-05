package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type profile struct {
	Root        string  `json:"root,omitempty"`
	Sidebar     bool    `json:"sidebar"`
	SidebarW    float32 `json:"sidebarWidth"`
	LayoutMode  int     `json:"layoutMode"`
	Thumb       float64 `json:"thumbSize"`
	Sort        string  `json:"sort"`
	Recursive   bool    `json:"recursive"`
	Destination string  `json:"destination,omitempty"`
	// Filters carries the type filter of the slot, so switching slots
	// restores what the browser shows as well as how it lays it out.
	Filters viewPreferences `json:"filters,omitempty"`
}

type viewPreferences struct {
	Hidden           bool    `json:"hidden"`
	AllTypes         bool    `json:"allTypes"`
	Images           bool    `json:"images"`
	RAW              bool    `json:"raw"`
	Videos           bool    `json:"videos"`
	Archives         bool    `json:"archives"`
	RecursiveFolders bool    `json:"recursiveFolders"`
	MediaUnit        bool    `json:"mediaUnit"`
	AutoPlaySelected bool    `json:"autoPlaySelected"`
	InternalPlayer   bool    `json:"internalPlayer"`
	Theme            string  `json:"theme"`
	AutoScrollSpeed  float64 `json:"autoScrollSpeed"`
	AutoPlayInterval float64 `json:"autoPlayInterval"`
	NoCaption        bool    `json:"noCaption"`
}

func defaultViewPreferences() viewPreferences {
	return viewPreferences{
		Images: true, RAW: true, Videos: true, Archives: true,
		AutoPlaySelected: true, InternalPlayer: true,
		Theme: "system", AutoScrollSpeed: 60, AutoPlayInterval: 2,
	}
}

type prefsFile struct {
	Active            int             `json:"active"`
	Profiles          []profile       `json:"profiles"`
	QuickRename       string          `json:"quickRename,omitempty"`
	ZipPassword       string          `json:"zipPassword,omitempty"`
	ZipUsePassword    bool            `json:"zipUsePassword,omitempty"`
	Favorites         []string        `json:"favorites,omitempty"`
	View              viewPreferences `json:"view"`
	ViewSet           bool            `json:"viewSet"`
	ImageDestination  string          `json:"imageDestination,omitempty"`
	VideoDestination  string          `json:"videoDestination,omitempty"`
	ImageCopyShortcut string          `json:"imageCopyShortcut,omitempty"`
	VideoCopyShortcut string          `json:"videoCopyShortcut,omitempty"`
	EnterOpens        bool            `json:"enterOpens,omitempty"`
	VideoShiftArrow   bool            `json:"videoShiftArrow"`
	PlaybackEnd       string          `json:"playbackEnd,omitempty"`
	Volume            float64         `json:"videoVolume,omitempty"`
	PreviousVolume    float64         `json:"previousVideoVolume,omitempty"`
	// RestoreSession holds the inverse of the stored opt-out; an absent
	// field means restore (the zero false is flipped by RestoreOff below).
	RestoreSession bool `json:"restoreSession"`
	// RestoreOff is the persisted opt-out for restoring the last session.
	RestoreOff bool `json:"restoreOff,omitempty"`
	// CloseAction is what the main window's close button and Cmd+W do.
	// closeToTray (the default) hides the window and leaves the app
	// running; closeQuit quits. Anything else is treated as closeToTray.
	CloseAction string `json:"closeAction,omitempty"`
}

const (
	// closeToTray hides the main window into the menu bar.
	closeToTray = "tray"
	// closeQuit quits the app when the main window closes.
	closeQuit = "quit"
)

func prefsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "lumiwerk", "prefs.json")
}

func loadPrefs() prefsFile {
	p := prefsFile{View: defaultViewPreferences(), ViewSet: true}
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		if !os.IsNotExist(err) {
			logWarnf("could not read preferences %s: %v", prefsPath(), err)
		}
		normalizePrefs(&p)
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		logWarnf("preferences %s are not valid JSON (%v); using defaults", prefsPath(), err)
		p = prefsFile{View: defaultViewPreferences(), ViewSet: true}
	}
	normalizePrefs(&p)
	return p
}

// normalizePrefs fills in the parts every run needs. It runs on every load
// path, so a missing file (a first launch) cannot leave Profiles nil: a nil
// slice marshals to null, which would drop the nine configuration slots and
// the startup folder on the next save. Existing profiles are kept.
func normalizePrefs(p *prefsFile) {
	if len(p.Profiles) != 9 {
		profiles := make([]profile, 9)
		copy(profiles, p.Profiles)
		p.Profiles = profiles
	}
	if strings.TrimSpace(p.QuickRename) == "" {
		p.QuickRename = "{folder}_{index}"
	}
	if !p.ViewSet {
		p.View = defaultViewPreferences()
		p.ViewSet = true
	}
	if p.View.AutoScrollSpeed == 0 {
		p.View.AutoScrollSpeed = 60
	}
	if p.View.AutoPlayInterval <= 0 {
		p.View.AutoPlayInterval = 2
	}
	if p.View.Theme == "" {
		p.View.Theme = "system"
	}
	if p.ImageCopyShortcut == "" {
		p.ImageCopyShortcut = "N"
	}
	if p.VideoCopyShortcut == "" {
		p.VideoCopyShortcut = "]"
	}
	if p.Volume < 0 || p.Volume > 2 {
		p.Volume = 1
	}
	if p.PreviousVolume <= 0 || p.PreviousVolume > 2 {
		p.PreviousVolume = 1
	}
	p.PlaybackEnd = normalizePlaybackEnd(p.PlaybackEnd)
	// Restoring the last session is the expected default for a media
	// browser; the setting only records an explicit opt-out.
	p.RestoreSession = !p.RestoreOff
	// Closing the window keeps the app running unless the user opted out.
	if p.CloseAction != closeQuit {
		p.CloseAction = closeToTray
	}
}

// hidesOnClose reports whether closing the main window should hide it
// instead of quitting. An absent or unknown value hides; only an explicit
// quit opt-out leaves.
func (p *prefsFile) hidesOnClose() bool {
	return p.CloseAction != closeQuit
}

// restoreSession reports whether this run should reopen the last session's
// folder: the stored choice unless the environment opted out.
func (p *prefsFile) restoreSession() bool {
	if os.Getenv("LUMIWERK_NO_RESTORE") == "1" {
		return false
	}
	return !p.RestoreOff
}

func savePrefs(p prefsFile) {
	p.ViewSet = true
	normalizePrefs(&p)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		logErrorf("could not encode preferences: %v", err)
		return
	}
	path := prefsPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logErrorf("could not create preferences directory %s: %v", dir, err)
		return
	}

	// Write to a temporary file first and rename it into place: a crash or a
	// full disk can otherwise leave a truncated preferences file behind.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		logErrorf("could not write preferences %s: %v", tmp, err)
		return
	}

	// Keep one previous generation before replacing it, so an unexpected
	// overwrite (a bug, a bad run) can still be undone from prefs.json.bak.
	if _, err := os.Stat(path); err == nil {
		_ = os.Rename(path, path+".bak")
	}
	if err := os.Rename(tmp, path); err != nil {
		logErrorf("could not install preferences %s: %v", path, err)
	}
}
