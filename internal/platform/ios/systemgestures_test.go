//go:build ios && nsgotest

package ios_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestUIKitControllerDefersSystemGestureEdges(t *testing.T) {
	var release func()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		release = mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
			IOSEdges: mobile.GestureEdgeBottom | mobile.GestureEdgeLeft,
		})
		return core.View{
			TestID:  "release-gestures",
			Style:   core.Style{Width: core.Pt(140), Height: core.Pt(70)},
			OnPress: func(core.PressEvent) { release() },
		}
	})
	// UIRectEdge.bottom = 4 and UIRectEdge.left = 2.
	app.WaitUntil(5*time.Second, func() bool { return ios.SystemGestureEdgesForTest() == 6 })
	app.Press("id:release-gestures")
	app.WaitUntil(5*time.Second, func() bool { return ios.SystemGestureEdgesForTest() == 0 })
}
