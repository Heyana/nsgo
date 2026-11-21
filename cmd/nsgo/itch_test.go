package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestItchPreflightsAllTargets(t *testing.T) {
	dir := t.TempDir()
	p := &project{Name: "TestGame", Version: "1.2.3"}
	p.Build.Output = dir
	p.Itch.Project = "example/test-game"
	p.Itch.Channels = map[string]string{"macos_arm64": "HTML5"}
	web := filepath.Join(p.packageDir("web"), "index.html")
	makeItchFile(t, web, "")
	if _, err := p.itchBuilds([]string{"web", "macos_arm64"}); err == nil || !strings.Contains(err.Error(), "both use channel") {
		t.Fatalf("duplicate channel error = %v", err)
	}
	delete(p.Itch.Channels, "macos_arm64")
	if _, err := p.itchBuilds([]string{"web", "macos_arm64"}); err == nil || !strings.Contains(err.Error(), "needs a completed nsgo build") {
		t.Fatalf("missing macOS build error = %v", err)
	}
	if _, err := p.itchBuilds([]string{"web", "android"}); err == nil || !strings.Contains(err.Error(), "not a web or desktop target") {
		t.Fatalf("unsupported target error = %v", err)
	}
}

func TestItchPublishesWithBuiltVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake butler uses a shell script")
	}
	dir := t.TempDir()
	makeItchFile(t, filepath.Join(dir, "nsgo.yaml"), `name: TestGame
version: 0.1.0
itch:
  project: example/test-game
  channels:
    macos_arm64: osx-beta
`)
	web := filepath.Join(dir, "packages", "TestGame-1.2.3-web")
	mac := filepath.Join(dir, "packages", "TestGame-1.2.3-macos_arm64")
	makeItchFile(t, filepath.Join(web, "index.html"), "")
	makeItchFile(t, filepath.Join(mac, "TestGame.app", "Contents", "MacOS", "TestGame"), "")

	bin := filepath.Join(dir, "bin", "butler")
	makeItchFile(t, bin, `#!/bin/sh
printf '%s\n' "$@" >> "$NSGO_ITCH_TEST_LOG"
if [ "$NSGO_ITCH_TEST_FAIL" = yes ]; then
    printf '%s\n' "$BUTLER_API_KEY" >&2
    exit 4
fi
`)
	if err := os.Chmod(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "butler-log")
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NSGO_ITCH_TEST_LOG", log)
	t.Setenv("NSGO_ITCH_TEST_FAIL", "")
	t.Setenv("BUTLER_API_KEY", "test-only-key")
	args := []string{"-target", "web,macos_arm64", "-version", "1.2.3", "-output", "packages", dir}
	if err := publishItch(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"push", web, "example/test-game:html5", "--userversion", "1.2.3",
		"push", mac, "example/test-game:osx-beta", "--userversion", "1.2.3", "",
	}, "\n")
	if string(raw) != want {
		t.Errorf("butler args = %q, want %q", raw, want)
	}

	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSGO_ITCH_TEST_FAIL", "yes")
	err = publishItch(context.Background(), args)
	if err == nil || strings.Contains(err.Error(), "test-only-key") || !strings.Contains(err.Error(), "example/test-game:html5") {
		t.Errorf("upload error = %v, want the channel with its key redacted", err)
	}
	raw, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "osx-beta") {
		t.Error("the second channel was attempted after the first upload failed")
	}

	if err := os.Remove(filepath.Join(mac, "TestGame.app", "Contents", "MacOS", "TestGame")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	if err := publishItch(context.Background(), args); err == nil || !strings.Contains(err.Error(), "needs a completed nsgo build") {
		t.Errorf("missing later target error = %v", err)
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("butler should not run until all builds pass preflight: %v", err)
	}
}

func makeItchFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
