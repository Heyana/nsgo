//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Trusted web view input on iOS using public UIKit/WebKit classes only:
// taps are delivered as UITouch/UIEvent subclasses through WKWebView's
// content view and its single-tap/touch gesture recognizers, hardware keys
// and US-layout text as UIKey/UIPress/UIPressesEvent subclasses through
// UIResponder's pressesBegan:withEvent:/pressesEnded:withEvent:, and non-US
// Unicode text through UIKeyInput's insertText:.
var (
	webInputOnce sync.Once
	webInputMu   sync.Mutex

	classWebTouch      objc.Class
	classWebTouchEvent objc.Class
	classWebKey        objc.Class
	classWebPress      objc.Class
	classWebPressEvent objc.Class

	webTouches     = map[objc.ID]*webTouchState{}
	webTouchEvents = map[objc.ID]*webTouchEventState{}
	webKeys        = map[objc.ID]*webKeyState{}
	webPresses     = map[objc.ID]*webPressState{}
	webPressEvents = map[objc.ID]*webPressEventState{}
)

type webTouchState struct {
	window    objc.ID
	view      objc.ID
	winPt     apple.CGPoint
	timestamp float64
	phase     int
	force     float64
}

type webTouchEventState struct {
	touches   objc.ID // retained NSSet
	timestamp float64
}

type webKeyState struct {
	chars         string
	plain         string
	modifierFlags int
	keyCode       int
}

type webPressState struct {
	window    objc.ID
	responder objc.ID
	key       objc.ID // retained NsgoWebKey
	timestamp float64
	phase     int
	pressType int
	force     float64
}

type webPressEventState struct {
	presses       objc.ID // retained NSSet
	timestamp     float64
	modifierFlags int
}

func registerWebInputClasses() {
	webInputOnce.Do(func() {
		freed := func(forget func(objc.ID)) objc.MethodDef {
			return objc.MethodDef{Cmd: sel("dealloc"), Fn: func(self objc.ID, cmd objc.SEL) {
				forget(self)
				apple.Super(self, cmd)
			}}
		}
		emptyArray := func(objc.ID, objc.SEL) objc.ID {
			return objc.ID(apple.Class("NSArray")).Send(sel("array"))
		}
		emptySet := func(objc.ID, objc.SEL) objc.ID {
			return objc.ID(apple.Class("NSSet")).Send(sel("set"))
		}
		touchLoc := func(self objc.ID, _ objc.SEL, view objc.ID) apple.CGPoint {
			webInputMu.Lock()
			st := webTouches[self]
			webInputMu.Unlock()
			if st == nil {
				return apple.CGPoint{}
			}
			if view == 0 || view == st.window {
				return st.winPt
			}
			return objc.Send[apple.CGPoint](view, sel("convertPoint:fromView:"), st.winPt, st.window)
		}
		if super := apple.Class("UITouch"); super != 0 {
			classWebTouch = register("NsgoWebTouch", super, nil, []objc.MethodDef{
				freed(func(self objc.ID) {
					webInputMu.Lock()
					delete(webTouches, self)
					webInputMu.Unlock()
				}),
				{Cmd: sel("timestamp"), Fn: func(self objc.ID, _ objc.SEL) float64 {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webTouches[self]; st != nil {
						return st.timestamp
					}
					return 0
				}},
				{Cmd: sel("phase"), Fn: func(self objc.ID, _ objc.SEL) int {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webTouches[self]; st != nil {
						return st.phase
					}
					return 0
				}},
				{Cmd: sel("tapCount"), Fn: func(objc.ID, objc.SEL) uint { return 1 }},
				{Cmd: sel("type"), Fn: func(objc.ID, objc.SEL) int { return 0 }},
				{Cmd: sel("window"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webTouches[self]; st != nil {
						return st.window
					}
					return 0
				}},
				{Cmd: sel("view"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webTouches[self]; st != nil {
						return st.view
					}
					return 0
				}},
				{Cmd: sel("gestureRecognizers"), Fn: emptyArray},
				{Cmd: sel("force"), Fn: func(self objc.ID, _ objc.SEL) float64 {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webTouches[self]; st != nil {
						return st.force
					}
					return 0
				}},
				{Cmd: sel("maximumPossibleForce"), Fn: func(objc.ID, objc.SEL) float64 { return 1 }},
				{Cmd: sel("majorRadius"), Fn: func(objc.ID, objc.SEL) float64 { return 5 }},
				{Cmd: sel("majorRadiusTolerance"), Fn: func(objc.ID, objc.SEL) float64 { return 1 }},
				{Cmd: sel("locationInView:"), Fn: touchLoc},
				{Cmd: sel("previousLocationInView:"), Fn: touchLoc},
				{Cmd: sel("preciseLocationInView:"), Fn: touchLoc},
				{Cmd: sel("precisePreviousLocationInView:"), Fn: touchLoc},
			})
		}
		if super := apple.Class("UIEvent"); super != 0 {
			eventTouches := func(self objc.ID, _ objc.SEL, _ objc.ID) objc.ID {
				webInputMu.Lock()
				st := webTouchEvents[self]
				webInputMu.Unlock()
				if st != nil && st.touches != 0 {
					return st.touches
				}
				return objc.ID(apple.Class("NSSet")).Send(sel("set"))
			}
			classWebTouchEvent = register("NsgoWebTouchEvent", super, nil, []objc.MethodDef{
				freed(func(self objc.ID) {
					webInputMu.Lock()
					st := webTouchEvents[self]
					delete(webTouchEvents, self)
					webInputMu.Unlock()
					if st != nil && st.touches != 0 {
						st.touches.Send(selRelease)
					}
				}),
				{Cmd: sel("type"), Fn: func(objc.ID, objc.SEL) int { return 0 }},
				{Cmd: sel("subtype"), Fn: func(objc.ID, objc.SEL) int { return 0 }},
				{Cmd: sel("timestamp"), Fn: func(self objc.ID, _ objc.SEL) float64 {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webTouchEvents[self]; st != nil {
						return st.timestamp
					}
					return 0
				}},
				{Cmd: sel("allTouches"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					st := webTouchEvents[self]
					webInputMu.Unlock()
					if st != nil && st.touches != 0 {
						return st.touches
					}
					return objc.ID(apple.Class("NSSet")).Send(sel("set"))
				}},
				{Cmd: sel("touchesForWindow:"), Fn: eventTouches},
				{Cmd: sel("touchesForView:"), Fn: eventTouches},
				{Cmd: sel("touchesForGestureRecognizer:"), Fn: eventTouches},
			})
		}
		if super := apple.Class("UIKey"); super != 0 {
			classWebKey = register("NsgoWebKey", super, nil, []objc.MethodDef{
				freed(func(self objc.ID) {
					webInputMu.Lock()
					delete(webKeys, self)
					webInputMu.Unlock()
				}),
				{Cmd: sel("copyWithZone:"), Fn: func(self objc.ID, _ objc.SEL, _ uintptr) objc.ID {
					return self.Send(sel("retain"))
				}},
				{Cmd: sel("characters"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					st := webKeys[self]
					webInputMu.Unlock()
					if st != nil {
						return apple.NSString(st.chars)
					}
					return apple.NSString("")
				}},
				{Cmd: sel("charactersIgnoringModifiers"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					st := webKeys[self]
					webInputMu.Unlock()
					if st != nil {
						return apple.NSString(st.plain)
					}
					return apple.NSString("")
				}},
				{Cmd: sel("modifierFlags"), Fn: func(self objc.ID, _ objc.SEL) int {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webKeys[self]; st != nil {
						return st.modifierFlags
					}
					return 0
				}},
				{Cmd: sel("keyCode"), Fn: func(self objc.ID, _ objc.SEL) int {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webKeys[self]; st != nil {
						return st.keyCode
					}
					return 0
				}},
			})
		}
		if super := apple.Class("UIPress"); super != 0 {
			classWebPress = register("NsgoWebPress", super, nil, []objc.MethodDef{
				freed(func(self objc.ID) {
					webInputMu.Lock()
					st := webPresses[self]
					delete(webPresses, self)
					webInputMu.Unlock()
					if st != nil && st.key != 0 {
						st.key.Send(selRelease)
					}
				}),
				{Cmd: sel("timestamp"), Fn: func(self objc.ID, _ objc.SEL) float64 {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.timestamp
					}
					return 0
				}},
				{Cmd: sel("phase"), Fn: func(self objc.ID, _ objc.SEL) int {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.phase
					}
					return 0
				}},
				{Cmd: sel("type"), Fn: func(self objc.ID, _ objc.SEL) int {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.pressType
					}
					return webIOSPressTypeNone
				}},
				{Cmd: sel("window"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.window
					}
					return 0
				}},
				{Cmd: sel("responder"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.responder
					}
					return 0
				}},
				{Cmd: sel("gestureRecognizers"), Fn: emptyArray},
				{Cmd: sel("force"), Fn: func(self objc.ID, _ objc.SEL) float64 {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.force
					}
					return 0
				}},
				{Cmd: sel("key"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPresses[self]; st != nil {
						return st.key
					}
					return 0
				}},
			})
		}
		if super := apple.Class("UIPressesEvent"); super != 0 {
			classWebPressEvent = register("NsgoWebPressesEvent", super, nil, []objc.MethodDef{
				freed(func(self objc.ID) {
					webInputMu.Lock()
					st := webPressEvents[self]
					delete(webPressEvents, self)
					webInputMu.Unlock()
					if st != nil && st.presses != 0 {
						st.presses.Send(selRelease)
					}
				}),
				{Cmd: sel("type"), Fn: func(objc.ID, objc.SEL) int { return 3 }},
				{Cmd: sel("subtype"), Fn: func(objc.ID, objc.SEL) int { return 0 }},
				{Cmd: sel("timestamp"), Fn: func(self objc.ID, _ objc.SEL) float64 {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPressEvents[self]; st != nil {
						return st.timestamp
					}
					return 0
				}},
				{Cmd: sel("modifierFlags"), Fn: func(self objc.ID, _ objc.SEL) int {
					webInputMu.Lock()
					defer webInputMu.Unlock()
					if st := webPressEvents[self]; st != nil {
						return st.modifierFlags
					}
					return 0
				}},
				{Cmd: sel("allPresses"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
					webInputMu.Lock()
					st := webPressEvents[self]
					webInputMu.Unlock()
					if st != nil && st.presses != 0 {
						return st.presses
					}
					return objc.ID(apple.Class("NSSet")).Send(sel("set"))
				}},
				{Cmd: sel("pressesForGestureRecognizer:"), Fn: func(objc.ID, objc.SEL, objc.ID) objc.ID {
					return emptySet(0, 0)
				}},
			})
		}
	})
}

func webUptime() float64 {
	return objc.Send[float64](objc.ID(apple.Class("NSProcessInfo")).Send(sel("processInfo")), sel("systemUptime"))
}

// webContentView returns WKWebView's interactive content view inside its
// scrollView (the subview conforming to UIKeyInput), falling back to web.
func webContentView(web objc.ID) objc.ID {
	scroll := web.Send(sel("scrollView"))
	if scroll == 0 {
		return web
	}
	subs := scroll.Send(sel("subviews"))
	var fallback objc.ID
	for i, n := uint(0), objc.Send[uint](subs, sel("count")); i < n; i++ {
		sub := subs.Send(sel("objectAtIndex:"), i)
		if objc.Send[bool](sub, sel("respondsToSelector:"), sel("insertText:")) {
			return sub
		}
		if fallback == 0 && objc.Send[bool](sub, sel("respondsToSelector:"), sel("pressesBegan:withEvent:")) {
			fallback = sub
		}
	}
	if fallback != 0 {
		return fallback
	}
	return web
}

// sendWebInput delivers trusted input to web, a WKWebView on iOS, taking
// keyboard focus first as a tap does.
func sendWebInput(web objc.ID, in driver.WebInput) {
	if web == 0 {
		return
	}
	window := web.Send(sel("window"))
	if window == 0 {
		return
	}
	registerWebInputClasses()
	content := webContentView(web)
	if !objc.Send[bool](content, sel("isFirstResponder")) && objc.Send[bool](content, sel("canBecomeFirstResponder")) {
		content.Send(selBecomeFirst)
	} else if content != web && !objc.Send[bool](web, sel("isFirstResponder")) && objc.Send[bool](web, sel("canBecomeFirstResponder")) {
		web.Send(selBecomeFirst)
	}
	switch in.Kind {
	case driver.WebInputClick:
		sendWebTap(web, content, window, in.Point)
	case driver.WebInputText:
		for _, step := range webIOSTextSteps(in.Text) {
			if step.insertText {
				if objc.Send[bool](content, sel("respondsToSelector:"), sel("insertText:")) {
					content.Send(sel("insertText:"), apple.NSString(step.chars))
				}
				continue
			}
			sendWebKeyPress(content, window, step)
		}
	case driver.WebInputKey:
		spec, ok := webIOSKeyLookup(in.Key)
		if !ok {
			return
		}
		if spec.insertText {
			if objc.Send[bool](content, sel("respondsToSelector:"), sel("insertText:")) {
				content.Send(sel("insertText:"), apple.NSString(spec.chars))
			}
			return
		}
		sendWebKeyPress(content, window, spec)
	}
}

func sendWebTap(web, content, window objc.ID, pt driver.Point) {
	if classWebTouch == 0 || classWebTouchEvent == 0 {
		return
	}
	zoom := 1.0
	if objc.Send[bool](web, sel("respondsToSelector:"), sel("pageZoom")) {
		zoom = objc.Send[float64](web, sel("pageZoom"))
	}
	vx, vy := webClickViewPoint(pt, zoom)
	webPt := apple.CGPoint{X: vx, Y: vy}
	winPt := objc.Send[apple.CGPoint](web, sel("convertPoint:toView:"), webPt, objc.ID(0))
	target := web.Send(sel("hitTest:withEvent:"), webPt, objc.ID(0))
	if target == 0 {
		target = content
	}
	now := webUptime()
	touch := objc.ID(classWebTouch).Send(selAlloc).Send(sel("init"))
	st := &webTouchState{window: window, view: target, winPt: winPt, timestamp: now, phase: 0, force: 1}
	webInputMu.Lock()
	webTouches[touch] = st
	webInputMu.Unlock()
	touchSet := objc.ID(apple.Class("NSSet")).Send(sel("setWithObject:"), touch)
	event := objc.ID(classWebTouchEvent).Send(selAlloc).Send(sel("init"))
	evState := &webTouchEventState{touches: touchSet.Send(sel("retain")), timestamp: now}
	webInputMu.Lock()
	webTouchEvents[event] = evState
	webInputMu.Unlock()
	defer func() {
		event.Send(selRelease)
		touch.Send(selRelease)
	}()

	recognizers := collectWebTapRecognizers(content, target)
	if len(recognizers) > 0 {
		for _, gr := range recognizers {
			gr.Send(sel("touchesBegan:withEvent:"), touchSet, event)
		}
	} else {
		target.Send(sel("touchesBegan:withEvent:"), touchSet, event)
	}
	endTime := webUptime()
	webInputMu.Lock()
	st.phase = touchEnded
	st.force = 0
	st.timestamp = endTime
	evState.timestamp = endTime
	webInputMu.Unlock()
	if len(recognizers) > 0 {
		for _, gr := range recognizers {
			gr.Send(sel("touchesEnded:withEvent:"), touchSet, event)
		}
	} else {
		target.Send(sel("touchesEnded:withEvent:"), touchSet, event)
	}
}

func collectWebTapRecognizers(content, target objc.ID) []objc.ID {
	tapClass := objc.ID(apple.Class("UITapGestureRecognizer"))
	excludedClasses := []objc.ID{
		objc.ID(apple.Class("UILongPressGestureRecognizer")),
		objc.ID(apple.Class("UIPanGestureRecognizer")),
		objc.ID(apple.Class("UISwipeGestureRecognizer")),
		objc.ID(apple.Class("UIPinchGestureRecognizer")),
		objc.ID(apple.Class("UIRotationGestureRecognizer")),
		objc.ID(apple.Class("UIHoverGestureRecognizer")),
	}
	seen := map[objc.ID]bool{}
	var out []objc.ID
	collect := func(v objc.ID) {
		if v == 0 {
			return
		}
		list := v.Send(sel("gestureRecognizers"))
		if list == 0 {
			return
		}
		for i, n := uint(0), objc.Send[uint](list, sel("count")); i < n; i++ {
			gr := list.Send(sel("objectAtIndex:"), i)
			if seen[gr] {
				continue
			}
			seen[gr] = true
			info := webTapRecognizerInfo{
				enabled: objc.Send[bool](gr, sel("isEnabled")),
			}
			if tapClass != 0 && objc.Send[bool](gr, selIsKindOf, tapClass) {
				info.isTap = true
				info.tapsRequired = int(objc.Send[uint](gr, sel("numberOfTapsRequired")))
				info.touchesRequired = int(objc.Send[uint](gr, sel("numberOfTouchesRequired")))
			}
			for _, ex := range excludedClasses {
				if ex != 0 && objc.Send[bool](gr, selIsKindOf, ex) {
					info.excluded = true
					break
				}
			}
			if shouldDeliverWebTap(info) {
				out = append(out, gr)
			}
		}
	}
	collect(content)
	if target != content {
		collect(target)
	}
	return out
}

func sendWebKeyPress(target, window objc.ID, spec webIOSKeySpec) {
	if classWebKey == 0 || classWebPress == 0 || classWebPressEvent == 0 {
		return
	}
	now := webUptime()
	key := objc.ID(classWebKey).Send(selAlloc).Send(sel("init"))
	webInputMu.Lock()
	webKeys[key] = &webKeyState{
		chars:         spec.chars,
		plain:         spec.plain,
		modifierFlags: spec.modifierFlags,
		keyCode:       spec.usage,
	}
	webInputMu.Unlock()

	press := objc.ID(classWebPress).Send(selAlloc).Send(sel("init"))
	pState := &webPressState{
		window:    window,
		responder: target,
		key:       key,
		timestamp: now,
		phase:     0, // UIPressPhaseBegan
		pressType: spec.pressType,
		force:     1,
	}
	webInputMu.Lock()
	webPresses[press] = pState
	webInputMu.Unlock()

	pressSet := objc.ID(apple.Class("NSSet")).Send(sel("setWithObject:"), press)
	event := objc.ID(classWebPressEvent).Send(selAlloc).Send(sel("init"))
	eState := &webPressEventState{
		presses:       pressSet.Send(sel("retain")),
		timestamp:     now,
		modifierFlags: spec.modifierFlags,
	}
	webInputMu.Lock()
	webPressEvents[event] = eState
	webInputMu.Unlock()
	defer func() {
		event.Send(selRelease)
		press.Send(selRelease)
	}()

	target.Send(sel("pressesBegan:withEvent:"), pressSet, event)
	endTime := webUptime()
	webInputMu.Lock()
	pState.phase = 3 // UIPressPhaseEnded
	pState.force = 0
	pState.timestamp = endTime
	eState.timestamp = endTime
	webInputMu.Unlock()
	target.Send(sel("pressesEnded:withEvent:"), pressSet, event)
}
