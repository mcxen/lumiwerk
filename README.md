# Lumiwerk

A native macOS media browser built with Go and MyGo v0.7.3.

Lumiwerk = French *lumière* (light) + German *Werk* (work) — "light work," a tool for browsing media art.

## Run

```sh
cd /Users/mcx/Documents/OpenSpring/Lumiwerk
go run .
```

Pick a folder from the menu (File → Open Folder…), drag a folder onto the window, or press Cmd+O.

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
