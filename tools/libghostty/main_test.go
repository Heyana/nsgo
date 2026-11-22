package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectLibraryTargets(t *testing.T) {
	selected, err := selectTargets("iossimulator-amd64,ioscatalyst-arm64,iossimulator-amd64")
	if err != nil || len(selected) != 2 {
		t.Fatalf("selected %v: %v", selected, err)
	}
	if selected[0].zig != "x86_64-ios-simulator" || selected[1].zig != "aarch64-maccatalyst" {
		t.Fatalf("wrong Apple targets: %+v", selected)
	}
	if _, err := selectTargets("missing-arm64"); err == nil {
		t.Error("unknown target accepted")
	}
	if selected, err := selectTargets("all"); err != nil || len(selected) != len(targets) {
		t.Fatalf("all: %v %v", selected, err)
	}
}

// TestRebuildSelectedLibraries invokes the generator's real build path through nsgo test. Source and compiler are explicit so native rebuilds remain opt-in.
func TestRebuildSelectedLibraries(t *testing.T) {
	names := os.Getenv("NSGO_GHOSTTY_REBUILD_TARGETS")
	if names == "" {
		t.Skip("set NSGO_GHOSTTY_REBUILD_TARGETS with explicit NSGO_GHOSTTY_SRC and NSGO_GHOSTTY_ZIG")
	}
	selected, err := selectTargets(names)
	if err != nil {
		t.Fatal(err)
	}
	src, zig := os.Getenv("NSGO_GHOSTTY_SRC"), os.Getenv("NSGO_GHOSTTY_ZIG")
	if !filepath.IsAbs(src) || !filepath.IsAbs(zig) {
		t.Fatal("source and Zig must be explicit absolute paths")
	}
	t.Chdir("../..")
	buildLibraries(src, zig, selected)
	for _, target := range selected {
		path := filepath.Join(libDir, target.goos+"-"+target.goarch, target.name)
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("rebuilt %s: %v", path, err)
		}
	}
}
