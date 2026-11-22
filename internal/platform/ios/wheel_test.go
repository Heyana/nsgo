//go:build ios && nsgotest

package ios_test

import (
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestWheelAndPinchRecognizersOnIOS checks that UIKit's scroll-only pan
// recognizers use UIScrollTypeMaskContinuous (2) for trackpads and
// UIScrollTypeMaskDiscrete (1) for mouse wheels, hold back outer scroll views
// while deferring to inner scroll views, and attach UIPinchGestureRecognizer.
func TestWheelAndPinchRecognizersOnIOS(t *testing.T) {
	const wait = 5 * time.Second
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Scroll{
			TestID: "outer-scroll",
			Style:  core.Style{FlexGrow: 1},
			Children: []core.Node{
				core.View{
					Style: core.Style{Height: core.Pt(800)},
					Children: []core.Node{
						core.View{
							TestID:  "canvas",
							Style:   core.Style{Width: core.Pt(240), Height: core.Pt(200)},
							OnWheel: func(core.WheelEvent) {},
							OnPinch: func(core.GestureEvent) {},
							Children: []core.Node{
								core.Scroll{
									TestID: "inner-scroll",
									Style:  core.Style{Width: core.Pt(100), Height: core.Pt(80)},
									Children: []core.Node{
										core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(240)}},
									},
								},
							},
						},
					},
				},
			},
		}
	})
	app.WaitFor("id:canvas", wait)

	var st ios.WheelRecognizersState
	app.WaitUntil(wait, func() bool {
		var ok bool
		st, ok = ios.WheelRecognizersForTest("canvas", "outer-scroll", "inner-scroll")
		return ok && st.HasContinuous && st.HasDiscrete && st.HasPinch
	})
	if st.ContinuousMask != 2 {
		t.Errorf("continuous recognizer allowedScrollTypesMask = %d, want 2 (UIScrollTypeMaskContinuous)", st.ContinuousMask)
	}
	if st.DiscreteMask != 1 {
		t.Errorf("discrete recognizer allowedScrollTypesMask = %d, want 1 (UIScrollTypeMaskDiscrete)", st.DiscreteMask)
	}
	if !st.HoldsOuterScroll {
		t.Errorf("OnWheel recognizer does not hold back outer UIScrollView's pan recognizer")
	}
	if !st.DefersInnerScroll {
		t.Errorf("OnWheel view did not detect inner UIScrollView to defer scrolling to it")
	}
}
