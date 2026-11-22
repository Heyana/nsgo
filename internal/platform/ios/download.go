//go:build ios && !tvos

package ios

import (
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Reconnect before UIKit delivers a completed background session's events, including a launch that has no scene and never starts the UI engine.
func configureDownloads() {
	if runner == nil {
		return
	}
	data, _, _ := appDirs(runner.id, runner.host.DataDir)
	apple.ConfigureDownloads(data, runner.id, true)
}
