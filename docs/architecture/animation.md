# Animation

Style transitions, animated values and layout animations run on the platform's main thread, stepped by its own frame clock, so a running animation costs the engine nothing per frame. They follow NSH's `docs/dev/animation.md`. Read this before changing `core/animated.go`, `internal/driver/animated.go`, `internal/driver/animator.go` or a backend's frame clock.

Layout runs in the engine, so the frame clock moves only what a platform draws without it: a transition of drawn properties and an animated value never change a view's frame. A view's size and position animate as a [layout animation](#layout-animation), of a whole commit or of one view's own changes.

`View.OnTransitionEnd` and `Overlay.OnTransitionEnd` run once a change to the view's style or classes has had its new `Transition`'s delay and duration after the commit that sends it, at once without a duration or under reduced motion; a later change starts the wait over. The core times it instead of hearing it from the platform, so it runs alike everywhere, whether or not anything moved.

## Frames

`Scope.RequestAnimationFrame(fn)` calls fn once, on the graph's goroutine, when the next display frame begins, with the frame's time: the display's where the platform gives one (the table below), so two frames' times differ by the display's frame length rather than by when the engine got to each, and the graph clock's otherwise. It is on the graph clock's base, as timers are, may lie up to a frame ahead of `Scope.Now` where it is when the frame shows, and never precedes the previous frame's. stop cancels the wait, and a callback whose owner is disposed does not run; registering again from fn is the loop that runs every frame, as `requestAnimationFrame`'s does. `Scope.Animate` steps this way, so an app draws at the display's rate without a timer of its own.

The core holds a callback queue for each surface. A native backend receives one request from `RootOptions.RequestSurfaceFrame` while its queue waits, and the engine delivers the matching platform's timestamp through `Tree.RunFrameAt` before committing what the callbacks changed. The desktop app's own tree runs on an attached window's clock. A host with one clock uses `RootOptions.RequestFrame` and `Scope.RunFrameAt` for all its surfaces. The platform converts its timestamp to the graph's base with `driver.DisplayTime`, moving `time.Now` by the difference between the timestamp and its source clock. The pipe delay cancels out even when Android reads `CLOCK_MONOTONIC` on its serve goroutine. A host with an app clock of its own (`engine.Options.Clock`) uses that clock's time.

`Scope.SetPreferredFrameRate` sets the current surface's range. Apple display links use `preferredFrameRateRange` when only app callbacks need frames, leaving the existing pace for other native motion. Android's regular views live in a window, so the runtime sets that window's `preferredRefreshRate`, the window equivalent of `Surface.setFrameRate`, or uses its own view's request for a host-embedded view on API 35 and later. The Android runtime answers frame requests from each view's Choreographer callbacks and skips frames before sending them over the pipe. The core also checks the elapsed display time for each callback batch, using the last time it delivered a callback so small timestamp differences do not accumulate. A changed range makes the next distinct frame eligible. Native animations keep using their platform clocks and `AtFrameRate` takes their own rate. [Android window refresh-rate hint](https://developer.android.com/reference/android/view/WindowManager.LayoutParams#preferredRefreshRate)

For a desktop app with several windows, the engine asks for the app tree's frames from the visible window that had focus most recently, while each window's own tree keeps asking its own window. If none has taken focus, it uses the lowest visible surface ID. Hiding, minimizing or closing the driver transfers a pending request to the next eligible window. A late tick from the former window cannot satisfy it. The first surface provides the fallback while no attached window is visible.

Each target's frame source is its frame clock, shared with the native animations:

| Target | Frame source | The frame's time |
| --- | --- | --- |
| macOS | The surface's display link, a 60 Hz timer before macOS 14 or while the link stalls | The link's `targetTimestamp`, when the frame shows; the graph clock's from a timer |
| iOS | The surface's `CADisplayLink` | Its `targetTimestamp` |
| Web | `requestAnimationFrame` | Its timestamp, when the frame began |
| Android | The runtime's `Choreographer`, which reports each frame over the pipe while the engine waits for one | Its `frameTimeNanos`, the frame's vsync |
| Linux | The surface's `GdkFrameClock`, at the display's rate, a timer every 100 ms while it stands still, as for a window not shown | The frame clock's frame time, when the frame began; the graph clock's from the timer |
| Windows | A goroutine waiting on `DwmFlush`, as below | When `DwmFlush` last returned, the composition's end |
| HarmonyOS | The host's `displaySync`, over ArkWeb's bridge | Its `targetTimestamp`, moved onto Go's clock by the bridge's quickest delivery seen (`driver.DisplayClock`), as the web view cannot read the host's clock; only a host built with the SDK (`harmony.CompileHost`) sends it, and the committed host, until `host/build.sh` rebuilds it, calls `nsgoFrame` without one and gets the graph clock's |
| watchOS | The host's `TimelineView(.animation)` | The timeline's date |
| Mini programs | A 16 ms timer of the logic layer, which has no display frames | The graph clock's |

Headless tests run frames as the manual clock advances: `nsgotest.App.Advance` moves the clock a frame at a time and runs a waiting frame at each, applying the same preference to callbacks as a platform.

## Animated values

The app builds a graph once: `AnimatedValue`s, nodes derived with `Interpolate` (numbers or colors, with React Native's extrapolation and easings), `Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` and `SnappingDiffClamp`, and animations that move values. `Style.Animated` binds a view's opacity, translation, scales, rotations about each axis, background, border color, shadow color, shadow radius, shadow opacity and text color to nodes, over its style, and a sticky list header's inset (`StickyInset`). A bound text color tints the whole drawn text, as NSH's does.

- `Timing`, `Spring` and `Decay` move one value; `Sequence`, `Parallel`, `Stagger`, `Delay` and `Loop` compose them. `Scope.StartAnimation` runs one, calls back once with whether it finished, and returns an `AnimationRun` whose `Stop` ends it where it is and `Reset` puts its values back where they were as it started. Each step of a composite starts at the exact end of the one before.
- `Scope.Tween` steps a number for what the graph cannot draw, such as text: Go's frames (`Scope.Animate`'s, with the delay and any of the graph's easings) call its function, while the platform runs a `Timing` of a value nothing shows, whose end gives the function the last number and holds `nsgotest` steps as any run that ends does. Its run's `Stop` halts the frames where they are and `Reset` gives the function its start again; reduced motion gives the end at once. TSX's `Animated.tween` returns that run with a promise of whether it finished.
- Drivers are closed-form functions of the time since they started, so dropped frames change nothing: React Native's damped spring, and NSH's decay, which stops at or bounces off its bounds at the exact time it reaches them. `SampleEasing`, `SampleSpring` and `SampleDecay` return their curves from the same code, to draw them.
- A value's `Set`, `Stop`, or another animation of it ends its run unfinished; so does the owner going. `Read` asks the platform for a value.
- `AtFrameRate` caps how often a run moves its values, for what loops as long as work lasts: the animator holds it to the rate on every target, Android's Java animator too, and macOS and iOS slow their display links to the fastest rate the runs ask for while nothing else needs the clock (a layout animation, a moving scroll view, a waiting frame, a selection's drag or the performance monitor), applying batches as they land meanwhile rather than holding them for frames that far apart, as iOS always does.
- `ValueKeyframes` plays a value through stops as Web Animations does: offsets spread evenly when missing, an easing per segment, a delay, iterations and directions. It becomes a composite of timings, so the platforms learn nothing new; `StepsEasing` adds CSS's `steps()`.
- A `Motion` moves a view's opacity, translation, scale and rotation through keyframes together, from a `MotionSpec` or one of NSH's `MotionPreset`s (entrances, exits, attention seekers), landing at once under reduced motion.
- `SnappingDiffClamp` is a `DiffClamp` that, when the scroll view its input follows comes to rest, eases in 200 ms to whichever end is nearer, so a collapsing header is never left half hidden.
- `View.PanValues` has drags move values by their translation, `PinchValue` scales a value by a pinch and `RotationValue` turns one by a rotation, each from where the value was as the gesture began, so a gesture grabs a value an animation is moving (`Animator.Gesture`). A pan's start and end report the values in `PanEvent.Values`, to snap from. `Scroll.OffsetValues` follows a scroll view's offset. Each moves in the frame the pointer or the content does.
- Nodes also drive a `Shader`'s uniform lanes and a `Canvas`'s drawing arguments, which the backend draws again when one changes ([shaders](shaders.md), [canvas](canvas.md)).
- Values are released with the owner that made them; derived nodes with the owner running when they were made.
- `ProjectDecay` returns where a decay brings a value to rest, to choose the point a released drag snaps to, and `RubberBandClamp` resists a value past its bounds as a scroll view's edge does: plain functions for handlers. `Resist` is `RubberBandClamp` as a node, so a view bound to it through a value `PanValues` drags slows at its ends on the frame clock; the value keeps the drag's own translation, which a pan's end reports to spring back from.

The graph travels as `ItemAnimation`, after the batch's views. Each backend keeps a `driver.Animator`, applies the ops, steps it once a frame while a run is live and draws what `Changed` reports with `AnimatedValues.Apply` over the view's style. Finished runs and reads come back as `EventAnimation`. AppKit and UIKit share the dispatch of those changes to view styles, canvases, native props and sticky insets.

| Target | Frame clock | Draws bound values with |
| --- | --- | --- |
| macOS | AppKit's `CADisplayLink` (macOS 14), else a 60 Hz timer | The layer's properties, without implicit animation |
| iOS | `CADisplayLink` at the screen's `maximumFramesPerSecond` | The layer's properties |
| Web | `requestAnimationFrame` | Inline styles |
| Windows | A goroutine that waits for each composition (`DwmFlush`) and posts a tick | The next paint |
| Linux | The surface's `GdkFrameClock` (`gtk_widget_add_tick_callback`) | The next paint |
| Android | `Choreographer` | A Java port of the animator on the UI thread |
| watchOS | SwiftUI host frames | The next host update, including values applied after a commit |

Android reads the CSS animation steps from Go's `KeyframeAnimation.Cycle` and applies them with `Keyframes.java`. A JVM test checks the wire steps and sampled looks against Go. The fixture uses the Java animator's cubic solver for Android's interpolator.

A display link runs only in the run loop mode object it is given: Foundation's `NSRunLoopCommonModes` (`apple.CommonModes`), not an equal string. A ProMotion iPhone grants links more than 60 Hz only to apps whose `Info.plist` sets `CADisableMinimumFrameDurationOnPhone`, which `nsgo` writes; iPads ignore it.

## Layout animation

`Scope.AnimateLayout` animates the next batch that changes anything, as React Native's `LayoutAnimation.configureNext`: the batch carries the `LayoutAnimation`, with React Native's presets as `LayoutEaseInEaseOutPreset`, `LayoutLinearPreset` and `LayoutSpringPreset`.

- Update: a view that existed before the batch moves and resizes from the frame it shows, mid-animation included, to its new one. Springs fit into the duration as UIKit's do.
- Create: the top view of each subtree the batch inserts fades in, or grows from its center.
- Delete: the top view of each subtree the batch deletes keeps drawing where it was while it fades or shrinks away, its ID already free for reuse. Views it holds go with it.
- A view whose style's `Transition` lists `TransitionLayout` (`transition-all`; TSX's `transitionProperty` `all`, `width`, `height`, `inset` and their kin) moves and resizes the same way, on its transition's timing and easing, when a change to its own style gives it a new frame. Its `ItemFrame` carries the `LayoutTransition`; a batch's `Update` wins. Its parent's resize alone moves it at once, as CSS transitions a percentage width only when the width changes. What it holds and its siblings take their new frames at once, so it suits a view nothing follows, such as a progress bar's indicator; a section that opens and pushes what is below it wants `AnimateLayout`.
- Reduced motion turns layout animations off. Web Animations play them on the web, springs sampled into CSS `linear()`.
- A view with a `MatchID` that a batch with an `Update` creates, while it deletes a view with the same ID, is that view moved, as SwiftUI's `matchedGeometryEffect` and Motion's `layoutId`: the core sends `ItemMatch` after the round's inserts and before its deletes, and the backend moves the new view from where the old one shows, scroll offsets and a running animation included, hiding the old one at once rather than fading it. Inside a subtree the batch inserts, the view still shows as that subtree's top appears.
- `LayoutKeyboard` is the software keyboard's curve, which a style's `EnvKeyboard` terms animate on: UIKit's as a Bézier, timed from the keyboard notification on iOS however late the batch arrives, and on Android the IME's `WindowInsetsAnimation`: its interpolator and duration, from where its motion already is.

## Exits

`Presence` is `Show` whose branches leave with their exits, gpui-base's `Presence` and Motion's `AnimatePresence`, which NSH lacks. A part of a branch registers an exit with `Scope.OnExit`, and `Motion.Exit` registers one that plays a `MotionSpec`; when `When` changes, the branch stays before the new one until every exit has called its `done`, while `Scope.Present` reads false in it. A branch with none goes at once. A leaving branch never comes back: the new value builds a branch beside it, or once it is gone with `Wait`. `PresenceOf` applies the same lifecycle to a comparable key and content function, with `MaxLeaving` bounding old branches. Exits run in the core, so the backends learn nothing new.

## Where Go departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| `createAnimatedValue`, nodes as `Copy` handles | `*AnimatedValue` and `AnimatedNode` values, released with their owner | Go's garbage collector and owners |
| Optional velocities | `Velocity` 0 keeps the value's current velocity | Zero values are defaults |
| `Easing` as data decoded in Rust | `Easing` values with a `Kind` and `Mode`; zero is linear | Zero values are defaults |
| A transition's properties are drawn ones; `width` and the like are unknown | `TransitionLayout` moves the frame a view's own style change sets | A component animates its own size without animating the rest of the commit |
| No presence: an exit is awaited, then the view removed | `Presence` with `Scope.OnExit` and `Motion.Exit` | A component's parts leave with their motion whatever removes them |
| No transition end | `OnTransitionEnd`, timed by the core | A popup leaves the tree once its exit transition is over |
