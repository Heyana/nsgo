package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// Download streams an appcast package to a temporary file in dir, verifies its
// size, SHA-256 and Ed25519 signature directly from the streamed file, and
// atomically moves it into place. Partial or unverified files are removed on
// any error or cancellation.
func Download(ctx context.Context, update driver.Update, dir string, progress func(driver.UpdateDownloadProgress)) (string, error) {
	if update.Version == "" || update.Download == "" {
		return "", errors.New("updater: update has no downloadable package")
	}
	u, err := url.Parse(update.Download)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("updater: update download is not an HTTP URL")
	}
	if imageload.HTTPToFile == nil && imageload.HTTP == nil {
		return "", errors.New("updater: no platform HTTP client")
	}
	key, sig, err := decodeSignature(update)
	if err != nil {
		return "", err
	}
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("updater: create download directory: %w", err)
	}
	name := filepath.Base(u.Path)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "update-" + strings.ReplaceAll(update.Version, "/", "-")
	}
	part, err := os.CreateTemp(dir, ".nsgo-update-*")
	if err != nil {
		return "", fmt.Errorf("updater: create staging file: %w", err)
	}
	partName := part.Name()
	defer os.Remove(partName)
	if err := part.Close(); err != nil {
		return "", err
	}
	report := func(loaded, total int64) {
		if total <= 0 && update.Size > 0 {
			total = update.Size
		}
		if progress != nil {
			progress(driver.UpdateDownloadProgress{Loaded: loaded, Total: total})
		}
	}
	headers := imageload.Header{"Cache-Control": {"no-cache"}}
	var status int
	if imageload.HTTPToFile != nil {
		resp, err := imageload.HTTPToFile(ctx, update.Download, headers, partName, report)
		if err != nil {
			return "", fmt.Errorf("updater: download: %w", err)
		}
		status = resp.Status
	} else {
		resp, err := imageload.HTTP(ctx, update.Download, headers, report)
		if err != nil {
			return "", fmt.Errorf("updater: download: %w", err)
		}
		status = resp.Status
		if status == 200 {
			if err := os.WriteFile(partName, resp.Body, 0o600); err != nil {
				return "", fmt.Errorf("updater: stage download: %w", err)
			}
		}
	}
	if status != 200 {
		return "", fmt.Errorf("updater: download answered %d", status)
	}
	if err := verifyFile(partName, update, key, sig); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("updater: replace old download: %w", err)
	}
	if err := os.Rename(partName, dst); err != nil {
		return "", fmt.Errorf("updater: commit download: %w", err)
	}
	return dst, nil
}

func decodeSignature(update driver.Update) (ed25519.PublicKey, []byte, error) {
	if update.Signature == "" || update.Key == "" {
		return nil, nil, errors.New("updater: update package is not signed with the app's key")
	}
	key, err := base64.StdEncoding.DecodeString(update.Key)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, nil, errors.New("updater: update key is not an Ed25519 public key")
	}
	sig, err := base64.StdEncoding.DecodeString(update.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, nil, errors.New("updater: update signature is invalid")
	}
	return ed25519.PublicKey(key), sig, nil
}

func verifyFile(path string, update driver.Update, key ed25519.PublicKey, sig []byte) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("updater: open staged download: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("updater: stat staged download: %w", err)
	}
	size := info.Size()
	if update.Size > 0 && size != update.Size {
		return fmt.Errorf("updater: download size %d, want %d", size, update.Size)
	}
	if update.SHA256 != "" {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("updater: hash download: %w", err)
		}
		got := fmt.Sprintf("%x", h.Sum(nil))
		if !strings.EqualFold(got, strings.TrimSpace(update.SHA256)) {
			return fmt.Errorf("updater: SHA-256 mismatch: got %s, want %s", got, update.SHA256)
		}
	}
	ok, err := verifyEd25519File(f, size, key, sig)
	if err != nil {
		return fmt.Errorf("updater: verify download: %w", err)
	}
	if !ok {
		return errors.New("updater: update signature is invalid")
	}
	return nil
}
