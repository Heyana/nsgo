# Packaged login fixture

This Windows-only app checks `SetLaunchAtLogin` and `LaunchAtLogin` in a real MSIX: disabled, enabled, then disabled again. It starts hidden, records those native replies and the launch reason in `startup-proof.txt` under its own cache directory, and exits. An unpackaged run exits before touching the Run key.

Build from the repository root:

```sh
go tool nsgo build -target windows_x64 -channel store ./internal/testhost/testdata/startup
```

The fixture uses Microsoft's unsigned-package test OID, under its own `NSGo.Startup1389.Test` identity. On a Windows 11 lab machine, an administrator can install its MSIX with `Add-AppxPackage -AllowUnsigned`, without changing certificate trust. Refuse an existing package of that name before installation.

Set `NSGO_STARTUP_FIXTURE_AUMID` to the fixture's `PackageFamilyName` plus `!App`, then run `go tool nsgo test -run '^TestActivateStartupFixture$' ./internal/platform/windows`. The test only activates this fixture. Inspect its proof: `initial=false`, `enabled=true`, `disabled=false`, and no timeout. Manual activation should report `launched-at-login=false`. An actual login StartupTask activation must be checked separately; this fixture never signs the user out or reboots.

Always remove the exact fixture package afterward with `Remove-AppxPackage`, and remove its own cache and transferred test files. A session that cannot activate packaged apps does not prove native startup behavior; keep that limitation in the result.
