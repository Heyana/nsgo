//go:build ios

package ios

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// The focused input is kept clear of the software keyboard by the
// innermost vertical scroll view around it, as NSH's: what the keyboard
// covers of the scroll view adds to its bottom inset, and the view
// scrolls the input above the keyboard, on the keyboard's own duration.

// keyboardMargin is how far above the keyboard an input is kept.
const keyboardMargin = 8

// keyboardWindow receives the window on the active responder chain, on the main thread.
var keyboardWindow objc.ID

// onWindowEvent is installed by testsurface.go (nsgotest/nsgoautomation) to observe completed window events on the main thread.
var onWindowEvent func(window, event objc.ID)

func activeKeyboardWindow() objc.ID {
	const foregroundActive = 0
	var active objc.ID
	for _, s := range scenes {
		w := s.window
		if w == 0 || objc.Send[int](w.Send(sel("windowScene")), sel("activationState")) != foregroundActive {
			continue
		}
		if active != 0 {
			return responderWindow()
		}
		active = w
	}
	return active
}

func responderWindow() objc.ID {
	// A window can be key in every scene, including a background scene.
	// When several scenes are active, ask UIKit which responder chain has input.
	keyboardWindow = 0
	app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
	app.Send(sel("sendAction:to:from:forEvent:"), sel("nsgoKeyboardWindow:"), objc.ID(0), objc.ID(0), objc.ID(0))
	return keyboardWindow
}

// keyboardMoved follows the software keyboard's frame and animation in the window receiving input.
func (s *surface) keyboardMoved(note objc.ID, hiding bool) {
	w := s.uiWindow()
	if w == 0 {
		return
	}
	info := note.Send(sel("userInfo"))
	get := func(key string) objc.ID { return info.Send(sel("objectForKey:"), apple.Global(uikitPath, key)) }
	s.keyboardFrame = apple.CGRect{}
	if frame := get("UIKeyboardFrameEndUserInfoKey"); !hiding && frame != 0 && (s.embedded || len(scenes) < 2 || w == activeKeyboardWindow()) {
		s.keyboardFrame = objc.Send[apple.CGRect](frame, sel("CGRectValue"))
	}
	seconds := objc.Send[float64](get("UIKeyboardAnimationDurationUserInfoKey"), sel("doubleValue"))
	s.setKeyboard(s.keyboardOverlap(), time.Duration(seconds*float64(time.Second)))
}

// keyboardOverlap measures the notified keyboard in the current surface, not
// the surface's size and position when the notification arrived.
func (s *surface) keyboardOverlap() driver.Points {
	w := s.uiWindow()
	if w == 0 || s.keyboardFrame.Size.Width <= 0 || s.keyboardFrame.Size.Height <= 0 {
		return 0
	}
	space := w.Send(sel("screen")).Send(sel("coordinateSpace"))
	local := objc.Send[apple.CGRect](s.native, sel("convertRect:fromCoordinateSpace:"), s.keyboardFrame, space)
	b := objc.Send[apple.CGRect](s.native, selBounds)
	if local.Origin.X >= b.Origin.X+b.Size.Width || local.Origin.X+local.Size.Width <= b.Origin.X {
		return 0
	}
	return driver.Points(max(min(b.Origin.Y+b.Size.Height, local.Origin.Y+local.Size.Height)-max(b.Origin.Y, local.Origin.Y), 0))
}

func (s *surface) setKeyboard(height driver.Points, duration time.Duration) {
	if height == 0 && s.keyboard == 0 {
		return
	}
	s.keyboard = height
	s.keyboardDuration = duration
	s.keyboardStart = s.clock()
	s.refresh()
	s.keepFocusedClear()
}

// keepFocusedClear follows the keyboard and the focus.
func (s *surface) keepFocusedClear() {
	var field *view
	if s.keyboard > 0 {
		for _, v := range s.views {
			if v != nil && v.input != nil && v.input.focused {
				field = v
				break
			}
		}
	}
	var scroll *view
	if field != nil {
		for n := field.native.Send(selSuperview); n != 0 && n != s.native; n = n.Send(selSuperview) {
			if v := views[n]; v != nil && v.native == n && v.sc != nil && !v.sc.config.Horizontal {
				scroll = v
				break
			}
		}
	}
	if old := s.keyboardScroll; old != nil && old != scroll && s.views[old.id] == old {
		old.sc.keyboard = 0
		old.setInset(old.sc.config.Behavior.ContentInset)
	}
	s.keyboardScroll = scroll
	if scroll == nil {
		return
	}
	bounds := objc.Send[apple.CGRect](scroll.native, selBounds)
	inSurface := objc.Send[apple.CGRect](s.native, sel("convertRect:fromView:"), bounds, scroll.native)
	top := objc.Send[apple.CGRect](s.native, selBounds).Size.Height - float64(s.keyboard)
	scroll.sc.keyboard = driver.Points(max(inSurface.Origin.Y+inSurface.Size.Height-top, 0))
	scroll.setInset(scroll.sc.config.Behavior.ContentInset)
	r := objc.Send[apple.CGRect](scroll.native, sel("convertRect:fromView:"), objc.Send[apple.CGRect](field.native, selBounds), field.native)
	r.Size.Height += keyboardMargin
	animations := objcblock.New(func(objc.Block) {
		scroll.native.Send(sel("scrollRectToVisible:animated:"), r, false)
	})
	objc.ID(apple.Class("UIView")).Send(sel("animateWithDuration:animations:"), s.keyboardDuration.Seconds(), animations)
	animations.Release()
}
