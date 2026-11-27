### Added

- Support reactive `DurationFunc` on `mobile.PictureInPicture` so custom Picture in Picture sessions can follow a loaded `core.Video`'s duration without remounting (#970).

### Fixed

- Fix iOS custom Picture in Picture render-size delegate ABI (`CMVideoDimensions`), clamp sample-buffer timebases to finite durations, move decoded PiP video layers across clipping holders, wake background frame rendering on session start, and disarm automatic background polling after PiP stops (#970).
