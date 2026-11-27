package driver

import (
	"math"
	"slices"
	"testing"
)

func TestValidFontTagAndPacking(t *testing.T) {
	for _, good := range []string{"wdth", "opsz", "slnt", "ss01", "zero", "CV01", "a   "} {
		if !ValidFontTag(good) {
			t.Errorf("ValidFontTag(%q) = false, want true", good)
		}
	}
	for _, bad := range []string{"", "wdt", "width", "    ", "ab\"c", "ab'c", "ab,c", "ab;c", "ab\\c", "ab\nc", "a\x7fbc"} {
		if ValidFontTag(bad) {
			t.Errorf("ValidFontTag(%q) = true, want false", bad)
		}
	}
	if got := FontTagBE("wdth"); got != 0x77647468 {
		t.Errorf("FontTagBE(wdth) = %#08x, want 0x77647468", got)
	}
	if got := FontTagLE("wdth"); got != 0x68746477 {
		t.Errorf("FontTagLE(wdth) = %#08x, want 0x68746477", got)
	}
}

func TestFontResolvedAxesAndFormatting(t *testing.T) {
	f := Font{
		Axes: map[string]float32{
			"wdth": 75.5,
			"opsz": 14,
			"slnt": -10,
			"ZERO": float32(math.Copysign(0, -1)),
			"bad":  100,
			"nan1": float32(math.NaN()),
			"inf1": float32(math.Inf(1)),
		},
	}
	want := []FontAxis{
		{Tag: "ZERO", Value: 0},
		{Tag: "opsz", Value: 14},
		{Tag: "slnt", Value: -10},
		{Tag: "wdth", Value: 75.5},
	}
	if got := ResolvedFontAxes(f); !slices.Equal(got, want) {
		t.Fatalf("ResolvedFontAxes() = %+v, want %+v", got, want)
	}
	if got, want := FontCSSVariationSettings(f), `"ZERO" 0, "opsz" 14, "slnt" -10, "wdth" 75.5`; got != want {
		t.Errorf("FontCSSVariationSettings() = %q, want %q", got, want)
	}
	if got, want := FontAndroidVariationSettings(f), `'ZERO' 0, 'opsz' 14, 'slnt' -10, 'wdth' 75.5`; got != want {
		t.Errorf("FontAndroidVariationSettings() = %q, want %q", got, want)
	}
	if got, want := FontPangoVariations(f), `ZERO=0,opsz=14,slnt=-10,wdth=75.5`; got != want {
		t.Errorf("FontPangoVariations() = %q, want %q", got, want)
	}
}

func TestFontResolvedFeaturesMergesVariantAndOverrides(t *testing.T) {
	f := Font{
		Variant: FontTabularNums | FontSmallCaps,
		Features: map[string]int{
			"tnum": 0, // explicitly disables Variant's FontTabularNums
			"ss01": 1,
			"zero": 1,
			"cv01": 2,
			"liga": -3, // clamped to 0
			"bad":  1,
		},
	}
	want := []FontFeature{
		{Tag: "cv01", Value: 2},
		{Tag: "liga", Value: 0},
		{Tag: "smcp", Value: 1},
		{Tag: "ss01", Value: 1},
		{Tag: "tnum", Value: 0},
		{Tag: "zero", Value: 1},
	}
	if got := ResolvedFontFeatures(f); !slices.Equal(got, want) {
		t.Fatalf("ResolvedFontFeatures() = %+v, want %+v", got, want)
	}
	if got, want := FontCSSFeatureSettings(f), `"cv01" 2, "liga" 0, "smcp" 1, "ss01" 1, "tnum" 0, "zero" 1`; got != want {
		t.Errorf("FontCSSFeatureSettings() = %q, want %q", got, want)
	}
	if got, want := FontAndroidFeatureSettings(f), `'cv01' 2, 'liga' 0, 'smcp' 1, 'ss01' 1, 'tnum' 0, 'zero' 1`; got != want {
		t.Errorf("FontAndroidFeatureSettings() = %q, want %q", got, want)
	}
	if got, want := FontPangoFeatures(f), `cv01 2, liga 0, smcp 1, ss01 1, tnum 0, zero 1`; got != want {
		t.Errorf("FontPangoFeatures() = %q, want %q", got, want)
	}
}

func TestFontKeyAndEncodingRoundTrip(t *testing.T) {
	a := Font{Size: 16, Axes: map[string]float32{"wdth": 75, "opsz": 14}, Features: map[string]int{"ss01": 1, "zero": 1}}
	b := Font{Size: 16, Axes: map[string]float32{"opsz": 14, "wdth": 75}, Features: map[string]int{"zero": 1, "ss01": 1}}
	c := Font{Size: 16, Axes: map[string]float32{"wdth": 125, "opsz": 14}, Features: map[string]int{"ss01": 1, "zero": 1}}
	d := Font{Size: 16, Axes: map[string]float32{"wdth": 75, "opsz": 14}, Features: map[string]int{"ss01": 0, "zero": 1}}

	if !a.Equal(b) || FontKeyOf(a) != FontKeyOf(b) {
		t.Fatalf("equivalent fonts should match: Equal=%v KeyEqual=%v", a.Equal(b), FontKeyOf(a) == FontKeyOf(b))
	}
	if a.Equal(c) || FontKeyOf(a) == FontKeyOf(c) {
		t.Fatal("fonts with different wdth must differ")
	}
	if a.Equal(d) || FontKeyOf(a) == FontKeyOf(d) {
		t.Fatal("fonts with different ss01 must differ")
	}
	if !(Font{}).IsZero() || a.IsZero() {
		t.Fatal("IsZero mismatch")
	}

	// Nil vs explicit empty map encoding distinction.
	if EncodeFontAxes(nil) != "" {
		t.Fatal("nil axes should encode empty")
	}
	if EncodeFontAxes(map[string]float32{}) == "" {
		t.Fatal("non-nil empty axes should encode non-empty marker")
	}
	if DecodeFontAxes(EncodeFontAxes(map[string]float32{})) != nil {
		t.Fatal("empty axes marker should decode to nil")
	}
	axes := DecodeFontAxes(EncodeFontAxes(a.Axes))
	if axes["wdth"] != 75 || axes["opsz"] != 14 || len(axes) != 2 {
		t.Fatalf("DecodeFontAxes = %+v", axes)
	}
	if EncodeFontFeatures(nil) != "" || EncodeFontFeatures(map[string]int{}) == "" {
		t.Fatal("EncodeFontFeatures nil vs empty mismatch")
	}
	feats := DecodeFontFeatures(EncodeFontFeatures(a.Features))
	if feats["ss01"] != 1 || feats["zero"] != 1 || len(feats) != 2 {
		t.Fatalf("DecodeFontFeatures = %+v", feats)
	}
}
