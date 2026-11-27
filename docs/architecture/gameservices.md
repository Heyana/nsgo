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
| Android | True when `com.google.android.gms.games.PlayGames` is present on the classpath (`more & 64` in the environment's second capability byte) | `internal/driver` game services JSON channel (`gameservices_android.go`; extended message `extGameServices` to Java, `msgGameServicesReport` back) backed by `GameServices.java` reflecting onto Google Play Games Services v2 (`com.google.android.gms:play-services-games-v2`) | Mirrors `CloudStorage.java`, `microphone` and `widget` without adding a compile-time Play Services dependency to the NSGo runtime. `GameServices.java` initializes `PlayGamesSdk`, probes `GamesSignInClient.isAuthenticated()` on attach, and handles `authenticate` (`GamesSignInClient.signIn()` + `PlayersClient.getCurrentPlayer()`), `reportAchievement` (`AchievementsClient.unlockImmediate` at 100%, `setStepsImmediate` with 1..99 steps for partial progress, requiring incremental achievements configured with 100 steps in Play Console), `submitScore` (`LeaderboardsClient.submitScoreImmediate`), `loadScores` (`LeaderboardsClient.loadTopScores`, capped at Play Games' 25-row page size), and `showAchievements`/`showLeaderboard` (`getAchievementsIntent`/`getLeaderboardIntent` launched via `startActivityForResult`). Play Services status codes map `SIGN_IN_REQUIRED` / `CLIENT_RECONNECT_REQUIRED` to `ErrUnauthenticated`, `CANCELED` or canceled tasks to `ErrCancelled`, and other errors to `ErrUnavailable`. Without `play-services-games-v2` on the classpath (or on older prebuilt runtimes without extended messages), `CanUseGameServices` stays false and commands answer `ErrUnavailable` cleanly. Apps packaging Play Games Services add `com.google.android.gms:play-services-games-v2` and declare `<meta-data android:name="com.google.android.gms.games.APP_ID" android:value="@string/game_services_project_id" />` in `AndroidManifest.xml`. |
| Windows, Linux, Web, watchOS, mini programs, HarmonyOS | False | Unsupported stubs (`gameservices_other.go`) | Return `ErrUnsupported` without panicking. Desktop apps shipping on Steam can pair a desktop-channel Steamworks adapter alongside `gameservices`. |

## Testing with `nsgotest`

Each `nsgotest.App` owns an isolated in-memory fake (`nsgotest/gameservices.go`) wired through `core.RootOptions.GameServices` so parallel tests never share process-global state:

- Enable game services in tests with `nsgotest.Environment(core.Environment{CanUseGameServices: true})` (default is false so tests can also verify the unsupported-platform path).
- Configure sign-in and friends with `App.SetGameServicesPlayer` and `App.SetGameServicesFriends`.
- Simulate failures with `App.SetGameServicesError` and `App.SetGameServicesAuthError`.
- Seed and inspect state with `App.SeedLeaderboardScores`, `App.GameServicesAchievements`, `App.GameServicesSubmittedScores`, `App.GameServicesScores`, `App.GameServicesShownAchievements` and `App.GameServicesShownLeaderboards`.
- `App.GameServicesScoreQueries` lists each `LoadScores` call with its player scope, time scope and limit, including calls that ended in an error; the fake records the time scope but does not filter by it.
