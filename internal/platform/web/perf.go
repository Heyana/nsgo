//go:build js && wasm

package web

import (
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// The performance monitor's panel (--nsgo-fps / ?nsgo-fps): a pointer-transparent
// overlay at the top right of the surface's host element, redrawn every
// engine.PerfInterval (500 ms). While a monitor is installed, a continuous
// requestAnimationFrame clock closes each display refresh with Vsync and records
// main-thread work and batch hop latency.

type hud struct {
	surface *surface
	monitor *engine.PerfMonitor
	clock   perfClock
	panel   js.Value
	trace   js.Value
	ctx     js.Value
	label   js.Value
	timer   js.Value
	tickFn  js.Func
	done    bool
}

// startPerf mounts the performance panel overlay inside s.host, starts the
// 500 ms redraw timer, and starts the continuous requestAnimationFrame clock.
func (s *surface) startPerf() {
	if s.perf == nil || s.hud != nil || s.closed {
		return
	}
	panel := s.doc.Call("createElement", "div")
	setStyle(panel, perfPanelStyles(s.whole))
	setAttr(panel, "aria-hidden", "true")
	setAttr(panel, "data-nsgo-perf", "panel")
	panel.Set("inert", true)

	trace := s.doc.Call("createElement", "canvas")
	setStyle(trace, perfTraceStyles())
	var ctx js.Value
	if trace.Get("getContext").Type() == js.TypeFunction {
		ctx = trace.Call("getContext", "2d")
	}

	label := s.doc.Call("createElement", "div")
	setStyle(label, perfLabelStyles())

	panel.Call("appendChild", trace)
	panel.Call("appendChild", label)
	s.host.Call("appendChild", panel)

	h := &hud{
		surface: s,
		monitor: s.perf,
		panel:   panel,
		trace:   trace,
		ctx:     ctx,
		label:   label,
	}
	s.hud = h
	h.update()

	g := js.Global()
	if g.Get("setInterval").Type() == js.TypeFunction {
		h.tickFn = js.FuncOf(func(js.Value, []js.Value) any {
			h.update()
			return nil
		})
		h.timer = g.Call("setInterval", h.tickFn, perfIntervalMs())
	}
	s.startTicking()
}

// stopPerf tears down the performance panel, stops its redraw timer, clears the
// monitor, and cancels any pending animation frame if nothing else animates.
func (s *surface) stopPerf() {
	if h := s.hud; h != nil {
		h.done = true
		if h.timer.Truthy() && js.Global().Get("clearInterval").Type() == js.TypeFunction {
			js.Global().Call("clearInterval", h.timer)
			h.timer = js.Undefined()
		}
		if !h.tickFn.IsUndefined() {
			h.tickFn.Release()
			h.tickFn = js.Func{}
		}
		if h.panel.Truthy() && h.panel.Get("remove").Type() == js.TypeFunction {
			h.panel.Call("remove")
		}
		s.hud = nil
	}
	s.perf = nil
	if !s.wantsTicking() {
		s.stopTicking()
	}
}

// perfWork starts timing main-thread work when a monitor is installed and
// returns the function recording it; dirty reports whether the work changed
// what the screen shows.
func (s *surface) perfWork(dirty bool) func() {
	if s == nil || s.perf == nil {
		return func() {}
	}
	start := time.Now()
	return func() {
		if s.perf != nil {
			s.perf.RecordWork(time.Since(start), dirty)
		}
	}
}

// vsync closes the current frame on the performance monitor using the
// requestAnimationFrame timestamp.
func (s *surface) vsync(stamp time.Duration) {
	if s.perf == nil {
		return
	}
	at := time.Now()
	if stamp > 0 {
		at = driver.DisplayTime(stamp, s.clock())
	}
	var interval time.Duration
	if s.hud != nil {
		interval = s.hud.clock.next(stamp, s.watch.refreshRate)
	}
	s.perf.Vsync(at, interval)
}

// update refreshes the panel's readings text and trace canvas.
func (h *hud) update() {
	if h.done || h.monitor == nil {
		return
	}
	snapshot := h.monitor.Snapshot()
	h.label.Set("textContent", snapshot.Readings())
	h.drawTrace(snapshot)
}

func (h *hud) drawTrace(snapshot engine.PerfSnapshot) {
	if !h.ctx.Truthy() {
		return
	}
	_, traceW := perfPanelLayout(h.surface.host.Get("clientWidth").Float())
	bars := perfTraceBars(snapshot, traceW)
	if len(bars) == 0 {
		return
	}
	dpr := perfPixelRatio(js.Global().Get("devicePixelRatio").Float())
	pw := perfCanvasPixels(traceW, dpr)
	ph := perfCanvasPixels(engine.PerfGraphHeight, dpr)
	if h.trace.Get("width").Int() != pw {
		h.trace.Set("width", pw)
	}
	if h.trace.Get("height").Int() != ph {
		h.trace.Set("height", ph)
	}
	ctx := h.ctx
	ctx.Call("setTransform", dpr, 0, 0, dpr, 0, 0)
	ctx.Call("clearRect", 0, 0, traceW, engine.PerfGraphHeight)
	var lastColor driver.Color
	hasColor := false
	for _, b := range bars {
		if !hasColor || b.Color != lastColor {
			lastColor = b.Color
			hasColor = true
			ctx.Set("fillStyle", webstyle.Color(b.Color))
		}
		ctx.Call("fillRect", b.X, b.Y, b.W, b.H)
	}
}

func (s *surface) hasStaleEffects() bool {
	for v := range s.effects {
		if v.fx != nil && (v.fx.stale || (v.fx.shader != nil && v.fx.shader.Running)) {
			return true
		}
	}
	return false
}
