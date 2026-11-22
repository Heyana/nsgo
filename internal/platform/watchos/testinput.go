package watchos

// Host.swift's scroll phases.
const (
	phaseOffset uint8 = iota
	phaseDragging
	phaseGliding
	phaseRest
	phaseAnimating
)

// testInputGate isolates the watchOS test host from physical touches, Digital
// Crown / finger scrolls, SwiftUI text-field edits and alert taps while
// allowing scripted testSurface steps and programmatic ScrollTo updates.
// Ordinary apps and live automation (nsgoautomation) leave isolation off and
// accept all input. Accessed only on the main thread.
type testInputGate struct {
	isolated bool
	depth    int
	// scrollSeen records scroll views that have reported their initial resting
	// offset from SwiftUI layout; scrollExpected records those awaiting an
	// opScrollTo update (from testSurface.Scroll or Ref.ScrollTo);
	// scrollPhysical records those currently being dragged or rotated with the
	// Digital Crown.
	scrollSeen     map[uint32]bool
	scrollExpected map[uint32]bool
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

// allowDirect reports whether a direct user-input callback (hostTouch,
// hostText, hostAlert) may run when test-mode isolation is isolated.
func (g *testInputGate) allowDirect(isolated bool) bool {
	if !isolated && !g.isolated {
		return true
	}
	return g.depth > 0
}

// expectScroll marks scroll view id as awaiting programmatic scroll callbacks
// from an opScrollTo command sent to SwiftUI.
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
	delete(g.scrollPhysical, id)
}

// allowScroll reports whether a hostScroll callback for scroll view id with
// phase may run when test-mode isolation is isolated. Physical drags and
// Digital Crown turns (phaseDragging, phaseGliding and offsets outside a
// programmatic ScrollTo or initial layout) are rejected.
func (g *testInputGate) allowScroll(isolated bool, id uint32, phase uint8) bool {
	if (!isolated && !g.isolated) || g.depth > 0 {
		return true
	}
	switch phase {
	case phaseDragging, phaseGliding:
		if g.scrollPhysical == nil {
			g.scrollPhysical = map[uint32]bool{}
		}
		g.scrollPhysical[id] = true
		delete(g.scrollExpected, id)
		return false
	case phaseRest:
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
		return !g.scrollPhysical[id] && g.scrollExpected[id]
	case phaseOffset:
		if g.scrollPhysical[id] {
			return false
		}
		if g.scrollExpected[id] {
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
