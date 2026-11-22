package updater

import (
	"context"

	"github.com/nsgo-dev/nsgo/core"
)

// Phase is what an in-app update is doing on Linux and Android.
type Phase = core.UpdatePhase

const (
	// PhaseIdle says no update is downloading or installing.
	PhaseIdle = core.UpdateIdle
	// PhaseDownloading says an update package is streaming to disk.
	PhaseDownloading = core.UpdateDownloading
	// PhaseInstalling says the verified package is being installed.
	PhaseInstalling = core.UpdateInstalling
)

// DownloadProgress reports bytes written while an update is downloaded.
type DownloadProgress struct {
	Loaded, Total int64
}

// Download streams an update's signed package directly to a temporary file in
// dir. It verifies size, SHA-256 and Ed25519 signature from the streamed file
// before making it visible, removing partial or invalid files on error.
func Download(ctx context.Context, update core.UpdateCheck, dir string, progress func(DownloadProgress)) (string, error) {
	return core.DownloadUpdate(ctx, update, dir, func(p core.UpdateDownloadProgress) {
		if progress != nil {
			progress(DownloadProgress(p))
		}
	})
}
