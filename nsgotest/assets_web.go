//go:build js && wasm

package nsgotest

import (
	"context"
	"os"

	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func assetReader() func(context.Context, string) ([]byte, error) {
	base := os.Getenv("NSGO_BASE")
	if base == "" {
		return nil
	}
	return func(ctx context.Context, path string) ([]byte, error) {
		return imageload.FetchAsset(ctx, base, path)
	}
}
