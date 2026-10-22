# Roadmap

This plan lists what NSGo still lacks compared with NSH, in the order to build it. Each item names its NSH reference in `nsh/` and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

## Order

1. Add media, embedding and native-driven animation.
2. Complete the tool, then add the embedding SDKs.
3. Finish the platforms NSGo already claims, so every target runs. This is left to community contributions.

## 1. Media, embedding and animation

| Item | NSH reference | Done when |
| --- | --- | --- |
| Video | `docs/dev/elements.md` | Playback with progress, resize modes and media info |
| Animated and vector images | `docs/dev/images.md`, `docs/dev/elements.md` | GIF, animated WebP and SVG load on every target |
| File drop | `docs/dev/file-drop.md` | Desktop and web views receive dropped files |
| InputAccessoryView | `docs/dev/elements.md` | A view rides above the iOS and Android keyboard |
| Native components | `docs/dev/embedding.md` | Apps mount their own native views inside the tree |
| LayoutAnimation | `docs/dev/animation.md`, `crates/nsh-core/src/animation` | Layout changes and animated values run on the platform without a Go frame per step |

## 2. Tool and embedding

| Item | NSH reference | Done when |
| --- | --- | --- |
| `nsgo test` | `crates/nsh-cli/src/test*.rs` | Runs an app's `nsgotest` tests, and on simulators and browsers |
| Store signing | `docs/dev/embedding.md` | `nsgo build` writes an IPA with a provisioning profile and an APK with a keystore |
| Embedding SDKs | `docs/dev/embedding.md` | iOS, Android and desktop host apps can show an NSGo view |

## 3. Finish the existing platforms

These items wait for community contributions; the items above do not depend on them.

| Item | NSH reference | Done when |
| --- | --- | --- |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and text selection are checked in the simulator; selections have touch handles |
| Linux web views | `crates/nsh-platform-linux/src/native/web.rs` | Web views are checked on a machine with WebKitGTK 6.0 |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler and `nsh check`, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and resources, since apps use `net/http` or Go libraries with `Scope.Post`.
