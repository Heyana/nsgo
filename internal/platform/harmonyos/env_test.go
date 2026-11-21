package harmonyos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBuildEnvironmentHarmonyOSCapabilities(t *testing.T) {
	phone := buildEnvironment(driver.Environment{}, deviceData{
		Width: 420, Height: 912, SafeTop: 36, SafeBottom: 24,
		Dark: true, PixelRatio: 3, FontScale: 1, OSMajor: 6, OSMinor: 1,
	})
	if phone.OS != driver.OSAndroid || phone.Idiom != driver.IdiomPhone || phone.Simulated {
		t.Fatalf("phone identity = OS %q Idiom %v Simulated %v, want android / IdiomPhone / false",
			phone.OS, phone.Idiom, phone.Simulated)
	}
	if phone.Locale != "zh-Hans" {
		t.Errorf("default Locale = %q, want zh-Hans", phone.Locale)
	}
	if phone.HeaderHeight != 44 {
		t.Errorf("HeaderHeight = %v, want 44", phone.HeaderHeight)
	}
	if !phone.HasWebView {
		t.Error("HarmonyOS: HasWebView = false, want true (ArkWeb hosts WebView)")
	}
	if !phone.Camera {
		t.Error("HarmonyOS: Camera = false, want true")
	}
	if phone.Audio || phone.Speak || phone.Speech || phone.ScanCode ||
		phone.CustomCursor || phone.SignInWithApple || phone.Purchases ||
		phone.ScheduledNotifications || phone.CanSendInput {
		t.Errorf("HarmonyOS reported unsupported capabilities as true: %+v", phone)
	}

	tablet := buildEnvironment(driver.Environment{}, deviceData{
		Tablet: true, Language: "en-US", PixelRatio: 2,
	})
	if tablet.Idiom != driver.IdiomTablet || tablet.Locale != "en-US" {
		t.Errorf("tablet = Idiom %v Locale %q, want IdiomTablet / en-US", tablet.Idiom, tablet.Locale)
	}
}
