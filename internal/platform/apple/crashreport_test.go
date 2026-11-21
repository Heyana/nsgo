//go:build darwin && !ios

package apple

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

func TestCaptureUncaughtObjectiveCException(t *testing.T) {
	const subprocess = "NSGO_TEST_OBJC_CRASH_PATH"
	if path := os.Getenv(subprocess); path != "" {
		CaptureUncaughtExceptions(func(trace string) { os.WriteFile(path, []byte(trace), 0o600) })
		withPool(func() {
			exception := objc.ID(Class("NSException")).Send(sel("exceptionWithName:reason:userInfo:"), NSString("NSGoTestException"), NSString("a deliberate test exception"), objc.ID(0))
			// Raise from a native thread so there is no Go stack to unwind.
			exception.Send(sel("performSelectorInBackground:withObject:"), sel("raise"), objc.ID(0))
		})
		select {}
	}
	path := filepath.Join(t.TempDir(), "trace")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCaptureUncaughtObjectiveCException$")
	cmd.Env = append(os.Environ(), subprocess+"="+path)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil || err == nil {
		t.Fatalf("exception did not end the process: %v\n%s", err, output)
	}
	trace, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no exception trace: %v\n%s", err, output)
	}
	if !strings.Contains(string(trace), "NSGoTestException: a deliberate test exception\n") || !strings.Contains(string(trace), "objc_exception_throw") {
		t.Fatalf("exception trace = %q", trace)
	}
}
