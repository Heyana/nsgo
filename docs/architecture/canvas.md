# Canvas

`core.Canvas` records 2D drawing on the engine and has the platform play it, as NSH's `Canvas` does (`nsh/docs/dev/canvas.md`). Read this before changing `core/canvas.go`, `internal/driver/canvas*.go`, a backend's canvas player or `internal/platform/canvastest`.

## Recording

`Canvas.Draw` runs on the engine when a signal it read, the canvas's size or the available fonts change and records `DrawOp`s into a `Drawing`, which travels in `ItemCanvas` and never changes after. `FontsChanged` schedules every live canvas's recording effect: a drawing can use `TextMetrics` to place shapes without recording text, so replaying its old operations would leave stale geometry. `Icon` and `SVG` record the same way ([icons](icons.md)). The methods follow the web's `CanvasRenderingContext2D`:

- State: `Save` and `Restore`, `Translate`, `Scale`, `Rotate`, `Transform`, `SetTransform` and `ResetTransform`, alpha, and `SetComposite`, Porter-Duff operators and blend modes named as `globalCompositeOperation`.
- Lines and text: widths, caps, joins, `SetMiterLimit`, `SetLineDash` (an odd list repeats, as the web's) and its offset, `SetShadow`, `SetTextAlign` and `SetTextBaseline`.
- Paths: lines, curves, `Arc`, `ArcTo`, `EllipseArc` for rotated ellipses, `RoundRectCorners` for per-corner radii, and `Path` values, which `ParsePath` makes from SVG path data.
- `FillPath`, `StrokePath` and `ClipPath` use a `Path` without touching the current path; `FillEvenOdd` and `ClipEvenOdd` take the even-odd rule.
- Paints: colors, and `CanvasLinear`, `CanvasRadial` and `CanvasConic` gradients.
- Text: `FillText` and `StrokeText` in a `Font`.
- Images and SVGs: `DrawImage` at the natural size, `DrawImageIn` into a rectangle and `DrawImageFrom` from a part of the image; `DrawSVG` at the document's intrinsic size and `DrawSVGIn` fitted into a rectangle by its `preserveAspectRatio` (clipping for `slice`), leaving the drawing's state and current path intact, resolving `currentColor` from the drawing's fill color (or black when unset), and laying out `<text>`/`<tspan>` elements with the drawing's `TextEngine` into native `DrawFillText`/`DrawStrokeText` ops so every platform renders SVG text with its own text engine.

A canvas has no size of its own unless `Em` gives it one in ems of the font size it inherits, scaled by the user's text size as text is, as `base/formula`'s formulas size themselves: in a text's attachment its baseline, `Em.Descent` ems above its bottom, sits on the line's ([text](text.md)). `EmFunc` and `LabelFunc` follow what they read, laying it out again and naming it anew.

The recorder resolves what needs the engine, so players stay plain: text alignment and baselines become an offset from the text's metrics, and an `ArcTo` becomes an arc from the current point. A `FillPath`, `StrokePath` or `ClipPath` (`FillRect` and `StrokeRect` among them) begins a path for its `Path` and another after it; the current path it leaves is recorded again, under the transforms it was built in and inside a `Save` and `Restore`, before the next op that builds on it or uses it (`DrawKind.UsesPath`). A run of `FillRect`s after a long line, as a chart's bars after its line, records each rectangle alone. Players keep only a transform, a clip and the paint state.

## Queries

A drawing answers without the platform, during `Draw` and after it, so `OnPress` can test the last drawing's shapes:

- `MeasureText` and `TextMetrics` lay text out with the platform's `TextEngine` and report the first line's width, ascent, descent and line height. Trailing white space counts, as in a browser: engines leave it out of a line, so such a text is measured with a mark after it, less the mark. Without an engine, as in some tests, they approximate: 0.6 em a character, 0.8 em up and 0.2 em down.
- `IsPointInPath` and `IsPointInStroke` replay what was recorded for its transform, current path and line style and test the flattened path; `PathContains` and `StrokeContains` test a `Path`. Strokes take joins as round and dashes as solid.
- Bound arguments count as recorded.

## View paints

`FillFromView`, `StrokeFromView`, `LineWidthFromView` and `LineDashFromView` record an operation whose value is the canvas view's SVG paint style ([styles](styles.md)), noted in `Drawing.ViewPaints` with the initial paint where no view sets one. The core resolves the inherited `Fill`, `Stroke`, `StrokeWidth` and `StrokeDash`, with `PaintCurrentColor` the view's text color, and sends `Drawing.Painted`: a change of them sends the drawing again without drawing it again, and no platform knows of them. A canvas that has drawn nothing sends no empty first drawing.

`FillBackgroundFromView(area)` fills the current path with the view's own `Background` and `Gradients`, placed over `area` as over a box of that size, as CSS's `clip-path` shows a box's background: a chart's wash under its line. The recorder leaves a transparent fill the core expands into a fill per paint, the gradients turned into `CanvasGradient`s, and moves the slots after it along. The view is then sent without its background, so it is drawn once, inside the shape; a drawing with nothing to fill yet records the fill over an empty path so the box never shows.

## Animated arguments

`Drawing.Bind(arg, node)` has the platform read argument `arg` (`ArgX` to `ArgColor`) of the last operation from an animated node. The drawing carries the bindings as `DrawSlot`s; the backend registers them with `Animator.BindCanvas` and, when `ChangedCanvases` reports a canvas, plays a copy from `CanvasOps` with the values patched in. The canvas redraws on the frame clock without a commit. Android's Java animator patches the encoded operations in place by their byte offsets.

macOS, iOS and Linux redraw only what the change covers, as NSH's partial redraws; Windows records the changed part again; Android and the web redraw the whole canvas.

- `driver.CanvasDamage` replays the operations last drawn and those drawn now for their geometry, and adds up the bounds of the painting operations that changed or paint with state that did: transform, path, alpha, paint, line, shadow.
- The view invalidates that area of its content layer, 2 points wider for anti-aliasing and rounded out (`apple.Damaged`). Core Graphics clips the replay of the whole drawing to it, so the pixels are a full redraw's.
- Bounds are conservative: curves by their control points, arcs by their circle, strokes by their reach at miter joins and square caps, shadows by twice their blur, text by 1.5 em a character.
- A changed clip, changed painting under a composite operation other than source-over, an image at its natural size and bounds that are not finite redraw the whole canvas.

What the ops before the first bound one paint is the same whatever the nodes hold: its static part (`driver.SplitCanvas`, NSH's `first_animated`). A backend keeps its pixels and plays only the painting after it over them; the ops before it still set the state, and `CanvasSplit.Skips` leaves out their painting and the path no later op uses.

| Target | Keeps the static part as | How a canvas draws |
| --- | --- | --- |
| macOS | A `CGLayer`, once a drawing of 128 ops or more before its first bound one was painted unchanged 30 times | At once on the main thread, or on Core Animation's threads (`drawsAsynchronously`) while it paints 10 times a second or more and keeps no layer |
| Linux | The pixels a full play left at the split | Into an image surface at the surface's scale, shown as 256-pixel textures: a change draws its area again over the kept pixels, and only the tiles it touches become new textures. A canvas outside a clipping ancestor or the window waits until a layout shows it |
| Windows | A Direct2D command list, played once | From command lists: the static part's and the rest's, played again when a bound node changes, or the whole drawing's. A rest that composites with what came before or clears plays whole |

`internal/platform/canvastest`'s `Split` drawings check that playing over a kept part draws what a full play does, on each of the three.

## Offscreen images

`cx.NewOffscreenImage(size, scale, draw)` records a 2D `Drawing` into a retained `core.OffscreenImage` at logical `size` and backing `scale` (pixels per point; non-positive values take the display pixel ratio or 1). `Source()` returns an `ImageSource` carrying `driver.OffscreenCanvas` (`ID`, `Version`, `Size`, `Scale`, `Drawing`) usable in `Drawing.DrawImage`, `Drawing.DrawImageIn`, `Drawing.DrawImageFrom` and `core.Image`, without loading or firing load events.

The callback runs lazily on first `Source()` call inside `Untracked`, and runs again only after `Invalidate()`, `Resize(size, scale)` (when size or scale changes) or `FontsChanged()`. While clean, repeated `Source()` calls return the same snapshot pointer and `Version`, so later frames pay one `DrawImage` operation per frame with zero re-rendering. `Release()` (also registered on `cx.Cleanup`) emits `ItemReleaseOffscreen` so platforms drop their cached surface.

| Target | Offscreen cache |
| --- | --- |
| macOS, iOS, watchOS | Process-wide mutex-guarded `CGImage` rendered through `CGBitmapContextCreate` with `Play`, keyed by `(ID, Version)` |
| Windows | Per-window GPU `ID2D1Bitmap1` (`d2dBitmapTarget` at `96 * scale` DPI) rendered through `canvasPlayer`, keyed by `(ID, Version, gfxGeneration)` |
| Linux | Per-window Cairo `CAIRO_FORMAT_ARGB32` image surface rendered through `playCanvasOps`, keyed by `(ID, Version)` |
| Web | Per-surface offscreen `<canvas>` rendered through `canvas2d.Player`, keyed by `(ID, Version)`, drawn directly with `drawImage` |
| Android, mini programs | `ItemReleaseOffscreen` filtered cleanly; offscreen rasterization in the Java/mini-program runtimes is not yet implemented |
| `nsgotest` | `App.Offscreen` tracks `Version`, `RenderCount`, `Size`, `Scale`, `Ops` and `Released` per ID |

## Nodes

`Drawing.Node` names a part of the drawing, a chart's bar or a grid's cell, with bounds in the canvas, a role, a label, a test ID and a selected state. A press on one is a press on the canvas at the node's center.

| Target | Nodes are |
| --- | --- |
| macOS | `NSAccessibilityElement`s, the canvas view's accessibility children |
| iOS | `UIAccessibilityElement`s in place of the canvas |
| Web | ARIA elements inside the `<canvas>`, which browsers expose at their bounds and never draw |
| Android | Virtual views of the canvas's `AccessibilityNodeProvider` |
| Windows | UI Automation providers, the canvas element's children |
| Linux | Empty `GtkLabel`s of the node's role at its bounds, which take no pointer |
| `nsgotest` | Views of the model: selectors find them by label, text and test ID, and a press presses the pressable view under their center |

## Players

Each backend plays the operations in order on the main thread; on macOS into a context Core Animation may draw on its own threads ([animated arguments](#animated-arguments)). Images load as image views' do, cached by `ImageSource.Key`; inline byte sources use a content digest, so a collected buffer's address cannot make a cache return an older image when the address is reused. A canvas draws again when one arrives and draws nothing in its place until then. Apple, Windows, Linux and Android keep the 64 unkept encoded sources drawn last, counting failed loads too, and release an evicted one's native image. `Scope.KeepImage` pins a source's resolved key (`ItemKeepImage` / `ItemReleaseImage`) so it stays out of the 64-source eviction bound while kept: pins are reference-counted, idempotent to release, released automatically when the owning scope disposes, and on Apple's shared cache released per surface when a surface closes; once a source's last pin releases, excess unkept entries trim back to 64. A load under way is never evicted, so loads under way exceed the bound until they finish: cancelling one would leave what waits for it, a canvas or a shader that draws only when its images arrive, without the image, and canvases drawn in passes of their own would cancel each other's loads forever. A pass of drawing (a paint, frame or Core Animation transaction: everything drawn before the main thread next runs posted work) never evicts a source it drew, so one drawing more than 64 keeps them all, rather than loading some again at every pass forever, until later passes draw others. Windows, Linux and Android keep the cache per window or surface and release it on close; Apple's is shared. Android requests bytes from Go on a cache miss, including after eviction; Go keeps sources while their mounted canvases or shaders use them, and a load until Java's cache cancels it or its result is sent. A request's generation rejects a late result. Java recycles evicted bitmaps after any shader frame using them finishes its upload, and cancels its loads and queued decodes when its surface closes.

Pixels draw at once on Apple, Windows and Linux, with a separate cache of up to eight frames. Android keeps the bitmaps of the drawing's current pixel frames and reads them on the first draw ([pixel buffers](images.md#pixel-buffers)).

| Target | Plays into | Composites | Shadows | Conic gradients | Stroked text |
| --- | --- | --- | --- | --- | --- |
| macOS, iOS | Core Graphics | Blend modes; operators that clear outside the shape draw in a transparency layer | Core Graphics's | `CGContextDrawConicGradient` from macOS 14 and iOS 17, wedges before | CoreText in the stroke drawing mode |
| Web | A 2D `<canvas>` | `globalCompositeOperation` | The canvas's, scaled to its pixels | `createConicGradient` | `strokeText` |
| Windows | Direct2D, through an offscreen bitmap when the drawing composites or clears | Direct2D's composite and blend effects | A command list through the shadow effect | A bitmap brush of the sweep | DirectWrite outlines, through a text renderer written in Go |
| Linux | Cairo | Cairo's operators | An alpha mask blurred with a box blur | A mesh pattern | Pango's layout path |
| Android | `android.graphics.Canvas` | `BlendMode`, or Porter-Duff modes before Android 10, in a layer for operators that clear outside the shape | The paint's shadow layer | `SweepGradient` | The paint's stroke style |

A call into the platform can cost more than the op it plays draws ([performance](performance.md#keeping-frames-cheap)). Apple's player calls the Core Graphics functions of pointers and integers through `purego.SyscallN` (`registerSyscall`), adds a `MoveTo` and the `LineTo`s after it in one `CGContextAddLines`, makes each color once a drawing, tracks whether the path is empty and asks Core Graphics only after shapes that may add nothing, and copies the path that a fill, stroke or clip keeps only when a later op uses it.

`internal/platform/canvastest` holds sample drawings, one for each group of features, and the pixels they must produce; the Apple, Windows and Linux players check them in their tests. Cache tests on Apple, Windows, Linux and Android exercise 1,000 distinct sources, eviction, reloading and loads under way; on Apple and Linux, canvases drawn in passes of their own load 88 slow images together; Android also runs its real pipe, decoder and player on the emulator. The web is checked by screenshots of the gallery's Canvas page.

The web replays canvases 120 ms after a scroll zoom's last step, at the screen's pixel ratio multiplied by the enclosing scroll zooms, with raster zoom clamped to 1–4 as on Apple. The zoom never takes a bitmap past 4096 × 4096 pixels or a side of 16384, where browsers stop drawing or memory runs short; a canvas already past that at the pixel ratio alone keeps the ratio. CSS scales the kept pixels during a pinch or animation. New drawings and moved content take their current ancestry's scale, read from the zoomed contents' `data-nsgo-zoom`; a zoom replays the recording without rerunning `Draw`. Once the replay's bitmap is exactly as large as the screen shows it, a canvas inside zoomed contents is drawn with `image-rendering: pixelated`: Chrome filters nested scale transforms even when they add up to one pixel per pixel, which greys sharp edges, and nearest sampling at that size changes nothing else. A zoom returns those canvases to smooth sampling until the next replay, and a capped or fractional size keeps it.

## Where NSGo departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| Any number or color may be an animated value, bound where it is passed | `Bind` binds the last operation's argument | Go has no union of a number and a node; one method keeps every drawing call plain |
| Nodes reach only GPUI's accessibility and the headless driver | Nodes reach each backend's accessibility, except Windows' and Linux's | NSGo has no GPUI backend; native accessibility is where screen readers look |
| `font` is a CSS shorthand string | A `Font` value | Styles are Go values |
