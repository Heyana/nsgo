//go:build darwin && !ios

package macos

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

var (
	cgDisplayCopyMode     func(uint32) uintptr
	cgDisplayCopyAllModes func(uint32, uintptr) uintptr
	cgDisplayModeRate     func(uintptr) float64
	cgDisplayModeWidth    func(uintptr) uintptr
	cgDisplayModeHeight   func(uintptr) uintptr
	bindDisplayMode       = sync.OnceFunc(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&cgDisplayCopyMode, cg, "CGDisplayCopyDisplayMode")
		purego.RegisterLibFunc(&cgDisplayCopyAllModes, cg, "CGDisplayCopyAllDisplayModes")
		purego.RegisterLibFunc(&cgDisplayModeRate, cg, "CGDisplayModeGetRefreshRate")
		purego.RegisterLibFunc(&cgDisplayModeWidth, cg, "CGDisplayModeGetPixelWidth")
		purego.RegisterLibFunc(&cgDisplayModeHeight, cg, "CGDisplayModeGetPixelHeight")
	})
)

// displayModeKey is a display at one pixel size, whose fastest mode displayModes keeps.
type displayModeKey struct {
	number        uint32
	width, height uintptr
}

var displayModes struct {
	sync.Mutex
	maximum map[displayModeKey]float64
}

func displayRates(screen objc.ID, number uint32) (rate, maximum float64) {
	// NSScreen's maximum is the selected mode's ceiling: 60 on a 120 Hz panel the user set to 60 Hz.
	if selector := sel("maximumFramesPerSecond"); objc.Send[bool](screen, sel("respondsToSelector:"), selector) {
		maximum = float64(max(0, objc.Send[int](screen, selector)))
	}
	bindDisplayMode()
	if cgDisplayCopyMode != nil && number != 0 {
		if mode := cgDisplayCopyMode(number); mode != 0 {
			rate = cgDisplayModeRate(mode)
			maximum = max(maximum, fastestDisplayMode(displayModeKey{number, cgDisplayModeWidth(mode), cgDisplayModeHeight(mode)}))
			apple.Release(mode)
		}
	}
	if rate <= 0 {
		rate = maximum
	}
	return rate, max(rate, maximum)
}

// fastestDisplayMode returns the highest rate among key's display's modes at key's pixel size, as Windows reads the modes at the current resolution. The list includes the HiDPI modes, whose pixel size is the current mode's on a Retina panel.
func fastestDisplayMode(key displayModeKey) float64 {
	displayModes.Lock()
	defer displayModes.Unlock()
	if maximum, ok := displayModes.maximum[key]; ok {
		return maximum
	}
	options := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"),
		objc.ID(apple.Class("NSNumber")).Send(sel("numberWithBool:"), true), apple.NSString("kCGDisplayShowDuplicateLowResolutionModes"))
	modes := cgDisplayCopyAllModes(key.number, uintptr(options))
	if modes == 0 {
		return 0
	}
	defer apple.Release(modes)
	maximum := 0.0
	list := objc.ID(modes)
	for i, count := uint(0), objc.Send[uint](list, sel("count")); i < count; i++ {
		mode := uintptr(list.Send(sel("objectAtIndex:"), i))
		if cgDisplayModeWidth(mode) == key.width && cgDisplayModeHeight(mode) == key.height {
			maximum = max(maximum, cgDisplayModeRate(mode))
		}
	}
	if displayModes.maximum == nil {
		displayModes.maximum = make(map[displayModeKey]float64)
	}
	displayModes.maximum[key] = maximum
	return maximum
}
