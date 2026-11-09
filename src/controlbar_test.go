package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The image strip is built from the same shell and buttons as the video
// transport, so both must expose the shared controls and stay interactive.
func TestImageControlBarSharedComponents(t *testing.T) {
	a := newLargeState()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	for _, label := range []string{"上一张", "下一张", "缩小", "放大", "适应", "1:1", "顺时针旋转", "逆时针旋转", "镜像", "全屏"} {
		if !te.HasText(label) {
			t.Fatalf("image control bar missing %q: %v", label, te.Texts())
		}
	}
	if err := te.Click("适应"); err != nil {
		t.Fatal(err)
	}
	if err := te.Click("放大"); err != nil {
		t.Fatal(err)
	}
	if a.Zoom != 1.25 {
		t.Fatalf("zoom in = %v, want 1.25", a.Zoom)
	}
	if err := te.Click("镜像"); err != nil {
		t.Fatal(err)
	}
	if !a.Mirror {
		t.Fatal("mirror toggle did not change state")
	}
	// Still renders and responds after the toggle (accent state).
	te.Frame()
	if err := te.Click("顺时针旋转"); err != nil {
		t.Fatal(err)
	}
	if a.Rotation != 90 {
		t.Fatalf("rotation = %v, want 90", a.Rotation)
	}
}

// TestImageControlBarIsACenteredCapsule checks the strip floats as a capsule
// sized to its controls, in the bottom centre, not as a full-width bar.
func TestImageControlBarIsACenteredCapsule(t *testing.T) {
	a := newLargeState()
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()
	first, ok1 := te.Find("上一张")
	last, ok2 := te.Find("全屏")
	if !ok1 || !ok2 {
		t.Fatalf("bar ends missing: %v", te.Texts())
	}
	span := last.X + last.W - first.X
	if span > 640 {
		t.Fatalf("image bar spans %.0fpt; a capsule fits its controls", span)
	}
	if centre := (first.X + last.X + last.W) / 2; absf(centre-640) > 12 {
		t.Fatalf("image capsule is not centered: middle at %.0f of 1280", centre)
	}
	if bottom := last.Y + last.H; bottom > 820-barBottom || bottom < 820-barBottom-barHeight {
		t.Fatalf("image capsule bottom at %.0f, want within the bottom %dpt", bottom, barBottom+barHeight)
	}
	if dir := os.Getenv("LW_CARD_SHOTS"); dir != "" {
		f, _ := os.Create(filepath.Join(dir, "image-capsule.png"))
		_ = png.Encode(f, te.Image())
		f.Close()
	}
}

func TestBarVisibleModes(t *testing.T) {
	now := time.Now()
	idle := now.Add(-10 * time.Second)
	fresh := now.Add(-500 * time.Millisecond)
	cases := []struct {
		name         string
		mode         string
		moved        time.Time
		active, hold bool
		shown        bool
	}{
		{"always ignores idle", barAlways, idle, false, false, true},
		{"zero clock is test mode", barAuto, time.Time{}, false, false, true},
		{"auto fades when idle", barAuto, idle, false, false, false},
		{"auto stays within idle time", barAuto, fresh, false, false, true},
		{"auto holds while paused", barAuto, idle, false, true, true},
		{"hover ignores paused", barHover, idle, false, true, false},
		{"hover shows in the zone", barHover, idle, true, false, true},
		{"hover grace after leaving", barHover, now.Add(-300 * time.Millisecond), false, false, true},
		{"hover hides after grace", barHover, now.Add(-900 * time.Millisecond), false, false, false},
		{"auto shows when active", barAuto, idle, true, false, true},
	}
	for _, tc := range cases {
		shown, restart, wake := barVisible(tc.mode, tc.moved, now, tc.active, tc.hold)
		if shown != tc.shown {
			t.Errorf("%s: shown = %v, want %v", tc.name, shown, tc.shown)
		}
		if shown && wake == 0 && !restart && !tc.moved.IsZero() && tc.mode != barAlways {
			t.Errorf("%s: shown without wake or restart would never fade", tc.name)
		}
	}
	if normalizeBarMode("") != barAuto || normalizeBarMode("x") != barAuto || normalizeBarMode(barHover) != barHover {
		t.Error("normalizeBarMode")
	}
}
