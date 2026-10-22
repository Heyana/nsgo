# Roadmap

This plan lists what NSGo still lacks compared with NSH, in the order to build it. Each item names its NSH reference in `nsh/` and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

## Order

1. Add the services and text features most apps need.
2. Add media, embedding and native-driven animation.
3. Complete the tool, then add Linux and the embedding SDKs.
4. Finish the platforms NSGo already claims, so every target runs. This is left to community contributions.

## 1. Services and text

| Item | NSH reference | Done when |
| --- | --- | --- |
| SelectionArea | `crates/nsh-base/src/selection_area.rs`, `crates/nsh-core/src/selection.rs` | Static text can be selected and copied |
| TextView | `crates/nsh-base/src/text_view` | Markdown renders with code highlighting, links and extensions |

## 2. Media, embedding and animation

| Item | NSH reference | Done when |
| --- | --- | --- |
| WebView | `docs/dev/elements.md` | An element hosts the platform's web view on every target |
| Video | `docs/dev/elements.md` | Playback with progress, resize modes and media info |
| Animated and vector images | `docs/dev/images.md`, `docs/dev/elements.md` | GIF, animated WebP and SVG load on every target |
| File drop | `docs/dev/file-drop.md` | Desktop and web views receive dropped files |
| InputAccessoryView | `docs/dev/elements.md` | A view rides above the iOS and Android keyboard |
| Native components | `docs/dev/embedding.md` | Apps mount their own native views inside the tree |
| LayoutAnimation | `docs/dev/animation.md`, `crates/nsh-core/src/animation` | Layout changes and animated values run on the platform without a Go frame per step |

## 3. Tool, Linux and embedding

| Item | NSH reference | Done when |
| --- | --- | --- |
| `nsgo test` | `crates/nsh-cli/src/test*.rs` | Runs an app's `nsgotest` tests, and on simulators and browsers |
| Icons | `docs/dev/embedding.md` | `nsgo build` turns the app icon into each target's format |
| Store signing | `docs/dev/embedding.md` | `nsgo build` writes an IPA with a provisioning profile and an APK with a keystore |
| Linux backend | `docs/dev/linux.md`, `crates/nsh-platform-linux` | GTK 4 through purego runs the gallery |
| Embedding SDKs | `docs/dev/embedding.md` | iOS, Android and desktop host apps can show an NSGo view |

## 4. Finish the existing platforms

These items wait for community contributions; the items above do not depend on them.

| Item | NSH reference | Done when |
| --- | --- | --- |
| Android runtime | `crates/nsh-platform-android` | The Java runtime compiles; `classes.dex` is committed with the command that rebuilds it |
| Android packaging | `docs/dev/embedding.md`, `crates/nsh-cli/src/android.rs` | `nsgo build` writes a signed APK with nothing beyond Go and Xcode installed |
| iOS touch | `crates/nsh-platform-ios` | Gestures and text input are checked in the simulator |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler and `nsh check`, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and resources, since apps use `net/http` or Go libraries with `Scope.Post`.
