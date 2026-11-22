package android

import (
	"bytes"
	"encoding/binary"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestScreenItemEncodesTitleFills verifies that ItemScreen writes TitleFills as bit 7 of the header flags byte so Header.java reads it accurately.
func TestScreenItemEncodesTitleFills(t *testing.T) {
	for _, fills := range []bool{false, true} {
		var w writer
		c := driver.ScreenConfig{
			Title:             "Search",
			HeaderTransparent: true,
			CanGoBack:         true,
			BackTitle:         "Home",
			Left:              11,
			Right:             12,
			TitleView:         13,
			TitleFills:        fills,
			HeaderHeight:      64,
		}
		w.str(c.Title)
		w.flags(c.HeaderHidden, c.HeaderTransparent, c.LargeTitle, c.CanGoBack, c.BackHidden, c.NoPopOnSwipe, c.NoPopOnBack, c.TitleFills)
		w.str(c.BackTitle)
		w.u32(uint32(c.Background))
		w.u32(uint32(c.TitleColor))
		w.u32(uint32(c.TintColor))
		w.u32(uint32(c.Left))
		w.u32(uint32(c.Right))
		w.u32(uint32(c.TitleView))
		w.points(c.HeaderHeight)

		r := reader{w.buf}
		if title := r.str(); title != "Search" {
			t.Fatalf("title = %q, want Search", title)
		}
		flags := r.byte()
		gotFills := flags&(1<<7) != 0
		if gotFills != fills {
			t.Fatalf("flags = %08b: bit 7 = %v, want %v", flags, gotFills, fills)
		}
		if flags&(1<<1) == 0 || flags&(1<<3) == 0 {
			t.Fatalf("flags = %08b: lost transparent/canGoBack bits", flags)
		}
		if back := r.str(); back != "Home" {
			t.Fatalf("backTitle = %q, want Home", back)
		}
		_, _, _ = r.u32(), r.u32(), r.u32()
		if left, right, tv := r.u32(), r.u32(), r.u32(); left != 11 || right != 12 || tv != 13 {
			t.Fatalf("header IDs = %d,%d,%d, want 11,12,13", left, right, tv)
		}
		if h := r.points(); h != 64 || len(r.buf) != 0 {
			t.Fatalf("height = %v (%d bytes left)", h, len(r.buf))
		}
	}
}

// TestHeaderJavaTitleFills compiles and runs Header.java on the JVM against AOSP Toolbar measurement semantics: left and right ends are measured before a filling titleView with MATCH_PARENT width, returning the exact remaining room in points, while an intrinsic titleView keeps its content width and toggling TitleFills restores its saved core width.
func TestHeaderJavaTitleFills(t *testing.T) {
	encodeScreen := func(canGoBack bool, left, right, titleView int32, titleFills bool) []byte {
		var buf bytes.Buffer
		writeStr := func(s string) {
			_ = binary.Write(&buf, binary.BigEndian, int32(len(s)))
			buf.WriteString(s)
		}
		writeStr("Title")
		var flags byte
		if canGoBack {
			flags |= 1 << 3
		}
		if titleFills {
			flags |= 1 << 7
		}
		buf.WriteByte(flags)
		writeStr("Back")
		_ = binary.Write(&buf, binary.BigEndian, int32(0)) // background
		_ = binary.Write(&buf, binary.BigEndian, int32(0)) // titleColor
		_ = binary.Write(&buf, binary.BigEndian, int32(0)) // tint
		_ = binary.Write(&buf, binary.BigEndian, left)
		_ = binary.Write(&buf, binary.BigEndian, right)
		_ = binary.Write(&buf, binary.BigEndian, titleView)
		_ = binary.Write(&buf, binary.BigEndian, float32(64))
		return buf.Bytes()
	}

	var input bytes.Buffer
	// 3 scenarios concatenated:
	// 1. Filling titleView with back arrow + left(40dp) + right(50dp)
	input.Write(encodeScreen(true, 1, 2, 3, true))
	// 2. Intrinsic titleView (toggled back to false) with same holders
	input.Write(encodeScreen(true, 1, 2, 3, false))
	// 3. Filling titleView without back arrow or ends
	input.Write(encodeScreen(false, 0, 0, 3, true))

	out := strings.TrimSpace(string(runJavaAlgorithm(t, "header", "HeaderCheck", input.Bytes(), "Header", "HeaderLayout", "Protocol")))
	want := strings.Join([]string{
		"fill-with-ends:order=1,2,3 lpWidth=-1 measured=196.0 titleWidth=196.0 afterSizeLpWidth=-1",
		"intrinsic-after-toggle:lpWidth=240 titleWidth=0.0 afterSizeLpWidth=200",
		"fill-alone:lpWidth=-1 measured=400.0 titleWidth=400.0",
	}, "\n")
	if out != want {
		t.Fatalf("HeaderCheck output:\n%s\nwant:\n%s", out, want)
	}
}
