package formula

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/caelyreth/untie/mathml"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func repMap(item core.ClipboardItem) map[string]string {
	out := make(map[string]string, len(item.Representations))
	for _, rep := range item.Representations {
		out[rep.Type] = string(rep.Data)
	}
	return out
}

func TestMathML(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		dialect Dialect
		display bool
		want    []string
	}{
		{
			name:    "inline TeX",
			source:  `E=mc^2`,
			dialect: TeX,
			display: false,
			want:    []string{`xmlns="http://www.w3.org/1998/Math/MathML"`, `display="inline"`, `<msup`, `<mi`, `<mn`},
		},
		{
			name:    "display fraction",
			source:  `\frac{1}{2}`,
			dialect: TeX,
			display: true,
			want:    []string{`display="block"`, `<mfrac`, `>1<`, `>2<`},
		},
		{
			name:    "radical and matrix",
			source:  `\sqrt[3]{x}+\begin{pmatrix}a&b\\c&d\end{pmatrix}`,
			dialect: TeX,
			display: true,
			want:    []string{`<mroot`, `<mtable`, `<mtr`, `<mtd`},
		},
		{
			name:    "typst fraction",
			source:  `frac(a, b)`,
			dialect: Typst,
			display: false,
			want:    []string{`display="inline"`, `<mfrac`},
		},
		{
			name:    "syntax error recovers as merror",
			source:  `\notacommand`,
			dialect: TeX,
			display: false,
			want:    []string{`<math `, `<merror`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MathML(tc.source, tc.dialect, tc.display)
			if err != nil {
				t.Fatalf("MathML(%q): %v", tc.source, err)
			}
			for _, sub := range tc.want {
				if !strings.Contains(got, sub) {
					t.Errorf("MathML(%q) = %s; missing %q", tc.source, got, sub)
				}
			}
			// Verify well-formed XML.
			var v any
			if err := xml.Unmarshal([]byte(got), &v); err != nil {
				t.Errorf("MathML(%q) is not well-formed XML: %v\n%s", tc.source, err, got)
			}
		})
	}

	// Verify round-trip through Untie's strict MathML importer for standard formulas.
	got, err := MathML(`x^2+\frac{1}{2}`, TeX, true)
	if err != nil {
		t.Fatal(err)
	}
	res, err := mathml.UnmarshalString(context.Background(), got, mathml.Options{})
	if err != nil || res.Formula == nil {
		t.Fatalf("mathml.UnmarshalString failed: %v (%+v)", err, res.Diagnostics)
	}
}

func TestClipboardItem(t *testing.T) {
	texItem := ClipboardItem(`E=mc^2`, TeX, false)
	reps := repMap(texItem)
	if reps["text/plain"] != `E=mc^2` {
		t.Errorf("text/plain = %q, want %q", reps["text/plain"], `E=mc^2`)
	}
	if reps[MIMETeX] != `E=mc^2` {
		t.Errorf("%s = %q, want %q", MIMETeX, reps[MIMETeX], `E=mc^2`)
	}
	if !strings.HasPrefix(reps[MIMEMathML], `<math xmlns="http://www.w3.org/1998/Math/MathML"`) {
		t.Errorf("%s = %q, want Presentation MathML", MIMEMathML, reps[MIMEMathML])
	}

	typstItem := ClipboardItem(`x^2`, Typst, true)
	typstReps := repMap(typstItem)
	if typstReps["text/plain"] != `x^2` || typstReps[MIMETypst] != `x^2` {
		t.Errorf("typst representations = %#v", typstReps)
	}
	if !strings.Contains(typstReps[MIMEMathML], `display="block"`) {
		t.Errorf("typst MathML = %q, want display=block", typstReps[MIMEMathML])
	}
}

func TestCopyFormulaAndFallback(t *testing.T) {
	var cx *core.Scope
	app := nsgotest.Start(t, func(scope *core.Scope) core.Node {
		cx = scope
		return core.View{}
	})

	app.Post(func() { Copy(cx, `\frac{1}{2}`, TeX, true) })
	app.Settle()

	if app.Clipboard != `\frac{1}{2}` {
		t.Errorf("Clipboard = %q, want %q", app.Clipboard, `\frac{1}{2}`)
	}
	if len(app.ClipboardItems) != 1 {
		t.Fatalf("ClipboardItems = %d, want 1", len(app.ClipboardItems))
	}
	reps := repMap(app.ClipboardItems[0])
	if reps["text/plain"] != `\frac{1}{2}` {
		t.Errorf("text/plain = %q", reps["text/plain"])
	}
	if reps[MIMETeX] != `\frac{1}{2}` {
		t.Errorf("%s = %q", MIMETeX, reps[MIMETeX])
	}
	if !strings.Contains(reps[MIMEMathML], `<mfrac`) || !strings.Contains(reps[MIMEMathML], `display="block"`) {
		t.Errorf("%s = %q", MIMEMathML, reps[MIMEMathML])
	}

	// Fallback when the platform does not support clipboard representations.
	app.SetEnvironment(core.Environment{ClipboardItems: false})
	app.Post(func() { Copy(cx, `a+b`, TeX, false) })
	app.Settle()
	if app.Clipboard != `a+b` {
		t.Errorf("fallback Clipboard = %q, want %q", app.Clipboard, `a+b`)
	}
}

func TestTextViewFormulaSelectionCopiesMathMLAndTeX(t *testing.T) {
	r := Default()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return textview.TextView{
			TestID:     "doc",
			Selectable: true,
			RenderMath: RenderMath,
			Markdown:   "Energy $E=mc^2$.\n\n$$\nx^2\n$$",
		}
	}, nsgotest.Size(360, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	waitForFont(app, r, "label:E equals m c squared", "label:x squared")

	// 1. Selecting only the inline formula copies plain TeX plus MathML and text/x-tex.
	// In "Energy \uFFFC.", \uFFFC sits at UTF-16 offset 7..8.
	app.SelectTextRange("id:doc", "text:Energy ￼.", 7, "text:Energy ￼.", 8)
	app.Key("mod+c")
	if app.Clipboard != "$E=mc^2$" {
		t.Fatalf("inline formula Clipboard = %q, want %q", app.Clipboard, "$E=mc^2$")
	}
	if len(app.ClipboardItems) != 1 {
		t.Fatalf("inline formula ClipboardItems = %d, want 1", len(app.ClipboardItems))
	}
	inlineReps := repMap(app.ClipboardItems[0])
	if inlineReps["text/plain"] != "$E=mc^2$" {
		t.Errorf("inline text/plain = %q", inlineReps["text/plain"])
	}
	if inlineReps[MIMETeX] != "E=mc^2" {
		t.Errorf("inline %s = %q, want %q", MIMETeX, inlineReps[MIMETeX], "E=mc^2")
	}
	if !strings.Contains(inlineReps[MIMEMathML], `display="inline"`) || !strings.Contains(inlineReps[MIMEMathML], `<msup`) {
		t.Errorf("inline %s = %q", MIMEMathML, inlineReps[MIMEMathML])
	}

	// 2. Selecting surrounding text together with the formula copies plain text only (no MathML or text/x-tex).
	app.SelectTextRange("id:doc", "text:Energy ￼.", 0, "text:Energy ￼.", 9)
	app.Key("mod+c")
	if app.Clipboard != "Energy $E=mc^2$." {
		t.Fatalf("mixed selection Clipboard = %q, want %q", app.Clipboard, "Energy $E=mc^2$.")
	}
	for _, item := range app.ClipboardItems {
		reps := repMap(item)
		if reps[MIMEMathML] != "" || reps[MIMETeX] != "" {
			t.Errorf("mixed selection unexpectedly wrote formula representations: %#v", reps)
		}
	}

	// 3. Selecting the display formula block copies plain $$...$$ plus display MathML and text/x-tex.
	app.SelectTextRange("id:doc", "text:￼ >> nth=1", 0, "text:￼ >> nth=1", 1)
	app.Key("mod+c")
	if app.Clipboard != "$$\nx^2\n$$" {
		t.Fatalf("display formula Clipboard = %q, want %q", app.Clipboard, "$$\nx^2\n$$")
	}
	if len(app.ClipboardItems) != 1 {
		t.Fatalf("display formula ClipboardItems = %d, want 1", len(app.ClipboardItems))
	}
	dispReps := repMap(app.ClipboardItems[0])
	if dispReps["text/plain"] != "$$\nx^2\n$$" {
		t.Errorf("display text/plain = %q", dispReps["text/plain"])
	}
	if dispReps[MIMETeX] != "x^2" {
		t.Errorf("display %s = %q, want %q", MIMETeX, dispReps[MIMETeX], "x^2")
	}
	if !strings.Contains(dispReps[MIMEMathML], `display="block"`) || !strings.Contains(dispReps[MIMEMathML], `<msup`) {
		t.Errorf("display %s = %q", MIMEMathML, dispReps[MIMEMathML])
	}
}

func TestViewAttachmentUpdatesCopyItemWithSourceFunc(t *testing.T) {
	r := Default()
	var src *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		src = core.NewSignal(cx, `x^2`)
		return core.Text{
			TestID:     "line",
			Selectable: core.SelectableOn,
			Spans: []core.Span{{
				ContentFunc: func() string { return "$" + src.Get() + "$" },
				Attachment:  View{TestID: "formula", SourceFunc: src.Get, Renderer: r},
			}},
		}
	})
	waitForFont(app, r, "id:formula")
	line := app.Find("id:line")
	if len(line.Spans) != 1 || line.Spans[0].CopyItem == nil {
		t.Fatalf("initial span = %+v, want CopyItem", line.Spans)
	}
	if got := repMap(*line.Spans[0].CopyItem)[MIMETeX]; got != `x^2` {
		t.Fatalf("initial %s = %q, want x^2", MIMETeX, got)
	}

	app.Post(func() { src.Set(`\sqrt{y}`) })
	app.Settle()
	line = app.Find("id:line")
	if line.Spans[0].Copy != `$\sqrt{y}$` || line.Spans[0].CopyItem == nil {
		t.Fatalf("updated span = %+v", line.Spans[0])
	}
	reps := repMap(*line.Spans[0].CopyItem)
	if reps["text/plain"] != `$\sqrt{y}$` || reps[MIMETeX] != `\sqrt{y}` || !strings.Contains(reps[MIMEMathML], `<msqrt`) {
		t.Errorf("updated reps = %#v", reps)
	}
}
