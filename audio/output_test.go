package audio_test

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNewOutputUnsupportedHeadless(t *testing.T) {
	var err error
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		_, err = audio.NewOutput(cx, audio.OutputConfig{
			SampleRate: 48000,
			Channels:   2,
			Render:     func([]float32) int { return 0 },
		})
		return core.View{}
	})
	if !errors.Is(err, audio.ErrUnsupported) {
		t.Fatalf("NewOutput headless error = %v, want ErrUnsupported", err)
	}
}
