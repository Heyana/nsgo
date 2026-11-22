package android

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The real recorder and sender run against a controlled PCM source and pipe, with no SDK or microphone. The device tests cover Android's service rules.
func TestRecordingServiceRuntime(t *testing.T) {
	classes, java := recordingRuntime(t)
	if out, err := exec.CommandContext(t.Context(), java, "-Xmx32m", "-cp", classes,
		"dev.oneo.nsgo.RecordingServiceCheck", t.TempDir()).CombinedOutput(); err != nil {
		t.Fatalf("recording runtime: %v\n%s", err, out)
	}
}

func TestRecordingServiceWriterFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("RLIMIT_FSIZE needs a POSIX shell")
	}
	classes, java := recordingRuntime(t)
	// Ignore the file-size signal so the real file write returns EFBIG. This exercises a partial write without adding a test hook to the recorder.
	if out, err := exec.CommandContext(t.Context(), "sh", "-c",
		"trap '' XFSZ; ulimit -f 32; exec \"$@\"", "recording-writer-failure",
		java, "-XX:-UsePerfData", "-Xmx32m", "-cp", classes,
		"dev.oneo.nsgo.RecordingServiceCheck", t.TempDir(), "write-failure").CombinedOutput(); err != nil {
		t.Fatalf("recording writer failure: %v\n%s", err, out)
	}
}

func recordingRuntime(t *testing.T) (string, string) {
	t.Helper()
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{"-d", classes, "java/dev/oneo/nsgo/RecordingService.java",
		"java/dev/oneo/nsgo/RecordingJournal.java", "java/dev/oneo/nsgo/Protocol.java"}
	if err := filepath.WalkDir("testdata/recording", func(path string, entry fs.DirEntry, err error) error {
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
	return classes, java
}
