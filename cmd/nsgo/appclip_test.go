package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/appstartup"
)

func TestAppClipStorageProfileSigning(t *testing.T) {
	for _, c := range []struct {
		name, err        string
		parent, clip, tv bool
	}{
		{name: "parent lacks group", err: "the provisioning profile does not grant the app group", clip: true},
		{name: "clip lacks group", err: "sign.clip_profile does not grant the app group", parent: true},
		{name: "both grant group", parent: true, clip: true},
		{name: "tvOS ignores clip capabilities", tv: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			p := &project{Name: "Cafe", ID: "dev.example.cafe", AppClip: &appClip{Name: "Cafe Clip", ID: "dev.example.cafe.Clip"}}
			p.Apple.AppGroup = "group.dev.example.cafe"
			p.Sign = signing{Identity: "test identity", Profile: filepath.Join(dir, "app.profile"), ClipProfile: filepath.Join(dir, "clip.profile")}
			parent := map[string]any{
				"application-identifier":                                 "TEAM." + p.ID,
				"com.apple.developer.associated-appclip-app-identifiers": []any{"TEAM." + p.AppClip.ID},
			}
			clip := map[string]any{
				"application-identifier":                             "TEAM." + p.AppClip.ID,
				"com.apple.developer.parent-application-identifiers": []any{"TEAM." + p.ID},
			}
			if c.parent {
				parent["com.apple.security.application-groups"] = []any{p.appGroup()}
			}
			if c.tv {
				delete(parent, "com.apple.developer.associated-appclip-app-identifiers")
			}
			if c.clip {
				clip["com.apple.security.application-groups"] = []any{p.appGroup()}
			}
			for file, ents := range map[string]map[string]any{p.Sign.Profile: parent, p.Sign.ClipProfile: clip} {
				if err := os.WriteFile(file, encodePlist(map[string]any{"Entitlements": ents}), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			app := filepath.Join(dir, "Cafe.app")
			if err := os.MkdirAll(iosClipApp(p, app), 0o755); err != nil {
				t.Fatal(err)
			}
			var signed []string
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				if cmd.name != "codesign" {
					t.Fatalf("unexpected signing command %s", cmd.name)
				}
				if !c.tv {
					for i, arg := range cmd.args {
						if arg != "--entitlements" {
							continue
						}
						data, err := os.ReadFile(cmd.args[i+1])
						if err != nil {
							t.Fatal(err)
						}
						profile, err := readProfile(t.Context(), data)
						if err != nil {
							t.Fatal(err)
						}
						if err := p.checkIOSAppGroup(profile, "signed entitlements"); err != nil {
							t.Fatal(err)
						}
					}
				}
				signed = append(signed, cmd.args[len(cmd.args)-1])
				return nil, nil
			})
			sys := iOS
			if c.tv {
				sys = tvOS
				p.Widgets = []widgetKind{{Kind: "count"}}
			}
			err := signForStore(ctx, p, sys, app)
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("signing = %v, want %q", err, c.err)
				}
				if len(signed) != 0 {
					t.Fatalf("signed without group grants: %v", signed)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if c.tv {
				want = 1
			}
			if len(signed) != want || signed[len(signed)-1] != app {
				t.Fatalf("signed bundles = %v, want %d with parent last", signed, want)
			}
		})
	}
}

// A clip-only app must claim the same explicit group in both bundles;
// neither widgets alone nor an unspecified group relocates storage.
func TestAppClipSharedStorage(t *testing.T) {
	for _, c := range []struct {
		name, config string
		shared       bool
	}{
		{"clip", "app_clip: {}\napple: {app_group: group.dev.example.shared}", true},
		{"clip and widgets", "app_clip: {}\nwidgets: [{kind: count}]\napple: {app_group: group.dev.example.shared}", true},
		{"clip without group", "app_clip: {}", false},
		{"widgets", "widgets: [{kind: count}]\napple: {app_group: group.dev.example.shared}", false},
		{"group alone", "apple: {app_group: group.dev.example.shared}", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Cafe\nid: dev.example.cafe\n"+c.config+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			p, err := load(dir)
			if err != nil {
				t.Fatal(err)
			}
			projects := []*project{p}
			if p.AppClip != nil {
				projects = append(projects, p.clipProject())
			}
			for _, bundle := range projects {
				plist := bundle.clipStoragePlist()
				if c.shared {
					want := "<key>" + appstartup.StorageAppGroupKey + "</key><string>group.dev.example.shared</string>"
					if plist != want {
						t.Errorf("%s storage metadata = %q", bundle.ID, plist)
					}
					if err := bundle.checkIOSAppGroup(bundle.simulatorEntitlements(), "simulator"); err != nil {
						t.Fatal(err)
					}
					for _, grants := range []map[string]any{nil, {"com.apple.security.application-groups": []any{"group.other"}}} {
						if err := bundle.checkIOSAppGroup(grants, "test.profile"); err == nil || !strings.Contains(err.Error(), "test.profile does not grant the app group group.dev.example.shared") {
							t.Errorf("%s accepted missing grant: %v", bundle.ID, err)
						}
					}
					if bundle.clipParent == "" {
						if ents := bundle.xcodeEntitlements(false, false); len(ents) != 0 {
							t.Errorf("non-iOS signing claims the clip group: %v", ents)
						}
						if err := bundle.checkIOSAppGroup(bundle.xcodeEntitlements(false, true), "Xcode"); err != nil {
							t.Fatal(err)
						}
					}
				} else if plist != "" {
					t.Errorf("%s unexpectedly relocates storage: %s", bundle.ID, plist)
				}
			}
		})
	}
}

func TestAppClipConfig(t *testing.T) {
	for _, c := range []struct {
		yaml, err string
	}{
		{"app_clip: {main: ./clip, domains: [example.com, '*.shop.example.com']}", ""},
		{"app_clip: {id: dev.other.clip}", "not under the app's ID"},
		{"app_clip: {minimum: '15.0'}", "from 16.0"},
		{"app_clip: {minimum: '16.bad'}", "from 16.0"},
		{"app_clip: {domains: ['https://example.com']}", "is not a domain"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: app\nid: dev.example.app\n"+c.yaml+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := load(dir)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: %v, want %q", c.yaml, err, c.err)
		}
	}
}

func TestAppClipProject(t *testing.T) {
	dir := t.TempDir()
	yaml := "name: Cafe\nid: dev.example.cafe\napp_clip: {main: ./clip, minimum: '17', location: true}\nwidgets: [{kind: a}]\n"
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.IOSMinimum() != "15.0" || p.clipPlist() != "" {
		t.Errorf("the app is built as a clip: %s, %q", p.IOSMinimum(), p.clipPlist())
	}
	c := p.clipProject()
	if c.Name != "Cafe Clip" || c.ID != "dev.example.cafe.Clip" || c.Dir != filepath.Join(dir, "clip") {
		t.Errorf("clip = %s, %s, %s", c.Name, c.ID, c.Dir)
	}
	if c.AppClip != nil || c.Widgets != nil {
		t.Error("the clip has the app's clip or widgets")
	}
	if c.IOSMinimum() != "17.0" || p.AppClip.limit() != clipDigitalSize {
		t.Errorf("minimum %s, limit %d", c.IOSMinimum(), p.AppClip.limit())
	}
	want := "<key>NSAppClip</key><dict><key>NSAppClipRequestEphemeralUserNotification</key><false/><key>NSAppClipRequestLocationConfirmation</key><true/></dict>"
	if got := c.clipPlist(); got != want {
		t.Errorf("clip plist = %s", got)
	}
	// The simulator launches nothing claiming a developer entitlement ad hoc.
	for _, ents := range []map[string]any{p.simulatorEntitlements(), c.simulatorEntitlements()} {
		for k := range ents {
			if strings.HasPrefix(k, "com.apple.developer.") {
				t.Errorf("ad hoc entitlement %s", k)
			}
		}
	}
}

func TestXcodeFreeTeam(t *testing.T) {
	prefs := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>IDEProvisioningTeamByIdentifier</key><dict>
<key>A</key><array>
<dict><key>isFreeProvisioningTeam</key><true/><key>teamID</key><string>FREE123</string></dict>
<dict><key>isFreeProvisioningTeam</key><false/><key>teamID</key><string>PAID123</string></dict>
</array></dict></dict></plist>`)
	if !xcodeFreeTeam(prefs, "FREE123") || xcodeFreeTeam(prefs, "PAID123") || xcodeFreeTeam(prefs, "OTHER") {
		t.Error("personal teams are misread")
	}
}

func TestQRLines(t *testing.T) {
	lines := qrLines("https://cafe.example.com/table?n=12")
	// Version 3 at level M: 29 modules and a quiet zone of 4 each side.
	if len(lines) != (29+8+1)/2 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, l := range lines {
		if n := strings.Count(l, "▀"); n != 29+8 {
			t.Errorf("line %d has %d modules", i, n)
		}
	}
	// The quiet zone's first line is white over white.
	if !strings.HasPrefix(lines[0], "\x1b[97;107m▀") {
		t.Errorf("first line starts %q", lines[0][:20])
	}
}
