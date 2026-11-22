# Updates

An app updates the way its platform and the channel it reached the user through do: package `updater` asks, the platform's own updater answers, and `nsgo build` decides from `nsgo.yaml`'s `updates` what each package carries. Read this before changing `updater`, `internal/platform/updates`, a backend's `updates.go` or the tool's ([tool](tool.md#updates)).

NSH's `nsh/updater` downloads, verifies and swaps in the next version itself. NSGo leaves that to Sparkle, WinSparkle and the stores, with their windows (decided by the user, 2026-11-10): they handle installs and restarts better than an app's own code would, and stores forbid self-updating apps anyway. Linux and Android have no such updater, so on the direct channel `updater` streams the appcast's signed package directly to disk, verifies it as Sparkle does, and hands it to the package manager that installed the app (`dpkg` or `pacman`), atomically replaces the running AppImage and restarts, or hands the APK to Android's package installer.

## Requests and answers

`CheckForUpdates` is the core's part of `updater.Check` and `CheckInBackground`: `ItemCheckForUpdates`, in the background when `On`, which the platform answers with `EventUpdate` ([services](services.md#requests-and-answers)).

- Where the platform's updater shows what it found in windows of its own, Sparkle's or WinSparkle's, the answer is `Finished` and `updater` shows nothing.
- Elsewhere it carries a `driver.Update`: its typed management reason and store, a newer version and its page, none, or why the check failed. `updater` says it in an alert in its catalog's words and opens the page with `OpenURL`.
- On Linux and Android the answer also names the newer version's package when it is signed with the app's key and, on Linux, matches how the running executable was installed (`dpkg` for `.deb`, `pacman` for `.pkg.tar.gz`/`.pkg.tar.zst`, or `APPIMAGE`/`APPDIR` for `.AppImage`). Choosing Update then runs `core.InstallUpdateInBackground` under the app's root scope, so it outlives the scope that asked: it streams the package directly to a temporary file in the cache's `nsgo.updater` within ten minutes while updating `updater.Status` and the platform task progress bar, checks its length, SHA-256 and Ed25519 signature from the streamed file, and hands it to Linux's installer (`pkexec dpkg`, `pkexec pacman`, or atomic AppImage swap) and restarts once it succeeds, or opens Android's package installer. A failure cleans up any partial file, resets progress, and says so in an alert that offers the page. Update chosen again while one runs is ignored, so a second run does not discard the package of the first, and a check that finds the app up to date meanwhile leaves it too. The staged package is removed once installed on Linux, and on Android by the next download or a check that finds the app up to date, as the installer reads it after it starts.
- Watch apps follow their iPhone app. Standalone web releases compare the running file descriptor with a fresh `nsgo.json`; mini programs use their platform update manager. A prepared update sets `core.UpdateCheck.Restart`, including when the platform exposes no version label. Confirming requests a restart through quit handlers instead of opening an empty URL.

| Target | Updater |
| --- | --- |
| macOS | Direct builds use Sparkle's `SPUStandardUpdaterController`, started when Info.plist names a feed; installing, it quits the app as `Quit` does. Store builds query the Mac App Store and offer its native page |
| Windows | WinSparkle from beside the executable. It starts once the app has mounted, in the language the app shows (`updates.WinSparkleLanguage`): English or a translation WinSparkle ships, else the Windows UI language, WinSparkle's default. It takes the language only before it starts, so one the app switches to later shows from the next launch. Its can-shutdown callback waits for `OnQuitRequest` approval before launching the installer; its shutdown callback then finishes that approved quit |
| Linux, direct Android | `internal/platform/updates` reads the target's appcast with the backend's HTTP client, verifies the package's SHA-256 and Ed25519 signature, then `updater` stages it and hands it to the platform installer |
| iOS, Mac App Store | Apple's lookup by the installed bundle's ID in the locale's country, against `CFBundleShortVersionString`; Mac requests `desktopSoftware` and returns a `macappstore` link (`apple.CheckAppStore`) |
| Google Play | A manual check enters Play In-App Updates' immediate flow when allowed, so Play owns consent, downloading, installation and restart; other stores remain managed |

## Browser and mini-program releases

The web loader passes its published descriptor to its own Go instance. A standalone page checks the current `nsgo.json` with HTTP caching disabled, coalesces concurrent requests, and bounds/aborts pending fetches. A changed immutable-file descriptor offers a reload even when its displayed version is unchanged. Embedded hosts own their lifetime and do not reload through this API. This path needs no appcast key; it runs when the app calls `updater.Check` or its background form, without a new automatic polling loop or preference setters.

Reloading does not force a waiting offline worker to activate under older live tabs. The active worker can serve verified immutable assets from the waiting release's cache while retaining the previous cache; normal activation still retires old caches after their clients close. A real two-release Chromium run verifies Later, confirmed reload into the new module, and a subsequent up-to-date check.

The mini-program runtime captures `getUpdateManager` events before WebAssembly starts and retains their state. Checks wait for readiness, no-update or failure with a bounded timeout; confirming a ready update calls `applyUpdate`. No version is invented for managers that expose none. Missing APIs fail explicitly. JavaScript and Go bridge fixtures verify this contract; published vendor-client upgrade acceptance remains in the roadmap. NSH v0.6.1 has no equivalent browser/mini-program updater; this follows the host APIs while keeping NSGo's shared confirmation UI.

## State and preferences

`updater.CapabilityOf(cx)` synchronously reports who updates this copy, without network I/O. The build retains its channel even when it carries no updater. `core.UpdateCheck.Managed` and `Store` carry the same reason; development, disabled-channel and store-managed checks have no `Err` and show that reason rather than a transient-failure message. A configured Sparkle framework or WinSparkle DLL that fails to load still answers with an `Err`.

| Managed | Copies |
| --- | --- |
| `ManagedSelf` | Direct macOS, Windows and Android builds; Linux where dpkg or pacman owns the executable, or it runs from an AppImage |
| `ManagedPage` | iOS and Mac App Store builds; direct Linux copies outside Debian, Arch and AppImage installs |
| `ManagedDevelopment` | `nsgo dev`, `nsgo test` and plain Go builds |
| `ManagedChannel` | `none`, Steam, and `store` builds outside Apple's App Stores |
| `ManagedStore` | Android store installers, Flatpak, Snap and TestFlight; `Store` names them, and Google Play alone supports manual native checks |
| `ManagedUnavailable` | Targets with no updater, or a configured native updater that failed to load |

Linux resolves Debian or Arch package ownership, or the running AppImage, once at launch before the first feed request. A copy outside those formats, or a failed or timed-out ownership probe, gives the release's page.

`updater.Status(cx)` is a tracked read of `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` and `LastCheck`, and of an in-app update's `Phase`, `Loaded` and `Total` on Linux and Android, which the core keeps across the platform's reports. It tracks native found/not-found notifications, check times and preferences across windows. Sparkle supplies version metadata through its delegate. WinSparkle's public callbacks carry no version, so a found callback reads the configured appcast for status metadata; WinSparkle alone chooses and installs its update. A failed metadata read retains the last successful metadata.

`SetAutomaticChecks(cx, on)` persists the user's choice in Sparkle or WinSparkle, or in the private `nsgo.updater` partition on Linux, Android and iOS. Their daily scheduler follows changes at runtime, including enabling a build whose default is off. The saved choice wins over `updates.automatic`; `LastCheck` stores the actual check time separately from the retry schedule. `SetAutomaticDownloads(cx, on)` changes Sparkle's persisted setting on macOS; `updates.automatic_downloads` supplies its first-install default, false unless set. Other platforms report false and ignore that setter. Unavailable, development, disabled-channel and store-managed builds report both preferences false.

## Schedule

While `updates.automatic` is on, Sparkle and WinSparkle check daily on their own schedule, where the user's choice in their settings wins over `nsgo.yaml`'s. Apps read and change that same choice through `Status` and `SetAutomaticChecks`.

Linux, direct Android and Apple App Store builds have no such schedule, so `nsgo.Run` calls `updater.CheckDaily` as the app starts and whenever it becomes active again, since a phone's app stays in memory for days. The persisted runtime preference, defaulting to `updates.automatic`, decides whether it checks. It checks in the background unless one ran within the day, so *Later* lasts the day across restarts, and tries a failed check again an hour later. The time lives in a storage partition of its own (`nsgo.updater`), which the app's `Storage` neither lists nor clears.

## Channels

One platform reaches users through several channels, and the channel, not the platform, decides who updates the app. `updates.channel`, or `nsgo build -channel`, names it per build:

- `direct`, the default with a key: the app's own downloads update themselves. macOS and Windows packages carry Sparkle and WinSparkle, and each release writes signed appcasts plus package manager manifests (`homebrew/<slug>.rb` with `auto_updates true` and `winget/` multi-file manifests) for the built macOS and Windows installers. On this channel the `macos_universal` target joins the two macOS architectures into one app with one DMG and feed.
- `store`: a store or package manager updates the app. Packages carry no self-updater or appcast. iOS and macOS ask their App Store without a key; Google Play installs support its native update flow when the app explicitly checks.
- `steam`: Steam updates the depot. The desktop build uses `nsgo_steam` to omit the platform self-updater, the Linux package installer and downloader hook, and the automatic schedule. The public update checks compile as no-ops.
- `none`: nothing checks, for packagers who repackage the app.

A store bans an app that carries a self-updater or sends its users to other downloads, so a store's build must be a separate one, never a direct build uploaded as it is. Downloads that a package manager fetches unchanged from the release, as Homebrew casks and winget manifests do, stay direct: Sparkle and WinSparkle update them, the cask says `auto_updates true`, and winget compares the `DisplayVersion` the installer writes.

| Platform | Direct | Store |
| --- | --- | --- |
| macOS | GitHub or a website, Homebrew casks: Sparkle | The Mac App Store, from its sandboxed package |
| Windows | GitHub or a website, winget: WinSparkle | The Microsoft Store, from the MSIX; a Store listing of the setup may keep WinSparkle |
| Linux | GitHub or a website: the release's Debian package through dpkg, Arch package through pacman, or AppImage in place, else the release's page | Distribution repositories and the AUR (`none`), Flathub from the Flatpak manifest, and Snap |
| Android | GitHub or a website: the release's APK through the package installer | Google Play from the App Bundle, and other stores |
| iOS, tvOS | — | The App Store; TestFlight checks nothing |

## Who installed the app

A build cannot know every channel it ends up in, so where the system says who installed the app, the backend checks that first, as the app starts, and leaves an app a store installed to that store (`Config.Store`): its checks answer with that store as the reason and the daily schedule never runs. Google Play is the exception for a manual check: `android.withoutUpdateChecks` sends a capability-guarded extended message to `PlayUpdates.java`, whose reflected SDK reports failure or handles its own consent UI. The app never downloads an APK from its appcast for a store install.

| Target | Sign | Store |
| --- | --- | --- |
| Android | The installing package, which the runtime passes as `NSGO_INSTALLER` | Any but the system's package installer and adb, so an unknown installer counts as a store |
| Linux | `/.flatpak-info`, `SNAP` | Flatpak, Snap |
| iOS | A receipt named `sandboxReceipt` | TestFlight and development builds on a device, whose versions the App Store does not list; the simulator still asks |

Linux cannot tell a distribution's package from a downloaded one, so its packagers build with `-channel none`. macOS and Windows select their stores through the build channel, using separate packages without a self-updater. macOS checks its store; Windows remains managed by its channel.

## Limits

- Play checks support the immediate flow only, preserve user cancellation, and resume a consented flow when its activity returns. Standalone runtime builds include the checksum-pinned Play In-App Updates SDK; embedding hosts supply it and retain its reflected entry points ([embedding](../public/en/embedding.md#android)).
- The direct channel reads `releases/latest/download/` by default, so every release must carry the appcast of each target it updates: a target a release leaves out answers 404 until one carries it again.
- WinSparkle saves `updates.automatic` as the user's choice on the first start, so a later change in `nsgo.yaml` reaches only new installs.
- Linux asks `OnQuitRequest` before restarting after its package manager finishes. Cancellation keeps the current process running; a deferred answer can finish saving first. The package is already installed on disk. On Windows, WinSparkle waits on its worker thread for the same decision, leaving the app's main thread free; denial prevents installer launch, and cleanup releases an unanswered callback before joining the updater's workers. The approved shutdown bypasses a second close request, so a tray or window-close handler cannot strand an installer beside the old process.
- The Linux appcast lists signed items for the release's Debian package (`.deb`), Arch package (`.pkg.tar.gz`) and AppImage (`.AppImage`). The Linux backend selects the matching item—installing `.deb` through `pkexec dpkg`, `.pkg.tar.gz`/`.pkg.tar.zst` through `pkexec pacman`, or atomically replacing `$APPIMAGE` with executable permissions before restarting—and falls back to the release's page for installs outside those formats; the backend resolves ownership once at launch, before any appcast request. Android hands its APK to the system installer, which asks the user and keeps store installs out of the direct channel.
- `updater.Install` sends the staged path with `ItemInstallPackage`. Linux replies after its package manager or AppImage replacement finishes, and Android replies when the system installer starts. Backends without a package installer reply that the service is unavailable. Headless tests can record paths in `App.Installs` and return errors through `App.InstallPackage`.
- Update downloads stream directly to a temporary file on disk (`imageload.HTTPToFile`: libsoup 3 on Linux and `HttpURLConnection` to a file on Android) and verify SHA-256 and Ed25519 directly from the staged file (via read-only `mmap` on Unix), reporting live progress to `core.UpdaterStatus` and `SetTaskProgress` instead of buffering the whole package in memory.

## Verification

- `updater`'s alerts and schedule are tested with `nsgotest`; the appcast reader, version order, App Store answers and the installer rules run headless, and the tool's tests cover channels, flags, signed appcasts, Homebrew casks (checked with `brew style` and strict `Cask::Audit`) and winget manifests (strict YAML schema round-trip and `winget validate` on Windows).
- End to end against a local appcast (2026-11-11), with a copy of the tool that took an `http` feed, Sparkle on macOS downloaded 0.2.0 in the background and installed it, ad hoc signed, as 0.1.0 quit.
- WinSparkle on Windows 11 x64 offered 0.2.0, verified the download, ran the setup with `/S /RELAUNCH` and started 0.2.0 (2026-11-11).
- WinSparkle on an English Windows 11 x64 spoke the language an app forced as it mounted: Simplified and Traditional Chinese, Brazilian Portuguese and Japanese (2026-11-16). wxWidgets matches the language set only in lower case (`zh_cn`), which `TestWinSparkleLanguage` keeps.
- Linux under Wayland offered 0.2.0 in `updater`'s alert as the app started, and with `SNAP` set asked nothing. iOS's check reaches the App Store from the simulator, and a development build on an iPhone, whose receipt is `sandboxReceipt`, checked nothing.
- Android emulators on API 29 and 34 checked an APK installed by adb or the package installer and left one installed as Google Play's alone. On API 35 ARM64, a release fixture read an HTTPS appcast, showed live download progress while its APK grew on disk, and updated from 1.0.0 to 1.0.1 through the real package installer after native confirmation (2026-11-23).
- Headless tests cover installing on Linux and Android: only signed packages are offered, multi-item Linux appcasts list `.deb`, `.pkg.tar.gz` and `.AppImage`, downloads stream to disk with progress reported via `updater.Status` and `SetTaskProgress` and clean up partial files on cancellation or bad SHA-256/Ed25519 signatures, Linux detects Debian, Arch (`.pkg.tar.gz`/`.pkg.tar.zst`) and AppImage installs, constructs `pkexec` commands and atomically replaces AppImages with executable bits and rollback on error, Android hands the APK to its installer, a failure offers the page, the install finishes after the scope that offered it is disposed, one runs at a time, and staged packages are removed. A native Linux fixture drove the install request through the engine with a fake `pkexec`, checking dpkg and pacman arguments, success, failure and cancelled authentication. Linux on the Steam channel and Chromium returned an unavailable service.
- An installed Debian release fixture under an owned X display confirmed its native update prompt, streamed the signed 1.0.1 package to disk, installed it through real `pkexec dpkg` and restarted into a new process with the new version; the staged package was removed (2026-11-23). The fixture ran as root, so this does not verify a non-root Polkit authentication prompt.
- An installed Flatpak fixture on GNOME 51 verifies the real sandbox marker, disables automatic checks for a direct-channel app, and returns Flatpak-managed answers for three explicit checks without fetching its appcast. Its instrumented transport has a positive direct-check control; the lower-level native test also verifies store policy before any ownership probe. See `internal/platform/linux/testdata/flatpak_updater`.
- An owned Mac bundle fixture supplied its installed ID/version to the shared store check, and the public lookup and Launch Services resolution to `com.apple.AppStore` passed, as did the Android 35 runtime pipe through the real Play SDK to `ERROR_INSTALL_NOT_ALLOWED` (-6) on the unpublished fixture; JVM fixtures exercise availability, launch, resume, cancellation, duplicate replies, missing SDK and foreground loss (2026-11-23). A published Play app with two eligible versions and a Mac App Store-installed app remain unverified.
- Not run yet: a Developer ID or notarized macOS update, Windows on arm64, a TestFlight build, native pacman/AppImage update-and-relaunch flows, or a physical Android phone. The opt-in `internal/testhost/testdata/installedupdates` fixture supports installed-package acceptance without replacing NSGo's updater or HTTP client.
