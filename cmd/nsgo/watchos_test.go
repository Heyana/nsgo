package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"debug/macho"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// TestWatchHostCompile compiles watch/Host.swift for both watchOS device and
// watchOS simulator SDKs and verifies the exported C bridge symbols and
// permission/usage-description guards stay intact.
func TestWatchHostCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs Xcode")
	}
	if err := exec.Command("xcrun", "--find", "swiftc").Run(); err != nil {
		t.Skip("needs Xcode's swiftc")
	}
	for _, sim := range []bool{true, false} {
		sdk := watchOS.sdkName(sim)
		t.Run(sdk, func(t *testing.T) {
			if err := exec.Command("xcrun", "--sdk", sdk, "--show-sdk-path").Run(); err != nil {
				t.Skipf("needs the %s SDK", sdk)
			}
			obj, err := compileWatchHost(t.Context(), sim)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(filepath.Dir(obj))

			out, err := exec.Command("nm", "-gU", obj).Output()
			if err != nil {
				t.Fatalf("nm %s: %v", obj, err)
			}
			syms := string(out)
			for _, sym := range []string{
				"_nsgo_watch_connect",
				"_nsgo_watch_main",
				"_nsgo_watch_apply",
				"_nsgo_watch_snapshot",
				"_nsgo_watch_tick",
			} {
				if !strings.Contains(syms, sym) {
					t.Errorf("%s host object exports no %s", sdk, sym)
				}
			}
		})
	}
}

// TestWatchHostUsageDescriptionGuards checks that Host.swift verifies the
// required Info.plist usage description keys before prompting for watchOS
// permissions, preventing TCC termination on missing keys.
func TestWatchHostUsageDescriptionGuards(t *testing.T) {
	src := string(watchHost)
	for _, key := range []string{
		"NSMicrophoneUsageDescription",
		"NSCalendarsFullAccessUsageDescription",
		"NSCalendarsUsageDescription",
		"NSRemindersFullAccessUsageDescription",
		"NSRemindersUsageDescription",
		"NSBluetoothAlwaysUsageDescription",
		"CBCentralManagerOptionShowPowerAlertKey",
		"digitalCrownRotation",
		"CrownReceiver",
		"case .crown:",
	} {
		if !strings.Contains(src, key) {
			t.Errorf("watch/Host.swift is missing %s", key)
		}
	}
}

func makeWatchFixtureApp(t *testing.T, dir string, p *project) string {
	t.Helper()
	app := filepath.Join(dir, p.Name+".app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, p.Name), []byte("watch-arm64-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := map[string]any{
		"CFBundleExecutable":             p.Name,
		"CFBundleIdentifier":             p.ID,
		"CFBundleName":                   p.Name,
		"CFBundlePackageType":            "APPL",
		"CFBundleShortVersionString":     p.Version,
		"CFBundleVersion":                p.BundleVersion(),
		"CFBundleSupportedPlatforms":     []any{"WatchOS"},
		"MinimumOSVersion":               "26.0",
		"UIDeviceFamily":                 []any{int64(4)},
		"UIRequiredDeviceCapabilities":   []any{"arm64"},
		"WKApplication":                  true,
		"WKWatchOnly":                    true,
		"WKCompanionAppBundleIdentifier": "stale.companion.id",
	}
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), encodePlist(info), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.writePrivacyManifest(app); err != nil {
		t.Fatal(err)
	}
	return app
}

func makeFakeXcodePlatforms(t *testing.T, stubBytes, wkBytes []byte) (iphonePlatform, watchSDK string) {
	t.Helper()
	root := t.TempDir()
	iphonePlatform = filepath.Join(root, "iPhoneOS.platform")
	stubPath := filepath.Join(iphonePlatform, filepath.FromSlash(watchStubBinaryRelPath))
	if err := os.MkdirAll(filepath.Dir(stubPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stubPath, stubBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	watchSDK = filepath.Join(root, "WatchOS.sdk")
	if wkBytes != nil {
		wkPath := filepath.Join(watchSDK, filepath.FromSlash(watchKitWKRelPath))
		if err := os.MkdirAll(filepath.Dir(wkPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(wkPath, wkBytes, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return iphonePlatform, watchSDK
}

func readZipMap(t *testing.T, path string) (map[string][]byte, map[string]os.FileMode) {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip %s: %v", path, err)
	}
	defer z.Close()
	contents := map[string][]byte{}
	modes := map[string]os.FileMode{}
	for _, f := range z.File {
		modes[f.Name] = f.Mode()
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents[f.Name] = data
	}
	return contents, modes
}

func writeTestProfile(t *testing.T, dir, name string, ents map[string]any, certs ...[]byte) (string, []byte) {
	t.Helper()
	certList := make([]any, len(certs))
	for i, c := range certs {
		certList[i] = c
	}
	data := encodePlist(map[string]any{
		"Name":                  name,
		"Entitlements":          ents,
		"DeveloperCertificates": certList,
	})
	path := filepath.Join(dir, name+".mobileprovision")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func TestWatchStoreIPALayoutAndStubPlist(t *testing.T) {
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{
		Dir:         dir,
		Name:        "Pulse",
		ID:          "dev.example.pulse",
		Version:     "1.2.0",
		BuildNumber: 7,
	}}
	p.Apple.Info = map[string]any{
		"ITSAppUsesNonExemptEncryption": false,
	}
	watchApp := makeWatchFixtureApp(t, dir, p)
	iphonePlatform, watchSDK := makeFakeXcodePlatforms(t, []byte("thin-ios-stub"), []byte("watchkit-wk-binary"))

	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		switch cmd.name {
		case "xcrun":
			switch {
			case slices.Equal(cmd.args, []string{"--sdk", "iphoneos", "--show-sdk-platform-path"}):
				return []byte(iphonePlatform + "\n"), nil
			case slices.Equal(cmd.args, []string{"--sdk", "iphoneos", "--show-sdk-version"}):
				return []byte("27.2\n"), nil
			case slices.Equal(cmd.args, []string{"--sdk", "iphoneos", "--show-sdk-build-version"}):
				return []byte("24B8118\n"), nil
			case slices.Equal(cmd.args, []string{"--sdk", "watchos", "--show-sdk-path"}):
				return []byte(watchSDK + "\n"), nil
			}
		case "xcodebuild":
			return []byte("Xcode 27.2\nBuild version 27B52\n"), nil
		case "codesign":
			t.Fatalf("unsigned build invoked codesign: %q", cmd.args)
		}
		t.Fatalf("unexpected command: %s %q", cmd.name, cmd.args)
		return nil, nil
	})

	ipa, err := packageWatchStoreIPA(ctx, p, watchApp, dir)
	if err != nil {
		t.Fatalf("packageWatchStoreIPA: %v", err)
	}
	if got, want := filepath.Base(ipa), "Pulse.ipa"; got != want {
		t.Fatalf("ipa name = %q, want %q", got, want)
	}

	files, modes := readZipMap(t, ipa)
	for _, want := range []string{
		"Payload/Pulse.app/Pulse",
		"Payload/Pulse.app/Info.plist",
		"Payload/Pulse.app/PrivacyInfo.xcprivacy",
		"Payload/Pulse.app/Watch/Pulse.app/Pulse",
		"Payload/Pulse.app/Watch/Pulse.app/Info.plist",
		"Payload/Pulse.app/Watch/Pulse.app/PrivacyInfo.xcprivacy",
		"WatchKitSupport2/WK",
	} {
		if _, ok := files[want]; !ok {
			t.Fatalf("IPA missing %s; entries: %v", want, slices.Sorted(func(yield func(string) bool) {
				for k := range files {
					if !yield(k) {
						return
					}
				}
			}))
		}
	}
	for _, unexpected := range []string{
		"Payload/Pulse.app/embedded.mobileprovision",
		"Payload/Pulse.app/Watch/Pulse.app/embedded.mobileprovision",
	} {
		if _, ok := files[unexpected]; ok {
			t.Errorf("unsigned IPA unexpectedly contains %s", unexpected)
		}
	}
	if !bytes.Equal(files["Payload/Pulse.app/Pulse"], []byte("thin-ios-stub")) {
		t.Errorf("stub executable mismatch: %q", files["Payload/Pulse.app/Pulse"])
	}
	if !bytes.Equal(files["WatchKitSupport2/WK"], []byte("watchkit-wk-binary")) {
		t.Errorf("WatchKitSupport2/WK mismatch: %q", files["WatchKitSupport2/WK"])
	}
	for _, exe := range []string{"Payload/Pulse.app/Pulse", "Payload/Pulse.app/Watch/Pulse.app/Pulse", "WatchKitSupport2/WK"} {
		if modes[exe]&0o111 == 0 {
			t.Errorf("%s is not executable in IPA (mode %v)", exe, modes[exe])
		}
	}

	// Verify outer iOS stub Info.plist matches Apple's watchapp2-container rules.
	stubRaw, err := decodePlist(files["Payload/Pulse.app/Info.plist"])
	if err != nil {
		t.Fatalf("decode stub Info.plist: %v", err)
	}
	stubPlist := stubRaw.(map[string]any)
	for key, want := range map[string]any{
		"CFBundleExecutable":            "Pulse",
		"CFBundleIdentifier":            "dev.example.pulse",
		"CFBundleName":                  "Pulse",
		"CFBundleDisplayName":           "Pulse",
		"CFBundlePackageType":           "APPL",
		"CFBundleShortVersionString":    "1.2.0",
		"CFBundleVersion":               "7",
		"CFBundleSupportedPlatforms":    []any{"iPhoneOS"},
		"ITSWatchOnlyContainer":         true,
		"LSApplicationLaunchProhibited": true,
		"LSRequiresIPhoneOS":            true,
		"MinimumOSVersion":              "15.0",
		"UIDeviceFamily":                []any{int64(1), int64(2)},
		"UIRequiredDeviceCapabilities":  []any{"arm64"},
		"DTPlatformName":                "iphoneos",
		"DTSDKName":                     "iphoneos27.2",
		"DTXcode":                       "2720",
		"ITSAppUsesNonExemptEncryption": false,
	} {
		if !reflect.DeepEqual(stubPlist[key], want) {
			t.Errorf("stub Info.plist[%s] = %#v, want %#v", key, stubPlist[key], want)
		}
	}

	// Verify nested watchOS app Info.plist uses <id>.watchkitapp and drops companion key.
	watchRaw, err := decodePlist(files["Payload/Pulse.app/Watch/Pulse.app/Info.plist"])
	if err != nil {
		t.Fatalf("decode nested watch Info.plist: %v", err)
	}
	watchPlist := watchRaw.(map[string]any)
	for key, want := range map[string]any{
		"CFBundleIdentifier":         "dev.example.pulse.watchkitapp",
		"CFBundleSupportedPlatforms": []any{"WatchOS"},
		"WKApplication":              true,
		"WKWatchOnly":                true,
		"UIDeviceFamily":             []any{int64(4)},
	} {
		if !reflect.DeepEqual(watchPlist[key], want) {
			t.Errorf("nested watch Info.plist[%s] = %#v, want %#v", key, watchPlist[key], want)
		}
	}
	if _, ok := watchPlist["WKCompanionAppBundleIdentifier"]; ok {
		t.Error("nested watch Info.plist kept WKCompanionAppBundleIdentifier for a watch-only app")
	}

	// Standalone <dir>/<Name>.app stays untouched for -run installs.
	standaloneData, err := os.ReadFile(filepath.Join(watchApp, "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	standaloneRaw, err := decodePlist(standaloneData)
	if err != nil {
		t.Fatal(err)
	}
	if got := standaloneRaw.(map[string]any)["CFBundleIdentifier"]; got != "dev.example.pulse" {
		t.Errorf("standalone watch Info.plist bundle ID changed to %#v", got)
	}

	// Verify App Store Connect IPA reader accepts the watch-only IPA.
	upload, err := readAppleIPA(t.Context(), ipa)
	if err != nil {
		t.Fatalf("readAppleIPA rejected watch-only IPA: %v", err)
	}
	wantUpload := appleUpload{bundle: "dev.example.pulse", version: "1.2.0", build: "7", platform: "IOS"}
	if upload != wantUpload {
		t.Errorf("readAppleIPA = %+v, want %+v", upload, wantUpload)
	}
}

func TestWatchStoreSigningOrderAndProfiles(t *testing.T) {
	sharedCert := []byte("shared-distribution-cert")
	otherCert := []byte("other-cert")
	sum := sha1.Sum(sharedCert)
	sharedSHA := strings.ToUpper(hex.EncodeToString(sum[:]))

	t.Run("explicit stub and watch profiles", func(t *testing.T) {
		dir := t.TempDir()
		p := &project{Config: projectcfg.Config{Dir: dir, Name: "Pulse", ID: "dev.example.pulse", Version: "1.0.0"}}
		p.Apple.SignInWithApple = true
		p.Cloud.KVStore = true

		stubProfile, stubProfileData := writeTestProfile(t, dir, "stub", map[string]any{
			"application-identifier":                "TEAM123456.dev.example.pulse",
			"com.apple.developer.team-identifier":   "TEAM123456",
			"com.apple.developer.applesignin":       []any{"Default"},
			"com.apple.security.application-groups": []any{"group.dev.example.pulse"},
		}, otherCert, sharedCert)
		watchProfile, watchProfileData := writeTestProfile(t, dir, "watch", map[string]any{
			"application-identifier":                          "TEAM123456.dev.example.pulse.watchkitapp",
			"com.apple.developer.team-identifier":             "TEAM123456",
			"com.apple.developer.applesignin":                 []any{"Default"},
			"com.apple.developer.ubiquity-kvstore-identifier": "TEAM123456.dev.example.pulse.watchkitapp",
		}, sharedCert)
		p.Sign.Profile = stubProfile
		p.Sign.WatchProfile = watchProfile

		stage := t.TempDir()
		stubApp := filepath.Join(stage, "Pulse.app")
		watchApp := filepath.Join(stubApp, "Watch", "Pulse.app")
		if err := os.MkdirAll(watchApp, 0o755); err != nil {
			t.Fatal(err)
		}

		type signCall struct {
			target   string
			identity string
			der      bool
			ents     map[string]any
		}
		var calls []signCall
		ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
			switch cmd.name {
			case "security":
				if slices.Equal(cmd.args, []string{"find-identity", "-v", "-p", "codesigning"}) {
					return []byte("  1) " + sharedSHA + ` "Apple Distribution: Example (TEAM123456)"` + "\n"), nil
				}
			case "codesign":
				c := signCall{
					target: cmd.args[len(cmd.args)-1],
					der:    slices.Contains(cmd.args, "--generate-entitlement-der"),
				}
				if i := slices.Index(cmd.args, "--sign"); i >= 0 {
					c.identity = cmd.args[i+1]
				}
				if i := slices.Index(cmd.args, "--entitlements"); i >= 0 {
					data, err := os.ReadFile(cmd.args[i+1])
					if err != nil {
						t.Fatalf("read entitlements: %v", err)
					}
					v, err := decodePlist(data)
					if err != nil {
						t.Fatalf("decode entitlements: %v", err)
					}
					c.ents, _ = v.(map[string]any)
				}
				calls = append(calls, c)
				return nil, nil
			}
			t.Fatalf("unexpected command: %s %q", cmd.name, cmd.args)
			return nil, nil
		})

		if err := signWatchStore(ctx, p, stubApp, watchApp); err != nil {
			t.Fatalf("signWatchStore: %v", err)
		}
		if len(calls) != 2 {
			t.Fatalf("got %d codesign calls, want 2: %+v", len(calls), calls)
		}
		// Inside-out signing order: nested watch app strictly before outer iOS stub.
		if calls[0].target != watchApp || calls[1].target != stubApp {
			t.Fatalf("signing order = [%s, %s], want [%s, %s]", calls[0].target, calls[1].target, watchApp, stubApp)
		}
		for i, c := range calls {
			if c.identity != sharedSHA || !c.der {
				t.Errorf("call %d: identity=%q der=%v, want %q / true", i, c.identity, c.der, sharedSHA)
			}
		}
		if got := calls[0].ents["application-identifier"]; got != "TEAM123456.dev.example.pulse.watchkitapp" {
			t.Errorf("watch app-id = %#v", got)
		}
		if got := calls[0].ents[signInWithAppleEntitlement]; !reflect.DeepEqual(got, []any{"Default"}) {
			t.Errorf("watch Sign in with Apple = %#v", got)
		}
		if got := calls[0].ents[kvStoreEntitlement]; got != "TEAM123456.dev.example.pulse.watchkitapp" {
			t.Errorf("watch KV store = %#v", got)
		}
		if got := calls[1].ents["application-identifier"]; got != "TEAM123456.dev.example.pulse" {
			t.Errorf("stub app-id = %#v", got)
		}
		for _, stripped := range []string{signInWithAppleEntitlement, "com.apple.security.application-groups"} {
			if _, ok := calls[1].ents[stripped]; ok {
				t.Errorf("stub entitlements did not strip %s", stripped)
			}
		}

		// Embedded provisioning profiles must match each bundle.
		gotWatchProv, err := os.ReadFile(filepath.Join(watchApp, "embedded.mobileprovision"))
		if err != nil || !bytes.Equal(gotWatchProv, watchProfileData) {
			t.Errorf("watch embedded.mobileprovision mismatch: %v", err)
		}
		gotStubProv, err := os.ReadFile(filepath.Join(stubApp, "embedded.mobileprovision"))
		if err != nil || !bytes.Equal(gotStubProv, stubProfileData) {
			t.Errorf("stub embedded.mobileprovision mismatch: %v", err)
		}
	})

	t.Run("wildcard sign.profile signs both bundles", func(t *testing.T) {
		dir := t.TempDir()
		p := &project{Config: projectcfg.Config{Dir: dir, Name: "Pulse", ID: "dev.example.pulse", Version: "1.0.0"}}
		wildProfile, wildData := writeTestProfile(t, dir, "wildcard", map[string]any{
			"application-identifier":              "TEAM123456.dev.example.*",
			"com.apple.developer.team-identifier": "TEAM123456",
		}, sharedCert)
		p.Sign.Profile = wildProfile
		p.Sign.Identity = "Apple Distribution: Example (TEAM123456)"

		stage := t.TempDir()
		stubApp := filepath.Join(stage, "Pulse.app")
		watchApp := filepath.Join(stubApp, "Watch", "Pulse.app")
		if err := os.MkdirAll(watchApp, 0o755); err != nil {
			t.Fatal(err)
		}

		var signedIDs []string
		var targets []string
		ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
			if cmd.name != "codesign" {
				t.Fatalf("unexpected command: %s %q", cmd.name, cmd.args)
			}
			targets = append(targets, cmd.args[len(cmd.args)-1])
			i := slices.Index(cmd.args, "--entitlements")
			data, err := os.ReadFile(cmd.args[i+1])
			if err != nil {
				t.Fatal(err)
			}
			v, err := decodePlist(data)
			if err != nil {
				t.Fatal(err)
			}
			signedIDs = append(signedIDs, v.(map[string]any)["application-identifier"].(string))
			return nil, nil
		})

		if err := signWatchStore(ctx, p, stubApp, watchApp); err != nil {
			t.Fatalf("signWatchStore wildcard: %v", err)
		}
		if !slices.Equal(targets, []string{watchApp, stubApp}) {
			t.Fatalf("targets = %v, want [%s %s]", targets, watchApp, stubApp)
		}
		wantIDs := []string{"TEAM123456.dev.example.pulse.watchkitapp", "TEAM123456.dev.example.pulse"}
		if !slices.Equal(signedIDs, wantIDs) {
			t.Fatalf("signed application-identifiers = %v, want %v", signedIDs, wantIDs)
		}
		for _, bundle := range []string{watchApp, stubApp} {
			got, err := os.ReadFile(filepath.Join(bundle, "embedded.mobileprovision"))
			if err != nil || !bytes.Equal(got, wildData) {
				t.Errorf("%s embedded.mobileprovision mismatch: %v", bundle, err)
			}
		}
	})
}

func TestWatchStoreProfileValidationErrors(t *testing.T) {
	dir := t.TempDir()
	certA := []byte("cert-a")
	certB := []byte("cert-b")

	validStub, _ := writeTestProfile(t, dir, "valid-stub", map[string]any{
		"application-identifier":              "TEAM111111.dev.example.pulse",
		"com.apple.developer.team-identifier": "TEAM111111",
	}, certA)
	wrongStubID, _ := writeTestProfile(t, dir, "wrong-stub", map[string]any{
		"application-identifier":              "TEAM111111.dev.example.other",
		"com.apple.developer.team-identifier": "TEAM111111",
	}, certA)
	wrongWatchID, _ := writeTestProfile(t, dir, "wrong-watch", map[string]any{
		"application-identifier":              "TEAM111111.dev.example.other.watchkitapp",
		"com.apple.developer.team-identifier": "TEAM111111",
	}, certA)
	otherTeamWatch, _ := writeTestProfile(t, dir, "other-team-watch", map[string]any{
		"application-identifier":              "TEAM222222.dev.example.pulse.watchkitapp",
		"com.apple.developer.team-identifier": "TEAM222222",
	}, certA)
	validWatchCertB, _ := writeTestProfile(t, dir, "valid-watch-certb", map[string]any{
		"application-identifier":              "TEAM111111.dev.example.pulse.watchkitapp",
		"com.apple.developer.team-identifier": "TEAM111111",
	}, certB)
	noEnts := filepath.Join(dir, "no-ents.mobileprovision")
	if err := os.WriteFile(noEnts, encodePlist(map[string]any{"Name": "Empty"}), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		profile      string
		watchProfile string
		identity     string
		signInApple  bool
		wantErr      string
	}{
		{
			name:         "watch_profile without profile",
			watchProfile: validWatchCertB,
			wantErr:      "needs sign.profile for the iOS stub dev.example.pulse",
		},
		{
			name:    "missing profile file",
			profile: filepath.Join(dir, "missing.mobileprovision"),
			wantErr: "no such file",
		},
		{
			name:    "profile missing entitlements",
			profile: noEnts,
			wantErr: "has no entitlements",
		},
		{
			name:    "mismatched stub bundle ID",
			profile: wrongStubID,
			wantErr: "sign.profile is for TEAM111111.dev.example.other, not dev.example.pulse",
		},
		{
			name:    "explicit stub profile without watch_profile",
			profile: validStub,
			wantErr: "needs sign.watch_profile",
		},
		{
			name:         "mismatched watch bundle ID",
			profile:      validStub,
			watchProfile: wrongWatchID,
			wantErr:      "sign.watch_profile is for TEAM111111.dev.example.other.watchkitapp, not dev.example.pulse.watchkitapp",
		},
		{
			name:         "mismatched team ID",
			profile:      validStub,
			watchProfile: otherTeamWatch,
			wantErr:      "must belong to sign.profile's team TEAM111111",
		},
		{
			name:         "missing Sign in with Apple entitlement",
			profile:      validStub,
			watchProfile: validWatchCertB,
			identity:     "Apple Distribution",
			signInApple:  true,
			wantErr:      "does not grant Sign in with Apple",
		},
		{
			name:         "no shared signing certificate",
			profile:      validStub,
			watchProfile: validWatchCertB,
			wantErr:      "share no signing certificate",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &project{Config: projectcfg.Config{Dir: dir, Name: "Pulse", ID: "dev.example.pulse", Version: "1.0.0"}}
			p.Sign.Profile = tc.profile
			p.Sign.WatchProfile = tc.watchProfile
			p.Sign.Identity = tc.identity
			p.Apple.SignInWithApple = tc.signInApple

			stage := t.TempDir()
			stubApp := filepath.Join(stage, "Pulse.app")
			watchApp := filepath.Join(stubApp, "Watch", "Pulse.app")
			if err := os.MkdirAll(watchApp, 0o755); err != nil {
				t.Fatal(err)
			}
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				t.Fatalf("unexpected external command during validation failure: %s %q", cmd.name, cmd.args)
				return nil, nil
			})
			err := signWatchStore(ctx, p, stubApp, watchApp)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("signWatchStore error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestWatchStubThinsFatBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "MessagesApplicationStub")
	dst := filepath.Join(dir, "Pulse")
	// 0xcafebabe is the big-endian Mach-O universal ("fat") magic.
	if err := os.WriteFile(src, []byte{0xca, 0xfe, 0xba, 0xbe, 0x00, 0x00, 0x00, 0x01}, 0o755); err != nil {
		t.Fatal(err)
	}
	var ranLipo bool
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != "lipo" || !slices.Equal(cmd.args, []string{"-thin", "arm64", src, "-output", dst}) {
			t.Fatalf("unexpected command: %s %q", cmd.name, cmd.args)
		}
		ranLipo = true
		return nil, os.WriteFile(dst, []byte("thinned-arm64"), 0o644)
	})
	if err := copyWatchStubBinary(ctx, src, dst); err != nil {
		t.Fatalf("copyWatchStubBinary: %v", err)
	}
	if !ranLipo {
		t.Fatal("expected lipo -thin arm64 for fat stub binary")
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("thinned stub is not executable: mode %v", info.Mode())
	}
}

func TestWatchStoreRealXcodeStubAndWK(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs macOS Xcode iPhoneOS and WatchOS SDKs")
	}
	if _, err := output(t.Context(), "xcrun", "--sdk", "watchos", "--show-sdk-path"); err != nil {
		t.Skipf("watchOS SDK unavailable: %v", err)
	}
	if _, err := output(t.Context(), "xcrun", "--sdk", "iphoneos", "--show-sdk-platform-path"); err != nil {
		t.Skipf("iPhoneOS platform unavailable: %v", err)
	}
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "Pulse", ID: "dev.example.pulse", Version: "1.0.0", BuildNumber: 3}}
	watchApp := makeWatchFixtureApp(t, dir, p)
	ipa, err := packageWatchStoreIPA(t.Context(), p, watchApp, dir)
	if err != nil {
		t.Fatalf("packageWatchStoreIPA with real Xcode: %v", err)
	}
	files, _ := readZipMap(t, ipa)
	stubBin, ok := files["Payload/Pulse.app/Pulse"]
	if !ok {
		t.Fatal("missing stub executable in IPA")
	}
	mf, err := macho.NewFile(bytes.NewReader(stubBin))
	if err != nil {
		t.Fatalf("stub executable is not a thin Mach-O binary: %v", err)
	}
	if mf.Cpu != macho.CpuArm64 {
		t.Errorf("stub executable CPU = %v, want Arm64", mf.Cpu)
	}
	if _, ok := files["Payload/Pulse.app/Assets.car"]; !ok {
		t.Error("stub bundle missing compiled Assets.car from MessagesApplicationStub.xcassets")
	}
	if _, ok := files["WatchKitSupport2/WK"]; !ok {
		t.Error("IPA missing WatchKitSupport2/WK from WatchOS SDK")
	}
	stubPlistRaw, err := decodePlist(files["Payload/Pulse.app/Info.plist"])
	if err != nil {
		t.Fatal(err)
	}
	stubPlist := stubPlistRaw.(map[string]any)
	if _, ok := stubPlist["CFBundleIcons"]; !ok {
		t.Error("stub Info.plist missing CFBundleIcons from MessagesApplicationStub.xcassets")
	}
	if stubPlist["ITSWatchOnlyContainer"] != true || stubPlist["LSApplicationLaunchProhibited"] != true {
		t.Errorf("stub Info.plist watch-only flags: ITSWatchOnlyContainer=%v LSApplicationLaunchProhibited=%v",
			stubPlist["ITSWatchOnlyContainer"], stubPlist["LSApplicationLaunchProhibited"])
	}
}
