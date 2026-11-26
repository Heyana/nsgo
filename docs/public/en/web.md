---
title: Publishing on the web
description: Publish an NSGo app with document semantics, page metadata and CDN assets.
group: Toolchain
order: 24
---

## Build the app

```sh
nsgo build -target web
```

Serve the generated directory over HTTP. Release assets have content hashes in their names, so cache them as immutable; revalidate `index.html`, `nsgo.json` and `manifest.webmanifest`. The web app keeps NSGo's layout, scrolling, focus and pointer behavior.

## Headings and links

A `core.View` or `core.Text` with `Role: core.RoleHeader` becomes `h1` through `h6`, selected by `AccessibilityLevel`; without a level it is `h2`. Markdown headings from `base/textview` carry their levels. The browser's heading margins and fonts do not change the app's styling.

`base.Link` and `ui.Link` publish their `Href` as an anchor. An ordinary click runs the app's `OnOpen` and `OnPress`; a middle click or a click with a modifier uses the browser's link action. The browser supplies its link context menu unless the app handles `OnContextMenu`. A disabled link has no active address. For a custom link-role view, set `core.View.Web` to `core.WebViewProps{Href: "/notes"}` or bind `HrefFunc`. TSX uses `web={{href: "/notes"}}`; headings use `accessibilityLevel={1}`. Unsafe executable URL schemes are refused. Confined content keeps navigation in its scoped press handlers and advertises no browser address.

## Page metadata

Import `github.com/nsgo-dev/nsgo/web` and place a `web.Head` beside a page's content:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` and `DescriptionFunc` follow signals. Later visible heads override the fields they set and inherit the others. Hiding or removing a page restores earlier heads; removing every head restores the original document. An empty static field inherits; a function returning an empty string clears it. A head adds no layout node. Native targets, embedded apps and confined content leave their host's document alone.

In TSX, import `Head` from `nsgo/web` and write `<Head title={title()} description="Your saved notes" />`. For static HTML previews, prerender each URL as part of the site's deployment; `Head` updates the running document.

## Serve assets from a CDN

The loader resolves all app files relative to `nsgo.json`. A page can load the app from another origin:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Allow the page's origin to fetch the CDN's files with CORS, and serve WebAssembly as `application/wasm`. Serve app entry HTML for any deep routes your app uses. [Embedding](embedding.md#web) shows how to mount into an element instead.
