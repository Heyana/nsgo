### Added

- Add package `cloud` for provider-neutral cloud key-value and file/snapshot storage (iCloud KV store and ubiquity containers on macOS and iOS, Google Play Games Saved Games bridge on Android, `ErrUnavailable` elsewhere), `core.Env.CanUseCloudStorage`, `nsgo.yaml` `cloud:`/`icloud:` Apple entitlements in `nsgo build`, and an isolated per-app in-memory cloud store in `nsgotest` with change and conflict simulation (#1241).
