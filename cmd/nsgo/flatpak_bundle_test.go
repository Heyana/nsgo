package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlatpakBundleAndBuilderManifest(t *testing.T) {
	dir := t.TempDir()
	if out := os.Getenv("NSGO_FLATPAK_APP_FIXTURE_DIR"); out != "" {
		dir = out
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := &project{Dir: dir, Name: "fixture", ID: "dev.nsgo.BundleFixture", Version: "1.0.0"}
	p.Build.Output = filepath.Join(dir, "out")
	p.Updates.Channel = channelStore
	stage := filepath.Join(dir, "stage")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"fixture":  "#!/bin/sh\nprintf 'NSGo bundle payload\\n'\n",
		"data.txt": "independent payload",
	} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeLinuxIcon(p, stage); err != nil {
		t.Fatal(err)
	}
	archive, err := buildLinuxArchive(context.Background(), p, stage, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(stage, p.ID+".desktop"))
	if err != nil || !strings.Contains(string(entry), "Categories=Utility;\n") {
		t.Fatalf("store category fallback: %q %v", entry, err)
	}
	bundle, err := buildFlatpakBundle(context.Background(), p, stage, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"flatpak\x00", "org.gnome.Platform/x86_64/51", "app/dev.nsgo.BundleFixture/x86_64/stable", "independent payload",
		"Exec=/usr/bin/flatpak run --branch=stable --arch=x86_64 \"--command=fixture\" dev.nsgo.BundleFixture\n", "X-Flatpak=dev.nsgo.BundleFixture\n"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("bundle lacks %q", want)
		}
	}
	if err := p.releaseAssets(archive, bundle); err != nil {
		t.Fatal(err)
	}
	if err := p.writeFlatpakManifest([]string{"linux_x64"}); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".flatpak", ".flatpak.sha256", ".tar.gz", ".tar.gz.sha256"} {
		if _, err := os.Stat(filepath.Join(p.releaseDir(), p.releaseName("linux_x64", suffix))); err != nil {
			t.Error(err)
		}
	}
	entries, err := linuxDirEntries(p, stage)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if isLinuxPackage(entry.name) {
			t.Errorf("nested package %s", entry.name)
		}
	}
}
