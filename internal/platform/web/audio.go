//go:build js && wasm

package web

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webAudio owns one engine's decoded sounds, voices and streamed players. The browser decodes sounds on its audio threads. An offline context can decode before a gesture and never opens the output.
type webAudio struct {
	doc             js.Value
	dispatch        func(driver.Event)
	activated       bool
	output, decoder js.Value
	sounds          map[uint32]*webSound
	voices          []*webSoundVoice
	players         map[uint32]*webAudioPlayer
	// clockGo and clockAt anchor the output context's currentTime to the Go clock, so that voices scheduled in different turns keep their spacing, which currentTime alone quantizes to its render callbacks.
	clockGo time.Time
	clockAt float64
}

type webSound struct {
	buffer js.Value
	cancel context.CancelFunc
}

type webSoundVoice struct {
	sound, playback      uint32
	source, gain, panner js.Value
	ended                js.Func
	// restricted marks a playback of a scope held to core.Access, which counts against driver.MaxSoundVoices among the restricted voices whatever the engine's limit.
	restricted bool
}

func webAudioAvailable() bool {
	g := js.Global()
	return g.Get("AudioContext").Type() == js.TypeFunction &&
		g.Get("OfflineAudioContext").Type() == js.TypeFunction
}

func (s *surface) activateAudio(event js.Value) {
	if !event.Get("isTrusted").Bool() {
		return
	}
	if activation := js.Global().Get("navigator").Get("userActivation"); activation.Truthy() && !activation.Get("isActive").Bool() {
		return
	}
	s.audioGesture = true
	if s.audio != nil {
		s.audio.activate()
	}
}

func (s *surface) closeAudio() {
	if s.audio != nil {
		s.audio.close()
		s.audio = nil
	}
}

func newWebAudio(doc js.Value, dispatch func(driver.Event), activated bool) *webAudio {
	return &webAudio{doc: doc, dispatch: dispatch, activated: activated,
		sounds: map[uint32]*webSound{}, players: map[uint32]*webAudioPlayer{}}
}

func (a *webAudio) report(id uint32, e driver.AudioEvent) {
	a.dispatch(driver.Event{Kind: driver.EventAudio, Count: id, Data: &e})
}

func (a *webAudio) fail(id uint32, code, message string) {
	a.report(id, driver.AudioEvent{Phase: driver.AudioFailure, Error: code, Message: message})
}

func (a *webAudio) handle(id uint32, r *driver.AudioRequest) {
	switch r.Command {
	case driver.AudioLoad:
		a.load(id, r.Source, r.MaxBytes, r.MaxDecodedBytes, r.MaxDuration)
	case driver.AudioPlay:
		a.play(id, r)
	case driver.AudioPlaybackSet, driver.AudioPlaybackStop:
		a.controlPlayback(id, r)
	case driver.AudioReadLatency:
		a.report(id, driver.AudioEvent{Phase: driver.AudioLatency, Latency: a.outputLatency()})
	case driver.AudioStop:
		a.stop(id)
	case driver.AudioUnload:
		a.unload(id)
	case driver.AudioOpen:
		a.closePlayer(id)
		a.players[id] = &webAudioPlayer{audio: a, id: id, source: r.Source,
			volume: r.Volume, rate: r.Rate, loop: r.Loop, loopStart: r.LoopStart, interval: r.ProgressInterval, position: r.Position}
	case driver.AudioClose:
		a.closePlayer(id)
	default:
		if p := a.players[id]; p != nil {
			p.apply(r)
		}
	}
}

// activate runs in a trusted browser event. It records input before the first sound too, so an app can load its first sound after the gesture.
func (a *webAudio) activate() {
	a.activated = true
	if len(a.sounds) > 0 {
		a.resumeOutput()
	}
}

func (a *webAudio) resumeOutput() {
	// The browser can refuse an output device. A sound has no play callback, so it remains silent in that case.
	defer func() { _ = recover() }()
	if !a.activated {
		return
	}
	if !a.output.Truthy() {
		a.output = js.Global().Get("AudioContext").New()
	}
	if a.output.Get("state").String() != "running" {
		then(a.output.Call("resume"), func(js.Value) {}, func(js.Value) {
			for len(a.voices) > 0 {
				a.endVoice(a.voices[0], true)
			}
		})
	}
}

func (a *webAudio) load(id uint32, source driver.ImageSource, bytes, decodedBytes int64, duration time.Duration) {
	a.unload(id)
	ctx, cancel := context.WithCancel(context.Background())
	sound := &webSound{cancel: cancel}
	a.sounds[id] = sound
	a.resumeOutput()
	go func() {
		defer cancel()
		data, code, message := a.soundBytes(ctx, source, bytes)
		if a.sounds[id] != sound {
			return
		}
		if code != "" {
			delete(a.sounds, id)
			a.fail(id, code, message)
			return
		}
		if !a.decoder.Truthy() {
			a.decoder = js.Global().Get("OfflineAudioContext").New(1, 1, 48000)
		}
		buffer, err := await(a.decoder.Call("decodeAudioData", audioBytes(data).Get("buffer")))
		if a.sounds[id] != sound {
			return
		}
		if err != nil {
			delete(a.sounds, id)
			a.fail(id, driver.AudioDecode, err.Error())
			return
		}
		length := mediaTime(buffer.Get("duration"))
		// The browser keeps 32-bit float samples per channel.
		size := int64(buffer.Get("length").Int()) * int64(buffer.Get("numberOfChannels").Int()) * 4
		if size > cmp.Or(decodedBytes, driver.MaxDecodedBytes) {
			delete(a.sounds, id)
			a.fail(id, driver.AudioQuotaExceeded, "the sound exceeds its decoded size limit")
			return
		}
		cut := length > duration
		if !cut {
			sound.buffer = buffer
		}
		a.report(id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: length, Cut: cut, Bytes: size})
	}()
}

func (a *webAudio) soundBytes(ctx context.Context, source driver.ImageSource, limit int64) ([]byte, string, string) {
	tooLarge := fmt.Sprintf("the sound exceeds its limit of %d encoded bytes", limit)
	if len(source.Data) > 0 {
		if int64(len(source.Data)) > limit {
			return nil, driver.AudioQuotaExceeded, tooLarge
		}
		return source.Data, "", ""
	}
	url := a.sourceURL(source.URI)
	if url == "" {
		return nil, driver.AudioNotSupported, "the browser cannot read this audio source"
	}
	// Use the browser's Fetch stream directly. Pulling in net/http for this reader would add its client and TLS dependencies to every web app, including those that never load a sound.
	g := js.Global()
	controller := g.Get("AbortController").New()
	stop := context.AfterFunc(ctx, func() { controller.Call("abort") })
	defer stop()
	res, err := await(g.Call("fetch", url, map[string]any{"signal": controller.Get("signal")}))
	if err != nil {
		return nil, driver.AudioNetwork, err.Error()
	}
	if !res.Get("ok").Bool() {
		controller.Call("abort")
		return nil, driver.AudioNetwork, "HTTP " + strconv.Itoa(res.Get("status").Int())
	}
	if n, err := strconv.ParseInt(res.Get("headers").Call("get", "content-length").String(), 10, 64); err == nil && n > limit {
		controller.Call("abort")
		return nil, driver.AudioQuotaExceeded, tooLarge
	}
	var data []byte
	if body := res.Get("body"); body.Truthy() {
		reader := body.Call("getReader")
		defer reader.Call("releaseLock")
		for {
			part, err := await(reader.Call("read"))
			if err != nil {
				return nil, driver.AudioNetwork, err.Error()
			}
			if part.Get("done").Bool() {
				break
			}
			chunk := part.Get("value")
			if int64(chunk.Length()) > limit-int64(len(data)) {
				reader.Call("cancel").Call("catch", ignore)
				return nil, driver.AudioQuotaExceeded, tooLarge
			}
			at := len(data)
			data = slices.Grow(data, chunk.Length())[:at+chunk.Length()]
			js.CopyBytesToGo(data[at:], chunk)
		}
	}
	return data, "", ""
}

func (a *webAudio) sourceURL(uri string) (resolved string) {
	if uri == "" {
		return ""
	}
	if path, ok := strings.CutPrefix(uri, "asset:"); ok {
		path = strings.TrimPrefix(path, "/")
		decoded, err := url.PathUnescape(path)
		if err != nil || decoded == "." || !fs.ValidPath(decoded) || strings.ContainsAny(decoded, ":\\\x00\r\n\t?#") {
			return ""
		}
		uri = resourceURL(path)
	}
	// The URL constructor throws for malformed input.
	defer func() { _ = recover() }()
	u := js.Global().Get("URL").New(uri, a.doc.Get("baseURI"))
	switch u.Get("protocol").String() {
	case "http:", "https:", "blob:", "data:":
		return u.Get("href").String()
	}
	return ""
}

func audioBytes(data []byte) js.Value {
	bytes := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(bytes, data)
	return bytes
}

// play starts a playback of sound id. At the request's playback limit the oldest voice stops; a restricted playback past driver.MaxSoundVoices restricted voices stops the oldest of those.
func (a *webAudio) play(id uint32, r *driver.AudioRequest) {
	sound := a.sounds[id]
	if sound == nil || !sound.buffer.Truthy() || !a.activated {
		a.playbackEnded(id, r.Playback)
		return
	}
	a.resumeOutput()
	if !a.output.Truthy() {
		a.playbackEnded(id, r.Playback)
		return
	}
	if r.Restricted {
		oldest, n := -1, 0
		for i, v := range a.voices {
			if v.restricted {
				if n == 0 {
					oldest = i
				}
				n++
			}
		}
		if n >= driver.MaxSoundVoices {
			a.endVoice(a.voices[oldest], true)
		}
	}
	// A limit lowered while voices play stops the oldest until the new playback fits.
	for len(a.voices) >= max(1, cmp.Or(r.MaxPlaybacks, driver.MaxSoundVoices)) {
		a.endVoice(a.voices[0], true)
	}
	source, gain := a.output.Call("createBufferSource"), a.output.Call("createGain")
	source.Set("buffer", sound.buffer)
	// A looping source repeats the whole decoded buffer and fires onended only once stopped.
	source.Set("loop", r.Loop)
	source.Get("playbackRate").Set("value", r.Rate)
	gain.Get("gain").Set("value", r.Volume)
	source.Call("connect", gain)
	voice := &webSoundVoice{sound: id, playback: r.Playback, source: source, gain: gain, restricted: r.Restricted}
	// A centered voice skips the panner; browsers without StereoPannerNode play every voice centered.
	if r.Pan != 0 && a.output.Get("createStereoPanner").Type() == js.TypeFunction {
		voice.panner = a.output.Call("createStereoPanner")
		voice.panner.Get("pan").Set("value", r.Pan)
		gain.Call("connect", voice.panner)
		voice.panner.Call("connect", a.output.Get("destination"))
	} else {
		gain.Call("connect", a.output.Get("destination"))
	}
	voice.ended = js.FuncOf(func(js.Value, []js.Value) any {
		a.endVoice(voice, false)
		return nil
	})
	source.Set("onended", voice.ended)
	a.voices = append(a.voices, voice)
	if !r.StartAt.IsZero() && time.Until(r.StartAt) > 0 {
		source.Call("start", a.contextTime(r.StartAt))
	} else {
		source.Call("start")
	}
}

// contextTime is the output context's time for t on the Go clock. The anchor moves when currentTime strays from it by more than a render callback, as when the context was suspended or its clock drifted.
func (a *webAudio) contextTime(t time.Time) float64 {
	now, current := time.Now(), a.output.Get("currentTime").Float()
	if a.clockGo.IsZero() || math.Abs(a.clockAt+now.Sub(a.clockGo).Seconds()-current) > 0.05 {
		a.clockGo, a.clockAt = now, current
	}
	return a.clockAt + t.Sub(a.clockGo).Seconds()
}

// outputLatency is the running output's base and output latency, zero when there is no running output or the browser does not report AudioContext.outputLatency.
func (a *webAudio) outputLatency() time.Duration {
	if !a.output.Truthy() || a.output.Get("state").String() != "running" {
		return 0
	}
	out := a.output.Get("outputLatency")
	if out.Type() != js.TypeNumber || !(out.Float() > 0) {
		return 0
	}
	seconds := out.Float()
	if base := a.output.Get("baseLatency"); base.Type() == js.TypeNumber && base.Float() > 0 {
		seconds += base.Float()
	}
	if seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func (a *webAudio) controlPlayback(id uint32, r *driver.AudioRequest) {
	if r.Playback == 0 {
		return
	}
	for _, voice := range a.voices {
		if voice.sound == id && voice.playback == r.Playback {
			if r.Command == driver.AudioPlaybackStop {
				a.endVoice(voice, true)
			} else {
				voice.gain.Get("gain").Set("value", r.Volume)
				voice.source.Get("playbackRate").Set("value", r.Rate)
			}
			return
		}
	}
}

func (a *webAudio) playbackEnded(sound, playback uint32) {
	if playback != 0 {
		a.report(sound, driver.AudioEvent{Phase: driver.AudioPlaybackEnded, Playback: playback})
	}
}

func (a *webAudio) endVoice(voice *webSoundVoice, stop bool) {
	i := slices.Index(a.voices, voice)
	if i < 0 {
		return
	}
	a.voices = slices.Delete(a.voices, i, i+1)
	voice.source.Set("onended", js.Null())
	voice.ended.Release()
	if stop {
		voice.source.Call("stop")
	}
	voice.source.Call("disconnect")
	voice.gain.Call("disconnect")
	if voice.panner.Truthy() {
		voice.panner.Call("disconnect")
	}
	a.playbackEnded(voice.sound, voice.playback)
}

func (a *webAudio) stop(id uint32) {
	for i := len(a.voices) - 1; i >= 0; i-- {
		if a.voices[i].sound == id {
			a.endVoice(a.voices[i], true)
		}
	}
}

func (a *webAudio) unload(id uint32) {
	a.stop(id)
	if sound := a.sounds[id]; sound != nil {
		sound.cancel()
		delete(a.sounds, id)
	}
}

func (a *webAudio) closePlayer(id uint32) {
	if p := a.players[id]; p != nil {
		delete(a.players, id)
		p.release()
	}
}

func (a *webAudio) close() {
	for id := range a.players {
		a.closePlayer(id)
	}
	for id := range a.sounds {
		a.unload(id)
	}
	if a.output.Truthy() {
		a.output.Call("close").Call("catch", ignore)
	}
	a.output, a.decoder = js.Undefined(), js.Undefined()
	a.clockGo = time.Time{}
}
