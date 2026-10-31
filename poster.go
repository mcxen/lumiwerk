package main

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// videoPosterPNG extracts the first frame of a video as a PNG using ffmpeg.
func videoPosterPNG(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-i", path, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
