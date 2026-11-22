//go:build windows

package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestConsentStatus(t *testing.T) {
	for _, c := range []struct {
		machine, user, app string
		packaged           bool
		want               int
	}{
		{"", "", "", false, driver.PermissionGranted},
		{"Allow", "Allow", "Allow", false, driver.PermissionGranted},
		{"Deny", "Allow", "Allow", false, driver.PermissionDenied},
		{"Allow", "deny", "", false, driver.PermissionDenied},
		{"Allow", "Allow", "Deny", false, driver.PermissionDenied},
		// A packaged app is asked as it first opens the device.
		{"Allow", "Allow", "", true, driver.PermissionNotAsked},
		{"Allow", "Allow", "Allow", true, driver.PermissionGranted},
		{"Allow", "Deny", "", true, driver.PermissionDenied},
	} {
		if got := consentStatus(c.machine, c.user, c.app, c.packaged); got != c.want {
			t.Errorf("consentStatus(%q, %q, %q, %v) = %d, want %d", c.machine, c.user, c.app, c.packaged, got, c.want)
		}
	}
	if status, _ := permissionStatus(driver.PermissionAccessibility); status != driver.PermissionNotGated {
		t.Errorf("Accessibility on Windows: %d", status)
	}
	if got := permissionPage(driver.PermissionNotifications); got != "ms-settings:notifications" {
		t.Errorf("notifications settings page: %q", got)
	}
	if got := permissionPage(driver.PermissionAutomation); got != "" {
		t.Errorf("automation settings page: %q, want empty", got)
	}
}
