package android

import (
	"os/exec"
	"testing"
)

// TestInputIsolation runs the Android Java runtime's TestInputGate checks on
// the host JVM, verifying that test mode drops physical input outside scripted
// TestAgent steps while normal / live-automation mode accepts all input.
func TestInputIsolation(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	build := exec.Command(javac, "-d", classes,
		"java/dev/oneo/nsgo/TestInputGate.java",
		"testdata/TestInputGateCheck.java",
	)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	run := exec.Command(java, "-cp", classes, "dev.oneo.nsgo.TestInputGateCheck")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
