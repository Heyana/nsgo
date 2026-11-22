### Added

- Stream Linux and Android update downloads directly to disk with live progress via `updater.Status` and platform task progress, verifying size, SHA-256 and Ed25519 signatures from the staged file (#1420).
- Support direct Arch Linux (`.pkg.tar.gz`, `.pkg.tar.zst`) updates via `pkexec pacman` and atomic in-place AppImage self-updates with multi-package Linux appcasts (#1420).
