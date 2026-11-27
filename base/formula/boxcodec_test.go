package formula

import (
	"context"
	"reflect"
	"testing"

	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/display"
	"github.com/caelyreth/untie/source"
	"golang.org/x/image/font/gofont/goregular"
)

func TestBoxCodecRoundTrip(t *testing.T) {
	eng, err := Default().engine()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		source   string
		dialect  untie.Dialect
		mode     untie.Mode
		maxWidth float32
	}{
		{name: "integral", source: `\int_0^\infty \frac{x^2}{\sqrt{1+x}}\,dx`, dialect: untie.TeX, mode: untie.Display},
		{name: "matrix-color", source: `\color{red}{\begin{pmatrix}a&b\\c&d\end{pmatrix}} + \notacommand`, dialect: untie.TeX, mode: untie.Inline},
		{name: "wrapped-display", source: wrappingFormula, dialect: untie.TeX, mode: untie.Display, maxWidth: 600},
		{name: "cjk-missing", source: `x+\text{中文}+y`, dialect: untie.TeX, mode: untie.Inline},
		{name: "typst", source: `sum_(i=1)^n x_i / sqrt(i)`, dialect: untie.Typst, mode: untie.Display},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr := untie.Parse(tc.source, tc.dialect, tc.mode, untie.ParseOptions{})
			style := untie.Style{Size: referenceSize, MaxWidth: tc.maxWidth, Align: untie.AlignCenter}
			want, err := eng.LayoutContextWithOptions(context.Background(), expr, style, untie.LayoutOptions{Recovery: untie.RecoveryVisible})
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeBox(encodeBox(want))
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != want.Bounds() {
				t.Fatalf("bounds = %+v, want %+v", got.Bounds(), want.Bounds())
			}
			if !reflect.DeepEqual(got.Items, want.Items) {
				t.Fatalf("items differ after round trip")
			}
			if !reflect.DeepEqual(got.Paths, want.Paths) {
				t.Fatalf("paths differ after round trip")
			}
			if !reflect.DeepEqual(got.Hits, want.Hits) {
				t.Fatalf("hits differ after round trip")
			}
			if !reflect.DeepEqual(got.CaretStops, want.CaretStops) {
				t.Fatalf("caret stops differ after round trip")
			}
			if !reflect.DeepEqual(got.FontUsage, want.FontUsage) {
				t.Fatalf("font usage = %+v, want %+v", got.FontUsage, want.FontUsage)
			}
			var a, b layout
			a.drawBox(want, nil)
			b.drawBox(got, nil)
			if a.em != b.em || len(a.runs) != len(b.runs) {
				t.Fatalf("drawn layout differs: em %+v vs %+v, runs %d vs %d", a.em, b.em, len(a.runs), len(b.runs))
			}
		})
	}
}

func TestBoxCodecPreservesFallbackFaceUsage(t *testing.T) {
	r, err := NewRenderer(Fonts{Math: newCMMath, Text: goregular.TTF, Fallbacks: [][]byte{newCMMath}})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := r.engine()
	if err != nil {
		t.Fatal(err)
	}
	expr := untie.ParseTeX(`\text{𝔸}`, untie.Inline, untie.ParseOptions{})
	want, err := eng.LayoutContextWithOptions(context.Background(), expr, untie.Style{Size: referenceSize}, untie.LayoutOptions{Recovery: untie.RecoveryVisible})
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeBox(encodeBox(want))
	if err != nil {
		t.Fatal(err)
	}
	if !got.FontUsage.UsesFallback() || got.FontUsage.NeedsFallback() || !reflect.DeepEqual(got.FontUsage, want.FontUsage) {
		t.Fatalf("font usage = %+v, want %+v", got.FontUsage, want.FontUsage)
	}
}

func TestRequestCodecsRoundTripAndRejectTruncated(t *testing.T) {
	lreq := layoutRequest{
		source:  `\frac{1}{2}`,
		dialect: untie.TeX,
		mode:    untie.Display,
		style: untie.Style{
			Size:        referenceSize,
			MaxWidth:    420,
			Align:       untie.AlignCenter,
			ScriptLevel: 1,
			Features:    []string{"ssty"},
			GlyphRefs:   true,
			GenerateAlt: false,
		},
		opts: untie.LayoutOptions{Recovery: untie.RecoveryVisible},
	}
	lgot, err := decodeLayoutRequest(encodeLayoutRequest(lreq))
	if err != nil || !reflect.DeepEqual(lgot, lreq) {
		t.Fatalf("layoutRequest round trip = %+v (%v), want %+v", lgot, err, lreq)
	}

	sreq := systemTextRequest{
		key:      layoutKey{source: `\text{中文}`, dialect: TeX, display: true, maxWidth: 300},
		language: "zh-Hans",
		missing: []untie.MissingFallback{{
			Source:     source.Range{Start: 6, End: 12},
			Text:       "中文",
			TriedFaces: []string{"math"},
		}},
		measured: []textMeasure{{width: 200, ascent: 80, descent: 20}},
	}
	sgot, err := decodeSystemTextRequest(encodeSystemTextRequest(sreq))
	if err != nil || !reflect.DeepEqual(sgot, sreq) {
		t.Fatalf("systemTextRequest round trip = %+v (%v), want %+v", sgot, err, sreq)
	}

	if _, err := decodeBox([]byte{1, 2, 3}); err == nil {
		t.Error("decodeBox accepted truncated input")
	}
	if _, err := decodeLayoutRequest([]byte{0, 0, 0, 10, 'a'}); err == nil {
		t.Error("decodeLayoutRequest accepted truncated input")
	}
	if _, err := decodeSystemTextRequest([]byte{0, 0, 0, 10, 'a'}); err == nil {
		t.Error("decodeSystemTextRequest accepted truncated input")
	}
	if got, err := decodeBox(nil); err != nil || got != nil {
		t.Fatalf("decodeBox(nil) = %v, %v", got, err)
	}
	if got := encodeBox(nil); got != nil {
		t.Fatalf("encodeBox(nil) = %v", got)
	}
	// Verify empty box with no items round-trips cleanly.
	empty := &untie.Box{Width: 10, Ascent: 8, Descent: 2, Items: nil, Paths: nil, Hits: nil, CaretStops: nil, FontUsage: display.FontUsage{}}
	if got, err := decodeBox(encodeBox(empty)); err != nil || !reflect.DeepEqual(got, empty) {
		t.Fatalf("empty box round trip = %+v (%v), want %+v", got, err, empty)
	}
}
