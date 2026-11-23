package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// testTimeout bounds one test's run on a target.
const testTimeout = 3 * time.Minute

// targetRunner runs one test of the built package on a target: the test
// binary with args, its environment holding env, writing what it prints
// to out. It returns the process's exit code.
type targetRunner struct {
	run   func(ctx context.Context, args, env []string, out io.Writer) (int, error)
	close func()
	// timesFromStart reports that run holds the test to -timeout itself, from the moment the test host starts, so the context it gets carries no deadline.
	timesFromStart bool
}

// targetDeviceLostError stops a target run once when the device disappears; a transient test-level failure remains attached to that test instead.
type targetDeviceLostError struct{ err error }

func (e *targetDeviceLostError) Error() string { return e.err.Error() }
func (e *targetDeviceLostError) Unwrap() error { return e.err }

// runTargetTests runs the app's tests on opts.target, each test in a
// process of its own, as the platform shows one app at a time: the test
// binary, built with the backend's test host, is the target's package.
func runTargetTests(ctx context.Context, p *project, opts testOptions) error {
	flags, err := parseTargetTestFlags(opts.args)
	if err != nil {
		return err
	}
	if flags.count <= 0 {
		plain("no tests run (-count=%d)", flags.count)
		return (&testRun{out: os.Stderr}).finish(opts, nil)
	}
	// Even a package with portable files can have tests that exist only on the target, so discover tests and their host as the target builds them.
	names, err := listTests(ctx, p, opts.target, opts.run, flags.args...)
	if err != nil {
		return err
	}
	if p.steps != nil {
		names = []string{stepsTest}
	}
	if len(names) == 0 {
		plain("no tests match")
		return nil
	}
	hosted, err := linksPlatform(ctx, p, opts.target)
	if err != nil {
		return err
	}
	if needsTestHost(opts.target) || opts.activate {
		funcs, err := testFuncs(ctx, p, opts.target)
		if err != nil {
			return err
		}
		if err := testHostError(opts.target, hosted, slices.Contains(funcs, "TestMain")); err != nil {
			return err
		}
	}
	p.testBinary = true
	progress := begin("building the tests for %s", opts.target)
	runner, err := targetFor(ctx, p, opts)
	if err != nil {
		progress.fail()
		return err
	}
	progress.done(fmt.Sprintf("built the tests for %s", opts.target), "")
	defer runner.close()
	shots := opts.screenshots
	if shots == "" {
		shots = filepath.Join(p.Dir, ".nsgo", "test", "screenshots", opts.target)
	} else if !filepath.IsAbs(shots) {
		shots, err = filepath.Abs(shots)
		if err != nil {
			return err
		}
	}
	r := &testRun{running: map[string]*testResult{}, packages: map[string]*testResult{}, trace: opts.trace, out: os.Stderr}
	savedPaths := map[string]bool{}
	repeated := hosted && (flags.count > 1 || len(flags.cpus) > 1)
	// A package that links no platform, as a library's, runs headless in the target's runtime, the web's engine say, but for the targets needsTestHost names, which refused it above.
	env := slices.Clone(opts.env)
	if hosted {
		env = append(env, "NSGO_TEST_TARGET="+opts.target)
	}
	if opts.activate {
		env = append(env, "NSGO_TEST_ACTIVATE=1")
	}
	if opts.trace {
		env = append(env, "NSGO_TEST_TRACE=1")
	}
	if opts.fps {
		env = append(env, "NSGO_TEST_FPS=1")
	}
	depth := runDepth(opts.run)
tests:
	for _, name := range names {
		iteration := 0
		for execution := range flags.executions(hosted) {
			iteration++
			var out bytes.Buffer
			w := io.MultiWriter(&out, traceWriter{r.out, opts.trace, opts.fps})
			tctx, cancel := context.WithCancel(ctx)
			if opts.timeout > 0 && !runner.timesFromStart {
				tctx, cancel = context.WithTimeout(ctx, opts.timeout)
			}
			started := time.Now()
			args := append(targetTestArgs(name, opts.run, opts.target, opts.timeout), execution...)
			code, err := runner.run(tctx, args, env, w)
			timedOut := errors.Is(tctx.Err(), context.DeadlineExceeded)
			cancel()
			var lost *targetDeviceLostError
			if errors.As(err, &lost) {
				// The tests that ran still get their summary and JUnit report; the rest did not run.
				return errors.Join(err, r.finish(opts, nil))
			}
			if timedOut {
				code, err = -1, timeoutError(opts.timeout)
			} else if ctx.Err() != nil {
				return ctx.Err()
			}
			dir := shots
			if repeated {
				dir = filepath.Join(shots, fmt.Sprintf("run-%d", iteration))
			}
			t, saved := targetResult(name, depth, out.String(), code, err, dir, savedPaths)
			if t.Elapsed == 0 {
				t.Elapsed = time.Since(started)
			}
			t.Package = p.ID
			if repeated {
				t.Name = fmt.Sprintf("%s [run %d]", t.Name, iteration)
			}
			r.results = append(r.results, t)
			r.print(t)
			for _, s := range saved {
				fmt.Fprintf(r.out, "%s  → %s\n", margin, rel(s))
			}
			if flags.failfast && t.Status == "fail" {
				break tests
			}
		}
	}
	return r.finish(opts, nil)
}

// needsTestHost reports whether target runs a test only through the platform's test host, which nsgotest.Main starts: Android's runtime reads its protocol from the test process's standard output, where go test would otherwise write its own lines, and the tool reads the test's output and exit code on Android and an iPhone between the marks the host writes (readMarkedRun).
func needsTestHost(target string) bool {
	return target == "android" || target == "ios" || target == "tui"
}

// testHostError says why a package's tests cannot run on a target that needs the test host, or with -activate, before the tool builds them: they do not link it, through github.com/nsgo-dev/nsgo, or declare no TestMain to start it from.
func testHostError(target string, hosted, testMain bool) error {
	switch {
	case !hosted:
		return fmt.Errorf("test: -target %s runs the tests in NSGo's test host, which this package's tests do not link: import github.com/nsgo-dev/nsgo in a test file (a blank import is enough), or run them headless", target)
	case !testMain:
		return fmt.Errorf("test: -target %s runs the tests in NSGo's test host, which needs the main thread: add func TestMain(m *testing.M) { nsgotest.Main(m) }", target)
	}
	return nil
}

// traceWriter passes a test's trace lines on as they come: the batches
// with trace, the performance monitor's readings with fps, and the marks
// of what they measure (nsgotest's Mark) always, so a run without the
// monitor can be timed by them too.
type traceWriter struct {
	w          io.Writer
	trace, fps bool
}

func (t traceWriter) Write(b []byte) (int, error) {
	for _, line := range strings.SplitAfter(string(b), "\n") {
		switch {
		case t.trace && (strings.HasPrefix(line, "──") || strings.HasPrefix(line, "   ")):
			io.WriteString(t.w, line)
		case strings.Contains(line, "nsgo: perf ") && (t.fps || strings.Contains(line, "nsgo: perf mark ")):
			io.WriteString(t.w, margin+line[strings.Index(line, "nsgo: perf ")+len("nsgo: "):])
		}
	}
	return len(b), nil
}

// linksPlatform reports whether the package's tests link the platform's test host, as target builds them, or this machine for "": through NSGo's root package, or a backend that registers the host itself, whose own tests cannot import the root package that imports them.
func linksPlatform(ctx context.Context, p *project, target string) (bool, error) {
	out, err := goListFor(ctx, p, target, "-test", "-deps", "-f", "{{.ImportPath}}")
	if err != nil {
		return false, err
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "github.com/nsgo-dev/nsgo" || slices.Contains(hostBackends, dep) {
			return true, nil
		}
	}
	return false, nil
}

// hostBackends are the backends that register their platform's test host (internal/testhost.Register) when built with the nsgotest tag.
var hostBackends = []string{
	"github.com/nsgo-dev/nsgo/internal/platform/android",
	"github.com/nsgo-dev/nsgo/internal/platform/harmonyos",
	"github.com/nsgo-dev/nsgo/internal/platform/ios",
	"github.com/nsgo-dev/nsgo/internal/platform/linux",
	"github.com/nsgo-dev/nsgo/internal/platform/macos",
	"github.com/nsgo-dev/nsgo/internal/platform/tui",
	"github.com/nsgo-dev/nsgo/internal/platform/watchos",
	"github.com/nsgo-dev/nsgo/internal/platform/web",
	"github.com/nsgo-dev/nsgo/internal/platform/windows",
}

// goListFor runs go list with args on the package as target's test build
// sees it, or this machine's for "".
func goListFor(ctx context.Context, p *project, target string, args ...string) ([]byte, error) {
	var env []string
	if target != "" {
		goos, _, _ := strings.Cut(target, "_")
		switch goos {
		case "ios", "tvos", "watchos":
			// Listing files needs only the target's build context, not its SDK or compiler, so the test host check runs before the tool looks for Xcode.
			env = []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1"}
		case "android":
			env = []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}
		default:
			var err error
			if env, err = vetEnv(ctx, target); err != nil {
				return nil, err
			}
		}
	}
	tags, err := goTags(ctx, p, env, append([]string{"nsgotest"}, targetTags(target)...)...)
	if err != nil {
		return nil, err
	}
	argv := append(append([]string{"list"}, tagsArgs(tags)...), args...)
	var stderr bytes.Buffer
	out, err := execute(ctx, processCommand{
		name: "go", args: append(argv, "."), dir: p.Dir, env: append(os.Environ(), env...), stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("go list: %w\n%s", err, stderr.Bytes())
	}
	return out, nil
}

// listTests lists the package's tests that run matches, as go test -list
// does on this machine, or from its sources as target sees them.
func listTests(ctx context.Context, p *project, target, run string, extra ...string) ([]string, error) {
	if run == "" {
		run = "."
	}
	top, _ := splitTestRun(run)
	if target != "" {
		return listTestSources(ctx, p, target, top)
	}
	tags, err := goTags(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	args := append([]string{"test", "-list", top}, tagsArgs(tags)...)
	args = append(args, extra...)
	// The TSX tests are in a file the tool adds.
	if tsxTest, err := p.tsxTestFile(); err != nil {
		return nil, err
	} else if tsxTest != "" {
		ov, err := p.overlay(map[string]string{"zz_nsgo_tsx_test.go": tsxTest})
		if err != nil {
			return nil, err
		}
		args = append(args, "-overlay", ov)
	}
	var stderr bytes.Buffer
	out, err := execute(ctx, processCommand{
		name: "go", args: append(args, "."), dir: p.Dir, stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("go test -list: %w\n%s%s", err, out, stderr.Bytes())
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") {
			names = append(names, strings.TrimSpace(line))
		}
	}
	return names, nil
}

// splitTestRun returns the top-level part of go test's hierarchical -run
// expression and the remaining subtest expression. Slashes inside character
// classes or parentheses are part of the regexp, as they are in testing.
func splitTestRun(run string) (top, sub string) {
	classes, groups := 0, 0
	for i := 0; i < len(run); i++ {
		switch run[i] {
		case '[':
			classes++
		case ']':
			if classes > 0 {
				classes--
			}
		case '(':
			if classes == 0 {
				groups++
			}
		case ')':
			if classes == 0 && groups > 0 {
				groups--
			}
		case '\\':
			i++
		case '/':
			if classes == 0 && groups == 0 {
				return run[:i], run[i+1:]
			}
		}
	}
	return run, ""
}

// runDepth is how many levels of subtests run selects below the top-level test, as go test splits it: 1 for TestGallery/dark-home, 0 without a slash.
func runDepth(run string) int {
	depth := 0
	for _, sub := splitTestRun(run); sub != ""; _, sub = splitTestRun(sub) {
		depth++
	}
	return depth
}

// targetTestArgs runs exactly one top-level test while retaining go test's
// hierarchical subtest matching when -run contains a slash.
func targetTestArgs(name, run, target string, timeout time.Duration) []string {
	_, sub := splitTestRun(run)
	pattern := "^" + regexp.QuoteMeta(name) + "$"
	if sub != "" {
		pattern += "/" + sub
	}
	verbose := "-test.v"
	switch target {
	case "ios", "ios_simulator", "tvos", "tvos_simulator", "watchos", "watchos_simulator":
		// Keep Go's result lines identifiable when the device console
		// combines platform library output with the test's stdout.
		verbose = "-test.v=test2json"
	}
	args := []string{"-test.run", pattern, verbose}
	if timeout > 0 {
		budget := timeout - time.Second
		if budget <= 0 {
			budget = timeout
		}
		args = append(args, "-test.timeout", budget.String())
	}
	return args
}

func timeoutError(timeout time.Duration) error {
	return fmt.Errorf("the test took over %v", timeout)
}

func testContextError(ctx context.Context, timeout time.Duration) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return timeoutError(timeout)
	}
	return ctx.Err()
}

// listTestSources lists the top-level tests and the examples with output that run matches in the package's test files for target, as go test -list does.
func listTestSources(ctx context.Context, p *project, target, run string) ([]string, error) {
	re, err := regexp.Compile(run)
	if err != nil {
		return nil, fmt.Errorf("-run: %w", err)
	}
	funcs, err := testFuncs(ctx, p, target)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range funcs {
		// TestMain runs the tests rather than being one.
		if name != "TestMain" && (isTestName(name) || strings.HasPrefix(name, "Example")) && re.MatchString(name) {
			names = append(names, name)
		}
	}
	return names, nil
}

// testFuncs lists the functions the package's test files declare for target, methods and examples without output aside, which go test compiles but does not run, with the file of TSX tests the tool adds to every target's test build.
func testFuncs(ctx context.Context, p *project, target string) ([]string, error) {
	out, err := goListFor(ctx, p, target, "-f", "{{range .TestGoFiles}}{{$.Dir}}/{{.}}\n{{end}}{{range .XTestGoFiles}}{{$.Dir}}/{{.}}\n{{end}}")
	if err != nil {
		return nil, err
	}
	paths := strings.Split(strings.TrimSpace(string(out)), "\n")
	tsxTest, err := p.tsxTestFile()
	if err != nil {
		return nil, err
	}
	if tsxTest != "" {
		paths = append(paths, tsxTest)
	}
	var names []string
	fset := token.NewFileSet()
	for _, path := range paths {
		if path == "" {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return nil, err
		}
		runs := map[string]bool{}
		for _, e := range doc.Examples(f) {
			if e.Output != "" || e.EmptyOutput {
				runs["Example"+e.Name] = true
			}
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && (!strings.HasPrefix(fn.Name.Name, "Example") || runs[fn.Name.Name]) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names, nil
}

// isTestName reports whether name names a test, as go test reads it:
// Test, then no lower-case letter.
func isTestName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Test")
	if !ok {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || !unicode.IsLower(r)
}

// targetResult turns one test process's output into its result and saves the screenshots it carries in dir, returning the files written.
func targetResult(name string, depth int, out string, code int, runErr error, dir string, seen map[string]bool) (*testResult, []string) {
	text, saved, saveErr := saveScreenshots(out, dir, seen)
	t := parseTestOutput(name, depth, text, code, runErr)
	// A screenshot the tool cannot save, as one whose path an earlier one took, fails its test, and the run goes on.
	if saveErr != nil {
		t.failFor(saveErr.Error())
	}
	return t, saved
}

// testLine matches go test -v's result line of a test, once its indentation is trimmed.
var testLine = regexp.MustCompile(`^--- (PASS|FAIL|SKIP): (\S+) \(([0-9.]+)s\)`)

// parseTestOutput turns one test process's output into its result, that of the subtest depth levels down when -run selected one (runDepth); a process that printed no result for it crashed or timed out.
func parseTestOutput(name string, depth int, out string, code int, runErr error) *testResult {
	// Go's test2json mode prefixes its own lines with SYN. A platform
	// library can write without ending its line before Go prints a result.
	const mark = "\x16"
	out = strings.ReplaceAll(out, "\n"+mark, "\n")
	out = strings.ReplaceAll(strings.TrimPrefix(out, mark), mark, "\n")
	t := &testResult{Name: name, Output: out, whole: true}
	// Results by name across in-process repetitions: elapsed times add up, a failure outweighs a pass and a pass a skip.
	results := map[string]*testResult{}
	outputs := map[string]string{}
	current := ""
	var subtests []string
	for _, line := range strings.Split(out, "\n") {
		// Each of Go's own lines, RUN, PAUSE, CONT, NAME and the rest, names the test the lines after it belong to, as its verbose printer tracks it.
		if fields := strings.Fields(line); len(fields) >= 3 && fields[0] == "===" {
			current = fields[2]
			continue
		}
		m := testLine.FindStringSubmatch(strings.TrimLeft(line, " \t"))
		if m == nil {
			if current == name || strings.HasPrefix(current, name+"/") {
				outputs[current] += line + "\n"
			}
			continue
		}
		// Go names a test again before any later output of it, so what follows a result line without a new name, as the closing PASS or the host's own logs, belongs to no test.
		current = ""
		if m[2] != name && !strings.HasPrefix(m[2], name+"/") {
			continue
		}
		s, _ := strconv.ParseFloat(m[3], 64)
		status := strings.ToLower(m[1])
		r := results[m[2]]
		if r == nil {
			r = &testResult{Name: m[2], Status: status}
			results[m[2]] = r
			if m[2] != name {
				subtests = append(subtests, m[2])
			}
		} else if status == "fail" || r.Status == "skip" {
			r.Status = status
		}
		r.Elapsed += time.Duration(s * float64(time.Second))
	}
	for key, r := range results {
		if r.Status == "skip" {
			r.Message = skipMessage(outputs[key])
		}
	}
	for _, sub := range subtests {
		if i := strings.LastIndex(sub, "/"); i >= 0 {
			if parent := results[sub[:i]]; parent != nil {
				parent.children = append(parent.children, results[sub])
			}
		}
	}
	if top := results[name]; top != nil {
		top.summarizeSkips()
	}
	// A run whose -run reached exactly one subtest at the depth it selects, as Test/sub or Test/sub/leaf does, reports that subtest, not its own subtests, unless the test failed outside it; otherwise the top-level result covers every subtest, so a test with several is never reported as its first.
	matched := name
	var selected []string
	for _, sub := range subtests {
		if depth > 0 && strings.Count(sub, "/") == depth {
			selected = append(selected, sub)
		}
	}
	if top := results[name]; len(selected) == 1 && (top == nil || top.Status != "fail" || results[selected[0]].Status == "fail") {
		matched = selected[0]
	}
	if r := results[matched]; r != nil {
		r.summarizeSkips()
		t.Name, t.Status, t.Elapsed, t.Message = matched, r.Status, r.Elapsed, r.Message
	}
	if t.Status == "" || runErr != nil || t.Status != "fail" && code != 0 {
		why := fmt.Sprintf("the test process exited with %d", code)
		if runErr != nil {
			why = runErr.Error()
		}
		// A process that crashed before its result keeps the crash's first line as the message.
		t.failFor(why)
	}
	return t
}

// screenshotLine is how nsgotest hands the tool a screenshot: its encoded name, the chunk's number and count, the PNG's length and CRC-32, then the PNG in base64, in chunks a device's log keeps whole.
var screenshotLine = regexp.MustCompile(`nsgo-screenshot (\S+) (\d+)/(\d+) (\d+):([0-9a-f]{8}) (\S*)`)

// saveScreenshots saves the screenshots out carries in dir and returns out without them, the files written and an error for each shot whose chunks are missing, repeated, inconsistent or do not add up to the PNG they declare, or whose path is invalid, taken before or cannot be written, which it skips.
func saveScreenshots(out, dir string, seen map[string]bool) (string, []string, error) {
	var kept strings.Builder
	// Each Screenshot call is a shot of its own, so a name taken twice in one process is a duplicate rather than one image's chunks overwritten.
	type shot struct {
		name  string
		total int
		// sum is the PNG's length and CRC-32 as its first chunk declares them.
		sum   string
		parts []string
		got   []bool
		errs  []error
	}
	var shots []*shot
	current := map[string]*shot{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	for sc.Scan() {
		line := sc.Text()
		m := screenshotLine.FindStringSubmatch(line)
		if m == nil {
			kept.WriteString(line + "\n")
			continue
		}
		name := m[1]
		if strings.HasPrefix(name, "~") {
			decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(name, "~"))
			if err != nil {
				kept.WriteString("screenshot: invalid name encoding\n")
				continue
			}
			name = string(decoded)
		}
		i, iErr := strconv.Atoi(m[2])
		n, nErr := strconv.Atoi(m[3])
		// A first chunk starts a new shot of the name.
		s := current[name]
		if s == nil || i == 1 {
			s = &shot{name: name, total: n, sum: m[4] + ":" + m[5]}
			if n > 0 {
				s.parts = make([]string, n)
				s.got = make([]bool, n)
			}
			current[name] = s
			shots = append(shots, s)
		}
		if iErr != nil || nErr != nil || n <= 0 || i <= 0 {
			s.errs = append(s.errs, fmt.Errorf("screenshot %q: invalid chunk %q/%q", name, m[2], m[3]))
			continue
		}
		if n != s.total || m[4]+":"+m[5] != s.sum {
			s.errs = append(s.errs, fmt.Errorf("screenshot %q: chunk %d declares %d parts of %s, want %d of %s", name, i, n, m[4]+":"+m[5], s.total, s.sum))
			continue
		}
		if i >= 1 && i <= len(s.parts) {
			if s.got[i-1] {
				s.errs = append(s.errs, fmt.Errorf("screenshot %q: chunk %d was repeated", name, i))
				continue
			}
			s.parts[i-1] = m[6]
			s.got[i-1] = true
		} else {
			s.errs = append(s.errs, fmt.Errorf("screenshot %q: chunk %d is outside 1/%d", name, i, s.total))
		}
	}
	var saved []string
	var errs []error
	for _, s := range shots {
		name := s.name
		errs = append(errs, s.errs...)
		if len(s.parts) == 0 {
			continue
		}
		var missing []string
		for i, ok := range s.got {
			if !ok {
				missing = append(missing, strconv.Itoa(i+1))
			}
		}
		if len(missing) > 0 {
			errs = append(errs, fmt.Errorf("screenshot %q: missing chunks %s of %d", name, strings.Join(missing, ", "), s.total))
			continue
		}
		data, err := base64.StdEncoding.DecodeString(strings.Join(s.parts, ""))
		if err != nil {
			errs = append(errs, fmt.Errorf("screenshot %q: %w", name, err))
			continue
		}
		// A chunk the device's log cut short can still decode.
		if got := fmt.Sprintf("%d:%08x", len(data), crc32.ChecksumIEEE(data)); got != s.sum {
			errs = append(errs, fmt.Errorf("screenshot %q: its chunks hold %s (length:CRC-32), not the %s it declares: the device's log cut a chunk short", name, got, s.sum))
			continue
		}
		// A name that cleans to "." names dir itself, which .png would turn into a file beside it.
		if name == "" || filepath.Clean(name) == "." || filepath.IsAbs(name) || screenshotPathHasParent(name) {
			errs = append(errs, fmt.Errorf("screenshot %q: name must be a relative file path without ..", name))
			continue
		}
		path := filepath.Join(dir, name)
		if filepath.Ext(path) != ".png" {
			path += ".png"
		}
		path = filepath.Clean(path)
		if seen[path] || sameFileAsSeen(path, seen) {
			errs = append(errs, fmt.Errorf("screenshot %q resolves to %s more than once", name, path))
			continue
		}
		seen[path] = true
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			errs = append(errs, fmt.Errorf("screenshot %q: create directory: %w", name, err))
			continue
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			errs = append(errs, fmt.Errorf("screenshot %q: %w", name, err))
			continue
		}
		saved = append(saved, path)
	}
	return kept.String(), saved, errors.Join(errs...)
}

// sameFileAsSeen reports whether path names the file of a path in seen under another spelling, as "Home.png" and "home.png" do on the case-insensitive volumes macOS and Windows default to.
func sameFileAsSeen(path string, seen map[string]bool) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	for p := range seen {
		if pi, err := os.Stat(p); err == nil && os.SameFile(fi, pi) {
			return true
		}
	}
	return false
}

func screenshotPathHasParent(name string) bool {
	for _, part := range strings.FieldsFunc(filepath.ToSlash(name), func(r rune) bool { return r == '/' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

// targetFor builds the tests as opts.target's package and returns how to
// run them there.
func targetFor(ctx context.Context, p *project, opts testOptions) (targetRunner, error) {
	base := p.cache("test", opts.target)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return targetRunner{}, err
	}
	sweepTargetBuilds(base, time.Now())
	// Another run may still serve or execute its build. Keep this run's files until its runner closes, without replacing anyone else's.
	dir, err := os.MkdirTemp(base, "run-")
	if err != nil {
		return targetRunner{}, err
	}
	runner, err := buildTargetRunner(ctx, p, dir, opts)
	if err != nil {
		os.RemoveAll(dir)
		return targetRunner{}, err
	}
	run, stop := runner.run, runner.close
	runner.run = func(ctx context.Context, args, env []string, out io.Writer) (int, error) {
		// A live run's directory stays fresh, so another run's sweep never takes it.
		now := time.Now()
		os.Chtimes(dir, now, now)
		return run(ctx, args, env, out)
	}
	runner.close = func() {
		stop()
		os.RemoveAll(dir)
	}
	return runner, nil
}

// staleTargetBuild is how long a run's build directory stays untouched before a later run takes it for a crashed run's.
const staleTargetBuild = 24 * time.Hour

// sweepTargetBuilds removes from base what no live run uses: the builds of runs that ended without closing their runner, as a killed tool's, once untouched for staleTargetBuild, and anything not a run's directory, as the single build earlier versions kept there.
func sweepTargetBuilds(base string, now time.Time) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "run-") {
			info, err := e.Info()
			if err != nil || now.Sub(info.ModTime()) < staleTargetBuild {
				continue
			}
		}
		os.RemoveAll(filepath.Join(base, e.Name()))
	}
}

// testTargets are the names nsgo test -target accepts, as buildTargetRunner
// runs them: one per OS, except the Apple simulators, which need Xcode.
var testTargets = []string{"macos", "windows", "linux", "tui", "web", "ios", "ios_simulator", "tvos_simulator", "watchos_simulator", "android", "harmonyos"}

// checkTestTarget normalizes -target to the test target it runs on, and
// checks -activate and the architecture it names, before runTests touches
// the project (#589).
func checkTestTarget(target string, activate bool) (string, error) {
	if target == "" {
		if activate {
			return "", errors.New("test: -activate goes with -target macos")
		}
		return "", nil
	}
	normalized, err := normalizeTestTarget(target)
	if err != nil {
		return "", err
	}
	if activate && normalized != "macos" {
		return "", errors.New("test: -activate goes with -target macos")
	}
	if err := checkTestTargetArch(target, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

// checkTestTargetArch rejects an arch-qualified spelling whose architecture
// is not the one the target's tests run on, naming the spelling to use
// instead (#589): a desktop target runs on this machine's architecture,
// Android's tests always run arm64.
//
// A desktop target whose OS is not this machine's is left to buildTargetRunner, which says where it runs: suggesting the plain name would only lead to that error (#716).
func checkTestTargetArch(target, normalized string) error {
	if normalized == "tui" {
		return nil
	}
	if target == normalized || desktopTargetElsewhere(normalized) {
		return nil
	}
	_, arch, _ := strings.Cut(target, "_")
	runs := "arm64"
	if normalized != "android" {
		runs = hostTestArch()
	}
	if arch == runs {
		return nil
	}
	why := fmt.Sprintf("the %s tests run for %s on this machine", normalized, runs)
	if normalized == "android" {
		why = "Android's tests always run arm64"
	}
	return fmt.Errorf("test: -target %s asks for %s, but %s; use -target %s", target, arch, why, normalized)
}

// desktopTargetElsewhere reports whether target is a desktop target whose OS is not this machine's.
func desktopTargetElsewhere(target string) bool {
	switch target {
	case "macos":
		return runtime.GOOS != "darwin"
	case "windows", "linux":
		return runtime.GOOS != target
	}
	return false
}

// hostTestArch is the architecture this machine's tests run for, in the
// spellings -target uses.
func hostTestArch() string {
	if runtime.GOARCH == "amd64" {
		return "x64"
	}
	return runtime.GOARCH
}

// normalizeTestTarget maps an arch-qualified spelling (windows_x64) to the
// test target it runs on, as check and build spell it, or reports the names
// test accepts (#589). Only the arch-qualified names build.go's targets
// lists are accepted, so windows_foo, macos_ or linux_riscv64 are refused.
func normalizeTestTarget(target string) (string, error) {
	if strings.HasPrefix(target, "tui_") && slices.Contains(targets, target) {
		if target != tuiTarget(runtime.GOOS, runtime.GOARCH) {
			return "", fmt.Errorf("terminal tests run on this machine; use -target tui")
		}
		return "tui", nil
	}
	if slices.Contains(testTargets, target) {
		return target, nil
	}
	if goos, _, ok := strings.Cut(target, "_"); ok && slices.Contains(targets, target) {
		switch goos {
		case "macos", "windows", "linux", "android":
			return goos, nil
		}
	}
	return "", fmt.Errorf("unknown test target %q; use %s", target, testTargetList())
}

// testTargetList is the names test accepts, as one phrase for errors.
func testTargetList() string {
	return strings.Join(testTargets[:len(testTargets)-1], ", ") + " or " + testTargets[len(testTargets)-1]
}

// buildTargetRunner builds into the run's own directory.
func buildTargetRunner(ctx context.Context, p *project, dir string, opts testOptions) (targetRunner, error) {
	local := func(bin string) targetRunner {
		return targetRunner{run: func(ctx context.Context, args, env []string, out io.Writer) (int, error) {
			return runLocal(ctx, bin, args, env, out)
		}, close: func() {}}
	}
	switch opts.target {
	case "tui":
		return tuiTestRunner(ctx, p, dir)
	case "macos":
		if runtime.GOOS != "darwin" {
			return targetRunner{}, fmt.Errorf("the macos target runs on a Mac")
		}
		app, err := buildMacApp(ctx, p, dir, runtime.GOARCH)
		if err != nil {
			return targetRunner{}, err
		}
		return local(filepath.Join(app, "Contents", "MacOS", p.Name)), nil
	case "windows":
		if runtime.GOOS != "windows" {
			return targetRunner{}, fmt.Errorf("the windows target runs on Windows")
		}
		if err := buildWindows(ctx, p, dir, runtime.GOARCH); err != nil {
			return targetRunner{}, err
		}
		return local(filepath.Join(dir, p.Name+".exe")), nil
	case "linux":
		if runtime.GOOS != "linux" {
			return targetRunner{}, fmt.Errorf("the linux target runs on Linux")
		}
		if err := buildLinux(ctx, p, dir, runtime.GOARCH); err != nil {
			return targetRunner{}, err
		}
		return local(filepath.Join(dir, p.Name)), nil
	case "web":
		return webTestRunner(ctx, p, dir, opts)
	case "ios":
		return iosDeviceTestRunner(ctx, p, iOS, dir, opts)
	case "ios_simulator":
		return iosTestRunner(ctx, p, iOS, dir, opts)
	case "tvos_simulator":
		return iosTestRunner(ctx, p, tvOS, dir, opts)
	case "watchos_simulator":
		return iosTestRunner(ctx, p, watchOS, dir, opts)
	case "android":
		return androidTestRunner(ctx, p, dir, opts)
	case "harmonyos":
		return harmonyTestRunner(ctx, p, dir, opts)
	}
	return targetRunner{}, fmt.Errorf("unknown test target %q; use %s", opts.target, testTargetList())
}

// runLocal runs bin on this machine.
func runLocal(ctx context.Context, bin string, args, env []string, out io.Writer) (int, error) {
	_, err := execute(ctx, processCommand{
		name: bin, args: args, dir: filepath.Dir(bin), env: append(os.Environ(), env...),
		stdout: out, stderr: out, output: streamOutput,
	})
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), nil
	}
	return 0, err
}
