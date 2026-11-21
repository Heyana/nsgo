package android

import (
	"bytes"
	"io/fs"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestBoxRimMatchesDriver runs Android's Look and Box on the JVM against
// Go's driver.ViewStyle.Rim across styles and densities, and verifies
// Box.draw stops the fill inset under the border with the rimmed color.
func TestBoxRimMatchesDriver(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{
		"-d", classes,
		"java/dev/oneo/nsgo/Protocol.java",
		"java/dev/oneo/nsgo/Animator.java",
		"java/dev/oneo/nsgo/Shape.java",
		"java/dev/oneo/nsgo/Look.java",
		"java/dev/oneo/nsgo/Box.java",
	}
	if err := filepath.WalkDir("testdata/box", func(path string, entry fs.DirEntry, err error) error {
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

	base := driver.ViewStyle{
		CornerRadius: 12,
		BorderWidth:  4,
		BorderColor:  0x00000080,
		Background:   0xffffffff,
	}
	type rimCase struct {
		name    string
		density float32
		style   driver.ViewStyle
	}
	var cases []rimCase
	add := func(name string, mutate func(*driver.ViewStyle)) {
		vs := base
		if mutate != nil {
			mutate(&vs)
		}
		for _, density := range []float32{1, 2, 2.625, 3} {
			cases = append(cases, rimCase{name: name, density: density, style: vs})
		}
	}
	add("opaque fill translucent border", nil)
	add("translucent fill opaque border", func(vs *driver.ViewStyle) {
		vs.Background, vs.BorderColor = 0xffffffeb, 0x808080ff
	})
	add("both opaque", func(vs *driver.ViewStyle) {
		vs.Background, vs.BorderColor = 0x112233ff, 0x445566ff
	})
	add("per-corner radius", func(vs *driver.ViewStyle) {
		vs.CornerRadius = 0
		vs.Corners = driver.Corners{TopLeft: 8}
	})
	add("continuous corners", func(vs *driver.ViewStyle) {
		vs.ContinuousCorners = true
	})
	add("explicit solid border", func(vs *driver.ViewStyle) {
		vs.BorderStyle = driver.BorderSolid
	})
	add("rounding edges", func(vs *driver.ViewStyle) {
		vs.Background, vs.BorderColor = 0x123456ff, 0xfedcba7f
	})
	add("alpha 1 over white", func(vs *driver.ViewStyle) {
		vs.Background, vs.BorderColor = 0xffffffff, 0x00000001
	})
	add("alpha 254 over black", func(vs *driver.ViewStyle) {
		vs.Background, vs.BorderColor = 0x000000ff, 0xfffffffe
	})
	add("square", func(vs *driver.ViewStyle) { vs.CornerRadius = 0 })
	add("no border", func(vs *driver.ViewStyle) { vs.BorderWidth = 0 })
	add("clear border", func(vs *driver.ViewStyle) { vs.BorderColor = 0 })
	add("no fill", func(vs *driver.ViewStyle) { vs.Background = 0 })
	add("both translucent", func(vs *driver.ViewStyle) { vs.Background = 0xffffffeb })
	add("dashed", func(vs *driver.ViewStyle) { vs.BorderStyle = driver.BorderDashed })
	add("dotted", func(vs *driver.ViewStyle) { vs.BorderStyle = driver.BorderDotted })
	add("per-side borders", func(vs *driver.ViewStyle) { vs.Borders = driver.Edges{Top: 1} })
	add("gradient", func(vs *driver.ViewStyle) {
		vs.Effects = &driver.Effects{Gradients: []driver.GradientPaint{{}}}
	})

	var input writer
	input.u32(uint32(len(cases)))
	for _, tc := range cases {
		vs := &tc.style
		input.float(tc.density)
		input.u32(uint32(vs.Background))
		input.points(vs.CornerRadius)
		input.points(vs.BorderWidth)
		input.u32(uint32(vs.BorderColor))
		input.points(vs.Borders.Top)
		input.points(vs.Borders.Right)
		input.points(vs.Borders.Bottom)
		input.points(vs.Borders.Left)
		input.u32(uint32(vs.BorderColors.Top))
		input.u32(uint32(vs.BorderColors.Right))
		input.u32(uint32(vs.BorderColors.Bottom))
		input.u32(uint32(vs.BorderColors.Left))
		input.points(vs.Corners.TopLeft)
		input.points(vs.Corners.TopRight)
		input.points(vs.Corners.BottomRight)
		input.points(vs.Corners.BottomLeft)
		input.byte(byte(vs.BorderStyle))
		grads := 0
		if vs.Effects != nil {
			grads = min(len(vs.Effects.Gradients), 255)
		}
		input.byte(byte(grads))
		input.flags(vs.ContinuousCorners)
	}

	cmd := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.BoxCheck")
	cmd.Stdin = bytes.NewReader(input.buf)
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("java: %v\n%s", err, diagnostics.Bytes())
	}
	if len(out) != 9*len(cases) {
		t.Fatalf("Java wrote %d bytes, want %d", len(out), 9*len(cases))
	}
	r := reader{out}
	for _, tc := range cases {
		gotOK := r.byte() != 0
		gotColor := r.u32()
		gotInset := r.float()

		wantRim, wantInsetPts, wantOK := tc.style.Rim()
		wantARGB := uint32(0)
		wantInsetPx := float32(0)
		if wantOK {
			rgba := uint32(wantRim)
			wantARGB = (rgba >> 8) | (rgba << 24)
			wantInsetPx = float32(wantInsetPts) * tc.density
		}
		if gotOK != wantOK {
			t.Errorf("%s (density %g): hasRim = %v, want %v", tc.name, tc.density, gotOK, wantOK)
			continue
		}
		if gotColor != wantARGB {
			t.Errorf("%s (density %g): rimColor = %#08x, want %#08x", tc.name, tc.density, gotColor, wantARGB)
		}
		if math.Abs(float64(gotInset-wantInsetPx)) > 1e-4 {
			t.Errorf("%s (density %g): rimInset = %g, want %g", tc.name, tc.density, gotInset, wantInsetPx)
		}
	}
}
