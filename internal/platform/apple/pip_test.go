//go:build darwin

package apple

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

func TestPictureInPictureAutomaticEntryAvailability(t *testing.T) {
	var configured bool
	cls := RegisterClass("NsgoTestPiPAutomaticController", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("isPictureInPicturePossible"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("isPictureInPictureActive"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("setCanStartPictureInPictureAutomaticallyFromInline:"), Fn: func(_ objc.ID, _ objc.SEL, value bool) { configured = value }},
	})
	p := &PictureInPicture{controller: objc.ID(cls).Send(sel("new"))}
	defer p.controller.Send(sel("release"))
	p.Configure(driver.PictureInPictureConfig{Automatically: true})
	want := runtime.GOOS == "ios" && !television()
	if configured != want || p.ContinuesInBackground() != want {
		t.Fatalf("automatic configuration = %v, background playback = %v, want %v", configured, p.ContinuesInBackground(), want)
	}
	p.Configure(driver.PictureInPictureConfig{})
	if p.ContinuesInBackground() {
		t.Fatal("disabling automatic entry kept inline playback alive in the background")
	}
	p.starting = true
	if !p.ContinuesInBackground() {
		t.Fatal("a starting session did not keep playback alive")
	}
}

func TestPictureInPictureWithoutAutomaticEntrySelector(t *testing.T) {
	cls := RegisterClass("NsgoTestPiPManualController", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("isPictureInPicturePossible"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("isPictureInPictureActive"), Fn: func(objc.ID, objc.SEL) bool { return false }},
	})
	p := &PictureInPicture{controller: objc.ID(cls).Send(sel("new"))}
	defer p.controller.Send(sel("release"))
	p.Configure(driver.PictureInPictureConfig{Automatically: true})
	if p.ContinuesInBackground() {
		t.Fatal("an unsupported automatic-entry request kept inline playback alive")
	}
}

func TestVideoPictureInPictureRetainsNowPlayingUntilStopped(t *testing.T) {
	loadAVKit()
	registerPiPDelegate()
	cls := RegisterClass("NsgoTestPiPNowPlayingController", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("stopPictureInPicture"), Fn: func(objc.ID, objc.SEL) {}},
		{Cmd: sel("setDelegate:"), Fn: func(objc.ID, objc.SEL, objc.ID) {}},
	})
	for _, closeVideo := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%v", closeVideo), func(t *testing.T) {
			// Keep a test reference so the count can be inspected after the
			// video releases its own reference at the stop callback.
			session := objc.ID(Class("NSObject")).Send(sel("new"))
			defer session.Send(sel("release"))
			p := &PictureInPicture{controller: objc.ID(cls).Send(sel("new")), state: driver.PictureInPictureState{Supported: true, Active: true}}
			p.delegate = objc.ID(pipDelegateClass).Send(sel("new"))
			pipDelegates.Store(p.delegate, p)
			v := &Video{pip: p, pipNowPlaying: session.Send(sel("retain"))}
			before := objc.Send[uint](session, sel("retainCount"))
			if closeVideo {
				v.close()
			} else {
				v.endPiP()
			}
			if v.pip != nil || v.pipNowPlaying != 0 || objc.Send[uint](session, sel("retainCount")) != before {
				t.Fatal("closing the generation released its Now Playing session before PiP stopped")
			}
			p.delegate.Send(sel("pictureInPictureControllerDidStopPictureInPicture:"), p.controller)
			if objc.Send[uint](session, sel("retainCount")) != before-1 {
				t.Fatal("the stopped generation did not release its Now Playing session")
			}
		})
	}
}

func TestPictureInPictureDelegateAndOwnership(t *testing.T) {
	loadAVKit()
	registerPiPDelegate()
	var starts, stops int
	cls := RegisterClass("NsgoTestPictureInPictureController", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("isPictureInPicturePossible"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("startPictureInPicture"), Fn: func(objc.ID, objc.SEL) { starts++ }},
		{Cmd: sel("stopPictureInPicture"), Fn: func(objc.ID, objc.SEL) { stops++ }},
		{Cmd: sel("setDelegate:"), Fn: func(objc.ID, objc.SEL, objc.ID) {}},
	})
	var events []driver.PictureInPictureEvent
	p := &PictureInPicture{controller: objc.ID(cls).Send(sel("new")), state: driver.PictureInPictureState{Supported: true}}
	p.events.Store(&pipEvents{dispatch: func(e driver.Event) { events = append(events, *e.Data.(*driver.PictureInPictureEvent)) }})
	p.snapshot.Store(&driver.PictureInPictureConfig{Paused: true, Duration: time.Minute})
	p.delegate = objc.ID(pipDelegateClass).Send(sel("new"))
	pipDelegates.Store(p.delegate, p)
	delegate := p.delegate
	p.Command(driver.PictureInPictureStart)
	p.Command(driver.PictureInPictureStart)
	if starts != 1 {
		t.Fatalf("started %d times during a pending transition", starts)
	}
	delegate.Send(sel("pictureInPictureControllerDidStartPictureInPicture:"), p.controller)
	p.Command(driver.PictureInPictureStart)
	if starts != 1 || !p.state.Active {
		t.Fatal("active start was not idempotent")
	}
	if !objc.Send[bool](delegate, sel("pictureInPictureControllerIsPlaybackPaused:"), p.controller) {
		t.Fatal("delegate lost paused state")
	}
	r := objc.Send[cmTimeRange](delegate, sel("pictureInPictureControllerTimeRangeForPlayback:"), p.controller)
	if r.Duration.duration() != time.Minute {
		t.Fatalf("delegate range %+v", r)
	}
	var skipped bool
	skip := objcblock.New(func(objc.Block) { skipped = true })
	defer skip.Release()
	delegate.Send(sel("pictureInPictureController:skipByInterval:completionHandler:"), p.controller, cmTimeOf(-15*time.Second), skip)
	if !skipped || events[len(events)-1].Time != -15*time.Second {
		t.Fatal("skip did not complete promptly")
	}
	var restored, answered bool
	restore := objcblock.New(func(_ objc.Block, ok bool) { answered = true; restored = ok })
	defer restore.Release()
	delegate.Send(sel("pictureInPictureController:restoreUserInterfaceForPictureInPictureStopWithCompletionHandler:"), p.controller, restore)
	if answered {
		t.Fatal("restoration completed before the engine committed")
	}
	CompletePictureInPictureRestore(events[len(events)-1].Token)
	if !answered || !restored {
		t.Fatal("restoration was not acknowledged")
	}
	released := false
	p.Close(func() { released = true })
	if released || stops != 1 {
		t.Fatal("source objects released before AVKit stopped")
	}
	delegate.Send(sel("pictureInPictureControllerDidStopPictureInPicture:"), p.controller)
	if !released || p.controller != 0 || p.delegate != 0 {
		t.Fatal("stopped session leaked source objects")
	}
}

func TestPictureInPictureFramesContainUpdatedPixels(t *testing.T) {
	layer := objc.ID(Class("CALayer")).Send(sel("new"))
	defer layer.Send(sel("release"))
	layer.Send(sel("setBounds:"), Rect(0, 0, 4, 2))
	cm := load("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	var getPixels func(uintptr) uintptr
	purego.RegisterLibFunc(&getPixels, cm, "CMSampleBufferGetImageBuffer")
	for _, color := range []struct {
		r, g, b float64
		want    [3]byte
	}{{1, 0, 0, [3]byte{0, 0, 255}}, {0, 0, 1, [3]byte{255, 0, 0}}} {
		c := cgColorCreateSRGB(color.r, color.g, color.b, 1)
		WithoutAnimation(func() { layer.Send(sel("setBackgroundColor:"), c) })
		Release(c)
		sample, err := sampleOfLayer(layer, 8, 4, 4, 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		pixels := getPixels(sample)
		if cvPixelBufferLock(pixels, 0) != 0 {
			t.Fatal("cannot read sample pixels")
		}
		address := cvPixelBufferBase(pixels)
		bytes := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&address))), 3)
		got := [3]byte{bytes[0], bytes[1], bytes[2]}
		cvPixelBufferUnlock(pixels, 0)
		Release(sample)
		if got != color.want {
			t.Fatalf("BGRA frame %v, want %v", got, color.want)
		}
	}
}

func TestPictureInPictureImageRefreshesWithoutEngineCommit(t *testing.T) {
	layer := objc.ID(Class("CALayer")).Send(sel("new"))
	defer layer.Send(sel("release"))
	layer.Send(sel("setBounds:"), Rect(0, 0, 4, 2))
	iv := NewImageView(layer)
	defer iv.Close()
	iv.Layout(CGSize{Width: 4, Height: 2})
	p := &PictureInPicture{}
	iv.Changed = p.Dirty
	cm := load("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	var getPixels func(uintptr) uintptr
	purego.RegisterLibFunc(&getPixels, cm, "CMSampleBufferGetImageBuffer")
	for _, color := range []struct {
		fill string
		want [3]byte
	}{{"red", [3]byte{0, 0, 255}}, {"blue", [3]byte{255, 0, 0}}} {
		img, err := decode(t.Context(), []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="4" height="2"><rect width="4" height="2" fill="%s"/></svg>`, color.fill)), request{})
		if err != nil {
			t.Fatal(err)
		}
		p.framePending = false
		// The asynchronous loader ends here, without another engine batch.
		iv.show(img, true, 0)
		if !p.framePending {
			t.Fatal("loaded image did not invalidate the PiP frame")
		}
		sample, err := sampleOfLayer(layer, 4, 2, 4, 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		pixels := getPixels(sample)
		if cvPixelBufferLock(pixels, 0) != 0 {
			t.Fatal("cannot read image sample pixels")
		}
		address := cvPixelBufferBase(pixels)
		b := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&address))), 3)
		got := [3]byte{b[0], b[1], b[2]}
		cvPixelBufferUnlock(pixels, 0)
		Release(sample)
		if got != color.want {
			t.Fatalf("loaded %s image frame %v, want %v", color.fill, got, color.want)
		}
	}
}

func TestPictureInPictureCoalescesFramesUnderBackpressure(t *testing.T) {
	loadAVKit()
	ready := false
	enqueued := 0
	controllerClass := RegisterClass("NsgoTestPictureInPictureFrameController", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("isPictureInPicturePossible"), Fn: func(objc.ID, objc.SEL) bool { return true }},
	})
	layerClass := RegisterClass("NsgoTestPictureInPictureFrameLayer", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("status"), Fn: func(objc.ID, objc.SEL) int { return 1 }},
		{Cmd: sel("isReadyForMoreMediaData"), Fn: func(objc.ID, objc.SEL) bool { return ready }},
		{Cmd: sel("enqueueSampleBuffer:"), Fn: func(objc.ID, objc.SEL, uintptr) { enqueued++ }},
	})
	p := &PictureInPicture{custom: true, controller: objc.ID(controllerClass).Send(sel("new")), layer: objc.ID(layerClass).Send(sel("new"))}
	defer p.controller.Send(sel("release"))
	defer p.layer.Send(sel("release"))
	p.snapshot.Store(&driver.PictureInPictureConfig{Size: driver.Size{Width: 4, Height: 2}})
	tree := objc.ID(Class("CALayer")).Send(sel("new"))
	defer tree.Send(sel("release"))
	tree.Send(sel("setBounds:"), Rect(0, 0, 4, 2))
	for range 20 {
		p.Dirty()
		if !p.DrawFrame(tree, 1) {
			t.Fatal("backpressure discarded the latest pending frame")
		}
	}
	if enqueued != 0 {
		t.Fatal("frames were enqueued while the display layer was full")
	}
	ready = true
	if p.DrawFrame(tree, 1) || enqueued != 1 {
		t.Fatal("the latest frame did not replace all stale frames")
	}
	p.DrawFrame(tree, 1)
	if enqueued != 1 {
		t.Fatal("an unchanged frame was rendered again")
	}
}

func TestPictureInPictureRestoreSurvivesOwnerDeletion(t *testing.T) {
	var calls int
	var restored bool
	block := objcblock.New(func(_ objc.Block, ok bool) { calls++; restored = ok })
	defer block.Release()
	token := holdPiPRestore(block)
	done := PictureInPictureRestoreCompletion(token)
	// Closing the owner may race with applying the restoration batch.
	completePiPRestore(token, false)
	if calls != 0 {
		t.Fatal("owner deletion answered an acknowledged restoration")
	}
	done()
	completePiPRestore(token, false)
	if calls != 1 || !restored {
		t.Fatalf("restore answered %d times, success %v", calls, restored)
	}
}

func TestPictureInPictureDisablingKeepsInlineLayer(t *testing.T) {
	parent := objc.ID(Class("CALayer")).Send(sel("new"))
	defer parent.Send(sel("release"))
	layer := objc.ID(Class("CALayer")).Send(sel("new"))
	defer layer.Send(sel("release"))
	parent.Send(sel("addSublayer:"), layer)
	p := &PictureInPicture{layer: layer.Send(sel("retain"))}
	p.Close(nil)
	if layer.Send(sel("superlayer")) != parent {
		t.Fatal("disabling PiP removed the video's inline layer")
	}
}
