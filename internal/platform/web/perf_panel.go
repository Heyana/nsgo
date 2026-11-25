package web

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// monospaceFamily is the CSS of core.FontMonospace. It lives here, outside the js/wasm files, so the panel's styles can be tested on any host.
const monospaceFamily = `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`

// perfPauseLimit caps refresh gaps included in the display period estimate:
// longer gaps reflect background tabs or suspended clocks rather than a slow
// display.
const perfPauseLimit = 100 * time.Millisecond

// perfWindow is how many recent refresh gaps perfClock estimates the display's period from.
const perfWindow = 16

// perfClock estimates the display's refresh period from consecutive requestAnimationFrame timestamps. The native backends pass Vsync their display link's nominal period; the web has none, so the clock takes the lower quartile of the recent gaps: a frame the page missed is a gap of two or more periods, which neither moves the estimate nor, through Vsync's rate, the drop budget, unless most recent frames were missed.
type perfClock struct {
	lastStamp time.Duration
	gaps      [perfWindow]time.Duration
	n, next   int
	sorted    [perfWindow]time.Duration
}

// period records stamp (on performance.now's clock) and returns the refresh interval to pass to PerfMonitor.Vsync, falling back to fallbackRate (Hz) until a refresh gap has been observed.
func (c *perfClock) period(stamp time.Duration, fallbackRate float64) time.Duration {
	if c.lastStamp > 0 && stamp > c.lastStamp {
		if gap := stamp - c.lastStamp; gap < perfPauseLimit {
			c.gaps[c.next] = gap
			c.next = (c.next + 1) % perfWindow
			c.n = min(c.n+1, perfWindow)
		}
	}
	c.lastStamp = stamp
	if c.n > 0 {
		sorted := c.sorted[:c.n]
		copy(sorted, c.gaps[:c.n])
		slices.Sort(sorted)
		return sorted[c.n/4]
	}
	if fallbackRate > 0 {
		return time.Duration(float64(time.Second) / fallbackRate)
	}
	return 0
}

// perfPanelLayout returns the panel width and trace canvas width in CSS points
// for a surface host of hostWidth points. Narrow embedded mounts clamp the panel
// within the host's margins instead of overflowing.
func perfPanelLayout(hostWidth float64) (panelW, traceW float64) {
	panelW = engine.PerfWidth
	if hostWidth > 0 {
		avail := max(hostWidth-2*engine.PerfMargin, 0)
		panelW = min(panelW, avail)
	}
	traceW = max(panelW-2*engine.PerfPadding, 0)
	return panelW, traceW
}

// perfPanelStyles returns the inline CSS properties for the performance panel
// container. Whole-page surfaces offset from the browser safe area; embedded
// mounts stay inset from their mount host's top-right corner.
func perfPanelStyles(whole bool) map[string]string {
	margin := webstyle.Px(engine.PerfMargin)
	top, right := margin, margin
	if whole {
		top = "calc(" + margin + " + env(safe-area-inset-top, 0px))"
		right = "calc(" + margin + " + env(safe-area-inset-right, 0px))"
	}
	return map[string]string{
		"position":            "absolute",
		"top":                 top,
		"right":               right,
		"width":               webstyle.Px(engine.PerfWidth),
		"max-width":           "calc(100% - " + webstyle.Px(2*engine.PerfMargin) + ")",
		"padding":             webstyle.Px(engine.PerfPadding),
		"box-sizing":          "border-box",
		"border-radius":       webstyle.Px(engine.PerfRadius),
		"background-color":    webstyle.Color(engine.PerfPanelColor),
		"color":               webstyle.Color(engine.PerfTextColor),
		"display":             "flex",
		"flex-direction":      "column",
		"gap":                 webstyle.Px(engine.PerfPadding),
		"pointer-events":      "none",
		"user-select":         "none",
		"-webkit-user-select": "none",
		"z-index":             "2147483647",
	}
}

// perfLabelStyles returns the inline CSS properties for the panel's readings text.
func perfLabelStyles() map[string]string {
	return map[string]string{
		"margin":               "0",
		"font-family":          monospaceFamily,
		"font-size":            webstyle.Px(engine.PerfFontSize),
		"line-height":          "1.3",
		"font-variant-numeric": "tabular-nums",
		"white-space":          "pre-wrap",
		"overflow-wrap":        "break-word",
		"color":                webstyle.Color(engine.PerfTextColor),
		"pointer-events":       "none",
	}
}

// perfTraceStyles returns the inline CSS properties for the panel's trace canvas.
func perfTraceStyles() map[string]string {
	return map[string]string{
		"display":        "block",
		"width":          "100%",
		"height":         webstyle.Px(engine.PerfGraphHeight),
		"pointer-events": "none",
	}
}

// perfTraceBars returns the trace rectangles for traceW, or nil when the host is
// too narrow for a trace area.
func perfTraceBars(s engine.PerfSnapshot, traceW float64) []engine.PerfBar {
	if traceW <= 0 {
		return nil
	}
	return engine.PerfTrace(s, traceW, engine.PerfGraphHeight)
}

func perfIntervalMs() int {
	return int(engine.PerfInterval / time.Millisecond)
}

func perfCanvasPixels(cssSize, dpr float64) int {
	if dpr < 1 {
		dpr = 1
	}
	px := int(cssSize*dpr + 0.5)
	if px < 1 {
		return 1
	}
	return px
}

func perfPixelRatio(v float64) float64 {
	if v > 0 {
		return v
	}
	return 1
}
