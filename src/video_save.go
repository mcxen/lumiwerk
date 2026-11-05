package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"
)

type saveVideoPhase int

const (
	saveIdle saveVideoPhase = iota
	saveChoosing
	saveRunning
	saveCanceling
	saveDone
	saveFailed
	saveCanceled
)

// saveVideoState 是「另存视频」的单一状态机，沿用 playbackPhase 的做法：
// phase 是唯一的状态来源，progress 是唯一的下载进度字段。它不读写
// playbackSession / playbackSnapshot，另存进度与播放、暂停、缓冲、跳转互不干扰。
type saveVideoState struct {
	phase       saveVideoPhase
	progress    float32
	message     string
	destination string
	job         *saveVideoJob
}

type saveVideoJob struct {
	cancel context.CancelFunc
}

func (s saveVideoState) active() bool {
	switch s.phase {
	case saveChoosing, saveRunning, saveCanceling:
		return true
	default:
		return false
	}
}

func (s saveVideoState) statusLabel() string {
	switch s.phase {
	case saveChoosing:
		return s.message
	case saveRunning:
		return fmt.Sprintf("正在另存 %d%%", int(s.progress*100+0.5))
	case saveCanceling, saveDone, saveFailed, saveCanceled:
		return s.message
	default:
		return ""
	}
}

var saveVideoDialog = func(opts mygo.SaveDialogOptions) (string, error) {
	return mygo.Dialog.Save(opts)
}

func (a *appState) chooseVideoSave(item *mediaItem) {
	if a.VideoSave.active() {
		return
	}
	if item == nil || !item.IsVideo || item.Path == "" {
		message := "请先打开一个视频。"
		a.VideoSave = saveVideoState{phase: saveFailed, message: message}
		a.toast(message)
		return
	}
	src := item.Path
	defaultPath := filepath.Base(src)
	message := "选择另存位置…"
	dialogMessage := "默认保存到下载目录，也可以选择其它目录。"
	if dir, err := downloadsDir(); err == nil {
		defaultPath = flowUnique(filepath.Join(dir, defaultPath), false, nil)
	} else {
		defaultPath = flowUnique(filepath.Join(filepath.Dir(src), defaultPath), false, nil)
		message = "下载目录不可用，请选择其它目录"
		dialogMessage = message
	}
	opts := mygo.SaveDialogOptions{
		Parent:            a.Window,
		Title:             "另存视频…",
		DefaultPath:       defaultPath,
		ButtonLabel:       "存储",
		Message:           dialogMessage,
		NameFieldLabel:    "文件名",
		CreateDirectories: true,
	}
	a.VideoSave = saveVideoState{phase: saveChoosing, destination: defaultPath, message: message}
	safeGo("video.save.pick", func() {
		dest, err := saveVideoDialog(opts)
		a.update(func() {
			if err != nil {
				message := videoSaveErrorMessage(stageVideoSaveError("dialog", err))
				a.VideoSave = saveVideoState{phase: saveFailed, message: message}
				a.toast(message)
				return
			}
			if dest == "" {
				a.VideoSave = saveVideoState{phase: saveIdle}
				return
			}
			a.beginVideoSave(src, dest)
		})
	})
}

func (a *appState) beginVideoSave(src, dest string) {
	dest = flowUnique(dest, false, nil)
	ctx, cancel := context.WithCancel(context.Background())
	job := &saveVideoJob{cancel: cancel}
	a.VideoSave = saveVideoState{phase: saveRunning, destination: dest, job: job}
	safeGo("video.save.copy", func() {
		err := streamVideoCopy(ctx, src, dest, func(progress float32) {
			a.update(func() {
				if a.VideoSave.job != job || (a.VideoSave.phase != saveRunning && a.VideoSave.phase != saveCanceling) {
					return
				}
				a.VideoSave.progress = progress
			})
		})
		a.update(func() {
			if a.VideoSave.job != job {
				return
			}
			switch {
			case errors.Is(err, context.Canceled):
				message := "已取消另存，半成品文件已删除。"
				a.VideoSave = saveVideoState{phase: saveCanceled, message: message, destination: dest}
				a.toast(message)
			case err != nil:
				message := videoSaveErrorMessage(err)
				a.VideoSave = saveVideoState{phase: saveFailed, message: message, destination: dest}
				a.toast(message)
				logErrorf("video.save: src=%s dest=%s err=%v", src, dest, err)
			default:
				message := "视频已另存：" + filepath.Base(dest)
				a.VideoSave = saveVideoState{phase: saveDone, progress: 1, message: message, destination: dest}
				a.toast(message)
			}
		})
	})
}

func (a *appState) cancelVideoSave() {
	if a.VideoSave.phase != saveRunning || a.VideoSave.job == nil {
		return
	}
	a.VideoSave.job.cancel()
	a.VideoSave.phase = saveCanceling
	a.VideoSave.message = "正在取消另存…"
}

type videoSaveStageError struct {
	stage string
	err   error
}

func (e *videoSaveStageError) Error() string { return e.err.Error() }
func (e *videoSaveStageError) Unwrap() error { return e.err }

func stageVideoSaveError(stage string, err error) error {
	return &videoSaveStageError{stage: stage, err: err}
}

func videoSaveErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	stage := "target"
	var stageErr *videoSaveStageError
	if errors.As(err, &stageErr) {
		stage = stageErr.stage
		err = stageErr.err
	}
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return "已取消另存，半成品文件已删除。"
	case isVideoSaveNoSpace(err, lower):
		return "磁盘空间不足，无法另存视频。"
	case stage == "dialog":
		return "无法选择另存位置：" + err.Error()
	case isVideoSavePermission(err, lower):
		if stage == "source" {
			return "源文件不可读，无法另存视频。"
		}
		return "目标目录不可写，无法另存视频。"
	case stage == "source" && (errors.Is(err, os.ErrNotExist) || strings.Contains(lower, "is a directory")):
		return "源文件不可读，无法另存视频。"
	case stage == "target" && errors.Is(err, os.ErrNotExist):
		return "目标目录不存在或不可写，无法另存视频。"
	case stage == "source":
		return "源文件不可读：" + err.Error()
	default:
		return "目标不可写或另存失败：" + err.Error()
	}
}

func isVideoSaveNoSpace(err error, lower string) bool {
	return errors.Is(err, syscall.ENOSPC) || strings.Contains(lower, "no space left") || strings.Contains(lower, "disk quota exceeded")
}

func isVideoSavePermission(err error, lower string) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EROFS) ||
		strings.Contains(lower, "permission denied") || strings.Contains(lower, "read-only file system")
}

func streamVideoCopy(ctx context.Context, src, dest string, report func(float32)) error {
	info, err := os.Stat(src)
	if err != nil {
		return stageVideoSaveError("source", err)
	}
	if info.IsDir() {
		return stageVideoSaveError("source", errors.New("源路径是文件夹"))
	}
	in, err := os.Open(src)
	if err != nil {
		return stageVideoSaveError("source", err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".lumiwerk-video-*.part")
	if err != nil {
		return stageVideoSaveError("target", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	defer tmp.Close()

	var written int64
	var lastReport time.Time
	reportProgress := func(n int64) {
		if report == nil {
			return
		}
		written += n
		progress := unitFraction(float64(written), float64(info.Size()))
		if progress < 1 && time.Since(lastReport) < 100*time.Millisecond {
			return
		}
		lastReport = time.Now()
		report(progress)
	}
	reader := &saveCancelReader{ctx: ctx, r: in}
	writer := &saveProgressWriter{ctx: ctx, w: tmp, onWrite: reportProgress}
	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return stageVideoSaveError("target", err)
	}
	if err := tmp.Sync(); err != nil {
		return stageVideoSaveError("target", err)
	}
	if err := tmp.Close(); err != nil {
		return stageVideoSaveError("target", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return stageVideoSaveError("target", err)
	}
	committed = true
	if report != nil {
		report(1)
	}
	return nil
}

type saveCancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *saveCancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, stageVideoSaveError("source", err)
	}
	return n, err
}

type saveProgressWriter struct {
	ctx     context.Context
	w       io.Writer
	onWrite func(int64)
}

func (w *saveProgressWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.w.Write(p)
	if n > 0 && w.onWrite != nil {
		w.onWrite(int64(n))
	}
	if err != nil {
		return n, stageVideoSaveError("target", err)
	}
	if n != len(p) {
		return n, stageVideoSaveError("target", io.ErrShortWrite)
	}
	return n, nil
}
