//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapLoginUsesDeclaredAutostartAndUserData(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "meta"), 0700); err != nil {
		t.Fatal(err)
	}
	metadata := "apps:\n  editor:\n    command: bin/editor\n    autostart: editor-login.desktop\n  viewer:\n    command: bin/viewer\n    autostart: viewer.desktop\n"
	if err := os.WriteFile(filepath.Join(root, "meta/snap.yaml"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e, err := snapLoginEntry("dev.nsgo.app", "NSGo test", filepath.Join(root, "bin/editor"), root, data, "demo", "snap.demo.editor (enforce)\n")
	if err != nil {
		t.Fatal(err)
	}
	if e.path != filepath.Join(data, ".config/autostart/editor-login.desktop") {
		t.Fatalf("autostart path %q", e.path)
	}
	if err := e.set(true); err != nil {
		t.Fatal(err)
	}
	if !e.enabled() {
		t.Fatal("Snap registration did not persist")
	}
	b, err := os.ReadFile(e.path)
	if err != nil || !strings.Contains(string(b), "--nsgo-launched-at-login") {
		t.Fatalf("startup command %q, %v", b, err)
	}
	other := filepath.Join(filepath.Dir(e.path), "viewer.desktop")
	if err := os.WriteFile(other, []byte("other app"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.set(false); err != nil {
		t.Fatal(err)
	}
	if e.enabled() {
		t.Fatal("Snap registration remained enabled")
	}
	if b, err := os.ReadFile(other); err != nil || string(b) != "other app" {
		t.Fatal("another Snap app registration changed")
	}
	// AppArmor identifies a shared executable's app; unconfined runs may use
	// the executable when metadata identifies exactly one matching app.
	if e, err := snapLoginEntry("dev.nsgo.app", "", filepath.Join(root, "bin/viewer"), root, data, "demo", "unconfined"); err != nil || filepath.Base(e.path) != "viewer.desktop" {
		t.Fatalf("unconfined app %+v, %v", e, err)
	}
}

func TestSnapLoginRejectsUnknownOrUnsafeMetadata(t *testing.T) {
	for _, tc := range []struct{ name, metadata, exe, label string }{
		{"missing declaration", "apps:\n  app:\n    command: bin/app\n", "bin/app", "snap.demo.app (enforce)"},
		{"traversal", "apps:\n  app:\n    autostart: ../another.desktop\n", "bin/app", "snap.demo.app (enforce)"},
		{"backslash", "apps:\n  app:\n    autostart: 'other\\file.desktop'\n", "bin/app", "snap.demo.app (enforce)"},
		{"shared executable", "apps:\n  first:\n    command: bin/app\n    autostart: first.desktop\n  second:\n    command: bin/app\n    autostart: second.desktop\n", "bin/app", "unconfined"},
		{"inherited environment", "apps:\n  app:\n    autostart: app.desktop\n", "../outside/app", "snap.demo.app (enforce)"},
		{"unknown app", "apps:\n  app:\n    autostart: app.desktop\n", "bin/app", "snap.demo.other (enforce)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "meta"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "meta/snap.yaml"), []byte(tc.metadata), 0600); err != nil {
				t.Fatal(err)
			}
			if e, err := snapLoginEntry("dev.nsgo.test", "Test", filepath.Join(root, tc.exe), root, t.TempDir(), "demo", tc.label); err == nil {
				t.Fatalf("unsafe metadata accepted: %+v", e)
			}
		})
	}
}
