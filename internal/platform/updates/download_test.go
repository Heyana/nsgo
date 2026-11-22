package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func signedUpdateFixture(t *testing.T, url string, body []byte) driver.Update {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return driver.Update{
		Version:   "2.0",
		Download:  url,
		Size:      int64(len(body)),
		SHA256:    fmt.Sprintf("%x", sum),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, body)),
		Key:       base64.StdEncoding.EncodeToString(public),
	}
}

func TestDownloadVerifiesAndStagesPackage(t *testing.T) {
	body := []byte("signed update")
	u := signedUpdateFixture(t, "https://example.com/hello-2.0.apk", body)
	wasHTTP, wasFile := imageload.HTTP, imageload.HTTPToFile
	imageload.HTTPToFile = nil
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		return &imageload.Response{Status: 200, Body: body}, nil
	}
	t.Cleanup(func() { imageload.HTTP, imageload.HTTPToFile = wasHTTP, wasFile })
	path, err := Download(context.Background(), u, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "hello-2.0.apk" {
		t.Fatalf("path %q", path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(body) {
		t.Fatalf("staged body %q, %v", got, err)
	}
}

func TestDownloadStreamsToFileWithProgress(t *testing.T) {
	body := bytes.Repeat([]byte("streamed-update-payload-"), 8192)
	u := signedUpdateFixture(t, "https://example.com/hello-2.0-linux-x64.AppImage", body)
	wasHTTP, wasFile := imageload.HTTP, imageload.HTTPToFile
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		t.Fatal("buffered HTTP called while HTTPToFile is available")
		return nil, nil
	}
	imageload.HTTPToFile = func(_ context.Context, _ string, _ imageload.Header, dst string, progress imageload.Progress) (*imageload.Response, error) {
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		const chunk = 16 << 10
		var written int64
		for off := 0; off < len(body); off += chunk {
			end := min(off+chunk, len(body))
			n, err := f.Write(body[off:end])
			if err != nil {
				return nil, err
			}
			written += int64(n)
			// Simulate a server omitting Content-Length (total=0); Download falls back to update.Size.
			progress(written, 0)
		}
		return &imageload.Response{Status: 200}, nil
	}
	t.Cleanup(func() { imageload.HTTP, imageload.HTTPToFile = wasHTTP, wasFile })

	dir := t.TempDir()
	var events []driver.UpdateDownloadProgress
	path, err := Download(context.Background(), u, dir, func(p driver.UpdateDownloadProgress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "hello-2.0-linux-x64.AppImage" {
		t.Fatalf("path %q", path)
	}
	if len(events) < 2 {
		t.Fatalf("progress events = %d, want multiple chunks", len(events))
	}
	for i, ev := range events {
		if ev.Total != int64(len(body)) {
			t.Fatalf("event[%d].Total = %d, want %d", i, ev.Total, len(body))
		}
		if i > 0 && ev.Loaded <= events[i-1].Loaded {
			t.Fatalf("progress not monotonic: %+v", events)
		}
	}
	if last := events[len(events)-1]; last.Loaded != int64(len(body)) {
		t.Fatalf("final loaded = %d, want %d", last.Loaded, len(body))
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("staged file mismatch: err=%v len=%d", err, len(got))
	}
	// Only the committed package should remain in dir; no temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "hello-2.0-linux-x64.AppImage" {
		t.Fatalf("dir entries = %v, %v", entries, err)
	}
}

func TestDownloadCleansUpPartialFilesOnCancelAndErrors(t *testing.T) {
	body := []byte("signed update package bytes")
	valid := signedUpdateFixture(t, "https://example.com/hello-2.0-linux-x64.pkg.tar.zst", body)

	cases := []struct {
		name   string
		update driver.Update
		stream func(ctx context.Context, dst string, progress imageload.Progress) (*imageload.Response, error)
		cancel bool
	}{
		{
			name:   "cancelled mid-stream",
			update: valid,
			cancel: true,
			stream: func(ctx context.Context, dst string, progress imageload.Progress) (*imageload.Response, error) {
				if err := os.WriteFile(dst, body[:10], 0o600); err != nil {
					return nil, err
				}
				progress(10, int64(len(body)))
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		{
			name:   "non-200 status",
			update: valid,
			stream: func(_ context.Context, dst string, _ imageload.Progress) (*imageload.Response, error) {
				_ = os.WriteFile(dst, []byte("not found"), 0o600)
				return &imageload.Response{Status: 404}, nil
			},
		},
		{
			name:   "size mismatch",
			update: valid,
			stream: func(_ context.Context, dst string, _ imageload.Progress) (*imageload.Response, error) {
				return &imageload.Response{Status: 200}, os.WriteFile(dst, body[:len(body)-1], 0o600)
			},
		},
		{
			name: "sha256 mismatch",
			update: func() driver.Update {
				u := valid
				u.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
				return u
			}(),
			stream: func(_ context.Context, dst string, _ imageload.Progress) (*imageload.Response, error) {
				return &imageload.Response{Status: 200}, os.WriteFile(dst, body, 0o600)
			},
		},
		{
			name:   "bad Ed25519 signature",
			update: valid,
			stream: func(_ context.Context, dst string, _ imageload.Progress) (*imageload.Response, error) {
				tampered := append([]byte(nil), body...)
				tampered[0] ^= 0xff
				u := tampered
				return &imageload.Response{Status: 200}, os.WriteFile(dst, u, 0o600)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wasHTTP, wasFile := imageload.HTTP, imageload.HTTPToFile
			imageload.HTTP = nil
			t.Cleanup(func() { imageload.HTTP, imageload.HTTPToFile = wasHTTP, wasFile })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			imageload.HTTPToFile = func(ctx context.Context, _ string, _ imageload.Header, dst string, progress imageload.Progress) (*imageload.Response, error) {
				if tc.cancel {
					cancel()
				}
				return tc.stream(ctx, dst, progress)
			}
			dir := t.TempDir()
			u := tc.update
			if tc.name == "bad Ed25519 signature" {
				// Clear SHA256 so verification reaches Ed25519 directly.
				u.SHA256 = ""
			}
			_, err := Download(ctx, u, dir, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel error = %v, want context.Canceled", err)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("failed download left files in staging dir: %v", entries)
			}
		})
	}
}

func TestDownloadRejectsBadSignature(t *testing.T) {
	wasHTTP, wasFile := imageload.HTTP, imageload.HTTPToFile
	imageload.HTTPToFile = nil
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		return &imageload.Response{Status: 200, Body: []byte("actual")}, nil
	}
	t.Cleanup(func() { imageload.HTTP, imageload.HTTPToFile = wasHTTP, wasFile })
	public, _, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	_, err := Download(context.Background(), driver.Update{
		Version: "2.0", Download: "https://example.com/update.apk", Signature: base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize)), Key: base64.StdEncoding.EncodeToString(public),
	}, dir, nil)
	if err == nil {
		t.Fatal("bad signature accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("bad signature left files: %v", entries)
	}
}

func TestDownloadRefusesUnsignedPackage(t *testing.T) {
	body := []byte("unsigned update")
	sum := sha256.Sum256(body)
	wasHTTP, wasFile := imageload.HTTP, imageload.HTTPToFile
	imageload.HTTPToFile = nil
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		return &imageload.Response{Status: 200, Body: body}, nil
	}
	t.Cleanup(func() { imageload.HTTP, imageload.HTTPToFile = wasHTTP, wasFile })
	public, _, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	for _, u := range []driver.Update{
		{Version: "2.0", Download: "https://example.com/update.deb", SHA256: fmt.Sprintf("%x", sum)},
		{Version: "2.0", Download: "https://example.com/update.deb", SHA256: fmt.Sprintf("%x", sum), Key: base64.StdEncoding.EncodeToString(public)},
	} {
		if _, err := Download(context.Background(), u, dir, nil); err == nil {
			t.Errorf("unsigned %+v accepted", u)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("an unsigned package left %v", entries)
	}
}
