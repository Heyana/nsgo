package android

import (
	"bytes"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runJavaAlgorithm checks runtime source on the JVM with the fixture's
// stand-ins for the Android framework classes it needs.
func runJavaAlgorithm(t *testing.T, fixture, class string, input []byte, sources ...string) []byte {
	t.Helper()
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{"-encoding", "UTF-8", "-d", classes}
	for _, source := range sources {
		args = append(args, filepath.Join("java", "dev", "oneo", "nsgo", source+".java"))
	}
	if err := filepath.WalkDir(filepath.Join("testdata", fixture), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".java") {
			args = append(args, path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	cmd := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo."+class)
	cmd.Stdin = bytes.NewReader(input)
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("java: %v\n%s", err, &diagnostic)
	}
	return out
}
