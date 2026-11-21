package driver

import (
	"context"
	"os"
)

// ReadAssetFile reads a resource without following symlinks outside root.
func ReadAssetFile(ctx context.Context, root, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadFile(name)
}
