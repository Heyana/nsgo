package android

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppLocaleMatchesAndroid33Contract runs the shipped AppLocale.java on the
// JVM with stubbed LocaleManager, LocaleList, SharedPreferences and Build so
// API gating, per-app pinning, system-locale preservation and user-setting
// precedence are verified without an emulator.
func TestAppLocaleMatchesAndroid33Contract(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{"-d", classes, "java/dev/oneo/nsgo/AppLocale.java"}
	if err := filepath.WalkDir("testdata/applocale", func(path string, entry fs.DirEntry, err error) error {
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
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.AppLocaleCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}
