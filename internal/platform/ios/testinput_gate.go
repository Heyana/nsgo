//go:build !ios || nsgotest || nsgoautomation

package ios

// testSenderID tags IOHIDEvents injected by testSurface on simulators.
const testSenderID = 0x4e53_474f // "NSGO"

// testInputGate isolates iOS and tvOS test hosts from physical user input while allowing both simulator IOHID events enqueued by testSurface (matched by testSenderID or bounded (window, HID timestamp) history) and synchronous scripted steps on physical devices (depth > 0). Ordinary apps and live automation (nsgoautomation) never install the filter and accept all input. Accessed only on the main thread.
type testInputGate struct {
	depth       int
	hid         [64]scriptedHID
	next, count int
}

type scriptedHID struct {
	window    uintptr
	timestamp uint64
}

func (g *testInputGate) enter() {
	g.depth++
}

func (g *testInputGate) leave() {
	if g.depth > 0 {
		g.depth--
	}
}

func (g *testInputGate) rememberHID(window uintptr, timestamp uint64) {
	if timestamp == 0 {
		return
	}
	g.hid[g.next] = scriptedHID{window: window, timestamp: timestamp}
	g.next = (g.next + 1) % len(g.hid)
	g.count = min(g.count+1, len(g.hid))
}

func (g *testInputGate) hasHIDTimestamp(window uintptr, timestamp uint64) bool {
	if timestamp == 0 {
		return false
	}
	for _, got := range g.hid[:g.count] {
		if got.window == window && got.timestamp == timestamp {
			return true
		}
	}
	return false
}

// allowEvent reports whether a UIEvent of UIEventType kind on window may be delivered when test-mode isolation is isolated. hasHID is true when the event carries an underlying IOHIDEvent with sender and timestamp.
func (g *testInputGate) allowEvent(isolated bool, kind int, window uintptr, hasHID bool, sender, timestamp uint64) bool {
	if !isolated || !isPhysicalUIEventType(kind) {
		return true
	}
	if g.depth > 0 {
		return true
	}
	if !hasHID {
		return false
	}
	return sender == testSenderID || g.hasHIDTimestamp(window, timestamp)
}
