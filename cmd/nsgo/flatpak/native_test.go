package flatpak

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in external-reader acceptance. Each installation and repository belongs to the test; the host's user/system Flatpak installations are never changed.
func TestNativeFlatpakInstallsAndRejectsCorruptPayload(t *testing.T) {
	tool := os.Getenv("NSGO_FLATPAK")
	if tool == "" {
		t.Skip("set NSGO_FLATPAK to the Flatpak executable")
	}
	b, entries := fixtureBundle()
	data, err := Write(b, entries)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "fixture.flatpak")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(installation string, args ...string) ([]byte, error) {
		cmd := exec.Command(tool, args...)
		cmd.Env = append(os.Environ(), "FLATPAK_USER_DIR="+filepath.Join(root, installation), "XDG_DATA_HOME="+filepath.Join(root, installation+"-data"), "XDG_CONFIG_HOME="+filepath.Join(root, installation+"-config"))
		return cmd.CombinedOutput()
	}
	if out, err := run("valid", "install", "--user", "--bundle", "--no-deps", "--noninteractive", file); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	out, err := run("valid", "info", "--user", "--show-location", b.ID)
	if err != nil {
		t.Fatalf("location: %v\n%s", err, out)
	}
	location := strings.TrimSpace(string(out))
	exe := filepath.Join(location, "files/lib/fixture/fixture")
	got, err := os.ReadFile(exe)
	if err != nil || !bytes.Equal(got, entries[1].Data) {
		t.Fatalf("payload: %q, %v", got, err)
	}
	info, err := os.Stat(exe)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("executable mode: %v %v", info, err)
	}
	link, err := os.Readlink(filepath.Join(location, "files/bin/fixture"))
	if err != nil || link != "../lib/fixture/fixture" {
		t.Fatalf("link: %q, %v", link, err)
	}
	if got, err := os.ReadFile(filepath.Join(location, "files/lib/fixture/empty")); err != nil || len(got) != 0 {
		t.Fatalf("empty file: %q %v", got, err)
	}
	if out, err := run("valid", "uninstall", "--user", "--noninteractive", b.ID); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	if _, err := os.Stat(location); !os.IsNotExist(err) {
		t.Fatalf("deployment remains after uninstall: %v", err)
	}
	i := bytes.Index(data, []byte("NSGo Flatpak fixture"))
	if i < 0 {
		t.Fatal("missing mutation target")
	}
	data[i] ^= 1
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("corrupt", "install", "--user", "--bundle", "--no-deps", "--noninteractive", file); err == nil {
		t.Fatalf("corrupt bundle installed:\n%s", out)
	}
}
