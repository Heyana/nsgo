//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

type fullScreenRequest struct {
	on        bool
	displayID string
}

type fullScreenTransition struct {
	busy    bool
	next    *fullScreenRequest
	restore *apple.CGRect
}

func fullScreenDisplay(id string) objc.ID {
	screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
	for i, count := uint(0), objc.Send[uint](screens, sel("count")); i < count; i++ {
		screen := screens.Send(sel("objectAtIndex:"), i)
		if screenDisplayID(screen, i) == id {
			return screen
		}
	}
	return 0
}

func (s *surface) setFullScreen(on bool, displayID string) {
	w, f := s.window, &s.fullScreenTransition
	if !s.ownWindow() {
		return
	}
	// AppKit ignores toggleFullScreen: for a window that never enters full screen, as an auxiliary one on every space does, and no notification would end the transition.
	if on && (s.constraints.NoMaximize || objc.Send[uint](w, sel("collectionBehavior"))&(fullScreenAuxiliary|fullScreenNone) != 0 || !s.showFirst && !objc.Send[bool](w, sel("isVisible"))) {
		return
	}
	var target objc.ID
	if on && displayID != "" {
		target = fullScreenDisplay(displayID)
		if target == 0 {
			return
		}
	}
	if f.busy || s.showFirst {
		f.next = &fullScreenRequest{on: on, displayID: displayID}
		return
	}
	fullScreen := objc.Send[uint](w, selStyleMask)&maskFullScreen != 0
	current := w.Send(sel("screen"))
	move := target != 0 && target != current && screenDisplayID(current, ^uint(0)) != displayID
	if fullScreen == on && !move {
		return
	}
	if fullScreen && move {
		// Native full screen can relocate only by leaving its Space first.
		f.next = &fullScreenRequest{on: true, displayID: displayID}
	} else if move {
		frame := objc.Send[apple.CGRect](w, sel("frame"))
		f.restore = &frame
		area := objc.Send[apple.CGRect](target, sel("visibleFrame"))
		staged := frame
		staged.Size.Width = min(frame.Size.Width, area.Size.Width)
		staged.Size.Height = min(frame.Size.Height, area.Size.Height)
		staged.Origin.X = area.Origin.X + (area.Size.Width-staged.Size.Width)/2
		staged.Origin.Y = area.Origin.Y + (area.Size.Height-staged.Size.Height)/2
		w.Send(sel("setFrame:display:"), staged, true)
	}
	f.busy = true
	w.Send(sel("toggleFullScreen:"), objc.ID(0))
}

func (s *surface) restoreFullScreenFrame() {
	f := &s.fullScreenTransition
	if f.restore != nil {
		s.window.Send(sel("setFrame:display:"), *f.restore, true)
		f.restore = nil
	}
}

func (s *surface) finishFullScreenTransition(restore bool) {
	f := &s.fullScreenTransition
	if restore {
		s.restoreFullScreenFrame()
	}
	f.busy = false
	next := f.next
	f.next = nil
	if next != nil {
		s.setFullScreen(next.on, next.displayID)
	}
}
