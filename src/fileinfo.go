package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func formatBytes(n int64) string {
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

// describeSelection is the single-file info sheet and the multi-file statistic.
func describeSelection(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return describeOne(paths[0])
	}
	var files, dirs int
	var size int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			dirs++
			continue
		}
		files++
		size += info.Size()
	}
	return fmt.Sprintf("共 %d 项\n文件: %d\n文件夹: %d\n大小: %s", len(paths), files, dirs, formatBytes(size))
}

func describeOne(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return path
	}
	kind := "文件"
	target := ""
	resolved := info
	resolvedPath := path
	if info.Mode()&os.ModeSymlink != 0 {
		kind = "符号链接"
		if dest, err := filepath.EvalSymlinks(path); err == nil {
			target = dest
			resolvedPath = dest
			if st, err := os.Stat(dest); err == nil {
				resolved = st
			}
		}
	} else if info.IsDir() {
		kind = "文件夹"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "名称: %s\n", info.Name())
	fmt.Fprintf(&b, "类型: %s\n", kind)
	if target != "" {
		fmt.Fprintf(&b, "原始路径: %s\n", target)
	}
	fmt.Fprintf(&b, "路径: %s\n", path)
	if resolved.IsDir() {
		b.WriteString(folderSummary(resolvedPath))
	} else {
		fmt.Fprintf(&b, "大小: %s\n", formatBytes(resolved.Size()))
	}
	if created := createdAtString(resolved); created != "" {
		fmt.Fprintf(&b, "创建: %s\n", created)
	}
	fmt.Fprintf(&b, "修改: %s\n", resolved.ModTime().Format("2006-01-02 15:04:05"))
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(resolvedPath)), ".")
	if isImageFile(ext) || isVideoFile(ext) {
		b.WriteString(formatMediaFacts(loadMediaInfo(resolvedPath, isVideoFile(ext))))
	}
	if tags, err := urlTags(path); err == nil && len(tags) > 0 {
		fmt.Fprintf(&b, "标签: %s\n", strings.Join(tags, "、"))
	}
	return strings.TrimRight(b.String(), "\n")
}

func folderSummary(root string) string {
	var files, dirs int
	var size int64
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if entry.IsDir() {
			dirs++
			return nil
		}
		files++
		if info, err := entry.Info(); err == nil {
			size += info.Size()
		}
		return nil
	})
	return fmt.Sprintf("包含文件: %d\n包含文件夹: %d\n合计: %s\n", files, dirs, formatBytes(size))
}

func formatMediaFacts(info mediaInfo) string {
	var b strings.Builder
	if info.Width > 0 && info.Height > 0 {
		fmt.Fprintf(&b, "尺寸: %d×%d\n", info.Width, info.Height)
	}
	if info.Format != "" {
		fmt.Fprintf(&b, "格式: %s\n", info.Format)
	}
	if info.Codec != "" {
		fmt.Fprintf(&b, "编码: %s\n", info.Codec)
	}
	if info.FrameRate != "" {
		fmt.Fprintf(&b, "帧率: %s fps\n", info.FrameRate)
	}
	if info.BitRate != "" {
		fmt.Fprintf(&b, "码率: %s\n", info.BitRate)
	}
	if info.Duration != "" {
		fmt.Fprintf(&b, "时长: %s\n", info.Duration)
	}
	if info.DPI != "" {
		fmt.Fprintf(&b, "分辨率: %s DPI\n", info.DPI)
	}
	if info.Bits != "" {
		fmt.Fprintf(&b, "位深: %s\n", info.Bits)
	}
	if info.Profile != "" {
		fmt.Fprintf(&b, "色彩: %s\n", info.Profile)
	}
	if cam := strings.TrimSpace(strings.TrimSpace(info.Camera) + " " + strings.TrimSpace(info.Model)); cam != "" {
		fmt.Fprintf(&b, "相机: %s\n", cam)
	}
	if info.Created != "" {
		fmt.Fprintf(&b, "拍摄: %s\n", info.Created)
	}
	return b.String()
}

func duplicateBeside(paths []string) ([]string, error) {
	var created []string
	var reserved []string
	for _, src := range paths {
		info, err := os.Stat(src)
		if err != nil {
			return created, err
		}
		dest := flowUnique(src, true, reserved)
		if info.IsDir() {
			err = copyTree(src, dest)
		} else {
			err = copyFile(src, dest)
		}
		if err != nil {
			return created, err
		}
		reserved = append(reserved, dest)
		created = append(created, dest)
	}
	return created, nil
}

// sharpReady reports whether zooming may use the original bitmap. A thumbnail
// stretched past fit looks soft, so the view stays fitted until the original arrives.
func sharpReady(item *mediaItem, zoomPercent int) bool {
	if item == nil || item.IsVideo || item.IsDir {
		return true
	}
	if item.Large != nil || zoomPercent <= 105 {
		return true
	}
	return false
}
