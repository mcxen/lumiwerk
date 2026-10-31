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
	Width    int
	Height   int
	Duration string
	Created  string
	Camera   string
	Model    string
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
	op      fileOp
	paths   []string
	dest    string
	newName string
}

// perform applies a real file operation. Rename uses a two-phase move through
// a temporary name so partial failures cannot leave a mismatched state.
func perform(op fileOperation) error {
	switch op.op {
	case opCopy:
		for _, src := range op.paths {
			dst := uniquePath(filepath.Join(op.dest, filepath.Base(src)))
			if err := copyFile(src, dst); err != nil {
				return err
			}
		}
	case opMove:
		for _, src := range op.paths {
			dst := uniquePath(filepath.Join(op.dest, filepath.Base(src)))
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	case opDelete:
		for _, path := range op.paths {
			if err := trashPath(path); err != nil {
				return err
			}
		}
	case opRename:
		if len(op.paths) != 1 {
			return errors.New("rename takes exactly one path")
		}
		src := op.paths[0]
		dir := filepath.Dir(src)
		final := filepath.Join(dir, op.newName)
		if _, err := os.Stat(final); err == nil {
			return fmt.Errorf("target exists: %s", final)
		}
		tmp := uniquePath(filepath.Join(dir, "."+filepath.Base(src)+".lumiwerk-tmp"))
		if err := os.Rename(src, tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, final); err != nil {
			_ = os.Rename(tmp, src)
			return err
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
