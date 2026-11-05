package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// videoThumb is a video's cover for the thumbnail cache. Release builds
// bundle libmpv but no ffmpeg binary, and an app started from Finder or
// the Start menu does not see a shell PATH (Homebrew's /opt/homebrew/bin),
// so a bare "ffmpeg" failed on most user machines and every video showed
// the plain icon. Try, in order: ffmpeg wherever it is installed, the
// platform's own thumbnailer (QuickLook on macOS), the bundled libmpv.
func videoThumb(path string, edge int) ([]byte, error) {
	if edge < 64 {
		edge = 64
	}
	var errs []error
	if bin := ffmpegBin(); bin != "" {
		data, err := ffmpegVideoThumb(bin, path, edge)
		if err == nil {
			return data, nil
		}
		errs = append(errs, fmt.Errorf("ffmpeg %s: %w", bin, err))
	} else {
		errs = append(errs, errors.New("ffmpeg: not found"))
	}
	if data, err := platformVideoThumb(path, edge); err == nil {
		return data, nil
	} else {
		errs = append(errs, fmt.Errorf("platform: %w", err))
	}
	if data, err := mpvVideoThumb(path, edge); err == nil {
		return data, nil
	} else {
		errs = append(errs, fmt.Errorf("libmpv: %w", err))
	}
	err := errors.Join(errs...)
	logWarnf("video thumbnail failed %s: %v", path, err)
	return nil, err
}

var (
	ffmpegOnce sync.Once
	ffmpegPath string
)

// ffmpegBin finds ffmpeg on PATH, beside the executable, or in the usual
// install folders a GUI app's PATH leaves out; "" when there is none.
func ffmpegBin() string {
	ffmpegOnce.Do(func() {
		name := "ffmpeg"
		if runtime.GOOS == "windows" {
			name = "ffmpeg.exe"
		}
		if p, err := exec.LookPath(name); err == nil {
			ffmpegPath = p
			return
		}
		var dirs []string
		if exe, err := os.Executable(); err == nil {
			dirs = append(dirs, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "..", "Resources"))
		}
		switch runtime.GOOS {
		case "darwin":
			dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin")
		case "windows":
			if d := os.Getenv("LOCALAPPDATA"); d != "" {
				dirs = append(dirs, filepath.Join(d, "Microsoft", "WinGet", "Links"))
			}
			if d := os.Getenv("USERPROFILE"); d != "" {
				dirs = append(dirs, filepath.Join(d, "scoop", "shims"))
			}
			dirs = append(dirs, `C:\ProgramData\chocolatey\bin`, `C:\ffmpeg\bin`)
		default:
			dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/snap/bin")
		}
		for _, d := range dirs {
			p := filepath.Join(d, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				ffmpegPath = p
				return
			}
		}
		logInfof("ffmpeg not found; video covers use the platform thumbnailer or libmpv")
	})
	return ffmpegPath
}

func ffmpegVideoThumb(bin, path string, edge int) ([]byte, error) {
	scale := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease", edge, edge)
	cmd := execCommand(bin, "-hide_banner", "-loglevel", "error", "-i", path,
		"-frames:v", "1", "-vf", scale, "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "5", "-")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, ee.Stderr)
		}
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("empty frame")
	}
	return out, nil
}
