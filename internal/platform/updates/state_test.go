package updates

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func TestUpdateCapabilityNeedsNoNetwork(t *testing.T) {
	old := imageload.HTTP
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		t.Fatal("capability or disabled check accessed the network")
		return nil, nil
	}
	t.Cleanup(func() { imageload.HTTP = old })
	for _, test := range []struct {
		name     string
		config   Config
		appStore bool
		want     driver.UpdateManaged
	}{
		{"development", Config{Development: true, Feed: "https://example.invalid/appcast"}, false, driver.UpdateManagedDevelopment},
		{"none", Config{Channel: "none"}, false, driver.UpdateManagedChannel},
		{"store", Config{Channel: "store"}, false, driver.UpdateManagedChannel},
		{"ios_store", Config{Channel: "store"}, true, driver.UpdateManagedPage},
		{"ios_none", Config{Channel: "none"}, true, driver.UpdateManagedChannel},
		{"direct", Config{Channel: "direct", Feed: "https://example.invalid/appcast"}, false, driver.UpdateManagedSelf},
		{"unowned", Config{Channel: "direct", Feed: "https://example.invalid/appcast", PageOnly: true}, false, driver.UpdateManagedPage},
		{"Flatpak", Config{Store: "Flatpak"}, false, driver.UpdateManagedStore},
		{"Snap", Config{Store: "Snap"}, false, driver.UpdateManagedStore},
		{"Play", Config{Store: "com.android.vending"}, false, driver.UpdateManagedStore},
		{"TestFlight", Config{Store: "TestFlight"}, true, driver.UpdateManagedStore},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := test.config.State(test.appStore)
			if got.Managed != test.want || got.Store != test.config.Store {
				t.Fatalf("capability=%+v", got)
			}
			if got.Managed == driver.UpdateManagedSelf || got.Managed == driver.UpdateManagedPage {
				return
			}
			var answer driver.Update
			if test.appStore {
				answer = FindInAppStore(t.Context(), test.config, "")
			} else {
				answer = Find(t.Context(), test.config)
			}
			if answer.Err != "" || answer.Managed != test.want || answer.Store != test.config.Store {
				t.Fatalf("disabled answer=%+v", answer)
			}
		})
	}
}

func TestLinuxOwnershipIsKnownBeforeTheFirstCheck(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "js" {
		t.Skip("exercises the Linux package probe through a POSIX shell fixture")
	}
	bin := t.TempDir()
	count := filepath.Join(bin, "ownership-probes")
	command := filepath.Join(bin, "dpkg-query")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf x >> '"+count+"'\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	c := LinuxConfig(Config{Feed: "https://example.invalid/appcast", Channel: "direct"})
	for range 3 {
		if state := c.State(false); state.Managed != driver.UpdateManagedSelf {
			t.Fatalf("owned state=%+v", state)
		}
	}
	data, err := os.ReadFile(count)
	if err != nil || string(data) != "x" {
		t.Fatalf("ownership was not probed once: %q, %v", data, err)
	}
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if state := LinuxConfig(Config{Feed: "https://example.invalid/appcast", Channel: "direct"}).State(false); state.Managed != driver.UpdateManagedPage {
		t.Fatalf("unowned state=%+v", state)
	}
}
