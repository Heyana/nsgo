package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWriteInstallScriptsFindsReleaseRepositoryThroughGit(t *testing.T) {
	dir := t.TempDir()
	p := &project{Dir: dir, Name: "Fixture", Version: "1.0.0"}
	p.Build.Output = filepath.Join(dir, "dist")
	calls := 0
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		want := []string{"-C", dir, "remote", "get-url", "origin"}
		if cmd.name != "git" || !slices.Equal(cmd.args, want) {
			t.Fatalf("repository lookup ran %s %q, want git %q", cmd.name, cmd.args, want)
		}
		calls++
		return []byte("git@github.com:fixture/example.git\n"), nil
	})
	if err := p.writeInstallScripts(ctx, []string{"macos_arm64"}); err != nil {
		t.Fatal(err)
	}
	sh, err := os.ReadFile(filepath.Join(p.releaseDir(), "install.sh"))
	if err != nil || !strings.Contains(string(sh), "fixture/example") || calls != 1 {
		t.Fatalf("installer lookup returned %v after %d commands", err, calls)
	}
}
