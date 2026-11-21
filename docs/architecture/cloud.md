# Cloud storage

Package `cloud` provides a provider-neutral asynchronous API for cloud key-value entries and cloud files/snapshots across platforms ([#1241](https://github.com/nsgo-dev/nsgo/issues/1241)).

## Design and guarantees

- **No silent drops.** Unavailable accounts, missing entitlements, or platforms without a native cloud backend always return `cloud.ErrUnavailable` from every read, write, delete and list call. Writes are never silently ignored or diverted to local storage behind the app's back.
- **Engine-goroutine delivery.** `Get`, `Set`, `Delete`, `Keys`, `ReadFile`, `WriteFile`, `DeleteFile` and `ListFiles` run I/O off the engine goroutine and post their callbacks through `core.Scope.Post`. `WatchChanges` and `OnConflict` likewise deliver events on the engine goroutine and register `cx.Cleanup` so subscriptions are removed automatically when `cx` is disposed.
- **Availability signal.** `core.Env.CanUseCloudStorage()` reports whether a cloud backend is available (`driver.Environment.CloudStorage`), updating reactively as availability changes.
- **Zero hand-written C.** Apple APIs are reached via `purego`, `objc` and `internal/platform/objcblock`; Android reaches its Java runtime over the existing binary pipe via `internal/driver`'s `SendCloudStorage` / `HearCloudStorage` bridge.

## Platform backends

| Target | Backend | Mechanism |
| --- | --- | --- |
| macOS, iOS (`cloud_apple.go`) | iCloud | `NSUbiquitousKeyValueStore.defaultStore` (`dataForKey:`, `stringForKey:`, `setObject:forKey:`, `removeObjectForKey:`, `synchronize`, `NSUbiquitousKeyValueStoreDidChangeExternallyNotification`) for KV; `NSFileManager.URLForUbiquityContainerIdentifier:` + atomic writes + `NSFileVersion.unresolvedConflictVersionsOfItemAtURL:` + `NSMetadataQuery` for ubiquity files. Checks `ubiquityIdentityToken != nil` and non-nil container URL before every operation. |
| Android (`cloud_android.go`) | Google Play Games Saved Games | JSON commands over `internal/driver`, sent as the extended message `extCloudStorage` (sub-kind 14, only to a runtime that sets `CAP_EXTENDED_MESSAGES`; an older runtime answers unavailable) and answered by `msgCloudStorageReport` (32), to `CloudStorage.java`, which reflects onto `com.google.android.gms.games.PlayGames` (`SnapshotsClient` / `GamesSignInClient`) without a compile-time Play Services dependency. The runtime probes Play Games' sign-in when its view attaches, reports it to package cloud and carries it in bit 128 of the environment's second capability byte. Returns `ErrUnavailable` whenever Play Games is absent or the user is not signed in. |
| Windows, Linux, Web, other (`cloud_other.go`) | Unavailable | Always returns `ErrUnavailable`. |

### Desktop stores and Steam Auto-Cloud

Desktop platforms such as Windows and Linux have no OS-wide cloud save daemon. Games shipped on Steam (and similar desktop storefronts) do **not** need extra cloud SDK code: configure Steam Auto-Cloud to sync `core.Scope.DataDir()` directly, and your ordinary local save files synchronize automatically across machines.

## Packaging and entitlements (`cmd/nsgo`)

Configure Apple iCloud entitlements in `nsgo.yaml` under `cloud:` (or its Apple aliases `icloud:` / `apple.icloud:`):

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

`nsgo build` validates every container starts with `iCloud.` and writes the corresponding Apple entitlements into the generated `.entitlements` plist for macOS, iOS and tvOS bundles:

- `com.apple.developer.ubiquity-kvstore-identifier`
- `com.apple.developer.icloud-container-identifiers`
- `com.apple.developer.icloud-services` (`["CloudDocuments"]`)

When signing with a provisioning profile (`sign.profile` or `sign.macos_profile`), `cmd/nsgo` verifies that the profile grants the requested iCloud entitlements and strips unrequested iCloud entitlements from the final signature. macOS grants iCloud only with a provisioning profile, which only Mac App Store builds (`sign.macos_profile`) embed: a Developer ID or ad hoc build leaves iCloud out with a warning, and the app sees `ErrUnavailable`.

## Testing (`nsgotest`)

`core` defines the store interface and its change, conflict and error types itself, since it owns `RootOptions.Cloud` and no backend implements them, and package cloud aliases them (`cloud.ChangeEvent` is `core.CloudChangeEvent`); `internal/driver` carries only the Android runtime bridge.

Each `nsgotest.App` gets its own isolated in-memory `core.CloudStore` via `core.RootOptions.Cloud`, so tests run safely with `t.Parallel()`:

- `nsgotest.CloudAvailable(bool)` sets initial availability before first render.
- `app.SetCloudAvailable(bool)` toggles availability and updates `Env.CanUseCloudStorage()` reactively.
- `app.SimulateCloudChange(kv, files, reason)` injects external multi-device updates/deletions and delivers `cloud.ChangeEvent` to `cloud.WatchChanges`.
- `app.SimulateCloudConflict(name, remote)` injects an unresolved remote conflict, delivers `cloud.Conflict` (with working `Resolve`) to `cloud.OnConflict`, and makes `cloud.ReadFile` fail with `*cloud.ConflictError` until resolved.
