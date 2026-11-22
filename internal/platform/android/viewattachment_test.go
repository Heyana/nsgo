package android

import (
	"os/exec"
	"testing"
)

func TestViewAttachmentRejectsStaleNativeWork(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", classes,
		"java/dev/oneo/nsgo/ViewAttachment.java", "testdata/ViewAttachmentCheck.java").CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.ViewAttachmentCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
