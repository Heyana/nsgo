//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

func TestLoginEntryRoundTrip(t *testing.T) {
	entry := loginEntry{path: filepath.Join(t.TempDir(), "autostart", "app.desktop"), name: "A demo", executable: "/opt/A demo/it\\s 100%"}
	if entry.enabled() {
		t.Fatal("missing entry is enabled")
	}
	if err := entry.set(true); err != nil {
		t.Fatal(err)
	}
	if !entry.enabled() {
		t.Fatal("written entry is not enabled")
	}
	data, err := os.ReadFile(entry.path)
	if err != nil {
		t.Fatal(err)
	}
	if want := `Exec="/opt/A demo/it\\\\s 100%%" --nsgo-launched-at-login` + "\n"; !strings.Contains(string(data), want) {
		t.Fatalf("entry = %q, want a line %q", data, want)
	}
	if info, err := os.Stat(entry.path); err != nil || info.Mode().Perm()&0o022 != 0 {
		t.Fatalf("entry mode = %v, %v", info, err)
	}
	moved := entry
	moved.executable = "/opt/other/app"
	if moved.enabled() {
		t.Fatal("an entry for another executable is enabled")
	}
	if err := entry.set(false); err != nil {
		t.Fatal(err)
	}
	if entry.enabled() {
		t.Fatal("removed entry is enabled")
	}
	if err := entry.set(false); err != nil {
		t.Fatal("removing a missing entry: ", err)
	}
}

// TestLoginEntryExecReadsAsArguments has GLib read the entry's Exec as the session does (GDesktopAppInfo: the key file's string, then g_shell_parse_argv, then field codes), so a path with spaces, quotes, dollars or backslashes still starts the app with the marker. A percent sign reads back too, but appLoginEntry refuses it, as GDesktopAppInfo looks the program up before it expands %%.
func TestLoginEntryExecReadsAsArguments(t *testing.T) {
	lib, err := purego.Dlopen("libglib-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip("GLib: ", err)
	}
	var newKeyFile func() uintptr
	var loadFromData func(kf uintptr, data string, length uintptr, flags int32, gerr *uintptr) bool
	var getString func(kf uintptr, group, key string, gerr *uintptr) uintptr
	var parseArgv func(command uintptr, argc *int32, argv *uintptr, gerr *uintptr) bool
	var freeKeyFile func(kf uintptr)
	var free func(p uintptr)
	var strfreev func(p uintptr)
	purego.RegisterLibFunc(&newKeyFile, lib, "g_key_file_new")
	purego.RegisterLibFunc(&loadFromData, lib, "g_key_file_load_from_data")
	purego.RegisterLibFunc(&getString, lib, "g_key_file_get_string")
	purego.RegisterLibFunc(&parseArgv, lib, "g_shell_parse_argv")
	purego.RegisterLibFunc(&freeKeyFile, lib, "g_key_file_free")
	purego.RegisterLibFunc(&free, lib, "g_free")
	purego.RegisterLibFunc(&strfreev, lib, "g_strfreev")
	for _, executable := range []string{
		"/usr/bin/app",
		"/opt/A demo/app",
		`/home/u/it's "$HOME" \n` + "`id`" + ` 100%u; (a|b) & <c> ~#*?/app`,
		"/tmp/tab\there\\/new\nline/app",
	} {
		e := loginEntry{path: "unused", name: "App", executable: executable}
		kf := newKeyFile()
		var gerr uintptr
		data := string(e.data())
		if !loadFromData(kf, data, uintptr(len(data)), 0, &gerr) {
			t.Fatalf("GLib refused the entry %q", data)
		}
		command := getString(kf, "Desktop Entry", "Exec", &gerr)
		freeKeyFile(kf)
		if command == 0 {
			t.Fatalf("GLib read no Exec in %q", data)
		}
		var argc int32
		var argv uintptr
		ok := parseArgv(command, &argc, &argv, &gerr)
		free(command)
		if !ok {
			t.Fatalf("GLib could not split the Exec of %q", data)
		}
		var args []string
		for i := range int(argc) {
			// GDesktopAppInfo expands field codes in each argument after splitting; %% is a percent.
			args = append(args, strings.ReplaceAll(goString(*(*uintptr)(unsafe.Add(cPointer(argv), i*int(unsafe.Sizeof(uintptr(0)))))), "%%", "%"))
		}
		strfreev(argv)
		if want := []string{executable, "--nsgo-launched-at-login"}; !slices.Equal(args, want) {
			t.Errorf("Exec of %q reads as %q, want %q", data, args, want)
		}
	}
}

func TestAppLoginEntryUsesXDGConfig(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	e, err := appLoginEntry("dev.nsgo.test", "Test app", "/tmp/app")
	if err != nil || e.path != filepath.Join(config, "autostart", "dev.nsgo.test.desktop") {
		t.Fatalf("entry = %+v, %v", e, err)
	}
	if e, err := appLoginEntry("", "", "/opt/demo/demo-app"); err != nil || filepath.Base(e.path) != "demo-app.desktop" || e.name != "demo-app" {
		t.Fatalf("entry without an ID = %+v, %v", e, err)
	}
	if _, err := appLoginEntry("bad/name", "Test app", "/tmp/app"); err == nil {
		t.Fatal("path separator accepted in autostart ID")
	}
	if _, err := appLoginEntry("dev.nsgo.test", "Test app", "/opt/100% demo/app"); err == nil {
		t.Fatal("an executable with a percent sign accepted, which GNOME cannot start")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	if e, err := appLoginEntry("dev.nsgo.test", "", "/tmp/app"); err != nil || e.path != filepath.Join(home, ".config", "autostart", "dev.nsgo.test.desktop") {
		t.Fatalf("entry with a relative XDG_CONFIG_HOME = %+v, %v", e, err)
	}
}

func TestLoginEntryStartsTheAppImage(t *testing.T) {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		t.Skip("runs in a Flatpak")
	}
	t.Setenv("SNAP", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPIMAGE", "/home/u/Apps/Demo-x86_64.AppImage")
	t.Setenv("APPDIR", "/home/u/.mount_other/AppDir")
	exe, _ := os.Executable()
	if e, err := currentLoginEntry("dev.nsgo.demo", "Demo"); err != nil || e.executable != exe {
		t.Fatalf("inherited AppImage entry = %+v, %v, want executable %q", e, err, exe)
	}
	t.Setenv("APPDIR", filepath.Dir(exe))
	if e, err := currentLoginEntry("dev.nsgo.demo", "Demo"); err != nil || e.executable != "/home/u/Apps/Demo-x86_64.AppImage" {
		t.Fatalf("AppImage entry = %+v, %v", e, err)
	}
	t.Setenv("APPIMAGE", "")

}
