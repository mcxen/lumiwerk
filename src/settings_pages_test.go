package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func newSettingsTestState(t *testing.T) *appState {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	a := &appState{Chosen: -1, LargeIndex: -1, Thumb: 192}
	a.Profiles = loadPrefs()
	a.openSettings()
	return a
}

// Every page renders and the nav reaches it; nothing that the toolbar or the
// View menu already controls is repeated in settings.
func TestSettingsPagesRenderAndNavigate(t *testing.T) {
	a := newSettingsTestState(t)
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 780, 900)
	te.Frame()
	for _, p := range settingsPages {
		if err := te.Click(p.title); err != nil {
			t.Fatalf("nav %q: %v", p.title, err)
		}
		te.Frame()
		te.Frame()
		if a.settingsPageID() != p.id {
			t.Fatalf("clicked %q, page = %q", p.title, a.settingsPageID())
		}
	}
	for _, gone := range []string{"默认布局", "默认缩略图", "默认排序依据", "配置槽", "媒体单元", "搜索设置"} {
		for _, p := range settingsPages {
			a.setPage(p.id)
			te.Frame()
			if te.HasText(gone) {
				t.Fatalf("%q should no longer be in settings (page %s)", gone, p.id)
			}
		}
	}
}

func TestSettingsBrowseControlsPersist(t *testing.T) {
	a := newSettingsTestState(t)
	a.setPage("browse")
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 780, 900)
	te.Frame()
	if err := te.Click("打开/关闭"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if !a.Profiles.EnterOpens || !loadPrefs().EnterOpens {
		t.Fatalf("enter opens not saved: %v", a.Profiles.EnterOpens)
	}
	if err := te.Click("循环"); err != nil {
		t.Fatal(err)
	}
	te.Frame()
	if got := loadPrefs().PlaybackEnd; got != playbackEndLoop {
		t.Fatalf("playback end = %q", got)
	}
}

func TestSettingsFilesPageTogglesZipDefault(t *testing.T) {
	a := newSettingsTestState(t)
	a.setPage("files")
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 780, 900)
	te.Frame()
	for _, want := range []string{"图片复制到", "视频复制到", "重命名规则", "默认密码", "默认加密"} {
		if !te.HasText(want) {
			t.Fatalf("files page missing %q: %v", want, te.Texts())
		}
	}
}

func TestSettingsFiguresAndPasswordBookPersist(t *testing.T) {
	a := newSettingsTestState(t)
	a.Settings.ZipBook = "alpha\nbeta"
	a.setPage("files")
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 780, 900)
	te.Frame()
	if !te.HasText("备用密码") || !te.HasText("依次试这 2 个密码") {
		t.Fatalf("password book missing: %v", te.Texts())
	}
	a.Settings.ZipBook = "alpha\nbeta\n\ngamma"
	te.Frame()
	if got := loadPrefs().ZipPasswords; strings.Join(got, ",") != "alpha,beta,gamma" {
		t.Fatalf("saved passwords = %v", got)
	}
	for page, caption := range map[string]string{
		"general": "跟随 macOS",
		"browse":  "播完停在最后一帧",
	} {
		a.setPage(page)
		te.Frame()
		if !te.HasText(caption) {
			t.Fatalf("%s figure caption %q missing: %v", page, caption, te.Texts())
		}
	}
}

func TestGeneralPageHasMenuBarGroup(t *testing.T) {
	a := newSettingsTestState(t)
	a.setPage("general")
	te := ui.NewTester(func(c *ui.Context) { a.settingsBody(c) }, 780, 1100)
	te.Frame()
	for _, want := range []string{"菜单栏", "在图标旁显示状态", "最近目录条数", "登录时启动"} {
		if !te.HasText(want) {
			t.Fatalf("general page missing %q: %v", want, te.Texts())
		}
	}
}
