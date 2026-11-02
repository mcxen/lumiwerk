package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Lumiwerk writes a rotating text log to ~/Library/Logs/Lumiwerk/lumiwerk.log.
// It records startup, file operations, decode failures and recoverable panics
// so a crash in the wild leaves evidence instead of vanishing with the window.
// LUMIWERK_LOG=debug|info|warn|error|off raises or silences the level; a log
// also goes to stderr when the app runs from a terminal.

type logLevel int

const (
	levelDebug logLevel = iota
	levelInfo
	levelWarn
	levelError
	levelOff
)

const (
	logMaxBytes int64 = 2 << 20 // rotate at 2 MiB
	logKeep           = 3       // lumiwerk.log.1 .. .3
)

var (
	logMu       sync.Mutex
	logFile     *os.File
	logPath     string
	logMinLevel = levelInfo
	logToStderr bool
)

func levelName(l logLevel) string {
	switch l {
	case levelDebug:
		return "DEBUG"
	case levelWarn:
		return "WARN"
	case levelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// initLogging opens the log file and rotates it when it has grown too large.
// It is safe to call more than once; only the first call opens the file.
func initLogging() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LUMIWERK_LOG"))) {
	case "debug":
		logMinLevel = levelDebug
	case "warn":
		logMinLevel = levelWarn
	case "error":
		logMinLevel = levelError
	case "off":
		logMinLevel = levelOff
	case "info", "":
		logMinLevel = levelInfo
	}
	if fi, err := os.Stderr.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		logToStderr = true
	}
	dir := logDirectory()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	logPath = filepath.Join(dir, "lumiwerk.log")
	rotateLog(logPath)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	logFile = f
}

func logDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Logs", appName)
}

// LogPath is the active log file, empty when logging is unavailable.
func LogPath() string {
	logMu.Lock()
	defer logMu.Unlock()
	return logPath
}

// rotateLog shifts lumiwerk.log to .1 (and .1 to .2, .2 to .3) when the
// current file has grown past logMaxBytes.
func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < logMaxBytes {
		return
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, logKeep))
	for i := logKeep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
	}
	_ = os.Rename(path, path+".1")
}

func logWrite(level logLevel, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	msg = strings.TrimRight(msg, "\n")
	caller := "?"
	if _, file, line, ok := runtime.Caller(2); ok {
		caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}
	recordDiagnostic(level, caller, msg)
	if level < logMinLevel {
		return
	}
	line := fmt.Sprintf("%s %-5s [%s] %s\n",
		time.Now().Format("2006-01-02 15:04:05.000"), levelName(level), caller, msg)

	logMu.Lock()
	if logFile != nil {
		if info, err := logFile.Stat(); err == nil && info.Size() >= logMaxBytes {
			name := logFile.Name()
			_ = logFile.Close()
			logFile = nil
			rotateLog(name)
			if f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				logFile = f
			}
		}
		if logFile != nil {
			_, _ = logFile.WriteString(line)
		}
	}
	toStderr := logToStderr
	logMu.Unlock()
	if toStderr {
		_, _ = os.Stderr.WriteString(line)
	}
}

func logDebugf(format string, args ...any) { logWrite(levelDebug, format, args...) }
func logInfof(format string, args ...any)  { logWrite(levelInfo, format, args...) }
func logWarnf(format string, args ...any)  { logWrite(levelWarn, format, args...) }
func logErrorf(format string, args ...any) { logWrite(levelError, format, args...) }

// logRecover records a panic with its stack. Use it in a deferred call:
//
//	defer logRecover("render")
//
// It swallows the panic, so one bad frame or background job logs the cause
// and the app keeps running instead of dying.
func logRecover(where string) {
	if r := recover(); r != nil {
		recordPanic()
		logWrite(levelError, "panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

// safeGo runs fn on its own goroutine, logging any panic instead of letting it
// take the process down.
func safeGo(where string, fn func()) {
	go func() {
		defer logRecover(where)
		fn()
	}()
}

// closeLogging flushes the log file at shutdown.
func closeLogging() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
		logFile = nil
	}
}
