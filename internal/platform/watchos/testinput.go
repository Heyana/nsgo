package watchos

// Host.swift's scroll phases.
const (
	phaseOffset uint8 = iota
	phaseDragging
	phaseGliding
	phaseRest
	phaseAnimating
)

// Host.swift's touch phases.
const (
	touchBegan uint8 = iota
	touchMoved
	touchEnded
	touchCancelled
)

// Host.swift's text events.
const (
	textChanged uint8 = iota
	textSubmitted
	textFocused
	textBlurred
)

// testInputGate isolates the watchOS test host from physical touches, Digital Crown / finger scrolls, SwiftUI text-field edits and alert taps while allowing scripted testSurface steps, native simulator HID touches posted by testSurface, and programmatic ScrollTo updates. Ordinary apps and live automation (nsgoautomation) leave isolation off and accept all input. Accessed only on the main thread.
type testInputGate struct {
	isolated bool
	depth    int

	// Native simulator HID touch injection tracking (testSurface on watchOS simulator).
	nativeTouchActive int
	nativeTouchSeq    uint64
	touchSeenSeq      uint64
	touchEndedSeq     uint64
	nativeFocusActive bool

	// scrollSeen records scroll views that have reported their initial resting offset from SwiftUI layout; scrollExpected records those awaiting an opScrollTo update (from testSurface.Scroll fallback or Ref.ScrollTo); scrollScripted records scroll views whose drag began during a scripted native HID touch; scrollPhysical records those currently being dragged or rotated physically.
	scrollSeen     map[uint32]bool
	scrollExpected map[uint32]bool
	scrollScripted map[uint32]bool
	scrollPhysical map[uint32]bool
}

func (g *testInputGate) enter() {
	g.depth++
}

func (g *testInputGate) leave() {
	if g.depth > 0 {
		g.depth--
	}
}

// beginNativeTouch opens a scripted native HID touch window and returns its sequence number.
func (g *testInputGate) beginNativeTouch() uint64 {
	g.nativeTouchActive++
	g.nativeTouchSeq++
	return g.nativeTouchSeq
}

// endNativeTouch closes a scripted native HID touch window.
func (g *testInputGate) endNativeTouch() {
	if g.nativeTouchActive > 0 {
		g.nativeTouchActive--
	}
}

func (g *testInputGate) setNativeFocusWindow(on bool) {
	g.nativeFocusActive = on
}

func (g *testInputGate) touchSeenSince(seq uint64) bool {
	return seq > 0 && g.touchSeenSeq >= seq
}

func (g *testInputGate) touchEndedSince(seq uint64) bool {
	return seq > 0 && g.touchEndedSeq >= seq
}

// allowDirect reports whether a direct user-input callback (hostAlert) may run when test-mode isolation is isolated.
func (g *testInputGate) allowDirect(isolated bool) bool {
	if !isolated && !g.isolated {
		return true
	}
	return g.depth > 0
}

// allowTouch reports whether a hostTouch callback may run when test-mode isolation is isolated, admitting both synchronous scripted calls and asynchronous callbacks from native simulator HID touches posted by testSurface.
func (g *testInputGate) allowTouch(isolated bool, phase uint8) bool {
	if !isolated && !g.isolated {
		return true
	}
	if g.depth > 0 {
		return true
	}
	if g.nativeTouchActive > 0 {
		g.touchSeenSeq = g.nativeTouchSeq
		if phase == touchEnded || phase == touchCancelled {
			g.touchEndedSeq = g.nativeTouchSeq
		}
		return true
	}
	return false
}

// allowText reports whether a hostText callback may run when test-mode isolation is isolated. Native SwiftUI focus/blur reports triggered by a scripted native tap are admitted while stray physical edits remain blocked.
func (g *testInputGate) allowText(isolated bool, kind uint8) bool {
	if !isolated && !g.isolated {
		return true
	}
	if g.depth > 0 {
		return true
	}
	if (g.nativeTouchActive > 0 || g.nativeFocusActive) && (kind == textFocused || kind == textBlurred) {
		return true
	}
	return false
}

// allowCrown reports whether a hostCrown callback may run when test-mode isolation is isolated.
func (g *testInputGate) allowCrown(isolated bool) bool {
	if !isolated && !g.isolated {
		return true
	}
	return g.depth > 0
}

// expectScroll marks scroll view id as awaiting programmatic scroll callbacks from an opScrollTo command sent to SwiftUI.
func (g *testInputGate) expectScroll(id uint32) {
	if g.scrollExpected == nil {
		g.scrollExpected = map[uint32]bool{}
	}
	g.scrollExpected[id] = true
	delete(g.scrollPhysical, id)
}

// forgetScroll drops tracked scroll state for a deleted scroll view.
func (g *testInputGate) forgetScroll(id uint32) {
	delete(g.scrollSeen, id)
	delete(g.scrollExpected, id)
	delete(g.scrollScripted, id)
	delete(g.scrollPhysical, id)
}

// allowScroll reports whether a hostScroll callback for scroll view id with phase may run when test-mode isolation is isolated. Physical drags and Digital Crown turns outside a scripted native touch or programmatic ScrollTo are rejected.
func (g *testInputGate) allowScroll(isolated bool, id uint32, phase uint8) bool {
	if (!isolated && !g.isolated) || g.depth > 0 {
		return true
	}
	switch phase {
	case phaseDragging:
		if g.nativeTouchActive > 0 {
			if g.scrollScripted == nil {
				g.scrollScripted = map[uint32]bool{}
			}
			g.scrollScripted[id] = true
			delete(g.scrollPhysical, id)
			return true
		}
		if g.scrollPhysical == nil {
			g.scrollPhysical = map[uint32]bool{}
		}
		g.scrollPhysical[id] = true
		delete(g.scrollExpected, id)
		delete(g.scrollScripted, id)
		return false
	case phaseGliding:
		if g.scrollScripted[id] {
			return true
		}
		if g.scrollPhysical == nil {
			g.scrollPhysical = map[uint32]bool{}
		}
		g.scrollPhysical[id] = true
		delete(g.scrollExpected, id)
		return false
	case phaseRest:
		if g.scrollScripted[id] {
			delete(g.scrollScripted, id)
			return true
		}
		if g.scrollPhysical[id] {
			delete(g.scrollPhysical, id)
			return false
		}
		if g.scrollExpected[id] {
			delete(g.scrollExpected, id)
			return true
		}
		return false
	case phaseAnimating:
		return !g.scrollPhysical[id] && (g.scrollExpected[id] || g.scrollScripted[id])
	case phaseOffset:
		if g.scrollPhysical[id] {
			return false
		}
		if g.scrollScripted[id] || g.nativeTouchActive > 0 || g.scrollExpected[id] {
			if g.scrollSeen == nil {
				g.scrollSeen = map[uint32]bool{}
			}
			g.scrollSeen[id] = true
			return true
		}
		// Allow the initial resting offset SwiftUI reports on first layout.
		if !g.scrollSeen[id] {
			if g.scrollSeen == nil {
				g.scrollSeen = map[uint32]bool{}
			}
			g.scrollSeen[id] = true
			return true
		}
		return false
	default:
		return false
	}
}
