package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

// copyAssets copies the app's assets directory, beside nsgo.yaml, into
// dir/assets, where "asset:" images resolve next to the executable.
func copyAssets(p *project, dir string) error {
	src := filepath.Join(p.Dir, "assets")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	dst := filepath.Join(dir, "assets")
	os.RemoveAll(dst)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}
