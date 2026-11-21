package audio_test

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

type denyAudioAccess struct{}

func (denyAudioAccess) Allows(core.AccessRequest) bool { return false }
func (denyAudioAccess) WebHosts() []string             { return nil }

func TestNewOutputUnsupportedHeadless(t *testing.T) {
	var err error
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		_, err = audio.NewOutput(cx, audio.OutputConfig{
			SampleRate: 48000,
			Channels:   2,
			Render:     func([]float32) int { return 0 },
		})
		return core.View{}
	}, playsAudio)
	if !errors.Is(err, audio.ErrUnsupported) {
		t.Fatalf("NewOutput headless = %v, want ErrUnsupported", err)
	}
}

func TestNewOutputRefusedWithoutAudioAccess(t *testing.T) {
	var err error
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		child := cx.Child()
		child.ProvideAccess(denyAudioAccess{})
		_, err = audio.NewOutput(child, audio.OutputConfig{
			SampleRate: 48000,
			Channels:   2,
			Render:     func([]float32) int { return 0 },
		})
		return core.View{}
	}, playsAudio)
	if !errors.Is(err, audio.NotAllowed) {
		t.Fatalf("NewOutput without AccessAudio = %v, want NotAllowed", err)
	}
}
