# Roadmap

This plan lists what NSGo still lacks compared with NSH, in the order to build it. Each item names its NSH reference in `nsh/` and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

## Order

1. Complete the tool, then add the embedding SDKs.
2. Finish the platforms NSGo already claims, so every target runs. This is left to community contributions.

## 1. Tool and embedding

| Item | NSH reference | Done when |
| --- | --- | --- |
| `nsgo test` | `crates/nsh-cli/src/test*.rs` | Runs an app's `nsgotest` tests, and on simulators and browsers |
| Store signing | `docs/dev/embedding.md` | `nsgo build` writes an IPA with a provisioning profile and an APK with a keystore |
| Embedding SDKs | `docs/dev/embedding.md` | iOS, Android and desktop host apps can show an NSGo view |

## 2. Finish the existing platforms

These items wait for community contributions; the items above do not depend on them.

| Item | NSH reference | Done when |
| --- | --- | --- |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and text selection are checked in the simulator; selections have touch handles |
| Linux web views and videos | `crates/nsh-platform-linux/src/native` | Web views and playback are checked on a machine with WebKitGTK 6.0 and GStreamer's decoders |
| Real file drags | `docs/dev/file-drop.md` | A real drag from Finder, Explorer and a Linux file manager is checked to reach a view, beyond the tests that drive the drop targets |
| Android before API 30 | `crates/nsh-platform-android` | The keyboard accessory bar is checked on API 28 and 29, which tell the keyboard's height by older insets |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler and `nsh check`, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and resources, since apps use `net/http` or Go libraries with `Scope.Post`.
