package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
)

// probeExif fills the camera and shooting-time fields of info from the
// file's EXIF metadata. Failures are silent: a file without EXIF simply
// leaves the fields empty.
func probeExif(path string, info *mediaInfo) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".tiff", ".tif", ".heic", ".heif", ".png", ".webp":
	default:
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	x, _ := imagemeta.Decode(f) // partial results are still useful
	info.Camera = strings.TrimSpace(x.IFD0.Make)
	info.Model = strings.TrimSpace(x.IFD0.Model)
	dt := x.ExifIFD.DateTimeOriginal
	if dt.IsZero() {
		dt = x.IFD0.ModifyDate
	}
	if !dt.IsZero() {
		info.Created = dt.Format("2006-01-02 15:04:05")
	}
	if info.Camera == "" && info.Model == "" && info.Created == "" {
		exifFallback(f, info)
	}
}

// exifFallback reads Make, Model and the shooting time with random access
// over the file head. imagemeta parses forward only and misses IFDs that
// sit after their data (GoPro and some older cameras write them so).
func exifFallback(f *os.File, info *mediaInfo) {
	buf := make([]byte, 256<<10)
	n, _ := f.ReadAt(buf, 0)
	b := buf[:n]
	start := bytes.Index(b, []byte("Exif\x00\x00"))
	if start >= 0 {
		b = b[start+6:]
	}
	if len(b) < 8 {
		return
	}
	var bo binary.ByteOrder
	switch string(b[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return
	}
	str := func(off, count uint32) string {
		if count <= 4 || int(off)+int(count) > len(b) {
			return ""
		}
		return strings.TrimRight(string(b[off:off+count]), "\x00 ")
	}
	var dateOrig, date string
	walk := func(ifd uint32) (sub uint32) {
		if int(ifd)+2 > len(b) {
			return 0
		}
		cnt := int(bo.Uint16(b[ifd:]))
		for i := 0; i < cnt; i++ {
			e := int(ifd) + 2 + 12*i
			if e+12 > len(b) {
				break
			}
			tag, count, val := bo.Uint16(b[e:]), bo.Uint32(b[e+4:]), bo.Uint32(b[e+8:])
			switch tag {
			case 0x010f:
				info.Camera = strings.TrimSpace(str(val, count))
			case 0x0110:
				info.Model = strings.TrimSpace(str(val, count))
			case 0x0132:
				date = str(val, count)
			case 0x9003:
				dateOrig = str(val, count)
			case 0x8769:
				sub = val
			}
		}
		return sub
	}
	if sub := walk(bo.Uint32(b[4:])); sub > 0 {
		walk(sub)
	}
	if dateOrig == "" {
		dateOrig = date
	}
	if t, err := time.ParseInLocation("2006:01:02 15:04:05", dateOrig, time.Local); err == nil {
		info.Created = t.Format("2006-01-02 15:04:05")
	}
}
