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
	if err != nil || !strings.Contains(string(b), "Exec=editor --nsgo-launched-at-login\n") {
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
		{"percent in app name", "apps:\n  app%1:\n    command: bin/app\n    autostart: app.desktop\n", "bin/app", "snap.demo.app%1 (enforce)"},
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

func TestSnapLoginSurvivesRevisionRefresh(t *testing.T) {
	base := t.TempDir()
	rev10Root, rev11Root := filepath.Join(base, "snap", "demo", "10"), filepath.Join(base, "snap", "demo", "11")
	rev10Data, rev11Data := filepath.Join(base, "user", "demo", "10"), filepath.Join(base, "user", "demo", "11")
	for _, tc := range []struct{ root, meta string }{
		{rev10Root, "apps:\n  editor:\n    command: bin/editor\n    autostart: editor-login.desktop\n"},
		{rev11Root, "apps:\n  editor:\n    command: usr/bin/editor\n    autostart: editor-login.desktop\n"},
	} {
		if err := os.MkdirAll(filepath.Join(tc.root, "meta"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tc.root, "meta", "snap.yaml"), []byte(tc.meta), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e10, err := snapLoginEntry("dev.nsgo.app", "NSGo test", filepath.Join(rev10Root, "bin", "editor"), rev10Root, rev10Data, "demo", "snap.demo.editor (enforce)")
	if err != nil {
		t.Fatal(err)
	}
	if err := e10.set(true); err != nil {
		t.Fatal(err)
	}
	// Simulate snap refresh copying SNAP_USER_DATA from revision 10 to revision 11.
	copied, err := os.ReadFile(e10.path)
	if err != nil {
		t.Fatal(err)
	}
	rev11EntryPath := filepath.Join(rev11Data, ".config", "autostart", "editor-login.desktop")
	if err := os.MkdirAll(filepath.Dir(rev11EntryPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rev11EntryPath, copied, 0600); err != nil {
		t.Fatal(err)
	}
	e11, err := snapLoginEntry("dev.nsgo.app", "NSGo test", filepath.Join(rev11Root, "usr", "bin", "editor"), rev11Root, rev11Data, "demo", "snap.demo.editor (enforce)")
	if err != nil {
		t.Fatal(err)
	}
	if !e11.enabled() {
		t.Fatalf("Snap registration lost after revision refresh: file=%q want=%q", copied, e11.data())
	}
}
