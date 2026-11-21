package miniprogram

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A mini program runs on its device's OS without its native parts: the environment reports that OS as Simulated, so core mounts a stack page's Header, with Apple's keys on Apple's systems.
func TestDeviceOSIsSimulated(t *testing.T) {
	for _, tc := range []struct {
		os    string
		apple bool
	}{{"ios", true}, {"android", false}, {"macos", true}} {
		var env driver.Environment
		deviceOS(&env, tc.os)
		if env.OS != driver.OS(tc.os) || !env.Simulated || env.AppleKeyboard != tc.apple {
			t.Errorf("%s: OS %q, Simulated %v, AppleKeyboard %v", tc.os, env.OS, env.Simulated, env.AppleKeyboard)
		}
	}
}

func TestBuildEnvironmentCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name         string
		info         deviceInfo
		wantOS       driver.OS
		wantIdiom    driver.Idiom
		wantAppleKey bool
		wantCamera   bool
	}{
		{
			name:         "iOS phone with camera API",
			info:         deviceInfo{OS: "ios", OSVersion: "17.4.1", Language: "zh_CN", PixelRatio: 3, HasCameraAPI: true},
			wantOS:       driver.OSIOS,
			wantIdiom:    driver.IdiomPhone,
			wantAppleKey: true,
			wantCamera:   true,
		},
		{
			name:         "Android tablet with camera API",
			info:         deviceInfo{OS: "android", OSVersion: "14", Language: "en_US", PixelRatio: 2, Tablet: true, HasCameraAPI: true},
			wantOS:       driver.OSAndroid,
			wantIdiom:    driver.IdiomTablet,
			wantAppleKey: false,
			wantCamera:   true,
		},
		{
			name:         "iOS phone without camera API",
			info:         deviceInfo{OS: "ios", HasCameraAPI: false},
			wantOS:       driver.OSIOS,
			wantIdiom:    driver.IdiomPhone,
			wantAppleKey: true,
			wantCamera:   false,
		},
		{
			name:         "macOS desktop host",
			info:         deviceInfo{OS: "macos", OSVersion: "15.1", HasCameraAPI: true},
			wantOS:       driver.OSMacOS,
			wantIdiom:    driver.IdiomPhone,
			wantAppleKey: true,
			wantCamera:   false,
		},
		{
			name:         "Windows desktop host",
			info:         deviceInfo{OS: "windows", OSVersion: "11.0", HasCameraAPI: true},
			wantOS:       driver.OSWindows,
			wantIdiom:    driver.IdiomPhone,
			wantAppleKey: false,
			wantCamera:   false,
		},
	} {
		env := buildEnvironment(driver.Environment{}, tc.info)
		if env.OS != tc.wantOS || env.Idiom != tc.wantIdiom || !env.Simulated || env.AppleKeyboard != tc.wantAppleKey {
			t.Errorf("%s: identity = OS %q Idiom %v Simulated %v AppleKeyboard %v",
				tc.name, env.OS, env.Idiom, env.Simulated, env.AppleKeyboard)
		}
		if env.Camera != tc.wantCamera {
			t.Errorf("%s: Camera = %v, want %v", tc.name, env.Camera, tc.wantCamera)
		}
		if env.Audio || env.Speak || env.Speech || env.ScanCode || env.HasWebView ||
			env.CustomCursor || env.SignInWithApple || env.Purchases || env.ScheduledNotifications {
			t.Errorf("%s: reported unsupported capabilities as true: %+v", tc.name, env)
		}
	}

	got := buildEnvironment(driver.Environment{}, deviceInfo{OS: "ios", OSVersion: "17.4.1", Language: "zh_CN"})
	if got.OSVersion != (driver.OSVersion{Major: 17, Minor: 4}) || got.Locale != "zh-CN" {
		t.Errorf("parsed version/locale = %v / %q, want 17.4 / zh-CN", got.OSVersion, got.Locale)
	}
}
