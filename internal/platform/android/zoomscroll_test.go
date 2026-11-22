package android

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestZoomScrollBrakingTouch(t *testing.T) {
	runScrollCheck(t, "ZoomScrollCheck")
}

func TestScrollKeyboardTaps(t *testing.T) {
	runScrollCheck(t, "ScrollKeyboardCheck")
}

func runScrollCheck(t *testing.T, name string) {
	t.Helper()
	javac, java := androidJDK(t)
	classes := t.TempDir()
	stubs, err := filepath.Glob(filepath.Join("testdata", "zoomscroll", "android", "*", "*.java"))
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-d", classes,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "ScrollPhysics.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "Scrolling.java"),
		filepath.Join("testdata", "zoomscroll", "dev", "oneo", "nsgo", name+".java"),
	}, stubs...)
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo."+name).CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
