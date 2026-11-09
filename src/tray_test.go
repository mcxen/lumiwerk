package main

import (
	"bytes"
	"image/png"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func trayTestApp(t *testing.T) *appState {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	return &appState{
		Chosen:     -1,
		LargeIndex: -1,
		Root:       root,
		Items: []mediaItem{
			{Path: root + "/a.jpg", Name: "a.jpg", Ext: "jpg"},
			{Path: root + "/b.jpg", Name: "b.jpg", Ext: "jpg"},
			{Path: root + "/notes.txt", Name: "notes.txt", Ext: "txt"},
		},
		Visible: []int{0, 1, 2},
		Tabs:    tabState{Tabs: []tab{{Root: root}, {Root: t.TempDir()}}, Active: 0},
	}
}

// The status panel names the open folder and how much media it holds,
// and offers the actions that have to work while the window is hidden.
func TestTrayPanelShowsFolderAndActions(t *testing.T) {
	a := trayTestApp(t)
	te := ui.NewTester(a.trayPanel, trayPanelW, trayPanelH)
	te.Frame()
	for _, want := range []string{"Lumiwerk", pathName(a.Root), a.Root, "2 个媒体", "2 个标签页", "主窗口已收起",
		"打开文件夹…", "偏好设置", "诊断", "退出", "最近", "收藏夹", "标签页"} {
		if !te.HasText(want) {
			t.Fatalf("panel missing %q in %q", want, te.Texts())
		}
	}
}

func TestTrayPanelEmptyFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1}
	te := ui.NewTester(a.trayPanel, trayPanelW, trayPanelH)
	te.Frame()
	for _, want := range []string{"未打开目录", "尚未打开目录", "还没有记录", "打开文件夹…"} {
		if !te.HasText(want) {
			t.Fatalf("empty panel missing %q in %q", want, te.Texts())
		}
	}
}

func TestTrayPanelListsRecentFavoritesAndTabs(t *testing.T) {
	a := trayTestApp(t)
	recent, fav := t.TempDir(), t.TempDir()
	a.Recent = []string{recent, "/nonexistent/gone"}
	a.Profiles.Favorites = []string{fav}
	a.Profiles.TrayTab = trayTabRecent
	te := ui.NewTester(a.trayPanel, trayPanelW, trayPanelH)
	te.Frame()
	if !te.HasText(pathName(recent)) || te.HasText("gone") {
		t.Fatalf("recent tab: %q", te.Texts())
	}
	if err := te.Click("收藏夹"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if !te.HasText(pathName(fav)) || te.HasText(pathName(recent)) {
		t.Fatalf("favorites tab: %q", te.Texts())
	}
	if err := te.Click("标签页"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if !te.HasText("当前") {
		t.Fatalf("tabs tab: %q", te.Texts())
	}
	if a.Profiles.TrayTab != trayTabTabs {
		t.Fatalf("tab not remembered: %q", a.Profiles.TrayTab)
	}
}

func TestTrayPanelShowsTaskProgress(t *testing.T) {
	a := trayTestApp(t)
	a.work.label, a.work.done, a.work.total = "压缩", 3, 10
	a.work.since = time.Now().Add(-time.Minute)
	te := ui.NewTester(a.trayPanel, trayPanelW, trayPanelH)
	te.Frame()
	if !te.HasText("压缩 3/10") {
		t.Fatalf("task missing: %q", te.Texts())
	}
	st := a.trayStatusNow()
	if st.Icon() != trayIconBusy || st.Title() != "30%" {
		t.Fatalf("status = %+v icon %v title %q", st, st.Icon(), st.Title())
	}
}

func TestTrayStatusHidesFreshTask(t *testing.T) {
	a := trayTestApp(t)
	a.work.label, a.work.since = "旋转", time.Now()
	if a.trayStatusNow().HasTask() {
		t.Fatal("a task younger than workShowDelay must not flash in the tray")
	}
}

func TestTrayStatusTitle(t *testing.T) {
	if got := (trayStatus{Media: "m.mp4", Playing: true, Position: 75}).Title(); got != "▶ 1:15" && got != "▶ 01:15" {
		t.Fatalf("playing title %q", got)
	}
	if got := (trayStatus{Task: "导出"}).Title(); got != "导出" {
		t.Fatalf("indeterminate title %q", got)
	}
	if got := (trayStatus{}).Title(); got != "" {
		t.Fatalf("idle title %q", got)
	}
}

func TestTrayIconVariantsDrawAMark(t *testing.T) {
	base, err := loadTrayIcon()
	if err != nil {
		t.Skip("tray icon unavailable:", err)
	}
	idle := trayIconVariant(base, trayIconIdle)
	if !bytes.Equal(idle, base) {
		t.Fatal("idle icon must be untouched")
	}
	for _, kind := range []trayIconKind{trayIconBusy, trayIconPlaying} {
		out := trayIconVariant(base, kind)
		if bytes.Equal(out, base) {
			t.Fatalf("kind %d drew nothing", kind)
		}
		img, err := png.Decode(bytes.NewReader(out))
		if err != nil || img.Bounds().Dx() != 32 {
			t.Fatalf("kind %d: %v %v", kind, err, img.Bounds())
		}
	}
}

func TestTrayDroppedFolder(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.jpg"
	if got := trayDroppedFolder([]string{file, dir}); got != dir {
		t.Fatalf("folder wins: %q", got)
	}
	if got := trayDroppedFolder([]string{file}); got != dir {
		t.Fatalf("file's parent: %q", got)
	}
	if got := trayDroppedFolder([]string{"/nonexistent/x.jpg"}); got != "" {
		t.Fatalf("missing: %q", got)
	}
}

func TestTrayPrefsDefaults(t *testing.T) {
	var p prefsFile
	normalizePrefs(&p)
	if p.TrayTab != trayTabRecent || p.trayRecentCount() != trayDefaultRecent || p.trayHotkey() != trayDefaultHotkey {
		t.Fatalf("defaults: %+v", p)
	}
	p.TrayHotkeyOff = true
	p.TrayRecent = 99
	normalizePrefs(&p)
	if p.trayHotkey() != "" || p.trayRecentCount() != 20 {
		t.Fatalf("overrides: %+v", p)
	}
}

func TestTrayMenuStructure(t *testing.T) {
	a := trayTestApp(t)
	a.Recent = []string{t.TempDir()}
	a.work.label, a.work.total, a.work.since = "压缩", 4, time.Now().Add(-time.Minute)
	labels := map[string]bool{}
	for _, it := range a.buildTrayMenu().Items() {
		labels[it.Label] = true
	}
	for _, want := range []string{"显示 Lumiwerk", "打开文件夹…", "最近打开", "收藏夹", "任务：压缩 0%", "偏好设置…", "运行诊断",
		"关闭窗口时隐藏到托盘", "登录时启动", "退出 Lumiwerk"} {
		if !labels[want] {
			t.Fatalf("menu missing %q in %v", want, labels)
		}
	}
	var closeItem *mygo.MenuItem
	for _, it := range a.buildTrayMenu().Items() {
		if it.Label == "关闭窗口时隐藏到托盘" {
			closeItem = it
		}
	}
	if closeItem == nil || !closeItem.Checked {
		t.Fatal("close-to-tray is the default and must be checked")
	}
	a.Profiles.CloseAction = closeQuit
	for _, it := range a.buildTrayMenu().Items() {
		if it.Label == "关闭窗口时隐藏到托盘" && it.Checked {
			t.Fatal("quit action must uncheck close-to-tray")
		}
	}
}
