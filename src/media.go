package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	cmd := execCommandContext(ctx, toolPath("ffprobe"), "-v", "error", "-select_streams", "v:0",
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
	opUntrash
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
	if err := rejectArchiveMembers(op); err != nil {
		return op, err
	}
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
	case opUntrash:
		var restored []pathPair
		for _, path := range op.paths {
			if err := untrashPath(path); err != nil {
				return op, err
			}
			restored = append(restored, pathPair{From: path, To: path})
		}
		op.mappings = restored
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

func aspectChanged(oldW, oldH, newW, newH int) bool {
	if newW <= 0 || newH <= 0 {
		return false
	}
	if oldW <= 0 || oldH <= 0 {
		return true
	}
	return int64(oldW)*int64(newH) != int64(oldH)*int64(newW)
}

func (a *appState) loadInfo(index int) {
	a.mu.Lock()
	if index < 0 || index >= len(a.Items) || a.Items[index].InfoLoaded {
		a.mu.Unlock()
		return
	}
	root := a.Root
	seq := a.scanSeq
	path := a.Items[index].Path
	isVideo := a.Items[index].IsVideo
	a.mu.Unlock()
	a.loadInfoFor(root, seq, index, path, isVideo)
}

func (a *appState) loadInfoFor(root string, seq, index int, path string, isVideo bool) {
	infoGate <- struct{}{}
	defer func() { <-infoGate }()

	a.mu.Lock()
	if a.shelved || a.Root != root || a.scanSeq != seq || index < 0 || index >= len(a.Items) || a.Items[index].Path != path {
		a.mu.Unlock()
		return
	}
	if a.Items[index].InfoLoaded {
		a.Items[index].InfoLoading = false
		a.mu.Unlock()
		return
	}
	// Drop queued probes for cards already scrolled off screen (unless selected
	// or open in large view) so fast scrolling does not stall behind stale I/O.
	if !a.InLargeView && a.Chosen != index && !a.Selected.Has(path) {
		seen := atomic.LoadInt64(&a.Items[index].Seen)
		frame := atomic.LoadInt64(&a.frameNow)
		if seen > 0 && frame > 0 && seen < frame {
			a.Items[index].InfoLoading = false
			a.mu.Unlock()
			return
		}
	}
	a.mu.Unlock()

	full := loadMediaInfo(path, isVideo)
	a.queueInfoResult(infoResult{root: root, seq: seq, index: index, path: path, full: full})
}

type infoResult struct {
	root  string
	seq   int
	index int
	path  string
	full  mediaInfo
}

func (a *appState) queueInfoResult(res infoResult) {
	if a.Window == nil {
		a.update(func() { a.applyInfoResults([]infoResult{res}) })
		return
	}
	a.infoMu.Lock()
	a.infoPending = append(a.infoPending, res)
	schedule := !a.infoFlushScheduled
	if schedule {
		a.infoFlushScheduled = true
	}
	a.infoMu.Unlock()
	if schedule {
		time.AfterFunc(30*time.Millisecond, func() {
			a.update(a.flushInfoResults)
		})
	}
}

func (a *appState) flushInfoResults() {
	a.infoMu.Lock()
	batch := a.infoPending
	a.infoPending = nil
	a.infoFlushScheduled = false
	a.infoMu.Unlock()
	if len(batch) > 0 {
		a.applyInfoResults(batch)
	}
}

func (a *appState) applyInfoResults(batch []infoResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	layoutChanged := false
	for _, res := range batch {
		if a.Root != res.root || a.scanSeq != res.seq || res.index < 0 || res.index >= len(a.Items) {
			continue
		}
		item := &a.Items[res.index]
		if item.Path != res.path {
			continue
		}
		full := res.full
		if full.Width == 0 && full.Height == 0 && !item.IsVideo {
			full.Width, full.Height = item.Info.Width, item.Info.Height
		}
		if aspectChanged(item.Info.Width, item.Info.Height, full.Width, full.Height) {
			layoutChanged = true
		}
		item.Info = full
		item.InfoLoaded = true
		item.InfoLoading = false
	}
	if layoutChanged {
		a.noteLayoutChanged()
	}
}

// probeVisibleAspects gives flow layouts an aspect for every visible item
// right now: probed sizes for loaded info, a header read for images, and 1
// for the rest. It runs off the UI goroutine and posts results in chunks.
func (a *appState) probeVisibleAspects() {
	a.mu.Lock()
	root := a.Root
	seq := a.scanSeq
	a.mu.Unlock()
	a.probeVisibleAspectsFor(root, seq)
}

func (a *appState) probeVisibleAspectsFor(root string, seq int) {
	a.mu.Lock()
	if a.shelved || a.Root != root || a.scanSeq != seq {
		a.mu.Unlock()
		return
	}
	visible := append([]int(nil), a.Visible...)
	paths := make([]string, len(visible))
	skip := make([]bool, len(visible))
	for i, idx := range visible {
		if idx < 0 || idx >= len(a.Items) {
			skip[i] = true
			continue
		}
		it := &a.Items[idx]
		if it.IsDir || it.IsVideo || it.InfoLoaded || (it.Info.Width > 0 && it.Info.Height > 0) {
			skip[i] = true
			continue
		}
		paths[i] = it.Path
	}
	a.mu.Unlock()

	// Probe the leading viewport first so initial cards settle in one update,
	// then sweep remaining items in bounded chunks.
	const firstChunk = 64
	const nextChunk = 256
	for start := 0; start < len(visible); {
		chunk := nextChunk
		if start == 0 {
			chunk = firstChunk
		}
		end := min(start+chunk, len(visible))
		a.mu.Lock()
		stale := a.shelved || a.Root != root || a.scanSeq != seq
		a.mu.Unlock()
		if stale {
			return
		}
		sizes := make([][2]int, end-start)
		hasAny := false
		for i := start; i < end; i++ {
			if skip[i] || paths[i] == "" {
				continue
			}
			w, h := probeImageSize(paths[i])
			if w > 0 && h > 0 {
				sizes[i-start] = [2]int{w, h}
				hasAny = true
			}
		}
		if hasAny {
			s, e := start, end
			a.update(func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if a.Root != root || a.scanSeq != seq {
					return
				}
				changed := false
				for i := s; i < e; i++ {
					idx := visible[i]
					if idx < 0 || idx >= len(a.Items) || a.Items[idx].Path != paths[i] || a.Items[idx].InfoLoaded {
						continue
					}
					if w, h := sizes[i-s][0], sizes[i-s][1]; w > 0 && h > 0 {
						if aspectChanged(a.Items[idx].Info.Width, a.Items[idx].Info.Height, w, h) {
							changed = true
						}
						a.Items[idx].Info.Width, a.Items[idx].Info.Height = w, h
					}
				}
				if changed {
					a.noteLayoutChanged()
				}
			})
		}
		start = end
	}
}
