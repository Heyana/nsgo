package pkg

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestCapabilitiesTravelInPackages(t *testing.T) {
	k := newKeys(t)
	caps := Capabilities{Network: &Network{Hosts: []string{"api.example.com"}}, Host: []string{"search"}, Clipboard: true}
	data, err := Build(Contents{App: App{ID: "com.example.app", Build: 1}, Source: "mount(1)", Capabilities: caps}, k.release, k.cert)
	if err != nil {
		t.Fatal(err)
	}
	p, err := k.trust().Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Capabilities.Allows("network", "https://api.example.com/x", "GET") || !p.Capabilities.Clipboard ||
		p.Capabilities.Allows("network", "https://evil.io/", "GET") || len(p.Ignored) != 0 {
		t.Fatalf("read %+v, ignored %v", p.Capabilities, p.Ignored)
	}
	// Without a declaration nothing is granted.
	bare, err := k.trust().Verify(k.craft(t, map[string]any{}, []byte("mount(1)")))
	if err != nil {
		t.Fatal(err)
	}
	if bare.Capabilities.Allows("network", "https://api.example.com/", "GET") || bare.Capabilities.Clipboard {
		t.Fatal("a package without capabilities was granted some")
	}
	// A newer signer's: what this runtime does not know is not granted.
	newer, err := k.trust().Verify(k.craft(t, map[string]any{"capabilities": map[string]any{
		"clipboard": true,
		"camera":    true,
		"network":   map[string]any{"hosts": []string{"a.com"}, "limit": 3},
		"webview":   map[string]any{"hosts": []string{"b.com"}},
	}}, []byte("mount(1)")))
	if err != nil {
		t.Fatal(err)
	}
	if newer.Capabilities.Network != nil || !newer.Capabilities.Clipboard || newer.Capabilities.WebView == nil ||
		!slices.Equal(newer.Ignored, []string{"capability camera", "capability network"}) {
		t.Fatalf("read %+v, ignored %v", newer.Capabilities, newer.Ignored)
	}
}

func TestParseCapabilitiesStrictly(t *testing.T) {
	for _, bad := range []string{
		`{"camera": true}`,
		`{"network": {"hosts": ["a.com"], "limit": 3}}`,
		`{"network": {"http": [{"host": "a.com", "schemes": ["ftp"]}]}}`,
		`{"network": {"http": [{"host": "a.com", "paths": ["v1"]}]}}`,
		`{"network": {"hosts": ["https://a.com"]}}`,
		`{"host": ["a/../b"]}`,
		`{"clipboard": "yes"}`,
		`{"webview": {"hosts": ["a.com"], "media": ["screen"]}}`,
		`{"fs": {"read": ["relative/dir"]}}`,
		`{"fs": {"read": [""]}}`,
		`{"fs": {"read": ["/tmp"], "write": ["/tmp"]}}`,
	} {
		if _, _, err := ParseCapabilities([]byte(bad), true); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	c, _, err := ParseCapabilities([]byte(`{"host": true, "openUrl": true}`), true)
	if err != nil || !c.Allows("host", "anything.at:all", "") || !c.Allows("openUrl", "mailto:a@b.c", "") {
		t.Fatalf("true: %+v %v", c, err)
	}
	// Leniently a rule with a key unknown goes alone.
	c, _, err = ParseCapabilities([]byte(`{"network": {"hosts": ["a.com"], "http": [{"host": "b.com", "new": 1}, {"host": "c.com"}]}}`), false)
	if err != nil || len(c.Network.HTTP) != 1 || c.Network.HTTP[0].Host != "c.com" {
		t.Fatalf("lenient rules: %+v %v", c.Network, err)
	}
}

func TestCapabilitiesAllow(t *testing.T) {
	c := Capabilities{
		Network: &Network{Hosts: []string{"*.cdn.com"}, HTTP: []HTTPRule{
			{Host: "api.com", Methods: []string{"GET"}, Schemes: []string{"https"}, PathPrefixes: []string{"/v1"}},
			{Host: "ws.com", Port: 8443, Paths: []string{"/live"}},
		}},
		WebView: &WebView{Hosts: []string{"docs.com"}, Media: []string{"microphone"}},
		OpenURL: &OpenURL{Schemes: []string{"https"}},
		Host:    []string{"player.*", "search"}, Biometrics: true, Secrets: true, Audio: true, Gamepad: true, AuthSession: true, SignInWithApple: true, Purchases: true, TextToSpeech: true,
	}
	for _, tc := range []struct {
		permission, resource, method string
		want                         bool
	}{
		{"network", "https://img.cdn.com/a.png", "GET", true},
		{"network", "https://cdn.com/a.png", "GET", false},
		{"network", "https://api.com/v1/x?q=1", "get", true},
		{"network", "https://api.com/v1", "GET", true},
		{"network", "https://api.com/v10", "GET", false},
		{"network", "https://api.com/v1/../v2/secret", "GET", false},
		{"network", "https://api.com/v1/%2e%2e/secret", "GET", false},
		{"network", "https://api.com\\@evil.io/v1/x", "GET", false},
		{"network", "https://api.com/v1/x", "POST", false},
		{"network", "http://api.com/v1/x", "GET", false},
		{"network", "https://api.com@evil.io/v1/", "GET", false},
		{"network", "wss://ws.com:8443/live", "GET", true},
		{"network", "wss://ws.com/live", "GET", false},
		{"network", "file:///etc/hosts", "GET", false},
		{"webview", "https://docs.com/a", "", true},
		{"webview", "https://evil.io/", "", false},
		{"webview", "", "input", false},
		{"webview", "microphone", "media", true},
		{"webview", "camera", "media", false},
		{"webview", "https://docs.com/", "cookies", false},
		{"webview", "", "cookies", false},
		{"openUrl", "https://a.com", "", true},
		{"openUrl", "tel:123", "", false},
		{"host", "player.play", "", true},
		{"host", "search", "", true},
		{"host", "searchAll", "", false},
		{"host", "player..x", "", false},
		{"clipboard", "", "", false},
		{"biometrics", "", "", true},
		{"signInWithApple", "", "", true},
		{"purchases", "", "", true},
		{"secrets", "", "", true},
		{"audio", "", "", true},
		{"gamepad", "", "", true},
		{"authSession", "", "", true},
		{"textToSpeech", "", "", true},
		{"speech", "", "", false},
	} {
		if got := c.Allows(tc.permission, tc.resource, tc.method); got != tc.want {
			t.Errorf("Allows(%s, %s, %s) = %v", tc.permission, tc.resource, tc.method, got)
		}
	}
}

func TestFSReadCapability(t *testing.T) {
	// Unknown keys in fs drop the capability leniently so newer fields never widen access.
	parsed, ignored, err := ParseCapabilities([]byte(`{"fs":{"read":["/tmp"],"future":true}}`), false)
	if err != nil || parsed.FS != nil || !slices.Equal(ignored, []string{"capability fs"}) {
		t.Fatalf("lenient fs = %+v, ignored %v, err %v", parsed.FS, ignored, err)
	}
	if runtime.GOOS == "js" {
		return
	}

	base := t.TempDir()
	allowed := filepath.Join(base, "media")
	sibling := filepath.Join(base, "media2")
	outside := filepath.Join(base, "secret")
	for _, d := range []string{filepath.Join(allowed, "sub"), sibling, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inFile := filepath.Join(allowed, "sub", "photo.png")
	siblingFile := filepath.Join(sibling, "other.png")
	secretFile := filepath.Join(outside, "key.png")
	for _, f := range []string{inFile, siblingFile, secretFile} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	safeLink := filepath.Join(allowed, "safe-link.png")
	escapeLink := filepath.Join(allowed, "escape-link.png")
	_ = os.Symlink(inFile, safeLink)
	_ = os.Symlink(secretFile, escapeLink)

	c := Capabilities{FS: &FS{Read: []string{allowed}}}
	cases := []struct {
		name   string
		path   string
		method string
		want   bool
	}{
		{"inside root", inFile, "read", true},
		{"cleaned inside root", filepath.Join(allowed, "sub", "..", "sub", "photo.png"), "read", true},
		{"symlink staying inside root", safeLink, "read", true},
		{"symlink escaping root", escapeLink, "read", false},
		{"parent traversal outside root", filepath.Join(allowed, "..", "secret", "key.png"), "read", false},
		{"sibling prefix dir", siblingFile, "read", false},
		{"outside root", secretFile, "read", false},
		{"root directory itself", allowed, "read", false},
		{"nonexistent file inside root", filepath.Join(allowed, "missing.png"), "read", false},
		{"write method refused", inFile, "write", false},
	}
	for _, tc := range cases {
		if got := c.Allows("fs", tc.path, tc.method); got != tc.want {
			t.Errorf("%s: Allows(fs, %q, %q) = %v, want %v", tc.name, tc.path, tc.method, got, tc.want)
		}
	}
}
