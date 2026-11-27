package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAndroidNDKCandidates(t *testing.T) {
	sdk := t.TempDir()
	t.Setenv("ANDROID_NDK_HOME", filepath.Join(sdk, "explicit"))
	t.Setenv("ANDROID_NDK_ROOT", filepath.Join(sdk, "root"))
	t.Setenv("ANDROID_HOME", filepath.Join(sdk, "android-home"))
	t.Setenv("ANDROID_SDK_ROOT", filepath.Join(sdk, "sdk-root"))
	t.Setenv("HOME", filepath.Join(sdk, "home"))
	got := androidNDKCandidates()
	want := []string{
		filepath.Join(sdk, "explicit"),
		filepath.Join(sdk, "root"),
		filepath.Join(sdk, "android-home", "ndk", androidNDKVersion),
		filepath.Join(sdk, "sdk-root", "ndk", androidNDKVersion),
		filepath.Join(sdk, "home", "Library", "Android", "sdk", "ndk", androidNDKVersion),
		filepath.Join(sdk, "home", "Android", "Sdk", "ndk", androidNDKVersion),
	}
	if len(got) != len(want) {
		t.Fatalf("Android NDK candidates = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAndroidNDKArchivePins(t *testing.T) {
	for _, host := range []string{"darwin", "linux", "windows"} {
		archive, ok := androidNDKArchives[host]
		if !ok || archive.name == "" || len(archive.sha1) != 40 {
			t.Errorf("Android NDK archive for %s = %+v", host, archive)
		}
	}
	if _, ok := androidNDKArchives["plan9"]; ok {
		t.Error("Android NDK has an unsupported host archive")
	}
}

func TestSelectLibraryTargets(t *testing.T) {
	selected, err := selectTargets("js-wasm,iossimulator-amd64,ioscatalyst-arm64,tvos-arm64,tvossimulator-arm64,tvossimulator-amd64,android-arm64,js-wasm")
	if err != nil || len(selected) != 7 {
		t.Fatalf("selected %v: %v", selected, err)
	}
	wantZig := []string{
		"wasm32-freestanding",
		"x86_64-ios-simulator",
		"aarch64-maccatalyst",
		"aarch64-tvos",
		"aarch64-tvos-simulator",
		"x86_64-tvos-simulator",
		"aarch64-linux-android",
	}
	for i, want := range wantZig {
		if selected[i].zig != want {
			t.Fatalf("target %d zig = %q, want %q (%+v)", i, selected[i].zig, want, selected[i])
		}
	}
	for _, target := range selected[3:6] {
		if target.name != "libghostty-vt.dylib" || target.built != "lib/libghostty-vt.dylib" {
			t.Fatalf("tvOS target paths = %+v, want libghostty-vt.dylib", target)
		}
	}
	if selected[6].name != "libghostty-vt.so" {
		t.Fatalf("android target = %+v, want libghostty-vt.so", selected[6])
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
