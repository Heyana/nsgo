### Fixed

- Let a focused view's `OnKeyDown` consume Android hardware keys before the keymap, activation and native dispatch, with each key reaching the app once, and order HarmonyOS's key bridge so view callbacks run before its Go keymap and releases reach the view that heard the down (#1352 by @houko).
