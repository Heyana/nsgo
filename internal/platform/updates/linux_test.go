package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestLinuxPackageFormat(t *testing.T) {
	for in, want := range map[string]LinuxPackageKind{
		"https://example.com/v1.2/hello-1.2-linux-x64.deb":          LinuxPackageDeb,
		"https://example.com/v1.2/hello-1.2-linux-x64.deb?sig=1":    LinuxPackageDeb,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.gz":   LinuxPackageArch,
		"https://example.com/v1.2/hello-1.2-linux-x64.pkg.tar.zst":  LinuxPackageArch,
		"https://example.com/v1.2/hello-1.2-linux-x64.AppImage":     LinuxPackageAppImage,
		"/cache/nsgo.updater/hello-1.2-linux-x64.pkg.tar.zst":       LinuxPackageArch,
		"/cache/nsgo.updater/hello-1.2-linux-x64.AppImage":          LinuxPackageAppImage,
		"https://example.com/v1.2/hello-1.2-linux-x64.tar.gz":       LinuxPackageNone,
		"https://example.com/v1.2/hello-1.2-linux-x64.appimage.zip": LinuxPackageNone,
	} {
		if got := LinuxPackageFormat(in); got != want {
			t.Errorf("LinuxPackageFormat(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLinuxInstallCommand(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/tmp/hello.deb", []string{"pkexec", "dpkg", "--install", "/tmp/hello.deb"}},
		{"/tmp/hello.pkg.tar.gz", []string{"pkexec", "pacman", "--upgrade", "--noconfirm", "/tmp/hello.pkg.tar.gz"}},
		{"/tmp/hello.pkg.tar.zst", []string{"pkexec", "pacman", "--upgrade", "--noconfirm", "/tmp/hello.pkg.tar.zst"}},
	} {
		got, err := LinuxInstallCommand(tc.path)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("LinuxInstallCommand(%q) = %v, %v; want %v", tc.path, got, err, tc.want)
		}
	}
	for _, unsupported := range []string{"/tmp/hello.tar.gz", "/tmp/hello.AppImage"} {
		if _, err := LinuxInstallCommand(unsupported); err == nil {
			t.Errorf("LinuxInstallCommand(%q) succeeded, want error", unsupported)
		}
	}
}

func TestLinuxOwnershipCommand(t *testing.T) {
	got, ok := LinuxOwnershipCommand("https://example.com/app.deb", `/opt/a*b?[c]\d/app`)
	want := []string{"dpkg-query", "--search", `/opt/a\*b\?\[c]\\d/app`}
	if !ok || !slices.Equal(got, want) {
		t.Fatalf("dpkg ownership cmd = %v, %v; want %v", got, ok, want)
	}
	for _, url := range []string{"https://example.com/app.pkg.tar.gz", "https://example.com/app.pkg.tar.zst"} {
		got, ok := LinuxOwnershipCommand(url, "/opt/app/app")
		want := []string{"pacman", "--query", "--owns", "/opt/app/app"}
		if !ok || !slices.Equal(got, want) {
			t.Fatalf("pacman ownership cmd for %q = %v, %v; want %v", url, got, ok, want)
		}
	}
	if _, ok := LinuxOwnershipCommand("https://example.com/app.AppImage", "/tmp/.mount/app"); ok {
		t.Fatal("AppImage should not use a package manager ownership command")
	}
}

func writeFakeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxSelfUpdatableDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	const exe = "/opt/hello/hello"
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	ctx := context.Background()

	// Arch install: pacman owns exe, dpkg does not.
	writeFakeScript(t, bin, "dpkg-query", "exit 1")
	writeFakeScript(t, bin, "pacman", `[ "$1" = --query ] && [ "$2" = --owns ] && [ "$3" = '/opt/hello/hello' ]`)
	for url, want := range map[string]bool{
		"https://example.com/hello-1.2-linux-x64.deb":         false,
		"https://example.com/hello-1.2-linux-x64.pkg.tar.gz":  true,
		"https://example.com/hello-1.2-linux-x64.pkg.tar.zst": true,
		"https://example.com/hello-1.2-linux-x64.AppImage":    false,
		"https://example.com/hello-1.2-linux-x64.tar.gz":      false,
	} {
		if got := LinuxSelfUpdatable(ctx, exe, "", url); got != want {
			t.Errorf("arch LinuxSelfUpdatable(%q) = %v, want %v", url, got, want)
		}
	}

	// AppImage install: appImage path is set; only .AppImage is self-updatable.
	for url, want := range map[string]bool{
		"https://example.com/hello-1.2-linux-x64.deb":         false,
		"https://example.com/hello-1.2-linux-x64.pkg.tar.gz":  false,
		"https://example.com/hello-1.2-linux-x64.pkg.tar.zst": false,
		"https://example.com/hello-1.2-linux-x64.AppImage":    true,
		"https://example.com/hello-1.2-linux-x64.tar.gz":      false,
	} {
		if got := LinuxSelfUpdatable(ctx, exe, "/home/u/Hello.AppImage", url); got != want {
			t.Errorf("appimage LinuxSelfUpdatable(%q) = %v, want %v", url, got, want)
		}
	}

	// Debian install: dpkg owns exe.
	writeFakeScript(t, bin, "dpkg-query", `[ "$1" = --search ] && [ "$2" = '/opt/hello/hello' ]`)
	writeFakeScript(t, bin, "pacman", "exit 1")
	if !LinuxSelfUpdatable(ctx, exe, "", "https://example.com/hello-1.2-linux-x64.deb") {
		t.Error("debian install should accept .deb")
	}
	if LinuxSelfUpdatable(ctx, exe, "", "https://example.com/hello-1.2-linux-x64.pkg.tar.zst") {
		t.Error("debian install should reject Arch package")
	}
}

func TestReplaceAppImageAtomicSwapExecBitsAndSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions test")
	}
	for _, tc := range []struct {
		name     string
		initPerm os.FileMode
		wantPerm os.FileMode
	}{
		{"0644 becomes 0755", 0o644, 0o755},
		{"0755 stays 0755", 0o755, 0o755},
		{"0700 stays 0700", 0o700, 0o700},
		{"0600 becomes 0700", 0o600, 0o700},
	} {
		t.Run(tc.name, func(t *testing.T) {
			targetDir := t.TempDir()
			stageDir := t.TempDir()
			target := filepath.Join(targetDir, "Hello.AppImage")
			staged := filepath.Join(stageDir, "hello-1.2.AppImage")
			if err := os.WriteFile(target, []byte("old-image"), tc.initPerm); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(staged, []byte("new-image-payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ReplaceAppImage(staged, target); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "new-image-payload" {
				t.Fatalf("target content = %q, %v", got, err)
			}
			info, err := os.Stat(target)
			if err != nil || info.Mode().Perm() != tc.wantPerm {
				t.Fatalf("target mode = %o, want %o (err=%v)", info.Mode().Perm(), tc.wantPerm, err)
			}
			entries, err := os.ReadDir(targetDir)
			if err != nil || len(entries) != 1 || entries[0].Name() != "Hello.AppImage" {
				t.Fatalf("unexpected leftover files in targetDir: %v, %v", entries, err)
			}
		})
	}

	// Symlink target resolves to the real AppImage file.
	realDir := t.TempDir()
	linkDir := t.TempDir()
	realTarget := filepath.Join(realDir, "Hello-real.AppImage")
	linkTarget := filepath.Join(linkDir, "Hello.AppImage")
	staged := filepath.Join(t.TempDir(), "hello-2.0.AppImage")
	if err := os.WriteFile(realTarget, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceAppImage(staged, linkTarget); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(realTarget); err != nil || string(got) != "v2" {
		t.Fatalf("realTarget content = %q, %v", got, err)
	}
	if fi, err := os.Lstat(linkTarget); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was overwritten instead of preserved: %v, %v", fi.Mode(), err)
	}
}

func TestReplaceAppImageRollbackAndErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions test")
	}
	t.Run("rename failure rolls back and removes temp file", func(t *testing.T) {
		targetDir := t.TempDir()
		target := filepath.Join(targetDir, "Hello.AppImage")
		staged := filepath.Join(t.TempDir(), "hello-2.0.AppImage")
		if err := os.WriteFile(target, []byte("original-v1"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(staged, []byte("new-v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		was := renameFile
		renameFile = func(string, string) error { return errors.New("simulated rename failure") }
		t.Cleanup(func() { renameFile = was })

		if err := ReplaceAppImage(staged, target); err == nil {
			t.Fatal("expected rename error")
		}
		if got, err := os.ReadFile(target); err != nil || string(got) != "original-v1" {
			t.Fatalf("original AppImage altered on failure: %q, %v", got, err)
		}
		if entries, _ := os.ReadDir(targetDir); len(entries) != 1 || entries[0].Name() != "Hello.AppImage" {
			t.Fatalf("temp file leaked after rollback: %v", entries)
		}
	})

	t.Run("invalid targets and missing staged file", func(t *testing.T) {
		staged := filepath.Join(t.TempDir(), "hello.AppImage")
		if err := os.WriteFile(staged, []byte("v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ReplaceAppImage(staged, ""); err == nil {
			t.Error("empty target accepted")
		}
		if err := ReplaceAppImage(staged, filepath.Join(t.TempDir(), "missing.AppImage")); err == nil {
			t.Error("missing target accepted")
		}
		if err := ReplaceAppImage(staged, t.TempDir()); err == nil {
			t.Error("directory target accepted")
		}
		target := filepath.Join(t.TempDir(), "Hello.AppImage")
		if err := os.WriteFile(target, []byte("v1"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := ReplaceAppImage(filepath.Join(t.TempDir(), "missing.AppImage"), target); err == nil {
			t.Error("missing staged file accepted")
		}
		if got, _ := os.ReadFile(target); string(got) != "v1" {
			t.Fatalf("target changed on missing staged file: %q", got)
		}
	})

	t.Run("read-only target directory leaves original intact", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory write bits")
		}
		targetDir := t.TempDir()
		target := filepath.Join(targetDir, "Hello.AppImage")
		staged := filepath.Join(t.TempDir(), "hello-2.0.AppImage")
		if err := os.WriteFile(target, []byte("v1"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(staged, []byte("v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(targetDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(targetDir, 0o700) })

		if err := ReplaceAppImage(staged, target); err == nil {
			t.Fatal("expected error for read-only directory")
		}
		if got, _ := os.ReadFile(target); string(got) != "v1" {
			t.Fatalf("target altered in read-only dir: %q", got)
		}
	})
}
