package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

func TestEmbedOnly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		targets []string
		want    []string
	}{
		{"ios collapsed", []string{"ios", "ios_simulator"}, []string{"ios"}},
		{"tvos simulator", []string{"tvos_simulator"}, []string{"tvos"}},
		{"tvos collapsed", []string{"tvos", "tvos_simulator"}, []string{"tvos"}},
		{"mixed", []string{"macos_arm64", "ios_simulator", "tvos_simulator", "android_arm64", "web"}, []string{"ios", "tvos", "android_arm64"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := embedOnly(tc.targets)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("embedOnly(%v) = %v, want %v", tc.targets, got, tc.want)
			}
		})
	}
	if _, err := embedOnly([]string{"macos_arm64", "web"}); err == nil ||
		!strings.Contains(err.Error(), "tvos") || !strings.Contains(err.Error(), "tvos_simulator") {
		t.Fatalf("embedOnly(desktop/web) = %v, want error naming tvos and tvos_simulator", err)
	}
}

func TestAppleEmbedSlices(t *testing.T) {
	tv := appleEmbedSlices(tvOS)
	if len(tv) != 2 {
		t.Fatalf("tvosSlices = %d slices, want 2", len(tv))
	}
	if tv[0].sdk != "appletvos" || tv[0].goos != "ios" || !slices.Equal(tv[0].archs, []string{"arm64"}) ||
		tv[0].target != "%s-apple-tvos%s" || tv[0].nativeTarget != "tvos" ||
		tv[0].ghosttyPlatform != "tvos" || tv[0].supportedPlatform != "AppleTVOS" || tv[0].catalyst {
		t.Fatalf("unexpected tvOS device slice: %+v", tv[0])
	}
	if tv[1].sdk != "appletvsimulator" || tv[1].goos != "ios" || !slices.Equal(tv[1].archs, []string{"arm64", "amd64"}) ||
		tv[1].target != "%s-apple-tvos%s-simulator" || tv[1].nativeTarget != "tvos_simulator" ||
		tv[1].ghosttyPlatform != "tvossimulator" || tv[1].supportedPlatform != "AppleTVSimulator" || tv[1].catalyst {
		t.Fatalf("unexpected tvOS simulator slice: %+v", tv[1])
	}
	ios := appleEmbedSlices(iOS)
	if len(ios) != 3 || ios[0].supportedPlatform != "iPhoneOS" || ios[1].supportedPlatform != "iPhoneSimulator" || !ios[2].catalyst {
		t.Fatalf("unexpected iOS slices: %+v", ios)
	}
}

func TestWriteTVOSFrameworkBundle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		header   string
	}{
		{"device app", "AppleTVOS", "/* header */"},
		{"simulator app", "AppleTVSimulator", "/* header */"},
		{"device runtime-only", "AppleTVOS", ""},
		{"simulator runtime-only", "AppleTVSimulator", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fw, err := writeFrameworkBundle(t.TempDir(), "SampleTV", "dev.nsgo.sampletv", tc.header, "17.0", tc.platform, false)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(fw, "Info.plist"))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodePlist(data)
			if err != nil {
				t.Fatal(err)
			}
			plist := decoded.(map[string]any)
			if plist["MinimumOSVersion"] != "17.0" {
				t.Errorf("MinimumOSVersion = %v, want 17.0", plist["MinimumOSVersion"])
			}
			platforms, _ := plist["CFBundleSupportedPlatforms"].([]any)
			if len(platforms) != 1 || platforms[0] != tc.platform {
				t.Errorf("CFBundleSupportedPlatforms = %v, want [%s]", platforms, tc.platform)
			}
			_, headerErr := os.Stat(filepath.Join(fw, "Headers", "NSGo.h"))
			_, moduleErr := os.Stat(filepath.Join(fw, "Modules", "module.modulemap"))
			if tc.header != "" {
				if headerErr != nil || moduleErr != nil {
					t.Errorf("app framework missing headers/modulemap: %v %v", headerErr, moduleErr)
				}
			} else {
				if !os.IsNotExist(headerErr) || !os.IsNotExist(moduleErr) {
					t.Errorf("runtime-only framework exposed headers/modulemap: %v %v", headerErr, moduleErr)
				}
			}
		})
	}
}

func TestEmbedTVOSMinimumValidation(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "SampleTV", ID: "dev.nsgo.sampletv", Version: "1.0.0"}}
	p.buildEmbed = true
	p.Apple.Info = map[string]any{"MinimumOSVersion": "16.0"}
	if err := buildTarget(t.Context(), p, "tvos", t.TempDir()); err == nil || !strings.Contains(err.Error(), "17.0") {
		t.Fatalf("tvos -embed with MinimumOSVersion 16.0 = %v, want error naming 17.0", err)
	}
}

func TestEmbedHeaderCompilesForAppleTargets(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS with Xcode SDKs")
	}
	for _, tc := range []struct {
		sdk    string
		target string
		source string
	}{
		{
			sdk:    "appletvos",
			target: "arm64-apple-tvos17.0",
			source: "#import \"NSGo.h\"\nvoid f(NSGoView *v) { NSGoStart(); [v stop]; }\n",
		},
		{
			sdk:    "appletvsimulator",
			target: "arm64-apple-tvos17.0-simulator",
			source: "#import \"NSGo.h\"\nvoid f(NSGoView *v) { NSGoStart(); [v stop]; }\n",
		},
		{
			sdk:    "appletvsimulator",
			target: "x86_64-apple-tvos17.0-simulator",
			source: "#import \"NSGo.h\"\nvoid f(NSGoView *v) { NSGoStart(); [v stop]; }\n",
		},
		{
			sdk:    "iphoneos",
			target: "arm64-apple-ios15.0",
			source: "#import \"NSGo.h\"\nUIStatusBarStyle f(NSGoView *v) { return v.preferredStatusBarStyle; }\n",
		},
	} {
		t.Run(tc.sdk+"/"+tc.target, func(t *testing.T) {
			sdk, err := output(t.Context(), "xcrun", "--sdk", tc.sdk, "--show-sdk-path")
			if err != nil {
				t.Skipf("%s SDK unavailable: %v", tc.sdk, err)
			}
			clang, err := output(t.Context(), "xcrun", "--sdk", tc.sdk, "-f", "clang")
			if err != nil {
				t.Skipf("clang for %s unavailable: %v", tc.sdk, err)
			}
			dir := t.TempDir()
			header := strings.NewReplacer("{{name}}", "Sample", "{{platform}}", "a tvOS").Replace(embedHeader)
			writeFiles(t, dir, map[string]string{
				"NSGo.h": header,
				"main.m": tc.source,
			})
			cmd := exec.CommandContext(t.Context(), clang, "-fobjc-arc", "-Wall", "-Werror", "-isysroot", sdk, "-target", tc.target, "-c", filepath.Join(dir, "main.m"), "-o", filepath.Join(dir, "main.o"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("compile NSGo.h for %s: %v\n%s", tc.target, err, out)
			}
		})
	}
}

func TestEmbedTVOSXCFramework(t *testing.T) {
	if testing.Short() {
		t.Skip("builds real tvOS XCFrameworks with Go and Xcode")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS with Xcode tvOS SDKs")
	}
	for _, sdk := range []string{"appletvos", "appletvsimulator"} {
		if _, err := output(t.Context(), "xcrun", "--sdk", sdk, "--show-sdk-path"); err != nil {
			t.Skipf("%s SDK unavailable: %v", sdk, err)
		}
	}
	for _, tc := range []struct {
		name    string
		target  string
		dynamic bool
		linked  bool
	}{
		{"static_simulator_target_with_terminal", "tvos_simulator", false, true},
		{"dynamic_device_target", "tvos", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := ghosttyProject(t, tc.linked)
			p.Name = "TVEmbed"
			p.ID = "dev.nsgo.test.tvembed"
			p.buildEmbed = true
			p.buildDynamic = tc.dynamic
			p.packageLicenses = false
			writeFiles(t, p.Dir, map[string]string{
				"resources/hello.txt": "tvos-resource",
			})
			out := t.TempDir()
			if err := buildTarget(t.Context(), p, tc.target, out); err != nil {
				t.Fatal(err)
			}
			xcfw := filepath.Join(out, "TVEmbed.xcframework")
			plistBytes, err := os.ReadFile(filepath.Join(xcfw, "Info.plist"))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodePlist(plistBytes)
			if err != nil {
				t.Fatal(err)
			}
			libs, _ := decoded.(map[string]any)["AvailableLibraries"].([]any)
			if len(libs) != 2 {
				t.Fatalf("AvailableLibraries count = %d, want 2: %v", len(libs), libs)
			}
			for _, slice := range []struct {
				dir      string
				platform string
				archs    string
			}{
				{"tvos-arm64", "AppleTVOS", "arm64"},
				{"tvos-arm64_x86_64-simulator", "AppleTVSimulator", "x86_64 arm64"},
			} {
				sliceDir := filepath.Join(xcfw, slice.dir)
				if !tc.dynamic {
					lib := filepath.Join(sliceDir, "libTVEmbed.a")
					gotArchs, err := output(t.Context(), "lipo", "-archs", lib)
					if err != nil {
						t.Fatalf("lipo %s: %v", lib, err)
					}
					if gotArchs != slice.archs {
						t.Errorf("%s archs = %q, want %q", slice.dir, gotArchs, slice.archs)
					}
					for _, name := range []string{"NSGo.h", "module.modulemap"} {
						if _, err := os.Stat(filepath.Join(sliceDir, "Headers", name)); err != nil {
							t.Errorf("%s missing Headers/%s: %v", slice.dir, name, err)
						}
					}
				} else {
					fw := filepath.Join(sliceDir, "TVEmbed.framework")
					bin := filepath.Join(fw, "TVEmbed")
					gotArchs, err := output(t.Context(), "lipo", "-archs", bin)
					if err != nil {
						t.Fatalf("lipo %s: %v", bin, err)
					}
					if gotArchs != slice.archs {
						t.Errorf("%s archs = %q, want %q", slice.dir, gotArchs, slice.archs)
					}
					syms, err := output(t.Context(), "nm", "-gU", bin)
					if err != nil || !strings.Contains(syms, "_NSGoStart") {
						t.Errorf("%s missing exported _NSGoStart: %v\n%s", slice.dir, err, syms)
					}
					fwPlistBytes, err := os.ReadFile(filepath.Join(fw, "Info.plist"))
					if err != nil {
						t.Fatal(err)
					}
					fwPlist, err := decodePlist(fwPlistBytes)
					if err != nil {
						t.Fatal(err)
					}
					dict := fwPlist.(map[string]any)
					if dict["MinimumOSVersion"] != "17.0" {
						t.Errorf("%s MinimumOSVersion = %v, want 17.0", slice.dir, dict["MinimumOSVersion"])
					}
					platforms, _ := dict["CFBundleSupportedPlatforms"].([]any)
					if len(platforms) != 1 || platforms[0] != slice.platform {
						t.Errorf("%s CFBundleSupportedPlatforms = %v, want [%s]", slice.dir, platforms, slice.platform)
					}
					for _, rel := range []string{"Headers/NSGo.h", "Modules/module.modulemap", "PrivacyInfo.xcprivacy"} {
						if _, err := os.Stat(filepath.Join(fw, rel)); err != nil {
							t.Errorf("%s missing %s: %v", slice.dir, rel, err)
						}
					}
				}
			}
			res, err := os.ReadFile(filepath.Join(out, "TVEmbedResources", "hello.txt"))
			if err != nil || string(res) != "tvos-resource" {
				t.Errorf("TVEmbedResources/hello.txt = %q, %v", res, err)
			}
			companion := filepath.Join(out, "GhosttyVT.xcframework")
			if tc.linked {
				for _, platform := range []string{"tvos-arm64", "tvos-arm64_x86_64-simulator"} {
					fw := filepath.Join(companion, platform, ghosttyFramework)
					if _, err := os.Stat(filepath.Join(fw, "GhosttyVT")); err != nil {
						t.Errorf("companion missing %s: %v", platform, err)
					}
				}
			} else if _, err := os.Stat(companion); !os.IsNotExist(err) {
				t.Errorf("unexpected GhosttyVT.xcframework without base/terminal: %v", err)
			}
		})
	}
}
