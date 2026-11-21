package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A malformed Android pipe can end before TestMain starts the host. The marked error must end the run immediately, without waiting for logcat (a stream that stays open) or accepting an earlier run's failure.
func TestReadMarkedRunReportsRuntimeFailureBeforeStart(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "before start", true: "after start"}[started], func(t *testing.T) {
			r, w := io.Pipe()
			t.Cleanup(func() { r.Close(); w.Close() })
			deadline := time.AfterFunc(5*time.Second, func() { w.Close() })
			defer deadline.Stop()
			go func() {
				io.WriteString(w, "nsgo-test-error old an earlier failure\n")
				if started {
					io.WriteString(w, "nsgo-test-start t1\n")
				}
				io.WriteString(w, "nsgo-test-error t1 NSGo pipe: stdout is reserved; call nsgotest.Main from TestMain\r\n")
			}()
			code, err := readMarkedRun(r, "t1", &strings.Builder{}, func() error {
				t.Error("waited for the device log to end")
				return errors.New("ended")
			}, nil)
			if code != -1 || err == nil || err.Error() != "NSGo pipe: stdout is reserved; call nsgotest.Main from TestMain" {
				t.Fatalf("code %d, err %v", code, err)
			}
		})
	}
}

func TestReadMarkedRunKeepsThisRunsOutput(t *testing.T) {
	lines := strings.Join([]string{
		"Launched application with dev.nsgo.app bundle identifier.",
		"nsgo-test-start old",
		"from an earlier run",
		"nsgo-test-start t1\r",
		"=== RUN   TestA",
		"nsgo-screenshot ~YQ 1/1 iVBOR",
		"--- PASS: TestA (0.01s)",
		"nsgo-test-exit t1 3",
		"after the mark",
	}, "\n")
	var out strings.Builder
	code, err := readMarkedRun(strings.NewReader(lines), "t1", &out, func() error { return errors.New("ended") }, nil)
	if err != nil || code != 3 {
		t.Fatalf("code %d, err %v", code, err)
	}
	if want := "=== RUN   TestA\nnsgo-screenshot ~YQ 1/1 iVBOR\n--- PASS: TestA (0.01s)\n"; out.String() != want {
		t.Errorf("out %q, want %q", out.String(), want)
	}
}

func TestReadMarkedRunWithoutAnExitMark(t *testing.T) {
	ended := errors.New("ended")
	if _, err := readMarkedRun(strings.NewReader("nsgo-test-start t1\n=== RUN TestA\n"), "t1", &strings.Builder{}, func() error { return ended }, nil); err != ended {
		t.Errorf("err %v", err)
	}
	if _, err := readMarkedRun(strings.NewReader("nsgo-test-start t1\nnsgo-test-exit t1 x\n"), "t1", &strings.Builder{}, func() error { return ended }, nil); err == nil || err == ended {
		t.Errorf("a mark without a code: %v", err)
	}
}

func TestWriteAndroidTimeoutReport(t *testing.T) {
	var out strings.Builder
	writeAndroidTimeoutReport(&out, "run-1", []byte("nsgo-test-start run-1\n=== RUN TestIME\n"), "native detail\n")
	if got := out.String(); !strings.Contains(got, "host started: true") || !strings.Contains(got, "native detail") {
		t.Fatalf("report = %q", got)
	}
	out.Reset()
	writeAndroidTimeoutReport(&out, "run-2", []byte("launcher detail\n"), "")
	if got := out.String(); !strings.Contains(got, "host started: false") || !strings.Contains(got, "launcher detail") {
		t.Fatalf("pre-start report = %q", got)
	}
}

// Exercise the launcher process itself: simctl can fail after a fast app has already reported its result, or exit either way after the app has crashed.
func TestSimulatorTestReadsTheAppsExitCode(t *testing.T) {
	for _, tc := range []struct {
		name, body, output string
		wantCode           int
		wantError          string
	}{
		{name: "fast app before launch failure", body: `printf 'nsgo-test-start %s\n--- PASS: TestFast (0.00s)\nnsgo-test-exit %s 0\n' "$SIMCTL_CHILD_NSGO_TEST_RUN" "$SIMCTL_CHILD_NSGO_TEST_RUN"
echo 'The process did launch, but has since exited or crashed' >&2
exit 3
`, output: "--- PASS: TestFast", wantCode: 0},
		{name: "failed app with successful launcher", body: `printf 'nsgo-test-start %s\n--- FAIL: TestFast (0.00s)\nnsgo-test-exit %s 7\n' "$SIMCTL_CHILD_NSGO_TEST_RUN" "$SIMCTL_CHILD_NSGO_TEST_RUN"
`, output: "--- FAIL: TestFast", wantCode: 7},
		{name: "crashed app with successful launcher", body: `printf 'nsgo-test-start %s\n=== RUN   TestCrash\n' "$SIMCTL_CHILD_NSGO_TEST_RUN"
echo 'panic: simulator crash fixture' >&2
`, output: "panic: simulator crash fixture", wantCode: -1, wantError: "without its exit code"},
		{name: "crashed app with failing launcher", body: `printf 'nsgo-test-start %s\n=== RUN   TestCrash\n' "$SIMCTL_CHILD_NSGO_TEST_RUN"
echo 'panic: simulator crash fixture' >&2
exit 2
`, output: "panic: simulator crash fixture", wantCode: -1, wantError: "without its exit code"},
		{name: "launch failure", body: "echo 'Simulator device failed to launch' >&2\nexit 4\n", output: "failed to launch", wantCode: -1, wantError: "launch simulator app"},
		{name: "host never started", body: "echo 'ok without a host'\n", output: "ok without a host", wantCode: -1, wantError: "nsgotest.Main"},
		{name: "completed app with lingering launcher", body: `printf 'nsgo-test-start %s\n--- PASS: TestFast (0.00s)\nnsgo-test-exit %s 0\n' "$SIMCTL_CHILD_NSGO_TEST_RUN" "$SIMCTL_CHILD_NSGO_TEST_RUN"
exec sleep 30
`, output: "--- PASS: TestFast", wantCode: 0},
		{name: "runtime error with lingering launcher", body: `printf 'nsgo-test-error %s runtime fixture failure\n' "$SIMCTL_CHILD_NSGO_TEST_RUN"
exec sleep 30
`, wantCode: -1, wantError: "runtime fixture failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkXcrun(t, "case \"$2\" in terminate) exit 0;; esac\n"+tc.body)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			var output strings.Builder
			code, err := (simulatorTest{udid: "simulator", id: "dev.nsgo.test", timeout: 2 * time.Second}).run(ctx, []string{"-test.run=^TestFast$"}, nil, &output)
			if code != tc.wantCode || tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("code=%d err=%v, want code=%d error=%q; output:\n%s", code, err, tc.wantCode, tc.wantError, output.String())
			}
			if !strings.Contains(output.String(), tc.output) {
				t.Fatalf("output lost %q: %s", tc.output, output.String())
			}
			// A lingering launcher must end with the marked result, not hold the run until the test's deadline.
			if ctx.Err() != nil {
				t.Fatal("the run waited for the launcher until its deadline")
			}
		})
	}
}

func TestSimulatorTestSpawnsAPlainProcess(t *testing.T) {
	checkXcrun(t, `[ "$1 $2 $3 $4" = "simctl spawn simulator /app/Plain" ] || exit 9
[ "$SIMCTL_CHILD_KEY" = value ] || exit 8
echo "--- FAIL: TestFast ($5)"
exit 3
`)
	var output strings.Builder
	code, err := (simulatorTest{udid: "simulator", id: "dev.nsgo.test", binary: "/app/Plain"}).run(t.Context(), []string{"-test.run=^TestFast$"}, []string{"KEY=value"}, &output)
	if code != 3 || err != nil || !strings.Contains(output.String(), "--- FAIL: TestFast (-test.run=^TestFast$)") {
		t.Fatalf("code=%d err=%v output=%q", code, err, output.String())
	}
}

// The launch and the test have deadlines of their own (#647): the launch's ends at the host's start mark, and -timeout counts from there, so a slow launch does not fail a fast test and a long test is not cut off by the launch's limit.
func TestSimulatorTestDeadlines(t *testing.T) {
	const marks = `printf 'nsgo-test-start %s\n' "$SIMCTL_CHILD_NSGO_TEST_RUN"
`
	const pass = `printf -- '--- PASS: TestFast (0.00s)\nnsgo-test-exit %s 0\n' "$SIMCTL_CHILD_NSGO_TEST_RUN"
`
	for _, tc := range []struct {
		name, body              string
		timeout, launchTimeout  time.Duration
		wantCode                int
		wantError, wantNotError string
		output                  string
	}{
		{name: "slow launch, fast test", body: "sleep 2\n" + marks + pass,
			timeout: time.Second, launchTimeout: 10 * time.Second, wantCode: 0, output: "--- PASS: TestFast"},
		{name: "test outlasts the launch limit", body: marks + "sleep 2\n" + pass,
			timeout: 10 * time.Second, launchTimeout: time.Second, wantCode: 0, output: "--- PASS: TestFast"},
		{name: "test outlasts the launch limit without a deadline", body: marks + "sleep 2\n" + pass,
			launchTimeout: time.Second, wantCode: 0, output: "--- PASS: TestFast"},
		{name: "test runs past its deadline", body: "sleep 2\n" + marks + "echo '=== RUN   TestSlow'\nexec sleep 30\n",
			timeout: time.Second, launchTimeout: 10 * time.Second, wantCode: -1, wantError: "the test took over 1s", output: "=== RUN   TestSlow"},
		{name: "launch never starts the host", body: "echo 'waiting for SpringBoard' >&2\nexec sleep 30\n",
			timeout: 10 * time.Second, launchTimeout: time.Second, wantCode: -1, wantError: "launch simulator app: the test host did not start within 1s", wantNotError: "took over", output: "waiting for SpringBoard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkXcrun(t, "case \"$2\" in terminate) exit 0;; esac\n"+tc.body)
			// Only a guard against a hung run: the runner owns the deadlines, as runTargetTests leaves them to it.
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			var output strings.Builder
			start := time.Now()
			code, err := (simulatorTest{udid: "simulator", id: "dev.nsgo.test", timeout: tc.timeout, launchTimeout: tc.launchTimeout}).run(ctx, nil, nil, &output)
			if code != tc.wantCode || tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) || tc.wantNotError != "" && err != nil && strings.Contains(err.Error(), tc.wantNotError) {
				t.Fatalf("code=%d err=%v, want code=%d error=%q; output:\n%s", code, err, tc.wantCode, tc.wantError, output.String())
			}
			if !strings.Contains(output.String(), tc.output) {
				t.Fatalf("output lost %q: %s", tc.output, output.String())
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Fatalf("the run took %v", elapsed)
			}
		})
	}
}

func TestSimulatorStopIsBounded(t *testing.T) {
	checkXcrun(t, "exec sleep 30\n")
	start := time.Now()
	(simulatorTest{udid: "simulator", id: "dev.nsgo.test"}).stop(t.Context())
	if elapsed := time.Since(start); elapsed > 7*time.Second {
		t.Fatalf("simulator cleanup took %v after its deadline", elapsed)
	}
}

// A timed-out test stops the app, which later tests launch again; only the run's end uninstalls the test build (#898).
func TestSimulatorCloseUninstallsTheTestBuild(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	checkXcrun(t, "echo \"$*\" >> '"+log+"'\n")
	s := simulatorTest{udid: "simulator", id: "dev.example.app.nsgotest"}
	s.stop(t.Context())
	s.close(t.Context())
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "simctl terminate simulator dev.example.app.nsgotest\nsimctl terminate simulator dev.example.app.nsgotest\nsimctl uninstall simulator dev.example.app.nsgotest\n"
	if string(data) != want {
		t.Fatalf("xcrun calls:\n%s\nwant:\n%s", data, want)
	}
}
