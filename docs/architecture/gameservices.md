# Platform game services

Issue [#1129](https://github.com/nsgo-dev/nsgo/issues/1129) decides whether NSGo wraps native platform game services (achievements, leaderboards and local player sign-in).

## Scope decision

**Decision:** YES — NSGo provides a thin, provider-neutral `gameservices` package beside `audio`, `microphone` and `widget`, gated by `core.Env.CanUseGameServices()`.

- **Why in NSGo rather than `nsgo-dev/game`:** Reaching GameKit on Apple platforms requires Objective-C runtime blocks (`internal/platform/objcblock`) and presentation over the active AppKit or UIKit window, and reaching Google Play Games Services on Android requires the runtime pipe channel in `internal/driver`. Keeping the thin platform bridge in NSGo lets `nsgo-dev/game` and apps stay pure portable Go on NSGo's public API without writing platform code.
- **Why a separate package rather than `core`:** Apps that do not use game services must not link GameKit or Play Games bridge code ([what an app links](layers.md#what-an-app-links)). `core` carries only `Env.CanUseGameServices()`, the type aliases, and `RootOptions.GameServices` / `core.GameServicesOf` for `nsgotest`'s per-app fake.
- **What stays out of scope:** Matchmaking, real-time/turn-based multiplayer sockets, cloud save slots and store-specific desktop SDKs (Steamworks, Epic Online Services) are left out of this package. Steam is a desktop-channel optional provider for apps targeting Steam distribution rather than a default OS framework.

## Public API

Package `gameservices` exposes five provider-neutral types and six non-blocking operations:

- `Player` (`ID`, `DisplayName`, `Authenticated bool`)
- `Achievement` (`ID`, `PercentComplete float64` in `0..100`, `Unlocked bool`)
- `ScoreEntry` (`LeaderboardID`, `PlayerID`, `DisplayName`, `Score int64`, `Rank int`)
- `TimeScope` (`AllTime`, `Week`, `Today`) and `PlayerScope` (`Global`, `Friends`)
- Operations:
  - `Authenticate(cx *core.Scope, done func(Player, error))`
  - `ReportAchievement(cx *core.Scope, id string, percent float64, done func(error))`
  - `SubmitScore(cx *core.Scope, leaderboardID string, score int64, done func(error))`
  - `ShowAchievements(cx *core.Scope)`
  - `ShowLeaderboard(cx *core.Scope, leaderboardID string)`
  - `LoadScores(cx *core.Scope, leaderboardID string, scope PlayerScope, timeScope TimeScope, limit int, done func([]ScoreEntry, error))`
- Errors: `ErrUnsupported`, `ErrUnavailable`, `ErrUnauthenticated`, `ErrCancelled`.

### Contracts

- **Engine-goroutine callbacks:** Every `done` callback is marshaled back onto `cx`'s engine goroutine through `core.Scope.Post`, so operations never block frames, callbacks may update signals directly, and callbacks for a disposed scope are skipped.
- **Input normalization:** `ReportAchievement` clamps `percent` to `[0, 100]` (`NaN` becomes `0`) and rejects an empty achievement ID. `SubmitScore` and `LoadScores` reject an empty leaderboard ID; `LoadScores` defaults non-positive limits to `25` and caps limits at `100`.
- **Capability gate:** When `cx.Env().CanUseGameServices()` is false, async operations answer `ErrUnsupported` without touching the platform and `ShowAchievements`/`ShowLeaderboard` are no-ops.

## Platform backends

| Target | `Env.CanUseGameServices` | Backend | Notes |
| --- | --- | --- | --- |
| macOS, iOS, tvOS | True when `GameKit.framework` loads and exposes `GKLocalPlayer` | `purego` + `objcblock` (`gameservices_apple.go`, `gameservices_macos.go`, `gameservices_ios.go`) | Zero cgo. Uses `GKLocalPlayer.localPlayer`, `GKAchievement`, `GKLeaderboard` (`submitScore:context:player:leaderboardIDs:completionHandler:` with `GKScore` fallback; `loadLeaderboardsWithIDs:completionHandler:` + `loadEntriesForPlayerScope:timeScope:range:completionHandler:`), and `GKGameCenterViewController` (`GKDialogController` on macOS, top `UIViewController` on iOS). Every ObjC class and selector is guarded with `dlopen` and `respondsToSelector:`, and unauthenticated calls return `ErrUnauthenticated` without crashing. |
| Android | True when the runtime environment reports the Play Games Services hook bit (`more & 64`); the Java runtime does not set it yet, so it is false today | `internal/driver` game services JSON channel (`gameservices_android.go`; extended message `extGameServices` to Java, `msgGameServicesReport` back; a runtime without extended messages or without the hook answers each command unavailable) | Mirrors `microphone` and `widget`. Because Play Games Services (`com.google.android.gms:play-services-games-v2`) is a Google Play library outside the base Android SDK, the Java runtime is to reach it through reflection when packaged with an app or embedding host. That Java hook is not in the runtime yet: the Go side and the message kinds are in place, and until the hook lands, or wherever the library is missing, `CanUseGameServices` is false and calls answer `ErrUnavailable` cleanly without sending unknown frames to older prebuilt `classes.dex` runtimes. |
| Windows, Linux, Web, watchOS, mini programs, HarmonyOS | False | Unsupported stubs (`gameservices_other.go`) | Return `ErrUnsupported` without panicking. Desktop apps shipping on Steam can pair a desktop-channel Steamworks adapter alongside `gameservices`. |

## Testing with `nsgotest`

Each `nsgotest.App` owns an isolated in-memory fake (`nsgotest/gameservices.go`) wired through `core.RootOptions.GameServices` so parallel tests never share process-global state:

- Enable game services in tests with `nsgotest.Environment(core.Environment{CanUseGameServices: true})` (default is false so tests can also verify the unsupported-platform path).
- Configure sign-in and friends with `App.SetGameServicesPlayer` and `App.SetGameServicesFriends`.
- Simulate failures with `App.SetGameServicesError` and `App.SetGameServicesAuthError`.
- Seed and inspect state with `App.SeedLeaderboardScores`, `App.GameServicesAchievements`, `App.GameServicesSubmittedScores`, `App.GameServicesScores`, `App.GameServicesShownAchievements` and `App.GameServicesShownLeaderboards`.
