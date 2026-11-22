# Mini programs

The `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` and `miniprogram_douyin` targets run a Go app in a mini program, as NSH's community platform `nsh-platform-miniprogram` runs NSH's Rust apps in WeChat's. Read this before changing `internal/platform/miniprogram`, its runtime in `cmd/nsgo/miniprogram` or how the tool builds it.

## Layers

A mini program has two layers: the logic layer runs JavaScript without a DOM, and the render layer, a web view, draws the page's WXML from the data the logic layer sets with `setData`. NSGo runs entirely in the logic layer:

```text
logic layer (miniprogram/common)
  app.wasm.br           The engine and the app, Go for js/wasm, tagged nsgominiprogram
                        and nsgo_noshader, loaded with WXWebAssembly or its likes
  nsgo/wasm_exec.js     Go's own, as a function of the globals it reaches
  nsgo/runtime.js       The bridge (nsgoMP), the offscreen canvas text is measured on,
                        the device, and the events of the page
  nsgo/api.js           The platform's APIs with WeChat's names, options and results
  nsgo/tree.js          The views as plain objects; each turn's batches become the
                        fewest setData paths
render layer (miniprogram/wechat, miniprogram/alipay)
  pages/index           The root view, then the modals, and a page container that
                        takes the back gesture while back goes through the app
  components/nsgo-node  A view at its frame, with its style, runs, picture, image or
                        field, and its children: itself again
```

- `mount.go` turns each batch into operations (its comment lists them) and `touch.go` the render layer's touches into presses, pans, pinches and `EventTouch`. Both are plain Go over a `host`, tested on any machine; `miniprogram.go` and `services.go` are the bridge.
- Frames are placed in the parent's padding box, as CSS places absolute children, so a child's frame is sent less its parent's border.
- Text is prepared, measured and laid out in Go (`text.go`): widths come from the logic layer's canvas, cached by font and text; lines break greedily after spaces and around CJK characters, spans keep their styles on one baseline, and attachments take their placeholders' places. The render layer draws each run with `white-space: pre` where it was placed, so both layers name their fonts rather than `sans-serif`, which they resolve differently. Span backgrounds are separate rounded boxes below the glyph runs, clipped to each line’s visible characters and inset against the font’s ascent-to-descent box. Painting a span does not split its measured runs or change kerning and line breaks.
- Canvases are played into one offscreen canvas with `canvas2d`, the web's player, and shown as pictures, which move and scroll as views do. Pixel buffers are uploaded into cached offscreen canvases and drawn from them.
- A WeChat pixel image mounts one 2D canvas at the picture's fitted rectangle. Its component hands the canvas node to the logic layer, which keeps a typed array and writes each version with `putImageData`. Only the placement goes through `setData`. QQ, Douyin and Alipay still lack the image-view binding. WeChat DevTools 2.01.2510290 read back 120 versions of a 32×32 buffer in an image view and in an offscreen drawing, matching every frame's color.
- Animated values run on the engine's `Animator`, frames timed by the runtime, and go to the render layer as an inline style over the view's. Keyframes and modal presentations run in the render layer through the component's `animate`, with no work per frame.
- Styles are the web's CSS, written by `webstyle`.
- The environment reports the device's OS as `Simulated`, as a mini program has none of its native parts: a stack's pages show their `Header`, as no system header stands in for it, `mobile.IsAvailable` is false, and "mod" is Command on iOS and macOS, as `AppleKeyboard` says.

## Platforms

One module and one logic layer serve every platform; `config.js` names the platform, and the tool writes its render layer and project files:

| Platform | API object | WebAssembly | Render layer | Project file | Tool |
| --- | --- | --- | --- | --- | --- |
| WeChat | `wx` | `WXWebAssembly` | `wechat/` | `project.config.json` | DevTools' command line |
| QQ | `qq` | `QQWebAssembly`, then `WebAssembly` | `wechat/` as `.qml`, `.qss`, `qs` and `qq:` | `project.config.json` | its app on macOS |
| Douyin and ByteDance's other apps | `tt` | `TTWebAssembly`, then `WebAssembly` | `wechat/` as `.ttml`, `.ttss`, `sjs` and `tt:` | `project.config.json` | its app on macOS |
| Alipay | `my` | `WebAssembly` | `alipay/` | `mini.project.json` | its app on macOS |

WeChat DevTools is refreshed with `cli close --project <dir>` followed by `cli auto --project <dir>` for every `-run` and `dev` build, as `cli open` neither compiles a project nor loads a rebuild into one already open; checked with DevTools 2.01.2510290. The other tools are opened once a `dev` session.

- Go calls the platform only through `nsgoMP.api`, which `api.js` makes from its object: WeChat's as it is, QQ and Douyin's with `chooseImage` standing in for `chooseMedia` where it is missing, and Alipay's translated call by call (`confirm` and `alert` for `showModal`, `{ key }` storage, `setNavigationBar`). An API a platform lacks stays undefined, and Go answers its request as failed.
- The runtime tries the platform's loaders in order, each with the module's path in the package; a platform that loads none fails to start, with the loaders it tried.
- Alipay names handlers statically and has no `mut-bind`, page container or component `animate`: every view binds every handler and `nsgo-node` keeps an event for the innermost view that takes it, by its time; a capturing view is a branch of its own with `catchTouchMove`; its scroll views' touches stand for their drags; back leaves the page and keyframes do not run. Events reach the page through the `onNsgo` prop each parent hands its children, as Alipay's components trigger no events their pages hear.

## Input

Taps go to the innermost view that takes them (`catchtap`), presses going down and up to the innermost that follows them (`mut-bind`), and touches to every view that follows them: the runtime numbers each touch event, so the views it bubbles through add nothing, and a finger belongs to the view it went down on. A pan that fails on vertical drags leaves them to the scroll views around it; others capture the drags they start (`catchtouchmove`), as a mini program cannot decide on a drag as it moves.

Text fields are WeChat's `input` and `textarea` in their content box, which leave the page in place (`adjust-position`) and report the keyboard's height to the environment instead. Scroll views are `scroll-view`s in enhanced mode, reporting drags, their offsets and an end 150 ms after the last.

## Services

Alerts are the platform's modal or action sheet, the clipboard holds text, storage is the mini program's under the app's prefix, photos come from `chooseMedia` and files from a chat (`chooseMessageFile`, which Alipay lacks). A link goes on the clipboard, as a mini program opens no browser. Requests the platform has no API for are answered as failed.

`Scope.SignInMiniProgram` requests a short-lived host login code: `wx.login`, `qq.login`, `tt.login`, or Alipay's `my.getAuthCode` with `auth_base`. It has its own `AccessMiniProgramSignIn` gate and environment capability, false on other backends. The runtime's `signin.js` normalizes the reply and answers once; the Wasm bridge reports a typed credential through `EventMiniProgramSignIn`. The app exchanges the code on its own server, which holds the platform app secret. No client-side identity or token is inferred from the code.

NSGo keeps this separate from `AuthSession`: these hosts neither open its authorization URL nor return an OAuth callback, so silently reusing that API would discard the app's URL/PKCE/state contract. Douyin's anonymous device code is never a signed-in user code. Its login is silent unless the app explicitly sets `MiniProgramSignInOptions.Prompt`; an unsigned host answers `MiniProgramSignInNotSignedIn`, and documented host-login cancellation code 113001 answers cancelled. Other failures return stable errors without credential-bearing host payloads. See [Douyin login](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/api/open-interface/log-in/tt-login) and [Alipay authorization codes](https://pds.alipay.com/docs/miniprogram/mpdev/API_OpenAPI_getAuthCode).

## The package

`WXWebAssembly` and its likes load only files of the code package, `.wasm.br` among them, and every platform holds each package to 2 MB, so the module goes in a subpackage of its own, which the page loads before it, and its size decides which apps fit:

- `nsgo_noshader` leaves the shader compilers out of `core` (`shader_nocompile.go`), as the render layer draws no shader: programs compile to none and take any uniform.
- The module is built with `GOEXPERIMENT=nojsonv2`, encoding/json's smaller first implementation, as every app's is where it can be ([packages](tool.md#packages)), and again without inlining when it is still too large.
- TSX does not run: a mini program evaluates no code it did not ship, which the page's engine needs, and goja does not fit. The tool refuses apps that link `tsx`.
- Under `nsgo dev` (without `-release`), the module builds with `nsgodev`, stripped with `-s -w` without `-trimpath` so DevTools keep local source links while staying within the 2 MB cap. `internal/devtools/fetch_miniprogram.go` talks to the dev server through `nsgoMP.api.request` (chunked streaming for SSE events, normalized for Alipay in `api.js`), avoiding `net/http`, `runtime/pprof` and `runtime/trace`, so `-devtools`, `-highlight`, `-layout` and `-trace` reach mini programs while release builds link none of the dev client.

On 2026-11-09, `examples/embedded` compressed to 1.9 MB and `examples/gallery-miniprogram` to 2.0 MB, about 40 KB short of the limit; those are historical measurements, not current package guarantees. NSH's gallery in Go does not fit, and neither does an app of `ui`'s controls: a `ui.Button` links the SVG parser and `encoding/xml` for its busy spinner, so the mini program gallery styles `base`'s components as `ui` does.

Formula packaging was measured separately for [#1442](https://github.com/nsgo-dev/nsgo/issues/1442) on 2026-11-23, using Go 1.27.4, Untie 0.5.0, release stripping, `nsgominiprogram,nsgo_noshader`, `GOEXPERIMENT=nojsonv2` and the tool's Brotli quality 11. The limit is 2,097,152 bytes for the module's package. The app fixtures use name `FormulaAudit`, ID `dev.nsgo.audit1442` and version `0.1.0`, with one `nsgo.Run` surface: `core.Text{Content: "Hello"}`, or `formula.View{Source: "E=mc^2"}` with the default font. The third fixture links Untie alone, loads the same font, parses TeX and reads the layout bounds; it is a size probe, not an NSGo app.

| Fixture | Compressed bytes | Without inlining |
| --- | ---: | ---: |
| One text | 2,399,441 | 2,134,586 |
| One formula | 4,649,775 | 4,106,282 |
| Untie alone | 2,907,534 | 2,569,923 |

The font is 1,432,068 bytes before compression. An optimistic probe zeroed its complete embedded data in the non-inlined modules and compressed them again with the same Go Brotli implementation: the formula app still takes 3,336,609 bytes, and Untie alone 1,814,175. Those probes cannot render with the zeroed font; they isolate the embedded font data's compression contribution. A smaller font or an external font alone therefore cannot make the current formula app fit.

The one-text fixture is already 37,434 bytes over the limit, so [#1505](https://github.com/nsgo-dev/nsgo/issues/1505) remains a dependency. A separately packaged math module plus font data is a possible direction, not an implemented or verified solution: it needs a module interface, lifecycle and font-resource loading beyond the current single-module bridge. Adding subpackages alone does not split a Go-linked module. The existing bridge calls a packaged path, as [WeChat's instantiate signature](https://github.com/wechat-miniprogram/api-typings/blob/master/types/wx/lib.wx.wasm.d.ts) takes; alternate buffer-based loaders have not been verified. [Douyin's mini-program rules](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/tutorial/basic-ability/subpackages/introduction/) also cap each subpackage at 2 MB; its mini-game packaging rules are different. These measurements establish a packaging blocker, not formula rendering acceptance on any mini-program platform.
