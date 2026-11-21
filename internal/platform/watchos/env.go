package watchos

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file builds on every system so tests can verify the watchOS environment capability contract without the SwiftUI watchOS runtime.

// buildEnvironment constructs the watchOS Environment from the SwiftUI host's display traits and Apple's locale/version readings.
func buildEnvironment(scale float64, safe driver.Edges, flags uint32, locale string, rtl bool, settings driver.LocaleSettings, version driver.OSVersion, lifecycle driver.Lifecycle) driver.Environment {
	steps := int(flags>>8) - 128
	return driver.Environment{
		SafeArea:         safe,
		Dark:             flags&1 != 0,
		ReduceMotion:     flags&2 != 0,
		IncreaseContrast: flags&8 != 0,
		ScreenReader:     flags&4 != 0,
		FontScale:        fontScale(steps),
		RTL:              rtl,
		PixelRatio:       float32(scale),
		Locale:           locale,
		LocaleSettings:   settings,
		OS:               driver.OSIOS,
		OSVersion:        version,
		Idiom:            driver.IdiomWatch,
		Lifecycle:        lifecycle,
	}
}

// fontScale is the text size multiplier steps from the default dynamic type size, as watchOS's body sizes grow.
func fontScale(steps int) float32 {
	return float32(math.Pow(1.1, float64(steps)))
}
