package formula

import (
	"context"
	"math"
	"testing"

	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/display"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Missing text must reach the platform as a complete run: splitting Arabic or combining characters into individual glyphs would lose their shaping.
func TestTextUsesSystemFallback(t *testing.T) {
	r := Default()
	texts := []string{"中文 日本語 한국어", "العربية हिन्दी", "Latin 中文 é"}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		children := make([]core.Node, 0, len(texts))
		for i, text := range texts {
			children = append(children, View{Source: `x+\text{` + text + `}=y`, Renderer: r, TestID: string(rune('a' + i))})
		}
		return core.View{Style: core.Style{FontSize: 28, Gap: 16, Padding: core.All(20), Background: core.RGB(0xffffff), Color: core.RGB(0x111111)}, Children: children}
	}, nsgotest.Size(700, 280))
	waitForFont(app, r, "id:a", "id:b", "id:c")
	for i, text := range texts {
		var got []string
		for _, op := range app.Find("id:" + string(rune('a'+i))).Ops {
			if op.Kind == driver.DrawFillText {
				got = append(got, op.Text)
			}
		}
		if len(got) != 1 || got[0] != text {
			t.Errorf("system text runs %q, want the complete run %q", got, text)
		}
	}
	app.Screenshot("formula-system-text")
}

func TestTextFallbackUsesPlatformDimensions(t *testing.T) {
	const text = "中文 日本語 العربية हिन्दी"
	var want core.TextMetrics
	var size *core.Signal[core.Points]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		size = core.NewSignal[core.Points](cx, 28)
		want = cx.MeasureText(text, core.Font{Size: referenceSize, Language: cx.Env().Language()})
		return core.View{StyleFunc: func() core.Style {
			return core.Style{FontSize: size.Get(), Gap: 24, Padding: core.All(20), Background: core.RGB(0xffffff), Color: core.RGB(0x111111)}
		}, Children: []core.Node{
			View{Source: `\color{blue}{\text{` + text + `}}`, Display: true, TestID: "native-width"},
			View{Source: `\frac{\text{中文}}{\text{العربية}}+x_{\text{日本語}}`, Display: true, TestID: "structure"},
		}}
	}, nsgotest.Size(900, 480))
	waitForFont(app, Default(), "id:native-width", "id:structure")
	for _, points := range []core.Points{28, 42} {
		app.Post(func() { size.Set(points) })
		app.Settle()
		got := app.Find("id:native-width").Frame.Size
		width, height := want.Width*float32(points)/referenceSize, (want.Ascent+want.Descent)*float32(points)/referenceSize
		if math.Abs(float64(got.Width)-float64(width)) > 1 || math.Abs(float64(got.Height)-float64(height)) > 1 {
			t.Errorf("at %v points formula is %+v, platform text is %v × %v", points, got, width, height)
		}
	}
	app.Screenshot("formula-system-structure")
}

func TestSystemTextMetricsAreLocal(t *testing.T) {
	r := Default()
	cached := r.parsed(layoutKey{source: `\text{中文}`, display: true}, true)
	for _, width := range []float32{135, 270} {
		got := r.withSystemText(cached, func(text string, font core.Font) core.TextMetrics {
			if text != "中文" || font.Language != "ja" || font.Size != referenceSize {
				t.Errorf("measurement %q in %+v", text, font)
			}
			return core.TextMetrics{Width: width, Ascent: 85, Descent: 25}
		}, "ja")
		if got.failed || math.Abs(float64(got.em.Width-width/referenceSize)) > .0001 || math.Abs(float64(got.em.Ascent-.85)) > .0001 || math.Abs(float64(got.em.Descent-.25)) > .0001 {
			t.Errorf("native metrics lost: %+v for width %v", got.em, width)
		}
	}
	if r.parsed(cached.key, true) != cached {
		t.Error("platform-specific metrics replaced the renderer's shared layout")
	}
}

func TestSystemTextKeepsFormulaStructure(t *testing.T) {
	r := Default()
	measure := func(string, core.Font) core.TextMetrics { return core.TextMetrics{Width: 200, Ascent: 80, Descent: 20} }
	for _, tc := range []struct {
		source  string
		dialect Dialect
		count   int
		check   func(*testing.T, *systemRun, display.Paint)
	}{
		{source: `x_{\text{中文}}`, count: 1, check: func(t *testing.T, r *systemRun, _ display.Paint) {
			if r.scale >= .9 || r.scale < .3 {
				t.Errorf("subscript scale %v", r.scale)
			}
		}},
		{source: `\frac{\text{中文}}{\text{العربية}}`, count: 2, check: func(t *testing.T, r *systemRun, _ display.Paint) {
			if r.scale >= .9 || r.scale < .3 {
				t.Errorf("fraction scale %v", r.scale)
			}
		}},
		{source: `\phantom{\text{中文}}`, count: 0},
		{source: `\color{blue}{\text{中文}}`, count: 1, check: func(t *testing.T, _ *systemRun, p display.Paint) {
			if !p.HasExplicit || p.Explicit.B != 255 || p.Explicit.R != 0 {
				t.Errorf("native text paint %+v", p)
			}
		}},
		{source: `\textbf{中文}`, count: 1, check: func(t *testing.T, r *systemRun, _ display.Paint) {
			if r.font.Weight != core.FontWeightBold {
				t.Errorf("bold text font %+v", r.font)
			}
		}},
		{source: `\textit{中文}`, count: 1, check: func(t *testing.T, r *systemRun, _ display.Paint) {
			if !r.font.Italic {
				t.Errorf("italic text font %+v", r.font)
			}
		}},
		{source: `"中文" + x`, dialect: Typst, count: 1},
	} {
		t.Run(tc.source, func(t *testing.T) {
			l := r.withSystemText(r.parsed(layoutKey{source: tc.source, dialect: tc.dialect}, true), measure, "en")
			count := 0
			for _, run := range l.runs {
				if run.text == nil {
					continue
				}
				count++
				if tc.check != nil {
					tc.check(t, run.text, run.paint)
				}
				r := run.text
				if r.x < -.001 || r.y-r.metrics.Ascent*r.scale/referenceSize < -.001 || r.x+r.metrics.Width*r.scale/referenceSize > l.em.Width+.001 || r.y+r.metrics.Descent*r.scale/referenceSize > l.em.Ascent+l.em.Descent+.001 {
					t.Errorf("text escapes formula box: %+v in %+v", r, l.em)
				}
			}
			if l.failed || count != tc.count {
				t.Errorf("native runs %d, want %d; failed=%v", count, tc.count, l.failed)
			}
		})
	}
}

func TestExplicitTextFallbackWins(t *testing.T) {
	r, err := NewRenderer(Fonts{Math: newCMMath, Text: goregular.TTF, Fallbacks: [][]byte{newCMMath}})
	if err != nil {
		t.Fatal(err)
	}
	key := layoutKey{source: `\text{𝔸}`}
	engine, _ := r.engine()
	box, err := engine.LayoutContextWithOptions(context.Background(), untie.ParseTeX(key.source, untie.Inline, untie.ParseOptions{}), untie.Style{Size: referenceSize}, untie.LayoutOptions{Recovery: untie.RecoveryVisible})
	if err != nil || !box.FontUsage.UsesFallback() || box.FontUsage.NeedsFallback() {
		t.Fatalf("explicit fallback unused: %+v; %v", box, err)
	}
	l := r.parsed(key, true)
	if got := r.withSystemText(l, func(string, core.Font) core.TextMetrics {
		t.Fatal("system font measured despite explicit fallback coverage")
		return core.TextMetrics{}
	}, "en"); got != l {
		t.Error("covered text was laid out again")
	}
}

// A display formula wrapped to its width keeps its system text runs: the wrapped layout comes from the renderer's shared cache too, so the view adds them again.
func TestWrappedDisplayFormulaKeepsSystemText(t *testing.T) {
	r := Default()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(160), FontSize: 20}, Children: []core.Node{
			View{Source: `a=\text{中文}+b+c+d+e+f+g+h+i+j+k+l+m+n`, Display: true, Renderer: r, TestID: "wrapped"},
		}}
	}, nsgotest.Size(600, 400))
	waitForFont(app, r, "id:wrapped")
	v := app.Find("id:wrapped")
	if v.Frame.Size.Width > 161 {
		t.Fatalf("formula did not wrap: %v", v.Frame.Size)
	}
	var texts []string
	for _, op := range v.Ops {
		if op.Kind == driver.DrawFillText {
			texts = append(texts, op.Text)
		}
	}
	if len(texts) != 1 || texts[0] != "中文" {
		t.Fatalf("wrapped formula text runs %q, want the system run", texts)
	}
}
