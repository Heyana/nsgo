package core

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Video plays a video in the platform's player, filling the view: the picture fits it by ResizeMode, and Controls shows playback controls. It plays unless Paused; Ref.Seek jumps to a time.
//
// A video with an alpha channel shows what is behind its transparent pixels where the platform decodes the alpha: HEVC with alpha in a QuickTime or MPEG-4 file on macOS (iOS uses the same player layer but is unchecked), and VP8 or VP9 with alpha in WebM on Linux with GStreamer's codecalpha plug-in (gst-plugins-bad). On macOS, iOS, Windows and Linux, a local file whose alpha the platform cannot keep (WebM with alpha on Apple platforms; any clip with alpha on Windows, until a Windows machine has checked what Media Foundation's decoders output for it; HEVC with alpha on Linux, and VP8 or VP9 with alpha there without codecalpha) reports OnError, a *MediaError whose Reason is MediaErrorAlphaUnsupported, instead of showing an opaque picture. Android and the web, and a remote source anywhere, play such clips as their decoder outputs them, which may be opaque. With Controls, AVKit's player draws black behind the picture.
type Video struct {
	// Source is a URL, a file path or an "asset:" path in the app's resources.
	Source     string
	SourceFunc func() string
	// Headers supplies HTTP request headers for the source on Android and
	// for direct HTTP media on Linux, macOS, iOS and tvOS. On Apple, it
	// also applies to a direct HTTP AudioSource file. Its fields are copied
	// when the video mounts.
	Headers map[string]string
	// Paused applies when the source opens and when its value changes. A non-repeating video stays at its end through other prop changes: set Paused true then false to replay it from the start, or from where its built-in controls scrubbed it back to, or Seek before the end, which plays on from there when Paused is false.
	Paused     bool
	PausedFunc func() bool
	Muted      bool
	MutedFunc  func() bool
	// Volume runs from 0 to 1; 0 plays at full volume, and Muted
	// silences the video.
	Volume     float32
	VolumeFunc func() float32
	// Rate is the playback speed; 0 plays at 1. VideoInfo.RateChangeable tells whether the loaded source supports changing speed.
	Rate     float32
	RateFunc func() float32
	// Repeat starts the video over when it ends.
	Repeat         bool
	ResizeMode     ResizeMode
	ResizeModeFunc func() ResizeMode
	Controls       bool
	// HitTestAlpha is the minimum video opacity, from 0 to 1, at which a
	// painted macOS window takes the pointer. It applies to videos
	// without Controls or an asset track transform once a frame is decoded.
	// Zero counts the whole video view.
	HitTestAlpha float32
	// PictureInPicture enables video Picture in Picture on iOS, tvOS, macOS and supporting web browsers: Ref.StartPictureInPicture and StopPictureInPicture start and stop it, and on iOS and tvOS the video holds the playback audio session from then on, paused or not. Other targets report Supported false through OnPictureInPicture. What the PiP window plays and pauses is reported by OnPlay and OnPause.
	PictureInPicture bool
	// AutoPictureInPicture lets iOS start PiP on its own as the app goes to the background while the video plays.
	AutoPictureInPicture bool
	// OnPictureInPicture receives the session's state as it changes: whether the platform supports PiP, whether it can start now, whether it is active, and why a start failed.
	OnPictureInPicture func(PictureInPictureState)
	// OnPictureInPictureRestore runs when the user returns to the app from the PiP window, so the app can show the video's page again; the system restores its interface once the commit that follows is applied.
	OnPictureInPictureRestore func()
	// ProgressInterval is the time between OnProgress calls while the
	// video plays: 250 milliseconds when 0, and at least 16.
	ProgressInterval time.Duration
	// AudioSource replaces the original sound with a separate audio track
	// for a finite video on macOS, iOS and tvOS. It accepts the same paths
	// and URLs as Source. Both tracks start together and seek together.
	AudioSource string
	// AudioSession is what the sound does to other apps' audio while it plays: by default the app's settings decide, and on iOS the device's silent mode silences it; a video of speech, as an audiobook's, plays in silent mode with AudioSessionPlayback or AudioSessionMixWithOthers. AudioSessionFunc changes it as the video plays: iOS holds the new session before letting the old go, so playback goes on without a gap, and Android opens the source again.
	AudioSession     AudioSession
	AudioSessionFunc func() AudioSession
	// Class is Tailwind CSS classes, as View's.
	Class     string
	Style     Style
	StyleFunc func() Style

	// OnLoadStart is called when a source starts opening, and OnLoad once
	// its duration and size are known.
	OnLoadStart func()
	OnLoad      func(VideoInfo)
	// OnProgress receives the position and duration while the video
	// plays, and once more when it pauses or ends.
	OnProgress func(position, duration time.Duration)
	// OnEnd is called each time the video reaches its end, before a
	// repeating one starts over.
	OnEnd func()
	// OnBuffer receives whether playback waits for data.
	OnBuffer func(buffering bool)
	// OnPause reports that the platform paused the video, not the app: its built-in controls, or the system for an interruption or a lost audio output. The video stays paused until Paused changes, or the controls or the system play it again (OnPlay); an app that shows its own play state sets Paused to true, and then false to play again.
	OnPause func(VideoPauseReason)
	// OnPlay reports that the platform played the video, not the app: its built-in controls, or the system as an interruption that paused it ends. The video plays on until Paused changes, even while Paused is true.
	OnPlay    func()
	OnError   func(err error)
	OnPress   func(PressEvent)
	OnLayout  func(Bounds)
	Ref       *Ref
	Label     string
	LabelFunc func() string
	TestID    string
}

// VideoPauseReason says why the platform paused a video (Video.OnPause).
type VideoPauseReason uint8

const (
	// VideoPauseInterruption is the system's: a call, Siri, another app taking the audio, or the app going to the background.
	VideoPauseInterruption VideoPauseReason = iota
	// VideoPauseRouteLost is the audio's output going away, such as headphones unplugged.
	VideoPauseRouteLost
	// VideoPauseControls is the platform's built-in controls (Controls), its Picture in Picture window or Control Center.
	VideoPauseControls
)

// String names a video pause reason for logs and diagnostics.
func (r VideoPauseReason) String() string {
	switch r {
	case VideoPauseInterruption:
		return "interruption"
	case VideoPauseRouteLost:
		return "route-lost"
	case VideoPauseControls:
		return "controls"
	default:
		return "unknown"
	}
}

// Render implements Node.
func (v Video) Render(*Scope) Node { return v }

func (v Video) mountIn(t *Tree, cx *Scope, parent *node) { t.mountVideo(cx, parent, v) }

// VideoInfo is what a video reports when it loads. Duration is 0 for a
// stream without one, such as a live broadcast.
type VideoInfo struct {
	Duration time.Duration
	// Size is the picture's natural size in points.
	Size Size
	// RateChangeable reports whether the backend can change this source's playback speed. Linux requires a seekable stream, GstPlay and GStreamer's scaletempo filter.
	RateChangeable bool
}

type videoNode struct {
	config, sent driver.VideoConfig
	loadStart    func()
	load         func(VideoInfo)
	progress     func(position, duration time.Duration)
	end          func()
	buffer       func(bool)
	pause        func(VideoPauseReason)
	play         func()
	loadError    func(error)
	// The last video and audio sources refused by Access.
	refused, refusedAudio string
	pip                   *pictureInPictureNode
}

func (t *Tree) mountVideo(cx *Scope, parent *node, v Video) {
	n := t.newNode(driver.KindVideo, parent, cx)
	t.setClasses(n, v.Class)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	t.setRef(n, v.Ref)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout}
	n.props = driver.ViewProps{Label: v.Label, TestID: v.TestID}
	t.bindAccessibilityText(cx, n, &n.props.Label, v.LabelFunc)
	vn := &videoNode{
		config: driver.VideoConfig{Source: v.Source, Headers: driver.NewVideoHeaders(v.Headers), Paused: v.Paused, Muted: v.Muted, Volume: videoVolume(v.Volume),
			Rate: videoRate(v.Rate), Repeat: v.Repeat, ResizeMode: v.ResizeMode, Controls: v.Controls, HitTestAlpha: alphaThreshold(v.HitTestAlpha),
			ProgressInterval: videoInterval(v.ProgressInterval), AudioSource: v.AudioSource, AudioSession: v.AudioSession,
			PictureInPicture: v.PictureInPicture, AutoPictureInPicture: v.AutoPictureInPicture},
		loadStart: v.OnLoadStart, load: v.OnLoad, progress: v.OnProgress, end: v.OnEnd,
		buffer: v.OnBuffer, pause: v.OnPause, play: v.OnPlay, loadError: v.OnError,
	}
	if v.PictureInPicture {
		vn.pip = &pictureInPictureNode{state: v.OnPictureInPicture, restore: v.OnPictureInPictureRestore}
	}
	n.video = vn
	if v.PictureInPicture && !videoPictureInPicturePlatform(cx) {
		t.reports = append(t.reports, report{owner: n, fn: func() {
			if !videoPictureInPicturePlatform(cx) && v.OnPictureInPicture != nil {
				v.OnPictureInPicture(PictureInPictureState{})
			}
		}})
	}
	bind := func(set func(*driver.VideoConfig)) {
		cx.renderEffect(func() {
			c := vn.config
			set(&c)
			if c != vn.config {
				vn.config = c
				t.touch(n)
			}
		})
	}
	if f := v.SourceFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.Source = f() })
	}
	if f := v.PausedFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.Paused = f() })
	}
	if f := v.MutedFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.Muted = f() })
	}
	if f := v.VolumeFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.Volume = videoVolume(f()) })
	}
	if f := v.ResizeModeFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.ResizeMode = f() })
	}
	if f := v.RateFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.Rate = videoRate(f()) })
	}
	if f := v.AudioSessionFunc; f != nil {
		bind(func(c *driver.VideoConfig) { c.AudioSession = f() })
	}
}

func videoVolume(v float32) float32 {
	if v <= 0 || v > 1 || v != v {
		return 1
	}
	return v
}

func videoRate(r float32) float32 {
	if r <= 0 || r != r {
		return 1
	}
	return r
}

func videoInterval(d time.Duration) time.Duration {
	if d == 0 {
		return 250 * time.Millisecond
	}
	return max(d, 16*time.Millisecond)
}

// dispatchVideo handles a video's events.
func (t *Tree) dispatchVideo(n *node, ev driver.Event) bool {
	vn := n.video
	call := func(fn func()) {
		t.within(n.owner, fn)
	}
	switch ev.Kind {
	case driver.EventLoadStart:
		if vn.loadStart != nil {
			call(vn.loadStart)
		}
	case driver.EventLoad:
		if vn.load != nil {
			call(func() { vn.load(VideoInfo{Duration: ev.Duration, Size: ev.Size, RateChangeable: ev.RateChangeable}) })
		}
	case driver.EventProgress:
		if vn.progress != nil {
			call(func() { vn.progress(ev.Time, ev.Duration) })
		}
	case driver.EventEnd:
		if vn.end != nil {
			call(vn.end)
		}
	case driver.EventBuffer:
		if vn.buffer != nil {
			call(func() { vn.buffer(ev.Buffering) })
		}
	case driver.EventVideoPause:
		if vn.pause != nil {
			call(func() { vn.pause(VideoPauseReason(ev.Index)) })
		}
	case driver.EventVideoPlay:
		if vn.play != nil {
			call(vn.play)
		}
	case driver.EventError:
		if vn.loadError != nil {
			call(func() { vn.loadError(&MediaError{Message: ev.Text, Reason: MediaErrorReason(ev.Index)}) })
		}
	default:
		return false
	}
	return true
}

// events is what a video reports.
func (vn *videoNode) events() driver.EventMask {
	var m driver.EventMask
	bit := func(k driver.EventKind, on bool) {
		if on {
			m |= 1 << k
		}
	}
	bit(driver.EventLoadStart, vn.loadStart != nil)
	bit(driver.EventLoad, vn.load != nil)
	bit(driver.EventProgress, vn.progress != nil)
	bit(driver.EventEnd, vn.end != nil)
	bit(driver.EventBuffer, vn.buffer != nil)
	bit(driver.EventVideoPause, vn.pause != nil)
	bit(driver.EventVideoPlay, vn.play != nil)
	bit(driver.EventNative, vn.pip != nil)
	bit(driver.EventError, vn.loadError != nil)
	return m
}

// MediaError is a video that failed to open or play. Message is the platform's words; Reason says why for the failures an app can act on.
type MediaError struct {
	Message string
	Reason  MediaErrorReason
}

func (e *MediaError) Error() string { return e.Message }

// MediaErrorReason says why a video failed (MediaError.Reason).
type MediaErrorReason uint8

const (
	// MediaErrorOther is any failure without a reason of its own: a missing file, a network error, a codec the platform lacks.
	MediaErrorOther MediaErrorReason = iota
	// MediaErrorAlphaUnsupported is a local file that declares an alpha channel the platform's player cannot keep, so it did not play rather than show an opaque picture; an app can decode such a clip itself and draw its frames with their alpha.
	MediaErrorAlphaUnsupported
)

// String names a media error reason for logs and diagnostics.
func (r MediaErrorReason) String() string {
	switch r {
	case MediaErrorOther:
		return "other"
	case MediaErrorAlphaUnsupported:
		return "alpha-unsupported"
	default:
		return "unknown"
	}
}

// Seek jumps the video r names to position. In the update that unpauses a video that played to its end, the video plays again from position.
func (r *Ref) Seek(position time.Duration) {
	if r.n == nil || r.n.deleted || r.n.video == nil {
		return
	}
	r.t.commands = append(r.t.commands, driver.MountItem{Kind: driver.ItemVideoSeek, ID: r.n.id, Time: max(position, 0)})
	r.t.changed = true
}

// emitSeeks sends the seeks of video n queued this round ahead of its new config, which keeps its source: a video that played to its end and is unpaused and sought in one update then plays from the new position, where AVPlayer, given the rate first, reaches the end again and stops before the seek. A seek in the update that mounts a video or changes its source stays after the config, so it applies to the source the config opens.
func (t *Tree) emitSeeks(n *node) {
	kept := t.commands[:0]
	for _, it := range t.commands {
		if it.Kind == driver.ItemVideoSeek && it.ID == n.id {
			t.items = append(t.items, it)
		} else {
			kept = append(kept, it)
		}
	}
	clear(t.commands[len(kept):])
	t.commands = kept
}
