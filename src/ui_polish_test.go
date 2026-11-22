package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func polishTestApp(t *testing.T, layout, n int) *appState {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	a := &appState{
		Chosen:       0,
		LargeIndex:   -1,
		keyPivot:     0,
		Thumb:        150,
		Root:         "/probe",
		LayoutMode:   layout,
		SidebarShown: false,
		Profiles:     prefsFile{View: defaultViewPreferences(), ViewSet: true},
	}
	a.Items = make([]mediaItem, n)
	a.Visible = make([]int, n)
	for i := range a.Items {
		name := fmt.Sprintf("pic-%02d.jpg", i)
		path := filepath.Join(a.Root, name)
		a.Items[i] = mediaItem{
			Path:       path,
			Name:       name,
			Ext:        "jpg",
			Size:       int64((i + 1) * 1024),
			ModTime:    1_700_000_000 + int64(i*60),
			Decoded:    true,
			InfoLoaded: true,
			Info:       mediaInfo{Width: 1200, Height: 800},
		}
		a.Visible[i] = i
	}
	if n > 0 {
		a.Selected.Add(a.Items[0].Path)
	}
	return a
}

// 1. Stepping in large view and closing via Esc syncs Chosen, Selected, and scrolls the chosen item into view.
func TestLargeViewStepAndEscapeSyncsBrowseSelection(t *testing.T) {
	a := polishTestApp(t, 0, 60)
	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()

	a.openLargeImage(0)
	te.Frame()
	for i := 0; i < 50; i++ {
		a.nextImage()
	}
	te.Frame()
	if a.LargeIndex != 50 {
		t.Fatalf("LargeIndex = %d, want 50", a.LargeIndex)
	}

	te.Key(0, ui.KeyEscape)
	if a.InLargeView {
		t.Fatal("Escape did not exit large view")
	}
	if a.Chosen != 50 {
		t.Fatalf("Chosen after exit = %d, want 50", a.Chosen)
	}
	if a.Selected.Len() != 1 || !a.Selected.Has(a.Items[50].Path) {
		t.Fatalf("Selected after exit = %v, want %q", a.Selected.All(), a.Items[50].Path)
	}
	te.Frame()
	if !te.HasText("pic-50.jpg") {
		t.Fatalf("closing large view did not scroll pic-50.jpg into view: %v", te.Texts())
	}
}

// 2. Status bar displays "第 X / Y 项" instead of the misleading "X 选中".
func TestStatusBarShowsPositionNotFakeSelectionCount(t *testing.T) {
	a := polishTestApp(t, 0, 4)
	a.chooseOnly(2)
	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()
	if !te.HasText("第 3 / 4 项") {
		t.Fatalf("expected position label '第 3 / 4 项', got %v", te.Texts())
	}
	if te.HasText("3 选中") {
		t.Fatalf("misleading '3 选中' still present: %v", te.Texts())
	}
}

// 3. Webtoon mode wakes the floating control bar on bottom-zone pointer entry in auto and hover modes.
func TestWebtoonWakesControlBarInAutoAndHover(t *testing.T) {
	for _, mode := range []string{barAuto, barHover} {
		t.Run(mode, func(t *testing.T) {
			a := polishTestApp(t, 0, 3)
			a.Profiles.Webtoon = true
			a.Profiles.ViewerBar = mode
			a.openLargeImage(0)
			te := ui.NewTester(a.view, 1100, 760)
			a.playerMoved = te.Now().Add(-10 * time.Second)
			te.Frame()
			if a.lastBarShown {
				t.Fatalf("mode %s: bar should start faded when idle", mode)
			}
			// Move pointer into the bottom hot zone over the stage.
			te.Move(550, 720)
			te.Frame()
			if !a.playerInZone {
				t.Fatalf("mode %s: webtoon did not register bottom hot zone", mode)
			}
			if !a.lastBarShown {
				t.Fatalf("mode %s: control bar did not wake on bottom hover in webtoon", mode)
			}
			if !te.HasText("纵向连续") {
				t.Fatalf("mode %s: control bar missing after wake: %v", mode, te.Texts())
			}
		})
	}
}

// 4. Image large view shortcuts (W webtoon, S no-op in webtoon, F mirror, Q/E rotate, =/0 zoom) and tooltips.
func TestLargeViewShortcutConflictsAndTooltips(t *testing.T) {
	a := polishTestApp(t, 0, 2)
	a.openLargeImage(0)
	a.playerMoved = time.Time{} // keep control bar visible for inspection
	te := ui.NewTester(a.view, 1280, 820)
	te.Frame()

	if info, ok := te.Access("全屏"); !ok || !strings.Contains(info.Description, "⌥Enter") {
		t.Fatalf("fullscreen tooltip = %+v (ok=%v), want ⌥Enter", info, ok)
	}
	if info, ok := te.Access("镜像"); !ok || !strings.Contains(info.Description, "F") {
		t.Fatalf("mirror tooltip = %+v (ok=%v), want F", info, ok)
	}

	// W toggles webtoon without zooming.
	te.Key(0, ui.KeyW)
	te.Frame()
	if !a.webtoon() || a.Zoom != 1 {
		t.Fatalf("after W: webtoon=%v zoom=%v, want true and 1", a.webtoon(), a.Zoom)
	}
	// S in webtoon must not zoom or trigger zoom HUD.
	a.HUD = ""
	te.Key(0, ui.KeyS)
	te.Frame()
	if a.Zoom != 1 || a.HUD != "" {
		t.Fatalf("S in webtoon changed zoom=%v hud=%q", a.Zoom, a.HUD)
	}
	// W again leaves webtoon.
	te.Key(0, ui.KeyW)
	te.Frame()
	if a.webtoon() {
		t.Fatal("second W did not exit webtoon")
	}

	// F mirrors image, E/Q rotate, =/0 zoom.
	te.Key(0, ui.KeyF)
	te.Frame()
	if !a.Mirror {
		t.Fatal("F did not mirror image")
	}
	te.Key(0, ui.KeyE)
	te.Frame()
	if a.Rotation != 90 {
		t.Fatalf("E rotation = %v, want 90", a.Rotation)
	}
	te.Key(0, ui.KeyQ)
	te.Frame()
	if a.Rotation != 0 {
		t.Fatalf("Q rotation = %v, want 0", a.Rotation)
	}
	te.Key(0, ui.KeyEqual)
	te.Frame()
	if a.Zoom != 1.25 {
		t.Fatalf("= zoom = %v, want 1.25", a.Zoom)
	}
	te.Key(0, ui.Key0)
	te.Frame()
	if a.Zoom != 1 {
		t.Fatalf("0 zoom = %v, want 1", a.Zoom)
	}
}

// 5. Gallery progress bar uses local x accurately across the width and skips directories.
func TestGalleryProgressScrubUsesLocalXAndSkipsDirs(t *testing.T) {
	a := polishTestApp(t, 0, 5)
	// Make index 4 a directory so clicking near the right end skips it to 3,
	// and index 1 a file so clicking ~30% selects index 1 cleanly without left offset.
	a.Items[4].IsDir = true
	a.Gallery = true
	a.galleryPeek = true
	a.openLargeImage(0)
	te := ui.NewTester(a.view, 1000, 700)
	te.Frame()
	te.Frame()

	// Find the progress bar hit row via the "1 / 5" counter beside it.
	cnt, ok := te.Find("1 / 5")
	if !ok {
		t.Fatalf("gallery counter missing: %v", te.Texts())
	}
	// Click around 50% of the track width (to the left of the counter).
	hitW := cnt.X - 16
	y := cnt.Y + cnt.H/2
	te.Press(8+hitW*0.50, y)
	te.Release(8+hitW*0.50, y)
	te.Frame()
	if a.LargeIndex != 2 {
		t.Fatalf("scrub at 50%% landed on %d, want 2", a.LargeIndex)
	}
	// Click at 95% (inside directory slot 4): nearestMediaIndex skips to 3.
	te.Press(8+hitW*0.95, y)
	te.Release(8+hitW*0.95, y)
	te.Frame()
	if a.LargeIndex != 3 {
		t.Fatalf("scrub at dir slot landed on %d, want 3", a.LargeIndex)
	}
}

// 6. justifiedFrames matches justifiedWidths on mixed-aspect rows and flowColumnNeighbor aligns vertically.
func TestJustifiedFramesMatchProportionalWidthsAndArrowNav(t *testing.T) {
	aspects := []float32{2.2, 0.6, 1.8, 0.7, 2.0, 0.8}
	const viewW, thumb = float32(600), float32(150)
	boxes := justifiedFrames(aspects, viewW, thumb)
	if len(boxes) != len(aspects) {
		t.Fatalf("got %d boxes", len(boxes))
	}
	// Item 0 (wide, ar=2.2) must be significantly wider than item 1 (tall, ar=0.6) on row 0.
	if !(boxes[0].W > boxes[1].W*2) {
		t.Fatalf("justifiedFrames still equal-width? box0=%+v box1=%+v", boxes[0], boxes[1])
	}
	// Row 0 items should span right edge (viewW - flowPad).
	var row0Right float32
	for _, b := range boxes {
		if b.Y == boxes[0].Y && b.X+b.W > row0Right {
			row0Right = b.X + b.W
		}
	}
	if abs32(row0Right-(viewW-flowPad)) > 1.5 {
		t.Fatalf("row0 right = %v, want %v", row0Right, viewW-flowPad)
	}

	// Verify flowColumnNeighbor uses the true proportional column positions.
	a := polishTestApp(t, 3, len(aspects))
	for i, ar := range aspects {
		a.Items[i].Info = mediaInfo{Width: int(ar * 1000), Height: 1000}
	}
	a.Thumb = float64(thumb)
	a.browseBox = browseBox{W: viewW, H: 600}
	to, ok := a.flowColumnNeighbor(0, 1)
	if !ok || boxes[to].Y <= boxes[0].Y {
		t.Fatalf("flowColumnNeighbor(0, +1) = %d (ok=%v) boxes=%+v", to, ok, boxes)
	}
}

// 7. Settings preserve Window/Pane on reopen, offer "" ("无"), persist paths, and clear keyCapture on page switch.
func TestSettingsPersistenceAndCaptureCleanup(t *testing.T) {
	a := newSettingsTestState(t)
	a.setPage("keys")
	keyCapture.id = "sidebar"
	keyCapture.conflict = "冲突"
	a.setPage("files")
	if keyCapture.id != "" || keyCapture.conflict != "" {
		t.Fatalf("switching page did not clear keyCapture: %+v", keyCapture)
	}

	// Reopening settings preserves Window and Pane.
	a.Settings.Pane = 2
	a.openSettings()
	if a.Settings.Pane != 2 {
		t.Fatalf("openSettings reset Pane to %d", a.Settings.Pane)
	}

	// shortcutChoices includes "" ("无") and clearing persists across loadPrefs.
	if len(shortcutChoices) == 0 || shortcutChoices[0] != "" || shortcutName(shortcutChoices[0]) != "无" {
		t.Fatalf("shortcutChoices[0] = %q, want empty string for 无", shortcutChoices[0])
	}
	a.Settings.ImageShortcut = ""
	a.applyActionSettings()
	if got := loadPrefs().ImageCopyShortcut; got != "" {
		t.Fatalf("cleared ImageCopyShortcut reverted to %q", got)
	}

	// Manual edits to StartFolder, Dest, and VideoDest persist across frames.
	wantStart := t.TempDir()
	wantImg := t.TempDir()
	wantVid := t.TempDir()
	a.setPage("general")
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 780, 900)
	te.Frame()
	startField, ok := te.AccessByRole(ui.RoleTextField, "启动目录")
	if !ok {
		t.Fatalf("StartFolder input missing: %v", te.Texts())
	}
	te.ClickAt(startField.Bounds.X+startField.Bounds.W/2, startField.Bounds.Y+startField.Bounds.H/2)
	te.Type(wantStart)
	te.Frame()
	if got := loadPrefs().Profiles[a.Profiles.Active].Root; got != wantStart {
		t.Fatalf("manual StartFolder not persisted: got %q, want %q", got, wantStart)
	}

	a.setPage("files")
	te.Frame()
	imgField, ok := te.AccessByRole(ui.RoleTextField, "图片复制到")
	if !ok {
		t.Fatalf("Dest input missing: %v", te.Texts())
	}
	te.ClickAt(imgField.Bounds.X+imgField.Bounds.W/2, imgField.Bounds.Y+imgField.Bounds.H/2)
	te.Type(wantImg)
	vidField, ok := te.AccessByRole(ui.RoleTextField, "视频复制到")
	if !ok {
		t.Fatalf("VideoDest input missing: %v", te.Texts())
	}
	te.ClickAt(vidField.Bounds.X+vidField.Bounds.W/2, vidField.Bounds.Y+vidField.Bounds.H/2)
	te.Type(wantVid)
	te.Frame()
	loaded := loadPrefs()
	if loaded.ImageDestination != wantImg || loaded.VideoDestination != wantVid {
		t.Fatalf("manual Dest/VideoDest not persisted: img=%q vid=%q", loaded.ImageDestination, loaded.VideoDestination)
	}
}

// 8. Cmd+F opens search bar, filters items, protects bare keys while focused, and Esc clears & closes.
func TestSearchBarShortcutFilteringAndEscape(t *testing.T) {
	a := polishTestApp(t, 0, 3)
	a.Items[0].Name, a.Items[0].Path = "alpha.jpg", "/probe/alpha.jpg"
	a.Items[1].Name, a.Items[1].Path = "beta.jpg", "/probe/beta.jpg"
	a.Items[2].Name, a.Items[2].Path = "gamma.jpg", "/probe/gamma.jpg"

	te := ui.NewTester(a.view, 1100, 760)
	te.Frame()
	te.Key(ui.Cmd, ui.KeyF)
	te.Frame()
	te.Frame()
	if !a.SearchOpen {
		t.Fatal("Cmd+F did not open search bar")
	}

	// Pressing 'w' / 'a' / 's' / 'd' while search is focused must type into query, not trigger browse navigation.
	rootBefore := a.Root
	te.Type("beta")
	te.Frame()
	if a.Root != rootBefore {
		t.Fatalf("typing in search changed root from %q to %q", rootBefore, a.Root)
	}
	if a.Query != "beta" || len(a.Visible) != 1 || a.Visible[0] != 1 {
		t.Fatalf("search query=%q visible=%v, want 'beta' and [1]", a.Query, a.Visible)
	}
	if !te.HasText("1 / 3") {
		t.Fatalf("search match count '1 / 3' missing: %v", te.Texts())
	}

	te.Key(0, ui.KeyEscape)
	te.Frame()
	if a.SearchOpen || a.Query != "" || len(a.Visible) != 3 {
		t.Fatalf("Esc did not reset search: open=%v query=%q visible=%v", a.SearchOpen, a.Query, a.Visible)
	}
}

// 9. Empty states provide actionable CTAs for opening folder, clearing search, clearing filter, and enabling recursion.
func TestEmptyStatesActionableCTAs(t *testing.T) {
	// No root -> 打开目录…
	a := polishTestApp(t, 0, 0)
	a.Root = ""
	te := ui.NewTester(a.view, 900, 600)
	te.Frame()
	if !te.HasText("打开目录…") {
		t.Fatalf("emptyView missing 打开目录…: %v", te.Texts())
	}

	// Active search with no matches -> 清除搜索
	a = polishTestApp(t, 0, 2)
	a.SearchOpen = true
	a.Query = "nope"
	a.applyFilter()
	te = ui.NewTester(a.view, 900, 600)
	te.Frame()
	if err := te.Click("清除搜索"); err != nil {
		t.Fatalf("empty search CTA missing/unclickable: %v (%v)", err, te.Texts())
	}
	te.Frame()
	if a.SearchOpen || a.Query != "" || len(a.Visible) != 2 {
		t.Fatalf("清除搜索 failed: open=%v query=%q visible=%d", a.SearchOpen, a.Query, len(a.Visible))
	}

	// Active media filter with no matches -> 显示全部文件
	a = polishTestApp(t, 0, 1)
	a.Items[0] = mediaItem{Path: "/probe/readme.txt", Name: "readme.txt", Ext: "txt"}
	a.FilterMode = filterImages
	a.applyFilter()
	te = ui.NewTester(a.view, 900, 600)
	te.Frame()
	if err := te.Click("显示全部文件"); err != nil {
		t.Fatalf("empty filter CTA missing/unclickable: %v (%v)", err, te.Texts())
	}
	te.Frame()
	if a.FilterMode != filterAll || len(a.Visible) != 1 {
		t.Fatalf("显示全部文件 failed: filter=%d visible=%d", a.FilterMode, len(a.Visible))
	}

	// Empty directory with recursion off -> 启用递归浏览
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "pic.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a = polishTestApp(t, 0, 0)
	a.Root = dir
	te = ui.NewTester(a.view, 900, 600)
	te.Frame()
	if err := te.Click("启用递归浏览"); err != nil {
		t.Fatalf("empty folder recursion CTA missing: %v (%v)", err, te.Texts())
	}
	if !a.Recursive {
		t.Fatal("启用递归浏览 did not enable Recursive")
	}
}

// 10. Batch rename dialog keeps Execute visible with 50 items, and infoOverlay clears tag dots & formats bytes.
func TestBatchDialogScrollsAndInfoOverlayLayout(t *testing.T) {
	a := polishTestApp(t, 0, 50)
	a.Selected.Clear()
	for i := range a.Items {
		a.Selected.Add(a.Items[i].Path)
	}
	a.startBatchRename()
	a.Batch.Rule.Prefix = "new_"
	a.updateBatchPreview()
	te := ui.NewTester(a.view, 900, 650)
	te.Frame()
	r, ok := te.Find("执行")
	if !ok {
		t.Fatalf("batch Execute button missing: %v", te.Texts())
	}
	if r.Y+r.H > 650 {
		t.Fatalf("batch Execute button pushed off screen: %+v", r)
	}
	if !te.HasText("共 50 项，将修改 50 项") {
		t.Fatalf("batch preview count missing: %v", te.Texts())
	}

	// infoOverlay clears tag dots (top >= 30), formats bytes as KB/B, and closes on click.
	a = polishTestApp(t, 0, 1)
	a.Items[0].Size = 2048
	a.Items[0].Tags = []string{"红色"}
	a.openLargeImage(0)
	a.Info.File = true
	te = ui.NewTester(a.view, 1100, 760)
	te.Frame()
	hdr, ok := te.Find("媒体信息")
	if !ok {
		t.Fatalf("infoOverlay header missing: %v", te.Texts())
	}
	if hdr.Y < 30 {
		t.Fatalf("infoOverlay Y=%v overlaps top-right tag dots", hdr.Y)
	}
	if !te.HasText("大小: 2.0 KB") {
		t.Fatalf("infoOverlay did not use formatBytes: %v", te.Texts())
	}
	if err := te.Click("关闭信息"); err != nil {
		t.Fatalf("close infoOverlay failed: %v", err)
	}
	te.Frame()
	if a.Info.File || te.HasText("媒体信息") {
		t.Fatal("infoOverlay did not close")
	}
}

// 11. Narrow video bar hides volume slider while keeping timeline usable; editor highlights active aspect & clamps bottom crop badge.
func TestNarrowVideoBarAndEditorCropPolish(t *testing.T) {
	backend := &fakePlaybackBackend{path: "/tmp/movie.mp4", duration: 120, position: 15, playing: true}
	a := fakeVideoState(backend)
	te := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() { a.videoControlsBar(c, &a.Items[1]) })
	}, 480, 400)
	te.Frame()
	if _, ok := te.Access("音量"); ok {
		t.Fatal("volume slider should hide at 480pt window width")
	}
	prog, ok := te.Find("播放进度")
	if !ok || prog.W < 80 {
		t.Fatalf("timeline degraded too small at 480pt: %+v (ok=%v)", prog, ok)
	}

	// Editor: active aspect highlights and bottom crop badge stays inside ViewH.
	a.Editor = editorState{
		Open:     true,
		Mode:     "crop",
		Path:     "/tmp/movie.mp4",
		Duration: 10,
		FrameW:   1920,
		FrameH:   1080,
		Segments: []editSegment{{End: 10}},
		CropOn:   true,
		Crop:     normRect{X: 0, Y: 0, W: 1, H: 1},
		Aspect:   "16:9",
	}
	te = ui.NewTester(a.editorView, 900, 600)
	te.Frame()
	te.Frame()
	badge, ok := te.Find("1920×1080")
	if !ok {
		t.Fatalf("editor crop badge missing: %v", te.Texts())
	}
	if badge.Y+badge.H > a.Editor.ViewH {
		t.Fatalf("crop dimension badge clipped below stage: badge=%+v ViewH=%v", badge, a.Editor.ViewH)
	}
	if err := te.Click("1:1"); err != nil {
		t.Fatal(err)
	}
	if a.Editor.Aspect != "1:1" {
		t.Fatalf("Editor.Aspect = %q, want 1:1", a.Editor.Aspect)
	}
}
