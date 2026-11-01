# Lumiwerk Development Guide

## Scope

Lumiwerk is a native macOS media browser using Go and `github.com/egoist/mygo`
v0.7.3. Read `TASKS.md` for delivered capabilities and `docs/development.md`
for the shared UI and diagnostics contracts.

## Required Checks

Before committing a code change:

```sh
gofmt -w <changed-go-files>
go build ./...
go vet ./...
go test -count=1 ./...
```

For UI changes, add or update a `ui.NewTester` regression that exercises the
changed behavior. For a crash, first reproduce the original panic in a test,
then verify the fixed view renders and remains interactive. Do not treat a
successful build as UI acceptance.

Use `go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -skip-notarize -o dist`
for a local signed app bundle. Launch it through `open -a <absolute-app-path>`.

## Safety

- Tests that load or save preferences must isolate `HOME` with `t.Setenv`.
- Never start a production window from a unit test.
- Keep existing user work and unrelated changes. Do not reset or clean the tree.
- Do not expose passwords, clipboard contents, tokens, or unrelated personal
  paths in diagnostics. ZIP passwords must never be logged.

## MyGo Contracts

- `ui.Selection[string]` requires every grid/list row key to be a string. Use
  `appState.mediaListState()` for media grids and lists.
- Window creation, updates and focus belong on the MyGo main thread. Use
  `appState.update` from background work and tolerate a nil window in tests.
- New views use `safeView`; new background goroutines use `safeGo`.
- Objective-C autorelease pools must use `withAutoreleasePool`, which pins the
  goroutine to one OS thread.
- Reuse `browserShell`, `browserLayout`, `settingsSection` and stable element
  keys rather than copying window or selection scaffolding.
- Stateful widgets take `c.Key(...)` before construction; do not chain `.Key`
  on `TextArea`, `TextInput`, `SearchField`, lists or similar controls.

## Diagnostics

Logs rotate at `~/Library/Logs/Lumiwerk/lumiwerk.log`. Use
`LUMIWERK_LOG=debug|info|warn|error|off` to control file verbosity. In the app,
open Display -> Run Diagnostics or press Cmd+Shift+D. The diagnostics window
contains recent warnings/errors, panic counts, runtime state, and actions to
copy a report or open the log. See `docs/development.md` for event scopes and
the required fields in a bug report.
