# System floating windows on Android

This is the design of issue [#53](https://github.com/nsgo-dev/nsgo/issues/53), windows over other apps, which `core`, `mobile`, `nsgotest` and the Android runtime implement; the runtime tests and device checks it needs are on the [roadmap](../roadmap.md#application-and-windows). An Android system overlay is a window owned by `WindowManager`, outside the app's activity, and is distinct from a core overlay inside an NSGo surface.

It builds on the engine that outlives its view in [#12](https://github.com/nsgo-dev/nsgo/issues/12)'s first stage and shares foreground-service decisions with long recording in [#38](https://github.com/nsgo-dev/nsgo/issues/38).

## The surface it is

The overlay is a surface the app opens, as `desktop.OpenPanel` opens one on macOS ([application and windows](application.md#surfaces)), with no commands or IDs of its own. The app has one overlay at a time, which can be shown, hidden, moved, resized within the screen's usable area, focused when its mode lets it and closed explicitly; it hosts an NSGo tree of the app's one process, and a future multi-overlay API must define ordering and resource budgets before it is added.

- `mobile` is the API's home, as only phones have it; `desktop` gets no entry point. `mobile.OpenOverlay(cx, OverlayOptions, content)` makes a tree and sends `ItemOpenSurface` with a `SurfaceConfig` whose `ID` is the tree's, of a kind of its own (`SurfaceOverlay`), since its touch, focus and permission are not a panel's. It returns an `Overlay`, `core.Window` as a panel's is, and nil off Android, where `Access` does not grant `AccessOverlay`, and while another overlay is open. The runtime attaches the overlay's view with `engine.Engine.Attach`, and the tree has its own `Env`, as every opened surface's.
- `OverlayOptions` gives the content's size in points, 320 by 180 by default, a `Frame` that places it, otherwise centered, `Hidden`, the input modes `Focusable`, `Passive` and `Accessible`, and `OnEnd`.
- Moving, resizing, showing, hiding and closing are the `ItemWindow` commands every surface takes: `WindowSetFrame`, which the overlay's content sends with `SetWindowFrame` as `desktop.SetWindowFrame` does, `WindowShow`, `WindowHide`, `WindowOrderFront` from `Focus`, and `WindowDestroy`. A removal by the platform ends the tree as a closed window's ends, with `EventWindowEnded` and its reason: `OnEnd` hears it once, as `OverlayEndDenied`, `OverlayEndUnavailable`, `OverlayEndRevoked`, `OverlayEndRemoved` or `OverlayEndFailed`; `Close` and the owner's going do not call it.
- Its owner is the opener's, as for every opened surface: it closes as that owner goes. Opened from the app's own scope (`nsgo.Run(App)`), it lives as long as the engine, which outlives the activity's view; opened from a view's scope, it closes with that view's content.
- The Android runtime owns the `WindowManager.LayoutParams`, the overlay's view, its touch flags and the service. No raw Android `View` or window token crosses into core.

## How Android shows it

Phones open no windows, so the first tree is the activity's view and `OpenWindow` and `OpenPanel` return nil. The overlay is the one surface Android opens beside it. `NsgoEngine` owns the process and keeps its first view, detached or not, until the process ends (#12's first stage). The overlay's view attaches as an opened surface. The pipe names the surface of what belongs to one:

1. Every batch, image and event carries its surface's ID, 0 for the activity's view, and `NsgoEngine`'s first view hands an overlay's on to the overlay's own `NsgoView`, with its own nodes, gestures, animations and environment.
2. The overlay's view reports its size, applied frame and environment with `SURFACE` messages, the first of which attaches its tree, and its end with the reason; a refused overlay is attached only to be ended.
3. The engine measures and lays out text separately for each surface at its own density and frees the cache as the overlay closes. Fonts, shader programs, downloads, folders and the host channel stay with the first view. Each view keeps its own canvas images, released as it closes ([canvas](canvas.md#players)).

A second view of an Activity, a fresh Activity attaching to a running engine and the `SurfaceAttach`/`SurfaceResync` protocol remain #12's later stages ([Android engine and view lifetime](android-lifecycle.md)).

## Permission and user consent

An app declares `SYSTEM_ALERT_WINDOW` in `android.permissions`; the tool never adds it because a package imports the API, and adds with it the overlay's service and the foreground-service permissions it takes, which no other app declares. `mobile.OverlayPermission` answers granted, denied or unavailable, the words of `NotificationPermission` ([services](services.md#notifications-and-the-badge)) without its not asked (`"default"`): Android's special access has no such state, and `Settings.canDrawOverlays` is all the backend reads. It is unavailable where the platform has no overlay ([other targets](#other-targets-and-tests)), the manifest does not ask for the permission or `Access` does not grant `AccessOverlay`.

`mobile.RequestOverlayPermission` opens `ACTION_MANAGE_OVERLAY_PERMISSION` for the app's package, only from a user action, and the answer is read again as the app returns to the front. A revoked grant is not a state of its own, since nothing tells it from one never given after the process restarts: the backend reads `canDrawOverlays` again as the app returns, as the system hides the window and, while an overlay is open, as `AppOpsManager.startWatchingMode` reports the app's `SYSTEM_ALERT_WINDOW` op changed, at once and again a moment later, since Android hides a revoked app's windows by policy without changing their views' visibility and the app may never return to the front, as when the user removed it from Recents. A revocation closes every open overlay with the reason `revoked`, removing its window and stopping the foreground service with the last. A denied grant creates no window and exposes no overlay frame.

Android 13's notification permission is independent: the service runs without it, and its notification then shows only in the system's list of running apps, so the overlay needs no answer about it.

## Service and lifecycle

The runtime adds the overlay's window first and starts the foreground service only once that window is visible (`View.onWindowVisibilityChanged`), after its first frame is committed, trying once more a moment later if the system refuses the start as the window shows. An app targeting Android 15 or later, as every app does unless `android.target_sdk` is below 35, that holds `SYSTEM_ALERT_WINDOW` may start a foreground service from the background only while a `TYPE_APPLICATION_OVERLAY` window of its own is visible, else `startForegroundService` throws `ForegroundServiceStartNotAllowedException`; in this order an overlay opened from the background, by a timer or a notification's action, starts its service on every version. The service, of the `specialUse` type with its subtype property, keeps the process in the foreground with an ongoing notification titled with the app's name; the window stays the first view's. A start refused all the same, or a service the system or the user stops, closes the overlay with `failed` or `removed`, rather than leaving one whose process the system may kill. The service stops as the last overlay closes.

The service may survive an activity rotation, task removal or a detached `NsgoView`, but not the end of the process: as the process ends the runtime removes the overlay's window and stops the service. A standalone app's `NsgoActivity` that ends while an overlay shows, as when the user removes its task, keeps its view's process running off screen unless the app quit, and the next `NsgoActivity` shows that view again, as the engine serves only its first view; the process ends with the last overlay if no activity took it back. Process death does not restore it; the app opens it again as it starts, as nothing tells it from an app that never opened one.

The app hears the overlay shown and hidden through `Visible`, focused through its tree's `Env().Lifecycle`, active only while the window has the focus, its frame through `Env().WindowFrame`, with `Env().CanPlaceWindows` true as `SetWindowFrame` places it, and its end through `OnEnd`. Every callback runs on the engine's goroutine, never on the service's binder or window-manager thread.

## Layout and input

An overlay's frame is in points, as every window's (`SurfaceConfig.Width` and `Height`, `WindowFrame`), and the runtime converts it to pixels with the display's density at the Java boundary, as `NsgoView` converts sizes and safe areas. The backend keeps a requested frame wholly on the display's usable area, shrinking it to fit (`WindowFrame.Within`), and reports the frame it applied. Rotation, display changes, cutouts and system bars move the window back onto the new usable area, at the size the app asked for where it fits, and trigger an environment update with that frame before the next commit.

By default the overlay takes touches but not the focus: `FLAG_NOT_FOCUSABLE`, which also lets touches outside the window reach the app below it (`FLAG_NOT_TOUCH_MODAL`), so presses and drags inside reach the overlay and keys stay with the app in front. `Focusable` drops `FLAG_NOT_FOCUSABLE` for text input and keeps `FLAG_NOT_TOUCH_MODAL`, so touches outside still go below. Its content is hidden from screen readers unless `Accessible`. A `Passive` overlay lets touches through (`FLAG_NOT_TOUCHABLE`) and never takes the focus: from Android 12 the system passes a touch through an app's `TYPE_APPLICATION_OVERLAY` windows only while their combined opacity is at most `InputManager.getMaximumObscuringOpacityForTouch()` (0.8 by default), so the runtime draws a passive overlay at that window alpha or less rather than offer a pass-through that drops touches. The overlay's touch region is its window, and the runtime sets no flag that lets it obscure security-sensitive system prompts.

The default window level is the platform overlay level. The API does not offer arbitrary z-order, click-through regions within a touchable overlay or hidden system-window types. Dragging and resizing are explicit gestures in the overlay tree, and the runtime applies them through `WindowManager.updateViewLayout` rather than stepping coordinates from pointer events.

## Security and data

Overlay content is a high-risk capability because it can obscure other apps or capture touches, so it is a capability of its own, `AccessOverlay`, which a host's `Access` may withhold and which TSX packages cannot declare. The runtime records no screenshots, keystrokes or other apps' content. An overlay receives only its own touch events and the app's declared host messages; what its content asks of an activity, such as a picker, answers as with no activity, and its alerts show over other apps.

## Other targets and tests

- iOS has no window over other apps: `OverlayPermission` answers unavailable and `OpenOverlay` returns nil, whose methods do nothing, as `OpenPanel` does on phones. Picture in Picture (#54) and Live Activities keep their own APIs.
- macOS, Windows, Linux and the web answer unavailable too. A desktop's floating window is `desktop.OpenPanel`, which macOS shows at the status level on every space.
- `nsgotest` keeps the overlay permission, `App.OverlayPermission`, which a test sets and takes away with `RevokeOverlayPermission`, as it keeps notifications', and models the opened overlay as `App.Overlay`, a surface with its frame kept within the app's, shown, hidden and closed with its end, so an app's tests assert that it opened, moved or was refused without the permission.

## Verification

The implementation is complete only when these checks exist:

- headless core tests cover permission state, owner disposal, surface attach/detach, frame clamping and lifecycle events;
- `nsgotest` tests open, move, hide and close an overlay, and find it refused without the permission and closed after a revocation;
- Android runtime tests use a fake `WindowManager` and service to cover permission revocation, activity recreation, process death, duplicate close, binder-thread isolation and the service started only after the window shows;
- API-level tests cover the special-access settings flow and foreground-service restrictions, a background start targeting API 35 among them, without relying on a real user's overlay permission; and
- a device test checks rotation, cutouts, touch routing, a passive overlay's pass-through, accessibility and removal from system settings, with no unrelated app content captured.

## Decisions

The user confirmed these on 2026-11-15, taking the answer the design proposes for each:

1. The first release has one overlay surface per app: `OpenOverlay` returns nil while one is open.
2. The service is a `specialUse` foreground service with an ongoing, low-importance notification titled with the app's name, which runs without the notification permission on Android 13 and later; the design proposed no type, and this is the most conservative one that fits an overlay, which no other type describes.
3. Process death does not restore the overlay: the app opens it again.
4. System overlay is a separate `AccessOverlay` capability, and sandboxed TSX packages cannot use it; the design proposed no answer, and this is the most conservative one.
5. Beyond the default, passive, focusable and screen-reader-visible overlays ship first, as options of their own.
6. iOS and the desktops answer unavailable, pointing nowhere: Picture in Picture and Live Activities keep their own APIs, and desktops keep `desktop.OpenPanel` rather than share this one.
