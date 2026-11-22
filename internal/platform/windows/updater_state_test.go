//go:build windows && !nsgo_steam

package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

func TestWinSparkleDisabledBuildDoesNotLoadAnUpdater(t *testing.T) {
	old, config, failure := winSparkle, winUpdateConfig, winUpdateFailure
	defer func() { winSparkle, winUpdateConfig, winUpdateFailure = old, config, failure }()
	winSparkle = nil
	for _, test := range []struct {
		config updates.Config
		want   driver.UpdateManaged
	}{
		{updates.Config{Development: true, Feed: "https://example.invalid/feed"}, driver.UpdateManagedDevelopment},
		{updates.Config{Channel: "none"}, driver.UpdateManagedChannel},
		{updates.Config{Channel: "store"}, driver.UpdateManagedChannel},
	} {
		prepareWinSparkle(test.config, 0)
		if state := winSparkleStatus(); state.Managed != test.want || winSparkle != nil || winUpdateFailure != "" {
			t.Fatalf("disabled updater=%+v, failure=%q", state, winUpdateFailure)
		}
	}
	winUpdateConfig = updates.Config{Channel: "direct", Feed: "https://example.invalid/feed"}
	winUpdateFailure = "DLL load failed"
	if state := winSparkleStatus(); state.Managed != driver.UpdateManagedUnavailable {
		t.Fatalf("failed updater=%+v", state)
	}
}
