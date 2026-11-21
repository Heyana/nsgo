package android

import (
	"os"
	"os/exec"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// androidJDK finds the tools used by source-based JVM tests. CI supplies
// a JDK explicitly so a missing compiler fails the job.
func androidJDK(t *testing.T) (javac, java string) {
	t.Helper()
	testhost.RequireSource(t)
	find := func(name string) string {
		t.Helper()
		path, err := exec.LookPath(name)
		if err == nil {
			err = exec.CommandContext(t.Context(), path, "-version").Run()
		}
		if err != nil {
			if os.Getenv("NSGO_REQUIRE_JDK") == "1" {
				t.Fatalf("JDK required: %s: %v", name, err)
			}
			t.Skipf("JDK unavailable: %s: %v", name, err)
		}
		return path
	}
	return find("javac"), find("java")
}
