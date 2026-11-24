//go:build darwin && !ios

package macos

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPrivacyPanes(t *testing.T) {
	dragPanes := []string{
		driver.PermissionAccessibility, driver.PermissionScreenRecording, driver.PermissionInputMonitoring,
		driver.PermissionFullDiskAccess, driver.PermissionAppManagement, driver.PermissionDeveloperTools,
		driver.PermissionBluetooth, driver.PermissionMediaLibrary,
	}
	noDragPanes := []string{
		driver.PermissionAutomation, driver.PermissionAutomation + ":com.apple.finder",
		driver.PermissionMicrophone, driver.PermissionCamera, driver.PermissionContacts,
		driver.PermissionCalendars, driver.PermissionReminders, driver.PermissionPhotos,
		driver.PermissionLocation, driver.PermissionBackgroundLocation, driver.PermissionNotifications,
		driver.PermissionLocalNetwork,
	}
	for _, name := range append(slices.Clone(dragPanes), noDragPanes...) {
		if privacySettingsURL(name) == "" {
			t.Errorf("%s has no pane", name)
		}
		want := slices.Contains(dragPanes, name)
		if takesDrag(name) != want {
			t.Errorf("%s takes a drag: %v, want %v", name, !want, want)
		}
	}
	if got := privacySettingsURL(driver.PermissionAccessibility); got != "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility" {
		t.Errorf("accessibility's pane: %s", got)
	}
	if got := privacySettingsURL(driver.PermissionAutomation + ":com.apple.finder"); got != "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation" {
		t.Errorf("automation target pane: %s", got)
	}
	if got := privacySettingsURL(driver.PermissionContacts); got != "x-apple.systempreferences:com.apple.preference.security?Privacy_Contacts" {
		t.Errorf("contacts pane: %s", got)
	}
	if got := privacySettingsURL(driver.PermissionPhotos); got != "x-apple.systempreferences:com.apple.preference.security?Privacy_Photos" {
		t.Errorf("photos pane: %s", got)
	}
	if got := privacySettingsURL(driver.PermissionLocation); got != "x-apple.systempreferences:com.apple.preference.security?Privacy_LocationServices" {
		t.Errorf("location pane: %s", got)
	}
	if got := privacySettingsURL(driver.PermissionLocalNetwork); got != "x-apple.systempreferences:com.apple.preference.security?Privacy_LocalNetwork" {
		t.Errorf("local network pane: %s", got)
	}
	if got := privacySettingsURL(driver.PermissionNotifications); got != "x-apple.systempreferences:com.apple.preference.notifications" {
		t.Errorf("notifications pane: %s", got)
	}
	if privacySettingsURL("nope") != "" {
		t.Error("an unknown key has a pane")
	}
	if !guidePermissionAllowed(driver.PermissionGranted) || !guidePermissionAllowed(driver.PermissionLimited) || guidePermissionAllowed(driver.PermissionDenied) {
		t.Error("guidePermissionAllowed did not accept granted and limited only")
	}
	if got := appDisplayName("/Applications/My App.app/"); got != "My App" {
		t.Errorf("app name %q", got)
	}
}

// Sensor panes stay navigable even when the sensor provider is unavailable.
func TestSensorPrivacyPanes(t *testing.T) {
	for _, tc := range []struct{ name, anchor string }{
		{driver.PermissionLocation, "Privacy_LocationServices"},
		{driver.PermissionBackgroundLocation, "Privacy_LocationServices"},
		{driver.PermissionMotion, "Privacy_Motion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := "x-apple.systempreferences:com.apple.preference.security?" + tc.anchor
			if got := privacySettingsURL(tc.name); got != want {
				t.Fatalf("pane = %q, want %q", got, want)
			}
			if takesDrag(tc.name) {
				t.Error("sensor pane must list apps that requested access, without a drag")
			}
		})
	}
}

func TestSplitRuns(t *testing.T) {
	app, permission := runMarker{guideAppMarker, "Notes"}, runMarker{guidePermissionMarker, "Accessibility"}
	got := splitRuns("Drag "+guideAppMarker+" to the list above to allow "+guidePermissionMarker+".", app, permission)
	want := []textRun{{"Drag ", false}, {"Notes", true}, {" to the list above to allow ", false}, {"Accessibility", true}, {".", false}}
	if !slices.Equal(got, want) {
		t.Errorf("runs %+v", got)
	}
	// A translation may name the permission first.
	got = splitRuns(guidePermissionMarker+"：拖移"+guideAppMarker, app, permission)
	want = []textRun{{"Accessibility", true}, {"：拖移", false}, {"Notes", true}}
	if !slices.Equal(got, want) {
		t.Errorf("reordered runs %+v", got)
	}
}

func TestGuideFrame(t *testing.T) {
	screen := guideRect{0, 0, 1440, 875}
	// Under the window, lined up with its pane.
	settings := guideRect{200, 300, 715, 500}
	if got := guideFrame(settings, screen, 100); got != (guideRect{430, 200, 485, 100}) {
		t.Errorf("under the window: %+v", got)
	}
	// Kept off the screen's right edge.
	settings = guideRect{1000, 300, 715, 500}
	if got := guideFrame(settings, screen, 100); got.maxX() != screen.maxX()-screenInset {
		t.Errorf("at the right edge: %+v", got)
	}
	// Over the window's bottom when no room is under it.
	settings = guideRect{200, 20, 715, 500}
	if got := guideFrame(settings, screen, 100); got.Y != screenInset {
		t.Errorf("at the bottom: %+v", got)
	}
	// Never narrower than its least width.
	if w := guideWidth(guideRect{0, 0, 400, 400}, screen); w != guideMinWidth {
		t.Errorf("narrow window: width %v", w)
	}
	if got := flipRect(guideRect{10, 20, 100, 50}, 900); got != (guideRect{10, 830, 100, 50}) {
		t.Errorf("flipped %+v", got)
	}
}

func TestScreenOfAndSettingsWindow(t *testing.T) {
	left, right := guideRect{0, 0, 1440, 900}, guideRect{1440, 0, 1920, 1080}
	if got := screenOf(guideRect{1300, 100, 600, 400}, []guideRect{left, right}); got != right {
		t.Errorf("screen %+v", got)
	}
	if got := screenOf(guideRect{-5000, 0, 10, 10}, []guideRect{left, right}); got != left {
		t.Errorf("off every screen: %+v", got)
	}
	main := guideRect{100, 100, 715, 600}
	got, ok := mainSettingsWindow([]settingsWindow{
		{bounds: guideRect{0, 0, 200, 100}, alpha: 1},
		{bounds: main, alpha: 1},
		{bounds: guideRect{0, 0, 2000, 2000}, layer: 25, alpha: 1},
		{bounds: guideRect{0, 0, 1000, 800}, alpha: 0},
	})
	if !ok || got != main {
		t.Errorf("settings window %+v %v", got, ok)
	}
	if _, ok := mainSettingsWindow(nil); ok {
		t.Error("a window without any")
	}
}
