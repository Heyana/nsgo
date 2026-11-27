package core

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ErrAudioOutputUnsupported is audio.ErrUnsupported, returned where the platform has no pull PCM output.
var ErrAudioOutputUnsupported = driver.ErrAudioOutputUnsupported

// AudioOutputConfig is the core's part of audio.OutputConfig.
type AudioOutputConfig struct {
	// SampleRate is the client's sample rate in hertz. Channels is 1 or 2.
	SampleRate, Channels int
	// Latency is a buffer duration hint. A zero value chooses the device's defaults.
	Latency time.Duration
	// Render fills interleaved Float32 samples in [-1, 1] and returns the number of frames written. It runs on the audio thread, never concurrently with itself. It must not block, allocate, use the graph or call Output methods. The buffer belongs to the callback. If Render returns fewer frames, the remainder is silent and the output counts one underrun.
	Render func(buf []float32) int
	// OnInterrupt reports a paused output after an interruption, route change or device error. It runs on the engine goroutine. Start resumes output.
	OnInterrupt func(error)
}

// AudioOutput is the core's part of audio.Output. The scope that opened it closes it on disposal.
type AudioOutput struct {
	backend driver.AudioOutput
	closed  atomic.Bool
}

// NewAudioOutput opens a paused PCM stream for audio.NewOutput.
func (cx *Scope) NewAudioOutput(conf AudioOutputConfig) (*AudioOutput, error) {
	var empty AudioSource
	if _, err := audioRefusal(cx, &empty); err != nil {
		return nil, err
	}
	t := cx.tree()
	if t == nil {
		return nil, ErrAudioOutputUnsupported
	}
	factory, ok := t.text.(driver.AudioOutputFactory)
	if !ok {
		return nil, ErrAudioOutputUnsupported
	}
	if conf.SampleRate <= 0 || conf.Channels < 1 || conf.Channels > 2 || conf.Render == nil || conf.Latency < 0 {
		return nil, audioFail(AudioNotSupported, "PCM output requires a positive sample rate, one or two channels, a render callback and nonnegative latency")
	}
	o := &AudioOutput{}
	var interrupt func(error)
	if conf.OnInterrupt != nil {
		owner := cx.current()
		interrupt = func(err error) {
			owner.Post(func() {
				if !o.closed.Load() {
					conf.OnInterrupt(err)
				}
			})
		}
	}
	backend, err := factory.OpenAudioOutput(driver.AudioOutputConfig{
		SampleRate: conf.SampleRate, Channels: conf.Channels, Latency: conf.Latency, Render: conf.Render, Interrupt: interrupt,
	})
	if err != nil {
		return nil, err
	}
	o.backend = backend
	cx.Cleanup(o.Close)
	return o, nil
}

// Start starts or resumes the output. It returns an error if the device cannot start.
func (o *AudioOutput) Start() error {
	if o.closed.Load() {
		return errors.New("nsgo: audio: PCM output is closed")
	}
	return o.backend.Start()
}

// Pause stops rendering until Start.
func (o *AudioOutput) Pause() { o.backend.Pause() }

// Close stops rendering and releases the output. Calling it again has no effect.
func (o *AudioOutput) Close() {
	o.closed.Store(true)
	o.backend.Close()
}

// Flush discards frames the renderer produced that have not reached the device and reports how many were discarded. Position never counts the discarded frames.
func (o *AudioOutput) Flush() int { return o.backend.Flush() }

// Position returns the monotonic number of frames that reached the device and the corresponding host time. The time is zero before the first frame.
func (o *AudioOutput) Position() (frames int64, at time.Time) { return o.backend.Position() }

// Latency reports the buffer latency the backend is using, or zero before it renders.
func (o *AudioOutput) Latency() time.Duration { return o.backend.Latency() }

// Underruns counts callbacks whose Render filled fewer frames than requested.
func (o *AudioOutput) Underruns() int { return o.backend.Underruns() }
