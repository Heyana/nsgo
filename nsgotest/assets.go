//go:build !js

package nsgotest

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// assetReader locates the project resources for a headless app. The
// tool names the project when testing across multiple Go packages.
func assetReader() func(context.Context, string) ([]byte, error) {
	root := os.Getenv("NSGO_TEST_RESOURCES")
	if root == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "nsgo.yaml")); err == nil {
				break
			}
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return nil
			}
			dir = parent
		}
		root = filepath.Join(dir, "resources")
	}
	return func(ctx context.Context, path string) ([]byte, error) {
		return driver.ReadAssetFile(ctx, root, path)
	}
}
