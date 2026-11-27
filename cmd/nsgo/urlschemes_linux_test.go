package main

import (
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLinuxURLSchemesNative installs the generated desktop entry in a private XDG data directory, then lets GIO find the app for its scheme and open a link with it.
func TestLinuxURLSchemesNative(t *testing.T) {
	for _, name := range []string{"gio", "update-desktop-database"} {
		if _, err := exec.LookPath(name); err != nil {
			if os.Getenv("NSGO_FILE_ASSOCIATIONS") != "" {
				t.Fatal(err)
			}
			t.Skipf("native desktop registration needs %s", name)
		}
	}
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "NSGo Links", ID: "dev.nsgo.links", Version: "1.0.0", URLSchemes: []string{"nsgotestlinks"}}}
	writeFiles(t, dir, map[string]string{p.Name: "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NSGO_ASSOC_OUTPUT\"\n"})
	if err := os.Chmod(filepath.Join(dir, p.Name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLinuxIcon(p, dir); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(dir, p.ID+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	dataHome := filepath.Join(dir, "data")
	apps := filepath.Join(dataHome, "applications")
	writeFiles(t, apps, map[string]string{p.ID + ".desktop": string(entry)})
	result := filepath.Join(dir, "opened")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("NSGO_ASSOC_OUTPUT", result)
	command := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(t.Context(), name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return string(out)
	}
	command("update-desktop-database", apps)
	if out := command("gio", "mime", "x-scheme-handler/nsgotestlinks"); !strings.Contains(out, p.ID+".desktop") {
		t.Fatalf("GIO did not find the app for its scheme: %s", out)
	}
	link := "nsgotestlinks://entry/word?q=a%20b"
	command("gio", "open", link)
	deadline := time.Now().Add(10 * time.Second)
	for {
		opened, _ := os.ReadFile(result)
		if string(opened) == link+"\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("GIO's link arguments = %q", opened)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := os.Remove(filepath.Join(apps, p.ID+".desktop")); err != nil {
		t.Fatal(err)
	}
	command("update-desktop-database", apps)
	if out := command("gio", "mime", "x-scheme-handler/nsgotestlinks"); strings.Contains(out, p.ID+".desktop") {
		t.Fatalf("GIO kept the uninstalled app: %s", out)
	}
}
