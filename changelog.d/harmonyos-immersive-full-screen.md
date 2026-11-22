### Added

- Support immersive full-screen requests on HarmonyOS, hiding the status bar, navigation bar and navigation indicator (#1124 by @houko).

### Fixed

- Report the display cutout and the system bars' side areas in the HarmonyOS safe area, so landscape has left and right insets (#1124 by @houko).
- Keep the three-button navigation bar on HarmonyOS outside immersive mode, and hide only the status bar for `Hidden` (#1124 by @houko).
