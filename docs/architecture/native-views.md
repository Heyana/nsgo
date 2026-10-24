# Hosted views

`WebView`, `Video` and `Native` host a platform view inside a view: the platform's web view and player, or one the app registered. Each follows NSH's design and behavior on every target; NSGo changes only the API's shape to Go's. Read this before changing these elements or a backend's host for them.

## Shared design

- The control fills a `KindWeb`, `KindVideo` or `KindNative` view, which keeps the decoration, frame and accessibility.
- One config item carries the whole state: `ItemWeb` with `WebConfig`, `ItemVideo` with `VideoConfig`; `ItemNative` carries a component's changed props. Commands ride their own items: `ItemWebCommand` for `Ref`'s history, scripts and messages, `ItemVideoSeek` for `Ref.Seek`, `ItemNativeCommand` for `Ref.Command`.
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

## Native components

An app registers its own platform views by name, as NSH's embedding apps do (`nsh/docs/dev/embedding.md`): `nsgo.Run(app, nsgo.Component(name, factory))` hands each backend a `core.NativeComponents` table, so a registry belongs to one surface, never to the process.

- `Native` names the component; its `Props` cross only when they change, a removed one as nil, and `Ref.Command` reaches the component's `Command`. The first `ItemNative` makes the component with a `NativeContext`, whose `Emit` reaches `OnEvent` and whose `SetSize` gives the element a natural size, as an image's.
- Factories run on the main thread and may return nil; an unknown name leaves an empty box and is logged once.
- Registering differs per platform only in what `View` returns, so an app registers a target's components in files built for it, as the base gallery's `components_*.go` do.

| Target | `View` returns | Hosted as |
| --- | --- | --- |
| Apple | An `NSView` or `UIView` | A subview resizing with the view |
| Windows | A window handle | A child window following its view after every frame, clipped with a window region to what its clipping ancestors leave visible, above what NSGo draws, as NSH's `NativeComponent::window` |
| Linux | A `GtkWidget` pointer | A widget hosted in the view's widget, as web views and videos are |
| Web | A DOM element (`js.Value`) | A child element filling the view |
| Android | The name of a `View` class | Built by reflection in the Java runtime, props set through its one-argument `set` methods and commands through its methods; the Go process cannot make views |

On Android the runtime also installs a listener on each `setOn…Listener` method the view's own class declares and reports its callbacks as events named after them, their arguments as a JSON array. A component may implement `android.PropMapper` to turn a prop into ordered setter calls, and `android.EventMapper` to receive those callbacks itself, as the base gallery's `SeekBar` does.

## Known gaps

- Linux keeps presses, hovers and wheels from a page or a video's controls where the app does not show them, as NSH does; drags that begin there are not followed.
