package android

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSwitchMirrorsInRTLJava compiles and runs the real Android Switch.java on
// the host JVM, verifying that onRtlPropertiesChanged invalidates the control
// and onDraw mirrors the thumb position between LTR and RTL (#1615).
func TestSwitchMirrorsInRTLJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	stubs := filepath.Join("testdata", "direction")
	files := []string{
		filepath.Join(stubs, "android", "R.java"),
		filepath.Join(stubs, "android", "content", "Context.java"),
		filepath.Join(stubs, "android", "content", "res", "Configuration.java"),
		filepath.Join(stubs, "android", "content", "res", "Resources.java"),
		filepath.Join(stubs, "android", "graphics", "Canvas.java"),
		filepath.Join(stubs, "android", "graphics", "Paint.java"),
		filepath.Join(stubs, "android", "graphics", "RectF.java"),
		filepath.Join(stubs, "android", "util", "DisplayMetrics.java"),
		filepath.Join(stubs, "android", "util", "TypedValue.java"),
		filepath.Join(stubs, "android", "view", "Gravity.java"),
		filepath.Join(stubs, "android", "view", "View.java"),
		filepath.Join(stubs, "android", "view", "accessibility", "AccessibilityNodeInfo.java"),
		filepath.Join(stubs, "android", "widget", "CompoundButton.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "Switch.java"),
		filepath.Join(stubs, "dev", "oneo", "nsgo", "SwitchDirectionCheck.java"),
	}
	args := append([]string{"-d", dir}, files...)
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.SwitchDirectionCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
