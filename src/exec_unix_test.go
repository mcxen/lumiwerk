//go:build unix

package main

import (
	"context"
	"testing"
	"time"
)

func TestExecCommandDetachesControllingTTY(t *testing.T) {
	a := execCommand("ffmpeg", "-version").SysProcAttr
	if a == nil || !a.Setsid {
		t.Fatalf("execCommand SysProcAttr = %+v, want Setsid=true", a)
	}
	b := execCommandContext(context.Background(), "ffprobe", "-version").SysProcAttr
	if b == nil || !b.Setsid {
		t.Fatalf("execCommandContext SysProcAttr = %+v, want Setsid=true", b)
	}
	if app := execAppCommand("open", "/tmp/a.png").SysProcAttr; app != nil && app.Setsid {
		t.Fatalf("execAppCommand unexpectedly set Setsid=true: %+v", app)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := execCommandContext(ctx, "/bin/sh", "-c", "exec </dev/tty")
	if err := cmd.Run(); err == nil {
		t.Fatal("child process unexpectedly opened /dev/tty")
	} else if ctx.Err() != nil {
		t.Fatalf("child hung opening /dev/tty: %v", ctx.Err())
	}
}
