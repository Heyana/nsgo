package main

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeNSGoMain is the main package of a stand-in nsgo module, which reports how it was run.
const fakeNSGoMain = `package main

import ("fmt"; "os")

func main() {
	fmt.Printf("fake nsgo %q delegated=%s from=%s\n", os.Args[1:], os.Getenv("NSGO_DELEGATED"), os.Getenv("NSGO_DELEGATED_FROM"))
	os.Exit(5)
}
`

// goEnv returns PATH with go's directory and the environment a test module builds offline in: proxy, a file URL or off, for modules, and a module cache of its own.
func goEnv(t *testing.T, proxy string) (string, []string) {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	return filepath.Dir(gobin), []string{"GOPROXY=" + proxy, "GOSUMDB=off", "GOFLAGS=-mod=mod -modcacherw", "GOTOOLCHAIN=local", "GOMODCACHE=" + t.TempDir()}
}

// nsgoProxy writes a module proxy holding a stand-in github.com/nsgo-dev/nsgo at version and returns its file URL.
func nsgoProxy(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	at := filepath.Join(dir, "github.com", "nsgo-dev", "nsgo", "@v")
	gomod := "module github.com/nsgo-dev/nsgo\n\ngo 1.26\n"
	writeFiles(t, at, map[string]string{
		"list":            version + "\n",
		version + ".info": `{"Version":"` + version + `","Time":"2026-01-01T00:00:00Z"}`,
		version + ".mod":  gomod,
	})
	f, err := os.Create(filepath.Join(at, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, content := range map[string]string{"go.mod": gomod, "cmd/nsgo/main.go": fakeNSGoMain} {
		w, err := z.Create("github.com/nsgo-dev/nsgo@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return "file://" + filepath.ToSlash(dir)
}

// pinnedModule writes a module whose go.mod declares the nsgo tool at version, with more lines after the require, and returns its directory.
func pinnedModule(t *testing.T, version, more string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/app\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo " + version + "\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n" + more})
	return dir
}

func TestDelegateToTheModulesNSGo(t *testing.T) {
	path, env := goEnv(t, nsgoProxy(t, "v0.9.0"))
	dir := pinnedModule(t, "v0.9.0", "")
	download := exec.Command("go", "mod", "download", "github.com/nsgo-dev/nsgo")
	download.Dir, download.Env = dir, append(os.Environ(), append(env, "GOWORK=off")...)
	if out, err := download.CombinedOutput(); err != nil {
		t.Fatalf("go mod download: %v\n%s", err, out)
	}
	own := displayVersion(toolVersion())

	out, code := runNsgo(t, dir, path, env, "version")
	if want := `fake nsgo ["version"] delegated=1 from=` + own; code != 5 || !strings.Contains(out, want) {
		t.Fatalf("exit %d, output %s; want the module's nsgo run as %q with exit 5", code, out, want)
	}
	// Delegation comes before the built-in commands and extensions too.
	if out, _ := runNsgo(t, dir, path, env, "doctor", "extra"); !strings.Contains(out, `fake nsgo ["doctor" "extra"]`) {
		t.Fatalf("doctor was not delegated:\n%s", out)
	}

	out, code = runNsgo(t, dir, path, append(env, "NSGO_NO_DELEGATE=1"), "version")
	if want := "nsgo " + own + " (NSGO_NO_DELEGATE=1; the module's is v0.9.0)\n"; code != 0 || out != want {
		t.Fatalf("NSGO_NO_DELEGATE=1: exit %d, output %q; want %q", code, out, want)
	}
	// The module's nsgo, which NSGO_DELEGATED marks, never delegates again.
	out, code = runNsgo(t, dir, path, append(env, "NSGO_DELEGATED=1", "NSGO_DELEGATED_FROM=v0.3.1"), "version")
	if want := "nsgo " + own + " (the module's; called as v0.3.1)\n"; code != 0 || out != want {
		t.Fatalf("NSGO_DELEGATED=1: exit %d, output %q; want %q", code, out, want)
	}
}

func TestNoDelegation(t *testing.T) {
	path, env := goEnv(t, "off")
	want := "nsgo " + displayVersion(toolVersion()) + "\n"
	noTool := t.TempDir()
	writeFiles(t, noTool, map[string]string{"go.mod": "module example.com/app\n\ngo 1.26\n\ntool example.com/app/cmd/demo\n"})
	for name, dir := range map[string]string{"outside a module": t.TempDir(), "without the nsgo tool": noTool} {
		if out, code := runNsgo(t, dir, path, env, "version"); code != 0 || out != want {
			t.Errorf("%s: exit %d, output %q; want %q", name, code, out, want)
		}
	}

	// A replacement by a directory has no version, as this test's nsgo has none unless the build stamped one.
	fake := t.TempDir()
	writeFiles(t, fake, map[string]string{"go.mod": "module github.com/nsgo-dev/nsgo\n\ngo 1.26\n", "cmd/nsgo/main.go": fakeNSGoMain})
	dir := pinnedModule(t, "v0.9.0", "\nreplace github.com/nsgo-dev/nsgo => "+filepath.ToSlash(fake)+"\n")
	out, _ := runNsgo(t, dir, path, env, "version")
	if delegated := strings.Contains(out, "fake nsgo"); delegated != (toolVersion() != "") {
		t.Errorf("same version: delegated %v with this nsgo at %q:\n%s", delegated, toolVersion(), out)
	}
}

func TestModuleNSGoVersion(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	for _, tc := range []struct {
		name, gomod, version string
		ok                   bool
	}{
		{"required", "module a\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n", "v0.4.0", true},
		{"replaced by a version", "module a\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n\nreplace github.com/nsgo-dev/nsgo v0.4.0 => example.com/fork v0.4.1\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n", "v0.4.1", true},
		{"replaced by a directory", "module a\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n\nreplace github.com/nsgo-dev/nsgo => ../nsgo\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n", "", true},
		{"NSGo's own module", "module github.com/nsgo-dev/nsgo\n\ngo 1.26\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n", "", false},
		{"no nsgo tool", "module a\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n\ntool a/cmd/x\n", "", false},
		{"no tools", "module a\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"go.mod": tc.gomod})
			t.Chdir(dir)
			if version, ok := moduleNSGoVersion(t.Context()); version != tc.version || ok != tc.ok {
				t.Errorf("got %q %v, want %q %v", version, ok, tc.version, tc.ok)
			}
		})
	}
	t.Chdir(t.TempDir())
	if _, ok := moduleNSGoVersion(t.Context()); ok {
		t.Error("outside a module: delegates")
	}
}
