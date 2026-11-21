### Added

- Add the `nsgo/gamepad` TSX module (`supported`, `pads`, `state`, `Button`, `Axis`) and its TypeScript declarations, gated by the `gamepad` capability (`core.AccessGamepad`) (#1116).
- Add opt-in gamepad focus navigation (`gamepad.EnableFocusNavigation`, `gamepad.CaptureInput`, `core.MoveFocus`, `core.ActivateFocused`, `core.TriggerBack`) so D-pad and left stick traverse native focus stops, South activates, East triggers back, and mounted gameplay views can pause navigation cleanly (#1123).
