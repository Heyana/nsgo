### Changed

- Name a key pressed by `nsgotest`'s `KeyCode`, `KeyCodeDown` and `KeyCodeUp` under `-target` with the platform's current keyboard layout on macOS, Windows, Linux and the web, so `KeyW` is `z` on AZERTY, and keep a modifier held by both `KeyDown` and `KeyCodeDown` down until both release it (#1242 by @houko).

### Fixed

- Label the Shift, Control and Alt keys in `core.KeyCodeLabel` on Windows (#1242 by @houko).
