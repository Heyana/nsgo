package android

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestAccessibilityRolesAndStatesOnJVM compiles and runs Protocol.java and Accessibility.java on the JVM against Go's real ItemDirection spoken words in English and Simplified Chinese, verifying all 11 NSH 0.4.0 roles and the Pressed and Busy states.
func TestAccessibilityRolesAndStatesOnJVM(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	args := []string{
		"-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "Accessibility.java"),
		filepath.Join("testdata", "AccessibilityCheck.java"),
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}

	var payload writer
	writeDirection(&payload, &driver.MountItem{Kind: driver.ItemDirection, Language: "en"})
	writeDirection(&payload, &driver.MountItem{Kind: driver.ItemDirection, Language: "zh-Hans"})

	cmd := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.AccessibilityCheck")
	cmd.Stdin = bytes.NewReader(payload.buf)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("AccessibilityCheck: %v\n%s", err, out)
	}
}
