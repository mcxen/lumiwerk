// Pure-Go image decoding: the thumbnail pipeline for platforms without a
// system image service, and the fallback when sips cannot read a file.
package main

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"sync"

	"golang.org/x/image/draw"
)

// decodePool reuses JPEG readers' internal buffers across calls.
var decodeBufs sync.Pool

// goThumbnailJPEG decodes path and writes a JPEG thumbnail whose longest
// side is edge, entirely in this process: no child process, works on every
// platform, reads what image.Decode reads plus TIFF.
func goThumbnailJPEG(path string, edge int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// EXIF orientation for JPEGs that carry one.
	if o := jpegOrientation(data); o > 1 {
		src = orientImage(src, o)
	}
	return encodeThumbnailJPEG(src, edge, 80)
}

// encodeThumbnailJPEG downsamples src so its longest side is edge and
// encodes it. It never upsamples.
func encodeThumbnailJPEG(src image.Image, edge, quality int) ([]byte, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("image has no pixels")
	}
	scale := 1.0
	if w > edge || h > edge {
		scale = float64(edge) / float64(max(w, h))
	}
	dw, dh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	// ApproxBiLinear is 5-10x faster than BiLinear and fine at thumbnail
	// sizes; CatmullRom showed no visible win on photo content.
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// jpegOrientation reads the EXIF orientation tag from a JPEG's APP1 segment.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	pos := 2
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			break
		}
		marker := data[pos+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			pos += 2
			continue
		}
		if pos+4 > len(data) {
			break
		}
		segLen := int(data[pos+2])<<8 | int(data[pos+3])
		if segLen < 2 || pos+2+segLen > len(data) {
			break
		}
		if marker == 0xE1 && segLen > 10 && string(data[pos+4:pos+10]) == "Exif\x00\x00" {
			return tiffOrientationBytes(data[pos+10 : pos+2+segLen])
		}
		if marker == 0xDA { // start of scan: no EXIF ahead
			break
		}
		pos += 2 + segLen
	}
	return 1
}

// tiffOrientationBytes reads orientation from an EXIF TIFF block.
func tiffOrientationBytes(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	le := t[0] == 'I' && t[1] == 'I'
	u16 := func(off int) int {
		if off+2 > len(t) {
			return 0
		}
		if le {
			return int(t[off]) | int(t[off+1])<<8
		}
		return int(t[off])<<8 | int(t[off+1])
	}
	u32 := func(off int) int {
		if off+4 > len(t) {
			return 0
		}
		if le {
			return int(t[off]) | int(t[off+1])<<8 | int(t[off+2])<<16 | int(t[off+3])<<24
		}
		return int(t[off])<<24 | int(t[off+1])<<16 | int(t[off+2])<<8 | int(t[off+3])
	}
	if u16(2) != 42 {
		return 1
	}
	ifd := u32(4)
	entries := u16(ifd)
	if entries > 512 {
		return 1
	}
	for i := 0; i < entries; i++ {
		e := ifd + 2 + i*12
		if e+12 > len(t) {
			break
		}
		if u16(e) == 0x0112 { // orientation tag
			return u16(e + 8)
		}
	}
	return 1
}

// orientImage rotates/flips src per an EXIF orientation value.
func orientImage(src image.Image, o int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	switch o {
	case 2:
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(x, y, src.At(b.Min.X+w-1-x, b.Min.Y+y))
			}
		}
		return dst
	case 3:
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(x, y, src.At(b.Min.X+w-1-x, b.Min.Y+h-1-y))
			}
		}
		return dst
	case 4:
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(x, y, src.At(b.Min.X+x, b.Min.Y+h-1-y))
			}
		}
		return dst
	case 5, 6, 7, 8:
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				nx, ny := x, y
				switch o {
				case 6:
					nx, ny = h-1-y, x
				case 8:
					nx, ny = y, w-1-x
				case 5:
					nx, ny = h-1-y, w-1-x
				case 7:
					nx, ny = y, x
				}
				dst.Set(nx, ny, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	}
	return src
}
