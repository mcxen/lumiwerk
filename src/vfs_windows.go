//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func execCommand(name string, args ...string) *exec.Cmd {
	// Hide the console window Windows would open for each child process.
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = hideConsoleAttr
	return cmd
}

// fileCreated reports a file's creation time, which NTFS keeps for real.
func fileCreated(info os.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, false
	}
	// Filetime is 100-ns intervals since 1601-01-01.
	nsec := int64(st.CreationTime.HighDateTime)<<32 | int64(st.CreationTime.LowDateTime)
	if nsec == 0 {
		return time.Time{}, false
	}
	const epochDiff = 116444736000000000 // 1601 → 1970 in 100-ns units
	return time.Unix(0, (nsec-epochDiff)*100), true
}

// fileHidden reads the Windows hidden attribute.
func fileHidden(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return st.FileAttributes&0x2 != 0 // FILE_ATTRIBUTE_HIDDEN
}

// platformTrash moves one path to the Windows Recycle Bin through PowerShell's
// Shell.Application verb, which needs no external tool.
func platformTrash(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	script := fmt.Sprintf(
		`(New-Object -ComObject Shell.Application).NameSpace(0).MoveHere('%s')`,
		strings.ReplaceAll(abs, "'", "''"))
	out, err := execCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return "", nil // the bin gives no new path back; undo restores by name
}

// mediaThumbJPEG builds a JPEG thumbnail of edge on the long side. Windows
// has no system image tool, so everything decodes through Go; videos go
// through ffmpeg when installed.
func mediaThumbJPEG(path string, video bool, edge int) ([]byte, error) {
	if video {
		return videoThumbJPEGFFmpeg(path, edge)
	}
	if edge < 64 {
		edge = 64
	}
	return goThumbnailJPEG(path, edge)
}

// videoThumbJPEGFFmpeg takes one scaled frame of a video via ffmpeg.
func videoThumbJPEGFFmpeg(path string, edge int) ([]byte, error) {
	if edge < 64 {
		edge = 64
	}
	scale := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease", edge, edge)
	cmd := execCommand("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", path,
		"-frames:v", "1", "-vf", scale, "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "5", "-")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("empty video thumbnail")
	}
	return out, nil
}

// platformTags: Windows has no file tags this app can read and write yet,
// so tags read as empty and writing keeps them empty.
func platformTags(path string) ([]string, error) {
	return nil, nil
}

func platformSetTags(path string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	return errors.New("此平台暂不支持文件标签")
}

// imageResampleJPEG decodes and downsamples in one pass through Go.
func imageResampleJPEG(path string, edge int, quality string) ([]byte, error) {
	return goThumbnailJPEG(path, edge)
}

// decodeLargeImage reads the original for 1:1 zoom.
func decodeLargeImage(path string, edge int) ([]byte, error) {
	_ = edge
	return os.ReadFile(path)
}

// finderLabelNames has no Windows counterpart; rating tags stay unnamed.
func finderLabelNames() []string {
	return []string{"红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "灰色"}
}

// sameDevice: Windows has no Stat_t.Dev, so same local-drive paths are
// treated as one device and UNC shares as another.
func sameDevice(a, b string) bool {
	volA, volB := filepath.VolumeName(a), filepath.VolumeName(b)
	if volA != "" || volB != "" {
		return volA == volB
	}
	// Both plain relative or both UNC (\servershare...).
	return (len(a) >= 2 && a[:2] == `\`) == (len(b) >= 2 && b[:2] == `\`)
}

// hideConsoleAttr hides the console window child processes would open.
var hideConsoleAttr *syscall.SysProcAttr

func init() {
	hideConsoleAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

var _ = unsafe.Sizeof(0) // keep unsafe imported for the attr plumbing
