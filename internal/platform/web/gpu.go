//go:build js && wasm

package web

import (
	"math"
	"sync"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// gpuForceWebGL2, when true (or when globalThis.__nsgoForceWebGL2 is truthy), skips WebGPU and forces the automatic WebGL 2 fallback path for testing.
var gpuForceWebGL2 bool

var (
	gpuAvailOnce sync.Once
	gpuAvailVal  bool
)

// webGPUAvailable reports whether the browser supports either WebGPU (navigator.gpu) or WebGL 2 on a <canvas>. Cached once to avoid leaking limited browser WebGL contexts.
func webGPUAvailable() bool {
	gpuAvailOnce.Do(func() {
		g := js.Global()
		hasNavGPU := hasNavigatorGPU()
		if hasNavGPU {
			gpuAvailVal = gpuEnvSupported(true, false)
			return
		}
		hasGL2 := probeWebGL2(g)
		gpuAvailVal = gpuEnvSupported(false, hasGL2)
	})
	return gpuAvailVal
}

func hasNavigatorGPU() bool {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() {
		return false
	}
	gpu := nav.Get("gpu")
	return gpu.Truthy()
}

func forceWebGL2Enabled() bool {
	if gpuForceWebGL2 {
		return true
	}
	return js.Global().Get("__nsgoForceWebGL2").Truthy()
}

func probeWebGL2(g js.Value) bool {
	doc := g.Get("document")
	if !doc.Truthy() {
		return false
	}
	canvas := doc.Call("createElement", "canvas")
	if !canvas.Truthy() {
		return false
	}
	canvas.Set("width", 1)
	canvas.Set("height", 1)
	gl := canvas.Call("getContext", "webgl2")
	if !gl.Truthy() {
		return false
	}
	if lose := gl.Call("getExtension", "WEBGL_lose_context"); lose.Truthy() {
		lose.Call("loseContext")
	}
	return true
}

// gpuBackend is the active WebGPU or WebGL 2 implementation backing one gpuView.
type gpuBackend interface {
	resize(width, height int)
	submit(frame driver.GPUFrame)
	prepareCapture()
	destroy()
}

// gpuView owns the dedicated <canvas>, lifecycle state machine, active backend (WebGPU primary or WebGL 2 fallback), and js.Func callbacks for one KindGPU view.
type gpuView struct {
	s         *surface
	v         *view
	canvas    js.Value
	ctrl      *gpuViewController
	backend   gpuBackend
	needsInit bool
	listeners listeners

	// pendingFuncs tracks all in-flight Promise and callback js.Funcs so every one is released on settlement or when the view unmounts.
	pendingFuncs map[*js.Func]struct{}

	// onEvent is an optional package-internal test observer invoked alongside engine.Dispatch when emitEvent fires.
	onEvent func(*driver.GPUEvent)
}

func (s *surface) createGPU(v *view) {
	v.inner = s.element("canvas")
	setStyle(v.inner, map[string]string{
		"width":          "100%",
		"height":         "100%",
		"display":        "block",
		"pointer-events": "none",
	})
	v.el.Call("appendChild", v.inner)
	v.gpu = &gpuView{
		s:            s,
		v:            v,
		canvas:       v.inner,
		ctrl:         newGPUViewController(v.id),
		pendingFuncs: map[*js.Func]struct{}{},
	}
}

func (s *surface) configureGPU(v *view, cfg driver.GPUViewConfig) {
	g := v.gpu
	if g == nil {
		return
	}
	g.ctrl.configure(cfg)
	g.syncSize(false)
	if g.ctrl.state == gpuStateUninitialized {
		g.needsInit = true
		s.gpuInitDue = append(s.gpuInitDue, v)
	}
}

func (s *surface) resizeGPU(v *view) {
	g := v.gpu
	if g == nil {
		return
	}
	g.syncSize(true)
}

func (s *surface) initPendingGPUs() {
	if len(s.gpuInitDue) == 0 {
		return
	}
	due := s.gpuInitDue
	s.gpuInitDue = s.gpuInitDue[:0]
	for _, v := range due {
		if v == nil || s.view(v.id) != v || v.gpu == nil || !v.gpu.needsInit {
			continue
		}
		v.gpu.needsInit = false
		v.gpu.syncSize(false)
		v.gpu.startInit()
	}
}

func (g *gpuView) contentCSSSize() (driver.Points, driver.Points) {
	w := math.Max(float64(g.v.size.Width-g.v.border.Left-g.v.border.Right), 0)
	h := math.Max(float64(g.v.size.Height-g.v.border.Top-g.v.border.Bottom), 0)
	return driver.Points(w), driver.Points(h)
}

func (g *gpuView) syncSize(emitEvent bool) {
	if g.ctrl.state == gpuStateDestroyed {
		return
	}
	cw, ch := g.contentCSSSize()
	ratio := g.s.env.PixelRatio
	if ratio <= 0 {
		if dpr := js.Global().Get("devicePixelRatio"); dpr.Type() == js.TypeNumber {
			ratio = float32(dpr.Float())
		}
		if ratio <= 0 {
			ratio = 1
		}
	}
	pw, ph, changed, ev := g.ctrl.resize(cw, ch, ratio)
	if g.canvas.Get("width").Int() != pw {
		g.canvas.Set("width", pw)
	}
	if g.canvas.Get("height").Int() != ph {
		g.canvas.Set("height", ph)
	}
	if changed && g.backend != nil {
		g.backend.resize(pw, ph)
	}
	if emitEvent && ev != nil {
		g.emitEvent(ev)
	}
}

func (g *gpuView) startInit() {
	if !g.ctrl.beginInit() {
		return
	}
	if !forceWebGL2Enabled() && hasNavigatorGPU() {
		g.initWebGPU()
		return
	}
	g.initWebGL2()
}

func (g *gpuView) submit(frame driver.GPUFrame) {
	if g == nil || g.backend == nil {
		return
	}
	if !g.ctrl.acceptFrame(frame) {
		return
	}
	g.backend.submit(frame)
}

func (g *gpuView) prepareCapture() {
	if g == nil || g.backend == nil || g.ctrl.state != gpuStateReady {
		return
	}
	g.backend.prepareCapture()
}

func (g *gpuView) emitEvent(ev *driver.GPUEvent) {
	if g == nil || ev == nil || g.ctrl.state == gpuStateDestroyed {
		return
	}
	if g.onEvent != nil {
		g.onEvent(ev)
	}
	if g.s == nil || g.s.engine == nil || g.s.view(g.v.id) != g.v {
		return
	}
	g.s.engine.Dispatch(driver.Event{
		Kind: driver.EventGPU,
		ID:   g.v.id,
		Data: ev,
	})
}

func (g *gpuView) registerFunc(fn js.Func) *js.Func {
	f := fn
	g.pendingFuncs[&f] = struct{}{}
	return &f
}

func (g *gpuView) releaseFunc(f *js.Func) {
	if f == nil {
		return
	}
	if _, ok := g.pendingFuncs[f]; ok {
		delete(g.pendingFuncs, f)
		f.Release()
	}
}

// promiseThen attaches resolve/reject handlers to a JS Promise, guaranteeing both js.Funcs are released as soon as the promise settles or on unmount.
func (g *gpuView) promiseThen(p js.Value, onResolve func(js.Value), onReject func(js.Value)) {
	if !p.Truthy() || g.ctrl.state == gpuStateDestroyed {
		if onReject != nil {
			onReject(js.ValueOf("invalid promise"))
		}
		return
	}
	var resPtr, rejPtr *js.Func
	resFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		g.releaseFunc(resPtr)
		g.releaseFunc(rejPtr)
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		if onResolve != nil {
			onResolve(v)
		}
		return nil
	})
	rejFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		g.releaseFunc(resPtr)
		g.releaseFunc(rejPtr)
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		if onReject != nil {
			onReject(v)
		}
		return nil
	})
	resPtr = g.registerFunc(resFn)
	rejPtr = g.registerFunc(rejFn)
	p.Call("then", *resPtr, *rejPtr)
}

func (g *gpuView) release() {
	if g == nil || g.ctrl.state == gpuStateDestroyed {
		return
	}
	g.ctrl.destroy()
	g.listeners.release()
	if g.backend != nil {
		// Destroying a WebGPU device resolves device.lost and rejects any pending mapAsync promises; their promiseThen handlers see gpuStateDestroyed, clean up late objects, and release their js.Funcs upon settlement without invoking a released callback.
		g.backend.destroy()
		g.backend = nil
	}
}

func jsErrorMessage(v js.Value) string {
	if !v.Truthy() {
		return "unknown GPU error"
	}
	if v.Type() == js.TypeString {
		return v.String()
	}
	if msg := v.Get("message"); msg.Type() == js.TypeString && msg.String() != "" {
		return msg.String()
	}
	return v.Call("toString").String()
}
