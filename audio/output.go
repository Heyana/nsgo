package audio

import "github.com/nsgo-dev/nsgo/core"

// Output is a scope-owned stream of PCM samples pulled by an audio device.
type Output = core.AudioOutput

// OutputConfig describes the sample rate, channels and callback for a PCM stream.
type OutputConfig = core.AudioOutputConfig

// ErrUnsupported reports a target without pull PCM output.
var ErrUnsupported = core.ErrAudioOutputUnsupported

// NewOutput opens a paused stream on macOS, iOS, Android and the web. Start begins pulling samples. Other targets return ErrUnsupported.
func NewOutput(cx *core.Scope, opts OutputConfig) (*Output, error) {
	return cx.NewAudioOutput(opts)
}
