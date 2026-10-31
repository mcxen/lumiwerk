package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// pathPair is one FlowVision-style rename or move mapping.
type pathPair struct {
	From string
	To   string
}

func pathKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func validBaseName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("无效的文件名：%s", name)
	}
	return nil
}

func pathsOverlap(paths []string) bool {
	norms := make([]string, len(paths))
	for i, path := range paths {
		norms[i] = pathKey(path)
	}
	for i, path := range norms {
		prefix := path + string(filepath.Separator)
		for j, other := range norms {
			if i != j && strings.HasPrefix(other, prefix) {
				return true
			}
		}
	}
	return false
}

// validateRename matches FlowVision: duplicate sources or targets, a folder
// renamed together with a descendant, a missing source, or a target that
// already exists outside the rename set. Case-only renames are allowed.
func validateRename(mappings []pathPair) error {
	if len(mappings) == 0 {
		return errors.New("没有要重命名的项目")
	}
	sources := map[string]struct{}{}
	targets := map[string]struct{}{}
	var sourcePaths []string
	for _, mapping := range mappings {
		if err := validBaseName(filepath.Base(mapping.To)); err != nil {
			return err
		}
		sourceKey := pathKey(mapping.From)
		targetKey := pathKey(mapping.To)
		if _, ok := sources[sourceKey]; ok {
			return fmt.Errorf("重复的重命名来源：%s", filepath.Base(mapping.From))
		}
		sources[sourceKey] = struct{}{}
		if _, ok := targets[targetKey]; ok {
			return fmt.Errorf("目标名称重复：%s", filepath.Base(mapping.To))
		}
		targets[targetKey] = struct{}{}
		sourcePaths = append(sourcePaths, mapping.From)
	}
	if pathsOverlap(sourcePaths) {
		return errors.New("不能同时重命名文件夹及其子项目。")
	}
	for _, mapping := range mappings {
		if _, err := os.Stat(mapping.From); err != nil {
			return fmt.Errorf("重命名源文件不存在：%s", filepath.Base(mapping.From))
		}
		if pathKey(mapping.From) == pathKey(mapping.To) {
			continue
		}
		if _, err := os.Stat(mapping.To); err == nil {
			if _, ok := sources[pathKey(mapping.To)]; !ok {
				return fmt.Errorf("文件或文件夹已存在：%s", filepath.Base(mapping.To))
			}
		}
	}
	return nil
}

type pendingRename struct {
	src string
	tmp string
	dst string
}

func rollbackRenames(pending []pendingRename, completed int) {
	for i := completed - 1; i >= 0; i-- {
		_ = os.Rename(pending[i].dst, pending[i].tmp)
	}
	for i := len(pending) - 1; i >= 0; i-- {
		_ = os.Rename(pending[i].tmp, pending[i].src)
	}
}

// applyRenameMappings moves every source aside, then onto its final name, so
// swaps and case-only renames cannot collide. A failure restores both phases.
func applyRenameMappings(mappings []pathPair) error {
	if err := validateRename(mappings); err != nil {
		return err
	}
	var pending []pendingRename
	for i, mapping := range mappings {
		if mapping.From == mapping.To {
			continue
		}
		tmp := filepath.Join(filepath.Dir(mapping.From), fmt.Sprintf(".lumiwerk-rename-%d-%d", time.Now().UnixNano(), i))
		if err := os.Rename(mapping.From, tmp); err != nil {
			rollbackRenames(pending, 0)
			return fmt.Errorf("重命名失败：%s", err.Error())
		}
		pending = append(pending, pendingRename{src: mapping.From, tmp: tmp, dst: mapping.To})
	}
	for completed, item := range pending {
		if err := os.Rename(item.tmp, item.dst); err != nil {
			rollbackRenames(pending, completed)
			return fmt.Errorf("重命名失败：%s", err.Error())
		}
	}
	return nil
}

func pathTaken(path string, reserved []string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	key := pathKey(path)
	for _, reservedPath := range reserved {
		if pathKey(reservedPath) == key {
			return true
		}
	}
	return false
}

// flowUnique matches FlowVision's destination names: name_2, name_3 outside
// the source folder, and name_copy, name_copy2 when pasting beside itself.
func flowUnique(path string, inPlace bool, reserved []string) string {
	if !pathTaken(path, reserved) {
		return path
	}
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	for counter := 1; ; counter++ {
		var name string
		if inPlace {
			if counter == 1 {
				name = base + "_copy"
			} else {
				name = fmt.Sprintf("%s_copy%d", base, counter)
			}
		} else {
			name = fmt.Sprintf("%s_%d", base, counter+1)
		}
		candidate := filepath.Join(dir, name+ext)
		if !pathTaken(candidate, reserved) {
			return candidate
		}
	}
}

func copiesIntoSelf(src, dstDir string) bool {
	src = filepath.Clean(src)
	dstDir = filepath.Clean(dstDir)
	if pathKey(src) == pathKey(dstDir) {
		return true
	}
	rel, err := filepath.Rel(src, dstDir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func isCrossDevice(err error) bool {
	var link *os.LinkError
	return errors.As(err, &link) && errors.Is(link.Err, syscall.EXDEV)
}

func hasNameConflict(paths []string, dest string) bool {
	for _, src := range paths {
		if pathKey(filepath.Dir(src)) == pathKey(dest) {
			continue
		}
		target := filepath.Join(dest, filepath.Base(src))
		if pathKey(target) == pathKey(src) {
			continue
		}
		if _, err := os.Stat(target); err == nil {
			return true
		}
	}
	return false
}

func sameDevice(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	statA, okA := infoA.Sys().(*syscall.Stat_t)
	statB, okB := infoB.Sys().(*syscall.Stat_t)
	if !okA || !okB {
		return false
	}
	return statA.Dev == statB.Dev
}

// placePath copies or moves src into destDir. Same-folder copies become
// name_copy. Existing names are renumbered unless replace is set, in which
// case a file is swapped in safely and a folder is merged.
func placePath(src, destDir string, move, replace bool, reserved []string) (string, bool, error) {
	inPlace := pathKey(filepath.Dir(src)) == pathKey(destDir)
	dst := filepath.Join(destDir, filepath.Base(src))
	if pathKey(dst) == pathKey(src) {
		return "", false, nil
	}
	exists := pathTaken(dst, reserved)
	if !exists || inPlace {
		if inPlace {
			dst = flowUnique(dst, true, reserved)
		}
		if move {
			return dst, false, movePath(src, dst)
		}
		return dst, false, copyTree(src, dst)
	}
	if replace {
		if err := replaceItem(src, dst, move); err != nil {
			return "", false, err
		}
		return dst, true, nil
	}
	dst = flowUnique(dst, false, reserved)
	if move {
		return dst, false, movePath(src, dst)
	}
	return dst, false, copyTree(src, dst)
}

func replaceItem(src, dst string, move bool) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() && dstInfo.IsDir() {
		return mergeTree(src, dst, move)
	}
	if srcInfo.IsDir() != dstInfo.IsDir() {
		return fmt.Errorf("无法覆盖不同类型：%s", filepath.Base(dst))
	}
	backup := dst + ".lumiwerk-replacing"
	if err := os.Rename(dst, backup); err != nil {
		return err
	}
	var putErr error
	if move {
		putErr = movePath(src, dst)
	} else {
		putErr = copyTree(src, dst)
	}
	if putErr != nil {
		_ = os.RemoveAll(dst)
		_ = os.Rename(backup, dst)
		return putErr
	}
	_ = os.RemoveAll(backup)
	return nil
}

func mergeTree(src, dst string, move bool) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if _, statErr := os.Stat(to); statErr == nil {
			if err := replaceItem(from, to, move); err != nil {
				return err
			}
			continue
		}
		if move {
			if err := movePath(from, to); err != nil {
				return err
			}
		} else if err := copyTree(from, to); err != nil {
			return err
		}
	}
	if move {
		_ = os.Remove(src)
	}
	return nil
}

func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return err
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func remapPath(path string, mappings []pathPair) string {
	bestFrom := ""
	bestTo := ""
	for _, mapping := range mappings {
		from := filepath.Clean(mapping.From)
		if path == from || strings.HasPrefix(path, from+string(filepath.Separator)) {
			if len(from) > len(bestFrom) {
				bestFrom = from
				bestTo = filepath.Clean(mapping.To)
			}
		}
	}
	if bestFrom == "" {
		return path
	}
	return bestTo + path[len(bestFrom):]
}

func swappedMappings(mappings []pathPair) []pathPair {
	out := make([]pathPair, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.From == mapping.To {
			continue
		}
		out = append(out, pathPair{From: mapping.To, To: mapping.From})
	}
	return out
}

// quickRenameMappings applies a FlowVision rule such as {folder}_{index:03}
// to the files in their current order, skipping names owned by other files.
func quickRenameMappings(paths []string, folderName, rule string) []pathPair {
	if strings.TrimSpace(rule) == "" {
		rule = "{folder}_{index}"
	}
	if strings.TrimSpace(folderName) == "" {
		folderName = "Folder"
	}
	originals := map[string]struct{}{}
	for _, path := range paths {
		originals[pathKey(path)] = struct{}{}
	}
	planned := map[string]struct{}{}
	var out []pathPair
	for i, original := range paths {
		base := expandRenameRule(rule, folderName, filepath.Base(filepath.Dir(original)), i+1, original)
		base = strings.TrimSpace(base)
		if base == "" {
			base = fmt.Sprintf("%s_%d", folderName, i+1)
		}
		ext := filepath.Ext(original)
		final := original
		for suffix := 1; ; suffix++ {
			candidateBase := base
			if suffix > 1 {
				candidateBase = fmt.Sprintf("%s_%d", base, suffix)
			}
			name := candidateBase + ext
			candidate := filepath.Join(filepath.Dir(original), name)
			key := pathKey(candidate)
			_, inSelection := originals[key]
			existsOutside := false
			if _, err := os.Stat(candidate); err == nil && !inSelection {
				existsOutside = true
			}
			if _, taken := planned[key]; !existsOutside && !taken {
				planned[key] = struct{}{}
				final = candidate
				break
			}
		}
		out = append(out, pathPair{From: original, To: final})
	}
	return out
}

func expandRenameRule(rule, folder, parent string, index int, path string) string {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := rule
	out = replaceIndexedToken(out, index)
	out = strings.ReplaceAll(out, "{folder}", folder)
	out = strings.ReplaceAll(out, "{name}", name)
	out = strings.ReplaceAll(out, "{ext}", ext)
	out = strings.ReplaceAll(out, "{parent}", parent)
	return out
}

func replaceIndexedToken(s string, index int) string {
	for {
		start := strings.Index(s, "{index")
		if start < 0 {
			return s
		}
		end := strings.Index(s[start:], "}")
		if end < 0 {
			return s
		}
		token := s[start : start+end+1]
		width := 0
		if rest, ok := strings.CutPrefix(token, "{index:"); ok {
			rest = strings.TrimSuffix(rest, "}")
			for _, r := range rest {
				if r < '0' || r > '9' {
					width = 0
					break
				}
				width = width*10 + int(r-'0')
			}
		}
		var replacement string
		if width > 0 {
			replacement = fmt.Sprintf("%0*d", width, index)
		} else {
			replacement = fmt.Sprintf("%d", index)
		}
		s = s[:start] + replacement + s[start+end+1:]
	}
}

// wheelZoomFactor is the large-view scroll multiplier. Notches jump by 10%,
// precise trackpad deltas stay proportional, and inertial scrolls do not zoom.
func wheelZoomFactor(delta float32, precise, momentum bool) float32 {
	if momentum || delta == 0 {
		return 1
	}
	if precise {
		factor := 1 + delta/400
		if factor < 0.5 {
			return 0.5
		}
		if factor > 1.5 {
			return 1.5
		}
		return factor
	}
	if delta > 0 {
		return 1.1
	}
	return 1 / 1.1
}

// edgeZones matches FlowVision's 15% edge-click bands, clamped to 100–200pt.
func edgeZones(width float32) (left, right float32) {
	if width <= 0 {
		return 0, 0
	}
	left = width * 0.15
	if left < 100 {
		left = 100
	}
	if left > 200 {
		left = 200
	}
	if left > width/2 {
		left = width / 2
	}
	return left, width - left
}

// seekPreview maps a horizontal drag to a media timestamp the way FlowVision
// maps the large-view width onto the whole duration.
func seekPreview(start, dx, width, duration float64) float64 {
	if width <= 0 || duration <= 0 {
		return start
	}
	next := start + dx*duration/width
	if next < 0 {
		return 0
	}
	if next > duration {
		return duration
	}
	return next
}
