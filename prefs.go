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

type prefsFile struct {
	Active         int       `json:"active"`
	Profiles       []profile `json:"profiles"`
	QuickRename    string    `json:"quickRename,omitempty"`
	ZipPassword    string    `json:"zipPassword,omitempty"`
	ZipUsePassword bool      `json:"zipUsePassword,omitempty"`
	Favorites      []string  `json:"favorites,omitempty"`
}

func prefsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "lumiwerk", "prefs.json")
}

func loadPrefs() prefsFile {
	var p prefsFile
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
	return p
}

func savePrefs(p prefsFile) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	path := prefsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o644)
}
