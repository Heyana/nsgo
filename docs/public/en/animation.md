---
title: Animation
description: Transitions, animated values, layout animation and motion presets that run on the platform's frame clock.
group: Core concepts
order: 58
---

## Animated values

An animation runs on the platform's main thread, stepped by its own frame clock, so it costs the engine nothing per frame. An `AnimatedValue` is a number the platform moves; `Style.Animated` binds a view's opacity, translation, scale, rotation, background, border color, shadow and text color to it.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` and `Decay` move one value, and `Sequence`, `Parallel`, `Stagger`, `Delay` and `Loop` compose them. `cx.StartAnimation` returns an `AnimationRun` whose `Stop` ends a run where it is and `Reset` puts its values back. A run starts from where its value is, and a new animation of a value ends the old one.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Derived nodes follow a value: `core.Interpolate` maps numbers or colors with its own easing and extrapolation, and `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` and `SnappingDiffClamp` compute other values from it. `ui.Lerp` is a short interpolation.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` runs a function once at the next display frame, and `Scope.Animate` steps a duration on that clock. Both run on the graph's goroutine, so they can set signals. The function gets the frame's time, the display's own on macOS, iOS, Android, the web, Linux, Windows, HarmonyOS and watchOS (on HarmonyOS when the build machine has the OpenHarmony SDK; an app packaged without it gets the engine's clock), so a loop that moves things by the time between two frames follows the display's pace; an `nsgotest` test gives the test clock's.

Set a window's callback rate with `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Pass `Minimum` and `Maximum` to give the display link a range, such as `{Minimum: 80, Maximum: 120, Preferred: 120}` on a ProMotion screen. Call it from the window's own scope. The preference applies to `RequestAnimationFrame` and `Animate` until it changes. A zero range restores the display's normal pace. The callback time remains the actual display time. The display and the OS choose the available refresh rate, and NSGo can skip display frames to pace callbacks.


`Scope.Tween` steps a number on that clock for what an animated value cannot reach, such as a figure counting up in a text: it calls a function with each number from one to another, as `Timing` would move a value, and once more with the last. An `nsgotest` step that starts it waits for the last, as for an animation, and under reduced motion it gives the last at once. TSX's `Animated.tween` is the same.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Transitions

A `Transition` animates a view's style or classes when they change, on the platform, with no work for the engine per frame. `Properties` says what moves; 0 means every property but the frame.

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` moves and resizes a view from where it shows when its own style gives it a new frame. A layout animation animates a whole commit instead, as React Native's `LayoutAnimation` does: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` animates the next batch's moves, insertions and removals, and the presets cover easing, linear and spring. A `LayoutSpring` transition takes `Response` (the spring's undamped natural period) and `DampingRatio` (`< 1` underdamped, `1` critically damped, `> 1` overdamped), runs until it settles rather than for a fixed duration, and carries its position and velocity into a new target when retargeted mid-flight.

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd` runs once a change has had its transition, whether or not anything moved. Reduced motion turns transitions and layout animations off, so respect the platform's setting by keeping the end state reachable.

## Motion

`core.Motion` moves one view's opacity, translation, scale and rotation through keyframes together, from a `MotionSpec` of the app's or a ready-made `MotionPreset`: entrances, exits, attention seekers and loops. Binding its style sets the view at rest; `Play` runs a spec, and `Exit` lets a view leave with its motion inside a `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` keeps a branch until every exit it registered has finished, so a sheet or a toast leaves with its motion whatever removes it. `core.PresenceOf` applies the same behavior to a comparable key and content function, and `MaxLeaving` bounds old branches while a value changes quickly. `MotionPresets()` lists every preset and `Kind()` says whether it enters, exits, draws attention or loops.

## Keyframes

`KeyframeAnimation` repeats keyframes on a platform: opacity, scale, rotation and vertical translation at each offset, with a duration, delay, easing and iterations. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` and `KeyframesBounce` are Tailwind's.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` moves an animated value through stops as Web Animations does, with offsets, a per-segment easing, a delay, iterations and a direction. Gestures can drive values too: `View.PanValues`, `PinchValue` and `RotationValue` move them on the main thread from where they were as the gesture began.

`core.PanEvent` includes `Position` in the view and `PagePosition` in the page on every pan phase, so drag tools do not need a separate press event to recover the finger location.

## Next

- [Styles and themes](styles.md): the properties a transition moves.
- [Lists and scrolling](lists.md): values that follow a scroll view.
- [Layout](layout.md): what animated frames are.
