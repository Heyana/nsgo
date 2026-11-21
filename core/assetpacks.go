package core

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// AssetPackReady is mobile's query for whether a store pack is available locally.
func AssetPackReady(ctx context.Context, name string) (bool, error) {
	if !validPackName(name) {
		return false, fs.ErrInvalid
	}
	if driver.AssetPacks.Ready == nil {
		return false, errors.ErrUnsupported
	}
	return driver.AssetPacks.Ready(ctx, name)
}

// OpenAssetPackFile is mobile's read-only access to a file in an available store pack.
func OpenAssetPackFile(name, path string) (io.ReadCloser, error) {
	if !validPackName(name) || path == "." || !fs.ValidPath(path) || strings.ContainsRune(path, '\\') {
		return nil, fs.ErrInvalid
	}
	if driver.AssetPacks.Open == nil {
		return nil, errors.ErrUnsupported
	}
	return driver.AssetPacks.Open(name, path)
}

func validPackName(name string) bool {
	if name == "" || name == "base" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
