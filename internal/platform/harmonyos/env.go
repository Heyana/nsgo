package harmonyos

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system so tests can verify the HarmonyOS environment capability contract without the js/wasm runtime.

// deviceData is what the host reports about the window and the device.
type deviceData struct {
	Width, Height                            float64
	SafeTop, SafeRight, SafeBottom, SafeLeft float64
	// Keyboard is how much of the bottom the software keyboard covers.
	Keyboard              float64
	Dark                  bool
	PixelRatio, FontScale float64
	Language, OS          string
	DeviceModel           string
	// OSMajor and OSMinor are the system's version; Tablet, RTL and ScreenReader are the environment the app reads.
	OSMajor, OSMinor          int
	Tablet, RTL, ScreenReader bool
	// Lifecycle is driver.Lifecycle's value; OpenedURL is the Want's uri.
	Lifecycle int
	OpenedURL string
	// CanvasAtlas says the host's canvas player draws an atlas from one op; a host built before it leaves it out.
	CanvasAtlas bool
}

// buildEnvironment updates prev from the ArkTS host's deviceData snapshot.
func buildEnvironment(env driver.Environment, d deviceData) driver.Environment {
	env.SafeArea = driver.Edges{Top: driver.Points(d.SafeTop), Right: driver.Points(d.SafeRight),
		Bottom: driver.Points(d.SafeBottom), Left: driver.Points(d.SafeLeft)}
	env.KeyboardHeight = driver.Points(d.Keyboard)
	env.Dark = d.Dark
	env.PixelRatio = float32(d.PixelRatio)
	env.FontScale = float32(d.FontScale)
	env.RTL = d.RTL
	env.ScreenReader = d.ScreenReader
	// The camera picker is the platform's, on every device that has one.
	env.Camera = true
	// ArkWeb hosts core.WebView nodes through ItemWeb and ItemWebCommand.
	env.HasWebView = true
	// Sounds and players have no host here yet, and the microphone's permission is the page's to answer.
	env.Audio = false
	env.Speak = false
	env.Speech = false
	env.ScanCode = false
	env.CustomCursor = false
	// A screen's system header is 44 points tall, which the core pads its content below and the render layer draws over it.
	env.HeaderHeight = 44
	env.Lifecycle = driver.Lifecycle(d.Lifecycle)
	env.OSVersion = driver.OSVersion{Major: d.OSMajor, Minor: d.OSMinor}
	env.OpenedURL = d.OpenedURL
	env.DeviceModel = d.DeviceModel
	if d.Language != "" {
		env.Locale = d.Language
	} else {
		env.Locale = "zh-Hans"
	}
	// HarmonyOS reads as Android under the platform name, as the mini programs' runtime already maps ohos to android.
	env.OS = driver.OSAndroid
	env.Idiom = driver.IdiomPhone
	if d.Tablet {
		env.Idiom = driver.IdiomTablet
	}
	return env
}
