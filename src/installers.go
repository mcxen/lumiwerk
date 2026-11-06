package main

import (
	"path/filepath"
	"strings"
)

// isInstallerName reports whether name looks like an OS install package:
// disk images, package installers, Windows/Linux packages, and macOS .app
// bundles (directories treated as a single file by the scanner).
func isInstallerName(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".app") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".dmg", ".pkg", ".mpkg", ".exe", ".msi", ".msix", ".appx",
		".appimage", ".deb", ".rpm", ".apk":
		return true
	default:
		return false
	}
}

// isInstallerBundle reports a directory that should be listed as one file
// (macOS .app bundles) instead of being opened as a folder.
func isInstallerBundle(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".app")
}
