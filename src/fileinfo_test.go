package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescribeAndDuplicate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	text := describeSelection([]string{file})
	if !strings.Contains(text, "note.txt") || !strings.Contains(text, "5 B") {
		t.Fatalf("info = %s", text)
	}
	sub := filepath.Join(dir, "folder")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	stats := describeSelection([]string{file, sub})
	if !strings.Contains(stats, "共 2 项") || !strings.Contains(stats, "文件夹: 1") {
		t.Fatalf("stats = %s", stats)
	}
	created, err := duplicateBeside([]string{file, sub})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %#v", created)
	}
	got, err := os.ReadFile(created[0])
	if err != nil || string(got) != "hello" {
		t.Fatalf("copy = %q err=%v", got, err)
	}
	if info, err := os.Stat(created[1]); err != nil || !info.IsDir() {
		t.Fatalf("dir copy = %v err=%v", info, err)
	}
}

func TestFinderTagRoundTrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pic.jpg")
	if err := os.WriteFile(file, []byte("not-a-real-jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setURLTags(file, []string{"LumiwerkTest"}); err != nil {
		t.Fatal(err)
	}
	tags, err := urlTags(file)
	if err != nil || !containsString(tags, "LumiwerkTest") {
		t.Fatalf("tags = %#v err=%v", tags, err)
	}
	added, err := toggleFinderTag([]string{file}, "LumiwerkTest")
	if err != nil || added {
		t.Fatalf("added = %v err=%v", added, err)
	}
	tags, err = urlTags(file)
	if err != nil || containsString(tags, "LumiwerkTest") {
		t.Fatalf("after remove = %#v err=%v", tags, err)
	}
	labels := finderLabelNames()
	if len(labels) < 7 {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestSharpReadyWaitsForOriginal(t *testing.T) {
	item := &mediaItem{}
	if sharpReady(item, 200) {
		t.Fatal("zoomed thumbnail should wait")
	}
	item.Large = nil
	if !sharpReady(item, 100) {
		t.Fatal("fit zoom can use the thumbnail")
	}
	item.Large = nil
	item.IsVideo = true
	if !sharpReady(item, 400) {
		t.Fatal("video keeps its own scaler")
	}
}

func TestOpenerAppsForText(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loadObjC() != nil || classWorkspace == 0 {
		t.Skip("AppKit is not loaded in this process")
	}
	apps := openerApps(file)
	if len(apps) == 0 || apps[0].Name == "" || apps[0].Path == "" {
		t.Fatalf("apps = %#v", apps)
	}
}

func TestDescribeImageFolderAndLink(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "photo.png")
	f, err := os.Create(img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0x89, 0x50, 0x4e, 0x47}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	// A real PNG so sips can read the pixel size.
	png := filepath.Join(dir, "dot.png")
	if err := os.WriteFile(png, tinyPNG(), 0o644); err != nil {
		t.Fatal(err)
	}
	text := describeSelection([]string{png})
	if !strings.Contains(text, "尺寸: 1×1") || !strings.Contains(text, "格式: png") {
		t.Fatalf("image info = %s", text)
	}
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	folder := describeSelection([]string{dir})
	if !strings.Contains(folder, "包含文件: 3") || !strings.Contains(folder, "包含文件夹: 1") {
		t.Fatalf("folder = %s", folder)
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(png, link); err != nil {
		t.Fatal(err)
	}
	linked := describeSelection([]string{link})
	if !strings.Contains(linked, "符号链接") || !strings.Contains(linked, png) {
		t.Fatalf("link = %s", linked)
	}
}

func tinyPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x05, 0xfe, 0x02, 0xfe, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
