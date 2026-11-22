package linux

import "time"

// testInputGate isolates a Linux test host's GTK windows from physical user input: while a window is opened by nsgotest (w.opts.test != nil), physical pointer, touch, scroll, keyboard and gesture callbacks are ignored unless they run inside a scripted testSurface step. Ordinary apps and live automation leave w.opts.test nil and accept all input. Accessed only on the GTK main thread.
type testInputGate struct {
	depth    int
	injected time.Time // until when the X11 events NSGo injected itself (Ref.SendInput through XTest) pass
}

func (g *testInputGate) enter() {
	g.depth++
}

// expectInjected lets physical events through until until, for the X11 events NSGo injects itself through XTest: they come back from the X server as physical input after the step that sent them has returned, and the web view they are for must still get them.
func (g *testInputGate) expectInjected(until time.Time) {
	if until.After(g.injected) {
		g.injected = until
	}
}

func (g *testInputGate) leave() {
	if g.depth > 0 {
		g.depth--
	}
}

// allow reports whether a GTK input callback may run on a window whose test-mode isolation is isolated.
func (g *testInputGate) allow(isolated bool) bool {
	return !isolated || g.depth > 0 || time.Now().Before(g.injected)
}

// allowGDKEvent reports whether a GDK event of type typ may pass through the capture-phase shield on a window whose test-mode isolation is isolated.
func (g *testInputGate) allowGDKEvent(isolated bool, typ int32) bool {
	if g.allow(isolated) {
		return true
	}
	return !isPhysicalGDKEvent(typ)
}

// isPhysicalGDKEvent reports whether a GDK 4 GdkEventType delivers physical pointer, touch, scroll, keyboard, touchpad or tablet-pad input rather than window lifecycle, focus or drag-and-drop state events.
func isPhysicalGDKEvent(typ int32) bool {
	switch {
	case typ >= 1 && typ <= 7: // GDK_MOTION_NOTIFY..GDK_LEAVE_NOTIFY
		return true
	case typ == 9 || typ == 10: // GDK_PROXIMITY_IN, GDK_PROXIMITY_OUT
		return true
	case typ == 15: // GDK_SCROLL
		return true
	case typ >= 17 && typ <= 27: // GDK_TOUCH_*, GDK_TOUCHPAD_*, GDK_PAD_*
		return true
	default:
		return false
	}
}
