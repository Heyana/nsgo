package updater_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/updater"
)

func TestDownloadDelegatesToPlatform(t *testing.T) {
	was := driver.UpdateDownload
	driver.UpdateDownload = func(context.Context, driver.Update, string, func(driver.UpdateDownloadProgress)) (string, error) {
		return filepath.Join("/tmp", "hello-2.0.apk"), nil
	}
	t.Cleanup(func() { driver.UpdateDownload = was })
	path, err := updater.Download(context.Background(), core.UpdateCheck{Version: "2.0"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "hello-2.0.apk" {
		t.Fatalf("path %q", path)
	}
}

func TestDownloadForwardsProgress(t *testing.T) {
	was := driver.UpdateDownload
	driver.UpdateDownload = func(_ context.Context, _ driver.Update, dir string, progress func(driver.UpdateDownloadProgress)) (string, error) {
		progress(driver.UpdateDownloadProgress{Loaded: 256, Total: 1024})
		progress(driver.UpdateDownloadProgress{Loaded: 1024, Total: 1024})
		return filepath.Join(dir, "hello-2.0.AppImage"), nil
	}
	t.Cleanup(func() { driver.UpdateDownload = was })

	var got []updater.DownloadProgress
	path, err := updater.Download(context.Background(), core.UpdateCheck{Version: "2.0", Download: "https://example.com/hello-2.0.AppImage"}, t.TempDir(), func(p updater.DownloadProgress) {
		got = append(got, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "hello-2.0.AppImage" {
		t.Fatalf("path %q", path)
	}
	want := []updater.DownloadProgress{{Loaded: 256, Total: 1024}, {Loaded: 1024, Total: 1024}}
	if !slices.Equal(got, want) {
		t.Fatalf("progress = %+v, want %+v", got, want)
	}
}

func TestDownloadPropagatesPlatformError(t *testing.T) {
	was := driver.UpdateDownload
	driver.UpdateDownload = func(context.Context, driver.Update, string, func(driver.UpdateDownloadProgress)) (string, error) {
		return "", fmt.Errorf("signature rejected")
	}
	t.Cleanup(func() { driver.UpdateDownload = was })
	_, err := updater.Download(context.Background(), core.UpdateCheck{Version: "2.0", Download: "https://example.com/update.apk"}, t.TempDir(), nil)
	if err == nil {
		t.Fatal("bad signature accepted")
	}
}
