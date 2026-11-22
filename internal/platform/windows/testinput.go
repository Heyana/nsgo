package windows

// testInputGate isolates a Windows test host's windows from physical user input: while a window is opened by nsgotest (w.opts.test != nil), physical mouse, pointer, touch, wheel, keyboard, gesture and context-menu messages are ignored unless they are dispatched inside a scripted testSurface step. Ordinary apps and live automation leave w.opts.test nil and accept all input. Accessed only on the window's thread.
type testInputGate struct {
	depth int
}

func (g *testInputGate) enter() {
	g.depth++
}

func (g *testInputGate) leave() {
	if g.depth > 0 {
		g.depth--
	}
}

// allow reports whether message may be processed on a window whose test-mode isolation is isolated.
func (g *testInputGate) allow(isolated bool, message uint32) bool {
	if !isolated || g.depth > 0 {
		return true
	}
	return !isPhysicalInputMessage(message)
}

// isPhysicalInputMessage reports whether a Win32 message delivers physical pointer, mouse, touch, wheel, keyboard, gesture or context-menu input rather than window lifecycle, paint, focus, menu command or IME state messages.
func isPhysicalInputMessage(message uint32) bool {
	switch {
	case message == 0x0021: // WM_MOUSEACTIVATE
		return true
	case message == 0x007B: // WM_CONTEXTMENU
		return true
	case message >= 0x00A0 && message <= 0x00AD: // WM_NCMOUSEMOVE..WM_NCXBUTTONDBLCLK
		return true
	case message >= 0x0100 && message <= 0x0107, message == 0x0109: // WM_KEYDOWN..WM_SYSDEADCHAR, WM_UNICHAR
		return true
	case message == 0x0119: // WM_GESTURE
		return true
	case message >= 0x0200 && message <= 0x020E: // WM_MOUSEMOVE..WM_MOUSEHWHEEL
		return true
	case message == 0x0240: // WM_TOUCH
		return true
	case message >= 0x0245 && message <= 0x024F: // WM_POINTERUPDATE..WM_POINTERHWHEEL
		return true
	case message >= 0x02A0 && message <= 0x02A3: // WM_NCMOUSEHOVER, WM_MOUSEHOVER, WM_NCMOUSELEAVE, WM_MOUSELEAVE
		return true
	case message == 0x0319: // WM_APPCOMMAND
		return true
	default:
		return false
	}
}
