package web

import (
	"strconv"
	"time"

	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// perfFontFamily matches web's monospace font stack (core.FontMonospace) without
// requiring js/wasm so layout helpers can be tested on any host.
const perfFontFamily = `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`

// perfPauseLimit caps refresh gaps included in the display period estimate:
// longer gaps reflect background tabs or suspended clocks rather than a slow
// display.
const perfPauseLimit = 100 * time.Millisecond

// perfClock tracks display refresh intervals from consecutive
// requestAnimationFrame timestamps.
type perfClock struct {
	lastStamp time.Duration
	period    time.Duration
}

// next records stamp (on performance.now's clock) and returns the refresh
// interval to pass to PerfMonitor.Vsync, falling back to fallbackRate (Hz)
// until two consecutive visible refreshes are observed.
func (c *perfClock) next(stamp time.Duration, fallbackRate float64) time.Duration {
	if c.lastStamp > 0 && stamp > c.lastStamp {
		if gap := stamp - c.lastStamp; gap < perfPauseLimit {
			c.period = gap
		}
	}
	c.lastStamp = stamp
	if c.period > 0 {
		return c.period
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
		"font-family":          perfFontFamily,
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

func perfDataAttr() string { return strconv.Itoa(int(engine.PerfWidth)) }
