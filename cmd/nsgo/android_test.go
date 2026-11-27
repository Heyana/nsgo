package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
	projectcfg "github.com/nsgo-dev/nsgo/project"
)

func TestAndroidRuntimeTargetsPlayAPI(t *testing.T) {
	if !strings.Contains(string(androidruntime.Manifest), `android:targetSdkVersion="36"`) {
		t.Fatal("the Android runtime does not target API 36")
	}
}

// TestFindAdbInTheSDK checks the fallback that finds adb without it being
// on PATH, so nsgo dev works after installing the SDK in its usual place.
func TestFindAdbInTheSDK(t *testing.T) {
	home := t.TempDir()
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	want := filepath.Join(home, "Android", "platform-tools", name)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	got, err := findAdb()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("findAdb = %q, want %q", got, want)
	}
}

// TestFindAdbWithoutTheSDK checks the error that names what to install.
func TestFindAdbWithoutTheSDK(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	if _, err := findAdb(); err == nil {
		t.Fatal("findAdb found adb in an empty machine")
	}
}

// TestAndroidArchitectures checks the two packages and the ABIs they run on.
func TestAndroidArchitectures(t *testing.T) {
	for goarch, abi := range map[string]string{"arm64": "arm64-v8a", "amd64": "x86_64"} {
		got, err := androidABI(goarch)
		if err != nil || got != abi {
			t.Errorf("androidABI(%q) = %q, %v; want %q", goarch, got, err, abi)
		}
		want, err := goarchForABI(abi)
		if err != nil || want != goarch {
			t.Errorf("goarchForABI(%q) = %q, %v; want %q", abi, want, err, goarch)
		}
	}
	if _, err := androidABI("386"); err == nil {
		t.Error("androidABI(386) succeeded")
	}
	// 32-bit ARM says why and where it is discussed; other ABIs do not claim to be 32-bit ARM.
	for abi, arm := range map[string]bool{"armeabi-v7a": true, "armeabi": true, "x86": false, "riscv64": false} {
		_, err := goarchForABI(abi)
		if err == nil {
			t.Errorf("goarchForABI(%q) succeeded", abi)
			continue
		}
		if says := strings.Contains(err.Error(), "/issues/41") && strings.Contains(err.Error(), "NDK"); says != arm {
			t.Errorf("goarchForABI(%q) error = %v; names 32-bit ARM's limit: %v, want %v", abi, err, says, arm)
		}
	}
}

// TestFindNDKInTheSDK checks that the emulator build finds the newest NDK
// in the SDK and the clang that targets the manifest's API.
func TestFindNDKInTheSDK(t *testing.T) {
	sdk := t.TempDir()
	name := "x86_64-linux-android28-clang"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	for _, version := range []string{"26.1.10909125", "27.2.12479018"} {
		bin := filepath.Join(sdk, "ndk", version, "toolchains", "llvm", "prebuilt", "linux-x86_64", "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ANDROID_HOME", sdk)
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("ANDROID_NDK_HOME", "")
	t.Setenv("ANDROID_NDK_ROOT", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	got, err := findNDK()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(sdk, "ndk", "27.2.12479018"); got != want {
		t.Fatalf("findNDK = %q, want %q", got, want)
	}
	clang, err := androidClang("amd64")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(got, "toolchains", "llvm", "prebuilt", "linux-x86_64", "bin", name); clang != want {
		t.Fatalf("androidClang = %q, want %q", clang, want)
	}
}

func TestAndroidGhosttyLibrary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib", "android-arm64", "libghostty-vt.so")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("ELF ghostty")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	// linkedDir is deliberately bypassed here: this test exercises the APK
	// path and ABI naming after dependency discovery has selected the package.
	file, err := androidGhosttyFile(dir, "arm64", "arm64-v8a")
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "lib/arm64-v8a/libghostty-vt.so" || !bytes.Equal(file.Data, want) {
		t.Fatalf("Android Ghostty file = %+v, want ABI path and bytes", file)
	}
	// x64 has no prebuild and cannot be listed without cgo, so it must not reach go list at all.
	if file, err := androidGhosttyLibrary(t.Context(), nil, "amd64", "x86_64"); err != nil || file.Data != nil {
		t.Fatalf("Android x64 Ghostty library = %+v, %v; want none", file, err)
	}
}

// TestAndroidLibraries checks that only an app whose function table names the fold geometry's reader and whose sources select it declares the window extensions.
func TestAndroidLibraries(t *testing.T) {
	if libs, err := androidLibraries(t.Context(), nil, "arm64", []byte("\x00github.com/nsgo-dev/nsgo/core.(*Env).Window\x00")); libs != nil || err != nil {
		t.Errorf("an app that reads no geometry declares %v (%v)", libs, err)
	}
	app := []byte("\x00" + geometryReader + "\x00")
	for _, c := range []struct {
		name, main string
		want       []string
	}{
		{"reads its geometry", "package main\n\nimport \"example.com/app/term\"\n\nfunc main() { var e interface{ Geometry() term.Geometry }; _ = e.Geometry() }\n", []string{windowExtensions}},
		// The name kept by reflection, as in a TSX app, with only a package's own Geometry in the sources.
		{"names a package's Geometry", "package main\n\nimport \"example.com/app/term\"\n\nfunc main() { _ = term.Geometry{} }\n", nil},
	} {
		dir := t.TempDir()
		for name, src := range map[string]string{
			"go.mod":       "module example.com/app\n\ngo 1.26\n",
			"main.go":      c.main,
			"term/term.go": "package term\n\ntype Geometry struct{}\n",
		} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		libs, err := androidLibraries(t.Context(), &project{Config: projectcfg.Config{Dir: dir}}, "arm64", app)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(libs, c.want) {
			t.Errorf("an app that %s declares %v, want %v", c.name, libs, c.want)
		}
	}
	if m := aarManifest(false, nil, false, false); strings.Contains(m, "uses-library") {
		t.Errorf("an AAR that reads no geometry declares a library:\n%s", m)
	}
	if m := aarManifest(false, []string{windowExtensions}, false, false); !strings.Contains(m, `<uses-library android:name="androidx.window.extensions" android:required="false" />`) {
		t.Errorf("the AAR's manifest lacks the window extensions:\n%s", m)
	}
	if m := aarManifest(false, nil, false, false); !strings.Contains(m, `android.intent.action.PROCESS_TEXT`) || !strings.Contains(m, `android:mimeType="text/plain"`) {
		t.Errorf("the AAR's manifest lacks the ACTION_PROCESS_TEXT query:\n%s", m)
	}
}

// Both package forms must keep action delivery private to the app's runtime.
func TestAndroidLiveUpdateReceiverManifests(t *testing.T) {
	data, err := os.ReadFile("../../internal/platform/android/runtime/AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	for name, manifest := range map[string]string{"APK": string(data), "AAR": aarManifest(false, nil, false, false)} {
		var doc struct {
			Application struct {
				Receivers []struct {
					Name     string `xml:"name,attr"`
					Exported string `xml:"exported,attr"`
				} `xml:"receiver"`
			} `xml:"application"`
		}
		if err := xml.Unmarshal([]byte(manifest), &doc); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range doc.Application.Receivers {
			if r.Name == "dev.oneo.nsgo.LiveUpdateReceiver" {
				found = true
				if r.Exported != "false" {
					t.Errorf("%s exports the action receiver", name)
				}
			}
		}
		if !found {
			t.Errorf("%s omits the action receiver", name)
		}
	}
}

func TestFileSelectsGeometry(t *testing.T) {
	names := map[string]string{"github.com/nsgo-dev/nsgo/core": "core", "example.com/vt/v2": "vt"}
	for _, c := range []struct {
		src  string
		want bool
	}{
		{`package a; import "github.com/nsgo-dev/nsgo/core"; func f(cx *core.Scope) { _ = cx.Env().Geometry() }`, true},
		{`package a; import "github.com/nsgo-dev/nsgo/core"; func f(e *core.Env) func() core.Geometry { return e.Geometry }`, true},
		{`package a; import "github.com/nsgo-dev/nsgo/core"; var g core.Geometry`, false},
		{`package a; import nsgo "github.com/nsgo-dev/nsgo/core"; var g nsgo.Geometry`, false},
		{`package a; import "example.com/vt/v2"; var g = vt.Geometry{}`, false},
		{`package a; func f(g struct{ Geometry int }) int { return g.Geometry }`, true},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), "a.go", c.src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		if got := fileSelectsGeometry(f, names); got != c.want {
			t.Errorf("%s: %v, want %v", c.src, got, c.want)
		}
	}
}

// TestAndroidDebuggableBuilds ensures only test binaries and non-release automation dev builds mark the APK debuggable, so release and ordinary builds never instantiate TestAgent.
func TestAndroidDebuggableBuilds(t *testing.T) {
	cases := []struct {
		name string
		p    project
		want bool
	}{
		{"release build", project{Release: true}, false},
		{"plain dev build", func() project {
			var p project
			p.prepareDev(devOptions{})
			return p
		}(), false},
		{"automation dev build", func() project {
			var p project
			p.prepareDev(devOptions{automation: true})
			return p
		}(), true},
		{"release with automation flag", func() project {
			var p project
			p.prepareDev(devOptions{automation: true, release: true})
			return p
		}(), false},
		{"test binary", project{testBinary: true}, true},
	}
	for _, tc := range cases {
		if got := androidDebuggable(&tc.p); got != tc.want {
			t.Errorf("%s: androidDebuggable = %v, want %v", tc.name, got, tc.want)
		}
		encoded, err := apk.EncodeManifest(androidruntime.Manifest, apk.Manifest{
			Package: "dev.oneo.nsgo.test", Label: "Test", VersionName: "1.0.0", VersionCode: 1,
			Debuggable: androidDebuggable(&tc.p),
		})
		if err != nil {
			t.Fatalf("%s: EncodeManifest: %v", tc.name, err)
		}
		// Binary XML string pool encodes attribute names in UTF-16LE.
		hasDebuggable := bytes.Contains(encoded, []byte{'d', 0, 'e', 0, 'b', 0, 'u', 0, 'g', 0, 'g', 0, 'a', 0, 'b', 0, 'l', 0, 'e', 0})
		if hasDebuggable != tc.want {
			t.Errorf("%s: encoded manifest debuggable = %v, want %v", tc.name, hasDebuggable, tc.want)
		}
	}
}

// TestAndroidAutomationStartArgs checks the am start intent extras for automation and engine flags, and verifies the extra name matches TestAgent.java.
func TestAndroidAutomationStartArgs(t *testing.T) {
	plain := androidStartArgs("dev.example.app", devOptions{fps: true, lang: "de"})
	wantPlain := []string{
		"shell", "am", "start", "-S", "-n", "dev.example.app/dev.oneo.nsgo.NsgoActivity",
		"--ez", "nsgo-fps", "true", "--es", "nsgo-lang", "de",
	}
	if !slices.Equal(plain, wantPlain) {
		t.Errorf("plain start args = %q, want %q", plain, wantPlain)
	}

	auto := androidStartArgs("dev.example.app", devOptions{automation: true, fps: true, lang: "de"})
	wantAuto := append(slices.Clone(wantPlain), "--ez", "nsgo.automation", "true")
	if !slices.Equal(auto, wantAuto) {
		t.Errorf("automation start args = %q, want %q", auto, wantAuto)
	}

	if spec := androidReverseSpec(8085); spec != "tcp:8085" {
		t.Errorf("androidReverseSpec(8085) = %q, want tcp:8085", spec)
	}

	javaSrc, err := os.ReadFile(filepath.Join("..", "..", "internal", "platform", "android", "java", "dev", "oneo", "nsgo", "TestAgent.java"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(javaSrc), `AUTOMATION_EXTRA = "`+androidAutomationExtra+`"`) {
		t.Errorf("cmd/nsgo's androidAutomationExtra %q does not match TestAgent.java", androidAutomationExtra)
	}
}

// TestAndroidRuntimeIncludesAutomationExtra verifies classes.dex and classes.jar were rebuilt after adding automation support.
func TestAndroidRuntimeIncludesAutomationExtra(t *testing.T) {
	for _, needle := range []string{androidAutomationExtra, "setAutomation"} {
		if !bytes.Contains(androidruntime.ClassesDex, []byte(needle)) {
			t.Errorf("classes.dex does not contain %q; rebuild with internal/platform/android/runtime/build.sh", needle)
		}
	}
	zr, err := zip.NewReader(bytes.NewReader(androidruntime.ClassesJar), int64(len(androidruntime.ClassesJar)))
	if err != nil {
		t.Fatalf("classes.jar: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"dev/oneo/nsgo/TestAgent.class", "dev/oneo/nsgo/NsgoActivity.class", "dev/oneo/nsgo/NsgoView.class"} {
		if !names[want] {
			t.Errorf("classes.jar is missing %s", want)
		}
	}
}

// TestAndroidSessionAutomationOverADB drives androidSession.start and stop with a fake adb executable to verify adb reverse setup/teardown and the nsgo.automation launch intent extra.
func TestAndroidSessionAutomationOverADB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell stub for adb")
	}
	for _, tc := range []struct {
		name       string
		automation bool
		failOn     string
	}{
		{"automation enabled", true, ""},
		{"automation disabled", false, ""},
		{"install failure cleans up reverse", true, "install"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binDir := t.TempDir()
			logFile := filepath.Join(t.TempDir(), "adb.log")
			adbScript := filepath.Join(binDir, "adb")
			script := "#!/bin/sh\n" +
				"if [ \"$1\" = \"devices\" ]; then\n" +
				"  printf 'List of devices attached\\nEMU123\\tdevice\\n'\n" +
				"  exit 0\n" +
				"fi\n" +
				"printf '%s\\n' \"$*\" >> '" + logFile + "'\n" +
				"if [ \"$3\" = \"logcat\" ]; then\n" +
				"  exec sleep 30\n" +
				"fi\n"
			if tc.failOn != "" {
				script += "if [ \"$3\" = \"" + tc.failOn + "\" ]; then\n  echo 'simulated failure' >&2\n  exit 1\nfi\n"
			}
			script += "exit 0\n"
			if err := os.WriteFile(adbScript, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("ANDROID_SERIAL", "")

			dir := t.TempDir()
			p := &project{Config: projectcfg.Config{Dir: dir, Name: "Demo", ID: "dev.example.demo"}}
			opts := devOptions{
				automation: tc.automation,
				fps:        true,
				server:     &devServer{addr: &net.TCPAddr{IP: net.IPv4zero, Port: 9123}, token: "tok"},
			}
			s := androidSession(opts)
			running, err := s.start(context.Background(), p)
			if tc.failOn != "" {
				if err == nil {
					running.stop()
					t.Fatal("expected start to fail")
				}
			} else {
				if err != nil {
					t.Fatalf("start failed: %v", err)
				}
				running.stop()
			}
			raw, err := os.ReadFile(logFile)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			wantReverse := "-s EMU123 reverse tcp:9123 tcp:9123"
			wantRemove := "-s EMU123 reverse --remove tcp:9123"
			if !slices.Contains(lines, wantReverse) {
				t.Errorf("missing %q in adb calls:\n%s", wantReverse, raw)
			}
			if !slices.Contains(lines, wantRemove) {
				t.Errorf("missing %q in adb calls:\n%s", wantRemove, raw)
			}
			if tc.failOn != "" {
				return
			}
			wantStart := "-s EMU123 shell am start -S -n dev.example.demo/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true"
			if tc.automation {
				wantStart += " --ez nsgo.automation true"
			}
			if !slices.Contains(lines, wantStart) {
				t.Errorf("missing %q in adb calls:\n%s", wantStart, raw)
			}
			wantStop := "-s EMU123 shell am force-stop dev.example.demo"
			if !slices.Contains(lines, wantStop) {
				t.Errorf("missing %q in adb calls:\n%s", wantStop, raw)
			}
		})
	}
}

func TestBackgroundMotionAARIsExplicitAndNonExported(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		for _, motion := range []bool{false, true} {
			for _, dataSync := range []bool{false, true} {
				text := aarManifest(overlay, nil, motion, dataSync)
				if strings.Contains(text, apk.MotionService) != motion || strings.Contains(text, `android:foregroundServiceType="shortService"`) != motion {
					t.Fatalf("motion service opt-in lost: overlay=%v motion=%v dataSync=%v", overlay, motion, dataSync)
				}
				if strings.Contains(text, apk.DataSyncService) != dataSync || strings.Contains(text, `android:foregroundServiceType="dataSync"`) != dataSync {
					t.Fatalf("dataSync service opt-in lost: overlay=%v motion=%v dataSync=%v", overlay, motion, dataSync)
				}
				if strings.Contains(text, "FOREGROUND_SERVICE_DATA_SYNC") != dataSync {
					t.Fatalf("dataSync permission mismatch: dataSync=%v", dataSync)
				}
				count := strings.Count(text, `android:name="android.permission.FOREGROUND_SERVICE"`)
				want := 0
				if overlay || motion || dataSync {
					want = 1
				}
				if count != want {
					t.Fatalf("%d foreground permissions, want %d (overlay=%v motion=%v dataSync=%v)", count, want, overlay, motion, dataSync)
				}
				if !strings.Contains(text, `android:name="dev.oneo.nsgo.SensorRegionReceiver" android:exported="false"`) {
					t.Fatal("geofence receiver can be invoked by other apps")
				}
			}
		}
	}
}
