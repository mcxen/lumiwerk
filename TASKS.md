# Lumiwerk — FlowVision Alignment Task List

Reference: FlowVision.app built from `/Users/mcx/Documents/OpenSpring/FlowVision`
Reference screenshots: `docs/ref-grid.png`, `docs/ref-large-image.png`

## Phase 1: Main Window Layout (grid view)
- [x] T1.1 Project scaffold (mygo v0.7.3, native UI, build passes)
- [x] T1.2 Three-pane split: sidebar | path-bar+grid | (no right pane in grid mode)
- [x] T1.3 Sidebar: directory tree (NSOutlineView equivalent) with Favorites, History, Finder-tags, volume list
- [x] T1.4 Toolbar: back/forward, path breadcrumb, search field, layout switcher, thumbnail size
- [x] T1.5 Media grid: icon-view with thumbnails, filename labels, quiet neutral selection fill (no accent box)
- [x] T1.9 Unified keyboard navigation: arrows move the selection in every layout, Shift extends, Home/End jump; wheel scrolls the virtual lists with no eased lag
- [x] T1.6 Hover tooltip: name, file size, image dimensions, dates
- [x] T1.7 Bottom path bar: "Macintosh HD › ... › FolderName (N 图像)"
- [x] T1.8 Dark/light theme auto-detect matching FlowVision colors (#FFFFFF / #2D2D2D grid bg)

## Phase 2: Large Image View
- [x] T2.1 Space/Enter toggles large image from grid
- [x] T2.2 Full-area image display with fit-to-window / 100% zoom
- [x] T2.3 Window title shows "filename (index/total)"
- [x] T2.4 Arrow keys (Cmd+Left/Right) navigate prev/next image
- [x] T2.5 Zoom: scroll wheel, long-press left = 100%, right-click = fit
- [x] T2.6 Rotate (E) and mirror, left-rotate (Q)
- [x] T2.7 Esc closes large view

## Phase 3: File System & Navigation
- [x] T3.1 Directory tree population with lazy expand
- [x] T3.2 Folder click navigates and loads media
- [x] T3.3 Right-mouse gesture: up=parent, left/right=prev/next folder with media, down=back
- [x] T3.4 W/A/S/D keys map to gestures
- [x] T3.5 Cmd+Up/Down parent/enter folder
- [x] T3.6 Recursive browsing mode (Cmd+Shift+R)
- [x] T3.7 File system watcher for auto-refresh

## Phase 4: Media Loading (no mock)
- [x] T4.1 Thumbnail decoding: PNG, JPEG, GIF, WebP, BMP, TIFF
- [x] T4.2 Thumbnail caching with LRU and size key
- [x] T4.3 Background concurrent decode with thread limit
- [x] T4.4 Full image decode for large view
- [x] T4.5 EXIF metadata extraction (dimensions, dates, camera)
- [x] T4.6 Video thumbnail (first frame via ffmpeg)
- [x] T4.7 Video playback (mpv or AVPlayer bridge)

## Phase 5: File Operations
- [x] T5.1 Copy/move/delete/rename with two-phase rename
- [x] T5.2 Undo/redo (Cmd+Z / Cmd+Shift+Z)
- [x] T5.3 Batch rename with preview and rule templates
- [x] T5.4 Quick rename in current folder
- [x] T5.5 Copy to configured folder (shortcuts)
- [x] T5.6 New folder (Cmd+Shift+N)
- [x] T5.7 Move to trash (Delete)

## Phase 6: Search & Sort
- [x] T6.1 Filename search with pinyin support
- [x] T6.2 Sort by name, date, size, EXIF date, random
- [x] T6.3 Filter by file type (image/video/raw)
- [x] T6.4 Finder tag filter

## Phase 7: Layout Modes
- [x] T7.1 Justified (Google Photos style, two-ended align)
- [x] T7.2 Waterfall (masonry)
- [x] T7.3 Grid (uniform)
- [x] T7.4 Detail (list view)
- [x] T7.5 Thumbnail size adjust (= / - / 0)

## Phase 8: Settings & Configuration
- [x] T8.1 Settings window with panes (general, custom, actions, tagging, advanced)
- [x] T8.2 9 configuration slots (Opt+1~9 switch, Cmd+Opt+1~9 save)
- [x] T8.3 Persist preferences to disk

## Phase 9: Polish
- [x] T9.1 Window management: maximize(1), fit(2), center(5), pin top(T)
- [x] T9.2 Tabbed browsing
- [x] T9.3 Finder tags and rating display
- [x] T9.4 EXIF info overlay (I)
- [x] T9.5 File info overlay (U)

## Phase 10: FlowVision Large-View Alignment
- [x] T10.1 Layered large view: image/video media, overlays, HUD, controls, edge arrows
- [x] T10.2 Pointer-anchored zoom with FlowVision original-width 1/2–10x limits
- [x] T10.3 Fit, 1:1, zoom buttons, keyboard shortcuts, long-press zoom, transient zoom HUD
- [x] T10.4 Drag pan, double-click close, edge-click previous/next, video pause on click
- [x] T10.5 Real ffmpeg video playback, 520x67 floating controls, seek, elapsed/total time
- [x] T10.6 Finder tag dots, rating stars, EXIF and file info overlays over media
- [x] T10.7 MyGo v0.7.3 release build, go test/vet/build, headless and real-window acceptance

## Phase 11: Finder-Style Waterfall
- [x] T11.1 Dark Finder-like media surface with compact captions and shared sidebar chrome
- [x] T11.2 No white card backing, padding or media gutters in grid and waterfall thumbnails
- [x] T11.3 Thumbnail images cover their card so letterbox/pillarbox white edges cannot appear
- [x] T11.4 Finder-sized multi-column waterfall with uneven column heights and shortest-column packing
- [x] T11.5 Real-window acceptance on Qxpicture: uneven columns, no UI white frame, responsive thumb sizing

## Phase 12: Display and Browsing Capabilities
- [x] T12.1 More menu with pin, settings, file visibility and window controls
- [x] T12.2 Independent hidden, image, RAW, video, archive and all-type filters
- [x] T12.3 Recursive folder inclusion and current-folder media-unit browsing
- [x] T12.4 Selected-video autoplay, internal/external player preference
- [x] T12.5 Configurable auto scroll and large-view slideshow with end stop
- [x] T12.6 System/light/dark appearance, persisted preferences and shortcuts
- [x] T12.7 Window maximize, suitable size, center, actual/current image size
- [x] T12.8 Finder-style sorting menu, folder-first/full-path and EXIF order
- [x] T12.9 Regression tests and isolated real-window acceptance

## Phase 13: Logging and Crash Hardening
- [x] T13.1 Rotating file log at `~/Library/Logs/Lumiwerk/lumiwerk.log` with `LUMIWERK_LOG` level override and stderr mirror
- [x] T13.2 Log startup/shutdown, preference, decode and file-operation failures
- [x] T13.3 `logRecover` and `safeGo` so panics in render and background jobs are logged, not fatal
- [x] T13.4 Fix `SIGSEGV` in `popPool`: pin the Objective-C autorelease pool to one OS thread with `runtime.LockOSThread`
- [x] T13.5 Nil-window safe `update` helper and out-of-range safe `visibleItem` for views racing background scans
- [x] T13.6 Stress tests (repeated `go test`, threaded share stress, `-race`) and real-window launch check

## Phase 14: Observable and Reusable UI Shell
- [x] T14.1 Fix grid/list selection-key mismatch and add real MyGo list rendering regression
- [x] T14.2 Unified browser shell, media selection contract, settings sections and safe view boundaries
- [x] T14.3 Diagnostic window with recent events, panic counters, runtime metrics, copy and open-log actions
- [x] T14.4 Route remaining background jobs through the common panic-recording task entry point
- [x] T14.5 Development guide covering MyGo contracts, isolation, recovery and real-window verification

## Phase 15: Native Playback
- [x] T15.1 IINA-style bottom controller with full-width timeline, transport, time, volume and more menu
- [x] T15.2 Unified Go playback session with libmpv and FFmpeg picture/sound fallback
- [x] T15.3 Preserve screenshot, crop/trim, frame stepping, playback speed and previous/next media
- [x] T15.4 Fake-backend state tests, real FFmpeg decoding tests and `ui.NewTester` interaction coverage

## Phase 16: Density and Localization
- [x] T16.1 Folder named once (breadcrumb); no in-content title, no count beside search
- [x] T16.2 Status bar shows the selection (name · size · W×H · modified, or count and total) from in-memory fields only; dimensions probe async
- [x] T16.3 New windows default to justified layout, thumb 150; justified cards sized by aspect ratio and rows filled; grid uses fit (no crop), tighter padding/gaps
- [x] T16.4 App declares zh-Hans (`mygo.App.SetLanguages`) so MyGo controls, menus and the updater speak Chinese; no remaining English UI literals
- [x] T16.5 Platform labels (`src/labels.go`): root (Macintosh HD / 此电脑 / /), reveal (Finder / 资源管理器 / 文件管理器); Finder tags hidden off macOS; "在 Lumiwerk 中打开所在文件夹" distinct from reveal

## Phase 17: Gallery Mode
- [x] T17.1 Immersive gallery (G / toolbar button): black stage, no chrome, Esc returns to the grid
- [x] T17.2 Rounded thumbnail strip near the bottom edge (H pins it), only visible thumbnails built and loaded
- [x] T17.3 Bottom progress bar with click/drag jump and compact `n / N`
- [x] T17.4 Mixed images and videos: videos in strip and progress bar, inline playback through the existing playback session (libmpv with `mpvbackend`, FFmpeg fallback)
- [x] T17.5 Plain large view kept: Space opens it, Up/Down step previous/next media (video volume on Shift+Up/Down)
- [x] T17.6 macOS hidden title bar (`TitleBarHiddenInset`): traffic lights over the toolbar, which pads past them and drags the window
- [x] T17.7 Sidebar Finder tags section removed on every platform (with the tag filter it drove); tags stay in sort and context menus
- [x] T17.8 Tiled browsing speed: queued thumbnail decodes no longer leave a scrolled-past card stuck loading; flow layouts build half a screen past each edge (justified 260→195 cards per frame); thumbnails ≤128 decode at 256 px (decode −17%, pixels −56%)

## Phase 18: Captionless Grid, Video Covers and History
- [x] T18.1 Esc leaves both the gallery and the Space large view back to the grid (verified)
- [x] T18.2 无字模式: toolbar toggle and 显示 menu (⇧⌘H), persisted; grid, compact and waterfall drop the caption band
- [x] T18.3 Video covers in release builds: ffmpeg resolved beyond the GUI PATH (Homebrew, exe folder, WinGet/Scoop/Chocolatey), then QuickLook (`qlmanage`) on macOS, then the bundled libmpv image output; failures logged with every attempt
- [x] T18.4 History rows show the last visit right-aligned (HH:mm today, else MM-dd HH:mm), stored in history.json `visited`
- [x] T18.5 清除历史记录 in the history header and the row context menu, saved at once
