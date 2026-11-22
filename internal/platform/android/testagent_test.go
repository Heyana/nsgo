package android

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAndroidTestAgentPressAndSignature compiles and executes TestAgent.java
// on the JVM with stubbed Android UI classes, verifying that press replies
// wait for runnables posted during ACTION_UP (such as View.PerformClick) and
// that signature() changes when native focus moves.
func TestAndroidTestAgentPressAndSignature(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{
		"-d", classes,
		"java/dev/oneo/nsgo/Protocol.java",
		"java/dev/oneo/nsgo/TestAgent.java",
	}
	if err := filepath.WalkDir("testdata/testagent", func(path string, entry fs.DirEntry, err error) error {
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
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.TestAgentCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
