//go:build android

package android

import (
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/perfhud"
)

type performance struct {
	monitor   *engine.PerfMonitor
	showPanel bool
	lastPanel time.Time
	updating  atomic.Bool
	sampler   perfhud.Sampler
}

// perfFrame records the work done by the primary view on the Android UI
// thread. It does not drive the engine's animation callbacks.
func (s *surface) perfFrame(r *reader) {
	p := s.perf
	if p == nil || len(r.buf) != 25 {
		return
	}
	at := frameTime(r)
	if at.IsZero() {
		at = time.Now()
	}
	period, work, changed := time.Duration(r.u64()), time.Duration(r.u64()), r.byte() != 0
	p.monitor.RecordWork(work, changed)
	p.monitor.Vsync(at, period)

	if !p.showPanel || at.Sub(p.lastPanel) < engine.PerfInterval || !p.updating.CompareAndSwap(false, true) {
		return
	}
	p.lastPanel = at
	// Keep the reader free to receive text replies while the writer sends the panel.
	go func() {
		defer p.updating.Store(false)
		p.monitor.SetProcess(p.sampler.Sample())
		snapshot := p.monitor.Snapshot()
		s.send(msgPerfPanel, func(w *writer) { writePerfPanel(w, snapshot) })
	}()
}

func writePerfPanel(w *writer, snapshot engine.PerfSnapshot) {
	const traceWidth = engine.PerfWidth - 2*engine.PerfPadding
	w.str(snapshot.Readings())
	w.floats(engine.PerfWidth, engine.PerfMargin, engine.PerfPadding, engine.PerfGraphHeight, engine.PerfFontSize, engine.PerfRadius)
	w.u32(uint32(engine.PerfPanelColor))
	w.u32(uint32(engine.PerfTextColor))
	bars := engine.PerfTrace(snapshot, traceWidth, engine.PerfGraphHeight)
	w.u32(uint32(len(bars)))
	for _, bar := range bars {
		w.floats(float32(bar.X/traceWidth), float32(bar.Y), float32(bar.W/traceWidth), float32(bar.H))
		w.u32(uint32(bar.Color))
	}
}
