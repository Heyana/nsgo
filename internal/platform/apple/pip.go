//go:build darwin

package apple

import (
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// PictureInPicture owns AVKit's session and its source layer. Only the
// main thread mutates it. The sample playback delegate reads an atomic
// snapshot, so AVKit never waits for the engine while it is suspended.
type PictureInPicture struct {
	events                      atomic.Pointer[pipEvents]
	controller, delegate, layer objc.ID
	state                       driver.PictureInPictureState
	automatic                   bool // the controller supports and has enabled automatic inline entry
	snapshot                    atomic.Pointer[driver.PictureInPictureConfig]
	custom, starting, closing   bool
	observing                   bool
	closed                      func()
	restoreTokens               []uint32
	framePending                bool
	timebase                    uintptr
	lastFrame                   time.Time
	lastStats                   time.Time
}

type pipEvents struct{ dispatch func(driver.Event) }

var pipDelegates sync.Map // objc.ID -> *PictureInPicture
var pipDelegateClass objc.Class

// cmTimeRange is CoreMedia's CMTimeRange. purego preserves its indirect
// return ABI on both arm64 devices and x86_64 simulators.
type cmTimeRange struct {
	_               structs.HostLayout
	Start, Duration cmTime
}

func pictureInPictureSupported() bool {
	if !loadAVKit() {
		return false
	}
	cls := Class("AVPictureInPictureController")
	return cls != 0 && objc.Send[bool](objc.ID(cls), sel("isPictureInPictureSupported"))
}

// NewPictureInPicture creates a player-layer session. A nil layer creates
// a sample-buffer session whose independent NSGo tree supplies frames.
func NewPictureInPicture(layer objc.ID, dispatch func(driver.Event)) *PictureInPicture {
	p := &PictureInPicture{custom: layer == 0}
	p.events.Store(&pipEvents{dispatch: dispatch})
	p.snapshot.Store(&driver.PictureInPictureConfig{})
	p.state.Supported = pictureInPictureSupported()
	// Custom sample-buffer playback is iOS-only: macOS has no custom-content path and tvOS still needs native acceptance.
	if p.custom && (runtime.GOOS != "ios" || television()) {
		p.state.Supported = false
	}
	if !p.state.Supported {
		p.report()
		return p
	}
	bundle := objc.ID(Class("NSBundle")).Send(sel("mainBundle"))
	modes := bundle.Send(sel("objectForInfoDictionaryKey:"), NSString("UIBackgroundModes"))
	if runtime.GOOS == "ios" && (modes == 0 || !objc.Send[bool](modes, sel("containsObject:"), NSString("audio"))) {
		p.fail("Picture in Picture requires apple.info.UIBackgroundModes: [audio]")
		return p
	}
	registerPiPDelegate()
	p.delegate = objc.ID(pipDelegateClass).Send(sel("new"))
	pipDelegates.Store(p.delegate, p)
	cls := objc.ID(Class("AVPictureInPictureController"))
	if p.custom {
		p.layer = objc.ID(Class("AVSampleBufferDisplayLayer")).Send(sel("new"))
		source := objc.ID(Class("AVPictureInPictureControllerContentSource")).Send(sel("alloc")).Send(sel("initWithSampleBufferDisplayLayer:playbackDelegate:"), p.layer, p.delegate)
		p.controller = cls.Send(sel("alloc")).Send(sel("initWithContentSource:"), source)
		source.Send(sel("release"))
	} else {
		p.layer = layer.Send(sel("retain"))
		p.controller = cls.Send(sel("alloc")).Send(sel("initWithPlayerLayer:"), layer)
	}
	if p.controller == 0 {
		p.fail("the system could not create a Picture in Picture controller")
		p.release()
		return p
	}
	p.controller.Send(sel("setDelegate:"), p.delegate)
	p.controller.Send(sel("addObserver:forKeyPath:options:context:"), p.delegate, NSString("pictureInPicturePossible"), uint(1), uintptr(0))
	p.observing = true
	// Keep playback active while PiP prepares or pauses, too. The audio coordinator restores the host's category after the last owner leaves.
	p.HoldAudio(driver.AudioSessionPlayback)
	p.update()
	p.report()
	return p
}

// pipStateEvent reports a session's state to its view.
func pipStateEvent(state driver.PictureInPictureState) driver.Event {
	return driver.Event{Kind: driver.EventNative, Text: driver.PictureInPictureEventName, Data: &driver.PictureInPictureEvent{State: state}}
}

func (p *PictureInPicture) report() {
	if sink := p.events.Load(); sink != nil && sink.dispatch != nil {
		sink.dispatch(pipStateEvent(p.state))
	}
}
func (p *PictureInPicture) action(action string, d time.Duration, token uint32) {
	if sink := p.events.Load(); sink != nil && sink.dispatch != nil {
		sink.dispatch(driver.Event{Kind: driver.EventNative, Text: driver.PictureInPictureEventName, Data: &driver.PictureInPictureEvent{Action: action, Time: d, Token: token}})
	}
}
func (p *PictureInPicture) fail(message string) {
	p.starting = false
	p.state.Error = message
	p.report()
}
func (p *PictureInPicture) update() {
	if p.controller == 0 || p.closing {
		return
	}
	possible := objc.Send[bool](p.controller, sel("isPictureInPicturePossible"))
	if p.state.Possible != possible {
		p.state.Possible = possible
		p.report()
	}
}

// ContinuesInBackground says whether the session plays on as the app enters the background: it is active or starting, or AVKit is about to start it automatically.
func (p *PictureInPicture) ContinuesInBackground() bool {
	if p.controller == 0 || p.closing {
		return false
	}
	if p.state.Active || p.starting || objc.Send[bool](p.controller, sel("isPictureInPictureActive")) {
		return true
	}
	return p.automatic && objc.Send[bool](p.controller, sel("isPictureInPicturePossible"))
}

// HoldAudio holds the shared audio session in the playback category PiP needs while the session lives, paused or not: mixing with other apps' audio for AudioSessionMixWithOthers, which a video asks for, and pausing it otherwise.
func (p *PictureInPicture) HoldAudio(session driver.AudioSession) {
	if p.controller == 0 || p.closing {
		return
	}
	if session != driver.AudioSessionMixWithOthers {
		session = driver.AudioSessionPlayback
	}
	audioPlaying(p, session, true)
}

// Configure updates the sample delegate's state without calling Go from
// an AVKit queue. Automatically is opt-in for either source kind.
func (p *PictureInPicture) Configure(c driver.PictureInPictureConfig) {
	p.snapshot.Store(&c)
	p.automatic = false
	if p.controller != 0 {
		// This public selector belongs to iOS; tvOS and macOS have no
		// automatic-inline-entry API, even if the controller supports PiP.
		auto := sel("setCanStartPictureInPictureAutomaticallyFromInline:")
		if runtime.GOOS == "ios" && !television() && objc.Send[bool](p.controller, sel("respondsToSelector:"), auto) {
			p.controller.Send(auto, c.Automatically)
			p.automatic = c.Automatically
		}
		if p.custom {
			p.configureTimebase(c)
			p.controller.Send(sel("invalidatePlaybackState"))
		}
	}
	p.update()
}

// Layer returns the source layer a custom session must mount inline for
// AVKit to recognize it as a possible PiP source.
func (p *PictureInPicture) Layer() objc.ID { return p.layer }

// Command starts or stops the native session. Repeated commands while a
// transition or session is already active do not start another one.
func (p *PictureInPicture) Command(command uint32) {
	if p.closing {
		return
	}
	switch command {
	case driver.PictureInPictureStart:
		if !p.state.Supported {
			p.report()
			return
		}
		if p.controller == 0 {
			p.fail("Picture in Picture has no content source")
			return
		}
		if p.state.Active || p.starting {
			return
		}
		p.update()
		if !p.state.Possible {
			p.fail("Picture in Picture is not possible yet")
			return
		}
		p.state.Error = ""
		p.starting = true
		p.controller.Send(sel("startPictureInPicture"))
	case driver.PictureInPictureStop:
		if p.controller != 0 && (p.state.Active || p.starting) {
			p.controller.Send(sel("stopPictureInPicture"))
		}
	}
}

// Close stops PiP and retains every source object until AVKit's stop
// callback. done releases the owning player's native objects afterward.
func (p *PictureInPicture) Close(done func()) {
	if p.closing {
		return
	}
	p.closing = true
	p.events.Store(nil)
	p.closed = done
	for _, token := range p.restoreTokens {
		completePiPRestore(token, false)
	}
	p.restoreTokens = nil
	if p.controller != 0 && (p.starting || p.state.Active) {
		p.controller.Send(sel("stopPictureInPicture"))
		return
	}
	p.release()
}
func (p *PictureInPicture) release() {
	audioPlaying(p, driver.AudioSessionDefault, false)
	if p.controller != 0 {
		if p.observing {
			p.controller.Send(sel("removeObserver:forKeyPath:"), p.delegate, NSString("pictureInPicturePossible"))
			p.observing = false
		}
		p.controller.Send(sel("setDelegate:"), objc.ID(0))
		p.controller.Send(sel("release"))
		p.controller = 0
	}
	if p.delegate != 0 {
		pipDelegates.Delete(p.delegate)
		p.delegate.Send(sel("release"))
		p.delegate = 0
	}
	if p.layer != 0 {
		if p.custom {
			p.layer.Send(sel("removeFromSuperlayer"))
		}
		p.layer.Send(sel("release"))
		p.layer = 0
	}
	if p.timebase != 0 {
		Release(p.timebase)
		p.timebase = 0
	}
	if done := p.closed; done != nil {
		p.closed = nil
		done()
	}
}

func pipFor(self objc.ID) *PictureInPicture {
	p, _ := pipDelegates.Load(self)
	v, _ := p.(*PictureInPicture)
	return v
}
func registerPiPDelegate() {
	if pipDelegateClass != 0 {
		return
	}
	var protocols []*objc.Protocol
	for _, name := range []string{"AVPictureInPictureControllerDelegate", "AVPictureInPictureSampleBufferPlaybackDelegate"} {
		if protocol := objc.GetProtocol(name); protocol != nil {
			protocols = append(protocols, protocol)
		}
	}
	pipDelegateClass = RegisterClass("NsgoPictureInPictureDelegate", Class("NSObject"), protocols, []objc.MethodDef{
		{Cmd: sel("observeValueForKeyPath:ofObject:change:context:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, controller objc.ID, _ objc.ID, _ uintptr) {
			if p := pipFor(self); p != nil {
				OnMain(func() {
					if p.controller == controller {
						p.update()
					}
				})
			}
		}},
		{Cmd: sel("pictureInPictureControllerDidStartPictureInPicture:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if p := pipFor(self); p != nil {
				p.starting = false
				p.state.Active = true
				p.state.Error = ""
				if p.closing {
					p.controller.Send(sel("stopPictureInPicture"))
				} else {
					p.report()
				}
			}
		}},
		{Cmd: sel("pictureInPictureControllerDidStopPictureInPicture:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if p := pipFor(self); p != nil {
				p.starting = false
				p.state.Active = false
				if p.closing {
					p.release()
				} else {
					p.report()
					p.update()
				}
			}
		}},
		{Cmd: sel("pictureInPictureController:failedToStartPictureInPictureWithError:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, err objc.ID) {
			if p := pipFor(self); p != nil {
				p.fail(GoString(err.Send(sel("localizedDescription"))))
				if p.closing {
					p.release()
				}
			}
		}},
		{Cmd: sel("pictureInPictureController:restoreUserInterfaceForPictureInPictureStopWithCompletionHandler:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, block objc.Block) {
			p := pipFor(self)
			if p == nil || p.closing || p.events.Load() == nil {
				callPiPBoolBlock(block, false)
				return
			}
			token := holdPiPRestore(block)
			p.restoreTokens = append(p.restoreTokens, token)
			p.action("restore", 0, token)
		}},
		{Cmd: sel("pictureInPictureController:setPlaying:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, playing bool) {
			if p := pipFor(self); p != nil {
				a := "pause"
				if playing {
					a = "play"
				}
				p.action(a, 0, 0)
			}
		}},
		{Cmd: sel("pictureInPictureControllerIsPlaybackPaused:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
			if p := pipFor(self); p != nil {
				return p.snapshot.Load().Paused
			}
			return true
		}},
		{Cmd: sel("pictureInPictureControllerTimeRangeForPlayback:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) cmTimeRange {
			if p := pipFor(self); p != nil {
				c := p.snapshot.Load()
				if c.Duration <= 0 {
					return cmTimeRange{Start: cmTimeOf(0), Duration: cmTime{Flags: 1 | 4}}
				} // positive infinity
				return cmTimeRange{Start: cmTimeOf(0), Duration: cmTimeOf(c.Duration)}
			}
			return cmTimeRange{}
		}},
		{Cmd: sel("pictureInPictureController:skipByInterval:completionHandler:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, interval cmTime, block objc.Block) {
			if p := pipFor(self); p != nil {
				p.action("skip", interval.duration(), 0)
			}
			callPiPBlock(block)
		}},
		{Cmd: sel("pictureInPictureController:didTransitionToRenderSize:"), Fn: func(objc.ID, objc.SEL, objc.ID, CGSize) {}},
	})
}

var restores struct {
	sync.Mutex
	next   uint32
	blocks map[uint32]objc.Block
}

func holdPiPRestore(block objc.Block) uint32 {
	restores.Lock()
	defer restores.Unlock()
	restores.next++
	if restores.blocks == nil {
		restores.blocks = map[uint32]objc.Block{}
	}
	token := restores.next
	restores.blocks[token] = block.Copy()
	// The owner may never commit again during suspension. Always answer
	// AVKit; a late engine acknowledgement safely becomes a no-op.
	time.AfterFunc(5*time.Second, func() { OnMain(func() { completePiPRestore(token, false) }) })
	return token
}
func completePiPRestore(token uint32, restored bool) {
	restores.Lock()
	b, ok := restores.blocks[token]
	delete(restores.blocks, token)
	restores.Unlock()
	if ok {
		callPiPBoolBlock(b, restored)
		b.Release()
	}
}

// CompletePictureInPictureRestore acknowledges an engine restoration
// after its mount batch has applied, even if that batch removed the view.
func CompletePictureInPictureRestore(token uint32) { completePiPRestore(token, true) }

// callPiPBoolBlock calls a completion handler AVKit gave, which purego's Block.Invoke cannot, with value.
func callPiPBoolBlock(block objc.Block, value bool) {
	var arg uintptr
	if value {
		arg = 1
	}
	CallBlock(objc.ID(block), arg)
}

// callPiPBlock calls a completion handler AVKit gave that takes no argument.
func callPiPBlock(block objc.Block) { CallBlock(objc.ID(block)) }

// PictureInPictureRestoreCompletion reserves the native completion for
// a mount batch. Owner deletion during that batch cannot cancel it.
func PictureInPictureRestoreCompletion(token uint32) func() {
	restores.Lock()
	block, ok := restores.blocks[token]
	delete(restores.blocks, token)
	restores.Unlock()
	if !ok {
		return func() {}
	}
	return func() { callPiPBoolBlock(block, true); block.Release() }
}
