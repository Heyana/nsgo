//go:build darwin

package apple

import (
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"structs"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha"
)

// Video is a video's AVPlayer, filling the view that holds it: in an AVPlayerLayer, or with the platform's controls in an AVPlayerView (macOS) or an AVPlayerViewController (iOS). AVFoundation and AVKit load on the first video, so apps without one do not pay for them. The layer is not opaque, so HEVC with alpha shows what is behind it; a local WebM file with alpha, which AVFoundation cannot open, reports EventError saying so.
type Video struct {
	// Dispatch reports the video's events.
	Dispatch func(driver.Event)
	// Parent is the view controller an iOS player's controls join when
	// the view is not in one yet.
	Parent objc.ID

	holder            objc.ID
	player            objc.ID
	shown             objc.ID // macOS: the layer view or the AVPlayerView
	layer             objc.ID // the AVPlayerLayer, without controls
	controller        objc.ID // iOS: the AVPlayerViewController, with them
	timeToken         objc.ID
	endToken          objc.ID
	rateToken         objc.ID
	stopInterruptions func()
	stopRoutes        func()
	stopBackground    func()
	status            *videoStatus
	config            driver.VideoConfig
	made              bool
	pip               *PictureInPicture
	pipLayer          objc.ID // iOS: the second AVPlayerLayer a video with controls starts PiP from
	pipHost           objc.ID // iOS: the view holding pipLayer in the controller's content overlay
	frame             pipVideo
}

// videoStatus is what a player reported so far.
type videoStatus struct {
	loaded, failed, buffering bool
	play                      videoPlayback
	// settling is until when the player may still post rate changes it finishes late, after the backend's own play or pause (asked); pending says one came meanwhile, for reason.
	settling time.Time
	pending  bool
	reason   int

	resource, audioResource *videoResource
	cancelLoading           func()
	seekWhileLoading        *time.Duration
}

// videoSettle is how long after the backend's own play or pause the player's rate changes are followed only once they settle. AVPlayer can finish a play only after a pause asked for right after it, and then post that play and the pause once more: on macOS 15 on Intel, a pause the app makes just after the controls played is followed, after it returned, by a play and, some 40ms later, a pause again.
const videoSettle = 500 * time.Millisecond

// asked records that the backend is about to play or pause the player itself.
func (st *videoStatus) asked() { st.settling = time.Now().Add(videoSettle) }

// cmTime is CoreMedia's CMTime.
type cmTime struct {
	_         structs.HostLayout
	Value     int64
	Timescale int32
	Flags     uint32
	Epoch     int64
}

func cmTimeOf(d time.Duration) cmTime {
	return cmTime{Value: int64(math.Round(d.Seconds() * 600)), Timescale: 600, Flags: 1}
}

// duration is t, or 0 when it is invalid or indefinite (live streams).
func (t cmTime) duration() time.Duration {
	const valid, indefinite = 1, 0x10
	if t.Flags&valid == 0 || t.Flags&indefinite != 0 || t.Timescale == 0 {
		return 0
	}
	return time.Duration(float64(t.Value) / float64(t.Timescale) * float64(time.Second))
}

var (
	avFoundation, avKit         bool
	avFoundationTried, kitTried bool
	classVideoLayerView         objc.Class
)

func loadAVFoundation() bool {
	if !avFoundationTried {
		avFoundationTried = true
		_, err := purego.Dlopen("/System/Library/Frameworks/AVFoundation.framework/AVFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		avFoundation = err == nil
		if avFoundation && runtime.GOOS != "ios" {
			// A layer-hosting view showing the player layer, which lets
			// the pointer through to the node's view.
			classVideoLayerView = RegisterClass("NsgoVideoLayerView", Class("NSView"), nil, []objc.MethodDef{
				{Cmd: sel("isFlipped"), Fn: func(objc.ID, objc.SEL) bool { return true }},
				{Cmd: sel("hitTest:"), Fn: func(objc.ID, objc.SEL, CGPoint) objc.ID { return 0 }},
			})
		}
	}
	return avFoundation
}

func loadAVKit() bool {
	if !kitTried {
		kitTried = true
		_, err := purego.Dlopen("/System/Library/Frameworks/AVKit.framework/AVKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		avKit = err == nil
	}
	return avKit
}

// playerClass is AVKit's class name when controls are asked for and
// AVKit loads, else 0.
func playerClass(controls bool, name string) objc.Class {
	if !controls || !loadAVKit() {
		return 0
	}
	return Class(name)
}

// NewVideo returns the video of holder, empty until configured.
func NewVideo(holder objc.ID) *Video {
	return &Video{holder: holder, status: &videoStatus{}}
}

// SetHolder moves the existing player layer or controls into holder.
func (v *Video) SetHolder(holder objc.ID) {
	if v.holder == holder {
		return
	}
	v.holder = holder
	WithoutAnimation(func() {
		if v.shown != 0 {
			fill(holder, v.shown)
		}
		if v.controller != 0 {
			fill(holder, v.controller.Send(sel("view")))
		}
		if runtime.GOOS == "ios" && v.layer != 0 {
			holder.Send(sel("layer")).Send(sel("addSublayer:"), v.layer)
			v.Layout()
		}
	})
}

// View returns the AppKit view showing the player (macOS), 0 before one.
func (v *Video) View() objc.ID { return v.shown }

// Configure applies c, opening the source again when it changed.
func (v *Video) Configure(c *driver.VideoConfig) {
	if !loadAVFoundation() {
		return
	}
	old, first := v.config, !v.made
	v.config, v.made = *c, true
	reopened := first || c.Source != old.Source || c.AudioSource != old.AudioSource || c.Headers != old.Headers
	if reopened {
		v.open()
	}
	if reopened || c.Controls != old.Controls {
		v.show()
	}
	if reopened || c.ProgressInterval != old.ProgressInterval {
		v.observe()
	}
	v.applyPlayback(old, reopened)
	v.applyGravity()
}

// applyPlayback applies the latest settings to a player with its item installed.
func (v *Video) applyPlayback(old driver.VideoConfig, reopened bool) {
	c := &v.config
	if v.player != 0 && v.player.Send(sel("currentItem")) != 0 {
		pausedChanged := reopened || c.Paused != old.Paused
		// A play or pause the controls made while the player settled is followed first, so the app's unchanged Paused is not applied over it.
		if !pausedChanged {
			v.settle(v.status)
		}
		play := &v.status.play
		v.player.Send(sel("setMuted:"), c.Muted)
		v.player.Send(sel("setVolume:"), c.Volume)
		// The controls, the PiP window and Control Center play at the player's default rate (macOS 13, iOS 16), which is the app's Rate: a play the app does not make keeps it.
		if objc.Send[bool](v.player, sel("respondsToSelector:"), sel("setDefaultRate:")) {
			v.player.Send(sel("setDefaultRate:"), c.Rate)
		}
		// What the controls or the system did holds until Paused changes (videoPlayback). The player posts its rate change before it returns, so the state it reaches is recorded first.
		switch {
		case !play.configure(pausedChanged):
			if play.playing && c.Rate != old.Rate {
				v.status.asked()
				v.player.Send(sel("setRate:"), c.Rate)
			}
		case c.Paused:
			play.applied(false)
			v.status.asked()
			v.player.Send(sel("pause"))
		default:
			// The controls' scrubber may have moved a completed player back: it plays on from there.
			if play.rewinds(v.position()) || v.atEnd() {
				v.player.Send(sel("seekToTime:"), cmTimeOf(0))
			}
			play.applied(true)
			v.status.asked()
			v.player.Send(sel("setRate:"), c.Rate)
		}
	}
	v.followAudio()
	v.configurePiP()
}

func (v *Video) applyGravity() {
	gravity := NSString("AVLayerVideoGravityResizeAspect")
	switch v.config.ResizeMode {
	case driver.Cover:
		gravity = NSString("AVLayerVideoGravityResizeAspectFill")
	case driver.Stretch:
		gravity = NSString("AVLayerVideoGravityResize")
	}
	switch {
	case v.layer != 0:
		v.layer.Send(sel("setVideoGravity:"), gravity)
	case v.controller != 0:
		v.controller.Send(sel("setVideoGravity:"), gravity)
	case v.shown != 0:
		v.shown.Send(sel("setVideoGravity:"), gravity)
	}
	if v.pipLayer != 0 {
		v.pipLayer.Send(sel("setVideoGravity:"), gravity)
	}
	v.layoutPiPVideo()
}

// locate finds source: "asset:" paths under root, a URL when it has a
// scheme, else a file path. ok is false when there is no root.
func locate(source, root string) (url, path string, ok bool) {
	if p, isAsset := strings.CutPrefix(source, "asset:"); isAsset {
		if root == "" {
			return "", "", false
		}
		return "", filepath.Join(root, filepath.FromSlash(strings.TrimLeft(p, "/"))), true
	}
	if strings.Contains(source, "://") {
		return source, "", true
	}
	return "", source, true
}

// videoURL is the NSURL of a located source: a URL, or a file path.
func videoURL(url, path string) objc.ID {
	nsURL := objc.ID(Class("NSURL"))
	if url != "" {
		return nsURL.Send(sel("URLWithString:"), NSString(url))
	}
	return nsURL.Send(sel("fileURLWithPath:"), NSString(path))
}

// local reports whether a located source is a file: a path, or a file URL.
func local(url string) bool {
	scheme, _, _ := strings.Cut(url, "://")
	return url == "" || strings.EqualFold(scheme, "file")
}

// videoAsset is the AVURLAsset of source, 0 when it has none. A local file
// asks for precise duration and timing: AVFoundation otherwise estimates the
// timing of a variable-bitrate file such as a FLAC, and a seek lands on a
// sample other than the one asked for while it still reports the time asked
// for, so what plays and what the player says disagree.
func videoAsset(source string) objc.ID {
	url, path, ok := locate(source, imageload.ResourceRoot)
	if !ok {
		return 0
	}
	options := objc.ID(0)
	if local(url) {
		options = objc.ID(Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"),
			objc.ID(Class("NSNumber")).Send(sel("numberWithBool:"), true),
			NSString("AVURLAssetPreferPreciseDurationAndTimingKey"))
	}
	location := videoURL(url, path)
	if location == 0 {
		return 0
	}
	return objc.ID(Class("AVURLAsset")).Send(sel("URLAssetWithURL:options:"), location, options)
}

// webmRefused says why a WebM file with alpha c does not play: AVFoundation opens no WebM, and keeps the alpha of HEVC with alpha in a QuickTime or MPEG-4 file instead, which the player layer composites over what is behind it.
func webmRefused(c videoalpha.Codec) string {
	return "AVFoundation cannot open WebM, so " + c.String() + " does not play on Apple platforms; use HEVC with alpha in a QuickTime file."
}

// open starts playing the source, or nothing.
func (v *Video) open() {
	v.close()
	st := &videoStatus{}
	v.status = st
	source := v.config.Source
	if source == "" {
		return
	}
	if c := videoalpha.Source(source, imageload.ResourceRoot); c == videoalpha.VP8 || c == videoalpha.VP9 {
		st.failed = true
		v.Dispatch(driver.Event{Kind: driver.EventError, Text: source + ": " + webmRefused(c), Index: driver.VideoErrorAlphaUnsupported})
		return
	}
	asset, resource := videoAssetWithHeaders(source, v.config.Headers)
	st.resource = resource
	if asset == 0 {
		st.failed = true
		v.Dispatch(driver.Event{Kind: driver.EventError, Text: source + ": cannot resolve the video's location"})
		return
	}
	v.Dispatch(driver.Event{Kind: driver.EventLoadStart})
	if v.config.AudioSource != "" {
		audio, audioResource := videoAssetWithHeaders(v.config.AudioSource, v.config.Headers)
		st.audioResource = audioResource
		if audio == 0 {
			st.failed = true
			v.Dispatch(driver.Event{Kind: driver.EventError, Text: "cannot resolve the separate audio source"})
			return
		}
		v.player = objc.ID(Class("AVPlayer")).Send(sel("alloc")).Send(sel("init"))
		st.cancelLoading = composeVideoAndAudio(asset, audio, func(composition objc.ID, err error) {
			if v.status != st {
				return
			}
			st.cancelLoading = nil
			if err != nil {
				st.failed = true
				v.Dispatch(driver.Event{Kind: driver.EventError, Text: err.Error()})
				return
			}
			v.installVideoItem(st, composition)
			if st.seekWhileLoading != nil {
				v.Seek(*st.seekWhileLoading)
				st.seekWhileLoading = nil
			}
			v.applyPlayback(v.config, true)
		})
		return
	}
	v.installVideoItem(st, asset)
}

func (v *Video) installVideoItem(st *videoStatus, asset objc.ID) {
	item := objc.ID(Class("AVPlayerItem")).Send(sel("playerItemWithAsset:"), asset)
	if v.player == 0 {
		v.player = objc.ID(Class("AVPlayer")).Send(sel("playerWithPlayerItem:"), item).Send(sel("retain"))
	} else {
		v.player.Send(sel("replaceCurrentItemWithPlayerItem:"), item)
	}
	// The end of the item: report it, and loop if asked.
	ended := objcblock.New(func(objc.Block, objc.ID) {
		if v.status != st || v.player == 0 {
			return
		}
		st.play.ended(v.config.Repeat, v.position())
		v.Dispatch(driver.Event{Kind: driver.EventEnd})
		if v.config.Repeat {
			st.asked()
			v.player.Send(sel("seekToTime:"), cmTimeOf(0))
			v.player.Send(sel("play"))
		}
		// Stopped, the other apps' audio may play again.
		v.followAudio()
	})
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	queue := objc.ID(Class("NSOperationQueue")).Send(selMainQueue)
	v.endToken = center.Send(selAddObserver, NSString("AVPlayerItemDidPlayToEndTimeNotification"),
		v.player.Send(sel("currentItem")), queue, ended).Send(sel("retain"))
	ended.Release()
	// The player's rate changes when its controls, the PiP window, Control Center or the system play or pause it (macOS 12 and iOS 15 say why).
	rated := objcblock.New(func(_ objc.Block, note objc.ID) {
		if v.status != st || v.player == 0 {
			return
		}
		reason := rateReason(note)
		// After the backend's own play or pause, what the player posts may be an earlier change it finished late: follow the rate it settles on.
		if wait := time.Until(st.settling); wait > 0 {
			if !st.pending {
				time.AfterFunc(wait, func() { OnMain(func() { v.settle(st) }) })
			}
			st.pending, st.reason = true, reason
			return
		}
		v.rateChanged(reason)
	})
	v.rateToken = center.Send(selAddObserver, NSString("AVPlayerRateDidChangeNotification"), v.player, queue, rated).Send(sel("retain"))
	rated.Release()
	v.watchSystemPauses(st)
	v.openPiPVideo()
	v.poll(st, 0)
}

// close stops the player and stops observing it. A macOS view keeps the
// last frame; an iOS one lets the player's layer or controller go.
func (v *Video) close() {
	st := v.status
	if st != nil && st.cancelLoading != nil {
		st.cancelLoading()
		st.cancelLoading = nil
	}
	v.closePiPVideo()
	audioPlaying(v, v.config.AudioSession, false)
	if v.player != 0 && v.timeToken != 0 {
		v.player.Send(sel("removeTimeObserver:"), v.timeToken)
	}
	if v.stopInterruptions != nil {
		v.stopInterruptions()
		v.stopInterruptions = nil
	}
	if v.stopRoutes != nil {
		v.stopRoutes()
		v.stopRoutes = nil
	}
	if v.stopBackground != nil {
		v.stopBackground()
		v.stopBackground = nil
	}
	if v.endToken != 0 || v.rateToken != 0 {
		center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
		for _, id := range []objc.ID{v.endToken, v.rateToken} {
			if id != 0 {
				center.Send(sel("removeObserver:"), id)
			}
		}
	}
	for _, id := range []objc.ID{v.timeToken, v.endToken, v.rateToken} {
		if id != 0 {
			id.Send(sel("release"))
		}
	}
	v.timeToken, v.endToken, v.rateToken = 0, 0, 0
	// The objects leave this Video before it opens again, but an active PiP session keeps its generation, the player and its layer or controller, until AVKit says it stopped.
	player, layer, controller, pipLayer, pipHost := v.player, v.layer, v.controller, v.pipLayer, v.pipHost
	v.player, v.pipLayer, v.pipHost = 0, 0, 0
	if runtime.GOOS == "ios" {
		v.layer, v.controller = 0, 0
	}
	release := func() {
		if st != nil {
			for _, resource := range []*videoResource{st.resource, st.audioResource} {
				if resource != nil {
					resource.close()
				}
			}
			st.resource, st.audioResource = nil, nil
		}
		if player != 0 {
			player.Send(sel("pause"))
			player.Send(sel("release"))
		}
		if runtime.GOOS == "ios" {
			releasePlayerViews(layer, controller)
		}
		releasePiPLayer(pipLayer, pipHost)
	}
	if p := v.pip; p != nil {
		v.pip = nil
		p.Close(release)
	} else {
		release()
	}
}

// show shows the player in a layer, or with the platform's controls.
func (v *Video) show() {
	if runtime.GOOS == "ios" {
		if v.frame.owner != nil {
			v.endPiP()
			v.detach()
		} else {
			v.attach()
		}
		return
	}
	if v.shown != 0 {
		v.shown.Send(sel("removeFromSuperview"))
		v.shown.Send(sel("release"))
		v.shown = 0
	}
	if v.layer != 0 {
		v.layer.Send(sel("release"))
		v.layer = 0
	}
	if v.player == 0 {
		return
	}
	var shown objc.ID
	if c := playerClass(v.config.Controls, "AVPlayerView"); c != 0 {
		shown = objc.ID(c).Send(sel("alloc")).Send(sel("initWithFrame:"), CGRect{})
		shown.Send(sel("setPlayer:"), v.player)
		const inline = 1 // AVPlayerViewControlsStyleInline
		shown.Send(sel("setControlsStyle:"), inline)
	} else if c := Class("AVPlayerLayer"); c != 0 {
		v.layer = objc.ID(c).Send(sel("playerLayerWithPlayer:"), v.player).Send(sel("retain"))
		shown = objc.ID(classVideoLayerView).Send(sel("alloc")).Send(sel("initWithFrame:"), CGRect{})
		// Layer-hosting: the player layer is the view's own, so it
		// follows the view's size.
		shown.Send(sel("setLayer:"), v.layer)
		shown.Send(sel("setWantsLayer:"), true)
	} else {
		return
	}
	WithoutAnimation(func() { fill(v.holder, shown) })
	v.shown = shown
}

// attach shows an iOS player in a layer, or in an
// AVPlayerViewController with its controls.
func (v *Video) attach() {
	v.endPiP()
	v.detach()
	if v.player == 0 {
		return
	}
	bounds := objc.Send[CGRect](v.holder, sel("bounds"))
	if c := playerClass(v.config.Controls, "AVPlayerViewController"); c != 0 {
		ctrl := objc.ID(c).Send(sel("new"))
		ctrl.Send(sel("setPlayer:"), v.player)
		ctrl.Send(sel("setShowsPlaybackControls:"), true)
		if view := ctrl.Send(sel("view")); view != 0 {
			view.Send(sel("setFrame:"), bounds)
			fill(v.holder, view)
		}
		if parent := v.parentController(); parent != 0 {
			parent.Send(sel("addChildViewController:"), ctrl)
			ctrl.Send(sel("didMoveToParentViewController:"), parent)
		}
		v.controller = ctrl
	} else if c := Class("AVPlayerLayer"); c != 0 {
		v.layer = objc.ID(c).Send(sel("playerLayerWithPlayer:"), v.player).Send(sel("retain"))
		WithoutAnimation(func() {
			v.layer.Send(sel("setFrame:"), bounds)
			v.holder.Send(sel("layer")).Send(sel("addSublayer:"), v.layer)
		})
	}
}

// parentController is the nearest view controller of the holder, else
// Parent: the view joins the window after its video is configured.
func (v *Video) parentController() objc.ID {
	vc := Class("UIViewController")
	for r := v.holder.Send(sel("nextResponder")); r != 0; r = r.Send(sel("nextResponder")) {
		if objc.Send[bool](r, sel("isKindOfClass:"), vc) {
			return r
		}
	}
	return v.Parent
}

// detach undoes attach.
func (v *Video) detach() {
	releasePlayerViews(v.layer, v.controller)
	v.layer, v.controller = 0, 0
}

// releasePlayerViews removes and releases an iOS player's layer and controller, either of which may be 0.
func releasePlayerViews(layer, controller objc.ID) {
	if layer != 0 {
		layer.Send(sel("removeFromSuperlayer"))
		layer.Send(sel("release"))
	}
	if controller != 0 {
		controller.Send(sel("willMoveToParentViewController:"), objc.ID(0))
		controller.Send(sel("view")).Send(sel("removeFromSuperview"))
		controller.Send(sel("removeFromParentViewController"))
		controller.Send(sel("release"))
	}
}

// fill adds view to holder at its size, resizing with it.
func fill(holder, view objc.ID) {
	view.Send(sel("setFrame:"), objc.Send[CGRect](holder, sel("bounds")))
	const flexibleSize = 2 | 16 // width and height, in AppKit and UIKit alike
	view.Send(sel("setAutoresizingMask:"), uint(flexibleSize))
	holder.Send(sel("addSubview:"), view)
}

// Layout keeps an iOS player layer at the view's size.
func (v *Video) Layout() {
	v.layoutPiPVideo()
	if v.pipLayer != 0 {
		WithoutAnimation(func() { v.pipLayer.Send(sel("setFrame:"), objc.Send[CGRect](v.pipHost, sel("bounds"))) })
	}
	if runtime.GOOS == "ios" && v.layer != 0 {
		WithoutAnimation(func() { v.layer.Send(sel("setFrame:"), objc.Send[CGRect](v.holder, sel("bounds"))) })
	}
}

// observe reports the progress every ProgressInterval while time passes,
// and the load, buffering and failure as they happen.
func (v *Video) observe() {
	if v.player == 0 {
		return
	}
	if v.timeToken != 0 {
		v.player.Send(sel("removeTimeObserver:"), v.timeToken)
		v.timeToken.Send(sel("release"))
		v.timeToken = 0
	}
	st := v.status
	// The block's CMTime argument is left out: the player's current time
	// is the same, and purego callbacks take no structs.
	tick := objcblock.New(func(objc.Block) {
		if v.status != st || v.player == 0 {
			return
		}
		v.report()
		at := max(objc.Send[cmTime](v.player, sel("currentTime")).duration(), 0)
		v.Dispatch(driver.Event{Kind: driver.EventProgress, Time: at, Duration: v.itemDuration()})
	})
	// A nil queue is the main queue.
	v.timeToken = v.player.Send(sel("addPeriodicTimeObserverForInterval:queue:usingBlock:"),
		cmTimeOf(v.config.ProgressInterval), objc.ID(0), tick).Send(sel("retain"))
	tick.Release()
}

// avName returns an Apple framework's exported NSString constant, falling back to the constant's documented value on systems that do not export it.
func avName(name string) objc.ID {
	for _, path := range []string{
		"/System/Library/Frameworks/AVFoundation.framework/AVFoundation",
		"/System/Library/Frameworks/AVFAudio.framework/AVFAudio",
	} {
		if id := Global(path, name); id != 0 {
			return id
		}
	}
	return NSString(name)
}

// watchSystemPauses follows the iOS audio session's interruptions and lost routes, which pause the player: they say why before the player says it stopped, and when an interruption ends whether the video may play on. It also pauses a playing video as the app enters the background. macOS has none of these, and hears only of its controls.
func (v *Video) watchSystemPauses(st *videoStatus) {
	if runtime.GOOS != "ios" {
		return
	}
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	v.stopInterruptions = Observe(center, avName("AVAudioSessionInterruptionNotification"), func(note objc.ID) {
		if v.status != st || v.player == 0 {
			return
		}
		const began, shouldResume = 1, 1 // AVAudioSessionInterruptionTypeBegan, AVAudioSessionInterruptionOptionShouldResume
		if notificationInt(note, "AVAudioSessionInterruptionTypeKey") == began {
			v.systemPause(driver.VideoPauseInterruption)
		} else if notificationInt(note, "AVAudioSessionInterruptionOptionKey")&shouldResume != 0 && st.play.resume() {
			st.asked()
			v.player.Send(sel("setRate:"), v.config.Rate)
			v.rateChanged(driver.VideoPauseInterruption)
		}
	})
	// iOS pauses a playing video as the app enters the background, reporting it as an audio-session interruption rather than AVPlayerRateDidChangeReasonAppBackgrounded, and plays it again as the app comes back. The backend pauses it first, for the background, so it stays paused as the design says, unless its Picture in Picture plays it on.
	v.stopBackground = Observe(center, NSString("UIApplicationDidEnterBackgroundNotification"), func(objc.ID) {
		if v.status == st && v.player != 0 && st.play.playing && !v.continuesInPictureInPicture() {
			v.systemPause(videoPauseBackground)
		}
	})
	v.stopRoutes = Observe(center, avName("AVAudioSessionRouteChangeNotification"), func(note objc.ID) {
		const oldDeviceUnavailable = 2 // AVAudioSessionRouteChangeReasonOldDeviceUnavailable
		// A muted video has nothing to stop when the output goes away.
		if v.status == st && v.player != 0 && !v.config.Muted && notificationInt(note, "AVAudioSessionRouteChangeReasonKey") == oldDeviceUnavailable {
			v.systemPause(driver.VideoPauseRouteLost)
		}
	})
}

func (v *Video) continuesInPictureInPicture() bool {
	return (v.pip != nil && v.pip.ContinuesInBackground()) ||
		(v.frame.owner != nil && v.frame.owner.ContinuesInBackground())
}

func notificationInt(note objc.ID, key string) int {
	info := note.Send(sel("userInfo"))
	if info == 0 {
		return 0
	}
	number := info.Send(selObjectForKey, avName(key))
	if number == 0 {
		return 0
	}
	return objc.Send[int](number, sel("integerValue"))
}

// rateReason is why an AVPlayerRateDidChangeNotification says the rate changed, as a pause's reason: an interruption for the audio session's, videoPauseBackground for the app going to the background, which videoPlayback reports as an interruption too, else the controls, which set the rate.
func rateReason(note objc.ID) int {
	info := note.Send(sel("userInfo"))
	if info == 0 {
		return driver.VideoPauseControls
	}
	why := info.Send(selObjectForKey, avName("AVPlayerRateDidChangeReasonKey"))
	if why == 0 {
		return driver.VideoPauseControls
	}
	for _, reason := range []struct {
		name string
		kind int
	}{
		{"AVPlayerRateDidChangeReasonAudioSessionInterrupted", driver.VideoPauseInterruption},
		{"AVPlayerRateDidChangeReasonAppBackgrounded", videoPauseBackground},
	} {
		if objc.Send[bool](why, sel("isEqual:"), avName(reason.name)) {
			return reason.kind
		}
	}
	return driver.VideoPauseControls
}

// systemPause pauses a playing player for reason, as the system is about to, and reports it: the session's notification may come before or after the player's own.
func (v *Video) systemPause(reason int) {
	st := v.status
	st.play.expect(reason)
	if st.play.playing {
		st.asked()
		v.player.Send(sel("pause"))
	}
	v.rateChanged(reason)
}

// settle follows the rate the player settled on, when it posted rate changes while it settled after the backend's own play or pause.
func (v *Video) settle(st *videoStatus) {
	if v.status == st && v.player != 0 && st.pending {
		st.pending = false
		v.rateChanged(st.reason)
	}
}

// rateChanged follows a play or pause the player made on its own, reporting it once; reason is a pause's unless the system said why before. A pause at the item's end is EventEnd's.
func (v *Video) rateChanged(reason int) {
	st := v.status
	playing := objc.Send[float32](v.player, sel("rate")) != 0
	if !playing && st.play.playing && v.atEnd() {
		return
	}
	if ev, ok := st.play.changed(playing, v.config.Paused, reason); ok {
		v.followAudio()
		v.requestPiPVideoFrame()
		v.Dispatch(ev)
	}
}

// atEnd reports whether the player is at the end of a finite item.
func (v *Video) atEnd() bool {
	d := v.itemDuration()
	return d > 0 && v.position() >= d
}

// position is the player's current time.
func (v *Video) position() time.Duration {
	return objc.Send[cmTime](v.player, sel("currentTime")).duration()
}

// followAudio holds the video's audio session while it plays, and lets it go when it does not: the session follows what plays, not what is mounted.
func (v *Video) followAudio() {
	audioPlaying(v, v.config.AudioSession, v.player != 0 && v.status != nil && v.status.play.playing)
}

// poll reports the player's state every 100ms until it loaded or failed,
// for at most 30 seconds: a paused player reports no time to the periodic
// observer.
func (v *Video) poll(st *videoStatus, attempt int) {
	if v.status != st || v.player == 0 {
		return
	}
	v.report()
	if st.loaded || st.failed || attempt >= 300 {
		return
	}
	time.AfterFunc(100*time.Millisecond, func() { OnMain(func() { v.poll(st, attempt+1) }) })
}

func (v *Video) itemDuration() time.Duration {
	item := v.player.Send(sel("currentItem"))
	if item == 0 {
		return 0
	}
	return objc.Send[cmTime](item, sel("duration")).duration()
}

// report reports a finished load and a failure, each once, and buffering
// changes.
func (v *Video) report() {
	if v.pip != nil {
		v.pip.update()
	}
	item := v.player.Send(sel("currentItem"))
	if item == 0 {
		return
	}
	st := v.status
	const ready, failed = 1, 2 // AVPlayerItemStatus
	switch state := objc.Send[int](item, sel("status")); {
	case state == ready && !st.loaded:
		st.loaded = true
		v.requestPiPVideoFrame()
		size := objc.Send[CGSize](item, sel("presentationSize"))
		v.Dispatch(driver.Event{Kind: driver.EventLoad, Duration: v.itemDuration(), RateChangeable: true,
			Size: driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)}})
	case state == failed && !st.failed:
		st.failed = true
		message := "the video failed to load"
		if err := item.Send(sel("error")); err != 0 {
			message = GoString(err.Send(sel("localizedDescription")))
		}
		v.Dispatch(driver.Event{Kind: driver.EventError, Text: message})
	}
	const waitingToPlay = 1 // AVPlayerTimeControlStatusWaitingToPlayAtSpecifiedRate
	waiting := objc.Send[int](v.player, sel("timeControlStatus")) == waitingToPlay && !st.failed
	if st.buffering != waiting {
		st.buffering = waiting
		v.Dispatch(driver.Event{Kind: driver.EventBuffer, Buffering: waiting})
	}
}

// Seek jumps to position exactly.
func (v *Video) Seek(position time.Duration) {
	position = max(position, 0)
	if st := v.status; st != nil && st.cancelLoading != nil {
		st.seekWhileLoading = &position
		return
	}
	if v.player != 0 {
		play := &v.status.play
		replay := play.completed && position < v.itemDuration()
		if replay {
			play.completed = false
		}
		zero := cmTimeOf(0)
		if v.frame.output != 0 {
			st := v.status
			complete := objcblock.New(func(_ objc.Block, finished bool) {
				if finished {
					OnMain(func() {
						if v.status == st {
							v.requestPiPVideoFrame()
						}
					})
				}
			})
			v.player.Send(sel("seekToTime:toleranceBefore:toleranceAfter:completionHandler:"), cmTimeOf(position), zero, zero, complete)
			complete.Release()
		} else {
			v.player.Send(sel("seekToTime:toleranceBefore:toleranceAfter:"), cmTimeOf(position), zero, zero)
		}
		if replay && !v.config.Paused {
			play.applied(true)
			v.status.asked()
			v.player.Send(sel("setRate:"), v.config.Rate)
			v.followAudio()
		}
	}
}

// Close stops the player. The view keeps the last frame while it still
// shows.
func (v *Video) Close() {
	v.close()
	v.frame = pipVideo{}
	v.status = nil
	for _, id := range []objc.ID{v.shown, v.layer} {
		if id != 0 {
			id.Send(sel("release"))
		}
	}
	v.shown, v.layer = 0, 0
}

// configurePiP gives a video with PictureInPicture its PiP session: from its own player layer, or, with controls, from a second layer in the controller's content overlay that code can start and stop.
func (v *Video) configurePiP() {
	if runtime.GOOS != "ios" {
		return
	}
	if v.frame.owner != nil {
		return
	}
	enabled := v.config.PictureInPicture
	if v.controller != 0 {
		// One session owns this player. AVPlayerViewController has no public stop, and its own PiP button would start a second session nothing owns, so every start, an automatic one included, goes through the overlay's layer.
		v.controller.Send(sel("setAllowsPictureInPicturePlayback:"), false)
		if objc.Send[bool](v.controller, sel("respondsToSelector:"), sel("setCanStartPictureInPictureAutomaticallyFromInline:")) {
			v.controller.Send(sel("setCanStartPictureInPictureAutomaticallyFromInline:"), false)
		}
	}
	if !enabled || v.player == 0 {
		v.endPiP()
		if enabled && v.Dispatch != nil {
			v.Dispatch(pipStateEvent(driver.PictureInPictureState{Supported: pictureInPictureSupported()}))
		}
		return
	}
	if v.pip == nil {
		layer := v.layer
		if layer == 0 {
			v.pipLayer = objc.ID(Class("AVPlayerLayer")).Send(sel("playerLayerWithPlayer:"), v.player).Send(sel("retain"))
			layer = v.pipLayer
			v.pipHost = newPiPPlayerHost(v.controller.Send(sel("contentOverlayView")), layer)
		}
		v.pip = NewPictureInPicture(layer, v.Dispatch)
	}
	v.pip.Configure(driver.PictureInPictureConfig{Automatically: v.config.AutoPictureInPicture})
	v.pip.HoldAudio(v.config.AudioSession)
}

// PictureInPictureCommand starts or stops the video's PiP session.
func (v *Video) PictureInPictureCommand(command uint32) {
	if v.pip != nil {
		v.pip.Command(command)
	} else if v.Dispatch != nil {
		v.Dispatch(pipStateEvent(driver.PictureInPictureState{Supported: pictureInPictureSupported(), Error: "Picture in Picture needs an enabled video with a playable source"}))
	}
}

// endPiP ends the video's PiP session and lets its extra layer go once the session stopped, leaving the video's own layer.
func (v *Video) endPiP() {
	layer, host := v.pipLayer, v.pipHost
	v.pipLayer, v.pipHost = 0, 0
	release := func() { releasePiPLayer(layer, host) }
	if p := v.pip; p != nil {
		v.pip = nil
		p.Close(release)
	} else {
		release()
	}
}

// releasePiPLayer removes and releases a controlled video's PiP layer and its host view, either of which may be 0.
func releasePiPLayer(layer, host objc.ID) {
	if host != 0 {
		host.Send(sel("removeFromSuperview"))
		host.Send(sel("release"))
	}
	if layer != 0 {
		layer.Send(sel("removeFromSuperlayer"))
		layer.Send(sel("release"))
	}
}
