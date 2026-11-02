package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// diagnosticsState owns the presentation state only. Event storage lives in
// diagnostics.go's package recorder so it also covers startup and tests.
type diagnosticsState struct {
	Window *mygo.Window
	Text   string
}

type diagnosticEvent struct {
	Time    time.Time
	Level   logLevel
	Scope   string
	Message string
}

type diagnosticCounters struct {
	Debug, Info, Warn, Error, Panics int
}

const diagnosticEventLimit = 200

var eventLog = struct {
	sync.RWMutex
	events   []diagnosticEvent
	counters diagnosticCounters
}{}

func recordDiagnostic(level logLevel, scope, message string) {
	eventLog.Lock()
	defer eventLog.Unlock()
	eventLog.events = append(eventLog.events, diagnosticEvent{
		Time: time.Now(), Level: level, Scope: scope, Message: message,
	})
	if len(eventLog.events) > diagnosticEventLimit {
		eventLog.events = append([]diagnosticEvent(nil), eventLog.events[len(eventLog.events)-diagnosticEventLimit:]...)
	}
	switch level {
	case levelDebug:
		eventLog.counters.Debug++
	case levelInfo:
		eventLog.counters.Info++
	case levelWarn:
		eventLog.counters.Warn++
	case levelError:
		eventLog.counters.Error++
	}
}

func recordPanic() {
	eventLog.Lock()
	eventLog.counters.Panics++
	eventLog.Unlock()
}

func diagnosticSnapshot() ([]diagnosticEvent, diagnosticCounters) {
	eventLog.RLock()
	defer eventLog.RUnlock()
	return append([]diagnosticEvent(nil), eventLog.events...), eventLog.counters
}

func diagnosticReport(a *appState) string {
	events, counts := diagnosticSnapshot()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s diagnostics\n", appName, appVersion)
	fmt.Fprintf(&b, "time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "runtime: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "log: %s\n", LogPath())
	fmt.Fprintf(&b, "preferences: %s\n", prefsPath())
	fmt.Fprintf(&b, "counts: debug=%d info=%d warn=%d error=%d panic=%d\n",
		counts.Debug, counts.Info, counts.Warn, counts.Error, counts.Panics)
	fmt.Fprintf(&b, "goroutines: %d\n", runtime.NumGoroutine())
	if a != nil {
		fmt.Fprintf(&b, "root: %s\nlayout: %d largeView: %t\nitems: %d visible: %d selected: %d\n",
			a.Root, a.LayoutMode, a.InLargeView, len(a.Items), len(a.Visible), a.Selected.Len())
		fmt.Fprintf(&b, "thumbnailCache: %d entries, %.1f/%.1f MiB, workers: %d\n", thumbLRU.Len(), float64(thumbLRU.Size())/(1024*1024), float64(thumbLRU.limit)/(1024*1024), cap(thumbGate))
	}
	b.WriteString("\nrecent events (newest first):\n")
	shown := 0
	for i := len(events) - 1; i >= 0 && shown < 60; i-- {
		e := events[i]
		if e.Level < levelWarn {
			continue
		}
		message := strings.ReplaceAll(e.Message, "\n", " | ")
		if len(message) > 1600 {
			message = message[:1600] + "..."
		}
		fmt.Fprintf(&b, "%s %-5s %-20s %s\n", e.Time.Format("15:04:05.000"), levelName(e.Level), e.Scope, message)
		shown++
	}
	if shown == 0 {
		b.WriteString("(no warnings or errors in this run)\n")
	}
	return b.String()
}

func (a *appState) showDiagnostics() {
	if a.Diagnostics.Window != nil {
		a.Diagnostics.Window.Focus()
		a.Diagnostics.Text = diagnosticReport(a)
		return
	}
	a.Diagnostics.Text = diagnosticReport(a)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: appName + " 诊断", Width: 900, Height: 700,
		MinWidth: 680, MinHeight: 480, Content: ui.View(a.diagnosticsView),
	})
	a.Diagnostics.Window = w
	w.OnClosed(func() { a.Diagnostics.Window = nil })
}

func (a *appState) diagnosticsView(c *ui.Context) {
	safeView("diagnostics", c, func() {
		t := c.Theme()
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).FillWidth().Padding(10, 14).Gap(8).AlignItems(ui.Center).Background(t.Surface).Children(func() {
				ui.Text(c, "运行诊断").FontSize(16).Bold().Grow(1)
				if ui.Button(c.Key("diagnostics-refresh"), "刷新").Clicked() {
					a.Diagnostics.Text = diagnosticReport(a)
				}
				if ui.Button(c.Key("diagnostics-copy"), "复制诊断").Clicked() {
					text := diagnosticReport(a)
					a.Diagnostics.Text = text
					if err := mygo.Clipboard.Write(transfer.TextData(text)); err != nil {
						logErrorf("diagnostics: clipboard write failed: %v", err)
					}
				}
				if ui.Button(c.Key("diagnostics-open-log"), "打开日志").Clicked() {
					if path := LogPath(); path != "" {
						if err := mygo.Shell.OpenPath(path); err != nil {
							logErrorf("diagnostics: open log failed: %v", err)
						}
					}
				}
			})
			ui.Divider(c)
			ui.TextArea(c.Key("diagnostics-report"), &a.Diagnostics.Text).ReadOnly(true).
				Font("monospace").FontSize(12).Fill().Padding(12)
		})
	})
}

func resetDiagnosticsForTest() {
	eventLog.Lock()
	eventLog.events = nil
	eventLog.counters = diagnosticCounters{}
	eventLog.Unlock()
}
