package audio_test

import (
	"encoding/binary"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestSoundPlaybackCompletes(t *testing.T) {
	switch nsgotest.Target() {
	case "ios", "ios_simulator", "macos", "linux", "web":
	default:
		t.Skip("runs a backend with individual sound playback controls")
	}
	data := silentWAV(2)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		sound := core.NewSignal[*audio.Sound](cx, nil)
		active := core.NewSignal[*audio.Playback](cx, nil)
		audio.LoadSound(cx, audio.Source{Data: data}, func(s *audio.Sound, err error) {
			if err != nil {
				t.Error(err)
			}
			sound.Set(s)
		})
		return core.View{Style: core.Style{Padding: core.All(40)}, Children: []core.Node{
			core.Text{ContentFunc: func() string {
				switch p := active.Get(); {
				case p != nil && p.Ended():
					return "Finished"
				case p != nil:
					return "Playing"
				case sound.Get() != nil:
					return "Ready"
				default:
					return "Loading"
				}
			}},
			core.Text{TestID: "play", Content: "Play", Style: core.Style{Padding: core.All(16)}, OnPress: func(core.PressEvent) {
				s := sound.Get()
				first := s.Play(audio.Voice{Muted: true})
				second := s.Play(audio.Voice{Muted: true})
				second.SetRate(2)
				first.Stop()
				active.Set(second)
			}},
		}}
	})
	app.WaitFor("text:Ready", 8*time.Second)
	app.Press("id:play")
	app.Find("text:Playing")
	app.WaitFor("text:Finished", 5*time.Second)
}

func silentWAV(seconds int) []byte {
	const rate = 48000
	size := seconds * rate * 2
	data := make([]byte, size+44)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(size))
	return data
}
