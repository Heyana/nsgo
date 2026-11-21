package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

type assetPackConfig struct {
	Name     string `yaml:"name"`
	Dir      string `yaml:"dir"`
	Delivery string `yaml:"delivery"`
}

func (p *project) validateAssetPacks() error {
	names := map[string]bool{}
	for i := range p.Mobile.AssetPacks {
		pack := &p.Mobile.AssetPacks[i]
		if !apk.ValidAssetPackName(pack.Name) || names[pack.Name] {
			return fmt.Errorf("mobile.asset_packs has invalid or duplicate name %q", pack.Name)
		}
		names[pack.Name] = true
		switch pack.Delivery {
		case "install-time", "fast-follow":
		default:
			return fmt.Errorf("mobile.asset_packs %s: delivery must be install-time or fast-follow", pack.Name)
		}
		if pack.Dir == "" {
			return fmt.Errorf("mobile.asset_packs %s needs a dir", pack.Name)
		}
		pack.Dir = p.resolve(pack.Dir)
		info, err := os.Lstat(pack.Dir)
		if err != nil {
			return fmt.Errorf("mobile.asset_packs %s: %w", pack.Name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("mobile.asset_packs %s: %s must be a directory", pack.Name, rel(pack.Dir))
		}
	}
	return nil
}

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
