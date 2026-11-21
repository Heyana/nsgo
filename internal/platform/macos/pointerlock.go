//go:build darwin && !ios

package macos

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

type pointerLockAPI struct {
	associate   func(uint32) int32
	hide, show  func(uint32) int32
	mainDisplay func() uint32
	createEvent func(uintptr) uintptr
	location    func(uintptr) apple.CGPoint
	warp        func(apple.CGPoint) int32
}

var loadPointerLock = sync.OnceValue(func() pointerLockAPI {
	var p pointerLockAPI
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return p
	}
	purego.RegisterLibFunc(&p.associate, lib, "CGAssociateMouseAndMouseCursorPosition")
	purego.RegisterLibFunc(&p.hide, lib, "CGDisplayHideCursor")
	purego.RegisterLibFunc(&p.show, lib, "CGDisplayShowCursor")
	purego.RegisterLibFunc(&p.mainDisplay, lib, "CGMainDisplayID")
	purego.RegisterLibFunc(&p.createEvent, lib, "CGEventCreate")
	purego.RegisterLibFunc(&p.location, lib, "CGEventGetLocation")
	purego.RegisterLibFunc(&p.warp, lib, "CGWarpMouseCursorPosition")
	return p
})

// Quartz's cursor association is process-wide. AppKit's main thread
// owns the one lock, shared by the app's own and embedded windows.
var lockedPointer struct {
	owner   *surface
	token   driver.PointerLock
	start   apple.CGPoint
	display uint32
}

func (s *surface) pointerLock(it *driver.MountItem) {
	token := driver.PointerLock{ID: it.ID, Count: it.Count}
	if !it.On {
		if lockedPointer.owner == s && lockedPointer.token == token {
			s.releasePointer(false)
		}
		return
	}
	if s.closed || token.ID == 0 || s.window == 0 || !s.canHoldPointer() {
		s.engine.Dispatch(token.Unlocked())
		return
	}
	if owner := lockedPointer.owner; owner != nil {
		if owner == s {
			lockedPointer.token = token
			return
		}
		owner.releasePointer(true)
	}
	p := loadPointerLock()
	if p.associate == nil {
		s.engine.Dispatch(token.Unlocked())
		return
	}
	event := p.createEvent(0)
	if event == 0 {
		s.engine.Dispatch(token.Unlocked())
		return
	}
	start := p.location(event)
	apple.Release(event)
	if p.associate(0) != 0 {
		s.engine.Dispatch(token.Unlocked())
		return
	}
	display := p.mainDisplay()
	if p.hide(display) != 0 {
		p.associate(1)
		s.engine.Dispatch(token.Unlocked())
		return
	}
	lockedPointer.owner, lockedPointer.token, lockedPointer.start, lockedPointer.display = s, token, start, display
}

func (s *surface) canHoldPointer() bool {
	return objc.Send[bool](s.window, sel("isKeyWindow")) &&
		objc.Send[bool](nsApp(), sel("isActive"))
}

func (s *surface) releasePointer(report bool) {
	if lockedPointer.owner != s {
		return
	}
	p := loadPointerLock()
	lock := lockedPointer
	lockedPointer.owner = nil
	p.associate(1)
	p.warp(lock.start)
	p.show(lock.display)
	if report && s.engine != nil {
		s.engine.Dispatch(lock.token.Unlocked())
	}
}

func (s *surface) pointerMotion(dx, dy float64) {
	if lockedPointer.owner == s && (dx != 0 || dy != 0) {
		s.engine.Dispatch(lockedPointer.token.Motion(dx, dy))
	}
}
