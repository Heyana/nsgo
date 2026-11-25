package web

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

func TestPerfClockIntervals(t *testing.T) {
	var clock perfClock

	// Before two frames arrive, fall back to any estimated display rate.
	if got := clock.next(16*time.Millisecond, 0); got != 0 {
		t.Fatalf("first frame without fallback = %v, want 0", got)
	}
	clock = perfClock{}
	if got := clock.next(16*time.Millisecond, 120); got != time.Second/120 {
		t.Fatalf("first frame with 120 Hz fallback = %v, want %v", got, time.Second/120)
	}

	// Consecutive 120 Hz refreshes record the measured cadence.
	step := time.Duration(8333 * time.Microsecond)
	if got := clock.next(16*time.Millisecond+step, 60); got != step {
		t.Fatalf("second frame interval = %v, want %v", got, step)
	}

	// A pause >= 100 ms (background tab) preserves the last valid period.
	if got := clock.next(16*time.Millisecond+step+250*time.Millisecond, 60); got != step {
		t.Fatalf("interval after pause = %v, want preserved %v", got, step)
	}

	// A subsequent 60 Hz cadence updates the period.
	slow := 16667 * time.Microsecond
	if got := clock.next(16*time.Millisecond+step+250*time.Millisecond+slow, 120); got != slow {
		t.Fatalf("updated interval = %v, want %v", got, slow)
	}
}

func TestPerfPanelLayoutAndTrace(t *testing.T) {
	panelW, traceW := perfPanelLayout(1200)
	if panelW != engine.PerfWidth || traceW != engine.PerfWidth-2*engine.PerfPadding {
		t.Fatalf("desktop layout = (%v, %v), want (%v, %v)", panelW, traceW, engine.PerfWidth, engine.PerfWidth-2*engine.PerfPadding)
	}

	// Narrow embedded hosts clamp the panel within margins without overflow.
	panelW, traceW = perfPanelLayout(200)
	if panelW != 180 || traceW != 180-2*engine.PerfPadding {
		t.Fatalf("narrow layout = (%v, %v), want (%v, %v)", panelW, traceW, 180.0, 180-2*engine.PerfPadding)
	}

	// Very narrow hosts leave no negative trace width.
	panelW, traceW = perfPanelLayout(16)
	if panelW != 0 || traceW != 0 {
		t.Fatalf("tiny layout = (%v, %v), want (0, 0)", panelW, traceW)
	}
	if bars := perfTraceBars(engine.PerfSnapshot{}, traceW); bars != nil {
		t.Fatalf("expected nil trace bars when traceW=0, got %d", len(bars))
	}

	// Monitor recordings format into Readings and PerfTrace bars.
	m := engine.NewPerfMonitor()
	t0 := time.Now()
	for i := range 8 {
		m.RecordWork(2*time.Millisecond, true)
		m.Vsync(t0.Add(time.Duration(i)*16*time.Millisecond), 16*time.Millisecond)
		m.RecordLatency(500 * time.Microsecond)
	}
	snap := m.Snapshot()
	if snap.RefreshRate == nil || *snap.RefreshRate < 50 {
		t.Fatalf("unexpected refresh rate: %v", snap.RefreshRate)
	}
	if !strings.Contains(snap.Readings(), "FPS") || !strings.Contains(snap.Readings(), "MAIN") || !strings.Contains(snap.Readings(), "HOP") {
		t.Fatalf("unexpected readings:\n%s", snap.Readings())
	}
	bars := perfTraceBars(snap, engine.PerfWidth-2*engine.PerfPadding)
	if len(bars) < 3 {
		t.Fatalf("expected background, frame bars and budget line, got %d bars", len(bars))
	}
	if bars[0].Color != engine.PerfTraceFill || bars[len(bars)-1].Color != engine.PerfTraceBudget {
		t.Fatalf("unexpected trace framing bars: first=%v last=%v", bars[0], bars[len(bars)-1])
	}
}

func TestPerfPanelStylesAndCanvasHelpers(t *testing.T) {
	whole := perfPanelStyles(true)
	if whole["pointer-events"] != "none" || whole["position"] != "absolute" {
		t.Fatalf("unexpected whole-page pointer/position styles: %+v", whole)
	}
	if !strings.Contains(whole["top"], "safe-area-inset-top") || !strings.Contains(whole["right"], "safe-area-inset-right") {
		t.Fatalf("whole-page panel should offset from safe-area insets: top=%q right=%q", whole["top"], whole["right"])
	}
	if whole["background-color"] != webstyle.Color(engine.PerfPanelColor) || whole["color"] != webstyle.Color(engine.PerfTextColor) {
		t.Fatalf("unexpected panel colors: bg=%q fg=%q", whole["background-color"], whole["color"])
	}
	if whole["border-radius"] != webstyle.Px(engine.PerfRadius) || whole["width"] != webstyle.Px(engine.PerfWidth) {
		t.Fatalf("unexpected panel box: radius=%q width=%q", whole["border-radius"], whole["width"])
	}

	embedded := perfPanelStyles(false)
	if embedded["top"] != webstyle.Px(engine.PerfMargin) || embedded["right"] != webstyle.Px(engine.PerfMargin) {
		t.Fatalf("embedded panel should offset directly from its mount host: top=%q right=%q", embedded["top"], embedded["right"])
	}
	if embedded["pointer-events"] != "none" {
		t.Fatalf("embedded panel must be pointer-transparent: %q", embedded["pointer-events"])
	}

	label := perfLabelStyles()
	if label["font-size"] != webstyle.Px(engine.PerfFontSize) || label["white-space"] != "pre-wrap" || label["pointer-events"] != "none" {
		t.Fatalf("unexpected label styles: %+v", label)
	}
	if perfIntervalMs() != 500 {
		t.Fatalf("perfIntervalMs = %d, want 500", perfIntervalMs())
	}
	if px := perfCanvasPixels(246, 2); px != 492 {
		t.Fatalf("perfCanvasPixels(246, 2) = %d, want 492", px)
	}
	if px := perfCanvasPixels(0, 0.5); px != 1 {
		t.Fatalf("perfCanvasPixels(0, 0.5) = %d, want 1", px)
	}
	if dpr := perfPixelRatio(0); dpr != 1 {
		t.Fatalf("perfPixelRatio(0) = %v, want 1", dpr)
	}
}
