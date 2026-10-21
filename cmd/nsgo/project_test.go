package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHotReload(t *testing.T) {
	for _, tt := range []struct {
		yaml string
		want bool
	}{
		{"", false},
		{"  hot_reload: true\n", true},
		{"  hot_reload: false\n", false},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "nsgo.yaml")
		if err := os.WriteFile(path, []byte("name: Test\ndev:\n"+tt.yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.hotReload(); got != tt.want {
			t.Errorf("dev.hot_reload %q: got %v, want %v", tt.yaml, got, tt.want)
		}
	}
}

func TestBuildOpen(t *testing.T) {
	for _, tt := range []struct {
		yaml string
		want bool
	}{
		{"", true},
		{"open: true\n", true},
		{"open: false\n", false},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "nsgo.yaml")
		if err := os.WriteFile(path, []byte("name: Test\nbuild:\n  "+tt.yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.openOutput(); got != tt.want {
			t.Errorf("build.open %q: got %v, want %v", tt.yaml, got, tt.want)
		}
	}
}
