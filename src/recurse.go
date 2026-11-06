package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// dirDepth is how many folders below root contain path. root is 0,
// root/a is 1, root/a/b.jpg is 2. A path outside root is 0.
func dirDepth(root, path string) int {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if path == root {
		return 0
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// recurseTooDeep reports that path is more than maxDepth folders below root.
// maxDepth 0 means every level is included.
func recurseTooDeep(root, path string, maxDepth int) bool {
	return maxDepth > 0 && dirDepth(root, path) > maxDepth
}

// acceptScanEntry is the recursive listing rule: hidden entries, the depth
// limit, optional folder rows, then the type filter.
func acceptScanEntry(root string, entry scanEntry, view viewPreferences, recursive bool, maxDepth int) scanEntryResult {
	path := entry.Path
	if entry.IsDir {
		if path != root && !view.Hidden && entry.Hidden {
			return scanEntryResult{skipDir: true}
		}
		if recursive && recurseTooDeep(root, path, maxDepth) {
			return scanEntryResult{skipDir: true}
		}
		// .app bundles are install packages listed as one file.
		if path != root && isInstallerBundle(entry.Name) {
			ext := "app"
			if !(view.AllTypes || isEnabledFile(path, ext, view)) {
				return scanEntryResult{skipDir: true}
			}
			size := int64(0)
			if entry.Info != nil {
				size = entry.Size
			}
			return scanEntryResult{keep: true, skipDir: true, item: mediaItem{
				Path: path, Name: entry.Name, Ext: ext, Size: size,
				ModTime: entry.ModTime.Unix(), InfoLoaded: true,
			}}
		}
		if recursive && view.RecursiveFolders && path != root && (!view.Hidden || !entry.Hidden) {
			return scanEntryResult{keep: true, item: mediaItem{Path: path, Name: entry.Name, ModTime: entry.ModTime.Unix(), IsDir: true, InfoLoaded: true}}
		}
		return scanEntryResult{}
	}
	if recursive && recurseTooDeep(root, filepath.Dir(path), maxDepth) {
		return scanEntryResult{}
	}
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if !view.Hidden && isHiddenPath(root, path) {
		return scanEntryResult{}
	}
	if !(view.AllTypes || isEnabledFile(path, ext, view)) {
		return scanEntryResult{}
	}
	if entry.Info == nil {
		return scanEntryResult{}
	}
	return scanEntryResult{keep: true, item: mediaItem{
		Path: path, Name: entry.Name, Ext: strings.ToLower(ext),
		Size: entry.Size, ModTime: entry.ModTime.Unix(), IsVideo: isVideoFile(ext),
	}}
}

// folderInRecurseScope is a subfolder of the open directory that the
// current depth still includes. The sidebar draws an amber edge on those.
func (a *appState) folderInRecurseScope(path string) bool {
	if !a.Recursive || a.Root == "" {
		return false
	}
	path = filepath.Clean(path)
	root := filepath.Clean(a.Root)
	if path == root {
		return false
	}
	depth := dirDepth(root, path)
	if depth < 1 {
		return false
	}
	maxDepth := a.Profiles.View.RecurseDepth
	return maxDepth == 0 || depth <= maxDepth
}

// sourceFolderLabel is the folder a file was flattened from, relative to
// the open directory. A file that already lives there has no label.
func sourceFolderLabel(root, path string) string {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Dir(filepath.Clean(path)))
	if err != nil || rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// recurseScopeStats counts the subfolders a flat listing opened and how
// the shown files split into pictures and videos. Other files count with
// the pictures so the two numbers add up to the total.
func recurseScopeStats(root string, items []mediaItem, visible []int) (folders, images, videos int) {
	seen := map[string]struct{}{}
	root = filepath.Clean(root)
	for _, idx := range visible {
		if idx < 0 || idx >= len(items) {
			continue
		}
		item := items[idx]
		if item.IsDir {
			continue
		}
		if item.IsVideo {
			videos++
		} else {
			images++
		}
		dir := filepath.Clean(filepath.Dir(item.Path))
		for dir != root {
			rel, err := filepath.Rel(root, dir)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				break
			}
			if _, ok := seen[dir]; !ok {
				seen[dir] = struct{}{}
				folders++
			}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}
	return folders, images, videos
}

func recurseSummaryText(folders, images, videos int) string {
	return fmt.Sprintf("已展开 %d 个子文件夹，平铺显示全部 %d 项（%d 图片 · %d 视频）", folders, images+videos, images, videos)
}

// formatClock is a short duration for a thumbnail corner: 1:23 or 1:02:03.
func formatClock(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	s := int(secs + 0.5)
	if s < 1 && secs > 0 {
		s = 1
	}
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// recurseControls is the toolbar switch and the depth next to it.
func (a *appState) recurseControls(c *ui.Context) {
	row := ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0)
	row.Children(func() {
		sw := ui.Switch(c.Key("recurse-switch"), &a.Recursive).Label("包含子文件夹（递归）")
		if sw.Changed() {
			a.setRecursive(a.Recursive)
		}
		ui.Text(c, "包含子文件夹（递归）").FontSize(12).SingleLine()
		depth := a.Profiles.View.RecurseDepth
		seg := ui.Segmented(c.Key("recurse-depth"), &depth, "全部层级", "1 层", "2 层").Label("递归深度").Disabled(!a.Recursive)
		if seg.Changed() {
			a.setRecurseDepth(depth)
		}
	})
}

// recurseBar is the line under the toolbar while subfolders are flattened.
func (a *appState) recurseBar(c *ui.Context) {
	if a.galleryActive() || a.InLargeView || !a.Recursive || a.Root == "" {
		return
	}
	folders, images, videos := recurseScopeStats(a.Root, a.Items, a.Visible)
	t := c.Theme()
	ui.Box(c).FillWidth().Padding(4, 12).Background(t.Surface).Children(func() {
		ui.Text(c, recurseSummaryText(folders, images, videos)).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
}

// paintThumbBadges draws the source folder on a waterfall card and the
// duration on any video thumbnail.
func (a *appState) paintThumbBadges(c *ui.Context, item *mediaItem, showFolder bool) {
	if item == nil || item.IsDir {
		return
	}
	if showFolder {
		if label := sourceFolderLabel(a.Root, item.Path); label != "" {
			thumbChip(c, label, ui.AnchorTopLeft)
		}
	}
	if item.IsVideo {
		if !item.InfoLoaded {
			for i := range a.Items {
				if &a.Items[i] == item {
					a.requestInfo(i)
					break
				}
			}
		}
		if item.Info.DurationSec > 0 {
			thumbChip(c, formatClock(item.Info.DurationSec), ui.AnchorBottomRight)
		}
	}
}

func thumbChip(c *ui.Context, text string, corner ui.Anchor) {
	chip := ui.Box(c).Attach(corner, corner).Margin(4).Padding(1, 5).Radius(3).Background(ui.RGBA(0, 0, 0, 0.55))
	chip.Children(func() {
		ui.Text(c, text).FontSize(10).TextColor(ui.RGB(255, 255, 255)).SingleLine().MaxWidth(180)
	})
}
