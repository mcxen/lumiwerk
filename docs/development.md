# Development, UI Shell and Observability

## Architecture

Lumiwerk uses MyGo's native UI, not a web view. The main code is deliberately
split by responsibility:

- `main.go`: application lifecycle and native open-file events.
- `shell.go`: shared browser chrome, selection wiring, settings layout and
  recoverable view boundaries.
- `aligned.go`: application state, browser behavior and feature composition.
- `layouts.go`, `largeview.go`, `video.go`: media presentation and playback.
- `playback.go`, `playback_mpv.go`: the Go playback kernel, its libmpv
  backend (via `gen2brain/go-mpv`, compiled only with `-tags mpvbackend`
  because the library loads in package init; release builds set
  `GOFLAGS=-tags=mpvbackend`) and the FFmpeg picture/sound fallback. The
  native controller uses `playbackSession`, never a second view-owned
  playhead. FFmpeg is used when libmpv is unavailable.
  The libmpv backend never blocks the main thread: mpv's update callback
  wakes a render thread that renders into pooled tight `*image.RGBA` frames
  (`playback_frames.go`, taken by `ui.Bitmap.Update` without a copy), an
  event thread caches observed properties (time-pos, duration, cache, size,
  tracks, eof) for the getters, and seeks/volume/speed are asynchronous. New
  frames redraw the window through `SetFrameNotify`; the view uploads each
  frame once. Do not add synchronous `GetProperty` or render calls to the
  getters `Snapshot` uses.
- Views must not touch the file system per frame. Use `folderPresent` for
  "does this folder still exist" checks in rows drawn every frame.
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

Lumiwerk keeps media selection in `appState` (`a.Chosen`, `a.Selected`, a
`ui.Selection[string]` keyed by canonical media paths) and draws it itself as
a quiet neutral fill (`selectionChrome`): the widgets' own selection chrome is
the big accent box and stays unwired. Keyboard navigation lives in
`browseKeys` (src/keys.go), the same for every layout — arrows move the chosen
item by its visual position, Shift extends from the pivot, Home/End jump.

Grid and list rows are still keyed with strings through
`appState.mediaListState()`, so element identity follows the media path across
re-sorts and background rescans:

- `Key = itemKey`, always returning a string;
- `Label = itemLabel`.

`itemKey` returns `!missing:<index>` for a transient stale index. This keeps the
key type stable during background scans. Never fall back to a raw integer, and
do not wire the widgets' `Selected`/`Selection` back up.

Use stable, unique `c.Key` values for controls that retain focus, editing or
other row state. Dynamic rows use the item identity, not their current index.
For stateful widgets (`TextArea`, `TextInput`, `SearchField`, `Slider`, lists
and similar), put the key on the context **before** construction:
`ui.TextArea(c.Key("report"), &text)`. Calling `.Key` after construction
panics once MyGo has initialized the widget's state.

## Marquee Selection and Context Menus

`src/marquee.go` is the rubber-band selection. The main column's input handler
(`browseInput` → `handleGesture` → `marqueeInput`) takes a left press only on
blank space of the browse area; a press on a card falls through to the card's
click, file drag and Finder-style chrome. Cards report their layout bounds each
frame (`noteCardRect`, content coordinates, kept while the layout key is
unchanged), which is how the band finds what it touches in the grid, the
waterfall, the justified rows and the list (list rects live one frame: its
table scrolls on its own state). The band is the `marquee-band` overlay, and
`marqueeAutoScroll` runs before the layout builds, because a scroll offset
written after the container was built is lost. The Tester's pointer events
carry no modifier keys: feed `a.marqueeInput` events with `Mods` (see
`dragBand` in `marquee_test.go`).

The right button is not taken by `browseInput`. A handler that takes a press
owns it, and context menus open on that press on macOS, so taking it left
every browse menu dead. The right-drag gesture reads the press and release
through `ObserveInput` instead (`observeGesture`).

`src/menus.go` is the only place menu items are written. `menuEntries` lists
every entry once with its surface, submenu, `Show`/`Off` predicates over a
`menuScope` (counts of folders, pictures, videos, archives among the targets).
`modelFor` filters it and tidies separators; `drawMenu` draws it. Add a command
there, not in a menu function. Targets come from `operationPaths`, which
collapses a folder and the files a recursive listing expanded from it into one
target (`collapseNested`); compress takes files of several folders (stored by
their path below the common parent).

## Settings Composition

The settings window is a slim nav plus pages made of `setGroup` cards (see
`settings_form.go`). A row is `setRow`/`setRowHint` with the label on the left
and the control on the right; use the typed helpers (`setToggle`, `setChoice`,
`setSelect`, `setText`, `setSlider`, `setButton`, `setChecks`) so spacing and
hairlines match. Use `c.Key` for stable control identities. Add a page to
`settingsPages` and the single switch in `settingsBody`.

Settings only holds preferences that exist nowhere else. Layout, thumbnail
size, sorting, hidden files and recursion are changed live from the toolbar
and the View menu, so they are not repeated here. Configuration slots are
Alt+1…9 (apply) and Cmd+Alt+1…9 (save).

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

## Platform support notes

- File tags and ★ ratings: macOS writes the Finder tag xattr on the file. Windows and Linux use the path-keyed store in `src/tagstore.go` (`<config dir>/lumiwerk/filetags.json`); in-app moves and renames migrate entries, changes made outside the app are not followed.
- Trash: macOS uses Finder, Windows the Recycle Bin, Linux `gio trash` or the XDG trash (`src/xdgtrash.go`, with `.trashinfo`).
- "Open with": macOS LaunchServices, Windows the Explorer `OpenWithList` registry key. Not available on Linux. The share menu is macOS only.

## ZIP password book

`prefsFile.ZipPasswords` holds extra passwords next to `ZipPassword` (the
default, also used for encrypted compression). `startArchive` hands the merged,
de-duplicated list (`zipPasswordCandidates`) to every extraction;
`extractWithPasswords` tries no password, the typed one, then each saved one,
and only returns `errNeedZipPassword` (the prompt) when none fit. Passwords are
never logged. Settings pages draw a small schematic of each effect from
`settings_figures.go`; add one with `setFigure` when a setting changes
something the user cannot see in the row itself.

## Shared MyGo components

Build screens from these helpers instead of new widget scaffolding:
`controlbar.go` (floating bar shell, icon buttons, chips), `settings_form.go`
(`setGroup`, `setRow`, `setSelect`, `setSegmented`, `setToggle`, `setText`,
`setBook`), `settings_controls.go` (`settingsPopup`, the one dropdown) and
`settings_figures.go` (`setFigure`). Rules learned the hard way:

- A mygo part such as `sel.Item(v)` takes its content from `.Children(...)`.
  Putting `ui.Icon`/`ui.Text` after it, un-nested, stacks them in a column and
  inflates the row. Give every row `.Row()` and an explicit `Children`.
- A toolbar overflows from its last control, so the controls that must stay
  visible (the display cluster) go last and the optional ones fold into the
  显示与浏览 menu via `a.toolbarCompact`. Never show the same command on the
  toolbar and in that menu at once.
- Each shared widget gets a `ui.NewTester` test of its geometry, not only its
  text (see `settings_controls_test.go`).
