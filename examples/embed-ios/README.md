# Embedding an NSGo app in an iOS app

`main.swift` is a UIKit app of its own that shows the NSGo app in
`examples/embedded` twice, below its own views.

1. Build the app as a framework: `go tool nsgo build -embed -target ios examples/embedded`.
   The output holds `Embedded.xcframework`, a static library for devices and the simulator
   with `NSGo.h`, and `EmbeddedResources`, the app's resources.
2. Add the framework to the app target ("Frameworks, Libraries, and Embedded Content"; it is
   static, so do not embed it) and `EmbeddedResources` as a folder reference, so it is copied
   into the bundle under that name. Swift then `import Embedded`.
3. Call `NSGoStart()` once as the app launches, before the first view: it starts the app's Go
   code and returns once the app is ready.
4. Make `NSGoView`s, a `UIView` each, and put them in your views. Each runs the app on its own
   from the moment it is in a window; it stops when it is deallocated or sent `stop()`.

Set what the app starts with before it is in a window, the rest at any time:

```swift
let app = NSGoView(frame: .zero)
app.params = ["user": "Ada"]                       // cx.HostParams()
app.dataDirectory = documents.path                 // where its storage lives
app.localeIdentifier = "de-CH"                     // in place of the system's
app.setFontScale(1.3)
app.overrideUserInterfaceStyle = .dark
app.tokens = ["accent": "#cf222e"]                 // cx.Env().Token("accent")
app.handle("time") { data, reply in reply(now(), nil) }      // cx.CallHost("time", …)
app.send("message", ["text": "Hello"])             // cx.OnHostEvent("message", …)
app.onOpenURL = { url in router.open(url) }        // cx.OpenURL, before the system
app.onError = { message in log(message) }
app.register(component: "host.badge") { context in Badge(context) }   // core.Native
```

Handlers, components and callbacks run on the main thread; `reply` may be called later from
any thread. Values cross as JSON: dictionaries, arrays, strings, numbers, booleans and
`NSNull`. A view controller that forwards the status bar returns the view's
`preferredStatusBarStyle` and `prefersStatusBarHidden`.

To try it without an Xcode project, compile `main.swift` for the simulator against the
framework's slice and bundle it with `Info.plist`:

```sh
FW=Embedded.xcframework/ios-arm64-simulator
xcrun --sdk iphonesimulator swiftc -target arm64-apple-ios15.0-simulator \
  -I $FW/Headers -L $FW -lEmbedded main.swift -o EmbedHost.app/EmbedHost
cp Info.plist EmbedHost.app/ && cp -R EmbeddedResources EmbedHost.app/
codesign --force --sign - EmbedHost.app && xcrun simctl install booted EmbedHost.app
```
