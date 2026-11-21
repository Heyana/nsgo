package ios

import (
	"testing"
	"time"

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

func TestPlanBackdropTransition(t *testing.T) {
	bgTransition := &driver.ViewStyle{
		Background: driver.RGBA(0x112233, 0.5),
		Transition: driver.Transition{Duration: time.Second, Properties: driver.TransitionBackground},
	}
	defaultTransition := &driver.ViewStyle{
		Background: driver.RGBA(0x112233, 0.5),
		Transition: driver.Transition{Duration: time.Second},
	}
	opacityOnly := &driver.ViewStyle{
		Background: driver.RGBA(0x112233, 0.5),
		Transition: driver.Transition{Duration: time.Second, Properties: driver.TransitionOpacity},
	}
	noTransition := &driver.ViewStyle{Background: driver.RGBA(0x112233, 0.5)}

	cases := []struct {
		name       string
		prev, next backdropMode
		vs         *driver.ViewStyle
		animatedBg bool
		want       backdropUpdate
	}{
		{
			name: "plain to blur clears outer animation without recreating",
			prev: backdropNone, next: backdropMaterial, vs: bgTransition,
			want: backdropUpdate{next: backdropMaterial, clearOuterAnim: true},
		},
		{
			name: "blur to blur (material change) keeps backdrop and tint",
			prev: backdropMaterial, next: backdropMaterial, vs: bgTransition,
			want: backdropUpdate{next: backdropMaterial},
		},
		{
			name: "blur to none with explicit background transition hands off tint to outer",
			prev: backdropMaterial, next: backdropNone, vs: bgTransition,
			want: backdropUpdate{next: backdropNone, recreate: true, handoffToOuter: true},
		},
		{
			name: "blur to none with default transition hands off tint to outer",
			prev: backdropMaterial, next: backdropNone, vs: defaultTransition,
			want: backdropUpdate{next: backdropNone, recreate: true, handoffToOuter: true},
		},
		{
			name: "blur to none with opacity-only transition skips handoff",
			prev: backdropMaterial, next: backdropNone, vs: opacityOnly,
			want: backdropUpdate{next: backdropNone, recreate: true},
		},
		{
			name: "blur to none without transition skips handoff",
			prev: backdropMaterial, next: backdropNone, vs: noTransition,
			want: backdropUpdate{next: backdropNone, recreate: true},
		},
		{
			name: "blur to none during animated background skips handoff",
			prev: backdropMaterial, next: backdropNone, vs: bgTransition, animatedBg: true,
			want: backdropUpdate{next: backdropNone, recreate: true},
		},
		{
			name: "blur to glass recreates and hands off when transitioning background",
			prev: backdropMaterial, next: backdropGlass, vs: bgTransition,
			want: backdropUpdate{next: backdropGlass, recreate: true, handoffToOuter: true},
		},
		{
			name: "glass to blur recreates and clears outer background animation",
			prev: backdropGlass, next: backdropMaterial, vs: bgTransition,
			want: backdropUpdate{next: backdropMaterial, recreate: true, clearOuterAnim: true},
		},
		{
			name: "glass to none recreates without tint handoff",
			prev: backdropGlass, next: backdropNone, vs: bgTransition,
			want: backdropUpdate{next: backdropNone, recreate: true},
		},
		{
			name: "none to glass creates glass without clearing outer background",
			prev: backdropNone, next: backdropGlass, vs: bgTransition,
			want: backdropUpdate{next: backdropGlass},
		},
	}

	for _, c := range cases {
		if got := planBackdropTransition(c.prev, c.next, c.vs, c.animatedBg); got != c.want {
			t.Errorf("%s: planBackdropTransition = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestOuterLayerStyleForBackdropTint(t *testing.T) {
	base := driver.ViewStyle{
		Opacity:      0.8,
		Background:   driver.RGBA(0x224466, 0.75),
		CornerRadius: 12,
		BorderWidth:  1,
		BorderColor:  driver.RGB(0xffffff),
		Clip:         true,
		Transform:    driver.Transform{TranslateX: 8},
	}

	// Without a blur backdrop tint, the outer layer keeps background and transitions untouched.
	withBgTransition := base
	withBgTransition.Transition = driver.Transition{
		Duration:   time.Second,
		Delay:      100 * time.Millisecond,
		Properties: driver.TransitionBackground,
		Easing:     driver.EaseInOut,
	}
	if got := outerLayerStyle(&withBgTransition, false); got != withBgTransition {
		t.Errorf("usesTint=false changed outer style: got %+v, want %+v", got, withBgTransition)
	}

	// With a blur backdrop tint and a background-only transition, outer stays transparent with no transition.
	got := outerLayerStyle(&withBgTransition, true)
	if got.Background != 0 {
		t.Errorf("usesTint=true kept outer background %#x", got.Background)
	}
	if got.Transition != (driver.Transition{}) {
		t.Errorf("usesTint=true kept background-only outer transition %+v", got.Transition)
	}
	if got.Opacity != base.Opacity || got.CornerRadius != base.CornerRadius || got.BorderWidth != base.BorderWidth ||
		got.BorderColor != base.BorderColor || !got.Clip || got.Transform != base.Transform {
		t.Errorf("usesTint=true altered non-background style properties: %+v", got)
	}

	// Default transition (Properties == 0) keeps other transitionable properties on outer while excluding background.
	withDefault := base
	withDefault.Transition = driver.Transition{
		Duration: 500 * time.Millisecond,
		Delay:    50 * time.Millisecond,
		Easing:   driver.EaseOut,
	}
	gotDefault := outerLayerStyle(&withDefault, true)
	if gotDefault.Background != 0 {
		t.Errorf("default transition kept outer background %#x", gotDefault.Background)
	}
	if gotDefault.Transition.Duration != withDefault.Transition.Duration ||
		gotDefault.Transition.Delay != withDefault.Transition.Delay ||
		gotDefault.Transition.Easing != withDefault.Transition.Easing {
		t.Errorf("default transition altered timing: got %+v", gotDefault.Transition)
	}
	if gotDefault.Transition.Properties == 0 || gotDefault.Transition.Properties&driver.TransitionBackground != 0 {
		t.Errorf("default transition still animates outer background: properties=%#x", gotDefault.Transition.Properties)
	}
	for _, prop := range []driver.TransitionProperty{
		driver.TransitionOpacity,
		driver.TransitionTransform,
		driver.TransitionBorder,
		driver.TransitionShadow,
	} {
		if gotDefault.Transition.Properties&prop == 0 {
			t.Errorf("default transition dropped outer property %#x", prop)
		}
	}

	// Mixed transition preserves non-background properties and drops background.
	withMixed := base
	withMixed.Transition = driver.Transition{
		Duration:   time.Second,
		Properties: driver.TransitionOpacity | driver.TransitionBackground | driver.TransitionTransform,
	}
	gotMixed := outerLayerStyle(&withMixed, true)
	if wantProps := driver.TransitionOpacity | driver.TransitionTransform; gotMixed.Transition.Properties != wantProps {
		t.Errorf("mixed transition properties = %#x, want %#x", gotMixed.Transition.Properties, wantProps)
	}

	// Opacity-only transition is preserved intact while background is cleared.
	withOpacity := base
	withOpacity.Transition = driver.Transition{
		Duration:   time.Second,
		Properties: driver.TransitionOpacity,
	}
	gotOpacity := outerLayerStyle(&withOpacity, true)
	if gotOpacity.Background != 0 {
		t.Errorf("opacity-only transition kept outer background %#x", gotOpacity.Background)
	}
	if gotOpacity.Transition != withOpacity.Transition {
		t.Errorf("opacity-only transition changed: got %+v, want %+v", gotOpacity.Transition, withOpacity.Transition)
	}
}
