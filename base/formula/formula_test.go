package formula

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caelyreth/untie"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base/formula/locales"
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// The SHA-256 of fonts/NewCMMath-Book.otf, as typst/typst-assets@94dcb990 has it.
const fontDigest = "2ea09ebc9167b1e1a66f31390dc917f2d4004ecfca72d51b28010e4ad6becd95"

func TestBundledFont(t *testing.T) {
	sum := sha256.Sum256(newCMMath)
	if got := hex.EncodeToString(sum[:]); got != fontDigest {
		t.Fatalf("the embedded font's digest is %s: it must be the unmodified file", got)
	}
	if _, err := Default().engine(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRenderer(Fonts{Math: []byte("no font")}); err == nil {
		t.Error("NewRenderer took a file that is no font")
	}
}

func BenchmarkDefaultLoad(b *testing.B) {
	for b.Loop() {
		if _, err := NewRenderer(Fonts{Math: newCMMath}); err != nil {
			b.Fatal(err)
		}
	}
}

// waitForFont waits for r's font to load and for the formulas selectors find to draw with it: a View learns of the font from a resource, after the renderer has it.
func waitForFont(app *nsgotest.App, r *Renderer, selectors ...string) {
	app.WaitUntil(10*time.Second, func() bool {
		if !r.loaded.Load() {
			return false
		}
		for _, sel := range selectors {
			if vs := app.Query(sel); len(vs) != 1 || len(vs[0].Ops) == 0 {
				return false
			}
		}
		return true
	})
}

// A formula laid out once at the reference size scales to any other: its size in ems is the same at 16 points.
func TestLayoutScalesLinearly(t *testing.T) {
	engine, err := Default().engine()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{`E=mc^2`, `\frac{1}{2}`, `x_i^2`, `\begin{pmatrix}a&b\\c&d\end{pmatrix}`, `\sqrt{x^2+1}`, `\sum_{i=1}^n i`, `\left(\frac{a}{b}\right)`} {
		for _, mode := range []untie.Mode{untie.Inline, untie.Display} {
			bounds := func(size float32) untie.Rect {
				box, err := engine.LayoutContext(context.Background(), untie.ParseTeX(src, mode, untie.ParseOptions{}), untie.Style{Size: size})
				if err != nil {
					t.Fatal(err)
				}
				b := box.Bounds()
				return untie.Rect{X: b.X / size, Y: b.Y / size, Width: b.Width / size, Height: b.Height / size}
			}
			at16, at100 := bounds(16), bounds(referenceSize)
			for _, d := range []float32{at16.X - at100.X, at16.Y - at100.Y, at16.Width - at100.Width, at16.Height - at100.Height} {
				if math.Abs(float64(d)) > 0.01 {
					t.Errorf("%s (mode %d): %+v ems at 16 points, %+v at 100", src, mode, at16, at100)
					break
				}
			}
		}
	}
}

func TestRendererKeepsItsLastLayouts(t *testing.T) {
	r := newRenderer(Fonts{Math: newCMMath})
	key := layoutKey{source: `x^2`}
	if a, b := r.parsed(key, true), r.parsed(key, true); a != b || !a.drawn || len(a.runs) == 0 {
		t.Fatal("the same formula was laid out twice")
	}
	if parsed := r.parsed(layoutKey{source: `y`}, false); parsed.drawn || parsed.description == nil {
		t.Error("a formula parsed without its font has no description or has shapes")
	}
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 100 {
				r.parsed(layoutKey{source: fmt.Sprintf("x_{%d}", g*100+i)}, true)
			}
		})
	}
	wg.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) != cacheSize || r.order.Len() != cacheSize {
		t.Errorf("%d layouts kept (%d in order), want %d", len(r.entries), r.order.Len(), cacheSize)
	}
}

// A View is sized by its font, sits on its text's baseline, waits for its font in an em-high space, and is read in the app's language.
func TestViewSizesByFontAndReads(t *testing.T) {
	i18n.SetApp(i18n.NewCatalog("en", "zh-Hans"))
	defer i18n.SetApp(nil)
	r := newRenderer(Fonts{Math: newCMMath})
	var size *core.Signal[core.Points]
	var source *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		size = core.NewSignal[core.Points](cx, 16)
		source = core.NewSignal(cx, `\frac{1}{2}`)
		return core.View{StyleFunc: func() core.Style { return core.Style{FontSize: size.Get(), AlignItems: core.AlignStart} }, Children: []core.Node{
			View{TestID: "half", SourceFunc: source.Get, Renderer: r},
			View{TestID: "empty", Renderer: r},
			core.Text{TestID: "line", Spans: []core.Span{{Content: "a "}, {Content: "$x_i^2$", Attachment: View{TestID: "inline", Source: `x_i^2`, Renderer: r}},
				{Attachment: core.View{TestID: "box", Style: core.Style{Width: core.Pt(4), Height: core.Pt(4)}}}}},
		}}
	}, nsgotest.Size(400, 300), nsgotest.Environment(core.Environment{Locale: "en-US"}))
	if !r.loaded.Load() {
		if got := app.Find("id:half").Frame.Size; got != (core.Size{Width: 16, Height: 16}) {
			t.Errorf("before its font, a formula is %v, want an em square", got)
		}
	}
	waitForFont(app, r, "id:half", "id:inline")
	half := app.Find("id:half")
	small := half.Frame.Size
	if small.Height <= 16 || small.Width <= 0 {
		t.Fatalf("a fraction at 16 points is %v", small)
	}
	if got := half.Props.Label; got != "1 over 2" {
		t.Errorf("label %q", got)
	}
	if got := app.Find("id:empty").Props.Label; got != "Formula" {
		t.Errorf("an empty formula's label %q", got)
	}
	line := app.Find("id:line")
	if len(line.Spans) != 3 || line.Spans[1].Copy != "$x_i^2$" {
		t.Errorf("the formula's span %+v", line.Spans)
	}
	// The box stands on the baseline; the formula's subscript hangs below it.
	bottom := func(id string) core.Points { f := app.Find("id:" + id).Parent.Frame; return f.Origin.Y + f.Size.Height }
	if below := bottom("inline") - bottom("box"); below < 2 || below > 6 {
		t.Errorf("the inline formula reaches %v below the baseline, want its subscript's depth", below)
	}

	app.Post(func() { size.Set(32) })
	app.Settle()
	app.Screenshot("formula-baseline")
	if got := app.Find("id:half").Frame.Size; math.Abs(float64(got.Width-2*small.Width)) > 1 || math.Abs(float64(got.Height-2*small.Height)) > 1 {
		t.Errorf("at 32 points %v, at 16 %v: it doubles", got, small)
	}
	app.Post(func() { source.Set(`x_i^2`) })
	app.Settle()
	if got := app.Find("id:half").Props.Label; got != "x sub i squared" {
		t.Errorf("label %q after its source changed", got)
	}
	app.SetEnvironment(core.Environment{Locale: "zh-CN"})
	if got := app.Find("id:half").Props.Label; got != "x下标i的平方" {
		t.Errorf("label %q in Chinese", got)
	}
	if got := app.Find("id:empty").Props.Label; got != "公式" {
		t.Errorf("an empty formula's label %q in Chinese", got)
	}
}

// A View in a container that stretches its children keeps the size its font gives it.
func TestViewKeepsItsSizeInAStretchingColumn(t *testing.T) {
	r := newRenderer(Fonts{Math: newCMMath})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FontSize: 16}, Children: []core.Node{
			View{TestID: "stretched", Source: `\frac{1}{2}`, Display: true, Renderer: r},
			core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
				View{TestID: "start", Source: `\frac{1}{2}`, Display: true, Renderer: r},
			}},
		}}
	}, nsgotest.Size(400, 300))
	waitForFont(app, r, "id:stretched", "id:start")
	// Frames round to pixels where they fall, so the two may differ by one.
	got, want := app.Find("id:stretched").Frame.Size, app.Find("id:start").Frame.Size
	if math.Abs(float64(got.Width-want.Width)) > 1 || math.Abs(float64(got.Height-want.Height)) > 1 || want.Width >= 400 {
		t.Errorf("in a stretching column a formula is %v, at the start %v", got, want)
	}
}

// A formula paints in the text color, which it follows without drawing again, keeps its own colors, and marks errors in ErrorStyle's color.
func TestViewPaints(t *testing.T) {
	r := Default()
	var color *core.Signal[core.Color]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		color = core.NewSignal(cx, core.RGB(0xff0000))
		return core.View{StyleFunc: func() core.Style { return core.Style{Color: color.Get(), AlignItems: core.AlignStart} }, Children: []core.Node{
			View{TestID: "ink", Source: `x`},
			View{TestID: "blue", Source: `\color{blue}{x}`},
			View{TestID: "error", Source: `\notacommand`, ErrorStyle: core.Style{Color: core.RGB(0x00ff00)}},
		}}
	})
	waitForFont(app, r, "id:ink", "id:blue", "id:error")
	fills := func(id string) []core.Color {
		var out []core.Color
		for _, op := range app.Find("id:" + id).Ops {
			if op.Kind == driver.DrawFillColor {
				out = append(out, op.Color)
			}
		}
		return out
	}
	if got := fills("ink"); len(got) != 1 || got[0] != core.RGB(0xff0000) {
		t.Errorf("ink fills %v, want the text color", got)
	}
	if len(app.Find("id:ink").Drawing.ViewPaints) == 0 {
		t.Error("ink is no view paint: a new text color would draw the formula again")
	}
	app.Post(func() { color.Set(core.RGB(0x0000ff)) })
	app.Settle()
	if got := fills("ink"); len(got) != 1 || got[0] != core.RGB(0x0000ff) {
		t.Errorf("ink fills %v after the text color changed", got)
	}
	if got := fills("blue"); len(got) != 1 || got[0] != core.RGB(0x0000ff) {
		t.Errorf("\\color{blue} fills %v", got)
	}
	if got := fills("error"); len(got) == 0 || got[0] != core.RGB(0x00ff00) {
		t.Errorf("an error fills %v, want ErrorStyle's color", got)
	}
}

// A formula whose renderer's fonts do not load shows its source in ErrorClass and ErrorStyle.
func TestViewFallsBackToItsSource(t *testing.T) {
	r := newRenderer(Fonts{Math: []byte("no font")})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View{TestID: "f", Source: `x^2`, Renderer: r, ErrorClass: "italic", ErrorStyle: core.Style{Color: core.RGB(0xff0000)}}
	})
	app.WaitUntil(10*time.Second, r.loaded.Load)
	v := app.WaitFor("text:x^2", 10*time.Second)
	if v.TextStyle.Color != core.RGB(0xff0000) {
		t.Errorf("the source shows in %v, want ErrorStyle's color", v.TextStyle.Color)
	}
	if !v.TextStyle.Font.Italic {
		t.Error("the source shows upright, want ErrorClass's italic")
	}
}

// RenderMath typesets a text view's inline formula and its display one, which TextView centers in a sideways scroll, both copying as TeX.
func TestRenderMath(t *testing.T) {
	r := Default()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{TestID: "doc", RenderMath: RenderMath,
			Markdown: "Energy $E=mc^2$.\n\n$$\nx^2\n$$\n\n$$\n" + strings.Repeat(`a+`, 60) + "b\n$$"}
	}, nsgotest.Size(300, 400))
	waitForFont(app, r, "label:E equals m c squared", "label:x squared")
	app.Screenshot("formula-textview")
	paragraph := app.Find("text:Energy ￼.")
	if paragraph.Spans[1].Copy != "$E=mc^2$" {
		t.Errorf("the inline formula copies as %q", paragraph.Spans[1].Copy)
	}
	narrow := app.Find("label:x squared")
	block := narrow.Parent.Parent // the attachment's view in its text
	text := block
	for text.Kind != nsgotest.KindText {
		text = text.Parent
	}
	if text.Spans[0].Copy != "$$\nx^2\n$$" {
		t.Errorf("the display formula copies as %q", text.Spans[0].Copy)
	}
	// Headless text lays lines out at the start: the text is centered as wide as its scroll.
	if text.TextStyle.Align != core.TextAlignCenter || text.Frame.Size.Width != text.Parent.Frame.Size.Width {
		t.Errorf("a display formula's text aligns %v, %v wide in a scroll %v wide: it centers across the scroll", text.TextStyle.Align, text.Frame.Size.Width, text.Parent.Frame.Size.Width)
	}
	var scrolls []*nsgotest.View
	var walk func(v *nsgotest.View)
	walk = func(v *nsgotest.View) {
		if v.Kind == nsgotest.KindScroll && v.Scroll.Horizontal {
			scrolls = append(scrolls, v)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(app.Find("id:doc"))
	if len(scrolls) != 2 {
		t.Fatalf("%d sideways scrolls, want one a display formula", len(scrolls))
	}
	if wide := scrolls[1]; wide.Content.Width <= wide.Frame.Size.Width {
		t.Errorf("a wide formula's scroll holds %v in %v: it scrolls", wide.Content, wide.Frame.Size)
	}
}

func TestSpeech(t *testing.T) {
	en, zh := locales.In("en"), locales.In("zh-Hans")
	for _, c := range []struct{ src, en, zh string }{
		{`E=mc^2`, "E equals m c squared", "E 等于 m c的平方"},
		{`\sqrt[3]{x}`, "cube root of x", "x的立方根"},
		{`\sum_{i=1}^{n} i`, "sum from i equals 1 to n i", "求和，从i 等于 1到n i"},
		{`\begin{pmatrix}a&b\\c&d\end{pmatrix}`, "left parenthesis matrix with 2 rows: row 1: a, b; row 2: c, d right parenthesis", "左括号 2行矩阵：第1行：a，b；第2行：c，d 右括号"},
		{`\hat{x}`, "x with hat", "x，带尖帽"},
		{`\mathbb{R}`, "double-struck R", "双线体R"},
		{`\notacommand`, `error: \notacommand`, `错误：\notacommand`},
		{`\sin x`, "sine x", "正弦 x"},
		{`\arcsin x`, "arcsine x", "反正弦 x"},
		{`\arccos x + \arctan y`, "arccosine x plus arctangent y", "反余弦 x 加 反正切 y"},
		{`\coth x`, "hyperbolic cotangent x", "双曲余切 x"},
		{`\deg p`, "degree p", "次数 p"},
		{`\hom(A, B)`, "homomorphisms left parenthesis A comma B right parenthesis", "同态 左括号 A 逗号 B 右括号"},
		{`\lg x`, "common logarithm x", "常用对数 x"},
		{`\liminf_{n} a_n`, "limit inferior from n a sub n", "下极限，从n a下标n"},
		{`\limsup a_n`, "limit superior a sub n", "上极限 a下标n"},
		{`\argmax_x f`, "arg max from x f", "最大值点，从x f"},
		{`\Pr(A)`, "probability left parenthesis A right parenthesis", "概率 左括号 A 右括号"},
		{`\tg x + \ctg x + \sh x + \ch x`, "tangent x plus cotangent x plus hyperbolic sine x plus hyperbolic cosine x", "正切 x 加 余切 x 加 双曲正弦 x 加 双曲余弦 x"},
		{`\arctg x + \cosec x`, "arctangent x plus cosecant x", "反正切 x 加 余割 x"},
		{`16.7\,\text{ms} = \tfrac{1000}{60}`, "16.7 ms equals 1000 over 60", "16.7 ms 等于 60分之1000"},
	} {
		d := untie.ParseTeX(c.src, untie.Inline, untie.ParseOptions{}).Alt().Description
		if got := speak(en, d); got != c.en {
			t.Errorf("%s: %q, want %q", c.src, got, c.en)
		}
		if got := speak(zh, d); got != c.zh {
			t.Errorf("%s in Chinese: %q, want %q", c.src, got, c.zh)
		}
	}
	if got := speak(en, nil); got != "Formula" {
		t.Errorf("no description: %q", got)
	}
}
