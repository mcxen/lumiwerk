package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCaptureKeyIgnoresUnknownKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Profiles: prefsFile{View: defaultViewPreferences()}}
	keyCapture.id = "pin"
	a.captureKey("pin", 0, ui.KeyUnknown)
	if keyCapture.id != "pin" {
		t.Fatal("unknown key ended capture")
	}
	if a.binding("pin").Key == ui.KeyUnknown {
		t.Fatal("unknown key stored as a shortcut")
	}
	keyCapture.id = ""
}

func TestLargeInputPassesUnknownKeys(t *testing.T) {
	a := &appState{Chosen: -1, LargeIndex: -1}
	item := &mediaItem{Name: "a.jpg", Ext: "jpg"}
	if a.largeInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUnknown}, item) {
		t.Fatal("unknown key was consumed")
	}
}

func TestGestureCanceledPressDoesNotNavigate(t *testing.T) {
	a := &appState{Chosen: -1, LargeIndex: -1, Root: "/album"}
	if !a.handleGesture(ui.InputEvent{Kind: ui.InputPointerDown, Button: 1, X: 10, Y: 10}) {
		t.Fatal("right press not taken")
	}
	if !a.Gesture.active {
		t.Fatal("gesture not started")
	}
	if !a.handleGesture(ui.InputEvent{Kind: ui.InputPointerUp, Button: 1, Canceled: true, X: 200, Y: 10}) {
		t.Fatal("canceled release not taken")
	}
	if a.Gesture.active {
		t.Fatal("canceled press left a gesture")
	}
	if a.Root != "/album" {
		t.Fatalf("canceled drag navigated to %s", a.Root)
	}
}

func TestLargeDragCanceledClearsState(t *testing.T) {
	a := &appState{Chosen: -1, LargeIndex: -1}
	item := &mediaItem{Name: "a.jpg", Ext: "jpg"}
	a.largeInput(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: 10, Y: 10}, item)
	if !a.LargeDrag.active {
		t.Fatal("drag not started")
	}
	if !a.largeInput(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, Canceled: true, X: 40, Y: 40}, item) {
		t.Fatal("canceled release not taken")
	}
	if a.LargeDrag.active {
		t.Fatal("canceled press left a large drag")
	}
}
