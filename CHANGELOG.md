# Changelog

## [Unreleased]

- Add a reactive Go engine with owners, memos, effects and regions (`Show`, `Each`, `Dynamic`, `Catch`).
- Add block, flexbox and grid layout matching Taffy, with styles, per-side borders, state styles, transforms and transitions.
- Add images, rich text with links, text selection across texts, canvas, virtualized lists, overlays, modals and navigation stacks.
- Add `TextView` for Markdown with tables, alerts, footnotes, app extensions and highlighted code, rebuilding only the blocks that change and closing a streamed reply's open syntax.
- Add unstyled base controls, popovers, menus, tooltips, dialogs, sheets, alerts and safe-area views.
- Add the `ui` package: base controls and popups styled with a light and dark theme drawn from the icon.
- Add pan, pinch and rotation gestures.
- Add custom desktop title bars, window drag regions, window controls and an application menu.
- Add `core.FontMonospace` for the platform's fixed-pitch font.
- Add environment signals, keyboard shortcuts, clipboard and screen-reader announcements.
- Add key-value storage with change signals, and each app's data and cache directories.
- Add native backends for macOS, iOS, Windows, Linux and the web.
- Add `nsgotest` for headless app tests.
- Add the `nsgo` tool with `init`, `dev` and `build`, configured by `nsgo.yaml`.
- Add `nsgo dev` and `nsgo build` flags that override `nsgo.yaml`'s values.
- Add `nsgo dev`'s native target on Windows and Linux.
- Add `nsgo build` opening its output directory, unless `build.open` is false.
- Add `dev.hot_reload` to rebuild on save, since `nsgo dev` now runs the app once and exits with it.
- Add a layered app icon.
- Add the official website.
- License the project under Apache-2.0, with NSH's attribution in `NOTICE`.
- Fix Windows' aliased rounded corners, borders and canvas curves.
