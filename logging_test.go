package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLogWriteAndRotate checks a log line lands on disk and that the file
// rolls over once it grows past the cap.
func TestLogWriteAndRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lumiwerk.log")

	logMu.Lock()
	prevFile, prevPath, prevStderr, prevLevel := logFile, logPath, logToStderr, logMinLevel
	logMu.Unlock()
	defer func() {
		logMu.Lock()
		logFile, logPath, logToStderr, logMinLevel = prevFile, prevPath, prevStderr, prevLevel
		logMu.Unlock()
	}()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	logMu.Lock()
	logFile, logPath, logToStderr, logMinLevel = f, path, false, levelDebug
	logMu.Unlock()

	logInfof("hello %s", "lumiwerk")
	logMu.Lock()
	if logFile != nil {
		_ = logFile.Sync()
	}
	logMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "INFO") || !strings.Contains(string(data), "hello lumiwerk") {
		t.Fatalf("log file missing entry: %q", data)
	}

	// Fill the current file past the cap, then log again to force rotation.
	logMu.Lock()
	big := strings.Repeat("x", int(logMaxBytes)+10)
	_, _ = logFile.WriteString(big)
	logMu.Unlock()
	logInfof("after rotation")

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	rotated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rotated), "after rotation") {
		t.Fatalf("new log file missing the post-rotation entry: %q", rotated)
	}
}

// TestLogRecoverSwallowsPanic makes sure a panic in a background job is
// recorded instead of unwinding the process.
func TestLogRecoverSwallowsPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lumiwerk.log")

	logMu.Lock()
	prevFile, prevPath, prevStderr, prevLevel := logFile, logPath, logToStderr, logMinLevel
	logMu.Unlock()
	defer func() {
		logMu.Lock()
		logFile, logPath, logToStderr, logMinLevel = prevFile, prevPath, prevStderr, prevLevel
		logMu.Unlock()
	}()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	logMu.Lock()
	logFile, logPath, logToStderr, logMinLevel = f, path, false, levelDebug
	logMu.Unlock()

	func() {
		defer logRecover("test-job")
		panic("boom")
	}()

	logMu.Lock()
	if logFile != nil {
		_ = logFile.Sync()
	}
	logMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "panic in test-job: boom") {
		t.Fatalf("panic was not logged: %q", data)
	}
}

// TestSafeGoLogsPanic runs many panicking jobs and checks the process survives
// and each panic is recorded.
func TestSafeGoLogsPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lumiwerk.log")

	logMu.Lock()
	prevFile, prevPath, prevStderr, prevLevel := logFile, logPath, logToStderr, logMinLevel
	logMu.Unlock()
	defer func() {
		logMu.Lock()
		logFile, logPath, logToStderr, logMinLevel = prevFile, prevPath, prevStderr, prevLevel
		logMu.Unlock()
	}()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	logMu.Lock()
	logFile, logPath, logToStderr, logMinLevel = f, path, false, levelDebug
	logMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		safeGo("stress", func() {
			defer wg.Done()
			panic("stress panic")
		})
	}
	wg.Wait()

	// safeGo logs the panic after fn returns, so wg.Wait() only proves the jobs
	// unwound. Poll until every panic has been written (or give up).
	deadline := time.Now().Add(2 * time.Second)
	got := 0
	for {
		logMu.Lock()
		if logFile != nil {
			_ = logFile.Sync()
		}
		logMu.Unlock()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got = strings.Count(string(data), "stress panic")
		if got >= 8 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got != 8 {
		t.Fatalf("logged %d stress panics, want 8", got)
	}
}
