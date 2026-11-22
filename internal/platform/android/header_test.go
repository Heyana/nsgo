package android

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestHeaderLayoutJava runs the real Material 3 top app bar layout and collapse
// math on the host JVM, verifying pinned toolbar placement, large-title band
// collapse, non-overlapping title crossfade, and color alpha modulation.
func TestHeaderLayoutJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "HeaderLayout.java"),
		filepath.Join("testdata", "HeaderCheck.java")).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.HeaderCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}

// TestScrollEdgeMathJava runs the Android scroll edge math on the host JVM,
// verifying progressive blur levels, band and capture rects, hairline geometry,
// mask gradients, and smoothstep tint stops match internal/driver/scroll.go.
func TestScrollEdgeMathJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "ScrollEdgeMath.java"),
		filepath.Join("testdata", "ScrollEdgeCheck.java")).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.ScrollEdgeCheck").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
}

// TestReadEnvironmentReadsAndroidHeaderHeights verifies that readEnvironment
// preserves Android's Material 3 regular (64 dp) and large (152 dp) bar heights.
func TestReadEnvironmentReadsAndroidHeaderHeights(t *testing.T) {
	var w writer
	for range 7 {
		w.float(1)
	}
	w.byte(0) // flags
	w.byte(0) // capabilities
	w.byte(byte(driver.IdiomPhone))
	w.str("en-US")
	w.str("Pixel")
	w.str("Google Pixel 9")
	w.float(64)  // Header.BAR
	w.float(152) // Header.LARGE_BAR
	w.u32(35)

	env := readEnvironment(&reader{w.buf})
	if env.HeaderHeight != 64 || env.LargeHeaderHeight != 152 {
		t.Fatalf("header heights = (%v, %v), want (64, 152)", env.HeaderHeight, env.LargeHeaderHeight)
	}
}
