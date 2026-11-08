//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"strings"
)

type openerApp struct {
	Name string
	Path string
}

// openerApps lists the programs Explorer's "Open with" remembers for the
// file's extension, read from the per-user FileExts registry key.
func openerApps(path string) []openerApp {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return nil
	}
	key := `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\` + ext + `\OpenWithList`
	out, err := execCommand("reg", "query", key).Output()
	if err != nil {
		return nil
	}
	var apps []openerApp
	seen := map[string]bool{}
	for _, exe := range parseOpenWithList(string(out)) {
		if seen[strings.ToLower(exe)] {
			continue
		}
		seen[strings.ToLower(exe)] = true
		cmdOut, err := execCommand("reg", "query", `HKCR\Applications\`+exe+`\shell\open\command`, "/ve").Output()
		if err != nil {
			continue
		}
		if exePath := commandExecutable(string(cmdOut)); exePath != "" {
			apps = append(apps, openerApp{Name: strings.TrimSuffix(exe, filepath.Ext(exe)), Path: exePath})
		}
	}
	return apps
}

func openWithApp(app string, paths []string) error {
	if app == "" {
		return errors.New("未指定应用")
	}
	return execCommand(app, paths...).Start()
}
