package main

import (
	"flag"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestDevDebugOptions(t *testing.T) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-debug"}); err != nil {
		t.Fatal(err)
	}
	opts := f.options()
	if !opts.debug {
		t.Fatal("-debug did not reach the dev options")
	}
	p := &project{}
	p.Dev.Target = "web"
	if err := checkDevDebug(p, opts); err == nil || !strings.Contains(err.Error(), "-target native") {
		t.Fatalf("web debugging: %v", err)
	}
	p.Dev.Target = "native"
	opts.release = true
	if err := checkDevDebug(p, opts); err == nil || !strings.Contains(err.Error(), "-release") {
		t.Fatalf("release debugging: %v", err)
	}
}

func TestDevServerKeepsBindAddressWhenPortIsTaken(t *testing.T) {
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	if port == 65535 {
		t.Skip("no higher port to try")
	}

	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-host", "127.0.0.1", "-port", strconv.Itoa(port)}); err != nil {
		t.Fatal(err)
	}
	opts := f.options()
	s, err := startDevServer(opts.host, opts.port)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if !s.addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || s.addr.Port <= port {
		t.Fatalf("listener = %s, want 127.0.0.1 on a port above %d", s.addr, port)
	}
	if urls := s.lanURLs(); len(urls) != 0 {
		t.Errorf("a loopback listener advertised LAN addresses: %v", urls)
	}
	resp := request(t, s, "GET", "/_nsgo/devtools/", "", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("DevTools at %s returned %s", s.url(), resp.Status)
	}
}

func TestDevEmbeddedChanges(t *testing.T) {
	for _, tc := range []struct {
		name, main string
		bundled    bool
	}{
		{"Go", `package main
import "embed"
//go:embed assets/*
var src embed.FS
func main() {}
`, false},
		{"TSX", `package main
import (
	"embed"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
)
//go:embed assets/*
var src embed.FS
var _ = compiler.App(src, "assets/App.tsx")
func main() {}
`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{
				"main.go":         tc.main,
				"tsconfig.json":   `{}`,
				"assets/map.json": `{"level":1}`,
				"assets/code.js":  `export const x = 1`,
				"assets/code.ts":  `export const y: number = 2`,
				"assets/App.tsx":  `import map from "./map.json"; import "./code.js"; import "./code.ts"; export default map`,
				"assets/note.txt": "original",
			})
			names := []string{"map.json", "code.js", "code.ts", "App.tsx", "note.txt"}
			files := []string{dir}
			for _, name := range names {
				files = append(files, filepath.Join(dir, "assets", name))
			}
			build, bundles := splitSources(dir, files)
			for _, name := range names {
				beforeBuild, beforeBundles := scan(build), scan(bundles)
				path := filepath.Join(dir, "assets", name)
				if err := os.WriteFile(path, []byte("the replacement differs in size from every original"), 0o644); err != nil {
					t.Fatal(err)
				}
				wantBundle := tc.bundled && name != "note.txt"
				if rebuilt, reloaded := scan(build) != beforeBuild, scan(bundles) != beforeBundles; rebuilt == wantBundle || reloaded != wantBundle {
					t.Errorf("saving %s: Go rebuild = %t, bundle reload = %t; want %t, %t",
						name, rebuilt, reloaded, !wantBundle, wantBundle)
				}
			}
		})
	}
}

func TestDevNewEmbeddedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module embedprobe\n\ngo 1.26\n",
		"main.go": `package main
import "embed"
//go:embed notes/*.txt
var notes embed.FS
//go:embed static
var static embed.FS
//go:embed all:raw
var raw embed.FS
func main() {}
`,
		"notes/a.txt":     "a",
		"static/index.md": "index",
		"raw/a":           "a",
	})
	pkgs, err := goPackages(t.Context(), dir, false, nil, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || len(pkgs[0].embeds) != 3 || len(pkgs[0].embedPatterns) != 3 {
		t.Fatalf("go list = %+v, want one package with 3 files and 3 patterns embedded", pkgs)
	}
	dirs, globs := sourceDirs(dir, pkgs), embedGlobs(pkgs)
	fingerprint := func() string { return scan(dirs) + embedMatches(globs) }
	for _, tc := range []struct {
		file    string
		rebuild bool
	}{
		{"notes/b.txt", true},
		{"notes/b.md", false},
		{"static/deep/page.md", true},
		{"static/_draft.md", false},
		{"static/.hidden/page.md", false},
		{"raw/_kept", true},
	} {
		before := fingerprint()
		writeFiles(t, dir, map[string]string{tc.file: "new"})
		if rebuilt := fingerprint() != before; rebuilt != tc.rebuild {
			t.Errorf("creating %s: rebuild = %t, want %t", tc.file, rebuilt, tc.rebuild)
		}
	}
}

func TestDevDebugKeepsSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module debugprobe\n\ngo 1.26\n",
		"main.go": "package main\nfunc main() {}\n",
	})
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "Debug probe", ID: "dev.nsgo.debugprobe"}}
	p.Build.LDFlags = []string{"-s -w"}
	p.prepareDev(devOptions{debug: true})
	cmd, done, err := goBuildCommand(t.Context(), p, filepath.Join(dir, "app"), nil, buildExtra{})
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if !slices.Contains(cmd.Args, "-gcflags=all=-N -l") {
		t.Errorf("debug build flags %q omit the optimization override", cmd.Args)
	}
	i := slices.Index(cmd.Args, "-ldflags")
	if i < 0 || !strings.HasSuffix(cmd.Args[i+1], " -s=false -w=false") {
		t.Errorf("debug linker flags %q can strip symbols", cmd.Args)
	}
}

func TestNativeSessionFollowsTheHost(t *testing.T) {
	p := &project{}
	p.Dev.Target = "native"
	s, err := sessionFor(p, devOptions{})
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		if err != nil {
			t.Fatal(err)
		}
		if s.build == nil || s.start == nil {
			t.Fatal("the native session is missing a step")
		}
	default:
		if err == nil || !strings.Contains(err.Error(), "web") {
			t.Fatalf("native on %s: %v", runtime.GOOS, err)
		}
	}
}

func TestWindowsDevBuildKeepsThePreviousExecutable(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module windowsdevprobe\n\ngo 1.26\n",
		"main.go": "package main\nfunc main() {}\n",
	})
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "Windows dev probe", ID: "dev.nsgo.windowsprobe", Version: "1.0.0"}}
	p.Dev.Target = "native"
	opts := devOptions{nativeDir: t.TempDir()}
	p.prepareDev(opts)
	s := windowsSession(opts)
	for count := 1; count <= 2; count++ {
		if err := s.build(t.Context(), p); err != nil {
			t.Fatal(err)
		}
		binaries, err := filepath.Glob(filepath.Join(opts.nativeDir, "app-*", p.Name+".exe"))
		if err != nil || len(binaries) != count {
			t.Fatalf("after %d builds, got %v and %v", count, binaries, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package invalid-build"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.build(t.Context(), p); err == nil {
		t.Fatal("the invalid project built")
	}
	if dirs, err := os.ReadDir(opts.nativeDir); err != nil || len(dirs) != 2 {
		t.Fatalf("after failed build, got %v and %v", dirs, err)
	}
}

func TestAndroidSessionInstallsOverADB(t *testing.T) {
	p := &project{}
	p.Dev.Target = "android"
	s, err := sessionFor(p, devOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.build == nil || s.start == nil {
		t.Fatal("the Android session is missing a step")
	}
}

func TestOpenCommandShowsURLs(t *testing.T) {
	name, args := openCommand("https://nsh.dev")
	if got := append([]string{name}, args...); len(got) == 0 || got[len(got)-1] != "https://nsh.dev" {
		t.Fatalf("open command = %q, want the target last", got)
	}
	switch runtime.GOOS {
	case "darwin":
		if name != "open" {
			t.Fatalf("macOS opens with %q", name)
		}
	case "windows":
		if name != "rundll32" || len(args) != 2 || args[0] != "url.dll,FileProtocolHandler" {
			t.Fatalf("Windows opens with %q %q", name, args)
		}
	case "linux":
		if name != "xdg-open" || len(args) != 1 {
			t.Fatalf("Linux opens with %q %q", name, args)
		}
	}
}
