package main

import (
	"context"
	"debug/macho"
	"encoding/binary"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"text/template"
)

func TestApplePackageMinimums(t *testing.T) {
	for _, tc := range []struct {
		name, key, floor, raised string
		plist                    *template.Template
		sys                      *appleOS
	}{
		{"macos", "LSMinimumSystemVersion", "13.0", "14.2.1", macPlist, nil},
		{"ios", "MinimumOSVersion", "15.0", "16.4", iosPlist, iOS},
		{"tvos", "MinimumOSVersion", "17.0", "18.1", tvosPlist, tvOS},
		{"watchos", "MinimumOSVersion", "26.0", "27.0", watchosPlist, watchOS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &project{Config: projectcfg.Config{Name: "Minimum", ID: "dev.nsgo.minimum", Version: "1.0.0"}}
			path := filepath.Join(t.TempDir(), "Info.plist")
			for _, want := range []string{tc.floor, tc.raised} {
				if want != tc.floor {
					p.Apple.Info = map[string]any{tc.key: want}
				}
				if err := writePlist(path, p, tc.plist, ""); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if count := strings.Count(string(data), "<key>"+tc.key+"</key>"); count != 1 {
					t.Fatalf("minimum occurs %d times", count)
				}
				plist, err := decodePlist(data)
				if err != nil {
					t.Fatal(err)
				}
				if got := plist.(map[string]any)[tc.key]; got != want {
					t.Fatalf("minimum = %v, want %s", got, want)
				}
				if tc.sys != nil && iosProjectData(p, tc.sys, "TEAM")["Minimum"] != want {
					t.Fatal("Xcode signing project loses the minimum")
				}
			}
			p.Apple.Info = map[string]any{tc.key: "10.0"}
			if err := writePlist(path, p, tc.plist, ""); err == nil || !strings.Contains(err.Error(), "runtime minimum") {
				t.Fatalf("lowering minimum: %v", err)
			}
		})
	}
}

func TestAppleMinimumRejectsMalformedValues(t *testing.T) {
	for _, value := range []any{16, false, "", "15.", "15..1", "+16", "16.0beta", "16.0.0.0", "１６.0", "4294967296"} {
		p := &project{Config: projectcfg.Config{Name: "App"}}
		p.Apple.Info = map[string]any{"MinimumOSVersion": value}
		if err := p.Validate(); err == nil {
			t.Errorf("accepted malformed minimum %#v", value)
		}
	}
	for _, value := range []string{"15", "15.0.0", "15.10", "16.2.1"} {
		p := &project{Config: projectcfg.Config{Name: "App"}}
		p.Apple.Info = map[string]any{"MinimumOSVersion": value}
		if err := p.Validate(); err != nil {
			t.Errorf("refused %s: %v", value, err)
		}
	}
}

func TestAppleMinimumChecksTheSelectedRuntime(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "App"}}
	p.Apple.Info = map[string]any{"MinimumOSVersion": "16.0"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.appleMinimum(iOS); err != nil {
		t.Fatal(err)
	}
	if _, err := buildIOSApp(t.Context(), p, tvOS, t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "17.0") {
		t.Fatalf("tvOS minimum: %v", err)
	}
	if _, err := buildWatchApp(t.Context(), p, t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "26.0") {
		t.Fatalf("watchOS minimum: %v", err)
	}
}

func TestAppClipKeepsItsOwnMinimum(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "Main", ID: "dev.nsgo.minimum", AppClip: &appClip{Minimum: "17.1"}}}
	p.Apple.Info = map[string]any{"MinimumOSVersion": "18.0"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.IOSMinimum() != "18.0" || p.clipProject().IOSMinimum() != "17.1" {
		t.Fatal("app and clip minima are coupled")
	}
}

func TestAppleMinimumReachesLinkingAndEmbeddedFrameworks(t *testing.T) {
	p := &project{}
	p.Apple.Info = map[string]any{"MinimumOSVersion": "18.2"}
	ctx := withProcessRunner(t, func(_ context.Context, c processCommand) ([]byte, error) {
		if c.name != "xcrun" {
			t.Fatalf("unexpected tool %s", c.name)
		}
		if slices.Contains(c.args, "--show-sdk-path") {
			return []byte("/SDK"), nil
		}
		return []byte("/clang"), nil
	})
	env, err := iosProjectEnv(ctx, p, iOS, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(env, "\n"), "arm64-apple-ios18.2-simulator") {
		t.Fatalf("deployment target missing: %v", env)
	}
	if iOS.minimum != "15.0" {
		t.Fatal("app minimum changed the shared runtime floor")
	}
	for _, catalyst := range []bool{false, true} {
		if catalyst && runtime.GOOS == "windows" {
			continue
		} // Framework symlinks require a Unix host.
		framework, err := writeIOSFramework(t.TempDir(), "Minimum", "dev.nsgo.minimum", "header", "18.2", catalyst)
		if err != nil {
			t.Fatal(err)
		}
		plistPath := filepath.Join(framework, "Info.plist")
		if catalyst {
			plistPath = filepath.Join(framework, "Resources", "Info.plist")
		}
		data, err := os.ReadFile(plistPath)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodePlist(data)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.(map[string]any)["MinimumOSVersion"] != "18.2" {
			t.Fatal("embedded framework lost minimum")
		}
	}
}

func TestMacCatalystMinimumIncludesItsGoRuntime(t *testing.T) {
	if got := frameworkMinimum("15.0", true); got != "16.0" {
		t.Fatalf("Catalyst minimum = %s", got)
	}
	if got := frameworkMinimum("18.2", true); got != "18.2" {
		t.Fatalf("raised Catalyst minimum = %s", got)
	}
	if got := frameworkMinimum("15.0", false); got != "15.0" {
		t.Fatalf("iOS minimum changed to %s", got)
	}
}

func TestMacOSPackageMinimumMatchesItsBinary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("bundles a Mac app, which codesign signs")
	}
	for _, minimum := range []string{"", "14.2.1"} {
		t.Run(minimum, func(t *testing.T) {
			dir := t.TempDir()
			config := "name: Minimum\nid: dev.nsgo.minimum\n"
			if minimum != "" {
				config += "apple:\n  info:\n    LSMinimumSystemVersion: '" + minimum + "'\n"
			}
			writeFiles(t, dir, map[string]string{"go.mod": "module example.com/minimum\n\ngo 1.27.3\n", "main.go": "package main\nfunc main(){}\n", "nsgo.yaml": config})
			p, err := load(dir)
			if err != nil {
				t.Fatal(err)
			}
			app, err := buildMacApp(t.Context(), p, t.TempDir(), "arm64")
			if err != nil {
				t.Fatal(err)
			}
			file, err := macho.Open(filepath.Join(app, "Contents", "MacOS", p.Name))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			want := minimum
			if want == "" {
				want = macOSRuntimeMinimum
			}
			version, _ := appleVersion(want)
			expected := version[0]<<16 | version[1]<<8 | version[2]
			found := false
			for _, load := range file.Loads {
				data := load.Raw()
				if len(data) >= 24 && binary.LittleEndian.Uint32(data) == 0x32 {
					found = true
					if got := binary.LittleEndian.Uint32(data[12:16]); got != expected {
						t.Fatalf("Mach-O minos = %#x, want %#x", got, expected)
					}
				}
			}
			if !found {
				t.Fatal("binary has no LC_BUILD_VERSION")
			}
		})
	}
}
