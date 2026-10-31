package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// mediaInfo is the metadata shown in EXIF and file info overlays.
type mediaInfo struct {
	Width       int
	Height      int
	Duration    string
	DurationSec float64
	Created     string
	Camera      string
	Model       string
}

func loadMediaInfo(path string, isVideo bool) mediaInfo {
	if isVideo {
		return probeVideo(path)
	}
	return probeImage(path)
}

func probeImage(path string) mediaInfo {
	var info mediaInfo
	cmd := exec.Command("sips", "-g", "pixelWidth", "-g", "pixelHeight", "-g", "creation", "-g", "make", "-g", "model", path)
	out, err := cmd.Output()
	if err != nil {
		return info
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "pixelWidth":
			info.Width, _ = strconv.Atoi(val)
		case "pixelHeight":
			info.Height, _ = strconv.Atoi(val)
		case "creation":
			info.Created = val
		case "make":
			info.Camera = val
		case "model":
			info.Model = val
		}
	}
	return info
}

func probeVideo(path string) mediaInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,codec_name,duration:format=duration",
		"-of", "default=noprint_wrappers=1", path)
	out, err := cmd.Output()
	if err != nil {
		return mediaInfo{}
	}
	var info mediaInfo
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch key {
		case "width":
			info.Width, _ = strconv.Atoi(val)
		case "height":
			info.Height, _ = strconv.Atoi(val)
		case "duration":
			if secs, err := strconv.ParseFloat(val, 64); err == nil {
				info.DurationSec = secs
				info.Duration = formatDuration(secs)
			}
		}
	}
	return info
}

func formatDuration(secs float64) string {
	d := time.Duration(secs * float64(time.Second))
	return fmt.Sprintf("%02d:%02d.%03d", int(d.Minutes()), int(d.Seconds())%60, d.Milliseconds()%1000)
}

// ============================
// File Operations
// ============================

type fileOp int

const (
	opCopy fileOp = iota
	opMove
	opDelete
	opRename
)

type fileOperation struct {
	op       fileOp
	paths    []string
	dest     string
	newName  string
	mappings []pathPair
	created  []string
	cut      bool
}

// perform applies a real file operation. Rename uses FlowVision's two-phase
// mapping so swaps and case-only changes roll back together. The returned
// operation records the paths that were actually created or moved.
func perform(op fileOperation) (fileOperation, error) {
	switch op.op {
	case opCopy:
		if err := rejectSelfCopy(op.paths, op.dest); err != nil {
			return op, err
		}
		var created []string
		for _, src := range op.paths {
			inPlace := pathKey(filepath.Dir(src)) == pathKey(op.dest)
			dst := filepath.Join(op.dest, filepath.Base(src))
			if inPlace {
				dst = flowUnique(dst, true, created)
			} else {
				dst = flowUnique(dst, false, created)
			}
			if err := copyTree(src, dst); err != nil {
				return op, err
			}
			created = append(created, dst)
		}
		op.created = created
	case opMove:
		if len(op.mappings) > 0 {
			for _, mapping := range op.mappings {
				if err := movePath(mapping.From, mapping.To); err != nil {
					return op, err
				}
			}
			return op, nil
		}
		if err := rejectSelfCopy(op.paths, op.dest); err != nil {
			return op, err
		}
		var mappings []pathPair
		var reserved []string
		for _, src := range op.paths {
			dst := flowUnique(filepath.Join(op.dest, filepath.Base(src)), false, reserved)
			if err := movePath(src, dst); err != nil {
				return op, err
			}
			mappings = append(mappings, pathPair{From: src, To: dst})
			reserved = append(reserved, dst)
		}
		op.mappings = mappings
	case opDelete:
		for _, path := range op.paths {
			if err := trashPath(path); err != nil {
				return op, err
			}
		}
	case opRename:
		mappings := op.mappings
		if len(mappings) == 0 {
			if len(op.paths) != 1 {
				return op, errors.New("rename takes exactly one path")
			}
			if err := validBaseName(op.newName); err != nil {
				return op, err
			}
			mappings = []pathPair{{From: op.paths[0], To: filepath.Join(filepath.Dir(op.paths[0]), op.newName)}}
		}
		if err := applyRenameMappings(mappings); err != nil {
			return op, err
		}
		op.mappings = mappings
	}
	return op, nil
}

func rejectSelfCopy(paths []string, dest string) error {
	for _, src := range paths {
		if copiesIntoSelf(src, dest) {
			return errors.New("不能将文件或文件夹复制到自身或其子目录中。")
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if info, err := os.Stat(src); err == nil {
		_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	}
	return nil
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); err != nil {
		return path
	}
	dir, name := filepath.Split(path)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
}

// trashPath moves a file to macOS Trash via Finder's native service.
func trashPath(path string) error {
	cmd := exec.Command("osascript", "-e", `tell application "Finder" to delete POSIX file "`+path+`"`)
	return cmd.Run()
}

func (a *appState) loadInfo(index int) {
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) {
		a.mu.Unlock()
		return
	}
	path := a.Items[index].Path
	isVideo := a.Items[index].IsVideo
	if a.Items[index].InfoLoaded {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	info := loadMediaInfo(path, isVideo)
	a.Window.Update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if index < len(a.Items) && a.Items[index].Path == path {
			a.Items[index].Info = info
			a.Items[index].InfoLoaded = true
		}
	})
}
