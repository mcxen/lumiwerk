# Lumiwerk — FlowVision Alignment Task List

Reference: FlowVision.app built from `/Users/mcx/Documents/OpenSpring/FlowVision`
Reference screenshots: `docs/ref-grid.png`, `docs/ref-large-image.png`

## Phase 1: Main Window Layout (grid view)
- [x] T1.1 Project scaffold (mygo v0.6.0, native UI, build passes)
- [ ] T1.2 Three-pane split: sidebar | path-bar+grid | (no right pane in grid mode)
- [ ] T1.3 Sidebar: directory tree (NSOutlineView equivalent) with Favorites, History, Finder-tags, volume list
- [ ] T1.4 Toolbar: back/forward, path breadcrumb, search field, layout switcher, thumbnail size
- [ ] T1.5 Media grid: icon-view with thumbnails, filename labels, selection highlight (blue outline)
- [ ] T1.6 Hover tooltip: name, file size, image dimensions, dates
- [ ] T1.7 Bottom path bar: "Macintosh HD › ... › FolderName (N 图像)"
- [ ] T1.8 Dark/light theme auto-detect matching FlowVision colors (#FFFFFF / #2D2D2D grid bg)

## Phase 2: Large Image View
- [ ] T2.1 Space/Enter toggles large image from grid
- [ ] T2.2 Full-area image display with fit-to-window / 100% zoom
- [ ] T2.3 Window title shows "filename (index/total)"
- [ ] T2.4 Arrow keys (Cmd+Left/Right) navigate prev/next image
- [ ] T2.5 Zoom: scroll wheel, long-press left = 100%, right-click = fit
- [ ] T2.6 Rotate (E) and mirror, left-rotate (Q)
- [ ] T2.7 Esc closes large view

## Phase 3: File System & Navigation
- [ ] T3.1 Directory tree population with lazy expand
- [ ] T3.2 Folder click navigates and loads media
- [ ] T3.3 Right-mouse gesture: up=parent, left/right=prev/next folder with media, down=back
- [ ] T3.4 W/A/S/D keys map to gestures
- [ ] T3.5 Cmd+Up/Down parent/enter folder
- [ ] T3.6 Recursive browsing mode (Cmd+Shift+R)
- [ ] T3.7 File system watcher for auto-refresh

## Phase 4: Media Loading (no mock)
- [ ] T4.1 Thumbnail decoding: PNG, JPEG, GIF, WebP, BMP, TIFF
- [ ] T4.2 Thumbnail caching with LRU and size key
- [ ] T4.3 Background concurrent decode with thread limit
- [ ] T4.4 Full image decode for large view
- [ ] T4.5 EXIF metadata extraction (dimensions, dates, camera)
- [ ] T4.6 Video thumbnail (first frame via ffmpeg)
- [ ] T4.7 Video playback (mpv or AVPlayer bridge)

## Phase 5: File Operations
- [ ] T5.1 Copy/move/delete/rename with two-phase rename
- [ ] T5.2 Undo/redo (Cmd+Z / Cmd+Shift+Z)
- [ ] T5.3 Batch rename with preview and rule templates
- [ ] T5.4 Quick rename in current folder
- [ ] T5.5 Copy to configured folder (shortcuts)
- [ ] T5.6 New folder (Cmd+Shift+N)
- [ ] T5.7 Move to trash (Delete)

## Phase 6: Search & Sort
- [ ] T6.1 Filename search with pinyin support
- [ ] T6.2 Sort by name, date, size, EXIF date, random
- [ ] T6.3 Filter by file type (image/video/raw)
- [ ] T6.4 Finder tag filter

## Phase 7: Layout Modes
- [ ] T7.1 Justified (Google Photos style, two-ended align)
- [ ] T7.2 Waterfall (masonry)
- [ ] T7.3 Grid (uniform)
- [ ] T7.4 Detail (list view)
- [ ] T7.5 Thumbnail size adjust (= / - / 0)

## Phase 8: Settings & Configuration
- [ ] T8.1 Settings window with panes (general, custom, actions, tagging, advanced)
- [ ] T8.2 9 configuration slots (Opt+1~9 switch, Cmd+Opt+1~9 save)
- [ ] T8.3 Persist preferences to disk

## Phase 9: Polish
- [ ] T9.1 Window management: maximize(1), fit(2), center(5), pin top(T)
- [ ] T9.2 Tabbed browsing
- [ ] T9.3 Finder tags and rating display
- [ ] T9.4 EXIF info overlay (I)
- [ ] T9.5 File info overlay (U)
