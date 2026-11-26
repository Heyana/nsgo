package main

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCLIProcess lets the real main reject arguments in a subprocess, before loading a project, opening a browser, building or starting an MCP server.
func TestCLIProcess(t *testing.T) {
	if os.Getenv("NSGO_CLI_PROCESS") != "1" {
		return
	}
	i := slices.Index(os.Args, "--")
	if i < 0 {
		t.Fatal("missing command arguments")
	}
	os.Args = append([]string{"nsgo"}, os.Args[i+1:]...)
	main()
	os.Exit(0)
}

func TestCommandsRefuseTrailingArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"build", "missing-app", "-target", "web"}, "directory: nsgo build -target"},
		{[]string{"dev", "missing-app", "--target=web"}, "directory: nsgo dev -target=web"},
		{[]string{"mcp", "missing-app", "-target", "web"}, "directory: nsgo mcp -target"},
		{[]string{"check", "missing-app", "-target", "all"}, "directory: nsgo check -target"},
		{[]string{"test", "missing-app", "-target", "web"}, "directory: nsgo test -target"},
		{[]string{"test", "missing-app", "--", "-test.target=web"}, "directory: nsgo test -target=web"},
		{[]string{"test", "--", "-target", "web"}, "directory: nsgo test -target"},
		{[]string{"init", "missing-app", "-name", "Name"}, "directory: nsgo init -name"},
		{[]string{"init", "-list", "app"}, "-list takes only -template"},
		{[]string{"init", "-list", "-name", "Name"}, "-list takes only -template"},
		{[]string{"init", "-list", "-module", "example.com/app"}, "-list takes only -template"},
		{[]string{"i18n", "missing-app", "-source", "en"}, "directory: nsgo i18n -source"},
		{[]string{"publish", "missing-app", "-entry", "App.tsx"}, "directory: nsgo publish -entry"},
		{[]string{"publish", "missing-app", "-store", "all"}, "directory: nsgo publish -store"},
		{[]string{"verify", "missing-app", "-root", "key"}, "directory: nsgo verify -root"},
		{[]string{"build", "first", "second"}, "takes one path"},
		{[]string{"dev", "first", "second"}, "takes one path"},
		{[]string{"mcp", "first", "second"}, "takes one path"},
		{[]string{"check", "first", "second"}, "takes one path"},
		{[]string{"test", "first", "second"}, "takes one directory"},
		{[]string{"doctor", "extra"}, "takes no positional arguments"},
		{[]string{"certify", "extra"}, "takes no positional arguments"},
		{[]string{"revoke", "extra"}, "takes no positional arguments"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := exec.Command(exe, append([]string{"-test.run=^TestCLIProcess$", "--"}, tc.args...)...)
			cmd.Dir = t.TempDir()
			// NSGO_DELEGATED=1 names the command nsgo wherever the test binary lives.
			cmd.Env = append(os.Environ(), "NSGO_CLI_PROCESS=1", "NSGO_DELEGATED=1")
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("exit %v, output %s; want a failure containing %q", err, out, tc.want)
			}
		})
	}
}

// The flags-order error names the command as it was called: go tool nsgo for the module's tool, nsgo for an installed one (#2420).
func TestMisplacedFlagsNameTheCommand(t *testing.T) {
	t.Setenv("NSGO_DELEGATED", "")
	t.Cleanup(func() { executable = os.Executable })
	for exe, want := range map[string]string{
		filepath.Join("/home/ada/.cache/go-build", "ab", strings.Repeat("ab", 32)+"-d", "nsgo"): "build: flags go before the directory: go tool nsgo build -target \"web\" \"app\"",
		filepath.Join("/home/ada/.local/bin", "nsgo"):                                           "build: flags go before the directory: nsgo build -target \"web\" \"app\"",
	} {
		executable = func() (string, error) { return exe, nil }
		fs := flag.NewFlagSet("build", flag.ContinueOnError)
		fs.String("target", "", "")
		if err := misplacedFlags(fs, "app", []string{"-target", "web"}, nil); err == nil || err.Error() != want {
			t.Errorf("from %s: %v, want %q", exe, err, want)
		}
	}
}

func TestTestArgumentsKeepGoTestFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		dir  string
		rest []string
	}{
		{[]string{"-target", "web", "app", "--", "-short"}, "app", []string{"-short"}},
		{[]string{"--", "-short", "-count=2"}, "", []string{"-short", "-count=2"}},
		{[]string{"app", "--", "-short"}, "app", []string{"-short"}},
		{[]string{"app", "-count", "2"}, "app", []string{"-count", "2"}},
		{[]string{"app", "--", "-args", "-target", "custom"}, "app", []string{"-args", "-target", "custom"}},
		{[]string{"app", "--", "-update=--target=custom"}, "app", []string{"-update=--target=custom"}},
		// go test owns these too, so after the directory they are its flags, not misplaced ones.
		{[]string{"--", "-timeout=20m", "-run", "TestOne"}, "", []string{"-timeout=20m", "-run", "TestOne"}},
		{[]string{"app", "-test.bench=.", "-trace", "trace.out"}, "app", []string{"-test.bench=.", "-trace", "trace.out"}},
		{[]string{"-ldflags", "-X a.b=c", "app", "-ldflags", "-X a.b=d"}, "app", []string{"-ldflags", "-X a.b=d"}},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.String("target", "", "")
		fs.String("run", "", "")
		fs.Duration("timeout", 0, "")
		fs.String("bench", "", "")
		fs.Bool("trace", false, "")
		fs.String("ldflags", "", "")
		if err := parseCommand(fs, tc.args, -1); err != nil {
			t.Fatal(err)
		}
		if dir, rest := splitTestArgs(fs.Args()); dir != tc.dir || !slices.Equal(rest, tc.rest) {
			t.Errorf("%q: dir=%q flags=%q, want %q %q", tc.args, dir, rest, tc.dir, tc.rest)
		}
	}
}

func TestHeadlessCLIFlagsWithoutDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module headlessflags\n\ngo 1.26\n",
		"flags_test.go": `package fixture
import ("os"; "testing")
func TestShort(t *testing.T) {
 if !testing.Short() { t.Fatal("short was not forwarded") }
 if err := os.WriteFile(os.Getenv("NSGO_CLI_MARK"), []byte("short"), 0600); err != nil { t.Fatal(err) }
}
`,
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(dir, "mark")
	cmd := exec.Command(exe, "-test.run=^TestCLIProcess$", "--", "test", "--", "-short", "-count=1", "-timeout=5m", "-run=^TestShort$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NSGO_CLI_PROCESS=1", "NSGO_CLI_MARK="+mark)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test -- -short -timeout -run: %v\n%s", err, out)
	}
	if got, err := os.ReadFile(mark); err != nil || string(got) != "short" {
		t.Fatalf("headless test did not run with -short: %q, %v", got, err)
	}
}

// TestHeadlessCLILdflags: test's -ldflags, repeated, adds to nsgo.yaml's build.ldflags in the test binary.
func TestHeadlessCLILdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test command")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":    "module headlessldflags\n\ngo 1.26\n",
		"nsgo.yaml": "name: Stamp\nbuild:\n  ldflags:\n    - -X headlessldflags.fromYAML=yaml\n",
		"stamp.go":  "package stamp\n\nvar fromYAML, fromFlag, fromSecond string\n",
		"stamp_test.go": `package stamp
import ("os"; "testing")
func TestStamp(t *testing.T) {
 if err := os.WriteFile(os.Getenv("NSGO_CLI_MARK"), []byte(fromYAML+"|"+fromFlag+"|"+fromSecond), 0600); err != nil { t.Fatal(err) }
}
`,
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(dir, "mark")
	cmd := exec.Command(exe, "-test.run=^TestCLIProcess$", "--", "test", "-ldflags", "-X 'headlessldflags.fromFlag=two words'", "-ldflags=-X headlessldflags.fromSecond=2", "--", "-count=1")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NSGO_CLI_PROCESS=1", "NSGO_CLI_MARK="+mark)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("test -ldflags: %v\n%s", err, out)
	}
	if got, err := os.ReadFile(mark); err != nil || string(got) != "yaml|two words|2" {
		t.Fatalf("the test binary's variables: %q, %v; want yaml|two words|2", got, err)
	}
}

func TestTargetTestFlagsNeverDisappear(t *testing.T) {
	f, err := parseTargetTestFlags([]string{"-short", "-test.count", "2", "--test.cpu=1,2", "-parallel=3", "-fullpath=false", "-failfast"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-test.count=2", "-test.cpu=1,2", "-test.failfast=true", "-test.fullpath=false", "-test.parallel=3", "-test.short=true"}
	if !slices.Equal(f.args, want) || f.count != 2 || !f.failfast {
		t.Fatalf("flags=%+v, want %q with count=2 and failfast", f, want)
	}
	for _, args := range [][]string{
		{"-race"}, {"-tags=extra"}, {"-shuffle=on"}, {"-run=TestOne"}, {"-test.timeout=1s"},
		{"-skip=TestOne"}, {"-args", "-custom=one"}, {"-coverprofile=cover.out"}, {"-v=false"},
		{"-short=maybe"}, {"-count"}, {"-count=x"}, {"-parallel=0"}, {"-cpu=1,0"},
	} {
		// A nil project proves the error precedes catalogs, listing, building and device discovery on every target.
		for _, target := range []string{"macos", "linux", "windows", "web", "ios", "tvos_simulator", "watchos_simulator", "android", "harmonyos"} {
			if err := runTests(t.Context(), nil, testOptions{target: target, args: args}); err == nil || !strings.Contains(err.Error(), "test:") {
				t.Errorf("%s %q: %v, want an explicit argument error", target, args, err)
			}
		}
	}
}

// The test command registers -json itself, so nsgo test -json <dir> parses before the directory is split off (#2273).
func TestTestCommandRegistersJSON(t *testing.T) {
	var o testOptions
	fs := testFlagSet(&o)
	if err := parseCommand(fs, []string{"-json", "app"}, -1); err != nil {
		t.Fatal(err)
	}
	if !o.jsonOutput {
		t.Fatal("-json did not reach the test options")
	}
	if dir, rest := splitTestArgs(fs.Args()); dir != "app" || len(rest) != 0 {
		t.Fatalf("dir=%q flags=%q, want app and no forwarded flags", dir, rest)
	}
}
