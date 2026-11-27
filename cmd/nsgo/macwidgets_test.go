package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMacStoreWidgetsProvisionBothBundles(t *testing.T) {
	const team = "ABCDE12345"
	for _, tc := range []struct {
		name, appGroup, widgetGroup, widgetID, widgetTeam, wantError string
	}{
		{"shared container", "group.shared", "group.shared", "dev.example.app.widgets", team, ""},
		{"app group denied", "group.other", "group.shared", "dev.example.app.widgets", team, "does not grant widget app group"},
		{"extension group denied", "group.shared", "group.other", "dev.example.app.widgets", team, "does not grant app group"},
		{"extension ID mismatch", "group.shared", "group.shared", "dev.example.other.widgets", team, "not dev.example.app.widgets"},
		{"extension team mismatch", "group.shared", "group.shared", "dev.example.app.widgets", "ZZZZZ12345", "must belong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			app := filepath.Join(dir, "Fixture.app")
			appex := macWidgetsAppex(app)
			if err := os.MkdirAll(filepath.Join(appex, "Contents"), 0o755); err != nil {
				t.Fatal(err)
			}
			cert := []byte("fixture certificate")
			sum := sha1.Sum(cert)
			identity := strings.ToUpper(hex.EncodeToString(sum[:]))
			profile := func(id, group, signingTeam string) []byte {
				return encodePlist(map[string]any{
					"DeveloperCertificates": []any{cert},
					"Entitlements": map[string]any{
						"com.apple.application-identifier":      signingTeam + "." + id,
						"com.apple.developer.team-identifier":   signingTeam,
						"com.apple.security.application-groups": []any{group},
					},
				})
			}
			p := &project{Config: projectcfg.Config{ID: "dev.example.app", Widgets: []widgetKind{{Kind: "counter"}}}}
			p.Updates.Channel = channelStore
			p.Release = true
			p.Apple.AppGroup = "group.shared"
			p.Sign.MacOSProfile, p.Sign.WidgetsProfile = filepath.Join(dir, "app.profile"), filepath.Join(dir, "widget.profile")
			p.Sign.Installer = "fixture installer"
			for path, data := range map[string][]byte{
				p.Sign.MacOSProfile:   profile(p.ID, tc.appGroup, team),
				p.Sign.WidgetsProfile: profile(tc.widgetID, tc.widgetGroup, tc.widgetTeam),
			} {
				if err := os.WriteFile(path, data, 0o600); err != nil {
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
					groups, _ := ents["com.apple.security.application-groups"].([]any)
					if !slices.Equal(groups, []any{"group.shared"}) || ents["com.apple.security.app-sandbox"] != true {
						t.Fatalf("sandbox groups not shared: %#v", ents)
					}
					if _, ok := ents["com.apple.security.temporary-exception.files.home-relative-path.read-only"]; ok {
						t.Fatal("store widget kept the home-directory exception")
					}
					signed = append(signed, cmd.args[len(cmd.args)-1])
					return nil, nil
				case "productbuild":
					if !slices.Equal(signed, []string{appex, app}) {
						t.Fatalf("package before extension/app signatures: %v", signed)
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
				t.Fatalf("package=%v, error=%v", packaged, err)
			}
			for _, bundle := range []string{app, appex} {
				if _, err := os.Stat(filepath.Join(bundle, "Contents", "embedded.provisionprofile")); err != nil {
					t.Fatal(err)
				}
			}
			// A build that omits its installer still provisions and signs both bundles, without consulting an installer identity.
			signed, packaged = nil, false
			p.Sign.Installer = ""
			if gotTeam, err := signMacStoreApp(ctx, p, app); err != nil || gotTeam != team || packaged || !slices.Equal(signed, []string{appex, app}) {
				t.Fatalf("app-only signing: team=%s signed=%v packaged=%v error=%v", gotTeam, signed, packaged, err)
			}
		})
	}
}

func TestMacStoreWidgetsRequireProfiles(t *testing.T) {
	p := &project{Config: projectcfg.Config{Widgets: []widgetKind{{Kind: "counter"}}}}
	for _, missing := range []string{"both", "app", "extension"} {
		p.Sign.MacOSProfile, p.Sign.WidgetsProfile = "app.profile", "widget.profile"
		if missing != "extension" {
			p.Sign.MacOSProfile = ""
		}
		if missing != "app" {
			p.Sign.WidgetsProfile = ""
		}
		if _, err := buildMacStorePackage(t.Context(), p, "", "", "arm64"); err == nil || !strings.Contains(err.Error(), "sandboxed widgets need") {
			t.Fatalf("missing %s: %v", missing, err)
		}
	}
}
