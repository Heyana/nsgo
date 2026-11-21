// Package audio plays sound: a Sound, a short sound decoded once and played any number of times, overlapping, and a Player of long audio, streamed and decoded as it plays, as NSH's Sound and AudioPlayer. Core does the engine's part, so both belong to the engine and the owner of the scope that made them, and the platform plays them where core.Env.CanPlayAudio says it does; elsewhere they fail with Unavailable.
//
// Both are called on the engine's goroutine, as every scope service is; other goroutines reach them through core.Scope.Post. Their callbacks run on the engine's goroutine too, in a later turn, so they may set signals. A closed sound's or player's callbacks never run.
package audio

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

type (
	// Source is what a sound or a player plays, as an image's source: URI, an "asset:" path in the app's resources, a file path or file: URL, a data: URL or an http(s) URL; or Data, the bytes of a file, such as one embedded with go:embed, named by the format their first bytes say. The platform resolves it as it resolves an image's source and decodes it with its own decoders: ship WAV, AAC (.m4a) or MP3.
	Source = core.AudioSource
	// Sound is a short sound decoded into memory once. Play starts a playback within a frame, as a command in the batch of the turn that calls it, so nothing is opened, fetched or decoded then; playbacks overlap, at most 32 at once, past which the platform stops the oldest. Stop stops every playback of its source, Duration is its decoded length and Close frees it, as the owner that loaded it does as it goes.
	Sound = core.Sound
	// Voice is how a playback of a Sound plays: Volume from 0 to 1 and Rate from 0.5 to 2, with 0 as 1 for both, and Pan from -1 (full left) to 1 (full right), with 0 centered; other values are clamped, and NaN takes the default. Loop repeats this playback until stopped.
	Voice = core.SoundVoice
	// Player plays long audio, as a Video without a picture. It loads nothing until Play or Preload; Pause, Seek (clamped to the duration), SetVolume, SetRate and SetLoop are commands in the batch, and a seek while paused or before the duration is known applies once the platform can seek, Play starting from there. Position, Duration (0 until loaded) and State are what core kept from the platform's events, so reading them asks no platform. GTK's Linux player plays at 1×. Close stops it and releases the platform's player, as the owner that made it does as it goes, so a window or an embedded view that closes stops its audio.
	Player = core.AudioPlayer
	// PlayerOptions are a Player's callbacks, progress interval, audio session and the position it returns to when looping.
	PlayerOptions = core.AudioPlayerOptions
	// PlayerState is where a player is: PlayerIdle to PlayerFailed.
	PlayerState = core.AudioPlayerState
	// Error is why a sound or a player failed, named as NSH's codes, which follow the web's MediaError: NotAllowed to Failed. A failure that says more wraps one with the platform's words, which errors.Is and errors.As find. A player's arrives through OnError and Play's callback.
	Error = core.AudioError
)

// The errors a sound or a player fails with.
const (
	// NotAllowed: refused by a capability, core.AccessAudio or, for an http(s) source, core.AccessNetwork; or by the web's autoplay policy before a user gesture.
	NotAllowed = core.AudioNotAllowed
	// Network: the source could not be fetched.
	Network = core.AudioNetwork
	// Decode: a format the platform cannot read, which is an error, never silence.
	Decode = core.AudioDecode
	// NotSupported: a source the platform does not play.
	NotSupported = core.AudioNotSupported
	// QuotaExceeded: past one of NSH's limits, which hold per engine: a sound of more than 8 MiB encoded or 5 seconds decoded (longer audio is a player's), a 65th sound, or sounds of more than 16 MiB decoded.
	QuotaExceeded = core.AudioQuotaExceeded
	// Unavailable: the platform plays no audio (core.Env.CanPlayAudio).
	Unavailable = core.AudioUnavailable
	// Failed: the platform failed otherwise, such as a player whose output went away. An output that fails is silence, not an error, for sounds.
	Failed = core.AudioFailed
)

// The states of a player.
const (
	// PlayerIdle: nothing asked of it yet.
	PlayerIdle = core.AudioPlayerIdle
	// PlayerLoading: loading its source, or waiting for data as it plays.
	PlayerLoading = core.AudioPlayerLoading
	// PlayerPlaying: playing.
	PlayerPlaying = core.AudioPlayerPlaying
	// PlayerPaused: loaded and not playing, paused by the app or by the system, as for a call.
	PlayerPaused = core.AudioPlayerPaused
	// PlayerEnded: played to its end; Play starts it over.
	PlayerEnded = core.AudioPlayerEnded
	// PlayerFailed: it failed, and plays nothing until Play or Preload loads it again.
	PlayerFailed = core.AudioPlayerFailed
)

// Now returns the shared clock used by Voice.StartAt. Headless tests use their manual clock.
func Now(cx *core.Scope) time.Time { return cx.Now() }

// CanSchedule reports whether this platform supports a timed sound start.
func CanSchedule(cx *core.Scope) bool { return cx.Env().CanScheduleAudio() }

// OutputLatency reads the current output route's presentation latency. The callback receives zero and false when the platform has no reading.
func OutputLatency(cx *core.Scope, done func(latency time.Duration, known bool)) {
	cx.AudioOutputLatency(done)
}

// LoadSound has the platform read and decode src into memory once, and calls done with the sound or why not, an Error. A source this engine already holds, the same URI or bytes of the same digest, shares the decoded sound, which goes with the last Sound over it. The sound belongs to cx itself rather than the running owner, so loading with the app's scope from a handler keeps it until cx goes or Close is called.
func LoadSound(cx *core.Scope, src Source, done func(*Sound, error)) { cx.LoadSound(src, done) }

// NewPlayer makes a player of src, which loads nothing until Play or Preload, so making one is free. Play calls back once the platform plays, or with the error. The player belongs to cx itself rather than the running owner, so creating one with the app's scope from a button or row handler keeps it playing after that handler's scope goes, closing only when cx goes or Close is called.
func NewPlayer(cx *core.Scope, src Source, opts PlayerOptions) *Player {
	return cx.NewAudioPlayer(src, opts)
}
