package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/nsgo-dev/nsgo/internal/appstartup"
)

// TestAppleBuildInfoDeviceAndSimulator writes the SDK and Xcode an iOS device bundle is built with into its Info.plist, as App Store Connect requires (#889), leaves them out of a simulator's, and leaves a key apple.info sets to it.
func TestAppleBuildInfoDeviceAndSimulator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Xcode fixtures are shell scripts")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "xcrun"), []byte(`#!/bin/sh
case "$3" in
--show-sdk-version) printf '18.5\n' ;;
--show-sdk-build-version) printf '22F76\n' ;;
*) echo "unexpected xcrun args: $*" >&2; exit 1 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "xcodebuild"), []byte("#!/bin/sh\nprintf 'Xcode 26.0.1\\nBuild version 17A400\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := appleBuildInfo(t.Context(), iOS, false)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"DTPlatformName": "iphoneos", "DTPlatformVersion": "18.5", "DTPlatformBuild": "22F76",
		"DTSDKName": "iphoneos18.5", "DTSDKBuild": "22F76", "DTXcode": "2601", "DTXcodeBuild": "17A400",
		"DTCompiler": "com.apple.compilers.llvm.clang.1_0",
	} {
		if got[key] != want {
			t.Errorf("%s = %#v, want %#v", key, got[key], want)
		}
	}
	if got := got["UIRequiredDeviceCapabilities"]; len(got.([]any)) != 1 || got.([]any)[0] != "arm64" {
		t.Errorf("device capabilities = %#v", got)
	}
	if got, err := appleBuildInfo(t.Context(), iOS, true); err != nil || got != nil {
		t.Errorf("simulator metadata = %#v, %v; want none", got, err)
	}

	p := &project{}
	p.Apple.Info = map[string]any{"DTPlatformName": "custom"}
	entries, err := appleBuildPlist(t.Context(), p, iOS, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(entries, "DTPlatformName") {
		t.Fatalf("apple.info value was not allowed to win: %s", entries)
	}
}

// TestLocalNetworkProbeInBonjourServices lists the Bonjour type core.PermissionLocalNetwork asks with in NSBonjourServices of an app that explains local network access, after the app's own types and once, and leaves an app that does not alone.
func TestLocalNetworkProbeInBonjourServices(t *testing.T) {
	for _, tc := range []struct {
		name string
		info map[string]any
		want []any
	}{
		{"none", map[string]any{"NSCameraUsageDescription": "Scans codes."}, nil},
		{"usage only", map[string]any{"NSLocalNetworkUsageDescription": "Finds your desktop."}, []any{appstartup.LocalNetworkProbeType}},
		{"own types", map[string]any{"NSLocalNetworkUsageDescription": "Finds your desktop.", "NSBonjourServices": []any{"_example._tcp", "_example._udp"}}, []any{"_example._tcp", "_example._udp", appstartup.LocalNetworkProbeType}},
		{"listed already", map[string]any{"NSLocalNetworkUsageDescription": "Finds your desktop.", "NSBonjourServices": []any{appstartup.LocalNetworkProbeType, "_example._tcp"}}, []any{appstartup.LocalNetworkProbeType, "_example._tcp"}},
	} {
		for _, target := range []struct {
			name  string
			plist *template.Template
		}{{"macos", macPlist}, {"ios", iosPlist}, {"tvos", tvosPlist}} {
			t.Run(tc.name+"/"+target.name, func(t *testing.T) {
				p := &project{Name: "Probe", ID: "dev.nsgo.probe", Version: "1.0.0"}
				p.Apple.Info = tc.info
				own, _ := tc.info["NSBonjourServices"].([]any)
				own = slices.Clone(own)
				path := filepath.Join(t.TempDir(), "Info.plist")
				if err := writePlist(path, p, target.plist, ""); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				plist, err := decodePlist(data)
				if err != nil {
					t.Fatal(err)
				}
				got, _ := plist.(map[string]any)["NSBonjourServices"].([]any)
				if !slices.Equal(got, tc.want) {
					t.Errorf("NSBonjourServices = %v, want %v", got, tc.want)
				}
				if after, _ := tc.info["NSBonjourServices"].([]any); !slices.Equal(after, own) {
					t.Errorf("apple.info's own list changed to %v", tc.info["NSBonjourServices"])
				}
			})
		}
	}
	p := &project{Name: "Probe", ID: "dev.nsgo.probe", Version: "1.0.0"}
	p.Apple.Info = map[string]any{"NSLocalNetworkUsageDescription": "Finds your desktop.", "NSBonjourServices": "_example._tcp"}
	if err := writePlist(filepath.Join(t.TempDir(), "Info.plist"), p, iosPlist, ""); err == nil || !strings.Contains(err.Error(), "NSBonjourServices must be a list") {
		t.Errorf("a string NSBonjourServices: %v", err)
	}
}
