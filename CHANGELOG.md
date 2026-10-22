# Changelog

## [Unreleased]

- Add a reactive Go engine with owners, memos, effects and regions (`Show`, `Each`, `Dynamic`, `Catch`).
- Add block, flexbox and grid layout matching Taffy, with styles, per-side borders, state styles, transforms and transitions.
- Add images, rich text with links, text selection across texts, canvas, virtualized lists, overlays, modals and navigation stacks.
- Add `TextView` for Markdown and HTML, including tables, alerts, footnotes, disclosures, formulas the app renders, inline images, extensions, highlighted code and streaming.
- Add span backgrounds and views inside text that flow with its lines.
- Add unstyled base controls, popovers, menus, tooltips, dialogs, sheets, alerts and safe-area views.
- Add the `ui` package with themed controls and popups in light and dark.
- Add pan, pinch and rotation gestures.
- Add custom desktop title bars, window drag regions, window controls and an application menu.
- Add `core.FontMonospace` for the platform's fixed-pitch font.
- Add environment signals, keyboard shortcuts, clipboard and screen-reader announcements.
- Add key-value storage with change signals, and each app's data and cache directories.
- Add native backends for macOS, iOS, Windows, Linux, Android and the web.
- Add `android_x64` packages and `nsgo dev`'s choice of the connected device's ABI, so apps run on x86_64 emulators too.
- Add `nsgotest` for headless app tests.
- Add the `nsgo` tool with `init`, `dev` and `build`, configured by `nsgo.yaml`.
- Add app icons to every package, from `assets/` or `nsgo.yaml`'s `icon`.
- Add `resources/`, shipped in each package and loaded by `asset:` images.
- Add `nsgo build -target all` for every platform that needs only Go and Xcode.
- Add `nsgo dev` and `nsgo build` flags that override `nsgo.yaml`'s values.
- Add `nsgo dev`'s native target on Windows and Linux and its Android target on connected devices.
- Add `nsgo dev`'s iOS target on connected iPhones, signed with Xcode's automatic signing.
- Add `nsgo build` opening its output directory, unless `build.open` is false.
- Add `dev.hot_reload` to rebuild on save, since `nsgo dev` now runs the app once and exits with it.
- Add a layered app icon.
- Add the official website.
- License the project under Apache-2.0, with NSH's attribution in `NOTICE`.
- Fix Android builds failing because Go's implied `linux` tag included the Linux backend.
- Fix strikethrough missing on macOS and iOS.
- Fix Windows' aliased rounded corners, borders and canvas curves.
