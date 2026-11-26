package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestInitTemplates writes each template's app and runs its tests, Go's
// and TSX's, against this checkout of NSGo.
func TestInitTemplates(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three apps")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range templates {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "app")
			if err := initApp(t.Context(), dir, "My App", "example.com/myapp", kind); err != nil {
				t.Fatal(err)
			}
			if err := initApp(t.Context(), dir, "My App", "", kind); err == nil {
				t.Error("init wrote over the app")
			}
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command("go", args...)
				cmd.Dir = dir
				// Out of any workspace, and with the module proxy on: a fresh runner's cache lacks go.mod files that tidy reads for modules the build never compiles, such as the old golang.org/x/sys esbuild requires.
				cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("go %v: %v\n%s", args, err, out)
				}
			}
			run("mod", "edit", "-require=github.com/nsgo-dev/nsgo@v0.0.0", "-replace=github.com/nsgo-dev/nsgo="+root)
			run("mod", "tidy")
			p, err := load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "go" {
				tsxTest, err := p.tsxTestFile()
				if err != nil || tsxTest == "" {
					t.Fatalf("no TSX test: %v", err)
				}
				ov, err := p.overlay(map[string]string{"zz_nsgo_tsx_test.go": tsxTest})
				if err != nil {
					t.Fatal(err)
				}
				run("test", "-overlay", ov, "-run", "TestTSX", ".")
				return
			}
			run("test", ".")
		})
	}
}

func TestDirAppName(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("my.app", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir("my.app")
	for dir, want := range map[string]string{".": "my-app", "": "my-app", "../2048 game": "game", "Hello World": "Hello World"} {
		if got, err := dirAppName(dir); err != nil || got != want {
			t.Errorf("dirAppName(%q) = %q, %v; want %q", dir, got, err, want)
		}
	}
	if _, err := dirAppName("../应用"); err == nil {
		t.Error("named an app after a directory with no letters it can use")
	}
}

// A module path that names a standard library package is refused before any file is written (#2270).
func TestInitRejectsAStandardLibraryModule(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	err := initApp(t.Context(), dir, "My App", "time", "go")
	if err == nil || !strings.Contains(err.Error(), "standard library") || !strings.Contains(err.Error(), "-module") {
		t.Fatalf("init -module time = %v, want a refusal naming the collision and -module", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("init wrote %s before refusing", dir)
	}
}

// The go command sets nothing for a tool it runs, so where the binary lives says how nsgo was called: its build cache and its work directory are go tool's (#2413).
func TestBuiltByGo(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	for exe, want := range map[string]bool{
		filepath.Join("/home/ada/.cache/go-build", "ab", hash+"-d", "nsgo"):               true,
		filepath.Join("/tmp/go-build1234567", "b001", "exe", "nsgo"):                      true,
		filepath.Join(`C:\Users\ada\AppData\Local\go-build`, "ab", hash+"-d", "nsgo.exe"): true,
		filepath.Join("/home/ada/.cache/go-build", "cd", hash+"-d", "nsgo"):               false,
		filepath.Join("/home/ada/go/bin", "nsgo"):                                         false,
		filepath.Join("/opt/homebrew/bin", "nsgo"):                                        false,
		filepath.Join("/home/ada/.local/bin", "nsgo"):                                     false,
		filepath.Join("/home/ada/project", "exe", "nsgo"):                                 false,
	} {
		if got := builtByGo(exe); got != want {
			t.Errorf("builtByGo(%q) = %v, want %v", exe, got, want)
		}
	}
}

// init's next step names the command as it was called: nsgo for an installed one, the one an installed nsgo delegated to included, and go tool nsgo for the module's tool (#2413).
func TestInitNextNamesTheCommand(t *testing.T) {
	// A module that already requires NSGo and runs it as a tool, which init leaves as it is.
	parent := t.TempDir()
	writeFiles(t, parent, map[string]string{"go.mod": "module example.com/apps\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.99.0\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n"})
	installed := filepath.Join("/home/ada/.local/bin", "nsgo")
	tool := filepath.Join("/home/ada/.cache/go-build", "ab", strings.Repeat("ab", 32)+"-d", "nsgo")
	for i, c := range []struct {
		exe, delegated, want string
	}{
		{installed, "", "next: nsgo dev "},
		{tool, "", "next: go tool nsgo dev "},
		{tool, "1", "next: nsgo dev "},
	} {
		t.Setenv("NSGO_DELEGATED", c.delegated)
		executable = func() (string, error) { return c.exe, nil }
		t.Cleanup(func() { executable = os.Executable })
		dir := filepath.Join(parent, "app"+strconv.Itoa(i))
		var err error
		out := captureStderr(t, func() { err = initApp(t.Context(), dir, "My App", "", "go") })
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("init from %s (NSGO_DELEGATED=%q) printed\n%s\nwant %q", c.exe, c.delegated, out, c.want)
		}
	}
}

// stubNSGo is a stand-in NSGo with the packages a new app imports, which is all go mod tidy reads of them.
var stubNSGo = map[string][]byte{
	"nsgo.go":              []byte("package nsgo\n"),
	"base/base.go":         []byte("package base\n"),
	"core/core.go":         []byte("package core\n"),
	"nsgotest/nsgotest.go": []byte("package nsgotest\n"),
	"tsx/compiler/c.go":    []byte("package compiler\n"),
	"tsx/tsx.go":           []byte("package tsx\n"),
	"cmd/nsgo/main.go":     []byte("package main\n\nfunc main() {}\n"),
}

const stubNSGoMod = "module github.com/nsgo-dev/nsgo\n\ngo 1.27.3\n"

// offlineInit has init require NSGo at version from the module proxy at proxy, offline, with a module cache of its own.
func offlineInit(t *testing.T, version, proxy string) {
	t.Helper()
	t.Setenv("GOPROXY", proxy)
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOFLAGS", os.Getenv("GOFLAGS")+" -modcacherw")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GOMODCACHE", t.TempDir())
	initNSGoVersion = func() string { return version }
	t.Cleanup(func() { initNSGoVersion = toolVersion })
}

// Outside any module, init writes one that requires this nsgo, declares it as a tool and has its go.sum, named after the directory or -module (#2420).
func TestInitWritesACompleteModule(t *testing.T) {
	proxy := t.TempDir()
	addNSGoVersion(t, proxy, "v0.99.0", stubNSGoMod, stubNSGo)
	offlineInit(t, "v0.99.0", "file://"+filepath.ToSlash(proxy))
	t.Chdir(t.TempDir())
	t.Setenv("NSGO_DELEGATED", "1")
	for _, c := range []struct{ dir, module, kind, want string }{
		{"hello", "", "go", "module hello\n"},
		{"named", "example.com/named", "go", "module example.com/named\n"},
		{"time", "", "tsx", "module example.com/time\n"},
	} {
		var err error
		out := captureStderr(t, func() { err = initApp(t.Context(), c.dir, "", c.module, c.kind) })
		if err != nil {
			t.Fatalf("init %s: %v\n%s", c.dir, err, out)
		}
		mod, err := os.ReadFile(filepath.Join(c.dir, "go.mod"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{c.want, "require github.com/nsgo-dev/nsgo v0.99.0\n", "tool github.com/nsgo-dev/nsgo/cmd/nsgo\n"} {
			if !strings.Contains(string(mod), want) {
				t.Errorf("%s/go.mod lacks %q:\n%s", c.dir, want, mod)
			}
		}
		if sum, err := os.ReadFile(filepath.Join(c.dir, "go.sum")); err != nil || !strings.Contains(string(sum), "github.com/nsgo-dev/nsgo v0.99.0 h1:") {
			t.Errorf("%s/go.sum = %q, %v; want NSGo's hash", c.dir, sum, err)
		}
		if want := "next: nsgo dev " + c.dir + "\n"; !strings.Contains(out, want) {
			t.Errorf("init %s printed\n%s\nwant %q", c.dir, out, want)
		}
	}
}

// Inside a module, init adds only the require and tool lines it lacks, keeps a required version, and changes nothing else, tidying nothing (#2420).
func TestInitAddsNSGoToTheEnclosingModule(t *testing.T) {
	offlineInit(t, "v0.99.0", "off")
	const head = "module example.com/apps\n\ngo 1.27.3\n\nrequire example.com/other v1.2.3 // a comment\n"
	for _, c := range []struct{ name, before, after string }{
		{"bare", head, "module example.com/apps\n\ngo 1.27.3\n\nrequire (\n\texample.com/other v1.2.3 // a comment\n\tgithub.com/nsgo-dev/nsgo v0.99.0\n)\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n"},
		{"required", head + "\nrequire github.com/nsgo-dev/nsgo v0.98.0\n", head + "\nrequire github.com/nsgo-dev/nsgo v0.98.0\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n"},
		{"complete", head + "\nrequire github.com/nsgo-dev/nsgo v0.98.0\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n", head + "\nrequire github.com/nsgo-dev/nsgo v0.98.0\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n"},
		{"nsgo", "module github.com/nsgo-dev/nsgo\n\ngo 1.27.3\n", "module github.com/nsgo-dev/nsgo\n\ngo 1.27.3\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			parent := t.TempDir()
			writeFiles(t, parent, map[string]string{"go.mod": c.before})
			if err := initApp(t.Context(), filepath.Join(parent, "apps", "hello"), "Hello", "", "go"); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(parent, "go.mod")); err != nil || string(got) != c.after {
				t.Errorf("go.mod = %q, %v; want %q", got, err, c.after)
			}
			for _, name := range []string{"go.sum", "apps/hello/go.mod"} {
				if _, err := os.Stat(filepath.Join(parent, name)); !os.IsNotExist(err) {
					t.Errorf("init wrote %s: %v", name, err)
				}
			}
		})
	}
}

// A build of nsgo that records no version it can require leaves go.mod with module and go only and prints the go get commands that add NSGo (#2420).
func TestInitDevelopmentBuildPrintsGoGet(t *testing.T) {
	t.Setenv("NSGO_DELEGATED", "1")
	for _, version := range []string{"", "v0.31.1-0.20261120000000-0123456789ab+dirty"} {
		offlineInit(t, version, "off")
		dir := filepath.Join(t.TempDir(), "hello")
		var err error
		out := captureStderr(t, func() { err = initApp(t.Context(), dir, "", "", "go") })
		if err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "go.mod")); err != nil || string(got) != "module hello\n\ngo 1.27.3\n" {
			t.Errorf("version %q: go.mod = %q, %v", version, got, err)
		}
		if want := "go get github.com/nsgo-dev/nsgo && go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo && nsgo dev"; !strings.Contains(out, want) {
			t.Errorf("version %q printed\n%s\nwant %q", version, out, want)
		}
	}
}

func TestRequirableVersion(t *testing.T) {
	for version, want := range map[string]bool{
		"v0.31.0": true, "v0.31.1-0.20261120000000-0123456789ab": true, "v2.0.0+incompatible": true,
		"": false, "v0.31.1-0.20261120000000-0123456789ab+dirty": false,
	} {
		if got := requirableVersion(version); got != want {
			t.Errorf("requirableVersion(%q) = %v, want %v", version, got, want)
		}
	}
}

// A module template's go.mod keeps the NSGo it requires and gains the tool line and go.sum (#2420).
func TestInitModuleTemplateCompletesItsModule(t *testing.T) {
	const module = "github.com/nsgo-dev/game/template"
	proxy := t.TempDir()
	addTemplateVersion(t, proxy, module, "v1.0.0", map[string][]byte{
		"nsgo.yaml":   []byte("name: {{.Name}}\nid: {{.ID}}\nversion: 0.1.0\n"),
		"main.go":     []byte("package main\n\nimport _ \"github.com/nsgo-dev/nsgo/core\"\n\nfunc main() {}\n"),
		"go.mod.tmpl": []byte("module {{.Module}}\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.98.0\n"),
	})
	addNSGoVersion(t, proxy, "v0.98.0", stubNSGoMod, stubNSGo)
	offlineInit(t, "v0.99.0", "file://"+filepath.ToSlash(proxy))
	dir := filepath.Join(t.TempDir(), "game")
	if err := initApp(t.Context(), dir, "Game", "", module+"@v1.0.0"); err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"module game\n", "require github.com/nsgo-dev/nsgo v0.98.0\n", "tool github.com/nsgo-dev/nsgo/cmd/nsgo\n"} {
		if !strings.Contains(string(mod), want) {
			t.Errorf("go.mod lacks %q:\n%s", want, mod)
		}
	}
	if sum, err := os.ReadFile(filepath.Join(dir, "go.sum")); err != nil || !strings.Contains(string(sum), "github.com/nsgo-dev/nsgo v0.98.0 h1:") {
		t.Errorf("go.sum = %q, %v", sum, err)
	}
}

// From an empty directory, nsgo init hello writes an app that builds and passes its tests with no other command, against this checkout served as the running nsgo's version (#2420).
func TestInitThenBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an app against a copy of NSGo")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	const version = "v0.999.0"
	proxy := t.TempDir()
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	addNSGoVersion(t, proxy, version, string(gomod), checkoutFiles(t, root))
	// NSGo's dependencies come from the module cache's downloads, the proxy after them only for what a fresh cache lacks.
	cache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := exec.Command("go", "env", "GOPROXY").Output()
	if err != nil {
		t.Fatal(err)
	}
	offlineInit(t, version, "file://"+filepath.ToSlash(proxy)+",file://"+filepath.ToSlash(filepath.Join(strings.TrimSpace(string(cache)), "cache", "download"))+","+strings.TrimSpace(string(upstream)))
	t.Chdir(t.TempDir())
	if err := initApp(t.Context(), "hello", "", "", "go"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list", "-deps", "tool"}, {"test", "."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = "hello"
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
	}
}

// checkoutFiles is the module zip's content of the NSGo checkout at root: the files Git tracks, as the working tree has them, outside the directories of nested modules.
func checkoutFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	names := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	var nested []string
	for _, name := range names {
		if dir, ok := strings.CutSuffix(name, "/go.mod"); ok {
			nested = append(nested, dir+"/")
		}
	}
	files := make(map[string][]byte)
	for _, name := range names {
		if name == "go.mod" || slices.ContainsFunc(nested, func(dir string) bool { return strings.HasPrefix(name, dir) }) {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if os.IsNotExist(err) || err == nil && !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = content
	}
	return files
}
