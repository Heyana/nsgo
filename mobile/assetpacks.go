package mobile

import (
	"context"
	"io"

	"github.com/nsgo-dev/nsgo/core"
)

// AssetPackReady reports whether the store pack is available locally.
// Call it off the UI thread. Platforms without store packs return errors.ErrUnsupported.
func AssetPackReady(ctx context.Context, name string) (bool, error) {
	return core.AssetPackReady(ctx, name)
}

// OpenAssetPackFile opens a slash-separated path in a downloaded store pack.
// Close the returned reader after use.
func OpenAssetPackFile(name, path string) (io.ReadCloser, error) {
	return core.OpenAssetPackFile(name, path)
}
