package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/licenseclassifier/v2/assets"
	"golang.org/x/mod/module"
)

// init makes a copy of the test binary named git record its arguments in NSGO_TEST_GIT_LOG and fail, so a test sees whether the go command ran git, and one named go run the go command in NSGO_TEST_GO_SWITCH as a toolchain switch does.
func init() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if root := os.Getenv("NSGO_TEST_GO_SWITCH"); name == "go" && root != "" {
		cmd := exec.Command(filepath.Join(root, "bin", "go"), os.Args[1:]...)
		cmd.Env = append(os.Environ(), "GOROOT="+root)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err := cmd.Run()
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	log := os.Getenv("NSGO_TEST_GIT_LOG")
	if name != "git" || log == "" {
		return
	}
	f, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err == nil {
		fmt.Fprintf(f, "git %s\n", strings.Join(os.Args[1:], " "))
		f.Close()
	}
	os.Exit(1)
}

// addProxyModule adds the module path at version, with gomod and files, to the module proxy in dir, published 2026-01-01. A pseudo-version is the module's @latest and stays out of its list of versions, as a proxy serves a module with no release.
func addProxyModule(t *testing.T, dir, path, version, gomod string, files map[string]string) {
	t.Helper()
	at := filepath.Join(dir, filepath.FromSlash(path), "@v")
	info := `{"Version":"` + version + `","Time":"2026-01-01T00:00:00Z"}`
	writeFiles(t, at, map[string]string{version + ".info": info, version + ".mod": gomod})
	listed := version + "\n"
	if module.IsPseudoVersion(version) {
		writeFiles(t, filepath.Dir(at), map[string]string{"@latest": info})
		listed = ""
	}
	list, err := os.OpenFile(filepath.Join(at, "list"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = list.WriteString(listed)
	if err := errors.Join(err, list.Close()); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	files["go.mod"] = gomod
	for name, content := range files {
		w, err := z.Create(path + "@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(at, version+".zip"), archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// licenseFixtureText reads a pristine license text from the license classifier's embedded assets.
func licenseFixtureText(t *testing.T, id string) string {
	t.Helper()
	data, err := assets.ReadLicenseFile("License/" + id + "/pristine.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// libraryProxy is a module proxy holding a stand-in NSGo at the releases v0.3.4 and v0.4.0 and at a commit after them, and the modules the tests add.
func libraryProxy(t *testing.T) string {
	t.Helper()
	proxy := t.TempDir()
	mit := licenseFixtureText(t, "MIT")
	addNSGoVersion(t, proxy, "v0.3.4", stubNSGoMod, stubNSGo)
	addNSGoVersion(t, proxy, "v0.4.0", stubNSGoMod, stubNSGo)
	addNSGoVersion(t, proxy, "v0.4.1-0.20261125111955-8d716d9b939c", stubNSGoMod, stubNSGo)
	addProxyModule(t, proxy, "example.com/dep", "v1.0.0", "module example.com/dep\n\ngo 1.26\n", map[string]string{"dep.go": "package dep\n", "LICENSE": mit})
	addProxyModule(t, proxy, "github.com/acme/chart", "v1.2.0", "module github.com/acme/chart\n\ngo 1.27.3\n\nrequire (\n\texample.com/dep v1.0.0\n\tgithub.com/nsgo-dev/nsgo v0.4.0\n)\n",
		map[string]string{"chart.go": "package chart\n\nimport _ \"example.com/dep\"\n", "LICENSE": mit})
	addProxyModule(t, proxy, "github.com/acme/media", "v0.1.0", "module github.com/acme/media\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.4.1-0.20261125111955-8d716d9b939c\n",
		map[string]string{"player/player.go": "package player\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/newgo", "v1.0.0", "module example.com/newgo\n\ngo 1.99\n", map[string]string{"newgo.go": "package newgo\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/old", "v1.0.0", "module example.com/old\n\ngo 1.26\n", map[string]string{"old.go": "package old\n", "LICENSE": mit})
	// The notice's second line stays out of the plan, as go get leaves it out.
	addProxyModule(t, proxy, "example.com/old", "v1.1.0", "// Deprecated: use example.com/new\n// instead, which is maintained.\nmodule example.com/old\n\ngo 1.26\n\nretract v1.0.0 // broken build\n", map[string]string{"old.go": "package old\n", "LICENSE": mit})
	// Notices with an escape that would hide the rest of the plan in a terminal.
	addProxyModule(t, proxy, "example.com/sneaky", "v1.0.0", "module example.com/sneaky\n\ngo 1.26\n", map[string]string{"sneaky.go": "package sneaky\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/sneaky", "v1.1.0", "// Deprecated: fine\x1b[8m\n// second line\nmodule example.com/sneaky\n\ngo 1.26\n\nretract v1.0.0 // broken\x1b[8m\n", map[string]string{"sneaky.go": "package sneaky\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/std", "v1.0.0", "module example.com/std\n\ngo 1.26\n", map[string]string{"std.go": "package std\n\nimport _ \"strings\"\n", "LICENSE": mit})
	addProxyModule(t, proxy, "github.com/acme/edge", "v0.0.0-20260101000000-abcdefabcdef", "module github.com/acme/edge\n\ngo 1.27\n", map[string]string{"edge.go": "package edge\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/lib", "v1.2.0", "module example.com/lib\n\ngo 1.26\n", map[string]string{"lib.go": "package lib\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/lib", "v1.3.0-rc.1", "module example.com/lib\n\ngo 1.26\n", map[string]string{"lib.go": "package lib\n", "LICENSE": mit})
	addProxyModule(t, proxy, "example.com/cmdonly", "v1.0.0", "module example.com/cmdonly\n\ngo 1.26\n", map[string]string{"cmd/tool/main.go": "package main\n\nfunc main() {}\n", "LICENSE": mit})
	return proxy
}

// offlineAdd has nsgo add, and the go commands the tests run, work offline against the module proxy in proxy, with a module cache of their own and outside any workspace.
func offlineAdd(t *testing.T, proxy string) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy))
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOFLAGS", os.Getenv("GOFLAGS")+" -modcacherw")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GONOSUMDB", "")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOMODCACHE", t.TempDir())
}

// nsgoApp writes an app's module that requires NSGo v0.3.4 and runs it as a tool, with its go.sum, and returns its directory.
func nsgoApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/app\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.3.4\n\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n"})
	goIn(t, dir, "mod", "tidy")
	return dir
}

// goIn runs go with args in dir, outside any workspace, and fails the test when it fails.
func goIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// planAdd runs nsgo add -dry-run with args in the module at dir and returns the plan it prints.
func planAdd(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	return runAdd(t, dir, addOptions{dryRun: true}, nil, args...)
}

// runAdd runs nsgo add with o and args in the module at dir, asking ask, and returns what it prints on standard output.
func runAdd(t *testing.T, dir string, o addOptions, ask func(context.Context) (bool, error), args ...string) (string, error) {
	t.Helper()
	t.Chdir(dir)
	var out bytes.Buffer
	var err error
	captureStderr(t, func() { err = addModules(t.Context(), o, args, &out, ask) })
	return out.String(), err
}

// moduleFiles reads go.mod and go.sum in dir; a missing go.sum reads as nil.
func moduleFiles(t *testing.T, dir string) (gomod, gosum []byte) {
	t.Helper()
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	gosum, err = os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return gomod, gosum
}

// goGetWrites copies the module at dir, without its other files, runs go get with args in the copy outside any workspace, and returns the go.mod and go.sum it writes.
func goGetWrites(t *testing.T, dir string, args ...string) (gomod, gosum []byte) {
	t.Helper()
	copied := t.TempDir()
	gomod, gosum = moduleFiles(t, dir)
	files := map[string]string{"go.mod": string(gomod)}
	if gosum != nil {
		files["go.sum"] = string(gosum)
	}
	writeFiles(t, copied, files)
	goIn(t, copied, append([]string{"get"}, args...)...)
	return moduleFiles(t, copied)
}

// The plan of a library that moves NSGo to a release names the module, its owner, every go.mod line that changes, the tool that then runs another NSGo and the import line, and applying it writes what go get writes (#2460).
func TestAddPlansAndAppliesWhatGoGetWrites(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	oldMod, oldSum := moduleFiles(t, dir)

	plan, err := planAdd(t, dir, "github.com/acme/chart")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	const want = `github.com/acme/chart
  module     github.com/acme/chart v1.2.0, published 2026-01-01
  owner      github.com/acme, the first module from it in go.mod
  licenses   github.com/acme/chart: MIT
             example.com/dep: MIT
  next       import "github.com/acme/chart"

example.com/app
  go.mod     + require github.com/acme/chart v1.2.0 // indirect
             + require example.com/dep v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.3.4 => v0.4.0, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.0, and so does an installed nsgo here
  checksums  GOSUMDB=off: go.sum takes the new hashes unchecked by the checksum database
`
	if plan != want {
		t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Fatalf("-dry-run wrote go.mod or go.sum:\n%s\n%s", gomod, gosum)
	}

	wantMod, wantSum := goGetWrites(t, dir, "github.com/acme/chart@v1.2.0")
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
		t.Fatal(err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, wantMod) || !bytes.Equal(gosum, wantSum) {
		t.Errorf("go.mod:\n%s\ngo.sum:\n%s\nwant what go get writes:\n%s\n%s", gomod, gosum, wantMod, wantSum)
	}
}

// Retraction and deprecation notices, a newer version than the one typed, an untagged version, a new go line and a module of a new owner all show (#2460).
func TestAddPlanShowsNotices(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	plan, err := planAdd(t, dir, "example.com/old@v1.0.0", "github.com/acme/edge")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	const want = `example.com/old
  module     example.com/old v1.0.0, published 2026-01-01; v1.1.0 is newer
  owner      example.com
  retracted  broken build
  deprecated use example.com/new
  licenses   example.com/old: MIT
  next       import "example.com/old"

github.com/acme/edge
  module     github.com/acme/edge v0.0.0-20260101000000-abcdefabcdef, published 2026-01-01
  owner      github.com/acme, the first module from it in go.mod
  untagged   v0.0.0-20260101000000-abcdefabcdef is a commit with no release tag
  licenses   github.com/acme/edge: MIT
  next       import "github.com/acme/edge"

example.com/tool
  go.mod     ~ go 1.26 => 1.27
             + require example.com/old v1.0.0 // indirect
             + require github.com/acme/edge v0.0.0-20260101000000-abcdefabcdef // indirect
  go.sum     + 4 lines
  checksums  GOSUMDB=off: go.sum takes the new hashes unchecked by the checksum database
`
	if plan != want {
		t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.sum")); !os.IsNotExist(err) {
		t.Errorf("-dry-run wrote go.sum: %v", err)
	}
}

// A notice holding a character a terminal does not print, such as an escape that would hide the rest of the plan, shows as the go command shows it, a placeholder (#2460).
func TestAddPlanHidesUnprintableNotices(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	plan, err := planAdd(t, dir, "example.com/sneaky@v1.0.0")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  retracted  (message omitted: contains non-printable characters)\n",
		"  deprecated (message omitted: contains non-printable characters)\n",
		"  go.mod     + require example.com/sneaky v1.0.0 // indirect\n",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
	if strings.ContainsAny(plan, "\x1b") || strings.Contains(plan, "second line") {
		t.Errorf("plan holds the module's notice as written:\n%q", plan)
	}
}

func TestShortMessage(t *testing.T) {
	for msg, want := range map[string]string{
		"":                              "default",
		" \n second line":               "default",
		"  use example.com/new  ":       "use example.com/new",
		"use example.com/new\nsecond":   "use example.com/new",
		"tab\tand space":                "tab\tand space",
		"fine\x1b[8m":                   "(message omitted: contains non-printable characters)",
		"bell\a":                        "(message omitted: contains non-printable characters)",
		"right-to-left \u202e override": "(message omitted: contains non-printable characters)",
		strings.Repeat("x", 500):        strings.Repeat("x", 500),
		strings.Repeat("x", 501):        "(message omitted: too long)",
	} {
		if got := shortMessage(msg, "default"); got != want {
			t.Errorf("shortMessage(%q) = %q, want %q", msg, got, want)
		}
	}
}

// A module without a root package has no import line, and a module go.mod replaces says so; its content comes from the replacement, so go.sum takes nothing (#2460).
func TestAddPlanReplacedModuleWithoutPackage(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	local := filepath.Join(t.TempDir(), "cmdonly")
	writeFiles(t, local, map[string]string{
		"go.mod":           "module example.com/cmdonly\n\ngo 1.26\n",
		"cmd/tool/main.go": "package main\n\nfunc main() {}\n",
		"LICENSE":          licenseFixtureText(t, "MIT"),
	})
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n\nreplace example.com/cmdonly => " + filepath.ToSlash(local) + "\n"})
	plan, err := planAdd(t, dir, "example.com/cmdonly@v1.0.0")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	want := `example.com/cmdonly
  module     example.com/cmdonly v1.0.0
  owner      example.com
  replaced   go.mod replaces it with ` + filepath.ToSlash(local) + `, which builds use instead
  licenses   example.com/cmdonly: MIT

example.com/tool
  go.mod     + require example.com/cmdonly v1.0.0 // indirect
`
	if plan != want {
		t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
	}
}

// A second add of the same module, by path or at the version go.mod has, writes nothing and exits 0, printing the version (#2460).
func TestAddTwiceWritesNothing(t *testing.T) {
	proxy := libraryProxy(t)
	path, env := goEnv(t, "file://"+filepath.ToSlash(proxy))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	if out, code := runNsgo(t, dir, path, env, "add", "-y", "example.com/dep"); code != 0 {
		t.Fatalf("first add: exit %d\n%s", code, out)
	}
	gomod, gosum := moduleFiles(t, dir)
	mod, err := os.Stat(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"example.com/dep", "example.com/dep@v1.0.0"} {
		out, code := runNsgo(t, dir, path, env, "add", arg)
		if code != 0 || !strings.Contains(out, "example.com/dep v1.0.0 is already in go.mod") {
			t.Errorf("add %s again: exit %d\n%s", arg, code, out)
		}
		again, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err != nil {
			t.Fatal(err)
		}
		if newMod, newSum := moduleFiles(t, dir); !bytes.Equal(newMod, gomod) || !bytes.Equal(newSum, gosum) || !again.ModTime().Equal(mod.ModTime()) {
			t.Errorf("add %s again wrote go.mod or go.sum:\n%s\n%s", arg, newMod, newSum)
		}
	}
}

// A library that requires an NSGo commit with no release is refused, -y or not, with the go get that moves NSGo there on purpose, and go.mod stays as it was (#2460).
func TestAddRefusesNSGoOffItsReleases(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	oldMod, oldSum := moduleFiles(t, dir)
	for _, o := range []addOptions{{dryRun: true}, {yes: true}} {
		plan, err := runAdd(t, dir, o, nil, "github.com/acme/media")
		want := "go get github.com/nsgo-dev/nsgo@v0.4.1-0.20261125111955-8d716d9b939c first"
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "a commit with no release") {
			t.Errorf("%+v: %v, want a refusal naming %q", o, err, want)
		}
		if !strings.Contains(plan, "~ require github.com/nsgo-dev/nsgo v0.3.4 => v0.4.1-0.20261125111955-8d716d9b939c") {
			t.Errorf("%+v: the plan does not show NSGo's move:\n%s", o, plan)
		}
		if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
			t.Fatalf("%+v: go.mod or go.sum changed:\n%s", o, gomod)
		}
	}
}

// A module whose go line is newer than the toolchain stops the plan with the go command's message and the go get that raises the go line, and no toolchain is downloaded, even with GOTOOLCHAIN=auto (#2460).
func TestAddRefusesANewerGo(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	t.Setenv("GOTOOLCHAIN", "auto")
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	plan, err := planAdd(t, dir, "example.com/newgo")
	if err == nil || !strings.Contains(err.Error(), "requires go >= 1.99") || !strings.Contains(err.Error(), "run go get go@1.99 first") {
		t.Fatalf("plan %q, error %v; want the go command's message and go get go@1.99", plan, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("GOMODCACHE"), "golang.org", "toolchain")); !os.IsNotExist(err) {
		t.Errorf("a toolchain was downloaded: %v", err)
	}
	if gomod, _ := moduleFiles(t, dir); string(gomod) != "module example.com/tool\n\ngo 1.26\n" {
		t.Errorf("go.mod changed:\n%s", gomod)
	}
}

// fakeGitOnPath puts a copy of the test binary named git in a new directory, which records its calls in the returned log, and returns the directory.
func fakeGitOnPath(t *testing.T) (dir, log string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	dst := filepath.Join(dir, "git")
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}
	if os.Link(exe, dst) != nil {
		data, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	log = filepath.Join(t.TempDir(), "git.log")
	t.Setenv("NSGO_TEST_GIT_LOG", log)
	return dir, log
}

// fakeToolchainSwitch puts a copy of the test binary named go in a new directory, which runs the go command in root with GOROOT=root, as a go command that switches to the toolchain in root runs it, and returns the directory.
func fakeToolchainSwitch(t *testing.T, root string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "go")
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}
	if os.Link(exe, dst) != nil {
		data, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSGO_TEST_GO_SWITCH", root)
	return dir
}

// The go command of the toolchain the module selects runs with that toolchain's GOROOT, not with another one the user exported, which would give it another standard library (#2460).
func TestAddRunsGoWithItsToolchainsGOROOT(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	root, err := exec.Command(gobin, "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeToolchainSwitch(t, strings.TrimSpace(string(root))))
	// Another toolchain's GOROOT, which the go found on PATH ignores once it switches: one without a standard library.
	t.Setenv("GOROOT", t.TempDir())
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	plan, err := planAdd(t, dir, "example.com/std")
	if err != nil || !strings.Contains(plan, "+ require example.com/std v1.0.0") {
		t.Fatalf("plan %q, %v; want it planned with the toolchain's own standard library", plan, err)
	}
}

// With direct after the proxy in GOPROXY, a module the proxy lacks fails without the go command starting git, which it does for go get itself (#2460).
func TestAddNeverFetchesFromAModulesHost(t *testing.T) {
	proxy := libraryProxy(t)
	offlineAdd(t, proxy)
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	bin, log := fakeGitOnPath(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Dir(gobin))
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy)+",direct")
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})

	// The fake git stands in for the real one: go get itself runs it for a module the proxy lacks.
	cmd := exec.Command("go", "get", "github.com/nsgo-test/missing@upgrade")
	cmd.Dir = t.TempDir()
	writeFiles(t, cmd.Dir, map[string]string{"go.mod": "module example.com/control\n\ngo 1.26\n"})
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("go get of a missing module succeeded:\n%s", out)
	}
	if calls, err := os.ReadFile(log); err != nil || !strings.Contains(string(calls), "git ") {
		t.Fatalf("go get ran no git (%v), so the test proves nothing", err)
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}

	plan, err := planAdd(t, dir, "github.com/nsgo-test/missing")
	if err == nil || !strings.Contains(err.Error(), "GOPROXY=off") {
		t.Errorf("plan %q, error %v; want the module's lookup disabled", plan, err)
	}
	if calls, err := os.ReadFile(log); !os.IsNotExist(err) {
		t.Errorf("nsgo add ran git:\n%s", calls)
	}
}

// Without a terminal and without -y, nothing is written and the exit status is 1, whatever standard input says (#2460).
func TestAddWithoutATerminalWritesNothing(t *testing.T) {
	proxy := libraryProxy(t)
	path, env := goEnv(t, "file://"+filepath.ToSlash(proxy))
	dir := t.TempDir()
	const gomod = "module example.com/tool\n\ngo 1.26\n"
	writeFiles(t, dir, map[string]string{"go.mod": gomod})
	out, code := runNsgo(t, dir, path, env, "add", "example.com/dep")
	if code != 1 || !strings.Contains(out, "+ require example.com/dep v1.0.0") || !strings.Contains(out, "the plan was not applied") || !strings.Contains(out, "add -y example.com/dep") {
		t.Errorf("exit %d, output:\n%s\nwant the plan, then that it was not applied, and exit 1", code, out)
	}
	if got, _ := moduleFiles(t, dir); string(got) != gomod {
		t.Errorf("go.mod changed:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.sum")); !os.IsNotExist(err) {
		t.Errorf("go.sum was written: %v", err)
	}
	// -dry-run prints the plan and exits 0.
	if out, code := runNsgo(t, dir, path, env, "add", "-dry-run", "example.com/dep"); code != 0 || !strings.Contains(out, "+ require example.com/dep v1.0.0") {
		t.Errorf("-dry-run: exit %d\n%s", code, out)
	}
}

// Asked in a terminal, a yes applies the plan and anything else leaves the module as it was (#2460).
func TestAddAsks(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	const gomod = "module example.com/tool\n\ngo 1.26\n"
	writeFiles(t, dir, map[string]string{"go.mod": gomod})
	no := func(context.Context) (bool, error) { return false, nil }
	if _, err := runAdd(t, dir, addOptions{}, no, "example.com/dep"); err == nil || !strings.Contains(err.Error(), "the plan was not applied") {
		t.Errorf("declined: %v", err)
	}
	if got, _ := moduleFiles(t, dir); string(got) != gomod {
		t.Errorf("declined, go.mod changed:\n%s", got)
	}
	yes := func(context.Context) (bool, error) { return true, nil }
	if _, err := runAdd(t, dir, addOptions{}, yes, "example.com/dep"); err != nil {
		t.Fatal(err)
	}
	if got, _ := moduleFiles(t, dir); !strings.Contains(string(got), "example.com/dep v1.0.0") {
		t.Errorf("confirmed, go.mod lacks the module:\n%s", got)
	}
}

// A go.mod or go.sum changed between planning and applying is refused, and nothing is written over it (#2460).
func TestAddRefusesFilesChangedSincePlanning(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	for _, name := range []string{"go.mod", "go.sum"} {
		dir := nsgoApp(t)
		edited := []byte("// edited meanwhile\n")
		edit := func(context.Context) (bool, error) {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return false, err
			}
			return true, os.WriteFile(filepath.Join(dir, name), append(data, edited...), 0o644)
		}
		_, err := runAdd(t, dir, addOptions{}, edit, "example.com/dep")
		if err == nil || !strings.Contains(err.Error(), name+" changed since the plan was made") {
			t.Errorf("%s edited: %v, want a refusal", name, err)
		}
		gomod, gosum := moduleFiles(t, dir)
		if strings.Contains(string(gomod), "example.com/dep") || strings.Contains(string(gosum), "example.com/dep") {
			t.Errorf("%s edited: add wrote the plan:\n%s\n%s", name, gomod, gosum)
		}
		if got := map[string][]byte{"go.mod": gomod, "go.sum": gosum}[name]; !bytes.HasSuffix(got, edited) {
			t.Errorf("%s edited: the edit is gone:\n%s", name, got)
		}
	}
}

// A write that fails after go.mod was replaced puts go.mod and go.sum back as they were (#2460).
func TestAddRestoresAfterAFailedWrite(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	oldMod, oldSum := moduleFiles(t, dir)
	failed := false
	addRename = func(from, to string) error {
		if filepath.Base(to) == "go.sum" && !failed {
			failed = true
			return errors.New("disk full")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { addRename = os.Rename })
	_, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart")
	if err == nil || !strings.Contains(err.Error(), "disk full") || !failed {
		t.Fatalf("add: %v, want the injected failure", err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("go.mod and go.sum after the failure:\n%s\n%s\nwant them as they were:\n%s\n%s", gomod, gosum, oldMod, oldSum)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "go.mod" && e.Name() != "go.sum" {
			t.Errorf("add left %s", e.Name())
		}
	}
}

// In a module under a go.work, the plan says go.mod is planned for the module alone, and applying writes what GOWORK=off go get writes (#2460).
func TestAddInAWorkspace(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.work":      "go 1.27.3\n\nuse (\n\t./tool\n\t./other\n)\n",
		"tool/go.mod":  "module example.com/tool\n\ngo 1.26\n",
		"other/go.mod": "module example.com/other\n\ngo 1.27.3\n",
	})
	// The go command finds the go.work above the module, as it does by default.
	t.Setenv("GOWORK", "")
	dir := filepath.Join(root, "tool")
	plan, err := planAdd(t, dir, "example.com/dep")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	if want := "  workspace  go.mod is planned for this module alone, as GOWORK=off go get writes it, not with ../go.work\n"; !strings.Contains(plan, want) {
		t.Errorf("plan:\n%s\nwant %q", plan, want)
	}
	wantMod, wantSum := goGetWrites(t, dir, "example.com/dep@v1.0.0")
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/dep"); err != nil {
		t.Fatal(err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, wantMod) || !bytes.Equal(gosum, wantSum) {
		t.Errorf("go.mod:\n%s\ngo.sum:\n%s\nwant what GOWORK=off go get writes:\n%s\n%s", gomod, gosum, wantMod, wantSum)
	}
}

// A version typed below the one go.mod has is a downgrade the plan names; a query that resolves below it is refused, naming the version to type (#2460).
func TestAddDowngrades(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n\nrequire example.com/lib v1.3.0-rc.1\n"})
	goIn(t, dir, "get", "example.com/lib@v1.3.0-rc.1")
	plan, err := planAdd(t, dir, "example.com/lib@v1.2.0")
	if err != nil {
		t.Fatalf("typed: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  module     example.com/lib v1.2.0, published 2026-01-01, a downgrade from v1.3.0-rc.1\n",
		"  go.mod     ~ require example.com/lib v1.3.0-rc.1 => v1.2.0, a downgrade\n",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("typed: plan\n%s\nlacks %q", plan, want)
		}
	}
	_, err = planAdd(t, dir, "example.com/lib@latest")
	if want := "example.com/lib@latest is v1.2.0, below the v1.3.0-rc.1 this module has: nsgo add example.com/lib@v1.2.0 lowers it on purpose"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("query: %v, want %q", err, want)
	}
	// Without a version, a module go.mod has is left as it is.
	if plan, err := planAdd(t, dir, "example.com/lib"); err != nil || plan != "example.com/lib v1.3.0-rc.1 is already in go.mod\n" {
		t.Errorf("untyped: %q, %v", plan, err)
	}
}

// A package path is refused, naming its module (#2460).
func TestAddRefusesAPackagePath(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	_, err := planAdd(t, dir, "github.com/acme/media/player@v0.1.0")
	if want := "github.com/acme/media/player is a package of the module github.com/acme/media, not a module: nsgo add github.com/acme/media"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%v, want %q", err, want)
	}
}

func TestParseAddRequests(t *testing.T) {
	got, err := parseAddRequests([]string{"example.com/a", "example.com/b@v1.2.0", "example.com/c@latest", "example.com/d#chat", "example.com/e@v1.0.0#field"})
	want := []addRequest{
		{path: "example.com/a", version: "upgrade"},
		{path: "example.com/b", version: "v1.2.0", typed: true},
		{path: "example.com/c", version: "latest", typed: true},
		{path: "example.com/d", component: "chat"},
		{path: "example.com/e", version: "v1.0.0", typed: true, component: "field"},
	}
	if err != nil || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("parseAddRequests = %v, %v; want %v", got, err, want)
	}
	for _, args := range [][]string{
		nil,
		{"chart"},
		{"example.com/a#"},
		{"example.com/a#Bad_Name"},
		{"example.com/a@none#chat"},
		{"example.com/a@"},
		{"example.com/a@none"},
		{"example.com/a", "-y"},
		{"example.com/a", "example.com/a@v1.0.0"},
		{"example.com/a#chat", "example.com/a@v1.0.0#chat"},
		{"example.com/a b"},
		{"Example.com/a"},
	} {
		if _, err := parseAddRequests(args); err == nil {
			t.Errorf("parseAddRequests(%q) succeeded", args)
		}
	}
}

func TestModuleOwner(t *testing.T) {
	for path, want := range map[string]string{
		"github.com/nsgo-dev/nsgo-media":       "github.com/nsgo-dev",
		"github.com/nsgo-dev/nsgo/v2":          "github.com/nsgo-dev",
		"gitlab.com/group/sub/project":         "gitlab.com/group",
		"golang.org/x/mod":                     "golang.org",
		"go.yaml.in/yaml/v3":                   "go.yaml.in",
		"example.com":                          "example.com",
		"git.sr.ht/~sircmpwn/getopt":           "git.sr.ht/~sircmpwn",
		"bitbucket.org/acme/chart":             "bitbucket.org/acme",
		"codeberg.org/acme/chart":              "codeberg.org/acme",
		"gitee.com/acme/chart":                 "gitee.com/acme",
		"github.com/acme":                      "github.com/acme",
		"nsgo.acme.example/libraries/chart/v3": "nsgo.acme.example",
	} {
		if got := moduleOwner(path); got != want {
			t.Errorf("moduleOwner(%q) = %q, want %q", path, got, want)
		}
	}
}

// The plan labels the modules it fetched from their own hosts, as GONOPROXY or GOPRIVATE marks them, and those whose hashes the checksum database does not check, as GONOSUMDB, GOPRIVATE or GOSUMDB=off says (#2460).
func TestAddPlanLabelsPrivateModules(t *testing.T) {
	p := &addPlan{
		m:      &addModule{sumDB: "sum.golang.org", noSumDB: "example.com/private,*.corp.example", noProxy: "example.com/private"},
		oldSum: []byte("example.com/public v1.0.0 h1:a=\n"),
		newSum: []byte("example.com/public v1.0.0 h1:a=\nexample.com/private/lib v1.0.0 h1:b=\nexample.com/private/lib v1.0.0/go.mod h1:c=\ngit.corp.example/x v1.0.0/go.mod h1:d=\nexample.com/public/two v1.0.0 h1:e=\n"),
	}
	if got, want := p.private(), "example.com/private/lib: from their own hosts, as GONOPROXY or GOPRIVATE says"; got != want {
		t.Errorf("private() = %q, want %q", got, want)
	}
	if got, want := p.checksums(), "example.com/private/lib, git.corp.example/x: unchecked by the checksum database, as GONOSUMDB or GOPRIVATE says"; got != want {
		t.Errorf("checksums() = %q, want %q", got, want)
	}
	p.m = &addModule{sumDB: "sum.golang.org"}
	if got, got2 := p.private(), p.checksums(); got != "" || got2 != "" {
		t.Errorf("no private modules: %q, %q", got, got2)
	}
	p.m.sumDB = "off"
	if got := p.checksums(); !strings.HasPrefix(got, "GOSUMDB=off: ") {
		t.Errorf("GOSUMDB=off: checksums() = %q", got)
	}
	p.newSum = p.oldSum
	if got := p.checksums(); got != "" {
		t.Errorf("no new hashes: checksums() = %q", got)
	}
}

// An interrupt after go.mod was replaced, before go.sum is, puts go.mod back, so the two never disagree (#2460).
func TestAddRestoresAfterAnInterrupt(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	oldMod, oldSum := moduleFiles(t, dir)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	addRename = func(from, to string) error {
		err := os.Rename(from, to)
		if filepath.Base(to) == "go.mod" {
			cancel()
		}
		return err
	}
	t.Cleanup(func() { addRename = os.Rename })
	t.Chdir(dir)
	var err error
	captureStderr(t, func() {
		err = addModules(ctx, addOptions{yes: true}, []string{"github.com/acme/chart"}, io.Discard, nil)
	})
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("add: %v, want it interrupted", err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("go.mod and go.sum after the interrupt:\n%s\n%s\nwant them as they were", gomod, gosum)
	}
}

// An interrupt while planning, or at the question, writes nothing and says so, so the command fails rather than ending as if it had applied the plan (#2460).
func TestAddInterruptedWritesNothing(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	const gomod = "module example.com/tool\n\ngo 1.26\n"
	writeFiles(t, dir, map[string]string{"go.mod": gomod})
	t.Chdir(dir)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var err error
	captureStderr(t, func() { err = addModules(ctx, addOptions{yes: true}, []string{"example.com/dep"}, io.Discard, nil) })
	if err == nil || !strings.Contains(err.Error(), "interrupted while planning, so nothing was written") {
		t.Errorf("interrupted while planning: %v", err)
	}

	// The question reads a standard input nobody answers, and the interrupt comes while it waits.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	stdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin })
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	ask := func(ctx context.Context) (bool, error) {
		cancel()
		return askToApply(ctx)
	}
	captureStderr(t, func() { err = addModules(ctx, addOptions{}, []string{"example.com/dep"}, io.Discard, ask) })
	if err == nil || !strings.Contains(err.Error(), "interrupted, so the plan was not applied") {
		t.Errorf("interrupted at the question: %v", err)
	}
	if got, _ := moduleFiles(t, dir); string(got) != gomod {
		t.Errorf("go.mod changed:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.sum")); !os.IsNotExist(err) {
		t.Errorf("go.sum was written: %v", err)
	}
}

// A module that vendors its dependencies gets go mod vendor as its next step, in the plan and once it is applied (#2460).
func TestAddNamesGoModVendorNext(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n", "vendor/modules.txt": ""})
	plan, err := planAdd(t, dir, "example.com/dep")
	if err != nil || !strings.HasSuffix(plan, "  next       go mod vendor\n") {
		t.Fatalf("plan %q, %v; want go mod vendor next", plan, err)
	}
	t.Chdir(dir)
	var out bytes.Buffer
	stderr := captureStderr(t, func() { err = addModules(t.Context(), addOptions{yes: true}, []string{"example.com/dep"}, &out, nil) })
	if err != nil || !strings.Contains(stderr, "next: go mod vendor") {
		t.Errorf("applied: %v\n%s", err, stderr)
	}
}

// nsgo-module.yaml drives the plan's import lines, the app targets the module lacks, the module's components with their copy commands, and the author's notes printed last and marked (#2462).
func TestAddPlanWithModuleManifest(t *testing.T) {
	proxy := libraryProxy(t)
	addProxyModule(t, proxy, "github.com/acme/av", "v1.0.0",
		"module github.com/acme/av\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n",
		map[string]string{
			"nsgo-module.yaml": `version: 1
import: [player, player/ui]
targets: [macos, windows_x64, linux_x64, ios, ios_simulator]
notes: |
  FFmpeg's libraries are LGPL 2.1: offer their source beside your downloads.
  Background playback on iOS needs apple.info.UIBackgroundModes: [audio].
components:
  controls: ui/controls
  player-bar: ui/playerbar
`,
			"LICENSE":                 licenseFixtureText(t, "MIT"),
			"player/player.go":        "package player\n",
			"player/ui/ui.go":         "package ui\n",
			"ui/controls/controls.go": "package controls\n",
			"ui/playerbar/bar.go":     "package playerbar\n",
		})
	offlineAdd(t, proxy)
	dir := nsgoApp(t)
	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": "name: Hello\nbuild:\n  targets: [macos_arm64, ios, android_arm64, web]\n",
	})

	plan, err := planAdd(t, dir, "github.com/acme/av")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	const want = `github.com/acme/av
  module     github.com/acme/av v1.0.0, published 2026-01-01
  owner      github.com/acme, the first module from it in go.mod
  licenses   github.com/acme/av: MIT
  targets    lacks android_arm64, web
  components controls: nsgo add github.com/acme/av#controls
             player-bar: nsgo add github.com/acme/av#player-bar
  next       import "github.com/acme/av/player"
             import "github.com/acme/av/player/ui"

example.com/app
  go.mod     + require github.com/acme/av v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.3.4 => v0.4.0, a release
  go.sum     + 4 lines
  tools      go tool nsgo then runs v0.4.0, and so does an installed nsgo here
  checksums  GOSUMDB=off: go.sum takes the new hashes unchecked by the checksum database

From github.com/acme/av, the author's notes:
  | FFmpeg's libraries are LGPL 2.1: offer their source beside your downloads.
  | Background playback on iOS needs apple.info.UIBackgroundModes: [audio].
`
	if plan != want {
		t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
	}

	t.Chdir(dir)
	var out bytes.Buffer
	stderr := captureStderr(t, func() {
		err = addModules(t.Context(), addOptions{yes: true}, []string{"github.com/acme/av"}, &out, nil)
	})
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, stderr)
	}
	for _, wantLine := range []string{
		`next: import "github.com/acme/av/player"`,
		`next: import "github.com/acme/av/player/ui"`,
	} {
		if !strings.Contains(stderr, wantLine) {
			t.Errorf("stderr lacks %q:\n%s", wantLine, stderr)
		}
	}
}

// import: [., catalog] prints both the root package and the subpackage, and build-constrained packages validate without failing on another host (#2462).
func TestAddPlanWithModuleManifestDotAndBuildConstrained(t *testing.T) {
	proxy := libraryProxy(t)
	addProxyModule(t, proxy, "example.com/flags", "v1.0.0",
		"module example.com/flags\n\ngo 1.26\n",
		map[string]string{
			"nsgo-module.yaml": "version: 1\nimport: [., catalog]\ncomponents:\n  picker: ui/picker\n",
			"LICENSE":          licenseFixtureText(t, "MIT"),
			"flags.go":         "package flags\n",
			"catalog/cat.go":   "//go:build plan9\n\npackage catalog\n",
			"ui/picker/p.go":   "//go:build plan9\n\npackage picker\n",
		})
	offlineAdd(t, proxy)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
	plan, err := planAdd(t, dir, "example.com/flags")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  components picker: nsgo add example.com/flags#picker\n",
		"  next       import \"example.com/flags\"\n",
		"             import \"example.com/flags/catalog\"\n",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
}

// Invalid nsgo-module.yaml package references (nested modules, main packages, component package name mismatches) and unknown fields in modules requiring a newer NSGo are refused (#2462).
func TestAddModuleManifestRefusals(t *testing.T) {
	proxy := libraryProxy(t)
	addNSGoVersion(t, proxy, "v0.5.0", stubNSGoMod, stubNSGo)
	addProxyModule(t, proxy, "github.com/acme/maincomp", "v1.0.0",
		"module github.com/acme/maincomp\n\ngo 1.26\n",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ncomponents:\n  app: cmd/app\n",
			"cmd/app/main.go":  "package main\n\nfunc main() {}\n",
		})
	addProxyModule(t, proxy, "github.com/acme/badname", "v1.0.0",
		"module github.com/acme/badname\n\ngo 1.26\n",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ncomponents:\n  chat: ui/chat\n",
			"ui/chat/chat.go":  "package chatbox\n",
		})
	addProxyModule(t, proxy, "github.com/acme/mainimp", "v1.0.0",
		"module github.com/acme/mainimp\n\ngo 1.26\n",
		map[string]string{
			"nsgo-module.yaml": "version: 1\nimport: [cmd/cli]\n",
			"cmd/cli/main.go":  "package main\n\nfunc main() {}\n",
		})
	addProxyModule(t, proxy, "github.com/acme/future", "v1.0.0",
		"module github.com/acme/future\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.5.0\n",
		map[string]string{
			"nsgo-module.yaml": "version: 1\nskills: [acme]\n",
			"future.go":        "package future\n",
		})
	addProxyModule(t, proxy, "github.com/acme/badfield", "v1.0.0",
		"module github.com/acme/badfield\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ntypo: true\n",
			"bad.go":           "package badfield\n",
		})
	offlineAdd(t, proxy)

	t.Run("component main package refused", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
		_, err := planAdd(t, dir, "github.com/acme/maincomp")
		if err == nil || !strings.Contains(err.Error(), "main package") {
			t.Fatalf("got %v, want main package refusal", err)
		}
	})

	t.Run("component directory not ending in package name refused", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
		_, err := planAdd(t, dir, "github.com/acme/badname")
		if err == nil || !strings.Contains(err.Error(), `has package name "chatbox"`) {
			t.Fatalf("got %v, want package name mismatch refusal", err)
		}
	})

	t.Run("import main package refused", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"go.mod": "module example.com/tool\n\ngo 1.26\n"})
		_, err := planAdd(t, dir, "github.com/acme/mainimp")
		if err == nil || !strings.Contains(err.Error(), "main package") {
			t.Fatalf("got %v, want main package refusal", err)
		}
	})

	t.Run("nested module import and component refused", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			manifest string
		}{
			{"import", "version: 1\nimport: [nested]\n"},
			{"component", "version: 1\ncomponents:\n  nested: nested\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				local := filepath.Join(t.TempDir(), "outer")
				writeFiles(t, local, map[string]string{
					"go.mod":           "module example.com/outer\n\ngo 1.26\n",
					"outer.go":         "package outer\n",
					"nsgo-module.yaml": tc.manifest,
					"nested/go.mod":    "module example.com/outer/nested\n\ngo 1.26\n",
					"nested/nested.go": "package nested\n",
				})
				dir := t.TempDir()
				writeFiles(t, dir, map[string]string{
					"go.mod": "module example.com/tool\n\ngo 1.26\n\nreplace example.com/outer => " + filepath.ToSlash(local) + "\nreplace example.com/outer/nested => " + filepath.ToSlash(filepath.Join(local, "nested")) + "\n",
				})
				_, err := planAdd(t, dir, "example.com/outer@v1.0.0")
				if err == nil || !strings.Contains(err.Error(), "nested module") {
					t.Fatalf("got %v, want nested module refusal", err)
				}
			})
		}
	})

	t.Run("unknown field in module requiring newer NSGo names that version", func(t *testing.T) {
		prev := moduleManifestNSGoVersion
		moduleManifestNSGoVersion = func() string { return "v0.4.0" }
		t.Cleanup(func() { moduleManifestNSGoVersion = prev })

		dir := nsgoApp(t)
		_, err := planAdd(t, dir, "github.com/acme/future")
		want := "github.com/acme/future needs NSGo v0.5.0: go get github.com/nsgo-dev/nsgo@v0.5.0"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("got %v, want %q", err, want)
		}

		// Same NSGo requirement gets the ordinary unknown-field error.
		_, err = planAdd(t, dir, "github.com/acme/badfield")
		if err == nil || !strings.Contains(err.Error(), `unknown field "typo"`) || strings.Contains(err.Error(), "needs NSGo") {
			t.Fatalf("got %v, want ordinary unknown field error", err)
		}
	})
}
