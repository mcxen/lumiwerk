package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// gestureState tracks a right-button drag for FlowVision-style folder navigation.
type gestureState struct {
	active bool
	x, y   float32
}

const gestureThreshold = 48.0

func (a *appState) nextMediaFolder(delta int) {
	if a.Root == "" {
		return
	}
	parent := filepath.Dir(a.Root)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(parent, e.Name())
		if hasMedia(p) {
			dirs = append(dirs, p)
		}
	}
	for i, p := range dirs {
		if p == a.Root {
			j := i + delta
			if j >= 0 && j < len(dirs) {
				a.openFolder(dirs[j])
			}
			return
		}
	}
}

func hasMedia(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if isMediaFile(ext) {
			return true
		}
	}
	return false
}

func (a *appState) handleGesture(ev ui.InputEvent) bool {
	if ev.Button != 1 {
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		a.Gesture = gestureState{active: true, x: ev.X, y: ev.Y}
		return true
	case ui.InputPointerMove:
		if a.Gesture.active {
			a.Gesture.x, a.Gesture.y = ev.X, ev.Y
			return true
		}
	case ui.InputPointerUp:
		if !a.Gesture.active {
			return false
		}
		dx, dy := ev.X-a.Gesture.x, ev.Y-a.Gesture.y
		a.Gesture = gestureState{}
		if abs32(dx) < gestureThreshold && abs32(dy) < gestureThreshold {
			return true
		}
		if abs32(dy) > abs32(dx) {
			if dy < 0 {
				a.goUp()
			} else {
				a.goBack()
			}
		} else {
			if dx > 0 {
				a.nextMediaFolder(1)
			} else {
				a.nextMediaFolder(-1)
			}
		}
		return true
	}
	return false
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
