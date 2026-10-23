package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// nativeAnimationDemos are the demos of what the platform animates on its
// own frame clock: animated values, drags that move them and layout
// animations.
func nativeAnimationDemos(cx *core.Scope, t *theme) []core.Node {
	// A ball on a track, driven by one value from 0 to 1.
	pos := core.NewAnimatedValue(cx, 0)
	spin := core.NewAnimatedValue(cx, 0)
	state := core.NewSignal(cx, "idle")
	at := 0.0
	run := func(name string, a *core.Animation) {
		state.Set(name + "…")
		cx.StartAnimation(a, func(finished bool) {
			if finished {
				state.Set(name + " finished")
			} else {
				state.Set(name + " stopped")
			}
		})
	}
	toggle := func() float64 { at = 1 - at; return at }
	slide := core.Interpolate(pos, core.Interpolation{Input: []float64{0, 1}, Output: []float64{0, 1}})
	ball := core.View{TestID: "ball", Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), CornerRadius: 18,
		Animated: &core.AnimatedStyle{
			TranslateX: core.Mul(slide, core.Constant(220)),
			Rotate:     core.Interpolate(spin, core.Interpolation{Input: []float64{0, 1}, Output: []float64{0, 360}}),
			Background: core.Interpolate(pos, core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{t.accent, t.success}}),
			Scale: core.Interpolate(pos, core.Interpolation{Input: []float64{0, 0.5, 1}, Output: []float64{1, 1.4, 1},
				Easing: core.Easing{Kind: core.EasingSin, Mode: core.EaseModeInOut}}),
		}},
		Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4,
			Background: t.onAccent, Margin: core.All(5)}}}}

	// A card dragged by the platform, springing home when let go.
	dx, dy := core.NewAnimatedValue(cx, 0), core.NewAnimatedValue(cx, 0)
	home := func(e core.PanEvent) {
		if e.Phase == core.GestureEnded || e.Phase == core.GestureCancelled {
			cx.StartAnimation(core.Parallel(
				core.Spring(dx, 0, core.SpringConfig{Velocity: float64(e.Velocity.X), Damping: 12}),
				core.Spring(dy, 0, core.SpringConfig{Velocity: float64(e.Velocity.Y), Damping: 12}),
			), nil)
		}
	}
	lift := core.Interpolate(core.Add(dx, dy), core.Interpolation{Input: []float64{-200, 0, 200}, Output: []float64{-12, 0, 12}})

	// A list whose changes the platform animates.
	next := 4
	items := core.NewSignalFunc(cx, []int{1, 2, 3}, slices.Equal)
	wide := core.NewSignal(cx, false)
	animate := func(fn func()) { cx.AnimateLayout(core.LayoutSpringPreset); fn() }

	return []core.Node{
		t.demo("Animated values", "AnimatedValue moves on the platform's frame clock: no engine work per frame. Interpolate maps it to a position, a color, a scale and a turn.",
			core.View{Style: core.Style{Height: core.Pt(52), Background: t.background, CornerRadius: radius, Padding: core.All(8),
				JustifyContent: core.JustifyCenter}, Children: []core.Node{ball}},
			flow(
				t.primary("Timing", func() {
					run("Timing", core.Timing(pos, toggle(), core.TimingConfig{Duration: 600 * time.Millisecond,
						Easing: core.BezierEasing(core.EaseInOut)}))
				}),
				t.plain("Spring", func() { run("Spring", core.Spring(pos, toggle(), core.SpringConfig{Damping: 6})) }),
				t.plain("Bounce", func() {
					run("Bounce", core.Timing(pos, toggle(), core.TimingConfig{Duration: 900 * time.Millisecond,
						Easing: core.Easing{Kind: core.EasingBounce, Mode: core.EaseModeOut}}))
				}),
				t.plain("Sequence", func() {
					at = 0
					run("Sequence", core.Sequence(
						core.Timing(pos, 1, core.TimingConfig{Duration: 400 * time.Millisecond}),
						core.Timing(spin, 1, core.TimingConfig{Duration: 500 * time.Millisecond}),
						core.Delay(200*time.Millisecond),
						core.Parallel(
							core.Timing(pos, 0, core.TimingConfig{Duration: 400 * time.Millisecond}),
							core.Timing(spin, 0, core.TimingConfig{Duration: 400 * time.Millisecond}),
						),
					))
				}),
				t.plain("Loop", func() {
					at = 0
					run("Loop", core.Loop(core.Timing(spin, 1, core.TimingConfig{Duration: 800 * time.Millisecond}), 3))
				}),
			),
			core.Text{TestID: "animated-state", Style: t.caption(), ContentFunc: state.Get},
		),
		t.demo("Platform drag", "PanValues move values as the pointer moves, on the main thread; letting go springs the card home with the drag's velocity.",
			core.View{Style: core.Style{Height: core.Pt(180), Background: t.background, CornerRadius: radius,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
				core.View{TestID: "platform-card", Label: "Platform-dragged card", PanValues: core.AnimatedXY{X: dx, Y: dy}, OnPan: home,
					Style: core.Style{Width: core.Pt(140), Height: core.Pt(90), CornerRadius: cardRadius, Background: t.accent,
						AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
						ShadowColor: core.RGBA(0, 0.2), ShadowRadius: 6, ShadowOffset: core.Point{Y: 3},
						Animated: &core.AnimatedStyle{TranslateX: dx, TranslateY: dy, Rotate: lift}},
					Children: []core.Node{core.Text{Content: "Drag me", Style: core.Style{Color: t.onAccent, FontWeight: core.FontWeightSemibold}}}},
			}},
		),
		t.demo("Layout animation", "Scope.AnimateLayout animates the next commit's moves, insertions and removals on the platform, with React Native's presets.",
			core.View{StyleFunc: func() core.Style {
				s := core.Style{Height: core.Pt(28), CornerRadius: 6, Background: t.accent, Width: core.Pct(30)}
				if wide.Get() {
					s.Width = core.Pct(100)
				}
				return s
			}},
			core.View{Style: core.Style{Gap: 6}, Children: []core.Node{core.Each(items.Get, func(i int) int { return i },
				func(cx *core.Scope, item func() int, _ func() int) core.Node {
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 10),
						Background: t.sunken, CornerRadius: 6}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return fmt.Sprintf("Row %d", item()) }},
					}}
				})}},
			flow(
				t.primary("Add", func() { animate(func() { items.Set(append(slices.Clone(items.GetUntracked()), next)); next++ }) }),
				t.plain("Remove", func() {
					animate(func() {
						if l := items.GetUntracked(); len(l) > 0 {
							i := rand.IntN(len(l))
							items.Set(slices.Delete(slices.Clone(l), i, i+1))
						}
					})
				}),
				t.plain("Shuffle", func() {
					animate(func() {
						l := slices.Clone(items.GetUntracked())
						rand.Shuffle(len(l), func(i, j int) { l[i], l[j] = l[j], l[i] })
						items.Set(l)
					})
				}),
				t.plain("Resize", func() { animate(func() { wide.Update(func(w bool) bool { return !w }) }) }),
			),
		),
	}
}
