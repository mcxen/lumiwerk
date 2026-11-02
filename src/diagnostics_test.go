package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDiagnosticReportIncludesFailuresAndRuntimeState(t *testing.T) {
	resetDiagnosticsForTest()
	t.Setenv("HOME", t.TempDir())
	a := &appState{Root: "/tmp/photos", LayoutMode: 1, Chosen: 0}
	a.Items = []mediaItem{{Path: "/tmp/photos/a.jpg"}}
	a.Visible = []int{0}
	a.Selected.Add("/tmp/photos/a.jpg")

	logWarnf("diagnostic warning")
	logErrorf("diagnostic error")
	func() {
		defer logRecover("diagnostic-test")
		panic("expected panic")
	}()

	report := diagnosticReport(a)
	for _, part := range []string{
		"warn=1 error=2 panic=1", "diagnostic warning", "diagnostic error",
		"panic in diagnostic-test: expected panic", "root: /tmp/photos",
		"layout: 1 largeView: false", "visible: 1 selected: 1",
	} {
		if !strings.Contains(report, part) {
			t.Fatalf("diagnostic report missing %q:\n%s", part, report)
		}
	}
}

func TestDiagnosticsViewRendersReport(t *testing.T) {
	resetDiagnosticsForTest()
	t.Setenv("HOME", t.TempDir())
	a := &appState{}
	a.Diagnostics.Text = diagnosticReport(a)
	te := ui.NewTester(a.diagnosticsView, 900, 700)
	te.Frame()
	te.Frame()
	a.Diagnostics.Text = diagnosticReport(a)
	te.Frame()
	if !te.HasText("运行诊断") || !te.HasText("复制诊断") || !te.HasText("打开日志") {
		t.Fatalf("diagnostic actions missing: %q", te.Texts())
	}
}

func TestMediaShellWiresListAndGridSelection(t *testing.T) {
	a := &appState{Chosen: 0}
	a.Items = []mediaItem{{Path: "/tmp/photo.jpg", Name: "photo.jpg"}}
	a.Visible = []int{0}
	state := a.mediaListState()
	for name, key := range map[string]any{
		"grid": state.Grid.Key(0), "list": state.List.Key(0),
	} {
		if key != "/tmp/photo.jpg" {
			t.Fatalf("%s key = %#v", name, key)
		}
	}
	if state.List.Selected != &a.Chosen || state.Grid.Selected != &a.Chosen {
		t.Fatal("list and grid do not share the chosen row")
	}
}
