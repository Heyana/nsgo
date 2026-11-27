---
title: Picture in Picture
description: Play video in iOS, tvOS, macOS, Android and browser Picture in Picture, or show a custom tree on iOS.
group: Platform features
order: 45
---

On iOS, declare background playback before using Picture in Picture; macOS needs no background mode:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

For a video, enable `PictureInPicture`, keep a `core.Ref`, and start or stop it from an app control. Follow the state callback to enable that control when `Possible` becomes true:

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` opts into automatic entry when an iOS app backgrounds; tvOS and macOS ignore this option. `Controls` still shows the native playback controls; the app's PiP button uses the same managed session for a controlled video. `OnPictureInPictureRestore` lets the app reveal its player page when the user returns. Keep the video mounted while PiP is active: deleting its scope stops playback and PiP.

On Android, the packaged activity already declares PiP support. A video enters the activity’s PiP window, with the rest of the app’s interface hidden, and keeps its player and position when it returns. `StopPictureInPicture` expands that same activity. The window’s play/pause controls report `OnPlay` and `OnPause`, with `VideoPauseControls`; `Ref.Seek` uses the same player. Automatic entry requires the explicit setting and a prepared, playing video. A start can still fail because of the system’s current state; handle `Error` even when `Possible` was true.

An embedded Android host must declare `android:supportsPictureInPicture="true"` and handle `orientation|screenSize|screenLayout|smallestScreenSize` configuration changes. Forward the activity’s `onPictureInPictureModeChanged` and `onUserLeaveHint` to the same methods on `NsgoView`; on Android 15+, forward `PictureInPictureUiState.isTransitioningToPip()` to `NsgoView.onPictureInPictureTransition` as well. A host or device without support reports `Supported: false`.

On the web, start from a user press in a browser offering the standard video PiP API. `Supported` also reflects the page's Permissions Policy, and `Possible` requires loaded video metadata. The browser's own PiP controls report the same active state. Stopping, replacing or clearing the source, or removing the video ends its session; a pending start cannot reopen the previous source. Web PiP shows only the video, without custom NSGo content, automatic background entry or `OnPictureInPictureRestore`. Unsupported browsers report `Supported: false`.

On iOS, `mobile.PictureInPicture` takes `Children` with a layout of their own, defaulting to 320 × 180 points. It occupies no space in the app. Its `Ref` has the same start and stop methods, while `OnState`, `OnRestore`, `OnPlay`, `OnPause` and `OnSkip` deliver system actions. Bind `PausedFunc`, `PositionFunc` and `DurationFunc` (or static `Duration`) to the playback state those callbacks update; a zero duration describes live content. A suspended app retains the last rendered frame.

Put a `core.Video` in the custom tree to compose its decoded frames with app-drawn captions or other overlays. Start the session using the outer `Ref`. Share a paused signal with the video, feed `DurationFunc` from `Video.OnLoad` and `PositionFunc` from `Video.OnProgress`, and forward `OnSkip` to the video's `Ref.Seek`. The video's audio plays through its own player.

The custom tree also renders text, views, images and Canvas. WebView, Metal shaders and backdrop effects are unavailable in the captured frames. Animated images and native layer transitions show their model state. Use explicit animated values for motion. App Store review may reject uses unrelated to video.

`Supported: false` distinguishes an unavailable platform from a supported session that is not ready or whose `Error` explains a failure. Video supports iOS and tvOS when AVKit reports the device supports PiP, macOS, Android, and web browsers offering the standard API; custom trees support iOS only. On iOS and tvOS, the shared audio session stays in the playback category while PiP is enabled, including while paused, mixing with other apps' audio when the video's `AudioSession` is `AudioSessionMixWithOthers`.

In headless tests, `nsgotest.PictureInPictureSupport(supported, possible)` sets what the system offers under an iOS, macOS or Android `nsgotest.Environment`, the view's `PiPState`, `PiPStarts` and `PiPStops` record the session, `app.ReportPictureInPicture` reports a state the system would, and `app.PictureInPictureControl` sends the window's `play`, `pause`, `skip` and `restore`. The video's PiP window plays and pauses the video's own player, which `Video.OnPlay` and `Video.OnPause` report, with `VideoPauseControls` for a pause; `app.PlayVideo` and `app.PauseVideo` report them in tests.

Run `nsgo dev -target ios examples/pip` for both paths, or `-target macos` or `-target android` for video PiP. Native Mac tests verify entry, exit and repeated commands with and without native controls, plus replacing an active controlled source. iOS simulator tests verify native entry, continuous custom frame rendering and exit on iPad simulators and the distinct unsupported state on iPhone simulators; on a physical iPhone, manual and automatic video PiP, fullscreen restoration, custom redraws and background frame retention have been verified, while opt-out automatic entry, audio mixing with another app playing and continuous background captioned video remain pending.

For tvOS video, run `nsgo dev -target tvos_simulator examples/pip`; NSGo associates the player with a native Now Playing session. The tvOS 27 simulator loads videos with and without native controls but AVKit reports PiP unsupported there. Actual PiP entry, stop and restoration still require verification on a physical Apple TV.

The web target regression uses a recorded local clip and trusted app button presses to verify the browser's actual PiP session through entry, exit, source replacement and deletion in Chromium. Other browser engines still need acceptance.
