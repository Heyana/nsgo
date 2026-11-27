### Fixed

- Preserve transparent `Video` compositing across backends by making Android's `TextureView` non-opaque, clearing `GtkVideo`'s black default background on Linux when `Controls` is set, explicitly clearing `AVPlayerLayer` and iOS `AVPlayerViewController` view backgrounds on Apple, and setting `<video>` backgrounds transparent on the web (#1360).
