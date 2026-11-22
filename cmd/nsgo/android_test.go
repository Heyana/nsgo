package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestAndroidRuntimeTargetsPlayAPI(t *testing.T) {
	if !strings.Contains(string(androidruntime.Manifest), `android:targetSdkVersion="36"`) {
		t.Fatal("the Android runtime does not target API 36")
	}
}

// TestFindAdbInTheSDK checks the fallback that finds adb without it being
// on PATH, so nsgo dev works after installing the SDK in its usual place.
func TestFindAdbInTheSDK(t *testing.T) {
	home := t.TempDir()
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	want := filepath.Join(home, "Android", "platform-tools", name)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	got, err := findAdb()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("findAdb = %q, want %q", got, want)
	}
}

// TestFindAdbWithoutTheSDK checks the error that names what to install.
func TestFindAdbWithoutTheSDK(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	if _, err := findAdb(); err == nil {
		t.Fatal("findAdb found adb in an empty machine")
	}
}

// TestAndroidArchitectures checks the two packages and the ABIs they run on.
func TestAndroidArchitectures(t *testing.T) {
	for goarch, abi := range map[string]string{"arm64": "arm64-v8a", "amd64": "x86_64"} {
		got, err := androidABI(goarch)
		if err != nil || got != abi {
			t.Errorf("androidABI(%q) = %q, %v; want %q", goarch, got, err, abi)
		}
		want, err := goarchForABI(abi)
		if err != nil || want != goarch {
			t.Errorf("goarchForABI(%q) = %q, %v; want %q", abi, want, err, goarch)
		}
	}
	if _, err := androidABI("386"); err == nil {
		t.Error("androidABI(386) succeeded")
	}
	// 32-bit ARM says why and where it is discussed; other ABIs do not claim to be 32-bit ARM.
	for abi, arm := range map[string]bool{"armeabi-v7a": true, "armeabi": true, "x86": false, "riscv64": false} {
		_, err := goarchForABI(abi)
		if err == nil {
			t.Errorf("goarchForABI(%q) succeeded", abi)
			continue
		}
		if says := strings.Contains(err.Error(), "/issues/41") && strings.Contains(err.Error(), "NDK"); says != arm {
			t.Errorf("goarchForABI(%q) error = %v; names 32-bit ARM's limit: %v, want %v", abi, err, says, arm)
		}
	}
}

// TestFindNDKInTheSDK checks that the emulator build finds the newest NDK
// in the SDK and the clang that targets the manifest's API.
func TestFindNDKInTheSDK(t *testing.T) {
	sdk := t.TempDir()
	name := "x86_64-linux-android28-clang"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	for _, version := range []string{"26.1.10909125", "27.2.12479018"} {
		bin := filepath.Join(sdk, "ndk", version, "toolchains", "llvm", "prebuilt", "linux-x86_64", "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ANDROID_HOME", sdk)
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("ANDROID_NDK_HOME", "")
	t.Setenv("ANDROID_NDK_ROOT", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	got, err := findNDK()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(sdk, "ndk", "27.2.12479018"); got != want {
		t.Fatalf("findNDK = %q, want %q", got, want)
	}
	clang, err := androidClang("amd64")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(got, "toolchains", "llvm", "prebuilt", "linux-x86_64", "bin", name); clang != want {
		t.Fatalf("androidClang = %q, want %q", clang, want)
	}
}

// TestAndroidLibraries checks that only an app whose function table names the fold geometry's reader and whose sources select it declares the window extensions.
func TestAndroidLibraries(t *testing.T) {
	if libs, err := androidLibraries(t.Context(), nil, "arm64", []byte("\x00github.com/nsgo-dev/nsgo/core.(*Env).Window\x00")); libs != nil || err != nil {
		t.Errorf("an app that reads no geometry declares %v (%v)", libs, err)
	}
	app := []byte("\x00" + geometryReader + "\x00")
	for _, c := range []struct {
		name, main string
		want       []string
	}{
		{"reads its geometry", "package main\n\nimport \"example.com/app/term\"\n\nfunc main() { var e interface{ Geometry() term.Geometry }; _ = e.Geometry() }\n", []string{windowExtensions}},
		// The name kept by reflection, as in a TSX app, with only a package's own Geometry in the sources.
		{"names a package's Geometry", "package main\n\nimport \"example.com/app/term\"\n\nfunc main() { _ = term.Geometry{} }\n", nil},
	} {
		dir := t.TempDir()
		for name, src := range map[string]string{
			"go.mod":       "module example.com/app\n\ngo 1.26\n",
			"main.go":      c.main,
			"term/term.go": "package term\n\ntype Geometry struct{}\n",
		} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		libs, err := androidLibraries(t.Context(), &project{Dir: dir}, "arm64", app)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(libs, c.want) {
			t.Errorf("an app that %s declares %v, want %v", c.name, libs, c.want)
		}
	}
	if m := aarManifest(false, nil); strings.Contains(m, "uses-library") {
		t.Errorf("an AAR that reads no geometry declares a library:\n%s", m)
	}
	if m := aarManifest(false, []string{windowExtensions}); !strings.Contains(m, `<uses-library android:name="androidx.window.extensions" android:required="false" />`) {
		t.Errorf("the AAR's manifest lacks the window extensions:\n%s", m)
	}
	if m := aarManifest(false, nil); !strings.Contains(m, `android.intent.action.PROCESS_TEXT`) || !strings.Contains(m, `android:mimeType="text/plain"`) {
		t.Errorf("the AAR's manifest lacks the ACTION_PROCESS_TEXT query:\n%s", m)
	}
}

func TestFileSelectsGeometry(t *testing.T) {
	names := map[string]string{"github.com/nsgo-dev/nsgo/core": "core", "example.com/vt/v2": "vt"}
	for _, c := range []struct {
		src  string
		want bool
	}{
		{`package a; import "github.com/nsgo-dev/nsgo/core"; func f(cx *core.Scope) { _ = cx.Env().Geometry() }`, true},
		{`package a; import "github.com/nsgo-dev/nsgo/core"; func f(e *core.Env) func() core.Geometry { return e.Geometry }`, true},
		{`package a; import "github.com/nsgo-dev/nsgo/core"; var g core.Geometry`, false},
		{`package a; import nsgo "github.com/nsgo-dev/nsgo/core"; var g nsgo.Geometry`, false},
		{`package a; import "example.com/vt/v2"; var g = vt.Geometry{}`, false},
		{`package a; func f(g struct{ Geometry int }) int { return g.Geometry }`, true},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), "a.go", c.src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		if got := fileSelectsGeometry(f, names); got != c.want {
			t.Errorf("%s: %v, want %v", c.src, got, c.want)
		}
	}
}
