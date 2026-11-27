package driver

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCanonicalHTTPURL(t *testing.T) {
	good := map[string]string{
		"https://EXAMPLE.COM/a.png":                        "https://example.com/a.png",
		"https://example.com:443/a.png":                    "https://example.com/a.png",
		"http://example.com:80":                            "http://example.com/",
		"http://example.com:8080/public/../private/a.png":  "http://example.com:8080/private/a.png",
		"https://example.com/a/%2e%2e/b.png?x=1":           "https://example.com/b.png?x=1",
		"http://[::1]:8080/img/./a.png":                    "http://[::1]:8080/img/a.png",
		"https://ok.com/a.png#frag":                        "https://ok.com/a.png#frag",
		"HTTPS://CDN.Example.Com:443/dir/sub/../../ok.png": "https://cdn.example.com/ok.png",
	}
	for raw, want := range good {
		got, err := CanonicalHTTPURL(raw)
		if err != nil || got != want {
			t.Errorf("CanonicalHTTPURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	bad := []string{
		"",
		" https://example.com/a.png",
		"https://example.com/a.png ",
		"https://example.com/a\r\nb.png",
		"https://example.com\\@evil.io/a.png",
		"https://example.com/a\\b.png",
		"https:opaque",
		"ftp://example.com/a.png",
		"file:///etc/passwd",
		"https:///no-host.png",
		"https://example.com:99999/a.png",
		"https://example.com:0/a.png",
		"https://example.com:abc/a.png",
	}
	for _, raw := range bad {
		if got, err := CanonicalHTTPURL(raw); err == nil {
			t.Errorf("CanonicalHTTPURL(%q) = %q, want error", raw, got)
		}
	}
}

func TestResolveLocalMediaPathAndCanonicalFileURL(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("no local filesystem on js")
	}
	dir := t.TempDir()
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "pic #1.png")
	if err := os.WriteFile(file, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(realDir, "sub", "pic #1.png")
	win := runtime.GOOS == "windows"
	fileURL := CanonicalFileURL(file, win)

	for _, raw := range []string{
		file,
		filepath.Join(sub, "..", "sub", "pic #1.png"),
		fileURL,
	} {
		got, err := ResolveLocalMediaPath(raw, win)
		if err != nil || got != wantPath {
			t.Errorf("ResolveLocalMediaPath(%q) = %q, %v; want %q", raw, got, err, wantPath)
		}
	}

	link := filepath.Join(dir, "alias.png")
	if err := os.Symlink(file, link); err == nil {
		got, err := ResolveLocalMediaPath(link, win)
		if err != nil || got != wantPath {
			t.Errorf("ResolveLocalMediaPath(symlink) = %q, %v; want %q", got, err, wantPath)
		}
	}

	for _, bad := range []string{
		"",
		"relative/pic.png",
		dir, // directory, not regular file
		filepath.Join(dir, "missing.png"),
		"file://evil.example/etc/passwd",
		fileURL + "?query=1",
		fileURL + "#frag",
		"file://user@localhost/etc/passwd",
		file + "\x00",
	} {
		if got, err := ResolveLocalMediaPath(bad, win); err == nil {
			t.Errorf("ResolveLocalMediaPath(%q) = %q, want error", bad, got)
		}
	}
}
