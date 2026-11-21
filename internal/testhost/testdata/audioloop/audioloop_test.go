package audioloop_test

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

func TestMusicPlaysItsIntroAndRepeatsFromTheLoopStart(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator":
	default:
		t.Skip("requires an Apple audio backend")
	}
	const loopStart = 300 * time.Millisecond
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		result := core.NewSignal(cx, "playing")
		var previous time.Duration
		var intro, looped bool
		player := audio.NewPlayer(cx, audio.Source{Data: track()}, audio.PlayerOptions{
			LoopStart:        loopStart,
			ProgressInterval: 20 * time.Millisecond,
			OnProgress: func(position, _ time.Duration) {
				intro = intro || position > 0 && position < loopStart
				looped = looped || intro && position >= loopStart && position < previous
				previous = position
				if looped {
					result.Set("looped")
				}
			},
			OnEnd:   func() { result.Set("ended before repeating") },
			OnError: func(err error) { result.Set(err.Error()) },
		})
		player.SetLoop(true)
		player.Play(nil)
		return core.Text{TestID: "result", ContentFunc: result.Get}
	})
	app.WaitUntil(15*time.Second, func() bool { return app.Find("id:result").Text != "playing" })
	if result := app.Find("id:result").Text; result != "looped" {
		t.Fatal(result)
	}
}

// track is a 750 ms, 8 kHz mono PCM track with a quiet fixed tone.
func track() []byte {
	const rate, frames = 8000, 6000
	data := make([]byte, 44+2*frames)
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
	binary.LittleEndian.PutUint32(data[40:], 2*frames)
	for i := range frames {
		binary.LittleEndian.PutUint16(data[44+2*i:], uint16((i%20)*8))
	}
	return data
}
