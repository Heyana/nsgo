package main

import (
	"archive/zip"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

func TestAndroidHostTracksPackagingInputs(t *testing.T) {
	dir := t.TempDir()
	p := &project{
		Config: projectcfg.Config{
			Dir: dir,
			Icon: icons{
				Android: &androidIcon{PNG: filepath.Join(dir, "assets/android-icon.png")},
			},
			LaunchScreen: &launchScreen{
				LaunchAppearance: launchAppearance{Image: filepath.Join(dir, "assets/splash.png")},
				Dark:             launchAppearance{Image: filepath.Join(dir, "assets/splash-dark.png")},
			},
		},
	}
	p.I18n.Dir = filepath.Join(dir, "locales")
	p.Android.Classes = []string{"native/views.dex"}
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(p.Dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("nsgo.yaml", "name: Demo\n")
	write("resources/nested/asset.txt", "first")
	write("native/views.dex", "first classes")
	fingerprint := androidHostInputs(p)
	write("main.go", "package main\n")
	write("locales/generated.go", "package locales\n")
	if got := androidHostInputs(p); got != fingerprint {
		t.Fatal("Go-only edit changed the APK inputs")
	}
	for _, change := range []struct{ path, contents string }{
		{"resources/nested/asset.txt", "second resource"},
		{"native/views.dex", "second classes"},
		{"assets/Icon-dev.png", "new launcher icon"},
		{"assets/android-icon.png", "android specific icon"},
		{"assets/splash.png", "light launch image"},
		{"assets/splash-dark.png", "dark launch image"},
		{"locales/en.yaml", "appName: Demo\n"},
		{"locales/en.yaml", "appName: App!\n"},
		{"nsgo.yaml", "name: App!\n"}, // same length as "name: Demo\n"
		{"nsgo.yaml", "name: Demo\nandroid:\n  permissions: [ACCESS_WIFI_STATE]\n"},
	} {
		write(change.path, change.contents)
		next := androidHostInputs(p)
		if next == fingerprint {
			t.Errorf("change to %s was not noticed", change.path)
		}
		fingerprint = next
	}
	if err := os.Remove(filepath.Join(p.Dir, "resources/nested/asset.txt")); err != nil {
		t.Fatal(err)
	}
	if androidHostInputs(p) == fingerprint {
		t.Fatal("removing a packaged resource was not noticed")
	}
}

// TestAndroidHostSessionFastReloadOverADB drives androidHostDev.build, start
// and close against a fake adb binary to verify that Go-only rebuilds skip APK
// repackaging, adb install and Activity restart while packaging input changes
// reinstall, and Activity exit recovers without reinstalling.
func TestAndroidHostSessionFastReloadOverADB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell stubs for adb and the host process")
	}
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	adbLog := filepath.Join(tmp, "adb.log")
	pidFile := filepath.Join(tmp, "activity.pid")
	if err := os.WriteFile(pidFile, []byte("4242\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adbScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"devices\" ]; then\n" +
		"  printf 'List of devices attached\\nEMU123\\tdevice\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s\\n' \"$*\" >> '" + adbLog + "'\n" +
		"if [ \"$3\" = \"forward\" ] && [ \"$4\" = \"tcp:0\" ]; then\n" +
		"  printf '45678\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$3\" = \"shell\" ] && [ \"$4\" = \"pidof\" ]; then\n" +
		"  cat '" + pidFile + "' 2>/dev/null || true\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "adb"), []byte(adbScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ANDROID_SERIAL", "")

	projDir := filepath.Join(tmp, "app")
	writeFiles(t, projDir, map[string]string{
		"nsgo.yaml":          "name: Demo\nid: dev.example.demo\n",
		"resources/data.txt": "v1",
	})
	p := &project{Config: projectcfg.Config{Dir: projDir, Name: "Demo", ID: "dev.example.demo", Version: "1.0.0"}}

	envLog := filepath.Join(tmp, "host-env.log")
	hostCalls := 0
	s := &androidHostDev{
		opts: devOptions{
			server: &devServer{addr: &net.TCPAddr{IP: net.IPv4zero, Port: 9123}, token: "devtok"},
		},
		endpoint: "nsgo-dev-0123456789abcdef0123456789abcdef",
		token:    "fedcba9876543210fedcba9876543210",
		compile: func(_ context.Context, _ *project, bin string) error {
			hostCalls++
			script := "#!/bin/sh\nprintf '%s %s\\n' \"$NSGO_ANDROID_HOST_ADDRESS\" \"$NSGO_ANDROID_HOST_TOKEN\" >> '" + envLog + "'\n"
			return os.WriteFile(bin, []byte(script), 0o755)
		},
	}
	defer s.close()

	ctx := t.Context()

	// 1. First build packages a debuggable host APK with __nsgo_android_host and installs it.
	if err := s.build(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s.apk == "" {
		t.Fatal("first build did not package the runtime APK")
	}
	zr, err := zip.OpenReader(s.apk)
	if err != nil {
		t.Fatal(err)
	}
	hasHostConfig := false
	for _, f := range zr.File {
		if f.Name == "assets/__nsgo_android_host" {
			hasHostConfig = true
		}
	}
	zr.Close()
	if !hasHostConfig {
		t.Fatal("host APK is missing assets/__nsgo_android_host")
	}
	run1, err := s.start(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := run1.wait(ctx); err != nil {
		t.Fatal(err)
	}
	run1.stop()

	// 2. Go-only rebuild skips APK packaging, adb install, and am start.
	writeFiles(t, projDir, map[string]string{"main.go": "package main\n// changed\n"})
	if err := s.build(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s.apk != "" {
		t.Fatalf("Go-only rebuild packaged an APK: %s", s.apk)
	}
	run2, err := s.start(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := run2.wait(ctx); err != nil {
		t.Fatal(err)
	}
	run2.stop()

	// 3. If the Activity exited on the device, Go-only rebuild relaunches it without reinstalling.
	if err := os.WriteFile(pidFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.build(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s.apk != "" {
		t.Fatalf("Go-only rebuild after Activity exit packaged an APK: %s", s.apk)
	}
	run3, err := s.start(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := run3.wait(ctx); err != nil {
		t.Fatal(err)
	}
	run3.stop()
	if err := os.WriteFile(pidFile, []byte("4243\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Editing a packaged resource rebuilds and reinstalls the APK.
	writeFiles(t, projDir, map[string]string{"resources/data.txt": "v2-updated"})
	if err := s.build(ctx, p); err != nil {
		t.Fatal(err)
	}
	if s.apk == "" {
		t.Fatal("resource edit did not rebuild the APK")
	}
	run4, err := s.start(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := run4.wait(ctx); err != nil {
		t.Fatal(err)
	}
	run4.stop()
	s.close()

	raw, err := os.ReadFile(adbLog)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	count := func(sub string) int {
		n := 0
		for _, line := range lines {
			if strings.Contains(line, sub) {
				n++
			}
		}
		return n
	}
	if got := count(" install -r "); got != 2 {
		t.Errorf("adb install count = %d, want 2 (initial + resource change)\nlog:\n%s", got, raw)
	}
	if got := count(" shell am start "); got != 3 {
		t.Errorf("am start count = %d, want 3 (initial + Activity recovery + resource change)\nlog:\n%s", got, raw)
	}
	if got := count(" forward tcp:0 localabstract:" + s.endpoint); got != 2 {
		t.Errorf("adb forward count = %d, want 2\nlog:\n%s", got, raw)
	}
	if got := count(" forward --remove tcp:45678"); got != 2 {
		t.Errorf("adb forward --remove count = %d, want 2\nlog:\n%s", got, raw)
	}
	if got := count(" shell am force-stop dev.example.demo"); got != 2 {
		t.Errorf("am force-stop count = %d, want 2\nlog:\n%s", got, raw)
	}
	if hostCalls != 4 {
		t.Errorf("host compile count = %d, want 4", hostCalls)
	}

	envRaw, err := os.ReadFile(envLog)
	if err != nil {
		t.Fatal(err)
	}
	wantEnv := "127.0.0.1:45678 " + s.token
	envLines := strings.Split(strings.TrimSpace(string(envRaw)), "\n")
	if !slices.Contains(envLines, wantEnv) {
		t.Errorf("host process env log = %q, want %q", envLines, wantEnv)
	}
}
