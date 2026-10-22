package main

import (
	"math"
	"sync"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// The brand's colors follow the app icon rather than the interface theme:
// its gradient from teal to green, and the yellow dot.
var (
	brandTeal   = core.RGB(0x58c6d3)
	brandGreen  = core.RGB(0x5dbd8a)
	brandYellow = core.RGB(0xf0e43c)
)

const repository = "github.com/oneo-me/nsgo"

// welcomePage is the gallery's front page, after Onevra's: the name, what
// NSGo is, its repository and the way to every page, over soft glows and a
// faint tilted field of Lucide icons. It enters with a brief, staggered
// rise.
func welcomePage(cx *core.Scope, t *theme) core.Node {
	g := galleryKey.Use(cx)
	toast := ui.UseToast(cx)
	env := cx.Env()
	compact := core.NewMemo(cx, func() bool { return env.Window().Width < 560 })
	enter := entrance(cx)
	copyLink := func() {
		cx.SetClipboard("https://" + repository)
		toast(ui.Toast{Title: "Link copied", Description: "https://" + repository, Icon: lucide.Copy(), Tone: ui.ToneAccent})
	}
	pill := core.Style{CornerRadius: 20, Padding: core.Symmetric(8, 16), Gap: 8, FlexShrink: 1}
	icon := func(svg *core.SVG, size core.Points) core.Node {
		return core.Icon{SVG: svg, Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0}}
	}
	wide := func(n core.Node) core.Node {
		return core.Show{When: func() bool { return !compact.Get() }, Then: func(*core.Scope) core.Node { return n }}
	}
	var badges []core.Node
	for _, b := range []string{"Version " + version, "Apache-2.0", "Go", "6 platforms"} {
		badges = append(badges, ui.Badge{Text: b, Style: core.Style{Color: t.muted, Background: withAlpha(t.surface, 0.8)}})
	}
	return core.View{Style: core.Style{FlexGrow: 1, Overflow: core.Hidden, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		TestID: "section-Welcome",
		Children: []core.Node{
			welcomeArt(t),
			core.View{Style: core.Style{MaxWidth: core.Pt(680), AlignSelf: core.AlignStretch, AlignItems: core.AlignCenter,
				Gap: 32, Padding: core.Symmetric(48, 24), AutoMargin: core.SideLeft | core.SideRight}, Children: []core.Node{
				enter(0, brandMark(t, 88)),
				enter(1, core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
					core.Text{Content: "NSGo", Role: core.RoleHeader, TestID: "welcome-name", MaxLines: 1,
						StyleFunc: func() core.Style {
							size := core.Points(64)
							if compact.Get() {
								size = 48
							}
							return core.Style{FontSize: size, FontWeight: core.FontWeightBold, LetterSpacing: -2, Color: t.ink}
						}},
					core.Text{Content: "Native UI, written in Go.", Style: core.Style{FontSize: 20, FontWeight: core.FontWeightSemibold,
						TextAlign: core.TextAlignCenter, Color: t.ink}},
					core.Text{Content: "A cross-platform UI engine that draws with each platform's own views, after the design of NSH. " +
						"Try every element and component here, theme them and see how they feel.",
						Style: core.Style{MaxWidth: core.Pt(520), LineHeight: 22, TextAlign: core.TextAlignCenter, Color: t.muted}},
				}}),
				enter(2, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyCenter, Gap: 12},
					Children: []core.Node{
						ui.Button{Variant: ui.Secondary, OnPress: copyLink, Label: "Copy the repository link", TestID: "welcome-repository",
							Style: pill.Merge(core.Style{Background: withAlpha(t.surface, 0.8)}),
							Children: []core.Node{
								icon(lucide.GitBranch(), 16),
								core.Text{Content: repository, MaxLines: 1, Style: core.Style{FlexShrink: 1}},
								wide(core.Icon{SVG: lucide.Copy(), Style: core.Style{Width: core.Pt(14), Height: core.Pt(14), Color: t.muted}}),
							}},
						ui.Button{OnPress: g.openPalette, Label: "Search everything", TestID: "welcome-search", Style: pill,
							Children: []core.Node{
								icon(lucide.Search(), 16),
								core.Text{Content: "Search everything", MaxLines: 1, Style: core.Style{FlexShrink: 1}},
								wide(core.Text{Content: shortcut(cx, "K"), Style: core.Style{FontSize: 12, Opacity: core.Alpha(0.7)}}),
							}},
					}}),
				enter(3, core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, JustifyContent: core.JustifyCenter, Gap: 8},
					Children: badges}),
				enter(4, core.Text{Content: "Built with NSGo · Ported from NSH · Written in Go",
					Style: core.Style{FontSize: 13, TextAlign: core.TextAlignCenter, Color: t.muted}}),
			}},
		}}
}

// entrance returns what fades and lifts a part of the page into place
// once, staggered by its order. The platform runs the transitions; under
// reduced motion the parts show at once.
func entrance(cx *core.Scope) func(order int, n core.Node) core.Node {
	reduce := core.Untracked(cx, cx.Env().ReduceMotion)
	return func(order int, n core.Node) core.Node {
		shown := core.NewSignal(cx, reduce)
		if !reduce {
			// The first commit places the part hidden, so the change animates.
			cx.AfterFunc(time.Duration(order)*70*time.Millisecond+16*time.Millisecond, func() { shown.Set(true) })
		}
		return core.View{Children: []core.Node{n}, StyleFunc: func() core.Style {
			s := core.Style{AlignSelf: core.AlignStretch, AlignItems: core.AlignCenter,
				Transition: core.Transition{Duration: 560 * time.Millisecond, Easing: core.EaseOut}}
			if !shown.Get() {
				s.Opacity, s.Transform = core.Alpha(0), core.Transform{TranslateY: 12}
			}
			return s
		}}
	}
}

// brandMark redraws the app icon at size, so it stays sharp: the
// gradient tile, the arch and the dot.
func brandMark(t *theme, size core.Points) core.Node {
	top, bottom, dot := brandTeal, brandGreen, brandYellow
	if t.dark {
		top, bottom, dot = core.RGB(0x0b4f58), core.RGB(0x123f2d), core.RGB(0xeaf7ef)
	}
	s := float32(size)
	return core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size * 0.23,
		Background: bottom, ShadowColor: withAlpha(core.RGB(0x1f6f6a), 0.35), ShadowRadius: size * 0.2, ShadowOffset: core.Point{Y: size * 0.06}},
		Children: []core.Node{core.Canvas{Label: "NSGo", Style: core.Style{Width: core.Pt(size), Height: core.Pt(size)},
			Draw: func(d *core.Drawing, _ core.Size) {
				d.Save().BeginPath().RoundRect(0, 0, s, s, s*0.23).Clip()
				// Bands blend the gradient, which canvases do not draw.
				const bands = 32
				for i := range bands {
					d.SetFillColor(ui.Mix(top, bottom, float64(i)/(bands-1))).FillRect(0, s*float32(i)/bands, s, s/bands+1)
				}
				d.Restore()
				// The arch: two legs joined by a half circle.
				w, r := s*0.12, s*0.2
				cx, cy := s/2, s*0.42
				d.SetStrokeColor(core.RGB(0xffffff)).SetLineWidth(w).SetLineCap(core.CapRound).
					BeginPath().MoveTo(cx-r, s*0.74).LineTo(cx-r, cy).Arc(cx, cy, r, math.Pi, 0, false).LineTo(cx+r, s*0.74).Stroke()
				d.SetFillColor(dot).BeginPath().Ellipse(cx, s*0.68, s*0.075, s*0.075).Fill()
			}}}}
}

// welcomeArt paints the page's backdrop: glows in the icon's teal and
// green from opposite corners, and a tilted lattice of icons tinted from
// one to the other, fading around the center so the title stays legible.
func welcomeArt(t *theme) core.Node {
	strength, alpha := 1.0, 0.13
	if t.dark {
		strength, alpha = 0.7, 0.16
	}
	return core.Canvas{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0)},
		Draw: func(d *core.Drawing, size core.Size) {
			w, h := float32(size.Width), float32(size.Height)
			reach := max(w, h)
			glow(d, w, 0, reach*0.45, brandTeal, 0.16*strength)
			glow(d, 0, h, reach*0.45, brandGreen, 0.14*strength)

			icons := welcomeIcons()
			const step, side = 56, 20
			angle := -14 * math.Pi / 180
			sin, cos := float32(math.Sin(angle)), float32(math.Cos(angle))
			half := float32(math.Hypot(float64(w), float64(h))) / 2
			d.Save().Translate(w/2, h/2).Rotate(float32(angle))
			for row := 0; float32(row)*step < 2*half+step; row++ {
				y := -half + float32(row)*step
				for col := 0; float32(col)*step < 2*half+step; col++ {
					x := -half + float32(col)*step - float32(row%2)*step/2
					// Skip what the rotation leaves outside the page.
					sx, sy := w/2+x*cos-y*sin, h/2+x*sin+y*cos
					if sx < -side || sy < -side || sx > w+side || sy > h+side {
						continue
					}
					diagonal := min(1, max(0, sx/w*0.5+(1-sy/h)*0.5))
					center := min(1, max(0.25, float32(math.Hypot(float64(sx-w/2), float64(sy-h/2)))/half*1.4))
					a := alpha * float64(center) * (0.45 + float64(diagonal)*0.55)
					color := withAlpha(ui.Mix(brandGreen, brandTeal, float64(diagonal)), float32(a))
					d.Save().Translate(x-side/2, y-side/2)
					icons[(row*7+col*3)%len(icons)].Draw(d, core.Size{Width: side, Height: side}, color)
					d.Restore()
				}
			}
			d.Restore()
		}}
}

// glow paints a soft light at x, y: stacked discs whose alphas add up to
// alpha at the center and fade to nothing at radius, as a radial gradient
// would.
func glow(d *core.Drawing, x, y, radius float32, c core.Color, alpha float64) {
	const discs = 24
	each := 1 - math.Pow(1-alpha, 1.0/discs)
	d.SetFillColor(ui.Fade(c, each))
	for i := discs; i > 0; i-- {
		r := radius * float32(i) / discs
		d.BeginPath().Ellipse(x, y, r, r).Fill()
	}
}

// welcomeIcons samples the catalog sparsely, so the field parses a few
// dozen icons once rather than the whole pack.
var welcomeIcons = sync.OnceValue(func() []*core.SVG {
	icons := make([]*core.SVG, 48)
	for i := range icons {
		icons[i] = iconCatalog[(i*97+13)%len(iconCatalog)].get()
	}
	return icons
})
