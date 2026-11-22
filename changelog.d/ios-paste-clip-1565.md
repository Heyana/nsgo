### Fixed

- Clip pastes past `MaxLength` (and single-line/submitting-multiline pastes) on iOS `UITextField` and `UITextView` at composed character sequence boundaries before inserting via `insertText:` so pasting and undoing never corrupts UIKit's undo manager (#1565).
