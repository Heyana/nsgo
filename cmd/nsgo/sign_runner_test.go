package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
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
