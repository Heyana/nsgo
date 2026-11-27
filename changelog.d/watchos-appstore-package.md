### Added

- Package `nsgo build -target watchos` as a watch-only App Store `.ipa` wrapping `Watch/<name>.app` (`<id>.watchkitapp`) inside Xcode's iOS stub (`ITSWatchOnlyContainer`) with `WatchKitSupport2/WK` and inside-out `sign.profile` / `sign.watch_profile` signing (#1488).
