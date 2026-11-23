### Added

- Load and play sounds in TSX with `nsgo/audio`, where signed packages keep the default sound quotas whatever the Go app raised and a sandbox holds at most `Limits.Sounds` (256) loads pending or open (#1222 by @houko).
