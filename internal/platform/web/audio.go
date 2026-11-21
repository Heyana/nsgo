//go:build js && wasm

package web

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webAudio owns one engine's decoded sounds, voices and streamed players.
// The browser decodes sounds on its audio threads. An offline context can
// decode before a gesture and never opens the output.
type webAudio struct {
	doc             js.Value
	dispatch        func(driver.Event)
	activated       bool
	output, decoder js.Value
	sounds          map[uint32]*webSound
	voices          []*webSoundVoice
	players         map[uint32]*webAudioPlayer
}

type webSound struct {
	buffer js.Value
	cancel context.CancelFunc
}

type webSoundVoice struct {
	sound        uint32
	source, gain js.Value
	ended        js.Func
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
		a.load(id, r.Source, r.MaxBytes, r.MaxDuration)
	case driver.AudioPlay:
		a.play(id, r.Volume, r.Rate)
	case driver.AudioStop:
		a.stop(id)
	case driver.AudioUnload:
		a.unload(id)
	case driver.AudioOpen:
		a.closePlayer(id)
		a.players[id] = &webAudioPlayer{audio: a, id: id, source: r.Source,
			volume: r.Volume, rate: r.Rate, loop: r.Loop, interval: r.ProgressInterval, position: r.Position}
	case driver.AudioClose:
		a.closePlayer(id)
	default:
		if p := a.players[id]; p != nil {
			p.apply(r)
		}
	}
}

// activate runs in a trusted browser event. It records input before the
// first sound too, so an app can load its first sound after the gesture.
func (a *webAudio) activate() {
	a.activated = true
	if len(a.sounds) > 0 {
		a.resumeOutput()
	}
}

func (a *webAudio) resumeOutput() {
	// The browser can refuse an output device. A sound has no play
	// callback, so it remains silent in that case.
	defer func() { _ = recover() }()
	if !a.activated {
		return
	}
	if !a.output.Truthy() {
		a.output = js.Global().Get("AudioContext").New()
	}
	if a.output.Get("state").String() != "running" {
		a.output.Call("resume").Call("catch", ignore)
	}
}

func (a *webAudio) load(id uint32, source driver.ImageSource, bytes int64, duration time.Duration) {
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
		cut := length > duration
		if !cut {
			sound.buffer = buffer
		}
		a.report(id, driver.AudioEvent{Phase: driver.AudioLoaded, Duration: length, Cut: cut,
			Bytes: int64(buffer.Get("length").Int()) * int64(buffer.Get("numberOfChannels").Int()) * 4})
	}()
}

func (a *webAudio) soundBytes(ctx context.Context, source driver.ImageSource, limit int64) ([]byte, string, string) {
	const tooLarge = "a sound has at most 8 MiB"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, driver.AudioNotSupported, err.Error()
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, driver.AudioNetwork, err.Error()
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, driver.AudioNetwork, fmt.Sprintf("HTTP %d", res.StatusCode)
	}
	if res.ContentLength > limit {
		return nil, driver.AudioQuotaExceeded, tooLarge
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if int64(len(data)) > limit {
		return nil, driver.AudioQuotaExceeded, tooLarge
	}
	if err != nil {
		return nil, driver.AudioNetwork, err.Error()
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

func (a *webAudio) play(id uint32, volume, rate float32) {
	sound := a.sounds[id]
	if sound == nil || !sound.buffer.Truthy() || !a.activated {
		return
	}
	a.resumeOutput()
	if !a.output.Truthy() {
		return
	}
	if len(a.voices) >= driver.MaxSoundVoices {
		a.endVoice(a.voices[0], true)
	}
	source, gain := a.output.Call("createBufferSource"), a.output.Call("createGain")
	source.Set("buffer", sound.buffer)
	source.Get("playbackRate").Set("value", rate)
	gain.Get("gain").Set("value", volume)
	source.Call("connect", gain)
	gain.Call("connect", a.output.Get("destination"))
	voice := &webSoundVoice{sound: id, source: source, gain: gain}
	voice.ended = js.FuncOf(func(js.Value, []js.Value) any {
		a.endVoice(voice, false)
		return nil
	})
	source.Set("onended", voice.ended)
	a.voices = append(a.voices, voice)
	source.Call("start")
}

func (a *webAudio) endVoice(voice *webSoundVoice, stop bool) {
	i := slices.Index(a.voices, voice)
	a.voices = slices.Delete(a.voices, i, i+1)
	voice.source.Set("onended", js.Null())
	voice.ended.Release()
	if stop {
		voice.source.Call("stop")
	}
	voice.source.Call("disconnect")
	voice.gain.Call("disconnect")
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
}
