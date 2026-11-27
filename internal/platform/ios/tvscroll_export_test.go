//go:build ios && nsgotest

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TVScrollState reports the native tvOS remote-surface scrolling state of a
// mounted scroll view, for tvscroll_native_test.go.
type TVScrollState struct {
	OnTV             bool
	IndirectPan      bool
	HasIndirectTouch bool
	CanBecomeFocused bool
	Offset           driver.Point
}

// TVScrollStateForTest inspects the mounted scroll view with testID on the
// UIKit main thread.
func TVScrollStateForTest(testID string) (TVScrollState, bool) {
	var st TVScrollState
	var ok bool
	onMainSync(func() {
		v := viewByTestID(testID)
		if v == nil || v.sc == nil || v.native == 0 {
			return
		}
		ok = true
		st.OnTV = onTV()
		st.IndirectPan = v.sc.indirectPan
		st.CanBecomeFocused = objc.Send[bool](v.native, selCanBecomeFocused)
		if pan := v.native.Send(selPanRecognizer); pan != 0 {
			if types := pan.Send(selAllowedTouchTypes); types != 0 {
				st.HasIndirectTouch = objc.Send[bool](types, sel("containsObject:"), indirectTouchTypeNumber())
			}
		}
		o := objc.Send[apple.CGPoint](v.native, selContentOffset)
		st.Offset = driver.Point{X: driver.Points(o.X), Y: driver.Points(o.Y)}
	})
	return st, ok
}
