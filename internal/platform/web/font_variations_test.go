//go:build js && wasm

package web

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestFontStyleEmitsVariationAndFeatureSettings(t *testing.T) {
	base := driver.TextStyle{
		Font: driver.Font{
			Family:  "Inter",
			Size:    16,
			Weight:  400,
			Variant: driver.FontTabularNums,
		},
	}
	css := fontStyle(base)
	if css["font-variation-settings"] != "normal" {
		t.Errorf("base font-variation-settings = %q, want normal", css["font-variation-settings"])
	}
	if css["font-feature-settings"] != "normal" {
		t.Errorf("base font-feature-settings = %q, want normal", css["font-feature-settings"])
	}
	if css["font-variant-numeric"] != "tabular-nums" {
		t.Errorf("base font-variant-numeric = %q, want tabular-nums", css["font-variant-numeric"])
	}

	rich := driver.TextStyle{
		Font: driver.Font{
			Family:   "Inter",
			Size:     16,
			Weight:   400,
			Variant:  driver.FontTabularNums,
			Axes:     map[string]float32{"wdth": 75, "opsz": 16},
			Features: map[string]int{"ss01": 1, "zero": 1, "tnum": 0},
		},
	}
	rcss := fontStyle(rich)
	if got, want := rcss["font-variation-settings"], `"opsz" 16, "wdth" 75`; got != want {
		t.Errorf("font-variation-settings = %q, want %q", got, want)
	}
	if got, want := rcss["font-feature-settings"], `"ss01" 1, "tnum" 0, "zero" 1`; got != want {
		t.Errorf("font-feature-settings = %q, want %q", got, want)
	}
	if _, ok := canvasFont(rich); ok {
		t.Error("canvasFont should decline styles with Axes or Features so the DOM measures them")
	}
}
