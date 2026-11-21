//go:build windows

package main

import (
	"context"
	"testing"
)

func TestExecCommandHidesWindowsConsole(t *testing.T) {
	a := execCommand("ffmpeg", "-version").SysProcAttr
	if a == nil {
		t.Fatal("command: missing SysProcAttr")
	}
	if !a.HideWindow {
		t.Fatal("command: HideWindow not set")
	}
	if a.CreationFlags&0x08000000 == 0 {
		t.Fatalf("command: CREATE_NO_WINDOW not set (%#x)", a.CreationFlags)
	}
	b := execCommandContext(context.Background(), "ffprobe", "-version").SysProcAttr
	if b == nil {
		t.Fatal("context: missing SysProcAttr")
	}
	if !b.HideWindow {
		t.Fatal("context: HideWindow not set")
	}
	if b.CreationFlags&0x08000000 == 0 {
		t.Fatalf("context: CREATE_NO_WINDOW not set (%#x)", b.CreationFlags)
	}
}
