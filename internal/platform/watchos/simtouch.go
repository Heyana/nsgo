//go:build ios && (nsgotest || nsgoautomation)

package watchos

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Native touch injection for the watchOS simulator. Under SwiftUI on the
// watchOS simulator, the app runs inside a UIApplication with a hosting
// UIWindow that accepts IOHID digitizer events via -[UIApplication
// _enqueueHIDEvent:]. Posting real digitizer events reaches SwiftUI's own
// ScrollView, TextField and DragGesture handlers as a finger does. On physical
// watches or whenever the private simulator symbols or key window are
// unavailable, watchHIDReady returns false and testSurface falls back cleanly.

var (
	watchHIDMu        sync.Mutex
	watchHIDLoaded    bool
	watchHIDBroken    atomic.Bool
	watchHIDDigitizer func(alloc uintptr, time uint64, transducer, index, identity, mask, buttons uint32, x, y, z, tip, barrel float64, inRange, touching uint8, options uint32) uintptr
	watchHIDFinger    func(alloc uintptr, time uint64, index, identity, mask uint32, x, y, z, tip, twist float64, inRange, touching uint8, options uint32) uintptr
	watchHIDAppend    func(parent, child uintptr, options uint32)
	watchHIDSetInt    func(event uintptr, field uint32, value int64)
	watchHIDSetFloat  func(event uintptr, field uint32, value float64)
	watchHIDSetSender func(event uintptr, sender uint64)
	watchBKSDigitizer func(event uintptr, context uint32, gesturePossible, gestureChange uint8, display uintptr, initial float64, force float32)
	watchMachTime     func() uint64
)

func loadWatchHID() error {
	watchHIDMu.Lock()
	defer watchHIDMu.Unlock()
	if watchHIDLoaded {
		return nil
	}
	open := func(path string) (uintptr, error) {
		return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	}
	iokit, err := open("/System/Library/Frameworks/IOKit.framework/IOKit")
	if err != nil {
		return err
	}
	bbs, err := open("/System/Library/PrivateFrameworks/BackBoardServices.framework/BackBoardServices")
	if err != nil {
		return err
	}
	system, err := open("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&watchHIDDigitizer, iokit, "IOHIDEventCreateDigitizerEvent")
	purego.RegisterLibFunc(&watchHIDFinger, iokit, "IOHIDEventCreateDigitizerFingerEvent")
	purego.RegisterLibFunc(&watchHIDAppend, iokit, "IOHIDEventAppendEvent")
	purego.RegisterLibFunc(&watchHIDSetInt, iokit, "IOHIDEventSetIntegerValue")
	purego.RegisterLibFunc(&watchHIDSetFloat, iokit, "IOHIDEventSetFloatValue")
	purego.RegisterLibFunc(&watchHIDSetSender, iokit, "IOHIDEventSetSenderID")
	purego.RegisterLibFunc(&watchBKSDigitizer, bbs, "BKSHIDEventSetDigitizerInfo")
	purego.RegisterLibFunc(&watchMachTime, system, "mach_absolute_time")
	watchHIDLoaded = true
	return nil
}

func watchResponds(obj objc.ID, selName string) bool {
	if obj == 0 {
		return false
	}
	return objc.Send[bool](obj, objc.RegisterName("respondsToSelector:"), objc.RegisterName(selName))
}

func watchWindowContext(w objc.ID) uint32 {
	if !watchResponds(w, "_contextId") {
		return 0
	}
	if watchResponds(w, "isHidden") && objc.Send[bool](w, objc.RegisterName("isHidden")) {
		return 0
	}
	return objc.Send[uint32](w, objc.RegisterName("_contextId"))
}

// watchHostWindowTarget returns the shared UIApplication and the active
// hosting UIWindow's CoreAnimation context ID, or (0, 0) when unavailable.
// Must be called on the main thread.
func watchHostWindowTarget() (app objc.ID, contextID uint32) {
	cls := apple.Class("UIApplication")
	if cls == 0 {
		return 0, 0
	}
	app = objc.ID(cls).Send(objc.RegisterName("sharedApplication"))
	if !watchResponds(app, "_enqueueHIDEvent:") {
		return 0, 0
	}
	if watchResponds(app, "connectedScenes") {
		scenes := app.Send(objc.RegisterName("connectedScenes"))
		apple.ForEach(scenes, func(scene objc.ID) {
			if contextID != 0 {
				return
			}
			if watchResponds(scene, "keyWindow") {
				if ctx := watchWindowContext(scene.Send(objc.RegisterName("keyWindow"))); ctx != 0 {
					contextID = ctx
					return
				}
			}
			if watchResponds(scene, "windows") {
				apple.ForEach(scene.Send(objc.RegisterName("windows")), func(w objc.ID) {
					if contextID == 0 {
						contextID = watchWindowContext(w)
					}
				})
			}
		})
	}
	if contextID == 0 && watchResponds(app, "keyWindow") {
		contextID = watchWindowContext(app.Send(objc.RegisterName("keyWindow")))
	}
	if contextID == 0 && watchResponds(app, "windows") {
		apple.ForEach(app.Send(objc.RegisterName("windows")), func(w objc.ID) {
			if contextID == 0 {
				contextID = watchWindowContext(w)
			}
		})
	}
	if contextID == 0 {
		return 0, 0
	}
	return app, contextID
}

// watchHIDReady reports whether native simulator HID touches can be posted to
// SwiftUI's hosting window.
func watchHIDReady() bool {
	return watchShouldUseNativeHID(
		watchIsSimulatorEnv(os.Getenv),
		watchHIDBroken.Load(),
		loadWatchHID,
		func() bool {
			var ok bool
			onMainSync(func() {
				_, ctx := watchHostWindowTarget()
				ok = ctx != 0
			})
			return ok
		},
	)
}

// postWatchHIDTouchOnMain enqueues one IOHID digitizer touch event at p in ph
// into the hosting UIWindow on the main thread. Returns false if the window or
// HID allocator was unavailable.
func postWatchHIDTouchOnMain(p driver.Point, ph watchHIDPhase) bool {
	app, ctxID := watchHostWindowTarget()
	if app == 0 || ctxID == 0 || watchHIDDigitizer == nil || watchHIDFinger == nil {
		return false
	}
	touching, mask := watchTouchPhaseHID(ph)
	now := watchMachTime()
	hand := watchHIDDigitizer(0, now, watchHIDTransducerHand, 0, 0, mask, 0, 0, 0, 0, 0, 0, touching, touching, 0)
	if hand == 0 {
		return false
	}
	watchHIDSetInt(hand, watchHIDFieldIntegrated, 1)
	finger := watchHIDFinger(0, now, 1, 2, mask, float64(p.X), float64(p.Y), 0, float64(touching), 0, touching, touching, 0)
	if finger == 0 {
		apple.Release(hand)
		return false
	}
	watchHIDSetFloat(finger, watchHIDFieldMajorRadius, 5)
	watchHIDSetFloat(finger, watchHIDFieldMinorRadius, 5)
	watchHIDSetInt(finger, watchHIDFieldIntegrated, 1)
	watchHIDAppend(hand, finger, 0)
	apple.Release(finger)

	watchHIDSetSender(hand, watchHIDTestSenderID)
	watchBKSDigitizer(hand, ctxID, 0, 0, 0, 0, 0)
	app.Send(objc.RegisterName("_enqueueHIDEvent:"), hand)
	apple.Release(hand)
	return true
}
