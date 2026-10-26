package main

import (
	"github.com/oneo-me/nsgo/core"
)

// livePulse is a pulse that loops on the platform while running: 0 to 1
// over 1.4 s, for a ring that grows and fades around a live dot.
func livePulse(cx *core.Scope, running func() bool) *core.AnimatedValue {
	v := core.NewAnimatedValue(cx, 0)
	var run *core.AnimationRun
	cx.Effect(func() {
		if running() {
			if run == nil {
				run = cx.StartAnimation(core.Loop(core.Timing(v, 1, core.TimingConfig{Duration: ms(1400), Easing: easeOutCubic}), 0), nil)
			}
			return
		}
		if run != nil {
			run.Stop()
			run = nil
		}
		timing(cx, v, 0, core.TimingConfig{Duration: ms(200)})
	})
	return v
}

// liveDot is a dot with a ring that pulses while the market runs.
func liveDot(cx *core.Scope, running func() bool, size core.Points) core.Node {
	p := livePulse(cx, running)
	return core.View{Style: core.Style{Width: core.Pt(size * 2.5), Height: core.Pt(size * 2.5), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
		core.View{Style: core.Style{Position: core.Absolute, Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, Background: colors.accent,
			Animated: &core.AnimatedStyle{Opacity: lerp(p, 0.55, 0), Scale: lerp(p, 1, 2.5)}}},
		core.View{StyleFunc: func() core.Style {
			c := colors.faint
			if running() {
				c = colors.accent
			}
			return core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, Background: c}
		}},
	}}
}

// scrubbing makes a view a chart that scrubs horizontally: the platform
// gives it the finger only once it has moved sideways, so vertical drags
// still scroll the page. at gets the position along the chart, then -1
// when the finger lifts.
func scrubbing(v core.View, at func(x core.Points)) core.View {
	v.PanActivation = core.PanActivation{ActiveOffsetX: 6, FailOffsetY: 8}
	v.OnPan = func(core.PanEvent) {}
	v.Responder = &core.Responder{Claims: []core.TouchClaim{{OnPan: true}}, Keep: true,
		OnGrant:     func(e core.ResponderEvent) { at(e.Position.X) },
		OnMove:      func(e core.ResponderEvent) { at(e.Position.X) },
		OnRelease:   func(core.ResponderEvent) { at(-1) },
		OnTerminate: func(core.ResponderEvent) { at(-1) },
	}
	// Desktops: the mouse over the chart moves the crosshair too.
	v.OnHoverMove = func(e core.HoverEvent) { at(e.Position.X) }
	v.OnHoverOut = func() { at(-1) }
	return v
}

// stockHatch is diagonal stripes over a rectangle: projections, zones.
func stockHatch(d *core.Drawing, x, y, w, h float32, c core.Color, gap float32) {
	if w <= 0 || h <= 0 {
		return
	}
	d.Save().BeginPath().Rect(x, y, w, h).Clip()
	hatchClip(d, x, y, w, h, c, gap)
	d.Restore()
}

// hline is a dashed horizontal line.
func hline(d *core.Drawing, x0, x1, y float32, c core.Color, dash ...float32) {
	if len(dash) == 0 {
		dash = []float32{3, 4}
	}
	d.SetStrokeColor(c).SetLineWidth(1).SetLineDash(dash...).BeginPath().MoveTo(x0, y).LineTo(x1, y).Stroke().SetLineDash()
}

// gridColor is the charts' grid lines: the hairline color, a step darker
// on the light page.
func gridColor() core.Color {
	if isDark {
		return colors.line
	}
	return colors.border
}

// springBar is a bar that grows from one edge of its track to share
// (0 to 1) with a spring on the platform: the bar is the track's width
// and slides in, clipped.
func springBar(share func() float64, c core.Color, fromRight bool, height, radius core.Points) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		x := core.NewAnimatedValue(cx, 0)
		var width core.Points
		placed := false
		place := func() {
			if width == 0 {
				return
			}
			s := clamp(core.Untracked(cx, share), 0, 1)
			dir := -1.0
			if fromRight {
				dir = 1
			}
			to := dir * (1 - s) * float64(width)
			if placed {
				spring(cx, x, to, core.SpringConfig{Stiffness: 180, Damping: 22})
			} else {
				x.Set(to)
			}
			placed = true
		}
		cx.Effect(func() {
			share()
			cx.Untrack(place)
		})
		if radius < 0 {
			radius = height / 2
		}
		return core.View{OnLayout: func(b core.Bounds) { width = b.Size.Width; place() },
			Style: core.Style{Height: core.Pt(height), Overflow: core.Hidden, CornerRadius: radius}, Children: []core.Node{
				core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), CornerRadius: radius,
					Background: c, Animated: &core.AnimatedStyle{TranslateX: x}}},
			}}
	})
}

// calmRange is an axis range that follows its data calmly: it widens at
// once, so nothing is clipped, and narrows by rate of the gap per call,
// so a redraw never jumps the scale.
type calmRange struct {
	rate, lo, hi float64
	set          bool
}

func (c *calmRange) fit(l, h float64) (float64, float64) {
	if !c.set {
		c.lo, c.hi, c.set = l, h, true
	} else {
		if l < c.lo {
			c.lo = l
		} else {
			c.lo += (l - c.lo) * c.rate
		}
		if h > c.hi {
			c.hi = h
		} else {
			c.hi += (h - c.hi) * c.rate
		}
	}
	return c.lo, c.hi
}

func (c *calmRange) reset() { c.set = false }

// block is a flat section of a screen: a title with a faint note (or a
// control) on the right, the content below, a hairline above all but the
// first.
func block(title string, noteFn func() string, right core.Node, first bool, children ...core.Node) core.Node {
	s := core.Style{Padding: core.Symmetric(14, 16), Gap: 10}
	if !first {
		s.Borders, s.BorderColors = core.Edges{Top: 1}, core.EdgeColors{Top: colors.line}
	}
	var nodes []core.Node
	if title != "" {
		head := []core.Node{core.Text{Content: title, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 15, FontWeight: semibold, Color: colors.text}}}
		if noteFn != nil {
			head = append(head, textf(noteFn, 12, regular, colors.faint))
		}
		if right != nil {
			head = append(head, right)
		}
		nodes = append(nodes, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(22)}, Children: head})
	}
	return core.View{Style: s, Children: append(nodes, children...)}
}

// head is a quiet header for columns: SYMBOL, LAST.
func head(label string, width core.Points, grow float32, align core.TextAlign) core.Node {
	s := core.Style{FlexGrow: grow, FlexShrink: grow, TextAlign: align, FontSize: 11, Color: colors.faint, LetterSpacing: 0.6}
	if width > 0 {
		s.Width = core.Pt(width)
	}
	return core.Text{Content: label, Style: s}
}
