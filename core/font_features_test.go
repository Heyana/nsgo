package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestStyleFontAxesAndFeaturesInheritAndClear(t *testing.T) {
	h := newHost(t, Size{Width: 400, Height: 200}, func(cx *Scope) Node {
		return View{
			Style: Style{
				FontSize:     16,
				FontVariant:  FontTabularNums,
				FontAxes:     map[string]float32{"wdth": 75, "opsz": 14},
				FontFeatures: map[string]int{"ss01": 1, "zero": 1},
			},
			Children: []Node{
				Text{TestID: "inherited", Content: "0123"},
				Text{
					TestID:  "cleared",
					Content: "0123",
					Style: Style{
						FontAxes:     map[string]float32{},
						FontFeatures: map[string]int{},
					},
				},
				Text{
					TestID:  "overridden",
					Content: "0123",
					Style: Style{
						FontAxes:     map[string]float32{"wdth": 125},
						FontFeatures: map[string]int{"tnum": 0, "zero": 1},
					},
				},
			},
		}
	})
	h.commit()

	inh := h.find("inherited").text.style.Font
	if inh.Axes["wdth"] != 75 || inh.Axes["opsz"] != 14 {
		t.Fatalf("inherited Axes = %+v", inh.Axes)
	}
	if inh.Features["ss01"] != 1 || inh.Features["zero"] != 1 || inh.Variant != FontTabularNums {
		t.Fatalf("inherited Features = %+v, Variant = %v", inh.Features, inh.Variant)
	}

	clr := h.find("cleared").text.style.Font
	if len(clr.Axes) != 0 || len(clr.Features) != 0 {
		t.Fatalf("cleared font kept Axes=%+v Features=%+v", clr.Axes, clr.Features)
	}

	ovr := h.find("overridden").text.style.Font
	if len(ovr.Axes) != 1 || ovr.Axes["wdth"] != 125 {
		t.Fatalf("overridden Axes = %+v", ovr.Axes)
	}
	if ovr.Features["tnum"] != 0 || ovr.Features["zero"] != 1 || ovr.Variant != FontTabularNums {
		t.Fatalf("overridden Features = %+v, Variant = %v", ovr.Features, ovr.Variant)
	}
}

func TestMeasureCacheDifferentiatesFontAxesAndFeatures(t *testing.T) {
	mc := newMeasureCache()
	base := &driver.AttributedText{
		Text:  "0123456789",
		Style: driver.TextStyle{Font: driver.Font{Size: 16, Weight: FontWeightNormal}},
	}
	narrow := &driver.AttributedText{
		Text:  "0123456789",
		Style: driver.TextStyle{Font: driver.Font{Size: 16, Weight: FontWeightNormal, Axes: map[string]float32{"wdth": 75}}},
	}
	wide := &driver.AttributedText{
		Text:  "0123456789",
		Style: driver.TextStyle{Font: driver.Font{Size: 16, Weight: FontWeightNormal, Axes: map[string]float32{"wdth": 125}}},
	}
	slashed := &driver.AttributedText{
		Text:  "0123456789",
		Style: driver.TextStyle{Font: driver.Font{Size: 16, Weight: FontWeightNormal, Features: map[string]int{"zero": 1}}},
	}
	if mc.hash(base) == mc.hash(narrow) || mc.hash(narrow) == mc.hash(wide) || mc.hash(base) == mc.hash(slashed) {
		t.Fatal("measureCache hash must differentiate Font.Axes and Font.Features")
	}
}

func TestStyleInternClonesFontMaps(t *testing.T) {
	rt := &runtime{}
	axes := map[string]float32{"wdth": 80}
	feats := map[string]int{"ss01": 1}
	s := rt.internStyle(&Style{FontAxes: axes, FontFeatures: feats})
	axes["wdth"] = 120
	feats["ss01"] = 0
	if s.FontAxes["wdth"] != 80 || s.FontFeatures["ss01"] != 1 {
		t.Fatalf("interned style mutated by caller: Axes=%+v Features=%+v", s.FontAxes, s.FontFeatures)
	}
	s2 := rt.internStyle(&Style{FontAxes: map[string]float32{"wdth": 80}, FontFeatures: map[string]int{"ss01": 1}})
	if s != s2 {
		t.Fatal("identical FontAxes/FontFeatures styles should intern to the same pointer")
	}
}
