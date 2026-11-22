package testhost

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPinSetsTheTestsDisplaySettings(t *testing.T) {
	platform := driver.Environment{Dark: true, ReduceMotion: true, IncreaseContrast: true, LaunchedAtLogin: true, FrontmostApp: driver.FrontmostAppInfo{ID: "com.apple.Terminal", PID: 7}, CanReadFrontmostApp: true, OS: driver.OSMacOS, Locale: "en-US", FontScale: 1, DeviceName: "Ada's Mac", DeviceModel: "Mac15,7"}
	got := Pin(platform, &driver.Environment{IncreaseContrast: false})
	if got.Dark || got.ReduceMotion || got.IncreaseContrast || got.Camera || got.ScanCode || got.LaunchedAtLogin || got.FrontmostApp != (driver.FrontmostAppInfo{}) || got.CanReadFrontmostApp || got.OS != driver.OSMacOS || got.Locale != "en-US" || got.FontScale != 1 || got.DeviceName != "Ada's Mac" || got.DeviceModel != "Mac15,7" {
		t.Errorf("pinned off: %+v", got)
	}
	got = Pin(driver.Environment{OS: driver.OSMacOS}, &driver.Environment{Dark: true, ReduceMotion: true, IncreaseContrast: true, Camera: true, ScanCode: true, LaunchedAtLogin: true, FrontmostApp: driver.FrontmostAppInfo{ID: "editor", FullScreen: true}, CanReadFrontmostApp: true, DeviceName: "Test phone", DeviceModel: "Pixel 8", VerticalBarEdge: driver.VerticalBarTrailing})
	if !got.Dark || !got.ReduceMotion || !got.IncreaseContrast || !got.Camera || !got.ScanCode || !got.LaunchedAtLogin || got.FrontmostApp != (driver.FrontmostAppInfo{ID: "editor", FullScreen: true}) || !got.CanReadFrontmostApp || got.DeviceName != "Test phone" || got.DeviceModel != "Pixel 8" || got.VerticalBarEdge != driver.VerticalBarTrailing {
		t.Errorf("pinned on: %+v", got)
	}
	// Leaving VerticalBarEdge unspecified in pin preserves the platform's reported edge.
	preserved := Pin(driver.Environment{OS: driver.OSIOS, VerticalBarEdge: driver.VerticalBarLeading}, &driver.Environment{Dark: true})
	if preserved.VerticalBarEdge != driver.VerticalBarLeading {
		t.Errorf("platform vertical bar edge not preserved: %v", preserved.VerticalBarEdge)
	}
}
