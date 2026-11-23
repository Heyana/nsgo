//go:build android || nsgoandroidhost

package android

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func init() {
	driver.AssetPacks.Ready = androidAssetPackReady
	driver.AssetPacks.Open = openAndroidAssetPackFile
}

func androidAssetPackReady(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f, err := openAndroidAssetPackFile(name, "_nsgo_pack")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, f.Close()
}

// Installed split APKs sit beside base.apk in the package directory.
func openAndroidAssetPackFile(name, path string) (io.ReadCloser, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(filepath.Dir(exe), "..", "..")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".apk") {
			continue
		}
		archive, err := zip.OpenReader(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		file, err := archive.Open("assets/nsgo_asset_packs/" + name + "/" + path)
		if err == nil {
			return &apkAssetFile{ReadCloser: file, archive: archive}, nil
		}
		archive.Close()
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fs.ErrNotExist
}

type apkAssetFile struct {
	io.ReadCloser
	archive *zip.ReadCloser
}

func (f *apkAssetFile) Close() error {
	return errors.Join(f.ReadCloser.Close(), f.archive.Close())
}
