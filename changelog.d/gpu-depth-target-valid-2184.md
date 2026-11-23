### Fixed

- `gpu.RenderTarget.Valid` and `Epoch` work for depth-only render targets: `Valid` checks every attachment the target has, and `Epoch` falls back to the depth attachment, so a depth-only shadow-map target is no longer reported invalid (#2184).
