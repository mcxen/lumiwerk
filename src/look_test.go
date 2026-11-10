package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestWashOfFallsBackToPaper(t *testing.T) {
	for _, name := range washNames {
		if _, ok := washTable[name]; !ok {
			t.Fatalf("wash %q listed but not defined", name)
		}
	}
	if len(washNames) != len(washLabels) {
		t.Fatalf("%d names, %d labels", len(washNames), len(washLabels))
	}
	if got := washOf("nonsense"); len(got.light) != len(washTable[defaultWash].light) {
		t.Fatalf("unknown wash did not fall back to %s", defaultWash)
	}
	base, glow := washGradients("mist", true)
	if len(base.Stops) < 2 || len(glow.Stops) != 2 {
		t.Fatalf("gradients: %d base stops, %d glow stops", len(base.Stops), len(glow.Stops))
	}
}

func TestControlsWantedFollowsTheDesktopAndTheEnvironment(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	none, some := mygo.WindowButtons{}, mygo.WindowButtons{End: []string{"close"}}
	cases := []struct {
		env     string
		buttons mygo.WindowButtons
		want    bool
	}{
		{"", some, true}, {"", none, false}, {"1", none, true}, {"off", some, false}, {" Show ", none, true},
	}
	for _, c := range cases {
		if got := controlsWanted(env(c.env), c.buttons); got != c.want {
			t.Errorf("env %q buttons %v: got %v, want %v", c.env, c.buttons, got, c.want)
		}
	}
}

// The card layout must keep the browser usable in both appearances, with
// the sidebar shown and hidden, and the window controls must not break a
// window that does not exist yet.
func TestCardLayoutRendersAndStaysInteractive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := showControls
	showControls = true
	defer func() { showControls = old }()
	for _, wash := range washNames {
		a := &appState{Root: "", SidebarShown: true, SidebarWidth: 220, Thumb: 128, Chosen: -1, LargeIndex: -1,
			Profiles: prefsFile{View: defaultViewPreferences()}}
		a.Profiles.View.Wash = wash
		tt := ui.NewTester(a.view, 1100, 700)
		tt.Frame()
		tt.Frame()
		if !tt.HasText("收藏夹") {
			t.Fatalf("wash %s: sidebar missing, texts %v", wash, tt.Texts())
		}
		a.SidebarShown = false
		tt.Frame()
		if tt.HasText("收藏夹") {
			t.Fatalf("wash %s: sidebar still shown after hiding", wash)
		}
	}
}
