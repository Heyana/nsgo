//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// updateConfig is how the app Run runs updates. The App Store updates iOS
// apps: a check asks it for the app's version in the user's country, and
// package updater offers its page, which the App Store opens.
var updateConfig updates.Config

// InstalledBy is "TestFlight" for a build TestFlight installed, or a
// development build on a device, whose receipt is the App Store's
// sandbox's: the store lists none of their versions, so they check
// nothing. It is "" for the App Store's own, and in the simulator, whose
// builds ask the App Store as its own would.
func InstalledBy() string {
	url := objc.ID(apple.Class("NSBundle")).Send(sel("mainBundle")).Send(sel("appStoreReceiptURL"))
	if url != 0 && apple.GoString(url.Send(sel("lastPathComponent"))) == "sandboxReceipt" {
		return "TestFlight"
	}
	return ""
}

// checkForUpdates asks the App Store off the main thread and answers with
// what it found. The bundle's identifier and version are the ones the
// store lists, whatever signing gave them.
func (s *surface) checkForUpdates(id uint32) {
	apple.CheckAppStore(updateConfig, false, id, s.engine.Dispatch)
}
