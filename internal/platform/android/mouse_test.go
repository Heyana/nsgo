package android

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMouseJava compiles and runs Mouse.java and Protocol.java on the host JVM
// with minimal Android MotionEvent/KeyEvent/InputDevice stubs, verifying hover
// enter/move/exit, ancestor chain transitions, modifiers and held-button
// bitmask mapping, button-down mouse drags, modifier changes under a resting
// pointer, PointerEvents/modal/disabled/deleted view handling, and TalkBack
// touchscreen hover isolation (#1553).
func TestMouseJava(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	stubs, err := filepath.Glob(filepath.Join("testdata", "mouse", "android", "view", "*.java"))
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-d", classes,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "Mouse.java"),
		filepath.Join("testdata", "mouse", "dev", "oneo", "nsgo", "MouseCheck.java"),
	}, stubs...)
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.MouseCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
