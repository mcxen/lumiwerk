package main

import (
	"testing"
)

func TestInstallerAndExpandedArchiveNames(t *testing.T) {
	for _, name := range []string{"App.dmg", "pkg.pkg", "Setup.EXE", "a.msi", "x.AppImage", "a.deb", "b.rpm", "c.apk", "Foo.app", "Bar.APP"} {
		if !isInstallerName(name) {
			t.Fatalf("expected installer: %s", name)
		}
	}
	if isInstallerName("photo.jpg") || isInstallerName("archive.zip") {
		t.Fatal("non-installer matched")
	}
	if !isInstallerBundle("Foo.app") || isInstallerBundle("Foo.dmg") {
		t.Fatal("bundle detection mismatch")
	}
	for _, name := range []string{"a.rar", "b.7z", "c.tar.gz", "d.gz", "e.iso", "f.cab", "g.zst"} {
		if !isArchiveName(name) {
			t.Fatalf("expected archive: %s", name)
		}
	}
}

func TestEnabledFileIncludesInstallers(t *testing.T) {
	view := defaultViewPreferences()
	view.AllTypes = false
	view.Images, view.RAW, view.Videos, view.Archives = false, false, false, false
	view.Installers = true
	if !isEnabledFile("Setup.dmg", "dmg", view) || !isEnabledFile("x.pkg", "pkg", view) {
		t.Fatal("dmg/pkg should pass the installer filter")
	}
	if !isEnabledFile("Tool.app", "app", view) {
		t.Fatal(".app should pass the installer filter")
	}
	if isEnabledFile("photo.jpg", "jpg", view) {
		t.Fatal("image should stay hidden when only installers are enabled")
	}
	view.Installers = false
	view.Archives = true
	if !isEnabledFile("pack.7z", "7z", view) || !isEnabledFile("pack.rar", "rar", view) {
		t.Fatal("expanded archives should pass the archive filter")
	}
}

func TestSplitJoinSortKey(t *testing.T) {
	base, desc := splitSortKey("size-desc")
	if base != "size" || !desc || joinSortKey(base, desc) != "size-desc" {
		t.Fatalf("size-desc => %s %v", base, desc)
	}
	base, desc = splitSortKey("name")
	if base != "name" || desc || joinSortKey(base, desc) != "name" {
		t.Fatalf("name => %s %v", base, desc)
	}
	base, desc = splitSortKey("random")
	if base != "name" {
		t.Fatalf("unknown base should fall back to name, got %s", base)
	}
}
