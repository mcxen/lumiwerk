// Package-level virtual filesystem and platform service layer.
//
// Every macOS- or Windows-specific file access the browser makes goes
// through vfs, so the rest of the app stays portable Go. The per-platform
// pieces live in vfs_darwin.go, vfs_windows.go and vfs_other.go.
package main

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// scanEntry is one directory entry the scanner hands to appState.
type scanEntry struct {
	Path    string
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
	Hidden  bool
	Info    os.FileInfo
}

// scanDir lists the immediate children of root, with platform-aware hidden
// detection. It is the single choke point for directory browsing.
func scanDir(root string) ([]scanEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]scanEntry, 0, len(entries))
	for _, e := range entries {
		entry := scanEntry{Path: filepath.Join(root, e.Name()), Name: e.Name(), IsDir: e.IsDir()}
		if info, infoErr := e.Info(); infoErr == nil {
			entry.Info = info
			entry.Size = info.Size()
			entry.ModTime = info.ModTime()
		}
		entry.Hidden = entryHidden(entry.Name, entry.Info)
		out = append(out, entry)
	}
	return out, nil
}

// walkDirMedia walks root recursively, calling fn for each entry. Skipping
// a directory is signalled with filepath.SkipDir, as filepath.WalkDir does.
func walkDirMedia(root string, fn func(scanEntry) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		entry := scanEntry{Path: path, Name: d.Name(), IsDir: d.IsDir()}
		if info, infoErr := d.Info(); infoErr == nil {
			entry.Info = info
			entry.Size = info.Size()
			entry.ModTime = info.ModTime()
		}
		entry.Hidden = entryHidden(entry.Name, entry.Info)
		return fn(entry)
	})
}

// entryHidden reports whether an entry is hidden: dotfiles everywhere, plus
// the platform hidden attribute where one exists (Windows).
func entryHidden(name string, info os.FileInfo) bool {
	if strings.HasPrefix(name, ".") && name != "." && name != ".." {
		return true
	}
	return fileHidden(info)
}

// probeImageSize reads an image's pixel size from its header alone. It never
// spawns a process and covers every format DecodeBitmap accepts plus TIFF.
func probeImageSize(path string) (int, int) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// createdAt formats a file's creation time when the platform keeps one.
func createdAtString(info os.FileInfo) string {
	if t, ok := fileCreated(info); ok && !t.IsZero() {
		return t.Format("2006-01-02 15:04:05")
	}
	return ""
}

// openWithDefault opens a path with the platform's default handler.
func openWithDefault(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return execWaitOrStart("open", path)
	case "windows":
		return execWaitOrStart("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return execWaitOrStart("xdg-open", path)
	}
}

// revealInFileBrowser shows one path selected in the platform file manager.
func revealInFileBrowser(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return execWaitOrStart("open", "-R", path)
	case "windows":
		return execWaitOrStart("explorer", "/select,", path)
	default:
		return execWaitOrStart("xdg-open", filepath.Dir(path))
	}
}

// openTerminalAt opens the platform terminal in a directory.
func openTerminalAt(dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return execWaitOrStart("open", "-a", "Terminal", dir)
	case "windows":
		return execWaitOrStart("cmd", "/c", "start", "cmd", "/k", "cd", "/d", dir)
	default:
		return execWaitOrStart("xdg-open", dir)
	}
}

func execWaitOrStart(name string, args ...string) error {
	cmd := execCommand(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

// trash moves paths to the platform trash. It reports the paths the platform
// answered with so undo can restore them; an empty result means best-effort.
func trashPaths(paths []string) (moved []string, err error) {
	for _, path := range paths {
		dest, trashErr := platformTrash(path)
		if trashErr != nil {
			return moved, trashErr
		}
		if dest != "" {
			moved = append(moved, dest)
		}
	}
	return moved, nil
}

// mediaTags reads the platform file tags of one path.
func mediaTags(path string) ([]string, error) {
	return platformTags(path)
}

// setMediaTags writes the platform file tags of one path.
func setMediaTags(path string, tags []string) error {
	return platformSetTags(path, tags)
}

// vfsWalkStats summarizes a folder tree for the info sheet.
func vfsWalkStats(root string) (files, dirs int, size int64) {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if entry.IsDir() {
			dirs++
			return nil
		}
		files++
		if info, infoErr := entry.Info(); infoErr == nil {
			size += info.Size()
		}
		return nil
	})
	return files, dirs, size
}

// humanBytes formats sizes the info sheet shows.
func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
	}
}

// sortedStrings is a small helper shared by tag readers.
func sortedStrings(list []string) []string {
	out := append([]string(nil), list...)
	sort.Strings(out)
	return out
}

var errTrashUnsupported = errors.New("此平台不支持移到废纸篓")
