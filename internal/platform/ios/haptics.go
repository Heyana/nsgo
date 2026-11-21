//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

const coreHaptics = "/System/Library/Frameworks/CoreHaptics.framework/CoreHaptics"

var hapticPatternsAvailable = sync.OnceValue(func() bool {
	if _, err := purego.Dlopen(coreHaptics, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return false
	}
	hardware := objc.ID(apple.Class("CHHapticEngine")).Send(sel("capabilitiesForHardware"))
	return apple.MsgBool(hardware, sel("supportsHaptics"))
})

type hapticIDs struct{ transient, continuous, intensity, sharpness objc.ID }

var hapticSymbols = sync.OnceValue(func() hapticIDs {
	return hapticIDs{
		transient:  apple.Global(coreHaptics, "CHHapticEventTypeHapticTransient"),
		continuous: apple.Global(coreHaptics, "CHHapticEventTypeHapticContinuous"),
		intensity:  apple.Global(coreHaptics, "CHHapticEventParameterIDHapticIntensity"),
		sharpness:  apple.Global(coreHaptics, "CHHapticEventParameterIDHapticSharpness"),
	}
})

// Core Haptics is process-wide. Only the surface playing the current
// pattern can stop it when that surface closes or leaves the foreground.
var hapticPlayback struct {
	engine, player objc.ID
	owner          *surface
}

func (s *surface) playHapticPattern(pattern *driver.HapticPattern) {
	if pattern == nil {
		s.stopHapticPattern()
		return
	}
	if !s.env.HapticPatterns || appLifecycle() == driver.LifecycleBackground {
		return
	}
	if hapticPlayback.owner != nil {
		hapticPlayback.owner.stopHapticPattern()
	}
	const noError = objc.ID(0)
	if hapticPlayback.engine == 0 {
		hapticPlayback.engine = objc.ID(apple.Class("CHHapticEngine")).Send(selAlloc).Send(sel("initAndReturnError:"), noError)
		hapticPlayback.engine.Send(sel("setPlaysHapticsOnly:"), true)
		hapticPlayback.engine.Send(sel("setAutoShutdownEnabled:"), true)
	}
	engine := hapticPlayback.engine
	if engine == 0 || !objc.Send[bool](engine, sel("startAndReturnError:"), noError) {
		return
	}
	symbols := hapticSymbols()
	if symbols.transient == 0 || symbols.continuous == 0 || symbols.intensity == 0 || symbols.sharpness == 0 {
		return
	}
	array := objc.ID(apple.Class("NSMutableArray"))
	events := array.Send(sel("arrayWithCapacity:"), uint(len(*pattern)))
	paramClass := objc.ID(apple.Class("CHHapticEventParameter"))
	eventClass := objc.ID(apple.Class("CHHapticEvent"))
	for _, event := range *pattern {
		params := array.Send(sel("arrayWithCapacity:"), uint(2))
		intensity := paramClass.Send(selAlloc).Send(sel("initWithParameterID:value:"), symbols.intensity, event.Intensity)
		sharpness := paramClass.Send(selAlloc).Send(sel("initWithParameterID:value:"), symbols.sharpness, event.Sharpness)
		if intensity == 0 || sharpness == 0 {
			intensity.Send(selRelease)
			sharpness.Send(selRelease)
			return
		}
		params.Send(sel("addObject:"), intensity)
		params.Send(sel("addObject:"), sharpness)
		intensity.Send(selRelease)
		sharpness.Send(selRelease)
		pulse := eventClass.Send(selAlloc)
		if event.Duration > 0 {
			pulse = pulse.Send(sel("initWithEventType:parameters:relativeTime:duration:"),
				symbols.continuous, params, event.At.Seconds(), event.Duration.Seconds())
		} else {
			pulse = pulse.Send(sel("initWithEventType:parameters:relativeTime:"),
				symbols.transient, params, event.At.Seconds())
		}
		if pulse == 0 {
			return
		}
		events.Send(sel("addObject:"), pulse)
		pulse.Send(selRelease)
	}
	obj := objc.ID(apple.Class("CHHapticPattern")).Send(selAlloc).Send(sel("initWithEvents:parameters:error:"),
		events, objc.ID(apple.Class("NSArray")).Send(sel("array")), noError)
	if obj == 0 {
		return
	}
	defer obj.Send(selRelease)
	player := engine.Send(sel("createPlayerWithPattern:error:"), obj, noError)
	if player != 0 && objc.Send[bool](player, sel("startAtTime:error:"), float64(0), noError) {
		hapticPlayback.player, hapticPlayback.owner = player.Send(sel("retain")), s
	}
}

func (s *surface) stopHapticPattern() {
	if hapticPlayback.owner == s {
		hapticPlayback.player.Send(sel("stopAtTime:error:"), float64(0), objc.ID(0))
		hapticPlayback.player.Send(selRelease)
		hapticPlayback.player = 0
		hapticPlayback.owner = nil
	}
}

// haptic plays h (driver.ItemHaptic) through UIKit's feedback generators,
// which play nothing on a device without a Taptic Engine or with system
// haptics off; tvOS has none of them.
func haptic(h driver.Haptic) {
	// UIImpactFeedbackStyle's and UINotificationFeedbackType's values.
	impacts := map[driver.Haptic]int{driver.HapticLight: 0, driver.HapticMedium: 1, driver.HapticHeavy: 2,
		driver.HapticSoft: 3, driver.HapticRigid: 4}
	outcomes := map[driver.Haptic]int{driver.HapticSuccess: 0, driver.HapticWarning: 1, driver.HapticError: 2}
	var gen objc.ID
	switch {
	case h == driver.HapticSelection:
		if c := apple.Class("UISelectionFeedbackGenerator"); c != 0 {
			gen = objc.ID(c).Send(selNew)
			gen.Send(sel("selectionChanged"))
		}
	case h >= driver.HapticLight && h <= driver.HapticRigid:
		// initWithStyle: is deprecated from iOS 17.5 for one made for a
		// view, which iOS 15 lacks.
		if c := apple.Class("UIImpactFeedbackGenerator"); c != 0 {
			gen = objc.ID(c).Send(selAlloc).Send(sel("initWithStyle:"), impacts[h])
			gen.Send(sel("impactOccurred"))
		}
	default:
		if c := apple.Class("UINotificationFeedbackGenerator"); c != 0 {
			gen = objc.ID(c).Send(selNew)
			gen.Send(sel("notificationOccurred:"), outcomes[h])
		}
	}
	if gen != 0 {
		gen.Send(selRelease)
	}
}
