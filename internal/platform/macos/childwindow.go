//go:build darwin && !ios

package macos

import (
	"slices"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// centerOverParent places the frame over its parent's, inside the usable
// area of the parent's screen. Both frames use AppKit's global coordinates.
func (s *surface) centerOverParent() {
	p := s.windowParent.window
	frame := objc.Send[apple.CGRect](p, sel("frame"))
	child := objc.Send[apple.CGRect](s.window, sel("frame"))
	point := apple.CGPoint{X: frame.Origin.X + (frame.Size.Width-child.Size.Width)/2,
		Y: frame.Origin.Y + (frame.Size.Height-child.Size.Height)/2}
	if screen := p.Send(sel("screen")); screen != 0 {
		area := objc.Send[apple.CGRect](screen, sel("visibleFrame"))
		point.X = min(max(point.X, area.Origin.X), area.Origin.X+max(0, area.Size.Width-child.Size.Width))
		point.Y = min(max(point.Y, area.Origin.Y), area.Origin.Y+max(0, area.Size.Height-child.Size.Height))
	}
	s.window.Send(sel("setFrameOrigin:"), point)
}

func (s *surface) showsChildren() bool {
	return !s.closed && objc.Send[bool](s.window, sel("isVisible")) && !objc.Send[bool](s.window, sel("isMiniaturized"))
}

// showChildWindow records a show request even while the parent is hidden.
func (s *surface) showChildWindow() bool {
	p := s.windowParent
	if p == nil {
		return true
	}
	s.childShown = true
	if !p.showsChildren() {
		return false
	}
	s.attachChildWindow(p.windowFullScreen || p.childAttached)
	return true
}

// attachChildWindow uses AppKit's native relationship during full screen
// to follow the parent's Space. In a regular Space the two windows can move
// independently between displays, including displays with separate Spaces.
func (s *surface) attachChildWindow(on bool) {
	if s.childAttached == on {
		return
	}
	p := s.windowParent.window
	s.childAttached = on
	if on {
		s.applySpaces()
		if s.window.Send(sel("screen")) != p.Send(sel("screen")) {
			s.centerOverParent()
		}
		p.Send(sel("addChildWindow:ordered:"), s.window, 1) // NSWindowAbove
	} else {
		p.Send(sel("removeChildWindow:"), s.window)
		s.applySpaces()
	}
}

// syncChildWindows restores the children's visibility and orders them
// above this window without making them key.
func (s *surface) syncChildWindows() {
	if len(s.windowChildren) == 0 {
		return
	}
	shown := s.showsChildren()
	above := s.window
	for _, child := range s.windowChildren {
		w := child.window
		if !shown || !child.childShown || objc.Send[bool](w, sel("isMiniaturized")) {
			child.attachChildWindow(false)
			if objc.Send[bool](w, sel("isVisible")) {
				w.Send(sel("orderOut:"), objc.ID(0))
			}
		} else {
			child.attachChildWindow(s.windowFullScreen || s.childAttached)
			screen := w.Send(sel("screen"))
			if !child.childAttached && (screen == 0 || screen == above.Send(sel("screen"))) {
				w.Send(sel("orderWindow:relativeTo:"), 1, objc.Send[int](above, sel("windowNumber")))
				above = w
			} else if !objc.Send[bool](w, sel("isVisible")) {
				w.Send(sel("orderFront:"), objc.ID(0))
			}
		}
		child.syncChildWindows()
	}
}

func (s *surface) childWindowEvent(name string) {
	if s.closed {
		return
	}
	switch name {
	case "NSWindowDidBecomeKeyNotification":
		if p := s.windowParent; p != nil && p.showsChildren() {
			p.windowChildren = slices.DeleteFunc(p.windowChildren, func(child *surface) bool { return child == s })
			p.windowChildren = append(p.windowChildren, s)
		}
	case "NSWindowWillEnterFullScreenNotification", "NSWindowDidEnterFullScreenNotification":
		s.windowFullScreen = true
	case "NSWindowDidExitFullScreenNotification":
		s.windowFullScreen = false
	case "NSWindowDidBecomeMainNotification", "NSWindowDidMiniaturizeNotification", "NSWindowDidDeminiaturizeNotification":
	default:
		return
	}
	s.syncChildWindows()
}

// releaseChildWindows drops native and Go references in either direction
// before either window is released. The core closes each child's surface.
func (s *surface) releaseChildWindows() {
	if p := s.windowParent; p != nil {
		s.attachChildWindow(false)
		p.windowChildren = slices.DeleteFunc(p.windowChildren, func(child *surface) bool { return child == s })
		s.windowParent = nil
	}
	children := s.windowChildren
	s.windowChildren = nil
	for _, child := range children {
		child.attachChildWindow(false)
		child.windowParent = nil
		child.window.Send(sel("orderOut:"), objc.ID(0))
		child.syncChildWindows()
	}
}
