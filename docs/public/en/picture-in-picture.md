---
title: Picture in Picture
description: Play video in iOS and tvOS Picture in Picture, or show a custom tree on iOS.
group: Platform features
order: 45
---

Declare background playback before using Picture in Picture:

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

`AutoPictureInPicture` opts into automatic entry when an iOS app backgrounds; tvOS ignores this option. `Controls` still shows the native playback controls; the app's PiP button uses the same managed session for a controlled video. `OnPictureInPictureRestore` lets the app reveal its player page when the user returns. Keep the video mounted while PiP is active: deleting its scope stops playback and PiP.

On iOS, `mobile.PictureInPicture` takes `Children` with a layout of their own, defaulting to 320 × 180 points. It occupies no space in the app. Its `Ref` has the same start and stop methods, while `OnState`, `OnRestore`, `OnPlay`, `OnPause` and `OnSkip` deliver system actions. Bind `PausedFunc` and `PositionFunc` to the playback state those callbacks update; `Duration: 0` describes live content. A suspended app retains the last rendered frame.

Put a `core.Video` in the custom tree to compose its decoded frames with app-drawn captions or other overlays. Start the session using the outer `Ref`. Share a paused signal with the video, feed the custom session's `PositionFunc` from `Video.OnProgress`, and forward `OnSkip` to the video's `Ref.Seek`. The video's audio plays through its own player.

The custom tree also renders text, views, images and Canvas. WebView, Metal shaders and backdrop effects are unavailable in the captured frames. Animated images and native layer transitions show their model state. Use explicit animated values for motion. App Store review may reject uses unrelated to video.

`Supported: false` distinguishes an unavailable platform from a supported session that is not ready or whose `Error` explains a failure. Video supports iOS and tvOS when AVKit reports the device supports PiP; custom trees support iOS only. macOS, Android and web PiP remain unavailable. The shared audio session stays in the playback category while PiP is enabled, including while paused, mixing with other apps' audio when the video's `AudioSession` is `AudioSessionMixWithOthers`.

In headless tests, `nsgotest.PictureInPictureSupport(supported, possible)` sets what the system offers under an iOS `nsgotest.Environment`, the view's `PiPState`, `PiPStarts` and `PiPStops` record the session, `app.ReportPictureInPicture` reports a state the system would, and `app.PictureInPictureControl` sends the window's `play`, `pause`, `skip` and `restore`. The video's PiP window plays and pauses the video's own player, which `Video.OnPlay` and `Video.OnPause` report, with `VideoPauseControls` for a pause; `app.PlayVideo` and `app.PauseVideo` report them in tests.

Run `go tool nsgo dev -target ios examples/pip` for both paths. iPhone acceptance, including controlled-video fullscreen and automatic entry, remains pending; the implementation currently has native delegate/frame tests and headless app coverage.

For tvOS video, run `go tool nsgo dev -target tvos_simulator examples/pip`; NSGo associates the player with a native Now Playing session. The tvOS 27 simulator loads videos with and without native controls but AVKit reports PiP unsupported there. Actual PiP entry, stop and restoration still require verification on a physical Apple TV.
