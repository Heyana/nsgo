# Sensor settings acceptance

Run each permission separately on macOS and an isolated Android emulator:

```sh
go tool nsgo test -target macos -env NSGO_TEST_SENSOR_SETTINGS=location ./internal/testhost/testdata/sensorsettings
go tool nsgo test -target android -device emulator-5792 -env NSGO_TEST_SENSOR_SETTINGS=motion ./internal/testhost/testdata/sensorsettings
```

Repeat with `location`, `background-location` and `motion`. The fixture calls only `OpenPermissionSettings`; it neither requests nor grants access and declares no sensor permissions. During its 15-second hold, inspect only System Settings or the owned Android test app's Settings activity. macOS must show Location Services for both location keys and Motion & Fitness for motion. Android must show App info for `dev.nsgo.test.sensorsettings1405.nsgotest`; inspect its resumed activity's `package:` data as well as the visible app label. A successful callback proves dispatch only; record destination observation separately. Leave all controls unchanged. Android's test runner uninstalls its package afterward.
