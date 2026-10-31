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
}

func defaultViewPreferences() viewPreferences {
	return viewPreferences{
		Images: true, RAW: true, Videos: true, Archives: true,
		AutoPlaySelected: true, InternalPlayer: true,
		Theme: "system", AutoScrollSpeed: 60, AutoPlayInterval: 2,
	}
}

type prefsFile struct {
	Active         int             `json:"active"`
	Profiles       []profile       `json:"profiles"`
	QuickRename    string          `json:"quickRename,omitempty"`
	ZipPassword    string          `json:"zipPassword,omitempty"`
	ZipUsePassword bool            `json:"zipUsePassword,omitempty"`
	Favorites      []string        `json:"favorites,omitempty"`
	View           viewPreferences `json:"view"`
	ViewSet        bool            `json:"viewSet"`
}

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
		return p
	}
	_ = json.Unmarshal(data, &p)
	if len(p.Profiles) != 9 {
		p.Profiles = make([]profile, 9)
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
	return p
}

func savePrefs(p prefsFile) {
	p.ViewSet = true
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	path := prefsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o644)
}
