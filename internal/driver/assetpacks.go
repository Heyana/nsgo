package driver

import (
	"context"
	"io"
)

// AssetPacks is the platform's file and availability access for store packs.
// A backend initializes it only on a supported platform.
var AssetPacks struct {
	Ready func(context.Context, string) (bool, error)
	Open  func(string, string) (io.ReadCloser, error)
}
