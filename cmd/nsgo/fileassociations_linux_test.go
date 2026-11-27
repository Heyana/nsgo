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

// TestLinuxFileAssociationsNative registers the generated files in a private XDG data directory, then lets GIO discover and launch the app.
func TestLinuxFileAssociationsNative(t *testing.T) {
	for _, name := range []string{"gio", "update-mime-database", "update-desktop-database"} {
		if _, err := exec.LookPath(name); err != nil {
			if os.Getenv("NSGO_FILE_ASSOCIATIONS") != "" {
				t.Fatal(err)
			}
			t.Skipf("native desktop registration needs %s", name)
		}
	}
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "NSGo $100% Documents", ID: "dev.nsgo.associations", Version: "1.0.0"}}
	p.FileTypes = []fileType{{Name: "NSGo Test Document", ID: p.ID + ".document", Extensions: []string{"nsgotestdoc"}}}
	writeFiles(t, dir, map[string]string{p.Name: "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NSGO_ASSOC_OUTPUT\"\n"})
	if err := os.Chmod(filepath.Join(dir, p.Name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLinuxIcon(p, dir); err != nil {
		t.Fatal(err)
	}
	dataHome := filepath.Join(dir, "data")
	apps, mime := filepath.Join(dataHome, "applications"), filepath.Join(dataHome, "mime")
	entry, err := os.ReadFile(filepath.Join(dir, p.ID+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := os.ReadFile(filepath.Join(dir, p.ID+".mime.xml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, apps, map[string]string{p.ID + ".desktop": string(entry)})
	writeFiles(t, filepath.Join(mime, "packages"), map[string]string{p.ID + ".xml": string(definitions)})
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
	command("update-mime-database", mime)
	command("update-desktop-database", apps)
	media := "application/x-" + p.FileTypes[0].ID
	if out := command("gio", "mime", media); !strings.Contains(out, p.ID+".desktop") {
		t.Fatalf("GIO did not discover the installed app: %s", out)
	}
	first, second := filepath.Join(dir, "first document.nsgotestdoc"), filepath.Join(dir, "日本語.nsgotestdoc")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out := command("gio", "info", "-a", "standard::content-type", first); !strings.Contains(out, media) {
		t.Fatalf("GIO did not recognize the owned extension: %s", out)
	}
	command("gio", "launch", filepath.Join(apps, p.ID+".desktop"), first, second)
	deadline := time.Now().Add(10 * time.Second)
	for {
		opened, _ := os.ReadFile(result)
		if string(opened) == first+"\n"+second+"\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("GIO's file arguments = %q", opened)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, path := range []string{filepath.Join(apps, p.ID+".desktop"), filepath.Join(mime, "packages", p.ID+".xml")} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	command("update-mime-database", mime)
	command("update-desktop-database", apps)
	if out := command("gio", "mime", media); strings.Contains(out, p.ID+".desktop") {
		t.Fatalf("GIO kept the uninstalled app: %s", out)
	}
}
