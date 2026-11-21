### Added

- Add `core.Style.GlassInteractive` (`glassInteractive` in TSX) for interactive Liquid Glass press feedback on macOS 27 (`NSGlassEffectView.effectIsInteractive`) and iOS 26 (`UIGlassEffect.isInteractive`), routing hit testing through the glass content view while keeping `OnPress` and pressable children working (#1637).
