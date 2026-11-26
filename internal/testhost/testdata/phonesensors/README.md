# Phone sensor acceptance

Checks location and motion through the native providers on an iOS simulator or an Android emulator, against an injected location, never a user's live one.

```sh
# iOS simulator
xcrun simctl location <udid> set 35.6812,139.7671
go tool nsgo test -target ios_simulator -device <udid> -env NSGO_TEST_LOCATION=35.6812,139.7671 ./internal/testhost/testdata/phonesensors

# Android emulator
adb -s emulator-5594 emu geo fix 139.7671 35.6812
go tool nsgo test -target android -device emulator-5594 -env NSGO_TEST_LOCATION=35.6812,139.7671 ./internal/testhost/testdata/phonesensors
```

The runner installs the test app afresh, so `TestGrantedLocation` asks for location; allow it in the system's prompt, or grant it while the run starts, repeating until the app is installed: `xcrun simctl privacy <udid> grant location dev.nsgo.test.phonesensors56.nsgotest`, or `adb shell pm grant dev.nsgo.test.phonesensors56.nsgotest android.permission.ACCESS_FINE_LOCATION` and `ACCESS_COARSE_LOCATION`. `TestDeniedLocation` runs with `-env NSGO_TEST_LOCATION_DENIED=1` on a fresh install, or with the simulator's permission revoked the same way. `TestMotion` reads the accelerometer, which a simulator has none of and answers unavailable.
