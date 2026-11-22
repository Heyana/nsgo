package android

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A batch that restyles views ends with their backdrop filters, after the geometry version and the text copies, which it then writes even when empty; one that restyles none keeps the footer it had.
func TestBackdropFooter(t *testing.T) {
	var w writer
	writeBatchFooter(&w, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemDelete, ID: 2}}})
	if len(w.buf) != 0 {
		t.Fatalf("a batch without styles or texts gained a %d-byte footer", len(w.buf))
	}

	frosted := []driver.Filter{{Kind: driver.FilterBlur, Amount: 8}, {Kind: driver.FilterSaturate, Amount: 1.8}, {Kind: driver.FilterBlur, Amount: 12}, {Kind: driver.FilterDropShadow, Shadow: driver.BoxShadow{Blur: 30}}}
	w = writer{}
	writeBatchFooter(&w, &driver.MountBatch{GeometryVersion: 9, Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &driver.ViewStyle{Effects: &driver.Effects{BackdropFilters: frosted}}},
		{Kind: driver.ItemText, ID: 4, Text: &driver.TextContent{Text: "plain"}},
		{Kind: driver.ItemViewStyle, ID: 5, ViewStyle: &driver.ViewStyle{}},
		{Kind: driver.ItemViewStyle, ID: 6},
	}})
	r := reader{w.buf}
	if r.u64() != 9 || r.u32() != 1 || r.u32() != 4 || r.u32() != 0 {
		t.Fatal("the version and text copies moved")
	}
	if n := r.u32(); n != 3 {
		t.Fatalf("%d backdrop entries, want one per restyled view", n)
	}
	m, _, _, _ := driver.SplitFilters(frosted)
	if r.u32() != 3 || r.byte() != 1 {
		t.Fatal("the frosted view lost its color matrix")
	}
	for i := range 20 {
		if got := r.float(); got != m[i] {
			t.Fatalf("matrix[%d] = %v, want %v", i, got, m[i])
		}
	}
	if blur := r.points(); blur != 12 {
		t.Fatalf("blur %v, want the widest, 12", blur)
	}
	for _, id := range []uint32{5, 6} {
		if r.u32() != id || r.byte() != 0 || r.points() != 0 {
			t.Fatalf("view %d without backdrop filters does not clear them", id)
		}
	}
	if len(r.buf) != 0 {
		t.Fatalf("%d trailing bytes", len(r.buf))
	}

	// Styles alone still write an empty version and text copies, so the backdrop section sits where NsgoView reads it.
	w = writer{}
	writeBatchFooter(&w, &driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemViewStyle, ID: 7}}})
	r = reader{w.buf}
	if r.u64() != 0 || r.u32() != 0 || r.u32() != 1 || r.u32() != 7 || r.byte() != 0 || r.points() != 0 || len(r.buf) != 0 {
		t.Fatal("a styles-only footer is misplaced")
	}
}

// BackdropMath reads what writeBackdrop writes, scales Android's color offsets and pads the blur's source as Backdrop draws it.
func TestBackdropMathJava(t *testing.T) {
	javac, java := androidJDK(t)
	type spec struct {
		density float32
		filters []driver.Filter
	}
	specs := []spec{
		{2, nil},
		{2.5, []driver.Filter{{Kind: driver.FilterBlur, Amount: 12}}},
		{3, []driver.Filter{{Kind: driver.FilterSaturate, Amount: 1.5}, {Kind: driver.FilterContrast, Amount: 1.2}}},
		{2, []driver.Filter{
			{Kind: driver.FilterBlur, Amount: 10}, {Kind: driver.FilterBrightness, Amount: 0.9}, {Kind: driver.FilterInvert, Amount: 0.25},
			{Kind: driver.FilterHueRotate, Amount: 45}, {Kind: driver.FilterGrayscale, Amount: 0.3}, {Kind: driver.FilterSepia, Amount: 0.2},
			{Kind: driver.FilterOpacity, Amount: 0.85}, {Kind: driver.FilterDropShadow, Shadow: driver.BoxShadow{Blur: 4}},
		}},
	}
	pads := []struct {
		in   [9]float32
		want string
	}{
		{[9]float32{100, 200, 300, 260, 20, 0, 0, 1080, 2400}, "pad 40 140 360 320"}, // padded by three times the blur
		{[9]float32{0, 0, 1080, 80, 20, 0, 0, 1080, 2400}, "pad 0 0 1080 140"},       // a top bar clamped to the surface
		{[9]float32{10.5, 20.2, 100.1, 80.9, 0, 0, 0, 500, 500}, "pad 10 20 101 81"}, // no blur: the box's pixels
		{[9]float32{600, 600, 700, 700, 10, 0, 0, 500, 500}, "pad null"},             // outside the surface
	}

	var w writer
	w.u32(uint32(len(specs)))
	var want []string
	bits := func(f float32) string { return fmt.Sprintf("%x", math.Float32bits(f)) }
	for _, s := range specs {
		w.float(s.density)
		writeBackdrop(&w, s.filters)
		m, color, blur, _ := driver.SplitFilters(s.filters)
		line := fmt.Sprintf("spec %v %s %v", blur > 0 || color, bits(float32(blur)*s.density), color)
		if color {
			for k, v := range m {
				if k%5 == 4 {
					v *= 255
				}
				line += " " + bits(v)
			}
		}
		want = append(want, line)
	}
	w.u32(uint32(len(pads)))
	for _, p := range pads {
		w.floats(p.in[:]...)
		want = append(want, p.want)
	}

	dir := t.TempDir()
	input := filepath.Join(dir, "backdrop.bin")
	if err := os.WriteFile(input, w.buf, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), javac, "-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "BackdropMath.java"),
		filepath.Join("testdata", "BackdropCheck.java")).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.BackdropCheck", input).CombinedOutput()
	if err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
	if got := strings.Split(strings.TrimSpace(string(out)), "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Java read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
