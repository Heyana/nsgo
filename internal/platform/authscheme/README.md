# Native scheme callback fixture

Run the opt-in public-API integration test on an isolated Linux or Windows desktop:

```sh
go tool nsgo test -env NSGO_TEST_AUTH_SCHEME_NATIVE=1 \
  -ldflags '-X github.com/nsgo-dev/nsgo/core.appID=dev.nsgo.authscheme1399' \
  -run TestRegisteredSchemeAuthCallback ./internal/platform/authscheme
```

The fixture starts the production `nsgo.Run` backend, registers a private scheme unique to its process, opens a local synthetic provider and receives the callback through a fresh OS-launched process. That process must hand its URL to the existing app through its normal single-instance channel. The test checks the callback, a synthetic S256 code exchange, cancellation and ordinary delivery of a late link.

On Linux, GIO resolves private desktop entries and MIME defaults under temporary XDG directories. A small test browser handles HTTP, follows the provider's redirect and invokes the registered scheme through GIO. The fixture needs GIO and `desktop-file-utils` and works on X11 or Wayland. If fresh XDG caches make GStreamer plugin discovery stall before the app starts, point `GST_REGISTRY` at an owned copy of the machine's existing `gstreamer-1.0/registry.*.bin`; the authentication test plays no media.

On Windows, only a fresh `HKCU\Software\Classes\com.nsgo.auth1399.p<PID>` key is registered, then removed. The existing default browser opens the provider URL. The provider invokes `ShellExecuteW` for the private callback, which exercises the OS handler and real process handoff without changing default-browser settings or its external-protocol consent. This does not verify that consent dialog. Use a lab browser with no unrelated windows. A runner may set `NSGO_TEST_AUTH_SCHEME_DATA` to its own temporary directory, warm the browser under that directory's `local` profile with `--no-first-run`, and close it before deleting that directory after the test. Without that override the fixture uses `t.TempDir`; close its provider tabs before cleanup.

For a Windows runner, export the same test executable through the tool, then execute it in that runner's interactive session with `NSGO_TEST_AUTH_SCHEME_NATIVE=1`:

```sh
go tool nsgo test -env GOOS=windows -env GOARCH=amd64 \
  -ldflags '-X github.com/nsgo-dev/nsgo/core.appID=dev.nsgo.authscheme1399' \
  ./internal/platform/authscheme -- -json=false -c -o /owned/path/authscheme.exe
```

No real account, provider credential or token is used. Ordinary package tests skip this fixture unless explicitly enabled.
