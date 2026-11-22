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
	a.handle(8, &driver.AudioRequest{Command: driver.AudioLoad, Source: driver.ImageSource{Data: wav}, MaxBytes: driver.MaxSoundBytes,
		MaxDecodedBytes: 1024, MaxDuration: driver.MaxSoundDuration})
	audioWait(t, "sound load", func() bool { _, ok := reports[8]; return ok })
	if e := reports[8]; e.Error != driver.AudioQuotaExceeded || a.sounds[8] != nil {
		t.Fatalf("sound beyond the decoded size limit: %+v", e)
	}
	a.handle(9, &driver.AudioRequest{Command: driver.AudioLoad, Source: driver.ImageSource{Data: audioWAV(6 * time.Second)}, MaxBytes: driver.MaxSoundBytes,
		MaxDuration: 10 * time.Second})
	audioWait(t, "sound load", func() bool { _, ok := reports[9]; return ok })
	if e := reports[9]; e.Phase != driver.AudioLoaded || e.Cut || !a.sounds[9].buffer.Truthy() {
		t.Fatalf("sound within a raised duration limit: %+v", e)
	}

	a.play(1, &driver.AudioRequest{Volume: 1, Rate: 1, Playback: 900})
	if a.output.Truthy() || len(a.voices) != 0 {
		t.Fatal("sound opened the playback output before a gesture")
	}
	if e := reports[1]; e.Phase != driver.AudioPlaybackEnded || e.Playback != 900 {
		t.Fatalf("a blocked playback stayed active: %+v", e)
	}
	var started time.Time
	var analyser js.Value
	pressAudio(t, doc, a, func() {
		started = time.Now()
		a.play(1, &driver.AudioRequest{Volume: 0.8, Rate: 1})
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
	a.play(1, &driver.AudioRequest{Volume: 0.5, Rate: 2, Loop: true})
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

	a.play(2, &driver.AudioRequest{Volume: 1, Rate: 1, Playback: 901})
	oldest := a.voices[0]
	for range driver.MaxSoundVoices {
		a.play(1, &driver.AudioRequest{Volume: 0.5, Rate: 1.2, Pan: -0.5})
	}
	if panner := a.voices[len(a.voices)-1].panner; !panner.Truthy() || math.Abs(panner.Get("pan").Get("value").Float()+0.5) > 1e-6 {
		t.Fatal("a panned voice played without its stereo panner")
	}
	if len(a.voices) != driver.MaxSoundVoices || a.voices[0] == oldest {
		t.Fatal("the oldest voice survived the overlap limit")
	}
	if e := reports[2]; e.Phase != driver.AudioPlaybackEnded || e.Playback != 901 {
		t.Fatalf("the evicted playback stayed active: %+v", e)
	}
	// A raised engine limit keeps more voices, while restricted playbacks stay within the default cap among themselves.
	const raised = driver.MaxSoundVoices + 8
	for range raised {
		a.play(1, &driver.AudioRequest{Volume: 0.1, Rate: 1, MaxPlaybacks: raised})
	}
	if len(a.voices) != raised {
		t.Fatalf("voices at a raised limit = %d, want %d", len(a.voices), raised)
	}
	for range driver.MaxSoundVoices + 1 {
		a.play(1, &driver.AudioRequest{Volume: 0.1, Rate: 1, MaxPlaybacks: raised * 2, Restricted: true})
	}
	restricted := 0
	for _, v := range a.voices {
		if v.restricted {
			restricted++
		}
	}
	if restricted != driver.MaxSoundVoices || len(a.voices) != raised+driver.MaxSoundVoices {
		t.Fatalf("restricted voices = %d of %d, want %d of %d", restricted, len(a.voices), driver.MaxSoundVoices, raised+driver.MaxSoundVoices)
	}
	a.play(1, &driver.AudioRequest{Volume: 0.1, Rate: 1, MaxPlaybacks: 4})
	if len(a.voices) != 4 {
		t.Fatalf("voices after lowering the limit = %d, want 4", len(a.voices))
	}
	a.play(2, &driver.AudioRequest{Volume: 1, Rate: 1, Playback: 901})
	a.stop(1)
	if len(a.voices) != 1 || a.voices[0].sound != 2 {
		t.Fatal("stopping one sound interrupted a different sound")
	}
	audioWait(t, "natural voice completion", func() bool { return len(a.voices) == 0 })
	if _, err := await(a.output.Call("close")); err != nil {
		t.Fatal(err)
	}
	a.play(1, &driver.AudioRequest{Volume: 1, Rate: 1, Playback: 902})
	audioWait(t, "refused output resume", func() bool {
		e := reports[1]
		return e.Phase == driver.AudioPlaybackEnded && e.Playback == 902 && len(a.voices) == 0
	})
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
	a.play(2, &driver.AudioRequest{Volume: 1, Rate: 1, StartAt: first})
	a.voices[len(a.voices)-1].gain.Call("connect", tap)
	// The second voice is scheduled in a later turn, after the context's clock moved on.
	time.Sleep(70 * time.Millisecond)
	a.play(2, &driver.AudioRequest{Volume: 1, Rate: 1, StartAt: first.Add(500 * time.Millisecond)})
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
	if p.el.Truthy() || p.node.Truthy() || p.track.Truthy() || p.timer.Truthy() || p.objectURL != "" {
		t.Fatal("closing left player resources")
	}

	// A loop with a start plays its intro once and then repeats from LoopStart, never ending.
	const loopStart = 400 * time.Millisecond
	reports = nil
	a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 0.25, Rate: 1, Loop: true, LoopStart: loopStart,
		ProgressInterval: 20 * time.Millisecond, Source: driver.ImageSource{Data: audioWAV(700 * time.Millisecond)}})
	p = a.players[1]
	send(driver.AudioPreload)
	if e := wait(driver.AudioOpened, nil); e.Duration != 700*time.Millisecond {
		t.Fatalf("unexpected decoded duration: %+v", e)
	}
	if p.el.Truthy() || !p.track.Truthy() {
		t.Fatal("a looping player streamed its track instead of decoding it")
	}
	pressAudio(t, doc, a, func() { send(driver.AudioResume) })
	wait(driver.AudioPlaying, nil)
	wait(driver.AudioProgress, func(e driver.AudioEvent) bool { return e.Laps == 1 })
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
	a.handle(1, &driver.AudioRequest{Command: driver.AudioSet, Volume: 0.25, Rate: 1})
	wait(driver.AudioEnded, nil)
	send(driver.AudioClose)

	// A track longer than a gapless loop decodes streams through the element, whose own loop repeats it.
	reports = nil
	a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: 1, Loop: true,
		Source: driver.ImageSource{Data: audioWAV(driver.MaxGaplessLoopDuration + time.Second)}})
	p = a.players[1]
	send(driver.AudioPreload)
	if e := wait(driver.AudioOpened, nil); e.Duration <= driver.MaxGaplessLoopDuration {
		t.Fatalf("unexpected long track: %+v", e)
	}
	if !p.noTrack || p.track.Truthy() || !p.el.Get("loop").Bool() {
		t.Fatal("a track too long to decode did not loop through the element")
	}
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

// TestWebAudioPlayerHandsOverToTrack turns a player's loop on after its element started: on one still loading, the loop decodes the track once the metadata loads; on one playing, the element plays on and the decoded track takes over at its end.
func TestWebAudioPlayerHandsOverToTrack(t *testing.T) {
	doc := js.Global().Get("document")
	var reports []driver.AudioEvent
	a := newWebAudio(doc, func(e driver.Event) { reports = append(reports, *e.Data.(*driver.AudioEvent)) }, false)
	defer a.close()
	const loopStart = 400 * time.Millisecond
	open := func() *webAudioPlayer {
		reports = nil
		a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: 1, LoopStart: loopStart,
			ProgressInterval: 20 * time.Millisecond, Source: driver.ImageSource{Data: audioWAV(700 * time.Millisecond)}})
		return a.players[1]
	}
	loop := &driver.AudioRequest{Command: driver.AudioSet, Volume: 1, Rate: 1, Loop: true}

	p := open()
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPreload})
	if p.loaded {
		t.Fatal("the element loaded before the test could turn the loop on")
	}
	a.handle(1, loop)
	audioWait(t, "the track decoded once the element loaded", func() bool { return p.track.Truthy() })
	a.handle(1, &driver.AudioRequest{Command: driver.AudioClose})

	p = open()
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPreload})
	audioWait(t, "the element to load", func() bool { return p.loaded })
	pressAudio(t, doc, a, func() { a.handle(1, &driver.AudioRequest{Command: driver.AudioResume}) })
	audioWait(t, "the element to play", func() bool { return p.playing })
	a.handle(1, loop)
	audioWait(t, "the decoded track", func() bool { return p.track.Truthy() })
	if !p.el.Truthy() || p.node.Truthy() {
		t.Fatal("the decoded track took over before the element's end")
	}
	audioWait(t, "two repeats", func() bool { return len(reports) > 0 && reports[len(reports)-1].Laps >= 2 })
	if p.el.Truthy() || !p.node.Truthy() {
		t.Fatal("the decoded track did not take over the loop")
	}
	if want := mediaTime(p.track.Get("duration")); p.duration != want {
		t.Fatalf("the player kept the element's duration %v, want the track's %v", p.duration, want)
	}
	lapped := false
	for _, e := range reports {
		if e.Phase == driver.AudioEnded || e.Phase == driver.AudioFailure {
			t.Fatalf("the handover reported %+v", e)
		}
		if e.Phase == driver.AudioProgress && e.Laps > 0 {
			lapped = true
		}
		if lapped && e.Phase == driver.AudioProgress && e.Position < loopStart-50*time.Millisecond {
			t.Fatalf("a repeat after the handover went back into the intro: %+v", e)
		}
	}
	a.handle(1, &driver.AudioRequest{Command: driver.AudioClose})
}

func TestWebAudioThroughApp(t *testing.T) {
	doc := js.Global().Get("document")
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-audio-test")
	setStyle(mount, map[string]string{"width": "200px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	var sound *core.Sound
	var playback *core.Playback
	var scope *core.Scope
	var loadErr, playErr error
	var loadedPlayer, played bool
	s := open(func(cx *core.Scope) core.Node {
		scope = cx
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
					first := sound.Play(core.SoundVoice{Muted: true})
					playback = sound.Play(core.SoundVoice{Muted: true})
					playback.SetRate(2)
					first.Stop()
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
	if playback == nil || playback.Ended() {
		t.Fatal("the app's independent sound playback did not stay active")
	}
	voice := s.audio.voices[0]
	if voice.gain.Get("gain").Get("value").Float() != 0 || voice.source.Get("playbackRate").Get("value").Float() != 2 {
		t.Fatal("the muted playback did not keep its live rate")
	}
	scope.Post(func() {
		playback.SetVolume(0.25)
		playback.SetRate(1)
	})
	audioWait(t, "live playback controls", func() bool {
		return voice.gain.Get("gain").Get("value").Float() == 0.25 && voice.source.Get("playbackRate").Get("value").Float() == 1
	})
	audioWait(t, "the app's playback completion", playback.Ended)
	if len(s.audio.voices) != 0 {
		t.Fatal("a completed playback kept its audio nodes")
	}
	a := s.audio
	js.Global().Call("dispatchEvent", js.Global().Get("PageTransitionEvent").New("pagehide"))
	if s.audio != nil || a.output.Truthy() || len(a.sounds) != 0 || len(a.voices) != 0 || len(a.players) != 0 {
		t.Fatal("leaving the page kept audio resources")
	}
}

// wavLevel is a stretch of a levelWAV: a level from -1 to 1 held for d.
type wavLevel struct {
	d     time.Duration
	level float64
}

// levelWAV is 16-bit mono PCM at rate holding each level in turn.
func levelWAV(rate int, levels ...wavLevel) []byte {
	n := 0
	for _, l := range levels {
		n += int(l.d.Seconds() * float64(rate))
	}
	data := make([]byte, 44+2*n)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], uint32(rate))
	binary.LittleEndian.PutUint32(data[28:], uint32(2*rate))
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*n))
	at := 44
	for _, l := range levels {
		for range int(l.d.Seconds() * float64(rate)) {
			binary.LittleEndian.PutUint16(data[at:], uint16(int16(l.level*32767)))
			at += 2
		}
	}
	return data
}

// TestWebAudioPlayerLoopsWithoutGap records a looping player's output: a 300 ms intro at one level, then a 300 ms body at another that repeats from the loop start, with no silence at the repeats and no intro after the first pass.
func TestWebAudioPlayerLoopsWithoutGap(t *testing.T) {
	doc := js.Global().Get("document")
	var reports []driver.AudioEvent
	a := newWebAudio(doc, func(e driver.Event) { reports = append(reports, *e.Data.(*driver.AudioEvent)) }, false)
	defer a.close()
	const loopStart = 300 * time.Millisecond
	track := levelWAV(48000, wavLevel{loopStart, 0.5}, wavLevel{300 * time.Millisecond, 0.25})
	a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: 1, Loop: true, LoopStart: loopStart,
		ProgressInterval: 20 * time.Millisecond, Source: driver.ImageSource{Data: track}})
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPreload})
	audioWait(t, "the decoded track", func() bool { return a.players[1].track.Truthy() })
	p := a.players[1]
	pressAudio(t, doc, a, func() { a.handle(1, &driver.AudioRequest{Command: driver.AudioResume}) })
	if !p.node.Truthy() {
		t.Fatal("the looping player did not play its decoded track")
	}
	if a.output.Get("createScriptProcessor").Type() != js.TypeFunction {
		t.Skip("the browser has no ScriptProcessorNode to record its output")
	}
	const block = 1024
	tap := a.output.Call("createScriptProcessor", block, 1, 1)
	tap.Call("connect", a.output.Get("destination"))
	defer tap.Call("disconnect")
	rate := a.output.Get("sampleRate").Float()
	// Each run is a stretch of contiguous blocks: a block the page did not process in time starts a new one.
	var runs [][]float32
	next := -1.0
	var data []byte
	process := js.FuncOf(func(_ js.Value, args []js.Value) any {
		e := args[0]
		at := e.Get("playbackTime").Float()
		samples := e.Get("inputBuffer").Call("getChannelData", 0)
		bytes := js.Global().Get("Uint8Array").New(samples.Get("buffer"), samples.Get("byteOffset"), samples.Get("byteLength"))
		data = slices.Grow(data[:0], bytes.Length())[:bytes.Length()]
		js.CopyBytesToGo(data, bytes)
		if len(runs) == 0 || math.Abs(at-next) > 0.5/rate {
			runs = append(runs, nil)
		}
		next = at + block/rate
		for i := 0; i+4 <= len(data); i += 4 {
			runs[len(runs)-1] = append(runs[len(runs)-1], math.Float32frombits(binary.LittleEndian.Uint32(data[i:])))
		}
		return nil
	})
	defer process.Release()
	tap.Set("onaudioprocess", process)
	p.gain.Call("connect", tap)
	audioWait(t, "four repeats", func() bool {
		return len(reports) > 0 && reports[len(reports)-1].Laps >= 4
	})
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPause})
	tap.Set("onaudioprocess", js.Null())

	// The longest silence between the first and the last sound of a run, and whether the intro's level came back after the body's.
	var longest, recorded int
	introAgain := false
	for _, run := range runs {
		first, last := -1, -1
		for i, v := range run {
			if math.Abs(float64(v)) > 0.05 {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			continue
		}
		recorded += last - first
		quiet, body := 0, 0
		for _, v := range run[first : last+1] {
			level := math.Abs(float64(v))
			switch {
			case level <= 0.05:
				quiet++
				longest = max(longest, quiet)
				continue
			case level < 0.35:
				body++
			case body > int(rate/100):
				// 10 ms of the body went by, so this is the intro again.
				introAgain = true
			}
			quiet = 0
		}
	}
	t.Logf("recorded %.0f ms of the loop, longest silence %.2f ms", float64(recorded)/rate*1000, float64(longest)/rate*1000)
	if float64(recorded) < 0.9*rate {
		t.Fatalf("recorded %d samples of the loop, want at least 0.9 s", recorded)
	}
	if gap := float64(longest) / rate; gap >= 0.005 {
		t.Fatalf("the loop fell silent for %.2f ms, want under 5 ms", gap*1000)
	}
	if introAgain {
		t.Fatal("a repeat played the intro again instead of starting at the loop start")
	}
}

// TestWebAudioPlayerLoopKeepsPitch plays a looping player at a rate other than 1 through the element, whose playbackRate keeps the pitch where a buffer source node's shifts it, and hands it back to the decoded track at rate 1.
func TestWebAudioPlayerLoopKeepsPitch(t *testing.T) {
	doc := js.Global().Get("document")
	var reports []driver.AudioEvent
	a := newWebAudio(doc, func(e driver.Event) { reports = append(reports, *e.Data.(*driver.AudioEvent)) }, false)
	defer a.close()
	set := func(rate float32) {
		a.handle(1, &driver.AudioRequest{Command: driver.AudioSet, Volume: 1, Rate: rate, Loop: true})
	}
	open := func(rate float32) *webAudioPlayer {
		reports = nil
		a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: rate, Loop: true,
			ProgressInterval: 20 * time.Millisecond, Source: driver.ImageSource{Data: audioWAV(400 * time.Millisecond)}})
		return a.players[1]
	}
	keepsPitch := func(p *webAudioPlayer, rate float64) {
		t.Helper()
		if p.node.Truthy() || !p.el.Truthy() {
			t.Fatalf("a looping player at %v played from a buffer source node, which shifts the pitch", rate)
		}
		if got := p.el.Get("playbackRate").Float(); math.Abs(got-rate) > 1e-6 {
			t.Fatalf("the element plays at %v, want %v", got, rate)
		}
		if p.el.Get("preservesPitch").Type() == js.TypeBoolean && !p.el.Get("preservesPitch").Bool() {
			t.Fatal("the element does not keep the pitch")
		}
	}

	// A player opened at 1.5 plays through the element though its track decoded.
	p := open(1.5)
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPreload})
	audioWait(t, "the decoded track", func() bool { return p.track.Truthy() })
	pressAudio(t, doc, a, func() { a.handle(1, &driver.AudioRequest{Command: driver.AudioResume}) })
	audioWait(t, "the element to play", func() bool { return p.playing })
	keepsPitch(p, 1.5)
	// Back at 1 the decoded track takes over at the element's end.
	set(1)
	audioWait(t, "the decoded track to take over", func() bool { return p.node.Truthy() })
	if p.el.Truthy() {
		t.Fatal("the element kept playing beside the decoded track")
	}
	// Away from 1 again, the element takes over from the node's position.
	set(0.75)
	if p.node.Truthy() || !p.el.Truthy() {
		t.Fatal("the node kept playing at 0.75")
	}
	audioWait(t, "the element to play again", func() bool { return p.playing })
	keepsPitch(p, 0.75)
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPause})
	for _, e := range reports {
		if e.Phase == driver.AudioEnded || e.Phase == driver.AudioFailure {
			t.Fatalf("changing the rate reported %+v", e)
		}
	}
	a.handle(1, &driver.AudioRequest{Command: driver.AudioClose})

	// A player paused on its track resumes at 1.5 through the element.
	p = open(1)
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPreload})
	audioWait(t, "the decoded track", func() bool { return p.track.Truthy() && p.loaded })
	set(1.5)
	a.handle(1, &driver.AudioRequest{Command: driver.AudioResume})
	audioWait(t, "the element to play", func() bool { return p.playing })
	keepsPitch(p, 1.5)
	a.handle(1, &driver.AudioRequest{Command: driver.AudioClose})
}

// TestWebAudioPlayerTrackLengthTimesOut loops a track whose metadata does not load in time through the element instead of waiting for it.
func TestWebAudioPlayerTrackLengthTimesOut(t *testing.T) {
	defer func(wait time.Duration) { trackLengthWait = wait }(trackLengthWait)
	trackLengthWait = time.Nanosecond
	doc := js.Global().Get("document")
	var reports []driver.AudioEvent
	a := newWebAudio(doc, func(e driver.Event) { reports = append(reports, *e.Data.(*driver.AudioEvent)) }, false)
	defer a.close()
	a.handle(1, &driver.AudioRequest{Command: driver.AudioOpen, Volume: 1, Rate: 1, Loop: true,
		Source: driver.ImageSource{Data: audioWAV(400 * time.Millisecond)}})
	p := a.players[1]
	a.handle(1, &driver.AudioRequest{Command: driver.AudioPreload})
	audioWait(t, "the element to load", func() bool { return p.loaded })
	if !p.noTrack || p.track.Truthy() || p.decode != nil || !p.el.Truthy() {
		t.Fatal("a track whose length did not load in time did not loop through the element")
	}
	a.handle(1, &driver.AudioRequest{Command: driver.AudioClose})
}
