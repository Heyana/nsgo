### Added

- Add compressed GPU texture formats (`BC1`, `BC3`, `BC7`, `ETC2`, `ASTC 4x4`) with capability queries, block-size validation, and pure-Go headless decoding, per-pass GPU timing queries (`PassTiming`, `TimestampQuery`, `RenderPassDesc.TimingLabel`, `Frame.PassTimings`, `ViewProps.OnPassTimings`), and GPU debug labels, groups, and markers stripped at zero cost in release builds (#1195, #1196, #1240).
