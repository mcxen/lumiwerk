package main

import "testing"

func TestDragEdgeKeepsMinimum(t *testing.T) {
	seg := editSegment{Start: 1, End: 4}
	got := dragEdge(seg, "start", 10, 20)
	if got.End-got.Start < minSegmentSeconds-0.001 || got.Start < 0 {
		t.Fatalf("start drag = %#v", got)
	}
	got = dragEdge(seg, "end", -10, 20)
	if got.End-got.Start < minSegmentSeconds-0.001 || got.End > 20 {
		t.Fatalf("end drag = %#v", got)
	}
}

func TestSplitAndDeleteSegments(t *testing.T) {
	segs := []editSegment{{End: 10}}
	segs, index := splitEditSegment(segs, 0, 10)
	if index != 1 || len(segs) != 2 || segs[0].End != 5 || segs[1].Start != 5 || segs[1].End != 10 {
		t.Fatalf("split = %#v index %d", segs, index)
	}
	segs, index = deleteEditSegment(segs, 0)
	if index != 0 || len(segs) != 1 || segs[0].Start != 5 {
		t.Fatalf("delete = %#v index %d", segs, index)
	}
}

func TestCenteredAspectAndCropFilter(t *testing.T) {
	full := centeredAspect(1920, 1080, 16.0/9)
	if full.W < 0.98 || full.H < 0.98 {
		t.Fatalf("16:9 should fill a 16:9 frame, got %#v", full)
	}
	square := centeredAspect(1920, 1080, 1)
	if abs32(square.W*1920-square.H*1080) > 2 || square.X <= 0 {
		t.Fatalf("square crop = %#v", square)
	}
	if _, ok := editorCropFilter(1920, 1080, normRect{W: 1, H: 1}, true); ok {
		t.Fatal("full frame should not crop")
	}
	filter, ok := editorCropFilter(1920, 1080, square, true)
	if !ok || filter == "" {
		t.Fatalf("square filter = %q ok %v", filter, ok)
	}
}

func TestCropHitAndDrag(t *testing.T) {
	rect := editorBox{X: 100, Y: 50, W: 200, H: 100}
	if got := cropHit(100, 50, rect, 14); got != "tl" {
		t.Fatalf("corner = %s", got)
	}
	if got := cropHit(200, 100, rect, 14); got != "move" {
		t.Fatalf("inside = %s", got)
	}
	if got := cropHit(10, 10, rect, 14); got != "create" {
		t.Fatalf("outside = %s", got)
	}
	moved := applyCropDrag(normRect{X: 0.2, Y: 0.2, W: 0.4, H: 0.4}, "move", 1000, 1000, 200, 100)
	if moved.X+moved.W > 1.01 || moved.Y+moved.H > 1.01 || moved.W != 0.4 {
		t.Fatalf("move escaped: %#v", moved)
	}
	canvas := containBox(800, 600, 1920, 1080)
	if canvas.W < 700 || canvas.H < 400 || canvas.Y <= 0 {
		t.Fatalf("contain = %#v", canvas)
	}
}

func TestTimelineMapsBothWays(t *testing.T) {
	if got := timelineTime(250, 1000, 8); got != 2 {
		t.Fatalf("time = %v", got)
	}
	if got := timelineX(2, 8, 1000); abs32(got-250) > 0.1 {
		t.Fatalf("x = %v", got)
	}
}
