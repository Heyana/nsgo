### Added

- `nsgo.LaunchFiles(parse)` lets a Windows or Linux app decide which command-line arguments open as files, or with `nil` open none (#2264 by @J4ck1e2000).

### Fixed

- Windows and Linux apps no longer open arguments after `--` as files, on a first launch or one handed to a running instance (#2264 by @J4ck1e2000).
