//go:build ios

package ios

import (
	"errors"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

func (s *surface) capture(id driver.NodeID, count uint32) {
	answer := func(data []byte, err error) {
		s.engine.Dispatch(driver.CaptureResult(count, data, err))
	}
	apple.OnMain(func() {
		host := s.native
		window := host.Send(sel("window"))
		if window == 0 {
			answer(nil, errors.New("ios: capture has no visible surface"))
			return
		}
		bounds := objc.Send[apple.CGRect](host, selBounds)
		region := bounds
		if id != 0 {
			v := s.viewOf(id)
			if v == nil || v.native.Send(sel("window")) != window {
				answer(nil, errors.New("ios: capture view is outside the surface"))
				return
			}
			region = objc.Send[apple.CGRect](v.native, sel("convertRect:toView:"),
				objc.Send[apple.CGRect](v.native, selBounds), host)
			region = apple.IntersectRects(region, bounds)
		}
		if region.Size.Width <= 0 || region.Size.Height <= 0 {
			answer(nil, errors.New("ios: capture has no visible pixels"))
			return
		}
		region = objc.Send[apple.CGRect](host, sel("convertRect:toView:"), region, window)
		bounds = objc.Send[apple.CGRect](window, selBounds)
		format := objc.ID(apple.Class("UIGraphicsImageRendererFormat")).Send(sel("preferredFormat"))
		format.Send(sel("setPreferredRange:"), 2) // UIGraphicsImageRendererFormatRangeStandard
		renderer := objc.ID(apple.Class("UIGraphicsImageRenderer")).Send(selAlloc).
			Send(sel("initWithSize:format:"), region.Size, format)
		drew := false
		draw := objcblock.New(func(_ objc.Block, _ objc.ID) {
			dst := apple.Rect(bounds.Origin.X-region.Origin.X, bounds.Origin.Y-region.Origin.Y, bounds.Size.Width, bounds.Size.Height)
			drew = objc.Send[bool](window, sel("drawViewHierarchyInRect:afterScreenUpdates:"), dst, true)
		})
		picture := renderer.Send(sel("imageWithActions:"), draw)
		draw.Release()
		renderer.Send(selRelease)
		img := apple.Retain(uintptr(picture.Send(sel("CGImage"))))
		if !drew || img == 0 {
			apple.Release(img)
			answer(nil, errors.New("ios: the surface drew no pixels"))
			return
		}
		go func() {
			defer apple.Release(img)
			data, err := apple.PNGImage(img)
			answer(data, err)
		}()
	})
}
