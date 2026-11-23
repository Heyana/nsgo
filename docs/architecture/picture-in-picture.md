# Picture in Picture

Issue [#54](https://github.com/nsgo-dev/nsgo/issues/54) has two paths: `core.Video` uses its native player, and `mobile.PictureInPicture` renders an independent NSGo tree into sample buffers. Video also supports macOS, and the web uses the browser's standard video PiP API. On Apple platforms both use `AVPictureInPictureController` through purego, with no Swift or C adapter. Android video uses its Activity and MediaPlayer. The user confirmed both paths, separate APIs from Android overlays, view-owned lifetimes, a retained background frame, playback callbacks, YAML arrays and programmatic starts with native controls on 2026-11-15.

## Video

Set `Video.PictureInPicture`, follow `OnPictureInPicture`, and call `Ref.StartPictureInPicture` or `StopPictureInPicture` from an app control. `AutoPictureInPicture` explicitly permits automatic entry when an iOS app backgrounds; tvOS has no automatic-inline-entry API and ignores that option. iOS pauses a video that plays as the app enters the background (native-views.md), except one whose PiP plays it on: active, starting, or about to start automatically. `PictureInPictureState` distinguishes unsupported, supported but not possible, active and failed states. Repeated starts while starting or active, and stops while inactive, are idempotent. Starting before AVKit is ready reports an error; it does not wait silently.

A video without controls uses its existing player layer. A controlled video has a second layer in `AVPlayerViewController.contentOverlayView`, between its video and its controls. AVPlayer vends video to one layer at a time, so a layer behind the native controller would disappear behind its empty video surface. A flexible host follows the overlay into full screen. On macOS the same source layer lives in `AVPlayerView.contentOverlayView` in an AppKit host; an uncontrolled video uses its own player layer. The iOS controller's independent PiP button is disabled; the app's control and automatic policy use the one owned session, whose public stop operation NSGo can call.

On tvOS, the video owns an `MPNowPlayingSession` containing its `AVPlayer`, automatically publishes its playback metadata and asks the session to become active when preparing or starting PiP. Both controlled and layer-only videos use the same managed PiP path. The Now Playing session stays with the player generation until PiP has stopped. Custom sample-buffer sessions remain unsupported on tvOS. NSH has no corresponding PiP subsystem; this follows Apple's [custom-player contract](https://developer.apple.com/documentation/avkit/adopting-picture-in-picture-in-a-custom-player).

The PiP window's play, pause and skip act on the `AVPlayer` directly, as the inline controls do: the backend hears the player's rate change, keeps what the window did until the app's `Paused` changes, so an unrelated change such as `Muted` leaves it alone, and reports it through `Video.OnPause`, with `VideoPauseControls`, and `Video.OnPlay` ([videos](native-views.md#videos)). Player time remains in AVPlayer; `Ref.Seek` and the window's skip controls use that player.

## Web video

`Video.PictureInPicture` uses the [standard](https://www.w3.org/TR/picture-in-picture/) `HTMLVideoElement.requestPictureInPicture` and the owning document's exit operation. Browser API availability and Permissions Policy determine `Supported`; loaded video metadata and a video track determine `Possible`. Start from a user press because browsers require transient user activation. Synchronous DOM exceptions and rejected requests reach `Error`. Native enter and leave events report the browser's actual session, including entry through its playback controls.

Each video owns only its element's session. Repeated requests are serialized; stopping a pending start, replacing or clearing its source, disabling PiP and deletion invalidate the request. A late successful entry exits instead of reviving stale content. Exit never stops another element's session. Clearing a source resets the media element so old metadata cannot leave `Possible` true.

Web PiP shows the browser's video surface. Custom NSGo content, automatic background entry and interface restoration callbacks remain iOS-only; no Document Picture-in-Picture or browser-specific prefixed API is used. A browser without the standard API reports unsupported.

## Android video

`core.Video` uses the same state, commands and restoration callback as iOS; detached custom content stays unsupported. `VideoPictureInPicture.java` owns the Activity’s one session and moves the selected video’s TextureView into a full-window host, preserving its MediaPlayer and the inline tree. Returning restores that view’s parent and layout; deleting the video stops the session and releases the player. Native mode callbacks report `Active`, and source readiness plus the Activity’s visibility and focus report `Possible`. Refused starts report an error; repeated starts and stops are idempotent.

The packaged Activity declares `supportsPictureInPicture` and handles the size/orientation configuration changes. Automatic entry requires `AutoPictureInPicture` and a prepared, playing video: Android 12+ gets `setAutoEnterEnabled`, Android 15’s transition callback prepares the video presentation early, and older systems enter from `onUserLeaveHint`. The native MediaSession plays, pauses and seeks the same player; control pauses report `VideoPauseControls`. Android has no public exit-PiP method, so `StopPictureInPicture` expands the same Activity through a reorder-to-front intent. The returning Activity reports restoration to the graph.

The pipe keeps old runtimes compatible: the video’s existing flag byte carries the two PiP flags, and a reserved `ItemNativeCommand` carries session commands, which an old runtime safely consumes and ignores. Capability bit `capVideoPiP`/`CAP_VIDEO_PIP` distinguishes a runtime that answers those commands; Go reports unsupported when it is absent. Native state is an `EventNative` with the existing `PictureInPictureEvent` payload.

An embedding host declares the same manifest capability and forwards its Activity’s mode-change and user-leave callbacks to `NsgoView.onPictureInPictureModeChanged` and `onUserLeaveHint`; on Android 15+, it also forwards `isTransitioningToPip()` to `onPictureInPictureTransition`.

## Custom content

`mobile.PictureInPicture` occupies no space where declared. Its children inherit their owner's context but have a detached layout root of `Size`, defaulting to 320 by 180 points. `Ref` starts and stops its session; `OnState`, `OnRestore`, `OnPlay`, `OnPause` and `OnSkip` report the system's actions on the engine goroutine. `PausedFunc`, `PositionFunc` and `Duration` describe the playback delegate's state; a zero duration is live content. Skip intervals are signed.

UIKit mounts a one-point sample display layer as AVKit's inline source anchor, while the content tree stays detached. After a commit or animated frame changes content, one dirty flag schedules rendering on the next display tick. `renderInContext:` draws into a locked BGRA CoreVideo buffer in sRGB, which CoreMedia wraps as a sample buffer with immediate display. A full display layer keeps the dirty flag and drops intermediate changes; no frame queue grows. Its control timebase and an atomic playback snapshot answer AVKit without waiting for the engine. Suspension keeps the last enqueued frame, and resuming renders the latest content.

Views, text, images, Canvas and Video render through this layer path. Image loads invalidate the sample after their pixels arrive, without requiring an app state change. WebView, Metal shaders, backdrop blur and glass do not render into these samples. Native layer transitions and animated images are captured at their model state. Use explicit animated values for changing content. The system window takes play, pause and skip commands.

An iOS video in the detached tree uses its AVPlayer for playback and attaches an AVPlayerItemVideoOutput to its current item. New BGRA frames become CGImages through VideoToolbox. A layer in the original video view holds the latest image, applying the track's orientation and the view's resize mode before the tree renders. This preserves the layout order of the app's overlays. The video's audio stays with AVPlayer. During active background PiP, a single timer polls at up to 60 Hz while the app can run, since UIKit's display link stops. During suspension AVKit keeps the last enqueued frame. Replacing a source or removing the view releases the output and its image, and a completed seek wakes the frame clock.

## Ownership and audio

A session belongs to its view. Deleting the view stops PiP; its controller, delegate, source layer and player stay referenced until AVKit reports that it stopped or failed. Reopening a source makes a new generation without releasing the old active player early. Disabling PiP releases its extra layer without removing an inline video's own layer.

Restoration queues an event on the engine and holds the native completion block. The resulting mount batch reserves the completion, applies all its changes and then acknowledges success, including when it removed the owner. Deletion before an acknowledgement or a five-second timeout answers false; each block is called and released once.

On iOS, the shared audio coordinator (`internal/audiosession`) holds the playback category while PiP is enabled, including while paused, and restores the previous category after its last owner leaves. A video's session mixes with other apps' audio when its `AudioSession` is `AudioSessionMixWithOthers`, as the video does while it plays, and pauses theirs otherwise; whether AVKit starts PiP with the mixing option is checked on a device with the rest. A custom session does not record or request microphone access. Mixing a silent custom session with another app's audio still needs device verification; the current session uses the playback category without mixing.

## Configuration

On iOS, declare background playback in the app or embedding host's Info.plist. macOS needs no background mode and ignores automatic entry, which AVKit exposes only on iOS. `apple.info` now accepts scalar values and arrays, preserving existing strings:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

An iOS session missing `audio` reports a configuration error. Video support on iOS and tvOS follows AVKit's device capability, and macOS video uses the same controller; unavailable systems report `Supported: false`. The web supports video in browsers offering the standard API. Android video uses the Activity's PiP window. Custom content remains iOS-only. The app's existing network access check applies to video URLs. Custom content stays in the app; NSGo uploads and persists no frame. App Store review may reject non-video uses of custom PiP, as issue #54 records.

## Verification

Core and headless tests exercise detached layout, changing content, ownership, support/readiness, duplicate commands, the window's play and pause, background state and restoration. Apple tests call real Objective-C delegate methods, including the `CMTimeRange` return and signed skip argument, invoke native blocks through their ABI, check deferred release, exercise back-pressure, and inspect the pixels of successive CoreVideo/CoreMedia frames. Plist tests cover arrays and escaping on every Apple template. `examples/pip` packages both paths with the required background mode.

Mac native tests open a real player window and enter and leave PiP with and without native controls, including duplicate commands and replacing an active source. Ownership tests retain retiring source views through the native stop callback when the source or controls change. Window-scoped Core Graphics and ScreenCaptureKit captures return a black PiP panel on the Mac runner while the inline view shows AVKit's PiP placeholder; changing-frame visual acceptance and the native return button remain in the roadmap.

On the tvOS 27 simulator, the bundled-video tests load both controlled and layer-only videos and report AVKit's `Supported: false` state. A native app-host test verifies the actual Now Playing session's player association and metadata policy, and that tvOS ignores automatic inline entry. These checks do not establish PiP entry.

On the iOS simulator `examples/pip` launches with both paths, which report the unsupported state AVKit gives there (`isPictureInPictureSupported` is false). Physical Apple TV verification remains for entering, stopping and restoring video PiP; Apple's tvOS [sample requires a physical device](https://developer.apple.com/documentation/avkit/adopting-picture-in-picture-playback-in-tvos). Native iPhone verification remains for entry and restoration, fullscreen controlled video, and continuous custom video with changing overlays in the background.

Web target tests record their own WebM clip, press real app controls through trusted browser input, and check the actual PiP owner against core callbacks through repeated entry/exit, browser exit, source clearing/reloading and view deletion. Controlled promises separately check stale completion, errors, unsupported browsers and ownership. These pass in Chromium; other browser engines still need acceptance.
