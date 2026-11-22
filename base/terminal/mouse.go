package terminal

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// MouseScrollMultiplier scales a program's scrolling: Precision for touchpads, Discrete for mouse wheels. Zero uses Ghostty's defaults, 1 and 3; positive values are clamped to [0.01, 10000].
type MouseScrollMultiplier struct {
	Precision, Discrete float64
}

func mouseMultiplier(value, fallback float64) float64 {
	if value <= 0 || math.IsNaN(value) {
		return fallback
	}
	return min(max(value, .01), 10000)
}

func (v *view) mouseEnabled() bool {
	return v.p.MouseReporting == nil || v.p.MouseReporting()
}

type wheelTarget uint8

const (
	wheelScrollback wheelTarget = iota
	wheelMouse
	wheelArrows
)

// wheelTarget is read with the terminal locked. A reporting preference cannot turn a program's explicit mouse mode into alternate-scroll keys.
func (v *view) wheelTarget(enabled bool) wheelTarget {
	t := v.t
	if t == nil || t.term == nil {
		return wheelScrollback
	}
	if t.term.MouseTracking() {
		if enabled {
			return wheelMouse
		}
		return wheelScrollback
	}
	if t.term.AltScreen() && t.term.Mode(vt.ModeAltScroll) {
		return wheelArrows
	}
	return wheelScrollback
}

// hitView keeps its native identity while mouse modes change. Only a program that needs wheel input installs OnWheel, so the main screen's scrollback retains the platform's native scrolling and inertia.
func (v *view) hitView() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		target := core.NewMemo(cx, func() wheelTarget {
			v.rev.Get()
			enabled := v.mouseEnabled()
			if v.t == nil {
				return wheelScrollback
			}
			v.t.mu.Lock()
			defer v.t.mu.Unlock()
			return v.wheelTarget(enabled)
		})
		element := core.NewElement(cx, core.View{})
		last := wheelScrollback
		cx.Effect(func() {
			mode := target.Get()
			if mode != last {
				v.wheelX, v.wheelY, last = 0, 0, mode
			}
			cursor := core.CursorText
			if v.linkOn.Get() {
				cursor = core.CursorPointer
			}
			props := core.View{NoFocus: true, TestID: v.testID("screen"),
				Style:       core.Style{Height: core.Pt(v.content.Get()), Cursor: cursor},
				OnPressDown: v.pressDown, OnPressUp: v.pressUp,
				OnMouseDown: v.mouseDown, OnMouseUp: v.mouseUp,
				OnMouseMove: v.mouseMove, OnMouseLeave: v.mouseLeave,
				OnLongPress: v.longPress, DelayLongPress: 200 * time.Millisecond}
			if mode != wheelScrollback {
				props.OnWheel = v.wheel
			}
			if mode == wheelMouse {
				// A program's right click must not also open an app menu.
				props.OnContextMenu = func(core.ContextMenuEvent) {}
			}
			element.Set(props)
		})
		return element
	})
}

func mouseButton(button core.MouseButton) int {
	switch button {
	case core.MouseButtonLeft:
		return vt.ButtonLeft
	case core.MouseButtonRight:
		return vt.ButtonRight
	case core.MouseButtonMiddle:
		return vt.ButtonMiddle
	case core.MouseButtonBack:
		return 8
	case core.MouseButtonForward:
		return 9
	}
	return vt.ButtonNone
}

func mouseMods(mods core.Modifiers) vt.Mods {
	var out vt.Mods
	if mods.Shift {
		out |= vt.ModShift
	}
	if mods.Control {
		out |= vt.ModCtrl
	}
	if mods.Alt {
		out |= vt.ModAlt
	}
	if mods.Platform {
		out |= vt.ModSuper
	}
	return out
}

// reportMouse encodes under t.mu. Cell protocols use cell centers on the integer geometry the ABI takes, avoiding drift from fractional advances. Pixel mode keeps the actual rendered pixels instead.
func (v *view) reportMouse(action vt.MouseAction, button int, mods core.Modifiers, pos core.Point) {
	t := v.t
	if t.term == nil {
		return
	}
	if v.mouse == nil {
		var err error
		if v.mouse, err = vt.NewMouseEncoder(); err != nil {
			return
		}
	}
	cw, ch := max(int(v.cellW*v.ratio), 1), max(int(v.cellH*v.ratio), 1)
	col, row, px, py := v.cellAt(pos)
	x, y := (float32(col)+.5)*float32(cw), (float32(row)+.5)*float32(ch)
	width, height := v.cols*cw, v.rows*ch
	if t.term.Mode(vt.Mode(1016)) {
		x, y = float32(px), float32(py)
		width, height = int(float32(v.cols)*v.cellW*v.ratio), int(float32(v.rows)*v.cellH*v.ratio)
	}
	v.mouse.Sync(t.term, width, height, cw, ch, 0, 0, v.mouseHeld != 0)
	if data := v.mouse.Encode(action, button, mouseMods(mods), x, y); len(data) > 0 {
		t.Send(data)
	}
}

func (v *view) mouseDown(e core.MouseButtonEvent) {
	enabled := v.mouseEnabled()
	t := v.t
	if t == nil || !enabled || e.Modifiers.Shift {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || !t.term.MouseTracking() {
		return
	}
	col, row, _, _ := v.cellAt(e.Position)
	if e.Button == core.MouseButtonLeft && v.opensLinks(e.Modifiers) && linkAt(t.term, col, row).url != "" {
		return
	}
	v.mouseHeld |= 1 << e.Button
	v.reportMouse(vt.MousePress, mouseButton(e.Button), e.Modifiers, e.Position)
	t.term.SetSelection(nil)
	t.redraw()
}

func (v *view) mouseUp(e core.MouseButtonEvent) {
	t := v.t
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if v.mouseHeld&(1<<e.Button) == 0 {
		return
	}
	v.mouseHeld &^= 1 << e.Button
	v.reportMouse(vt.MouseRelease, mouseButton(e.Button), e.Modifiers, e.Position)
}

func (v *view) reportMotion(e core.MouseEvent) {
	enabled := v.mouseEnabled()
	t := v.t
	if t == nil || !enabled || e.Modifiers.Shift {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term != nil && t.term.MouseTracking() {
		button := e.PressedButton
		if button == core.MouseButtonNone {
			for b := core.MouseButtonLeft; b <= core.MouseButtonForward; b++ {
				if v.mouseHeld&(1<<b) != 0 {
					button = b
					break
				}
			}
		}
		v.reportMouse(vt.MouseMotion, mouseButton(button), e.Modifiers, e.Position)
	}
}

func (v *view) wheel(e core.WheelEvent) {
	enabled := v.mouseEnabled()
	t := v.t
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	mode := v.wheelTarget(enabled)
	if mode == wheelScrollback {
		return
	}
	if e.Phase == core.GestureBegan {
		v.wheelX, v.wheelY = 0, 0
	}
	xUnit, yUnit := float64(v.cellW), float64(v.cellH)
	mult := mouseMultiplier(v.p.MouseScrollMultiplier.Precision, 1)
	if !e.Touchpad {
		unit := 40.0
		if os := v.cx.Env().OS(); os == core.OSWindows || os == core.OSLinux {
			unit = 33
		}
		xUnit, yUnit, mult = unit, unit, mouseMultiplier(v.p.MouseScrollMultiplier.Discrete, 3)
	}
	x := wheelSteps(&v.wheelX, float64(e.Delta.X)/max(xUnit, 1)*mult)
	y := wheelSteps(&v.wheelY, float64(e.Delta.Y)/max(yUnit, 1)*mult)
	t.term.SetSelection(nil)
	if mode == wheelMouse {
		for range abs(y) {
			button := vt.WheelDown
			if y < 0 {
				button = vt.WheelUp
			}
			v.reportMouse(vt.MousePress, button, e.Modifiers, e.Position)
		}
		for range abs(x) {
			button := vt.WheelRight
			if x < 0 {
				button = vt.WheelLeft
			}
			v.reportMouse(vt.MousePress, button, e.Modifiers, e.Position)
		}
	} else {
		sequence := "\x1b[B"
		if y < 0 {
			sequence = "\x1b[A"
		}
		if t.term.Mode(vt.ModeCursorKeys) {
			sequence = "\x1bOB"
			if y < 0 {
				sequence = "\x1bOA"
			}
		}
		for range abs(y) {
			t.Send([]byte(sequence))
		}
	}
	t.redraw()
}

func wheelSteps(pending *float64, delta float64) int {
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return 0
	}
	*pending += delta
	steps := int(math.Trunc(*pending))
	*pending -= float64(steps)
	return steps
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
