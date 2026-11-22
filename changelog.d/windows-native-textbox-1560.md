### Added

- Host Windows `Input` fields through windowless RichEdit (`ITextServices2` / `ITextHost2` in `msftedit.dll`) drawn into Direct2D with `TxDrawD2D`, delegating editing, undo/redo, OS spell checking, password masking and TSF/IMM32 input methods to Windows with fallback to the DirectWrite editor when unavailable (#1560).
