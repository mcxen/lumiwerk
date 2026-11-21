package main

import (
	"context"
	"os/exec"
)

// execCommand starts a child process. On Windows it hides the console
// window a GUI app would otherwise open for ffmpeg, ffprobe and other
// console tools. Cover loading, playback and file tools must go through
// this helper (or execCommandContext) rather than os/exec directly.
func execCommand(name string, args ...string) *exec.Cmd {
	return prepareExec(exec.Command(name, args...))
}

// execCommandContext is execCommand with a cancellable context.
func execCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return prepareExec(exec.CommandContext(ctx, name, args...))
}

// execAppCommand starts a user-facing external application. On Windows it
// keeps CREATE_NO_WINDOW so console shims do not flash a CMD window, without
// setting HideWindow (SW_HIDE) which would hide GUI apps that honor nCmdShow.
func execAppCommand(name string, args ...string) *exec.Cmd {
	return prepareAppExec(exec.Command(name, args...))
}
