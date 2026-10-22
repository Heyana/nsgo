package main

import (
	"fmt"
	"math"
	"time"

	"github.com/oneo-me/nsgo/core"
)

func animationPage(cx *core.Scope, t *theme) core.Node {
	progress := core.NewSignal(cx, 0.0)
	var stop func()
	run := func() {
		if stop != nil {
			stop()
		}
		from := progress.GetUntracked()
		to := 1.0
		if from >= 1 {
			to = 0
		}
		stop = cx.Animate(1200*time.Millisecond, core.EaseInOut, func(p float64) { progress.Set(from + (to-from)*p) })
	}

	moved := core.NewSignal(cx, false)
	ease := core.Transition{Duration: 450 * time.Millisecond, Easing: core.EaseInOut}
	curves := []struct {
		name  string
		curve core.Curve
	}{{"Linear", core.Linear}, {"Ease", core.Ease}, {"EaseIn", core.EaseIn}, {"EaseOut", core.EaseOut}, {"EaseInOut", core.EaseInOut}}
	var lanes []core.Node
	for _, c := range curves {
		lanes = append(lanes, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			core.Text{Content: c.name, Style: t.caption().Merge(core.Style{Width: core.Pt(72)})},
			core.View{Style: core.Style{FlexGrow: 1, Height: core.Pt(20), Background: t.sunken, CornerRadius: 10}, Children: []core.Node{
				core.View{StyleFunc: func() core.Style {
					s := core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10, Background: t.accent,
						Transition: core.Transition{Duration: 900 * time.Millisecond, Easing: c.curve, Properties: core.TransitionTransform}}
					if moved.Get() {
						s.Transform.TranslateX = 200
					}
					return s
				}},
			}},
		}})
	}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Animate", "Scope.Animate steps a value once a frame on the engine, for what the platform cannot animate itself.",
			core.View{Style: core.Style{Height: core.Pt(10), Background: t.sunken, CornerRadius: 5, Overflow: core.Hidden}, Children: []core.Node{
				core.View{StyleFunc: func() core.Style {
					return core.Style{Height: core.Pt(10), Width: core.Pct(float32(progress.Get() * 100)), Background: t.accent}
				}},
			}},
			hstack(
				t.primary("Run", run),
				core.Text{ContentFunc: func() string { return fmt.Sprintf("%3.0f%%", progress.Get()*100) },
					Style: core.Style{FontFamily: "Menlo", FontSize: 15}},
			),
		),
		t.demo("Transitions", "A style Transition runs on the platform: no engine work per frame.",
			core.View{StyleFunc: func() core.Style {
				s := core.Style{Width: core.Pt(72), Height: core.Pt(72), CornerRadius: 12, Background: t.accent, Transition: ease,
					ShadowColor: core.RGBA(0, 0.15), ShadowRadius: 4, ShadowOffset: core.Point{Y: 2}}
				if moved.Get() {
					s.Background, s.CornerRadius = t.success, 36
					s.Transform = core.Transform{TranslateX: 160, Rotate: 180, Scale: 0.8}
					s.Opacity = core.Alpha(0.7)
					s.ShadowRadius, s.ShadowOffset = 18, core.Point{Y: 10}
				}
				return s
			}},
			t.note("The same move with each easing curve:"),
			core.View{Style: core.Style{Gap: 6}, Children: lanes},
			t.primary("Toggle", func() { moved.Update(func(m bool) bool { return !m }) }),
		),
	}}
}

func gesturesPage(cx *core.Scope, t *theme) core.Node {
	// The card follows the pan, then springs back with a transition.
	drag := core.NewSignal(cx, core.PanEvent{})
	dragging := func() bool { p := drag.Get().Phase; return p == core.GestureBegan || p == core.GestureChanged }

	// The slider's value from 0 to 1, the value a pan began at and the
	// track's width.
	value := core.NewSignal(cx, float32(0.3))
	var start float32
	track := core.NewSignal[core.Points](cx, 1)
	const thumb core.Points = 24

	// The target keeps its zoom and turn between gestures: each one
	// multiplies or adds to what the last left.
	pinch := core.NewSignal(cx, core.GestureEvent{Value: 1})
	rotation := core.NewSignal(cx, core.GestureEvent{})
	zoom := core.NewSignal(cx, float32(1))
	turn := core.NewSignal(cx, float32(0))
	var zoomStart, turnStart float32
	onPinch := func(e core.GestureEvent) {
		if e.Phase == core.GestureBegan {
			zoomStart = zoom.GetUntracked()
		}
		pinch.Set(e)
		zoom.Set(min(max(zoomStart*e.Value, 0.4), 3))
	}
	onRotation := func(e core.GestureEvent) {
		if e.Phase == core.GestureBegan {
			turnStart = turn.GetUntracked()
		}
		rotation.Set(e)
		turn.Set(turnStart + e.Value)
	}
	degrees := func(radians float32) float32 { return radians * 180 / math.Pi }
	phase := func(p core.GesturePhase) string {
		return [...]string{"idle", "began", "changed", "ended", "cancelled"}[p]
	}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Pan", "View.OnPan follows a drag; the card moves by its Transform and snaps back when the pan ends.",
			core.View{Style: core.Style{Height: core.Pt(180), Background: t.background, CornerRadius: radius,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
				core.View{TestID: "drag-card", Label: "Draggable card",
					OnPan: drag.Set,
					StyleFunc: func() core.Style {
						p := drag.Get()
						s := core.Style{Width: core.Pt(140), Height: core.Pt(90), CornerRadius: cardRadius, Background: t.accent,
							AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
							ShadowColor: core.RGBA(0, 0.2), ShadowRadius: 6, ShadowOffset: core.Point{Y: 3}}
						if dragging() {
							s.Transform = core.Transform{TranslateX: p.Translation.X, TranslateY: p.Translation.Y, Scale: 1.05}
							s.ShadowRadius, s.ShadowOffset = 18, core.Point{Y: 10}
						} else {
							s.Transition = core.Transition{Duration: 300 * time.Millisecond, Easing: core.EaseOut}
						}
						return s
					},
					Children: []core.Node{core.Text{Content: "Drag me", Style: core.Style{Color: t.onAccent, FontWeight: core.FontWeightSemibold}}}},
			}},
			core.Text{TestID: "pan-status", Style: t.caption(), ContentFunc: func() string {
				p := drag.Get()
				return fmt.Sprintf("%s · translation %.0f, %.0f · velocity %.0f, %.0f pt/s", phase(p.Phase),
					p.Translation.X, p.Translation.Y, p.Velocity.X, p.Velocity.Y)
			}},
		),
		t.demo("Slider in a scroll view", "PanActivation{ActiveOffsetX: 10, FailOffsetY: 10} takes horizontal drags and leaves vertical ones to scrolling.",
			core.View{TestID: "slider", Role: core.RoleNone, Label: "Volume",
				PanActivation: core.PanActivation{ActiveOffsetX: 10, FailOffsetY: 10},
				OnLayout:      func(b core.Bounds) { track.Set(max(b.Size.Width-thumb, 1)) },
				OnPan: func(e core.PanEvent) {
					if e.Phase == core.GestureBegan {
						start = value.GetUntracked()
					}
					value.Set(min(max(start+float32(e.Translation.X/track.GetUntracked()), 0), 1))
				},
				Style: core.Style{Height: core.Pt(32), JustifyContent: core.JustifyCenter},
				Children: []core.Node{
					core.View{Style: core.Style{Height: core.Pt(6), CornerRadius: 3, Background: t.sunken, Margin: core.Symmetric(0, thumb/2)},
						Children: []core.Node{core.View{StyleFunc: func() core.Style {
							return core.Style{Height: core.Pt(6), CornerRadius: 3, Background: t.accent, Width: core.Pct(value.Get() * 100)}
						}}}},
					core.View{StyleFunc: func() core.Style {
						return core.Style{Position: core.Absolute, Top: core.Pt(4), Left: core.Pt(core.Points(value.Get()) * track.Get()),
							Width: core.Pt(thumb), Height: core.Pt(thumb), CornerRadius: thumb / 2, Background: core.RGB(0xffffff),
							BorderWidth: 1, BorderColor: t.line, ShadowColor: core.RGBA(0, 0.2), ShadowRadius: 3, ShadowOffset: core.Point{Y: 1}}
					}},
				}},
			core.Text{TestID: "slider-value", Style: t.caption(), ContentFunc: func() string { return fmt.Sprintf("Volume %.0f%%", value.Get()*100) }},
		),
		t.demo("Pinch and rotate", "OnPinch reports a scale, 1 when it begins; OnRotation an angle in radians. Use two fingers or a trackpad.",
			core.View{Style: core.Style{Height: core.Pt(200), Background: t.background, CornerRadius: radius,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Overflow: core.Hidden}, Children: []core.Node{
				core.View{TestID: "pinch-target", Label: "Pinch and rotate target",
					OnPinch: onPinch, OnRotation: onRotation,
					StyleFunc: func() core.Style {
						return core.Style{Width: core.Pt(110), Height: core.Pt(110), CornerRadius: 18, Background: t.success,
							AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
							Transform: core.Transform{Scale: zoom.Get(), Rotate: degrees(turn.Get())}}
					},
					Children: []core.Node{core.Text{Content: "Pinch me", Style: core.Style{Color: t.onAccent, FontWeight: core.FontWeightSemibold}}}},
			}},
			core.Text{TestID: "pinch-status", Style: t.caption(), ContentFunc: func() string {
				p, r := pinch.Get(), rotation.Get()
				return fmt.Sprintf("pinch %.2f (%s) · rotation %.0f° (%s) · zoom %.2f, turned %.0f°",
					p.Value, phase(p.Phase), degrees(r.Value), phase(r.Phase), zoom.Get(), degrees(turn.Get()))
			}},
			t.plain("Reset", func() { zoom.Set(1); turn.Set(0) }),
		),
	}}
}
