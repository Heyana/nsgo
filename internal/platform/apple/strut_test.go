//go:build darwin

package apple

import (
	"image"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestApplyLineHeight(t *testing.T) {
	cases := []struct {
		name         string
		rawA, rawD   float64
		minLH, maxLH float64
		wantA, wantD float64
		wantClamped  bool
	}{
		{
			name: "unconstrained",
			rawA: 20.421, rawD: 5.188,
			wantA: 20.421, wantD: 5.188,
			wantClamped: false,
		},
		{
			name: "natural matches target",
			rawA: 23.32, rawD: 7.48,
			minLH: 30, maxLH: 30,
			wantA: 23.32, wantD: 7.48,
			wantClamped: false,
		},
		{
			name: "expanded to min line height",
			rawA: 20.421, rawD: 5.188,
			minLH: 30, maxLH: 30,
			wantA: 22, wantD: 8,
			wantClamped: true,
		},
		{
			name: "compressed to max line height",
			rawA: 37.5, rawD: 30.05,
			minLH: 40, maxLH: 40,
			wantA: 24, wantD: 16,
			wantClamped: true,
		},
		{
			name: "fractional min line height",
			rawA: 13.923, rawD: 3.538,
			minLH: 20.3, maxLH: 20.3,
			wantA: 15, wantD: 5.3,
			wantClamped: true,
		},
		{
			name: "min only keeps taller natural height",
			rawA: 37.5, rawD: 30.05,
			minLH: 40.3, maxLH: 0,
			wantA: 37.5, wantD: 30.05,
			wantClamped: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, d, clamped := applyLineHeight(tc.rawA, tc.rawD, tc.minLH, tc.maxLH)
			if clamped != tc.wantClamped || math.Abs(a-tc.wantA) > 1e-6 || math.Abs(d-tc.wantD) > 1e-6 {
				t.Fatalf("applyLineHeight(%v, %v, %v, %v) = (%v, %v, %v), want (%v, %v, %v)",
					tc.rawA, tc.rawD, tc.minLH, tc.maxLH, a, d, clamped, tc.wantA, tc.wantD, tc.wantClamped)
			}
		})
	}
}

func TestStackLines(t *testing.T) {
	t.Run("unadjusted keeps CoreText origins", func(t *testing.T) {
		ctY := []float64{16, 36, 56}
		eff := []lineMetrics{
			{ascent: 12.32, descent: 3.68},
			{ascent: 12.32, descent: 3.68},
			{ascent: 12.32, descent: 3.68},
		}
		adj := []bool{false, false, false}
		y, changed, h := stackLines(ctY, eff, adj)
		if changed {
			t.Fatalf("expected changed=false")
		}
		for i := range ctY {
			if y[i] != ctY[i] {
				t.Fatalf("y[%d] = %v, want %v", i, y[i], ctY[i])
			}
		}
		if h != 60 {
			t.Fatalf("height = %v, want 60", h)
		}
	})

	t.Run("fallback-only lines stack with strut", func(t *testing.T) {
		ctY := []float64{21, 49, 77, 105}
		eff := []lineMetrics{
			{ascent: 37.5, descent: 30.05},
			{ascent: 37.5, descent: 30.05},
			{ascent: 37.5, descent: 30.05},
			{ascent: 37.5, descent: 30.05},
		}
		adj := []bool{true, true, true, true}
		y, changed, h := stackLines(ctY, eff, adj)
		if !changed {
			t.Fatalf("expected changed=true")
		}
		wantY := []float64{38, 106, 174, 242}
		for i := range wantY {
			if y[i] != wantY[i] {
				t.Fatalf("y[%d] = %v, want %v", i, y[i], wantY[i])
			}
		}
		if h != 273 {
			t.Fatalf("height = %v, want 273", h)
		}
	})

	t.Run("mixed lines preserve primary spacing and expand fallback line", func(t *testing.T) {
		ctY := []float64{20, 45, 81, 116}
		eff := []lineMetrics{
			{ascent: 20.421, descent: 5.188},
			{ascent: 20.421, descent: 5.188, leading: 11},
			{ascent: 20.421, descent: 5.188, leading: 11},
			{ascent: 20.421, descent: 5.188, leading: 11},
		}
		adj := []bool{false, false, false, true}
		y, changed, h := stackLines(ctY, eff, adj)
		if !changed {
			t.Fatalf("expected changed=true")
		}
		wantY := []float64{20, 45, 81, 117}
		for i := range wantY {
			if y[i] != wantY[i] {
				t.Fatalf("y[%d] = %v, want %v", i, y[i], wantY[i])
			}
		}
		if h != 123 {
			t.Fatalf("height = %v, want 123", h)
		}
	})

	t.Run("clamped line height aligns baseline without exceeding line height", func(t *testing.T) {
		ctY := []float64{27, 64}
		eff := []lineMetrics{
			{ascent: 24, descent: 16, clamped: true},
			{ascent: 24, descent: 16, clamped: true},
		}
		adj := []bool{true, false}
		y, changed, h := stackLines(ctY, eff, adj)
		if !changed {
			t.Fatalf("expected changed=true")
		}
		wantY := []float64{24, 64}
		for i := range wantY {
			if y[i] != wantY[i] {
				t.Fatalf("y[%d] = %v, want %v", i, y[i], wantY[i])
			}
		}
		if h != 80 {
			t.Fatalf("height = %v, want 80", h)
		}
	})
}

func requireInstalledFamily(t *testing.T, name string) {
	t.Helper()
	f := familyFont(driver.Family{Name: name}, 16, "")
	if f == 0 {
		t.Skipf("font family %q is not installed", name)
	}
	Release(f)
}

func primaryFontMetrics(f driver.Font) (ascent, descent, leading float64) {
	ct := Font(f)
	defer Release(ct)
	return ctFontGetAscent(ct), ctFontGetDescent(ct), ctFontGetLeading(ct)
}

func lineBaseline(line driver.TextLine) float64 {
	leading := line.Bounds.Size.Height - line.Ascender - line.Descender
	return float64(line.Bounds.Origin.Y + line.Ascender + leading/2)
}

// TestFontStrutMatchesMixedLineMetrics verifies that a line whose characters all come from a later family in a fontFamily list keeps at least the first family's ascent and descent, matching a mixed line across Measure, TextLines, placed baselines, CaretRect, RangeRects, HitTest and IndexAt.
func TestFontStrutMatchesMixedLineMetrics(t *testing.T) {
	cases := []struct {
		name     string
		first    string
		second   string
		size     driver.Points
		fallback string
		mixed    string
	}{
		{"Zapfino + PingFang SC", "Zapfino", "PingFang SC", 20, "漢字", "AB漢字"},
		{"Menlo + Hiragino Sans", "Menlo", "Hiragino Sans", 22, "漢字", "AB漢字"},
		{"Didot + Times New Roman", "Didot", "Times New Roman", 20, "Ж", "ABЖ"},
		{"Courier New + Hiragino Sans", "Courier New", "Hiragino Sans", 16, "漢", "a漢"},
		{"Zapfino + Apple Color Emoji", "Zapfino", "Apple Color Emoji", 20, "😀", "A😀"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireInstalledFamily(t, tc.first)
			requireInstalledFamily(t, tc.second)
			font := driver.Font{Family: tc.first + ", " + tc.second, Size: tc.size, Weight: driver.FontWeightNormal}
			strutA, strutD, _ := primaryFontMetrics(font)
			st := driver.TextStyle{Font: font, Color: driver.RGB(0)}

			fp := TextEngine{}.Prepare(&driver.AttributedText{Text: tc.fallback, Style: st})
			defer fp.Release()
			mp := TextEngine{}.Prepare(&driver.AttributedText{Text: tc.mixed, Style: st})
			defer mp.Release()

			fSize := fp.Measure(driver.Auto)
			mSize := mp.Measure(driver.Auto)
			if math.Abs(float64(fSize.Height-mSize.Height)) > 1e-3 {
				t.Errorf("fallback height %v != mixed height %v", fSize.Height, mSize.Height)
			}

			fl := fp.Layout(fSize).(*Layout)
			defer fl.Release()
			ml := mp.Layout(mSize).(*Layout)
			defer ml.Release()

			fLines, mLines := fl.TextLines(), ml.TextLines()
			if len(fLines) != 1 || len(mLines) != 1 {
				t.Fatalf("got %d and %d lines, want 1", len(fLines), len(mLines))
			}
			if float64(fLines[0].Ascender) < strutA-1e-3 {
				t.Errorf("fallback ascender %v < primary strut ascent %v", fLines[0].Ascender, strutA)
			}
			if float64(fLines[0].Descender) < strutD-1e-3 {
				t.Errorf("fallback descender %v < primary strut descent %v", fLines[0].Descender, strutD)
			}
			_, fBases, _ := fl.placed()
			if math.Abs(lineBaseline(fLines[0])-fBases[0].Y) > 1e-3 {
				t.Errorf("TextLines baseline %v != placed baseline %v", lineBaseline(fLines[0]), fBases[0].Y)
			}
			// Carets and selection rects agree with the strut-adjusted box.
			cr, ok := fl.CaretRect(driver.Caret{Offset: 0})
			if !ok {
				t.Fatal("CaretRect returned false")
			}
			if float64(cr.Size.Height) < strutA+strutD-1e-3 {
				t.Errorf("CaretRect height %v < strut ascent+descent %v", cr.Size.Height, strutA+strutD)
			}
			if math.Abs(float64(cr.Origin.Y)-(fBases[0].Y-float64(fLines[0].Ascender))) > 1e-2 {
				t.Errorf("CaretRect.Y %v != baseline - ascender (%v)", cr.Origin.Y, fBases[0].Y-float64(fLines[0].Ascender))
			}
			rects := fl.RangeRects(0, len(fl.toUTF8)-1, nil)
			if len(rects) != 1 || math.Abs(float64(rects[0].Size.Height-cr.Size.Height)) > 1e-2 {
				t.Errorf("RangeRects %v disagrees with CaretRect %v", rects, cr)
			}
			// IndexAt and HitTest agree at the strutted baseline and inside the strut's descent band.
			box := Rect(0, 0, float64(fSize.Width), float64(fSize.Height))
			midX := float64(fSize.Width) / 4
			if idx := fl.IndexAt(CGPoint{X: midX, Y: fBases[0].Y}, box); idx != 0 {
				t.Errorf("IndexAt baseline (%v, %v) = %d, want 0", midX, fBases[0].Y, idx)
			}
			lowY := fBases[0].Y + strutD - 0.5
			if idx := fl.IndexAt(CGPoint{X: midX, Y: lowY}, box); idx != 0 {
				t.Errorf("IndexAt strut descent (%v, %v) = %d, want 0", midX, lowY, idx)
			}
			if hit := fl.HitTest(driver.Point{X: driver.Points(midX), Y: driver.Points(fBases[0].Y)}); hit.Offset != 0 {
				t.Errorf("HitTest at baseline = %+v, want offset 0", hit)
			}
		})
	}
}

// TestFontStrutMultiLineWrappingDrawingAndHitTesting checks multi-line wrapped fallback-only text: every line stacks with the strut, drawn pixels land at the strut-adjusted baselines (not CoreText's unadjusted frame origins), and TextLines, CaretLayout and IndexAt agree on every line.
func TestFontStrutMultiLineWrappingDrawingAndHitTesting(t *testing.T) {
	requireInstalledFamily(t, "Zapfino")
	requireInstalledFamily(t, "PingFang SC")
	st := driver.TextStyle{
		Font:  driver.Font{Family: "Zapfino, PingFang SC", Size: 20, Weight: driver.FontWeightNormal},
		Color: driver.RGB(0),
	}
	// Wrap "漢字漢字漢字" onto 3 lines at 45pt (each 2-char line is ~40pt wide).
	const wrapW driver.Points = 45
	at := &driver.AttributedText{Text: "漢字漢字漢字", Style: st}
	p := TextEngine{}.Prepare(at)
	defer p.Release()
	size := p.Measure(wrapW)

	// Compare with 3 lines of primary-font text.
	refP := TextEngine{}.Prepare(&driver.AttributedText{Text: "AB\nAB\nAB", Style: st})
	defer refP.Release()
	refSize := refP.Measure(driver.Auto)
	if size.Height != refSize.Height {
		t.Fatalf("3 wrapped fallback lines height = %v, want 3 primary lines %v", size.Height, refSize.Height)
	}

	img, l := renderWith(t, p, driver.Size{Width: wrapW, Height: size.Height})
	lines := l.TextLines()
	_, baselines, _ := l.placed()
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	// Verify every line's baseline, CaretRect, HitTest and IndexAt agree.
	box := Rect(0, 0, float64(wrapW), float64(size.Height))
	for i, tl := range lines {
		if math.Abs(lineBaseline(tl)-baselines[i].Y) > 1e-3 {
			t.Errorf("line %d: TextLines baseline %v != placed baseline %v", i, lineBaseline(tl), baselines[i].Y)
		}
		if i > 0 {
			dy := baselines[i].Y - baselines[i-1].Y
			if math.Abs(dy-68) > 1 {
				t.Errorf("line %d step dy = %v, want ~68 (Zapfino strut)", i, dy)
			}
		}
		idx := l.IndexAt(CGPoint{X: 5, Y: baselines[i].Y}, box)
		if idx != tl.Start {
			t.Errorf("line %d: IndexAt(5, %v) = %d, want line start %d", i, baselines[i].Y, idx, tl.Start)
		}
		hit := l.HitTest(driver.Point{X: 1, Y: driver.Points(baselines[i].Y)})
		wantUTF16 := i * 2 // 2 BMP CJK chars per line
		if hit.Offset != wantUTF16 {
			t.Errorf("line %d: HitTest(1, %v) = %+v, want offset %d", i, baselines[i].Y, hit, wantUTF16)
		}
		cr, ok := l.CaretRect(driver.Caret{Offset: wantUTF16})
		if !ok || cr.Origin.Y > driver.Points(baselines[i].Y) || cr.Origin.Y+cr.Size.Height < driver.Points(baselines[i].Y) {
			t.Errorf("line %d: CaretRect %v does not span baseline %v", i, cr, baselines[i].Y)
		}
	}

	// Verify actual rendered pixels: without strut, line 0 drew at y=21 (glyphs y~3..24) and line 2 drew at y=77 (glyphs y~59..80). With Zapfino strut, line 0 draws at y=38 (glyphs y~19..42), line 1 at y=106 (glyphs y~87..110), line 2 at y=174 (glyphs y~155..178).
	if dark, _ := inked(img, image.Rect(0, 0, int(wrapW), 14)); dark != 0 {
		t.Errorf("found %d dark pixels in top strut padding y=[0,14): glyphs were drawn un-strutted", dark)
	}
	for i, base := range baselines {
		band := image.Rect(0, int(base.Y)-18, int(wrapW), int(base.Y)+4)
		if dark, _ := inked(img, band); dark < 20 {
			t.Errorf("line %d (baseline y=%v): only %d dark pixels in %v, expected rendered glyphs", i, base.Y, dark, band)
		}
	}
	// The gap between line 0's descent (~45) and line 1's glyphs (~84) must be blank.
	if dark, _ := inked(img, image.Rect(0, 50, int(wrapW), 80)); dark != 0 {
		t.Errorf("found %d dark pixels in strut gap y=[50,80): lines were not stacked with strut", dark)
	}
}

// TestFontStrutWithMaxLinesAndTruncation verifies that MaxLines truncation on fallback-only text keeps the strut height and aligns Measure, Layout, TextLines, CaretRect and IndexAt across all ellipsize modes.
func TestFontStrutWithMaxLinesAndTruncation(t *testing.T) {
	requireInstalledFamily(t, "Zapfino")
	requireInstalledFamily(t, "PingFang SC")
	st := driver.TextStyle{
		Font:  driver.Font{Family: "Zapfino, PingFang SC", Size: 20, Weight: driver.FontWeightNormal},
		Color: driver.RGB(0),
	}
	const wrapW driver.Points = 45
	uncutP := TextEngine{}.Prepare(&driver.AttributedText{Text: "漢字漢字", Style: st})
	defer uncutP.Release()
	wantH := uncutP.Measure(wrapW).Height

	for name, mode := range map[string]driver.EllipsizeMode{
		"tail":   driver.EllipsizeTail,
		"head":   driver.EllipsizeHead,
		"middle": driver.EllipsizeMiddle,
		"clip":   driver.EllipsizeClip,
	} {
		t.Run(name, func(t *testing.T) {
			at := &driver.AttributedText{Text: "漢字漢字漢字漢字", Style: st, MaxLines: 2, Ellipsize: mode}
			p := TextEngine{}.Prepare(at)
			defer p.Release()
			got := p.Measure(wrapW)
			if got.Height != wantH {
				t.Fatalf("truncated height = %v, want 2 uncut strutted lines %v", got.Height, wantH)
			}
			_, l := renderWith(t, p, driver.Size{Width: wrapW, Height: got.Height})
			lines := l.TextLines()
			if len(lines) != 2 {
				t.Fatalf("got %d lines, want 2", len(lines))
			}
			_, bases, _ := l.placed()
			for i := range lines {
				if math.Abs(lineBaseline(lines[i])-bases[i].Y) > 1e-3 {
					t.Errorf("line %d: TextLines baseline %v != placed baseline %v", i, lineBaseline(lines[i]), bases[i].Y)
				}
			}
			if idx := l.IndexAt(CGPoint{X: 5, Y: bases[1].Y}, Rect(0, 0, float64(wrapW), float64(got.Height))); idx < 0 {
				t.Errorf("IndexAt on truncated second line returned -1")
			}
		})
	}
}

// TestFontStrutWithLineHeight verifies explicit LineHeight interacts cleanly with struts: fixed LineHeight keeps baselines aligned between primary and fallback-only lines without exceeding LineHeight, while attachment-enabled minLineHeight still respects the strut.
func TestFontStrutWithLineHeight(t *testing.T) {
	requireInstalledFamily(t, "Zapfino")
	requireInstalledFamily(t, "PingFang SC")
	requireInstalledFamily(t, "Menlo")
	requireInstalledFamily(t, "Hiragino Sans")

	cases := []struct {
		name       string
		family     string
		size       driver.Points
		lineHeight driver.Points
		fallback   string
		mixed      string
	}{
		{"Zapfino compressed LH=40", "Zapfino, PingFang SC", 20, 40, "漢字", "AB漢字"},
		{"Zapfino expanded LH=80", "Zapfino, PingFang SC", 20, 80, "漢字", "AB漢字"},
		{"Menlo+Hiragino LH=30", "Menlo, Hiragino Sans", 22, 30, "漢字", "AB漢字"},
		{"Menlo+Hiragino fractional LH=20.3", "Menlo, Hiragino Sans", 15, 20.3, "漢字", "AB漢字"},
		{"Menlo+Hiragino tight LH=18", "Menlo, Hiragino Sans", 22, 18, "漢字", "AB漢字"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := driver.TextStyle{
				Font:       driver.Font{Family: tc.family, Size: tc.size, Weight: driver.FontWeightNormal},
				Color:      driver.RGB(0),
				LineHeight: tc.lineHeight,
			}
			fp := TextEngine{}.Prepare(&driver.AttributedText{Text: tc.fallback, Style: st})
			defer fp.Release()
			mp := TextEngine{}.Prepare(&driver.AttributedText{Text: tc.mixed, Style: st})
			defer mp.Release()

			fSize, mSize := fp.Measure(driver.Auto), mp.Measure(driver.Auto)
			if fSize.Height != mSize.Height {
				t.Fatalf("fallback height %v != mixed height %v", fSize.Height, mSize.Height)
			}
			fl := fp.Layout(fSize).(*Layout)
			defer fl.Release()
			ml := mp.Layout(mSize).(*Layout)
			defer ml.Release()

			_, fBases, _ := fl.placed()
			_, mBases, _ := ml.placed()
			if math.Abs(fBases[0].Y-mBases[0].Y) > 1e-3 {
				t.Errorf("fallback baseline %v != mixed baseline %v", fBases[0].Y, mBases[0].Y)
			}
			fLines, mLines := fl.TextLines(), ml.TextLines()
			if math.Abs(float64(fLines[0].Ascender-mLines[0].Ascender)) > 1e-3 ||
				math.Abs(float64(fLines[0].Descender-mLines[0].Descender)) > 1e-3 {
				t.Errorf("fallback metrics (%v, %v) != mixed (%v, %v)",
					fLines[0].Ascender, fLines[0].Descender, mLines[0].Ascender, mLines[0].Descender)
			}
		})
	}
}

// TestFontStrutKeepsTallerFallbackAndPlacesDecorationsAndAttachments verifies that a taller fallback font is never shrunk to the strut, and that span backgrounds, strikethroughs and inline attachments align with the strut-adjusted baseline.
func TestFontStrutKeepsTallerFallbackAndPlacesDecorationsAndAttachments(t *testing.T) {
	requireInstalledFamily(t, "Menlo")
	requireInstalledFamily(t, "PingFang SC")
	requireInstalledFamily(t, "Zapfino")

	t.Run("taller fallback keeps max metrics", func(t *testing.T) {
		font := driver.Font{Family: "Menlo, PingFang SC", Size: 22, Weight: driver.FontWeightNormal}
		pfA, pfD, _ := primaryFontMetrics(driver.Font{Family: "PingFang SC", Size: 22, Weight: driver.FontWeightNormal})
		p := TextEngine{}.Prepare(&driver.AttributedText{
			Text:  "漢字",
			Style: driver.TextStyle{Font: font, Color: driver.RGB(0)},
		})
		defer p.Release()
		sz := p.Measure(driver.Auto)
		l := p.Layout(sz).(*Layout)
		defer l.Release()
		tl := l.TextLines()[0]
		if float64(tl.Ascender) < pfA-1e-2 || float64(tl.Descender) < pfD-1e-2 {
			t.Fatalf("taller fallback metrics (%v, %v) shrunk below PingFang SC (%v, %v)",
				tl.Ascender, tl.Descender, pfA, pfD)
		}
	})

	t.Run("span background and strikethrough follow strut baseline", func(t *testing.T) {
		st := driver.TextStyle{
			Font:  driver.Font{Family: "Zapfino, PingFang SC", Size: 20, Weight: driver.FontWeightNormal},
			Color: driver.RGB(0),
		}
		red := driver.RGB(0xff0000)
		spanSt := st
		spanSt.Decoration = driver.TextDecorationLineThrough
		spanSt.DecorationColor = red
		at := &driver.AttributedText{
			Text:  "漢字",
			Style: st,
			Spans: []driver.TextSpan{{
				Start:      0,
				End:        len("漢字"),
				Style:      spanSt,
				Background: driver.TextBackground{Color: driver.RGB(0xffeeee)},
			}},
		}
		p := TextEngine{}.Prepare(at)
		defer p.Release()
		sz := p.Measure(driver.Auto)
		img, l := renderWith(t, p, sz)
		_, bases, _ := l.placed()
		if len(l.under) != 1 || len(l.over) != 1 {
			t.Fatalf("expected 1 background and 1 strike box, got %d and %d", len(l.under), len(l.over))
		}
		// Background spans Zapfino's ascent above and descent below baseline.
		if math.Abs(l.under[0].rect.Origin.Y-(bases[0].Y-37.5)) > 1 {
			t.Errorf("background top %v does not match strut baseline %v - 37.5", l.under[0].rect.Origin.Y, bases[0].Y)
		}
		// Strikethrough sits near baseline - xHeight/2 and draws red pixels there.
		strikeY := int(l.over[0].rect.Origin.Y)
		if _, redPx := inked(img, image.Rect(0, strikeY-2, int(sz.Width), strikeY+4)); redPx < 10 {
			t.Errorf("expected red strikethrough pixels near y=%d, got %d", strikeY, redPx)
		}
	})

	t.Run("inline attachment aligns to strutted fallback text baseline", func(t *testing.T) {
		requireInstalledFamily(t, "Hiragino Sans")
		root := core.NewRoot(core.RootOptions{})
		t.Cleanup(root.Dispose)
		tree := core.NewTree(root, TextEngine{})
		tree.Resize(core.Size{Width: 300, Height: 150}, core.Geometry{})

		var outer, inner core.Ref
		tree.Mount(func(*core.Scope) core.Node {
			return core.Text{Ref: &outer, Style: core.Style{FontFamily: "Menlo, Hiragino Sans", FontSize: 22}, Spans: []core.Span{
				{Content: "AB "},
				{Attachment: core.View{Children: []core.Node{
					core.Text{Ref: &inner, Content: "漢字"},
				}}},
			}}
		})
		var outside, inside *Layout
		for _, it := range tree.Commit().Items {
			if it.Kind != driver.ItemText {
				continue
			}
			l := it.Text.Layout.(*Layout)
			t.Cleanup(l.Release)
			if it.Text.Text == "漢字" {
				inside = l
			} else {
				outside = l
			}
		}
		if outside == nil || inside == nil {
			t.Fatal("both texts should be mounted")
		}
		_, outerBases, _ := outside.placed()
		_, innerBases, _ := inside.placed()
		outerBox, _ := outer.Bounds()
		innerBox, _ := inner.Bounds()
		want := float64(outerBox.Origin.Y) + outerBases[0].Y
		got := float64(innerBox.Origin.Y) + innerBases[0].Y
		if math.Abs(got-want) > 0.05 {
			t.Fatalf("strutted fallback attachment baseline = %v, surrounding line baseline = %v", got, want)
		}
	})

	t.Run("smaller span on one-line ASCII text keeps primary strut", func(t *testing.T) {
		defer func() { oneLineFast = true }()
		st := driver.TextStyle{
			Font:  driver.Font{Family: "Zapfino", Size: 20, Weight: driver.FontWeightNormal},
			Color: driver.RGB(0),
		}
		small := driver.TextStyle{
			Font:  driver.Font{Family: "Menlo", Size: 10, Weight: driver.FontWeightNormal},
			Color: driver.RGB(0),
		}
		at := &driver.AttributedText{
			Text:  "abc",
			Style: st,
			Spans: []driver.TextSpan{{Start: 0, End: 3, Style: small}},
		}
		compareOneLine(t, "Zapfino+Menlo10", at)
		p := TextEngine{}.Prepare(at)
		defer p.Release()
		sz := p.Measure(driver.Auto)
		if sz.Height < 68 {
			t.Fatalf("smaller span shrank line height to %v, want Zapfino strut >= 68", sz.Height)
		}
	})

	t.Run("link spans hit-test at strut-adjusted baselines", func(t *testing.T) {
		st := driver.TextStyle{
			Font:  driver.Font{Family: "Zapfino, PingFang SC", Size: 20, Weight: driver.FontWeightNormal},
			Color: driver.RGB(0),
		}
		text := "漢字漢字"
		linkStart := len("漢字")
		spans := []driver.TextSpan{
			{Start: 0, End: linkStart, Style: st},
			{Start: linkStart, End: len(text), Style: st, Link: true},
		}
		p := TextEngine{}.Prepare(&driver.AttributedText{Text: text, Style: st, Spans: spans})
		defer p.Release()
		const wrapW driver.Points = 45
		sz := p.Measure(wrapW)
		l := p.Layout(driver.Size{Width: wrapW, Height: sz.Height}).(*Layout)
		defer l.Release()
		_, bases, _ := l.placed()
		if len(bases) != 2 {
			t.Fatalf("got %d lines, want 2", len(bases))
		}
		box := Rect(0, 0, float64(wrapW), float64(sz.Height))
		// Clicking line 0 hits non-link span; clicking line 1 at its strutted baseline hits the link span.
		if idx := l.IndexAt(CGPoint{X: 10, Y: bases[0].Y}, box); driver.PressesAt(spans, idx) {
			t.Errorf("line 0 hit idx=%d (link), want non-link", idx)
		}
		if idx := l.IndexAt(CGPoint{X: 10, Y: bases[1].Y}, box); !driver.PressesAt(spans, idx) || idx < linkStart {
			t.Errorf("line 1 at strut baseline y=%v hit idx=%d, want link span [%d, %d)", bases[1].Y, idx, linkStart, len(text))
		}
	})

	t.Run("line height with attachments respects strut and tall attachments", func(t *testing.T) {
		st := driver.TextStyle{
			Font:       driver.Font{Family: "Zapfino, PingFang SC", Size: 20, Weight: driver.FontWeightNormal},
			Color:      driver.RGB(0),
			LineHeight: 40,
		}
		// Small attachment on a fallback-only line: minLineHeight=40, maxLineHeight=0 -> keeps Zapfino's 69pt strut.
		smallAttText := "漢" + string(driver.AttachmentRune)
		sp := TextEngine{}.Prepare(&driver.AttributedText{
			Text:  smallAttText,
			Style: st,
			Attachments: []driver.TextAttachment{{
				Offset: len("漢"),
				Size:   driver.Size{Width: 12, Height: 12},
			}},
		})
		defer sp.Release()
		if sz := sp.Measure(driver.Auto); sz.Height < 68 {
			t.Fatalf("fallback line with small attachment and LineHeight=40 measured %v, want Zapfino strut >= 68", sz.Height)
		}
		// Tall attachment (80pt above baseline) grows past Zapfino's 37.5pt ascent while keeping its 30.05pt descent.
		tp := TextEngine{}.Prepare(&driver.AttributedText{
			Text:  smallAttText,
			Style: st,
			Attachments: []driver.TextAttachment{{
				Offset: len("漢"),
				Size:   driver.Size{Width: 20, Height: 80},
			}},
		})
		defer tp.Release()
		tSz := tp.Measure(driver.Auto)
		if tSz.Height < 110 {
			t.Fatalf("fallback line with 80pt attachment measured %v, want >= 110 (80 ascent + 30.05 Zapfino descent)", tSz.Height)
		}
		tl := tp.Layout(tSz).(*Layout)
		defer tl.Release()
		attBox, ok := tl.Attachment(0)
		_, bases, _ := tl.placed()
		if !ok || math.Abs(float64(attBox.Origin.Y)-(bases[0].Y-80)) > 0.5 {
			t.Fatalf("tall attachment box %v (ok=%v) disagrees with baseline %v", attBox, ok, bases[0].Y)
		}
	})
}
