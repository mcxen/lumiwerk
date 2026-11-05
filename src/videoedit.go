package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func evenCrop(n int) int {
	if n < 2 {
		return 0
	}
	return n - n%2
}

// centerCropFilter is FlowVision's centered pixel crop, forced to even sizes.
func centerCropFilter(width, height int) (string, error) {
	w, h := evenCrop(width), evenCrop(height)
	if w < 2 || h < 2 {
		return "", errors.New("视频裁剪尺寸至少需要 2 像素。")
	}
	return fmt.Sprintf("crop=%d:%d:(iw-%d)/2:(ih-%d)/2,setsar=1", w, h, w, h), nil
}

func trimOutputName(src string, start, end float64, index int) string {
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	if index < 0 {
		index = 0
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	return fmt.Sprintf("%s_clip_%02d_%09d-%09d%s", base, index+1, int64(start*1000), int64(end*1000), filepath.Ext(src))
}

func replaceWithTemp(original, temp string) error {
	backup := original + ".lumiwerk-bak"
	_ = os.Remove(backup)
	if err := os.Rename(original, backup); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, original); err != nil {
		_ = os.Rename(backup, original)
		_ = os.Remove(temp)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func runFFmpeg(args []string) error {
	cmd := exec.Command(toolPath("ffmpeg"), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// rectCropFilter is an even top-left crop. The view is top-left, so this
// does not flip Y the way FlowVision does for AppKit's bottom origin.
func rectCropFilter(frameW, frameH, x, y, width, height int) (string, error) {
	if frameW < 2 || frameH < 2 {
		return "", errors.New("无法读取视频尺寸。")
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= frameW || y >= frameH {
		return "", errors.New("裁剪起点超出画面。")
	}
	if x+width > frameW {
		width = frameW - x
	}
	if y+height > frameH {
		height = frameH - y
	}
	x -= x % 2
	y -= y % 2
	width, height = evenCrop(width), evenCrop(height)
	if x+width > frameW {
		width = evenCrop(frameW - x)
	}
	if y+height > frameH {
		height = evenCrop(frameH - y)
	}
	if width < 2 || height < 2 {
		return "", errors.New("视频裁剪尺寸至少需要 2 像素。")
	}
	return fmt.Sprintf("crop=%d:%d:%d:%d,setsar=1", width, height, x, y), nil
}

// cropVideoFile center-crops and replaces the original, keeping the audio.
func cropVideoFile(path string, width, height int) error {
	filter, err := centerCropFilter(width, height)
	if err != nil {
		return err
	}
	return cropWithFilter(path, filter)
}

// cropVideoRect replaces the original with an arbitrary even crop.
func cropVideoRect(path string, x, y, width, height int) error {
	info := probeVideo(path)
	filter, err := rectCropFilter(info.Width, info.Height, x, y, width, height)
	if err != nil {
		return err
	}
	return cropWithFilter(path, filter)
}

func cropWithFilter(path, filter string) error {
	if _, err := lookTool("ffmpeg"); err != nil {
		return errors.New("未找到 ffmpeg。")
	}
	ext := filepath.Ext(path)
	var last error
	for _, codec := range []string{"h264_videotoolbox", "libx264", "mpeg4"} {
		temp := filepath.Join(filepath.Dir(path), fmt.Sprintf(".lumiwerk-crop-%d%s", os.Getpid(), ext))
		_ = os.Remove(temp)
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", path,
			"-map", "0:v:0", "-map", "0:a?", "-map_metadata", "0",
			"-vf", filter, "-c:v", codec, "-c:a", "copy", temp}
		if codec == "libx264" {
			args = []string{"-hide_banner", "-loglevel", "error", "-y", "-i", path,
				"-map", "0:v:0", "-map", "0:a?", "-map_metadata", "0",
				"-vf", filter, "-c:v", codec, "-pix_fmt", "yuv420p", "-c:a", "copy", temp}
		}
		last = runFFmpeg(args)
		if last != nil {
			_ = os.Remove(temp)
			continue
		}
		if err := replaceWithTemp(path, temp); err != nil {
			return err
		}
		return nil
	}
	if last == nil {
		last = errors.New("视频裁剪失败。")
	}
	return last
}

// trimVideoLossless copies one time range without re-encoding.
func trimVideoLossless(src, dest string, start, end float64) error {
	if end <= start {
		return errors.New("裁剪结束时间必须晚于开始时间。")
	}
	if _, err := lookTool("ffmpeg"); err != nil {
		return errors.New("未找到 ffmpeg。")
	}
	tmp := dest + ".partial" + filepath.Ext(dest)
	_ = os.Remove(tmp)
	args := []string{"-hide_banner", "-loglevel", "error", "-y",
		"-ss", fmt.Sprintf("%.3f", start), "-i", src,
		"-t", fmt.Sprintf("%.3f", end-start),
		"-map", "0:v:0", "-map", "0:a?", "-map_metadata", "0",
		"-c", "copy", "-copytb", "1", "-avoid_negative_ts", "make_zero", tmp}
	if err := runFFmpeg(args); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// exportVideoEdit writes one clip. An empty filter copies the range; a crop
// filter re-encodes that range into a new file and leaves the source alone.
func exportVideoEdit(src, dest, filter string, start, end float64) error {
	if filter == "" {
		return trimVideoLossless(src, dest, start, end)
	}
	if end <= start {
		return errors.New("裁剪结束时间必须晚于开始时间。")
	}
	if _, err := lookTool("ffmpeg"); err != nil {
		return errors.New("未找到 ffmpeg。")
	}
	tmp := dest + ".partial" + filepath.Ext(dest)
	_ = os.Remove(tmp)
	var last error
	for _, codec := range []string{"h264_videotoolbox", "libx264", "mpeg4"} {
		args := []string{"-hide_banner", "-loglevel", "error", "-y",
			"-ss", fmt.Sprintf("%.3f", start), "-i", src,
			"-t", fmt.Sprintf("%.3f", end-start),
			"-map", "0:v:0", "-map", "0:a?", "-map_metadata", "0",
			"-vf", filter, "-c:v", codec, "-c:a", "copy", tmp}
		if codec == "libx264" {
			args = []string{"-hide_banner", "-loglevel", "error", "-y",
				"-ss", fmt.Sprintf("%.3f", start), "-i", src,
				"-t", fmt.Sprintf("%.3f", end-start),
				"-map", "0:v:0", "-map", "0:a?", "-map_metadata", "0",
				"-vf", filter, "-c:v", codec, "-pix_fmt", "yuv420p", "-c:a", "copy", tmp}
		}
		last = runFFmpeg(args)
		if last != nil {
			_ = os.Remove(tmp)
			continue
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return nil
	}
	if last == nil {
		last = errors.New("视频导出失败。")
	}
	return last
}

func allVideos(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !isVideoFile(strings.TrimPrefix(filepath.Ext(path), ".")) {
			return false
		}
	}
	return true
}

func parseCropSize(widthText, heightText string) (int, int, error) {
	width, errW := strconv.Atoi(strings.TrimSpace(widthText))
	height, errH := strconv.Atoi(strings.TrimSpace(heightText))
	if errW != nil || errH != nil {
		return 0, 0, errors.New("请输入有效的视频裁剪尺寸。")
	}
	if _, err := centerCropFilter(width, height); err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func parseTrimRange(startText, endText string) (float64, float64, error) {
	start, errS := strconv.ParseFloat(strings.TrimSpace(startText), 64)
	end, errE := strconv.ParseFloat(strings.TrimSpace(endText), 64)
	if errS != nil || errE != nil || start < 0 || end <= start {
		return 0, 0, errors.New("请输入有效的起止时间。")
	}
	return start, end, nil
}

// videoRotateFilter matches FlowVision: 90 and 180 go through transpose=1,
// 270 through transpose=2. The pixels are baked in so thumbnails follow.
func videoRotateFilter(degrees int) (string, error) {
	switch degrees {
	case 90:
		return "transpose=1", nil
	case 180:
		return "transpose=1,transpose=1", nil
	case 270:
		return "transpose=2", nil
	default:
		return "", errors.New("不支持的旋转角度")
	}
}

func rotateVideoFile(path string, degrees int) error {
	filter, err := videoRotateFilter(degrees)
	if err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(path), ".lumiwerk-rot-"+filepath.Base(path))
	_ = os.Remove(temp)
	err = runFFmpeg([]string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", path,
		"-map", "0",
		"-filter:v:0", filter,
		"-c:a", "copy",
		"-c:s", "copy",
		"-metadata:s:v:0", "rotate=0",
		temp,
	})
	if err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := replaceWithTemp(path, temp); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}
