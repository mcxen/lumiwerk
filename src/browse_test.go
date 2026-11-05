package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPlaybackEdgeFollowsZoom(t *testing.T) {
	if playbackEdge(1) != 2560 || playbackEdge(0.4) != 2560 {
		t.Fatalf("fit edge = %d", playbackEdge(1))
	}
	if playbackEdge(2) != 3840 {
		t.Fatalf("2x edge = %d", playbackEdge(2))
	}
	if playbackEdge(4) != 5120 {
		t.Fatalf("4x edge = %d", playbackEdge(4))
	}
	args := playbackArgs("/tmp/a.mp4", 0, 0, false, playbackEdge(2))
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "min(3840,iw)") || !strings.Contains(joined, "lanczos") || !strings.Contains(joined, "videotoolbox") {
		t.Fatalf("args = %s", joined)
	}
}

func TestThumbBytesStaySmall(t *testing.T) {
	if _, err := os.Stat("/usr/bin/sips"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	img := image.NewRGBA(image.Rect(0, 0, 900, 400))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := thumbBytes(path, false, 128)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > 128 || bounds.Dy() > 128 {
		t.Fatalf("thumb = %dx%d", bounds.Dx(), bounds.Dy())
	}
	large, err := decodeLargeBitmap(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if large == nil {
		t.Fatal("missing large bitmap")
	}
}

func TestDragPathsFollowSelection(t *testing.T) {
	a := &appState{}
	a.Items = []mediaItem{{Path: "/a"}, {Path: "/b"}, {Path: "/c"}}
	a.Selected.Add("/a")
	a.Selected.Add("/b")
	got := a.dragPaths("/a")
	if len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Fatalf("selection drag = %#v", got)
	}
	one := a.dragPaths("/c")
	if len(one) != 1 || one[0] != "/c" {
		t.Fatalf("single drag = %#v", one)
	}
}

func setTrashPathForTest(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	original := trashPath
	trashPath = fn
	t.Cleanup(func() { trashPath = original })
}

func TestTrashDeletePutsBack(t *testing.T) {
	dir := t.TempDir()
	trashDir := filepath.Join(dir, "trash")
	if err := os.Mkdir(trashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	setTrashPathForTest(t, func(src string) (string, error) {
		dest := filepath.Join(trashDir, filepath.Base(src))
		if err := os.Rename(src, dest); err != nil {
			return "", err
		}
		return dest, nil
	})
	applied, err := perform(fileOperation{op: opDelete, paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original remained: %v", err)
	}
	if len(applied.mappings) != 1 || applied.mappings[0].From != path {
		t.Fatalf("trash mapping = %#v", applied.mappings)
	}
	trashed := applied.mappings[0].To
	if _, err := os.Stat(trashed); err != nil {
		t.Fatalf("trashed copy missing: %v", err)
	}
	inverse, ok := inverseOf(applied)
	if !ok || inverse.op != opMove || inverse.mappings[0].To != path {
		t.Fatalf("inverse = %#v ok=%v", inverse, ok)
	}
	if _, err := perform(inverse); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "keep" {
		t.Fatalf("restored = %q err=%v", got, err)
	}
	if _, err := os.Stat(trashed); !os.IsNotExist(err) {
		t.Fatalf("trash copy remained: %v", err)
	}
}

func TestTrashDeleteFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	trashDir := filepath.Join(dir, "trash")
	if err := os.Mkdir(trashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	setTrashPathForTest(t, func(src string) (string, error) {
		if filepath.Base(src) == "second.txt" {
			return "", errors.New("模拟废纸篓失败")
		}
		dest := filepath.Join(trashDir, filepath.Base(src))
		if err := os.Rename(src, dest); err != nil {
			return "", err
		}
		return dest, nil
	})
	applied, err := perform(fileOperation{op: opDelete, paths: []string{first, second}})
	if err == nil {
		t.Fatal("failed trash should report an error")
	}
	if len(applied.mappings) != 0 {
		t.Fatalf("partial mappings survived: %#v", applied.mappings)
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was not restored: %v", filepath.Base(path), err)
		}
	}
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("trash still holds %v", entries)
	}
}

func TestTrashPermissionErrorReachesUser(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mapped := finderTrashError([]byte("execution error: Not authorized to send Apple events to Finder. (-1743)"), errors.New("exit status 1"))
	var perm *trashPermissionError
	if !errors.As(mapped, &perm) || !errors.Is(mapped, errTrashPermission) {
		t.Fatalf("permission error = %#v", mapped)
	}
	if strings.Contains(mapped.Error(), "exit status") || !strings.Contains(mapped.Error(), "自动化") {
		t.Fatalf("unclear user-facing error %q", mapped)
	}
	setTrashPathForTest(t, func(string) (string, error) { return "", mapped })
	a := &appState{}
	err := a.performTracked(fileOperation{op: opDelete, paths: []string{"/ignored.txt"}})
	if !errors.Is(err, errTrashPermission) {
		t.Fatalf("tracked operation error = %#v", err)
	}
	if !strings.Contains(a.HUD, "自动化") || strings.Contains(a.HUD, "exit status") {
		t.Fatalf("toast = %q", a.HUD)
	}
}

func TestMediaClockStaysShared(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	clock := mediaClock{origin: 5}
	if clock.at(start.Add(time.Second)) != 5 {
		t.Fatal("paused clock moved")
	}
	clock.play(start)
	got := clock.at(start.Add(100 * time.Millisecond))
	if math.Abs(got-5.1) > 1e-9 {
		t.Fatalf("playhead = %v", got)
	}
	if soundtrackAt(clock) != 5 {
		t.Fatalf("soundtrack join = %v", soundtrackAt(clock))
	}
	clock.pause(start.Add(100 * time.Millisecond))
	if math.Abs(clock.at(start.Add(time.Second))-5.1) > 1e-9 {
		t.Fatal("pause did not freeze the playhead")
	}
	frames := []timedFrame{{at: 0}, {at: 0.1}, {at: 0.2}, {at: 0.3}}
	show, ok, ahead, hasAhead := shownFrame(frames, 0.25)
	if !ok || show.at != 0.2 || !hasAhead || ahead.at != 0.3 {
		t.Fatalf("shown=%v ok=%v ahead=%v has=%v", show.at, ok, ahead.at, hasAhead)
	}
}

func TestThumbBandAndNudge(t *testing.T) {
	if thumbBand(400) != 384 || thumbBand(600) != 512 || thumbBand(800) != 768 || thumbBand(1000) != 1024 {
		t.Fatalf("bands = %d %d %d %d", thumbBand(400), thumbBand(600), thumbBand(800), thumbBand(1000))
	}
	band := thumbBand(thumbEdge(64))
	a := &appState{Thumb: 64, Items: []mediaItem{{Path: "a.jpg", Decoded: true, ThumbEdge: band}}}
	a.nudgeThumb(1.05)
	if !a.Items[0].Decoded || !a.thumbDue.IsZero() {
		t.Fatal("same band should keep the thumbnail")
	}
	a.nudgeThumb(8)
	if a.Thumb != 512 || !a.Items[0].Decoded || a.thumbDue.IsZero() {
		t.Fatalf("thumb=%v decoded=%v due=%v", a.Thumb, a.Items[0].Decoded, a.thumbDue)
	}
	if a.applyThumbSettle(a.thumbDue.Add(-time.Millisecond)) || !a.Items[0].Decoded {
		t.Fatal("gesture should keep the current thumbnail")
	}
	if !a.applyThumbSettle(a.thumbDue) || a.Items[0].Decoded {
		t.Fatal("sharper band should reload after the gesture")
	}
	shrunk := &appState{Thumb: 512, Items: []mediaItem{{Decoded: true, ThumbEdge: 1024}}}
	shrunk.nudgeThumb(0.1)
	if shrunk.Thumb != 64 || !shrunk.applyThumbSettle(shrunk.thumbDue) || !shrunk.Items[0].Decoded {
		t.Fatal("a finer thumbnail should stay when the grid shrinks")
	}
	now := time.Unix(1_700_000_000, 0)
	if shouldRestartVideoEdge(2560, 2, time.Time{}, now) {
		t.Fatal("video edge should wait for the zoom to settle")
	}
	if shouldRestartVideoEdge(2560, 2, now.Add(time.Second), now) {
		t.Fatal("video edge restarted early")
	}
	if !shouldRestartVideoEdge(2560, 2, now, now) {
		t.Fatal("settled zoom should restart the decoder")
	}
	if shouldRestartVideoEdge(3840, 2, now, now) {
		t.Fatal("matching edge should stay")
	}
}

func TestFavoritesAndFrameName(t *testing.T) {
	list := toggleFavoriteList(nil, "/A")
	list = toggleFavoriteList(list, "/b")
	if len(list) != 2 || list[0] != "/b" {
		t.Fatalf("added = %#v", list)
	}
	list = toggleFavoriteList(list, "/A")
	if favoriteContains(list, "/a") || len(list) != 1 {
		t.Fatalf("removed = %#v", list)
	}
	if got := frameFileName("/tmp/clip.mp4", 1.25); got != "clip_frame_1250.png" {
		t.Fatalf("frame name = %s", got)
	}
}

func TestRotateImageFileSwapsEdges(t *testing.T) {
	if _, err := os.Stat("/usr/bin/sips"); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.png")
	img := image.NewRGBA(image.Rect(0, 0, 30, 10))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rotateImageFile(path, 90); err != nil {
		t.Fatal(err)
	}
	w, h := probeImageSize(path)
	if w != 10 || h != 30 {
		t.Fatalf("rotated = %dx%d", w, h)
	}
}

func TestWaterfallColumnsFollowThumb(t *testing.T) {
	wide := waterfallColumns(1200, 128)
	narrow := waterfallColumns(1200, 512)
	if wide <= narrow {
		t.Fatalf("larger thumbs should use fewer columns: %d vs %d", wide, narrow)
	}
	if waterfallColumns(40, 512) != 1 {
		t.Fatal("a narrow view should keep one column")
	}
	rows := justifiedRows([]float64{1, 1, 1, 2}, 400, 200, 8)
	if len(rows) < 2 {
		t.Fatalf("rows = %#v", rows)
	}
	short := justifiedRows([]float64{1, 1, 1, 2}, 400, 80, 8)
	if len(short) > len(rows) {
		t.Fatalf("shorter rows packed less: %#v vs %#v", short, rows)
	}
}

func TestWaterfallColumnsUseFinderSizedTilesWithoutCardGutters(t *testing.T) {
	cols := waterfallColumns(1200, 128)
	if cols < 4 || cols > 6 {
		t.Fatalf("columns = %d, want a Finder-like 4-6 for a 1200px view", cols)
	}
	if large := waterfallColumns(1200, 512); large != 2 {
		t.Fatalf("large-thumb columns = %d, want 2", large)
	}
	flex, _ := waterfallColumnWidth(1200, cols)
	frames := waterfallFrames([]float32{1.78, 0.75, 1, 1.78, 0.75, 1}, 1200, 128)
	if frames[0].X != flowPad || frames[0].W != flex {
		t.Fatalf("first card = %+v, want x=%v width=%v", frames[0], flowPad, flex)
	}
}

func TestAnchoredGridKeepsTheItemUnderThePointer(t *testing.T) {
	y := anchoredGridScroll(200, 50, 40, 300, 100, 200, 3, 2, 30)
	if abs32(y-640) > 0.01 {
		t.Fatalf("scroll = %v, want 640", y)
	}
	same := anchoredScroll(100, 40, 80, 160)
	if abs32(same-240) > 0.01 {
		t.Fatalf("uniform = %v, want 240", same)
	}
}

func TestBatchNameNumbersTokensInsideTheOriginalName(t *testing.T) {
	got := composeBatchName("shot{index}", ".jpg", "roll", 4, renameRule{Pattern: "{name}"}, time.Unix(0, 0))
	if got != "shot4.jpg" {
		t.Fatalf("name = %q", got)
	}
}

func TestPinchKeepsTheGridItemUnderThePointer(t *testing.T) {
	a := &appState{Thumb: 96, Root: "/tmp/photos", SidebarShown: true, SidebarWidth: 180}
	for i := 0; i < 48; i++ {
		name := fmt.Sprintf("frame-%02d.jpg", i)
		a.Items = append(a.Items, mediaItem{Name: name, Path: "/tmp/photos/" + name, Ext: ".jpg"})
		a.Visible = append(a.Visible, i)
	}
	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()
	te.Frame()
	if a.browseBox.H < 10 {
		t.Fatalf("browse box = %+v", a.browseBox)
	}
	te.Wheel(a.browseBox.X+40, a.browseBox.Y+80, 0, 240)
	te.Advance(400 * time.Millisecond)
	te.Frame()
	before, ok := te.Find("frame-08.jpg")
	if !ok {
		t.Fatalf("frame-08 not visible, texts %v scroll %v", te.Texts(), a.browseScroll.Y)
	}
	anchorY := before.Y + before.H/2
	te.Pinch(before.X+before.W/2, anchorY, 1.6)
	te.Frame()
	after, ok := te.Find("frame-08.jpg")
	if !ok {
		t.Fatalf("frame-08 left the view after pinch, thumb %v scroll %v", a.Thumb, a.browseScroll.Y)
	}
	if d := after.Y + after.H/2 - anchorY; d < -36 || d > 36 {
		t.Fatalf("item jumped by %v (before %v after %v) thumb %v scroll %v box %+v", d, before, after, a.Thumb, a.browseScroll.Y, a.browseBox)
	}
}

func TestAnchoredFlowKeepsTheCardUnderThePointer(t *testing.T) {
	aspects := make([]float32, 40)
	for i := range aspects {
		aspects[i] = 1
		if i%5 == 0 {
			aspects[i] = 1.8
		}
	}
	const viewW = 360
	oldThumb := float32(110)
	newThumb := float32(190)
	for _, frames := range []func([]float32, float32, float32) []flowBox{waterfallFrames, justifiedFrames} {
		old := frames(aspects, viewW, oldThumb)
		next := frames(aspects, viewW, newThumb)
		hit := old[23]
		if hit.H < 1 {
			t.Fatal("missing card")
		}
		localY := float32(50)
		scroll := hit.Y + hit.H*0.4 - localY
		if scroll < 0 {
			t.Fatalf("scroll %v for %v", scroll, hit)
		}
		y, ok := anchoredFlowScroll(old, next, 0, scroll, hit.X+hit.W/2, localY)
		if !ok {
			t.Fatal("anchor missed")
		}
		got := y + localY
		want := next[23].Y + next[23].H*0.4
		if abs32(got-want) > 0.6 {
			t.Fatalf("point %v, want %v (scroll %v card %+v)", got, want, y, next[23])
		}
	}
}

func TestPinchKeepsTheFlowItemUnderThePointer(t *testing.T) {
	for _, mode := range []int{2, 3} {
		mode := mode
		t.Run(fmt.Sprintf("layout-%d", mode), func(t *testing.T) {
			// A short window keeps the masonry taller than the view after the
			// pinch, so the anchor has to scroll instead of sitting at 0.
			a := &appState{Thumb: 150, Root: "/tmp/photos", SidebarShown: true, SidebarWidth: 180, LayoutMode: mode}
			for i := 0; i < 80; i++ {
				name := fmt.Sprintf("frame-%02d.jpg", i)
				a.Items = append(a.Items, mediaItem{Name: name, Path: "/tmp/photos/" + name, Ext: ".jpg", InfoLoaded: true})
				a.Visible = append(a.Visible, i)
			}
			te := ui.NewTester(a.view, 860, 460)
			te.Frame()
			te.Frame()
			if a.browseBox.H < 10 {
				t.Fatalf("browse box = %+v", a.browseBox)
			}
			// Flow views build only the scroll window now, so scroll until
			// frame-17 is genuinely on screen before pinching it.
			te.Wheel(a.browseBox.X+40, a.browseBox.Y+80, 0, 280)
			te.Advance(400 * time.Millisecond)
			te.Frame()
			var before ui.Rect
			var ok bool
			for hop := 0; hop < 60; hop++ {
				if before, ok = te.Find("frame-17.jpg"); ok {
					break
				}
				a.browseScroll.Y += 120
				// Stay past the pinch-settle wait so thumb rebanding never
				// rebuilds cards mid-approach.
				te.Advance(zoomSettle + 10*time.Millisecond)
				te.Frame()
			}
			if !ok {
				t.Fatalf("frame-17 not found, texts %v scroll %v mode %d", te.Texts(), a.browseScroll.Y, mode)
			}
			anchorY := before.Y + before.H/2
			te.Pinch(before.X+before.W/2, anchorY, 1.6)
			te.Frame()
			te.Frame()
			after, ok := te.Find("frame-17.jpg")
			if !ok {
				t.Fatalf("frame-17 left the view after pinch, thumb %v scroll %v mode %d", a.Thumb, a.browseScroll.Y, mode)
			}
			if d := after.Y + after.H/2 - anchorY; d < -48 || d > 48 {
				t.Fatalf("item jumped by %v (before %v after %v) thumb %v scroll %v box %+v mode %d", d, before, after, a.Thumb, a.browseScroll.Y, a.browseBox, mode)
			}
		})
	}
}
