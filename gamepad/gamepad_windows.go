//go:build windows

package gamepad

import (
	"fmt"
	"math"
	"structs"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	xinputMaxUsers = 4

	xinputDPadUp        = 0x0001
	xinputDPadDown      = 0x0002
	xinputDPadLeft      = 0x0004
	xinputDPadRight     = 0x0008
	xinputStart         = 0x0010
	xinputBack          = 0x0020
	xinputLeftThumb     = 0x0040
	xinputRightThumb    = 0x0080
	xinputLeftShoulder  = 0x0100
	xinputRightShoulder = 0x0200
	xinputGuide         = 0x0400
	xinputA             = 0x1000
	xinputB             = 0x2000
	xinputX             = 0x4000
	xinputY             = 0x8000

	xinputLeftThumbDeadzone  = 7849
	xinputRightThumbDeadzone = 8689
	xinputTriggerThreshold   = 30
)

type xinputGamepad struct {
	_            structs.HostLayout
	buttons      uint16
	leftTrigger  uint8
	rightTrigger uint8
	thumbLX      int16
	thumbLY      int16
	thumbRX      int16
	thumbRY      int16
}

type xinputState struct {
	_            structs.HostLayout
	packetNumber uint32
	gamepad      xinputGamepad
}

type xinputVibration struct {
	_               structs.HostLayout
	leftMotorSpeed  uint16
	rightMotorSpeed uint16
}

type xinputCapabilities struct {
	_         structs.HostLayout
	typ       uint8
	subType   uint8
	flags     uint16
	gamepad   xinputGamepad
	vibration xinputVibration
}

var (
	xinputOnce            sync.Once
	xinputGetState        *syscall.LazyProc
	xinputGetCapabilities *syscall.LazyProc
	xinputSetState        *syscall.LazyProc
	winDefault            = &windowsSource{}
)

func loadXInput() *syscall.LazyProc {
	xinputOnce.Do(func() {
		for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
			dll := syscall.NewLazyDLL(name)
			getState := dll.NewProc("XInputGetState")
			if getState.Find() == nil {
				xinputGetState = getState
				if p := dll.NewProc("XInputGetCapabilities"); p.Find() == nil {
					xinputGetCapabilities = p
				}
				if p := dll.NewProc("XInputSetState"); p.Find() == nil {
					xinputSetState = p
				}
				return
			}
		}
	})
	return xinputGetState
}

func platformSource() driver.GamepadSource {
	return winDefault
}

type windowsSource struct {
	mu        sync.Mutex
	connected [xinputMaxUsers]bool
	canRumble [xinputMaxUsers]bool
	timers    [xinputMaxUsers]*time.Timer
	gens      [xinputMaxUsers]uint64
}

func (w *windowsSource) Poll() []driver.GamepadState {
	proc := loadXInput()
	if proc == nil {
		return nil
	}
	var out []driver.GamepadState
	for slot := 0; slot < xinputMaxUsers; slot++ {
		var st xinputState
		r, _, _ := proc.Call(uintptr(slot), uintptr(unsafe.Pointer(&st)))
		if r != 0 {
			w.mu.Lock()
			if w.connected[slot] {
				w.connected[slot] = false
				w.canRumble[slot] = false
				if w.timers[slot] != nil {
					w.timers[slot].Stop()
					w.timers[slot] = nil
				}
				w.gens[slot]++
			}
			w.mu.Unlock()
			continue
		}
		w.mu.Lock()
		wasConnected := w.connected[slot]
		canRumble := w.canRumble[slot]
		w.mu.Unlock()
		if !wasConnected {
			canRumble = queryXInputCanRumble(slot)
			w.mu.Lock()
			w.connected[slot] = true
			w.canRumble[slot] = canRumble
			w.mu.Unlock()
		}
		out = append(out, mapXInputState(slot, st.gamepad, canRumble))
	}
	return out
}

func queryXInputCanRumble(slot int) bool {
	if xinputSetState == nil || xinputGetCapabilities == nil {
		return false
	}
	var caps xinputCapabilities
	r, _, _ := xinputGetCapabilities.Call(uintptr(slot), 0, uintptr(unsafe.Pointer(&caps)))
	if r != 0 {
		return false
	}
	return caps.vibration.leftMotorSpeed != 0 || caps.vibration.rightMotorSpeed != 0
}

func (w *windowsSource) connectedMask() uint8 {
	proc := loadXInput()
	if proc == nil {
		return 0
	}
	var mask uint8
	for slot := 0; slot < xinputMaxUsers; slot++ {
		var st xinputState
		if r, _, _ := proc.Call(uintptr(slot), uintptr(unsafe.Pointer(&st))); r == 0 {
			mask |= 1 << slot
		}
	}
	return mask
}

func (w *windowsSource) Watch(onChange func()) func() {
	if onChange == nil || loadXInput() == nil {
		return func() {}
	}
	done := make(chan struct{})
	last := w.connectedMask()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if cur := w.connectedMask(); cur != last {
					last = cur
					onChange()
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

func (w *windowsSource) Rumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	if loadXInput() == nil || xinputSetState == nil || id < 0 || id >= xinputMaxUsers {
		return
	}
	vib := xinputVibration{
		leftMotorSpeed:  uint16(math.Round(float64(clamp01(lowFreq)) * 0xffff)),
		rightMotorSpeed: uint16(math.Round(float64(clamp01(highFreq)) * 0xffff)),
	}
	w.mu.Lock()
	if w.timers[id] != nil {
		w.timers[id].Stop()
		w.timers[id] = nil
	}
	w.gens[id]++
	gen := w.gens[id]
	_, _, _ = xinputSetState.Call(uintptr(id), uintptr(unsafe.Pointer(&vib)))
	if duration > 0 && (vib.leftMotorSpeed != 0 || vib.rightMotorSpeed != 0) {
		w.timers[id] = time.AfterFunc(duration, func() {
			w.mu.Lock()
			if w.gens[id] != gen {
				w.mu.Unlock()
				return
			}
			w.timers[id] = nil
			var zero xinputVibration
			_, _, _ = xinputSetState.Call(uintptr(id), uintptr(unsafe.Pointer(&zero)))
			w.mu.Unlock()
		})
	}
	w.mu.Unlock()
}

func (w *windowsSource) StopRumble(id int) {
	if loadXInput() == nil || xinputSetState == nil || id < 0 || id >= xinputMaxUsers {
		return
	}
	w.mu.Lock()
	if w.timers[id] != nil {
		w.timers[id].Stop()
		w.timers[id] = nil
	}
	w.gens[id]++
	var zero xinputVibration
	_, _, _ = xinputSetState.Call(uintptr(id), uintptr(unsafe.Pointer(&zero)))
	w.mu.Unlock()
}

func mapXInputState(slot int, g xinputGamepad, canRumble bool) driver.GamepadState {
	pad := driver.GamepadState{
		ID:        slot,
		Name:      fmt.Sprintf("XInput Controller %d", slot+1),
		Family:    driver.GamepadFamilyXbox,
		CanRumble: canRumble,
	}
	btn := func(mask uint16) float32 {
		if g.buttons&mask != 0 {
			return 1
		}
		return 0
	}
	pad.Buttons[ButtonSouth] = btn(xinputA)
	pad.Buttons[ButtonEast] = btn(xinputB)
	pad.Buttons[ButtonWest] = btn(xinputX)
	pad.Buttons[ButtonNorth] = btn(xinputY)
	pad.Buttons[ButtonLB] = btn(xinputLeftShoulder)
	pad.Buttons[ButtonRB] = btn(xinputRightShoulder)
	pad.Buttons[ButtonLT] = normalizeTrigger(float64(g.leftTrigger), xinputTriggerThreshold, 255)
	pad.Buttons[ButtonRT] = normalizeTrigger(float64(g.rightTrigger), xinputTriggerThreshold, 255)
	pad.Buttons[ButtonBack] = btn(xinputBack)
	pad.Buttons[ButtonStart] = btn(xinputStart)
	pad.Buttons[ButtonLeftStick] = btn(xinputLeftThumb)
	pad.Buttons[ButtonRightStick] = btn(xinputRightThumb)
	pad.Buttons[ButtonDPadUp] = btn(xinputDPadUp)
	pad.Buttons[ButtonDPadDown] = btn(xinputDPadDown)
	pad.Buttons[ButtonDPadLeft] = btn(xinputDPadLeft)
	pad.Buttons[ButtonDPadRight] = btn(xinputDPadRight)
	pad.Buttons[ButtonHome] = btn(xinputGuide)

	pad.Axes[AxisLeftX], pad.Axes[AxisLeftY] = normalizeRadialStick(g.thumbLX, g.thumbLY, xinputLeftThumbDeadzone)
	pad.Axes[AxisRightX], pad.Axes[AxisRightY] = normalizeRadialStick(g.thumbRX, g.thumbRY, xinputRightThumbDeadzone)
	return pad
}
