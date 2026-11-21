//go:build js && wasm && nsgoharmony && nsgotest

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestHarmonyOSSandboxFiles(t *testing.T) {
	var dataDir, cacheDir, logsDir string
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		dataDir, cacheDir, logsDir = cx.DataDir(), cx.CacheDir(), cx.LogsDir()
		return core.View{}
	})
	for _, dir := range []string{dataDir, cacheDir, logsDir} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("sandbox directory %q: %v", dir, err)
		}
	}

	dir := filepath.Join(dataDir, "nested", "files")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings")
	if err := os.WriteFile(path, []byte("old value"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(dir, "settings-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	want := bytes.Repeat([]byte{0, 1, 127, 128, 255}, 28_000)
	if _, err := file.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("read replaced file: got %d bytes, want %d, error %v", len(got), len(want), err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "settings" {
		t.Fatalf("read directory: %v, %v", entries, err)
	}
	if err := os.Remove(dir); err == nil {
		t.Fatal("Remove deleted a nonempty directory")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat after removal: %v", err)
	}
	if _, err := os.Stat("/"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("stat outside the app directories: %v", err)
	}
}
