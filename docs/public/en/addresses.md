---
title: Application addresses
description: Read and update a web app's address through browser history.
group: Core concepts
order: 59
---

`cx.AppURL()` reads the application's address as a tracked value. On the web the initial page URL is available before the root renders, so a reload or shared link can choose the same view. An app decides how the URL maps to its views.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` updates the tracked address immediately without requesting a page or navigating a stack. It replaces the current browser entry by default; `HistoryPush` requests another entry. A navigation or modal push in the same commit supplies that entry instead, so one action does not create two back steps. Several writes in a commit keep the last address and preserve any push request.

Relative addresses resolve against the current address, or an explicit HTML base. A browser address must stay on the page's origin. Invalid addresses, whitespace, control characters, backslashes, and addresses over 16 KiB return an error without changing it.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` observes browser traversals and hash edits. `change.Reason` is `"back"`, `"forward"` or `"address"`; the returned function unsubscribes, and scope disposal removes the listener. Read `AppURL` for the initial address. The app's own writes update bindings but do not send a change notification. A traversal that keeps the same address sends no change.

The browser's back still dismisses a modal or pops a navigation screen. A back the app refuses restores the existing entry and its address. A dismissal handler can set its own replacement address. A programmatic pop updates the address after the asynchronous history traversal, and writes arriving during that traversal wait for it. Going back past this player's entries leaves the app, including entries retained from a previous load.

For paths such as `/films/42`, set the deployed app directory and have the server serve its `index.html` for application routes:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` writes an HTML base so scripts, styles, Wasm and resources still load when a deep address is opened directly. Without it, the existing relative asset paths are kept; query or hash addresses can use those unchanged. Server fallback rules must leave real asset requests alone. A strict Content Security Policy must allow this same-origin base (`base-uri 'self'`).

An app embedded in another page keeps its address internally and leaves the containing page's history alone. A same-origin iframe can explicitly opt in with `data-nsgo-history`. Confined subtrees cannot read or change the application's address.

On other targets, `AppURL` keeps the last link opened with the app or the value `SetAppURL` stored; setting it changes no native window or system browser. Mini programs keep this value too, without changing their host's routes.

TSX uses the same state:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
