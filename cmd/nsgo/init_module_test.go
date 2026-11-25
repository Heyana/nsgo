package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitModuleTemplate(t *testing.T) {
	const module = "github.com/nsgo-dev/game/template"
	proxy := t.TempDir()
	blob := []byte{0, '{', '{', 0xff, '}', '}'}
	addTemplateVersion(t, proxy, module, "v1.0.0", map[string][]byte{
		"nsgo.yaml":            []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"main.go":              []byte("package main\nfunc main() { println({{printf \"%q\" .Slug}}) }\n"),
		"go.mod.tmpl":          []byte("module {{.Module}}\n\ngo 1.27.3\n"),
		"assets/images/sprite": blob,
	})
	addTemplateVersion(t, proxy, module, "v1.1.0", map[string][]byte{
		"nsgo.yaml.tmpl": []byte("name: {{.Name}}\nid: {{.ID}}\nversion: 0.2.0\n"),
		"main.go":        []byte("package main\nfunc main() {}\n"),
	})
	server := httptest.NewServer(http.FileServer(http.Dir(proxy)))
	t.Cleanup(server.Close)
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOPROXY", server.URL+",direct")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GONOPROXY", "*")
	t.Setenv("GOMODCACHE", t.TempDir())

	dir := filepath.Join(t.TempDir(), "mygame")
	if err := initApp(t.Context(), dir, "My Game", "example.com/mygame", module+"@v1.0.0"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]byte{
		"nsgo.yaml":            []byte("name: \"My Game\"\nid: dev.nsgo.mygame\nversion: 0.1.0\n"),
		"main.go":              []byte("package main\nfunc main() { println(\"my-game\") }\n"),
		"go.mod":               []byte("module example.com/mygame\n\ngo 1.27.3\n"),
		"assets/images/sprite": blob,
	} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod.tmpl")); !os.IsNotExist(err) {
		t.Errorf("go.mod.tmpl was copied: %v", err)
	}

	dir = filepath.Join(t.TempDir(), "mygame")
	if err := initApp(t.Context(), dir, "", "example.com/latestgame", module); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "nsgo.yaml"))
	if err != nil || !strings.Contains(string(got), "version: 0.2.0") {
		t.Errorf("latest nsgo.yaml = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil || !strings.HasPrefix(string(got), "module example.com/latestgame\n") {
		t.Errorf("generated go.mod = %q, %v", got, err)
	}

	dir = t.TempDir()
	const existing = "package main\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initApp(t.Context(), dir, "My Game", "example.com/mygame", module+"@v1.0.0"); err == nil || !strings.Contains(err.Error(), "main.go exists") {
		t.Errorf("init with an existing main.go: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "main.go" {
		t.Errorf("init created files before checking for conflicts: %v, %v", entries, err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil || string(got) != existing {
		t.Errorf("existing main.go = %q, %v; want %q", got, err, existing)
	}
	t.Setenv("GOPROXY", "direct")
	if _, err := downloadInitModule(t.Context(), module+"@v1.2.0"); err == nil || !strings.Contains(err.Error(), "GOPROXY=off") {
		t.Errorf("proxy disabled: %v", err)
	}
}

func addTemplateVersion(t *testing.T, proxy, module, version string, files map[string][]byte) {
	t.Helper()
	dir := filepath.Join(proxy, filepath.FromSlash(module), "@v")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zipper := zip.NewWriter(&archive)
	mod := []byte("module " + module + "\n\ngo 1.27.3\n")
	files["../go.mod"] = mod
	for name, contents := range files {
		name, rootFile := strings.CutPrefix(name, "../")
		if !rootFile {
			name = "template/" + name
		}
		w, err := zipper.Create(module + "@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{
		version + ".mod":  mod,
		version + ".info": []byte(fmt.Sprintf(`{"Version":%q,"Time":"2026-11-19T00:00:00Z"}`, version)),
		version + ".zip":  archive.Bytes(),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := os.OpenFile(filepath.Join(dir, "list"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := fmt.Fprintln(list, version)
	if closeErr := list.Close(); writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
}

// A slug that names a standard library package gets a module path of its own, and an explicit standard library module is refused before anything is written (#2270).
func TestInitModuleTemplateStandardLibraryCollisions(t *testing.T) {
	const module = "github.com/nsgo-dev/game/template"
	proxy := t.TempDir()
	addTemplateVersion(t, proxy, module, "v1.0.0", map[string][]byte{
		"nsgo.yaml":   []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"main.go":     []byte("package main\nfunc main() {}\n"),
		"go.mod.tmpl": []byte("module {{.Module}}\n\ngo 1.27.3\n"),
	})
	server := httptest.NewServer(http.FileServer(http.Dir(proxy)))
	t.Cleanup(server.Close)
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOPROXY", server.URL+",direct")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GONOPROXY", "*")
	t.Setenv("GOMODCACHE", t.TempDir())

	dir := filepath.Join(t.TempDir(), "time")
	if err := initApp(t.Context(), dir, "Time", "", module+"@v1.0.0"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil || !strings.Contains(string(got), "module example.com/time") {
		t.Fatalf("go.mod = %q, %v; want module example.com/time", got, err)
	}

	dir = filepath.Join(t.TempDir(), "time")
	err = initApp(t.Context(), dir, "Time", "time", module+"@v1.0.0")
	if err == nil || !strings.Contains(err.Error(), "standard library") {
		t.Fatalf("init -module time = %v, want a refusal", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("init wrote %s before refusing", dir)
	}
}

func TestInitModuleNamedTemplates(t *testing.T) {
	const module = "github.com/nsgo-dev/game/templates"
	proxy := t.TempDir()
	addTemplateVersion(t, proxy, module, "v1.0.0", map[string][]byte{
		"platformer/nsgo.yaml":       []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"platformer/main.go":         []byte("package main\nfunc main() { println(\"platformer\") }\n"),
		"platformer/assets/hero.png": []byte{1, 2, 3},
		"rpg/nsgo.yaml.tmpl":         []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"rpg/main.go.tmpl":           []byte("package main\nfunc main() { println(\"rpg\") }\n"),
		"2d/shooter/nsgo.yaml":       []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"2d/shooter/main.go":         []byte("package main\nfunc main() { println(\"shooter\") }\n"),
		"shared/README.md":           []byte("not a template\n"),
	})
	addTemplateVersion(t, proxy, module, "v1.1.0", map[string][]byte{
		"platformer/nsgo.yaml": []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.2.0\n"),
		"platformer/main.go":   []byte("package main\nfunc main() { println(\"platformer-v1.1\") }\n"),
		"rpg/nsgo.yaml":        []byte("name: {{printf \"%q\" .Name}}\nid: {{.ID}}\nversion: 0.2.0\n"),
		"rpg/main.go":          []byte("package main\nfunc main() { println(\"rpg-v1.1\") }\n"),
	})
	server := httptest.NewServer(http.FileServer(http.Dir(proxy)))
	t.Cleanup(server.Close)
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOPROXY", server.URL+",direct")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GONOPROXY", "*")
	t.Setenv("GOMODCACHE", t.TempDir())

	dir := filepath.Join(t.TempDir(), "plat")
	if err := initApp(t.Context(), dir, "Plat", "example.com/plat", module+"@v1.0.0#platformer"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil || !strings.Contains(string(got), "println(\"platformer\")") {
		t.Fatalf("platformer main.go = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "assets/hero.png")); err != nil || !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("platformer asset = %v, %v", got, err)
	}

	dir = filepath.Join(t.TempDir(), "rpg")
	if err := initApp(t.Context(), dir, "RPG", "example.com/rpg", module+"#rpg"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil || !strings.Contains(string(got), "println(\"rpg-v1.1\")") {
		t.Fatalf("latest rpg main.go = %q, %v", got, err)
	}

	dir = filepath.Join(t.TempDir(), "shooter")
	if err := initApp(t.Context(), dir, "Shooter", "example.com/shooter", module+"@v1.0.0#2d/shooter"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil || !strings.Contains(string(got), "println(\"shooter\")") {
		t.Fatalf("nested shooter main.go = %q, %v", got, err)
	}

	if err := initApp(t.Context(), filepath.Join(t.TempDir(), "missing"), "Missing", "", module+"@v1.0.0#missing"); err == nil || !strings.Contains(err.Error(), "\"missing\"") {
		t.Fatalf("missing named template = %v, want error naming missing", err)
	}

	for _, bad := range []string{module + "#", module + "@v1.0.0#", module + "#.", module + "#../escape", module + "#/abs"} {
		if err := initApp(t.Context(), filepath.Join(t.TempDir(), "bad"), "Bad", "", bad); err == nil {
			t.Errorf("initApp(%q) succeeded, want error", bad)
		}
	}

	var buf bytes.Buffer
	if err := listInitTemplates(t.Context(), &buf, "go"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "go\ntsx\nmixed\n"; got != want {
		t.Errorf("builtin list = %q, want %q", got, want)
	}

	buf.Reset()
	if err := listInitTemplates(t.Context(), &buf, module+"@v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "2d/shooter\nplatformer\nrpg\n"; got != want {
		t.Errorf("module v1.0.0 list = %q, want %q", got, want)
	}

	if err := listInitTemplates(t.Context(), &buf, module+"@v1.0.0#platformer"); err == nil || !strings.Contains(err.Error(), "-list does not take a #template name") {
		t.Errorf("listInitTemplates with #name = %v, want a refusal of the name", err)
	}

	err = initApp(t.Context(), filepath.Join(t.TempDir(), "unnamed"), "Unnamed", "", module+"@v1.0.0")
	if err == nil || !strings.Contains(err.Error(), module+"@v1.0.0#<name>") || !strings.Contains(err.Error(), "2d/shooter, platformer, rpg") {
		t.Errorf("init without #name in a module of named templates = %v, want the names to choose from", err)
	}

	// A template/ directory that is itself a template is copied whole, so its subdirectories are not listed as named templates.
	const mixedModule = "github.com/nsgo-dev/mixed/templates"
	addTemplateVersion(t, proxy, mixedModule, "v1.0.0", map[string][]byte{
		"nsgo.yaml":          []byte("name: {{.Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"main.go":            []byte("package main\nfunc main() {}\n"),
		"extra/nsgo.yaml":    []byte("name: {{.Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"extra/main.go":      []byte("package main\nfunc main() {}\n"),
		"extra/assets/x.txt": []byte("x\n"),
	})
	buf.Reset()
	if err := listInitTemplates(t.Context(), &buf, mixedModule+"@v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "(default)\n"; got != want {
		t.Errorf("root template with a nested one, list = %q, want %q", got, want)
	}

	const singleModule = "github.com/nsgo-dev/single/template"
	addTemplateVersion(t, proxy, singleModule, "v1.0.0", map[string][]byte{
		"nsgo.yaml": []byte("name: {{.Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"main.go":   []byte("package main\nfunc main() {}\n"),
	})
	buf.Reset()
	if err := listInitTemplates(t.Context(), &buf, singleModule+"@v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "(default)\n"; got != want {
		t.Errorf("single-template module list = %q, want %q", got, want)
	}
}
