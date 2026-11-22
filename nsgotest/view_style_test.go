package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMountedStylesKeepValuesAndSharedAppResources(t *testing.T) {
	effects := &core.Effects{Shadows: []core.BoxShadow{{Color: core.RGB(0x112233)}}}
	cursor := &core.CustomCursor{Hotspot: core.Point{X: 2, Y: 3}, Fallback: core.CursorText}
	native := driver.ViewStyle{Background: core.RGB(0xffffff), Opacity: 0.8, CornerRadius: 20,
		BorderWidth: 4, BorderColor: core.RGB(0), Transform: core.Transform{TranslateX: 4, TranslateY: 5},
		Effects: effects, CursorImage: cursor, Elevation: 7, Glass: core.GlassRegular}
	model := &App{t: t, views: map[driver.NodeID]*View{}}
	model.apply(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &native},
	}})
	got := model.views[1].Style
	native.Opacity, native.Transform.TranslateX = 0.1, 90
	if got.Opacity != 0.8 || got.Transform.TranslateX != 4 || got.Elevation != 7 || got.Glass != core.GlassRegular {
		t.Fatalf("mounted style changed: %+v", got)
	}
	if got.Effects != effects || got.CursorImage != cursor {
		t.Fatal("immutable effects or cursor lost their app-value identity")
	}
	if !got.Rounded() || got.Radii(core.Size{Width: 20, Height: 10}) != core.AllCorners(5) {
		t.Fatal("recorded radii no longer fit the view")
	}
	if color, inset, ok := got.Rim(); !ok || color != core.RGB(0) || inset != 3 {
		t.Fatalf("rounded rim = %v, %v, %v", color, inset, ok)
	}
	if x, y := got.Matrix(core.Size{Width: 20, Height: 10}).Apply(0, 0); x != 4 || y != 5 {
		t.Fatalf("recorded transform moved the origin to %v, %v", x, y)
	}
}

func TestAnimatedStyleRecordAppliesOnlyBoundValues(t *testing.T) {
	base := ViewStyle{Opacity: 1, Background: core.RGB(0xffffff), Transform: core.Transform{TranslateY: 9},
		Borders: core.All(2), BorderColors: core.AllColors(core.RGB(0)), ShadowColor: core.RGB(0x336699)}
	values := AnimatedValues{Set: 1<<PropOpacity | 1<<PropTranslateX | 1<<PropBackground | 1<<PropBorderColor | 1<<PropShadowOpacity,
		Opacity: 0.25, TranslateX: 12, Background: core.RGB(0x112233), BorderColor: core.RGB(0xff0000), ShadowOpacity: 0}
	got := values.Apply(base)
	if !values.Has(PropOpacity) || values.Has(PropTranslateY) || got.Opacity != 0.25 || got.Transform.TranslateX != 12 || got.Transform.TranslateY != 9 {
		t.Fatalf("bound transform values = %+v", got)
	}
	if got.Background != values.Background || got.BorderColors != core.AllColors(values.BorderColor) || got.ShadowColor&0xff != 0 {
		t.Fatalf("bound colors = %+v", got)
	}
	if base.Opacity != 1 || base.Transform.TranslateX != 0 || base.Background != core.RGB(0xffffff) {
		t.Fatal("applying recorded values changed the base style")
	}
}

func TestRecordedAnimatedStyleFollowsTheAnimator(t *testing.T) {
	if Target() != "" {
		t.Skip("headless animator records")
	}
	var opacity *core.AnimatedValue
	app := Start(t, func(cx *core.Scope) core.Node {
		opacity = core.NewAnimatedValue(cx, 0.25)
		return core.View{TestID: "box", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), Opacity: core.Alpha(1),
			Animated: &core.AnimatedStyle{Opacity: opacity}}}
	})
	if got := app.Animated("id:box"); got.Opacity != 0.25 {
		t.Fatalf("initial animated opacity = %v", got.Opacity)
	}
	app.Post(func() { opacity.Set(0.75) })
	app.Settle()
	view := app.Find("id:box")
	if !view.Animated.Has(PropOpacity) || view.Animated.Opacity != 0.75 || app.Animated("id:box").Opacity != 0.75 || view.Style.Opacity != 1 {
		t.Fatalf("recorded animated values = %+v, base = %+v", view.Animated, view.Style)
	}
}
