//go:build !nsgo_steam

package updates

import "github.com/nsgo-dev/nsgo/internal/driver"

// EnableDownload connects core.DownloadUpdate to updates.Download on targets
// that stage and verify in-app update packages (Linux and Android). Calling it
// from those targets' update paths rather than a package init keeps Download
// and crypto/ed25519 out of apps on platforms that never stage downloads in Go.
func EnableDownload() {
	if driver.UpdateDownload == nil {
		driver.UpdateDownload = Download
	}
}
