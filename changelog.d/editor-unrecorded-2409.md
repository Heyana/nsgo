### Added

- `editor.Ref.SetValueOutsideHistory` replaces the text without an undo step, moving the history and the selections through it, so a merge from a sync server stays when the user undoes (#2409 by @im-apratap).

### Changed

- `editor.Editor.Value` replaces the text outside the undo history, as another app's change to the file does (#2409 by @im-apratap).
