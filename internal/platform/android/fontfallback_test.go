package android

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestFontFallbackOnJVM compiles and runs FontFallback.java on the JVM, verifying CSS fontFamily parsing (including parity with driver.FontFamilies), TrueType/OpenType/TTC name-table reading and CustomFallbackBuilder chain planning.
func TestFontFallbackOnJVM(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	args := []string{
		"-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "FontFallback.java"),
		filepath.Join("testdata", "FontFallbackCheck.java"),
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.FontFallbackCheck").CombinedOutput(); err != nil {
		t.Fatalf("FontFallbackCheck: %v\n%s", err, out)
	}

	// Cross-check Java's parser output directly against Go's driver.FontFamilies.
	sample := ` Inter , "Noto Sans SC",'Brand, Inc', ui-monospace, sans-serif, "serif",`
	out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.FontFallbackCheck", "parse", sample).Output()
	if err != nil {
		t.Fatalf("FontFallbackCheck parse: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	goFamilies := driver.FontFamilies(sample)
	if len(lines) != len(goFamilies) {
		t.Fatalf("Java parsed %d entries (%q), Go parsed %d (%+v)", len(lines), lines, len(goFamilies), goFamilies)
	}
	for i, f := range goFamilies {
		var want string
		switch f.Kind {
		case driver.FamilySystem, driver.FamilyRounded:
			want = "generic:sans-serif"
		case driver.FamilySerif:
			want = "generic:serif"
		case driver.FamilyMonospace:
			want = "generic:monospace"
		default:
			want = "named:" + f.Name
		}
		if lines[i] != want {
			t.Errorf("entry %d: Java = %q, want %q", i, lines[i], want)
		}
	}
}
