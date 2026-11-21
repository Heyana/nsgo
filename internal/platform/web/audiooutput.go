//go:build js && wasm

package web

import (
	"fmt"
	"math"
	"sync"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type webPCMMode uint8

const (
	webPCMModeLoading webPCMMode = iota
	webPCMModeWorklet
	webPCMModeFallback
)

type scheduledAudioSource struct {
	node   js.Value
	endSec float64
}

var ignorePromiseReject = sync.OnceValue(func() js.Func {
	return js.FuncOf(func(js.Value, []js.Value) any { return nil })
})

func catchPromise(v js.Value) {
	if v.Truthy() && v.Get("catch").Type() == js.TypeFunction {
		v.Call("catch", ignorePromiseReject())
	}
}

func jsTry(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("nsgo: audio: %v", r)
			}
		}
	}()
	fn()
	return nil
}

// webPCMWorkletJS is the inline AudioWorkletProcessor registered via a Blob URL.
// It consumes interleaved Float32 chunks pushed over its MessagePort from Go,
// outputs silence when paused or starved, and posts progress back so Go can
// refill the ring buffer promptly.
const webPCMWorkletJS = `
class NsgoPCMProcessor extends AudioWorkletProcessor {
	constructor(options) {
		super();
		const opts = (options && options.processorOptions) || {};
		this.channels = opts.channels === 1 ? 1 : 2;
		this.cap = opts.capacityFrames || 8192;
		this.ring = new Float32Array(this.cap * this.channels);
		this.readPos = 0;
		this.writePos = 0;
		this.buffered = 0;
		this.playing = false;
		this.closed = false;
		this.consumedSinceReport = 0;
		this.underrunsSinceReport = 0;
		this.reportEvery = opts.reportFrames || 256;
		this.port.onmessage = (e) => {
			const msg = e.data;
			if (!msg) return;
			if (msg.type === 'push') {
				const src = msg.samples;
				if (!src) return;
				const ch = this.channels;
				const frames = (src.length / ch) | 0;
				const space = this.cap - this.buffered;
				const n = frames < space ? frames : space;
				let wp = this.writePos;
				const cap = this.cap;
				for (let i = 0; i < n; i++) {
					const sOff = i * ch;
					const dOff = wp * ch;
					for (let c = 0; c < ch; c++) {
						this.ring[dOff + c] = src[sOff + c];
					}
					wp++;
					if (wp === cap) wp = 0;
				}
				this.writePos = wp;
				this.buffered += n;
			} else if (msg.type === 'flush') {
				this.readPos = 0;
				this.writePos = 0;
				this.buffered = 0;
				this.consumedSinceReport = 0;
				this.underrunsSinceReport = 0;
			} else if (msg.type === 'state') {
				this.playing = !!msg.playing;
			} else if (msg.type === 'close') {
				this.playing = false;
				this.closed = true;
			}
		};
	}

	process(_inputs, outputs) {
		if (this.closed) return false;
		const out = outputs[0];
		if (!out || out.length === 0) return true;
		const frames = out[0].length;
		if (!this.playing) {
			for (let c = 0; c < out.length; c++) out[c].fill(0);
			return true;
		}
		const ch = this.channels;
		const cap = this.cap;
		const avail = this.buffered < frames ? this.buffered : frames;
		let rp = this.readPos;
		if (ch === 1) {
			const ch0 = out[0];
			for (let i = 0; i < avail; i++) {
				ch0[i] = this.ring[rp++];
				if (rp === cap) rp = 0;
			}
			if (avail < frames) ch0.fill(0, avail);
			for (let c = 1; c < out.length; c++) out[c].set(ch0);
		} else {
			const ch0 = out[0];
			const ch1 = out.length > 1 ? out[1] : null;
			for (let i = 0; i < avail; i++) {
				const off = rp * 2;
				ch0[i] = this.ring[off];
				if (ch1) ch1[i] = this.ring[off + 1];
				rp++;
				if (rp === cap) rp = 0;
			}
			if (avail < frames) {
				ch0.fill(0, avail);
				if (ch1) ch1.fill(0, avail);
			}
		}
		this.readPos = rp;
		this.buffered -= avail;
		this.consumedSinceReport += frames;
		if (avail < frames) {
			this.underrunsSinceReport++;
		}
		if (this.consumedSinceReport >= this.reportEvery || this.buffered <= this.reportEvery) {
			this.port.postMessage({
				type: 'progress',
				consumed: this.consumedSinceReport,
				buffered: this.buffered,
				underruns: this.underrunsSinceReport,
				currentTime: currentTime,
			});
			this.consumedSinceReport = 0;
			this.underrunsSinceReport = 0;
		}
		return true;
	}
}
registerProcessor('nsgo-pcm-output', NsgoPCMProcessor);
`

// OpenAudioOutput opens a paused pull PCM stream backed by Web Audio's
// AudioContext + AudioWorkletNode, falling back to scheduled
// AudioBufferSourceNodes when AudioWorklet is unavailable (such as non-secure
// contexts) or when the browser's AudioContext sample rate differs.
func (*surface) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	return openWebAudioOutput(c)
}

type webAudioOutput struct {
	conf  driver.AudioOutputConfig
	mu    sync.Mutex
	state *webPCMState
	mode  webPCMMode

	ctx     js.Value
	node    js.Value
	port    js.Value
	timer   js.Value
	sources []scheduledAudioSource

	drainBuf         []float32
	interleavedBytes []byte
	planarBytes      []byte

	onStateChange js.Func
	onPortMsg     js.Func
	onPumpTimer   js.Func
	funcsAlive    bool
}

func openWebAudioOutput(conf driver.AudioOutputConfig) (driver.AudioOutput, error) {
	g := js.Global()
	ctor := g.Get("AudioContext")
	if !ctor.Truthy() {
		ctor = g.Get("webkitAudioContext")
	}
	if !ctor.Truthy() {
		return nil, driver.ErrAudioOutputUnsupported
	}

	opts := map[string]any{"sampleRate": conf.SampleRate}
	if conf.Latency > 0 {
		opts["latencyHint"] = conf.Latency.Seconds()
	} else {
		opts["latencyHint"] = "interactive"
	}

	var ctx js.Value
	err := jsTry(func() { ctx = ctor.New(opts) })
	if err != nil || !ctx.Truthy() {
		err = jsTry(func() { ctx = ctor.New(map[string]any{"latencyHint": opts["latencyHint"]}) })
	}
	if err != nil || !ctx.Truthy() {
		err = jsTry(func() { ctx = ctor.New() })
	}
	if err != nil || !ctx.Truthy() {
		if err != nil {
			return nil, fmt.Errorf("nsgo: audio: open Web Audio context: %w", err)
		}
		return nil, driver.ErrAudioOutputUnsupported
	}

	st := newWebPCMState(conf.SampleRate, conf.Channels, conf.Latency)
	// NewOutput opens paused; suspend immediately if the browser started running.
	if ctx.Get("state").String() == "running" {
		st.pausing = true
		if ctx.Get("suspend").Type() == js.TypeFunction {
			catchPromise(ctx.Call("suspend"))
		}
	}

	o := &webAudioOutput{
		conf:             conf,
		state:            st,
		mode:             webPCMModeFallback,
		ctx:              ctx,
		drainBuf:         make([]float32, st.geom.chunkFrames*conf.Channels),
		interleavedBytes: make([]byte, st.geom.chunkFrames*conf.Channels*4),
		planarBytes:      make([]byte, st.geom.chunkFrames*4),
		funcsAlive:       true,
	}

	o.onStateChange = js.FuncOf(func(js.Value, []js.Value) any {
		o.handleStateChange()
		return nil
	})
	o.onPortMsg = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			o.handlePortMessage(args[0])
		}
		return nil
	})
	o.onPumpTimer = js.FuncOf(func(js.Value, []js.Value) any {
		o.mu.Lock()
		if o.state.active && !o.state.closed {
			o.pumpLocked()
		}
		o.mu.Unlock()
		return nil
	})

	if ctx.Get("addEventListener").Type() == js.TypeFunction {
		ctx.Call("addEventListener", "statechange", o.onStateChange)
	}

	if o.canUseWorklet() {
		o.startWorkletLoad()
	}
	return o, nil
}

func (o *webAudioOutput) canUseWorklet() bool {
	g := js.Global()
	aw := o.ctx.Get("audioWorklet")
	if !aw.Truthy() || aw.Get("addModule").Type() != js.TypeFunction {
		return false
	}
	if !g.Get("AudioWorkletNode").Truthy() || !g.Get("Blob").Truthy() {
		return false
	}
	urlCtor := g.Get("URL")
	if !urlCtor.Truthy() || urlCtor.Get("createObjectURL").Type() != js.TypeFunction {
		return false
	}
	ctxRate := o.ctx.Get("sampleRate").Float()
	if math.IsNaN(ctxRate) || int(math.Round(ctxRate)) != o.conf.SampleRate {
		// Fall back to AudioBufferSourceNode so Web Audio resamples natively.
		return false
	}
	return true
}

func (o *webAudioOutput) startWorkletLoad() {
	g := js.Global()
	urlCtor := g.Get("URL")
	var blobURL js.Value
	err := jsTry(func() {
		blob := g.Get("Blob").New([]any{webPCMWorkletJS}, map[string]any{"type": "application/javascript"})
		blobURL = urlCtor.Call("createObjectURL", blob)
	})
	if err != nil || !blobURL.Truthy() {
		o.mode = webPCMModeFallback
		return
	}
	o.mode = webPCMModeLoading

	revoke := func() {
		if blobURL.Truthy() && urlCtor.Get("revokeObjectURL").Type() == js.TypeFunction {
			_ = jsTry(func() { urlCtor.Call("revokeObjectURL", blobURL) })
			blobURL = js.Undefined()
		}
	}

	var onReady, onFail js.Func
	releaseLoadFuncs := func() {
		onReady.Release()
		onFail.Release()
	}
	onReady = js.FuncOf(func(js.Value, []js.Value) any {
		defer releaseLoadFuncs()
		revoke()
		o.finishWorkletLoad()
		return nil
	})
	onFail = js.FuncOf(func(js.Value, []js.Value) any {
		defer releaseLoadFuncs()
		revoke()
		o.mu.Lock()
		if !o.state.closed {
			o.mode = webPCMModeFallback
			if o.state.active {
				o.pumpLocked()
			}
		}
		o.mu.Unlock()
		return nil
	})

	var prom js.Value
	err = jsTry(func() {
		prom = o.ctx.Get("audioWorklet").Call("addModule", blobURL)
	})
	if err != nil || !prom.Truthy() || prom.Get("then").Type() != js.TypeFunction {
		revoke()
		releaseLoadFuncs()
		o.mode = webPCMModeFallback
		return
	}
	prom.Call("then", onReady, onFail)
}

func (o *webAudioOutput) finishWorkletLoad() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state.closed {
		return
	}
	var node js.Value
	err := jsTry(func() {
		opts := map[string]any{
			"numberOfInputs":     0,
			"numberOfOutputs":    1,
			"outputChannelCount": []any{o.conf.Channels},
			"processorOptions": map[string]any{
				"channels":       o.conf.Channels,
				"capacityFrames": o.state.geom.ringCapFrames,
				"reportFrames":   o.state.geom.reportFrames,
			},
		}
		node = js.Global().Get("AudioWorkletNode").New(o.ctx, "nsgo-pcm-output", opts)
		node.Call("connect", o.ctx.Get("destination"))
	})
	if err != nil || !node.Truthy() {
		o.mode = webPCMModeFallback
		if o.state.active {
			o.pumpLocked()
		}
		return
	}
	o.node = node
	o.port = node.Get("port")
	if o.port.Truthy() {
		o.port.Set("onmessage", o.onPortMsg)
	}
	o.mode = webPCMModeWorklet
	if o.state.active {
		o.postWorkletStateLocked(true)
		o.pumpLocked()
	}
}

func (o *webAudioOutput) handleStateChange() {
	o.mu.Lock()
	if o.state.closed {
		o.mu.Unlock()
		return
	}
	st := o.ctx.Get("state").String()
	interrupt, err := o.state.onContextState(st)
	if interrupt {
		nowSec, wallRef := o.clockLocked()
		o.state.pause(nowSec, wallRef)
		o.stopPumpLocked()
		o.discardDeviceLocked()
		cb := o.conf.Interrupt
		o.mu.Unlock()
		if cb != nil {
			cb(err)
		}
		return
	}
	if st == "running" && o.state.active {
		o.pumpLocked()
	}
	o.mu.Unlock()
}

func (o *webAudioOutput) handlePortMessage(ev js.Value) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state.closed || !o.state.active {
		return
	}
	data := ev.Get("data")
	if !data.Truthy() {
		return
	}
	if u := data.Get("underruns"); u.Type() == js.TypeNumber && u.Int() > 0 {
		o.state.addWorkletUnderruns(u.Int())
	}
	o.pumpLocked()
}

func (o *webAudioOutput) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.state.start(); err != nil {
		return err
	}
	if o.ctx.Get("state").String() == "running" {
		o.state.seenRunning = true
	} else if o.ctx.Get("resume").Type() == js.TypeFunction {
		catchPromise(o.ctx.Call("resume"))
	}
	if o.mode == webPCMModeWorklet {
		o.postWorkletStateLocked(true)
	}
	o.startPumpLocked()
	o.pumpLocked()
	return nil
}

func (o *webAudioOutput) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state.closed || !o.state.active {
		return
	}
	nowSec, wallRef := o.clockLocked()
	o.state.pause(nowSec, wallRef)
	o.stopPumpLocked()
	o.discardDeviceLocked()
	if o.ctx.Get("suspend").Type() == js.TypeFunction {
		catchPromise(o.ctx.Call("suspend"))
	}
}

func (o *webAudioOutput) Flush() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	nowSec, wallRef := o.clockLocked()
	dropped := o.state.flush(nowSec, wallRef)
	o.flushDeviceLocked()
	if o.state.active && !o.state.closed {
		o.pumpLocked()
	}
	return dropped
}

func (o *webAudioOutput) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	nowSec, wallRef := o.clockLocked()
	if !o.state.close(nowSec, wallRef) {
		return
	}
	o.stopPumpLocked()
	o.discardDeviceLocked()
	if o.port.Truthy() {
		o.port.Call("postMessage", map[string]any{"type": "close"})
		o.port.Set("onmessage", js.Null())
		o.port = js.Undefined()
	}
	if o.node.Truthy() {
		_ = jsTry(func() { o.node.Call("disconnect") })
		o.node = js.Undefined()
	}
	if o.ctx.Truthy() {
		if o.ctx.Get("removeEventListener").Type() == js.TypeFunction {
			o.ctx.Call("removeEventListener", "statechange", o.onStateChange)
		}
		if o.ctx.Get("close").Type() == js.TypeFunction {
			catchPromise(o.ctx.Call("close"))
		}
	}
	if o.funcsAlive {
		o.funcsAlive = false
		o.onStateChange.Release()
		o.onPortMsg.Release()
		o.onPumpTimer.Release()
	}
}

func (o *webAudioOutput) Position() (int64, time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	nowSec, wallRef := o.clockLocked()
	return o.state.position(nowSec, wallRef)
}

func (o *webAudioOutput) Latency() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	curSec := o.currentTimeLocked()
	baseSec := o.floatPropLocked("baseLatency")
	outSec := o.floatPropLocked("outputLatency")
	return o.state.latency(baseSec, outSec, curSec)
}

func (o *webAudioOutput) Underruns() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state.underruns
}

func (o *webAudioOutput) startPumpLocked() {
	if o.timer.Truthy() {
		return
	}
	o.timer = js.Global().Call("setInterval", o.onPumpTimer, o.state.geom.pumpIntervalMs)
}

func (o *webAudioOutput) stopPumpLocked() {
	if !o.timer.Truthy() {
		return
	}
	js.Global().Call("clearInterval", o.timer)
	o.timer = js.Undefined()
}

func (o *webAudioOutput) postWorkletStateLocked(playing bool) {
	if o.port.Truthy() {
		o.port.Call("postMessage", map[string]any{"type": "state", "playing": playing})
	}
}

func (o *webAudioOutput) flushDeviceLocked() {
	if o.mode == webPCMModeWorklet && o.port.Truthy() {
		o.port.Call("postMessage", map[string]any{"type": "flush"})
	}
	o.stopSourcesLocked()
}

func (o *webAudioOutput) discardDeviceLocked() {
	if o.port.Truthy() {
		o.port.Call("postMessage", map[string]any{"type": "state", "playing": false})
		o.port.Call("postMessage", map[string]any{"type": "flush"})
	}
	o.stopSourcesLocked()
}

func (o *webAudioOutput) stopSourcesLocked() {
	for _, s := range o.sources {
		_ = jsTry(func() { s.node.Call("stop") })
		_ = jsTry(func() { s.node.Call("disconnect") })
	}
	o.sources = o.sources[:0]
}

func (o *webAudioOutput) pumpLocked() {
	if !o.state.active || o.state.closed {
		return
	}
	curSec := o.currentTimeLocked()
	needed := o.state.neededChunks(curSec)
	for range needed {
		o.state.pullIntoRing(o.conf.Render)
	}
	switch o.mode {
	case webPCMModeWorklet:
		o.drainWorkletLocked(curSec)
	case webPCMModeFallback:
		o.drainFallbackLocked(curSec)
	}
}

func (o *webAudioOutput) drainWorkletLocked(curSec float64) {
	if !o.port.Truthy() {
		return
	}
	g := js.Global()
	u8Ctor := g.Get("Uint8Array")
	f32Ctor := g.Get("Float32Array")
	for {
		frames, produced, ok := o.state.ring.readChunk(o.drainBuf)
		if !ok {
			return
		}
		o.state.timeline.schedule(curSec, int64(frames), int64(produced))
		samples := frames * o.conf.Channels
		byteLen := samples * 4
		encodeInterleavedFloat32LE(o.interleavedBytes[:byteLen], o.drainBuf[:samples])
		jsBytes := u8Ctor.New(byteLen)
		js.CopyBytesToJS(jsBytes, o.interleavedBytes[:byteLen])
		jsFloats := f32Ctor.New(jsBytes.Get("buffer"), jsBytes.Get("byteOffset"), samples)
		o.port.Call("postMessage", map[string]any{"type": "push", "samples": jsFloats}, []any{jsFloats.Get("buffer")})
	}
}

func (o *webAudioOutput) drainFallbackLocked(curSec float64) {
	// Prune finished source nodes.
	kept := o.sources[:0]
	for _, s := range o.sources {
		if s.endSec > curSec {
			kept = append(kept, s)
		}
	}
	o.sources = kept

	g := js.Global()
	u8Ctor := g.Get("Uint8Array")
	f32Ctor := g.Get("Float32Array")
	dest := o.ctx.Get("destination")
	for {
		frames, produced, ok := o.state.ring.readChunk(o.drainBuf)
		if !ok {
			return
		}
		startSec := o.state.timeline.schedule(curSec, int64(frames), int64(produced))
		var buf js.Value
		if err := jsTry(func() { buf = o.ctx.Call("createBuffer", o.conf.Channels, frames, o.conf.SampleRate) }); err != nil || !buf.Truthy() {
			return
		}
		byteLen := frames * 4
		for ch := range o.conf.Channels {
			encodePlanarChannelFloat32LE(o.planarBytes[:byteLen], o.drainBuf[:frames*o.conf.Channels], ch, o.conf.Channels, frames)
			jsBytes := u8Ctor.New(byteLen)
			js.CopyBytesToJS(jsBytes, o.planarBytes[:byteLen])
			jsFloats := f32Ctor.New(jsBytes.Get("buffer"), jsBytes.Get("byteOffset"), frames)
			buf.Call("copyToChannel", jsFloats, ch)
		}
		src := o.ctx.Call("createBufferSource")
		src.Set("buffer", buf)
		src.Call("connect", dest)
		_ = jsTry(func() { src.Call("start", startSec) })
		endSec := startSec + float64(frames)/float64(o.conf.SampleRate)
		o.sources = append(o.sources, scheduledAudioSource{node: src, endSec: endSec})
	}
}

func (o *webAudioOutput) currentTimeLocked() float64 {
	if !o.ctx.Truthy() {
		return 0
	}
	v := o.ctx.Get("currentTime")
	if v.Type() != js.TypeNumber {
		return 0
	}
	sec := v.Float()
	if math.IsNaN(sec) || math.IsInf(sec, 0) || sec < 0 {
		return 0
	}
	return sec
}

func (o *webAudioOutput) floatPropLocked(name string) float64 {
	if !o.ctx.Truthy() {
		return 0
	}
	v := o.ctx.Get(name)
	if v.Type() != js.TypeNumber {
		return 0
	}
	return v.Float()
}

func (o *webAudioOutput) clockLocked() (float64, time.Time) {
	wallNow := time.Now()
	curSec := o.currentTimeLocked()
	outLatSec := o.floatPropLocked("outputLatency")
	var tsCtx, tsPerf, perfNow float64
	if o.ctx.Truthy() && o.ctx.Get("getOutputTimestamp").Type() == js.TypeFunction {
		var ts js.Value
		if err := jsTry(func() { ts = o.ctx.Call("getOutputTimestamp") }); err == nil && ts.Truthy() {
			if ct := ts.Get("contextTime"); ct.Type() == js.TypeNumber {
				tsCtx = ct.Float()
			}
			if pt := ts.Get("performanceTime"); pt.Type() == js.TypeNumber {
				tsPerf = pt.Float()
			}
		}
	}
	if perf := js.Global().Get("performance"); perf.Truthy() && perf.Get("now").Type() == js.TypeFunction {
		if pn := perf.Call("now"); pn.Type() == js.TypeNumber {
			perfNow = pn.Float()
		}
	}
	return webOutputClock(curSec, tsCtx, tsPerf, perfNow, outLatSec, wallNow)
}
