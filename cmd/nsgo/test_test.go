package main

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"flag"
	"fmt"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSplitTestRun(t *testing.T) {
	for _, tc := range []struct {
		pattern, top, sub string
	}{
		{"TestGallery/dark-home", "TestGallery", "dark-home"},
		{"TestGallery/[dark/home]", "TestGallery", "[dark/home]"},
		{"TestGallery/(dark/home)", "TestGallery", "(dark/home)"},
		{"TestGallery", "TestGallery", ""},
	} {
		if top, sub := splitTestRun(tc.pattern); top != tc.top || sub != tc.sub {
			t.Errorf("splitTestRun(%q) = %q, %q; want %q, %q", tc.pattern, top, sub, tc.top, tc.sub)
		}
	}
	for pattern, want := range map[string]int{"": 0, "TestGallery": 0, "TestGallery/dark-home": 1, "TestGallery/[dark/home]": 1, "TestGallery/a/b": 2, "TestA|TestB/(x/y)/z": 2} {
		if got := runDepth(pattern); got != want {
			t.Errorf("runDepth(%q) = %d, want %d", pattern, got, want)
		}
	}
}

func TestTargetTestArgs(t *testing.T) {
	if got := targetTestArgs("TestGallery", "TestGallery/dark-home", "ios_simulator", 15*time.Minute); !slices.Equal(got,
		[]string{"-test.run", "^TestGallery$/dark-home", "-test.v=test2json", "-test.timeout", "14m59s"}) {
		t.Errorf("targetTestArgs = %q", got)
	}
	if got := targetTestArgs("TestGallery", "", "web", 0); !slices.Equal(got, []string{"-test.run", "^TestGallery$", "-test.v"}) {
		t.Errorf("Web targetTestArgs = %q", got)
	}
}

func TestParseTargetSubtestOutput(t *testing.T) {
	got := parseTestOutput("TestGallery", 1, "--- PASS: TestGallery/dark-home (0.2s)\n--- PASS: TestGallery (0.2s)\n", 0, nil)
	if got.Name != "TestGallery/dark-home" || got.Status != "pass" {
		t.Fatalf("result = %+v, want the selected subtest", got)
	}
}

func TestParseTargetResultAfterNativeOutput(t *testing.T) {
	out := "\x16=== RUN   TestVideo\nunrecognised property key\x16--- PASS: TestVideo (0.2s)\n\x16PASS\n"
	got := parseTestOutput("TestVideo", 0, out, 0, nil)
	if got.Status != "pass" || got.Elapsed != 200*time.Millisecond || strings.Contains(got.Output, "\x16") {
		t.Fatalf("result = %+v, want the marked pass without control bytes", got)
	}
}

func TestFailedTargetKeepsItsProcessError(t *testing.T) {
	output := "=== RUN   TestCrash\n--- FAIL: TestCrash (0.00s)\npanic: exit fixture\nstack trace\n"
	why := "the test process ended without its exit code (status 2)"
	result := parseTestOutput("TestCrash", 0, output, -1, fmt.Errorf("%s", why))
	if result.Status != "fail" || !strings.Contains(result.Output, output) || !strings.Contains(result.Output, why) {
		t.Fatalf("process failure lost after the FAIL line: %+v", result)
	}
}

func TestRepeatedTargetResults(t *testing.T) {
	for _, tc := range []struct {
		output, want string
		code         int
	}{
		{"--- SKIP: TestOne (0.1s)\n--- PASS: TestOne (0.2s)\n", "pass", 0},
		{"--- PASS: TestOne (0.1s)\n--- SKIP: TestOne (0.2s)\n", "pass", 0},
		{"--- SKIP: TestOne (0.1s)\n--- FAIL: TestOne (0.2s)\n", "fail", 1},
		{"--- FAIL: TestOne (0.1s)\n--- PASS: TestOne (0.2s)\n", "fail", 1},
		{"--- SKIP: TestOne (0.1s)\nprocess failed during teardown\n", "fail", 1},
	} {
		got := parseTestOutput("TestOne", 0, tc.output, tc.code, nil)
		if got.Status != tc.want {
			t.Errorf("%q (exit %d): %s, want %s", tc.output, tc.code, got.Status, tc.want)
		}
		if tc.code == 0 && got.Elapsed != 300*time.Millisecond {
			t.Errorf("repeated elapsed=%v, want 300ms", got.Elapsed)
		}
	}
	got := parseTestOutput("TestOne", 1, "    --- PASS: TestOne/selected (0.1s)\n--- PASS: TestOne (0.2s)\n", 0, nil)
	if got.Name != "TestOne/selected" || got.Status != "pass" {
		t.Fatalf("indented subtest=%+v", got)
	}
	// A test with several subtests is reported as itself, not as its first subtest, failing when any subtest failed.
	got = parseTestOutput("TestOne", 0, "    --- PASS: TestOne/a (0.1s)\n    --- FAIL: TestOne/b (0.1s)\n--- FAIL: TestOne (0.2s)\n", 1, nil)
	if got.Name != "TestOne" || got.Status != "fail" || got.Elapsed != 200*time.Millisecond {
		t.Fatalf("several subtests=%+v", got)
	}
	got = parseTestOutput("TestOne", 0, "    --- PASS: TestOne/a (0.1s)\n    --- PASS: TestOne/b (0.1s)\n--- PASS: TestOne (0.2s)\n", 0, nil)
	if got.Name != "TestOne" || got.Status != "pass" {
		t.Fatalf("several passing subtests=%+v", got)
	}
	// One subtest that passed does not hide a failure of its parent.
	got = parseTestOutput("TestOne", 1, "    --- PASS: TestOne/a (0.1s)\n--- FAIL: TestOne (0.2s)\n", 1, nil)
	if got.Name != "TestOne" || got.Status != "fail" {
		t.Fatalf("failed parent=%+v", got)
	}
	// A nested selection, -run TestOne/a/b, reports its deepest subtest; a failure outside it still reports the parent.
	got = parseTestOutput("TestOne", 2, "        --- FAIL: TestOne/a/b (0.1s)\n    --- FAIL: TestOne/a (0.1s)\n--- FAIL: TestOne (0.2s)\n", 1, nil)
	if got.Name != "TestOne/a/b" || got.Status != "fail" {
		t.Fatalf("nested subtest=%+v", got)
	}
	got = parseTestOutput("TestOne", 2, "        --- PASS: TestOne/a/b (0.1s)\n    --- FAIL: TestOne/a (0.1s)\n--- FAIL: TestOne (0.2s)\n", 1, nil)
	if got.Name != "TestOne" || got.Status != "fail" {
		t.Fatalf("nested subtest under a failed parent=%+v", got)
	}
	// The selection reports the subtest -run names, not its own lone subtest, and a whole test with one subtest reports itself.
	got = parseTestOutput("TestOne", 1, "        --- PASS: TestOne/a/leaf (0.1s)\n    --- PASS: TestOne/a (0.1s)\n--- PASS: TestOne (0.2s)\n", 0, nil)
	if got.Name != "TestOne/a" || got.Status != "pass" {
		t.Fatalf("selected parent of a lone subtest=%+v", got)
	}
	got = parseTestOutput("TestOne", 0, "    --- PASS: TestOne/a (0.1s)\n--- PASS: TestOne (0.2s)\n", 0, nil)
	if got.Name != "TestOne" || got.Status != "pass" {
		t.Fatalf("whole test with one subtest=%+v", got)
	}
}

// TestSelectedSubtestCounts checks that a target run whose only result is a selected subtest, alone or as one of a host-linked test's repetitions, counts it in the tally and fails the run when it failed.
func TestSelectedSubtestCounts(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		var out bytes.Buffer
		r := &testRun{out: &out}
		for i, status := range []string{"PASS", "FAIL"} {
			code := 0
			if status == "FAIL" {
				code = 1
			}
			res := parseTestOutput("TestSubs", 1, "    --- "+status+": TestSubs/b (0.1s)\n--- "+status+": TestSubs (0.1s)\n", code, nil)
			if repeated {
				res.Name = fmt.Sprintf("%s [run %d]", res.Name, i+1)
			}
			r.results = append(r.results, res)
		}
		err := r.finish(testOptions{}, nil)
		if err == nil || err.Error() != "1 test failed" || !strings.Contains(out.String(), "1 passed, 1 failed, 0 skipped") {
			t.Errorf("repeated=%v: %v, tally %q", repeated, err, out.String())
		}
	}
}

func TestTreeOf(t *testing.T) {
	for in, want := range map[string]string{
		"":              "",
		".":             ".",
		"...":           ".",
		"./...":         ".",
		"./core/...":    filepath.FromSlash("./core"),
		"examples/tsx":  "examples/tsx",
		"/abs/pkg/...":  filepath.FromSlash("/abs/pkg"),
		"/...":          "/",
		"App.tsx":       "App.tsx",
		"core/x...y":    "core/x...y",
		"core/...extra": "core/...extra",
	} {
		if got := treeOf(in); got != want {
			t.Errorf("treeOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPackageArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"./other"}, []string{"./other"}},
		{[]string{"./a", "./b"}, []string{"./a", "./b"}},
		{[]string{"github.com/example/app"}, []string{"github.com/example/app"}},
		{[]string{"-run", "TestOne"}, nil},
		{[]string{"-run=TestOne"}, nil},
		{[]string{"--run", "TestOne"}, nil},
		{[]string{"-count", "1"}, nil},
		{[]string{"-shuffle", "on", "-cpuprofile", "cpu.out", "-exec", "wrapper", "-tags", "x"}, nil},
		{[]string{"-test.count", "2", "-test.cpuprofile", "cpu.out"}, nil},
		// A flag of the test binary's own takes the next argument, as go test assumes.
		{[]string{"-update", "golden"}, nil},
		{[]string{"-update=true", "./b"}, []string{"./b"}},
		{[]string{"-v", "./b"}, []string{"./b"}},
		{[]string{"-race", "-short", "-benchmem", "-test.v", "./b"}, []string{"./b"}},
		{[]string{"-v", "-run", "X", "./b"}, []string{"./b"}},
		{[]string{"-args", "./not-a-package"}, nil},
		{[]string{"--args", "./not-a-package"}, nil},
		{[]string{"-v", "--", "./not-a-package"}, nil},
	} {
		if got := packageArgs(tc.args); !slices.Equal(got, tc.want) {
			t.Errorf("packageArgs(%q) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

// TestRunTestsKeepsTheTreeWithFlags: go test flags after the directory, whose values are not packages, and a test binary's flags still run every package under it, the one in a subdirectory too.
func TestRunTestsKeepsTheTreeWithFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	dir := t.TempDir()
	const update = "import (\n\t\"flag\"\n\t\"os\"\n\t\"testing\"\n)\n\nvar update = flag.String(\"update\", \"\", \"\")\n\n"
	for name, content := range map[string]string{
		"go.mod":          "module treetest\n\ngo 1.26\n",
		"root_test.go":    "package root\n\n" + update + "func TestRoot(t *testing.T) { _ = os.Getenv }\n",
		"sub/sub_test.go": "package sub\n\n" + update + "func TestSub(t *testing.T) {\n\tif err := os.WriteFile(os.Getenv(\"TREE_MARK\"), []byte(*update), 0o644); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, args := range [][]string{
		{"-count", "1"},
		{"-count=1", "-update", "golden"},
		{"-count=1", "-args", "-update=golden"},
		{"-count=1", "-skip", "-fuzz=not-a-flag", "-args", "--", "-test.fuzz=not-a-go-flag"},
	} {
		mark := filepath.Join(t.TempDir(), "mark")
		if err := runTests(t.Context(), p, testOptions{args: args, env: []string{"TREE_MARK=" + mark}}); err != nil {
			t.Errorf("%d: runTests with %q: %v", i, args, err)
		}
		if _, err := os.Stat(mark); err != nil {
			t.Errorf("%d: runTests with %q did not run sub's test: %v", i, args, err)
		}
	}
}

func TestTestFuzzEnabled(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"-fuzz", "FuzzX"}, true},
		{[]string{"--fuzz=FuzzX"}, true},
		{[]string{"--test.fuzz", "FuzzX"}, true},
		{[]string{"-test.fuzz=FuzzX"}, true},
		{[]string{"-fuzz="}, false},
		{[]string{"-fuzz", "FuzzX", "--test.fuzz="}, false},
		{[]string{"-run", "-fuzz=literal"}, false},
		{[]string{"-ldflags", "-test.fuzz=literal"}, false},
		{[]string{"-test.run", "-args", "-fuzz=FuzzX"}, true},
		{[]string{"-v", "-short", "-custom", "-fuzz=FuzzX"}, true},
		{[]string{"-args", "-fuzz=FuzzX"}, false},
		{[]string{"--args", "-test.fuzz=FuzzX"}, false},
		{[]string{"--", "-fuzz=FuzzX"}, false},
		{[]string{"-fuzz=FuzzX", "-args", "-test.fuzz="}, true},
		{[]string{"-fuzztime=3s", "-fuzzminimizetime=1x"}, false},
	} {
		if got := testFuzzEnabled(c.args); got != c.want {
			t.Errorf("testFuzzEnabled(%q) = %t, want %t", c.args, got, c.want)
		}
	}
}

func TestRunTestsFuzzesOnlyNamedPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	dir := t.TempDir()
	// The named package has a child and is also eligible for the extra dev-client run.
	writeFiles(t, dir, map[string]string{
		"go.mod":              "module " + nsgoModule + "\n\ngo 1.26\n",
		"internal/dev/dev.go": "package dev\n",
		"internal/dev/fuzz_test.go": `//go:build nsgocheck

package dev

import ("os"; "testing")

func FuzzX(f *testing.F) {
	// Without an explicit seed, the body only runs when Go actually fuzzes.
	f.Fuzz(func(t *testing.T, _ string) {
		if err := os.WriteFile(os.Getenv("FUZZ_MARK"), []byte("fuzzed"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
}
`,
		"internal/dev/sub/sub_test.go": `package sub

import "testing"

func TestSub(t *testing.T) { t.Fatal("fuzz run included a subpackage") }
`,
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(dir, "fuzzed")
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestCLIProcess$", "--", "test", "./internal/dev",
		"-fuzz", "^FuzzX$", "-fuzztime=16x", "-parallel=1", "-timeout=1m")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "NSGO_CLI_PROCESS=1", "FUZZ_MARK="+mark)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("headless fuzz: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "testing the dev client with nsgodev") {
		t.Errorf("fuzz run repeated the dev-client tests:\n%s", out)
	}
	if got, err := os.ReadFile(mark); err != nil || string(got) != "fuzzed" {
		t.Fatalf("the nsgocheck fuzz target did not run: %q, %v\n%s", got, err, out)
	}
}

func TestTargetTestsRejectFuzzFlags(t *testing.T) {
	for _, args := range [][]string{{"-fuzz", "FuzzX"}, {"-test.fuzz=FuzzX"}} {
		err := runTests(t.Context(), nil, testOptions{target: "web", args: args})
		if err == nil || !strings.Contains(err.Error(), "target runs cannot accept") {
			t.Errorf("target flags %q: %v, want an explicit argument error", args, err)
		}
	}
}

// Repository tests must exercise the dev-only code as well as the normal build, under their own suite, from the root and from the dev client's directory alike, and a dev-only failure must fail the command and its combined report.
func TestRunTestsIncludesTheDevClient(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test command")
	}
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module " + nsgoModule + "\n\ngo 1.26\n",
		"root.go": `package nsgo
func Root() int { return 1 }
`,
		"root_test.go": `package nsgo
import ("flag"; "os"; "testing")
var tags = flag.String("tags", "", "")
func TestFixtureRoot(t *testing.T) {
	if *tags != os.Getenv("NSGO_FIXTURE_ARG") { t.Fatalf("binary flag = %q", *tags) }
	if Root() != 1 { t.Fatal("Root") }
}
`,
		"internal/dev/base_test.go": `package dev
import ("flag"; "os"; "testing")
var tags = flag.String("tags", "", "")
func TestFixtureBase(t *testing.T) {
	if *tags != os.Getenv("NSGO_FIXTURE_ARG") { t.Fatalf("binary flag = %q", *tags) }
}
`,
		"internal/dev/client.go": `//go:build nsgodev

package dev
func Client() int { return 1 }
`,
		"internal/dev/client_test.go": `//go:build nsgodev

package dev
import ("os"; "testing")
func TestFixtureDev(t *testing.T) {
	if os.Getenv("NSGO_FIXTURE_FAIL") == "1" || Client() != 1 { t.Fatal("dev assertion failed") }
}
`,
		// A command-line build tag must reach the dev run too.
		"internal/dev/tagged_test.go": `//go:build nsgodev && fixture

package dev
import "testing"
func TestFixtureTagged(t *testing.T) {}
`,
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const (
		root      = "root TestFixtureRoot"
		base      = "internal/dev TestFixtureBase"
		devBase   = "internal/dev [nsgodev] TestFixtureBase"
		dev       = "internal/dev [nsgodev] TestFixtureDev"
		devTagged = "internal/dev [nsgodev] TestFixtureTagged"
		plainDev  = "internal/dev TestFixtureDev"
	)
	both := []string{root, base, devBase, dev}
	for _, c := range []struct {
		name, sub, tags, run string
		patterns, args       []string
		fail                 bool
		want                 []string
	}{
		{"normal", "", "", "", nil, nil, false, both},
		{"dev failure", "", "", "", nil, nil, true, both},
		// A failing ordinary run, as go test's failing package, does not keep the dev client's tests from running.
		{"ordinary failure", "", "", "", nil, []string{"-args", "-tags", "wrong"}, true, both},
		{"filtered", "", "", "^TestFixtureRoot$", nil, nil, true, []string{root}},
		{"root pattern skips dev client", "", "", "", []string{"."}, nil, false, []string{root}},
		{"internal pattern runs dev client", "", "", "", []string{"./internal/..."}, nil, false, []string{base, devBase, dev}},
		{"environment tag", "", "fixture", "", nil, nil, false, append(both, devTagged)},
		{"command-line tag", "", "", "", nil, []string{"-tags", "fixture"}, false, append(both, devTagged)},
		{"override dev tag", "", "nsgodev", "", nil, []string{"-tags=fixture"}, false, append(both, devTagged)},
		{"test binary tags", "", "", "", nil, []string{"-args", "-tags", "literal"}, false, both},
		{"already dev", "", "nsgodev", "", nil, nil, false, []string{root, base, plainDev}},
		{"already dev on command line", "", "", "", nil, []string{"--tags=nsgodev"}, false, []string{root, base, plainDev}},
		{"dev client directory", "internal/dev", "", "", nil, nil, false, []string{base, devBase, dev}},
		{"dev client directory failure", "internal/dev", "", "", nil, nil, true, []string{base, devBase, dev}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := loadForTest(filepath.Join(dir, filepath.FromSlash(c.sub)), nil)
			if err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(t.TempDir(), "tests.xml")
			binaryTag := ""
			if slices.Contains(c.args, "-args") {
				binaryTag = "literal"
			}
			err = runTests(t.Context(), p, testOptions{run: c.run, junit: report, patterns: c.patterns,
				env:  []string{"GOWORK=off", "GOFLAGS=-tags=" + c.tags, "NSGO_FIXTURE_ARG=" + binaryTag, "NSGO_FIXTURE_FAIL=" + map[bool]string{false: "0", true: "1"}[c.fail]},
				args: append([]string{"-count=1"}, c.args...)})
			wantFail := c.fail && c.run == ""
			if (err != nil) != wantFail {
				t.Errorf("runTests = %v, want failure %v", err, wantFail)
			}
			suites := readJUnitFixture(t, report)
			var got []string
			for _, suite := range suites.Suites {
				pkg := strings.TrimPrefix(strings.TrimPrefix(suite.Name, nsgoModule), "/")
				if pkg == "" {
					pkg = "root"
				}
				for _, test := range suite.Cases {
					got = append(got, pkg+" "+test.Name)
				}
			}
			slices.Sort(got)
			want := slices.Sorted(slices.Values(c.want))
			if !slices.Equal(got, want) {
				t.Errorf("reported tests = %q, want %q", got, want)
			}
			if (suites.Failures > 0) != wantFail {
				t.Errorf("report has %d failures, want failure %v", suites.Failures, wantFail)
			}
		})
	}

	// Both runs' coverage reaches the profile -coverprofile names, read against -outputdir as go test reads it.
	t.Run("coverage", func(t *testing.T) {
		p, err := loadForTest(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := t.TempDir()
		if err := runTests(t.Context(), p, testOptions{env: []string{"GOWORK=off", "GOFLAGS=", "NSGO_FIXTURE_ARG=", "NSGO_FIXTURE_FAIL=0"},
			args: []string{"-count=1", "-coverprofile", "cover.out", "-outputdir", out}}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(out, "cover.out"))
		if err != nil {
			t.Fatal(err)
		}
		profile := string(data)
		if n := strings.Count(profile, "mode: "); n != 1 || !strings.HasPrefix(profile, "mode: ") {
			t.Errorf("profile has %d mode lines, want one at the start:\n%s", n, profile)
		}
		for _, file := range []string{nsgoModule + "/root.go:", nsgoModule + "/internal/dev/client.go:"} {
			if !strings.Contains(profile, file) {
				t.Errorf("profile lacks %s:\n%s", file, profile)
			}
		}
	})
}

func readJUnitFixture(t *testing.T, path string) junitSuites {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var suites junitSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatal(err)
	}
	return suites
}

func TestTestTagArgs(t *testing.T) {
	dir, abs := filepath.FromSlash("/repo"), t.TempDir()
	for _, c := range []struct {
		tags, args, wantTags, wantFlags []string
		wantCover                       string
	}{
		{[]string{"a"}, nil, []string{"a"}, nil, ""},
		{[]string{"a"}, []string{"-tags", "b,c", "-v"}, []string{"b", "c"}, []string{"-v"}, ""},
		{nil, []string{"-tags=b", "-count", "1", "--tags", "c d", "-race"}, []string{"c", "d"}, []string{"-count", "1", "-race"}, ""},
		{nil, []string{"-run", "X", "-args", "-tags", "x"}, nil, []string{"-run", "X", "-args", "-tags", "x"}, ""},
		{nil, []string{"-coverprofile", "c.out", "-v"}, nil, []string{"-v"}, filepath.Join(dir, "c.out")},
		{nil, []string{"-test.coverprofile=c.out", "-outputdir", "out"}, nil, []string{"-outputdir", "out"}, filepath.Join(dir, "out", "c.out")},
		{nil, []string{"-outputdir=" + abs, "-coverprofile=c.out"}, nil, []string{"-outputdir=" + abs}, filepath.Join(abs, "c.out")},
	} {
		tags, flags, _, cover := testTagArgs(dir, c.tags, c.args)
		if !slices.Equal(tags, c.wantTags) || !slices.Equal(flags, c.wantFlags) || cover != c.wantCover {
			t.Errorf("testTagArgs(%q, %q) = %q, %q, %q, want %q, %q, %q", c.tags, c.args, tags, flags, cover, c.wantTags, c.wantFlags, c.wantCover)
		}
	}
}

// Headless tests build with nsgocheck, which keeps core's ownership checks, whether or not the command line or GOFLAGS name tags of their own.
func TestRunTestsBuildWithChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":          "module checkedtest\n\ngo 1.26\n",
		"checked_test.go": "//go:build nsgocheck\n\npackage checked\n\nimport \"testing\"\n\nfunc TestChecked(t *testing.T) {}\n",
		"plain_test.go":   "//go:build !nsgocheck\n\npackage checked\n\nimport \"testing\"\n\nfunc TestPlain(t *testing.T) { t.Fatal(\"built without nsgocheck\") }\n",
		"tagged_test.go":  "//go:build fixture\n\npackage checked\n\nimport \"testing\"\n\nfunc TestTagged(t *testing.T) {}\n",
	})
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		goflags string
		args    []string
		want    []string
	}{
		{"", nil, []string{"TestChecked"}},
		{"", []string{"-tags", "fixture"}, []string{"TestChecked", "TestTagged"}},
		{"-tags=fixture", nil, []string{"TestChecked", "TestTagged"}},
		{"-tags=fixture", []string{"-tags=other"}, []string{"TestChecked"}},
	} {
		report := filepath.Join(t.TempDir(), "tests.xml")
		err := runTests(t.Context(), p, testOptions{junit: report, env: []string{"GOWORK=off", "GOFLAGS=" + c.goflags}, args: append([]string{"-count=1"}, c.args...)})
		if err != nil {
			t.Errorf("GOFLAGS %q, args %q: runTests = %v", c.goflags, c.args, err)
		}
		var got []string
		for _, suite := range readJUnitFixture(t, report).Suites {
			for _, test := range suite.Cases {
				got = append(got, test.Name)
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("GOFLAGS %q, args %q: ran %q, want %q", c.goflags, c.args, got, c.want)
		}
	}
}

func TestRunTestsRejectsAPackageAmongItsFlags(t *testing.T) {
	p, err := loadForTest(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"", "web"} {
		err := runTests(t.Context(), p, testOptions{target: target, args: []string{"-v", "./b"}})
		if err == nil || !strings.Contains(err.Error(), `takes package patterns before its flags, not "./b" after them`) {
			t.Errorf("target %q: runTests = %v, want an error naming ./b", target, err)
		}
	}
}

func TestForwardedTestFlagsDoNotNameADirectory(t *testing.T) {
	for _, tc := range []struct {
		in       []string
		patterns []string
		args     []string
	}{
		{[]string{"--", "-timeout=20m"}, nil, []string{"-timeout=20m"}},
		{[]string{".", "--", "-timeout=20m"}, []string{"."}, []string{"-timeout=20m"}},
		{[]string{"./core"}, []string{"./core"}, nil},
		{[]string{".", "./cmd/...", "./ui/...", "--", "-short"}, []string{".", "./cmd/...", "./ui/..."}, []string{"-short"}},
		{[]string{"App.tsx", "--", "-count=1"}, []string{"App.tsx"}, []string{"-count=1"}},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		if err := fs.Parse(tc.in); err != nil {
			t.Fatal(err)
		}
		patterns, args := splitTestArgs(fs.Args())
		if !slices.Equal(patterns, tc.patterns) || !slices.Equal(args, tc.args) {
			t.Errorf("args %q: patterns=%q, forwarded=%q; want %q, %q", tc.in, patterns, args, tc.patterns, tc.args)
		}
	}
}

func TestLoadForTestTakesBareTrees(t *testing.T) {
	dir := t.TempDir()
	p, err := loadForTest(filepath.Join(dir, "..."), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.bare || p.Dir != dir || p.ID != generatedID(filepath.Base(dir)) || !p.idDefault {
		t.Fatalf("loadForTest = %+v, want a bare project with generated ID at %s", p, dir)
	}
	if _, err := loadForTest(filepath.Join(dir, "missing"), nil); err == nil {
		t.Fatal("loadForTest took a missing directory")
	}
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, err = loadForTest(dir, nil); err != nil || p.bare || p.Name != "App" {
		t.Fatalf("loadForTest = %+v, %v, want the app", p, err)
	}
}

// TestBenchmarkResults feeds the events go test -json prints for
// benchmarks, which end with no event of their own, one result split
// over two lines of output.
func TestBenchmarkResults(t *testing.T) {
	var out bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"Action":"run","Package":"p","Test":"BenchmarkTick"}
{"Action":"output","Package":"p","Test":"BenchmarkTick","Output":"=== RUN   BenchmarkTick\n"}
{"Action":"output","Package":"p","Test":"BenchmarkTick","Output":"BenchmarkTick\n"}
{"Action":"output","Package":"p","Test":"BenchmarkTick","Output":"BenchmarkTick-10   \t"}
{"Action":"output","Package":"p","Test":"BenchmarkTick","Output":"  100\t  77375 ns/op\t  26 allocs/op\n"}
{"Action":"output","Package":"p","Test":"BenchmarkBoard/fixed","Output":"BenchmarkBoard/fixed-10 \t 1\t 291583 ns/op\n"}
{"Action":"output","Package":"p","Output":"PASS\n"}
{"Action":"pass","Package":"p","Elapsed":0.2}
`))
	if err := r.finish(testOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	want := margin + "✓ BenchmarkTick-10 100 77375 ns/op 26 allocs/op\n" +
		margin + "✓ BenchmarkBoard/fixed-10 1 291583 ns/op\n" +
		margin + "2 passed, 0 failed, 0 skipped\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A failing Example prints the text go test compared, which starts at the left edge, so its own indentation survives while the indentation go test adds to a test's log lines still comes off.
func TestExampleFailureKeepsItsIndentation(t *testing.T) {
	var out bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"Action":"run","Package":"p","Test":"Example_indent"}
{"Action":"output","Package":"p","Test":"Example_indent","Output":"=== RUN   Example_indent\n"}
{"Action":"output","Package":"p","Test":"Example_indent","Output":"got:\n"}
{"Action":"output","Package":"p","Test":"Example_indent","Output":"a:\n"}
{"Action":"output","Package":"p","Test":"Example_indent","Output":"    b: 1\n"}
{"Action":"output","Package":"p","Test":"Example_indent","Output":"want:\n"}
{"Action":"fail","Package":"p","Test":"Example_indent","Elapsed":0.01}
{"Action":"fail","Package":"p","Elapsed":0.01}
`))
	if err := r.finish(testOptions{}, nil); err == nil {
		t.Fatal("finish reported no failure")
	}
	want := margin + "✗ Example_indent (10ms)\n" +
		margin + "    got:\n" +
		margin + "    a:\n" +
		margin + "        b: 1\n" +
		margin + "    want:\n" +
		margin + "0 passed, 1 failed, 0 skipped\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A test's log lines keep their layout: go test indents one by four spaces and a continued line by eight, and both lose the four.
func TestLogFailureKeepsItsLayout(t *testing.T) {
	var out bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"Action":"run","Package":"p","Test":"TestLogs"}
{"Action":"output","Package":"p","Test":"TestLogs","Output":"=== RUN   TestLogs\n"}
{"Action":"output","Package":"p","Test":"TestLogs","Output":"    x_test.go:7: deep message\n"}
{"Action":"output","Package":"p","Test":"TestLogs","Output":"        second line\n"}
{"Action":"fail","Package":"p","Test":"TestLogs","Elapsed":0.02}
{"Action":"fail","Package":"p","Elapsed":0.02}
`))
	if err := r.finish(testOptions{}, nil); err == nil {
		t.Fatal("finish reported no failure")
	}
	want := margin + "✗ TestLogs (20ms)\n" +
		margin + "    x_test.go:7: deep message\n" +
		margin + "        second line\n" +
		margin + "0 passed, 1 failed, 0 skipped\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A process panic can end the package without any closing test event. Keep its last test's output, fail the tests it cut short, a paused parallel one included, and leave another package's tests alone.
func TestInterruptedTestsKeepTheirOutput(t *testing.T) {
	var out bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"Action":"run","Package":"p","Test":"TestParallel"}
{"Action":"pause","Package":"p","Test":"TestParallel"}
{"Action":"run","Package":"p","Test":"TestCrash"}
{"Action":"run","Package":"q","Test":"TestOther"}
{"Action":"output","Package":"p","Test":"TestCrash","Output":"panic: native callback failed\nstack trace\n"}
{"Action":"output","Package":"p","Output":"native crash reported by the test process\n"}
{"Action":"output","Package":"p","Output":"FAIL\tp\t0.1s\n"}
{"Action":"fail","Package":"p","Elapsed":0.1}
{"Action":"pass","Package":"q","Test":"TestOther","Elapsed":0.2}
{"Action":"pass","Package":"q","Elapsed":0.2}
`))
	err := r.finish(testOptions{}, nil)
	if err == nil || err.Error() != "2 tests failed" {
		t.Errorf("finish = %v, want the interrupted tests to fail", err)
	}
	for _, want := range []string{"✗ TestCrash", "panic: native callback failed", "stack trace", "✗ TestParallel", "✓ TestOther", "1 passed, 2 failed, 0 skipped"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("console misses %q:\n%s", want, out.String())
		}
	}
	if got := strings.Count(out.String(), "native crash reported by the test process"); got != 1 {
		t.Errorf("console shows the package diagnostic %d times:\n%s", got, out.String())
	}
	if len(r.running) != 0 {
		t.Errorf("unfinished tests = %v", r.running)
	}
}

// A process that crashes outside a test, in an init function or a benchmark, which ends with no event of its own, shows the crash under its package and is not called a build failure; a package that did not build still is.
func TestProcessCrashOutsideTests(t *testing.T) {
	var out bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"Action":"output","Package":"i","Output":"panic: init failed\n"}
{"Action":"output","Package":"i","Output":"FAIL\ti\t0.1s\n"}
{"Action":"fail","Package":"i","Elapsed":0.1}
{"Action":"run","Package":"b","Test":"BenchmarkTick"}
{"Action":"output","Package":"b","Test":"BenchmarkTick","Output":"BenchmarkTick-10 \t 100\t 77375 ns/op\n"}
{"Action":"output","Package":"b","Test":"BenchmarkTick","Output":"panic: benchmark failed\n"}
{"Action":"output","Package":"b","Output":"FAIL\tb\t0.1s\n"}
{"Action":"fail","Package":"b","Elapsed":0.1}
`))
	err := r.finish(testOptions{}, nil)
	if err == nil || err.Error() != "a test process failed outside its tests" {
		t.Errorf("finish = %v, want a crash outside the tests", err)
	}
	for _, want := range []string{"✗ i", "panic: init failed", "✗ b", "panic: benchmark failed", "1 passed, 0 failed, 0 skipped"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("console misses %q:\n%s", want, out.String())
		}
	}
	if len(r.running) != 0 {
		t.Errorf("unfinished tests = %v", r.running)
	}

	out.Reset()
	r = &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"ImportPath":"c [c.test]","Action":"build-output","Output":"c_test.go:5:30: undefined: undefined\n"}
{"ImportPath":"c [c.test]","Action":"build-fail"}
{"Action":"output","Package":"c","Output":"FAIL\tc [build failed]\n"}
{"Action":"fail","Package":"c","Elapsed":0,"FailedBuild":"c [c.test]"}
{"Action":"output","Package":"i","Output":"panic: init failed\n"}
{"Action":"fail","Package":"i","Elapsed":0.1}
`))
	if err := r.finish(testOptions{}, nil); err == nil || err.Error() != "tests did not build" {
		t.Errorf("finish = %v, want the build failure", err)
	}
	if !strings.Contains(out.String(), "undefined: undefined") {
		t.Errorf("console misses the compiler's output:\n%s", out.String())
	}
}

// Exercise the real go test event stream, including a panic on another goroutine during a subtest and a test the timeout ends, and the JUnit artifact CI consumers read.
func TestRunTestsReportsProcessPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a test process that panics")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/crash\n\ngo 1.26\n",
		"crash_test.go": `package crash
import "testing"
func TestCrash(t *testing.T) {
    t.Run("worker", func(t *testing.T) {
        go func() { panic("nsgo process panic fixture") }()
        <-make(chan struct{})
    })
}
`,
		"slow/slow_test.go": `package slow
import "testing"
func TestSlow(t *testing.T) { <-make(chan struct{}) }
`,
	})
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "report.xml")
	if err := runTests(t.Context(), p, testOptions{junit: report, args: []string{"-count=1", "-timeout=3s"}}); err == nil || err.Error() != "2 tests failed" {
		t.Fatalf("runTests = %v, want failed tests rather than a build failure", err)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var suites junitSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"TestCrash/worker": "panic: nsgo process panic fixture", "TestSlow": "panic: test timed out after 3s"}
	for _, s := range suites.Suites {
		for _, c := range s.Cases {
			if w, ok := want[c.Name]; ok && c.Failure != nil && strings.Contains(c.Failure.Body, w) {
				delete(want, c.Name)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("JUnit lost the interrupted tests %v or their panics:\n%s", want, data)
	}
}

func TestTSXTestsSkipOwnTrees(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"app/nsgo.yaml", "app/app.test.tsx", "lib/go.mod", "lib/lib.test.tsx", "testdata/x.test.tsx"} {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := &project{Config: projectcfg.Config{Dir: dir}, bare: true}
	if file, err := p.tsxTestFile(); err != nil || file != "" {
		t.Fatalf("tsxTestFile = %q, %v, want none", file, err)
	}
}

func TestIsTestName(t *testing.T) {
	for name, want := range map[string]bool{
		"Test": true, "TestFeel": true, "Test_x": true, "Test2": true,
		"Testing": false, "testFeel": false, "BenchmarkFeel": false,
	} {
		if got := isTestName(name); got != want {
			t.Errorf("isTestName(%q) = %v", name, got)
		}
	}
}

// TestTargetTestsNeedTheTestHost runs a library package's tests on Android and an iPhone, and with macOS activation, which need the native host: the tool refuses the package before it builds or looks for a device, with what to add, whether the tests link no host or start none from TestMain.
func TestTargetTestsNeedTheTestHost(t *testing.T) {
	if testing.Short() {
		t.Skip("lists a module's packages")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	// NSGo's own requirements, which go test -list wants listed in a module that imports it, and NSGo itself from this tree.
	requires := strings.Replace(string(mod), "module github.com/nsgo-dev/nsgo", "module example.com/lib", 1)
	requires, _, _ = strings.Cut(requires, "\ntool ")
	requires += "\nrequire github.com/nsgo-dev/nsgo v0.0.0\n\nreplace github.com/nsgo-dev/nsgo => " + root + "\n"
	const test = "func TestChat(t *testing.T) {}\n"
	for _, tc := range []struct {
		name, file, want string
	}{
		{"unhosted", "package lib\n\nimport \"testing\"\n\n" + test, "do not link: import github.com/nsgo-dev/nsgo in a test file"},
		{"target-only", "//go:build ios || android\n\npackage lib\n\nimport \"testing\"\n\n" + test, "do not link: import github.com/nsgo-dev/nsgo in a test file"},
		{"no TestMain", "package lib\n\nimport (\n\t\"testing\"\n\n\t_ \"github.com/nsgo-dev/nsgo\"\n)\n\n" + test, "add func TestMain(m *testing.M) { nsgotest.Main(m) }"},
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"go.mod":      requires,
			"go.sum":      string(sum),
			"lib.go":      "package lib\n",
			"lib_test.go": tc.file,
		})
		p, err := loadForTest(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		targets := []string{"android", "ios", "macos"}
		if tc.name == "target-only" {
			// Its tests build only for a phone, so macOS has none to refuse: the run finds no tests match.
			targets = targets[:2]
		}
		for _, target := range targets {
			// A device would be looked for, and the tests built, only after the check: none is named, so any other error is the tool going on.
			err := runTests(t.Context(), p, testOptions{target: target, activate: target == "macos", device: "no-such-device"})
			if err == nil || !strings.Contains(err.Error(), "-target "+target+" runs the tests in NSGo's test host") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s on %s: runTests = %v, want an error containing %q", tc.name, target, err, tc.want)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".nsgo", "test")); err == nil {
			t.Errorf("%s: the tool started a build in .nsgo/test", tc.name)
		}
	}
}

func TestActivateNeedsMacOSTarget(t *testing.T) {
	// "all" is not a test target: it errors as unknown before the
	// activate check, so it is not in this list.
	for _, target := range []string{"", "windows", "linux", "web", "ios_simulator", "android"} {
		err := runTests(t.Context(), nil, testOptions{target: target, activate: true})
		if err == nil || !strings.Contains(err.Error(), "-activate goes with -target macos") {
			t.Errorf("target %q: %v", target, err)
		}
	}
}

// An arch-qualified spelling runs the test target of its OS, and an
// unknown target lists the names test accepts (#589).
func TestNormalizeTestTarget(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"windows", "windows"},
		{"windows_x64", "windows"},
		{"windows_arm64", "windows"},
		{"macos_arm64", "macos"},
		{"macos_x64", "macos"},
		{"linux_x64", "linux"},
		{"linux_arm64", "linux"},
		{"android_arm64", "android"},
		{"web", "web"},
		{"ios_simulator", "ios_simulator"},
	} {
		got, err := normalizeTestTarget(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("normalizeTestTarget(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := normalizeTestTarget("plan9"); err == nil || !strings.Contains(err.Error(), "macos, windows, linux") {
		t.Errorf("normalizeTestTarget(plan9) = %v, want an error listing the names test accepts", err)
	}
	// Only the arch-qualified names build.go's targets lists are
	// accepted: any other suffix is refused.
	for _, name := range []string{"windows_foo", "macos_", "linux_riscv64", "miniprogram_wechat"} {
		if _, err := normalizeTestTarget(name); err == nil || !strings.Contains(err.Error(), "unknown test target") {
			t.Errorf("normalizeTestTarget(%q) = %v, want an unknown-target error", name, err)
		}
	}
}

// checkTestTarget normalizes -target first, so -activate sees the plain name, and refuses an architecture the target cannot run on (#589). A desktop target of another OS is left to the host check, as suggesting its plain name would only lead there (#716).
func TestCheckTestTarget(t *testing.T) {
	hostArch, otherArch := "x64", "arm64"
	if runtime.GOARCH == "arm64" {
		hostArch, otherArch = "arm64", "x64"
	}
	here, elsewhere := map[string]string{"darwin": "macos", "windows": "windows", "linux": "linux"}[runtime.GOOS], "windows"
	if here == "windows" {
		elsewhere = "linux"
	}
	if here == "" {
		t.Skipf("no desktop test target runs on %s", runtime.GOOS)
	}
	for _, tc := range []struct {
		in       string
		activate bool
		want     string
		wantErr  string
	}{
		{"", false, "", ""},
		{"", true, "", "-activate goes with -target macos"},
		{"macos_" + hostArch, true, "macos", ""},
		{here + "_" + hostArch, false, here, ""},
		{here + "_" + otherArch, false, "", "use -target " + here},
		{elsewhere + "_" + hostArch, false, elsewhere, ""},
		{elsewhere + "_" + otherArch, false, elsewhere, ""},
		{"android_arm64", false, "android", ""},
		{"android_x64", false, "", "use -target android"},
		{"all", true, "", "unknown test target"},
		{"windows_foo", false, "", "unknown test target"},
		{"windows", true, "", "-activate goes with -target macos"},
	} {
		got, err := checkTestTarget(tc.in, tc.activate)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("checkTestTarget(%q, activate=%v) = %q, %v; want an error containing %q", tc.in, tc.activate, got, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("checkTestTarget(%q, activate=%v) = %q, %v; want %q, nil", tc.in, tc.activate, got, err, tc.want)
		}
	}
}

func TestRunTestsUnknownTargetListsNames(t *testing.T) {
	err := runTests(t.Context(), nil, testOptions{target: "plan9"})
	if err == nil || !strings.Contains(err.Error(), "macos, windows, linux") {
		t.Errorf("runTests(plan9) = %v, want an error listing the names test accepts", err)
	}
}

func TestTestHostError(t *testing.T) {
	if err := testHostError("android", true, true); err != nil {
		t.Errorf("testHostError(hosted, TestMain) = %v, want nil", err)
	}
	if !needsTestHost("android") || !needsTestHost("ios") || needsTestHost("ios_simulator") || needsTestHost("web") || needsTestHost("macos") {
		t.Error("needsTestHost names other targets than android and ios")
	}
}

// TestBackendTestsLinkTheirHost has the tool see a backend's own tests as linking the test host, which they register without NSGo's root package, which imports the backend and so cannot be imported back; and keeps hostBackends to the backends that register one.
func TestBackendTestsLinkTheirHost(t *testing.T) {
	if testing.Short() {
		t.Skip("lists a package's dependencies")
	}
	p, err := loadForTest("../../internal/platform/android", nil)
	if err != nil {
		t.Fatal(err)
	}
	if hosted, err := linksPlatform(t.Context(), p, ""); err != nil || !hosted {
		t.Errorf("linksPlatform(internal/platform/android) = %v, %v; want true", hosted, err)
	}
	files, err := filepath.Glob("../../internal/platform/*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	var registering []string
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		pkg := "github.com/nsgo-dev/nsgo/internal/platform/" + filepath.Base(filepath.Dir(f))
		if !strings.HasSuffix(f, "_test.go") && bytes.Contains(src, []byte("testhost.Register(")) && !slices.Contains(registering, pkg) {
			registering = append(registering, pkg)
		}
	}
	slices.Sort(registering)
	if !slices.Equal(registering, hostBackends) {
		t.Errorf("backends registering a test host: %v; hostBackends lists %v", registering, hostBackends)
	}
}

func TestListTestSourcesLeavesOutTestMain(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":      "module example.com/lib\n\ngo 1.26\n",
		"lib_test.go": "package lib\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) { m.Run() }\n\nfunc TestChat(t *testing.T) {}\n\nfunc helper() {}\n",
	})
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	names, err := listTestSources(t.Context(), p, "", ".")
	if err != nil || !slices.Equal(names, []string{"TestChat"}) {
		t.Fatalf("listTestSources = %q, %v; want [TestChat]", names, err)
	}
}

func TestListTestsUsesTargetBuildTags(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/lib\n\ngo 1.26\n",
		"lib.go": "package lib\n",
		// go test runs an example only with an output comment.
		"lib_test.go":     "package lib\nimport \"testing\"\nfunc TestPortable(t *testing.T) {}\nfunc ExampleShown() {\n\t// Output:\n}\nfunc ExampleCompiled() {}\n",
		"ios_test.go":     "//go:build ios && nsgotest\n\npackage lib\nimport \"testing\"\nfunc TestIOS(t *testing.T) {}\n",
		"android_test.go": "//go:build android && nsgotest\n\npackage lib\nimport \"testing\"\nfunc TestAndroid(t *testing.T) {}\n",
		"desktop_test.go": "//go:build darwin && !ios\n\npackage lib\nimport \"testing\"\nfunc TestDesktop(t *testing.T) {}\n",
		// The tool adds the TSX tests' file to every target's test build.
		"app.test.tsx": "",
	})
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string][]string{
		"ios_simulator": {"ExampleShown", "TestIOS", "TestPortable", "TestTSX"},
		"android":       {"ExampleShown", "TestAndroid", "TestPortable", "TestTSX"},
		"macos_arm64":   {"ExampleShown", "TestDesktop", "TestPortable", "TestTSX"},
		"web":           {"ExampleShown", "TestPortable", "TestTSX"},
	} {
		names, err := listTests(t.Context(), p, target, ".")
		slices.Sort(names)
		if err != nil || !slices.Equal(names, want) {
			t.Errorf("listTests(%q) = %q, %v; want %q", target, names, err, want)
		}
	}
}

func TestSaveScreenshotsKeepsNamesAndRejectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	encode := func(name string) string { return "~" + base64.RawURLEncoding.EncodeToString([]byte(name)) }
	line := func(name string) string { return "nsgo-screenshot " + encode(name) + " 1/1 8:7a0709a4 iVBORw0KGgo=" }
	// chunks is a PNG's lines as nsgotest prints them, size base64 characters to a chunk.
	chunks := func(name string, png []byte, size int) []string {
		data := base64.StdEncoding.EncodeToString(png)
		n := (len(data) + size - 1) / size
		var out []string
		for i := range n {
			out = append(out, fmt.Sprintf("nsgo-screenshot %s %d/%d %d:%08x %s", encode(name), i+1, n, len(png), crc32.ChecksumIEEE(png), data[i*size:min((i+1)*size, len(data))]))
		}
		return out
	}
	seen := map[string]bool{}
	if _, saved, err := saveScreenshots(line("en/my library")+"\n"+line("ja/my library")+"\n", dir, seen); err != nil {
		t.Fatal(err)
	} else if len(saved) != 2 {
		t.Fatalf("saved %v, want two screenshots", saved)
	}
	for _, name := range []string{"en/my library.png", "ja/my library.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("screenshot %s: %v", name, err)
		}
	}
	if _, saved, err := saveScreenshots(line("en/my library")+"\n"+line("after")+"\n", dir, seen); err == nil {
		t.Fatal("duplicate screenshot path succeeded")
	} else if len(saved) != 1 || filepath.Base(saved[0].path) != "after.png" {
		t.Fatalf("saved %v beside a duplicate, want after.png", saved)
	}
	// Two shots of one name in one process, the second larger, fail without corrupting the first.
	two := strings.Join(chunks("twice", []byte("a larger second shot"), 8), "\n") + "\n"
	if _, saved, err := saveScreenshots(line("twice")+"\n"+two, dir, map[string]bool{}); err == nil {
		t.Fatal("a name shot twice in one process succeeded")
	} else if len(saved) != 1 {
		t.Fatalf("saved %v, want the first shot", saved)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "twice.png")); err != nil || len(b) != 8 {
		t.Fatalf("twice.png holds %d bytes (%v), want the first shot's 8", len(b), err)
	}
	if _, _, err := saveScreenshots(line("../outside")+"\n", dir, map[string]bool{}); err == nil {
		t.Fatal("parent screenshot path succeeded")
	}
	for _, name := range []string{".", "./"} {
		if _, _, err := saveScreenshots(line(name)+"\n", dir, map[string]bool{}); err == nil {
			t.Errorf("screenshot %q succeeded", name)
		}
	}
	if _, err := os.Stat(dir + ".png"); err == nil {
		t.Error("screenshot \".\" wrote a file beside the directory")
	}
	// A path an earlier screenshot made a directory cannot be written.
	if _, saved, err := saveScreenshots(line("shot.png/a")+"\n"+line("shot.png")+"\n", dir, map[string]bool{}); err == nil {
		t.Fatal("a screenshot over a directory succeeded")
	} else if len(saved) != 1 {
		t.Fatalf("saved %v, want only shot.png/a.png", saved)
	}
	// What a device's log loses or cuts fails the shot by name before a file is written: a dropped middle chunk, a chunk cut short (one that still decodes, as a cut at a multiple of four characters does), an empty chunk, chunks that disagree and data that is not base64.
	png := []byte("\x89PNG\r\n\x1a\n twelve chunks of a PNG's bytes, base64 in eight characters each")
	parts := fmt.Sprintf("/%d ", len(chunks("x", png, 8)))
	cut := func(name string, edit func([]string) []string) string {
		lines := edit(chunks(name, png, 8))
		return strings.Join(lines, "\n") + "\n"
	}
	for _, c := range []struct{ name, out, want string }{
		{"dropped", cut("dropped", func(l []string) []string { return append(l[:3:3], l[4:]...) }), `screenshot "dropped": missing chunks 4 of`},
		{"short", cut("short", func(l []string) []string { l[4] = l[4][:len(l[4])-4]; return l }), `screenshot "short": its chunks hold`},
		{"empty", cut("empty", func(l []string) []string { l[4] = l[4][:len(l[4])-8]; return l }), `screenshot "empty": its chunks hold`},
		{"total", cut("total", func(l []string) []string { l[2] = strings.Replace(l[2], parts, "/99 ", 1); return l }), `screenshot "total": chunk 3 declares 99 parts`},
		{"base64", cut("base64", func(l []string) []string { l[1] = l[1][:len(l[1])-8] + "!!!!!!!!"; return l }), `screenshot "base64": illegal base64`},
	} {
		if _, saved, err := saveScreenshots(c.out, dir, map[string]bool{}); err == nil || !strings.Contains(err.Error(), c.want) || len(saved) != 0 {
			t.Errorf("%s: saved %v, error %v; want %q", c.name, saved, err, c.want)
		}
		if _, err := os.Stat(filepath.Join(dir, c.name+".png")); err == nil {
			t.Errorf("%s: wrote a file", c.name)
		}
	}
	if _, saved, err := saveScreenshots(strings.Join(chunks("complete", png, 8), "\n")+"\n", dir, map[string]bool{}); err != nil || len(saved) != 1 {
		t.Fatalf("complete chunks: saved %v, error %v", saved, err)
	} else if b, _ := os.ReadFile(saved[0].path); string(b) != string(png) {
		t.Fatalf("complete chunks saved %q, want %q", b, png)
	}
	// Names that differ only in case are one file where the volume ignores case.
	seen = map[string]bool{}
	if _, _, err := saveScreenshots(line("en/Home")+"\n", dir, seen); err != nil {
		t.Fatal(err)
	}
	_, folds := os.Stat(filepath.Join(dir, "en", "home.png"))
	_, _, err := saveScreenshots(line("en/home")+"\n", dir, seen)
	if folds == nil && err == nil {
		t.Error("en/home overwrote en/Home on a case-insensitive volume")
	} else if folds != nil && err != nil {
		t.Errorf("en/home beside en/Home on a case-sensitive volume: %v", err)
	}
}

func TestSaveScreenshotsMarksFallback(t *testing.T) {
	dir := t.TempDir()
	encode := func(name string) string { return "~" + base64.RawURLEncoding.EncodeToString([]byte(name)) }
	png := []byte("\x89PNG\r\n\x1a\n multi-chunk fallback screenshot bytes")
	data := base64.StdEncoding.EncodeToString(png)
	sum := fmt.Sprintf("%d:%08x", len(png), crc32.ChecksumIEEE(png))
	var multiFallback []string
	const size = 12
	n := (len(data) + size - 1) / size
	for i := range n {
		multiFallback = append(multiFallback, fmt.Sprintf("nsgo-screenshot %s %d/%d %s:fallback %s",
			encode("degraded"), i+1, n, sum, data[i*size:min((i+1)*size, len(data))]))
	}
	normalLine := "nsgo-screenshot " + encode("composited") + " 1/1 8:7a0709a4 iVBORw0KGgo="
	out := normalLine + "\n" + strings.Join(multiFallback, "\n") + "\n"
	_, saved, err := saveScreenshots(out, dir, map[string]bool{})
	if err != nil {
		t.Fatalf("saveScreenshots: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("saved %v, want 2 screenshots", saved)
	}
	if saved[0].fallback || !saved[1].fallback {
		t.Fatalf("fallback flags = [%v, %v], want [false, true]", saved[0].fallback, saved[1].fallback)
	}
	if got := formatSavedScreenshot(saved[0]); strings.Contains(got, "(fallback)") || !strings.Contains(got, "composited.png") {
		t.Errorf("normal screenshot output = %q, want path without (fallback)", got)
	}
	if got := formatSavedScreenshot(saved[1]); !strings.Contains(got, "degraded.png (fallback)") {
		t.Errorf("fallback screenshot output = %q, want path followed by (fallback)", got)
	}
	// Chunks of one shot that disagree on fallback fail without saving.
	mixed := append([]string(nil), multiFallback...)
	mixed[len(mixed)-1] = strings.Replace(mixed[len(mixed)-1], ":fallback ", " ", 1)
	if _, got, err := saveScreenshots(strings.Join(mixed, "\n")+"\n", dir, map[string]bool{}); err == nil || len(got) != 0 {
		t.Fatalf("mixed fallback chunks: saved=%v err=%v, want error and no saved file", got, err)
	}
}

// A passing test the tool fails, for a screenshot it cannot save or a process that exits badly, reports the tool's reason as its JUnit message and prints none of go test's closing lines.
func TestToolFailureIsTheMessage(t *testing.T) {
	dir := t.TempDir()
	shot := "nsgo-screenshot ~" + base64.RawURLEncoding.EncodeToString([]byte("home")) + " 1/1 8:7a0709a4 iVBORw0KGgo=\n"
	passed := "=== RUN   TestShot\n    shot_test.go:9: took it\n" + shot + shot + "--- PASS: TestShot (0.01s)\nPASS\n"
	dup, _ := targetResult("TestShot", 0, passed, 0, nil, dir, map[string]bool{})
	exited, _ := targetResult("TestExit", 0, "=== RUN   TestExit\n--- PASS: TestExit (0.01s)\nPASS\n", 1, nil, dir, map[string]bool{})
	for _, c := range []struct {
		res  *testResult
		want string
	}{
		{dup, `screenshot "home" resolves to`},
		{exited, "the test process exited with 1"},
	} {
		if c.res.Status != "fail" {
			t.Fatalf("%s: status %q, want fail", c.res.Name, c.res.Status)
		}
		c.res.Package = "example.com/app"
		report := filepath.Join(dir, c.res.Name+".xml")
		if err := writeJUnit(report, []*testResult{c.res}, nil); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(report)
		if err != nil {
			t.Fatal(err)
		}
		var suites junitSuites
		if err := xml.Unmarshal(data, &suites); err != nil {
			t.Fatal(err)
		}
		if got := suites.Suites[0].Cases[0].Failure.Message; !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: JUnit message %q, want %q…", c.res.Name, got, c.want)
		}
		var console bytes.Buffer
		r := &testRun{out: &console}
		r.print(c.res)
		for _, line := range strings.Split(console.String(), "\n") {
			if strings.TrimSpace(line) == "PASS" {
				t.Errorf("%s: console prints go test's PASS line:\n%s", c.res.Name, console.String())
			}
		}
		if !strings.Contains(console.String(), c.want) {
			t.Errorf("%s: console lacks %q:\n%s", c.res.Name, c.want, console.String())
		}
	}
}

// Every repetition of a benchmark is reported, though go test attaches all but the first to the package (#2273).
func TestBenchmarkRepetitionsAreReported(t *testing.T) {
	var out bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out}
	r.read(strings.NewReader(`{"Action":"run","Package":"x","Test":"BenchmarkX"}
{"Action":"output","Package":"x","Test":"BenchmarkX","Output":"=== RUN   BenchmarkX\n"}
{"Action":"output","Package":"x","Test":"BenchmarkX","Output":"BenchmarkX\n"}
{"Action":"output","Package":"x","Test":"BenchmarkX","Output":"BenchmarkX-12    \t 6696271\t         1.685 ns/op\n"}
{"Action":"output","Package":"x","Output":"BenchmarkX-12    \t 7485183\t         1.615 ns/op\n"}
{"Action":"output","Package":"x","Output":"PASS\n"}
{"Action":"pass","Package":"x","Elapsed":0.284}
`))
	if err := r.finish(testOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	want := margin + "✓ BenchmarkX-12 6696271 1.685 ns/op\n" +
		margin + "✓ BenchmarkX-12 7485183 1.615 ns/op\n" +
		margin + "2 passed, 0 failed, 0 skipped\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

// The raw stream reaches the writer -json sets, so a tool can read go test's own events (#2273).
func TestJSONWritesTheRawStream(t *testing.T) {
	var out, raw bytes.Buffer
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, out: &out, raw: &raw}
	stream := `{"Action":"run","Package":"x","Test":"TestOne"}
{"Action":"pass","Package":"x","Test":"TestOne","Elapsed":0.1}
`
	r.read(strings.NewReader(stream))
	if err := r.finish(testOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if raw.String() != stream {
		t.Fatalf("raw stream:\n%s\nwant:\n%s", raw.String(), stream)
	}
}

func TestTestCommandPackagePatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test command")
	}
	dir := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":                    "module patternstest\n\ngo 1.26\n",
		"root_test.go":              "package root\n\nimport \"testing\"\n\nfunc TestRoot(t *testing.T) {}\n",
		"sub/sub_test.go":           "package sub\n\nimport \"testing\"\n\nfunc TestSub(t *testing.T) {}\n",
		"sub/leaf/leaf_test.go":     "package leaf\n\nimport \"testing\"\n\nfunc TestLeaf(t *testing.T) {}\n",
		"other/other_test.go":       "package other\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) {}\n",
		"nested/go.mod":             "module nestedmod\n\ngo 1.26\n",
		"nested/pkg/nested_test.go": "package pkg\n\nimport \"testing\"\n\nfunc TestNested(t *testing.T) {}\n",
		"app/nsgo.yaml":             "name: Sample\n",
		"app/app_test.go":           "package app\n\nimport \"testing\"\n\nfunc TestApp(t *testing.T) {}\n",
		"app/child/child_test.go":   "package child\n\nimport \"testing\"\n\nfunc TestAppChild(t *testing.T) {}\n",
	})
	writeFiles(t, outside, map[string]string{
		"go.mod":          "module outsidemod\n\ngo 1.26\n",
		"ext/ext_test.go": "package ext\n\nimport \"testing\"\n\nfunc TestExt(t *testing.T) {}\n",
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"root package only", []string{"."}, []string{"TestRoot"}},
		{"multiple patterns", []string{".", "./sub/..."}, []string{"TestLeaf", "TestRoot", "TestSub"}},
		{"nested go module", []string{"./nested"}, []string{"TestNested"}},
		{"app with nsgo.yaml", []string{"./app"}, []string{"TestApp", "TestAppChild"}},
		{"external module dir", []string{outside}, []string{"TestExt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := filepath.Join(t.TempDir(), "tests.xml")
			cliArgs := append([]string{"-test.run=^TestCLIProcess$", "--", "test", "-junit", report}, tc.args...)
			cliArgs = append(cliArgs, "--", "-count=1")
			cmd := exec.CommandContext(t.Context(), exe, cliArgs...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "NSGO_CLI_PROCESS=1")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("nsgo test %q: %v\n%s", tc.args, err, out)
			}
			var got []string
			for _, suite := range readJUnitFixture(t, report).Suites {
				for _, c := range suite.Cases {
					got = append(got, c.Name)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("ran %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTargetTestsLoadPackageDirectoryWithoutYAML(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the test command")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":          "module targetdir\n\ngo 1.26\n",
		"root_test.go":    "package root\n\nimport \"testing\"\n\nfunc TestRootOnly(t *testing.T) {}\n",
		"pkg/pkg_test.go": "package pkg\n\nimport \"testing\"\n\nfunc TestPkgOnly(t *testing.T) {}\n",
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runCLI := func(args ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), exe, append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "NSGO_CLI_PROCESS=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Selecting a test in ./pkg discovers it in ./pkg (reaching the test host check), instead of silently finding 0 tests in the repo root.
	out, err := runCLI("test", "-target", "android", "-run", "^TestPkgOnly$", "./pkg")
	if err == nil || !strings.Contains(out, "runs the tests in NSGo's test host") {
		t.Fatalf("target test of ./pkg for TestPkgOnly: err=%v, out=%s", err, out)
	}
	// Selecting a test that exists only in the root must not match when ./pkg is targeted.
	out, err = runCLI("test", "-target", "android", "-run", "^TestRootOnly$", "./pkg")
	if err != nil || !strings.Contains(out, "no tests match") {
		t.Fatalf("target test of ./pkg for TestRootOnly: err=%v, out=%s", err, out)
	}
	// A non-existent directory fails cleanly rather than loading cwd.
	out, err = runCLI("test", "-target", "android", "./missing")
	if err == nil || !strings.Contains(out, "no directory ./missing") {
		t.Fatalf("target test of ./missing: err=%v, out=%s", err, out)
	}

	// Direct runTests callers passing a single pattern with -target also resolve that directory.
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	if err := runTests(t.Context(), p, testOptions{target: "android", run: "^TestPkgOnly$", patterns: []string{"./pkg"}}); err == nil || !strings.Contains(err.Error(), "runs the tests in NSGo's test host") {
		t.Fatalf("direct runTests(-target android, patterns=./pkg) = %v, want test host error", err)
	}
	if err := runTests(t.Context(), p, testOptions{target: "android", patterns: []string{"./missing"}}); err == nil || !strings.Contains(err.Error(), "no directory") {
		t.Fatalf("direct runTests(-target android, patterns=./missing) = %v, want no directory error", err)
	}
}

func TestRunTestsRejectsMultiplePatternsWithTargetAndFuzz(t *testing.T) {
	if err := runTests(t.Context(), nil, testOptions{target: "web", patterns: []string{"./a", "./b"}}); err == nil || !strings.Contains(err.Error(), "one directory, not several package patterns") {
		t.Errorf("multi-pattern -target: %v", err)
	}
	if err := runTests(t.Context(), nil, testOptions{patterns: []string{"./a", "./b"}, args: []string{"-fuzz", "FuzzX"}}); err == nil || !strings.Contains(err.Error(), "-fuzz runs one package") {
		t.Errorf("multi-pattern -fuzz: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"test", "-target", "web", "./a", "./b"}, "one directory, not several package patterns"},
		{[]string{"test", "./a", "./b", "-fuzz", "FuzzX"}, "-fuzz runs one package"},
	} {
		cmd := exec.CommandContext(t.Context(), exe, append([]string{"-test.run=^TestCLIProcess$", "--"}, tc.args...)...)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_CLI_PROCESS=1")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), tc.want) {
			t.Errorf("CLI %q: err=%v, out=%s; want %q", tc.args, err, out, tc.want)
		}
	}
}

func TestRunCoversGatesDevClient(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":              "module " + nsgoModule + "\n\ngo 1.26\n",
		"root.go":             "package nsgo\n",
		"internal/dev/dev.go": "package dev\n",
		"other/other.go":      "package other\n",
	})
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	dev := nsgoModule + "/internal/dev"
	for _, tc := range []struct {
		name     string
		dir      string
		patterns []string
		want     bool
	}{
		{"nil patterns", dir, nil, true},
		{"whole tree", dir, []string{"./..."}, true},
		{"internal tree", dir, []string{"./internal/..."}, true},
		{"exact dev package", dir, []string{"./internal/dev"}, true},
		{"root package only", dir, []string{"."}, false},
		{"other packages", dir, []string{".", "./other/..."}, false},
		{"go list error returns false", filepath.Join(dir, "missing"), []string{"./..."}, false},
	} {
		if got := runCovers(t.Context(), tc.dir, tc.patterns, dev); got != tc.want {
			t.Errorf("%s: runCovers(%q) = %v, want %v", tc.name, tc.patterns, got, tc.want)
		}
	}
}
