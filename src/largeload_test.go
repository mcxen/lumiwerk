package main

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The large-view open path: the decode edge policy and that the opened
// bitmap of an oversized source is resampled, not full size, while a
// small source keeps its pixels.
func TestLargeStageEdge(t *testing.T) {
	if got := largeStageEdge(0); got != 2560 {
		t.Fatalf("fallback edge = %d, want 2560 (1280 * 2)", got)
	}
	if got := largeStageEdge(800); got != 1600 {
		t.Fatalf("800pt stage edge = %d, want 1600", got)
	}
	if got := largeStageEdge(1000); got != 2000 {
		t.Fatalf("1000pt stage edge = %d, want 2000 (2x oversample)", got)
	}
	if got := largeStageEdge(9000); got != largeMaxSide {
		t.Fatalf("huge stage edge = %d, want cap %d", got, largeMaxSide)
	}
}

func TestDecodeLargeImageResamplesOversizedSources(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.png")
	writeSizedPNG(t, small, 64, 48)

	data, err := decodeLargeImage(small, 1600)
	if err != nil {
		t.Fatal(err)
	}
	w, h := decodeSize(t, data)
	if w != 64 || h != 48 {
		t.Fatalf("small source resampled to %dx%d, want original 64x48", w, h)
	}

	big := filepath.Join(dir, "big.png")
	writeSizedPNG(t, big, 4000, 3000)
	data, err = decodeLargeImage(big, 1600)
	if err != nil {
		t.Fatal(err)
	}
	w, h = decodeSize(t, data)
	if w != 1600 || h != 1200 {
		t.Fatalf("oversized source decoded at %dx%d, want resample to 1600x1200", w, h)
	}
}

func TestDecodeLargeBitmapRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.png")
	writeSizedPNG(t, path, 300, 200)
	bmp, err := decodeLargeBitmap(path, 1600)
	if err != nil {
		t.Fatal(err)
	}
	if bmp == nil {
		t.Fatal("no bitmap")
	}
	w, h := bmp.Size()
	if w != 300 || h != 200 {
		t.Fatalf("bitmap = %dx%d, want 300x200", w, h)
	}
}

// writeSizedPNG writes a real w×h gray PNG.
func writeSizedPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// decodeSize reads an image's pixel size back; the pipeline may hand back
// PNG (pass-through) or JPEG (sips resample output).
func decodeSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err == nil {
		b := img.Bounds()
		return b.Dx(), b.Dy()
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}
