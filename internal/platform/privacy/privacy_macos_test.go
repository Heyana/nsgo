//go:build darwin && !ios

package privacy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBundleDeclares(t *testing.T) {
	bundle := func(name, plist string) string {
		contents := filepath.Join(t.TempDir(), name+".app", "Contents")
		if err := os.MkdirAll(contents, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(contents, "MacOS", name)
	}
	const key = "NSMicrophoneUsageDescription"
	if !bundleDeclares(bundle("With", "<key>NSMicrophoneUsageDescription</key>"), key) {
		t.Error("a declared description was missed")
	}
	if bundleDeclares(bundle("Without", "<key>CFBundleName</key>"), key) {
		t.Error("an app without a description declares one")
	}
	if bundleDeclares("/usr/local/bin/tool", key) {
		t.Error("a bare program declares a description")
	}
}

func TestFromTCC(t *testing.T) {
	for raw, want := range map[int]int{0: driver.PermissionNotAsked, 1: driver.PermissionDenied, 2: driver.PermissionDenied,
		3: driver.PermissionGranted, 4: driver.PermissionDenied, 9: driver.PermissionUnknown} {
		if got := fromTCC(raw); got != want {
			t.Errorf("fromTCC(%d) = %d, want %d", raw, got, want)
		}
	}
	for raw, want := range map[int]int{0: driver.PermissionNotAsked, 1: driver.PermissionDenied, 2: driver.PermissionDenied,
		3: driver.PermissionGranted, 4: driver.PermissionLimited, 9: driver.PermissionUnknown} {
		if got := fromTCCLimited(raw); got != want {
			t.Errorf("fromTCCLimited(%d) = %d, want %d", raw, got, want)
		}
	}
	for raw, want := range map[int32]int{0: driver.PermissionGranted, aeErrWouldRequireUserConsent: driver.PermissionNotAsked,
		aeErrNotPermitted: driver.PermissionDenied, -600: driver.PermissionUnknown, -50: driver.PermissionUnknown} {
		if got := fromAEStatus(raw); got != want {
			t.Errorf("fromAEStatus(%d) = %d, want %d", raw, got, want)
		}
	}
}

func TestUsageKeysAndPlatformAsks(t *testing.T) {
	for name, want := range map[string]string{
		driver.PermissionContacts:                   "NSContactsUsageDescription",
		driver.PermissionPhotos:                     "NSPhotoLibraryUsageDescription",
		driver.PermissionAutomation + ":com.a.test": "NSAppleEventsUsageDescription",
		driver.PermissionAutomation:                 "",
	} {
		if got := usageKey(name); got != want {
			t.Errorf("usageKey(%q) = %q, want %q", name, got, want)
		}
	}
	for _, async := range []string{driver.PermissionContacts, driver.PermissionPhotos, driver.PermissionAutomation + ":com.apple.finder"} {
		if platformAsks(async) {
			t.Errorf("platformAsks(%q) = true, want false", async)
		}
	}
	if !platformAsks(driver.PermissionAutomation) {
		t.Error("bare automation should be handled synchronously by platformRequest")
	}
}

func TestCentralManagerOptionsTurnThePowerAlertOff(t *testing.T) {
	key := global(coreBluetoothPath, "CBCentralManagerOptionShowPowerAlertKey")
	if key == 0 {
		t.Fatal("CoreBluetooth exports no CBCentralManagerOptionShowPowerAlertKey")
	}
	options := centralManagerOptions()
	defer options.Send(sel("release"))
	value := options.Send(sel("objectForKey:"), key)
	if value == 0 || objc.Send[bool](value, sel("boolValue")) {
		t.Errorf("the power alert option is %#x, want NO", value)
	}
}

func TestStatusAnswersEveryPermission(t *testing.T) {
	validStatus := func(st int) bool {
		return st == driver.PermissionNotAsked || st == driver.PermissionGranted || st == driver.PermissionLimited ||
			st == driver.PermissionDenied || st == driver.PermissionUnknown
	}
	for _, name := range []string{driver.PermissionAccessibility, driver.PermissionScreenRecording, driver.PermissionInputMonitoring,
		driver.PermissionFullDiskAccess, driver.PermissionAppManagement, driver.PermissionDeveloperTools, driver.PermissionAutomation,
		driver.PermissionAutomation + ":com.apple.finder", driver.PermissionMicrophone, driver.PermissionCamera,
		driver.PermissionContacts, driver.PermissionCalendars, driver.PermissionReminders, driver.PermissionPhotos,
		driver.PermissionBluetooth, driver.PermissionMediaLibrary} {
		if st := Status(name); !validStatus(st) {
			t.Errorf("%s: status %d on macOS", name, st)
		}
	}
	// A non-running target app reports procNotFound -> PermissionUnknown without blocking or crashing.
	if st := Status(driver.PermissionAutomation + ":dev.nsgo.not-running-app"); st != driver.PermissionUnknown {
		t.Errorf("non-running automation target: status %d, want PermissionUnknown", st)
	}
	// Location reports a valid status (or unavailable if Location Services are disabled on the host).
	if st := Status(driver.PermissionLocation); !validStatus(st) && st != driver.PermissionUnavailable {
		t.Errorf("location: status %d on macOS", st)
	}
	// Motion and background location (without UIBackgroundModes) are unavailable on macOS.
	if st := Status(driver.PermissionMotion); st != driver.PermissionUnavailable {
		t.Errorf("motion on macOS: %d, want PermissionUnavailable", st)
	}
	if st := Status(driver.PermissionBackgroundLocation); st != driver.PermissionUnavailable {
		t.Errorf("background location on macOS: %d, want PermissionUnavailable", st)
	}
	if st := Status("nope"); st != driver.PermissionUnavailable {
		t.Errorf("an unknown key: %d", st)
	}
}
