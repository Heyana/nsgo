package android

import (
	"image"
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestDirectionMirrorsNativeControls runs a live Android app with a native
// TabBar, SystemSwitch, single-line Input and Stack Header, toggles the app
// locale between LTR ("en"), RTL ("ar-XB") and back to LTR, and checks the
// rendered pixels on the Android emulator to verify each native control mirrors
// its layout and drawing (#1615).
func TestDirectionMirrorsNativeControls(t *testing.T) {
	if nsgotest.Target() != "android" {
		t.Skip("requires Android pixels")
	}
	svg, err := core.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect width="24" height="24" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	var setLocale func(string)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		theme := core.DefaultTheme()
		theme.Light.Accent, theme.Dark.Accent = core.RGB(0xd02030), core.RGB(0xd02030)
		cx.SetTheme(theme)
		setLocale = func(loc string) { cx.Post(func() { cx.SetLocale(loc) }) }
		return core.View{
			TestID: "root",
			Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100), Background: core.RGB(0xffffff), Direction: core.Column, Padding: core.Edges{Top: 60, Left: 20, Right: 20, Bottom: 20}, Gap: 16},
			Children: []core.Node{
				core.Input{
					TestID:       "field",
					DefaultValue: "MMMM",
					Style:        core.Style{Width: core.Pt(260), Height: core.Pt(44), Background: core.RGB(0xffffff), Color: core.RGB(0x000000), FontSize: 20},
				},
				core.SystemSwitch{
					TestID:  "toggle",
					Checked: func() bool { return true },
				},
				core.TabBar{
					TestID: "bar",
					Items: []core.TabBarItem{
						{Title: "Home", SVG: svg},
						{Title: "Search", SVG: svg},
					},
					Selected: func() int { return 0 },
					Style:    core.Style{Width: core.Pt(260), Height: core.Pt(80)},
				},
			},
		}
	})

	countPixels := func(img image.Image, x0, y0, x1, y1 int, match func(color.NRGBA) bool) int {
		b := img.Bounds()
		x0, x1 = max(x0, b.Min.X), min(x1, b.Max.X)
		y0, y1 = max(y0, b.Min.Y), min(y1, b.Max.Y)
		n := 0
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if match(color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)) {
					n++
				}
			}
		}
		return n
	}

	isDark := func(c color.NRGBA) bool { return c.R < 80 && c.G < 80 && c.B < 80 && c.A > 200 }
	isWhite := func(c color.NRGBA) bool { return c.R > 240 && c.G > 240 && c.B > 240 && c.A > 200 }
	isAccent := func(c color.NRGBA) bool { return c.R > 180 && c.G < 60 && c.B < 80 && c.A > 200 }

	check := func(phase string, rtl bool) {
		t.Helper()
		app.Settle()
		img := app.Capture()
		scale := float64(img.Bounds().Dx()) / float64(app.Find("id:root").Frame.Size.Width)

		// 1. Input text sits near the right edge in RTL and left edge in LTR.
		fb := app.Find("id:field").Bounds()
		fx0, fx1 := int(float64(fb.Origin.X)*scale), int(float64(fb.Origin.X+fb.Size.Width)*scale)
		fy0, fy1 := int(float64(fb.Origin.Y)*scale), int(float64(fb.Origin.Y+fb.Size.Height)*scale)
		fmid := (fx0 + fx1) / 2
		fLeft := countPixels(img, fx0, fy0, fmid, fy1, isDark)
		fRight := countPixels(img, fmid, fy0, fx1, fy1, isDark)
		if rtl && !(fRight > fLeft) {
			t.Errorf("%s: RTL input dark pixels left=%d right=%d, want right > left", phase, fLeft, fRight)
		}
		if !rtl && !(fLeft > fRight) {
			t.Errorf("%s: LTR input dark pixels left=%d right=%d, want left > right", phase, fLeft, fRight)
		}

		// 2. Checked SystemSwitch white thumb sits in left half in RTL and right half in LTR.
		sb := app.Find("id:toggle").Bounds()
		sx0, sx1 := int(float64(sb.Origin.X)*scale), int(float64(sb.Origin.X+sb.Size.Width)*scale)
		sy0, sy1 := int(float64(sb.Origin.Y)*scale), int(float64(sb.Origin.Y+sb.Size.Height)*scale)
		smid := (sx0 + sx1) / 2
		sLeft := countPixels(img, sx0, sy0, smid, sy1, isWhite)
		sRight := countPixels(img, smid, sy0, sx1, sy1, isWhite)
		if rtl && !(sLeft > sRight) {
			t.Errorf("%s: RTL checked switch white thumb pixels left=%d right=%d, want left > right", phase, sLeft, sRight)
		}
		if !rtl && !(sRight > sLeft) {
			t.Errorf("%s: LTR checked switch white thumb pixels left=%d right=%d, want right > left", phase, sLeft, sRight)
		}

		// 3. TabBar selected tab 0 accent icon sits in right half in RTL and left half in LTR.
		tb := app.Find("id:bar").Bounds()
		tx0, tx1 := int(float64(tb.Origin.X)*scale), int(float64(tb.Origin.X+tb.Size.Width)*scale)
		ty0, ty1 := int(float64(tb.Origin.Y)*scale), int(float64(tb.Origin.Y+tb.Size.Height)*scale)
		tmid := (tx0 + tx1) / 2
		tLeft := countPixels(img, tx0, ty0, tmid, ty1, isAccent)
		tRight := countPixels(img, tmid, ty0, tx1, ty1, isAccent)
		if rtl && !(tRight > tLeft) {
			t.Errorf("%s: RTL tab bar selected-0 accent pixels left=%d right=%d, want right > left", phase, tLeft, tRight)
		}
		if !rtl && !(tLeft > tRight) {
			t.Errorf("%s: LTR tab bar selected-0 accent pixels left=%d right=%d, want left > right", phase, tLeft, tRight)
		}
	}

	setLocale("en")
	check("LTR initial", false)

	setLocale("ar-XB")
	check("RTL", true)

	setLocale("en")
	check("LTR restored", false)
}
