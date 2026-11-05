package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewMenuIsShortAndLabelled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Profiles: prefsFile{View: defaultViewPreferences()}}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().ContextMenu(a.viewMenu)
	}, 400, 300)
	te.Frame()
	te.RightClickAt(200, 150)
	te.Frame()
	labels := te.Menu()
	t.Log(labels)
	items := 0
	for _, l := range labels {
		if strings.TrimSpace(l) == "" {
			t.Fatalf("blank entry in %q", labels)
		}
		if l != "-" {
			items++
		}
		for _, dup := range []string{"无字模式", "隐藏/显示侧栏", "黑暗模式", "显示图像文件", "说明…", "返回上一级目录"} {
			if l == dup {
				t.Fatalf("%q should not be in the view menu: %q", dup, labels)
			}
		}
	}
	if items > 10 {
		t.Fatalf("view menu has %d entries: %q", items, labels)
	}
}

func TestShortcutRebindAndConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Profiles: prefsFile{View: defaultViewPreferences()}}
	keyCapture.id = "pin"
	a.captureKey("pin", 0, ui.Key1) // taken by 最大化窗口
	if keyCapture.conflict == "" || a.binding("pin") != shortcutDefault("pin") {
		t.Fatalf("conflict not refused: %+v %v", keyCapture, a.binding("pin"))
	}
	a.captureKey("pin", ui.Cmd|ui.Shift, ui.KeyP)
	if got := a.binding("pin"); got != (keyBinding{ui.Cmd | ui.Shift, ui.KeyP}) || keyCapture.id != "" {
		t.Fatalf("rebind = %v", got)
	}
	if acc := a.accel("pin"); acc != "CmdOrCtrl+Shift+P" {
		t.Fatalf("accel = %q", acc)
	}
	a.setBinding("pin", shortcutDefault("pin"))
	if len(a.Profiles.Shortcuts) != 0 {
		t.Fatalf("reset kept override: %v", a.Profiles.Shortcuts)
	}
}
