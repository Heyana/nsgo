//go:build (darwin || linux || windows) && !ios && !android

package crashreport

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCaptureGoFatal(t *testing.T) {
	const subprocess = "NSGO_TEST_CRASH_REPORTS_DIR"
	if logs := os.Getenv(subprocess); logs != "" {
		if _, err := Capture(logs, "1.2.3", "42"); err != nil {
			t.Fatal(err)
		}
		runtime.GOMAXPROCS(2)
		values := make(map[int]int)
		go func() {
			for {
				values[0]++
			}
		}()
		for {
			values[0] = 1
		}
	}
	logs := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCaptureGoFatal$")
	cmd.Env = append(os.Environ(), subprocess+"="+logs)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "fatal error: concurrent map") {
		t.Fatalf("subprocess did not die of concurrent map access: %v\n%s", err, output)
	}
	for range 2 {
		reports := pending(t, logs, 1)
		report := reports[0]
		if report.Platform != runtime.GOOS || report.Version != "1.2.3" || report.Build != "42" || report.Kind != "go" || report.Time.IsZero() {
			t.Fatalf("unexpected crash metadata: %+v", report)
		}
		trace := readTrace(t, report)
		if !strings.Contains(trace, "TestCaptureGoFatal") || !strings.Contains(trace, "goroutine ") {
			t.Fatalf("trace lacks goroutine frames: %q", trace)
		}
	}
	report := pending(t, logs, 1)[0]
	for range 2 {
		if err := report.Acknowledge(); err != nil {
			t.Fatal(err)
		}
	}
	pending(t, logs, 0)
	if _, err := report.Open(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("opening an acknowledged report returned %v", err)
	}
}

func TestPendingSkipsLiveAndCleanRuns(t *testing.T) {
	logs := t.TempDir()
	stop, err := Capture(logs, "0.1.0", "1")
	if err != nil {
		t.Fatal(err)
	}
	stop = sync.OnceFunc(stop)
	t.Cleanup(stop)
	if err := Record(logs, Header{Platform: runtime.GOOS, Kind: "native"}, "exception\nframe\n"); err != nil {
		t.Fatal(err)
	}
	report := pending(t, logs, 1)[0]
	if got := readTrace(t, report); got != "exception\nframe\n" {
		t.Errorf("trace = %q", got)
	}
	if entries, err := os.ReadDir(filepath.Join(logs, directory)); err != nil || len(entries) != 2 {
		t.Fatalf("the live capture should still have its file: %v, %v", entries, err)
	}
	stop()
	if entries, err := os.ReadDir(filepath.Join(logs, directory)); err != nil || len(entries) != 1 {
		t.Fatalf("the clean capture should have removed its file: %v, %v", entries, err)
	}
}

func pending(t *testing.T, logs string, count int) []Report {
	t.Helper()
	reports, err := Pending(logs)
	if err != nil || len(reports) != count {
		t.Fatalf("Pending returned %d reports and %v, want %d reports", len(reports), err, count)
	}
	return reports
}

func readTrace(t *testing.T, report Report) string {
	t.Helper()
	reader, err := report.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	trace, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(trace)
}
