### Added

- Support `gpu.View` capture in `Scope.Capture` across macOS, iOS, Windows, Linux, Android and the web (preserving WebGPU swapchain frames for capture and shader effects), add Android overlay window capture, and add headless `nsgotest.App.CaptureSurface` (#1243).

### Fixed

- Download Linux GTK capture textures on the GTK main loop before background PNG encoding, fall back to WARP/PrintWindow on Windows when hardware WGC is unavailable, and answer `ItemCapture` with `CodeUnavailable` on the terminal backend (#1243).
