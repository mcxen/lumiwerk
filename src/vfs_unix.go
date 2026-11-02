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

// platformTrash tries gio, then falls back to a manual XDG trash that
// restores nothing.
func platformTrash(path string) (string, error) {
	if out, err := exec.Command("gio", "trash", path).CombinedOutput(); err == nil {
		return "", nil
	} else if _, lookErr := exec.LookPath("gio"); lookErr == nil {
		return "", fmt.Errorf("%v: %s", err, out)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errTrashUnsupported
	}
	if err := os.Rename(path, home+"/.local/share/Trash/files/"+baseName(path)); err != nil {
		return "", err
	}
	return "", nil
}

func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// mediaThumbJPEG decodes through Go; videos go through ffmpeg when installed.
func mediaThumbJPEG(path string, video bool, edge int) ([]byte, error) {
	if video {
		return videoThumbJPEGFFmpeg(path, edge)
	}
	if edge < 64 {
		edge = 64
	}
	return goThumbnailJPEG(path, edge)
}

func videoThumbJPEGFFmpeg(path string, edge int) ([]byte, error) {
	if edge < 64 {
		edge = 64
	}
	scale := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease", edge, edge)
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", path,
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

// platformTags: Linux has no file tags this app can read and write yet.
func platformTags(path string) ([]string, error) {
	return nil, nil
}

func platformSetTags(path string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	return errors.New("此平台暂不支持文件标签")
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
