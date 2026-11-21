package main

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func TestDevDebugKeepsSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module debugprobe\n\ngo 1.26\n",
		"main.go": "package main\nfunc main() {}\n",
	})
	p := &project{Dir: dir, Name: "Debug probe", ID: "dev.nsgo.debugprobe"}
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
	p := &project{Dir: dir, Name: "Windows dev probe", ID: "dev.nsgo.windowsprobe", Version: "1.0.0"}
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
