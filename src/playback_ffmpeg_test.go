package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pamFrame(w, h int, fill byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "P7\nWIDTH %d\nHEIGHT %d\nDEPTH 4\nMAXVAL 255\nTUPLTYPE RGB_ALPHA\nENDHDR\n", w, h)
	px := bytes.Repeat([]byte{fill, fill, fill, 255}, w*h)
	b.Write(px)
	return b.Bytes()
}

// The fallback reads raw RGBA PAM frames into tight RGBA images, which
// ui.Bitmap.Update takes without converting them on the main thread.
func TestDecodeFrameReadsPAMStream(t *testing.T) {
	stream := append(pamFrame(4, 2, 10), pamFrame(3, 5, 200)...)
	r := bufio.NewReader(bytes.NewReader(stream))
	for _, want := range []struct {
		w, h int
		v    byte
	}{{4, 2, 10}, {3, 5, 200}} {
		img, err := decodeFrame(r)
		if err != nil {
			t.Fatal(err)
		}
		rgba, ok := img.(*image.RGBA)
		if !ok || rgba.Rect != image.Rect(0, 0, want.w, want.h) || rgba.Stride != want.w*4 {
			t.Fatalf("frame %T %v, want tight %dx%d RGBA", img, img.Bounds(), want.w, want.h)
		}
		if rgba.Pix[0] != want.v || rgba.Pix[3] != 255 {
			t.Fatalf("pixel = %v", rgba.Pix[:4])
		}
	}
	if _, err := decodeFrame(r); !errors.Is(err, io.EOF) {
		t.Fatalf("end of stream = %v, want EOF", err)
	}
	truncated := pamFrame(4, 4, 1)
	if _, err := decodeFrame(bufio.NewReader(bytes.NewReader(truncated[:len(truncated)-3]))); !errors.Is(err, io.EOF) {
		t.Fatalf("a cut frame = %v, want EOF", err)
	}
	bad := strings.Replace(string(pamFrame(2, 2, 1)), "DEPTH 4", "DEPTH 3", 1)
	if _, err := decodeFrame(bufio.NewReader(strings.NewReader(bad))); err == nil {
		t.Fatal("RGB without alpha must be rejected")
	}
}

func TestPlaybackFrameDurationCapsAndDefaults(t *testing.T) {
	for _, tc := range []struct{ in, want time.Duration }{
		{0, time.Second / 30},
		{time.Second / 60, time.Second / 30},
		{time.Second / 24, time.Second / 24},
		{100 * time.Millisecond, 100 * time.Millisecond},
	} {
		if got := playbackFrameDuration(tc.in); got != tc.want {
			t.Errorf("playbackFrameDuration(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// A variable-frame-rate source (screen recordings are) must come out of the
// pump with frame times that match the media: the old pump added a nominal
// interval per frame and drifted away from the sound.
func TestFramePumpTimesMatchVariableRateSource(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
	video := filepath.Join(t.TempDir(), "vfr.mp4")
	// 4 s: the first 2 s at 10 fps, the last 2 s at 50 fps, kept VFR.
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=50:duration=4",
		"-vf", "select='if(lt(t,2),not(mod(n,5)),1)'", "-fps_mode", "vfr",
		"-pix_fmt", "yuv420p", "-c:v", "libx264", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	dec := newFFmpegDecoderAt(video, 0)
	dec.plain = true
	frames := startFramePump(dec)
	defer dec.Close()
	var last timedFrame
	var prev []byte
	count, repeatsEarly, repeatsLate := 0, 0, 0
	timeout := time.After(30 * time.Second)
	for done := false; !done; {
		select {
		case f := <-frames:
			if f.err != nil {
				done = true
				break
			}
			if count == 0 {
				if b := f.img.Bounds(); b.Dx() != 320 || b.Dy() != 240 {
					t.Fatalf("frame size %v", b)
				}
			}
			pix := f.img.(*image.RGBA).Pix
			if prev != nil && bytes.Equal(prev, pix) {
				if f.at < 1.9 {
					repeatsEarly++
				} else if f.at > 2.1 {
					repeatsLate++
				}
			}
			prev = append(prev[:0], pix...)
			last = f
			count++
		case <-timeout:
			t.Fatal("pump did not finish")
		}
	}
	// At a constant 30 fps each 10 fps source frame of the first 2 s shows
	// three times (two repeats each), so the picture stays where the sound
	// is; the 50 fps part drops frames and never repeats one.
	if repeatsEarly < 30 || repeatsLate > 2 {
		t.Fatalf("repeats early=%d late=%d: output is not constant-rate, frame times drift from the media", repeatsEarly, repeatsLate)
	}
	interval := dec.frameDuration().Seconds()
	end := last.at + interval
	if math.Abs(end-4) > 3*interval {
		t.Fatalf("pump ended at %.3f s after %d frames (interval %.4f); the media is 4 s", end, count, interval)
	}
}

// A decoder that runs ahead of the clock queues many future frames; each
// one must still be shown when its time comes. The old poll drained the
// queue, kept the first future frame and dropped the rest.
func TestFFmpegPollShowsEveryQueuedFrameInTime(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	now := t0
	orig := playbackNow
	playbackNow = func() time.Time { return now }
	t.Cleanup(func() { playbackNow = orig })
	b := &ffmpegBackend{volume: 1, speed: 1, playing: true, audioRunning: true, duration: 10}
	b.frames = make(chan timedFrame, 16)
	for i := 0; i < 10; i++ {
		b.frames <- timedFrame{img: image.NewRGBA(image.Rect(0, 0, 2, 2)), at: float64(i) * 0.1}
	}
	b.clock.seek(0, true, t0)
	for k := 0; k < 10; k++ {
		now = t0.Add(time.Duration(k) * 100 * time.Millisecond)
		b.pollLocked()
		if math.Abs(b.frameAt-float64(k)*0.1) > 1e-9 {
			t.Fatalf("at %.1f s the frame shown is from %.3f s", float64(k)*0.1, b.frameAt)
		}
		if b.stalled {
			t.Fatalf("at %.1f s a queued, on-time stream reports buffering", float64(k)*0.1)
		}
	}
}
