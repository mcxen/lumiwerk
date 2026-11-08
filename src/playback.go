package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

// playbackBackend owns picture and sound. Position and Duration are media
// seconds, not a second clock in the view. Calls are serialized by the session.
type playbackBackend interface {
	Name() string
	Open(path string, at float64, playing bool) error
	Close() error
	Frame() image.Image
	Position() float64
	Duration() float64
	SetPlaying(playing bool)
	Seek(seconds float64)
	SetVolume(volume float64)
	SetSpeed(speed float64)
	FrameStep(direction int) (float64, error)
}

func clampPlaybackTime(seconds, duration float64) float64 {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0
	}
	if duration > 0 && seconds > duration {
		return duration
	}
	return seconds
}

func normalizedPlaybackSpeed(speed float64) float64 {
	if math.IsNaN(speed) || math.IsInf(speed, 0) {
		return 1
	}
	return min(max(speed, 0.25), 4)
}

func normalizedPlaybackVolume(volume float64) float64 {
	if math.IsNaN(volume) || math.IsInf(volume, 0) {
		return 1
	}
	return min(max(volume, 0), 2)
}

// playbackPhase is the only play state the native view renders. Views do not
// keep a second set of playing/paused/buffering flags.
type playbackPhase int

const (
	playIdle playbackPhase = iota
	playLoading
	playPlaying
	playPaused
	playBuffering
	playEnded
	playError
)

func (p playbackPhase) label() string {
	switch p {
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

// playbackTrack is one audio or video stream the backend can switch to.
type playbackTrack struct {
	ID       int
	Label    string
	Selected bool
}

type trackSource interface {
	AudioTracks() []playbackTrack
	VideoTracks() []playbackTrack
	SelectAudio(id int) error
	SelectVideo(id int) error
}

type bufferReporter interface{ Buffered() float64 }
type stallReporter interface{ Stalled() bool }

// playbackSnapshot is the only state the native view needs to draw a frame.
type playbackSnapshot struct {
	Backend   string
	Path      string
	Frame     image.Image
	Position  float64
	Duration  float64
	Buffered  float64
	Playing   bool
	Ended     bool
	Ready     bool
	Speed     float64
	Err       error
	Phase     playbackPhase
	Scrubbing bool
	Seeking   bool
	Audio     []playbackTrack
	Video     []playbackTrack
	// FramePushed tells that the backend announces new frames itself
	// (framePusher), so the view need not poll for them.
	FramePushed bool
}

// playbackSession owns backend selection and lifecycle. In particular, seeking
// and stepping never replace the backend or start a competing audio stream.
type playbackSession struct {
	mu              sync.Mutex
	backend         playbackBackend
	path            string
	frame           image.Image
	position        float64
	duration        float64
	playing         bool
	ended           bool
	ready           bool
	speed           float64
	volume          float64
	err             error
	phase           playbackPhase
	buffered        float64
	stalled         bool
	seekWant        float64
	seekPending     bool
	seekSince       time.Time
	scrubbing       bool
	scrubAt         float64
	scrubWasPlaying bool
}

var playbackProbe = func() (playbackBackend, error) { return newMPVBackend() }
var newFallbackPlayback = func() (playbackBackend, error) { return newFFmpegBackend() }

func newPlaybackSession() *playbackSession {
	return &playbackSession{speed: 1, volume: 1}
}

func (s *playbackSession) Open(path string, at float64, playing bool) error {
	if path == "" {
		return errors.New("empty media path")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openLocked(path, at, playing)
}

func (s *playbackSession) failOpen(path string, err error) error {
	s.path = path
	s.playing = false
	s.err = err
	return err
}

func (s *playbackSession) openLocked(path string, at float64, playing bool) error {
	if s.backend == nil {
		backend, err := playbackProbe()
		if err != nil || backend == nil {
			if err != nil {
				logInfof("video.kernel: libmpv unavailable, using FFmpeg: %v", err)
			}
			backend, err = newFallbackPlayback()
			if err != nil {
				return s.failOpen(path, err)
			}
		}
		s.backend = backend
	}
	s.path = path
	s.frame = nil
	s.position = clampPlaybackTime(at, 0)
	s.playing = playing
	s.ended = false
	s.ready = false
	s.err = nil
	s.seekPending = false
	s.scrubbing = false
	s.buffered = s.position
	s.stalled = false
	s.phase = playLoading
	if err := s.backend.Open(path, at, playing); err != nil {
		return s.failOpen(path, err)
	}
	s.backend.SetVolume(s.volume)
	s.backend.SetSpeed(s.speed)
	logInfof("video.kernel: backend=%s path=%s", s.backend.Name(), path)
	return nil
}

func (s *playbackSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend != nil {
		_ = s.backend.Close()
		s.backend = nil
	}
	s.path = ""
	s.frame = nil
	s.playing = false
	s.ready = false
	s.ended = false
	s.seekPending = false
	s.scrubbing = false
	s.phase = playIdle
	s.err = nil
}

// Park pauses and unloads the current file without destroying the backend
// (libmpv render thread / FFmpeg process). The next Open reuses it.
func (s *playbackSession) Park() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil {
		return
	}
	s.playing = false
	s.backend.SetPlaying(false)
	if p, ok := s.backend.(interface{ Park() }); ok {
		p.Park()
	}
	s.path = ""
	s.frame = nil
	s.ready = false
	s.ended = false
	s.seekPending = false
	s.scrubbing = false
	s.phase = playIdle
	s.err = nil
}

func (s *playbackSession) SetPlaying(playing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil {
		return
	}
	if playing && s.ended {
		_ = s.openLocked(s.path, 0, true)
		return
	}
	s.playing = playing
	if playing {
		s.err = nil
	}
	s.backend.SetPlaying(playing)
}

func (s *playbackSession) Seek(at float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil {
		return
	}
	s.position = clampPlaybackTime(at, s.duration)
	s.seekWant = s.position
	s.seekPending = true
	s.seekSince = time.Now()
	s.scrubbing = false
	s.ended = s.duration > 0 && s.position >= s.duration-0.0005
	if s.ended {
		s.playing = false
	}
	s.buffered = s.position
	s.backend.Seek(s.position)
	s.backend.SetPlaying(s.playing)
}

// BeginScrub freezes picture updates and remembers whether playback should
// resume. The displayed position then follows ScrubTo until EndScrub seeks.
func (s *playbackSession) BeginScrub() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil || s.scrubbing {
		return
	}
	s.scrubWasPlaying = s.playing && !s.ended
	s.scrubAt = s.position
	if s.seekPending {
		s.scrubAt = s.seekWant
	}
	s.scrubbing = true
	s.playing = false
	s.ended = false
	s.backend.SetPlaying(false)
}

// ScrubTo moves the displayed playhead without decoding a new position.
func (s *playbackSession) ScrubTo(at float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil {
		return
	}
	if !s.scrubbing {
		s.scrubWasPlaying = s.playing && !s.ended
		s.playing = false
		s.ended = false
		s.scrubbing = true
		s.backend.SetPlaying(false)
	}
	s.scrubAt = clampPlaybackTime(at, s.duration)
	s.position = s.scrubAt
}

// EndScrub seeks to the scrubbed position and resumes when playback was
// running before the drag.
func (s *playbackSession) EndScrub() {
	s.mu.Lock()
	if !s.scrubbing {
		s.mu.Unlock()
		return
	}
	at := s.scrubAt
	resume := s.scrubWasPlaying && !(s.duration > 0 && at >= s.duration-0.0005)
	s.scrubbing = false
	s.mu.Unlock()
	s.Seek(at)
	if resume {
		s.SetPlaying(true)
	}
}

func (s *playbackSession) Scrubbing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scrubbing
}

func (s *playbackSession) SelectAudio(id int) {
	s.selectTrack(id, true)
}

func (s *playbackSession) SelectVideo(id int) {
	s.selectTrack(id, false)
}

func (s *playbackSession) selectTrack(id int, audio bool) {
	s.mu.Lock()
	backend := s.backend
	at := s.position
	if s.seekPending {
		at = s.seekWant
	}
	s.mu.Unlock()
	src, ok := backend.(trackSource)
	if !ok {
		return
	}
	var err error
	if audio {
		err = src.SelectAudio(id)
	} else {
		err = src.SelectVideo(id)
	}
	if err != nil {
		logWarnf("video.track: audio=%t id=%d err=%v", audio, id, err)
		return
	}
	s.mu.Lock()
	s.position = at
	s.seekWant = at
	s.seekPending = true
	s.seekSince = time.Now()
	s.mu.Unlock()
}

func (s *playbackSession) SetVolume(volume float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volume = normalizedPlaybackVolume(volume)
	if s.backend != nil {
		s.backend.SetVolume(s.volume)
	}
}

func (s *playbackSession) SetSpeed(speed float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speed = normalizedPlaybackSpeed(speed)
	if s.backend != nil {
		s.backend.SetSpeed(s.speed)
	}
}

func (s *playbackSession) Step(direction int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil || direction == 0 {
		return nil
	}
	s.playing = false
	s.ended = false
	s.seekPending = false
	s.scrubbing = false
	s.backend.SetPlaying(false)
	at, err := s.backend.FrameStep(direction)
	if err != nil {
		return err
	}
	// Pause keeps the last snapshot frame. A step has a new one; take it
	// now or the paused snapshot will never ask again.
	if frame := s.backend.Frame(); frame != nil {
		s.frame = frame
	}
	s.position = clampPlaybackTime(at, s.duration)
	return nil
}

func seekCalibrated(backendAt, want float64, since time.Time) bool {
	if math.IsNaN(backendAt) || math.IsInf(backendAt, 0) || backendAt < 0 {
		return time.Since(since) > 2*time.Second
	}
	if math.Abs(backendAt-want) <= 0.35 {
		return true
	}
	return time.Since(since) > 2*time.Second
}

// Snapshot is called on the MyGo main thread. Backends keep blocking codec work
// off this method and return their most recent frame and state. A seek keeps
// the displayed position on the target until the backend arrives there.
func (s *playbackSession) Snapshot() playbackSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := playbackSnapshot{
		Path: s.path, Frame: s.frame, Position: s.position, Duration: s.duration,
		Buffered: s.buffered, Playing: s.playing, Ended: s.ended, Ready: s.ready,
		Speed: s.speed, Err: s.err, Phase: s.phase,
	}
	if s.backend == nil {
		s.derivePhase()
		snap.Phase = s.phase
		snap.Err = s.err
		snap.Playing, snap.Ended = s.playing, s.ended
		return snap
	}
	snap.Backend = s.backend.Name()
	_, snap.FramePushed = s.backend.(framePusher)
	// A pause keeps the picture already on screen. Pulling another frame
	// here would keep showing whatever the backend is still decoding, so
	// the view would move after the user had paused it.
	if !s.scrubbing && (s.playing || s.seekPending || s.frame == nil) {
		if frame := s.backend.Frame(); frame != nil {
			s.frame = frame
			snap.Frame = frame
		}
	}
	backendAt := s.backend.Position()
	if duration := s.backend.Duration(); duration > 0 && !math.IsNaN(duration) && !math.IsInf(duration, 0) {
		s.duration = duration
	}
	switch {
	case s.scrubbing:
		s.position = clampPlaybackTime(s.scrubAt, s.duration)
	case s.seekPending && seekCalibrated(backendAt, s.seekWant, s.seekSince):
		s.seekPending = false
		if !math.IsNaN(backendAt) && !math.IsInf(backendAt, 0) && backendAt >= 0 {
			s.position = clampPlaybackTime(backendAt, s.duration)
		}
	case s.seekPending:
		s.position = clampPlaybackTime(s.seekWant, s.duration)
	default:
		if !math.IsNaN(backendAt) && !math.IsInf(backendAt, 0) && backendAt >= 0 {
			s.position = clampPlaybackTime(backendAt, s.duration)
		}
	}
	if s.scrubbing || s.seekPending {
		if s.buffered < s.position {
			s.buffered = s.position
		}
	} else if reporter, ok := s.backend.(bufferReporter); ok {
		buffered := reporter.Buffered()
		if math.IsNaN(buffered) || math.IsInf(buffered, 0) || buffered < s.position {
			buffered = s.position
		}
		if s.duration > 0 && buffered > s.duration {
			buffered = s.duration
		}
		s.buffered = buffered
	} else {
		s.buffered = s.position
	}
	s.stalled = false
	if !s.scrubbing && !s.seekPending {
		if reporter, ok := s.backend.(stallReporter); ok {
			s.stalled = reporter.Stalled()
		}
	}
	if s.frame != nil || s.position > 0 {
		s.ready = true
	}
	// Reaching the end has to pause the backend too. Leaving it running
	// keeps the sound going, and the next play click reopens the file
	// because ended is set. A later position well before the end clears
	// that flag so a clock glitch cannot stick the player on "ended".
	reachedEnd := !s.scrubbing && !s.seekPending && s.duration > 0 && s.position >= s.duration-0.0005
	if reachedEnd {
		if s.playing || !s.ended {
			s.playing = false
			if s.backend != nil {
				s.backend.SetPlaying(false)
			}
		}
		s.ended = true
		s.position = s.duration
	} else if s.ended && !s.scrubbing && !s.seekPending && s.duration > 0 && s.position+0.25 < s.duration {
		s.ended = false
	}
	if s.scrubbing || s.seekPending {
		s.ended = s.duration > 0 && s.position >= s.duration-0.0005
	}
	if tracks, ok := s.backend.(trackSource); ok {
		snap.Audio = tracks.AudioTracks()
		snap.Video = tracks.VideoTracks()
	}
	s.derivePhase()
	snap.Frame = s.frame
	snap.Position = s.position
	snap.Duration = s.duration
	snap.Buffered = s.buffered
	snap.Playing = s.playing
	snap.Ended = s.ended
	snap.Ready = s.ready
	snap.Speed = s.speed
	snap.Err = s.err
	snap.Phase = s.phase
	snap.Scrubbing = s.scrubbing
	snap.Seeking = s.seekPending
	return snap
}

func (s *playbackSession) derivePhase() {
	switch {
	case s.err != nil && s.frame == nil:
		s.phase = playError
	case s.path == "" && s.backend == nil:
		s.phase = playIdle
	case s.ended:
		s.phase = playEnded
	case s.scrubbing:
		s.phase = playPaused
	case !s.ready && s.path != "":
		s.phase = playLoading
	case s.playing && s.stalled:
		s.phase = playBuffering
	case s.playing:
		s.phase = playPlaying
	case s.path == "":
		s.phase = playIdle
	default:
		s.phase = playPaused
	}
}

// formatPlaybackTime renders a media clock as mm:ss, or h:mm:ss past an hour.
func formatPlaybackTime(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	total := int(seconds)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func (s *playbackSession) retarget(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	if backend, ok := s.backend.(*ffmpegBackend); ok {
		backend.mu.Lock()
		backend.path = path
		backend.mu.Unlock()
	}
}

// ffmpegBackend shares one FFmpeg process and one PCM stream. Its media clock
// starts only after both have their first data ready, preventing video from
// running ahead while a seek primes the decoder.
type ffmpegBackend struct {
	mu           sync.Mutex
	path         string
	decoder      *ffmpegDecoder
	sound        *soundtrack
	player       *audio.Player
	clock        mediaClock
	volume       float64
	speed        float64
	position     float64
	duration     float64
	playing      bool
	ended        bool
	frame        image.Image
	frameAt      float64
	held         *timedFrame
	frames       chan timedFrame
	edge         int
	generation   int
	closed       bool
	audioRunning bool
	lastErr      error
	notify       func()
	audioMap     int
	videoMap     int
	audioTracks  []playbackTrack
	videoTracks  []playbackTrack
	stalled      bool
}

var openPlaybackSound = func(path string, at, speed float64) (*soundtrack, error) {
	return newSoundtrackRate(path, at, speed)
}

var playbackSoundPrime = func(dec *soundtrack) error { return dec.prime() }

var playbackNow = time.Now

func newFFmpegBackend() (*ffmpegBackend, error) {
	if _, err := execLookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("FFmpeg playback unavailable: %w", err)
	}
	return &ffmpegBackend{volume: 1, speed: 1, edge: 2560, audioMap: -1, videoMap: -1}, nil
}

func (b *ffmpegBackend) validate(path string) error {
	dec := newFFmpegDecoder(path)
	if err := dec.start(); err != nil {
		return err
	}
	_, err := dec.Next()
	closeErr := dec.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// execLookPath is a variable so fallback construction is testable without a
// machine-specific media installation.
var execLookPath = lookTool

func (b *ffmpegBackend) Name() string { return "FFmpeg" }

func (b *ffmpegBackend) Open(path string, at float64, playing bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return io.EOF
	}
	b.stopLocked()
	at = max(0, at)
	b.path = path
	if err := b.validate(path); err != nil {
		b.path = ""
		b.playing = false
		return err
	}
	b.position, b.frameAt = at, at
	b.clock.seek(at, false, playbackNow())
	b.playing = false
	b.ended = false
	b.frame = nil
	b.held = nil
	b.lastErr = nil
	b.decoder = newFFmpegDecoderAt(path, at)
	b.decoder.edge = b.edge
	b.decoder.audioMap = b.audioMap
	b.decoder.videoMap = b.videoMap
	if err := b.decoder.start(); err != nil {
		return err
	}
	b.frames = startFramePump(b.decoder)
	if b.duration <= 0 {
		b.duration = probeVideo(path).DurationSec
	}
	if playing {
		b.playing = true
		b.restartAudioLocked()
	}
	b.kickTrackProbe()
	return nil
}

func (b *ffmpegBackend) stopLocked() {
	b.generation++
	if b.decoder != nil {
		_ = b.decoder.Close()
		b.decoder = nil
	}
	b.frames = nil
	b.held = nil
	b.stopAudioLocked()
	b.audioRunning = false
}

func (b *ffmpegBackend) Close() error {
	b.mu.Lock()
	b.closed = true
	b.stopLocked()
	b.mu.Unlock()
	return nil
}

// Park stops the decoder/audio but leaves the backend reusable for Open.
func (b *ffmpegBackend) Park() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.stopLocked()
	b.path = ""
	b.playing = false
	b.frame = nil
}

func (b *ffmpegBackend) SetNotify(fn func()) {
	b.mu.Lock()
	b.notify = fn
	b.mu.Unlock()
}

func (b *ffmpegBackend) stopAudioLocked() {
	if b.player != nil {
		b.player.Pause()
		b.player.Close()
		b.player = nil
	}
	if b.sound != nil {
		_ = b.sound.Close()
		b.sound = nil
	}
}

// watchSound primes PCM asynchronously before the shared media clock starts.
func (b *ffmpegBackend) watchSound(sound *soundtrack, generation int) {
	safeGo("video.soundPrime", func() {
		err := playbackSoundPrime(sound)
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.generation != generation || b.sound != sound {
			_ = sound.Close()
			return
		}
		if err != nil {
			logWarnf("video.audio: %v", err)
			b.stopAudioLocked()
			b.audioRunning = true
			b.clock.seek(b.clock.origin, true, playbackNow())
			if b.notify != nil {
				b.notify()
			}
			return
		}
		b.player = audio.NewPlayer(sound)
		b.player.SetSpeed(b.speed)
		b.player.SetVolume(b.volume)
		b.player.Play()
		b.audioRunning = true
		b.clock.seek(b.clock.origin, true, playbackNow())
		if b.notify != nil {
			b.notify()
		}
	})
}

func (b *ffmpegBackend) restartAudioLocked() {
	b.generation++
	generation := b.generation
	b.stopAudioLocked()
	b.audioRunning = false
	if !b.playing || b.path == "" {
		return
	}
	at := b.clock.origin
	sound, err := openPlaybackSound(b.path, at, b.speed)
	if err != nil {
		logWarnf("video.audio: %v", err)
		b.audioRunning = true
		b.clock.seek(at, true, playbackNow())
		return
	}
	sound.audioMap = b.audioMap
	sound.videoMap = b.videoMap
	b.sound = sound
	b.watchSound(sound, generation)
}

func (b *ffmpegBackend) pollLocked() {
	now := playbackNow()
	if b.playing && b.audioRunning && !b.ended {
		b.position = b.clock.at(now)
	}
	b.stalled = false
	var batch []timedFrame
	if b.held != nil {
		batch = append(batch, *b.held)
		b.held = nil
	}
	for b.frames != nil {
		select {
		case frame, ok := <-b.frames:
			if !ok {
				b.frames = nil
				continue
			}
			batch = append(batch, frame)
		default:
			goto picked
		}
	}
picked:
	show, ok, ahead, hasAhead := shownFrame(batch, b.position)
	if hasAhead {
		held := ahead
		b.held = &held
	}
	if ok && show.err != nil {
		if b.frame == nil && !errors.Is(show.err, io.EOF) {
			b.lastErr = show.err
		}
		if b.playing {
			b.playing = false
			b.ended = errors.Is(show.err, io.EOF)
			b.clock.pause(now)
			b.stopAudioLocked()
			if b.duration > 0 && b.ended {
				b.position = b.duration
			}
		}
	}
	if ok && show.img != nil && show.at <= b.position+0.0005 {
		b.frame = show.img
		b.frameAt = show.at
	}
	if b.duration > 0 && b.position >= b.duration {
		b.position = b.duration
		if b.playing {
			b.playing = false
			b.ended = true
			b.clock.seek(b.duration, false, now)
			b.stopAudioLocked()
		}
	}
	if b.playing && !b.ended && b.frame != nil {
		waitingAudio := !b.audioRunning
		ahead := 0.0
		if b.audioRunning {
			ahead = b.clock.at(now) - b.frameAt
		}
		queued := 0
		if b.frames != nil {
			queued = len(b.frames)
		}
		b.stalled = waitingAudio || (ahead > 0.5 && queued == 0 && b.held == nil)
	}
}

func (b *ffmpegBackend) Frame() image.Image {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pollLocked()
	return b.frame
}

func (b *ffmpegBackend) Position() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.position
}

func (b *ffmpegBackend) Duration() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.duration
}

func (b *ffmpegBackend) SetPlaying(playing bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.decoder == nil {
		return
	}
	if playing {
		if b.ended || (b.duration > 0 && b.position >= b.duration) {
			b.reopenLocked(0, true)
			return
		}
		if b.playing && b.audioRunning {
			return
		}
		b.playing = true
		b.clock.seek(b.position, false, playbackNow())
		b.restartAudioLocked()
		return
	}
	b.position = b.clock.at(playbackNow())
	if b.duration > 0 {
		b.position = min(b.position, b.duration)
	}
	b.playing = false
	b.clock.seek(b.position, false, playbackNow())
	b.restartAudioLocked()
}

func (b *ffmpegBackend) Seek(seconds float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seekLocked(seconds)
}

func (b *ffmpegBackend) seekLocked(seconds float64) {
	if b.closed || b.decoder == nil {
		return
	}
	at := clampPlaybackTime(seconds, b.duration)
	playing := b.playing
	b.stopLocked()
	b.position, b.frameAt = at, at
	b.clock.seek(at, false, playbackNow())
	b.ended = b.duration > 0 && at >= b.duration
	// Keep the last picture until the seeked frame arrives so the view does
	// not flash empty while the clock jumps.
	b.held = nil
	b.decoder = newFFmpegDecoderAt(b.path, at)
	b.decoder.audioMap = b.audioMap
	b.decoder.videoMap = b.videoMap
	b.decoder.edge = b.edge
	if err := b.decoder.start(); err != nil {
		b.lastErr = err
		b.playing = false
		return
	}
	b.frames = startFramePump(b.decoder)
	b.playing = playing && !b.ended
	b.restartAudioLocked()
}

func (b *ffmpegBackend) reopenLocked(at float64, playing bool) {
	b.stopLocked()
	b.playing = playing
	b.position = at
	b.clock.seek(at, false, playbackNow())
	b.ended = false
	b.frame, b.held = nil, nil
	b.decoder = newFFmpegDecoderAt(b.path, at)
	b.decoder.edge = b.edge
	b.decoder.audioMap = b.audioMap
	b.decoder.videoMap = b.videoMap
	if err := b.decoder.start(); err != nil {
		b.lastErr = err
		b.playing = false
		return
	}
	b.frames = startFramePump(b.decoder)
	b.restartAudioLocked()
}

func (b *ffmpegBackend) FrameStep(direction int) (float64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.decoder == nil {
		return b.position, io.EOF
	}
	b.playing = false
	b.restartAudioLocked()
	frameDuration := 1.0 / 30
	if b.decoder.sourceFrame > 0 {
		frameDuration = b.decoder.sourceFrame.Seconds()
	}
	// mpv and IINA define forward frame step as the next decoded frame. Seek
	// just beyond the current presentation timestamp, rather than accumulating
	// a wall-clock position that may not correspond to an actual decoded frame.
	at := b.frameAt + frameDuration/2
	if direction < 0 {
		at = b.position - frameDuration
	}
	if b.duration > 0 {
		at = min(at, b.duration)
	}
	at = max(0, at)
	b.seekLocked(at)
	b.position = at
	b.ended = false
	return at, nil
}

func (b *ffmpegBackend) SetVolume(volume float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.volume = normalizedPlaybackVolume(volume)
	if b.player != nil {
		b.player.SetVolume(b.volume)
	}
}

func (b *ffmpegBackend) SetSpeed(speed float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.speed = normalizedPlaybackSpeed(speed)
	if b.playing {
		b.restartAudioLocked()
	}
}

func (b *ffmpegBackend) SetRenderEdge(edge int) {
	b.mu.Lock()
	b.edge = edge
	if b.decoder != nil {
		b.decoder.edge = edge
	}
	b.mu.Unlock()
}

func (b *ffmpegBackend) Buffered() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	buf := b.position
	if b.held != nil && b.held.at > buf {
		buf = b.held.at
	}
	if b.frames != nil && len(b.frames) > 0 {
		frameDur := 1.0 / 30
		if b.decoder != nil && b.decoder.sourceFrame > 0 {
			frameDur = b.decoder.sourceFrame.Seconds()
		}
		if est := b.position + float64(len(b.frames))*frameDur; est > buf {
			buf = est
		}
	}
	if b.duration > 0 && buf > b.duration {
		buf = b.duration
	}
	return buf
}

func (b *ffmpegBackend) Stalled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stalled
}

func (b *ffmpegBackend) AudioTracks() []playbackTrack {
	b.mu.Lock()
	defer b.mu.Unlock()
	return markPlaybackTrack(b.audioTracks, b.audioMap)
}

func (b *ffmpegBackend) VideoTracks() []playbackTrack {
	b.mu.Lock()
	defer b.mu.Unlock()
	return markPlaybackTrack(b.videoTracks, b.videoMap)
}

func (b *ffmpegBackend) SelectAudio(id int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.audioMap = id
	if b.decoder == nil || b.path == "" {
		return nil
	}
	b.seekLocked(b.position)
	return b.lastErr
}

func (b *ffmpegBackend) SelectVideo(id int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.videoMap = id
	if b.decoder == nil || b.path == "" {
		return nil
	}
	b.seekLocked(b.position)
	return b.lastErr
}

func (b *ffmpegBackend) kickTrackProbe() {
	path := b.path
	safeGo("video.tracks", func() {
		audio, video := probePlaybackTracks(path)
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.path != path {
			return
		}
		b.audioTracks = audio
		b.videoTracks = video
	})
}

func markPlaybackTrack(tracks []playbackTrack, selected int) []playbackTrack {
	if len(tracks) == 0 {
		return nil
	}
	out := append([]playbackTrack(nil), tracks...)
	for i := range out {
		out[i].Selected = selected < 0 && i == 0 || out[i].ID == selected
	}
	return out
}

func probePlaybackTracks(path string) (audio, video []playbackTrack) {
	out, err := exec.Command(toolPath("ffprobe"), "-v", "error", "-print_format", "json", "-show_streams", path).Output()
	if err != nil || len(out) == 0 {
		return nil, nil
	}
	var probed struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Channels  int    `json:"channels"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probed); err != nil {
		return nil, nil
	}
	audioN, videoN := 0, 0
	for _, stream := range probed.Streams {
		label := stream.Tags.Title
		if label == "" {
			label = stream.Tags.Language
		}
		switch stream.CodecType {
		case "audio":
			if label == "" {
				label = stream.CodecName
			}
			if stream.Channels > 0 {
				label = strings.TrimSpace(fmt.Sprintf("%s %dch", label, stream.Channels))
			}
			if label == "" {
				label = fmt.Sprintf("音轨 %d", audioN+1)
			}
			audio = append(audio, playbackTrack{ID: audioN, Label: label})
			audioN++
		case "video":
			if stream.Width > 0 && stream.Height > 0 {
				size := fmt.Sprintf("%d×%d", stream.Width, stream.Height)
				if label == "" {
					label = size
				} else {
					label = label + " " + size
				}
			}
			if stream.CodecName != "" {
				label = strings.TrimSpace(label + " " + stream.CodecName)
			}
			if label == "" {
				label = fmt.Sprintf("画质 %d", videoN+1)
			}
			video = append(video, playbackTrack{ID: videoN, Label: label})
			videoN++
		}
	}
	return audio, video
}
