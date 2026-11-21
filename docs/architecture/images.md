# Images

`Image` shows a picture the platform loads: a URL, a file, an `asset:` path in the app's resources, or bytes the app holds; or the pixels the app draws, which load nothing ([pixel buffers](#pixel-buffers)). Bitmaps, animated GIF and WebP files and SVG documents all load as images, as in NSH's `docs/dev/images.md`. An `asset:` source lists the `@Nx` variants it ships in `Scales`, and the core sends the one for the pixel ratio, as NSH's does; without `Scales` the plain file loads. Read this before changing how a backend loads, decodes or plays images.

## Fit, position, blur and fade

- `ResizeMode.Fit` is NSH's `ResizeMode::fit`: it sizes the natural size by the mode (`Center` keeps it, as CSS's `none`; `ScaleDown` shrinks a larger image, as CSS's `scale-down`) and places it at the style's `ObjectPosition`.
- Every backend draws at that rectangle, clipped by the view; Android's Java runtime repeats it in `ImageNode.fit`.
- The natural size is the size the source declares, an SVG's intrinsic size, or the pixels over a name's `@2x` density. Android takes its drawable's intrinsic size and ignores a declared size.
- The position is the image's, not the view's: it travels in `ImageConfig.Position`, and a change sends `ItemImage` again, never `ItemViewStyle`. Android receives each axis as a fraction of the free space plus points, which any position resolves to.
- Apple frames a picture layer at the fitted rectangle inside the clipping layer, the tint and its mask inside it; the web sets the background's or mask's size and position in pixels.
- `ImageConfig.Sampling` carries the scaling filter. Smooth is the default. Apple and Android skip decode-time reduction when nearest is requested and apply nearest filtering to layers and drawables. Windows sets the Direct2D bitmap interpolation mode, Linux sets the Cairo source filter, and the web sets `image-rendering` to `crisp-edges` where supported or `pixelated` otherwise. Pixel-buffer and offscreen sources go through the same draws and take the same filter; Android sets it on a pixel frame's drawable without reloading, and draws no offscreen images yet.
- `BlurRadius` is in points: Core Image on Apple, CSS's `blur()` on the web, `RenderEffect` on Android 12 and newer, before it a stack blur of the image shrunk to its drawn size, a Direct2D Gaussian blur on Windows and a GSK blur on Linux.
- Windows and Linux draw a blurred image enlarged under a clip to its own rectangle, so its edges stay opaque.
- A source that loads fades in over its placeholder for `FadeDuration`: 0 by default, as NSH's iOS, except Android's 300 ms. Windows and Linux fade on their frame clocks, even when the bytes came from the cache.
- Videos fit with `Fit` on Windows and Android; Apple's `AVPlayerLayer` and GTK's `GtkPicture` have no natural-size fit, so `Center` there shrinks as `ScaleDown` does, and Android's video shows both as `Contain`.

## Painted window hit tests

On macOS and Windows, `Image.HitTestAlpha` travels with `ImageConfig`. In a painted window, a positive threshold tests the decoded alpha at the pointer after undoing the view transform and fitting the active picture or placeholder. Transparent pixels pass the hit to the rest of the view tree, including when the image listens for a press; its background and borders still count under the normal painted rule. macOS keeps one alpha byte per decoded pixel and follows Core Animation's displayed contents. With an unblurred pixel buffer it reads the core's immutable RGBA copy directly. Windows reads an alpha byte from the current BGRA frame. The hit test samples the bitmap before compositing it with the rest of the view.

## Pixel buffers

`ImageSource.Pixels` is an `*image.RGBA` the app draws, premultiplied as Go's type is, with `Version` telling frames written into the same buffer apart (#107, NSH's `ImageSrc::rgba`, which takes straight RGBA it never copies: Go's own image type is what decoders hand an app, and the copy lets the app reuse one buffer). `Image` and `Canvas` show it, and `Shader` reads it, with nothing loaded or decoded, and a buffer with no pixels as no image:

- The core copies the pixels when the commit that carries a new buffer or `Version` runs, on the engine goroutine, into a buffer of the platform's at the origin with rows of 4 bytes a pixel (`driver.CopyPixels`). The app may write the next frame into the same buffer once that commit ran, from its next `RequestAnimationFrame` callback for one; a decoder on another goroutine writes into a second buffer and posts the swap. A commit that resends the view for another reason, such as its position, sends the same copy, so the platform keeps its frame; the same `Version` is not drawn again, however the buffer changed.
- Versions set between two commits coalesce into the last. The core copies each version it commits, a hidden or minimized window's too, although #107 asks that such a window copy nothing; the roadmap records it.
- On Apple platforms, Windows, Linux, Android and the web the platform swaps the frame in place: the previous one stays until the new one is drawn. Wherever they show, pixels come with no `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` or `OnLoadEnd` (the core drops any a backend reports for pixels), no `FadeDuration`, no placeholder and no cache entry. `ResizeMode`, `ObjectPosition`, corners, tint and blur apply as to any image. Without `Size` a buffer measures its pixels in points.
- It is an ordinary view, so views mounted after it, texts and controls and backdrop blurs, draw above it, unlike a `Native` component's child window on Windows.
- A canvas's `DrawImage`, `DrawImageIn` and `DrawImageFrom` copy the pixels they draw in the same way (`canvasNode.withFrames`), and draw them in the same play, never waiting for a load. Apple, Windows and Linux cache the latest copies their canvases drew, at most 8 and 8 Mi pixels together. Android keeps the bitmaps the current drawing uses and gives up the ones an updated drawing drops.
- A shader's `Images` copy theirs in the same way, image by image (`shaderNode.withFrames`), and a shader sent again for its uniforms sends the same copies. Each backend keeps a texture of premultiplied RGBA for each image of pixels, made at the copy's size and again only when the size changes, writes each new copy into it before the frame that reads it, and writes nothing for a copy it holds, so a new version costs one copy in the core and one upload.

| Target | Image | Canvas | Shader |
| --- | --- | --- | --- |
| Apple | A `CGImage` over a copy of the pixels (`pixelsImage`), set as the layer's contents on the main thread | The same image (`canvasFrame`) | `replaceRegion` on the layer's render goroutine once the frame drawn last finished |
| Windows | Premultiplied BGRA (`imagedecode.FromPixels`), a Direct2D bitmap with premultiplied alpha | The same | `UpdateSubresource` |
| Linux | Premultiplied BGRA, a Cairo ARGB32 surface | The same | `glTexSubImage2D` in the area's context |
| Web | One `<canvas>` per picture, updated with straight RGBA via `putImageData` and sized to the fitted rectangle with CSS | A cached `<canvas>` for each recent copy, filled through the same upload path | `texSubImage2D` from a `Uint8Array` kept for the image |
| Android | Premultiplied RGBA through the pipe before the batch, copied into the view's mutable `Bitmap` of the frame's size with `copyPixelsFromBuffer` | The same transfer for new copies, with bitmaps held by the drawing and reused by size | The bytes with the `ItemShader`, only for a copy Java lacks, at most 32 MiB of them in a batch (immediately following shader-only batches send copies that did not fit), and `glTexSubImage2D` on the GL thread |
| HarmonyOS | Not shown | Not drawn | No shaders |
| Mini programs | WeChat's 2D canvas node holds the current frame, updated with `putImageData`. QQ, Douyin and Alipay have no canvas view binding | Cached offscreen canvases filled with `putImageData`, replayed into the drawing's picture | No shaders |

On Android, each new copy is a separate pipe message, subject to the pipe's 64 MiB per-message limit. The reader stages the bytes and the next batch copies them into the bitmaps on the UI thread before drawing. Style updates and drawings that keep the same copy send no pixel bytes.

On a Pixel 3 XL running Android 12, a 960 × 540 image view rewrote 2,073,600 bytes per callback at 59–60 fps. An eight-second screen recording contained 477 consecutive version markers with their expected colors. After warm-up, across four 120-frame samples spanning the recording, writing each frame's header and bytes to the pipe took a median of 5.07–5.29 ms and a 95th percentile of 6.20–6.71 ms. These times cover the Go writes to the pipe, excluding the engine's copy, Java's bitmap update and rendering.

`Scope.ImageSize` answers a pixel source at once with its buffer's size, `PrefetchImage` with success, and `KeepImage` with a no-op release, without asking the platform.

## Size, prefetch and keeping

`Scope.ImageSize` and `Scope.PrefetchImage` are NSH's `Image.getSize` and `Image.prefetch`: `ItemImageInfo` carries the source, its `asset:` variant picked as for views, and the platform answers with `EventImageInfo`. The backends that load in Go call `imageload.Info`, so the bytes stay in `imageload`'s 32 MiB cache for the views that show them next, and read the size from the header (`imagedecode.Size`; ImageIO on Apple). Android answers in its Go process. The web loads an `<img>` outside the page and keeps the bytes in the browser's cache. The size is the one `OnLoad` reports.

`Scope.KeepImage` pins an encoded source's resolved `ImageSource.Key` in the backend's canvas and shader image cache (`ItemKeepImage` / `ItemReleaseImage`) so atlases and long-lived textures stay out of the 64-source unkept LRU bound until the returned `release` runs or the owning scope disposes ([canvas players](canvas.md#players)).

`imageload` turns a `file:` URL into a path by the rules `core.FileFromURL` uses (`driver.FileURLPath`): on Windows `file:///C:/...` keeps its drive letter and `file://server/share/...` becomes a UNC path; elsewhere the path is the URL's decoded path, and a host other than `localhost` fails rather than read a local file of the same path.

## Disk cache

Every native backend loads `http(s)` images through `imageload`, which keeps their bytes in a disk cache under the app's cache directory (`imageload.UseDiskCache`, 256 MiB), as NSH's `DiskCache`. Loads of one URL share a fetch, which stops once nobody waits for it; a load asked for after that starts a fetch of its own rather than fail with the cancelled one. The web leaves caching to the browser.

`imageload` downloads through the system's HTTP client, which each backend sets as `imageload.HTTP`, so apps link no Go HTTP and TLS stack: URLSession on Apple platforms, WinHTTP, libsoup 3 on Linux, and on Android the runtime's `HttpURLConnection`, which Java runs for the Go process (`msgFetch`, `fetch.go` and `Downloads.java`). Downloads follow the system's proxies, certificates and policies: a plain `http` image needs App Transport Security's exception on Apple platforms (`apple.info`) and cleartext traffic allowed on Android, as web views and videos do.

- The policy is RFC 9111's for a private cache, simplified as NSH's: `200` and `203` are kept unless `no-store`, `Vary: *` or over an eighth of the cache.
- A copy is fresh for `max-age`, else `Expires` less `Date`, else a tenth of the time since `Last-Modified`, at most 7 days, less its age; `no-cache` or no freshness makes it stale at once.
- A stale copy is revalidated with its ETag and `Last-Modified`: `304` refreshes its headers, `4xx` drops it, and a failed request or a `5xx` serves it unless `must-revalidate` or `no-cache`.
- Entries are keyed by URL. Each is one file named by a digest of it, with a CRC, written under a temporary name and renamed, so every process sees a whole entry or none; one that does not read back whole is deleted.
- The least recently used go first: a hit refreshes a file's time at most every 10 minutes, and past the limit a scan deletes the oldest down to three quarters, and temporary files over an hour old.

## Decoding

- Formats are sniffed from the bytes, never from names or types: `imagedecode.Sniff` finds an SVG after a byte order mark, an XML declaration, comments and a doctype, or inside gzip.
- At most as many images as processors decode at once, each in a slot of `imagedecode.Wait`, as NSH's decode on a dispatch queue the system sizes; Android's Java decodes bitmaps one at a time, from bytes the Go process sends it over the pipe: a message there is at most 64 MiB (`maxMessage`, `Protocol.MAX_MESSAGE_SIZE`), so the Go side refuses a larger one rather than end the app as a corrupt pipe would: a view's source fails with `OnError`, a canvas's image is logged and not drawn, and a font the app registers returns an error. A load cancelled while it waits never decodes, so a fast scroll down a grid of pictures decodes about the visible ones and stays near its resting footprint.
- `OnLoad` reports the source's size in pixels, whatever size a view decodes it at, and an SVG's intrinsic size in whole points.
- An animated image plays on its own in each view, with each frame's delay (10 ms or less shows for 100 ms, as in browsers) and its loop count. A tinted one shows its first frame, as NSH's does, and so does a blurred one on Apple platforms.
- `internal/platform/imagedecode` decodes in pure Go for the backends without a decoder of their own: PNG, JPEG, every frame of GIF and WebP animations (at most 600 frames and 48 MiB of them), and SVG through its own rasterizer within NSH's bounds ([icons](icons.md#svg-documents)).

| Target | Bitmaps and animations | SVG |
| --- | --- | --- |
| Apple | ImageIO, at the view's pixel size; GIF, WebP and APNG frames play as a discrete keyframe animation of the layer's contents | `imagedecode`, its shorter side as long as the view's longer side in pixels |
| Web | `<img>` | `<img>` from an `image/svg+xml` blob |
| Windows, Linux | `imagedecode`; frames on the backend's frame clock | `imagedecode`, 4 pixels per point, at most 2048 a side |
| Android | `ImageDecoder`, with `AnimatedImageDrawable` for animations | Rasterized by `imagedecode` in the Go process at the view's size when it loads, and sent as a bitmap |

## SVG drawing

`imagedecode` draws an SVG at the size NSH's `svg` module would: covering a view's pixel size, at its intrinsic size times a scale, or oversampled 4 times to at most 2048 pixels a side, within 8192 a side and 16 Mi pixels.

Its scanline rasterizer computes exact coverage across each row with 16 samples down it, flattens curves within 0.05 pixels, strokes with caps, joins and a miter limit of 4 as a union of convex pieces, and paints gradients through a 1024-color table. A document is at most 16 MiB, an `.svgz` once inflated, and drawing one is bounded by edge work, so a small file cannot ask for minutes.

## Verification

`go tool nsgo test ./internal/driver` checks the paths of Windows and other targets' file URLs on every host; `go tool nsgo test ./internal/platform/imageload` reads local files through their file URLs with `Load` and checks the disk cache's policy against a test server; each backend's tests hold its HTTP client to the same checks (`imageloadtest.CheckHTTP`). `go tool nsgo test ./internal/platform/imagedecode` checks decoded frames, rasterized pixels and the bounds, and pixel buffers' BGRA and PNGs; `go tool nsgo test ./core` checks that pixels are copied once per version, for image views, canvases and shaders, and fire no load events; `go tool nsgo test -target android -run TestPixelBuffersInImageAndCanvas ./internal/platform/android` checks changed pixels, alpha and the three canvas image operations through the Android pipe; `go tool nsgo test -target web ./internal/platform/web` checks 120 versions in one 960×540 canvas and the fit, tint and source changes; the Apple tests check an image view swapping pixels in place and a shader layer writing each version into the texture it keeps, which Metal then reads, and the Linux tests draw pixels' alpha over a color in an image view and a canvas; the Windows and Linux native tests check animated GIFs and SVG pixels in a window. The Windows and Linux native tests also check a fit at a position, `Center`'s natural size, blur, fade and `ImageSize` in a window. The gallery's Images page shows every resize mode, blur, placeholders and fades, `ImageSize`, `PrefetchImage` and progress, an animated GIF and WebP, and SVG, plain and tinted.
