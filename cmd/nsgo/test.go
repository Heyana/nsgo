package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// testOptions are nsgo test's flags.
type testOptions struct {
	run         string        // the tests to run, as go test's -run
	timeout     time.Duration // per-test target timeout; zero means none
	bench       string        // the benchmarks to run, as go test's -bench
	junit       string        // where to write a JUnit report
	trace       bool          // print each step's batches and commit times
	activate    bool          // explicitly activate macOS's test app for keyboard input
	target      string        // the platform to run on; headless when ""
	screenshots string        // screenshot output directory for target runs
	fps         bool          // log the performance monitor's readings on target
	device      string        // the simulator or device a mobile target runs on
	team        string        // the Apple developer team an iPhone's test build is signed with
	ldflags     []string      // app linker flags -ldflags adds to nsgo.yaml's build.ldflags
	// env are the tests' KEY=VALUE variables, on every target: a
	// phone's tests see no others of this machine's.
	env  []string
	args []string
}

// testEvent is a line of go test -json (test2json's format).
type testEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
	// ImportPath names what a build-output event's compiler output is
	// of, which FailedBuild names on the package's fail event.
	ImportPath  string
	FailedBuild string
}

// testResult is one test's outcome.
type testResult struct {
	Package, Name string
	Status        string // pass, fail or skip
	Elapsed       time.Duration
	Output        string
	// Message is a skip's reason or why the tool failed a test after it ran, used in place of the output's first line.
	Message  string
	reported int // how much of a benchmark's Output it reported
	// whole marks the result of one target test process, which counts in the tally whatever its name, a selected subtest's included, since no parent result is reported beside it.
	whole    bool
	children []*testResult
}

// summarizeSkips counts a successful group with only skipped children as skipped, preserving their reasons. A group's own failure always wins.
func (t *testResult) summarizeSkips() {
	for _, child := range t.children {
		child.summarizeSkips()
	}
	if t.Status != "pass" || len(t.children) == 0 {
		return
	}
	for _, child := range t.children {
		if child.Status != "skip" {
			return
		}
	}
	t.Status = "skip"
	reasons := []string{"all subtests skipped"}
	for _, child := range t.children {
		if child.Message != "" {
			reasons = append(reasons, child.Name+": "+child.Message)
		}
	}
	t.Message = strings.Join(reasons, "\n")
}

// failFor fails t for why, a reason the tool found after the test ran, and adds why to its output.
func (t *testResult) failFor(why string) {
	if t.Status == "pass" || t.Status == "skip" {
		t.Message = why
	}
	t.Status = "fail"
	if t.Output != "" && !strings.HasSuffix(t.Output, "\n") {
		t.Output += "\n"
	}
	t.Output += why + "\n"
}

// testRun gathers what go test -json reports.
type testRun struct {
	results []*testResult
	running map[string]*testResult
	// packages hold what a package printed outside its tests, such as
	// build errors, and whether it failed.
	packages map[string]*testResult
	// builds hold the compiler's output by import path.
	builds map[string]string
	// unbuilt are the packages whose tests did not build.
	unbuilt map[string]bool
	// label follows the names of the packages the current run tests (testCommand).
	label string
	trace bool
	out   io.Writer
}

// runTests runs the app's tests headless, or on a target, printing a line
// per test and failing when any failed.
func runTests(ctx context.Context, p *project, opts testOptions) error {
	// -target is normalized and checked before the project is touched:
	// an arch-qualified spelling runs the target of its OS, -activate
	// goes with macos, and an architecture the target cannot run is
	// refused (#589).
	target, err := checkTestTarget(opts.target, opts.activate)
	if err != nil {
		return err
	}
	opts.target = target
	// go test would take another package inside the app's directory, and a target never reads it.
	if err := testPathError(opts.args); err != nil {
		return err
	}
	if opts.target != "" {
		if _, err := parseTargetTestFlags(opts.args); err != nil {
			return err
		}
	}
	p.Build.LDFlags = append(p.Build.LDFlags, opts.ldflags...)
	if err := p.prepareCatalogs(ctx, "./...", false); err != nil {
		return err
	}
	if opts.fps && !slices.Contains([]string{"macos", "windows", "linux", "ios_simulator", "web", "android"}, opts.target) {
		return errors.New("test: -fps goes with -target macos, windows, linux, ios_simulator, web or android, whose monitors it reads")
	}
	if opts.bench != "" && (opts.target != "" || p.steps != nil) {
		return errors.New("test: -bench runs headless, on the package's tests")
	}
	if filepath.IsAbs(opts.screenshots) {
		return errors.New("test: -screenshots takes a relative directory")
	}
	if opts.target != "" {
		return runTargetTests(ctx, p, opts)
	}
	if p.steps != nil {
		opts.run = "^" + stepsTest + "$"
	}
	args := []string{"test", "-json"}
	if ldflags := p.appLdflags(); ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	if opts.bench != "" {
		// Benchmarks alone, unless -run names tests to run with them.
		args = append(args, "-bench", opts.bench)
		if opts.run == "" {
			opts.run = "^$"
		}
	}
	tags, err := goTags(ctx, p, opts.env)
	if err != nil {
		return err
	}
	if opts.run != "" {
		args = append(args, "-run", opts.run)
	}
	if opts.trace {
		// Traces print as the tests run; -v keeps go test from hiding them.
		args = append(args, "-v", "-count=1")
	}
	tsxTest, err := p.tsxTestFile()
	if err != nil {
		return err
	}
	if tsxTest != "" {
		ov, err := p.overlay(map[string]string{"zz_nsgo_tsx_test.go": tsxTest})
		if err != nil {
			return err
		}
		args = append(args, "-overlay", ov)
	}
	command := func(pattern string, tags, flags []string) *exec.Cmd {
		a := append(slices.Clone(args), tagsArgs(tags)...)
		// Packages precede flags a test binary may take, including -args.
		a = append(append(a, pattern), flags...)
		cmd := exec.CommandContext(ctx, "go", a...)
		cmd.Dir = p.Dir
		cmd.Env = append(os.Environ(), opts.env...)
		if opts.trace {
			cmd.Env = append(cmd.Env, "NSGO_TEST_TRACE=1")
		}
		return cmd
	}
	// Headless tests build with nsgocheck, which keeps core's graph ownership checks in the build; a -tags on the command line replaces the tags before it, as go test does, without dropping nsgocheck.
	runTags, flags, profile := testTagArgs(p.Dir, tags, opts.args)
	if profile != "" {
		flags = append([]string{"-coverprofile=" + profile}, flags...)
	}
	runs := []testCommand{{cmd: command("./...", uniqueTags(append(runTags, "nsgocheck")), flags)}}
	// As check does, NSGo's own repository also tests the dev client, whose files build only with nsgodev, unless the run's tags already include it; there are no automation-only dev tests. Its coverage profile goes to a file of its own, appended to the run's once both have ended, since go test would overwrite the first run's.
	var cover, devCover string
	if dev, ok := devClientPattern(p.Dir); ok && p.steps == nil {
		devTags, flags, profile := testTagArgs(p.Dir, tags, opts.args)
		if !slices.Contains(devTags, "nsgodev") {
			if profile != "" {
				f, err := os.CreateTemp("", "nsgo-dev-*.cover")
				if err != nil {
					return err
				}
				f.Close()
				defer os.Remove(f.Name())
				cover, devCover = profile, f.Name()
				flags = append([]string{"-coverprofile=" + devCover}, flags...)
			}
			runs = append(runs, testCommand{cmd: command(dev, append(devTags, "nsgodev"), flags), label: devClientLabel, title: "testing the dev client with nsgodev"})
		}
	}
	err = reportTests(opts, runs...)
	if cover != "" {
		if cerr := appendCoverProfile(cover, devCover); err == nil {
			err = cerr
		}
	}
	return err
}

// devClientLabel follows the names of the dev client's packages tested with nsgodev, as go test names a test variant of a package, so the report sets them apart from the ordinary build's.
const devClientLabel = " [nsgodev]"

// devClientPattern is the pattern naming NSGo's dev client, internal/dev, from dir, when dir is the repository's root or a directory holding the dev client.
func devClientPattern(dir string) (string, bool) {
	path, err := importPathOf(dir)
	if err != nil {
		return "", false
	}
	dev := nsgoModule + "/internal/dev"
	if path == dev {
		return ".", true
	}
	sub, ok := strings.CutPrefix(dev, path+"/")
	if !ok || (path != nsgoModule && !strings.HasPrefix(path, nsgoModule+"/")) {
		return "", false
	}
	if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(sub))); err != nil || !fi.IsDir() {
		return "", false
	}
	return "./" + sub, true
}

// testTagArgs reads the command line's -tags as Go does, replacing the tags before it, and returns the other flags, so a later -tags cannot drop the nsgocheck or nsgodev added to the tags, leaving the test binary's own arguments intact. It takes -coverprofile out as well, returning the file it names, resolved as go test does against -outputdir and dir, so the dev run can write its profile elsewhere.
func testTagArgs(dir string, tags, args []string) (devTags, flags []string, cover string) {
	var outputDir string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-args" || a == "--args" || a == "--" {
			flags = append(flags, args[i:]...)
			break
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		name = strings.TrimPrefix(name, "test.")
		if name == "tags" || name == "coverprofile" {
			if !hasValue && i+1 < len(args) {
				i++
				value = args[i]
			}
			if name == "tags" {
				tags = strings.Fields(strings.ReplaceAll(value, ",", " "))
			} else {
				cover = value
			}
			continue
		}
		flags = append(flags, a)
		if strings.HasPrefix(a, "-") && !hasValue && !goTestBoolFlags[name] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			flags = append(flags, args[i])
			value = args[i]
		}
		if name == "outputdir" {
			outputDir = value
		}
	}
	if cover != "" {
		if !filepath.IsAbs(cover) {
			if !filepath.IsAbs(outputDir) {
				outputDir = filepath.Join(dir, outputDir)
			}
			cover = filepath.Join(outputDir, cover)
		}
	}
	return uniqueTags(tags), flags, cover
}

// appendCoverProfile adds the blocks of the coverage profile at from to the one at to, as go test joins its packages' profiles, or moves it there when the first run wrote none.
func appendCoverProfile(to, from string) error {
	data, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	f, err := os.OpenFile(to, os.O_WRONLY|os.O_APPEND, 0)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(to, data, 0o644)
	}
	if err != nil {
		return err
	}
	if strings.HasPrefix(string(data), "mode:") {
		_, data, _ = bytes.Cut(data, []byte("\n"))
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// splitTestArgs takes the optional project directory before forwarded Go flags. flag.Parse already consumes -- when no directory precedes it.
func splitTestArgs(rest []string) (dir string, args []string) {
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		dir, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	return dir, rest
}

// loadForTest reads the app's nsgo.yaml; a directory without one still
// has tests to run headless. dir may end in /..., as go's patterns do,
// which names the same tree. Given an app's TSX entry, it reads its
// directory's, and the steps standard input holds, none from a terminal.
func loadForTest(dir string, stdin *os.File) (*project, error) {
	dir = treeOf(dir)
	if ext := filepath.Ext(dir); (ext == ".tsx" || ext == ".ts") && !strings.HasSuffix(dir, ".test"+ext) {
		if fi, err := os.Stat(dir); err != nil || fi.IsDir() {
			return nil, fmt.Errorf("test: no entry %s", dir)
		}
		p, err := loadForTest(filepath.Dir(dir), nil)
		if err != nil {
			return nil, err
		}
		s := &steps{entry: filepath.ToSlash(filepath.Base(dir))}
		if fi, err := stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
			b, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("test: the steps of standard input: %w", err)
			}
			s.src = string(b)
		}
		p.steps = s
		return p, nil
	}
	return loadOrDir(dir)
}

// treeOf is the directory a go pattern ending in /... names, or dir.
func treeOf(dir string) string {
	if dir == "..." {
		return "."
	}
	if t, ok := strings.CutSuffix(filepath.ToSlash(dir), "/..."); ok {
		if t == "" {
			return "/"
		}
		return filepath.FromSlash(t)
	}
	return dir
}

// goTestBoolFlags are go test's flags that take no value, its build flags' and the test binary's it forwards, with or without the test. prefix.
var goTestBoolFlags = map[string]bool{
	"a": true, "n": true, "x": true, "asan": true, "msan": true, "race": true, "linkshared": true, "trimpath": true, "work": true, "modcacherw": true, "buildvcs": true, "cover": true,
	"c": true, "json": true, "artifacts": true, "benchmem": true, "failfast": true, "fullpath": true, "short": true, "v": true,
	"test.artifacts": true, "test.benchmem": true, "test.failfast": true, "test.fullpath": true, "test.short": true, "test.v": true,
}

// packageArgs reports the arguments among go test's that it would take as packages: those that are neither flags nor their values, before -args or --. As go test does, it takes a flag it does not know, one of the test binary's own, to have a value unless the flag is written -name=value.
func packageArgs(args []string) []string {
	var packages []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || a == "-args" || a == "--args" {
			break
		}
		if len(a) < 2 || a[0] != '-' {
			packages = append(packages, a)
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimPrefix(a[1:], "-"), "=")
		if !hasValue && !goTestBoolFlags[name] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
		}
	}
	return packages
}

// testCommand is a go test -json run. label, when set, follows the names of its packages, which another run tests too, with other tags, and title names the run on the console before it starts.
type testCommand struct {
	cmd          *exec.Cmd
	label, title string
}

// reportTests runs the commands one after another, as go test goes on past a package's failures, and reports their tests together. A run that did not start or end by itself, as when the command is interrupted, stops the rest.
func reportTests(opts testOptions, runs ...testCommand) error {
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, trace: opts.trace, out: os.Stderr}
	var waitErr error
	for _, run := range runs {
		if run.title != "" {
			event("%s", run.title)
		}
		stdout, err := run.cmd.StdoutPipe()
		if err == nil {
			run.cmd.Stderr = os.Stderr
			err = run.cmd.Start()
		}
		if err != nil {
			waitErr = cmp.Or(waitErr, err)
			break
		}
		r.label, r.builds = run.label, nil
		r.read(stdout)
		err = run.cmd.Wait()
		waitErr = cmp.Or(waitErr, err)
		var exit *exec.ExitError
		if err != nil && (!errors.As(err, &exit) || !exit.Exited()) {
			break
		}
	}
	if err := r.finish(opts, waitErr); err != nil {
		return err
	}
	return waitErr
}

func (r *testRun) read(in io.Reader) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var ev testEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			fmt.Fprintln(r.out, sc.Text())
			continue
		}
		r.event(ev)
	}
}

func (r *testRun) event(ev testEvent) {
	if ev.Action == "build-output" {
		if r.builds == nil {
			r.builds = map[string]string{}
		}
		r.builds[ev.ImportPath] += ev.Output
		return
	}
	ev.Package += r.label
	if ev.Test == "" {
		pkg := r.packages[ev.Package]
		if pkg == nil {
			pkg = &testResult{Package: ev.Package}
			r.packages[ev.Package] = pkg
		}
		switch ev.Action {
		case "output":
			pkg.Output += ev.Output
		case "fail":
			pkg.Status = "fail"
			// A package that did not build shows why.
			if ev.FailedBuild != "" {
				pkg.Output += r.builds[ev.FailedBuild]
				if r.unbuilt == nil {
					r.unbuilt = map[string]bool{}
				}
				r.unbuilt[ev.Package] = true
			}
			r.interrupted(ev.Package)
		case "pass", "skip":
			pkg.Status = ev.Action
		}
		return
	}
	key := ev.Package + "\x00" + ev.Test
	t := r.running[key]
	if t == nil {
		t = &testResult{Package: ev.Package, Name: ev.Test}
		r.running[key] = t
	}
	switch ev.Action {
	case "output":
		t.Output += ev.Output
		if strings.HasPrefix(ev.Test, "Benchmark") {
			r.benchmark(t)
		}
		if r.trace && !strings.HasPrefix(ev.Output, "=== ") && !strings.HasPrefix(ev.Output, "--- ") {
			fmt.Fprint(r.out, ev.Output)
		}
	case "pass", "fail", "skip":
		t.Status, t.Elapsed = ev.Action, time.Duration(ev.Elapsed*float64(time.Second))
		if t.Status == "skip" {
			t.Message = skipMessage(t.Output)
		}
		t.summarizeSkips()
		if i := strings.LastIndex(ev.Test, "/"); i >= 0 {
			if parent := r.running[ev.Package+"\x00"+ev.Test[:i]]; parent != nil {
				parent.children = append(parent.children, t)
			}
		}
		delete(r.running, key)
		r.results = append(r.results, t)
		r.print(t)
	}
}

// interrupted fails the tests still running when pkg's process ends without
// their closing events, as after a panic or timeout. Each shows its own output.
// A benchmark hands its output to the package because it has no closing event.
func (r *testRun) interrupted(pkg string) {
	var keys []string
	for key, t := range r.running {
		if t.Package == pkg {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		t := r.running[key]
		delete(r.running, key)
		if strings.HasPrefix(t.Name, "Benchmark") {
			r.packages[pkg].Output += t.Output
			continue
		}
		t.failFor("the test process exited before reporting an outcome")
		r.results = append(r.results, t)
		r.print(t)
	}
	// finish skips package output when a failed test already explains the failure.
	// An interrupted test can leave diagnostic lines attached to the package.
	if len(keys) > 0 && r.anyFailed(pkg) {
		var diagnostics strings.Builder
		for line := range strings.SplitSeq(r.packages[pkg].Output, "\n") {
			if line == "" || goTestLine(line) || strings.HasPrefix(line, "FAIL\t") {
				continue
			}
			diagnostics.WriteString(line)
			diagnostics.WriteByte('\n')
		}
		if diagnostics.Len() > 0 {
			fmt.Fprintf(r.out, "%s  %s\n%s", margin, pkg, indent(diagnostics.String()))
		}
	}
}

// benchmark reports each result line a benchmark has printed whole as a
// passed test, since go test -json ends benchmarks with no event.
func (r *testRun) benchmark(t *testResult) {
	for {
		line, _, ok := strings.Cut(t.Output[t.reported:], "\n")
		if !ok {
			return
		}
		t.reported += len(line) + 1
		if !strings.Contains(line, " ns/op") {
			continue
		}
		res := &testResult{Package: t.Package, Name: t.Name, Status: "pass", Output: line}
		r.results = append(r.results, res)
		fmt.Fprintf(r.out, "%s✓ %s\n", margin, strings.Join(strings.Fields(line), " "))
	}
}

// print shows one finished test: a line, and a failure's output, a
// subtest's under its own line (go test gives each test its own).
func (r *testRun) print(t *testResult) {
	mark := map[string]string{"pass": "✓", "fail": "✗", "skip": "-"}[t.Status]
	fmt.Fprintf(r.out, "%s%s %s (%s)\n", margin, mark, t.Name, t.Elapsed.Round(time.Millisecond))
	if t.Status == "fail" {
		for _, line := range strings.Split(strings.TrimRight(t.Output, "\n"), "\n") {
			if goTestLine(line) {
				continue
			}
			fmt.Fprintf(r.out, "%s    %s\n", margin, strings.TrimPrefix(line, "    "))
		}
	}
}

// finish prints the tally, writes the JUnit report and reports failure.
func (r *testRun) finish(opts testOptions, waitErr error) error {
	var passed, failed, skipped int
	for _, t := range r.results {
		// Subtests count with their test, unless a target process reported one alone; a benchmark's every result counts, its subbenchmarks' too.
		if !t.whole && strings.Contains(t.Name, "/") && !strings.HasPrefix(t.Name, "Benchmark") {
			continue
		}
		switch t.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	pkgFailed, unbuilt := false, false
	names := make([]string, 0, len(r.packages))
	for name := range r.packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pkg := r.packages[name]
		// A package that failed with no failed test did not build or
		// crashed outside a test.
		if pkg.Status == "fail" && !r.anyFailed(name) {
			pkgFailed = true
			unbuilt = unbuilt || r.unbuilt[name]
			fmt.Fprintf(r.out, "%s✗ %s\n%s", margin, name, indent(pkg.Output))
		}
	}
	fmt.Fprintf(r.out, "%s%d passed, %d failed, %d skipped\n", margin, passed, failed, skipped)
	if opts.junit != "" {
		if err := writeJUnit(opts.junit, r.results, r.packages); err != nil {
			return err
		}
		note("%s", rel(opts.junit))
	}
	switch {
	case failed > 0:
		if failed == 1 {
			return errors.New("1 test failed")
		}
		return fmt.Errorf("%d tests failed", failed)
	case unbuilt:
		return errors.New("tests did not build")
	case pkgFailed:
		return errors.New("a test process failed outside its tests")
	case waitErr != nil && passed+skipped == 0:
		return waitErr
	}
	return nil
}

func (r *testRun) anyFailed(pkg string) bool {
	for _, t := range r.results {
		if t.Package == pkg && t.Status == "fail" {
			return true
		}
	}
	return false
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(margin + "    " + line + "\n")
	}
	return b.String()
}

// The JUnit report's format, as CI systems read it.
type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Time     string       `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Time     string      `xml:"time,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Class   string        `xml:"classname,attr"`
	Name    string        `xml:"name,attr"`
	Time    string        `xml:"time,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
	Skipped *junitSkipped `xml:"skipped,omitempty"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

// writeJUnit writes results as a JUnit XML report at path, a suite per
// package; a package that did not build is a failed case of its own.
func writeJUnit(path string, results []*testResult, packages map[string]*testResult) error {
	bySuite := map[string]*junitSuite{}
	var order []string
	suite := func(pkg string) *junitSuite {
		s := bySuite[pkg]
		if s == nil {
			s = &junitSuite{Name: pkg}
			bySuite[pkg] = s
			order = append(order, pkg)
		}
		return s
	}
	var total, failures int
	var elapsed time.Duration
	add := func(t *testResult) {
		s := suite(t.Package)
		c := junitCase{Class: t.Package, Name: t.Name, Time: seconds(t.Elapsed)}
		switch t.Status {
		case "fail":
			msg := t.Message
			if msg == "" {
				msg = firstLine(t.Output)
			}
			c.Failure = &junitFailure{Message: msg, Body: t.Output}
			s.Failures++
			failures++
		case "skip":
			// Message holds the reason wherever a skip is reported; a target result's Output is its whole process's, host logs included.
			c.Skipped = &junitSkipped{Message: t.Message}
			s.Skipped++
		}
		s.Tests++
		total++
		s.Cases = append(s.Cases, c)
		elapsed += t.Elapsed
	}
	for _, t := range results {
		add(t)
	}
	for name, pkg := range packages {
		failed := false
		for _, t := range results {
			failed = failed || t.Package == name && t.Status == "fail"
		}
		if pkg.Status == "fail" && !failed {
			add(&testResult{Package: name, Name: "build", Status: "fail", Output: pkg.Output})
		}
	}
	sort.Strings(order)
	out := junitSuites{Tests: total, Failures: failures, Time: seconds(elapsed)}
	for _, name := range order {
		s := bySuite[name]
		var d time.Duration
		for _, c := range s.Cases {
			v, _ := time.ParseDuration(c.Time + "s")
			d += v
		}
		s.Time = seconds(d)
		out.Suites = append(out.Suites, *s)
	}
	data, err := xml.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), append(data, '\n')...), 0o644)
}

func seconds(d time.Duration) string { return fmt.Sprintf("%.3f", d.Seconds()) }

// firstLine is a failure's message: the first line it printed that is not go test's own.
func firstLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" && !goTestLine(line) {
			return line
		}
	}
	return ""
}

// skipMessage keeps the test's diagnostics, including multiline Skip reasons, without Go's progress and result lines.
func skipMessage(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" && !goTestLine(line) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// goTestLine reports whether line is go test -v's own rather than the test's: a test's start or result, or the process's closing PASS or FAIL.
func goTestLine(line string) bool {
	return strings.HasPrefix(line, "=== ") || strings.HasPrefix(line, "--- ") || line == "PASS" || line == "FAIL"
}
