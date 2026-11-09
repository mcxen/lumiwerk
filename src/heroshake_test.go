package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// shakeTrace advances in 16 ms frames and records the red box width each
// frame: the trace of one transition.
func shakeTrace(te *ui.Tester, frames int, each func(i int)) []int {
	var out []int
	for i := 0; i < frames; i++ {
		if each != nil {
			each(i)
		}
		te.Advance(16 * time.Millisecond)
		out = append(out, redBox(te.Image()).Dx())
	}
	return out
}

// monotone fails the test when the trace moves against its direction by more
// than slack pixels: a twitch rather than a glide.
func monotone(t *testing.T, name string, trace []int, grow bool, slack int) {
	t.Helper()
	for i := 1; i < len(trace); i++ {
		d := trace[i] - trace[i-1]
		if !grow {
			d = -d
		}
		if d < -slack {
			t.Errorf("%s: frame %d moved the wrong way (%d -> %d); trace %v", name, i, trace[i-1], trace[i], trace)
			return
		}
	}
}

// TestHeroTransitionHasNoTwitch opens and closes the large view the way the
// Space key does, with a thumbnail whose pixel size differs from the original
// and the full-size bitmap arriving mid-flight, and checks both directions
// glide: the picture never steps back, and it never jumps in one frame.
func TestHeroTransitionHasNoTwitch(t *testing.T) {
	for _, layout := range []int{0, 2, 3} {
		a := heroApp(t, layout)
		// A real thumbnail is a scaled copy whose size is rounded; the
		// original arrives later and is capped to the stage edge.
		thumb := heroSample(t, 256, 145)
		a.Items[0].Bitmap = thumb
		a.Items[0].Large = nil
		te := ui.NewTester(a.view, 1100, 760)
		te.Frame()
		te.Advance(time.Second)

		a.openLargeImage(0)
		te.Frame()
		open := shakeTrace(te, 40, func(i int) {
			if i == 6 {
				a.Items[0].Large = heroSample(t, 1280, 720)
			}
		})
		monotone(t, "open", open, true, 1)
		for i := 1; i < len(open); i++ {
			if open[i]-open[i-1] > 330 {
				t.Errorf("layout %d: open jumped %d px in one frame: %v", layout, open[i]-open[i-1], open)
				break
			}
		}

		a.closeLargeImage()
		te.Frame()
		closing := shakeTrace(te, 40, nil)
		monotone(t, "close", closing, false, 1)
		for i := 1; i < len(closing); i++ {
			if closing[i-1]-closing[i] > 330 {
				t.Errorf("layout %d: close jumped %d px in one frame: %v", layout, closing[i-1]-closing[i], closing)
				break
			}
		}
	}
}
