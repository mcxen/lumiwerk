package main

import "testing"

func TestPlatformLabels(t *testing.T) {
	for _, c := range []struct{ goos, root, reveal string }{
		{"darwin", "Macintosh HD", "在 Finder 中显示"},
		{"windows", "此电脑", "在资源管理器中显示"},
		{"linux", "/", "在文件管理器中显示"},
	} {
		if got := rootNameFor(c.goos); got != c.root {
			t.Errorf("%s root = %q, want %q", c.goos, got, c.root)
		}
		if got := revealLabelFor(c.goos); got != c.reveal {
			t.Errorf("%s reveal = %q, want %q", c.goos, got, c.reveal)
		}
	}
	if openParentLabel == revealLabelFor("darwin") {
		t.Error("open-parent and reveal must read differently")
	}
}
