package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"time"
)

// What the menu-bar icon, its optional title and the panel say about the
// app: a background task, a playing video, or nothing.

const (
	trayDefaultRecent = 8
	trayDefaultHotkey = "CmdOrCtrl+Alt+L"

	trayTabRecent    = "recent"
	trayTabFavorites = "favorites"
	trayTabTabs      = "tabs"
)

type trayIconKind int

const (
	trayIconIdle trayIconKind = iota
	trayIconBusy
	trayIconPlaying
)

// trayStatus is a snapshot taken on the main thread.
type trayStatus struct {
	Task      string
	TaskDone  int
	TaskTotal int
	// Media is the video loaded in the large view, "" when none.
	Media    string
	Playing  bool
	Position float64
	Duration float64
}

func (s trayStatus) HasTask() bool  { return s.Task != "" }
func (s trayStatus) HasMedia() bool { return s.Media != "" }

// TaskProgress is 0..1, or -1 while the total is unknown.
func (s trayStatus) TaskProgress() float64 {
	if s.TaskTotal <= 1 {
		return -1
	}
	return float64(min(s.TaskDone, s.TaskTotal)) / float64(s.TaskTotal)
}

func (s trayStatus) Icon() trayIconKind {
	switch {
	case s.Playing:
		return trayIconPlaying
	case s.HasTask():
		return trayIconBusy
	}
	return trayIconIdle
}

// Title is the short text beside the icon: task progress wins over the
// playback clock because a task is the thing the user is waiting for.
func (s trayStatus) Title() string {
	if s.HasTask() {
		if p := s.TaskProgress(); p >= 0 {
			return fmt.Sprintf("%d%%", int(p*100))
		}
		return s.Task
	}
	if s.Playing {
		return "▶ " + formatClock(s.Position)
	}
	return ""
}

// trayStatusNow reads the work tracker and the playback session. Main
// thread: a.Video belongs to it.
func (a *appState) trayStatusNow() trayStatus {
	var s trayStatus
	w := &a.work
	w.mu.Lock()
	if w.label != "" && time.Since(w.since) >= workShowDelay {
		s.Task, s.TaskDone, s.TaskTotal = w.label, w.done, w.total
	}
	w.mu.Unlock()
	if v := a.Video; v != nil && v.kernel != nil && v.path != "" {
		snap := v.kernel.Snapshot()
		s.Media = pathName(v.path)
		s.Playing = snap.Playing && !snap.Ended
		s.Position, s.Duration = snap.Position, snap.Duration
	}
	return s
}

func (p *prefsFile) trayRecentCount() int {
	if p.TrayRecent <= 0 {
		return trayDefaultRecent
	}
	return min(max(p.TrayRecent, 3), 20)
}

func (p *prefsFile) trayHotkey() string {
	if p.TrayHotkeyOff {
		return ""
	}
	if strings.TrimSpace(p.TrayHotkey) == "" {
		return trayDefaultHotkey
	}
	return p.TrayHotkey
}

func normalizeTrayPrefs(p *prefsFile) {
	switch p.TrayTab {
	case trayTabRecent, trayTabFavorites, trayTabTabs:
	default:
		p.TrayTab = trayTabRecent
	}
	if p.TrayRecent != 0 {
		p.TrayRecent = p.trayRecentCount()
	}
}

// trayRecentFolders is the newest-first folder list for the panel and the
// menu: existing folders only, at most n.
func (a *appState) trayRecentFolders(n int) []string {
	var out []string
	for _, path := range a.Recent {
		if len(out) >= n {
			break
		}
		if path != "" && isDir(path) {
			out = append(out, path)
		}
	}
	return out
}

// trayFavoriteFolders lists the sidebar favorites that still exist.
func (a *appState) trayFavoriteFolders() []string {
	var out []string
	for _, path := range a.Profiles.Favorites {
		if path != "" && isDir(path) {
			out = append(out, path)
		}
	}
	return out
}

// trayIconVariant draws a status mark in the corner of the template icon:
// a dot for a running task, a play triangle while media plays. A clear
// ring around the mark keeps it legible once macOS tints the icon.
func trayIconVariant(base []byte, kind trayIconKind) []byte {
	if kind == trayIconIdle {
		return base
	}
	src, err := png.Decode(bytes.NewReader(base))
	if err != nil {
		return base
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	size := min(b.Dx(), b.Dy())
	r := max(size/6, 2)
	cx, cy := b.Max.X-r-1, b.Max.Y-r-1
	ink := color.NRGBA{A: 255}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy > (r+2)*(r+2) {
				continue
			}
			inside := dx*dx+dy*dy <= r*r
			if kind == trayIconPlaying {
				// right-pointing triangle inside the same disc
				inside = inside && dx*2 <= r*2-trayAbs(dy)*2+r/2 && dx >= -r*2/3
			}
			if inside {
				dst.SetNRGBA(x, y, ink)
			} else {
				dst.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return base
	}
	return out.Bytes()
}

func trayAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
