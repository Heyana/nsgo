package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAppLinksConfig(t *testing.T) {
	for _, tc := range []struct{ hosts, wantError string }{
		{"[EXAMPLE.com, shop.example.com]", ""},
		{"[example.com, EXAMPLE.COM]", "listed twice"},
		{"['https://example.com']", "explicit DNS host"},
		{"['example.com/path']", "explicit DNS host"},
		{"['example.com:443']", "explicit DNS host"},
		{"['*.example.com']", "explicit DNS host"},
		{"['127.0.0.1']", "explicit DNS host"},
		{"['localhost']", "explicit DNS host"},
		{"['" + strings.Repeat("x", 64) + ".example.com']", "longer than 63"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\nid: dev.example.app\napp_links: "+tc.hosts+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if tc.wantError != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("%s: %v", tc.hosts, err)
			}
			continue
		}
		if err != nil || !slices.Equal(p.AppLinks, []string{"example.com", "shop.example.com"}) {
			t.Fatalf("normalized hosts=%v, error=%v", p, err)
		}
	}
}
