package engine

import "github.com/nsgo-dev/nsgo/internal/driver"

// A test's observer leaves the native PCM stream on the platform it wraps.
func (o observed) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	if f, ok := o.Platform.(driver.AudioOutputFactory); ok {
		return f.OpenAudioOutput(c)
	}
	return nil, driver.ErrAudioOutputUnsupported
}
