package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

const (
	playbackEndStop = "stop"
	playbackEndNext = "next"
	playbackEndLoop = "loop"

	// timelineInset keeps the knob inside the scrub box at both ends.
	timelineInset = float32(6)
)

func normalizePlaybackEnd(mode string) string {
	switch mode {
	case playbackEndNext, playbackEndLoop:
		return mode
	default:
		return playbackEndStop
	}
}

func playbackStatusLabel(snapshot playbackSnapshot) string {
	switch snapshot.Phase {
	case playLoading:
		return "正在加载"
	case playBuffering:
		return "正在缓冲"
	case playEnded:
		return "播放结束"
	case playError:
		return "无法播放"
	default:
		return ""
	}
}

func formatPlaybackSpeed(speed float64) string {
	if speed == float64(int(speed)) {
		return fmt.Sprintf("%.0f×", speed)
	}
	return fmt.Sprintf("%g×", speed)
}

func samePlaybackSpeed(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.01
}

func timelineFraction(x, width, inset float32) float64 {
	if width <= 1 {
		return 0
	}
	start, usable := inset, width-2*inset
	if usable < 24 {
		start, usable = 0, width
	}
	f := (x - start) / usable
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return float64(f)
}

func unitFraction(part, total float64) float32 {
	if total <= 0 || part <= 0 {
		return 0
	}
	if part >= total {
		return 1
	}
	return float32(part / total)
}

func (a *appState) playheadDuration() float64 {
	if a.videoKernel() != nil {
		if d := a.videoSnapshot().Duration; d > 0 {
			return d
		}
	}
	return a.videoDuration()
}

func (a *appState) nudgePlayhead(delta float64) {
	a.seekVideo(clampPlaybackTime(a.playhead()+delta, a.playheadDuration()))
}

func (a *appState) nudgeSpeed(delta float64) {
	kernel := a.videoKernel()
	if kernel == nil {
		return
	}
	kernel.SetSpeed(a.videoSnapshot().Speed + delta)
}

func (a *appState) toggleFullscreen() {
	if a.Window == nil {
		return
	}
	a.Window.ToggleFullScreen()
}

func (a *appState) tickPlaybackEnd() {
	if a.videoKernel() == nil || a.Editor.Open {
		return
	}
	snap := a.videoSnapshot()
	if snap.Phase != playEnded {
		a.playerEndSeen = false
		return
	}
	if a.playerEndSeen {
		return
	}
	a.playerEndSeen = true
	switch normalizePlaybackEnd(a.Profiles.PlaybackEnd) {
	case playbackEndLoop:
		a.playVideo()
	case playbackEndNext:
		a.nextImage()
	}
}

func (a *appState) videoPlayerKeys(c *ui.Context) {
	if !a.InLargeView || a.Editor.Open || a.modalOpen() || !a.videoLargeView() {
		return
	}
	switch {
	case c.Shortcut(ui.Shift, ui.KeyLeft):
		a.nudgePlayhead(-1)
	case c.Shortcut(ui.Shift, ui.KeyRight):
		a.nudgePlayhead(1)
	case c.Shortcut(0, ui.KeyLeft):
		a.nudgePlayhead(-5)
	case c.Shortcut(0, ui.KeyRight):
		a.nudgePlayhead(5)
	case c.Shortcut(ui.Cmd, ui.KeyLeft):
		a.stepVideo(-1)
	case c.Shortcut(ui.Cmd, ui.KeyRight):
		a.stepVideo(1)
	case c.Shortcut(0, ui.KeyHome):
		a.seekVideo(0)
	case c.Shortcut(0, ui.KeyEnd):
		a.seekVideo(a.playheadDuration())
	case c.Shortcut(0, ui.KeyM):
		a.toggleVideoVolume()
	case c.Shortcut(0, ui.KeyF):
		a.toggleFullscreen()
	case c.Shortcut(ui.Cmd, ui.KeyS):
		a.captureCurrentFrame()
	case c.Shortcut(0, ui.KeyK):
		a.toggleVideoPlayback()
	case c.Shortcut(0, ui.KeyJ):
		a.nudgePlayhead(-10)
	case c.Shortcut(0, ui.KeyL):
		a.nudgePlayhead(10)
	case c.Shortcut(ui.Shift, ui.KeyUp):
		a.setVideoVolume(a.VideoVolume + 0.1)
	case c.Shortcut(ui.Shift, ui.KeyDown):
		a.setVideoVolume(a.VideoVolume - 0.1)
	case c.Shortcut(ui.Shift, ui.KeyComma), c.Shortcut(0, ui.KeyBracketLeft):
		a.nudgeSpeed(-0.25)
	case c.Shortcut(ui.Shift, ui.KeyPeriod), c.Shortcut(0, ui.KeyBracketRight):
		a.nudgeSpeed(0.25)
	default:
		for i := 0; i < 10; i++ {
			if c.Shortcut(0, ui.Key0+ui.Key(i)) {
				a.seekVideo(a.playheadDuration() * float64(i) / 10)
				return
			}
		}
	}
}

func (a *appState) scrubTimelineTo(x float32, duration float64) {
	kernel := a.videoKernel()
	if kernel == nil || duration <= 0 {
		return
	}
	at := duration * timelineFraction(x, a.playerTimelineW, timelineInset)
	if !kernel.Scrubbing() {
		kernel.BeginScrub()
	}
	kernel.ScrubTo(at)
}

// videoControlsBar is the floating transport. The timeline is one full-width
// target; played and buffered ranges are drawn inside it, and the time
// labels sit on the ends. Hidden only after the fade finishes, so a click
// on the picture is not swallowed by an invisible bar.
func (a *appState) videoControlsBar(c *ui.Context, item *mediaItem) {
	if a.videoKernel() == nil {
		return
	}
	snapshot := a.videoSnapshot()
	duration := snapshot.Duration
	if duration <= 0 {
		duration = item.Info.DurationSec
	}
	if duration <= 0 {
		duration = 1
	}
	pos := clampPlaybackTime(snapshot.Position, duration)
	buffered := snapshot.Buffered
	if buffered < pos {
		buffered = pos
	}
	// One row like the image strip: the timeline takes the width left
	// between the transport buttons and the trailing controls. The loading
	// and buffering state is the badge on the stage, not part of the bar.
	winW, _ := c.Size()
	bar := a.controlBar(c, "player-bar", controlBarWidth(winW), func() {
		a.playerMenu = false
		a.videoTransport(c, item, snapshot, pos, buffered, duration)
	})
	// A paused, loading or failed clip keeps the bar up in auto mode:
	// the user is about to press something.
	hold := snapshot.Phase != playPlaying
	fadeControlBar(bar, "player-fade", a.controlBarShown(c, bar.Hovered(), bar.FocusWithin(), snapshot.Scrubbing, hold))
}

func (a *appState) videoTimeline(c *ui.Context, pos, buffered, duration float64) {
	box := ui.Box(c.Key("video-progress")).Grow(1).MinWidth(80).Height(barBtnSize).Label("播放进度").Tooltip("拖动跳转")
	width := box.LayoutBounds().W
	if width > 1 {
		// Input handlers run between builds, when an Element is no longer
		// valid. Keep the committed width in state for scrub callbacks.
		a.playerTimelineW = width
	}
	var bubble string
	var bubbleLeft float32
	if x, _, over := box.PointerPosition(); over {
		at := duration * timelineFraction(x, width, timelineInset)
		bubble = formatPlaybackTime(at)
		span := width - 2*timelineInset
		if span < 24 {
			span = width
		}
		bubbleLeft = timelineInset + float32(timelineFraction(x, width, timelineInset))*span - 24
		if bubbleLeft < 0 {
			bubbleLeft = 0
		}
		if width > 48 && bubbleLeft > width-48 {
			bubbleLeft = width - 48
		}
	}
	box.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Button != 0 {
			return false
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			a.scrubTimelineTo(ev.X, duration)
			return true
		case ui.InputPointerMove:
			if kernel := a.videoKernel(); kernel == nil || !kernel.Scrubbing() {
				return false
			}
			a.scrubTimelineTo(ev.X, duration)
			return true
		case ui.InputPointerUp:
			if kernel := a.videoKernel(); kernel != nil && kernel.Scrubbing() {
				if !ev.Canceled {
					a.scrubTimelineTo(ev.X, duration)
				}
				kernel.EndScrub()
			}
			return true
		default:
			return false
		}
	})
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		inset := timelineInset
		if r.W < inset*2+24 {
			inset = 0
		}
		track := ui.Rect{X: r.X + inset, Y: r.Y + r.H/2 - 2, W: max(0, r.W-2*inset), H: 4}
		p.Fill(track, ui.RGB(255, 255, 255).Alpha(0.18), 2)
		bufW := track.W * unitFraction(buffered, duration)
		playW := track.W * unitFraction(pos, duration)
		p.Fill(ui.Rect{X: track.X, Y: track.Y, W: bufW, H: track.H}, ui.RGB(255, 255, 255).Alpha(0.40), 2)
		p.Fill(ui.Rect{X: track.X, Y: track.Y, W: playW, H: track.H}, ui.RGB(10, 132, 255), 2)
		knobX := track.X + playW - 6
		p.Fill(ui.Rect{X: knobX, Y: track.Y - 4, W: 12, H: 12}, ui.RGB(255, 255, 255), 6)
	})
	box.Children(func() {
		if bubble != "" {
			// Above the bar, so the pointer never covers it.
			ui.Text(c, bubble).Absolute().Left(bubbleLeft).Top(-30).
				Padding(2, 6).Radius(4).Background(ui.RGBA(0, 0, 0, 0.82)).
				FontSize(11).TextColor(ui.RGB(255, 255, 255)).PassThrough()
		}
	})
}

func (a *appState) videoTransport(c *ui.Context, item *mediaItem, snapshot playbackSnapshot, pos, buffered, duration float64) {
	row := controlBarRow(c)
	row.Children(func() {
		if barPrimaryButton(c, "video-play", playbackButtonLabel(snapshot.Playing), "播放 / 暂停（空格）", playbackButtonIcon(snapshot.Playing)).Clicked() {
			a.toggleVideoPlayback()
		}
		if barIconButton(c, "video-previous", "上一个媒体", "上一个媒体", lucideSkipBack).Clicked() {
			a.prevImage()
		}
		if barIconButton(c, "video-next", "下一个媒体", "下一个媒体", lucideSkipForward).Clicked() {
			a.nextImage()
		}
		ui.Box(c).Width(4)
		barText(c, formatPlaybackTime(pos), false)
		a.videoTimeline(c, pos, buffered, duration)
		barText(c, formatPlaybackTime(duration), true)
		ui.Box(c).Width(4)
		// Saving lives in the 更多 menu; its progress shows here while it
		// runs or after it fails, so it stays visible during playback.
		if status := a.VideoSave.statusLabel(); status != "" {
			ui.Text(c, status).FontSize(11).TextColor(barFGDim).SingleLine()
			if a.VideoSave.phase == saveRunning {
				ui.Progress(c, float64(a.VideoSave.progress)).Width(60).Label("另存进度")
				if barChip(c, "video-save-cancel", "取消", "取消另存", "取消").Clicked() {
					a.cancelVideoSave()
				}
			}
		}
		if barIconButton(c, "video-volume", "静音", "静音 / 恢复音量（M）", volumeButtonIcon(a.VideoVolume)).Clicked() {
			a.toggleVideoVolume()
		}
		volume := a.VideoVolume
		if ui.Slider(c.Key("video-volume-slider"), &volume, 0, 1).Width(64).Height(22).
			Step(0.01).Label("音量").Changed() {
			a.setVideoVolume(volume)
		}
		barChip(c, "video-speed", "倍速", "播放速度", formatPlaybackSpeed(snapshot.Speed)).
			PopoverMenuButton(func(m *ui.Menu) { a.fillSpeedMenu(m, snapshot.Speed) })
		if len(snapshot.Audio) > 0 || len(snapshot.Video) > 1 {
			title := "音轨"
			if len(snapshot.Video) > 1 {
				title = "画质"
			}
			barChip(c, "video-tracks", title, title, title).
				PopoverMenuButton(func(m *ui.Menu) { a.fillTrackMenu(m, snapshot) })
		}
		if barIconButton(c, "video-fullscreen", "全屏", "全屏（F）", lucideExpand).Clicked() {
			a.toggleFullscreen()
		}
		more := barIconButton(c, "video-more", "更多播放操作", "更多播放操作", lucideMore)
		more.PopoverMenuButton(func(m *ui.Menu) { a.videoMoreMenu(m, item, snapshot) })
	})
}

func (a *appState) fillSpeedMenu(m *ui.Menu, speed float64) {
	a.playerMenu = true
	for _, rate := range []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4} {
		rate := rate
		label := fmt.Sprintf("倍速 %.2g×", rate)
		if rate == 1 {
			label = "正常速度"
		}
		if m.Item(label).Radio(samePlaybackSpeed(speed, rate)).Chosen() {
			if kernel := a.videoKernel(); kernel != nil {
				kernel.SetSpeed(rate)
			}
		}
	}
}

func (a *appState) fillTrackMenu(m *ui.Menu, snapshot playbackSnapshot) {
	a.playerMenu = true
	kernel := a.videoKernel()
	if kernel == nil {
		return
	}
	if len(snapshot.Video) > 1 {
		for _, track := range snapshot.Video {
			track := track
			label := track.Label
			if label == "" {
				label = fmt.Sprintf("画质 %d", track.ID+1)
			}
			if m.Item(label).Radio(track.Selected).Chosen() {
				kernel.SelectVideo(track.ID)
			}
		}
	}
	if len(snapshot.Video) > 1 && len(snapshot.Audio) > 0 {
		m.Separator()
	}
	for _, track := range snapshot.Audio {
		track := track
		label := track.Label
		if label == "" {
			label = fmt.Sprintf("音轨 %d", track.ID+1)
		}
		if m.Item(label).Radio(track.Selected).Chosen() {
			kernel.SelectAudio(track.ID)
		}
	}
}

func (a *appState) videoMoreMenu(m *ui.Menu, item *mediaItem, snapshot playbackSnapshot) {
	a.playerMenu = true
	if m.Item("上一帧").Chosen() {
		a.stepVideo(-1)
	}
	if m.Item("下一帧").Chosen() {
		a.stepVideo(1)
	}
	if m.Item("截存当前画面").Chosen() {
		a.captureCurrentFrame()
	}
	if m.Item("另存视频…").Chosen() {
		a.chooseVideoSave(item)
	}
	if m.Item("裁剪画面").Chosen() {
		if item != nil {
			a.promptCrop([]string{item.Path})
		}
	}
	if m.Item("裁剪片段").Chosen() {
		a.promptTrim()
	}
	m.Separator()
	a.fillSpeedMenu(m, snapshot.Speed)
	m.Separator()
	for _, opt := range []struct{ id, label string }{
		{playbackEndStop, "停在末帧"},
		{playbackEndNext, "自动下一个"},
		{playbackEndLoop, "循环播放"},
	} {
		opt := opt
		if m.Item(opt.label).Radio(normalizePlaybackEnd(a.Profiles.PlaybackEnd) == opt.id).Chosen() {
			a.Profiles.PlaybackEnd = opt.id
			savePrefs(a.Profiles)
		}
	}
	m.Separator()
	if m.Item("适应窗口").Chosen() {
		a.resetZoom()
	}
	if m.Item("实际大小").Chosen() {
		a.zoom100()
	}
	if m.Item("顺时针旋转").Chosen() {
		a.Rotation += 90
	}
	if m.Item("水平翻转").Checked(a.Mirror).Chosen() {
		a.Mirror = !a.Mirror
	}
	m.Separator()
	if m.Item("显示媒体信息").Chosen() {
		a.Info.File = !a.Info.File
	}
	if m.Item(revealLabel).Chosen() {
		a.revealInFinder()
	}
}
