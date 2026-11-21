---
title: Images and icons
description: Show a picture from a URL, the app's resources, bytes or pixels the app draws, and draw Lucide icons and SVGs.
group: Core concepts
order: 54
---

## A picture

`core.Image` shows a bitmap or an SVG: a URL, a file, an `asset:` path in the app's resources, or bytes the app holds. The platform loads, decodes and caches it off the main thread, and the main thread draws the result.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` fills the box and crops what overflows; `core.Contain` fits the whole picture, `core.Center` keeps its natural size, and `core.ScaleDown` keeps it unless it is larger than the box. `BlurRadius` blurs the picture in points, `TintColor` recolors its opaque pixels, and `ObjectPosition` places it in the box.

Scaled bitmaps use smooth sampling by default. Set `Sampling: core.ImageSamplingNearest` in Go or `sampling="nearest"` in TSX to keep the hard edges of pixel art and pixel buffers. Apple platforms, Windows, Linux, Android and the web support this setting; HarmonyOS and mini programs apply it too, though their output is not yet checked on devices. For exact pixel edges in a browser using CSS `pixelated`, scale to a whole number of device pixels per source pixel.

`Source` declares the size layout uses: one dimension derives the other, and an image without one is 0 by 0 until its style sizes it. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` shows bytes embedded with `go:embed`, and `Size` gives an SVG its intrinsic size.

With [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` lets the pointer through pixels less than half opaque on macOS and Windows. TSX sets the same threshold with `hitTestAlpha`.

## Frames the app draws

`Pixels` shows an `*image.RGBA` the app draws, a video player's or an emulator's frames, with nothing encoded, loaded or decoded. Write each frame into the same buffer and bump `Version`: the view shows it with no load events and no fade, its alpha kept, and on macOS, iOS, tvOS, watchOS, Android, Windows and Linux swaps it in place with no blank frame in between. Without `Size` it measures its pixels in points.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // writes the next frame into frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo copies the pixels when the commit that carries a new `Version` runs, so the app may write the next frame from its next `RequestAnimationFrame` callback; a decoder on a goroutine of its own writes into a second buffer and posts the swap. The same `Version` is not drawn again. `cx.ImageSize` answers a pixel source at once with its buffer's size. A `core.Canvas` draws pixels with `DrawImage`, copied the same way, on every platform but HarmonyOS and mini programs. A `core.Shader`'s `Images` take pixels the same way, so a shader can filter a frame the app writes, such as an emulator's screen, on every platform that draws shaders.

macOS, iOS, tvOS, watchOS, Android, Windows and Linux draw the pixels directly. The web shows an image view's pixels through a PNG made once per version, loaded as a new image, so the view is blank between versions until it loads; the web's canvases draw the pixels directly. HarmonyOS and mini programs do not show pixels yet.

## Resources and scales

Files under `resources/` ship with the app. `core.Src("asset:images/cover.png")` loads one, and `Scales: core.ScalesOf(1, 2, 3)` picks `cover.png`, `cover@2x.png` or `cover@3x.png` for the screen's pixel ratio.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` shows a source while the picture loads, and `FadeDuration` fades the loaded picture in over it; 0 lets the platform choose, and a nanosecond shows it at once. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` and `OnLoadEnd` follow the load. `cx.ImageSize` reads a source's size in pixels without showing it, `cx.PrefetchImage` downloads one into the cache, `cx.SetImageCacheLimit` sets the in-memory encoded image byte budget (default `core.DefaultImageCacheLimit`, 32 MiB; negative values clamp to 0), and `cx.KeepImage` pins an atlas or texture out of cache eviction until its returned `release` runs or its scope disposes. Native backends keep remote images in a disk cache of their own.

## Animated and vector images

GIF and WebP animations play in the view, each frame for its own delay, and loop as the file says. SVG documents load as images and stay sharp at any size; a one-color SVG can be tinted with `TintColor`.

## Icons

`core.Icon` draws an SVG in the text color it inherits, as high as the font size unless its style sizes it. Its box keeps the SVG's aspect ratio, so a square box does not stretch it.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` has an accessor per Lucide icon, named in PascalCase (`lucide.ArrowUpRight`). An icon is decoration to assistive technology unless `Label` names it. `ui.Icon` is the same with `ui`'s defaults, and `core.SVG` and `core.SVGView` draw paths the app builds itself.

## The styled image

`ui.Image` is a picture in a rounded well, with a placeholder while it loads and a faint picture icon when it fails. It keeps the source's aspect ratio, or 3:2 without one, unless its style sizes both sides.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Next

- [Text](text.md): images inside a line, as spans.
- [Styles and themes](styles.md): corners, shadows and tints.
- [Lists and scrolling](lists.md): rows of pictures that recycle.
