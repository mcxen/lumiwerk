package main

import (
	"strings"

	"errors"
	"fmt"
	"github.com/dhowden/tag"
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
// the plain icon. Try, in order: cover art embedded in the container
// (pure Go, a few ms), ffmpeg wherever it is installed, the
// platform's own thumbnailer (QuickLook on macOS), the bundled libmpv.
func videoThumb(path string, edge int) ([]byte, error) {
	if edge < 64 {
		edge = 64
	}
	var errs []error
	if data, err := embeddedCover(path); err == nil {
		return data, nil
	} else if !errors.Is(err, errNoEmbeddedCover) {
		errs = append(errs, fmt.Errorf("embedded: %w", err))
	}
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
	toolMu    sync.Mutex
	toolPaths = map[string]string{}
)

// ffmpegBin is the ffmpeg toolBin finds, or "".
func ffmpegBin() string { return toolBin("ffmpeg") }

// toolBin finds an FFmpeg tool (ffmpeg, ffprobe) on PATH, beside the
// executable, or in the usual install folders a GUI app's PATH leaves out;
// "" when there is none. The answer is kept for the run.
func toolBin(tool string) string {
	toolMu.Lock()
	defer toolMu.Unlock()
	if p, ok := toolPaths[tool]; ok {
		return p
	}
	p := findTool(tool)
	toolPaths[tool] = p
	if p == "" {
		logInfof("%s not found on PATH or in the usual install folders", tool)
	}
	return p
}

// toolPath is toolBin, or the bare name so exec reports it missing.
func toolPath(tool string) string {
	if p := toolBin(tool); p != "" {
		return p
	}
	return tool
}

// lookTool is exec.LookPath with toolBin's wider search.
func lookTool(tool string) (string, error) {
	if p := toolBin(tool); p != "" {
		return p, nil
	}
	return "", &exec.Error{Name: tool, Err: exec.ErrNotFound}
}

func findTool(tool string) string {
	name := tool
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
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
			return p
		}
	}
	return ""
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

var errNoEmbeddedCover = errors.New("no embedded cover")

// embeddedCover reads the cover art an MP4/MOV/M4V carries in its metadata
// (iTunes covr atom) with dhowden/tag: no decoding, only the moov atom.
func embeddedCover(path string) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov", ".3gp":
	default:
		return nil, errNoEmbeddedCover
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, errNoEmbeddedCover // no metadata atoms
	}
	if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
		return pic.Data, nil
	}
	return nil, errNoEmbeddedCover
}
