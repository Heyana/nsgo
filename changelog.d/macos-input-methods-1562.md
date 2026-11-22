### Fixed

- Commit marked input-method text (including live conversion) and end the `NSTextInputContext` session before macOS inputs lose focus, turn read-only/disabled, or resign key, and fall back to the input's screen rect in `firstRectForCharacterRange:actualRange:` so IME candidates open beside the caret of point-wide editor sinks rather than at the screen's corner (#1562).
