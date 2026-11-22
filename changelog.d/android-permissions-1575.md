### Added

- Check and request Android runtime permissions (`RECORD_AUDIO`, `CAMERA`, `READ_CALENDAR`/`WRITE_CALENDAR`, `BLUETOOTH_CONNECT`/`BLUETOOTH`, `READ_MEDIA_AUDIO`/`READ_EXTERNAL_STORAGE`) through `checkSelfPermission`, `requestPermissions` and `permissionResult` with persisted asked state, and open the app's settings page via `ACTION_APPLICATION_DETAILS_SETTINGS` (#1575).
