//go:build js && wasm

package web

import (
	"context"
	"io/fs"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func readAsset(ctx context.Context, path string) ([]byte, error) {
	if resourceNames != nil {
		name, ok := resourceNames[path]
		if !ok {
			return nil, fs.ErrNotExist
		}
		path = name
	}
	base := resourceBase
	if base == "" {
		base = js.Global().Get("document").Get("baseURI").String()
	}
	return imageload.FetchAsset(ctx, base, path)
}
