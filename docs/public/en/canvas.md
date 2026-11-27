---
title: Canvas and shaders
description: Draw 2D graphics on the engine, and fill a view with a GPU fragment shader.
group: Core concepts
order: 59
---

## A canvas

`core.Canvas` records 2D drawing on the engine and has the platform play it, as a web page's `<canvas>` does. `Draw` runs again when a signal it read, the canvas's size or the available fonts change, and the platform draws the recording on the main thread. A font loaded through `cx.RegisterFont` reruns `Draw` once available, updating both text and geometry based on `TextMetrics`.

Use `Em` or `EmFunc` for a canvas sized in ems of its inherited font. For content whose height depends on width, `MeasureEm(width, fontSize)` returns an `EmBox` at the available width in points (`core.Auto` when unbounded) and the resolved font size. Its reads are tracked, and it also runs during layout, so keep it free of signal writes. `Ref.ContentBox()` tracks a mounted view's layout box inside padding and borders, in its own coordinates.

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

The recording follows the web's `CanvasRenderingContext2D`: `Save` and `Restore`, transforms, paths of lines, curves, rectangles and arcs, fills and strokes with colors and gradients, clipping, dashes, shadows, text and images. `core.ParsePath` makes a `core.Path` from SVG path data, and `FillFromView` and `StrokeFromView` take the canvas view's inherited `Fill` and `Stroke` styles, so a chart follows the app's theme.

On the web, canvases inside a zoomed `core.Scroll` or a scaled `Style.Transform` / `AnimatedStyle` replay at the cumulative scale once the motion rests, up to four times the screen's pixel ratio. A canvas whose zoomed bitmap would exceed 4096 × 4096 pixels draws only its visible part and updates as it scrolls or pans, keeping lines and glyphs sharp without rerunning `Draw`. Apple, Android, Windows and Linux likewise keep zoomed canvases and text sharp under both scroll zoom and style transforms.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Resolution

`ResolutionScale` draws a canvas with fewer pixels and stretches them to its box, as a game's quality setting does: 0.5 draws half the pixels per side, times the screen's density and any zoom or transform around the canvas. Zero and values outside (0, 1) keep the screen's density. The stretch is smooth unless `Sampling` is `core.ImageSamplingNearest`, which keeps pixel art blocky. `ResolutionScaleFunc` and `SamplingFunc` follow signals and play the last drawing again without rerunning `Draw`. Hit tests, `OnPress` and the size `Draw` gets stay in points. HarmonyOS still draws at the screen's density.

```go
core.Canvas{
	Style:               core.Style{FlexGrow: 1},
	ResolutionScaleFunc: func() float32 { return quality.Get() }, // 0.5 on low
	Sampling:            core.ImageSamplingNearest,
	Draw:                drawScene,
}
```

## Text, images and parts

`MeasureText`, `TextMetrics` and `SetTextAlign` lay text out on the platform's text engine, so labels use the same fonts as the rest of the app. `DrawImage`, `DrawImageIn` and `DrawImageFrom` draw pictures, `DrawSVG` and `DrawSVGIn` draw parsed `*core.SVG` documents (`core.ParseSVG`) at their intrinsic size or fitted into a rectangle — with `<text>` and `<tspan>` elements measured and rendered by the platform's own text engine and `currentColor` taken from the drawing's fill color — and `OnPress` can test the last drawing with `IsPointInPath` and `IsPointInStroke`. Native backends cache the 64 most recently drawn unkept canvas image sources; call `release := cx.KeepImage(src)` to pin sprite atlases or long-lived textures out of that eviction bound until `release()` runs or `cx` disposes.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Scaled bitmaps use smooth sampling by default. For pixel art, call `d.SetImageSmoothing(false)` before `DrawImage`, `DrawImageFrom` or `DrawAtlas`, or set `ctx.imageSmoothingEnabled = false` in TSX. `Save` and `Restore` keep the setting with the rest of the drawing state. Nearest-neighbor sampling is available on Apple platforms, Windows, Linux, Android and the web; HarmonyOS and mini programs apply it too, though their output is not yet checked on devices.

`d.DrawVertices(image, vertices)` draws a textured mesh. Each consecutive three `core.CanvasVertex` values make one triangle. Set `Position` in canvas points and `TexCoord` in image points, as in `DrawImageFrom`. Each vertex's `Color` multiplies the texture and interpolates over the triangle. Its zero value leaves the texture unchanged. Update the points and record the drawing again to deform the mesh. The image can be any `ImageSource`, including a pixel buffer or an `OffscreenImage`'s `Source()`; Android's canvases and mini programs draw no offscreen images yet. `SetImageSmoothing(false)` applies to meshes too. The web needs inline or same-origin images for arbitrary meshes. Mini programs draw meshes with the web's player, which is not yet checked in WeChat DevTools or on a device. HarmonyOS clips each triangle and draws the image through its affine transform; as with `DrawImage`, it draws encoded images and pixel buffers but not offscreen images, and it plays each triangle's average vertex alpha but no color tint. It hides the seams between opaque triangles only, and is not yet checked on a device.

`DrawTintedImage`, `DrawTintedImageIn` and `DrawTintedImageFrom` multiply an image's sRGB and alpha channels by a color before compositing. With the default source-over mode, transparent pixels leave the backdrop visible. Use `drawTintedImage(image, tint, ...args)` in TSX with the same coordinate forms as `drawImage`. A cross-origin source on the web must permit anonymous CORS requests for its pixels to be tinted. HarmonyOS tints images from bytes, the app's resources and pixel buffers, though it is not yet checked on a device; offscreen images are not drawn there yet.

Use `d.DrawAtlas(source, rects, xforms, alphas)` to draw many sprites from one image in order. Each `core.Bounds` selects a tile in the image's own points. The matching `core.RSXform` maps the tile's top-left corner to `TX, TY`, with `SCos` and `SSin` set to the scale times the cosine and sine of the rotation. For example, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` draws a 32-point tile at half size at (40, 20). Pass `nil` for full sprite opacity, or supply factors that multiply the current drawing alpha.

`Drawing.Node` names a part of a drawing, such as a chart's bar or a map's region, with bounds, a role, a label and a selected state. Assistive technology reads those parts as the canvas's children, and tests find them by label, text or test ID; a press on one is a press at its center.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Offscreen images

`cx.NewOffscreenImage(size, scale, draw)` renders a `core.Drawing` once into a retained offscreen image that later frames draw with `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` or `core.Image`. Passing `scale <= 0` uses the current display pixel ratio. The drawing is re-rendered only when `Invalidate()` or `Resize(size, scale)` is called (or when new fonts register), and is automatically released when `cx` is disposed or `Release()` is called.

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## Animated arguments

`Drawing.Bind` binds an argument of the last operation to an animated node, and the platform redraws only that part on its frame clock, without a commit. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` and `ArgColor` name the arguments; macOS, iOS and Linux redraw only the changed area, while Windows records it again and Android and the web redraw the whole canvas.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shaders

`core.Shader` fills its box with a fragment shader the GPU runs for every pixel. `core.CompileShader` takes a program written in Go's syntax with a `Fragment` function, and `core.CompileWGSL` takes WGSL. `Uniforms` sets the program's exported variables, and `Images` gives it up to four textures.

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

A Go shader receives the pixel's center in `dst` and the same point in points in `pos`, and has GLSL's built-in functions; `rectSize`, `pixelScale`, `image0At` to `image3At` and their sizes are NSGo's additions. A WGSL program writes `fn shade(uv: vec2f) -> vec4f` and reads `nsh.time` and `nsh.size`; `Paused` stops its clock. A platform without a GPU path for shaders draws nothing there.

A canvas runs programs too, in its drawing's order, under its transform, clip, alpha and composite operator. `program.Paint(uniforms, images...)` makes a `core.ShaderPaint`. `d.DrawShader(paint, x, y, w, h)` shades a rectangle, with `pos` in its own points, `rectSize` its size and `pixelScale` the pixels a point of it covers on screen. `d.SetShader(paint)` shades the images that `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` and `DrawVertices` draw after it, until `d.ClearShader()`; `Save` and `Restore` keep it. The image drawn is `image0`, read at `dst.xy`, its pixels; `pos` is the same point in its points, and the paint's own images become `image1` on. The program's color takes each sprite's alpha, each vertex's color and a tint. One `DrawAtlas` call shades a whole run of glyphs from a signed distance field atlas in one pass:

```go
var sdf = core.MustCompileShader(`
package main

var Color vec4
var Glow float

func Fragment(dst vec4, pos vec2) vec4 {
	d := image0At(dst.xy).a
	w := fwidth(d)
	return vec4(Color.rgb*Color.a, Color.a) * smoothstep(0.5-w-Glow, 0.5+w, d)
}
`)

d.SetShader(sdf.Paint(core.Uniforms{"Color": core.RGB(0xff3b30), "Glow": pulse}))
d.DrawAtlas(glyphs, rects, xforms, nil)
d.ClearShader()
```

`image0At` reads the texel under a point, so a program that magnifies blends neighboring texels itself. An `AnimatedNode` among the uniforms redraws the canvas on the frame clock, without a commit, as `Bind` does. A WGSL program reads `nsh.time` as 0 there. Apple platforms, Windows, Linux and the web shade canvases. Android, HarmonyOS, watchOS and mini programs leave `DrawShader` out and draw shaded images plain.

`core.ShaderEffect` draws its children through WGSL and `core.ScreenEffect` covers the surface. `core.ShaderTransition` and `core.ScreenTransition` sample the pictures before and after `Value` changes. `core.TransitionShader` does this for navigation and dialogs. On Android, capturing views for effects requires Android 10 or later and OpenGL ES 3.0 with external texture support. `core.Canvas` and `core.Shader` both take `Label` and their children draw over them.

## Exporting a drawing

Call `cx.RenderDrawing(d, size, scale, done)` to rasterize a recorded `*core.Drawing` off the screen into an `image.Image` (`*image.RGBA`) without opening a window, ready for `image/png` export or thumbnailing. `cx.Env().CanRenderDrawings()` reports whether the target supports it (macOS, iOS, tvOS, watchOS, Windows, Linux and the web); elsewhere `RenderDrawing` calls `done` with `core.CodeUnavailable`. `size` is in points, `scale` is pixels per point (both finite and positive, up to 16 Mi pixels), and animation slots and view paints must already be resolved.

## Next

- [Images and icons](images.md): the pictures a canvas draws.
- [Animation](animation.md): values a canvas binds.
- [Accessibility](accessibility.md): the parts a canvas exposes.
