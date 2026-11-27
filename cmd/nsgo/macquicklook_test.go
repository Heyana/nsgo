package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

func TestBuildMacAppEmbedsQuickLookExtension(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS .app bundle build needs darwin codesign")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module qlprobe\n\ngo 1.26\n",
		"nsgo.yaml": `name: Scenra
id: dev.example.scenra
version: 1.2.3
file_types:
  - name: Scenra Scene
    extensions: [scenra]
quicklook:
  main: ./cmd/quicklook
`,
		"main.go":               "package main\nfunc main() {}\n",
		"cmd/quicklook/main.go": "package main\nfunc main() {}\n",
		"resources/theme.txt":   "light",
	})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true

	app, err := buildMacApp(t.Context(), p, t.TempDir(), runtime.GOARCH)
	if err != nil {
		t.Fatalf("buildMacApp: %v", err)
	}
	appex := macQuickLookAppex(app)
	bin := filepath.Join(appex, "Contents", "MacOS", quickLookExec)
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		t.Fatalf("QuickLook executable missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appex, "Contents", "Resources", "theme.txt")); err != nil {
		t.Fatalf("QuickLook resources missing: %v", err)
	}
	plistBytes, err := os.ReadFile(filepath.Join(appex, "Contents", "Info.plist"))
	if err != nil {
		t.Fatalf("read QuickLook Info.plist: %v", err)
	}
	v, err := decodePlist(plistBytes)
	if err != nil {
		t.Fatalf("decode QuickLook Info.plist: %v", err)
	}
	info := v.(map[string]any)
	if info["CFBundleIdentifier"] != "dev.example.scenra.quicklook" {
		t.Errorf("CFBundleIdentifier = %v", info["CFBundleIdentifier"])
	}
	if info["CFBundlePackageType"] != "XPC!" {
		t.Errorf("CFBundlePackageType = %v", info["CFBundlePackageType"])
	}
	ext, _ := info["NSExtension"].(map[string]any)
	if ext["NSExtensionPointIdentifier"] != "com.apple.quicklook.preview" {
		t.Errorf("NSExtensionPointIdentifier = %v", ext["NSExtensionPointIdentifier"])
	}
	if ext["NSExtensionPrincipalClass"] != quickLookPrincipalClass {
		t.Errorf("NSExtensionPrincipalClass = %v", ext["NSExtensionPrincipalClass"])
	}
	attrs, _ := ext["NSExtensionAttributes"].(map[string]any)
	types, _ := attrs["QLSupportedContentTypes"].([]any)
	if !slices.Equal(types, []any{"dev.example.scenra.scenra"}) {
		t.Errorf("QLSupportedContentTypes = %#v", types)
	}
	if attrs["QLIsDataBasedPreview"] != false {
		t.Errorf("QLIsDataBasedPreview = %#v, want false", attrs["QLIsDataBasedPreview"])
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", appex); err != nil {
		t.Fatalf("verify appex signature: %v", err)
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", app); err != nil {
		t.Fatalf("verify host app signature: %v", err)
	}
	entOut, err := exec.Command("codesign", "-d", "--entitlements", ":-", appex).Output()
	if err != nil {
		t.Fatalf("read appex entitlements: %v", err)
	}
	if !strings.Contains(string(entOut), "com.apple.security.app-sandbox") {
		t.Errorf("appex entitlements missing app-sandbox:\n%s", entOut)
	}
}

func TestBuildMacUniversalEmbedsUniversalQuickLookExtension(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS universal build needs darwin lipo and codesign")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module qluni\n\ngo 1.26\n",
		"nsgo.yaml": `name: Scenra
id: dev.example.scenra
version: 1.0.0
build:
  installers: false
file_types:
  - name: Scenra Scene
    extensions: [scenra]
quicklook:
  main: ./cmd/quicklook
`,
		"main.go":               "package main\nfunc main() {}\n",
		"cmd/quicklook/main.go": "package main\nfunc main() {}\n",
	})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true
	outDir := t.TempDir()
	if err := buildMacUniversal(t.Context(), p, outDir); err != nil {
		t.Fatalf("buildMacUniversal: %v", err)
	}
	app := filepath.Join(outDir, "Scenra.app")
	appex := macQuickLookAppex(app)
	bin := filepath.Join(appex, "Contents", "MacOS", quickLookExec)
	lipoOut, err := exec.Command("lipo", "-info", bin).Output()
	if err != nil {
		t.Fatalf("lipo -info: %v", err)
	}
	if !strings.Contains(string(lipoOut), "arm64") || !strings.Contains(string(lipoOut), "x86_64") {
		t.Errorf("QuickLook binary is not universal: %s", lipoOut)
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", appex); err != nil {
		t.Fatalf("verify universal appex signature: %v", err)
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", app); err != nil {
		t.Fatalf("verify universal host app signature: %v", err)
	}
}

func TestSignMacAppSignsQuickLookBeforeBundle(t *testing.T) {
	for _, notarize := range []bool{false, true} {
		name := "developer-id"
		if notarize {
			name = "notarized"
		}
		t.Run(name, func(t *testing.T) {
			app := filepath.Join(t.TempDir(), "Fixture.app")
			appex := macQuickLookAppex(app)
			if err := os.MkdirAll(filepath.Join(appex, "Contents"), 0o755); err != nil {
				t.Fatal(err)
			}
			p := &project{Config: projectcfg.Config{
				ID:        "dev.example.fixture",
				QuickLook: &projectcfg.QuickLook{Main: "./ql", ID: "dev.example.fixture.quicklook", Types: []string{"dev.example.fixture.doc"}},
			}}
			p.Sign.Identity = "Developer ID Application: Example (ABCDE12345)"
			if notarize {
				p.Sign.Notarize = "notary-profile"
				p.Release = true
			}
			var signed []string
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				if cmd.name != "codesign" {
					t.Fatalf("unexpected command %s %q", cmd.name, cmd.args)
				}
				target := cmd.args[len(cmd.args)-1]
				signed = append(signed, target)
				if target == appex {
					i := slices.Index(cmd.args, "--entitlements")
					if i < 0 {
						t.Fatalf("QuickLook appex signed without entitlements: %q", cmd.args)
					}
					data, err := os.ReadFile(cmd.args[i+1])
					if err != nil {
						t.Fatal(err)
					}
					v, err := decodePlist(data)
					if err != nil {
						t.Fatal(err)
					}
					ents := v.(map[string]any)
					if ents["com.apple.security.app-sandbox"] != true {
						t.Fatalf("QuickLook appex missing app-sandbox: %#v", ents)
					}
					hasRuntime := slices.Contains(cmd.args, "runtime") && slices.Contains(cmd.args, "--timestamp")
					if hasRuntime != notarize {
						t.Fatalf("QuickLook hardened runtime/timestamp = %v, want %v: %q", hasRuntime, notarize, cmd.args)
					}
				}
				return nil, nil
			})
			if err := signMacApp(ctx, p, app); err != nil {
				t.Fatalf("signMacApp: %v", err)
			}
			if !slices.Equal(signed, []string{appex, app}) {
				t.Fatalf("signed order = %v, want [%s %s]", signed, appex, app)
			}
		})
	}
}

func TestMacStoreQuickLookProvisioning(t *testing.T) {
	const team = "ABCDE12345"
	for _, tc := range []struct {
		name, qlID, qlTeam, wantError string
		missingProfile                bool
	}{
		{"valid profile", "dev.example.app.quicklook", team, "", false},
		{"missing profile", "dev.example.app.quicklook", team, "needs sign.quicklook_profile", true},
		{"ID mismatch", "dev.example.other.quicklook", team, "not dev.example.app.quicklook", false},
		{"team mismatch", "dev.example.app.quicklook", "ZZZZZ12345", "must belong", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			app := filepath.Join(dir, "Fixture.app")
			appex := macQuickLookAppex(app)
			if err := os.MkdirAll(filepath.Join(appex, "Contents"), 0o755); err != nil {
				t.Fatal(err)
			}
			cert := []byte("fixture certificate")
			sum := sha1.Sum(cert)
			identity := strings.ToUpper(hex.EncodeToString(sum[:]))
			profile := func(id, signingTeam string) []byte {
				return encodePlist(map[string]any{
					"DeveloperCertificates": []any{cert},
					"Entitlements": map[string]any{
						"com.apple.application-identifier":    signingTeam + "." + id,
						"com.apple.developer.team-identifier": signingTeam,
					},
				})
			}
			p := &project{Config: projectcfg.Config{
				ID:        "dev.example.app",
				QuickLook: &projectcfg.QuickLook{Main: "./ql", ID: "dev.example.app.quicklook", Types: []string{"dev.example.app.doc"}},
			}}
			p.Updates.Channel = channelStore
			p.Release = true
			p.Sign.MacOSProfile = filepath.Join(dir, "app.profile")
			if !tc.missingProfile {
				p.Sign.QuickLookProfile = filepath.Join(dir, "ql.profile")
			}
			p.Sign.Installer = "fixture installer"
			if err := os.WriteFile(p.Sign.MacOSProfile, profile(p.ID, team), 0o600); err != nil {
				t.Fatal(err)
			}
			if !tc.missingProfile {
				if err := os.WriteFile(p.Sign.QuickLookProfile, profile(tc.qlID, tc.qlTeam), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var signed []string
			packaged := false
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				switch cmd.name {
				case "security":
					return []byte("1) " + identity + " fixture"), nil
				case "codesign":
					i := slices.Index(cmd.args, "--entitlements")
					if i < 0 {
						t.Fatalf("missing sandbox entitlements: %q", cmd.args)
					}
					data, err := os.ReadFile(cmd.args[i+1])
					if err != nil {
						t.Fatal(err)
					}
					v, err := decodePlist(data)
					if err != nil {
						t.Fatal(err)
					}
					ents := v.(map[string]any)
					if ents["com.apple.security.app-sandbox"] != true {
						t.Fatalf("missing app-sandbox: %#v", ents)
					}
					signed = append(signed, cmd.args[len(cmd.args)-1])
					return nil, nil
				case "productbuild":
					if !slices.Equal(signed, []string{appex, app}) {
						t.Fatalf("packaged before extension/app signatures: %v", signed)
					}
					packaged = true
					return nil, nil
				default:
					t.Fatalf("unexpected %s %q", cmd.name, cmd.args)
					return nil, nil
				}
			})
			_, err := buildMacStorePackage(ctx, p, app, dir, "arm64")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || packaged {
					t.Fatalf("error=%v, packaged=%v; want %q", err, packaged, tc.wantError)
				}
				return
			}
			if err != nil || !packaged {
				t.Fatalf("packaged=%v, error=%v", packaged, err)
			}
			for _, bundle := range []string{app, appex} {
				if _, err := os.Stat(filepath.Join(bundle, "Contents", "embedded.provisionprofile")); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
