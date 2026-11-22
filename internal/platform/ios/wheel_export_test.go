//go:build ios && nsgotest

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// WheelRecognizersState reports the native scroll-only pan and pinch recognizers
// attached to the view with testID, for wheel_test.go.
type WheelRecognizersState struct {
	HasContinuous     bool
	ContinuousMask    int
	HasDiscrete       bool
	DiscreteMask      int
	HasPinch          bool
	HoldsOuterScroll  bool
	DefersInnerScroll bool
}

// WheelRecognizersForTest inspects the mounted view's scroll-only pan and pinch
// gesture recognizers and verifies their UIKit delegate priority against
// optional outer and inner scroll views.
func WheelRecognizersForTest(testID, outerScrollID, innerScrollID string) (WheelRecognizersState, bool) {
	var st WheelRecognizersState
	var ok bool
	onMainSync(func() {
		v := viewByTestID(testID)
		if v == nil || v.native == 0 {
			return
		}
		ok = true
		if v.scrolls != 0 {
			st.HasContinuous = true
			st.ContinuousMask = objc.Send[int](v.scrolls, sel("allowedScrollTypesMask"))
		}
		if v.notches != 0 {
			st.HasDiscrete = true
			st.DiscreteMask = objc.Send[int](v.notches, sel("allowedScrollTypesMask"))
		}
		st.HasPinch = v.pinch != 0
		if outerScrollID != "" && v.scrolls != 0 {
			if outer := viewByTestID(outerScrollID); outer != nil && outer.native != 0 {
				pan := outer.native.Send(selPanRecognizer)
				st.HoldsOuterScroll = wheelHoldsBack(v.native, 0, v.scrolls, pan)
			}
		}
		if innerScrollID != "" && v.scrolls != 0 {
			if inner := viewByTestID(innerScrollID); inner != nil && inner.native != 0 {
				// Simulate a hit landing inside innerScroll: check wheelShouldBegin
				// against a synthetic point inside inner's frame.
				bounds := objc.Send[apple.CGRect](inner.native, selBounds)
				mid := apple.CGPoint{X: bounds.Size.Width / 2, Y: bounds.Size.Height / 2}
				inView := objc.Send[apple.CGPoint](v.native, sel("convertPoint:fromView:"), mid, inner.native)
				hit := v.native.Send(sel("hitTest:withEvent:"), inView, objc.ID(0))
				scrollClass := apple.Class("UIScrollView")
				for n := hit; n != 0 && n != v.native; n = n.Send(selSuperview) {
					if objc.Send[bool](n, selIsKindOf, scrollClass) && objc.Send[bool](n, sel("isScrollEnabled")) {
						st.DefersInnerScroll = true
						break
					}
				}
			}
		}
	})
	return st, ok
}

func viewByTestID(testID string) *view {
	for _, v := range views {
		if v == nil || v.native == 0 {
			continue
		}
		id := v.native.Send(sel("accessibilityIdentifier"))
		if id != 0 && apple.GoString(id) == testID {
			return v
		}
	}
	return nil
}
