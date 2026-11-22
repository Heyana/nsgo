### Changed

- Overlay `ui.NavPage`'s Android toolbar transparently over the page with content scrolling under a progressive `RenderEffect` blur on Android 12+ (and a translucent gradient scrim on older versions), matching iOS while respecting explicit custom header backgrounds (#1608).
