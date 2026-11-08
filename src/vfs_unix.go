//go:build unix && !darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// fileCreated: most Linux filesystems keep no birth time in stat.
func fileCreated(info os.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}

// fileHidden is always false where dotfiles cover hiding.
func fileHidden(info os.FileInfo) bool {
	return false
}

// platformTrash follows the XDG trash spec: it writes the original path to
// a .trashinfo record so file managers can restore it. gio is preferred when
// present. The returned path lets undo move the file back.
func platformTrash(path string) (string, error) {
	if _, lookErr := exec.LookPath("gio"); lookErr == nil {
		if out, err := exec.Command("gio", "trash", path).CombinedOutput(); err != nil {
			return "", fmt.Errorf("%v: %s", err, out)
		}
		return "", nil
	}
	return xdgTrash(path, xdgDataHome())
}

// mediaThumbJPEG decodes through Go; videos go through ffmpeg when installed.
func mediaThumbJPEG(path string, video bool, edge int) ([]byte, error) {
	if video {
		return videoThumb(path, edge)
	}
	if edge < 64 {
		edge = 64
	}
	return goThumbnailJPEG(path, edge)
}

// platformVideoThumb: no built-in video thumbnailer is used here yet.
func platformVideoThumb(path string, edge int) ([]byte, error) {
	return nil, errors.New("no platform thumbnailer")
}

// platformTags reads tags from the app's own tag store; this platform has
// no file-attached tag metadata the app can write.
func platformTags(path string) ([]string, error) {
	return fileTagStore.get(path), nil
}

func platformSetTags(path string, tags []string) error {
	return fileTagStore.set(path, tags)
}

func imageResampleJPEG(path string, edge int, quality string) ([]byte, error) {
	return goThumbnailJPEG(path, edge)
}

func decodeLargeImage(path string, edge int) ([]byte, error) {
	_ = edge
	return os.ReadFile(path)
}

func finderLabelNames() []string {
	return []string{"红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "灰色"}
}

// sameDevice compares stat device IDs, so a move stays a rename.
func sameDevice(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	type statDev interface{ Dev() uint64 }
	_ = statDev(nil)
	statA, okA := infoA.Sys().(*syscall.Stat_t)
	statB, okB := infoB.Sys().(*syscall.Stat_t)
	if !okA || !okB {
		return false
	}
	return statA.Dev == statB.Dev
}
