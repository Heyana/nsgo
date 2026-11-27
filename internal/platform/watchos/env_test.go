package watchos

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBuildEnvironmentWatchOSCapabilities(t *testing.T) {
	// dark (1) | reduceMotion (2) | voiceOver (4) | contrast (8) | step +1 (129 << 8)
	flags := uint32(1 | 2 | 4 | 8 | (129 << 8))
	safe := driver.Edges{Top: 18, Bottom: 12}
	env := buildEnvironment(2, safe, flags, "en-US", false, driver.LocaleSettings{Region: "US"},
		driver.OSVersion{Major: 11, Minor: 2}, driver.LifecycleActive)

	if env.OS != driver.OSIOS || env.Idiom != driver.IdiomWatch || env.Simulated {
		t.Fatalf("identity = OS %q Idiom %v Simulated %v, want ios / IdiomWatch / false", env.OS, env.Idiom, env.Simulated)
	}
	if !env.Dark || !env.ReduceMotion || !env.ScreenReader || !env.IncreaseContrast {
		t.Errorf("traits = dark:%v reduce:%v reader:%v contrast:%v, want all true",
			env.Dark, env.ReduceMotion, env.ScreenReader, env.IncreaseContrast)
	}
	if math.Abs(float64(env.FontScale)-1.1) > 1e-5 {
		t.Errorf("FontScale = %v, want 1.1", env.FontScale)
	}
	if env.PixelRatio != 2 || env.SafeArea != safe || env.Locale != "en-US" {
		t.Errorf("display/locale = ratio:%v safe:%+v locale:%q", env.PixelRatio, env.SafeArea, env.Locale)
	}
	if !env.Speak {
		t.Errorf("watchOS Speak = false, want true")
	}
	// watchOS has no audio playback, speech recognition, camera, code scanner, WebKit, StoreKit/Apple sign-in bridge, scheduled notifications, or custom cursors.
	if env.Audio || env.Speech || env.Camera || env.ScanCode ||
		env.HasWebView || env.SignInWithApple || env.Purchases || env.ScheduledNotifications ||
		env.CustomCursor || env.ScreenColor || env.CanSendInput ||
		env.CanSetWindowLevel || env.CanPlaceWindows || env.CanSetWindowWorkspaces {
		t.Errorf("watchOS reported unsupported capabilities as true: %+v", env)
	}
}
