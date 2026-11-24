//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

func prepareExec(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

func prepareAppExec(cmd *exec.Cmd) *exec.Cmd { return cmd }
