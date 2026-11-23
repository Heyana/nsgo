---
title: Platform services
description: Alerts, the clipboard, files, photos, sharing, storage, secrets, speech, speaking text, networking, signing in, in-app purchases and the app's lifecycle.
group: Platform features
order: 44
---

## Alerts and the clipboard

`cx.ShowAlert` shows the platform's alert or action sheet. Its `Actions` come back by index, and a role draws the cancel and destructive buttons the platform's way. On macOS, alerts and file pickers use a sheet while their window is visible. A hidden, minimized or offscreen window, or a tray app without a window, opens the dialog separately instead. Closing the requesting window cancels its dialogs.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` writes text and `SetClipboardImage` an image. `ReadClipboard`, `ReadClipboardImage` and `ReadClipboardFiles` answer asynchronously, and `HasClipboardText` and `HasClipboardImage` read what the clipboard holds without a paste notice. `core.FileFromURL` opens the URLs `ReadClipboardFiles` and `View.OnFileDrop` give as a `core.File`: `file:` URLs, and on the web `blob:` URLs.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

## Opening links and files

`cx.OpenURL` opens a URL in the system's browser or app, and `core.CheckURL` says what it refuses: no scheme, over 16 KiB, spaces, a web address without a host, and `javascript:`, `data:`, `blob:`, `file:`, `about:` and `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` opens a file the app has in the app associated with its type, and `cx.Share` shows the system's share sheet for files, text or both across macOS, iOS, Android, Windows, Linux (through the desktop portal or file manager) and the web (`navigator.share`). A `core.SharedFile` is a path on disk — or, for `Share` on the web and anywhere a file is held in memory, its `Data` bytes — with the name and type the other app sees. On desktops, `desktop.RevealFile` shows a file or folder selected in Finder, Explorer or the file manager, and `desktop.TrashFile` moves it to the Trash or Recycle Bin; it never deletes permanently, answering `core.ErrNoTrash` and leaving the item in place where the volume has no trash or the trash would not keep it, such as a file larger than the Recycle Bin.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

On Android, `mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` receives text and files for the MIME types in [`android.share_types`](building.md#incoming-shares-on-android). A share that starts the app waits for the first handler. Text arrives in `Text`, and files arrive as readable cache copies with `Path`, `Name` and `MIME`. Move or copy a file to persistent storage if it needs to outlive the cache.

`desktop.LookUp` shows the system's dictionary panel for a text, as a three-finger tap does on macOS, drawn over a box in a view: a selection's text and box with its `Selectable` view's `Ref`. `desktop.CanLookUp` says whether it can, in a window on macOS; Windows, Linux and the web have no such panel. In tests, `nsgotest.App.LookUps` lists the panels shown where `nsgotest.Environment` sets `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` asks for file previews, starting at an index, and updates the files when called again while the panel is open. `desktop.CanQuickLook` reports availability in an app's own window on macOS and Windows, and on Linux when the session exposes GNOME Sushi's running or activatable D-Bus service. Windows uses the file type's registered preview handler; supported formats depend on the installed handlers or previewer. A true result means the platform was asked, not that it rendered the file.

Space or Escape closes the panel. Arrow keys select files in the request on Windows and in Sushi versions that report selection and ownership on X11. On Wayland, Linux currently opens an unparented preview; automatic owner-close and arrow selection are unavailable. KDE-specific previewers and the web are unsupported.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Native menu icons

`desktop.MenuItem.Icon` shows an icon beside an item in application, Dock, tray and popup menus on macOS, Windows and Linux. A `desktop.MenuIcon` takes a parsed vector icon (`SVG`, such as `lucide.Folder()`), encoded image bytes (`Data`, PNG, JPEG, GIF, WebP or SVG) or a local/asset/pixel `core.ImageSource` (`Source`). Setting `Template` tints a monochrome image with the menu's text color, following light and dark appearance (and highlight state on macOS); monochrome `SVG` icons are treated as templates by default.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Icons load synchronously from local sources; remote URLs and invalid images leave the item without an icon. Menu labels, shortcuts and check/radio indicators keep their native behavior.

In tests, `nsgotest.MenuItem.Icon` records each item's icon in `app.Menu`, `app.DockMenu` and `app.PopupMenu`.

## Photos and files

`cx.PickImages` shows the system's photo picker and answers with `core.PickedFile`s, whose bytes, name, type and pixel size the app reads. `cx.TakePhoto` shows the camera where the platform has one; `cx.Env().CanTakePhoto` says whether it does. On macOS, Windows and Linux it shows the scanner's camera preview with a Take Photo button. `cx.ScanCode` opens a camera preview and answers with the first code it reads, a QR code unless `ScanOptions.Kinds` asks for others, or nil when the user closes it; `cx.Env().CanScanCode` says whether the platform has a scanner, on macOS, iOS and Android devices with a camera, Windows machines with one, Linux desktops with the Camera portal and GStreamer `pipewiresrc`, and browsers with `getUserMedia` and `BarcodeDetector`. macOS presents a sheet when the requesting window is visible, Windows and Linux a modal over the window, and the web an overlay over the page. Put `NSCameraUsageDescription` in `apple.info` for macOS and iOS, and `CAMERA` in `android.permissions` for Android. Android, Windows and Linux read every kind but PDF417. Treat the code's text as untrusted input.

In tests, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` and `nsgotest.App.ScanCode` receive `nsgotest.ImagePick` records for picks and captures and `nsgotest.CodeScan` records for scans, and answer in place of system UI. The hooks work headless and under supported `nsgo test -target` hosts. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` gives a target without a camera one.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` picks files of any type, `cx.PickDirectories` picks folders as an `fs.FS`, and `cx.SaveFile` saves a file or bytes through the system's save dialog. Both pickers take a `Title`, a `Button` label and a `Start` folder, and `PickFilesOptions.TypesName` names the type menu, so an app can show them in its own language; these are hints, which macOS, Windows and Linux follow, iOS only for `Start`, and Android and the web not at all. `cx.PickScreenColor` picks a color anywhere on the screen where `cx.Env().CanPickScreenColor()` is true: macOS, Windows, Linux with the desktop Screenshot portal, and browsers with `EyeDropper`.

Set `core.SaveOptions.Start` to an existing absolute folder to open the save dialog there on macOS, Windows, Linux and iOS. With an empty or unusable hint, or on other targets, the system chooses the folder.

Set `core.SaveOptions.Accept` to validate the chosen destination before writing. It runs on the engine's goroutine. Returning an error leaves the destination untouched. On macOS, Windows and in the in-process GTK chooser on Linux, the dialog shows the reason and lets the user choose another path. Cancelling after a refusal reports `saved == false`, the refused path and the error. On Android and in the Linux desktop portal, a refusal completes the save with that result. Android lets the user choose a folder and supplies a tree URI with the requested file name as its `name` query parameter. It creates the document after approval and reports the new document's URI on success. The platform abandons the write if the check does not answer within five seconds. iOS and HarmonyOS report an error for saves requiring a check. The web has no destination path and uses its ordinary save flow.

In headless tests, `App.PickFiles`, `App.PickDirectories` and `App.SaveFile` receive `nsgotest.FilePick`, `nsgotest.DirectoryPick` and `nsgotest.FileSave` records. The same records are in `app.FilePicks`, `app.DirectoryPicks` and `app.Saves`.

### Bundled assets

Put files under the app's `resources/` directory and use `cx.ReadAsset("asset:maps/level.bin", done)` to read them from Go. The callback receives `([]byte, error)` on the engine's goroutine after the read and is dropped if its owner goes away or runs again. Use `errors.Is(err, core.ErrAssetNotFound)` to identify missing files. The read loads the entire file into memory.

## Pull PCM audio

Use `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` to play interleaved Float32 PCM your app decodes or synthesizes. `Start` begins pulling samples, `Pause` stops the pulls and the owning scope closes the output as it goes. macOS, iOS, Android, Windows, Linux and the web support this stream. Other targets return `audio.ErrUnsupported`.

`Render` returns the number of frames it filled. It runs on the audio thread and must avoid blocking, allocation and graph access. A short fill plays silence for the remainder and increments `Underruns`. `Position` returns the monotonic frame count and the host time at that frame. Use `Flush` to discard pending frames and `Latency` to read the buffer latency. Route changes and interruptions pause output and call `OnInterrupt` on the engine. The app can resume it with `Start`.

## Storage

`cx.Storage()` is the app's key-value storage, kept on the device and shared by every graph of the app. Reads are synchronous and read inside a computation subscribe it to changes; `SetItem` returns a `core.StorageFullError` past the quota (`core.StorageQuota`, 5 MiB by default). It is a plain file any program of the user's can read, so keep tokens and passwords in [secrets](#secrets) instead.

`cx.Store()` returns the underlying store, for a host that creates another root with `core.RootOptions.Storage`, such as one running a plugin on a goroutine of its own: both roots then share the app's entries and what is kept on the device. It is the whole store, not the partition `cx.ProvideStorage` gives code the app did not write.

A host supplying its own persistence calls `core.NewStore(backend)` with a `core.StorageBackend`: `Load() (map[string]string, error)` loads once, and `Write(core.StorageChange)` receives changes in order. Switch on `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) and use `change.Key` and `change.Value`. Pass `nil` for storage held only in memory.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` lists the keys, `RemoveItem` deletes one and `Clear` empties the storage. `cx.DataDir()` and `cx.CacheDir()` give the app's folders for files the key-value storage is too small for; `cx.LogsDir()` gives the platform's directory for log files, and a `Scope.ProvideStorage` gives a subtree a partition of its own.

An iOS App Clip and its full app share these data and storage locations when `app_clip` and explicit `apple.app_group` are configured; their caches stay separate. Enabling the group does not migrate existing sandbox data. [App Clip storage](building.md#app-clips) covers configuration and signing.

On HarmonyOS, Go file operations such as `os.ReadFile`, `os.WriteFile` and `os.Rename` work under these directories and `os.TempDir()`. Relative paths start in the files directory. Advisory file locks and symbolic links are unavailable.

### Crash reports

Native release builds keep Go crash traces under `cx.LogsDir()/crashes`. Apple records uncaught Objective-C exceptions, Android records uncaught Java exceptions, and Windows records SEH exceptions that reach its top-level filter. List pending reports on the next launch with `cx.CrashReports()`. Each report identifies the version, build and platform that produced it. Call `Open()` to read the diagnostic text, close the reader, then call `Acknowledge()` to delete the report when the app has processed it. Run these file operations on a goroutine.

## Disk space

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` queries the volume containing `cx.DataDir()` asynchronously and calls back on the engine goroutine. `Total` is the system-reported capacity, `Available` is what the app can use for a user-requested operation, and `Opportunistic` is what it can use for caches and prefetching. All three are in bytes. `Available` includes space the system reclaims on demand where it reports that figure. Platforms with a single free-space figure return it in both fields. On the web, `Total` is the browser's estimated storage quota for the origin and `Available` is the unused portion. When the service is unavailable it returns `core.ErrDiskSpaceUnavailable`.

Apple builds require `apple.privacy.accessed_apis` in `nsgo.yaml` to declare `NSPrivacyAccessedAPICategoryDiskSpace`. Declare `E174.1` to check space before writing or to free files when space is low, with a change the user can observe. Declare `85F4.1` to show the user the available space. The app must follow Apple's restrictions on using and sharing the readings. A build without the category returns `core.ErrNotDeclared`. [Apple's approved reasons](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Set a test's answer with `nsgotest.DiskSpace(space, err)`.

## Secrets

`cx.Secrets()` is the app's secure store, for refresh tokens, API keys and passwords: `Get`, `Set` and `Delete` a string by account, which the platform protects. A missing account reads as `("", false, nil)`, and where the platform has no secure store, as on the web, every call returns `core.ErrSecretsUnavailable`; NSGo never falls back to plain storage. The system may make a call wait, as a keychain asking the user does, so call them off the engine's goroutine and hand the result back with `cx.Post`.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Platform | Where a secret is kept |
| --- | --- |
| macOS | A generic password of the login keychain: `kSecAttrService` is the service, `kSecAttrAccount` the account |
| iOS, tvOS, watchOS | A generic password of the keychain, the same way |
| Windows | A generic credential of the Credential Manager whose target is `service/account`, at most 2560 bytes |
| Linux | An item of the Secret Service (GNOME Keyring, KWallet) with `service` and `account` attributes; unavailable where the session runs none |
| Android | Encrypted with an AES key of the Android Keystore, in the app's private files, outside backups; reads also fall back to values `expo-secure-store` encrypted into its `SecureStore` `SharedPreferences` |
| WeChat mini programs | Platform-encrypted storage (`wx.setStorage` / `wx.getStorage` with `encrypt: true`, base library 2.21.3+); unavailable on older base libraries |
| Web, other mini programs | Unavailable (`core.ErrSecretsUnavailable`): browsers have no OS-protected client secret store (keep web sessions in server-set `HttpOnly` cookies), and QQ, Alipay and Douyin mini programs lack encrypted storage |

The service is the app's ID (`core.App().ID`) by default. An app that moves from another framework keeps its users signed in by naming the items that framework wrote: `Service`, and on Apple platforms `AccessGroup` and `Accessible`. On Android, set `Service` to `expo-secure-store`'s `keychainService` (`"key_v1"` by default) to read values it encrypted into `SharedPreferences`; writing or deleting clears the migrated entry. On iOS, `core.SecretAfterFirstUnlock` lets background work read an item while the phone is locked, and a `ThisDeviceOnly` accessibility keeps it out of backups. On macOS an `AccessGroup` moves the items to the data protection keychain, which needs a build signed by a team whose `keychain-access-groups` entitlement lists the group, configured under `macos.entitlements` in `nsgo.yaml` for both Mac App Store and Developer ID builds.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` keeps an app's secrets in memory, in `App.Secrets`, and `nsgotest.Secrets` starts a test with some, so a test never touches the machine's keychain. A TSX package reaches the store with the `secrets` capability, under a service of its own.

## Networking

`cx.Env().Network()` reports whether the system has a usable route (`Online`) and whether it considers that route metered or expensive (`Expensive`). Bindings follow it like any other environment value. `cx.OnNetworkChange(func(core.Network))` calls the handler on the engine's goroutine for each change while its scope lives. The system's route says nothing about whether a particular server answers. A target that cannot report connectivity defaults to online, and unknown cost defaults to false.

Set `core.Environment{Network: &core.Network{Online: false}}` through `nsgotest.Environment` or `App.SetEnvironment` to test offline behavior.

Go's own `net/http` makes requests on every target, with the system's TLS and proxy settings. Signals belong to the engine's goroutine, so a response from another goroutine reaches them through `cx.Post`.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` runs its function on the graph's goroutine, where signals may be set. On Android the manifest asks for the internet permission, and image downloads cache themselves on disk.

`core.NewResource` does the same for a value loaded from a source. Its fetcher runs on a goroutine of its own and loads again when the source changes:

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()` and `items.Loading()` are signals that bindings follow. A new query cancels the request still running through `ctx`, so a late answer never replaces a newer one. `items.Refetch()` retries, keeping the last list while it loads, and `items.Mutate` sets a value at once for an optimistic update.

`core.Suspense` shows a fallback while the resources its content reads load their first value, so a screen needs no loading flag of its own:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

The content stays mounted but hidden while it waits, so it shows at once when the values arrive. A refetch keeps it shown with the last value.

### Discovering local services

On macOS, iOS, Android, Windows and Linux, `cx.AdvertiseService` publishes a listening port and TXT attributes. `cx.BrowseServices` reports resolved services with their host, port and TXT attributes. `Host` may be a DNS hostname or an IP address. When a service leaves, the callback receives it again with `Removed` set. Check `cx.Env().CanDiscoverServices()` for platform support. Both calls return a stop function and end when their scope is disposed. A platform error is reported asynchronously on the engine goroutine. Linux uses Avahi and reports an error if the daemon is unavailable.

For Bonjour on Apple platforms, declare the service types and explain local network access in `nsgo.yaml`. The system manages the permission prompt and remembers the user's choice.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android uses `NsdManager`. Android 12 and earlier and Android 13 before T Extensions 7 require `CHANGE_WIFI_MULTICAST_STATE` for Wi-Fi discovery. Standalone builds declare that permission. If you embed NSGo in an Android host, add it to the host manifest for those devices. Advertised TXT values must be valid UTF-8 on devices without Android's raw TXT API (API 37 or T Extensions 22). Browsing preserves raw TXT values. If an Android host targets API 37 or higher, it must declare and request `ACCESS_LOCAL_NETWORK` on Android 17 or later.

Use `nsgotest.LocalServices` to supply browse results in a headless test.

## Background downloads

Use `cx.Download` and `cx.OnDownloadUpdate` in standalone iOS and macOS apps, and on Android. Embedded Apple views have no transfer host. Android apps enable `android.background_downloads: true` in `nsgo.yaml`. Start a download from a user action while the app is visible. Use an absolute destination with an existing parent directory the app can write.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

The transfer uses GET and accepts optional request headers. It uses the normal HTTPS trust store unless `CertificatePEM` pins the server's leaf certificate. The pin also requires a matching hostname and a valid certificate. Use the final URL as the service requires a successful response from that URL.

On Apple, a self-signed certificate at a public hostname needs an App Transport Security exception for that host in `apple.info`. The transfer still checks its exact certificate pin, hostname and validity. A local NAS connection also needs an `NSLocalNetworkUsageDescription` explaining access, and unqualified or `.local` hosts use `NSAllowsLocalNetworking`. [Apple's ATS settings](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

The Android foreground service continues after the activity leaves the screen. It posts a progress notification, subject to Android's notification permission. Progress uses byte counts and sets `Total` to -1 until known. Success atomically replaces the destination. `task.Pause()` keeps the partial file, `task.Resume()` uses an HTTP range when the server supplies a validator, and `task.Cancel()` removes the partial file.

iOS uses a system background URLSession that continues transfers while the app is suspended. The app reconnects its session on launch and publishes files before acknowledging background completion. macOS transfers run in process. Apple platforms use native resume data for paused downloads and keep request metadata in private app storage until a terminal result is saved. A server that cannot resume starts the transfer again.

Register `OnDownloadUpdate` again when the app starts to see active tasks and saved results. Use `p.Task.Forget()` after handling a final result. Android can stop the service, for example when its foreground transfer time expires. The app then receives `DownloadInterrupted`. On Windows, Linux, tvOS, watchOS and the web, `Download` returns `ErrDownloadUnavailable`.

## Speech and the lifecycle

`cx.StartSpeech` recognizes speech with the system's recognizer where `cx.Env().CanRecognizeSpeech()` says it exists (macOS, iOS, Android, Windows and browsers exposing `SpeechRecognition` or `webkitSpeechRecognition`), and answers with the transcript as it changes. On Apple platforms declare `NSMicrophoneUsageDescription` and `NSSpeechRecognitionUsageDescription` in `apple.info`; on Android add `RECORD_AUDIO` to `android.permissions`. In a headless test, `app.Speech` holds a `nsgotest.SpeechSession` while recognition is active. Use `app.HearSpeech` and `app.EndSpeech` to deliver results.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` says whether the app is active, inactive or in the background: on desktops the window `cx` is in, so a minimized window's content can pause while another window shows, and in the app's own scope all its windows together. `desktop.Minimized(cx)` says whether that window is minimized, and `desktop.FrontmostApp(cx)` which application is in front on the desktop, the app itself included, and whether its window is in full screen, as a desktop pet or a time tracker follows it; `desktop.CanReadFrontmostApp(cx)` says whether it is followed, on macOS, Windows, X11 and supported Wayland compositors. `cx.OnQuit` cleans up as it quits, however it does, with 200 ms for its work. `desktop.OnQuitRequest` can cancel or defer a desktop Quit menu item or `desktop.RequestQuit(cx)`. On Windows and Linux it also runs after `OnClosing` when closing a window would end the app. `desktop.RequestRestart(cx)` asks the same handlers before restarting; `desktop.Restart(cx)` remains immediate. Updater relaunches on Linux and Windows also ask, so saving can cancel or defer them. When the system reports logout, restart or shutdown, `q.Forced()` is true and the app proceeds through `OnQuit`. Check `Forced()` before asking the user for a decision. `cx.SetAppearance` forces light or dark, `cx.SetLocale` chooses the language, `cx.OnKey` binds a keyboard shortcut, which rests while what its owner shows is hidden or under a dialog unless `core.KeyOptions{OverModals: true}` keeps it, which takes its key before the application menu does unless `YieldsToMenu` leaves it to an enabled menu item taking the same shortcut where the focus is (while a text field or web view has the focus, a `MenuNotEditing` item leaves the binding its key and keeps the key from later items with the same shortcut, and Linux's menu takes no key pressed without Control, Alt or a logo key, so the binding keeps such a key there), which works in the window `cx` is in unless `AllSurfaces` binds it in every window and panel of the app, those opened later too, for an app-wide shortcut, which rests in all of them while what its owner shows is hidden in the window it is bound in, and which leaves a focused control the keys it uses, a button its Return and Space and a slider, tab bar or radio group its arrows, unless bound with `core.KeyAlways` or `core.KeyNotEditing`; `cx.OnOpenFiles` hears documents opened on macOS, Windows, Linux and iOS. iOS imports regular files into app-owned temporary storage; move them to `cx.DataDir()` to retain them. On macOS, closing the window that supplied the application menu restores the most recent surviving window's menu, or the default when none remains.

On Wayland, frontmost-app tracking requires version 2 or later of the compositor's wlr foreign-toplevel protocol; other compositors report `CanReadFrontmostApp` false. Its ID is the Wayland application ID, its name comes from the desktop entry when available, and its PID is 0. Packaged Windows apps use their AUMID instead of a frame-host path. macOS follows AppKit's fullscreen presentation state rather than inferring it from window size.

An editing role in the application menu takes the focused editor before its `OnSelect` fallback. On macOS, Undo and Redo follow that editor’s history, and Cut follows its selection, including while the app is inactive; with no editor, the fallback stays enabled.

On iOS, give `cx.OnKeyWith` a `core.KeyOptions.Title` function to offer the shortcut in the system's keyboard menu while the binding can fire. `Group` names its menu on iPadOS 26 and later. The functions can read the app's catalog so the shortcut titles follow the language. Use `app.KeyCommands()` in tests to inspect the current list.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` is the name the user gave the device, and `cx.Env().DeviceModel()` its hardware model, such as `Mac15,7`, `iPhone15,2` or `Google Pixel 8`, so an app can tell two paired devices apart. macOS, Android, Windows and Linux give both; iOS gives the model, and the name only to an app with Apple's user-assigned device name entitlement. watchOS gives its hardware identifier, HarmonyOS its market name or product model, and mini programs the platform's brand and model; those targets leave the user-assigned name empty. The web gives neither. Tests set them with `nsgotest.Environment`.

`cx.Env().ThermalState()` reports `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` or `ThermalCritical` on Apple platforms and Android API 29 or later. `cx.Env().LowPowerMode()` reports `core.LowPowerOn` or `LowPowerOff` on Apple platforms, Android, Windows and Linux sessions with power-profiles-daemon. Where the system gives no value, the reads return `ThermalUnknown` and `LowPowerUnknown`. Each read follows changes independently, so an app can adjust its workload while it runs. In tests set `ThermalState` and `LowPowerMode` on `core.Environment` through `nsgotest.Environment` or `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` reads the pointer even outside the app's windows on macOS, Windows and X11. It takes one sample per call. The point is in the same screen coordinates as `desktop.Monitors(cx)` and `Window.ShowAt`. On Wayland and other unsupported targets, or if the scope lacks `core.AccessCursorPoint`, `available` is false.

`desktop.SetWindowConstraints` limits content size and controls resizing, minimizing and maximizing; set `WindowOptions.Constraints` when opening a window or use `nsgo.WindowConstraints` for the main window. Linux applies nonfixed maximums on X11 and removes forbidden minimize/maximize buttons from GTK's native titlebar on X11 and Wayland. Wayland currently supports fixed sizes only for an upper bound.

`desktop.SetWindowLevel` floats a window above the others, and `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` or `WindowOptions.Workspaces` keeps it on every workspace, as a desktop pet, a HUD or a timer is: on macOS on every Space and over other apps' full screen spaces, out of Cmd-`, and on Linux on every virtual desktop of an X11 window manager. Windows and Wayland leave it on the one it opened on, and `desktop.CanSetWindowWorkspaces` says whether it takes effect: true on macOS and under an X11 window manager that supports `_NET_WM_DESKTOP`. A main window goes on every workspace by calling `desktop.SetWindowWorkspaces` from its content. In tests, `nsgotest.App.Window` lists the requests, and `nsgotest.Environment` sets `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` binds a global shortcut, which works while any app is frontmost, on macOS, on Linux and in a Windows app run with `nsgo.Run(App)`. Its handle's `State` says whether the system took it: `HotKeyPending` until it answers, then `HotKeyRegistered`, `HotKeyTaken` when another registration holds the combination, or `HotKeyUnsupported` for a key or a platform without global shortcuts, so a shortcut recorder can ask for another combination. On macOS a combination counts as taken only when the app holds it already or another app registered it exclusively. On Wayland the compositor asks the user to bind the shortcut: it is registered once they have, and taken when they or the compositor leave it out. In tests, `nsgotest.HotKeyTaken` and `nsgotest.HotKeyUnsupported` make combinations answer so.

With `desktop.TitleBarHidden`, Windows and Linux offer the application menu as a popup on Alt pressed and released alone or F10. Alt with a top-level label’s first letter or digit opens that submenu or chooses that command, in the label’s script. App bindings keep their keys; Shift+F10 remains the context-menu key, and other Alt chords stay with the system. Escape closes the popup and restores keyboard focus.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Desktop resizes on macOS, Windows and Linux wait up to 16 ms for the new layout before drawing. Keep reactive computations and event handlers quick so layout can meet that deadline; perform slow I/O on a goroutine and post results through `cx.Post`. A busy app continues resizing and shows its layout when it catches up.

`desktop.SetWindowFrame` moves a window when `WindowFrame.Move` is true, using `X` and `Y` in points from the main screen's top left. Unset dimensions keep their current sizes. Check `desktop.CanPlaceWindows`: macOS, Windows and Linux on X11 support positioning; an ordinary Wayland toplevel keeps the compositor's position. `desktop.Frame` follows the native window's position on these desktops.

Set `desktop.WindowOptions.Parent` to an open window of the app to close both when the parent closes. On desktops the child stays above its parent and follows it when it hides or minimizes; a child explicitly hidden stays hidden when the parent returns. On macOS it also centers over its parent when `Frame` is unset and follows it into full screen.

`desktop.SetWindowPointerEvents` lets the pointer through a window to the windows below it, of any app: `WindowPointerNone` everywhere, as a heads-up display does, and `WindowPointerPainted` wherever no view draws something or hears the pointer, as around a desktop pet that still takes presses, drags and right clicks on itself; the title bar the system draws counts as nothing drawn. `WindowOptions.PointerEvents` and `PanelOptions.PointerEvents` open a window so. A `Desktop.WindowDrag` region still drags inside a pressable ancestor; a nearer pressable or editing control keeps its own input instead of dragging the window. A window that takes no pointer hears no presses, so the app gives it back from a shortcut, a tray icon or another window. Code an `Access` holds needs `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

On macOS and Windows, an image in a painted window can set `Image.HitTestAlpha` (TSX `hitTestAlpha`) to a value above 0 and at most 1. For example, 0.5 takes the pointer on picture pixels that are at least half opaque. The threshold also applies to the image's `OnPress`. A view background or border still counts as painted. Zero uses the whole image view, as do images on Linux.

In a painted macOS window, `Video.HitTestAlpha` (TSX `hitTestAlpha`) applies the same threshold to decoded video frames. It works with `Controls` off and an untransformed video track, such as HEVC with alpha for a desktop pet. Zero, an unavailable frame, and the other desktop hosts use the video view's bounds.

`desktop.NewTray` can draw a live icon with `TrayOptions.View` on macOS and Windows. Windows fits the view's transparent rendering to the notification area's icon size and updates it when the view changes or an image finishes loading; use `TrayOptions.OnClick`, `Panel` and `Menu` for its actions. Linux currently shows PNG icons; drawing its tray view remains a goal.

`desktop.OpenPanel` opens a panel such a shortcut or a tray icon shows: a borderless window above the others. Like `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` and `core.NewHolder`, it belongs directly to the `cx` you pass rather than the running handler's owner, so passing the app's scope from a button's `OnPress` keeps the window, panel or player alive after that button's scope goes; reactive primitives (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) instead belong to the running owner, which `cx.Run` switches. A popup card draws itself on a panel with `Transparent`, which has no background, frame or shadow of its own, so the card's rounded corners and `BoxShadow` are all that shows, with room for the shadow inside the panel; `FitContent` sizes the panel to its content, up to `Width` and `Height`, before it first shows and as the content changes. `desktop.Transparent` says whether the panel draws nothing of its own now, as it does for a transparent window, where `desktop.CanMakeWindowsTransparent` only says whether a window could be: where it is false, as on Linux without a compositor, give the content a background.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` shows `TrayOptions.Title` beside its image or drawn view on macOS. For a custom popup, read `tray.ClickBounds()` inside `OnClick` or `Menu` and pass the available rectangle to `panel.ShowAt(bounds, core.PlaceBottom)`. It uses the same screen points as `Env.Displays` and includes the title. Bounds are unavailable before the first click, after removal, on Linux, and when the Windows shell cannot supply them. Each click replaces the previous bounds, so an unavailable result never reuses an old rectangle. Setting `TrayOptions.Panel` keeps the built-in click-to-toggle behavior instead.

`PanelOptions.Dock` keeps a panel centered along a display's usable edge through `Show`, `Focus`, `Toggle` and content resizing. Set `Edge` to `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` or `PlaceRight`, `Display` to an `Env.Displays` ID (empty for the primary display), and `Margin` to the distance from that edge in points. A docked panel cannot use `ShowAt` or `MoveTo`; `SetWindowFrame` changes only its size. A tray icon shows it at its dock rather than under the icon.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` reports docking support separately from arbitrary window placement. macOS, Windows and X11 support it; Wayland needs the optional runtime library `libgtk4-layer-shell.so.0` and a compositor implementing `zwlr_layer_shell_v1`. NSGo loads that library before GTK; an embedding host that already loaded Wayland must preload gtk4-layer-shell before starting the host. A missing library, protocol or selected display leaves the panel hidden and calls `OnPlacementError`. A Wayland dock uses the top layer without reserving workspace: `NoActivate` leaves the keyboard with the current window, while a panel without it takes exclusive keyboard focus until hidden. Ordinary Wayland windows retain their usual compositor placement.

A shortcut recorder lets the user choose such a key. A `Focusable` view's `OnKeyDown` hears each key that goes down while it has the focus, before the app's bindings, with the modifiers held, a modifier pressed alone included; returning `true` keeps the key from the bindings, so recording Command+S does not save. `KeyEvent.Shortcut` writes the press as `OnKey` and `RegisterHotKey` take it, each modifier by name ("ctrl+shift+k", "alt+space", "shift+meta+k" for Command+Shift+K), and "" for a modifier alone. Its `OnKeyUp` hears each of those keys come up, even after `OnKeyDown` consumed it or the focus moved, with the modifiers still held. Both work on macOS, Windows, Linux, the web, where the browser keeps the shortcuts it gives no page, Apple TV and in `nsgotest`; Android delivers hardware keyboard events to focused views asynchronously, so their `OnKeyDown` return value cannot consume the key. iPhone and iPad support polling hardware keys. A shortcut the app registered globally takes its key before the view, so a recorder unregisters it while it records.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

A game or a canvas polls the keys instead. `core.KeysDown(cx)` lists the keys held on the window and `core.KeyHeld(cx, "left")` says whether one is: tracked reads, which a `RequestAnimationFrame` loop reads too. The first read has the platform report every key from then on, so a key already held stays unknown until it is pressed again; keys typed into a field or a web view, or while an input method composes, are never held, and a key still held as the window loses the focus comes up then. They work where `OnKeyUp` does and on iPhone and iPad with a hardware keyboard.

For mouse buttons, `View.OnMouseDown` and `View.OnMouseUp` deliver a `core.MouseButtonEvent` for every button (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) on macOS, Windows, Linux, the web and `nsgotest`, without making the view a keyboard focus stop. `core.MouseButtonsDown(cx)` and `core.MouseButtonHeld(cx, core.MouseButtonRight)` are tracked reads of the buttons held on the window, clearing automatically when the window loses focus.

For a control tied to a physical position, read `KeyEvent.Code`, `core.KeyCodesDown(cx)` or `core.KeyCodeHeld(cx, "KeyW")`. The code uses `KeyboardEvent.code` names: on AZERTY the QWERTY W position has `Code: "KeyW"` and `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` gives the active layout's display label for a prompt. On iOS, Android and browsers without a layout map, it uses the last key name reported at that position and returns an empty string until a key is seen.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Keeping the display awake

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` holds the display on until the returned release function runs or the current scope is disposed. On Android the hold applies while the app is visible. On the web it works in a secure context with Screen Wake Lock support, and the page requests the lock again when it becomes visible. Safari can require a press or key before granting the first request. Computer-only holds have no effect on these two platforms.

## Pointer lock

In a game camera or a drag that needs to continue at the screen edge, call `ref.LockPointer()` from a press on the view that holds the ref. The pointer stays in place and is hidden while the view receives relative movement in `OnPointerDelta(dx, dy)`. Positive values move right and down.

Call the returned function to release the lock. Escape, a window losing focus, or hiding or removing the view also releases it. Check `cx.Env().CanLockPointer()` to see whether the surface supports it. The web requires a user gesture such as a press. Linux on Wayland requires a compositor that supports pointer constraints and relative motion.

## Sound limits

Use `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` before loading sounds to change the engine's limits. `MaxEncodedBytes`, `MaxDecodedBytes` and `MaxPlaybacks` can also be set. Zero fields use the defaults of 8 MiB encoded and 5 seconds per sound, 64 sources, 16 MiB decoded across sounds, and 32 overlapping playbacks per output. Existing sounds stay loaded and count toward the new limits. Restricted scopes keep the default limits within the engine's budget. Android's SoundPool plays at most 32 sounds at once and decodes at most 1 MiB of each, whatever the limits.

## Full screen

`desktop.SetFullScreen(cx, true, displayID)` fills the display named by `cx.Env().Displays()` on macOS, Windows and Linux. Omit the ID to use the window's current display. An unknown ID leaves the window where it is. Repeating the same request has no effect. `desktop.SetFullScreen(cx, false)` leaves full screen and restores the window's previous frame. On the web, enter from a press handler. The browser chooses the display, and `cx.Env().FullScreen()` follows entry and exit, including Esc.

## Displays

`cx.Env().Display()` returns the window's current `core.DisplayInfo` and updates bindings when the window moves to another display. `cx.Env().Displays()` lists the displays the platform reports. `RefreshRate` and `MaxRefreshRate` are in Hz, with zero for an unavailable rate. iOS reports the panel's maximum. The web reports the current screen and marks its frame callback measurement with `RefreshRateEstimated`. In tests, set `Displays` and `DisplayID` through `nsgotest.Environment`.

## Speaking text

`cx.Speak` reads text aloud in the system's voice for a language where `cx.Env().CanSpeak()` says the platform speaks: macOS, iOS, tvOS, Android, Windows, the web and Linux with speech-dispatcher. One utterance speaks at a time: another `Speak` cuts the one speaking, and so do `cx.StopSpeaking` and the end of the scope that spoke it. `done` hears once how it ended: finished, stopped, or failed with a `core.SpeakError`, such as `core.SpeakLanguageUnavailable` when no voice installed speaks the language.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` and `Pitch` run from 0.5 to 2, 1 being the voice's own. `cx.Voices("ja", ...)` lists the voices installed for a language, so the app can ask the user to install one it lacks or let them pick one, whose `ID` goes in `core.Voice`. Browsers speak only after the user has pressed something on the page, and fail with `core.SpeakNotAllowed` before. A TSX package's Go components speak only with the `text_to_speech` capability.

## Looping audio

Set `audio.Voice.Loop` to repeat a sound playback until `Stop` or `Close`. For streamed music, set `audio.PlayerOptions.LoopStart` and call `SetLoop(true)`. The first pass includes the intro. Later passes start at the offset. A zero offset repeats the full track.

On macOS 14 and iOS 17 or later, a finite track uses the native queue to prepare each repetition before the current one ends. Earlier Apple versions seek to the loop start when the current pass ends, with a short gap. On Linux and the web, a looping player decodes a track of up to 32 MiB and 5 minutes into memory, about 23 MB a minute, and repeats it without a gap. On Linux a larger or longer track streams through its own decoder and repeats without a gap too; on the web it seeks back at the end, with a short gap. On Windows and Android, a loop with a start seeks back to it at the end, with a short gap. On the web a looping player at a rate other than 1 seeks back at the end too, as the browser's gapless playback would shift the pitch with the rate. On the web a looping player, like a sound, plays only after the user pressed something on the page, and fails with `NotAllowed` before that.

## Biometrics

`cx.BiometricAvailability` reports whether Face ID, Touch ID or a device fingerprint is enrolled; Android does not say which kind is enrolled, so there it reports a fingerprint when one is enrolled and otherwise face when the device has face hardware. `cx.AuthenticateBiometric` asks the system to confirm the user with a reason, which must not be empty, and reports success, failure, cancellation or unavailability. It does not choose a passcode fallback for the app.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Face ID apps declare `NSFaceIDUsageDescription` under `apple.info`; Android apps list `USE_BIOMETRIC` under `android.permissions`.

## Signing in

`cx.AuthSession` signs the user in with an OAuth provider in the system's authentication session rather than a web view: `ASWebAuthenticationSession` on macOS, iOS and watchOS, a Custom Tab or browser on Android, the default browser on Windows, Linux and a Mac without a scheme, and a popup on the web. On the web, the redirect page also sends its callback over a same-origin `BroadcastChannel`, so a sign-in completes when the provider's `Cross-Origin-Opener-Policy` cuts the popup off from the page. Such a popup reads as closed to the page, so closing a popup after it showed the provider's page ends the session cancelled only after 5 minutes; give the session a cancel button. The app keeps the protocol: it makes the PKCE verifier and the state, builds the authorization URL for the redirect it is given, and checks the callback before trading its code. Cancelling is not failing: `errors.Is(err, core.AuthCancelled)` tells the user closing the session, or the returned `cancel` ending it.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

The redirect is `com.example.app:/oauth2redirect` where a scheme is set (Windows, Linux and Android need it in `nsgo.yaml`'s `url_schemes` so the OS hands the callback to the running app; macOS's, iOS's and watchOS's system session catches it without that declaration), and `http://127.0.0.1:<port>/oauth2redirect` on the desktops' loopback, so register both with the provider, often as clients of their own. Leave `Scheme` empty for loopback alone, or set it to `https` with `Host` for the app's associated domain, which iOS 17.4, macOS 14.4 and watchOS 10.4 catch. watchOS requires a scheme or associated HTTPS domain; it has no loopback fallback, and NSGo's existing watchOS 26 minimum still applies. A closed browser tab tells a loopback session nothing, so give it a `Timeout` or a cancel button. `Ephemeral` asks macOS, iOS and watchOS not to share the browser's cookies, and requests a private Custom Tab on Android where the browser supports one; do not assume cookie isolation on Android. tvOS and mini programs answer `core.AuthUnavailable` for now. In TSX, `AuthSession.start(url, options)` needs the `auth_session` capability and gives a session whose `result` resolves with the callback, or null once cancelled.

Android consumes a matching callback before ordinary `OnOpenLinks`, and returning without a callback ends the session cancelled. Its callback must use an app-owned private scheme; an empty scheme, HTTPS callback or undeclared scheme returns `core.AuthUnavailable`, as does an Android runtime built before sign-in sessions.

A browser session consumes only its matching callback link; unrelated and late links still reach `OnOpenLinks`. Starting another browser session for the same callback URI cancels the earlier one; distinct URIs can wait in different windows.

Use the exact redirect passed to the authorization URL function. Web and desktop loopback redirects encode Unicode paths and resolve dot segments before the function receives them; a loopback `Path` is decoded text, so a literal `%` is escaped too.

Mini programs use `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` instead. The callback receives a `core.MiniProgramCredential` with its `Platform` (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` or `MiniProgramAlipay`) and a nonempty, short-lived `Code`. Send it to your server for the host's code exchange; keep the app secret there. This is `wx.login`/`qq.login`/`tt.login`, or Alipay's basic `auth_base` authorization, without requesting profile information. It does not open a URL or run an OAuth URL builder.

`cx.Env().CanSignInMiniProgram()` reports the host API's availability. Other targets answer `core.MiniProgramSignInUnavailable`; code without `core.AccessMiniProgramSignIn` gets `AccessDenied`. Douyin asks silently by default: an unsigned host returns `MiniProgramSignInNotSignedIn`, and `MiniProgramSignInOptions{Prompt: true}` permits its host-account login window. Closing that window returns `MiniProgramSignInCancelled`. An anonymous device code never counts as a signed-in user. Tests set `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` and `app.MiniProgramSignIn`; `app.MiniProgramSignIns` records requests. TSX does not run in mini programs.

An app whose web views should be signed in as the user already is elsewhere, such as in their own browser, writes the cookies of that sign-in with `cx.SetWebCookies`, HttpOnly ones included, which page scripts cannot set, then loads the pages: done runs once the store holds them. The store is the persistent one every web view of the app shares, so cookies with an `Expires` outlive the app; Incognito views do not see them, except on Android, which keeps one store per app. The web, tvOS and watchOS answer `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

To remove one cookie, write it again with the same name, domain and path and `Expires` in the past; a zero expiry makes a session cookie. Wait for the callback before loading a page that should observe the removal.

The other way round, `cx.WebCookies` lists the store's cookies, HttpOnly ones and their domain, path, expiry and flags included, which `document.cookie` cannot read, whether or not a page of the site is showing: a `core.WebCookieFilter` picks those a request to a `URL` would send, or those of a `Domain` and every host under it, and the zero filter all of them. An app calling the site from Go with the user's sign-in, or showing where the user is signed in, reads them this way. `cx.ClearWebsiteData` signs the user out of every site: it removes the store's cookies, local and session storage, IndexedDB, service workers and caches, and done runs once they are gone. Android's `CookieManager` answers only the names and values a URL would send, so there only a `URL` filter lists, filling `Domain` with its host and leaving the other fields empty, and a `Domain` filter or the zero one answers `Unavailable`; clearing there leaves the HTTP cache. The web, tvOS and watchOS answer `Unavailable` to both.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Sign in with Apple

`cx.SignInWithApple` shows the system's Sign in with Apple sheet from the app's own button on macOS, iOS and tvOS, where `cx.Env().CanSignInWithApple()` is true. It answers once with the credential, or with an error: `core.AppleSignInCancelled` when the user closes the sheet, which is not a failure, else `AppleSignInFailed` or, on other platforms, `AppleSignInUnavailable`. Apple gives the email and the name only on the user's first sign-in to the app, so keep them then. `Nonce` goes into the request as its SHA-256 in lowercase hex, which the identity token's `nonce` claim holds: the server hashes the nonce it gave the app and compares. `cx.AppleCredentialState` tells whether the user has since revoked it. In headless tests, `app.AppleSignIn` receives a `nsgotest.AppleSignInRequest` and supplies the credential or error.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

The app must be signed with the Sign in with Apple entitlement, which `build` adds when `nsgo.yaml` asks for it; without it the sheet fails:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode registers the capability for device builds from `nsgo dev`; a provisioning profile in `sign.profile` or `sign.macos_profile` must come from an App ID with Sign in with Apple turned on. macOS grants it only with a profile, so a disk image or a `nsgo dev` run on a Mac cannot sign in, nor can the simulator, whose builds claim no developer entitlement.

## In-app purchases

`cx.Products`, `cx.Purchase` and the calls beside them sell through the App Store on macOS, iOS and tvOS, where `cx.Env().CanPurchase()` is true, in a build whose `nsgo.yaml` asks for StoreKit:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Show the prices the store gives, `DisplayPrice`, never prices of the app's own: the store prices each storefront itself. A purchase answers once, with the transaction or an error: `core.PurchaseCancelled` when the user closes the sheet, `core.PurchasePending` while it waits for approval, as Ask to Buy does, and otherwise `core.PurchaseFailed`, with the store's words, or `PurchasesUnavailable` elsewhere. Send the transaction's `Signed`, the store's JWS, to the app's server to verify, deliver what it bought, then finish it; the store hands an unfinished transaction to the app again at every launch.

`cx.ManageSubscriptions` opens Apple's subscription management UI (the system sheet on iOS, the App Store subscriptions page on macOS and tvOS). `cx.RequestRefund` starts a refund request for a transaction and answers `core.RefundSuccess` or `core.RefundCancelled`.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` returns `PurchaseNotFound` for an already finished transaction, including one returned by `CurrentEntitlements`; a finished non-consumable remains owned.

Register `OnPurchaseUpdate` as the app starts: updates that come before any handler wait for the first. `cx.CurrentEntitlements` answers what the user owns now, to unlock it at launch, and a Restore Purchases button calls `cx.SyncPurchases`, which may ask the user to sign in. `AccountToken` is a UUID of the app's own that the transaction and the App Store's server notifications carry. macOS sells only in a Mac App Store build, and an App Clip sells nothing; Android, watchOS, Windows, Linux, the web and mini programs answer `PurchasesUnavailable` for now. Test purchases with `nsgotest`, whose `Products`, `PurchaseAnswer`, `UpdatePurchase` and `UnfinishedTransactions` stand in for the store. `app.PurchaseRequests` records store operations as `nsgotest.PurchaseRequest` values. In TSX, the `Purchases` module needs the `purchases` capability: `purchase` resolves with `{ status: "purchased", transaction }`, `"pending"` or `"cancelled"`, and `on("update", listener)` hears updates.

## Interface orientation

Call `cx.RequestOrientation(core.OrientationLandscape)` in a full-screen player's scope to ask iOS, Android or the browser for landscape. The latest request whose scope is alive takes effect. When that scope ends, the previous request resumes or the app returns to its usual orientation policy.

On iOS 16 and later the request can rotate the interface immediately. On older iOS versions it restricts the next system rotation. A presented modal's `IOS.SupportedOrientations` takes precedence. On Android, a mix of portrait and landscape directions that the system cannot represent is reduced to one allowed direction, preferring the one already shown. Android may disregard orientation requests in large-screen or multi-window modes.

In a web app, call `desktop.ToggleFullScreen(cx)` from a press handler that opens the player's scope. The browser enters full screen from the press, then NSGo applies the player's orientation request. Leaving full screen releases the lock. The browser console reports unsupported operations and refusals.

`nsgotest.App.Orientation` records the latest requested mask, with zero for the usual orientation policy.

## Store reviews

`cx.RequestReview()` asks for an in-app rating prompt from the App Store on macOS and iOS, or Google Play on Android. `cx.Env().CanRequestReview()` reports whether the service is available. The store decides whether and when to show the prompt and reports no result. Use a development build to test the Apple prompt. TestFlight builds suppress it.

Standalone Android packages include the Play Review SDK, its dialog activity and Play Store visibility query; building them needs no Android SDK or JDK. A device with no enabled Play Store reports the service as unavailable. Test a released package using [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test).

When embedding NSGo, add `com.google.android.play:review:2.0.2` to the Gradle host. In the host's manifest, add `<package android:name="com.android.vending" />` under `<queries>` so the app can see the Play Store. If the host shrinks its code, preserve the Play Review and Tasks APIs with the rules below, as NSGo loads them by name.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

In tests, set `CanRequestReview` in `nsgotest.Environment` and check `nsgotest.App.ReviewRequests`.

## System edge gestures

A game with touch controls at the edge of the screen can call `mobile.DeferSystemGestures` while the controls are active. Use the returned function to release the request early. It also ends when its scope is cleaned up. Requests from multiple scopes in the same window combine.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS uses physical screen edges. Android 10 and later use rectangles in surface points measured from the top-left. Keep the rectangles around the controls that need them. Android reserves its home gesture and caps the area an app can exclude from Back. `nsgotest.App.SystemGestures` gives tests the current combined request on their main surface. Use `Surface.SystemGestures()` for another window.

## Overlays on Android

`mobile.OpenOverlay` shows a view of the app's over other apps on Android, a tree of its own that shares the app's signals, and a foreground service keeps the app running while it shows. The app lists `SYSTEM_ALERT_WINDOW` under `android.permissions`, and the user grants it in the system's settings, which `mobile.RequestOverlayPermission` opens; without it the overlay ends at once and `OnEnd` hears why. It takes touches but leaves the keys and the touches around it to the app in front, unless it is `Focusable` or `Passive`, and moves with `desktop.SetWindowFrame` from its content. Other platforms open none.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Location and motion

`cx.Locate` gets one fix, `cx.WatchLocation` streams fixes and `cx.WatchMotion` streams accelerometer, gyroscope, magnetometer and fused device-motion samples. Each returns a `*core.SensorStream` whose `Stop` cancels it, as the scope that started it ending does; callbacks run on the engine goroutine. The permissions are `core.PermissionLocation`, `PermissionBackgroundLocation` and `PermissionMotion` ([permissions](permissions.md)): starting a stream never asks, and one without its permission fails with `core.SensorNotAllowed`.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` waits for a new fix, 30 seconds by default (`Timeout`); only `MaxAge` lets it take a cached one, which says so with `Cached` and `Age`. `Approximate` tells a fix the user allowed only approximately; Android refreshes such a fix at most every ten minutes, so give `Locate` a `MaxAge` for it there. Coordinates are WGS 84; a measurement the provider lacks has `Valid` false, and `Altitude` is ellipsoidal where `MSLAltitude` is above mean sea level. Motion uses the device's natural axes, acceleration in m/s² (+9.81 on z lying face up), rotation in rad/s and the magnetic field in µT, every 20 ms by default, held to 10 ms–1 s. Under load a stream keeps the newest sample per sensor and counts the ones it replaced in `Dropped`.

Streams go on while the app is inactive and pause in the background, marking the first sample after it with `Gap`; a pending `Locate` keeps the rest of its timeout. `LocationOptions.Background` keeps a location stream going in the background under `PermissionBackgroundLocation`: Apple apps add the `location` background mode, and on Android a foreground service shows a notification with `NotificationTitle` and `NotificationBody` and keeps the app running after its activity ends. Nothing starts a process the system ended.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS and watchOS give location and motion, macOS location, tvOS one fix at a time and Android both through the framework's own providers. A secure browser supplies geolocation fixes and, on mobile, accelerometer, gyroscope and fused motion readings. Call `cx.RequestPermission(core.PermissionMotion, done)` from a press handler on Safari. The browser reports location unavailable without `navigator.geolocation`, motion unavailable on desktops and for magnetometer streams. In TSX, `Sensors` has `permission`, `requestPermission`, `locate`, `watchLocation` and `watchMotion`, under the package capabilities `location`, `backgroundLocation` and `motion`, in milliseconds.

## Audio

The `audio` package plays short sounds decoded once into memory (`audio.LoadSound`) and long streamed audio (`audio.NewPlayer`) where `cx.Env().CanPlayAudio()` is true; elsewhere calls fail with `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` sets per-playback `Volume` (0 to 1, 0 as 1), `Rate` (0.5 to 2, 0 as 1) and `Pan` (-1 full left to 1 full right, 0 centered); values outside those ranges are clamped and NaN takes the default. `Sound.Stop` stops all playbacks of the sound and `Close` frees it, as its owning scope does when disposed. `Player` supports `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` and `Close`. macOS, iOS, Windows, Linux, Android and the web play audio today. Apple, Windows and browser backends support WAV, AAC and MP3. Linux uses GStreamer codecs, and its streamed player plays at 1×. Android pauses a player when its view leaves the screen and limits each decoded sound to 1 MiB. tvOS, watchOS, HarmonyOS and mini programs still report `CanPlayAudio` false.

Browsers require a user gesture for audible playback. Load short sounds ahead of time, then call `Sound.Play` after a click, tap or key press. A sound played before a gesture is silent. A player's `Play` callback receives `audio.NotAllowed` if the browser blocks playback. Retry from a user gesture.

`Sound.Play` returns an `audio.Playback`, which on macOS, iOS, Windows, Linux and the web can set its own volume and rate or stop independently. `Ended()` is reactive and becomes true when that playback finishes, stops, is displaced by the voice limit, or cannot start. Backends without individual controls return an ended handle.

Use `audio.Voice{Muted: true}` to start an effect silently and `playback.SetVolume(0)` to silence an active effect. For long audio, `audio.NewPlayer` streams a source and `player.SetMuted(true)` silences the output while playback continues. Unmuting restores the player's configured volume. `Voice.Volume` and `Player.SetVolume` use zero for default full volume.

## Gamepads

Package `gamepad` reads connected game controllers in the W3C Standard Gamepad layout (`ButtonSouth`/`ButtonA` through `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` reports whether the target supports controllers (macOS, iOS, Windows, Linux, Android and browsers with the Gamepad API).

`gamepad.Pads(cx)` is a tracked reactive read that rerenders scopes when controllers connect or disconnect without rerendering on button or stick changes. Each `gamepad.Pad` exposes its `Family` (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` or `FamilyGeneric`, also classified via `gamepad.DetectFamily`), USB `VendorID` and `ProductID` where the platform reports them, and `CanRumble` for haptic motor support. Inside a `RequestAnimationFrame` loop, `gamepad.State(cx, id)` returns the controller's buttons (`[0, 1]`, with `Pad.Pressed` true at `>= 0.5`) and axes (`[-1, 1]`, negative left/up and positive right/down) snapshotted once per display frame.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` drives the low-frequency (heavy/left) and high-frequency (light/right) rumble motors with intensities clamped to `[0, 1]`, replacing any running rumble on that controller and stopping automatically after `duration`, when the app goes to the background or loses focus (`cx.Env().Lifecycle() != core.LifecycleActive`), when `cx` disposes, or when the controller disconnects. `gamepad.StopRumble(cx, id)` stops rumble early; controllers with `CanRumble == false` ignore rumble calls cleanly.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Call `gamepad.EnableFocusNavigation(cx)` to opt into navigating native focus stops with a controller (D-pad and left stick move focus, `ButtonSouth` activates the focused control and `ButtonEast` triggers back). Active gameplay views call `gamepad.CaptureInput(cx)` to pause focus navigation while they are mounted. In TSX, `nsgo/gamepad` exposes `supported()`, `pads()`, `state(id)`, `Button` and `Axis`, gated by the `gamepad` package capability.

## Audio timing

Use `audio.Now(cx)` as the clock for timed playback. Set `audio.Voice.StartAt` to a future time when `audio.CanSchedule(cx)` is true. Call `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` for the active output route. macOS, iOS, Linux and the web support timed starts. The callback's `known` value tells whether the route reported a latency.

## Game services

Package `gameservices` provides a provider-neutral API for player sign-in, achievements, leaderboards and native Game Center / Play Games overlays where `cx.Env().CanUseGameServices()` is true. Every callback runs on the engine goroutine so it may set signals directly without blocking frames.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` clamps progress to `0..100` (`100` unlocks the achievement). `gameservices.LoadScores` reads ranked `gameservices.ScoreEntry` rows for `Global` or `Friends` over `AllTime`, `Week` or `Today`, and `gameservices.ShowAchievements` and `gameservices.ShowLeaderboard` open the platform's native overlays. macOS, iOS and tvOS reach GameKit via purego; Android will reach Play Games Services through a Java runtime hook that is not in the runtime yet, so it reports `CanUseGameServices() == false` for now; Windows, Linux, the web, watchOS and mini programs report `CanUseGameServices() == false` and answer `gameservices.ErrUnsupported`.

In tests, enable game services with `nsgotest.Environment(core.Environment{CanUseGameServices: true})` and drive each app's isolated fake with `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` and `App.GameServicesShownLeaderboards`.

## Next

- [Notifications](notifications.md): permission, badges and presses.
- [Forms](forms.md): text fields and the keyboard.
- [Embedding](embedding.md): services a host app gives its embedded app.
- [Building and shipping](building.md): the permissions and files a package needs.

### Fitness, regions and bounded motion

`cx.WatchSteps`, `cx.WatchActivity` and `cx.WatchPressure` return scoped streams. Ask for `core.PermissionActivityRecognition` before steps/activity, and before pressure on Apple. Apple supplies all three through Core Motion; Android supplies step counters and pressure, and reports activity classification unavailable. Counts belong to the sample's `Since` window; resumed windows carry `Gap`. Pressure is in pascals, with optional relative altitude in meters. Missing distances, floors and altitude have `Valid` false.

`cx.WatchGeofences` takes `core.GeofenceOptions` with one to twenty distinct `GeofenceRegion` circles (WGS 84 coordinates and radius in meters). It reports `GeofenceInside`, `GeofenceOutside` or `GeofenceUnknown` per region. `Background` requires background location access/declarations and Apple's Always authorization; Android also needs a notification title. Stop or disposal removes its registrations; no subscription survives process death.

`cx.RequestPreciseLocation("route", done)` explicitly asks for precise authorization and answers a boolean, never a position. In `apple.info`, declare `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android needs both coarse and fine permissions. A false answer may leave approximate location authorized.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` requests a finite live stream. It needs `AccessMotion` and `AccessBackgroundMotion`; TSX packages declare `motion` and `backgroundMotion`. iOS may expire its execution lease sooner. Android apps set `android.background_motion: true` and declare `FOREGROUND_SERVICE`; the nonsticky service stops within three minutes even if the requested duration is longer. Duration or OS expiry reports `SensorTimedOut`. Stop/disposal ends the provider and lease; nothing records after termination. Other targets report unavailable.

TSX `Sensors` mirrors these six methods and uses milliseconds. Headless tests feed `FeedSteps`, `FeedActivity`, `FeedPressure` and `FeedGeofence`, and set `PreciseLocationAnswer` for accuracy requests. Physical fitness, region wake-up and suspended-background behavior still need device acceptance.
