package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
	installed := filepath.Join("/home/ada/.local/bin", "nsgo")
	tool := filepath.Join("/home/ada/.cache/go-build", "ab", strings.Repeat("ab", 32)+"-d", "nsgo")
	for _, c := range []struct {
		exe, delegated, want string
	}{
		{installed, "", "next: nsgo dev "},
		{tool, "", "next: go tool nsgo dev "},
		{tool, "1", "next: nsgo dev "},
	} {
		t.Setenv("NSGO_DELEGATED", c.delegated)
		executable = func() (string, error) { return c.exe, nil }
		t.Cleanup(func() { executable = os.Executable })
		dir := filepath.Join(t.TempDir(), "app")
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
