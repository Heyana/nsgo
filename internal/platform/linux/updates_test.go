//go:build linux && !android && !nsgo_steam

package linux

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCommand puts a shell script named name into dir.
func fakeCommand(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSelfUpdatableAsksThePackageManagerThatOwnsTheApp(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", "")
	t.Setenv("APPDIR", "")
	escaped := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(exe)
	bin := t.TempDir()
	// dpkg owns the running executable; pacman owns nothing.
	fakeCommand(t, bin, "dpkg-query", `[ "$1" = --search ] && [ "$2" = '`+escaped+`' ]`)
	fakeCommand(t, bin, "pacman", "exit 1")
	t.Setenv("PATH", bin)
	ctx := context.Background()
	for download, want := range map[string]bool{
		"https://example.com/v1.2/hello-1.2-linux-x64.deb":         true,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.gz":  false,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.zst": false,
		"https://example.com/v1.2/hello-1.2-linux-x64.AppImage":    false,
		"https://example.com/v1.2/hello-1.2-linux-x64.tar.gz":      false,
		"https://example.com/v1.2/hello-1.2-linux-x64.deb?x=1.zip": true,
	} {
		if got := selfUpdatable(ctx, download); got != want {
			t.Errorf("selfUpdatable(%q) = %v, want %v", download, got, want)
		}
	}
	// Arch install: pacman owns the running executable; dpkg owns nothing.
	fakeCommand(t, bin, "dpkg-query", "exit 1")
	fakeCommand(t, bin, "pacman", `[ "$1" = --query ] && [ "$2" = --owns ] && [ "$3" = '`+exe+`' ]`)
	for download, want := range map[string]bool{
		"https://example.com/v1.2/hello-1.2-linux-x64.deb":         false,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.gz":  true,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.zst": true,
		"https://example.com/v1.2/hello-1.2-linux-x64.AppImage":    false,
	} {
		if got := selfUpdatable(ctx, download); got != want {
			t.Errorf("arch selfUpdatable(%q) = %v, want %v", download, got, want)
		}
	}
	// AppImage install: exe lives inside APPDIR and APPIMAGE names the image.
	image := filepath.Join(t.TempDir(), "hello.AppImage")
	t.Setenv("APPIMAGE", image)
	t.Setenv("APPDIR", filepath.Dir(exe))
	for download, want := range map[string]bool{
		"https://example.com/v1.2/hello-1.2-linux-x64.deb":        false,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.gz": false,
		"https://example.com/v1.2/hello-1.2-linux-x64.AppImage":   true,
		"https://example.com/v1.2/hello-1.2-linux-x64.tar.gz":     false,
	} {
		if got := selfUpdatable(ctx, download); got != want {
			t.Errorf("appimage selfUpdatable(%q) = %v, want %v", download, got, want)
		}
	}
	// A system without dpkg or pacman installs neither package.
	t.Setenv("APPIMAGE", "")
	t.Setenv("APPDIR", "")
	t.Setenv("PATH", t.TempDir())
	if selfUpdatable(ctx, "https://example.com/hello-1.2-linux-x64.deb") {
		t.Error("a Debian package is installable without dpkg")
	}
}

func TestInstallUpdateRunsThePackageManagerAndReplacesAppImage(t *testing.T) {
	bin, dir := t.TempDir(), t.TempDir()
	log := filepath.Join(dir, "pkexec.log")
	fakeCommand(t, bin, "pkexec", `echo "$@" >> '`+log+`'`)
	t.Setenv("PATH", bin)
	install := func(path string) error {
		errs := make(chan error, 1)
		installUpdate(path, func(err error) { errs <- err })
		return <-errs
	}
	for _, path := range []string{
		"/cache/updates/hello-1.2-linux-x64.deb",
		"/cache/updates/hello-1.2-linux-x64.pkg.tar.gz",
		"/cache/updates/hello-1.2-linux-x64.pkg.tar.zst",
	} {
		if err := install(path); err != nil {
			t.Errorf("install %s: %v", path, err)
		}
	}
	if err := install("/cache/updates/hello-1.2-linux-x64.tar.gz"); err == nil {
		t.Error("a plain archive installed")
	}
	got, _ := os.ReadFile(log)
	want := "dpkg --install /cache/updates/hello-1.2-linux-x64.deb\n" +
		"pacman --upgrade --noconfirm /cache/updates/hello-1.2-linux-x64.pkg.tar.gz\n" +
		"pacman --upgrade --noconfirm /cache/updates/hello-1.2-linux-x64.pkg.tar.zst\n"
	if string(got) != want {
		t.Errorf("pkexec ran %q, want %q", got, want)
	}
	// AppImage replaces $APPIMAGE atomically and sets executable bits.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "Hello.AppImage")
	if err := os.WriteFile(target, []byte("old-appimage"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(t.TempDir(), "hello-1.2-linux-x64.AppImage")
	if err := os.WriteFile(staged, []byte("new-appimage"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", target)
	t.Setenv("APPDIR", filepath.Dir(exe))
	if err := install(staged); err != nil {
		t.Fatalf("install AppImage: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "new-appimage" {
		t.Fatalf("AppImage content = %q, %v", data, err)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("AppImage mode = %v, %v", info.Mode(), err)
	}
	// A refused authentication, which pkexec answers with 126, fails the install.
	fakeCommand(t, bin, "pkexec", "exit 126")
	if err := install("/cache/updates/hello-1.2-linux-x64.deb"); err == nil || !strings.Contains(err.Error(), "126") {
		t.Errorf("a refused authentication answered %v", err)
	}
}
