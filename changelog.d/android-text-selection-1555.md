### Added

- Report `EventSelectionChange` from Android selection areas (`Selection.java` / `NsgoView.java`) with the selected text and the area-relative bounding box of the texts it touches when the area listens for `OnTextSelection`, deduplicating unchanged updates and reporting clears so `DiffView` frames follow text selection on Android (#1555).
