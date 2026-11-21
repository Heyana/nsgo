//go:build windows

package gamepad

import (
	"fmt"
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

var (
	xinputOnce     sync.Once
	xinputGetState *syscall.LazyProc
	winDefault     = &windowsSource{}
)

func loadXInput() *syscall.LazyProc {
	xinputOnce.Do(func() {
		for _, name := range []string{"xinput1_4.dll", "xinput1_3.dll", "xinput9_1_0.dll"} {
			dll := syscall.NewLazyDLL(name)
			proc := dll.NewProc("XInputGetState")
			if proc.Find() == nil {
				xinputGetState = proc
				return
			}
		}
	})
	return xinputGetState
}

func platformSource() driver.GamepadSource {
	return winDefault
}

type windowsSource struct{}

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
			continue
		}
		out = append(out, mapXInputState(slot, st.gamepad))
	}
	return out
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

func mapXInputState(slot int, g xinputGamepad) driver.GamepadState {
	pad := driver.GamepadState{
		ID:   slot,
		Name: fmt.Sprintf("XInput Controller %d", slot+1),
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
