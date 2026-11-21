# Audio playback

NSGo ports NSH 0.5.0's audio design (`nsh/docs/dev/audio-design.md`) for issue #36. The `audio` package uses `Sound` for short sounds and `Player` for long audio. macOS, iOS, Windows, Linux, Android and the web have backends, and `nsgotest` provides a fake. The [roadmap](../roadmap.md#services-and-components-since-040) tracks the remaining backends. Read both designs before extending audio.

## The API

The API's home is the `audio` package, which imports `core` alone, as `mobile` does: `core` implements its engine part and names `audio` in that part's comments. Its functions take the `*core.Scope` asking, so a sound or a player belongs to that scope's engine and directly to the `cx` passed (rather than the running owner, matching `OpenWindow` and `NewHolder`), so starting playback with the app's or a page's scope from a button or list-row handler keeps it alive after that handler's scope goes.

```go
// A short sound: decoded once, played any number of times, overlapping.
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // fire and forget
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps the one above, panned left
})

// Long audio: streamed and decoded as it plays.
player := audio.NewPlayer(cx, audio.Source{URI: "https://example.com/voice.m4a"}, audio.PlayerOptions{
	OnProgress: func(position, duration time.Duration) { elapsed.Set(position) },
	OnEnd:      func() {},
	OnError:    func(err error) {},
})
player.Play(func(err error) {}) // runs once it plays, or with why it cannot
player.Seek(12500 * time.Millisecond)
player.SetRate(1.5)
player.Close()
```

A `Source` is what an image's is: an `asset:` path in the app's resources, a file path or `file:` URL, a `data:` URL, an `http(s)` URL, or `Data`, bytes such as a file embedded with `go:embed`, named by the format their first bytes say. Backends resolve it as they resolve images, so an `asset:` sound is found in a macOS bundle's resources, beside the executable on Windows and Linux, inside the APK on Android and beside the page on the web.

`Sound` is NSH's `Sound`:

- `LoadSound` has the platform read and decode the source into memory once, and calls back with the sound or why not. Loading a source this engine already holds in the same surface (the same URI, or bytes of the same digest) shares the decoded buffer, which goes with the last `Sound` over it. Sharing stops at a surface, a window or an embedded view, because a surface's audio goes as it closes while the commands of a sound go to the surface that loaded it.
- `Play(Voice)` starts a playback within a frame: it is a command in the batch of the turn that calls it, so nothing is opened, fetched or decoded then. Playbacks overlap. `Volume` runs from 0 to 1 and `Rate` from 0.5 to 2, with 0 as 1 for both, as `Video`'s; `Pan` runs from -1 (full left) to 1 (full right), with 0 centered. Values outside those ranges are clamped, where NSH throws a `RangeError`, and NaN takes the default. A playback that cannot be heard, the device muted or a voice dropped past the limit, is no error.
- `Voice.Loop` repeats one playback's decoded PCM buffer until it stops. Other voices of the same sound can play once or loop independently. A looping voice continues to count against the playback limit.
- `Play` returns an `audio.Playback`. Its `SetVolume` takes zero as silence, `SetRate` changes speed in place, `Stop` stops it alone, and tracked `Ended` follows completion, stop, eviction and failure. `Voice.Muted` starts it at zero volume. macOS, iOS, Windows, Linux and the web support playback controls. A backend without them still plays fire-and-forget and returns an ended handle.
- `Stop` stops every playback of the sound, `Duration` is the decoded length, and `Close` frees the buffer. The owner that loaded the sound closes it as it goes, and the engine as it closes.

`Player` is NSH's `AudioPlayer`, `Video` without a picture:

- `NewPlayer` loads nothing until `Play` or `Preload`, so making one is free. `Play` loads if needed and calls back once the platform plays, or with the error.
- `Pause`, `Seek` (clamped to the duration), `SetVolume`, `SetRate` and `SetLoop` are commands in the batch; `SetVolume` and `SetRate` take `Voice`'s ranges, 0 as 1. `SetMuted` sends volume zero while retaining the requested volume, so unmuting restores it at the current position. A seek while paused or before the duration is known applies when the platform can seek, and `Play` starts from there.
- `PlayerOptions.LoopStart` is the position to resume from when a player with `SetLoop(true)` reaches its end. The first pass plays from its normal start. Zero repeats the whole track. An offset outside the track uses zero.
- `Position`, `Duration` (0 until loaded) and `State` (idle, loading, playing, paused, ended or failed) are kept by `core` from the platform's events, so reading them asks no platform. The platform counts the seeks it applied and the loops it started over in its progress reports, so `core` drops a report from before the last seek and keeps the position from going back otherwise.
- `PlayerOptions` has `OnLoad` with the duration, `OnProgress` with a position that goes back only with a seek or a loop starting over, and the duration, every `ProgressInterval` while playing (250 milliseconds when 0, at least 16, as `Video`'s) and once more as it pauses or ends; `OnBuffer`, `OnEnd` once per play-through (never while looping) and `OnError`. It also has `AudioSession` ([sessions](#sessions)).
- `Close` stops it and releases the platform's player. A player is not tied to a view, but it is to the owner that made it and to the engine: they close it as they go, so a window or an embedded view (`nsgo.NewView`) that closes stops its audio.

Both are called on the engine's goroutine, as every scope service is; other goroutines reach them through `Scope.Post`. Their callbacks run on the engine's goroutine too, in a later turn, as an image's or a speech session's do, so they may set signals. A closed sound's or player's callbacks never run: `core` numbers sounds and players from one counter of the process, so a late event finds no one.

`Env.CanPlayAudio` says the platform plays audio. Elsewhere, `LoadSound` and `Play` fail at once with `Unavailable`, without asking the platform.

## Timing

Set `audio.Voice.StartAt` to a future `time.Time` from `audio.Now(cx)` to schedule a loaded sound. All voices share the engine clock. A zero or past time plays at the next opportunity. On macOS and iOS the backend translates the remaining interval to an `AVAudioTime` on the hardware host clock, schedules the decoded buffer and starts its player node at that time. On Linux the voice's pad offset in the GStreamer mixer moves its first sample to the pipeline running time the interval reaches, at least one mixer block ahead. On the web `AudioBufferSourceNode.start(when)` takes the output context's time, which the backend anchors to the Go clock so that voices scheduled in different turns keep their spacing. A pending voice counts toward the playback limit and keeps the output ready until it plays or stops.

`audio.CanSchedule(cx)` reports support for a timed start. `audio.OutputLatency(cx, done)` asks the current audio route for its presentation latency. The callback returns a duration and whether the route reported it. Apple reads the I/O node's `presentationLatency`, with the audio session's `outputLatency` as an iOS fallback. A query can create the output graph and leaves it stopped until a sound starts. Linux answers with the mixer pipeline's live latency query once its output plays, opening the output as a sound does. The web reports `AudioContext.baseLatency` plus `outputLatency` for a running output, and an unknown latency before a gesture opens one or where the browser has no `outputLatency`.

The `nsgotest` audio fake uses its manual clock and records `StartAt` in each sound's `Plays`. Set `App.AudioOutputLatency` to model a known delay.

## Errors and limits

Failures are an `audio.Error`, named as NSH's codes, which follow the web's `MediaError`: `NotAllowed` (refused by a capability, or the web's autoplay policy before a user gesture), `Network`, `Decode` (a format the platform cannot read: an error, never silence), `NotSupported`, `QuotaExceeded`, `Unavailable` and `Failed`. A player's error arrives through `OnError` and `Play`'s callback, and it is failed: it plays nothing until `Play` or `Preload` loads it again.

An output that fails makes silence, not an error: a device unplugged or changed, an engine reconfigured or a system that refuses the output. The platform reopens on the default device when it can and keeps the sounds loaded; a player that loses its output reports `Failed` once and stops.

The defaults follow NSH. An engine holds up to 64 sources, including loads in progress, and 16 MiB of decoded sound across its surfaces. Each sound takes up to 8 MiB encoded and 5 seconds decoded. A load beyond any of these returns `QuotaExceeded`. A platform output admits 32 overlapping playbacks by default and replaces the oldest eligible voice at the cap.

Trusted Go code calls `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` before loading to raise those limits for its engine. `MaxEncodedBytes`, `MaxDecodedBytes` and `MaxPlaybacks` can also be set. A zero field uses its default. Negative values return `NotSupported`. Already loaded sounds count toward the new budgets and a pending load keeps the duration cap sent to its decoder. The platform enforces native capacity, including AVAudioPCMBuffer's 32-bit frame count on Apple. `MaxPlaybacks` holds per playback output: each surface's on Apple and the web, each window and the windows it opened on Linux. A restricted scope's playbacks stay within 32 of that output's voices whatever the engine's limit. Every backend also refuses a sound whose decoded buffer alone exceeds `MaxDecodedBytes`: Apple before it allocates one, Linux as it decodes and the web once the browser decoded it.

An owner held to `core.Access` keeps the default limits within the engine's budget and cannot set them. Its sources and decoded bytes are counted separately so sharing a URI with the Go app cannot give it a buffer that exceeds its limits. It needs `AccessAudio` to load or play audio, and `AccessNetwork` for an HTTP(S) source.

## Sessions

The engines' audio objects are per engine, but iOS has one `AVAudioSession` per process, which videos, speech, `microphone` and now audio all set. A shared package, `internal/audiosession`, holds it for all of them: it imports nothing of the module, so `microphone` and the Apple backend both import it (a rule of its own in `layering_test.go`, with `microphone`'s widened to it and the package added to `platformShared`), it guards its holders with a mutex, so the main thread, the audio workers and `microphone`'s caller may all call it, and it does nothing outside iOS. It replaced `apple/audiosession_ios.go`'s list and `speech_ios.go`'s and `microphone_ios.go`'s own copies, so a video that starts playing with a session of its own no longer takes it from a recording, which the roadmap's *Recording beside video on iOS* row checks on a device.

Each holder asks for a level, and the highest held sets the session:

1. A recording, `microphone.Capture` or `StartSpeech`: `PlayAndRecord` with `AllowBluetooth` and `DefaultToSpeaker`, in speech's `Measurement` mode while a speech session listens, else `Default`. Sounds and players play through it, so a cue never stops a recording; the Ring/Silent switch does not silence this category.
2. Playback, a video or player with `AudioSessionPlayback`: `Playback` in the `SpokenAudio` mode, pausing other apps' audio, as videos have it now.
3. Mixing, `AudioSessionMixWithOthers`: the same with `MixWithOthers`.
4. Ambient, every sound and a player with `AudioSessionDefault`: `Ambient`, as NSH sets it, which mixes with other apps' audio and which the Ring/Silent switch and the screen lock silence.
5. Kept, a video with `AudioSessionDefault`: the category from before the first holder, the app's own, kept active, since deactivating the session under a playing `AVPlayer` stops it, as a playing video switching to the default session did.
6. None held: the category from before the first holder, then deactivation telling other apps they may play.

A video's `AudioSessionDefault` leaves the category to the app, as it did when it held nothing. Audio's is ambient instead because leaving the session alone would play under `SoloAmbient`, iOS's default category, which stops other apps' audio.

Deactivating waits for the system, and longer while a player's output still runs down, as an `AVPlayer`'s does for a while after its pause: on an iPhone XR, deactivating at the last release blocked the main thread for about 600 ms at every pause. So the last release only arranges the deactivation, which runs a second later on a goroutine of its own; a hold before it keeps the session active, with no call at all when its level is the session's, as for a play right after a pause, and a hold while it runs waits for it and then activates the session again.

A holder holds only while it plays, so a paused or ended player and a sound with no playback hold nothing. On Android, sounds ask for no audio focus, and a player takes a video's: `AUDIOFOCUS_GAIN_TRANSIENT` while it plays with `AudioSessionPlayback`, pausing for another app's focus. Desktops and the web play through the system mixer, under the app's own volume there, and pause no other application. Mobile players pause when the app leaves the foreground, as NSH's do; background playback and lock-screen controls are not part of this design.

## The platforms

Each engine starts its audio objects with its first load or player, so an app that makes no sound never opens the output, and releases them as it closes. Decoding runs off the main thread and off the engine's goroutine.

| Target | `Sound` | `Player` |
| --- | --- | --- |
| macOS, iOS | One `AVAudioEngine` per surface, with a buffer scheduled on an `AVAudioPlayerNode` for each voice (`setPan:` on the voice mixer for `Voice.Pan`) | `AVQueuePlayer` with no layer, with `AVPlayerLooper` for a finite track on macOS 14 and iOS 17 or later |
| Windows | One `XAudio2` engine per engine, a source voice per playback over its mastering voice (output channel matrix via `driver.AudioPanGains` for `Voice.Pan`); Media Foundation's source reader decodes at load, on one decoding thread for the process | Media Foundation's media engine, as `Video`'s, with no video output |
| Linux | GStreamer through purego: decoded once (`decodebin` into 48 kHz stereo floats, cut at the cap), played by one live pipeline per engine with an `appsrc` and `audiopanorama` (`panorama` for `Voice.Pan`) per playback into an `audiomixer` | `GtkMediaFile`, as `Video`'s; GTK's streams have no rate, so `SetRate` does nothing there |
| Android | One `SoundPool` per engine with 32 streams (left/right volume via `driver.AudioPanGains` for `Voice.Pan`), its duration read with `MediaExtractor` | `MediaPlayer` with its own media looper and transient audio focus for playback sessions |
| Web | One `AudioContext` per engine for output after a trusted user gesture. An offline context decodes with `decodeAudioData`, and an `AudioBufferSourceNode` (`loop` for `Voice.Loop`) and `StereoPannerNode` (`pan.value` for `Voice.Pan`) play each voice | An `HTMLAudioElement`, as `Video`'s `<video>`, with the element's `loop` for a whole-track loop |
| Mini programs, tvOS, watchOS | Not in the first stages: `Env.CanPlayAudio` is false and calls fail with `Unavailable` until each gets a roadmap row of its own | The same |

On Apple platforms, a voice keeps its mixer for its lifetime because detaching mixers from a running `AVAudioEngine` can break playback. Reusing a voice for another format replaces the player and rate effect upstream of the mixer. The voice keeps the same main mixer input bus, where the new source format is converted to match the output device.

On Linux, GStreamer decodes each sound on a worker into 48 kHz stereo float PCM. The decoder stops pulling samples at the duration cap. The first play opens a live `audiomixer` pipeline using the default system audio sink. Each voice sends its decoded samples through `appsrc`, applies volume, resamples when the requested rate changes the pitch and length, and pans off-center voices with `audiopanorama`'s balance method. A looping voice keeps its `appsrc` open and queues sample-contiguous copies of the sound half a second ahead on each audio tick, so it repeats without a gap until stopped. Each voice's mixer pad applies volume to the current output block. A live rate change reconnects the remaining PCM at the new rate, preserving position and eviction order; a looping voice resumes its copies from the same point in the sound. Completion includes the mixer's reported output latency, or a 500 ms drain allowance when it is unknown. The mixer closes when idle and reopens on the next play. `GtkMediaFile` streams files and URLs through GTK's GStreamer backend, which plays at 1×. Byte sources and data URLs are staged in temporary files on a worker. The player retains its file for retries and deletes it on close. A whole-track loop uses the stream's own loop; a loop with a `LoopStart` seeks back to it when the stream ends, with the short gap of a seek.

An Ubuntu 24.04 arm64 container running GStreamer 1.24 detected all 13 plays of a loaded 100 ms tone through a PulseAudio null sink monitor. From the Go `Play` call to the first monitor buffer containing the tone, the first play took 216 ms and the next 12 took 175–196 ms, with a 183 ms median. Plays were about 700 ms apart after detection. The measurement covers the virtual audio output and capture path.

On macOS and iOS (`internal/platform/apple/audio.go`, as NSH's `audio.rs`), a surface creates its audio with the first `ItemAudio` and closes it with the surface. `AVAudioFile` decodes a sound's temporary file in a goroutine with an autorelease pool. It reports a sound beyond the duration cap before decoding. Each playback takes one voice, up to the output's configured limit. Each voice is an `AVAudioPlayerNode` connected through `AVAudioUnitVarispeed` for rate and a mixer of its own for volume and stereo pan (`setPan:`), since an effect between a player and the main mixer keeps the player's own mixing properties from it. A voice keeps one format. Each playback has its own ID. Apple applies live settings to its varispeed and mixer, and reports AVAudioPlayerNode's played-back completion on the main queue. A queued completion only affects the playback that scheduled it. A stop, eviction or engine interruption retires the affected handles. The engine starts on playback and pauses two seconds after the last voice finishes.

A player writes source bytes and data URLs to a temporary file and removes it on close. It polls the item every 100 ms for up to a minute while loading. In a native loop, the queue advances from the original item to replicas prepared by `AVPlayerLooper` from `LoopStart` to the end. Progress counts a lap at each transition. Older Apple versions and sources without a finite duration seek to the loop start at the end. An ordinary end comes from `AVPlayerItemDidPlayToEndTimeNotification`.

The time observer and `AVPlayerRateDidChangeNotification` report a system pause. On iOS, an interruption or the app entering the background pauses the players and releases their audio session. The app resumes them with `Play`. A failed player reopens on play or preload and resumes at the last reported position, or starts from the beginning after an end. Unsupported paths and unreadable local sources return `NotSupported`. HTTP(S) failures return `Network`. tvOS reports `Env.CanPlayAudio` false.

The audio fake records each voice's current settings and `Loop` and each player's `LoopStart`. Its clock advances a voice's remaining duration after its scheduled start, which a looping voice never runs out of, and a player through the first pass, wrapping the remainder into the looping range.

On the web, short sounds fetch through the browser and cap the encoded body before decoding it. An offline context can decode before the first gesture without opening the output. Web Audio decodes the full file as one operation and the backend drops buffers longer than five seconds. The live context starts only once the engine has both a sound and a trusted gesture. A cue played before the gesture is silent. The streaming player uses an `HTMLAudioElement`, with blob URLs for bytes, and reports a browser's autoplay refusal as `NotAllowed`. A later play can retry after a gesture. A loop of the whole track uses the element's seamless `loop`, which always restarts at 0; a loop with a `LoopStart` plays to the end, then on `ended` plays again from the loop start, with the short gap of a seek.

The web updates each voice's GainNode and AudioBufferSourceNode playback rate, and reports its ended event, explicit stop, eviction or a refused output resume.

Windows uses a single Media Foundation decoding thread for sounds. It passes interleaved float PCM to XAudio2, keeping the first two channels of a multichannel source. A surface opens the default mastering voice on its first `Play` and reuses compatible source voices. `Voice.Pan` scales the front left and front right rows of a voice's default output matrix by `driver.AudioPanGains`; a mono output takes no pan. It destroys the oldest voice when all 32 are still playing. Players open an audio-only Media Foundation engine on preload or play, streaming URLs directly and wrapping bytes in a native memory stream. Each end starts the next lap when looping, so the engine can report every lap to `core`. Playback handles update the source voice’s volume and frequency ratio in place. A 10 ms timer reads queued-buffer state on the window thread independently of display frames and reports each tracked completion once; stop, eviction and output failure report it too. The player receives zero effective volume while muted and restores the configured volume when unmuted. Native CI decoded WAV, AAC and MP3, but skipped output tests because its runner had no default endpoint. A native test can time the first samples processed by XAudio2 on a warm output. Measuring when those samples become audible requires a Windows playback device. A looping voice submits its buffer with `XAUDIO2_LOOP_INFINITE`, so it plays until stopped, and a looping player seeks to its `LoopStart` when Media Foundation reports the end, with the short gap of a seek.

In five headless Chrome 158 runs on macOS arm64, a preloaded WAV cue requested on the first trusted click produced its first observed nonzero Web Audio sample in 9–46 ms, with a median of 10 ms (2026-11-21). The test reads an analyser in the browser's audio graph. Speaker output adds device latency.

On Android, the Java runtime reads each sound into a seekable file on a worker, stopping at the encoded-size limit. `MediaExtractor` supplies its duration, sample rate and channel count. The runtime checks the duration and estimated decoded size before passing the source to `SoundPool`, which decodes into 16-bit PCM. Android reports `QuotaExceeded` for sounds above the pool's 1 MiB per-sample limit. The first sound load creates the pool, shared by the engine's views. Each `MediaPlayer` streams its source on the media looper. Byte sources and data URLs arrive as staged files. A player reports the newest native seek completion and counts each loop restart. It pauses when its view is hidden or a playback session loses audio focus.

On a Pixel 3 XL running Android 12, the first play of a loaded 100 ms tone took 231 ms from the Go `Play` call to acoustic onset. Twelve plays about 700 ms apart afterward took 48–59 ms, with a 56 ms median. The device's built-in microphone detected the tone in 5 ms windows. The probe used the microphone's monotonic sample timestamp on the same clock as the Go call, so the interval includes the speaker-to-microphone path.

The Apple backends read sounds through `imageload`. All backends use native decoders. WAV, AAC (`.m4a`) and MP3 are the portable formats. FLAC, Opus and Vorbis decode where the platform supports them and otherwise return `Decode`.

## NSH's design and this one

- **The package.** NSH's `Sound` and `AudioPlayer` are modules of its core runtime. Here the API's home is `audio`, as issue #36 asked, so `core`'s API does not grow, while `core` does the engine's part as it does `mobile`'s. A package of its own beside `microphone`, importing nothing, cannot: it would reach no backend, `asset:` resource, iOS session or Android runtime, and its callbacks would run off the engine.
- **`Player`, not `AudioPlayer`.** Go names drop their package's (`dock.Area`); TSX keeps `AudioPlayer`.
- **Go's shapes.** Callbacks where NSH returns promises and fires events, `Close` where it has `dispose`, and clamping where it throws a `RangeError`, since a fire-and-forget call has no error to return.
- **No `PlayFile` or `PlayBytes`.** Issue #36 proposed a call that opens and decodes a file each time; NSH rules that out, since it starts too late to be feedback and hides a read in a tap handler. A cue is a sound loaded once and played many times.
- **A player's `AudioSession`.** NSH plays everything ambient and reserves a `session` option for later. NSGo's videos have it already, and an audiobook or a voice message needs to play in silent mode, so a player takes it, ambient by default as NSH's.
- **The shared session.** NSH sets `Ambient` once. NSGo also holds the session for videos, speech and `microphone`, so the levels above settle who wins.
- **Reading.** NSH reads a sound's bytes in its core through its image loader, so its limits are checked there. NSGo's image loader lives in the backends, so they read and check the encoded size, and `core` counts the rest.

## Delivery

Each stage is one reviewable change, in NSH's order:

1. The `audio` package with its rule in `layering_test.go`, `core`'s part, the protocol's requests and events, `Env.CanPlayAudio`, `AccessAudio` and the `audio` capability, and `nsgotest`'s fake: it records the sounds loaded and played and each player's commands, answers a load with a duration the test sets, advances progress only on the test's clock, and can fail a load with `Decode` and a play with `NotAllowed`, so tests assert on what an action played without sleeping. Built (2026-11-15).
2. `internal/audiosession`, with videos, speech and `microphone` moved onto it. Built, not yet run on a device (2026-11-15).
3. macOS, iOS, Linux, the web, Windows and Android, each with format and error tests. macOS, iOS, Windows, Linux, the web and Android are implemented. Windows' audible latency is unmeasured. The Linux native tests pass with GTK and a PulseAudio virtual output, and the Android ones on a Pixel 3 XL. iOS device verification remains open.
4. The gallery: a sound for each kind of moment and a player demo of both levels.

Issue #36 closes, and the roadmap's row with it, when a WAV and an MP3 play as sounds and through a player on macOS, iOS, Windows, Linux, Android and the web, and a FLAC either plays or fails with `Decode`; a sound plays overlapping and starts within a frame; `OnProgress` never goes back but with a seek or a loop, and a seek while paused shows the new position on `Play`; `Close` on a playing player stops it, and ten open and close cycles leave no handle behind (on macOS, no audio device stays open); on an iPhone, a sound during a playing video neither pauses nor stops it, a recording and `StartSpeech` keep working through a sound, and the Ring/Silent switch silences sounds; and the gallery's demos are in.
