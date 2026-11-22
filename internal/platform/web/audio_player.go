//go:build js && wasm

package web

import (
	"context"
	"math"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webAudioPlayer streams in a browser audio element. Its generation guards late play-promise replies after a pause or a fresh element.
//
// A looping player decodes its whole track into an AudioBuffer, within driver.MaxGaplessLoopBytes and driver.MaxGaplessLoopDuration, and plays it from an AudioBufferSourceNode, whose loop repeats from the loop start without a gap; once it has the track it plays every pass from it at rate 1. A node's playbackRate shifts the pitch, while the element's keeps it, so at any other rate the element plays, as does a track it cannot decode whole, with the gap of a seek at each repeat.
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
	loopStart                                               time.Duration
	seeks, laps                                             uint32
	generation                                              uint64
	loop, loaded, wanted, playing, buffering, ended, failed bool

	// track is the decoded track, node and gain play it, and decode cancels its decoding. noTrack says the track could not be decoded whole, so the element plays it.
	track, node, gain js.Value
	nodeEnded         js.Func
	decode            context.CancelFunc
	noTrack, opened   bool
	// node plays on from position from at the output context's time at, after lapsAt laps.
	at     float64
	from   time.Duration
	lapsAt uint32
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
		// A playing node's position is anchored before its rate or loop changes.
		if p.node.Truthy() {
			p.readPosition()
		}
		p.volume, p.rate, p.loop = r.Volume, r.Rate, r.Loop
		if p.rate != 1 && !p.el.Truthy() && (p.node.Truthy() || p.wanted) {
			p.leaveTrack()
			return
		}
		p.settings()
	}
}

// trackPlays says the decoded track plays in place of the element: at rate 1 only, since a node's playbackRate shifts the pitch.
func (p *webAudioPlayer) trackPlays() bool {
	return p.track.Truthy() && p.rate == 1
}

// leaveTrack hands a player that plays, or waits to play, from its track over to the element when its rate moves away from 1, so that the pitch stays.
func (p *webAudioPlayer) leaveTrack() {
	wanted := p.wanted
	p.stopNode()
	p.stopTimer()
	p.buffer(false)
	p.wanted, p.playing = false, false
	if wanted {
		p.resume()
	}
}

// fromTrack says the player plays from its decoded track, or waits for the track it decodes instead of loading the element. At a rate other than 1 the element plays.
func (p *webAudioPlayer) fromTrack() bool {
	return p.rate == 1 && (p.track.Truthy() || !p.el.Truthy() && (p.decode != nil || p.loop && !p.noTrack))
}

func (p *webAudioPlayer) preload() bool {
	if p.trackPlays() {
		p.loaded = true
		return true
	}
	if p.fromTrack() {
		p.decodeTrack()
		return true
	}
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
	p.listen("loadedmetadata", func() {
		p.loaded = true
		p.duration = mediaTime(el.Get("duration"))
		p.settings()
		if p.position > 0 || p.seeks != 0 {
			p.applyPosition(p.position)
		}
		p.opened = true
		p.report(driver.AudioOpened)
	})
	p.listen("durationchange", func() {
		p.duration = mediaTime(el.Get("duration"))
		p.settings()
	})
	p.listen("canplay", func() { p.buffer(false) })
	p.listen("waiting", func() { p.buffer(p.wanted) })
	p.listen("playing", func() {
		p.buffer(false)
		if p.wanted {
			p.startTimer()
		}
	})
	p.listen("pause", func() {
		if p.wanted && el.Get("paused").Bool() && !el.Get("ended").Bool() {
			p.paused()
		}
	})
	p.listen("seeked", p.progress)
	p.listen("ended", func() {
		if !el.Get("ended").Bool() {
			return
		}
		if p.loop {
			if p.trackPlays() && p.output() {
				// The track decoded while the element played: it takes over at the loop start.
				p.laps++
				p.position = p.loopFrom()
				p.releaseElement()
				p.startNode()
				p.report(driver.AudioProgress)
				return
			}
			p.loopAgain()
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

// output opens the engine's output context for the track, which needs an earlier trusted gesture.
func (p *webAudioPlayer) output() bool {
	p.audio.resumeOutput()
	return p.audio.output.Truthy()
}

// decodeTrack decodes the player's whole track for a gapless loop, unless it has it, is decoding it or could not. An element that plays meanwhile plays on.
func (p *webAudioPlayer) decodeTrack() {
	if p.track.Truthy() || p.decode != nil || p.noTrack {
		return
	}
	if p.el.Truthy() && p.loaded && p.duration > driver.MaxGaplessLoopDuration {
		p.noTrack = true
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.decode = cancel
	go func() {
		defer cancel()
		track := p.audio.decodeTrack(ctx, p.source)
		if ctx.Err() != nil {
			return
		}
		p.decode = nil
		if !track.Truthy() {
			p.noTrack = true
			p.trackFailed()
			return
		}
		p.adopt(track)
	}()
}

// trackFailed loads the element in place of a track that could not be decoded whole, and plays it if the player was asked to play.
func (p *webAudioPlayer) trackFailed() {
	if p.el.Truthy() {
		return
	}
	if p.wanted {
		p.wanted = false
		p.resume()
		return
	}
	p.preload()
}

// adopt takes the decoded track. An element that plays hands over to it at its end, the next loop start, and one at a rate other than 1 keeps playing; otherwise the track takes over at once from the element's position.
func (p *webAudioPlayer) adopt(track js.Value) {
	p.track = track
	if p.el.Truthy() && (p.wanted || p.rate != 1) {
		if p.wanted {
			p.output()
		}
		p.settings()
		return
	}
	if p.el.Truthy() {
		p.readPosition()
		p.releaseElement()
	}
	p.loaded, p.failed = true, false
	p.duration = mediaTime(track.Get("duration"))
	p.position = min(p.position, p.duration)
	if !p.opened {
		p.opened = true
		p.report(driver.AudioOpened)
	}
	if p.wanted {
		if p.rate != 1 {
			p.leaveTrack()
			return
		}
		if !p.output() {
			p.fail(driver.AudioUnavailable, "the browser did not open an audio output")
			return
		}
		p.startNode()
	}
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
		// The element's own loop is seamless but always restarts at 0, so a loop that starts later restarts from the ended event, as does one the decoded track takes over at the end.
		p.el.Set("loop", p.loop && p.loopFrom() == 0 && !p.trackPlays())
		if p.loop && p.loaded {
			p.decodeTrack()
		}
	}
	if p.node.Truthy() {
		p.gain.Get("gain").Set("value", p.volume)
		p.node.Get("playbackRate").Set("value", p.rate)
		// The node repeats from the loop start to the end of the track.
		p.node.Set("loop", p.loop)
		p.node.Set("loopStart", p.loopFrom().Seconds())
		p.node.Set("loopEnd", p.track.Get("duration"))
	}
}

// loopFrom is where a looping player starts over: LoopStart inside a known duration, else 0.
func (p *webAudioPlayer) loopFrom() time.Duration {
	return driver.AudioLoopStart(p.loopStart, p.duration)
}

// loopAgain starts a looping player over at its loop start after the element ended, counting the lap. play goes first, since on an ended element it seeks to 0, which the later seek replaces.
func (p *webAudioPlayer) loopAgain() {
	start := p.loopFrom()
	p.laps++
	p.position = start
	if p.wanted {
		run := p.generation
		then(p.el.Call("play"), func(js.Value) {}, func(e js.Value) {
			if run == p.generation && p.wanted {
				p.fail(driver.AudioFailed, jsError(e).Error())
			}
		})
	}
	p.el.Set("currentTime", start.Seconds())
	p.report(driver.AudioProgress)
}

func (p *webAudioPlayer) resume() {
	if p.fromTrack() {
		p.resumeTrack()
		return
	}
	// An ended player starts over from 0, also in an element it loads in place of its track.
	restart := p.ended
	if !p.preload() {
		return
	}
	if restart {
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

// resumeTrack plays the decoded track, or waits for it to decode. The output context starts in the trusted gesture that asked to play, as an element's play would.
func (p *webAudioPlayer) resumeTrack() {
	if !p.audio.activated {
		p.fail(driver.AudioNotAllowed, "the browser plays audio only after a gesture on the page")
		return
	}
	if !p.output() {
		p.fail(driver.AudioUnavailable, "the browser did not open an audio output")
		return
	}
	p.failed = false
	if p.ended {
		p.ended = false
		p.position = 0
	}
	if p.el.Truthy() {
		p.readPosition()
		p.releaseElement()
	}
	p.wanted = true
	p.preload()
	if p.track.Truthy() {
		p.startNode()
	} else {
		p.buffer(true)
	}
}

// startNode plays the track from the player's position in a new source node, which replaces any playing one.
func (p *webAudioPlayer) startNode() {
	a := p.audio
	p.stopNode()
	// The track's length can differ from the element's that played before it, as by an MP3's padding; the node loops at the track's.
	p.duration = mediaTime(p.track.Get("duration"))
	from := min(p.position, p.duration)
	if p.loop && from >= p.duration {
		from = p.loopFrom()
	}
	node, gain := a.output.Call("createBufferSource"), a.output.Call("createGain")
	node.Set("buffer", p.track)
	node.Call("connect", gain)
	gain.Call("connect", a.output.Get("destination"))
	p.node, p.gain = node, gain
	p.settings()
	p.nodeEnded = js.FuncOf(func(js.Value, []js.Value) any {
		p.trackEnded(node)
		return nil
	})
	node.Set("onended", p.nodeEnded)
	p.at, p.from, p.lapsAt = a.output.Get("currentTime").Float(), from, p.laps
	p.position = from
	node.Call("start", 0, from.Seconds())
	p.startTimer()
	p.buffer(false)
	if !p.playing {
		p.playing = true
		p.report(driver.AudioPlaying)
	}
}

// trackPosition is where the playing node is in the track at the output context's time now, and how many times it started over since at: it plays on from the anchor at its rate, then ends at the track's end or wraps back to the loop start.
func (p *webAudioPlayer) trackPosition(now float64) (time.Duration, uint32) {
	elapsed := max(0, now-p.at) * float64(p.rate)
	position := p.from + time.Duration(elapsed*float64(time.Second))
	if position < p.duration {
		return position, 0
	}
	start := p.loopFrom()
	if !p.loop || p.duration <= start {
		return p.duration, 0
	}
	over, span := position-p.duration, p.duration-start
	return start + over%span, uint32(over/span) + 1
}

// trackEnded ends a player whose node played to the end of the track, not one stopped or replaced.
func (p *webAudioPlayer) trackEnded(node js.Value) {
	if !node.Equal(p.node) {
		return
	}
	p.stopNode()
	p.stopTimer()
	p.wanted, p.playing, p.ended = false, false, true
	p.position = p.duration
	p.report(driver.AudioEnded)
}

func (p *webAudioPlayer) stopNode() {
	if !p.node.Truthy() {
		return
	}
	p.node.Set("onended", js.Null())
	p.nodeEnded.Release()
	p.node.Call("stop")
	p.node.Call("disconnect")
	p.gain.Call("disconnect")
	p.node, p.gain = js.Undefined(), js.Undefined()
}

func (p *webAudioPlayer) pause() {
	if p.failed || p.ended {
		return
	}
	p.paused()
	if p.el.Truthy() {
		p.el.Call("pause")
	}
	p.stopNode()
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
	if p.track.Truthy() && !p.el.Truthy() {
		p.position = min(position, p.duration)
		if p.node.Truthy() {
			p.startNode()
		}
		p.report(driver.AudioProgress)
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
	if !p.node.Truthy() && (!p.loaded || !p.el.Truthy() || p.el.Get("seeking").Bool()) {
		return
	}
	p.readPosition()
	p.report(driver.AudioProgress)
}

// readPosition reads where the player is. A playing node's position becomes its new anchor, so that its rate and loop can change from there.
func (p *webAudioPlayer) readPosition() {
	if p.node.Truthy() {
		now := p.audio.output.Get("currentTime").Float()
		position, laps := p.trackPosition(now)
		p.at, p.from = now, position
		p.lapsAt += laps
		p.position, p.laps = position, p.lapsAt
		return
	}
	if !p.loaded || !p.el.Truthy() || p.el.Get("seeking").Bool() {
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
	p.cancelDecode()
	p.release()
	p.failed = true
	p.audio.fail(p.id, code, message)
}

func (p *webAudioPlayer) startTimer() {
	if p.timer.Truthy() {
		return
	}
	if !p.tick.Truthy() {
		p.tick = js.FuncOf(func(js.Value, []js.Value) any {
			p.progress()
			return nil
		})
	}
	p.timer = js.Global().Call("setInterval", p.tick, max(16, p.interval.Milliseconds()))
}

func (p *webAudioPlayer) stopTimer() {
	if p.timer.Truthy() {
		js.Global().Call("clearInterval", p.timer)
		p.timer = js.Undefined()
	}
}

// release stops the player and frees its element and node; a decoded track stays for the next play.
func (p *webAudioPlayer) release() {
	p.generation++
	p.wanted, p.playing, p.loaded, p.buffering, p.ended = false, false, false, false, false
	p.stopTimer()
	if p.tick.Truthy() {
		p.tick.Release()
		p.tick = js.Func{}
	}
	p.stopNode()
	p.releaseElement()
}

func (p *webAudioPlayer) releaseElement() {
	if !p.el.Truthy() {
		return
	}
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

func (p *webAudioPlayer) cancelDecode() {
	if p.decode != nil {
		p.decode()
		p.decode = nil
	}
}

// close releases the player and drops its track, cancelling a decode in progress.
func (p *webAudioPlayer) close() {
	p.cancelDecode()
	p.track = js.Undefined()
	p.release()
}

func mediaTime(value js.Value) time.Duration {
	seconds := value.Float()
	if math.IsNaN(seconds) || seconds < 0 || seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}
