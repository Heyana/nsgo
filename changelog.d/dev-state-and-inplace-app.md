### Added

- Keep serializable state with `core.DevKeep` and `core.DevKeepFunc` and restore `base.NewNav` stacks across `nsgo dev` restarts, compiled out of release builds (#658).

### Changed

- Update macOS dev `.app` bundles in place on Go-only edits and overlap old and new processes until the new window applies its first frame (#658).
