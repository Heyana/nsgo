package main

import (
	"fmt"
	"math"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// dots is a dot matrix behind a stage, drawn once.
func dots(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	c := colors.line
	return core.Canvas{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)},
		Draw: func(d *core.Drawing, size core.Size) {
			d.SetFillColor(c)
			for x := float32(8); x < float32(size.Width); x += 14 {
				for y := float32(8); y < float32(size.Height); y += 14 {
					d.FillRect(x, y, 1.5, 1.5)
				}
			}
		}}
}

// stage is the darker surface a demo plays on.
func stage(cx *core.Scope, height core.Points) core.Style {
	colors := colorsOf(cx)
	fin := finOf(cx)
	return core.Style{Height: core.Pt(height), CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden,
		Background: colors.background, BorderWidth: 1, BorderColor: colors.line, Shadows: fin.track.shadows}
}

// knobShadows are the highlight and shadow of a small colored knob.
func knobShadows(cx *core.Scope) []core.BoxShadow {
	fin := finOf(cx)
	isDark := cx.Dark()
	if isDark {
		return []core.BoxShadow{inner(0, 1, 0, white(0.35)), shadow(0, 3, 10, 0, black(0.45))}
	}
	return fin.knob.shadows
}

// apiLine is a plain label with the API it uses, in code type.
func apiLine(cx *core.Scope, label, names string) core.Node {
	colors := colorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
		core.Text{Content: label, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, Color: colors.muted}},
		core.Text{Content: names, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.faint}},
	}}
}

// transformable is one photo dragged, pinched and rotated at the same
// time, all on the platform's main thread.
func transformable(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	x, y := core.NewAnimatedValue(cx, 0), core.NewAnimatedValue(cx, 0)
	scale, angle := core.NewAnimatedValue(cx, 1), core.NewAnimatedValue(cx, 0)
	type result struct {
		scale, angle float32
		drag         int
	}
	last := core.NewSignal(cx, result{scale: 1})
	reset := func() {
		cx.StartAnimation(core.Parallel(core.Spring(x, 0, core.SpringConfig{}), core.Spring(y, 0, core.SpringConfig{}),
			core.Spring(scale, 1, core.SpringConfig{}), core.Spring(angle, 0, core.SpringConfig{})), nil)
		last.Set(result{scale: 1})
	}
	degrees := core.Mul(angle, core.Constant(180/math.Pi))
	return card(cx, cardProps{icon: iconGestures, title: "Pan, pinch and rotation", subtitle: "Three recognizers on one view, driven natively"},
		core.View{Style: stage(cx, 260).Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), Children: []core.Node{
			dots(cx),
			core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Width: core.Pt(200), Height: core.Pt(150), CornerRadius: 18,
				BorderWidth: 1, BorderColor: colors.faint, BorderStyle: core.BorderDashed}},
			core.View{TestID: "photo", PanValues: core.AnimatedXY{X: x, Y: y}, PinchValue: scale, RotationValue: angle,
				OnPan: func(e core.PanEvent) {
					if e.Phase == core.GestureEnded {
						r := last.GetUntracked()
						r.drag = int(math.Round(math.Hypot(float64(e.Translation.X), float64(e.Translation.Y))))
						last.Set(r)
					}
				},
				OnPinch: func(e core.GestureEvent) {
					if e.Phase == core.GestureEnded {
						r := last.GetUntracked()
						r.scale = e.Value
						last.Set(r)
					}
				},
				OnRotation: func(e core.GestureEvent) {
					if e.Phase == core.GestureEnded {
						r := last.GetUntracked()
						r.angle = e.Value * 180 / math.Pi
						last.Set(r)
					}
				},
				Style: core.Style{Width: core.Pt(200), Height: core.Pt(150), CornerRadius: 18, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden,
					BorderWidth: 1, BorderColor: colors.border, Background: colors.raised,
					Animated: &core.AnimatedStyle{TranslateX: x, TranslateY: y, Scale: scale, Rotate: degrees}},
				Children: []core.Node{core.Image{Source: core.Src("https://picsum.photos/seed/nsh-card/400/300"), Style: core.Style{FlexGrow: 1}}}},
		}},
		core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
			stat(cx, "Drag", func() string { return fmt.Sprintf("%d pt", last.Get().drag) }, toneNeutral, core.Pct(33)),
			stat(cx, "Scale", func() string { return fmt.Sprintf("%.2f×", last.Get().scale) }, toneAccent, core.Pct(33)),
			stat(cx, "Angle", func() string { return fmt.Sprintf("%d°", int(math.Round(float64(last.Get().angle)))) }, toneInfo, core.Pct(34)),
		}},
		note(cx, hint(cx, "Pinch and rotate with two fingers (Option-drag in the simulator).", "Drag with the mouse; pinch and rotate on a touchpad or a touch screen.")),
		button(buttonProps{label: "Reset", variant: secondary}, reset),
	)
}

// ball is a ball dragged as the touch responder (positions go through the
// engine). Its responder keeps the page from scrolling while it is
// dragged.
func ball(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	pos := core.NewSignal(cx, core.Point{})
	type state struct {
		label string
		tone  tone
	}
	st := core.NewSignal(cx, state{"Idle", toneNeutral})
	grab := core.NewAnimatedValue(cx, 0)
	var start core.Point
	return card(cx, cardProps{icon: iconScroll, title: "Touch responder", subtitle: "Positions go through the engine; the page does not scroll",
		right: core.Dynamic(func(*core.Scope) core.Node { s := st.Get(); return badge(cx, s.label, s.tone) })},
		core.View{Style: stage(cx, 200), Children: []core.Node{
			dots(cx),
			core.View{TestID: "ball", Responder: &core.Responder{
				Claims: []core.TouchClaim{{}},
				OnGrant: func(core.ResponderEvent) {
					start = pos.GetUntracked()
					spring(cx, grab, 1, core.SpringConfig{Stiffness: 420, Damping: 24})
					st.Set(state{"Dragging", toneAccent})
				},
				OnMove: func(e core.ResponderEvent) {
					pos.Set(core.Point{X: start.X + e.Gesture.Delta.X, Y: start.Y + e.Gesture.Delta.Y})
				},
				OnRelease: func(e core.ResponderEvent) {
					spring(cx, grab, 0, core.SpringConfig{Stiffness: 300, Damping: 20})
					v := e.Gesture.Velocity
					st.Set(state{fmt.Sprintf("%.2f, %.2f pt/ms", v.X/1000, v.Y/1000), toneInfo})
				},
				OnTerminate: func(core.ResponderEvent) {
					spring(cx, grab, 0, core.SpringConfig{})
					st.Set(state{"Taken over", toneWarn})
				},
			}, StyleFunc: func() core.Style {
				p := pos.Get()
				return core.Style{Position: core.Absolute, Left: core.Pct(50), Top: core.Pt(70), Margin: core.Edges{Left: -30}, Width: core.Pt(60), Height: core.Pt(60),
					CornerRadius: 30, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: colors.info, Shadows: knobShadows(cx),
					Transform: core.Transform{TranslateX: p.X, TranslateY: p.Y},
					Animated: &core.AnimatedStyle{
						Background: core.Interpolate(grab, core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{colors.info, colors.accentFill}}),
						Scale:      lerp(grab, 1, 1.15),
					}}
			}, Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9, Background: core.RGBA(0x1e1408, 0.35)}}}},
		}},
		caption(cx, "Position", func() string { p := pos.Get(); return fmt.Sprintf("%.0f, %.0f", p.X, p.Y) }),
		button(buttonProps{label: "Center", variant: secondary}, func() { pos.Set(core.Point{}) }),
	)
}

type finger struct {
	id   int
	x, y core.Points
}

// touchPad is the raw touch events, every finger drawn where it is.
func touchPad(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	type entry struct{ name, text string }
	log := core.NewSignalFunc(cx, []entry(nil), slices.Equal)
	fingers := core.NewSignalFunc(cx, []finger(nil), slices.Equal)
	names := map[core.TouchPhase]string{core.TouchStart: "start", core.TouchMove: "move", core.TouchEnd: "end", core.TouchCancel: "cancel"}
	return card(cx, cardProps{icon: iconSystem, title: "Touch events", subtitle: hint(cx, "Start, move, end and cancel, for every finger", "Start, move, end and cancel, for the mouse and every finger"),
		right: core.Dynamic(func(*core.Scope) core.Node {
			n := len(fingers.Get())
			t := toneNeutral
			if n > 0 {
				t = toneAccent
			}
			return badge(cx, fmt.Sprintf("%d down", n), t)
		})},
		core.View{TestID: "touch-pad", OnTouch: func(e core.TouchEvent) {
			var down []finger
			var at core.Touch
			for _, t := range e.Touches {
				if t.Changed {
					at = t
				}
				if e.Phase == core.TouchCancel || (e.Phase == core.TouchEnd && t.Changed) {
					continue
				}
				down = append(down, finger{t.ID, t.Position.X, t.Position.Y})
			}
			fingers.Set(down)
			log.Update(func(l []entry) []entry {
				out := append([]entry{{names[e.Phase], fmt.Sprintf("%.0f, %.0f · page %.0f, %.0f · %d down", at.Position.X, at.Position.Y, at.PagePosition.X, at.PagePosition.Y, len(down))}}, l...)
				return out[:min(len(out), 4)]
			})
		}, Style: stage(cx, 150).Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), Children: []core.Node{
			dots(cx),
			core.Show{When: func() bool { return len(fingers.Get()) == 0 }, Then: func(*core.Scope) core.Node {
				return text(hint(cx, "Touch here, try two fingers", "Press and drag here with the mouse"), 14, regular, colors.muted)
			}},
			core.Each(fingers.Get, func(f finger) int { return f.id }, func(cx *core.Scope, f func() finger, _ func() int) core.Node {
				colors := colorsOf(cx)
				tints := tintsOf(cx)
				return core.View{PointerEvents: core.PointerEventsNone, StyleFunc: func() core.Style {
					return core.Style{Position: core.Absolute, Left: core.Pt(f().x - 26), Top: core.Pt(f().y - 26), Width: core.Pt(52), Height: core.Pt(52), CornerRadius: 26,
						BorderWidth: 2, BorderColor: colors.accent, Background: tints.accent}
				}}
			}),
		}},
		core.View{Style: core.Style{Gap: 4, MinHeight: core.Pt(72)}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			l := log.Get()
			if len(l) == 0 {
				return text("No touches yet", 13, regular, colors.faint)
			}
			var rows []core.Node
			for i, e := range l {
				c, n := colors.faint, colors.faint
				if i == 0 {
					c, n = colors.text, colors.accent
				}
				rows = append(rows, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: []core.Node{
					core.Text{Content: e.name, Style: core.Style{Width: core.Pt(52), FontSize: 12, FontFamily: mono, Color: n}},
					core.Text{Content: e.text, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, Color: c}},
				}})
			}
			return core.Group(rows)
		})}},
	)
}

// pressEvents lights each press event's chip as it arrives.
func pressEvents(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	names := []string{"pressIn", "press", "longPress", "pressOut"}
	flashes := make([]*core.AnimatedValue, len(names))
	for i := range flashes {
		flashes[i] = core.NewAnimatedValue(cx, 0)
	}
	n := core.NewSignal(cx, 0)
	log := core.NewSignalFunc(cx, []string(nil), slices.Equal)
	fire := func(name string) {
		n.Update(func(c int) int { return c + 1 })
		log.Update(func(l []string) []string {
			out := append([]string{name}, l...)
			return out[:min(len(out), 6)]
		})
		if i := slices.Index(names, name); i >= 0 {
			flashes[i].Set(1)
			timing(cx, flashes[i], 0, core.TimingConfig{Duration: 700 * time.Millisecond, Easing: core.Easing{Kind: core.EasingQuad, Mode: core.EaseModeOut}})
		}
	}
	var chips []core.Node
	for i, name := range names {
		chips = append(chips, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Height: core.Pt(30), CornerRadius: 15, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			BorderWidth: 1, BorderColor: colors.border, Background: colors.raised, Shadows: fin.raised.shadows,
			Animated: &core.AnimatedStyle{Background: core.Interpolate(flashes[i], core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{colors.raised, colors.accentFill}})}},
			Children: []core.Node{core.Text{Content: name, Style: core.Style{FontSize: 11, FontFamily: mono, Color: colors.text}}}})
	}
	return card(cx, cardProps{icon: iconComponents, title: "Press events", subtitle: "In, press, long press and out; native feedback",
		right: badgef(cx, func() string { return itoa(n.Get()) }, toneNeutral, false)},
		core.View{Role: core.RoleButton, Label: "Press events", TestID: "press-pad",
			OnPressDown: func(core.PressEvent) { fire("pressIn") }, OnPressUp: func(core.PressEvent) { fire("pressOut") },
			OnPress: func(core.PressEvent) { fire("press") }, OnLongPress: func(core.PressEvent) { fire("longPress") },
			Style: fin.light.over(core.Style{Height: core.Pt(64), CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, Background: colors.light,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.55)}}),
			Children: []core.Node{text(hint(cx, "Tap, hold, or slide off", "Click, hold, or slide off"), 15, semibold, colors.onLight)}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: chips},
		core.Text{ContentFunc: func() string {
			if l := log.Get(); len(l) > 0 {
				return strings.Join(l, " · ")
			}
			return "No events yet"
		}, MaxLines: 1, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.faint}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 10}, Children: []core.Node{
			core.View{Role: core.RoleButton, Label: "Ripple", OnPress: func(core.PressEvent) { fire("ripple") }, OnLongPress: func(core.PressEvent) { fire("fast long press") },
				DelayLongPress: 250 * time.Millisecond, Android: core.AndroidViewProps{Ripple: &core.Ripple{Color: alpha(colors.accentFill, 0.25)}},
				Style: fin.raised.over(core.Style{FlexGrow: 1, Height: core.Pt(56), CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, Background: colors.raised,
					BorderWidth: 1, BorderColor: colors.border, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 2, Active: &core.Style{Background: colors.border}}),
				Children: []core.Node{text("Ripple", 14, regular, colors.text), text("Long press after 250 ms", 12, regular, colors.faint)}},
			core.View{Role: core.RoleButton, Label: "Star", OnPress: func(core.PressEvent) { fire("borderless") },
				Android: core.AndroidViewProps{Ripple: &core.Ripple{Borderless: true, Radius: 40}},
				Style:    core.Style{Width: core.Pt(56), Height: core.Pt(56), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.4)}},
				Children: []core.Node{icon(iconStar, 24, colors.warn)}},
		}},
		core.Text{Content: "Pressable text, dimmed while pressed", OnPress: func(core.PressEvent) { fire("text") }, Role: core.RoleLink,
			Style: core.Style{AlignSelf: core.AlignStart, FontSize: 15, Color: colors.accent, Padding: core.Symmetric(12, 0)}},
	)
}

// hitSlop is a tiny dot with a ring showing the 30 pt around it that
// also takes taps.
func hitSlop(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	tints := tintsOf(cx)
	taps := core.NewSignal(cx, 0)
	ring := core.NewAnimatedValue(cx, 0)
	sine := core.Easing{Kind: core.EasingSin, Mode: core.EaseModeInOut}
	run := cx.StartAnimation(core.Loop(core.Sequence(
		core.Timing(ring, 1, core.TimingConfig{Duration: 1400 * time.Millisecond, Easing: sine}),
		core.Timing(ring, 0, core.TimingConfig{Duration: 1400 * time.Millisecond, Easing: sine}),
	), 0), nil)
	cx.Cleanup(run.Stop)
	return card(cx, cardProps{icon: iconSearch, title: "Hit slop", subtitle: hint(cx, "A 12 pt dot that takes taps 30 pt around it", "A 12 pt dot that takes clicks 30 pt around it"),
		right: core.Dynamic(func(*core.Scope) core.Node {
			t := toneNeutral
			if taps.Get() > 0 {
				t = toneAccent
			}
			return badge(cx, fmt.Sprintf("%d taps", taps.Get()), t)
		})},
		core.View{Style: stage(cx, 140).Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), Children: []core.Node{
			dots(cx),
			core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Width: core.Pt(72), Height: core.Pt(72), CornerRadius: 36,
				BorderWidth: 1, BorderStyle: core.BorderDashed, BorderColor: colors.danger, Background: tints.danger,
				Animated: &core.AnimatedStyle{Opacity: lerp(ring, 0.45, 1), Scale: lerp(ring, 0.96, 1.04)}}},
			press(pressProps{hitSlop: 30, scale: 0.8, onPress: func() { taps.Update(func(n int) int { return n + 1 }) }, label: "Tiny dot"},
				core.View{Style: core.Style{Width: core.Pt(12), Height: core.Pt(12), CornerRadius: 6, Background: colors.danger, Shadows: knobShadows(cx)}}),
		}},
		apiLine(cx, hint(cx, "The dashed ring is the touch area", "The dashed ring is the click area"), "HitSlop: All(30)"),
	)
}

// pointerEventsCard is an overlay that lets taps through to the button
// under it.
func pointerEventsCard(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	tints := tintsOf(cx)
	through := core.NewSignal(cx, 0)
	warn, hatch := colors.warn, alpha(colors.warn, 0.28)
	return card(cx, cardProps{icon: iconNav, title: "Pointer events", subtitle: hint(cx, "An overlay that lets taps through", "An overlay that lets clicks through"),
		right: badgef(cx, func() string { return itoa(through.Get()) }, toneUp, false)},
		core.View{Style: core.Style{Height: core.Pt(88)}, Children: []core.Node{
			press(pressProps{onPress: func() { through.Update(func(n int) int { return n + 1 }) }, scale: 0.98, label: "Button under an overlay", style: core.Style{FlexGrow: 1}},
				core.View{Style: core.Style{FlexGrow: 1, CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, Background: tints.up, BorderWidth: 1, BorderColor: colors.up,
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{text("Button under an overlay", 15, semibold, colors.up)}}),
			core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)},
				Children: []core.Node{core.Canvas{Style: core.Style{FlexGrow: 1}, Draw: func(d *core.Drawing, size core.Size) {
					w, h := float32(size.Width), float32(size.Height)
					// A hatched band across the middle, framed with dashes.
					d.Save().BeginPath().Rect(w*0.18, 0, w*0.64, h).Clip()
					d.SetStrokeColor(hatch).SetLineWidth(1).BeginPath()
					for i := -h; i < w; i += 8 {
						d.MoveTo(i, h).LineTo(i+h, 0)
					}
					d.Stroke().Restore()
					d.SetLineDash(4, 4).SetStrokeColor(warn).BeginPath().RoundRect(w*0.18+0.5, 0.5, w*0.64-1, h-1, 10).Stroke()
				}}}},
		}},
		apiLine(cx, hint(cx, "Taps on the hatched band reach the button", "Clicks on the hatched band reach the button"), "PointerEventsNone"),
	)
}

// hover is the mouse over a view: it comes in, moves and leaves.
func hover(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	over := core.NewSignal(cx, false)
	at := core.NewSignal(cx, core.Point{})
	return card(cx, cardProps{icon: iconGestures, title: "Hover", subtitle: hint(cx, "A pointer over a view: iPad, desktops", "The mouse over a view"),
		right: core.Dynamic(func(*core.Scope) core.Node {
			if over.Get() {
				return badge(cx, "Over", toneAccent)
			}
			return badge(cx, "Away", toneNeutral)
		})},
		core.View{TestID: "hover-pad", OnHoverIn: func() { over.Set(true) }, OnHoverOut: func() { over.Set(false) },
			OnHoverMove: func(e core.HoverEvent) { at.Set(e.Position) },
			Style:       stage(cx, 100).Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Hover: &core.Style{Background: colors.surface}}),
			Children: []core.Node{textf(func() string {
				if !over.Get() {
					return "Pointer away"
				}
				p := at.Get()
				return fmt.Sprintf("Pointer at %.0f, %.0f", p.X, p.Y)
			}, 14, regular, colors.muted)}},
		apiLine(cx, "In, move and out", "OnHoverIn · OnHoverMove · OnHoverOut"),
	)
}

// fileDrop is files dragged in from other apps, on desktops and the web.
func fileDrop(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	tints := tintsOf(cx)
	over := core.NewSignal(cx, false)
	dropped := core.NewSignalFunc(cx, []string(nil), slices.Equal)
	return card(cx, cardProps{icon: iconFile, title: "Dropped files", subtitle: "Files dragged in from other apps, as file: URLs",
		right: badgef(cx, func() string { return count(len(dropped.Get()), "file") }, toneNeutral, false)},
		core.View{TestID: "drop-zone", Label: "Drop files here",
			OnFileDragEnter: func() { over.Set(true) }, OnFileDragLeave: func() { over.Set(false) },
			OnFileDrop: func(uris []string) { over.Set(false); dropped.Set(uris) },
			StyleFunc: func() core.Style {
				s := stage(cx, 120).Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, BorderWidth: 2, BorderStyle: core.BorderDashed})
				if over.Get() {
					s.BorderColor, s.Background = colors.accent, tints.accent
				}
				return s
			},
			Children: []core.Node{textf(func() string {
				if over.Get() {
					return "Release to drop"
				}
				return hint(cx, "Desktops and the web take dropped files", "Drop files here")
			}, 14, regular, colors.muted)}},
		core.Text{TestID: "dropped", ContentFunc: func() string {
			uris := dropped.Get()
			if len(uris) == 0 {
				return "Nothing dropped yet"
			}
			names := make([]string, len(uris))
			for i, u := range uris {
				names[i] = fileName(u)
			}
			return strings.Join(names, ", ")
		}, Style: core.Style{FontSize: 13, Color: colors.muted}},
		apiLine(cx, "On the web, blob: URLs named by their fragment", "OnFileDrop"),
	)
}

// fileName is the name a dropped file's URL gives it.
func fileName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	if u.Fragment != "" {
		return u.Fragment
	}
	return path.Base(u.Path)
}

// accessibleRows are elements labelled for VoiceOver and TalkBack.
func accessibleRows(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	last := core.NewSignal(cx, "")
	items := []struct {
		label, hint string
		icon        iconName
	}{{"Play", "Starts the song", iconPlay}, {"Like", "Adds to favorites", iconStar}, {"Notify", "Turns on alerts", iconBell}}
	var rows []core.Node
	for i, it := range items {
		rows = append(rows, core.View{Accessible: true, Role: core.RoleButton, Label: it.label, Hint: it.hint, OnPress: func(core.PressEvent) { last.Set(it.label) },
			Children: []core.Node{listRow(cx, i == 0, nil,
				core.Dynamic(func(*core.Scope) core.Node {
					if last.Get() == it.label {
						return core.View{Style: fin.raised.over(core.Style{Width: core.Pt(34), Height: core.Pt(34), CornerRadius: 10, BorderWidth: 1, BorderColor: colors.border,
							Background: colors.raised, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), Children: []core.Node{icon(it.icon, 17, colors.accent)}}
					}
					return iconTile(cx, it.icon, 34)
				}),
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 1}, Children: []core.Node{text(it.label, 15, regular, colors.text), text(it.hint, 12, regular, colors.muted)}},
				core.Text{Content: "button", Style: core.Style{FontSize: 11, FontFamily: mono, Color: colors.faint}},
			)}})
	}
	return card(cx, cardProps{icon: iconText, title: "Accessibility", subtitle: "Roles, labels and hints for screen readers",
		right: core.Dynamic(func(*core.Scope) core.Node {
			if l := last.Get(); l != "" {
				return badge(cx, l, toneInfo)
			}
			return core.Group{}
		})}, rows...)
}

// gesturesDemo is the platform's gestures: pans, pinches and rotations
// moving values on its main thread, the touch responder, raw touches,
// press events, hit slop, pointer events, hover and dropped files.
func gesturesDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, transformable, ball, hover, touchPad, pressEvents, hitSlop, pointerEventsCard, fileDrop, accessibleRows)
}
