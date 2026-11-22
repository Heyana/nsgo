//go:build ios && nsgotest

package ios

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// WindowGeometryForTest reads the actual UIKit content bounds and safe area.
func WindowGeometryForTest() (size core.Size, safe core.Edges) {
	onMainSync(func() {
		b := objc.Send[apple.CGRect](runner.native, selBounds)
		in := objc.Send[edgeInsets](runner.native, sel("safeAreaInsets"))
		size = core.Size{Width: core.Points(b.Size.Width), Height: core.Points(b.Size.Height)}
		safe = core.Edges{Top: core.Points(in.Top), Right: core.Points(in.Right), Bottom: core.Points(in.Bottom), Left: core.Points(in.Left)}
	})
	return
}

// NotifyKeyboardFrameForTest delivers one controlled keyboard notification,
// converting its surface-local rectangle to UIKit's screen coordinates. An
// empty frame hides it. This does not show or move the system keyboard.
func NotifyKeyboardFrameForTest(frame core.Bounds) {
	onMainSync(func() {
		space := runner.window.Send(sel("screen")).Send(sel("coordinateSpace"))
		local := apple.CGRect{Origin: apple.CGPoint{X: float64(frame.Origin.X), Y: float64(frame.Origin.Y)}, Size: apple.CGSize{Width: float64(frame.Size.Width), Height: float64(frame.Size.Height)}}
		screen := objc.Send[apple.CGRect](runner.native, sel("convertRect:toCoordinateSpace:"), local, space)
		value := objc.ID(apple.Class("NSValue")).Send(sel("valueWithCGRect:"), screen)
		info := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), value, apple.Global(uikitPath, "UIKeyboardFrameEndUserInfoKey"))
		note := objc.ID(apple.Class("NSNotification")).Send(sel("notificationWithName:object:userInfo:"), apple.Global(uikitPath, "UIKeyboardWillChangeFrameNotification"), objc.ID(0), info)
		runner.keyboardMoved(note, frame.Size.Width <= 0 || frame.Size.Height <= 0)
	})
}
