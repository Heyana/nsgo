package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func withProcessRunner(t *testing.T, runner processRunner) context.Context {
	t.Helper()
	return context.WithValue(t.Context(), processRunnerKey{}, runner)
}

func TestProcessHelpersUseRunner(t *testing.T) {
	dir := t.TempDir()
	failure := errors.New("process failed")
	var calls []processCommand
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		calls = append(calls, cmd)
		if cmd.output == captureCombined {
			return []byte("diagnostic\n"), failure
		}
		return []byte(" value\n"), nil
	})
	if err := run(ctx, "tool", "build", "a b"); !errors.Is(err, failure) || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("run returned %v", err)
	}
	if got, err := output(ctx, "tool", "inspect"); err != nil || got != "value" {
		t.Fatalf("output returned %q, %v", got, err)
	}
	if err := goTool(ctx, dir, []string{"NSGO_TEST_PROCESS=1"}, "vet", "./..."); err == nil || err.Error() != "go vet failed\ndiagnostic" {
		t.Fatalf("goTool returned %v", err)
	}
	if len(calls) != 3 || calls[0].name != "tool" || !slices.Equal(calls[0].args, []string{"build", "a b"}) ||
		!slices.Equal(calls[1].args, []string{"inspect"}) {
		t.Fatal("run and output did not send their arguments to the process runner")
	}
	c := calls[2]
	if c.name != "go" || !slices.Equal(c.args, []string{"vet", "./..."}) || c.dir != dir ||
		!slices.Contains(c.env, "NSGO_TEST_PROCESS=1") {
		t.Errorf("goTool ran %s %q in %s with the test environment entry present: %v",
			c.name, c.args, c.dir, slices.Contains(c.env, "NSGO_TEST_PROCESS=1"))
	}
}

func TestRunLocalUsesTargetCommandEnvironment(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fixture")
	var out strings.Builder
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != bin || cmd.dir != dir || !slices.Equal(cmd.args, []string{"-test.run=^TestOne$"}) ||
			cmd.output != streamOutput || cmd.stderr != cmd.stdout || !slices.Contains(cmd.env, "NSGO_TEST_RUN=abc") {
			t.Fatalf("local target ran %s %q in %s", cmd.name, cmd.args, cmd.dir)
		}
		_, err := io.WriteString(cmd.stdout, "result")
		return nil, err
	})
	code, err := runLocal(ctx, bin, []string{"-test.run=^TestOne$"}, []string{"NSGO_TEST_RUN=abc"}, &out)
	if code != 0 || err != nil || out.String() != "result" {
		t.Fatalf("local run returned %d, %v with output %q", code, err, out.String())
	}
}

func TestExecuteCaptureOwnsItsStreams(t *testing.T) {
	// Capture modes call Output/CombinedOutput, which refuse pre-assigned
	// streams; configured stdout/stderr must not turn into "Stdout already set".
	out, err := execute(context.Background(), processCommand{
		name: "go", args: []string{"env", "GOFLAGS"}, stdout: io.Discard, stderr: io.Discard, output: captureCombined,
	})
	if err != nil {
		t.Fatalf("captureCombined with configured streams: %v", err)
	}
	if len(out) == 0 && os.Getenv("GOFLAGS") == "" {
		// go env prints an empty line for an unset value; a missing result is
		// only suspicious when the environment sets it.
		t.Log("go env GOFLAGS is empty")
	}
}
