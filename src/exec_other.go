//go:build !windows

package main

import "os/exec"

func prepareExec(cmd *exec.Cmd) *exec.Cmd { return cmd }
