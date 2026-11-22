### Added

- Deliver trusted web view input (`Ref.SendInput`, with `Env.CanSendInput`) on Windows (WebView2 `SendMouseInput` and `Input.dispatchKeyEvent`), Linux (WebKitGTK via X11 XTest and `InsertText` fallback) and iOS (`WKWebView` via `UITouch`/`UIEvent`, `UIKey`/`UIPress`/`UIPressesEvent` and `insertText:`) (#1587).
