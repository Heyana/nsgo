---
title: Haptics
description: Play preset haptic feedback or a timed sequence of pulses.
group: Platform features
order: 46
---

## Playing one

`mobile.PlayHaptic` plays feedback under the user's finger on iOS and Android, and does nothing elsewhere, so pair it with what the user sees. `HapticSelection` marks a value changing, a picker's, a segment's or a slider's step; `HapticLight` to `HapticRigid` mark elements colliding, by their weight and give; and `HapticSuccess`, `HapticWarning` and `HapticError` mark a task ending so.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Playing a pattern

`cx.Env().CanPlayHapticPattern()` reports whether timed haptics are available. A pattern contains pulses with a start time `At` relative to the pattern. Zero `Duration` makes a transient, and a positive duration sustains the pulse for up to 30 seconds. `Intensity` and `Sharpness` range from 0 to 1. A new pattern replaces the previous one. Pass an empty pattern to stop playback. The pattern stops when the app moves to the background.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS plays the events through Core Haptics on supported hardware. Android follows the timing and uses intensity when the vibrator supports amplitude control. Android browsers with `navigator.vibrate` follow the on and off timings after a user interaction. Android and the web play a transient as a 20 ms pulse, and sharpness is available on iOS. On an unavailable platform, the call has no effect.

In tests, set `HapticPatterns: true` through `nsgotest.Environment` and read `app.HapticPatterns`. The history includes a nil entry when the app cancels playback.

## When

Play it as the gesture happens, not as the answer arrives: a selection as a value passes, an impact as a view lands, an outcome as a task ends. Play one for a change the user caused, and none for what the app did on its own.

## Next

- [Notifications](notifications.md): the notifications and badges that carry the same news.
- [Platform services](services.md): the rest of what the system offers.
- [Accessibility](accessibility.md): the announcement a user who feels nothing hears.
