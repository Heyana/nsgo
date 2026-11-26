package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTargetTestDiscoveryUsesProjectAndGoFlagTags(t *testing.T) {
	p := &project{Dir: t.TempDir()}
	p.Build.Tags = []string{"project", "shared"}
	calls := 0
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != "go" || cmd.dir != p.Dir || !slices.Contains(cmd.env, "GOOS=android") ||
			!slices.Contains(cmd.env, "GOARCH=arm64") || !slices.Contains(cmd.env, "CGO_ENABLED=0") {
			t.Fatalf("discovery ran %s %q in %s", cmd.name, cmd.args, cmd.dir)
		}
		calls++
		switch calls {
		case 1:
			if !slices.Equal(cmd.args, []string{"env", "GOFLAGS"}) {
				t.Fatalf("first command args = %q, want env GOFLAGS", cmd.args)
			}
			return []byte("-tags=env,shared -mod=readonly\n"), nil
		case 2:
			want := []string{"list", "-tags", "env,shared,project,nsgotest", "-json", "."}
			if !slices.Equal(cmd.args, want) {
				t.Fatalf("go list args = %q, want %q", cmd.args, want)
			}
			return []byte("fixture list result"), nil
		default:
			t.Fatalf("unexpected %s %q", cmd.name, cmd.args)
			return nil, nil
		}
	})
	out, err := goListFor(ctx, p, "android", "-json")
	if err != nil || string(out) != "fixture list result" || calls != 2 {
		t.Fatalf("goListFor returned %q, %v after %d commands", out, err, calls)
	}
}

func TestTargetTestsRejectExtraPackagesBeforeStartingGo(t *testing.T) {
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		t.Fatalf("invalid package args started %s %q", cmd.name, cmd.args)
		return nil, nil
	})
	p := &project{Dir: t.TempDir()}
	err := runTests(ctx, p, testOptions{target: "android", args: []string{"-count=2", "./second"}})
	if err == nil || !strings.Contains(err.Error(), `takes package patterns before its flags, not "./second" after them`) {
		t.Fatalf("multiple-package target test returned %v", err)
	}
}

func TestBareSimulatorTestCleanupUsesGeneratedBundleID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "library")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := loadForTest(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := simulatorTest{udid: "fixture-simulator", id: testProject(p).ID}
	var actions []string
	ctx := withProcessRunner(t, func(ctx context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name != "xcrun" || len(cmd.args) != 4 || cmd.args[0] != "simctl" ||
			cmd.args[2] != "fixture-simulator" || cmd.args[3] != "dev.nsgo.library.nsgotest" {
			t.Fatalf("cleanup ran %s %q", cmd.name, cmd.args)
		}
		if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
			t.Fatalf("cleanup context has no active deadline: %v", ctx.Err())
		}
		actions = append(actions, cmd.args[1])
		return nil, nil
	})
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	s.close(cancelled)
	if !slices.Equal(actions, []string{"terminate", "uninstall"}) {
		t.Fatalf("cleanup actions = %v", actions)
	}
}
