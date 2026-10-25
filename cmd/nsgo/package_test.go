package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestProfilesSignTheirBundleIDs(t *testing.T) {
	for _, c := range []struct {
		appID, id string
		allows    bool
		signed    string
	}{
		{"ABCDE12345.com.example.app", "com.example.app", true, "ABCDE12345.com.example.app"},
		{"ABCDE12345.com.example.app", "com.example.other", false, ""},
		{"ABCDE12345.*", "com.example.app", true, "ABCDE12345.com.example.app"},
		{"ABCDE12345.com.example.*", "com.example.app", true, "ABCDE12345.com.example.app"},
		{"ABCDE12345.com.other.*", "com.example.app", false, ""},
		{"", "com.example.app", false, ""},
	} {
		if got := profileAllows(c.appID, c.id); got != c.allows {
			t.Errorf("profileAllows(%q, %q) = %v", c.appID, c.id, got)
		}
		if c.allows {
			if got := signedAppID(c.appID, c.id); got != c.signed {
				t.Errorf("signedAppID(%q, %q) = %q, want %q", c.appID, c.id, got, c.signed)
			}
		}
	}
}

func TestPropertyListsRoundTrip(t *testing.T) {
	v := map[string]any{"application-identifier": "T.a.b", "get-task-allow": true,
		"keychain-access-groups": []any{"T.*"}, "n": int64(3), "d": []byte{1, 2, 3}}
	got, err := decodePlist(encodePlist(v))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, v) {
		t.Fatalf("round trip gave %#v", got)
	}
}

// A published site names its files by their content, keeps its page and
// manifest, and its page names them.
func TestWebReleasesNameFilesByContent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644)
	}
	write("resources/fonts/a.ttf", "font")
	p := &project{Dir: dir, Name: "Site", ID: "dev.example.site"}
	got, err := copyHashedResources(p, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	if !hashed.MatchString("/" + got) {
		t.Fatalf("resources went to %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", got, "fonts", "a.ttf")); err != nil {
		t.Fatal(err)
	}
	name := hashedName("app.wasm", []byte("wasm"))
	if !hashed.MatchString("/"+name) || hashedName("app.wasm", []byte("wasm2")) == name {
		t.Fatalf("hashed name %q", name)
	}
	if hashed.MatchString("/index.html") || hashed.MatchString("/manifest.webmanifest") {
		t.Fatal("the page or manifest would be cached for good")
	}
}

func TestArchivesHoldTheAppUnderItsName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "pkg")
	os.MkdirAll(filepath.Join(src, "resources"), 0o755)
	os.WriteFile(filepath.Join(src, "App"), []byte("bin"), 0o755)
	os.WriteFile(filepath.Join(src, "resources", "a.txt"), []byte("a"), 0o644)
	if err := archiveDir(src, filepath.Join(dir, "a.tar.gz"), "App"); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(filepath.Join(dir, "a.tar.gz"))
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if h.Name == "App/App" && h.Mode&0o111 == 0 {
			t.Error("the executable lost its mode")
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"App/App", "App/resources/a.txt"}) {
		t.Fatalf("tar holds %v", names)
	}
	if err := archiveDir(src, filepath.Join(dir, "a.zip"), "App"); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(filepath.Join(dir, "a.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 || zr.File[0].Name[:4] != "App/" {
		t.Fatalf("zip holds %v", zr.File)
	}
}

func TestSimulatorsArePickedByName(t *testing.T) {
	sims, err := parseSimulators([]byte(`{"devices": {
		"com.apple.CoreSimulator.SimRuntime.iOS-17-5": [{"udid": "A", "name": "iPhone 15", "state": "Shutdown"}],
		"com.apple.CoreSimulator.SimRuntime.iOS-18-2": [{"udid": "B", "name": "iPhone 16", "state": "Shutdown"},
			{"udid": "C", "name": "iPad Air", "state": "Booted"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for wanted, udid := range map[string]string{"": "C", "iphone 15": "A", "B": "B", "iPad Air": "C"} {
		s, err := pickSimulator(sims, wanted)
		if err != nil || s.UDID != udid {
			t.Errorf("pick %q = %v, %v; want %s", wanted, s, err, udid)
		}
	}
	if _, err := pickSimulator(sims, "Vision"); err == nil {
		t.Error("an unknown name picked a simulator")
	}
}
