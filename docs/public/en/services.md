---
title: Platform services
description: Alerts, the clipboard, files, photos, sharing, storage, secrets, speech, speaking text, networking, signing in, in-app purchases and the app's lifecycle.
group: Platform features
order: 44
---

## Alerts and the clipboard

`cx.ShowAlert` shows the platform's alert or action sheet. Its `Actions` come back by index, and a role draws the cancel and destructive buttons the platform's way.

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

`cx.OpenFile` opens a file the app has in the app associated with its type, and `cx.Share` shows the system's share sheet for files, text or both. A `core.SharedFile` is a path, with the name and type the other app sees. On desktops, `desktop.RevealFile` shows a file or folder selected in Finder, Explorer or the file manager, and `desktop.TrashFile` moves it to the Trash or Recycle Bin; it never deletes permanently, answering `core.ErrNoTrash` and leaving the item in place where the volume has no trash or the trash would not keep it, such as a file larger than the Recycle Bin.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

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

`desktop.QuickLook` shows the system's file preview panel on files, starting at an index, as Space does in the Finder, and shows the new files when called again while it is open. `desktop.CanQuickLook` says whether it can, in a window on macOS; Windows, Linux and the web have no such panel.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Photos and files

`cx.PickImages` shows the system's photo picker and answers with `core.PickedFile`s, whose bytes, name, type and pixel size the app reads. `cx.TakePhoto` shows the camera where the platform has one; `cx.Env().CanTakePhoto` says whether it does. `cx.ScanCode` shows the camera full screen and answers with the first code it reads, a QR code unless `ScanOptions.Kinds` asks for others, or nil when the user closes it; `cx.Env().CanScanCode` says whether the platform has a scanner: iOS and Android devices so far. It needs `NSCameraUsageDescription` on iOS, as `TakePhoto` does, and `CAMERA` in `android.permissions` on Android, which reads every kind but PDF417; a code's text is untrusted input.

In tests, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` and `nsgotest.App.ScanCode` answer picks, camera requests and code scans in place of system UI, headless and under `nsgo test -target`; `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` gives a target without a camera one.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` picks files of any type, `cx.PickDirectories` picks folders as an `fs.FS`, and `cx.SaveFile` saves a file or bytes through the system's save dialog. Both pickers take a `Title`, a `Button` label and a `Start` folder, and `PickFilesOptions.TypesName` names the type menu, so an app can show them in its own language; these are hints, which macOS, Windows and Linux follow, iOS only for `Start`, and Android and the web not at all. `cx.PickScreenColor` picks a color anywhere on the screen where the platform allows it.

Set `core.SaveOptions.Start` to an existing absolute folder to open the save dialog there on macOS, Windows, Linux and iOS. With an empty or unusable hint, or on other targets, the system chooses the folder.

Set `core.SaveOptions.Accept` to validate the chosen destination before writing. It runs on the engine's goroutine. Returning an error completes the save with `saved == false`, the destination and that error. The desktop dialog closes when the user chooses a destination. Android lets the user choose a folder and supplies a tree URI with the requested file name as its `name` query parameter. It creates the document after approval and reports the new document's URI on success. The platform abandons the write if the check does not answer within five seconds. iOS and HarmonyOS report an error for saves requiring a check. The web has no destination path and uses its ordinary save flow.

## Storage

`cx.Storage()` is the app's key-value storage, kept on the device and shared by every graph of the app. Reads are synchronous and read inside a computation subscribe it to changes; `SetItem` returns a `core.StorageFullError` past the quota (`core.StorageQuota`, 5 MiB by default). It is a plain file any program of the user's can read, so keep tokens and passwords in [secrets](#secrets) instead.

`cx.Store()` returns the underlying store, for a host that creates another root with `core.RootOptions.Storage`, such as one running a plugin on a goroutine of its own: both roots then share the app's entries and what is kept on the device. It is the whole store, not the partition `cx.ProvideStorage` gives code the app did not write.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` lists the keys, `RemoveItem` deletes one and `Clear` empties the storage. `cx.DataDir()` and `cx.CacheDir()` give the app's folders for files the key-value storage is too small for; `cx.LogsDir()` gives the platform's directory for log files, and a `Scope.ProvideStorage` gives a subtree a partition of its own.

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
| Android | Encrypted with an AES key of the Android Keystore, in the app's private files, outside backups |
| Web, mini programs | Unavailable |

The service is the app's ID (`core.App().ID`) by default. An app that moves from another framework keeps its users signed in by naming the items that framework wrote: `Service`, and on Apple platforms `AccessGroup` and `Accessible`. On iOS, `core.SecretAfterFirstUnlock` lets background work read an item while the phone is locked, and a `ThisDeviceOnly` accessibility keeps it out of backups. On macOS an `AccessGroup` moves the items to the data protection keychain, which needs a build signed by a team whose `keychain-access-groups` entitlement lists the group, as `macos.entitlements` gives a Mac App Store build.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` keeps an app's secrets in memory, in `App.Secrets`, and `nsgotest.Secrets` starts a test with some, so a test never touches the machine's keychain. A TSX package reaches the store with the `secrets` capability, under a service of its own.

## Networking

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

## Speech and the lifecycle

`cx.StartSpeech` recognizes speech with the system's recognizer where `cx.Env().CanRecognizeSpeech()` says it exists, and answers with the transcript as it changes. In a headless test, `app.Speech` holds a `nsgotest.SpeechSession` while recognition is active. Use `app.HearSpeech` and `app.EndSpeech` to deliver results.

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

`cx.Env().Lifecycle()` says whether the app is active, inactive or in the background: on desktops the window `cx` is in, so a minimized window's content can pause while another window shows, and in the app's own scope all its windows together. `desktop.Minimized(cx)` says whether that window is minimized, and `desktop.FrontmostApp(cx)` which application is in front on the desktop, the app itself included, and whether its window is in full screen, as a desktop pet or a time tracker follows it; `desktop.CanReadFrontmostApp(cx)` says whether it is followed, on macOS, Windows and X11. `cx.OnQuit` cleans up as it quits, however it does, with 200 ms for its work. `desktop.OnQuitRequest` can cancel or defer a desktop Quit menu item or `desktop.RequestQuit(cx)`. On Windows and Linux it also runs after `OnClosing` when closing a window would end the app. When the system reports logout, restart or shutdown, `q.Forced()` is true and the app proceeds through `OnQuit`. Check `Forced()` before asking the user for a decision. `cx.SetAppearance` forces light or dark, `cx.SetLocale` chooses the language, `cx.OnKey` binds a keyboard shortcut, which rests while what its owner shows is hidden or under a dialog unless `core.KeyOptions{OverModals: true}` keeps it, which takes its key before the application menu does unless `YieldsToMenu` leaves it to an enabled menu item taking the same shortcut where the focus is (while a text field or web view has the focus, a `MenuNotEditing` item leaves the binding its key and keeps the key from later items with the same shortcut, and Linux's menu takes no key pressed without Control, Alt or a logo key, so the binding keeps such a key there), which works in the window `cx` is in unless `AllSurfaces` binds it in every window and panel of the app, those opened later too, for an app-wide shortcut, which rests in all of them while what its owner shows is hidden in the window it is bound in, and which leaves a focused control the keys it uses, a button its Return and Space and a slider, tab bar or radio group its arrows, unless bound with `core.KeyAlways` or `core.KeyNotEditing`; `cx.OnOpenFiles` hears the documents a macOS app opens. On macOS, closing the window that supplied the application menu restores the most recent surviving window's menu, or the default when none remains.

On iOS, give `cx.OnKeyWith` a `core.KeyOptions.Title` function to offer the shortcut in the system's keyboard menu while the binding can fire. `Group` names its menu on iPadOS 26 and later. The functions can read the app's catalog so the shortcut titles follow the language. Use `app.KeyCommands()` in tests to inspect the current list.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` is the name the user gave the device, and `cx.Env().DeviceModel()` its hardware model, such as `Mac15,7`, `iPhone15,2` or `Google Pixel 8`, so an app can tell two paired devices apart. macOS, Android, Windows and Linux give both; iOS gives the model, and the name only to an app with Apple's user-assigned device name entitlement; the web gives neither. Tests set them with `nsgotest.Environment`.

`desktop.SetWindowLevel` floats a window above the others, and `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` or `WindowOptions.Workspaces` keeps it on every workspace, as a desktop pet, a HUD or a timer is: on macOS on every Space and over other apps' full screen spaces, out of Cmd-`, and on Linux on every virtual desktop of an X11 window manager. Windows and Wayland leave it on the one it opened on, and `desktop.CanSetWindowWorkspaces` says whether it takes effect: true on macOS and under an X11 window manager that supports `_NET_WM_DESKTOP`. A main window goes on every workspace by calling `desktop.SetWindowWorkspaces` from its content. In tests, `nsgotest.App.Window` lists the requests, and `nsgotest.Environment` sets `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` binds a global shortcut, which works while any app is frontmost, on macOS, on Linux and in a Windows app run with `nsgo.Run(App)`. Its handle's `State` says whether the system took it: `HotKeyPending` until it answers, then `HotKeyRegistered`, `HotKeyTaken` when another registration holds the combination, or `HotKeyUnsupported` for a key or a platform without global shortcuts, so a shortcut recorder can ask for another combination. On macOS a combination counts as taken only when the app holds it already or another app registered it exclusively. On Wayland the compositor asks the user to bind the shortcut: it is registered once they have, and taken when they or the compositor leave it out. In tests, `nsgotest.HotKeyTaken` and `nsgotest.HotKeyUnsupported` make combinations answer so.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

`desktop.SetWindowPointerEvents` lets the pointer through a window to the windows below it, of any app: `WindowPointerNone` everywhere, as a heads-up display does, and `WindowPointerPainted` wherever no view draws something or hears the pointer, as around a desktop pet that still takes presses, drags and right clicks on itself; the title bar the system draws counts as nothing drawn. `WindowOptions.PointerEvents` and `PanelOptions.PointerEvents` open a window so. A window that takes no pointer hears no presses, so the app gives it back from a shortcut, a tray icon or another window. Code an `Access` holds needs `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

`desktop.OpenPanel` opens a panel such a shortcut or a tray icon shows: a borderless window above the others. A popup card draws itself on a panel with `Transparent`, which has no background, frame or shadow of its own, so the card's rounded corners and `BoxShadow` are all that shows, with room for the shadow inside the panel; `FitContent` sizes the panel to its content, up to `Width` and `Height`, before it first shows and as the content changes. `desktop.Transparent` says whether the panel draws nothing of its own now, as it does for a transparent window, where `desktop.CanMakeWindowsTransparent` only says whether a window could be: where it is false, as on Linux without a compositor, give the content a background.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

A shortcut recorder lets the user choose such a key. A `Focusable` view's `OnKeyDown` hears each key that goes down while it has the focus, before the app's bindings, with the modifiers held, a modifier pressed alone included; returning `true` keeps the key from the bindings, so recording Command+S does not save. `KeyEvent.Shortcut` writes the press as `OnKey` and `RegisterHotKey` take it, each modifier by name ("ctrl+shift+k", "alt+space", "shift+meta+k" for Command+Shift+K), and "" for a modifier alone. Its `OnKeyUp` hears each of those keys come up, even after `OnKeyDown` consumed it or the focus moved, with the modifiers still held. Both work on macOS, Windows, Linux, the web, where the browser keeps the shortcuts it gives no page, Apple TV and in `nsgotest`; Android, iPhone and iPad report no keys to a view. A shortcut the app registered globally takes its key before the view, so a recorder unregisters it while it records.

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

## Speaking text

`cx.Speak` reads text aloud in the system's voice for a language where `cx.Env().CanSpeak()` says the platform speaks: macOS, iOS, tvOS, Android, Windows, the web and Linux with speech-dispatcher. One utterance speaks at a time: another `Speak` cuts the one speaking, and so do `cx.StopSpeaking` and the end of the scope that spoke it. `done` hears once how it ended: finished, stopped, or failed with a `core.SpeakError`, such as `core.SpeakLanguageUnavailable` when no voice installed speaks the language.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` and `Pitch` run from 0.5 to 2, 1 being the voice's own. `cx.Voices("ja", ...)` lists the voices installed for a language, so the app can ask the user to install one it lacks or let them pick one, whose `ID` goes in `core.Voice`. Browsers speak only after the user has pressed something on the page, and fail with `core.SpeakNotAllowed` before. A TSX package's Go components speak only with the `text_to_speech` capability.

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

`cx.AuthSession` signs the user in with an OAuth provider in the system's authentication session rather than a web view: `ASWebAuthenticationSession` on macOS and iOS, the default browser with a loopback redirect on Windows, Linux and a Mac without a scheme, and a popup on the web. The app keeps the protocol: it makes the PKCE verifier and the state, builds the authorization URL for the redirect it is given, and checks the callback before trading its code. Cancelling is not failing: `errors.Is(err, core.AuthCancelled)` tells the user closing the session, or the returned `cancel` ending it.

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

The redirect is `com.example.app:/oauth2redirect` where the system's session catches the scheme, which the app need not declare, and `http://127.0.0.1:<port>/oauth2redirect` on the desktops' loopback, so register both with the provider, often as clients of their own. Leave `Scheme` empty for loopback alone, or set it to `https` with `Host` for the app's associated domain, which iOS 17.4 and macOS 14.4 catch. A closed browser tab tells a loopback session nothing, so give it a `Timeout` or a cancel button. `Ephemeral` asks macOS and iOS not to share the browser's cookies. Android, tvOS, watchOS and mini programs answer `core.AuthUnavailable` for now. In TSX, `AuthSession.start(url, options)` needs the `auth_session` capability and gives a session whose `result` resolves with the callback, or null once cancelled.

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

Register `OnPurchaseUpdate` as the app starts: updates that come before any handler wait for the first. `cx.CurrentEntitlements` answers what the user owns now, to unlock it at launch, and a Restore Purchases button calls `cx.SyncPurchases`, which may ask the user to sign in. `AccountToken` is a UUID of the app's own that the transaction and the App Store's server notifications carry. macOS sells only in a Mac App Store build, and an App Clip sells nothing; Android, watchOS, Windows, Linux, the web and mini programs answer `PurchasesUnavailable` for now. Test purchases with `nsgotest`, whose `Products`, `PurchaseAnswer`, `UpdatePurchase` and `UnfinishedTransactions` stand in for the store. `app.PurchaseRequests` records store operations as `nsgotest.PurchaseRequest` values. In TSX, the `Purchases` module needs the `purchases` capability: `purchase` resolves with `{ status: "purchased", transaction }`, `"pending"` or `"cancelled"`, and `on("update", listener)` hears updates.

## Interface orientation

Call `cx.RequestOrientation(core.OrientationLandscape)` in a full-screen player's scope to ask iOS, Android or the browser for landscape. The latest request whose scope is alive takes effect. When that scope ends, the previous request resumes or the app returns to its usual orientation policy.

On iOS 16 and later the request can rotate the interface immediately. On older iOS versions it restricts the next system rotation. A presented modal's `IOS.SupportedOrientations` takes precedence. On Android, a mix of portrait and landscape directions that the system cannot represent is reduced to one allowed direction, preferring the one already shown. Android may disregard orientation requests in large-screen or multi-window modes.

In a web app, call `desktop.ToggleFullScreen(cx)` from a press handler that opens the player's scope. The browser enters full screen from the press, then NSGo applies the player's orientation request. Leaving full screen releases the lock. The browser console reports unsupported operations and refusals.

`nsgotest.App.Orientation` records the latest requested mask, with zero for the usual orientation policy.

## Store reviews

`cx.RequestReview()` asks for an in-app rating prompt from the App Store on macOS and iOS, or Google Play in an embedded Android app. `cx.Env().CanRequestReview()` reports whether the service is available. The store decides whether and when to show the prompt and reports no result. Use a development build to test the Apple prompt. TestFlight builds suppress it.

On Android, add `com.google.android.play:review:2.0.2` to the Gradle host that embeds NSGo. In the host's manifest, add `<package android:name="com.android.vending" />` under `<queries>` so the app can see the Play Store. If the host shrinks its code, preserve the Play Review and Tasks APIs with the rules below, as NSGo loads them by name. Test using [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test). Standalone NSGo Android packages report the service as unavailable.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

In tests, set `CanRequestReview` in `nsgotest.Environment` and check `nsgotest.App.ReviewRequests`.

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

iOS and watchOS give location and motion, macOS location, tvOS one fix at a time and Android both through the framework's own providers, without Google Play services. The web, Windows, Linux and mini programs answer unavailable. In TSX, `Sensors` has `permission`, `requestPermission`, `locate`, `watchLocation` and `watchMotion`, under the package capabilities `location`, `backgroundLocation` and `motion`, in milliseconds.

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

`audio.Voice` sets per-playback `Volume` (0 to 1, 0 as 1), `Rate` (0.5 to 2, 0 as 1) and `Pan` (-1 full left to 1 full right, 0 centered); values outside those ranges are clamped and NaN takes the default. `Sound.Stop` stops all playbacks of the sound and `Close` frees it, as its owning scope does when disposed. `Player` supports `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` and `Close`. macOS, iOS and the web play audio today, with WAV, AAC and MP3 supported there; Windows, Linux and Android still report `CanPlayAudio` false.

Browsers require a user gesture for audible playback. Load short sounds ahead of time, then call `Sound.Play` after a click, tap or key press. A sound played before a gesture is silent. A player's `Play` callback receives `audio.NotAllowed` if the browser blocks playback. Retry from a user gesture.

## Gamepads

Package `gamepad` reads connected game controllers in the W3C Standard Gamepad layout (`ButtonSouth`/`ButtonA` through `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` reports whether the target supports controllers (macOS, iOS, Windows, Linux, Android and browsers with the Gamepad API).

`gamepad.Pads(cx)` is a tracked reactive read that rerenders scopes when controllers connect or disconnect without rerendering on button or stick changes. Inside a `RequestAnimationFrame` loop, `gamepad.State(cx, id)` returns the controller's buttons (`[0, 1]`, with `Pad.Pressed` true at `>= 0.5`) and axes (`[-1, 1]`, negative left/up and positive right/down) snapshotted once per display frame.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Next

- [Notifications](notifications.md): permission, badges and presses.
- [Forms](forms.md): text fields and the keyboard.
- [Embedding](embedding.md): services a host app gives its embedded app.
- [Building and shipping](building.md): the permissions and files a package needs.
