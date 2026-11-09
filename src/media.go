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
	Format      string
	DPI         string
	Profile     string
	Bits        string
	Codec       string
	FrameRate   string
	BitRate     string
}

func loadMediaInfo(path string, isVideo bool) mediaInfo {
	if isVideo {
		return probeVideo(path)
	}
	return probeImage(path)
}

func probeImage(path string) mediaInfo {
	var info mediaInfo
	info.Width, info.Height = probeImageSize(path)
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."); ext != "" {
		info.Format = ext
	}
	probeExif(path, &info)
	return info
}

func probeVideo(path string) mediaInfo {
	if info, ok := probeVideoNative(path); ok {
		return info
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, toolPath("ffprobe"), "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,codec_name,avg_frame_rate,bit_rate:format=duration,bit_rate",
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
		case "codec_name":
			info.Codec = val
		case "avg_frame_rate":
			info.FrameRate = formatRate(val)
		case "bit_rate":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil && n > 0 {
				info.BitRate = formatBits(n)
			}
		case "duration":
			if secs, err := strconv.ParseFloat(val, 64); err == nil && info.DurationSec == 0 {
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

func formatRate(raw string) string {
	num, den, ok := strings.Cut(raw, "/")
	if !ok {
		return ""
	}
	n, errN := strconv.ParseFloat(num, 64)
	d, errD := strconv.ParseFloat(den, 64)
	if errN != nil || errD != nil || d == 0 || n == 0 {
		return ""
	}
	return strconv.FormatFloat(n/d, 'f', 2, 64)
}

func formatBits(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2f Mbps", float64(n)/1_000_000)
	case n >= 1000:
		return fmt.Sprintf("%.0f kbps", float64(n)/1000)
	default:
		return fmt.Sprintf("%d bps", n)
	}
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
	opCompress
	opExtract
)

type fileOperation struct {
	op        fileOp
	paths     []string
	dest      string
	newName   string
	mappings  []pathPair
	created   []string
	cut       bool
	replace   bool
	replaced  bool
	permanent bool
	password  string
	// passwords are the saved candidates an extraction tries in turn when
	// the archive is encrypted and password did not open it.
	passwords  []string
	dropSource bool
	detail     string
	// onStep, when set, runs after each path a copy or move places, from
	// the goroutine running the operation, for its progress.
	onStep func()
}

// perform applies a real file operation. Rename uses FlowVision's two-phase
// mapping so swaps and case-only changes roll back together. The returned
// operation records the paths that were actually created or moved.
func perform(op fileOperation) (fileOperation, error) {
	switch op.op {
	case opCopy, opMove:
		if len(op.mappings) > 0 && op.op == opMove {
			for _, mapping := range op.mappings {
				if err := movePath(mapping.From, mapping.To); err != nil {
					return op, err
				}
				op.step()
			}
			return op, nil
		}
		if err := rejectSelfCopy(op.paths, op.dest); err != nil {
			return op, err
		}
		var created []string
		var mappings []pathPair
		var reserved []string
		for _, src := range op.paths {
			dst, replaced, err := placePath(src, op.dest, op.op == opMove, op.replace, reserved)
			if err != nil {
				return op, err
			}
			if dst == "" {
				continue
			}
			if replaced {
				op.replaced = true
			}
			reserved = append(reserved, dst)
			op.step()
			if op.op == opMove {
				mappings = append(mappings, pathPair{From: src, To: dst})
			} else {
				created = append(created, dst)
			}
		}
		op.created = created
		op.mappings = mappings
	case opDelete:
		if op.permanent {
			for _, path := range op.paths {
				if err := os.RemoveAll(path); err != nil {
					return op, err
				}
			}
		} else {
			op.mappings = nil
			moved, err := trashPaths(op.paths)
			if err != nil {
				restoreTrashed(moved)
				return op, err
			}
			for _, mapping := range moved {
				if mapping.To != "" {
					op.mappings = append(op.mappings, mapping)
				}
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
	case opCompress:
		dest, err := compressPaths(op.paths, op.password, time.Now())
		if err != nil {
			return op, err
		}
		op.created = []string{dest}
		op.detail = "已压缩为 " + filepath.Base(dest)
		if op.dropSource {
			for _, path := range op.paths {
				_ = discardPath(path)
			}
		}
	case opExtract:
		extracted, failed, err := extractArchives(op.paths, op.password, op.passwords...)
		if err != nil {
			return op, err
		}
		for _, item := range extracted {
			op.created = append(op.created, item.dest)
			if op.dropSource {
				_ = discardPath(item.archive)
			}
		}
		if len(failed) > 0 {
			op.detail = "部分压缩包解压失败：" + previewNames(failed)
		} else if len(op.created) == 1 {
			op.detail = "已解压到 " + filepath.Base(op.created[0])
		} else {
			op.detail = fmt.Sprintf("已解压 %d 个压缩包", len(op.created))
		}
	}
	return op, nil
}

func (op *fileOperation) step() {
	if op.onStep != nil {
		op.onStep()
	}
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
	// The header probe is cheap and gives flow layouts an aspect before the
	// full probe (spawned processes on macOS) answers.
	info := mediaInfo{}
	if !isVideo {
		info.Width, info.Height = probeImageSize(path)
	}
	full := loadMediaInfo(path, isVideo)
	if full.Width == 0 && full.Height == 0 && !isVideo {
		full.Width, full.Height = info.Width, info.Height
	}
	a.update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if index < len(a.Items) && a.Items[index].Path == path {
			a.Items[index].Info = full
			a.Items[index].InfoLoaded = true
			a.Items[index].InfoLoading = false
			a.noteLayoutChanged()
		}
	})
}

// probeVisibleAspects gives flow layouts an aspect for every visible item
// right now: probed sizes for loaded info, a header read for images, and 1
// for the rest. It runs off the UI goroutine and posts the result.
func (a *appState) probeVisibleAspects() {
	a.mu.Lock()
	visible := append([]int(nil), a.Visible...)
	paths := make([]string, len(visible))
	for i, idx := range visible {
		if idx >= 0 && idx < len(a.Items) {
			paths[i] = a.Items[idx].Path
		}
	}
	video := make([]bool, len(visible))
	for i, idx := range visible {
		if idx >= 0 && idx < len(a.Items) {
			video[i] = a.Items[idx].IsVideo
		}
	}
	a.mu.Unlock()
	sizes := make([][2]int, len(paths))
	for i, path := range paths {
		if path == "" || video[i] {
			continue
		}
		w, h := probeImageSize(path)
		sizes[i] = [2]int{w, h}
	}
	a.update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for i, idx := range visible {
			if idx < 0 || idx >= len(a.Items) || a.Items[idx].InfoLoaded {
				continue
			}
			if w, h := sizes[i][0], sizes[i][1]; w > 0 && h > 0 {
				a.Items[idx].Info.Width, a.Items[idx].Info.Height = w, h
			}
		}
		a.noteLayoutChanged()
	})
}
