//go:build android

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// OpenAudioOutput opens a pull PCM output stream backed by Android's AudioTrack in the Java runtime.
func (s *surface) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	return s.audioOutputs.open(c)
}

// OpenAudioOutput shares the primary surface's audio outputs with overlay trees.
func (o *overlay) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	return o.s.OpenAudioOutput(c)
}

func (s *surface) audioOutputReport(r *reader) {
	s.audioOutputs.report(r)
}

func (s *surface) closeAudioOutputs() {
	s.audioOutputs.closeAll()
}
