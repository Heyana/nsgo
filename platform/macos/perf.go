//go:build darwin && !ios

package macos

import (
	"sync"
	"syscall"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
	"github.com/oneo-me/nsgo/platform/internal/apple"
)

// The performance monitor's HUD (`nsgo dev -fps`): a panel at the top right
// of the surface that shows what its frames and commits cost (engine/perf.go),
// redrawn twice a second. It follows NSH's macOS HUD.
//
// - FPS: frames shown over the last second (display refreshes in which the
//   surface changed: an idle app reads low, which is not slowness); MAX is
//   what the main thread could keep up with (one over the mean frame cost,
//   capped at the display's rate), then the display link's rate.
// - TICK: the gaps between refreshes as they arrived (mean, p95) and gap,
//   the p95 gap between shown frames.
// - MAIN: main-thread time per shown frame, its 95th percentile, and drop,
//   the share of frames over one refresh.
// - APP / CORE: per commit, the app's code and the core's work; commits per
//   second and mount items per commit.
// - HOP: how long batches waited between the engine goroutine and the main
//   thread; the engine goroutine's busy share.
// - CPU (100 is one core), GPU (this process' share, from the IO registry),
//   MEM (the physical footprint).
// - The trace: the last 120 frames' main-thread cost against the frame
//   budget (the line), green within half of it, amber within it, red over.
//
// The panel lets the mouse through and fades out while the pointer is over
// it, so what it covers stays visible. It measures nothing while hidden: the
// monitor is removed with it.

// How often the readings are redrawn.
const perfInterval = 500 * time.Millisecond

// The panel's width and its distance from the surface's edges (points).
const (
	perfWidth       = 262.0
	perfMargin      = 10.0
	perfPadding     = 8.0
	perfGraphHeight = 36.0
)

var (
	selFittingSize             = sel("fittingSize")
	selSetBackgroundColor      = sel("setBackgroundColor:")
	selSetCornerRadius         = sel("setCornerRadius:")
	selTargetTimestamp         = sel("targetTimestamp")
	selPreferredMaxLayoutWidth = sel("setPreferredMaxLayoutWidth:")
	selSubviews                = sel("subviews")
	selLastObject              = sel("lastObject")
)

// The panel's colors: a dark translucent fill with light text, and the
// trace's four levels.
var (
	perfPanelColor  = core.RGBA(0x121214, 0.86)
	perfTextColor   = core.RGB(0xebedf2)
	perfTraceFill   = core.RGBA(0xffffff, 0.08)
	perfTraceBudget = core.RGBA(0xffffff, 0.45)
	perfTraceGood   = core.RGBA(0x4dd973, 0.9)
	perfTraceWarn   = core.RGBA(0xfabf40, 0.9)
	perfTraceBad    = core.RGBA(0xfa5954, 0.9)
)

// measuring starts timing main-thread work when a monitor measures, and
// returns the function that records it; dirty means it changes what the
// screen shows. Without a monitor the returned function does nothing.
func (s *surface) measuring(dirty bool) func() {
	if s == nil || s.perf == nil {
		return func() {}
	}
	start := time.Now()
	return func() { s.perf.RecordWork(time.Since(start), dirty) }
}

// hud is one surface's performance panel.
type hud struct {
	surface *surface
	monitor *engine.PerfMonitor
	panel   objc.ID
	label   objc.ID
	trace   objc.ID
	sampler processSampler
	gpu     *apple.GPUProbe
	// The trace's frames (milliseconds) and the budget (milliseconds).
	frames []float64
	budget float64
	done   chan struct{}
	once   sync.Once
}

// The HUDs by their trace views, for the view's drawRect.
var (
	perfTraces                     = map[objc.ID]*hud{}
	perfClassOnce                  sync.Once
	classPerfPanel, classPerfTrace objc.Class
)

// newHud shows a panel over s and starts measuring its surface.
func newHud(s *surface, monitor *engine.PerfMonitor) *hud {
	registerPerfClasses()
	panel := objc.ID(classPerfPanel).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, 0, 0))
	panel.Send(selSetWantsLayer, true)
	panel.Send(selSetAccessibilityElement, false)
	panel.Send(selSetAccessibilityHidden, true)
	layer := panel.Send(selLayer)
	fill := apple.CGColor(perfPanelColor)
	layer.Send(selSetBackgroundColor, fill)
	apple.Release(fill)
	layer.Send(selSetCornerRadius, 8.0)
	trackingArea(panel, false).Send(selRelease)

	label := objc.ID(objc.GetClass("NSTextField")).Send(sel("labelWithString:"), apple.NSString(""))
	font := objc.ID(objc.GetClass("NSFont")).Send(sel("monospacedSystemFontOfSize:weight:"), 10.5, 0.0)
	label.Send(selSetFont, font)
	label.Send(selSetTextColor, nsColor(perfTextColor))
	label.Send(selSetAccessibilityElement, false)
	label.Send(selSetAccessibilityHidden, true)
	label.Send(sel("cell")).Send(sel("setWraps:"), true)
	label.Send(selPreferredMaxLayoutWidth, perfWidth-2*perfPadding)

	trace := objc.ID(classPerfTrace).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, 0, 0))
	trace.Send(selSetAccessibilityElement, false)
	trace.Send(selSetAccessibilityHidden, true)
	panel.Send(selAddSubview, label)
	panel.Send(selAddSubview, trace)
	trace.Send(selRelease)
	s.native.Send(selAddSubview, panel)

	h := &hud{
		surface: s,
		monitor: monitor,
		panel:   panel,
		label:   label,
		trace:   trace,
		gpu:     apple.NewGPUProbe(),
		done:    make(chan struct{}),
	}
	perfTraces[trace] = h
	h.update()
	go h.run()
	return h
}

// run samples the process and redraws the readings while the HUD shows.
func (h *hud) run() {
	ticker := time.NewTicker(perfInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sample := h.sampler.sample()
			if h.gpu != nil {
				sample.GPU = h.gpu.Sample()
			}
			h.monitor.SetProcess(sample)
			apple.OnMain(h.update)
		case <-h.done:
			return
		}
	}
}

// update redraws the readings and the trace.
func (h *hud) update() {
	select {
	case <-h.done:
		return
	default:
	}
	snapshot := h.monitor.Snapshot()
	h.label.Send(selSetString, apple.NSString(snapshot.Readings()))
	budget := 1000.0 / 60.0
	if snapshot.RefreshRate != nil {
		budget = 1000.0 / float64(*snapshot.RefreshRate)
	}
	frames := make([]float64, len(snapshot.Frames))
	for i, d := range snapshot.Frames {
		frames[i] = float64(d) / float64(time.Millisecond)
	}
	h.frames, h.budget = frames, budget
	h.layout()
	h.trace.Send(selSetNeedsDisplay, true)
}

// layout sizes the panel to its text, at the top right of its superview
// (a flipped surface: y grows downwards).
func (h *hud) layout() {
	parent := h.panel.Send(selSuperview)
	if parent == 0 {
		return
	}
	bounds := objc.Send[apple.CGRect](parent, selBounds)
	fit := objc.Send[apple.CGSize](h.label, selFittingSize)
	height := 3*perfPadding + fit.Height + perfGraphHeight
	h.panel.Send(selSetFrame, apple.Rect(
		bounds.Size.Width-perfWidth-perfMargin,
		perfMargin,
		perfWidth, height))
	h.label.Send(selSetFrame, apple.Rect(perfPadding, 2*perfPadding+perfGraphHeight,
		perfWidth-2*perfPadding, fit.Height))
	h.trace.Send(selSetFrame, apple.Rect(perfPadding, perfPadding,
		perfWidth-2*perfPadding, perfGraphHeight))
}

// raise puts the panel above the views a batch inserted after it.
func (h *hud) raise() {
	parent := h.panel.Send(selSuperview)
	if parent == 0 {
		return
	}
	if parent.Send(selSubviews).Send(selLastObject) == h.panel {
		return
	}
	h.panel.Send(selRemoveFromSuperview)
	parent.Send(selAddSubview, h.panel)
}

// stop removes the panel and stops sampling.
func (h *hud) stop() {
	h.once.Do(func() {
		close(h.done)
		delete(perfTraces, h.trace)
		h.panel.Send(selRemoveFromSuperview)
		h.panel.Send(selRelease)
		h.surface.perf = nil
		h.surface.hud = nil
	})
}

// registerPerfClasses registers the panel and the trace view once, when the
// HUD is first shown.
func registerPerfClasses() {
	perfClassOnce.Do(func() {
		nsView := objc.GetClass("NSView")
		passThrough := func(objc.ID, objc.SEL, apple.CGPoint) objc.ID { return 0 }
		// Tracking areas ignore hitTest, and a transparent view keeps its
		// own; a hidden one would not report the pointer leaving.
		fade := func(alpha float64) func(objc.ID, objc.SEL, objc.ID) {
			return func(self objc.ID, _ objc.SEL, _ objc.ID) {
				self.Send(sel("animator")).Send(sel("setAlphaValue:"), alpha)
			}
		}
		classPerfPanel = register("NsgoPerfPanel", nsView, nil, []objc.MethodDef{
			{Cmd: sel("hitTest:"), Fn: passThrough},
			{Cmd: sel("mouseEntered:"), Fn: fade(0)},
			{Cmd: sel("mouseExited:"), Fn: fade(1)},
		})
		classPerfTrace = register("NsgoPerfTrace", nsView, nil, []objc.MethodDef{
			{Cmd: sel("hitTest:"), Fn: passThrough},
			{Cmd: sel("drawRect:"), Fn: drawPerfTrace},
		})
	})
}

// drawPerfTrace draws the trace of a HUD's trace view.
func drawPerfTrace(self objc.ID, _ objc.SEL, _ apple.CGRect) {
	h := perfTraces[self]
	if h == nil {
		return
	}
	context := objc.ID(objc.GetClass("NSGraphicsContext")).Send(selCurrentContext)
	if context == 0 {
		return
	}
	ctx := objc.Send[uintptr](context, selCGContext)
	if ctx == 0 {
		return
	}
	bounds := objc.Send[apple.CGRect](self, selBounds)
	drawTrace(ctx, bounds, h.frames, h.budget)
}

// drawTrace draws bars of the frames' cost, newest at the right; the scale is
// twice the budget, its line at half height.
func drawTrace(ctx uintptr, bounds apple.CGRect, frames []float64, budget float64) {
	w, h := bounds.Size.Width, bounds.Size.Height
	fillColor := apple.CGColor(perfTraceFill)
	apple.FillRects(ctx, []apple.CGRect{apple.Rect(0, 0, w, h)}, fillColor)
	apple.Release(fillColor)
	if budget < 0.1 {
		budget = 0.1
	}
	scale := h / (2 * budget)
	const slots = 120
	bar := w / slots
	start := slots - min(len(frames), slots)
	var good, warn, bad []apple.CGRect
	for i, ms := range frames[max(0, len(frames)-slots):] {
		height := min(max(ms*scale, 1), h)
		x := float64(start+i) * bar
		rect := apple.Rect(x, 0, max(bar-0.5, 0.5), height)
		switch {
		case ms <= budget/2:
			good = append(good, rect)
		case ms <= budget:
			warn = append(warn, rect)
		default:
			bad = append(bad, rect)
		}
	}
	fillRects(ctx, good, perfTraceGood)
	fillRects(ctx, warn, perfTraceWarn)
	fillRects(ctx, bad, perfTraceBad)
	lineColor := apple.CGColor(perfTraceBudget)
	apple.FillRects(ctx, []apple.CGRect{apple.Rect(0, float64(int(h/2)), w, 1)}, lineColor)
	apple.Release(lineColor)
}

func fillRects(ctx uintptr, rects []apple.CGRect, c core.Color) {
	if len(rects) == 0 {
		return
	}
	color := apple.CGColor(c)
	apple.FillRects(ctx, rects, color)
	apple.Release(color)
}

// processSampler reads the process' CPU and memory use. CPU use is a rate:
// each sample reports the CPU time since the previous one over the wall time
// between them (the first reports none).
type processSampler struct {
	at  time.Time
	cpu time.Duration
	has bool
}

func (p *processSampler) sample() engine.ProcessSample {
	now := time.Now()
	var out engine.ProcessSample
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
		used := time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
		if p.has {
			if wall := now.Sub(p.at); wall > 0 {
				percent := float32(float64(used-p.cpu) / float64(wall) * 100)
				out.CPU = &percent
			}
		}
		p.at, p.cpu, p.has = now, used, true
	}
	if memory, ok := apple.ProcessMemory(); ok {
		out.Memory = &memory
	}
	return out
}
