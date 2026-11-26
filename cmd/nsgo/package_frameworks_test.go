package main

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// frameworkMachO writes a thin arm64 Mach-O dynamic library as a framework's binary: its install name, an LC_BUILD_VERSION for platform and minos unless platform is 0, and an LC_LOAD_DYLIB for each of loads.
func frameworkMachO(platform uint32, minos string, install string, loads ...string) []byte {
	order := binary.LittleEndian
	var cmds bytes.Buffer
	var ncmds uint32
	dylib := func(cmd uint32, name string) {
		size := (24 + len(name) + 1 + 7) &^ 7
		b := make([]byte, size)
		order.PutUint32(b, cmd)
		order.PutUint32(b[4:], uint32(size))
		order.PutUint32(b[8:], 24)
		copy(b[24:], name)
		cmds.Write(b)
		ncmds++
	}
	dylib(machoIDDylib, install)
	if platform != 0 {
		version, _ := appleVersion(minos)
		b := make([]byte, 24)
		order.PutUint32(b, machoBuildVersion)
		order.PutUint32(b[4:], 24)
		order.PutUint32(b[8:], platform)
		order.PutUint32(b[12:], version[0]<<16|version[1]<<8|version[2])
		order.PutUint32(b[16:], 27<<16)
		cmds.Write(b)
		ncmds++
	}
	for _, load := range loads {
		dylib(machoLoadDylib, load)
	}
	header := make([]byte, 32)
	order.PutUint32(header, macho.Magic64)
	order.PutUint32(header[4:], uint32(macho.CpuArm64))
	order.PutUint32(header[12:], uint32(macho.TypeDylib))
	order.PutUint32(header[16:], ncmds)
	order.PutUint32(header[20:], uint32(cmds.Len()))
	return append(header, cmds.Bytes()...)
}

// fatMachO joins thin files into a universal one, as lipo does.
func fatMachO(thin ...[]byte) []byte {
	const align = 1 << 14
	data := make([]byte, 8+20*len(thin))
	binary.BigEndian.PutUint32(data, macho.MagicFat)
	binary.BigEndian.PutUint32(data[4:], uint32(len(thin)))
	for i, slice := range thin {
		data = append(data, make([]byte, (len(data)+align-1)&^(align-1)-len(data))...)
		entry := data[8+20*i:]
		binary.BigEndian.PutUint32(entry, binary.LittleEndian.Uint32(slice[4:]))
		binary.BigEndian.PutUint32(entry[4:], binary.LittleEndian.Uint32(slice[8:]))
		binary.BigEndian.PutUint32(entry[8:], uint32(len(data)))
		binary.BigEndian.PutUint32(entry[12:], uint32(len(slice)))
		binary.BigEndian.PutUint32(entry[16:], 14)
		data = append(data, slice...)
	}
	return data
}

// iosListEnv is enough of iosEnv for go list, which reads no SDK: Go builds iOS only with cgo.
var iosListEnv = []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1"}

// frameworkFixture is nativeFixture with a version 2 file that declares data as Fixture.framework for target and [] for the others of extra.
func frameworkFixture(t *testing.T, target string, data []byte, extra ...string) *project {
	t.Helper()
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	manifest := fmt.Sprintf("version: 2\nlibraries:\n  %s:\n    - name: Fixture.framework\n      path: lib/Fixture\n      sha256: %s\n      licenses: [legal/native.txt]\n", target, nativeDigest(data))
	for _, other := range extra {
		manifest += "  " + other + ": []\n"
	}
	writeFiles(t, filepath.Join(filepath.Dir(p.Dir), "dep"), map[string]string{"lib/Fixture": string(data), nativeManifestName: manifest})
	return p
}

// passThrough runs cmd as it would run without the test's process runner.
func passThrough(ctx context.Context, cmd processCommand) ([]byte, error) {
	return execute(context.WithValue(ctx, processRunnerKey{}, nil), cmd)
}

func readFrameworkPlist(t *testing.T, fw string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fw, "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := decodePlist(data)
	if err != nil {
		t.Fatal(err)
	}
	plist, _ := v.(map[string]any)
	return plist
}

// A framework the app could not load is refused with the reason: one for another platform, a simulator's for a device, one that needs a newer OS, one under another install name, one that loads a peer no package declares, and a file that is no arm64 dynamic library. Frameworks the app loads pass, a universal one by its arm64 slice.
func TestPackageFrameworksRefuseMachO(t *testing.T) {
	const install = "@rpath/Fixture.framework/Fixture"
	declared := []string{"Fixture.framework", "Peer.framework"}
	for _, c := range []struct {
		name      string
		sys       *appleOS
		simulator bool
		data      []byte
		reason    string
	}{
		{"iOS device framework", iOS, false, frameworkMachO(2, "15.0", install, "/usr/lib/libSystem.B.dylib", "@rpath/Peer.framework/Peer"), ""},
		{"iOS simulator framework", iOS, true, frameworkMachO(7, "12.0", install), ""},
		{"tvOS simulator framework", tvOS, true, frameworkMachO(8, "17.0", install), ""},
		{"universal framework", iOS, true, fatMachO(nativeMachO(false), frameworkMachO(7, "15.0", install)), ""},
		{"binary for another platform", iOS, false, frameworkMachO(1, "13.0", install), "built for macOS, not iOS devices"},
		{"iOS binary for tvOS", tvOS, false, frameworkMachO(2, "15.0", install), "built for iOS devices, not tvOS devices"},
		{"simulator binary for a device", iOS, false, frameworkMachO(7, "15.0", install), "built for the iOS simulator, not iOS devices"},
		{"device binary for the simulator", tvOS, true, frameworkMachO(3, "17.0", install), "built for tvOS devices, not the tvOS simulator"},
		{"newer minimum OS", iOS, true, frameworkMachO(7, "16.1", install), "needs iOS 16.1, newer than the app's minimum 15.0"},
		{"another install name", iOS, true, frameworkMachO(7, "15.0", "@rpath/libfixture.dylib"), `its install name is "@rpath/libfixture.dylib", not @rpath/Fixture.framework/Fixture`},
		{"missing peer", iOS, true, frameworkMachO(7, "15.0", install, "@rpath/libavutil.framework/libavutil"), "loads @rpath/libavutil.framework/libavutil, which no framework packaged for ios_simulator provides"},
		{"peer in another layout", iOS, true, frameworkMachO(7, "15.0", install, "@rpath/Peer.framework/Versions/A/Peer"), "loads @rpath/Peer.framework/Versions/A/Peer, which no framework"},
		{"no platform", iOS, true, frameworkMachO(0, "", install), "names no platform: it has no LC_BUILD_VERSION"},
		{"Intel binary", iOS, true, nativeMachO(false), "not an arm64 Mach-O dynamic library"},
		{"universal binary without arm64", iOS, true, fatMachO(nativeMachO(false)), "not an arm64 Mach-O dynamic library"},
		{"ELF file", iOS, true, nativeELF(true), "not an arm64 Mach-O dynamic library"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Fixture")
			if err := os.WriteFile(path, c.data, 0o644); err != nil {
				t.Fatal(err)
			}
			minimum := c.sys.minimum
			err := checkFramework(path, c.sys, c.simulator, "Fixture.framework", minimum, declared)
			if c.reason == "" {
				if err != nil {
					t.Fatalf("refused a framework the app loads: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.reason) {
				t.Fatalf("got %v, want the reason %q", err, c.reason)
			}
		})
	}
}

// A build for the iOS or tvOS simulator writes each framework with its Info.plist and signs it ad hoc, its notices joining the app's; the app's own minimum, not the runtime's, is the framework's.
func TestPackageFrameworksSimulatorBundles(t *testing.T) {
	for _, c := range []struct {
		sys      *appleOS
		platform uint32
		name     string
	}{
		{iOS, 7, "iPhoneSimulator"},
		{tvOS, 8, "AppleTVSimulator"},
	} {
		t.Run(c.sys.name, func(t *testing.T) {
			target := c.sys.target + "_simulator"
			data := frameworkMachO(c.platform, "15.0", "@rpath/Fixture.framework/Fixture")
			p := frameworkFixture(t, target, data)
			p.Apple.Info = map[string]any{"MinimumOSVersion": "17.2"}
			if _, err := p.licenseNoticesFile(t.Context(), target, iosListEnv, nil, nil, ""); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(p.thirdPartyNotices, "native library Fixture.framework") || !strings.Contains(p.thirdPartyNotices, "legal/native.txt") {
				t.Fatalf("the framework's licenses are not among the notices:\n%s", p.thirdPartyNotices)
			}
			app := filepath.Join(t.TempDir(), "Fixture.app")
			fw := filepath.Join(app, "Frameworks", "Fixture.framework")
			var signed []string
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				signed = append(signed, cmd.name+" "+strings.Join(cmd.args, " "))
				return nil, nil
			})
			if err := addPackageFrameworks(ctx, p, c.sys, app, true); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(fw, "Fixture")); err != nil || !bytes.Equal(got, data) {
				t.Fatalf("framework binary: %v", err)
			}
			plist := readFrameworkPlist(t, fw)
			for key, want := range map[string]any{
				"CFBundleExecutable":         "Fixture",
				"CFBundleIdentifier":         "dev.example.fixture.fixture",
				"CFBundleName":               "Fixture",
				"CFBundlePackageType":        "FMWK",
				"CFBundleSupportedPlatforms": []any{c.name},
				"MinimumOSVersion":           "17.2",
			} {
				if got := plist[key]; fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("Info.plist %s = %v, want %v", key, got, want)
				}
			}
			if want := []string{"codesign --force --sign - " + fw}; !slices.Equal(signed, want) {
				t.Fatalf("ran %q, want %q", signed, want)
			}
		})
	}
}

// A device build leaves its frameworks unsigned, and sign.profile signs them with the app's identity before the app, whose signature seals them.
func TestPackageFrameworksSignedBeforeDeviceApp(t *testing.T) {
	p := frameworkFixture(t, "ios", frameworkMachO(2, "15.0", "@rpath/Fixture.framework/Fixture"))
	if _, err := p.licenseNoticesFile(t.Context(), "ios", iosListEnv, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	app := filepath.Join(dir, "Fixture.app")
	var signed []string
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != "codesign" {
			t.Fatalf("unexpected %s", cmd.name)
		}
		signed = append(signed, filepath.Base(cmd.args[len(cmd.args)-1]))
		if !slices.Contains(cmd.args, "fixture identity") {
			t.Errorf("signed %s with %q", cmd.args[len(cmd.args)-1], cmd.args)
		}
		return nil, nil
	})
	if err := addPackageFrameworks(ctx, p, iOS, app, false); err != nil {
		t.Fatal(err)
	}
	if len(signed) != 0 {
		t.Fatalf("a device build signed %v ad hoc", signed)
	}
	if plist := readFrameworkPlist(t, filepath.Join(app, "Frameworks", "Fixture.framework")); fmt.Sprint(plist["CFBundleSupportedPlatforms"]) != "[iPhoneOS]" {
		t.Fatalf("device framework platforms = %v", plist["CFBundleSupportedPlatforms"])
	}
	p.Sign = signing{Identity: "fixture identity", Profile: filepath.Join(dir, "app.profile")}
	if err := os.WriteFile(p.Sign.Profile, encodePlist(map[string]any{"Entitlements": map[string]any{"application-identifier": "TEAM." + p.ID}}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := signForStore(ctx, p, iOS, app); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Fixture.framework", "Fixture.app"}; !slices.Equal(signed, want) {
		t.Fatalf("signed %v, want %v", signed, want)
	}
}

// A version 1 file declares nothing for iOS: an iOS build that links it packages and notices nothing from it, as before version 2, and still needs no entry for iOS.
func TestPackageFrameworksVersion1Unchanged(t *testing.T) {
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	for _, target := range []string{"ios", "ios_simulator", "tvos", "tvos_simulator"} {
		if _, err := p.licenseNoticesFile(t.Context(), target, iosListEnv, nil, nil, ""); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if len(p.nativeLibraries) != 0 || strings.Contains(p.thirdPartyNotices, "native library") {
			t.Fatalf("%s took %v from a version 1 file", target, p.nativeLibraries)
		}
		app := filepath.Join(t.TempDir(), "Fixture.app")
		sys, simulator, _ := appleOSOf(target)
		if err := addPackageFrameworks(t.Context(), p, sys, app, simulator); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(app); !os.IsNotExist(err) {
			t.Fatalf("%s wrote %s", target, app)
		}
	}
	dep := filepath.Join(filepath.Dir(p.Dir), "dep", nativeManifestName)
	data, err := os.ReadFile(dep)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dep, bytes.Replace(data, []byte("version: 1"), []byte("version: 2"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.licenseNoticesFile(t.Context(), "ios", iosListEnv, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "has no ios entry") {
		t.Fatalf("a version 2 file without ios: %v", err)
	}
}

// The terminal targets read their desktop's entries, and a terminal build puts the libraries beside its executable, signed on macOS.
func TestPackageNativesTerminalTargets(t *testing.T) {
	for target, want := range map[string]string{
		"tui_macos_arm64": "macos_arm64", "tui_windows_x64": "windows_x64", "tui_linux_arm64": "linux_arm64",
		"macos_x64": "macos_x64", "ios_simulator": "ios_simulator", "tvos": "tvos",
		"macos_universal": "", "watchos": "", "android_arm64": "", "web": "", "tui": "", "": "",
	} {
		if got := nativeTarget(target); got != want {
			t.Errorf("nativeTarget(%q) = %q, want %q", target, got, want)
		}
	}
	data := nativeMachO(true)
	p := nativeFixture(t, "macos_arm64", "libfixture.dylib", data)
	dir := t.TempDir()
	var signed []string
	ctx := withProcessRunner(t, func(ctx context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != "codesign" {
			return passThrough(ctx, cmd)
		}
		signed = append(signed, strings.Join(cmd.args, " "))
		return nil, nil
	})
	if err := buildTUI(ctx, p, dir, "tui_macos_arm64"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "libfixture.dylib")); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("the terminal build lacks its library: %v", err)
	}
	if notices, err := os.ReadFile(filepath.Join(dir, noticesName)); err != nil || !bytes.Contains(notices, []byte("native library libfixture.dylib")) {
		t.Fatalf("the terminal build's notices: %v", err)
	}
	if want := []string{"--force --sign - " + filepath.Join(dir, "libfixture.dylib")}; !slices.Equal(signed, want) {
		t.Fatalf("signed %q, want %q", signed, want)
	}
	if err := buildTUI(ctx, p, t.TempDir(), "tui_linux_x64"); err == nil || !strings.Contains(err.Error(), "no linux_x64 entry") {
		t.Fatalf("a terminal build for another desktop: %v", err)
	}
}

// -embed has nowhere to put a package's frameworks, so it stops at the first slice that links one, naming the package.
func TestPackageFrameworksRefusedByEmbed(t *testing.T) {
	p := frameworkFixture(t, "ios", frameworkMachO(2, "15.0", "@rpath/Fixture.framework/Fixture"), "ios_simulator")
	p.Build.embed = true
	p.packageLicenses = false
	// The file -embed adds to the app imports NSGo, which go list then only has to find.
	writeFiles(t, filepath.Dir(p.Dir), map[string]string{
		"app/go.mod":   "module example.com/app\ngo 1.27.3\nrequire (\n\texample.com/native v0.0.0\n\tgithub.com/nsgo-dev/nsgo v0.0.0\n)\nreplace example.com/native => ../dep\nreplace github.com/nsgo-dev/nsgo => ../nsgo\n",
		"nsgo/go.mod":  "module github.com/nsgo-dev/nsgo\ngo 1.27.3\n",
		"nsgo/nsgo.go": "package nsgo\n\nfunc StartEmbedded(func()) {}\n",
	})
	ctx := withProcessRunner(t, func(ctx context.Context, cmd processCommand) ([]byte, error) {
		switch cmd.name {
		case "xcrun":
			return []byte("/nonexistent/fixture\n"), nil
		case "go":
			return passThrough(ctx, cmd)
		}
		t.Fatalf("ran %s before refusing", cmd.name)
		return nil, nil
	})
	err := buildTarget(ctx, p, "ios", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "-embed cannot carry the ios frameworks that example.com/native declares") {
		t.Fatalf("-embed with a package's frameworks: %v", err)
	}
	// A package whose frameworks are for tvOS only gives an iOS host nothing to carry.
	p = frameworkFixture(t, "tvos", frameworkMachO(3, "17.0", "@rpath/Fixture.framework/Fixture"), "ios", "ios_simulator")
	p.Build.embed = true
	p.packageLicenses = false
	if _, err := p.buildOverlay(t.Context(), "ios", iosListEnv, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}

// Two packages may share a framework only as the same file.
func TestPackageFrameworksConflict(t *testing.T) {
	data := frameworkMachO(7, "15.0", "@rpath/Fixture.framework/Fixture")
	p := frameworkFixture(t, "ios_simulator", data)
	root := filepath.Dir(p.Dir)
	license, err := os.ReadFile(filepath.Join(root, "dep", "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	other := frameworkMachO(7, "14.0", "@rpath/Fixture.framework/Fixture")
	writeFiles(t, root, map[string]string{
		"app/go.mod":                  "module example.com/app\ngo 1.27.3\nrequire (\n\texample.com/native v0.0.0\n\texample.com/other v0.0.0\n)\nreplace example.com/native => ../dep\nreplace example.com/other => ../other\n",
		"app/main.go":                 "package main\nimport (\n\t_ \"example.com/native\"\n\t_ \"example.com/other\"\n)\nfunc main() {}\n",
		"other/go.mod":                "module example.com/other\ngo 1.27.3\n",
		"other/other.go":              "package other\n",
		"other/LICENSE":               string(license),
		"other/lib/Fixture":           string(other),
		"other/" + nativeManifestName: fmt.Sprintf("version: 2\nlibraries:\n  ios_simulator:\n    - name: fixture.framework\n      path: lib/Fixture\n      sha256: %s\n      licenses: [LICENSE]\n", nativeDigest(other)),
	})
	if _, err := p.licenseNoticesFile(t.Context(), "ios_simulator", iosListEnv, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "conflicts between") {
		t.Fatalf("two files under one name: %v", err)
	}
}

func TestPackageFrameworksRejectInvalidManifests(t *testing.T) {
	sum := strings.Repeat("a", 64)
	good := "version: 2\nlibraries:\n  ios:\n    - name: Fixture.framework\n      path: lib/Fixture\n      sha256: " + sum + "\n      licenses: [LICENSE]\n"
	for _, valid := range []string{
		good,
		strings.ReplaceAll(good, "Fixture.framework", "lib-av.util2.framework"),
		strings.ReplaceAll(good, "path: lib/Fixture", "url: https://example.com/ios/Fixture"),
		"version: 2\nlibraries:\n  linux_x64:\n    - name: libfixture.so\n      path: lib.so\n      sha256: " + sum + "\n      licenses: [LICENSE]\n  tvos_simulator: []\n",
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, nativeManifestName), []byte(valid), 0o644)
		if _, err := readNativeManifest(dir); err != nil {
			t.Errorf("refused %q: %v", valid, err)
		}
	}
	framework := "must be <Name>.framework"
	for name, c := range map[string]struct{ manifest, reason string }{
		"version 3":               {strings.ReplaceAll(good, "version: 2", "version: 3"), "this nsgo reads versions 1 and 2"},
		"ios in version 1":        {strings.ReplaceAll(good, "version: 2", "version: 1"), "ios needs version: 2"},
		"watchOS":                 {strings.ReplaceAll(good, "  ios:", "  watchos:"), `unsupported native-library target "watchos"`},
		"Android":                 {strings.ReplaceAll(good, "  ios:", "  android_arm64:"), `unsupported native-library target "android_arm64"`},
		"not a framework":         {strings.ReplaceAll(good, "Fixture.framework", "libfixture.dylib"), framework},
		"no name":                 {strings.ReplaceAll(good, "Fixture.framework", ".framework"), framework},
		"path in the name":        {strings.ReplaceAll(good, "Fixture.framework", "../Fixture.framework"), framework},
		"hidden name":             {strings.ReplaceAll(good, "Fixture.framework", ".Fixture.framework"), framework},
		"name ending in a dot":    {strings.ReplaceAll(good, "Fixture.framework", "Fixture..framework"), framework},
		"name with an underscore": {strings.ReplaceAll(good, "Fixture.framework", "Fix_ture.framework"), framework},
		"name with a space":       {strings.ReplaceAll(good, "Fixture.framework", "\"Fix ture.framework\""), framework},
		"non-ASCII name":          {strings.ReplaceAll(good, "Fixture.framework", "Fixtüre.framework"), framework},
		"http":                    {strings.ReplaceAll(good, "path: lib/Fixture", "url: http://example.com/ios/Fixture"), "needs an https URL"},
		"http for a desktop":      {"version: 2\nlibraries:\n  linux_x64:\n    - name: libfixture.so\n      url: http://example.com/lib.so\n      sha256: " + sum + "\n      licenses: [LICENSE]\n", "needs an https URL"},
		"two sources":             {strings.ReplaceAll(good, "path: lib/Fixture", "path: lib/Fixture\n      url: https://example.com/Fixture"), "exactly one url or path"},
		"no license":              {strings.ReplaceAll(good, "[LICENSE]", "[]"), "needs license file paths"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, nativeManifestName), []byte(c.manifest), 0o644)
			if _, err := readNativeManifest(dir); err == nil || !strings.Contains(err.Error(), c.reason) {
				t.Fatalf("got %v, want the reason %q", err, c.reason)
			}
		})
	}
}

// Version 2 downloads over https only: a redirect to http fails, where version 1 follows it.
func TestPackageFrameworksHTTPSDownloads(t *testing.T) {
	data := frameworkMachO(2, "15.0", "@rpath/Fixture.framework/Fixture")
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, plain.URL+"/Fixture", http.StatusFound)
			return
		}
		w.Write(data)
	}))
	defer secure.Close()
	nativeTransport = secure.Client().Transport
	t.Cleanup(func() { nativeTransport = http.DefaultTransport })
	library := nativeLibrary{Name: "Fixture.framework", URL: secure.URL + "/Fixture", SHA256: nativeDigest(data), version: 2}
	if _, err := library.fetch(t.Context(), t.TempDir()); err != nil {
		t.Fatalf("an https download: %v", err)
	}
	library.URL = secure.URL + "/redirect"
	if _, err := library.fetch(t.Context(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "which is not https") {
		t.Fatalf("a redirect to http: %v", err)
	}
	library.version = 1
	if _, err := library.fetch(t.Context(), t.TempDir()); err != nil {
		t.Fatalf("version 1 refused a redirect it followed before: %v", err)
	}
}

// Opt in on a Mac with Xcode, as for TestPackageNativesMacUniversalE2E: nsgo build for the iOS and tvOS simulators, nsgo dev and -run, with frameworks that clang links for the test, signed and verified by codesign. -run installs into a simulator the process runner stands in for.
func TestPackageFrameworksSimulatorE2E(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("NSGO_NATIVE_LIBRARY_E2E") != "1" {
		t.Skip("set NSGO_NATIVE_LIBRARY_E2E=1 on a Mac with Xcode")
	}
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	dep := filepath.Join(filepath.Dir(p.Dir), "dep")
	manifest := "version: 2\nlibraries:\n"
	for _, c := range []struct{ target, sdk, triple string }{
		{"ios_simulator", "iphonesimulator", "arm64-apple-ios15.0-simulator"},
		{"tvos_simulator", "appletvsimulator", "arm64-apple-tvos17.0-simulator"},
	} {
		// An empty library: what NSGo checks, places and signs is the file, not its code.
		lib := filepath.Join(dep, "lib", c.target, "Fixture")
		if err := os.MkdirAll(filepath.Dir(lib), 0o755); err != nil {
			t.Fatal(err)
		}
		link := exec.CommandContext(t.Context(), "xcrun", "--sdk", c.sdk, "clang", "-target", c.triple, "-dynamiclib",
			"-install_name", "@rpath/Fixture.framework/Fixture", "-o", lib, "-x", "c", "/dev/null")
		if output, err := link.CombinedOutput(); err != nil {
			t.Fatalf("link the %s framework: %v\n%s", c.target, err, output)
		}
		data, err := os.ReadFile(lib)
		if err != nil {
			t.Fatal(err)
		}
		manifest += fmt.Sprintf("  %s:\n    - name: Fixture.framework\n      path: lib/%s/Fixture\n      sha256: %s\n      licenses: [legal/native.txt]\n", c.target, c.target, nativeDigest(data))
	}
	writeFiles(t, dep, map[string]string{nativeManifestName: manifest})
	verify := func(app, platform string) {
		t.Helper()
		fw := filepath.Join(app, "Frameworks", "Fixture.framework")
		if plist := readFrameworkPlist(t, fw); fmt.Sprint(plist["CFBundleSupportedPlatforms"]) != "["+platform+"]" {
			t.Fatalf("%s: platforms %v", fw, plist["CFBundleSupportedPlatforms"])
		}
		for _, args := range [][]string{{"--verify", "--strict", fw}, {"--verify", "--deep", "--strict", app}} {
			if output, err := exec.CommandContext(t.Context(), "codesign", args...).CombinedOutput(); err != nil {
				t.Fatalf("codesign %v: %v\n%s", args, err, output)
			}
		}
	}
	for target, platform := range map[string]string{"ios_simulator": "iPhoneSimulator", "tvos_simulator": "AppleTVSimulator"} {
		q := *p
		q.Release = true
		dir := t.TempDir()
		if err := buildTarget(t.Context(), &q, target, dir); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		app := filepath.Join(dir, "Fixture.app")
		verify(app, platform)
		if notices, err := os.ReadFile(filepath.Join(app, noticesName)); err != nil || !bytes.Contains(notices, []byte("native library Fixture.framework")) {
			t.Fatalf("%s notices: %v", target, err)
		}
	}
	dev := *p
	dev.packageLicenses = false
	if err := iosSession(iOS, devOptions{}).build(t.Context(), &dev); err != nil {
		t.Fatalf("nsgo dev: %v", err)
	}
	verify(filepath.Join(dev.cache("dev", "ios_simulator"), "Fixture.app"), "iPhoneSimulator")
	var installed []string
	ctx := withProcessRunner(t, func(ctx context.Context, cmd processCommand) ([]byte, error) {
		switch {
		case cmd.name == "open":
			return nil, nil
		case cmd.name != "xcrun" || len(cmd.args) < 2 || cmd.args[0] != "simctl":
			return passThrough(ctx, cmd)
		case cmd.args[1] == "list":
			return []byte(`{"devices": {"com.apple.CoreSimulator.SimRuntime.iOS-27-0": [{"udid": "FIXTURE", "name": "iPhone 18 Pro", "state": "Booted"}]}}`), nil
		case cmd.args[1] == "install":
			app := cmd.args[len(cmd.args)-1]
			verify(app, "iPhoneSimulator")
			installed = append(installed, app)
		}
		return nil, nil
	})
	run := *p
	run.Build.Targets, run.Build.run, run.Build.Output = []string{"ios_simulator"}, true, filepath.Join(t.TempDir(), "dist")
	if err := buildAll(ctx, &run); err != nil {
		t.Fatalf("nsgo build -run: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("-run installed %v", installed)
	}
}
