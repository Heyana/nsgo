package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

func (p *project) storeAssetPacks() ([]apk.AssetPack, error) {
	var packs []apk.AssetPack
	for _, cfg := range p.Mobile.AssetPacks {
		pack := apk.AssetPack{Name: cfg.Name, Delivery: cfg.Delivery}
		err := filepath.WalkDir(cfg.Dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("asset pack %s: %s must be a regular file", cfg.Name, rel(path))
			}
			name, err := filepath.Rel(cfg.Dir, path)
			if err != nil {
				return err
			}
			pack.Files = append(pack.Files, apk.AssetPackFile{Name: filepath.ToSlash(name), Source: path})
			return nil
		})
		if err != nil {
			return nil, err
		}
		if len(pack.Files) == 0 {
			return nil, fmt.Errorf("asset pack %s has no files", cfg.Name)
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

func (p *project) inAssetPack(path string) bool {
	for _, pack := range p.Mobile.AssetPacks {
		if path == pack.Dir || strings.HasPrefix(path, pack.Dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
