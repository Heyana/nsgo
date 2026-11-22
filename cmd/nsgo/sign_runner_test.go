package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestSignMacAppSignsLibrariesBeforeBundle(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Fixture.app")
	frameworks := filepath.Join(app, "Contents", "Frameworks")
	lib := filepath.Join(frameworks, "fixture.dylib")
	if err := os.MkdirAll(frameworks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := &project{}
	p.Sign.Identity = "fixture identity"
	paths := []string{lib, app}
	calls := 0
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if calls >= len(paths) {
			t.Fatalf("unexpected signing command %s %q", cmd.name, cmd.args)
		}
		want := []string{"--force", "--sign", "fixture identity", paths[calls]}
		if cmd.name != "codesign" || !slices.Equal(cmd.args, want) {
			t.Fatalf("sign command %d: %s %q, want codesign %q", calls, cmd.name, cmd.args, want)
		}
		calls++
		return nil, nil
	})
	if err := signMacApp(ctx, p, app); err != nil || calls != len(paths) {
		t.Fatalf("signing returned %v after %d commands", err, calls)
	}
}

func TestReadProfileSendsCMSToSecurity(t *testing.T) {
	calls := 0
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		data, err := io.ReadAll(cmd.stdin)
		if cmd.name != "security" || !slices.Equal(cmd.args, []string{"cms", "-D"}) || string(data) != "CMS fixture" || err != nil {
			t.Fatalf("decoding invoked %s %q with an unexpected stdin", cmd.name, cmd.args)
		}
		calls++
		return encodePlist(map[string]any{"Name": "Fixture"}), nil
	})
	profile, err := readProfile(ctx, []byte("CMS fixture"))
	if err != nil || profile["Name"] != "Fixture" || calls != 1 {
		t.Fatalf("profile = %v, %v after %d commands", profile, err, calls)
	}
}

func TestSignMacAppClaimsKeychainAccessGroups(t *testing.T) {
	const manifest = `name: Fixture
id: dev.example.fixture
apple:
  info:
    NSMicrophoneUsageDescription: Calls
macos:
  entitlements:
    com.apple.security.files.downloads.read-write: true
    keychain-access-groups:
      - ABCDE12345.dev.example.shared
      - ABCDE12345.dev.example.fixture
`
	for _, tc := range []struct {
		name     string
		identity string
		notarize string
		want     map[string]any
	}{
		{
			name:     "Developer ID without notarization",
			identity: "Developer ID Application: Jane (ABCDE12345)",
			want: map[string]any{
				"keychain-access-groups": []any{"ABCDE12345.dev.example.shared", "ABCDE12345.dev.example.fixture"},
			},
		},
		{
			name:     "Developer ID with notarization merges hardened entitlements",
			identity: "Developer ID Application: Jane (ABCDE12345)",
			notarize: "nsgo",
			want: map[string]any{
				"com.apple.security.device.audio-input": true,
				"keychain-access-groups":                []any{"ABCDE12345.dev.example.shared", "ABCDE12345.dev.example.fixture"},
			},
		},
		{
			name:     "ad hoc omits keychain-access-groups",
			identity: "-",
			want:     nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p project
			if err := yaml.Unmarshal([]byte(manifest), &p); err != nil {
				t.Fatal(err)
			}
			p.Sign.Identity = tc.identity
			p.Sign.Notarize = tc.notarize
			p.Release = tc.notarize != ""
			app := filepath.Join(t.TempDir(), "Fixture.app")
			if err := os.MkdirAll(app, 0o755); err != nil {
				t.Fatal(err)
			}
			var gotEnts map[string]any
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				if cmd.name != "codesign" {
					t.Fatalf("unexpected command %s %q", cmd.name, cmd.args)
				}
				if i := slices.Index(cmd.args, "--entitlements"); i >= 0 {
					data, err := os.ReadFile(cmd.args[i+1])
					if err != nil {
						t.Fatalf("reading entitlements plist: %v", err)
					}
					v, err := decodePlist(data)
					if err != nil {
						t.Fatalf("decoding entitlements plist: %v", err)
					}
					gotEnts, _ = v.(map[string]any)
				}
				return nil, nil
			})
			if err := signMacApp(ctx, &p, app); err != nil {
				t.Fatalf("signMacApp: %v", err)
			}
			if !reflect.DeepEqual(gotEnts, tc.want) {
				t.Fatalf("entitlements = %#v, want %#v", gotEnts, tc.want)
			}
		})
	}
}
