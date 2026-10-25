# Changelog

## [Unreleased]

- Add a reactive Go engine with owners, memos, effects and regions that commits each update to native views in one batch.
- Add native backends for macOS, iOS, Windows, Linux, Android and the web, every one built from a Mac with Go and Xcode.
- Add block, flexbox and grid layout with static, sticky and fixed positions, intrinsic sizes, stacking order and right-to-left subtrees.
- Add CSS-like styles: per-corner and continuous corners, dashed borders, outlines, layered shadows, gradients, filters, backdrop blur, masks, blend modes, 3D transforms, state and group styles, transitions and keyframes, with animated values, keyframes, motion presets and layout animations on each platform's frame clock.
- Add rich and selectable text with the app's own fonts, inputs, images including GIF, WebP and SVG, Canvas 2D drawing with hit tests, accessible parts and animated arguments, GPU fragment shaders written in Go that read images, and SVG icons with the bundled Lucide pack.
- Add scroll views, virtualized lists that load more at either end, overlays, modals, navigation stacks with measured system headers and collapsing large titles, the system tab bar, web views, videos and the app's own native views.
- Add pointer, touch, focus, keyboard and gesture input, file drops, accessibility, clipboard, storage and environment signals.
- Add custom desktop title bars, window controls, application menus, scroll bars and window materials.
- Add `base`, unstyled buttons, checkboxes, radios, switches, tabs, progress bars, collapsible regions, menus and context menus that become sheets on touch screens, popovers, tooltips, dialogs, sheets that follow drags, alerts and action sheets, safe areas, resizable panels, a dock whose panels keep their state as they move and that reads and writes gpui-base's layouts, and a Markdown and HTML `TextView` that streams with a caret.
- Add `nsgotest` for headless app tests.
- Add the `nsgo` tool, which creates apps, runs them with hot reload and a performance monitor, and packages them from `nsgo.yaml`.
