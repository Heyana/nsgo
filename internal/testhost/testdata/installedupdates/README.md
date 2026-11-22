# Installed update fixture

Build an owned copy twice with `go tool nsgo build`: version 1.0.0, then 1.0.1 in both `nsgo.yaml` and `version.txt`. Give that copy a unique app ID/name, a private generated updates key, `updates.channel: direct`, `updates.automatic: false` and an HTTPS `updates.feed`. Keep the first installers before the second build replaces the releases directory. Keep the same `.nsgo/android/debug.pem` between APK builds; it is private signing material, never a deliverable.

Install the first Debian package or APK, then serve the second package and its signed appcast through that feed. Check for updates and confirm the native prompt. Debian must install through real `pkexec dpkg` and restart into 1.0.1; Android must show its package installer and install 1.0.1 after confirmation. Use only an owned package identity, device/display and temporary feed, then remove them.

The app displays live download progress. Its loopback-only port 47820 exposes `GET /status`, `GET /events` and `POST /check`; the latter invokes the same public action as the button and leaves confirmation to the native UI. Events include staged bytes while downloading and the version/PID after restart. Use `adb -s <owned-device> forward tcp:<owned-port> tcp:47820` for Android. These endpoints exist only in this opt-in fixture.
