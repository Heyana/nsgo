package audiooutput_test

import (
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAudioOutputUnsupported(t *testing.T) {
	if target := nsgotest.Target(); target == "macos" || target == "ios" || target == "ios_simulator" || target == "android" || target == "web" || target == "windows" {
		t.Skip("this target supports pull PCM output")
	}
	var err error
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		_, err = audio.NewOutput(cx, audio.OutputConfig{
			SampleRate: 48000, Channels: 2, Render: func([]float32) int { return 0 },
		})
		return core.View{}
	})
	if !errors.Is(err, audio.ErrUnsupported) {
		t.Fatalf("NewOutput returned %v, want ErrUnsupported", err)
	}
}

func TestAudioOutputNative(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator", "android", "web", "windows":
	default:
		t.Skip("run on macOS, iOS, Android, Windows or the web")
	}
	const rate, channels = 48000, 2
	var calls atomic.Int64
	var badSlice atomic.Bool
	var nextSample int // only the audio thread writes it
	var output *audio.Output
	var openErr error
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		output, openErr = audio.NewOutput(cx, audio.OutputConfig{
			SampleRate: rate,
			Channels:   channels,
			Latency:    10 * time.Millisecond,
			Render: func(samples []float32) int {
				badSlice.Store(len(samples) == 0 || len(samples)%channels != 0)
				n := calls.Add(1)
				// Start with a short underrun, then give the device a sine wave.
				if n <= 2 {
					return 0
				}
				for i := 0; i < len(samples); i += channels {
					signal := 0.08 * float32(math.Sin(float64(nextSample)*2*math.Pi*440/rate))
					samples[i], samples[i+1] = signal, signal
					nextSample++
				}
				return len(samples) / channels
			},
		})
		return core.View{}
	})
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer output.Close()
	if frames, at := output.Position(); frames != 0 || !at.IsZero() || calls.Load() != 0 {
		t.Fatalf("output rendered before Start, position %d at %v, %d callbacks", frames, at, calls.Load())
	}
	if err := output.Start(); err != nil {
		t.Fatal(err)
	}
	var lastFrames int64
	var lastAt time.Time
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		frames, at := output.Position()
		if frames < lastFrames || at.Before(lastAt) || at.After(time.Now().Add(2*time.Millisecond)) {
			t.Fatalf("position %d at %v followed %d at %v", frames, at, lastFrames, lastAt)
		}
		lastFrames, lastAt = frames, at
		if frames >= rate/8 && calls.Load() > 2 && output.Underruns() >= 2 {
			if badSlice.Load() || output.Latency() <= 0 || time.Since(at) > 500*time.Millisecond {
				t.Fatalf("invalid slice, latency or position time: %v, %v, %v", badSlice.Load(), output.Latency(), at)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only reached frame %d, %d callbacks, %d underruns", frames, calls.Load(), output.Underruns())
		}
	}
	dropped := output.Flush()
	if frames, at := output.Position(); frames < lastFrames || at.Before(lastAt) || dropped < 0 {
		t.Fatalf("Flush dropped %d and moved position from %d at %v to %d at %v", dropped, lastFrames, lastAt, frames, at)
	}
	output.Pause()
	paused, at := output.Position()
	pausedCalls := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if frames, pausedAt := output.Position(); frames != paused || !pausedAt.Equal(at) || calls.Load() != pausedCalls {
		t.Fatalf("the paused output kept rendering: %d at %v, callbacks %d", frames, pausedAt, calls.Load())
	}
	if err := output.Start(); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if frames, at := output.Position(); frames > paused && at.After(lastAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Start did not resume the paused output")
		}
	}
	output.Close()
	output.Close()
	if err := output.Start(); err == nil {
		t.Fatal("the closed stream could be started")
	}
}
