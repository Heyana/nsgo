//go:build linux && !android && !nsgo_steam

package linux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// Run this test executable inside an installed Flatpak. A host-side environment variable is insufficient: InstalledBy must see the sandbox's real marker.
func TestInstalledFlatpakNeverFetchesItsAppcast(t *testing.T) {
	if _, err := os.Stat("/.flatpak-info"); err != nil {
		if os.Getenv("NSGO_REQUIRE_FLATPAK") == "1" {
			t.Fatalf("required installed Flatpak sandbox: %v", err)
		}
		t.Skip("requires an installed Flatpak sandbox")
	}
	if got := InstalledBy(); got != "Flatpak" {
		t.Fatalf("native install sign = %q, want Flatpak", got)
	}
	var calls int
	was := imageload.HTTP
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		calls++
		return nil, errors.New("native fixture records every attempted appcast fetch")
	}
	t.Cleanup(func() { imageload.HTTP = was })
	c := updates.Config{App: "Flatpak fixture", Version: "1.0", Channel: "direct", Feed: "https://fixture.invalid/appcast.xml", Automatic: true}
	// Show that the transport interception observes an ordinary direct check.
	if answer := updates.Find(context.Background(), c); calls != 1 || answer.Err == "" {
		t.Fatalf("transport control: calls=%d, answer=%+v", calls, answer)
	}
	calls = 0
	probeDir := t.TempDir()
	probe := filepath.Join(probeDir, "queried")
	for _, command := range []string{"dpkg-query", "pacman"} {
		if err := os.WriteFile(filepath.Join(probeDir, command), []byte("#!/bin/sh\n: > \"$NSGO_TEST_PACKAGE_PROBE\"\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSGO_TEST_PACKAGE_PROBE", probe)
	t.Setenv("PATH", probeDir)
	_ = exec.Command("dpkg-query").Run()
	if _, err := os.Stat(probe); err != nil {
		t.Fatalf("ownership probe control: %v", err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	c.Store = InstalledBy()
	c = configureUpdates(c)
	state := c.State(false)
	if c.Checks(false) || state.Managed != driver.UpdateManagedStore || state.Store != "Flatpak" || state.AutomaticChecks {
		t.Fatalf("Flatpak policy = %+v", state)
	}
	for range 3 {
		answer := updates.Find(context.Background(), c)
		if answer.Managed != driver.UpdateManagedStore || answer.Store != "Flatpak" || answer.Err != "" || answer.Version != "" {
			t.Fatalf("managed check = %+v", answer)
		}
	}
	if calls != 0 {
		t.Fatalf("Flatpak attempted %d appcast fetches", calls)
	}
	if _, err := os.Stat(probe); !os.IsNotExist(err) {
		t.Fatalf("Flatpak queried a host package manager: %v", err)
	}
}
