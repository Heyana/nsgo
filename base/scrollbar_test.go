package base_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestScrollbarTouchDragVerticalAndHorizontalOnPhone(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  core.Environment
	}{
		{"ios", core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}},
		{"android", core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var vRef, hRef *core.Ref
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				vRef, hRef = &core.Ref{}, &core.Ref{}
				return core.View{Style: core.Style{Width: core.Pt(390), Height: core.Pt(500)}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(390), Height: core.Pt(240)}, Children: []core.Node{
						core.Scroll{
							Ref:           vRef,
							TestID:        "vscroll",
							HideIndicator: true,
							Style:         core.Style{FlexGrow: 1},
							Children:      []core.Node{core.View{Style: core.Style{Height: core.Pt(1440)}}},
						},
						base.Scrollbar{
							Scroll:     func() *core.Ref { return vRef },
							TestID:     "vbar",
							Mode:       func() base.ScrollbarMode { return base.ScrollbarAlways },
							Style:      core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Right: core.Pt(0), Width: core.Pt(24)},
							ThumbStyle: core.Style{Background: core.RGB(0x222222)},
						},
					}},
					core.View{Style: core.Style{Width: core.Pt(390), Height: core.Pt(160)}, Children: []core.Node{
						core.Scroll{
							Ref:           hRef,
							TestID:        "hscroll",
							Horizontal:    true,
							HideIndicator: true,
							Style:         core.Style{FlexGrow: 1},
							Children:      []core.Node{core.View{Style: core.Style{Width: core.Pt(1950), Height: core.Pt(160)}}},
						},
						base.Scrollbar{
							Scroll:     func() *core.Ref { return hRef },
							Horizontal: true,
							TestID:     "hbar",
							Mode:       func() base.ScrollbarMode { return base.ScrollbarAlways },
							Style:      core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Height: core.Pt(24)},
							ThumbStyle: core.Style{Background: core.RGB(0x222222)},
						},
					}},
				}}
			}, nsgotest.Environment(tc.env), nsgotest.Size(390, 500))

			// Touch drag vertical thumb downward.
			vf := app.Touch("vbar", core.Point{X: 12, Y: 10})
			vf.Move(core.Point{X: 12, Y: 110})
			vf.Lift()
			if got := app.Find("vscroll").Offset.Y; got <= 400 {
				t.Fatalf("vertical scrollbar touch drag offset = %v, want > 400", got)
			}

			// Touch drag horizontal thumb rightward.
			hf := app.Touch("hbar", core.Point{X: 10, Y: 12})
			hf.Move(core.Point{X: 190, Y: 12})
			hf.Lift()
			if got := app.Find("hscroll").Offset.X; got <= 600 {
				t.Fatalf("horizontal scrollbar touch drag offset = %v, want > 600", got)
			}
		})
	}
}

func TestScrollbarThemeReplacesPhoneHandle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		base.SetScrollbarTheme(cx, &base.ScrollbarTheme{
			Mode: base.ScrollbarAlways,
			Vertical: &base.ScrollbarLook{
				Style:      core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Right: core.Pt(0), Width: core.Pt(16)},
				ThumbStyle: core.Style{Background: core.RGB(0x444444)},
			},
		})
		return core.Scroll{
			TestID:   "themed",
			Style:    core.Style{Width: core.Pt(390), Height: core.Pt(200)},
			Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(1200)}}},
		}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}), nsgotest.Size(390, 200))

	if _, ok := app.ScrollHandle("themed"); ok {
		t.Fatal("custom ScrollbarTheme should hide native phone scroll handle")
	}

	// Short scroll view with ScrollbarAlways stays hidden (opacity 0, PointerEventsNone).
	shortApp := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ref := &core.Ref{}
		return core.View{Style: core.Style{Width: core.Pt(390), Height: core.Pt(200)}, Children: []core.Node{
			core.Scroll{Ref: ref, TestID: "short", HideIndicator: true, Style: core.Style{FlexGrow: 1},
				Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(100)}}}},
			base.Scrollbar{Scroll: func() *core.Ref { return ref }, TestID: "sbar",
				Mode:  func() base.ScrollbarMode { return base.ScrollbarAlways },
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Right: core.Pt(0), Width: core.Pt(16)}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}), nsgotest.Size(390, 200))

	if op := shortApp.Find("sbar").Animated.Opacity; math.Abs(float64(op)) > 1e-6 {
		t.Fatalf("short scroll view custom scrollbar opacity = %v, want 0", op)
	}
}
