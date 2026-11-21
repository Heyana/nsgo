//go:build darwin && !ios

package macos

import (
	"errors"
	"math"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

var loadCapture = sync.OnceFunc(func() {
	// The current-process ScreenCaptureKit API needs no access to other apps.
	purego.Dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL); err == nil {
		if address, err := purego.Dlsym(cg, "CGWindowListCreateImage"); err == nil {
			purego.RegisterFunc(&captureLegacyWindow, address)
		}
	}
})

var captureLegacyWindow func(apple.CGRect, uint32, uint32, uint32) uintptr

func (s *surface) capture(id driver.NodeID, count uint32) {
	answer := func(data []byte, err error) {
		s.engine.Dispatch(driver.CaptureResult(count, data, err))
	}
	apple.OnMain(func() {
		if s.native == 0 {
			answer(nil, errors.New("macos: capture has no surface"))
			return
		}
		w := s.native.Send(sel("window"))
		if w == 0 {
			answer(nil, errors.New("macos: capture has no window"))
			return
		}
		number := objc.Send[int](w, sel("windowNumber"))
		if number <= 0 {
			answer(nil, errors.New("macos: capture has no window number"))
			return
		}
		screenRect := func(v objc.ID) apple.CGRect {
			r := objc.Send[apple.CGRect](v, sel("convertRect:toView:"), objc.Send[apple.CGRect](v, selBounds), objc.ID(0))
			return objc.Send[apple.CGRect](w, sel("convertRectToScreen:"), r)
		}
		region := screenRect(s.native)
		if id != 0 {
			v := s.views[id]
			if v == nil || v.native.Send(sel("window")) != w {
				answer(nil, errors.New("macos: capture view is outside the surface"))
				return
			}
			region = apple.IntersectRects(region, screenRect(v.native))
		}
		if region.Size.Width <= 0 || region.Size.Height <= 0 {
			answer(nil, errors.New("macos: capture has no visible pixels"))
			return
		}
		scale := objc.Send[float64](w, sel("backingScaleFactor"))
		frame := objc.Send[apple.CGRect](w, sel("frame"))
		screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
		main := screens.Send(sel("firstObject"))
		height := objc.Send[apple.CGRect](main, sel("frame")).Size.Height
		regionTop := height - region.Origin.Y - region.Size.Height
		loadCapture()
		if captureOwnWindow(uint32(number), frame, region, scale, answer) {
			return
		}
		if !objc.Send[bool](w, sel("isVisible")) || !objc.Send[bool](w, sel("isOnActiveSpace")) {
			answer(nil, errors.New("macos: capture has no visible window"))
			return
		}
		go func() {
			if captureLegacyWindow == nil {
				answer(nil, errors.New("macos: window capture is unavailable"))
				return
			}
			// A window number and IncludingWindow keep every other app out.
			const includingWindow, ignoreFrame, bestResolution = 1 << 3, 1 << 0, 1 << 3
			rect := apple.Rect(region.Origin.X, regionTop, region.Size.Width, region.Size.Height)
			img := captureLegacyWindow(rect, includingWindow, uint32(number), ignoreFrame|bestResolution)
			defer apple.Release(img)
			var w, h int
			if img != 0 {
				w, h = apple.ImageSize(img)
			}
			data, err := apple.CroppedPNG(img, apple.Rect(0, 0, float64(w), float64(h)))
			answer(data, err)
		}()
	})
}

// captureOwnWindow uses the async screenshot API on macOS 14.4 and later.
func captureOwnWindow(number uint32, frame, region apple.CGRect, scale float64, answer func([]byte, error)) bool {
	content := objc.ID(apple.Class("SCShareableContent"))
	manager := objc.ID(apple.Class("SCScreenshotManager"))
	current := sel("getCurrentProcessShareableContentWithCompletionHandler:")
	if content == 0 || manager == 0 || !objc.Send[bool](content, sel("respondsToSelector:"), current) {
		return false
	}
	ready := objcblock.New(func(_ objc.Block, list, failure objc.ID) {
		if list == 0 || failure != 0 {
			answer(nil, errors.New("macos: cannot enumerate this app's windows"))
			return
		}
		windows := list.Send(sel("windows"))
		for i := range objc.Send[uint](windows, sel("count")) {
			w := windows.Send(sel("objectAtIndex:"), i)
			if objc.Send[uint32](w, sel("windowID")) == number {
				captureSCWindow(manager, w, frame, region, scale, answer)
				return
			}
		}
		answer(nil, errors.New("macos: this app's window is unavailable for capture"))
	})
	content.Send(current, ready)
	ready.Release()
	return true
}

func captureSCWindow(manager, window objc.ID, frame, region apple.CGRect, scale float64, answer func([]byte, error)) {
	filter := objc.ID(apple.Class("SCContentFilter")).Send(sel("alloc")).Send(sel("initWithDesktopIndependentWindow:"), window)
	config := objc.ID(apple.Class("SCStreamConfiguration")).Send(sel("new"))
	config.Send(sel("setWidth:"), int(math.Round(frame.Size.Width*scale)))
	config.Send(sel("setHeight:"), int(math.Round(frame.Size.Height*scale)))
	config.Send(sel("setShowsCursor:"), false)
	config.Send(sel("setIgnoreShadowsSingleWindow:"), true)
	x := math.Floor((region.Origin.X - frame.Origin.X) * scale)
	y := math.Floor((frame.Origin.Y + frame.Size.Height - region.Origin.Y - region.Size.Height) * scale)
	crop := apple.Rect(x, y, math.Ceil(region.Size.Width*scale), math.Ceil(region.Size.Height*scale))
	finished := objcblock.New(func(_ objc.Block, img uintptr, failure objc.ID) {
		if failure != 0 {
			answer(nil, errors.New("macos: "+apple.GoString(failure.Send(sel("localizedDescription")))))
			return
		}
		if img == 0 {
			answer(nil, errors.New("macos: the window produced no screenshot"))
			return
		}
		apple.Retain(img)
		go func() {
			defer apple.Release(img)
			data, err := apple.CroppedPNG(img, crop)
			answer(data, err)
		}()
	})
	manager.Send(sel("captureImageWithFilter:configuration:completionHandler:"), filter, config, finished)
	finished.Release()
	config.Send(selRelease)
	filter.Send(selRelease)
}
