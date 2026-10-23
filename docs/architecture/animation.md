# Animation

Style transitions, animated values and layout animations run on the platform's main thread, stepped by its own frame clock, so a running animation costs the engine nothing per frame. They follow NSH's `docs/dev/animation.md`. Read this before changing `core/animated.go`, `core/animator.go`, `core/layoutanim.go` or a backend's frame clock.

## Animated values

The app builds a graph once: `AnimatedValue`s, nodes derived with `Interpolate` (numbers or colors, with React Native's extrapolation and easings), `Add`, `Sub`, `Mul`, `Div`, `Mod` and `DiffClamp`, and animations that move values. `Style.Animated` binds a view's opacity, translation, scales, rotation, background, border color, shadow color and shadow radius to nodes, over its style.

- `Timing`, `Spring` and `Decay` move one value; `Sequence`, `Parallel`, `Stagger`, `Delay` and `Loop` compose them. `Scope.StartAnimation` runs one and calls back once with whether it finished. Each step of a composite starts at the exact end of the one before.
- Drivers are closed-form functions of the time since they started, so dropped frames change nothing: React Native's damped spring, and NSH's decay, which stops at or bounces off its bounds at the exact time it reaches them.
- A value's `Set`, `Stop`, or another animation of it ends its run unfinished; so does the owner going. `Read` asks the platform for a value.
- `View.PanValues` has drags move values to their translation, and `Scroll.OffsetValues` follows a scroll view's offset, both in the frame the pointer or the content moves.
- Values are released with the owner that made them; derived nodes with the owner running when they were made.

The graph travels as `ItemAnimation`, after the batch's views. Each backend keeps a `core.Animator`, applies the ops, steps it once a frame while a run is live and draws what `Changed` reports with `AnimatedValues.Apply` over the view's style. Finished runs and reads come back as `EventAnimation`.

| Target | Frame clock | Draws bound values with |
| --- | --- | --- |
| macOS | AppKit's `CADisplayLink` (macOS 14), else a 60 Hz timer | The layer's properties, without implicit animation |
| iOS | `CADisplayLink` | The layer's properties |
| Web | `requestAnimationFrame` | Inline styles |
| Windows, Linux | The backend's 16 ms frame timer | The next paint |
| Android | `Choreographer` | A Java port of the animator on the UI thread |

A display link runs only in the run loop mode object it is given: Foundation's `NSRunLoopCommonModes` (`apple.CommonModes`), not an equal string.

## Layout animation

`Scope.AnimateLayout` animates the next batch that changes anything, as React Native's `LayoutAnimation.configureNext`: the batch carries the `LayoutAnimation`, with React Native's presets as `LayoutEaseInEaseOutPreset`, `LayoutLinearPreset` and `LayoutSpringPreset`.

- Update: a view that existed before the batch moves and resizes from the frame it shows, mid-animation included, to its new one. Springs fit into the duration as UIKit's do.
- Create: the top view of each subtree the batch inserts fades in, or grows from its center.
- Delete: the top view of each subtree the batch deletes keeps drawing where it was while it fades or shrinks away, its ID already free for reuse. Views it holds go with it.
- Reduced motion turns layout animations off. Web Animations play them on the web, springs sampled into CSS `linear()`.

## Where Go departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| `createAnimatedValue`, nodes as `Copy` handles | `*AnimatedValue` and `AnimatedNode` values, released with their owner | Go's garbage collector and owners |
| Optional velocities | `Velocity` 0 keeps the value's current velocity | Zero values are defaults |
| A text's `color` is bindable | Not bindable | Text is drawn by the platform's text engine from a prepared layout |
| `Easing` as data decoded in Rust | `Easing` values with a `Kind` and `Mode`; zero is linear | Zero values are defaults |
