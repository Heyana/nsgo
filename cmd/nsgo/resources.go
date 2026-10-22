package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

// resourcesDir is the directory beside nsgo.yaml that packages carry.
// Files the project keeps but does not ship, such as its icons, go in
// assets instead.
const resourcesDir = "resources"

// copyResources copies the app's resources directory into root, the
// package's resource root, where "asset:" images resolve.
func copyResources(p *project, root string) error {
	return walkResources(p, func(rel string, data []byte) error {
		out := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

// walkResources calls f with each file under the app's resources
// directory, by its slash-separated path inside it.
func walkResources(p *project, f func(rel string, data []byte) error) error {
	src := filepath.Join(p.Dir, resourcesDir)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return f(filepath.ToSlash(rel), data)
	})
}
