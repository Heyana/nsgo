### Added

- Picture what the window drew behind an effect host for WGSL `sample_backdrop` on Windows (copying the Direct2D target behind the host before it draws itself) and Linux (snapshotting the surface up to the host into a GSK render node), matching macOS, iOS and the web (#1534).
