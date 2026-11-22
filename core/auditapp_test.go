package core_test

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The layout audit's border and grid colors, as core draws them.
var (
	auditBorder      = core.RGBA(0x3b82f6, 0.9)
	auditMargin      = core.RGBA(0xf97316, 0.8)
	auditInside      = core.RGBA(0x22c55e, 0.8)
	auditGrid        = core.RGBA(0x269182, 0.16)
	auditCursorGuide = core.RGBA(0xd8508a, 0.35)
)

// auditSample is an app of a padded column with a button, an input and a popover that closes on presses outside it, which log what the app hears, under DevTools' overlay with the layout audit starting as layout says.
func auditSample(log *[]string, layout core.LayoutAudit) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		popover := core.NewSignal(cx, true)
		cx.OnKey("escape", func() { *log = append(*log, "escape") })
		cx.OnKey("mod+shift+g", func() { *log = append(*log, "app grid") })
		return core.Group{
			core.View{TestID: "app", Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 8}, Children: []core.Node{
				core.View{TestID: "button", Style: core.Style{Height: core.Pt(40), Margin: core.Edges{Top: 4}},
					OnPress: func(core.PressEvent) { *log = append(*log, "press") }},
				core.Input{TestID: "input", Style: core.Style{Height: core.Pt(30)}},
				core.Show{When: popover.Get, Then: func(cx *core.Scope) core.Node {
					return core.Overlay{TestID: "popover", Style: core.Style{Top: core.Pt(300), Left: core.Pt(100), Width: core.Pt(80), Height: core.Pt(40)},
						OnPressOutside: func(core.Point) { *log = append(*log, "outside"); popover.Set(false) }}
				}},
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: layout},
		}
	}
}

// strokes counts the strokes drawn in color among a canvas's ops.
func strokes(ops []driver.DrawOp, color core.Color) int {
	n := 0
	for _, op := range ops {
		if op.Kind == driver.DrawStrokeColor && op.Color == color {
			n++
		}
	}
	return n
}

func TestLayoutAuditIsAbsentWhenHidden(t *testing.T) {
	var log []string
	plain := nsgotest.Start(t, func(cx *core.Scope) core.Node { return auditSample(&log, core.LayoutAudit{})(cx).(core.Group)[0] })
	audited := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{}))
	if plain.Find("id:button").Frame != audited.Find("id:button").Frame {
		t.Errorf("the button is at %v under DevTools' overlay, %v without", audited.Find("id:button").Frame, plain.Find("id:button").Frame)
	}
	if audited.Exists("id:nsgo-layout-audit") {
		t.Error("an audit that starts hidden shows")
	}
}

func TestLayoutAuditBoundsAndGridPassInputThrough(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{Bounds: true, Grid: 8}))
	canvas := app.Find("id:nsgo-layout-canvas")
	// The root view, the app, the button, the input and the popover.
	if got := strokes(canvas.Ops, auditBorder); got != 5 {
		t.Errorf("drew %d border boxes, want 5", got)
	}
	if strokes(canvas.Ops, auditMargin) != 1 || strokes(canvas.Ops, auditInside) != 1 {
		t.Error("the button's margin or the app's padding is not drawn")
	}
	if strokes(canvas.Ops, auditGrid) != 1 {
		t.Error("no grid")
	}
	app.Press("id:button")
	app.Type("id:input", "hi")
	if app.Find("id:input").Text != "hi" {
		t.Errorf("input = %q, want what was typed through the audit", app.Find("id:input").Text)
	}
	if !slices.Equal(log, []string{"outside", "press"}) {
		t.Errorf("log = %q, want the press through the audit, closing the popover", log)
	}
	if app.Exists("id:popover") {
		t.Error("the popover stayed open")
	}
	// The drawing follows the layout.
	app.Resize(300, 500)
	if got := strokes(app.Find("id:nsgo-layout-canvas").Ops, auditBorder); got != 4 {
		t.Errorf("drew %d border boxes after the popover closed, want 4", got)
	}
}

func TestLayoutAuditKeysAndReadout(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{Bounds: true}))
	app.Press("id:nsgo-layout-bounds")
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditBorder) != 0 {
		t.Error("the boxes' button left them drawn")
	}
	app.Press("id:nsgo-layout-grid")
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditGrid) != 1 {
		t.Error("the grid's button drew no grid")
	}
	app.Key("mod+shift+g")
	if !slices.Equal(log, []string{"app grid"}) {
		t.Errorf("log = %q: the audit binds keys besides mod+alt+shift+l", log)
	}
	app.Key("mod+alt+shift+l")
	if app.Exists("id:nsgo-layout-audit") {
		t.Fatal("mod+alt+shift+l did not hide the audit")
	}
	app.Key("mod+alt+shift+l")
	ops := app.Find("id:nsgo-layout-canvas").Ops
	if strokes(ops, auditGrid) != 1 || strokes(ops, auditBorder) != 0 {
		t.Error("the audit did not show again as it was")
	}
	app.Press("id:nsgo-layout-grid")
	app.Key("mod+alt+shift+l")
	app.Key("mod+alt+shift+l")
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditBorder) == 0 {
		t.Error("an audit shown with neither boxes nor grid draws no boxes")
	}
}

func TestLayoutAuditRulerMeasuresWithoutPressingTheApp(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{Bounds: true}))
	app.Press("id:nsgo-layout-measure")
	log = nil
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 40, Y: 30})
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 210, Y: 130})
	if len(log) != 0 {
		t.Fatalf("log = %q, want the ruler to keep presses from the app and its popover", log)
	}
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 170  ↕ 100" {
		t.Errorf("measurement = %q", got)
	}
	app.Key("escape")
	if len(log) != 0 || app.Exists("id:nsgo-layout-ruler") {
		t.Fatalf("log = %q, ruler shown %v: Escape should leave the ruler before the app hears it", log, app.Exists("id:nsgo-layout-ruler"))
	}
	if app.Exists("id:nsgo-layout-measurement") {
		t.Error("leaving the ruler kept its measurement")
	}
	// The ruler's overlay and the readout it holds are DevTools' own, which the audit leaves out of its boxes.
	before := strokes(app.Find("id:nsgo-layout-canvas").Ops, auditBorder)
	app.Press("id:nsgo-layout-measure")
	if got := strokes(app.Find("id:nsgo-layout-canvas").Ops, auditBorder); got != before {
		t.Errorf("drew %d border boxes while measuring, %d before: the audit outlines its ruler", got, before)
	}
	app.Press("id:nsgo-layout-measure")
	app.Press("id:button")
	if !slices.Equal(log, []string{"outside", "press"}) {
		t.Errorf("log = %q, want presses to reach the app again", log)
	}
}

func TestLayoutAuditRulerKeepsPressesFromAModalsPopover(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Group{
			core.Modal{TestID: "dialog", Children: []core.Node{
				core.Overlay{TestID: "popover", Style: core.Style{Top: core.Pt(300), Left: core.Pt(100), Width: core.Pt(80), Height: core.Pt(40)},
					OnPressOutside: func(core.Point) { log = append(log, "outside") }},
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
		}
	})
	app.Press("id:nsgo-layout-measure")
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 40, Y: 30})
	if len(log) != 0 {
		t.Fatalf("log = %q, want the ruler, above the modal, to keep presses from its popover", log)
	}
}

func TestLayoutAuditReadoutTogglesTheRulerAndBackLeavesIt(t *testing.T) {
	var log []string
	var modal *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		modal = core.NewSignal(cx, false)
		return core.Group{
			core.Show{When: modal.Get, Then: func(cx *core.Scope) core.Node {
				return core.Modal{TestID: "dialog", OnRequestClose: func(string) { log = append(log, "close"); modal.Set(false) }}
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
		}
	})
	app.Press("id:nsgo-layout-measure")
	if !app.Exists("id:nsgo-layout-ruler") {
		t.Fatal("the readout did not start the ruler")
	}
	app.Press("id:nsgo-layout-measure")
	if app.Exists("id:nsgo-layout-ruler") {
		t.Fatal("the readout did not stop the ruler")
	}
	app.Press("id:nsgo-layout-measure")
	app.Post(func() { modal.Set(true) })
	app.Back()
	if app.Exists("id:nsgo-layout-ruler") || len(log) != 0 {
		t.Fatalf("log = %q, ruler shown %v: back should leave the ruler before the app's modal", log, app.Exists("id:nsgo-layout-ruler"))
	}
	app.Back()
	if !slices.Equal(log, []string{"close"}) {
		t.Errorf("log = %q, want the next back at the modal", log)
	}
}

func TestLayoutAuditResizeClearsTheRuler(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{Bounds: true}))
	app.Press("id:nsgo-layout-measure")
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 10, Y: 10})
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 20, Y: 30})
	if !app.Exists("id:nsgo-layout-measurement") {
		t.Fatal("no measurement")
	}
	app.Resize(300, 600)
	if app.Exists("id:nsgo-layout-measurement") {
		t.Error("a resize kept the measurement")
	}
}

func TestDevToolsShowAndHideTheLayoutAudit(t *testing.T) {
	d := startDevApp(t)
	var shown []bool
	d.on(func() { d.surface.WatchLayout(func(on bool) { shown = append(shown, on) }) })
	d.on(func() { d.surface.Layout(true) })
	if !d.app.Exists("id:nsgo-layout-audit") || strokes(d.app.Find("id:nsgo-layout-canvas").Ops, auditBorder) == 0 {
		t.Fatal("DevTools did not show the audit with its boxes")
	}
	// Only an audit that started showing, as nsgo dev -layout's, binds its key.
	d.app.Key("mod+alt+shift+l")
	if !d.app.Exists("id:nsgo-layout-audit") {
		t.Fatal("mod+alt+shift+l hid the audit of an overlay that started without it")
	}
	d.on(func() { d.surface.Layout(false) })
	if d.app.Exists("id:nsgo-layout-audit") {
		t.Fatal("DevTools did not hide the audit")
	}
	d.on(func() { d.surface.WatchLayout(nil) })
	d.on(func() { d.surface.Layout(true) })
	if !slices.Equal(shown, []bool{false, true, false}) {
		t.Errorf("DevTools heard %v, want the audit hidden, then shown, then hidden, and nothing once they stopped", shown)
	}
	// The boxes are the app's: DevTools' overlay and the outlines of what commits changed are left out.
	var root devtools.Node
	d.on(func() { root = d.surface.Elements() })
	var walk func(n devtools.Node) bool
	walk = func(n devtools.Node) bool {
		if n.TestID == "nsgo-layout-audit" {
			return true
		}
		return slices.ContainsFunc(n.Children, walk)
	}
	if walk(root) {
		t.Error("Elements lists the audit's own views")
	}
}

// A surface without DevTools' overlay, as a window desktop.OpenWindow opens, answers that its audit is hidden, so the page's Layout follows it.
func TestDevToolsHearNoLayoutAuditWithoutTheOverlay(t *testing.T) {
	devtools.Enabled = true
	t.Cleanup(func() { devtools.Enabled = false })
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return core.View{TestID: "app"} })
	surfaces := devtools.Surfaces()
	if len(surfaces) == 0 {
		t.Fatal("the tree did not register")
	}
	s := surfaces[len(surfaces)-1]
	var shown []bool
	app.Post(func() {
		s.Layout(true)
		s.WatchLayout(func(on bool) { shown = append(shown, on) })
	})
	app.Settle()
	if !slices.Equal(shown, []bool{false}) {
		t.Errorf("DevTools heard %v, want the audit hidden", shown)
	}
}

func TestLayoutAuditReadoutTakesFocusOnTVAndSkipsTabOnDesktop(t *testing.T) {
	var desktopLog []string
	desktop := nsgotest.Start(t, auditSample(&desktopLog, core.LayoutAudit{Bounds: true}),
		nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	for _, id := range []string{"nsgo-layout-bounds", "nsgo-layout-grid", "nsgo-layout-measure"} {
		if !desktop.Find("id:" + id).Props.NoFocus {
			t.Errorf("desktop: %s is focusable, want out of Tab order", id)
		}
	}
	desktop.Key("Tab")
	desktop.Key("Tab")
	desktop.Key("Tab")
	for _, id := range []string{"nsgo-layout-bounds", "nsgo-layout-grid", "nsgo-layout-measure"} {
		if desktop.Find("id:" + id).Focused {
			t.Errorf("desktop Tab reached %s", id)
		}
	}

	var tvLog []string
	tv := nsgotest.Start(t, auditSample(&tvLog, core.LayoutAudit{Bounds: true}),
		nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomTV}))
	for _, id := range []string{"nsgo-layout-bounds", "nsgo-layout-grid", "nsgo-layout-measure"} {
		if tv.Find("id:" + id).Props.NoFocus {
			t.Errorf("tvOS: %s is NoFocus, want focusable from the remote", id)
		}
	}
	tv.Focus("id:nsgo-layout-bounds")
	if !tv.Find("id:nsgo-layout-bounds").Focused {
		t.Fatal("tvOS focus did not reach nsgo-layout-bounds")
	}
	if tv.Find("id:nsgo-layout-bounds").Style.Outline.Width <= 0 {
		t.Error("focused readout button shows no focus outline")
	}
	tv.Key("Return")
	if strokes(tv.Find("id:nsgo-layout-canvas").Ops, auditBorder) != 0 {
		t.Error("Return on focused nsgo-layout-bounds did not hide boxes")
	}
	tv.Focus("id:nsgo-layout-grid")
	tv.Key("Space")
	if strokes(tv.Find("id:nsgo-layout-canvas").Ops, auditGrid) != 1 {
		t.Error("Space on focused nsgo-layout-grid did not show grid")
	}
}

func TestLayoutAuditRemoteRulerMeasuresAndRestoresFocus(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{Bounds: true}),
		nsgotest.Size(400, 400),
		nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomTV}))

	app.Focus("id:nsgo-layout-measure")
	app.Key("Return")
	if !app.Exists("id:nsgo-layout-ruler") {
		t.Fatal("activating nsgo-layout-measure did not open the ruler")
	}
	if !app.Find("id:nsgo-layout-ruler").Focused {
		t.Fatal("opening the ruler on tvOS did not focus nsgo-layout-ruler")
	}
	for _, id := range []string{"nsgo-layout-bounds", "nsgo-layout-grid", "nsgo-layout-measure"} {
		if !app.Find("id:" + id).Props.NoFocus {
			t.Errorf("while measuring on tvOS, %s should not steal focus from the crosshair", id)
		}
	}
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditCursorGuide) == 0 {
		t.Fatal("the remote ruler drew no crosshair guides")
	}

	// Move from center (200, 150) by +16 across, +8 down to (216, 158) and pick the first point with Select (Press at origin).
	app.Key("Right")
	app.Key("Right")
	app.Key("Down")
	app.Press("id:nsgo-layout-ruler")

	// Move +26 across (3×8 + 2×1 fine) and +15 down (2×8 - 1 fine); readout updates live before pinning the second point.
	app.Key("Right")
	app.Key("Right")
	app.Key("Right")
	app.Key("Shift+Right")
	app.Key("Shift+Right")
	app.Key("Down")
	app.Key("Down")
	app.Key("Shift+Up")
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 26  ↕ 15" {
		t.Fatalf("live measurement = %q, want %q", got, "↔ 26  ↕ 15")
	}

	// Pin the second point with Return; moving the crosshair afterward keeps the locked measurement.
	app.Key("Return")
	app.Key("Right")
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 26  ↕ 15" {
		t.Fatalf("locked measurement = %q, want %q", got, "↔ 26  ↕ 15")
	}

	// A third pick (Space) starts a fresh measurement, and swiping/dragging moves the crosshair smoothly.
	app.Key("Space")
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 0  ↕ 0" {
		t.Fatalf("restarted measurement = %q, want %q", got, "↔ 0  ↕ 0")
	}
	app.Drag("id:nsgo-layout-ruler", driver.Point{X: 35, Y: 20})
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 35  ↕ 20" {
		t.Fatalf("after pan measurement = %q, want %q", got, "↔ 35  ↕ 20")
	}

	if len(log) != 0 || !app.Exists("id:popover") {
		t.Fatalf("log = %q, popover shown %v: remote ruler leaked input to the app", log, app.Exists("id:popover"))
	}

	// Menu (Back) exits the ruler, clears the measurement and crosshair, and restores focus to the measure button.
	app.Back()
	if app.Exists("id:nsgo-layout-ruler") || app.Exists("id:nsgo-layout-measurement") {
		t.Fatal("Back did not leave the ruler and clear its measurement")
	}
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditCursorGuide) != 0 {
		t.Error("leaving the ruler kept crosshair guides drawn")
	}
	if !app.Find("id:nsgo-layout-measure").Focused {
		t.Error("leaving the ruler did not restore focus to nsgo-layout-measure")
	}

	// Re-entering from the refocused measure button and leaving with Escape also restores focus without leaking Escape to the app.
	app.Key("Return")
	if !app.Find("id:nsgo-layout-ruler").Focused {
		t.Fatal("re-entering the ruler did not focus nsgo-layout-ruler")
	}
	app.Key("Escape")
	if len(log) != 0 || app.Exists("id:nsgo-layout-ruler") || !app.Find("id:nsgo-layout-measure").Focused {
		t.Fatalf("Escape: log=%q, ruler=%v, measureFocused=%v", log, app.Exists("id:nsgo-layout-ruler"), app.Find("id:nsgo-layout-measure").Focused)
	}
}

func TestLayoutAuditRemoteRulerFollowsGridStepAndWorksOverModal(t *testing.T) {
	var log []string
	var modal *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		modal = core.NewSignal(cx, true)
		return core.Group{
			core.Show{When: modal.Get, Then: func(cx *core.Scope) core.Node {
				return core.Modal{TestID: "dialog", OnRequestClose: func(string) { log = append(log, "close"); modal.Set(false) },
					Children: []core.Node{core.View{TestID: "modal-btn", Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { log = append(log, "modal-press") }}}}
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true, Grid: 12}},
		}
	}, nsgotest.Size(400, 300), nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomTV}))

	app.Focus("id:nsgo-layout-measure")
	app.Key("Return")
	if !app.Find("id:nsgo-layout-ruler").Focused {
		t.Fatal("ruler did not take focus over open modal")
	}
	app.Key("Return")
	app.Key("Right")
	app.Key("Down")
	app.Key("Down")
	app.Key("Return")
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 12  ↕ 24" {
		t.Fatalf("grid-stepped measurement = %q, want %q", got, "↔ 12  ↕ 24")
	}
	app.Back()
	if app.Exists("id:nsgo-layout-ruler") || !app.Exists("id:dialog") || len(log) != 0 {
		t.Fatalf("Back should close the ruler before the modal: ruler=%v dialog=%v log=%q", app.Exists("id:nsgo-layout-ruler"), app.Exists("id:dialog"), log)
	}
	app.Back()
	if !slices.Equal(log, []string{"close"}) {
		t.Errorf("second Back log = %q, want [close]", log)
	}
}

func TestLayoutAuditKeyboardCrosshairOnDesktop(t *testing.T) {
	var log []string
	app := nsgotest.Start(t, auditSample(&log, core.LayoutAudit{Bounds: true}),
		nsgotest.Size(400, 300),
		nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	app.Press("id:nsgo-layout-measure")
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditCursorGuide) != 0 {
		t.Fatal("desktop pointer ruler drew crosshair guides before any arrow key")
	}
	app.Key("Right")
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditCursorGuide) == 0 {
		t.Fatal("arrow key did not activate the crosshair on desktop")
	}
	app.Key("Return")
	app.Key("Down")
	app.Key("Return")
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 0  ↕ 8" {
		t.Fatalf("keyboard measurement on desktop = %q, want %q", got, "↔ 0  ↕ 8")
	}
	// Switching back to pointer presses clears the crosshair and picks exact pointer points.
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 10, Y: 20})
	app.PressAt("id:nsgo-layout-ruler", driver.Point{X: 60, Y: 50})
	if strokes(app.Find("id:nsgo-layout-canvas").Ops, auditCursorGuide) != 0 {
		t.Error("pointer press kept the keyboard crosshair active")
	}
	if got := app.Find("id:nsgo-layout-measurement").Text; got != "↔ 50  ↕ 30" {
		t.Fatalf("pointer measurement = %q, want %q", got, "↔ 50  ↕ 30")
	}
}
