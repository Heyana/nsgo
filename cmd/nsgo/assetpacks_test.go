package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMobileAssetPackIsOutsideBaseResources(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "resources", "levels"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"nsgo.yaml":                  "name: Test\nmobile:\n  asset_packs:\n  - {name: levels, dir: resources/levels, delivery: install-time}\n",
		"resources/levels/level.txt": "separate",
		"resources/icon.txt":         "base",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var resources []string
	if err := walkMobileResources(p, func(name string, _ []byte) error {
		resources = append(resources, name)
		return nil
	}); err != nil || len(resources) != 1 || resources[0] != "icon.txt" {
		t.Fatalf("base resources = %v, %v", resources, err)
	}
	packs, err := p.storeAssetPacks()
	if err != nil || len(packs) != 1 || len(packs[0].Files) != 1 || packs[0].Files[0].Name != "level.txt" {
		t.Fatalf("asset pack files = %+v, %v", packs, err)
	}
}
