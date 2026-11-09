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
		sw := ui.Switch(c.Key("recurse-switch"), &a.Recursive).Label("递归")
		if sw.Changed() {
			a.setRecursive(a.Recursive)
		}
		ui.Text(c, "递归").FontSize(12).SingleLine()
		if !a.Recursive || a.toolbarCompact {
			// Depth and folders mean nothing until recursion is on; in a
			// narrow window they move into the 显示与浏览 menu.
			return
		}
		// Depth and 含文件夹 share one compact menu, so recursion costs the
		// toolbar a single small button and the right-hand controls stay put.
		depth := a.Profiles.View.RecurseDepth
		names := []string{"全部层级", "1 层", "2 层"}
		ui.MenuButton(c.Key("recurse-options"), names[min(max(depth, 0), 2)], func(m *ui.Menu) {
			for i, name := range names {
				if m.Item(name).Checked(depth == i).Chosen() {
					a.setRecurseDepth(i)
				}
			}
			m.Separator()
			if m.Item("包含文件夹").Checked(a.Profiles.View.RecursiveFolders).Chosen() {
				a.toggleViewOption("folders")
			}
		}).Label("递归深度").Tooltip("递归深度与文件夹")
	})
}

// recurseNotice is the flattened-folder summary for the bottom status bar.
func (a *appState) recurseNotice() string {
	if a.galleryActive() || a.InLargeView || !a.Recursive || a.Root == "" {
		return ""
	}
	folders, images, videos := recurseScopeStats(a.Root, a.Items, a.Visible)
	return recurseSummaryText(folders, images, videos)
}

// paintThumbBadges draws the source folder on a waterfall card and the
// duration on any video thumbnail.
func (a *appState) paintThumbBadges(c *ui.Context, item *mediaItem, showFolder bool) {
	if item == nil || item.IsDir {
		return
	}
	s := cardScale(float32(a.Thumb))
	if showFolder {
		if label := sourceFolderLabel(a.Root, item.Path); label != "" {
			thumbChip(c, label, ui.AnchorTopLeft, s)
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
			thumbChip(c, formatClock(item.Info.DurationSec), ui.AnchorBottomRight, s)
		}
	}
}

func thumbChip(c *ui.Context, text string, corner ui.Anchor, s float32) {
	chip := ui.Box(c).Attach(corner, corner).Margin(4*s).Padding(1*s, 5*s).Radius(3 * s).Background(ui.RGBA(0, 0, 0, 0.55))
	chip.Children(func() {
		ui.Text(c, text).FontSize(10 * s).TextColor(ui.RGB(255, 255, 255)).SingleLine().MaxWidth(180 * s)
	})
}
