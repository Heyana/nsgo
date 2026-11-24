# Windows installer fixture

Build from the repository root on a Mac with Go and Xcode:

```sh
go tool nsgo build -target windows_x64 internal/testhost/testdata/windowsinstaller
```

The normal build must produce the `*-windows-x64-setup.exe` release asset with no makensis on `PATH`. The Windows machine running it needs neither Go nor an NSIS installation. Use an isolated desktop and a copied fixture with a unique ID, extensions and schemes for each run.

Install silently into an owned directory using `/S /D=<directory>` with `/D` last. Keep an unrelated sentinel file there, start the app with `--stay`, build version 1.0.1 after changing `resources/version.txt` and dropping one extension/scheme, then upgrade with `/S` alone. Check the previous install location, owned process shutdown, PATH, shortcuts, dropped declarations and the preservation of a reassigned open command. Open an owned document and scheme through the shell and inspect the fixture's argument log. Uninstall with `Uninstall.exe /S`, retaining the sentinel and any deliberately reassigned declarations. Remove only the test's remaining files and unique keys afterward.

For visible acceptance, invoke the same installer without `/S`; `/LANG=2052` selects its bundled Simplified Chinese UI without changing Windows' language settings. Check folder selection, progress, desktop-shortcut choice and the finish page's app launch. The fixture records toast-hook invocations; it does not implement or verify the notification service itself. Self-signed test certificates can check signatures and payload integrity, but do not prove publisher trust or SmartScreen reputation.
