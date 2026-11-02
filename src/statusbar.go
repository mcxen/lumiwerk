package main

import (
	"fmt"
	"strings"
	"time"
)

// New windows open in the justified (aspect-ratio) layout with a smaller
// thumbnail, so more fit per screen; saved profiles keep their own choice.
const (
	defaultThumb  = 150
	defaultLayout = 3
)

// gridCellPad is the room around a thumbnail in the icon grid.
const gridCellPad = 16

// statusText describes the selection from fields already in memory, so
// building the bar never touches the disk: name, size, dimensions and
// modified date of one item, totals for several, else the media count.
// The folder itself is named once, in the breadcrumb.
func (a *appState) statusText() string {
	if a.Root == "" {
		return "未打开目录"
	}
	paths := a.selectedPaths()
	switch {
	case len(paths) > 1:
		var total int64
		set := make(map[string]bool, len(paths))
		for _, p := range paths {
			set[p] = true
		}
		for i := range a.Items {
			if set[a.Items[i].Path] {
				total += a.Items[i].Size
			}
		}
		return fmt.Sprintf("已选 %d 项 · %s", len(paths), formatBytes(total))
	case len(paths) == 1:
		for i := range a.Items {
			if a.Items[i].Path == paths[0] {
				if !a.Items[i].IsDir && !a.Items[i].InfoLoaded {
					a.requestInfo(i) // async; dimensions show on a later frame
				}
				return itemStatus(&a.Items[i])
			}
		}
	}
	return fmt.Sprintf("%d 个媒体", a.mediaCount())
}

func itemStatus(it *mediaItem) string {
	parts := []string{it.Name}
	if it.IsDir {
		return it.Name
	}
	parts = append(parts, formatBytes(it.Size))
	if it.Info.Width > 0 && it.Info.Height > 0 {
		parts = append(parts, fmt.Sprintf("%d×%d", it.Info.Width, it.Info.Height))
	}
	if it.Info.Duration != "" {
		parts = append(parts, it.Info.Duration)
	}
	if it.ModTime > 0 {
		parts = append(parts, modTimeString(it.ModTime))
	}
	return strings.Join(parts, " · ")
}

// modTimeString formats a modification time in Unix seconds.
func modTimeString(v int64) string { return time.Unix(v, 0).Format("2006-01-02 15:04") }
