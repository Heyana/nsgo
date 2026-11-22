package engine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAppDirs(t *testing.T) {
	env := map[string]string{"HOME": "/h", "APPDATA": `C:\r`, "LOCALAPPDATA": `C:\l`}
	get := func(k string) string { return env[k] }
	for _, c := range []struct{ goos, id, data, cache, logs string }{
		{"darwin", "dev.app", "/h/Library/Application Support/dev.app", "/h/Library/Caches/dev.app", "/h/Library/Logs/dev.app"},
		{"darwin", "a/b", "/h/Library/Application Support/default", "/h/Library/Caches/default", "/h/Library/Logs/default"},
		{"ios", "", "/h/Library/Application Support/default", "/h/Library/Caches/default", "/h/Library/Application Support/default/logs"},
		{"linux", "../x", "/h/.local/share/default", "/h/.cache/default", "/h/.local/state/default"},
		{"windows", "dev.app", filepath.Join(`C:\r`, "dev.app"), filepath.Join(`C:\l`, "dev.app"), filepath.Join(`C:\l`, "dev.app", "Logs")},
		{"windows", "..", filepath.Join(`C:\r`, "default"), filepath.Join(`C:\l`, "default"), filepath.Join(`C:\l`, "default", "Logs")},
		{"android", "dev.app", "", "", ""},
		{"js", "dev.app", "", "", ""},
	} {
		data, cache, logs := appDirs(c.goos, c.id, get)
		if data != filepath.FromSlash(c.data) && data != c.data || cache != filepath.FromSlash(c.cache) && cache != c.cache {
			t.Errorf("%s %q: %q, %q; want %q, %q", c.goos, c.id, data, cache, c.data, c.cache)
		}
		if logs != filepath.FromSlash(c.logs) && logs != c.logs {
			t.Errorf("%s %q: logs %q, want %q", c.goos, c.id, logs, c.logs)
		}
	}
	if data, cache, logs := appDirs("linux", "a", func(string) string { return "" }); data != "" || cache != "" || logs != "" {
		t.Errorf("without a home: %q, %q, %q", data, cache, logs)
	}
	if _, _, logs := appDirs("windows", "a", func(k string) string { return map[string]string{"APPDATA": `C:\r`}[k] }); logs != "" {
		t.Errorf("without LOCALAPPDATA: logs %q", logs)
	}
	env["NSGO_DATA_DIR"], env["NSGO_CACHE_DIR"] = "/app/files", "/app/cache"
	if data, cache, logs := appDirs("android", "dev.app", get); data != "/app/files" || cache != "/app/cache" || logs != filepath.Join(data, "logs") {
		t.Errorf("Android runtime paths: data %q, cache %q, logs %q", data, cache, logs)
	}
	env["XDG_STATE_HOME"] = "/state"
	for id, want := range map[string]string{"dev.app": "/state/dev.app", "default": "/state/default", "": "/state/default"} {
		if _, _, logs := appDirs("linux", id, get); logs != filepath.FromSlash(want) {
			t.Errorf("XDG_STATE_HOME, %q: logs %q, want %q", id, logs, want)
		}
	}
}

// graphOn returns storage for a graph over store.
func graphOn(store *core.Store) core.Storage {
	return core.NewRoot(core.RootOptions{Post: func(func()) {}, Storage: store}).Storage()
}

func TestFileStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	store := OpenFileStorage(dir)
	if OpenFileStorage(dir) != store {
		t.Fatal("a second open made another store")
	}
	s := graphOn(store)
	s.SetItem("a", "1")
	s.SetItem("b", "2")
	s.RemoveItem("a")
	SyncStorage()
	data, err := os.ReadFile(filepath.Join(dir, "storage.json"))
	if err != nil || string(data) != `{"b":"2"}` {
		t.Fatalf("storage.json = %s, %v", data, err)
	}

	// A file that does not parse is set aside, not overwritten.
	other := filepath.Join(t.TempDir(), "app")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "storage.json"), []byte("{oops"), 0o644)
	s = graphOn(OpenFileStorage(other))
	if keys := s.Keys(); len(keys) != 0 {
		t.Fatalf("keys = %v", keys)
	}
	s.SetItem("c", "3")
	SyncStorage()
	if data, _ := os.ReadFile(filepath.Join(other, "storage.json.corrupt")); string(data) != "{oops" {
		t.Fatalf("set aside: %s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(other, "storage.json")); string(data) != `{"c":"3"}` {
		t.Fatalf("storage.json = %s", data)
	}
}

func TestEngineMakesAppDirs(t *testing.T) {
	root := t.TempDir()
	opts := Options{Size: driver.Size{Width: 100, Height: 100}}
	opts.SetDirs(filepath.Join(root, "data"), filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	var data, logs string
	r := &recorder{mounted: make(chan struct{}, 8)}
	e := Start(r, func(cx *core.Scope) core.Node {
		data = cx.DataDir()
		logs = cx.LogsDir()
		return core.View{}
	}, opts)
	<-r.mounted
	e.Close()
	if data != opts.DataDir {
		t.Fatalf("DataDir = %q", data)
	}
	if logs != opts.LogsDir {
		t.Fatalf("LogsDir = %q", logs)
	}
	for _, d := range []string{opts.DataDir, opts.CacheDir, opts.LogsDir} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("%s: %v", d, err)
		}
	}
}

// A host's data directory (nsgo.DataDir, an embedding's) moves the logs with the data, so an embedded app writes nothing in the system's directories for its ID.
func TestHostDataDirMovesLogs(t *testing.T) {
	root := t.TempDir()
	opts := Options{Size: driver.Size{Width: 100, Height: 100}}
	opts.SetDirs(filepath.Join(root, "system", "data"), filepath.Join(root, "system", "cache"), filepath.Join(root, "system", "logs"))
	host := filepath.Join(root, "host")
	opts.Host.DataDir = host
	var data, logs string
	r := &recorder{mounted: make(chan struct{}, 8)}
	e := Start(r, func(cx *core.Scope) core.Node {
		data, logs = cx.DataDir(), cx.LogsDir()
		return core.View{}
	}, opts)
	<-r.mounted
	e.Close()
	if want := filepath.Join(host, "logs"); data != host || logs != want {
		t.Fatalf("DataDir, LogsDir = %q, %q; want %q, %q", data, logs, host, want)
	}
	if fi, err := os.Stat(logs); err != nil || !fi.IsDir() {
		t.Fatalf("%s: %v", logs, err)
	}
	for _, d := range []string{"data", "logs"} {
		if _, err := os.Stat(filepath.Join(root, "system", d)); !os.IsNotExist(err) {
			t.Fatalf("the system's %s directory was made: %v", d, err)
		}
	}
}

// platformStorageRecorder implements only the private platform contract.
type platformStorageRecorder struct {
	changes []driver.StorageChange
}

func (*platformStorageRecorder) Load() (map[string]string, error) {
	return map[string]string{"kept": "old"}, nil
}

func (b *platformStorageRecorder) Write(c driver.StorageChange) {
	b.changes = append(b.changes, c)
}

func TestStorageBackendAdaptsHostChanges(t *testing.T) {
	backend := &platformStorageRecorder{}
	s := graphOn(NewStorage(backend))
	if v, ok := s.GetItem("kept"); !ok || v != "old" {
		t.Fatalf("loaded value = %q, %v", v, ok)
	}
	if err := s.SetItem("new", "value"); err != nil {
		t.Fatal(err)
	}
	s.RemoveItem("kept")
	s.Clear()
	want := []driver.StorageChange{
		{Op: driver.StorageSet, Key: "new", Value: "value"},
		{Op: driver.StorageRemove, Key: "kept"},
		{Op: driver.StorageClear},
	}
	if !slices.Equal(backend.changes, want) {
		t.Fatalf("platform changes = %#v, want %#v", backend.changes, want)
	}

	// No adapter is installed for an in-memory store.
	memory := graphOn(NewStorage(nil))
	if err := memory.SetItem("key", "value"); err != nil {
		t.Fatal(err)
	}
	memory.Clear()
	if len(memory.Keys()) != 0 {
		t.Fatal("in-memory storage retained a cleared key")
	}
}
