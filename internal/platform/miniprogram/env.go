package miniprogram

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// deviceInfo is the plain-Go snapshot read from the mini program runtime's device bridge and API surface.
type deviceInfo struct {
	SafeArea     driver.Edges
	Dark         bool
	PixelRatio   float32
	FontScale    float32
	Language     string
	OS           string
	OSVersion    string
	Tablet       bool
	HasCameraAPI bool
	CanSpeak     bool
	DeviceModel  string
}

// deviceOS sets the OS the runtime reports, the device's, as a mini program has it: Simulated, as none of that OS's native parts are there, so the app's own views stand in for its system header and tab bar and a stack's pages show their Header; and Apple's keys on iOS and macOS, which Simulated alone would turn off.
func deviceOS(env *driver.Environment, os string) {
	env.OS = driver.OS(os)
	env.Simulated = true
	env.AppleKeyboard = env.OS == driver.OSIOS || env.OS == driver.OSMacOS
}

// buildEnvironment updates prev from the mini program runtime's device snapshot.
func buildEnvironment(env driver.Environment, d deviceInfo) driver.Environment {
	env.SafeArea = d.SafeArea
	env.Dark = d.Dark
	env.PixelRatio = d.PixelRatio
	env.FontScale = d.FontScale
	env.Locale = strings.ReplaceAll(d.Language, "_", "-")
	deviceOS(&env, d.OS)
	env.OSVersion = osVersion(d.OSVersion)
	env.DeviceModel = d.DeviceModel
	env.Idiom = driver.IdiomPhone
	if d.Tablet {
		env.Idiom = driver.IdiomTablet
	}
	// Mobile mini programs open the camera through chooseMedia / chooseImage (ItemTakePhoto); desktop mini program hosts do not.
	env.Camera = d.HasCameraAPI && (env.OS == driver.OSIOS || env.OS == driver.OSAndroid)
	env.Audio = false
	env.Speak = d.CanSpeak
	env.Speech = false
	env.ScanCode = false
	env.HasWebView = false
	env.CustomCursor = false
	return env
}

// osVersion reads "17.4.1" as 17.4.
func osVersion(v string) driver.OSVersion {
	var out driver.OSVersion
	parts := strings.SplitN(v, ".", 3)
	for i, p := range parts[:min(len(parts), 2)] {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		if i == 0 {
			out.Major = n
		} else {
			out.Minor = n
		}
	}
	return out
}
