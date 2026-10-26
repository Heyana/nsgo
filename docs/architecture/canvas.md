# Canvas

`core.Canvas` records 2D drawing on the engine and has the platform play it, as NSH's `Canvas` does (`nsh/docs/dev/canvas.md`). Read this before changing `core/canvas.go`, `internal/driver/canvas*.go`, a backend's canvas player or `internal/platform/canvastest`.

## Recording

`Canvas.Draw` runs on the engine when a signal it read or the canvas's size changes and records `DrawOp`s into a `Drawing`, which travels in `ItemCanvas` and never changes after. `Icon` and `SVG` record the same way ([icons](icons.md)). The methods follow the web's `CanvasRenderingContext2D`:

- State: `Save` and `Restore`, `Translate`, `Scale`, `Rotate`, `Transform`, `SetTransform` and `ResetTransform`, alpha, and `SetComposite`, Porter-Duff operators and blend modes named as `globalCompositeOperation`.
- Lines and text: widths, caps, joins, `SetMiterLimit`, `SetLineDash` (an odd list repeats, as the web's) and its offset, `SetShadow`, `SetTextAlign` and `SetTextBaseline`.
- Paths: lines, curves, `Arc`, `ArcTo`, `EllipseArc` for rotated ellipses, `RoundRectCorners` for per-corner radii, and `Path` values, which `ParsePath` makes from SVG path data.
- `FillPath`, `StrokePath` and `ClipPath` use a `Path` without touching the current path; `FillEvenOdd` and `ClipEvenOdd` take the even-odd rule.
- Paints: colors, and `CanvasLinear`, `CanvasRadial` and `CanvasConic` gradients.
- Text: `FillText` and `StrokeText` in a `Font`.
- Images: `DrawImage` at the natural size, `DrawImageIn` into a rectangle and `DrawImageFrom` from a part of the image.

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

macOS and iOS redraw only what the change covers, as NSH's partial redraws; the other backends redraw the whole canvas.

- `driver.CanvasDamage` replays the operations last drawn and those drawn now for their geometry, and adds up the bounds of the painting operations that changed or paint with state that did: transform, path, alpha, paint, line, shadow.
- The view invalidates that area of its content layer, 2 points wider for anti-aliasing and rounded out (`apple.Damaged`). Core Graphics clips the replay of the whole drawing to it, so the pixels are a full redraw's.
- Bounds are conservative: curves by their control points, arcs by their circle, strokes by their reach at miter joins and square caps, shadows by twice their blur, text by 1.5 em a character.
- A changed clip, changed painting under a composite operation other than source-over, an image at its natural size and bounds that are not finite redraw the whole canvas.

## Nodes

`Drawing.Node` names a part of the drawing, a chart's bar or a grid's cell, with bounds in the canvas, a role, a label, a test ID and a selected state. A press on one is a press on the canvas at the node's center.

| Target | Nodes are |
| --- | --- |
| macOS | `NSAccessibilityElement`s, the canvas view's accessibility children |
| iOS | `UIAccessibilityElement`s in place of the canvas |
| Web | ARIA elements inside the `<canvas>`, which browsers expose at their bounds and never draw |
| Android | Virtual views of the canvas's `AccessibilityNodeProvider` |
| Windows, Linux | Nothing yet, as these backends expose no accessibility ([roadmap](../roadmap.md)) |
| `nsgotest` | Views of the model: selectors find them by label, text and test ID, and a press presses the pressable view under their center |

## Players

Each backend plays the operations in order on the main thread; on macOS into a context Core Animation draws on its own threads ([text](text.md#text-and-drawing)). Images load as image views' do, cached by `ImageSource.Key` (Apple keeps the 64 drawn last); a canvas draws again when one arrives and draws nothing in its place until then. Android's Go side reads each image once and sends its bytes for Java to decode.

| Target | Plays into | Composites | Shadows | Conic gradients | Stroked text |
| --- | --- | --- | --- | --- | --- |
| macOS, iOS | Core Graphics | Blend modes; operators that clear outside the shape draw in a transparency layer | Core Graphics's | `CGContextDrawConicGradient` from macOS 14 and iOS 17, wedges before | CoreText in the stroke drawing mode |
| Web | A 2D `<canvas>` | `globalCompositeOperation` | The canvas's, scaled to its pixels | `createConicGradient` | `strokeText` |
| Windows | Direct2D, through an offscreen bitmap when the drawing composites or clears | Direct2D's composite and blend effects | A command list through the shadow effect | A bitmap brush of the sweep | DirectWrite outlines, through a text renderer written in Go |
| Linux | Cairo | Cairo's operators | An alpha mask blurred with a box blur | A mesh pattern | Pango's layout path |
| Android | `android.graphics.Canvas` | `BlendMode`, or Porter-Duff modes before Android 10, in a layer for operators that clear outside the shape | The paint's shadow layer | `SweepGradient` | The paint's stroke style |

A call into the platform can cost more than the op it plays draws ([performance](performance.md#keeping-frames-cheap)). Apple's player calls the Core Graphics functions of pointers and integers through `purego.SyscallN` (`registerSyscall`), adds a `MoveTo` and the `LineTo`s after it in one `CGContextAddLines`, makes each color once a drawing, tracks whether the path is empty and asks Core Graphics only after shapes that may add nothing, and copies the path that a fill, stroke or clip keeps only when a later op uses it.

`internal/platform/canvastest` holds sample drawings, one for each group of features, and the pixels they must produce; the Apple, Windows and Linux players check them in their tests. The web and Android are checked by screenshots of the gallery's Canvas page.

## Where NSGo departs from NSH

| NSH | NSGo | Why |
| --- | --- | --- |
| Any number or color may be an animated value, bound where it is passed | `Bind` binds the last operation's argument | Go has no union of a number and a node; one method keeps every drawing call plain |
| Nodes reach only GPUI's accessibility and the headless driver | Nodes reach each backend's accessibility, except Windows' and Linux's | NSGo has no GPUI backend; native accessibility is where screen readers look |
| `font` is a CSS shorthand string | A `Font` value | Styles are Go values |
