# Web views and videos

`WebView` and `Video` host a platform control inside a view. Each follows NSH's design and behavior on every target; NSGo changes only the API's shape to Go's. Read this before changing either element or a backend's host for it.

## Shared design

- The control fills a `KindWeb` or `KindVideo` view, which keeps the decoration, frame and accessibility.
- One config item carries the whole state: `ItemWeb` with `WebConfig`, `ItemVideo` with `VideoConfig`. Commands ride their own items: `ItemWebCommand` for `Ref`'s history, scripts and messages, `ItemVideoSeek` for `Ref.Seek`.
- Core resolves defaults before sending, so platforms see final values: a video's volume, rate, progress interval and resize mode (Contain).
- Platforms report an event only when the view's `EventMask` has it.
- NSGo keeps Go's zero values as defaults: `NoScroll` and `NoJavaScript` invert NSH's `scrollable` and `javaScriptEnabled`, and `window.nsgo` replaces `window.nsh`.
- NSH's navigation check against an app's `webview` capability is not ported; see [Open questions](../../ARCHITECTURE.md#open-questions).

## Web views

- Every page starts with `WebBridge`, defining `window.nsgo.postMessage(data)`, which reaches `OnMessage` as a string. `PostMessage` dispatches a `MessageEvent` on the page's window.
- A page loads again only when the source or an injected script changes. Other settings apply to the live view.
- Load start and load each come with a navigation state change, carrying `WebNavigation`. Errors arrive as `*WebError` with the failing URL.

| Target | Host |
| --- | --- |
| Apple | One `WKWebView` host in `platform/internal/apple`; `NoJavaScript` sets the live configuration's webpage preferences, which apply from the next page |
| Windows | A WebView2 composition controller in a DirectComposition visual, between layers of what is drawn after it; the backend hit-tests and forwards input. The Evergreen runtime is found in the registry and its DLL called as WebView2's loader would, so nothing ships with the app |
| Linux | WebKitGTK 6.0 hosted in the view's widget, mapped once the view has a size, with website data per app ID and NSH's history keys and mouse buttons |
| Android | `android.webkit.WebView` with `window.nsgo` as a JavaScript interface; scripts meant for the document's start run when the page starts, as android.webkit has no earlier hook |
| Web | An `<iframe>`. HTML loads as `srcdoc` in a sandbox with an origin of its own and gets the bridge over a `MessageChannel`; a URL gets only `PostMessage` and load events, as the browser hides the page |

## Videos

- A source is a URL, a file path or an `asset:` path resolved against the app's resources.
- `EventLoad` carries the duration and the picture's size in points; `EventProgress` comes every `ProgressInterval` while playing.
- `EventEnd` follows the platform: AVFoundation and Android report every end, then a repeating video starts over; a looping Media Foundation engine, `<video>` or GTK stream never ends.
- `Controls` shows the platform's controls. Windows has none, so it draws NSH's bar.

| Target | Player |
| --- | --- |
| Apple | `AVPlayer` in an `AVPlayerLayer`, or `AVPlayerView` and `AVPlayerViewController` with controls; AVFoundation and AVKit load on the first video |
| Windows | Media Foundation's media engine in frame-server mode, drawing each frame as a Direct2D bitmap |
| Linux | `GtkMediaFile` (GStreamer) in a `GtkPicture` hosted in the view's widget, or a `GtkVideo` with controls; GTK streams have no playback rate |
| Android | `MediaPlayer` on a `TextureView`, with a `MediaController` for controls |
| Web | A `<video>` element |

## Known gaps

- Linux keeps presses, hovers and wheels from a page or a video's controls where the app does not show them, as NSH does; drags that begin there are not followed.
