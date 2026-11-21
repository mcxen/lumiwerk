package main

import (
	"context"
	"testing"
)

func TestExecCommandKeepsArgs(t *testing.T) {
	cmd := execCommand("ffmpeg", "-hide_banner", "-version")
	if got := cmd.Args; len(got) != 3 || got[0] != "ffmpeg" || got[1] != "-hide_banner" || got[2] != "-version" {
		t.Fatalf("args = %v", got)
	}
}

func TestExecCommandContextKeepsArgs(t *testing.T) {
	cmd := execCommandContext(context.Background(), "ffprobe", "-v", "error")
	if got := cmd.Args; len(got) != 3 || got[0] != "ffprobe" || got[1] != "-v" || got[2] != "error" {
		t.Fatalf("args = %v", got)
	}
}
