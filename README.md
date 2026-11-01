# Lumiwerk

A native media browser built with Go and MyGo v0.7.3.

Lumiwerk = French *lumière* (light) + German *Werk* (work) — "light work," a tool for browsing media art.

## Run

```sh
cd /Users/mcx/Documents/OpenSpring/Lumiwerk
go run .
```

Pick a folder from the menu (File → Open Folder…), drag a folder onto the window, or press Cmd+O.

## Desktop releases

Pushing a `v*` tag runs tests and builds macOS Universal, Windows x64/ARM64,
and Linux x64/ARM64 packages. All packages are attached to one GitHub Release
with SHA-256 checksums. The tag must match `version` in `mygo.json`:

```sh
git tag v0.2.0
git push origin v0.2.0
```

The **Desktop Release** workflow can also be run manually to produce build
artifacts without creating a release. macOS packages use ad-hoc signatures;
Windows installers are unsigned. Install FFmpeg separately for video playback
and video tools.

## Logging

Lumiwerk writes a rotating log to `~/Library/Logs/Lumiwerk/lumiwerk.log`. When
the file passes 2 MiB it rolls over to `lumiwerk.log.1` … `.3`, so a crash in
the wild leaves evidence instead of vanishing with the window.

Recorded events include startup and shutdown, preferences load/save failures,
thumbnail and video decode failures, file-operation failures, and recoverable
panics (with a full stack) from the render loop and background jobs. A panic in
a background job is logged and swallowed rather than taking the process down.

Set `LUMIWERK_LOG` to `debug`, `info` (default), `warn`, `error`, or `off` to
raise or silence the level. When the app is launched from a terminal the same
lines are mirrored to stderr. `LogPath()` returns the active file so callers
can surface it.

Open **Display -> Run Diagnostics** (Cmd+Shift+D) to inspect recent warnings
and errors, panic counts, runtime metrics, copy a diagnostic report, or open
the current log. The same panel is available in Settings -> Advanced.

## Development

See `AGENTS.md` and `docs/development.md` for the required verification gate,
shared MyGo UI shell, selection-key contract, background-task boundaries, and
crash reproduction workflow.
