package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestChooseBackdrop(t *testing.T) {
	colorFilters := []struct {
		name   string
		filter driver.Filter
	}{
		{"brightness", driver.Filter{Kind: driver.FilterBrightness, Amount: 0.5}},
		{"contrast", driver.Filter{Kind: driver.FilterContrast, Amount: 1.5}},
		{"saturate", driver.Filter{Kind: driver.FilterSaturate, Amount: 0}},
		{"grayscale", driver.Filter{Kind: driver.FilterGrayscale, Amount: 1}},
		{"sepia", driver.Filter{Kind: driver.FilterSepia, Amount: 0.8}},
		{"invert", driver.Filter{Kind: driver.FilterInvert, Amount: 1}},
		{"hue-rotate", driver.Filter{Kind: driver.FilterHueRotate, Amount: 90}},
		{"opacity", driver.Filter{Kind: driver.FilterOpacity, Amount: 0.6}},
	}

	// Every color filter alone uses CABackdropLayer regardless of Material.
	for _, cf := range colorFilters {
		for _, m := range []driver.BackdropMaterial{driver.BackdropUltraThin, driver.BackdropThin, driver.BackdropRegular} {
			vs := driver.ViewStyle{
				Material: m,
				Effects:  &driver.Effects{BackdropFilters: []driver.Filter{cf.filter}},
			}
			if got := chooseBackdrop(&vs, false); got != backdropFiltered {
				t.Errorf("%s alone (material %d): chooseBackdrop = %d, want backdropFiltered", cf.name, m, got)
			}
		}
	}

	// Color filters combined with a blur use CABackdropLayer when Material is
	// BackdropUltraThin (the zero value), and keep the named UIVisualEffectView
	// material otherwise, matching macOS.
	for _, cf := range colorFilters {
		vs := driver.ViewStyle{
			Material: driver.BackdropUltraThin,
			Effects: &driver.Effects{BackdropFilters: []driver.Filter{
				{Kind: driver.FilterBlur, Amount: 8},
				cf.filter,
			}},
		}
		if got := chooseBackdrop(&vs, false); got != backdropFiltered {
			t.Errorf("blur+%s (default material): chooseBackdrop = %d, want backdropFiltered", cf.name, got)
		}
		vs.Material = driver.BackdropThin
		if got := chooseBackdrop(&vs, false); got != backdropMaterial {
			t.Errorf("blur+%s (named material): chooseBackdrop = %d, want backdropMaterial", cf.name, got)
		}
	}

	cases := []struct {
		name     string
		vs       *driver.ViewStyle
		hasGlass bool
		want     backdropMode
	}{
		{"nil style", nil, false, backdropNone},
		{"empty style", &driver.ViewStyle{}, false, backdropNone},
		{"empty effects", &driver.ViewStyle{Effects: &driver.Effects{}}, false, backdropNone},
		{"zero blur alone", &driver.ViewStyle{Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 0}}}}, false, backdropNone},
		{"drop shadow alone", &driver.ViewStyle{Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterDropShadow, Shadow: driver.BoxShadow{Blur: 4}}}}}, false, backdropNone},
		{"zero blur with color filter", &driver.ViewStyle{Material: driver.BackdropRegular, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 0}, {Kind: driver.FilterBrightness, Amount: 0.5}}}}, false, backdropFiltered},
		{"plain blur default material", &driver.ViewStyle{Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 12}}}}, false, backdropMaterial},
		{"plain blur named material", &driver.ViewStyle{Material: driver.BackdropChrome, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 12}}}}, false, backdropMaterial},
		{"glass when available", &driver.ViewStyle{Glass: driver.GlassRegular, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 12}, {Kind: driver.FilterSaturate, Amount: 0}}}}, true, backdropGlass},
		{"glass without effects when available", &driver.ViewStyle{Glass: driver.GlassClear}, true, backdropGlass},
		{"glass fallback to filtered", &driver.ViewStyle{Glass: driver.GlassRegular, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 12}, {Kind: driver.FilterBrightness, Amount: 0.8}}}}, false, backdropFiltered},
		{"glass fallback to material", &driver.ViewStyle{Glass: driver.GlassRegular, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 12}}}}, false, backdropMaterial},
		{"glass fallback to none", &driver.ViewStyle{Glass: driver.GlassRegular}, false, backdropNone},
	}
	for _, tc := range cases {
		if got := chooseBackdrop(tc.vs, tc.hasGlass); got != tc.want {
			t.Errorf("%s: chooseBackdrop = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestHidesOwnBackground(t *testing.T) {
	bg := driver.RGBA(0x112233, 0.5)
	cases := []struct {
		name     string
		vs       *driver.ViewStyle
		hasGlass bool
		want     bool
	}{
		{"nil style", nil, false, false},
		{"filtered with background", &driver.ViewStyle{Background: bg, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBrightness, Amount: 0.5}}}}, false, true},
		{"filtered blur+color with background", &driver.ViewStyle{Background: bg, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 10}, {Kind: driver.FilterInvert, Amount: 1}}}}, false, true},
		{"filtered without background", &driver.ViewStyle{Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBrightness, Amount: 0.5}}}}, false, false},
		{"material blur with background", &driver.ViewStyle{Background: bg, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 10}}}}, false, false},
		{"glass with background", &driver.ViewStyle{Background: bg, Glass: driver.GlassRegular, Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBrightness, Amount: 0.5}}}}, true, false},
		{"no backdrop with background", &driver.ViewStyle{Background: bg}, false, false},
	}
	for _, tc := range cases {
		if got := hidesOwnBackground(tc.vs, tc.hasGlass); got != tc.want {
			t.Errorf("%s: hidesOwnBackground = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBlurStyle(t *testing.T) {
	cases := []struct {
		m    driver.BackdropMaterial
		want int
	}{
		{driver.BackdropUltraThin, 6},
		{driver.BackdropThin, 7},
		{driver.BackdropRegular, 8},
		{driver.BackdropThick, 9},
		{driver.BackdropChrome, 10},
	}
	for _, tc := range cases {
		if got := blurStyle(tc.m); got != tc.want {
			t.Errorf("blurStyle(%d) = %d, want %d", tc.m, got, tc.want)
		}
	}
}
