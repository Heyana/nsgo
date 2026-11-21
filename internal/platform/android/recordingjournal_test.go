package android

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRecordingJournal runs the Java runtime's RecordingJournal on the JVM, one process per step, as the Go processes that hear its records come and go: a repaired recording's record outlives the process it was reported to until the app takes it, a newer record of a file replaces the older, and a deleted file's record is dropped. It needs a JDK on PATH.
func TestRecordingJournal(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	build := exec.Command(javac, "-d", classes, "java/dev/oneo/nsgo/RecordingJournal.java", "testdata/RecordingJournalCheck.java")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	dir := t.TempDir()
	step := func(args ...string) []string {
		t.Helper()
		out, err := exec.Command(java, append([]string{"-cp", classes, "dev.oneo.nsgo.RecordingJournalCheck", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return strings.Fields(string(out))
	}
	pending := func(want ...string) {
		t.Helper()
		if got := step("pending"); !slices.Equal(got, want) {
			t.Errorf("pending = %q, want %q", got, want)
		}
	}
	journal := filepath.Join(dir, ".earlier")
	repaired, abandoned := filepath.Join(dir, "repaired.wav"), filepath.Join(dir, "abandoned.wav")
	for _, f := range []string{repaired, abandoned} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pending()
	step("take", repaired)
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Errorf("taking from no journal made one: %v", err)
	}
	// A start repairs a file and reports it; its process ends before the app takes the record.
	step("put", repaired, "partial")
	pending("partial")
	pending("partial")
	step("put", abandoned, "abandoned")
	step("put", repaired, "partial-again")
	pending("abandoned", "partial-again")
	// The app's Earlier took the repaired recording.
	step("take", repaired)
	pending("abandoned")
	if err := os.Remove(abandoned); err != nil {
		t.Fatal(err)
	}
	pending()
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Errorf("an empty journal is left: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".earlier*")); len(left) != 0 {
		t.Errorf("left %q", left)
	}
}
