//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// UISemanticContentAttribute's forced directions and NSWritingDirection's values.
const (
	semanticLTR = 3
	semanticRTL = 4
	writingLTR  = 0
	writingRTL  = 1
)

var selSetSemantic = sel("setSemanticContentAttribute:")

func uiWritingDirection(rtl bool) int {
	if rtl {
		return writingRTL
	}
	return writingLTR
}

// applyDirection lays UIKit's views out in the app's direction, as its
// language is written, whatever the system's: the bars, fields, alerts
// and native components the surface shows. A standalone app's appearance
// proxy gives it to the views UIKit makes later too, as its alerts' and
// pickers'; an embedded surface leaves the host's views alone.
func (s *surface) applyDirection() {
	if !s.embedded {
		objc.ID(apple.Class("UIView")).Send(sel("appearance")).Send(selSetSemantic, s.semantic())
	}
	s.directTree(s.native)
	for _, v := range s.views {
		if v != nil && v.bar != nil {
			s.setSwipeBack(v)
		}
	}
}

func (s *surface) semantic() int {
	if s.rtl {
		return semanticRTL
	}
	return semanticLTR
}

// direct gives a view the app's direction, once the app has one.
func (s *surface) direct(v objc.ID) {
	if s.directed && v != 0 {
		apple.Msg(v, selSetSemantic, uintptr(s.semantic()))
	}
}

// directTree gives a view and the views inside it the app's direction.
func (s *surface) directTree(v objc.ID) {
	if !s.directed || v == 0 {
		return
	}
	apple.Msg(v, selSetSemantic, uintptr(s.semantic()))
	apple.Msg(v, selSetNeedsLayout)
	subs := objc.ID(apple.Msg(v, sel("subviews")))
	n := apple.Msg(subs, sel("count"))
	for i := uintptr(0); i < n; i++ {
		s.directTree(objc.ID(apple.Msg(subs, sel("objectAtIndex:"), i)))
	}
}
