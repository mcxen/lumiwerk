//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func init() {
	finderTagsFallback = mdlsTags
}

// sameDevice compares stat device IDs, so a move stays a rename.
func sameDevice(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	statA, okA := infoA.Sys().(*syscall.Stat_t)
	statB, okB := infoB.Sys().(*syscall.Stat_t)
	if !okA || !okB {
		return false
	}
	return statA.Dev == statB.Dev
}

// fileCreated reports a file's birth time from the stat record.
func fileCreated(info os.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	t := time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
	if t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}

// fileHidden is always false on macOS: dotfiles cover hiding.
func fileHidden(info os.FileInfo) bool {
	return false
}

// platformTrash moves one path to the macOS Trash through Finder's native
// service, so files deleted from external volumes land in that volume's
// .Trashes, and it returns the new path for undo.
func platformTrash(path string) (string, error) {
	script := fmt.Sprintf(`tell application "Finder"
  set trashed to delete (POSIX file %s as alias)
  POSIX path of (trashed as alias)
end tell`, strconv.Quote(path))
	out, err := finderTrashCommand(script)
	if mapped := finderTrashError(out, err); mapped != nil {
		return "", mapped
	}
	trashed := strings.TrimSpace(string(out))
	if trashed == "" {
		return "", errors.New("废纸篓没有返回位置")
	}
	return trashed, nil
}

// mediaThumbJPEG builds a JPEG thumbnail of edge on the long side. On macOS
// the system ImageIO service (sips) decodes HEIC, RAW and everything else.
func mediaThumbJPEG(path string, video bool, edge int) ([]byte, error) {
	if video {
		return videoThumb(path, edge)
	}
	if edge < 64 {
		edge = 64
	}
	file, err := os.CreateTemp("", "lumiwerk-thumb-*.jpg")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	_ = file.Close()
	defer os.Remove(name)
	quality := "70"
	if edge >= 768 {
		quality = "82"
	}
	cmd := exec.Command("sips", "-Z", strconv.Itoa(edge), "-s", "format", "jpeg", "-s", "formatOptions", quality, path, "--out", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(name)
}

// platformVideoThumb asks QuickLook (qlmanage ships with macOS) for the
// cover of the formats AVFoundation reads.
func platformVideoThumb(path string, edge int) ([]byte, error) {
	dir, err := os.MkdirTemp("", "lumiwerk-ql-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	cmd := exec.Command("/usr/bin/qlmanage", "-t", "-s", strconv.Itoa(edge), "-o", dir, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("qlmanage: %v %s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.Base(path)+".png"))
	if err != nil {
		return nil, fmt.Errorf("qlmanage: no thumbnail: %w", err)
	}
	return data, nil
}

// platformTags reads Finder tags through the xattr metadata.
func platformTags(path string) ([]string, error) {
	out, err := exec.Command("xattr", "-px", finderTagXattr, path).Output()
	if err != nil || len(out) == 0 {
		return mdlsTags(path), nil
	}
	raw, err := hexDecode(string(out))
	if err != nil || len(raw) == 0 {
		return mdlsTags(path), nil
	}
	cmd := exec.Command("plutil", "-convert", "xml1", "-o", "-", "-")
	cmd.Stdin = strings.NewReader(string(raw))
	xmlOut, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	tags := plistStrings(xmlOut)
	for i, tag := range tags {
		if j := strings.IndexByte(tag, '\n'); j >= 0 {
			tags[i] = strings.TrimSpace(tag[:j])
		} else {
			tags[i] = strings.TrimSpace(tag)
		}
	}
	return nonEmpty(tags), nil
}

// mdlsTags asks Spotlight when the xattr read fails.
func mdlsTags(path string) []string {
	out, err := exec.Command("/usr/bin/mdls", "-name", "kMDItemUserTags", "-raw", path).Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	s := strings.TrimSpace(string(out))
	if s == "(null)" || s == "" {
		return nil
	}
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	parts := strings.Split(s, ",")
	var tags []string
	for _, p := range parts {
		tag := strings.Trim(strings.TrimSpace(p), `"`)
		if i := strings.IndexByte(tag, '\n'); i >= 0 {
			tag = tag[:i]
		}
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// platformSetTags writes Finder tags as the binary plist xattr Finder uses.
func platformSetTags(path string, tags []string) error {
	data, err := binaryPlistStrings(tags)
	if err != nil {
		return err
	}
	cmd := exec.Command("xattr", "-w", "-x", finderTagXattr, hexEncode(data), path)
	_, err = cmd.CombinedOutput()
	return err
}

func binaryPlistStrings(tags []string) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	b.WriteString(`<plist version="1.0"><array>`)
	for _, tag := range tags {
		b.WriteString("<string>")
		xmlEscape(&b, tag)
		b.WriteString("</string>")
	}
	b.WriteString(`</array></plist>`)
	cmd := exec.Command("plutil", "-convert", "binary1", "-o", "-", "-")
	cmd.Stdin = strings.NewReader(b.String())
	return cmd.Output()
}

func plistStrings(data []byte) []string {
	var tags []string
	for _, chunk := range strings.Split(string(data), "<string>") {
		end := strings.Index(chunk, "</string>")
		if end < 0 {
			continue
		}
		tags = append(tags, chunk[:end])
	}
	return tags
}

func xmlEscape(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
}

const finderTagXattr = "com.apple.metadata:_kMDItemUserTags"

func hexEncode(data []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0f]
	}
	return string(out)
}

func hexDecode(s string) ([]byte, error) {
	var digits []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		digits = append(digits, c)
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	for i := range out {
		hi, err := hexVal(digits[i*2])
		if err != nil {
			return nil, err
		}
		lo, err := hexVal(digits[i*2+1])
		if err != nil {
			return nil, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexVal(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("bad hex digit %q", c)
	}
}

// imageResample decodes through sips for formats Go's stdlib cannot read and
// for originals too large to decode in memory.
func imageResampleJPEG(path string, edge int, quality string) ([]byte, error) {
	file, err := os.CreateTemp("", "lumiwerk-resample-*.jpg")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	_ = file.Close()
	defer os.Remove(name)
	cmd := exec.Command("sips", "-Z", strconv.Itoa(edge), "-s", "format", "jpeg", "-s", "formatOptions", quality, path, "--out", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(name)
}

// decodeLargeImage decodes the original for 1:1 zoom. Sources well past
// the on-screen size are resampled to edge first: sips decodes at file
// speed and a smaller JPEG both reads and uploads faster, so a 48 MP
// photo opens in about half the time of a full 8000x6000 decode. The
// largeMaxSide guard stays, so one photo cannot exhaust memory.
func decodeLargeImage(path string, edge int) ([]byte, error) {
	w, h := probeImageSize(path)
	if w > largeMaxSide || h > largeMaxSide {
		return imageResampleJPEG(path, largeMaxSide, "82")
	}
	if edge > 0 && w > edge && h > edge {
		return imageResampleJPEG(path, edge, "88")
	}
	return os.ReadFile(path)
}

// finderLabelNames answers through the AppKit bridge in finder_darwin.go.
func finderLabelNames() []string { return finderLabelNamesDarwin() }
