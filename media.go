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
	cmd := exec.Command("sips", "-g", "pixelWidth", "-g", "pixelHeight", "-g", "creation", "-g", "make", "-g", "model",
		"-g", "format", "-g", "dpiWidth", "-g", "dpiHeight", "-g", "profile", "-g", "bitsPerSample", path)
	out, err := cmd.Output()
	if err != nil {
		return info
	}
	var dpiW, dpiH string
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
		case "format":
			info.Format = val
		case "dpiWidth":
			dpiW = val
		case "dpiHeight":
			dpiH = val
		case "profile":
			info.Profile = val
		case "bitsPerSample":
			info.Bits = val
		}
	}
	if dpiW != "" && dpiH != "" && dpiW != "0.000" && dpiH != "0.000" {
		info.DPI = dpiW + " × " + dpiH
	}
	return info
}

func probeVideo(path string) mediaInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
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
	op         fileOp
	paths      []string
	dest       string
	newName    string
	mappings   []pathPair
	created    []string
	cut        bool
	replace    bool
	replaced   bool
	permanent  bool
	password   string
	dropSource bool
	detail     string
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
			if op.op == opMove {
				mappings = append(mappings, pathPair{From: src, To: dst})
			} else {
				created = append(created, dst)
			}
		}
		op.created = created
		op.mappings = mappings
	case opDelete:
		for _, path := range op.paths {
			var err error
			if op.permanent {
				err = os.RemoveAll(path)
			} else {
				trashed, trashErr := trashItem(path)
				err = trashErr
				if trashErr == nil && trashed != "" {
					op.mappings = append(op.mappings, pathPair{From: path, To: trashed})
				}
			}
			if err != nil {
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
		extracted, failed, err := extractArchives(op.paths, op.password)
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

// trashItem moves a file to the Trash and returns its new path so undo can
// put it back. Finder's delete is the same service FlowVision uses.
func trashItem(path string) (string, error) {
	script := fmt.Sprintf(`tell application "Finder"
  set trashed to delete (POSIX file %s as alias)
  POSIX path of (trashed as alias)
end tell`, strconv.Quote(path))
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return "", err
	}
	trashed := strings.TrimSpace(string(out))
	if trashed == "" {
		return "", errors.New("废纸篓没有返回位置")
	}
	return trashed, nil
}

// trashPath moves a file to macOS Trash via Finder's native service.
func trashPath(path string) error {
	_, err := trashItem(path)
	return err
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
