### Added

- Ask for and learn Local Network access before an app's first LAN connection with `core.PermissionLocalNetwork`, which iOS and macOS 15 answer through a Bonjour probe whose type `nsgo build` adds to `NSBonjourServices`, and which Android, Windows and Linux answer not gated (#2347).
