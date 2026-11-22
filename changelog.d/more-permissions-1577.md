### Added

- Add `PermissionContacts`, `PermissionPhotos` (with `PermissionLimited` for partial access), `PermissionAutomation` / `PermissionAutomationOf` (macOS `AEDeterminePermissionToAutomateTarget`), and `PermissionNotifications` to `core.Permission`, with Apple `CNContactStore`, `PHPhotoLibrary`, `CLLocationManager`, `CMMotionManager.authorizationStatus` and notification/settings support across platforms (#1577).
