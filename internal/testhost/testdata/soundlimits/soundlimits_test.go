package soundlimits_test

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/audio"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestLoadsTwoHundredSoundsWithRaisedLimits(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios_simulator", "ios", "web":
	default:
		t.Skip("requires a backend that plays sounds")
	}
	const count = 200
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		status := core.NewSignal(cx, "loading")
		err := audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: count, MaxDuration: 20 * time.Second})
		if err != nil {
			return core.Text{TestID: "result", Content: err.Error()}
		}
		loaded, failed := 0, false
		for i := range count {
			length := 10 * time.Millisecond
			if i == 0 {
				length = 20 * time.Second
			}
			audio.LoadSound(cx, audio.Source{Data: wave(i, length)}, func(sound *audio.Sound, err error) {
				switch {
				case err != nil:
					failed = true
					status.Set(fmt.Sprintf("sound %d: %v", i, err))
				// The browser decodes at its own sample rate, so its duration can round off by a sample.
				case (sound.Duration() - length).Abs() > time.Millisecond:
					failed = true
					status.Set(fmt.Sprintf("sound %d: decoded %s, want %s", i, sound.Duration(), length))
				default:
					loaded++
					if loaded == count && !failed {
						status.Set("loaded")
					}
				}
			})
		}
		return core.Text{TestID: "result", ContentFunc: status.Get}
	})
	app.WaitUntil(20*time.Second, func() bool { return app.Find("id:result").Text != "loading" })
	if result := app.Find("id:result").Text; result != "loaded" {
		t.Fatal(result)
	}
}

// wave builds a distinct 16-bit mono WAV at 8 kHz.
func wave(id int, length time.Duration) []byte {
	const rate = 8000
	size := int(length*rate/time.Second) * 2
	data := make([]byte, 44+size)
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
	binary.LittleEndian.PutUint16(data[44:], uint16(id))
	return data
}
