### Added

- Press physical key positions with `nsgotest`'s `KeyCode`, `KeyCodeDown` and `KeyCodeUp` under `-target` on macOS, Windows, Linux, the web, iOS, Android and HarmonyOS (#1242 by @houko).

### Fixed

- Report a hardware keyboard's keypad digits on Android as digits when Num Lock is off, where they were not reported at all (#1242 by @houko).
