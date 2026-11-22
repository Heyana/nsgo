# Installed Flatpak updater fixture

This direct-channel app enables automatic updates and supplies a valid-format public key and an HTTPS feed. It must run inside a real installed Flatpak. The app checks startup capability, makes three explicit checks through the engine/backend and exits. Its transport records every fetch; an ordinary direct-check positive control must reach that transport, while the installed app must make zero requests. No live server or user data is used.

Build the app and lower-level native regression from the repository root:

```sh
go tool nsgo build -target linux_x64 -output /absolute/task/app internal/platform/linux/testdata/flatpak_updater
go tool nsgo test -env GOOS=linux -env GOARCH=amd64 ./internal/platform/linux -- -json=false -c -ldflags="-s -w" -o /absolute/task/nsgo-test
```

On Linux, stage the app executable and `nsgo-test` under a task-owned Flatpak build directory's `files/bin/`. Write its `metadata` as:

```ini
[Application]
name=dev.nsgo.Updater1419
runtime=org.gnome.Platform/x86_64/51
sdk=org.gnome.Sdk/x86_64/51
command=NSGoUpdater1419
```

Set `FLATPAK_USER_DIR`, `XDG_DATA_HOME` and `XDG_CONFIG_HOME` to task-owned directories for all commands. Install GNOME Platform 51 there from a configured remote, then use `flatpak build-finish`, `flatpak build-export` and `flatpak build-bundle` to package the directory. Install its bundle with `flatpak install --user --bundle --no-deps ./fixture.flatpak`.

Run the application under an owned Xvfb/D-Bus session, or an available desktop:

```sh
xvfb-run -a dbus-run-session -- flatpak run --user --socket=x11 --share=ipc --env=GTK_A11Y=none --env=GSETTINGS_BACKEND=memory dev.nsgo.Updater1419
flatpak run --user --command=nsgo-test --env=NSGO_REQUIRE_FLATPAK=1 dev.nsgo.Updater1419 -test.run '^TestInstalledFlatpakNeverFetchesItsAppcast$' -test.v
flatpak uninstall --user --noninteractive dev.nsgo.Updater1419
```

Both runs fail outside a real sandbox when explicitly required. The application is expected to report zero network requests and exit successfully. The native test also checks that the installation reason precedes Linux ownership probing. These are Flatpak results; they do not verify a TestFlight receipt on an Apple device.
