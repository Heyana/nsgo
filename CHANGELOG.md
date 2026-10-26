# Changelog

## [Unreleased]

- Add a reactive Go engine with owners, memos, effects and regions that lays out and sends only what each update changed, to native views in one batch.
- Add native backends for macOS, iOS, Windows, Linux, Android and the web, every one built from a Mac with Go and Xcode.
- Add block, flexbox and grid layout with static, sticky and fixed positions, intrinsic sizes, stacking order and right-to-left subtrees.
- Add CSS-like styles: corners, borders, outlines, shadows, gradients, filters, backdrop blur, masks, blend modes, 3D transforms, state styles and the SVG paint canvases draw with.
- Add transitions, keyframes, animated values, motion presets and layout animations that run on each platform's frame clock.
- Add selectable, hit-testable rich text with the app's fonts, inputs, GIF, WebP and SVG images, Canvas 2D drawing, GPU shaders written in Go and Lucide icons.
- Add scroll views with pull to refresh and the app's own refresh indicators, virtualized lists, overlays, modals, navigation stacks with system headers, the system tab bar, web views, videos and native views.
- Add pointer, touch, focus, keyboard and gesture input, file drops, accessibility, storage and environment signals.
- Add clipboard images and files, opening links, the photo picker, forced light or dark appearance, toasts and a disk cache for downloaded images.
- Add custom title bars, window frames and levels, application and context menus, scroll bars and window materials, and return from `nsgo.Run` on quit.
- Add `base`: unstyled controls, popups, menus that become sheets on phones, dialogs, sheets, safe areas, resizable panels, a dock and a streaming `TextView`.
- Add embedding in other apps with `nsgo.NewView` on desktops and `nsgo build -embed` frameworks for iOS and Android, with a channel to the host.
- Add `nsgotest` for app tests that act as a user does, headless or on every platform with its native input and screenshots.
- Add the `nsgo` tool, which creates, runs with hot reload, tests, packages and signs apps for stores from `nsgo.yaml`.
- Add NSH's gallery translated to Go, with a demo of every element, component and service; its web build is the website.
