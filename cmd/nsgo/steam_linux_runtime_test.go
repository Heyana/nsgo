package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSteamLinuxDepotLaunch(t *testing.T) {
	p, dir := &project{Name: "Cave Run's Probe"}, t.TempDir()
	if err := addSteamLinuxRuntime(p, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lib/libgtk-4.so.1", "lib/libglib-2.0.so.0", "share/glib-2.0/schemas/gschemas.compiled"} {
		info, err := os.Stat(filepath.Join(dir, steamLinuxRuntimeDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Errorf("missing runtime file %s: %v", name, err)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	const app = "#!/bin/sh\nprintf '%s\\n' \"$LD_LIBRARY_PATH\" \"$XDG_DATA_DIRS\" \"$1\" \"$2\"\n"
	if err := os.WriteFile(filepath.Join(dir, p.Name), []byte(app), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LD_LIBRARY_PATH", "/session/lib")
	t.Setenv("XDG_DATA_DIRS", "/session/share")
	cmd := exec.CommandContext(t.Context(), filepath.Join(dir, steamLinuxLauncher), "two words", "$literal")
	got, err := cmd.CombinedOutput()
	// The launcher resolves the depot's physical path (including /var's
	// symlink on macOS).
	resolved, resolveErr := filepath.EvalSymlinks(dir)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	want := []string{filepath.Join(resolved, steamLinuxRuntimeDir, "lib") + ":/session/lib",
		filepath.Join(resolved, steamLinuxRuntimeDir, "share") + ":/session/share", "two words", "$literal", ""}
	if err != nil || string(got) != strings.Join(want, "\n") {
		t.Fatalf("launcher output %q (%v), want %q", got, err, strings.Join(want, "\n"))
	}
}
