# Development, UI Shell and Observability

## Architecture

Lumiwerk uses MyGo's native UI, not a web view. The main code is deliberately
split by responsibility:

- `main.go`: application lifecycle and native open-file events.
- `shell.go`: shared browser chrome, selection wiring, settings layout and
  recoverable view boundaries.
- `aligned.go`: application state, browser behavior and feature composition.
- `layouts.go`, `largeview.go`, `video.go`: media presentation and playback.
- `actions.go`, `fileops.go`, `undo.go`: file operations and their inverses.
- `logging.go`, `diagnostics.go`: persistent logs, in-memory events, runtime
  metrics and the diagnostic report UI.
- `prefs.go`: normalized, atomic preference persistence with one backup.

`appState` remains the feature model. The shared shell does not introduce a
second source of truth or duplicate media selection state.

## Unified Browser Shell

`rootView` handles input and delegates composition to `browserLayout`.
`browserLayout` owns the optional split sidebar. `browserShell` owns the main
body and modal overlays in a fixed order. New browser overlays belong there;
new media layouts belong in `mainView`'s layout switch.

Both main and settings windows call `safeView`, which records a panic and its
stack. Background jobs call `safeGo`. These boundaries are for diagnosis and
containment, not for hiding invalid state: every recovered panic must also have
a regression test and a code fix.

## Selection and Keys

MyGo defaults an unconfigured list/grid row key to its integer index. Lumiwerk
uses `ui.Selection[string]` keyed by canonical media paths. A grid/list with a
selection therefore **must** use `appState.mediaListState()`. This method wires:

- `Key = itemKey`, always returning a string;
- `Label = itemLabel`;
- `Selected = &a.Chosen`;
- `Selection = &a.Selected`.

`itemKey` returns `!missing:<index>` for a transient stale index. This keeps the
key type stable during background scans. Never fall back to a raw integer.

Use stable, unique `c.Key` values for controls that retain focus, editing or
other row state. Dynamic rows use the item identity, not their current index.
For stateful widgets (`TextArea`, `TextInput`, `SearchField`, `Slider`, lists
and similar), put the key on the context **before** construction:
`ui.TextArea(c.Key("report"), &text)`. Calling `.Key` after construction
panics once MyGo has initialized the widget's state.

## Settings Composition

`settingsSection` provides the shared label/content layout used by action and
advanced settings. Use `c.Key` for stable control identities. Add a new pane to
the single switch in `settingsBody`; do not copy the tab strip or window
lifecycle. Reopen an existing settings/diagnostics window with `Focus` instead
of creating duplicates.

## Logging and Diagnostics

The log writer emits a timestamp, level, source location and message. Events
also enter a bounded, mutex-protected in-memory buffer even when their level is
below the file threshold. The current buffer holds 200 events; files rotate at
2 MiB with three historical generations.

Use a stable scope and an actionable message:

```go
logErrorf("video.decode: path=%s err=%v", path, err)
safeGo("video.framePump", func() { decoder.runPump(frames) })
```

Do not log ZIP passwords, tokens, full clipboard data or large buffers. Media
paths are needed to reproduce local failures and are included only in local
logs/reports. A copied diagnostic report contains recent warnings/errors,
panic count, Go/macOS architecture, log and preference paths, current directory,
layout, media counts, selection count, goroutine count and thumbnail capacity.

Diagnostics is available from Display -> Run Diagnostics, Cmd+Shift+D, and
Settings -> Advanced -> Open Diagnostics. Refresh takes a new snapshot. Copy
Diagnostics writes plain text through MyGo's clipboard API. Open Log uses
MyGo's shell API; it does not invoke a second shell by hand.

## Recovery and Verification

For every crash fix:

1. Preserve the panic message and the first application/framework frames.
2. Add a focused state/contract test and, for UI, a `ui.NewTester` rendering or
   interaction test that would fail on the old implementation.
3. Fix the shared contract when more than one view has the same risk.
4. Run the complete build/vet/test gate with `-count=1`.
5. Build the signed `.app`, launch it normally and exercise the affected window.
6. Confirm no new panic appears in the log and that a clean shutdown is logged.

Headless rendering and build success are supporting evidence, not substitutes
for real-window verification of a crash fix.

## Preferences and Test Isolation

`loadPrefs` always normalizes defaults and nine profiles. `savePrefs` writes a
temporary file, preserves the previous generation as `prefs.json.bak`, then
renames the new file atomically. Any test that directly or indirectly reaches
these functions must set a temporary `HOME`. This includes tests invoking
methods such as `commitAutoConfig`, not only tests calling `savePrefs` directly.
