package formula

import (
	"math"
	"reflect"
	"testing"

	"github.com/caelyreth/untie/display"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The final canvas height, including user font scaling and pixel rounding,
// determines the grid. Changing density must redraw even at the same size.
func TestViewRulesFollowFontAndDensity(t *testing.T) {
	// A native target keeps the display's own pixel ratio (testhost.Pin), so only the headless model can set the densities this walks through; TestFormulaRuleNativePixels checks the rules' native pixels.
	if nsgotest.Target() != "" {
		t.Skip("sets the pixel ratio, which a native target keeps as the display's")
	}
	r := Default()
	if _, err := r.engine(); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{`\frac{123}{456}`, `\sqrt{x+1}`} {
		t.Run(source, func(t *testing.T) {
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return View{Source: source, Renderer: r, TestID: "formula", Style: core.Style{FontSize: 13}}
			}, nsgotest.Environment(core.Environment{PixelRatio: 1, FontScale: 1}))
			l := r.parsed(layoutKey{source: source}, true)
			for _, fontScale := range []float32{1, 1.4, 2} {
				for _, ratio := range []float32{1, 1.5, 2, 3} {
					app.SetEnvironment(core.Environment{PixelRatio: ratio, FontScale: fontScale})
					v := app.Find("id:formula")
					scale := float32(v.Frame.Size.Height) / (l.em.Ascent + l.em.Descent)
					rules := 0
					for _, op := range v.Ops {
						if op.Kind != driver.DrawRect {
							continue
						}
						rules++
						// Untie deliberately leaves rules thicker than three pixels alone.
						var original paintRun
						for _, run := range l.runs {
							if run.path == nil {
								original = run
							}
						}
						if original.rect.Height*scale*ratio > 3 {
							if op.Y != original.rect.Y || op.H != original.rect.Height {
								t.Error("a thick rule was snapped")
							}
							continue
						}
						for _, x := range []float32{op.Y * scale * ratio, op.H * scale * ratio} {
							if math.Abs(float64(x)-math.Round(float64(x))) > 0.0001 {
								t.Errorf("font scale %g, density %g: rule edge/thickness %g physical pixels", fontScale, ratio, x)
							}
						}
					}
					if rules != 1 {
						t.Fatalf("got %d rules, want one", rules)
					}
				}
			}
		})
	}
}

func TestRuleGridLeavesGlyphsAndRuleLengthAlone(t *testing.T) {
	l := Default().parsed(layoutKey{source: `\color{red}{\frac{x}{y}}+\sqrt{z}`}, true)
	var glyphs, rules int
	for _, run := range l.runs {
		a, b := driver.NewDrawing(nil), driver.NewDrawing(nil)
		run.draw(a, display.PixelGrid{})
		run.draw(b, display.PixelGrid{Scale: 13, PixelRatio: 1.5})
		if run.path != nil {
			glyphs++
			if !reflect.DeepEqual(a.Ops, b.Ops) {
				t.Error("snapping changed a glyph path")
			}
			continue
		}
		rules++
		if run.snap != display.SnapY {
			t.Errorf("rule lost horizontal snapping metadata: %v", run.snap)
		}
		for _, op := range b.Ops {
			if op.Kind == driver.DrawRect && (op.X != run.rect.X || op.W != run.rect.Width) {
				t.Error("snapping changed the rule's major axis")
			}
		}
	}
	if glyphs == 0 || rules != 2 {
		t.Fatalf("got %d glyph runs and %d rules", glyphs, rules)
	}
}

func TestViewRuleDensityChangeAtFixedSize(t *testing.T) {
	// A native target keeps the display's own pixel ratio (testhost.Pin), so only the headless model can set the densities this walks through; TestFormulaRuleNativePixels checks the rules' native pixels.
	if nsgotest.Target() != "" {
		t.Skip("sets the pixel ratio, which a native target keeps as the display's")
	}
	r := Default()
	if _, err := r.engine(); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View{Source: `\frac{x}{y}`, Renderer: r, TestID: "formula",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(24)}}
	}, nsgotest.Environment(core.Environment{PixelRatio: 1}))
	before := app.Find("id:formula")
	size := before.Frame.Size
	ops := append([]driver.DrawOp(nil), before.Ops...)
	app.SetEnvironment(core.Environment{PixelRatio: 2})
	after := app.Find("id:formula")
	if after.Frame.Size != size {
		t.Fatal("density changed the fixed canvas size")
	}
	if len(ops) != len(after.Ops) {
		t.Fatal("density changed the drawing's structure")
	}
	changed := false
	for i, op := range ops {
		if op.Kind == driver.DrawRect {
			changed = changed || op.Y != after.Ops[i].Y || op.H != after.Ops[i].H
		} else if !reflect.DeepEqual(op, after.Ops[i]) {
			t.Error("density changed glyph geometry or paint")
		}
	}
	if !changed {
		t.Error("rules did not redraw for the new density at the same canvas size")
	}
}
