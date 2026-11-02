package main

import (
	"path/filepath"
	"testing"
)

func TestProbeExifReadsCameraAndDate(t *testing.T) {
	path := filepath.Join("testdata", "exif-sample.jpg")
	var info mediaInfo
	probeExif(path, &info)
	if info.Camera != "TestCam" {
		t.Errorf("Camera = %q, want TestCam", info.Camera)
	}
	if info.Model != "X100" {
		t.Errorf("Model = %q, want X100", info.Model)
	}
	if info.Created != "2024-05-01 10:00:00" {
		t.Errorf("Created = %q, want 2024-05-01 10:00:00", info.Created)
	}
}

func TestProbeExifSilentOnNonImage(t *testing.T) {
	var info mediaInfo
	probeExif("no-such-file.jpg", &info)
	if info.Camera != "" || info.Created != "" {
		t.Errorf("probeExif on a missing file must leave fields empty, got %+v", info)
	}
}
