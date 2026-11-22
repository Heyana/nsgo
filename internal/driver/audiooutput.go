package driver

import (
	"errors"
	"time"
)

// ErrAudioOutputUnsupported reports a platform without pull PCM output.
var ErrAudioOutputUnsupported = errors.New("nsgo: audio: PCM output is unavailable")

// AudioOutputConfig describes the client's interleaved stream.
type AudioOutputConfig struct {
	SampleRate, Channels int
	Latency              time.Duration
	Render               func([]float32) int
	Interrupt            func(error)
}

// AudioOutputFactory opens a stream for the audio package. It is optional on platforms whose UI and app engine run in the same process.
type AudioOutputFactory interface {
	OpenAudioOutput(AudioOutputConfig) (AudioOutput, error)
}

// AudioOutput is a pull PCM stream owned by the app's scope.
type AudioOutput interface {
	Start() error
	Pause()
	Close()
	Flush() int
	Position() (int64, time.Time)
	Latency() time.Duration
	Underruns() int
}
