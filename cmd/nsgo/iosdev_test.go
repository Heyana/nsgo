package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestIOSSimulatorDevReplacesOnlyTheExecutable(t *testing.T) {
	p := &project{Name: "Dev", ID: "dev.nsgo.iosdev"}
	dir := t.TempDir()
	bundle := filepath.Join(dir, "build", "Dev.app")
	installed := filepath.Join(dir, "installed", "Dev.app")
	commands := filepath.Join(dir, "commands")
	t.Setenv("NSGO_SIMULATOR_APP", installed)
	t.Setenv("NSGO_SIMULATOR_COMMANDS", commands)
	checkXcrun(t, `printf '%s\n' "$2" >> "$NSGO_SIMULATOR_COMMANDS"
case "$2" in
install) mkdir -p "$NSGO_SIMULATOR_APP" && cp -R "$4/." "$NSGO_SIMULATOR_APP" ;;
get_app_container) test -d "$NSGO_SIMULATOR_APP" && printf '%s\n' "$NSGO_SIMULATOR_APP" ;;
*) exit 1 ;;
esac`)
	writeFiles(t, bundle, map[string]string{
		"Info.plist":                   "initial plist",
		"_CodeSignature/CodeResources": "initial seal",
		"Dev":                          "initial executable",
	})
	ctx := t.Context()
	var session iosSimulatorInstall
	update := func(udid, inputs string) {
		t.Helper()
		if err := session.update(ctx, p, udid, bundle, inputs); err != nil {
			t.Fatal(err)
		}
	}
	update("simulator", "initial inputs")
	initial, err := os.Stat(filepath.Join(installed, "Dev"))
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, bundle, map[string]string{"Dev": "rebuilt executable"})
	update("simulator", "initial inputs")
	updated, err := os.Stat(filepath.Join(installed, "Dev"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(initial, updated) {
		t.Fatal("reload overwrote the original inode")
	}
	if bin, err := os.ReadFile(filepath.Join(installed, "Dev")); err != nil || string(bin) != "rebuilt executable" {
		t.Fatalf("installed executable = %q, %v", bin, err)
	}
	writeFiles(t, bundle, map[string]string{"_CodeSignature/CodeResources": "new resource seal"})
	update("simulator", "initial inputs")
	writeFiles(t, bundle, map[string]string{"Info.plist": "new plist"})
	update("simulator", "initial inputs")
	update("simulator", "edited nsgo.yaml")
	if err := os.Rename(installed, filepath.Join(dir, "uninstalled")); err != nil {
		t.Fatal(err)
	}
	update("simulator", "edited nsgo.yaml")
	update("another simulator", "edited nsgo.yaml")

	log, err := os.ReadFile(commands)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "get_app_container", "get_app_container", "install",
		"get_app_container", "install", "install", "get_app_container", "install", "install"}
	if got := strings.Fields(string(log)); !slices.Equal(got, want) {
		t.Errorf("simctl commands = %v, want %v", got, want)
	}
}

func TestIOSSimulatorDevReusesStagedBundle(t *testing.T) {
	p := &project{Name: "Dev", ID: "dev.nsgo.iosdev"}
	dir := t.TempDir()
	stageDir := filepath.Join(dir, "stage")
	bundle := filepath.Join(stageDir, "Dev.app")
	installed := filepath.Join(dir, "installed", "Dev.app")
	commands := filepath.Join(dir, "commands")
	t.Setenv("NSGO_SIMULATOR_APP", installed)
	t.Setenv("NSGO_SIMULATOR_COMMANDS", commands)
	checkXcrun(t, `printf '%s\n' "$2" >> "$NSGO_SIMULATOR_COMMANDS"
case "$2" in
install) rm -rf "$NSGO_SIMULATOR_APP" && mkdir -p "$NSGO_SIMULATOR_APP" && cp -R "$4/." "$NSGO_SIMULATOR_APP" ;;
get_app_container) test -d "$NSGO_SIMULATOR_APP" && printf '%s\n' "$NSGO_SIMULATOR_APP" ;;
*) exit 1 ;;
esac`)

	const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>Dev</string>
<key>CFBundleIdentifier</key><string>dev.nsgo.iosdev</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>
`
	readBin := func(name, fallback string) []byte {
		if runtime.GOOS == "darwin" {
			data, err := os.ReadFile(filepath.Join("/usr/bin", name))
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		return []byte(fallback)
	}
	firstBin := readBin("false", "first-bin")
	secondBin := readBin("true", "second-bin")

	var fullCalls, binCalls, signCalls int
	var binErr error
	nextBin := secondBin
	nextFrameworks := iosSimulatorFrameworkInputs(p, false)

	signBundle := func(ctx context.Context, _ *project, app string) error {
		signCalls++
		if runtime.GOOS == "darwin" {
			return run(ctx, "codesign", "--force", "--sign", "-", app)
		}
		writeFiles(t, app, map[string]string{"_CodeSignature/CodeResources": "signed-seal"})
		return nil
	}

	session := iosSimulatorInstall{
		buildFull: func(ctx context.Context, p *project, outDir string) (string, error) {
			fullCalls++
			app := filepath.Join(outDir, p.Name+".app")
			writeFiles(t, app, map[string]string{"Info.plist": plist, "asset.txt": "asset"})
			if err := os.WriteFile(filepath.Join(app, p.Name), firstBin, 0o755); err != nil {
				return "", err
			}
			if err := signBundle(ctx, p, app); err != nil {
				return "", err
			}
			return app, nil
		},
		buildBin: func(_ context.Context, _ *project, out string) (string, error) {
			binCalls++
			if binErr != nil {
				return "", binErr
			}
			if err := os.WriteFile(out, nextBin, 0o755); err != nil {
				return "", err
			}
			return nextFrameworks, nil
		},
		sign: signBundle,
	}

	ctx := t.Context()
	// 1. Initial build packages the full bundle and installs it.
	if err := session.build(ctx, p, stageDir, "inputs-v1"); err != nil {
		t.Fatal(err)
	}
	if err := session.update(ctx, p, "sim-1", bundle, "inputs-v1"); err != nil {
		t.Fatal(err)
	}
	if fullCalls != 1 || binCalls != 0 {
		t.Fatalf("after initial build: fullCalls=%d binCalls=%d, want 1 and 0", fullCalls, binCalls)
	}
	stagedPlist, err := os.Stat(filepath.Join(bundle, "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}

	// 2. Go-only rebuild reuses the staged bundle and skips simctl install.
	if err := session.build(ctx, p, stageDir, "inputs-v1"); err != nil {
		t.Fatal(err)
	}
	if err := session.update(ctx, p, "sim-1", bundle, "inputs-v1"); err != nil {
		t.Fatal(err)
	}
	if fullCalls != 1 || binCalls != 1 {
		t.Fatalf("after Go-only rebuild: fullCalls=%d binCalls=%d, want 1 and 1", fullCalls, binCalls)
	}
	reusedPlist, err := os.Stat(filepath.Join(bundle, "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(stagedPlist, reusedPlist) {
		t.Fatal("Go-only rebuild repackaged Info.plist instead of reusing the staged bundle")
	}
	if runtime.GOOS == "darwin" {
		if err := run(ctx, "codesign", "--verify", "--strict", installed); err != nil {
			t.Fatalf("installed bundle failed codesign --verify after staged reuse: %v", err)
		}
		if err := run(ctx, filepath.Join(installed, p.Name)); err != nil {
			t.Fatalf("replaced executable did not run /usr/bin/true: %v", err)
		}
	}

	// 3. A failed Go compile keeps the staged bundle intact so the next fix still reuses it.
	binErr = errors.New("compile error")
	if err := session.build(ctx, p, stageDir, "inputs-v1"); err == nil {
		t.Fatal("expected compile error")
	}
	binErr = nil
	if err := session.build(ctx, p, stageDir, "inputs-v1"); err != nil {
		t.Fatal(err)
	}
	if fullCalls != 1 || binCalls != 3 {
		t.Fatalf("after compile recovery: fullCalls=%d binCalls=%d, want 1 and 3", fullCalls, binCalls)
	}

	// 4. Adding a Go-linked native framework falls back to a full bundle build.
	nextFrameworks = iosSimulatorFrameworkInputs(p, true)
	if err := session.build(ctx, p, stageDir, "inputs-v1"); err != nil {
		t.Fatal(err)
	}
	if fullCalls != 2 || binCalls != 4 {
		t.Fatalf("after framework change: fullCalls=%d binCalls=%d, want 2 and 4", fullCalls, binCalls)
	}

	// 5. Changing packaging inputs triggers a full bundle build and simctl install.
	if err := session.build(ctx, p, stageDir, "inputs-v2"); err != nil {
		t.Fatal(err)
	}
	if err := session.update(ctx, p, "sim-1", bundle, "inputs-v2"); err != nil {
		t.Fatal(err)
	}
	if fullCalls != 3 {
		t.Fatalf("after input change: fullCalls=%d, want 3", fullCalls)
	}

	log, err := os.ReadFile(commands)
	if err != nil {
		t.Fatal(err)
	}
	wantCmds := []string{"install", "get_app_container", "install"}
	if got := strings.Fields(string(log)); !slices.Equal(got, wantCmds) {
		t.Errorf("simctl commands = %v, want %v", got, wantCmds)
	}
}

func TestIOSSimulatorDevRetainsTheBundleSignature(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs codesign")
	}
	p := &project{Name: "Dev", ID: "dev.nsgo.iosdev"}
	dir := t.TempDir()
	bundle, installed := filepath.Join(dir, "build", "Dev.app"), filepath.Join(dir, "installed", "Dev.app")
	const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>Dev</string>
<key>CFBundleIdentifier</key><string>dev.nsgo.iosdev</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>
`
	for _, app := range []struct{ folder, bin string }{{installed, "false"}, {bundle, "true"}} {
		program, err := os.ReadFile(filepath.Join("/usr/bin", app.bin))
		if err != nil {
			t.Fatal(err)
		}
		writeFiles(t, app.folder, map[string]string{"Info.plist": plist, "asset.txt": "asset"})
		if err := os.WriteFile(filepath.Join(app.folder, p.Name), program, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := run(t.Context(), "codesign", "--force", "--sign", "-", app.folder); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSGO_SIMULATOR_APP", installed)
	checkXcrun(t, `test "$2" = get_app_container && printf '%s\n' "$NSGO_SIMULATOR_APP"`)
	replaced, err := replaceSimulatorExecutable(t.Context(), p, "simulator", bundle)
	if err != nil || !replaced {
		t.Fatalf("replacing a signed executable = %v, %v", replaced, err)
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", installed); err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), filepath.Join(installed, p.Name)); err != nil {
		t.Fatal(err)
	}
}

func TestIOSSimulatorDevWatchesPackageInputs(t *testing.T) {
	dir := t.TempDir()
	p := &project{
		Dir: dir,
		LaunchScreen: &launchScreen{
			launchAppearance: launchAppearance{Image: filepath.Join(dir, "assets/splash.png")},
			Dark:             launchAppearance{Image: filepath.Join(dir, "assets/splash-dark.png")},
		},
	}
	p.I18n.Dir = filepath.Join(dir, "locales")
	if iosSession(iOS, devOptions{}).watch == nil || iosSession(tvOS, devOptions{}).watch != nil ||
		iosSession(iOS, devOptions{clip: "https://example.invalid/clip"}).watch != nil {
		t.Fatal("the bundle watcher should follow iOS simulator apps")
	}
	last := iosSimulatorInputs(p)
	for _, update := range []struct{ name, contents string }{
		{"main.go", "package main"},
		{"locales/generated.go", "package locales"},
		{"resources/nested/file.txt", "asset"},
		{"resources/nested/file.txt", "updated asset"},
		{"assets/AppIcon-dev.icon/icon.json", "icon"},
		{"assets/splash.png", "light splash"},
		{"assets/splash-dark.png", "dark splash"},
		{"locales/en.yaml", "appName: Demo\n"},
		{"locales/en.yaml", "appName: App!\n"},
		{"nsgo.yaml", "name: Dev"},
		{"nsgo.yaml", "name: App"},
	} {
		writeFiles(t, p.Dir, map[string]string{update.name: update.contents})
		now := iosSimulatorInputs(p)
		wantChange := !strings.HasSuffix(update.name, ".go")
		if (last != now) != wantChange {
			t.Errorf("editing %s changed the package inputs = %v, want %v", update.name, last != now, wantChange)
		}
		last = now
	}
}
