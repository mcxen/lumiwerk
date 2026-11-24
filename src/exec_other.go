//go:build !windows && !unix

package main

import "os/exec"

func prepareExec(cmd *exec.Cmd) *exec.Cmd { return cmd }

func prepareAppExec(cmd *exec.Cmd) *exec.Cmd { return cmd }
