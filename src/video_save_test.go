package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func stubSaveVideoDialog(t *testing.T, fn func(mygo.SaveDialogOptions) (string, error)) {
	t.Helper()
	original := saveVideoDialog
	saveVideoDialog = fn
	t.Cleanup(func() { saveVideoDialog = original })
}

func saveVideoHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestSaveVideoControlBarUsesDownloadsDefault(t *testing.T) {
	home := saveVideoHome(t)
	optsCh := make(chan mygo.SaveDialogOptions, 1)
	stubSaveVideoDialog(t, func(opts mygo.SaveDialogOptions) (string, error) {
		optsCh <- opts
		return "", nil
	})
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 20, playing: true}
	a := fakeVideoState(backend)
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 1000, 620)
	te.Frame()
	if !te.HasText("另存视频…") {
		t.Fatalf("control bar is missing the save action: %q", te.Texts())
	}
	if err := te.Click("另存视频…"); err != nil {
		t.Fatal(err)
	}
	select {
	case opts := <-optsCh:
		want := filepath.Join(home, "Downloads", "movie.mp4")
		if opts.DefaultPath != want || opts.Title != "另存视频…" {
			t.Fatalf("dialog options = %+v, want default %q", opts, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("save dialog was not requested")
	}
	if a.VideoSave.phase != saveChoosing {
		t.Fatalf("save phase = %v", a.VideoSave.phase)
	}
}

func TestSaveVideoContextMenuEntry(t *testing.T) {
	saveVideoHome(t)
	called := make(chan struct{}, 1)
	stubSaveVideoDialog(t, func(mygo.SaveDialogOptions) (string, error) {
		called <- struct{}{}
		return "", nil
	})
	a := &appState{Chosen: -1, LargeIndex: -1}
	item := &mediaItem{Path: "/tmp/movie.mp4", Name: "movie.mp4", IsVideo: true}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().ContextMenu(func(m *ui.Menu) { a.largeContextMenu(m, item) })
	}, 800, 600)
	te.Frame()
	te.RightClickAt(400, 300)
	te.Frame()
	found := false
	for _, label := range te.Menu() {
		if label == "另存视频…" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("context menu is missing the save action: %q", te.Menu())
	}
	if err := te.ChooseMenuItem("另存视频…"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("context menu did not open the save dialog")
	}
}

func TestSaveVideoProgressStateKeepsPlayback(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 20, playing: true}
	a := fakeVideoState(backend)
	before := a.videoSnapshot().Phase
	a.VideoSave = saveVideoState{phase: saveRunning, progress: 0.5, job: &saveVideoJob{cancel: func() {}}}
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 1000, 620)
	te.Frame()
	if !te.HasText("正在另存 50%") || !te.HasText("取消") {
		t.Fatalf("save progress is missing: %q", te.Texts())
	}
	if !te.HasText("00:20") || !te.HasText("02:00") {
		t.Fatalf("playback clock was disturbed: %q", te.Texts())
	}
	if after := a.videoSnapshot().Phase; after != before {
		t.Fatalf("playback phase changed from %v to %v", before, after)
	}
	a.VideoSave = saveVideoState{phase: saveFailed, message: "磁盘空间不足，无法另存视频。"}
	te.Frame()
	if !te.HasText("磁盘空间不足") {
		t.Fatalf("failure reason is not visible: %q", te.Texts())
	}
}

func TestSaveVideoStreamsCopyAndCancels(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "movie.mp4")
	content := bytes.Repeat([]byte("0123456789"), 200_000)
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(outDir, "movie.mp4")
	var last float32
	if err := streamVideoCopy(context.Background(), src, dest, func(p float32) { last = p }); err != nil {
		t.Fatal(err)
	}
	if last != 1 {
		t.Fatalf("final progress = %v", last)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("copy mismatch: len=%d err=%v", len(got), err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected output files: %v err=%v", entries, err)
	}

	cancelDest := filepath.Join(outDir, "cancel.mp4")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = streamVideoCopy(ctx, src, cancelDest, func(p float32) {
		if p > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	if _, err := os.Stat(cancelDest); !os.IsNotExist(err) {
		t.Fatalf("partial destination remained: %v", err)
	}
	entries, err = os.ReadDir(outDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial file was not deleted: %v err=%v", entries, err)
	}
}

func TestSaveVideoFailureMessagesAreChinese(t *testing.T) {
	cases := []struct {
		stage string
		err   error
		want  string
	}{
		{"target", fmt.Errorf("write: %w", syscall.ENOSPC), "磁盘空间不足"},
		{"target", os.ErrPermission, "目标目录不可写"},
		{"source", os.ErrPermission, "源文件不可读"},
		{"source", os.ErrNotExist, "源文件不可读"},
	}
	for _, tc := range cases {
		got := videoSaveErrorMessage(stageVideoSaveError(tc.stage, tc.err))
		if !strings.Contains(got, tc.want) {
			t.Fatalf("stage %s error %q, want %q", tc.stage, got, tc.want)
		}
	}
}

func TestSaveVideoCancelTransitionsState(t *testing.T) {
	canceled := false
	a := &appState{VideoSave: saveVideoState{
		phase:    saveRunning,
		progress: 0.4,
		job:      &saveVideoJob{cancel: func() { canceled = true }},
	}}
	a.cancelVideoSave()
	if !canceled || a.VideoSave.phase != saveCanceling || a.VideoSave.statusLabel() != "正在取消另存…" {
		t.Fatalf("cancel state = %+v canceled=%v", a.VideoSave, canceled)
	}
}

func TestSaveVideoBackgroundCopyUpdatesOneState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(src, bytes.Repeat([]byte("video"), 20_000), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "copy.mp4")
	done := make(chan saveVideoState, 1)
	var a *appState
	hook := func(fn func()) {
		fn()
		if a.VideoSave.phase == saveDone || a.VideoSave.phase == saveFailed || a.VideoSave.phase == saveCanceled {
			select {
			case done <- a.VideoSave:
			default:
			}
		}
	}
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })
	a = &appState{}
	a.beginVideoSave(src, dest)
	select {
	case state := <-done:
		if state.phase != saveDone || state.progress != 1 || !strings.Contains(state.message, "视频已另存") {
			t.Fatalf("final save state = %+v", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background copy did not finish")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("saved copy missing: %v", err)
	}
}

func TestSaveVideoBackgroundFailureShowsChineseReason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	done := make(chan saveVideoState, 1)
	var a *appState
	hook := func(fn func()) {
		fn()
		if a.VideoSave.phase == saveDone || a.VideoSave.phase == saveFailed || a.VideoSave.phase == saveCanceled {
			select {
			case done <- a.VideoSave:
			default:
			}
		}
	}
	prev := headlessUpdate.Load()
	headlessUpdate.Store(&hook)
	t.Cleanup(func() { headlessUpdate.Store(prev) })
	a = &appState{}
	a.beginVideoSave(filepath.Join(dir, "missing.mp4"), filepath.Join(dir, "copy.mp4"))
	select {
	case state := <-done:
		if state.phase != saveFailed || !strings.Contains(state.message, "源文件不可读") {
			t.Fatalf("final save state = %+v", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background failure did not reach the UI state")
	}
}
