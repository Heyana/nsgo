//go:build js && wasm

package harmonyos

import (
	"context"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func readAsset(ctx context.Context, path string) ([]byte, error) {
	base := js.Global().Get("document").Get("baseURI").String()
	return imageload.FetchAsset(ctx, base, "resources/"+path)
}
