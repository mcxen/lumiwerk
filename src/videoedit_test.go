package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCenterCropAndTrimName(t *testing.T) {
	got, err := centerCropFilter(1919, 1080)
	if err != nil || got != "crop=1918:1080:(iw-1918)/2:(ih-1080)/2,setsar=1" {
		t.Fatalf("filter = %s err=%v", got, err)
	}
	if _, err := centerCropFilter(1, 80); err == nil {
		t.Fatal("tiny crop should fail")
	}
	name := trimOutputName("/tmp/clip.mp4", 1.25, 2.5, 0)
	if name != "clip_clip_01_000001250-000002500.mp4" {
		t.Fatalf("name = %s", name)
	}
}

func TestCropAndTrimTinyVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	makeCmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=320x240:d=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-shortest", "-c:v", "mpeg4", "-c:a", "aac", src)
	if out, err := makeCmd.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize video: %v %s", err, out)
	}
	if err := cropVideoFile(src, 160, 120); err != nil {
		t.Fatal(err)
	}
	w, h := probedSize(t, src)
	if w != 160 || h != 120 {
		t.Fatalf("cropped = %dx%d", w, h)
	}
	dest := filepath.Join(dir, trimOutputName(src, 0, 0.4, 0))
	if err := trimVideoLossless(src, dest, 0, 0.4); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dest); err != nil || info.Size() == 0 {
		t.Fatalf("trim output missing: %v", err)
	}
}

func probedSize(t *testing.T, path string) (int, int) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0:s=x", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		t.Fatalf("probe = %q", out)
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil {
		t.Fatal(errW, errH)
	}
	return w, h
}

func TestRectCropFilterClampsEven(t *testing.T) {
	got, err := rectCropFilter(1919, 1080, 11, 7, 100, 81)
	if err != nil || got != "crop=100:80:10:6,setsar=1" {
		t.Fatalf("filter = %s err=%v", got, err)
	}
	got, err = rectCropFilter(320, 240, 300, 200, 100, 100)
	if err != nil || got != "crop=20:40:300:200,setsar=1" {
		t.Fatalf("clamped = %s err=%v", got, err)
	}
	if _, err := rectCropFilter(100, 80, 99, 0, 10, 10); err == nil {
		t.Fatal("crop past the right edge should fail")
	}
}

func TestVideoRotateFilter(t *testing.T) {
	got, err := videoRotateFilter(90)
	if err != nil || got != "transpose=1" {
		t.Fatalf("90 = %s %v", got, err)
	}
	got, err = videoRotateFilter(180)
	if err != nil || got != "transpose=1,transpose=1" {
		t.Fatalf("180 = %s %v", got, err)
	}
	got, err = videoRotateFilter(270)
	if err != nil || got != "transpose=2" {
		t.Fatalf("270 = %s %v", got, err)
	}
	if _, err := videoRotateFilter(45); err == nil {
		t.Fatal("expected unsupported angle")
	}
}

func TestRotateVideoSwapsEdges(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "wide.mp4")
	makeCmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=blue:s=320x240:d=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-shortest", "-c:v", "mpeg4", "-c:a", "aac", src)
	if out, err := makeCmd.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize video: %v %s", err, out)
	}
	if err := rotateVideoFile(src, 90); err != nil {
		t.Fatal(err)
	}
	w, h := probedSize(t, src)
	if w != 240 || h != 320 {
		t.Fatalf("rotated size = %dx%d", w, h)
	}
}
