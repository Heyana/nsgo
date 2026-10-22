# Roadmap

This plan lists what NSGo still lacks compared with NSH, in the order to build it. Each item names its NSH reference in `nsh/` and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

## Order

1. Complete text: what `TextView` and `Text` still lack.
2. Add media, embedding and native-driven animation.
3. Complete the tool, then add the embedding SDKs.
4. Finish the platforms NSGo already claims, so every target runs. This is left to community contributions.

## 1. Text

| Item | NSH reference | Done when |
| --- | --- | --- |
| Streaming repair | `crates/nsh-base/src/text_view/markdown/streaming.rs` | A streamed reply never shows syntax its end leaves open, such as `**` or a table's header alone |
| HTML in text views | `crates/nsh-base/src/text_view/html` | `TextView` reads HTML documents and the HTML inside Markdown |
| Formulas | `docs/dev/text-view.md` | `$…$` and `$$…$$` reach the app's renderer, else show as TeX |
| Inline attachments | `docs/dev/text.md` | Views, images and custom nodes flow inside a text's lines |
| Span backgrounds | `docs/dev/text.md` | Spans draw backgrounds, so inline code sits on a tint |
| Strikethrough on Apple platforms | `crates/nsh-platform-ios/src/text.rs` | CoreText text draws its struck spans |

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

## 3. Tool and embedding

| Item | NSH reference | Done when |
| --- | --- | --- |
| `nsgo test` | `crates/nsh-cli/src/test*.rs` | Runs an app's `nsgotest` tests, and on simulators and browsers |
| Icons | `docs/dev/embedding.md` | `nsgo build` turns the app icon into each target's format |
| Store signing | `docs/dev/embedding.md` | `nsgo build` writes an IPA with a provisioning profile and an APK with a keystore |
| Embedding SDKs | `docs/dev/embedding.md` | iOS, Android and desktop host apps can show an NSGo view |

## 4. Finish the existing platforms

These items wait for community contributions; the items above do not depend on them.

| Item | NSH reference | Done when |
| --- | --- | --- |
| Android runtime | `crates/nsh-platform-android` | The Java runtime compiles; `classes.dex` is committed with the command that rebuilds it |
| Android packaging | `docs/dev/embedding.md`, `crates/nsh-cli/src/android.rs` | `nsgo build` writes a signed APK with nothing beyond Go and Xcode installed |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and text selection are checked in the simulator; selections have touch handles |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler and `nsh check`, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and resources, since apps use `net/http` or Go libraries with `Scope.Post`.
