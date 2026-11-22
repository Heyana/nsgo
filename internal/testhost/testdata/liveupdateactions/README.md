# Android Live Update actions

Run from the repository root with a rebuilt Java runtime:

```sh
PATH="$JAVA_HOME/bin:$PATH" ANDROID_HOME=<sdk> internal/platform/android/runtime/build.sh
go tool nsgo build -target android_arm64 internal/testhost/testdata/liveupdateactions
```

Install the generated APK on the owned Android device and launch `dev.nsgo.actions1465/dev.oneo.nsgo.NsgoActivity`. Tap **Start owned activity**, grant notification permission if asked, then go Home. Open the notification shade and tap **Pause 1465** on this fixture's ongoing notification. The notification body must increment and Logcat's `ACTION1465` must show the same PID as `ROOT1465`, with `action=pause kind=actions1465`. The launcher must remain behind the notification shade; the press must add no `NsgoActivity` launch or resume. Repeat the actual shade tap to check that its replacement notification preserves delivery.

The Pixel's older Android displays an ongoing notification. Android 16's promoted Live Update and status-bar chip need a separate Android 16 device run. Sending an `adb broadcast` or invoking the pending intent directly does not satisfy this acceptance.

Do not commit the rebuilt runtime's `classes.dex` or `classes.jar`; CI rebuilds them from Java and maintainers publish them separately.
