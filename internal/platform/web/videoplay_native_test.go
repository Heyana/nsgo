//go:build js && wasm && nsgotest

package web

import (
	"encoding/binary"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Trusted pointer input drives the browser’s own play button on a real media
// element, and core receives the resulting playback callbacks.
func TestVideoControlsReportAndHoldPlayback(t *testing.T) {
	wav := make([]byte, 44+16000*20)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(wav)-44))
	data := js.Global().Get("Uint8Array").New(len(wav))
	js.CopyBytesToJS(data, wav)
	blob := js.Global().Get("Blob").New([]any{data}, map[string]any{"type": "audio/wav"})
	url := js.Global().Get("URL").Call("createObjectURL", blob).String()
	defer js.Global().Get("URL").Call("revokeObjectURL", url)
	events := make(chan string, 20)
	native, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		return core.Video{Source: url, Muted: true, Controls: true, Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)}, OnPause: func(reason core.VideoPauseReason) {
			if reason != core.VideoPauseControls {
				events <- "wrong reason"
			} else {
				events <- "pause"
			}
		}, OnPlay: func() { events <- "play" }}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	host := native.(*testSurface)
	defer host.s.engine.Close()
	host.Sync()
	var v *view
	for _, candidate := range host.s.views {
		if candidate != nil && candidate.video != nil {
			v = candidate
			break
		}
	}
	if v == nil {
		t.Fatal("no native video")
	}
	vd := v.video
	config := driver.VideoConfig{Source: url, Muted: true, Controls: true, Volume: 1, Rate: 1, ProgressInterval: time.Second}
	wait := func(playing bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); vd.el.Get("paused").Bool() == playing || vd.el.Get("readyState").Int() < 2; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("media element did not reach playing=%v", playing)
			}
		}
	}
	event := func(want string) {
		t.Helper()
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("event=%s, want %s", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("missing %s callback", want)
		}
	}
	quiet := func() {
		t.Helper()
		time.Sleep(100 * time.Millisecond)
		select {
		case got := <-events:
			t.Fatalf("unexpected callback %s", got)
		default:
		}
	}
	wait(true)
	quiet()
	control := func() {
		rect := vd.el.Call("getBoundingClientRect")
		point := driver.Point{X: driver.Points(rect.Get("left").Float() + 24), Y: driver.Points(rect.Get("bottom").Float() - 24)}
		if err := host.Hover(point); err != nil {
			t.Fatal(err)
		}
		if err := host.Press(point, 0, driver.Modifiers{}); err != nil {
			t.Fatal(err)
		}
	}
	control()
	wait(false)
	event("pause")
	config.Volume = .3
	host.s.setVideo(v, &config)
	quiet()
	if !vd.el.Get("paused").Bool() {
		t.Fatal("volume change resumed a controls pause")
	}
	control()
	wait(true)
	event("play")
	// A config can arrive before the browser dispatches the control’s event.
	vd.el.Call("pause")
	config.Volume = .4
	host.s.setVideo(v, &config)
	event("pause")
	quiet()
	if !vd.el.Get("paused").Bool() {
		t.Fatal("config overtook the queued native pause")
	}
	vd.el.Call("play")
	config.Volume = .5
	host.s.setVideo(v, &config)
	event("play")
	quiet()
	config.Paused = true
	host.s.setVideo(v, &config)
	wait(false)
	quiet()
	control()
	wait(true)
	event("play")
	config.Rate = 1.25
	host.s.setVideo(v, &config)
	quiet()
	if vd.el.Get("paused").Bool() {
		t.Fatal("rate change stopped a controls play while Paused stayed true")
	}
	config.Paused = false
	host.s.setVideo(v, &config)
	quiet()
	config.Paused = true
	host.s.setVideo(v, &config)
	wait(false)
	quiet()
}
