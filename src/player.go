package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	playbackEndStop = "stop"
	playbackEndNext = "next"
	playbackEndLoop = "loop"

	timelineInset = float32(64)
	playerIdle    = 2 * time.Second
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
	if a.Video != nil && a.Video.kernel != nil {
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
	if a.Video == nil || a.Video.kernel == nil {
		return
	}
	a.Video.kernel.SetSpeed(a.videoSnapshot().Speed + delta)
}

func (a *appState) toggleFullscreen() {
	if a.Window == nil {
		return
	}
	a.Window.ToggleFullScreen()
}

// notePlayerPointer starts the idle clock only after the pointer actually
// moves. A still pointer, including the first sample, does not count.
func (a *appState) notePlayerPointer(x, y float32) {
	if !a.playerPtrSeen {
		a.playerPtrSeen = true
		a.playerPtrX, a.playerPtrY = x, y
		return
	}
	if abs32(x-a.playerPtrX) <= 0.5 && abs32(y-a.playerPtrY) <= 0.5 {
		return
	}
	a.playerPtrX, a.playerPtrY = x, y
	a.playerMoved = time.Now()
}

// playerBarShown keeps the chrome up while the pointer is on it, a menu is
// open, or a scrub is in progress. playerMoved zero means the bar stays
// visible and does not schedule a wake, which is how tests render it.
func (a *appState) playerBarShown(c *ui.Context, hover, focus, scrubbing bool) bool {
	if a.playerMoved.IsZero() {
		return true
	}
	if hover || focus || a.playerMenu || scrubbing {
		a.playerMoved = c.Now()
		return true
	}
	remain := playerIdle - c.Now().Sub(a.playerMoved)
	if remain > 0 {
		c.After(remain)
		return true
	}
	return false
}

func (a *appState) tickPlaybackEnd() {
	if a.Video == nil || a.Video.kernel == nil || a.Editor.Open {
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
	if a.Video == nil || a.Video.kernel == nil || duration <= 0 {
		return
	}
	at := duration * timelineFraction(x, a.playerTimelineW, timelineInset)
	if !a.Video.kernel.Scrubbing() {
		a.Video.kernel.BeginScrub()
	}
	a.Video.kernel.ScrubTo(at)
}

// videoControlsBar is the floating transport. The timeline is one full-width
// target; played and buffered ranges are drawn inside it, and the time
// labels sit on the ends. Hidden only after the fade finishes, so a click
// on the picture is not swallowed by an invisible bar.
func (a *appState) videoControlsBar(c *ui.Context, item *mediaItem) {
	if a.Video == nil || a.Video.kernel == nil {
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
	bar := ui.Column(c.Key("player-bar")).Attach(ui.AnchorBottom, ui.AnchorBottom).
		FillWidth().Margin(8, 8).Padding(8, 12, 10).Gap(6).Radius(12).
		Background(ui.RGBA(16, 16, 18, 0.82))
	bar.Shadow(0, -6, 18, 0, ui.RGBA(0, 0, 0, 0.45))
	bar.Children(func() {
		a.playerMenu = false
		a.videoTimeline(c, pos, buffered, duration)
		a.videoTransport(c, item, snapshot, pos, duration)
	})
	shown := a.playerBarShown(c, bar.Hovered(), bar.FocusWithin(), snapshot.Scrubbing)
	target := float32(0)
	if shown {
		target = 1
	}
	opacity := bar.AnimateTo("player-fade", target, ui.Animation{Duration: 180 * time.Millisecond})
	bar.Opacity(opacity)
	if !shown && opacity < 0.04 {
		bar.Hidden(true)
	}
}

func (a *appState) videoTimeline(c *ui.Context, pos, buffered, duration float64) {
	box := ui.Box(c.Key("video-progress")).FillWidth().Height(36).Label("播放进度").Tooltip("拖动跳转")
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
			if a.Video == nil || a.Video.kernel == nil || !a.Video.kernel.Scrubbing() {
				return false
			}
			a.scrubTimelineTo(ev.X, duration)
			return true
		case ui.InputPointerUp:
			if a.Video != nil && a.Video.kernel != nil && a.Video.kernel.Scrubbing() {
				a.scrubTimelineTo(ev.X, duration)
				a.Video.kernel.EndScrub()
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
		ui.Text(c, formatPlaybackTime(pos)).Absolute().Left(0).Top(10).
			FontSize(12).TextColor(ui.RGB(245, 245, 247)).PassThrough()
		ui.Text(c, formatPlaybackTime(duration)).Absolute().Right(0).Top(10).
			FontSize(12).TextColor(ui.RGB(180, 180, 185)).PassThrough()
		if bubble != "" {
			ui.Text(c, bubble).Absolute().Left(bubbleLeft).Top(0).
				Padding(2, 6).Radius(4).Background(ui.RGBA(0, 0, 0, 0.82)).
				FontSize(11).TextColor(ui.RGB(255, 255, 255)).PassThrough()
		}
	})
}

func (a *appState) videoTransport(c *ui.Context, item *mediaItem, snapshot playbackSnapshot, pos, duration float64) {
	row := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center)
	row.Children(func() {
		if playerIconButton(c, "video-play", playbackButtonLabel(snapshot.Playing), "播放 / 暂停（空格）", playbackButtonIcon(snapshot.Playing)).Clicked() {
			a.toggleVideoPlayback()
		}
		if playerIconButton(c, "video-previous", "上一个媒体", "上一个媒体", lucideSkipBack).Clicked() {
			a.prevImage()
		}
		if playerIconButton(c, "video-next", "下一个媒体", "下一个媒体", lucideSkipForward).Clicked() {
			a.nextImage()
		}
		if playerIconButton(c, "video-volume", "静音", "静音 / 恢复音量（M）", volumeButtonIcon(a.VideoVolume)).Clicked() {
			a.toggleVideoVolume()
		}
		volume := a.VideoVolume
		if ui.Slider(c.Key("video-volume-slider"), &volume, 0, 1).Width(76).Height(22).
			Step(0.01).Label("音量").Changed() {
			a.setVideoVolume(volume)
		}
		if save := ui.Button(c.Key("video-save"), "另存视频…").
			Tooltip("把当前视频另存一份").Padding(4, 8).Radius(5).
			TextColor(ui.RGB(245, 245, 247)).Disabled(a.VideoSave.active()); save.Clicked() {
			a.chooseVideoSave(item)
		}
		if status := a.VideoSave.statusLabel(); status != "" {
			ui.Text(c, status).FontSize(11).TextColor(ui.RGB(180, 180, 185)).SingleLine()
			if a.VideoSave.phase == saveRunning {
				ui.Progress(c, float64(a.VideoSave.progress)).Width(70).Label("另存进度")
				if ui.Button(c.Key("video-save-cancel"), "取消").Clicked() {
					a.cancelVideoSave()
				}
			}
		}
		ui.Box(c).Grow(1)
		clock := formatPlaybackTime(pos) + " / " + formatPlaybackTime(duration)
		if status := playbackStatusLabel(snapshot); status != "" {
			clock = status + "  " + clock
		}
		ui.Text(c, clock).FontSize(12).TextColor(ui.RGB(245, 245, 247)).SingleLine()
		ui.ButtonBase(c.Key("video-speed")).Label("倍速").Tooltip("播放速度").
			Padding(4, 8).Radius(5).TextColor(ui.RGB(245, 245, 247)).
			Children(func() {
				ui.Text(c, formatPlaybackSpeed(snapshot.Speed)).FontSize(12).TextColor(ui.RGB(245, 245, 247))
			}).
			PopoverMenuButton(func(m *ui.Menu) { a.fillSpeedMenu(m, snapshot.Speed) })
		if len(snapshot.Audio) > 0 || len(snapshot.Video) > 1 {
			title := "音轨"
			if len(snapshot.Video) > 1 {
				title = "画质"
			}
			ui.ButtonBase(c.Key("video-tracks")).Label(title).Tooltip(title).
				Padding(4, 8).Radius(5).TextColor(ui.RGB(245, 245, 247)).
				Children(func() {
					ui.Text(c, title).FontSize(12).TextColor(ui.RGB(245, 245, 247))
				}).
				PopoverMenuButton(func(m *ui.Menu) { a.fillTrackMenu(m, snapshot) })
		}
		if playerIconButton(c, "video-fullscreen", "全屏", "全屏（F）", lucideExpand).Clicked() {
			a.toggleFullscreen()
		}
		more := playerIconButton(c, "video-more", "更多播放操作", "更多播放操作", lucideMore)
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
			if a.Video != nil && a.Video.kernel != nil {
				a.Video.kernel.SetSpeed(rate)
			}
		}
	}
}

func (a *appState) fillTrackMenu(m *ui.Menu, snapshot playbackSnapshot) {
	a.playerMenu = true
	if a.Video == nil || a.Video.kernel == nil {
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
				a.Video.kernel.SelectVideo(track.ID)
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
			a.Video.kernel.SelectAudio(track.ID)
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
