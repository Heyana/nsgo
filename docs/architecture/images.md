# Images

`Image` shows a picture the platform loads: a URL, a file, an `asset:` path in the app's resources, or bytes the app holds. Bitmaps, animated GIF and WebP files and SVG documents all load as images, as in NSH's `docs/dev/images.md`. Read this before changing how a backend loads, decodes or plays images.

## Decoding

- Formats are sniffed from the bytes, never from names or types: `imagedecode.Sniff` finds an SVG after a byte order mark, an XML declaration, comments and a doctype, or inside gzip.
- `OnLoad` reports the source's size in pixels, whatever size a view decodes it at, and an SVG's intrinsic size in whole points.
- An animated image plays on its own in each view, with each frame's delay (10 ms or less shows for 100 ms, as in browsers) and its loop count. A tinted one shows its first frame, as NSH's does, and so does a blurred one on Apple platforms.
- `platform/internal/imagedecode` decodes in pure Go for the backends without a decoder of their own: PNG, JPEG, every frame of GIF and WebP animations (at most 600 frames and 48 MiB of them), and SVG through its own rasterizer within NSH's bounds ([icons](icons.md#svg-documents)).

| Target | Bitmaps and animations | SVG |
| --- | --- | --- |
| Apple | ImageIO, at the view's pixel size; GIF, WebP and APNG frames play as a discrete keyframe animation of the layer's contents | `imagedecode`, its shorter side as long as the view's longer side in pixels |
| Web | `<img>` | `<img>` from an `image/svg+xml` blob |
| Windows, Linux | `imagedecode`; frames on the backend's frame clock | `imagedecode`, 4 pixels per point, at most 2048 a side |
| Android | `ImageDecoder`, with `AnimatedImageDrawable` for animations | Rasterized by `imagedecode` in the Go process at the view's size when it loads, and sent as a bitmap |

## SVG drawing

`imagedecode` draws an SVG at the size NSH's `svg` module would: covering a view's pixel size, at its intrinsic size times a scale, or oversampled 4 times to at most 2048 pixels a side, within 8192 a side and 16 Mi pixels. Its scanline rasterizer computes exact coverage across each row with 16 samples down it, flattens curves within 0.05 pixels, strokes with caps, joins and a miter limit of 4 as a union of convex pieces, and paints gradients through a 1024-color table. A document is at most 16 MiB, an `.svgz` once inflated, and drawing one is bounded by edge work, so a small file cannot ask for minutes.

## Verification

`go test ./platform/internal/imagedecode` checks decoded frames, rasterized pixels and the bounds; the Windows and Linux native tests check animated GIFs and SVG pixels in a window. The gallery's Images section shows an animated GIF and SVG badges, plain and tinted.
