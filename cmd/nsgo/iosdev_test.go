package main

import (
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
	p := &project{Dir: t.TempDir()}
	if iosSession(iOS, devOptions{}).watch == nil || iosSession(tvOS, devOptions{}).watch != nil ||
		iosSession(iOS, devOptions{clip: "https://example.invalid/clip"}).watch != nil {
		t.Fatal("the bundle watcher should follow iOS simulator apps")
	}
	last := iosSimulatorInputs(p)
	for _, update := range []struct{ name, contents string }{
		{"main.go", "package main"},
		{"resources/nested/file.txt", "asset"},
		{"resources/nested/file.txt", "updated asset"},
		{"assets/AppIcon-dev.icon/icon.json", "icon"},
		{"nsgo.yaml", "name: Dev"},
		{"nsgo.yaml", "name: App"},
	} {
		writeFiles(t, p.Dir, map[string]string{update.name: update.contents})
		now := iosSimulatorInputs(p)
		wantChange := update.name != "main.go"
		if (last != now) != wantChange {
			t.Errorf("editing %s changed the package inputs = %v, want %v", update.name, last != now, wantChange)
		}
		last = now
	}
}
