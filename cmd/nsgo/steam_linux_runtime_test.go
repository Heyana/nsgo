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
	for _, name := range []string{"lib/libgtk-4.so.1", "lib/libglib-2.0.so.0", "share/glib-2.0/schemas/gschemas.compiled", "licenses/NOTICE", "licenses/build.sh", "licenses/sources.lock"} {
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
	// The launcher resolves the depot's physical path (including /var's symlink on macOS).
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	bundled := []string{filepath.Join(resolved, steamLinuxRuntimeDir, "lib") + ":/session/lib", filepath.Join(resolved, steamLinuxRuntimeDir, "share") + ":/session/share"}
	system := []string{"/session/lib", "/session/share"}
	// A fake ldconfig stands for the system's library cache, which lists a GTK whose real name carries its version.
	sys := t.TempDir()
	for _, tc := range []struct {
		name, gtk string
		want      []string
	}{
		{"no system GTK", "", bundled},
		{"GTK 4.6", "libgtk-4.so.1.600.9", bundled},
		{"GTK 4.14", "libgtk-4.so.1.1400.2", system},
		{"GTK 4.22", "libgtk-4.so.1.2204.4", system},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			cache := "\t12 libs found in cache `/etc/ld.so.cache'\n"
			if tc.gtk != "" {
				real := filepath.Join(sys, tc.gtk)
				link := filepath.Join(sys, tc.name, "libgtk-4.so.1")
				if err := os.WriteFile(real, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, link); err != nil {
					t.Fatal(err)
				}
				cache += "\tlibgtk-4.so.1 (libc6) => /usr/lib/i386-linux-gnu/libgtk-4.so.1\n\tlibgtk-4.so.1 (libc6,x86-64) => " + link + "\n"
			}
			ldconfig := "#!/bin/sh\n[ \"$1\" = -p ] && printf '%s' " + shQuote(cache) + "\n"
			if err := os.WriteFile(filepath.Join(bin, "ldconfig"), []byte(ldconfig), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("LD_LIBRARY_PATH", "/session/lib")
			t.Setenv("XDG_DATA_DIRS", "/session/share")
			got, err := exec.CommandContext(t.Context(), filepath.Join(dir, steamLinuxLauncher), "two words", "$literal").CombinedOutput()
			want := strings.Join(append(tc.want, "two words", "$literal", ""), "\n")
			if err != nil || string(got) != want {
				t.Fatalf("launcher output %q (%v), want %q", got, err, want)
			}
		})
	}
}
