//go:build windows

package main

import (
	"context"
	"strings"
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

func TestExecAppCommandDoesNotHideGUIWindow(t *testing.T) {
	attr := execAppCommand("rundll32", "url.dll,FileProtocolHandler", `C:\test.jpg`).SysProcAttr
	if attr == nil {
		t.Fatal("app command: missing SysProcAttr")
	}
	if attr.HideWindow {
		t.Fatal("app command: HideWindow must be false so GUI apps do not receive SW_HIDE")
	}
	if attr.CreationFlags&0x08000000 == 0 {
		t.Fatalf("app command: CREATE_NO_WINDOW not set (%#x)", attr.CreationFlags)
	}
}

func TestWindowsTrashScriptTargetsRecycleBin(t *testing.T) {
	script := windowsTrashScript(`C:\Users\o'neil\pic.jpg`)
	if !strings.Contains(script, ".NameSpace(10).MoveHere(") {
		t.Fatalf("script = %q, want Recycle Bin NameSpace(10)", script)
	}
	if !strings.Contains(script, `'C:\Users\o''neil\pic.jpg'`) {
		t.Fatalf("script did not escape single quotes: %q", script)
	}
}
