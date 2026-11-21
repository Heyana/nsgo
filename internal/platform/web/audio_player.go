//go:build js && wasm

package web

import (
	"math"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webAudioPlayer streams in a browser audio element. Its generation guards late play-promise replies after a pause or a fresh element.
type webAudioPlayer struct {
	audio                                                   *webAudio
	id                                                      uint32
	source                                                  driver.ImageSource
	el                                                      js.Value
	objectURL                                               string
	listeners                                               listeners
	timer                                                   js.Value
	tick                                                    js.Func
	volume, rate                                            float32
	interval                                                time.Duration
	position, duration                                      time.Duration
	seeks, laps                                             uint32
	generation                                              uint64
	loop, loaded, wanted, playing, buffering, ended, failed bool
}

func (p *webAudioPlayer) apply(r *driver.AudioRequest) {
	switch r.Command {
	case driver.AudioPreload:
		p.preload()
	case driver.AudioResume:
		p.resume()
	case driver.AudioPause:
		p.pause()
	case driver.AudioSeek:
		p.seek(r.Position)
	case driver.AudioSet:
		p.volume, p.rate, p.loop = r.Volume, r.Rate, r.Loop
		p.settings()
	}
}

func (p *webAudioPlayer) preload() bool {
	if p.el.Truthy() && !p.failed {
		return true
	}
	p.release()
	p.failed = false
	url := p.audio.sourceURL(p.source.URI)
	if len(p.source.Data) > 0 {
		g := js.Global()
		data := audioBytes(p.source.Data)
		blob := g.Get("Blob").New([]any{data}, map[string]any{"type": driver.AudioType(p.source.Data)})
		url = g.Get("URL").Call("createObjectURL", blob).String()
		p.objectURL = url
	}
	if url == "" {
		p.fail(driver.AudioNotSupported, "the browser cannot read this audio source")
		return false
	}
	el := p.audio.doc.Call("createElement", "audio")
	p.el = el
	el.Set("preload", "auto")
	p.settings()
	p.tick = js.FuncOf(func(js.Value, []js.Value) any {
		p.progress()
		return nil
	})
	p.listen("loadedmetadata", func() {
		p.loaded = true
		p.duration = mediaTime(el.Get("duration"))
		if p.position > 0 || p.seeks != 0 {
			p.applyPosition(p.position)
		}
		p.report(driver.AudioOpened)
	})
	p.listen("durationchange", func() { p.duration = mediaTime(el.Get("duration")) })
	p.listen("canplay", func() { p.buffer(false) })
	p.listen("waiting", func() { p.buffer(p.wanted) })
	p.listen("playing", func() {
		p.buffer(false)
		if p.wanted && !p.timer.Truthy() {
			p.timer = js.Global().Call("setInterval", p.tick, max(16, p.interval.Milliseconds()))
		}
	})
	p.listen("pause", func() {
		if p.wanted && el.Get("paused").Bool() && !el.Get("ended").Bool() {
			p.paused()
		}
	})
	p.listen("seeked", p.progress)
	p.listen("ended", func() {
		if !el.Get("ended").Bool() || p.loop {
			return
		}
		p.stopTimer()
		p.wanted, p.playing, p.ended = false, false, true
		p.position = mediaTime(el.Get("currentTime"))
		p.report(driver.AudioEnded)
	})
	p.listen("error", func() {
		e := el.Get("error")
		if !e.Truthy() {
			return
		}
		code := driver.AudioFailed
		switch e.Get("code").Int() {
		case 2: // MEDIA_ERR_NETWORK
			code = driver.AudioNetwork
		case 3, 4: // MEDIA_ERR_DECODE, MEDIA_ERR_SRC_NOT_SUPPORTED
			code = driver.AudioDecode
		}
		p.fail(code, e.Get("message").String())
	})
	el.Set("src", url)
	return true
}

func (p *webAudioPlayer) listen(name string, fn func()) {
	p.listeners.on(p.el, name, func(js.Value) {
		if !p.failed {
			fn()
		}
	})
}

func (p *webAudioPlayer) settings() {
	if p.el.Truthy() {
		p.el.Set("volume", p.volume)
		p.el.Set("playbackRate", p.rate)
		p.el.Set("loop", p.loop)
	}
}

func (p *webAudioPlayer) resume() {
	if !p.preload() {
		return
	}
	if p.ended {
		p.ended = false
		p.position = 0
		p.el.Set("currentTime", 0)
	}
	p.wanted = true
	p.generation++
	run := p.generation
	current := func() bool { return run == p.generation && p.wanted }
	then(p.el.Call("play"), func(js.Value) {
		if current() {
			p.playing = true
			p.report(driver.AudioPlaying)
		}
	}, func(e js.Value) {
		if current() {
			code := driver.AudioFailed
			switch errorName(e) {
			case "NotAllowedError":
				code = driver.AudioNotAllowed
			case "NotSupportedError":
				code = driver.AudioDecode
			}
			p.fail(code, jsError(e).Error())
		}
	})
}

func (p *webAudioPlayer) pause() {
	if p.failed || p.ended {
		return
	}
	p.paused()
	if p.el.Truthy() {
		p.el.Call("pause")
	}
}

func (p *webAudioPlayer) paused() {
	p.generation++
	p.readPosition()
	p.wanted, p.playing = false, false
	p.stopTimer()
	p.buffer(false)
	p.report(driver.AudioPaused)
}

func (p *webAudioPlayer) seek(position time.Duration) {
	p.seeks++
	p.ended = false
	p.position = position
	if !p.loaded {
		return
	}
	p.applyPosition(position)
	p.report(driver.AudioProgress)
}

func (p *webAudioPlayer) applyPosition(position time.Duration) {
	if p.duration > 0 {
		position = min(position, p.duration)
	}
	p.position = position
	p.el.Set("currentTime", position.Seconds())
}

func (p *webAudioPlayer) progress() {
	if !p.loaded || p.el.Get("seeking").Bool() {
		return
	}
	p.readPosition()
	p.report(driver.AudioProgress)
}

func (p *webAudioPlayer) readPosition() {
	if !p.loaded || p.el.Get("seeking").Bool() {
		return
	}
	position := mediaTime(p.el.Get("currentTime"))
	if p.loop && p.playing && position < p.position {
		p.laps++
	}
	p.position = position
}

func (p *webAudioPlayer) buffer(on bool) {
	if p.buffering != on {
		p.buffering = on
		p.report(driver.AudioBuffer)
	}
}

func (p *webAudioPlayer) report(phase driver.AudioPhase) {
	p.audio.report(p.id, driver.AudioEvent{Phase: phase, Duration: p.duration, Position: p.position,
		Seeks: p.seeks, Laps: p.laps, Buffering: p.buffering})
}

func (p *webAudioPlayer) fail(code, message string) {
	p.release()
	p.failed = true
	p.audio.fail(p.id, code, message)
}

func (p *webAudioPlayer) stopTimer() {
	if p.timer.Truthy() {
		js.Global().Call("clearInterval", p.timer)
		p.timer = js.Undefined()
	}
}

func (p *webAudioPlayer) release() {
	p.generation++
	p.wanted, p.playing, p.loaded, p.buffering, p.ended = false, false, false, false, false
	if !p.el.Truthy() {
		return
	}
	p.stopTimer()
	p.tick.Release()
	p.listeners.release()
	p.el.Call("pause")
	p.el.Call("removeAttribute", "src")
	p.el.Call("load")
	p.el = js.Undefined()
	if p.objectURL != "" {
		js.Global().Get("URL").Call("revokeObjectURL", p.objectURL)
		p.objectURL = ""
	}
}

func mediaTime(value js.Value) time.Duration {
	seconds := value.Float()
	if math.IsNaN(seconds) || seconds < 0 || seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}
