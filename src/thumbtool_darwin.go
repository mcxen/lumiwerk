//go:build darwin

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// quickLookTimeout bounds one qlmanage run. QuickLook normally answers in
// well under a second; a file it cannot open (missing, on a stalled network
// volume, an unknown container) can leave qlmanage waiting forever, and
// before this limit those processes outlived the app and the test binary.
var quickLookTimeout = 15 * time.Second

// thumbToolWaitDelay is how long Wait keeps reading output after the
// process group was killed, in case a grandchild still holds the pipes.
const thumbToolWaitDelay = 2 * time.Second

// runThumbTool runs an external thumbnailer and returns its combined
// output. The command runs in its own process group under ctx plus
// timeout; when either ends first, the whole group gets SIGKILL, so no
// helper the tool forked is left behind, and Wait always reaps the child.
func runThumbTool(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Setpgid made the child the group leader, so its pid is the pgid.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = thumbToolWaitDelay
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out.Bytes(), fmt.Errorf("%s: %w", name, ctxErr)
	}
	return out.Bytes(), err
}
