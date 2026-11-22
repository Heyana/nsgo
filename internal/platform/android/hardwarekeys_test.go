package android

import (
	"os/exec"
	"testing"
)

func TestHardwareKeyContinuations(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", dir,
		"java/dev/oneo/nsgo/HardwareKeys.java", "testdata/HardwareKeysCheck.java").CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.HardwareKeysCheck").CombinedOutput(); err != nil {
		t.Fatalf("key continuation fixture: %v\n%s", err, out)
	}
}
