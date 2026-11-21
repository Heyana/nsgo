package driver

import (
	"math"
	"time"
)

// The limits a sound is held to, NSH's: its encoded bytes, which the platform's reader checks before decoding; its decoded length, which the platform decodes no more of; the sounds one engine holds and their decoded bytes, which the core counts; and the playbacks at once, past which the platform stops the oldest.
const (
	MaxSoundBytes    = 8 << 20
	MaxSoundDuration = 5 * time.Second
	MaxSounds        = 64
	MaxDecodedBytes  = 16 << 20
	MaxSoundVoices   = 32
)

// AudioCommand is what an ItemAudio asks of a sound or a player.
type AudioCommand uint8

const (
	// AudioLoad has the platform read sound Count's Source, failing with AudioQuotaExceeded past MaxBytes, and decode at most MaxDuration of it into memory, reporting AudioLoaded or AudioFailure.
	AudioLoad AudioCommand = iota
	// AudioPlay starts a playback of sound Count at Volume, Rate and Pan over those playing. A nonzero Playback gets AudioPlaybackEnded when it stops.
	AudioPlay
	// AudioStop stops every playback of sound Count.
	AudioStop
	// AudioUnload frees sound Count, its playbacks stopped. Nothing is reported of it after this.
	AudioUnload
	// AudioOpen makes player Count for Source, with Volume, Rate, Loop, Session and ProgressInterval, starting at Position. It loads nothing: another command follows it in the same batch.
	AudioOpen
	// AudioPreload has player Count load without playing, reporting AudioOpened once it knows its duration.
	AudioPreload
	// AudioResume plays player Count from where it is, loading it first if it has not or failed, and reports AudioPlaying. After AudioEnded it plays from the start, unless an AudioSeek came since.
	AudioResume
	// AudioPause pauses player Count, reporting AudioPaused.
	AudioPause
	// AudioSeek moves player Count to Position, clamped to its duration, once it can seek, and reports AudioProgress from there.
	AudioSeek
	// AudioSet changes player Count's Volume, Rate and Loop.
	AudioSet
	// AudioClose stops and frees player Count. Nothing is reported of it after this.
	AudioClose
	// AudioReadLatency reads the current output route's latency.
	AudioReadLatency
	// AudioPlaybackSet updates Playback's volume and rate.
	AudioPlaybackSet
	// AudioPlaybackStop stops Playback alone.
	AudioPlaybackStop
)

var audioCommandNames = [...]string{"load", "play", "stop", "unload", "open", "preload", "resume", "pause", "seek", "set", "close", "read-latency", "playback-set", "playback-stop"}

// String returns the command's name, as traces print it: load, play, ...
func (c AudioCommand) String() string {
	if int(c) < len(audioCommandNames) {
		return audioCommandNames[c]
	}
	return "audio-command"
}

// AudioRequest is an ItemAudio's request. Count numbers the sound or the player, from one counter of the process, so a sound's and a player's are never the same.
type AudioRequest struct {
	Command AudioCommand
	// Playback identifies one playback of the sound named by Count. Zero plays without tracking it.
	Playback uint32
	// Source is AudioLoad's and AudioOpen's: an "asset:" path in the app's resources, a file path or file: URL, a data: URL, an http(s) URL, or Data. A platform reads it as it reads an image's.
	Source ImageSource
	// MaxBytes and MaxDuration are AudioLoad's limits.
	MaxBytes    int64
	MaxDuration time.Duration
	// Volume runs from 0 to 1, Rate from 0.5 to 2 and Pan from -1 (full left) to 1 (full right); AudioPlay's, with Volume and Rate AudioPlaybackSet's, AudioOpen's and AudioSet's too.
	Volume, Rate, Pan float32
	// StartAt is AudioPlay's scheduled start in the engine clock's time.
	StartAt time.Time
	// Loop repeats a playback of a sound or a player. A player uses LoopStart from AudioOpen and sends no AudioEnded while it loops.
	Loop      bool
	LoopStart time.Duration
	// Session is a player's: what it does to other apps' audio while it plays. Sounds mix with other apps' audio.
	Session AudioSession
	// ProgressInterval is the time between a playing player's AudioProgress reports, at least 16 milliseconds.
	ProgressInterval time.Duration
	// Position is AudioOpen's and AudioSeek's.
	Position time.Duration
}

// AudioLoopStart keeps a loop's start inside a finite track. An offset at or beyond the end falls back to the start of the track.
func AudioLoopStart(offset, duration time.Duration) time.Duration {
	if offset >= duration {
		return 0
	}
	return max(0, offset)
}

// AudioPhase is what an EventAudio reports of a sound or a player.
type AudioPhase uint8

const (
	// AudioLoaded says a sound decoded: Duration, Bytes and Cut.
	AudioLoaded AudioPhase = iota
	// AudioOpened says a player knows its Duration, 0 for a live stream.
	AudioOpened
	// AudioPlaying says a player plays.
	AudioPlaying
	// AudioPaused says a player paused, the app's Pause or the system's, such as an interruption or the app leaving the foreground.
	AudioPaused
	// AudioProgress says where a playing player is, every ProgressInterval and after a seek: Position, Duration, Seeks and Laps. The core reports the position once more as a player pauses or ends, so a platform need not.
	AudioProgress
	// AudioBuffer says whether a player waits for data (Buffering).
	AudioBuffer
	// AudioEnded says a player played to its end; never while it loops.
	AudioEnded
	// AudioFailure says a sound did not load or a player failed, with Error and Message. A player plays nothing more until a command loads it again.
	AudioFailure
	// AudioLatency reports the output route's latency.
	AudioLatency
	// AudioPlaybackEnded reports a sound playback's completion, stop, eviction, or failure to start.
	AudioPlaybackEnded
)

// The errors a sound or a player fails with, named as NSH's, which follow the web's MediaError.
const (
	AudioNotAllowed    = "not-allowed"
	AudioNetwork       = "network"
	AudioDecode        = "decode"
	AudioNotSupported  = "not-supported"
	AudioQuotaExceeded = "quota-exceeded"
	AudioUnavailable   = "unavailable"
	AudioFailed        = "failed"
)

// AudioEvent is an EventAudio's report.
type AudioEvent struct {
	Phase              AudioPhase
	Playback           uint32
	Duration, Position time.Duration
	// Latency is the output route's latency in AudioLatency, zero when unknown.
	Latency time.Duration
	// Bytes is a loaded sound's decoded size, which the core counts against MaxDecodedBytes, and Cut says it was longer than the MaxDuration decoded.
	Bytes int64
	Cut   bool
	// Buffering is AudioBuffer's.
	Buffering bool
	// Seeks counts the AudioSeeks the player applied and Laps the times a looping one started over, so the core drops a report from before the last seek and lets the position go back only with one of them.
	Seeks, Laps uint32
	// Error is one of the Audio errors and Message the platform's words.
	Error, Message string
}

// AudioType is the media type the first bytes of a sound or a player's Data say they are, for decoders that go by a file's name or type, as NSH names bytes the app gives: audio/wav, audio/aiff, audio/x-caf, audio/flac, audio/ogg, audio/mp4, audio/mpeg or audio/aac, and application/octet-stream for none of these.
func AudioType(data []byte) string {
	at := func(offset int, magic string) bool {
		return len(data) >= offset+len(magic) && string(data[offset:offset+len(magic)]) == magic
	}
	switch {
	case at(0, "RIFF") && at(8, "WAVE"):
		return "audio/wav"
	case at(0, "FORM") && (at(8, "AIFF") || at(8, "AIFC")):
		return "audio/aiff"
	case at(0, "caff"):
		return "audio/x-caf"
	case at(0, "fLaC"):
		return "audio/flac"
	case at(0, "OggS"):
		return "audio/ogg"
	case at(4, "ftyp"):
		return "audio/mp4"
	case at(0, "ID3"):
		return "audio/mpeg"
	case len(data) >= 2 && data[0] == 0xFF:
		// An MPEG audio frame's sync word: layer bits 00 are AAC's ADTS, the others MP3's.
		switch {
		case data[1]&0xF6 == 0xF0:
			return "audio/aac"
		case data[1]&0xE0 == 0xE0:
			return "audio/mpeg"
		}
	}
	return "application/octet-stream"
}

// AudioPanGains splits pan, from -1 (full left) to 1 (full right) with 0 centered, into left and right channel gains from 0 to 1 using a cosine balance curve: (1, 1) at 0, (1, 0) at -1 and (0, 1) at 1, for backends whose stereo voices take per-channel gains.
func AudioPanGains(pan float32) (left, right float32) {
	switch {
	case pan == 0 || pan != pan:
		return 1, 1
	case pan <= -1:
		return 1, 0
	case pan >= 1:
		return 0, 1
	case pan < 0:
		return 1, float32(math.Cos(float64(-pan) * math.Pi / 2))
	default:
		return float32(math.Cos(float64(pan) * math.Pi / 2)), 1
	}
}
