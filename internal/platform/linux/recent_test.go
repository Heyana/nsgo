//go:build linux && !android

package linux

import (
	"bytes"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
)

// TestRecentFilesPreserveOtherApplications runs with fresh GLib XDG state: it
// must never inspect or change the developer's actual recently-used.xbel.
func TestRecentFilesPreserveOtherApplications(t *testing.T) {
	if testing.Short() {
		t.Skip("needs GTK")
	}
	if os.Getenv("NSGO_TEST_RECENT_FILES") == "" {
		dir := t.TempDir()
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestRecentFilesPreserveOtherApplications$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_RECENT_FILES=1", "XDG_DATA_HOME="+dir, "XDG_CONFIG_HOME="+filepath.Join(dir, "config"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated recent files: %v\n%s", err, out)
		}
		if !bytes.Contains(out, []byte("recent-native-ok")) {
			t.Skipf("native helper unavailable: %s", out)
		}
		return
	}
	ready := make(chan *core.Scope, 1)
	w := startTestWindow(t, core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		ready <- cx
	}), Options{Title: "nsgo-recent", ID: "dev.nsgo.recenttest", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	cx := <-ready
	dir := os.Getenv("XDG_DATA_HOME")
	filename := filepath.Join(dir, "recently-used.xbel")
	shared, own, other := filepath.Join(dir, "shared # 文档.txt"), filepath.Join(dir, "own.txt"), filepath.Join(dir, "other.txt")
	for _, path := range []string{shared, own, other} {
		if err := os.WriteFile(path, []byte("document"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	type registration struct {
		Name  string `xml:"name,attr"`
		Exec  string `xml:"exec,attr"`
		Count int    `xml:"count,attr"`
	}
	type bookmark struct {
		URI          string         `xml:"href,attr"`
		Applications []registration `xml:"info>metadata>applications>application"`
	}
	read := func() map[string][]registration {
		t.Helper()
		data, err := os.ReadFile(filename)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Bookmarks []bookmark `xml:"bookmark"`
		}
		if err := xml.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		items := map[string][]registration{}
		for _, b := range doc.Bookmarks {
			items[b.URI] = b.Applications
		}
		return items
	}
	wait := func(test func(map[string][]registration) bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if test(read()) {
				return
			}
			if time.Now().After(end) {
				t.Fatalf("recent documents never reached expected state: %+v", read())
			}
		}
	}
	// Seed another application's documents through the same public GTK API.
	w.onLoop(t, func() {
		bindRecentFiles()
		m := gtkRecentManagerGetDefault()
		for _, path := range []string{shared, other} {
			if err := addRecentFile(m, path, "dev.nsgo.other", "/tmp/other app"); err != nil {
				t.Error(err)
			}
		}
		gSignalEmitByName(m, cstr("changed"))
	})
	cx.Post(func() { cx.AddRecentFile(shared); cx.AddRecentFile(own); cx.AddRecentFile(own) })
	wait(func(items map[string][]registration) bool {
		return len(items) == 3 && len(items[linuxFileURI(shared)]) == 2 && len(items[linuxFileURI(own)]) == 1 && items[linuxFileURI(own)][0].Count == 2
	})
	beforeOther := read()[linuxFileURI(other)]
	cx.Post(cx.ClearRecentFiles)
	wait(func(items map[string][]registration) bool {
		return len(items) == 2 && len(items[linuxFileURI(shared)]) == 1 && items[linuxFileURI(shared)][0].Name == "dev.nsgo.other"
	})
	if got := read()[linuxFileURI(other)]; len(got) != 1 || got[0] != beforeOther[0] {
		t.Errorf("another application's metadata changed: %+v", got)
	}
	if st, err := os.Stat(filename); err != nil || st.Mode().Perm() != 0600 {
		t.Errorf("recent file permissions: %v, %v", st, err)
	}
	// Clear followed by add in one batch must not resurrect the old documents.
	cx.Post(func() { cx.AddRecentFile(shared); cx.ClearRecentFiles(); cx.AddRecentFile(own) })
	wait(func(items map[string][]registration) bool {
		return len(items) == 3 && len(items[linuxFileURI(shared)]) == 1 && len(items[linuxFileURI(own)]) == 1
	})
	// GTK must construct a usable app launcher from our quoting and URI, even
	// with apostrophes and percent signs in the executable's filename.
	w.onLoop(t, func() {
		var lookup func(manager uintptr, uri *byte, err *uintptr) uintptr
		var createApp func(info uintptr, app *byte, err *uintptr) uintptr
		var infoUnref func(info uintptr)
		var getCommandline func(app uintptr) uintptr
		var parseArgs func(commandline uintptr, count *int32, argv *uintptr, err *uintptr) int32
		purego.RegisterLibFunc(&lookup, gtkLib, "gtk_recent_manager_lookup_item")
		purego.RegisterLibFunc(&createApp, gtkLib, "gtk_recent_info_create_app_info")
		purego.RegisterLibFunc(&infoUnref, gtkLib, "gtk_recent_info_unref")
		purego.RegisterLibFunc(&getCommandline, gioLib, "g_app_info_get_commandline")
		purego.RegisterLibFunc(&parseArgs, glibLib, "g_shell_parse_argv")
		executable := "/tmp/NSGo 'quoted' 100%.AppImage"
		manager := gtkRecentManagerGetDefault()
		if err := addRecentFile(manager, shared, "dev.nsgo.quoted", executable); err != nil {
			t.Error(err)
			return
		}
		info := lookup(manager, cstr(linuxFileURI(shared)), nil)
		if info == 0 {
			t.Error("GTK did not find the quoted document")
			return
		}
		defer infoUnref(info)
		app := createApp(info, cstr("dev.nsgo.quoted"), nil)
		if app == 0 {
			t.Error("GTK could not construct the recent document's app launcher")
			return
		}
		defer gObjectUnref(app)
		var count int32
		var argv uintptr
		if parseArgs(getCommandline(app), &count, &argv, nil) == 0 || count < 2 {
			t.Error("GTK produced no document command")
			return
		}
		defer gStrfreev(argv)
		args := unsafe.Slice((*uintptr)(cPointer(argv)), count)
		if got := goString(args[0]); got != executable {
			t.Errorf("recent executable %q, want %q", got, executable)
		}
		if got := goString(args[1]); got != linuxFileURI(shared) {
			t.Errorf("recent document %q, want %q", got, linuxFileURI(shared))
		}
	})
	// GTK's user preference is respected by its add_full implementation.
	w.onLoop(t, func() {
		settings := gtkSettingsGetDefault()
		was := settingBool(settings, "gtk-recent-files-enabled")
		gObjectSetBool(settings, cstr("gtk-recent-files-enabled"), int32(0), uintptr(0))
		defer gObjectSetBool(settings, cstr("gtk-recent-files-enabled"), boolInt(was), uintptr(0))
		if err := addRecentFile(gtkRecentManagerGetDefault(), other, "dev.nsgo.disabled", "/tmp/disabled"); err != nil {
			t.Error(err)
		}
		gSignalEmitByName(gtkRecentManagerGetDefault(), cstr("changed"))
	})
	if data, err := os.ReadFile(filename); err != nil || strings.Contains(string(data), "dev.nsgo.disabled") {
		t.Errorf("disabled history recorded an application: %v", err)
	}
	t.Log("recent-native-ok")
}

// A failed selective clear must never replace corrupt or unreadable history
// with a newly empty file.
func TestRecentClearKeepsMalformedHistory(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	bindRecentFiles()
	filename := filepath.Join(t.TempDir(), "recently-used.xbel")
	if err := clearRecentApplications(filename, "dev.nsgo.test"); err != nil {
		t.Fatal(err)
	}
	data := []byte("<xbel><broken history")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := clearRecentApplications(filename, "dev.nsgo.test"); err == nil {
		t.Error("malformed history was accepted")
	}
	after, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(after, data) {
		t.Errorf("malformed history changed: %q, %v", after, err)
	}
}
