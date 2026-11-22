//go:build js && wasm && nsgotest

package web

import (
	_ "embed"
	"encoding/binary"
	"math"
	"slices"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

//go:embed testdata/audio/tone.m4a
var aacTone []byte

//go:embed testdata/audio/tone.mp3
var mp3Tone []byte

func audioWAV(duration time.Duration) []byte {
	const sampleRate = 8000
	n := int(duration.Seconds() * sampleRate)
	data := make([]byte, 44+2*n)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], sampleRate)
	binary.LittleEndian.PutUint32(data[28:], 2*sampleRate)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*n))
	for i := range n {
		tone := int16(6000 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		binary.LittleEndian.PutUint16(data[44+2*i:], uint16(tone))
	}
	return data
}

func audioWait(t *testing.T, what string, ready func() bool) {
	t.Helper()
	for until := time.Now().Add(5 * time.Second); !ready(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(until) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func pressAudio(t *testing.T, doc js.Value, a *webAudio, play func()) {
	t.Helper()
	control := doc.Call("createElement", "button")
	control.Set("textContent", "Play")
	control.Set("style", "position:fixed;top:0;left:0;width:60px;height:60px;z-index:1000")
	doc.Get("body").Call("appendChild", control)
	defer control.Call("remove")
	var ls listeners
	defer ls.release()
	ls.on(control, "pointerdown", func(e js.Value) {
		if e.Get("isTrusted").Bool() {
			a.activate()
		}
	})
	ls.on(control, "click", func(js.Value) { play() })
	host := &testSurface{}
	if err := host.do(map[string]any{"op": "press", "x": 30, "y": 30, "hold": 0}); err != nil {
		t.Fatal(err)
	}
}

func TestWebAudioSounds(t *testing.T) {
	doc := js.Global().Get("document")
	reports := map[uint32]driver.AudioEvent{}
	a := newWebAudio(doc, func(e driver.Event) { reports[e.Count] = *e.Data.(*driver.AudioEvent) }, false)
	defer a.close()
	if !webAudioAvailable() || a.output.Truthy() {
		t.Fatal("the browser must support audio without opening the output")
	}
	if a.sourceURL("asset:/audio/tone.wav") == "" || a.sourceURL("asset:/audio/../tone.wav") != "" {
		t.Fatal("the browser did not validate audio asset paths")
	}
	wav := audioWAV(600 * time.Millisecond)
	load := func(id uint32, src driver.ImageSource, bytes int64) driver.AudioEvent {
		t.Helper()
		a.handle(id, &driver.AudioRequest{Command: driver.AudioLoad, Source: src, MaxBytes: bytes, MaxDuration: driver.MaxSoundDuration})
		audioWait(t, "sound load", func() bool { _, ok := reports[id]; return ok })
		return reports[id]
	}
	for i, data := range [][]byte{wav, aacTone, mp3Tone} {
		if e := load(uint32(i+1), driver.ImageSource{Data: data}, driver.MaxSoundBytes); e.Phase != driver.AudioLoaded ||
			e.Duration < 550*time.Millisecond || e.Duration > 700*time.Millisecond || e.Bytes <= 0 {
			t.Fatalf("sound %d: %+v", i, e)
		}
	}
	url := js.Global().Get("URL")
	blob := js.Global().Get("Blob").New([]any{audioBytes(wav)}, map[string]any{"type": "audio/wav"})
	uri := url.Call("createObjectURL", blob).String()
	defer url.Call("revokeObjectURL", uri)
	if e := load(4, driver.ImageSource{URI: uri}, driver.MaxSoundBytes); e.Phase != driver.AudioLoaded {
		t.Fatalf("sound from URL: %+v", e)
	}
	if e := load(5, driver.ImageSource{URI: uri}, 100); e.Error != driver.AudioQuotaExceeded {
		t.Fatalf("sound beyond the encoded limit: %+v", e)
	}
	if e := load(6, driver.ImageSource{Data: []byte("not audio")}, driver.MaxSoundBytes); e.Error != driver.AudioDecode {
		t.Fatalf("undecodable sound: %+v", e)
	}
	if e := load(7, driver.ImageSource{Data: audioWAV(6 * time.Second)}, driver.MaxSoundBytes); !e.Cut || a.sounds[7].buffer.Truthy() {
		t.Fatalf("sound beyond the duration limit: %+v", e)
	}

	a.play(1, 1, 1, 0, time.Time{}, false)
	if a.output.Truthy() || len(a.voices) != 0 {
		t.Fatal("sound opened the playback output before a gesture")
	}
	var started time.Time
	var analyser js.Value
	pressAudio(t, doc, a, func() {
		started = time.Now()
		a.play(1, 0.8, 1, 0, time.Time{}, false)
		if len(a.voices) == 1 {
			analyser = a.output.Call("createAnalyser")
			analyser.Set("fftSize", 256)
			a.voices[0].gain.Call("connect", analyser)
		}
	})
	if !analyser.Truthy() {
		t.Fatal("the first trusted gesture dropped sound playback")
	}
	samples := js.Global().Get("Float32Array").New(256)
	audioWait(t, "a rendered audio sample", func() bool {
		analyser.Call("getFloatTimeDomainData", samples)
		for i := range samples.Length() {
			if math.Abs(samples.Index(i).Float()) > 0.005 {
				return true
			}
		}
		return false
	})
	t.Logf("first Play to rendered audio signal: %s", time.Since(started).Round(time.Millisecond))
	analyser.Call("disconnect")
	a.stop(1)

	// A looping voice outlives its 600 ms buffer until its sound stops.
	a.play(1, 0.5, 2, 0, time.Time{}, true)
	looping := a.voices[len(a.voices)-1]
	if !looping.source.Get("loop").Bool() {
		t.Fatal("a looping voice played its buffer once")
	}
	time.Sleep(700 * time.Millisecond)
	if !slices.Contains(a.voices, looping) {
		t.Fatal("a looping voice ended on its own")
	}
	a.stop(1)
	if len(a.voices) != 0 {
		t.Fatal("stopping the sound kept its looping voice")
	}

	a.play(2, 1, 1, 0, time.Time{}, false)
	oldest := a.voices[0]
	for range driver.MaxSoundVoices {
		a.play(1, 0.5, 1.2, -0.5, time.Time{}, false)
	}
	if panner := a.voices[len(a.voices)-1].panner; !panner.Truthy() || math.Abs(panner.Get("pan").Get("value").Float()+0.5) > 1e-6 {
		t.Fatal("a panned voice played without its stereo panner")
	}
	if len(a.voices) != driver.MaxSoundVoices || a.voices[0] == oldest {
		t.Fatal("the oldest voice survived the overlap limit")
	}
	a.play(2, 1, 1, 0, time.Time{}, false)
	a.stop(1)
	if len(a.voices) != 1 || a.voices[0].sound != 2 {
		t.Fatal("stopping one sound interrupted a different sound")
	}
	audioWait(t, "natural voice completion", func() bool { return len(a.voices) == 0 })
	a.unload(2)
	if a.sounds[2] != nil {
		t.Fatal("unload left a decoded sound")
	}
}

// onsetTap posts the context frame of every sample above the threshold that follows at least options.silence quiet frames; a tone's zero crossings are much shorter. An unconnected input renders no channels and counts as one quiet render quantum.
const onsetTap = `registerProcessor("nsgo-onset-tap", class extends AudioWorkletProcessor {
	constructor(options) {
		super();
		this.silence = options.processorOptions.silence;
		this.quiet = 0;
	}
	process(inputs, outputs) {
		const samples = inputs[0][0];
		if (!samples) {
			this.quiet += outputs[0][0]?.length ?? 128;
			return true;
		}
		for (let i = 0; i < samples.length; i++) {
			if (Math.abs(samples[i]) < 0.01) {
				this.quiet++;
				continue;
			}
			if (this.quiet > this.silence) this.port.postMessage(currentFrame + i);
			this.quiet = 0;
		}
		return true;
	}
});`

// TestWebAudioSchedulesStarts measures two voices scheduled 500 ms apart, in different turns, in the samples the output context renders.
func TestWebAudioSchedulesStarts(t *testing.T) {
	doc := js.Global().Get("document")
	reports := map[uint32]driver.AudioEvent{}
	a := newWebAudio(doc, func(e driver.Event) { reports[e.Count] = *e.Data.(*driver.AudioEvent) }, false)
	defer a.close()
	a.handle(1, &driver.AudioRequest{Command: driver.AudioReadLatency})
	if e, ok := reports[1]; !ok || e.Phase != driver.AudioLatency || e.Latency != 0 {
		t.Fatalf("latency without an output: %+v", e)
	}
	a.handle(2, &driver.AudioRequest{Command: driver.AudioLoad, Source: driver.ImageSource{Data: audioWAV(100 * time.Millisecond)},
		MaxBytes: driver.MaxSoundBytes, MaxDuration: driver.MaxSoundDuration})
	audioWait(t, "sound load", func() bool { _, ok := reports[2]; return ok })
	if e := reports[2]; e.Phase != driver.AudioLoaded {
		t.Fatalf("sound load: %+v", e)
	}
	pressAudio(t, doc, a, a.resumeOutput)
	if !a.output.Truthy() {
		t.Fatal("the gesture did not open the output")
	}
	audioWait(t, "a running output", func() bool { return a.output.Get("state").String() == "running" })
	a.handle(3, &driver.AudioRequest{Command: driver.AudioReadLatency})
	if e := reports[3]; e.Phase != driver.AudioLatency || e.Latency < 0 {
		t.Fatalf("latency of the running output: %+v", e)
	}
	t.Logf("output latency %v", reports[3].Latency)
	worklet := a.output.Get("audioWorklet")
	if !worklet.Truthy() {
		t.Skip("the browser has no AudioWorklet to record its output")
	}
	// The tap finds onsets on the audio thread and reports them as frames of the context's own timeline, where source.start is sample accurate. A ScriptProcessorNode's playbackTime is read on the main thread when its event runs, so a busy main thread shifts it by whole render quanta (Chromium's ScriptProcessorHandler::FireProcessEvent), and it may drop a buffer altogether.
	module := js.Global().Get("Blob").New([]any{onsetTap}, map[string]any{"type": "text/javascript"})
	moduleURL := js.Global().Get("URL").Call("createObjectURL", module)
	defer js.Global().Get("URL").Call("revokeObjectURL", moduleURL)
	if _, err := await(worklet.Call("addModule", moduleURL)); err != nil {
		t.Fatalf("load the onset tap: %v", err)
	}
	rate := a.output.Get("sampleRate").Float()
	tap := js.Global().Get("AudioWorkletNode").New(a.output, "nsgo-onset-tap", map[string]any{"processorOptions": map[string]any{"silence": int(rate / 50)}})
	tap.Call("connect", a.output.Get("destination"))
	defer tap.Call("disconnect")
	var onsets []float64
	report := js.FuncOf(func(_ js.Value, args []js.Value) any {
		onsets = append(onsets, args[0].Get("data").Float()/rate)
		return nil
	})
	defer report.Release()
	tap.Get("port").Set("onmessage", report)
	defer tap.Get("port").Set("onmessage", js.Null())
	first := time.Now().Add(300 * time.Millisecond)
	a.play(2, 1, 1, 0, first, false)
	a.voices[len(a.voices)-1].gain.Call("connect", tap)
	// The second voice is scheduled in a later turn, after the context's clock moved on.
	time.Sleep(70 * time.Millisecond)
	a.play(2, 1, 1, 0, first.Add(500*time.Millisecond), false)
	a.voices[len(a.voices)-1].gain.Call("connect", tap)
	audioWait(t, "two scheduled starts", func() bool { return len(onsets) >= 2 })
	if gap := onsets[1] - onsets[0]; math.Abs(gap-0.5) > 0.001 {
		t.Fatalf("scheduled starts %.4fs apart in the output, want 0.5s within 1ms", gap)
	}
}

func TestWebAudioPlayer(t *testing.T) {
	doc := js.Global().Get("document")
	var reports []driver.AudioEvent
	a := newWebAudio(doc, func(e driver.Event) { reports = append(reports, *e.Data.(*driver.AudioEvent)) }, false)
	defer a.close()
	send := func(command driver.AudioCommand) {
		a.handle(1, &driver.AudioRequest{Command: command})
	}
	wait := func(phase driver.AudioPhase, check func(e driver.AudioEvent) bool) driver.AudioEvent {
		t.Helper()
		var last driver.AudioEvent
		audioWait(t, "player report", func() bool {
			for _, r := range reports {
				if r.Phase == phase && (check == nil || check(r)) {
					last = r
					return true
				}
			}
			return false
		})
		return last
	}
	a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: 1,
		ProgressInterval: 20 * time.Millisecond, Source: driver.ImageSource{Data: audioWAV(700 * time.Millisecond)}})
	p := a.players[1]
	if p.el.Truthy() || a.output.Truthy() {
		t.Fatal("opening a player before preload created an output")
	}
	send(driver.AudioPreload)
	if e := wait(driver.AudioOpened, nil); e.Duration != 700*time.Millisecond {
		t.Fatalf("unexpected preload report: %+v", e)
	}
	send(driver.AudioResume)
	if e := wait(driver.AudioFailure, nil); e.Error != driver.AudioNotAllowed {
		t.Fatalf("play before activation: %+v", e)
	}
	reports = nil
	pressAudio(t, doc, a, func() { send(driver.AudioResume) })
	wait(driver.AudioPlaying, nil)
	send(driver.AudioPause)
	wait(driver.AudioPaused, nil)
	a.handle(1, &driver.AudioRequest{Command: driver.AudioSeek, Position: 350 * time.Millisecond})
	wait(driver.AudioProgress, func(e driver.AudioEvent) bool { return e.Position == 350*time.Millisecond && e.Seeks == 1 })
	a.handle(1, &driver.AudioRequest{Command: driver.AudioSet, Volume: 0.25, Rate: 2, Loop: true})
	reports = nil
	send(driver.AudioResume)
	wait(driver.AudioPlaying, nil)
	wait(driver.AudioProgress, func(e driver.AudioEvent) bool { return e.Laps > 0 })
	for _, e := range reports {
		if e.Phase == driver.AudioEnded {
			t.Fatal("a looping player emitted an end event")
		}
	}
	a.handle(1, &driver.AudioRequest{Command: driver.AudioSet, Volume: 0.4, Rate: 2})
	wait(driver.AudioEnded, nil)
	reports = nil
	send(driver.AudioResume)
	wait(driver.AudioPlaying, nil)
	wait(driver.AudioProgress, func(e driver.AudioEvent) bool { return e.Position < 350*time.Millisecond })
	send(driver.AudioClose)
	if p.el.Truthy() || p.timer.Truthy() || p.objectURL != "" || a.output.Truthy() {
		t.Fatal("closing left player resources")
	}

	// A loop with a start plays its intro once and then repeats from LoopStart, never ending.
	const loopStart = 400 * time.Millisecond
	reports = nil
	a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 0.25, Rate: 2, Loop: true, LoopStart: loopStart,
		ProgressInterval: 20 * time.Millisecond, Source: driver.ImageSource{Data: audioWAV(700 * time.Millisecond)}})
	p = a.players[1]
	send(driver.AudioPreload)
	wait(driver.AudioOpened, nil)
	if p.el.Get("loop").Bool() {
		t.Fatal("a loop with a start used the element's loop, which restarts at 0")
	}
	pressAudio(t, doc, a, func() { send(driver.AudioResume) })
	wait(driver.AudioPlaying, nil)
	wait(driver.AudioProgress, func(e driver.AudioEvent) bool { return e.Laps == 1 && e.Position == loopStart })
	wait(driver.AudioProgress, func(e driver.AudioEvent) bool { return e.Laps == 2 })
	lapped := false
	for _, e := range reports {
		if e.Phase == driver.AudioEnded {
			t.Fatal("a player looping from its start emitted an end event")
		}
		if e.Phase == driver.AudioProgress && e.Laps > 0 {
			lapped = true
		}
		if lapped && e.Phase == driver.AudioProgress && e.Position < loopStart-50*time.Millisecond {
			t.Fatalf("a repeat went back into the intro: %+v", e)
		}
	}
	a.handle(1, &driver.AudioRequest{Command: driver.AudioSet, Volume: 0.25, Rate: 2})
	wait(driver.AudioEnded, nil)
	send(driver.AudioClose)

	for _, data := range [][]byte{aacTone, mp3Tone, []byte("not audio")} {
		reports = nil
		a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: 1, Source: driver.ImageSource{Data: data}})
		send(driver.AudioPreload)
		if driver.AudioType(data) == "application/octet-stream" {
			if e := wait(driver.AudioFailure, nil); e.Error != driver.AudioDecode {
				t.Fatalf("undecodable player media: %+v", e)
			}
		} else if e := wait(driver.AudioOpened, nil); e.Duration < 550*time.Millisecond || e.Duration > 700*time.Millisecond {
			t.Fatalf("unexpected encoded player duration: %+v", e)
		}
		send(driver.AudioClose)
	}
}

func TestWebAudioThroughApp(t *testing.T) {
	doc := js.Global().Get("document")
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-audio-test")
	setStyle(mount, map[string]string{"width": "200px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	var sound *core.Sound
	var loadErr, playErr error
	var loadedPlayer, played bool
	s := open(func(cx *core.Scope) core.Node {
		wav := audioWAV(2 * time.Second)
		cx.LoadSound(core.AudioSource{Data: wav}, func(loaded *core.Sound, err error) {
			sound, loadErr = loaded, err
		})
		player := cx.NewAudioPlayer(core.AudioSource{Data: wav}, core.AudioPlayerOptions{
			OnLoad: func(time.Duration) { loadedPlayer = true },
		})
		player.Preload()
		return core.View{
			Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)},
			OnPress: func(core.PressEvent) {
				if sound != nil {
					sound.Play(core.SoundVoice{})
				}
				player.Play(func(err error) { played, playErr = true, err })
			},
		}
	}, Options{Mount: "#nsgo-audio-test"}, nil)
	defer s.engine.Close()
	audioWait(t, "the app's sound and player to load", func() bool { return sound != nil && loadedPlayer || loadErr != nil })
	if loadErr != nil || !s.env.Audio || s.audio.output.Truthy() {
		t.Fatalf("preload through the app: availability %t, error %v", s.env.Audio, loadErr)
	}
	host := &testSurface{s: s}
	if err := host.Press(driver.Point{X: 20, Y: 20}, 0, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	audioWait(t, "the app's audio to start", func() bool { return len(s.audio.voices) == 1 && played })
	if playErr != nil {
		t.Fatal(playErr)
	}
	a := s.audio
	js.Global().Call("dispatchEvent", js.Global().Get("PageTransitionEvent").New("pagehide"))
	if s.audio != nil || a.output.Truthy() || len(a.sounds) != 0 || len(a.voices) != 0 || len(a.players) != 0 {
		t.Fatal("leaving the page kept audio resources")
	}
}
