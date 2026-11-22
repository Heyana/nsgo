//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// NSUserInterfaceLayoutDirection's and NSWritingDirection's values.
const (
	layoutLTR  = 0
	layoutRTL  = 1
	writingLTR = 0
	writingRTL = 1
)

var (
	selLayoutDirection     = sel("userInterfaceLayoutDirection")
	selSetLayoutDirection  = sel("setUserInterfaceLayoutDirection:")
	selSetWritingDirection = sel("setBaseWritingDirection:")
)

func nsWritingDirection(rtl bool) int {
	if rtl {
		return writingRTL
	}
	return writingLTR
}

func (s *surface) layoutDirection() int {
	if s.rtl {
		return layoutRTL
	}
	return layoutLTR
}

// directTree lays a view and the views inside it out in the app's
// direction, as its language is written, whatever the system's: the
// fields, scroll bars and native components the surface shows.
func (s *surface) directTree(v objc.ID) {
	if !s.directed || v == 0 {
		return
	}
	want := s.layoutDirection()
	if objc.Send[int](v, selLayoutDirection) != want {
		v.Send(selSetLayoutDirection, want)
	}
	subs := v.Send(sel("subviews"))
	n := objc.Send[uint](subs, sel("count"))
	for i := uint(0); i < n; i++ {
		s.directTree(subs.Send(sel("objectAtIndex:"), i))
	}
}

// inheritLayoutDirection propagates a view's userInterfaceLayoutDirection to
// subviews AppKit adds lazily (such as NSTextField's internal label subview).
func inheritLayoutDirection(self objc.ID, cmd objc.SEL, sub objc.ID) {
	apple.Super(self, cmd, sub)
	if sub == 0 {
		return
	}
	want := objc.Send[int](self, selLayoutDirection)
	if objc.Send[int](sub, selLayoutDirection) != want {
		sub.Send(selSetLayoutDirection, want)
	}
}

// directMenu lays a menu the surface shows, and its submenus, out in the
// app's direction.
func (s *surface) directMenu(m objc.ID) {
	if !s.directed || m == 0 {
		return
	}
	m.Send(selSetLayoutDirection, s.layoutDirection())
	items := m.Send(sel("itemArray"))
	n := objc.Send[uint](items, sel("count"))
	for i := uint(0); i < n; i++ {
		s.directMenu(items.Send(sel("objectAtIndex:"), i).Send(sel("submenu")))
	}
}
