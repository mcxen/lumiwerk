//go:build darwin

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stubQuickLook points quickLookBin at a shell script for the test.
func stubQuickLook(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "qlmanage")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := quickLookBin
	quickLookBin = script
	t.Cleanup(func() { quickLookBin = original })
	return dir
}

// waitGone polls until pid no longer exists.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("process %d outlived the thumbnail job", pid)
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stub never wrote %s", path)
	return 0
}

// A video path that does not exist (the fixture paths view tests use)
// must not start QuickLook at all. Before the fix every such render left
// a `qlmanage -t ... /tmp/v.mp4` process hanging forever.
func TestVideoThumbMissingPathStartsNoQuickLook(t *testing.T) {
	dir := stubQuickLook(t, `touch "$(dirname "$0")/ran"`)
	missing := filepath.Join(t.TempDir(), "gone.mp4")
	if _, err := videoThumb(missing, 256); err == nil {
		t.Fatal("missing video produced a thumbnail")
	}
	if _, err := platformVideoThumb(missing, 256); err == nil {
		t.Fatal("platformVideoThumb accepted a missing path")
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Fatal("qlmanage was started for a missing path")
	}
}

// A QuickLook run that never returns is killed at quickLookTimeout, and
// the helper it forked dies with it. Before the fix this test hung and
// left both processes behind.
func TestPlatformVideoThumbKillsStalledQuickLook(t *testing.T) {
	dir := stubQuickLook(t, `echo $$ > "$(dirname "$0")/leader"
sleep 600 &
echo $! > "$(dirname "$0")/child"
wait`)
	original := quickLookTimeout
	quickLookTimeout = 2 * time.Second // room for the stub to start under a loaded test run
	t.Cleanup(func() { quickLookTimeout = original })

	video := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(video, []byte("not really a movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := platformVideoThumb(video, 256)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stalled qlmanage error = %v, want deadline exceeded", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("platformVideoThumb did not time out a stalled qlmanage")
	}
	waitGone(t, readPid(t, filepath.Join(dir, "leader")))
	waitGone(t, readPid(t, filepath.Join(dir, "child")))
}

// Cancelling the job's context kills the tool's whole process group.
func TestRunThumbToolCancelKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runThumbTool(ctx, time.Minute, "/bin/sh", "-c", `sleep 600 & echo $! > "$1"; wait`, "sh", childFile)
		done <- err
	}()
	child := readPid(t, childFile)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled run error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runThumbTool ignored cancellation")
	}
	waitGone(t, child)
}
