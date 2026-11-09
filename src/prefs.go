package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
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
	Hidden           bool `json:"hidden"`
	AllTypes         bool `json:"allTypes"`
	Images           bool `json:"images"`
	RAW              bool `json:"raw"`
	Videos           bool `json:"videos"`
	Archives         bool `json:"archives"`
	Installers       bool `json:"installers"`
	RecursiveFolders bool `json:"recursiveFolders"`
	// RecurseDepth limits 递归浏览: 0 is every level, 1 is the child
	// folders only, 2 includes one level below those.
	RecurseDepth     int     `json:"recurseDepth,omitempty"`
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
		AllTypes: true, Images: true, Videos: true,
		AutoPlaySelected: true, InternalPlayer: true,
		Theme: "system", AutoScrollSpeed: 60, AutoPlayInterval: 2,
	}
}

type prefsFile struct {
	Active            int                   `json:"active"`
	Profiles          []profile             `json:"profiles"`
	QuickRename       string                `json:"quickRename,omitempty"`
	Shortcuts         map[string]keyBinding `json:"shortcuts,omitempty"`
	ZipPassword       string                `json:"zipPassword,omitempty"`
	ZipPasswords      []string              `json:"zipPasswords,omitempty"`
	ZipUsePassword    bool                  `json:"zipUsePassword,omitempty"`
	Favorites         []string              `json:"favorites,omitempty"`
	View              viewPreferences       `json:"view"`
	ViewSet           bool                  `json:"viewSet"`
	ImageDestination  string                `json:"imageDestination,omitempty"`
	VideoDestination  string                `json:"videoDestination,omitempty"`
	UpdateSource      string                `json:"updateSource,omitempty"`
	ImageCopyShortcut string                `json:"imageCopyShortcut,omitempty"`
	VideoCopyShortcut string                `json:"videoCopyShortcut,omitempty"`
	EnterOpens        bool                  `json:"enterOpens,omitempty"`
	VideoShiftArrow   bool                  `json:"videoShiftArrow"`
	PlaybackEnd       string                `json:"playbackEnd,omitempty"`
	Volume            float64               `json:"videoVolume,omitempty"`
	PreviousVolume    float64               `json:"previousVideoVolume,omitempty"`
	// RestoreSession holds the inverse of the stored opt-out; an absent
	// field means restore (the zero false is flipped by RestoreOff below).
	RestoreSession bool `json:"restoreSession"`
	// RestoreOff is the persisted opt-out for restoring the last session.
	RestoreOff bool `json:"restoreOff,omitempty"`
	// CloseAction is what the main window's close button and Cmd+W do.
	// closeToTray (the default) hides the window into the tray and leaves
	// the app running; closeQuit quits. Anything else is treated as closeToTray.
	CloseAction string `json:"closeAction,omitempty"`
	// DetailColumns keeps the widths and order of the list layout's columns.
	DetailColumns ui.TableLayout `json:"detailColumns,omitempty"`
	// FavoritesClosed and HistoryClosed keep the sidebar's sections folded.
	FavoritesClosed bool `json:"favoritesClosed,omitempty"`
	HistoryClosed   bool `json:"historyClosed,omitempty"`
	// ThumbCacheMiB is the decoded-thumbnail cache budget in MiB. Absent
	// (0) means the default; normalizePrefs clamps it to 64–1024.
	ThumbCacheMiB int `json:"thumbCacheMiB,omitempty"`
	// RememberPlayProgress resumes videos where they left off. The zero
	// value is flipped by RememberPlayOff below so the default stays on.
	RememberPlayProgress bool `json:"rememberPlayProgress"`
	// RememberPlayOff is the persisted opt-out for resume-on-reopen.
	RememberPlayOff bool `json:"rememberPlayOff,omitempty"`
	// TypeFilterRev marks which type-filter default has been applied.
	// 0 is the old default (every known type on). 1 showed images and
	// videos only. 2 shows every file type, the current default.
	TypeFilterRev int `json:"typeFilterRev,omitempty"`
	// TrayTitle shows a short status (task progress, playback time) next
	// to the menu-bar icon. Off by default.
	TrayTitle bool `json:"trayTitle,omitempty"`
	// TrayTab is the quick-access tab the status panel opens on.
	TrayTab string `json:"trayTab,omitempty"`
	// TrayRecent is how many recent folders the panel and menu list; 0
	// means the default, normalizePrefs clamps it to 3–20.
	TrayRecent int `json:"trayRecent,omitempty"`
	// TrayHotkey is the global accelerator that shows or hides the main
	// window; empty means the default, TrayHotkeyOff turns it off.
	TrayHotkey    string `json:"trayHotkey,omitempty"`
	TrayHotkeyOff bool   `json:"trayHotkeyOff,omitempty"`
	// HideDockWhenShelved drops the Dock icon while the main window is
	// hidden in the tray, leaving a menu-bar-only app.
	HideDockWhenShelved bool `json:"hideDockWhenShelved,omitempty"`
}

const typeFilterRevAll = 2

const (
	// closeToTray hides the main window into the system tray.
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
		p.TypeFilterRev = typeFilterRevAll
		normalizePrefs(&p)
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		logWarnf("preferences %s are not valid JSON (%v); using defaults", prefsPath(), err)
		p = prefsFile{View: defaultViewPreferences(), ViewSet: true, TypeFilterRev: typeFilterRevAll}
		normalizePrefs(&p)
		return p
	}
	migrateViewInstallers(data, &p)
	if migrateTypeFilterDefault(&p) {
		normalizePrefs(&p)
		savePrefs(p)
		return p
	}
	normalizePrefs(&p)
	return p
}

// migrateTypeFilterDefault moves a saved copy of an older default (every
// type on, or images and videos only) to 全部. A view the user customized
// is left alone. It reports whether the file should be rewritten so a later
// launch does not apply the change again.
func migrateTypeFilterDefault(p *prefsFile) bool {
	if p.TypeFilterRev >= typeFilterRevAll {
		return false
	}
	v := &p.View
	everyType := v.Images && v.RAW && v.Videos && v.Archives && v.Installers
	mediaOnly := v.Images && v.Videos && !v.RAW && !v.Archives && !v.Installers
	if !v.AllTypes && (everyType || mediaOnly) {
		v.AllTypes = true
	}
	p.TypeFilterRev = typeFilterRevAll
	return true
}

// migrateViewInstallers turns on Installers when an older prefs.json has no
// installers key, so custom-type folders of packages keep showing after upgrade.
func migrateViewInstallers(data []byte, p *prefsFile) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	viewRaw, ok := raw["view"]
	if !ok {
		return
	}
	var viewMap map[string]json.RawMessage
	if json.Unmarshal(viewRaw, &viewMap) != nil {
		return
	}
	if _, ok := viewMap["installers"]; !ok {
		p.View.Installers = true
	}
}

// normalizePrefs fills in the parts every run needs. It runs on every load
// path, so a missing file (a first launch) cannot leave Profiles nil: a nil
// slice marshals to null, which would drop the nine configuration slots and
// the startup folder on the next save. Existing profiles are kept.
func normalizePrefs(p *prefsFile) {
	normalizeTrayPrefs(p)
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
	if p.View.RecurseDepth < 0 || p.View.RecurseDepth > 2 {
		p.View.RecurseDepth = 0
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
	p.ThumbCacheMiB = normalizeThumbCacheMiB(p.ThumbCacheMiB)
	// Restoring the last session is the expected default for a media
	// browser; the setting only records an explicit opt-out.
	p.RestoreSession = !p.RestoreOff
	// Remembering play progress is on unless the user opted out.
	p.RememberPlayProgress = !p.RememberPlayOff
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

func (p *prefsFile) rememberPlay() bool {
	return !p.RememberPlayOff
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

// normalizeThumbCacheMiB maps a stored thumbnail cache budget onto the
// settings slider's range; zero (an older prefs.json) means the default.
func normalizeThumbCacheMiB(mib int) int {
	switch {
	case mib <= 0:
		return thumbBudget >> 20
	case mib < 64:
		return 64
	case mib > 1024:
		return 1024
	}
	return mib
}
