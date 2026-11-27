### Changed

- Composite `Canvas.DrawAtlas` batches in pure Go on macOS and iOS—writing directly into compatible sRGB/DeviceRGB bitmap contexts and falling back to a single-blit scratch image for async display-list layers and clipped states—replacing per-sprite `CGContextDrawImage` calls (nsgo-dev/game#64).
