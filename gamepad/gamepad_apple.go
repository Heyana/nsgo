//go:build darwin && !(ios && nsgowatch)

package gamepad

import (
	"slices"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

const gameControllerFramework = "/System/Library/Frameworks/GameController.framework/GameController"

var (
	gcOnce         sync.Once
	gcLib          uintptr
	gcClass        objc.Class
	gcNoteConnect  objc.ID
	gcNoteDisconn  objc.ID
	gcReady        bool
	gcAppleDefault = &appleSource{ids: map[objc.ID]int{}}

	selControllers           objc.SEL
	selCount                 objc.SEL
	selObjectAtIndex         objc.SEL
	selVendorName            objc.SEL
	selProductCategory       objc.SEL
	selUTF8String            objc.SEL
	selExtendedGamepad       objc.SEL
	selMicroGamepad          objc.SEL
	selRespondsToSelector    objc.SEL
	selButtonA               objc.SEL
	selButtonB               objc.SEL
	selButtonX               objc.SEL
	selButtonY               objc.SEL
	selLeftShoulder          objc.SEL
	selRightShoulder         objc.SEL
	selLeftTrigger           objc.SEL
	selRightTrigger          objc.SEL
	selButtonOptions         objc.SEL
	selButtonMenu            objc.SEL
	selButtonHome            objc.SEL
	selLeftThumbstickButton  objc.SEL
	selRightThumbstickButton objc.SEL
	selDPad                  objc.SEL
	selUp                    objc.SEL
	selDown                  objc.SEL
	selLeft                  objc.SEL
	selRight                 objc.SEL
	selLeftThumbstick        objc.SEL
	selRightThumbstick       objc.SEL
	selXAxis                 objc.SEL
	selYAxis                 objc.SEL
	selValue                 objc.SEL
	selDefaultCenter         objc.SEL
	selAddObserver           objc.SEL
	selRemoveObserver        objc.SEL
	selRetain                objc.SEL
	selRelease               objc.SEL
)

func loadGameController() bool {
	gcOnce.Do(func() {
		lib, err := purego.Dlopen(gameControllerFramework, purego.RTLD_NOW)
		if err != nil {
			return
		}
		cls := objc.GetClass("GCController")
		if cls == 0 {
			return
		}
		gcLib = lib
		gcClass = cls
		gcNoteConnect = frameworkGlobalID(lib, "GCControllerDidConnectNotification")
		gcNoteDisconn = frameworkGlobalID(lib, "GCControllerDidDisconnectNotification")

		selControllers = objc.RegisterName("controllers")
		selCount = objc.RegisterName("count")
		selObjectAtIndex = objc.RegisterName("objectAtIndex:")
		selVendorName = objc.RegisterName("vendorName")
		selProductCategory = objc.RegisterName("productCategory")
		selUTF8String = objc.RegisterName("UTF8String")
		selExtendedGamepad = objc.RegisterName("extendedGamepad")
		selMicroGamepad = objc.RegisterName("microGamepad")
		selRespondsToSelector = objc.RegisterName("respondsToSelector:")
		selButtonA = objc.RegisterName("buttonA")
		selButtonB = objc.RegisterName("buttonB")
		selButtonX = objc.RegisterName("buttonX")
		selButtonY = objc.RegisterName("buttonY")
		selLeftShoulder = objc.RegisterName("leftShoulder")
		selRightShoulder = objc.RegisterName("rightShoulder")
		selLeftTrigger = objc.RegisterName("leftTrigger")
		selRightTrigger = objc.RegisterName("rightTrigger")
		selButtonOptions = objc.RegisterName("buttonOptions")
		selButtonMenu = objc.RegisterName("buttonMenu")
		selButtonHome = objc.RegisterName("buttonHome")
		selLeftThumbstickButton = objc.RegisterName("leftThumbstickButton")
		selRightThumbstickButton = objc.RegisterName("rightThumbstickButton")
		selDPad = objc.RegisterName("dpad")
		selUp = objc.RegisterName("up")
		selDown = objc.RegisterName("down")
		selLeft = objc.RegisterName("left")
		selRight = objc.RegisterName("right")
		selLeftThumbstick = objc.RegisterName("leftThumbstick")
		selRightThumbstick = objc.RegisterName("rightThumbstick")
		selXAxis = objc.RegisterName("xAxis")
		selYAxis = objc.RegisterName("yAxis")
		selValue = objc.RegisterName("value")
		selDefaultCenter = objc.RegisterName("defaultCenter")
		selAddObserver = objc.RegisterName("addObserverForName:object:queue:usingBlock:")
		selRemoveObserver = objc.RegisterName("removeObserver:")
		selRetain = objc.RegisterName("retain")
		selRelease = objc.RegisterName("release")
		gcReady = true
	})
	return gcReady
}

func frameworkGlobalID(lib uintptr, name string) objc.ID {
	sym, err := purego.Dlsym(lib, name)
	if err != nil || sym == 0 {
		return 0
	}
	return **(**objc.ID)(unsafe.Pointer(&sym))
}

func platformSource() driver.GamepadSource {
	return gcAppleDefault
}

type appleSource struct {
	mu  sync.Mutex
	ids map[objc.ID]int
}

func (a *appleSource) Poll() []driver.GamepadState {
	if !loadGameController() {
		return nil
	}
	arr := objc.ID(gcClass).Send(selControllers)
	if arr == 0 {
		a.mu.Lock()
		clear(a.ids)
		a.mu.Unlock()
		return nil
	}
	n := objc.Send[uint](arr, selCount)
	seen := make(map[objc.ID]bool, n)
	controllers := make([]objc.ID, 0, n)
	for i := uint(0); i < n; i++ {
		ctrl := arr.Send(selObjectAtIndex, i)
		if ctrl == 0 {
			continue
		}
		seen[ctrl] = true
		controllers = append(controllers, ctrl)
	}

	a.mu.Lock()
	for ptr := range a.ids {
		if !seen[ptr] {
			delete(a.ids, ptr)
		}
	}
	assigned := make([]int, len(controllers))
	for i, ctrl := range controllers {
		id, ok := a.ids[ctrl]
		if !ok {
			id = a.nextFreeIDLocked()
			a.ids[ctrl] = id
		}
		assigned[i] = id
	}
	a.mu.Unlock()

	if len(controllers) == 0 {
		return nil
	}
	out := make([]driver.GamepadState, 0, len(controllers))
	for i, ctrl := range controllers {
		out = append(out, readGCController(assigned[i], ctrl))
	}
	slices.SortFunc(out, func(x, y driver.GamepadState) int { return x.ID - y.ID })
	return out
}

func (a *appleSource) nextFreeIDLocked() int {
	used := make(map[int]bool, len(a.ids))
	for _, id := range a.ids {
		used[id] = true
	}
	for id := 0; ; id++ {
		if !used[id] {
			return id
		}
	}
}

func (a *appleSource) Watch(onChange func()) func() {
	if onChange == nil || !loadGameController() {
		return func() {}
	}
	centerCls := objc.GetClass("NSNotificationCenter")
	if centerCls == 0 {
		return func() {}
	}
	center := objc.ID(centerCls).Send(selDefaultCenter)
	block := objcblock.New(func(objc.Block, objc.ID) { onChange() })
	var tokens []objc.ID
	for _, note := range []objc.ID{gcNoteConnect, gcNoteDisconn} {
		if note == 0 {
			continue
		}
		tok := center.Send(selAddObserver, note, objc.ID(0), objc.ID(0), block)
		if tok != 0 {
			tokens = append(tokens, tok.Send(selRetain))
		}
	}
	block.Release()
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, tok := range tokens {
				center.Send(selRemoveObserver, tok)
				tok.Send(selRelease)
			}
		})
	}
}

func readGCController(id int, ctrl objc.ID) driver.GamepadState {
	name := gcNSString(ctrl.Send(selVendorName))
	if name == "" && objc.Send[bool](ctrl, selRespondsToSelector, selProductCategory) {
		name = gcNSString(ctrl.Send(selProductCategory))
	}
	if name == "" {
		name = "Gamepad"
	}
	st := driver.GamepadState{ID: id, Name: name}

	if ext := ctrl.Send(selExtendedGamepad); ext != 0 {
		st.Buttons[ButtonSouth] = gcButtonVal(ext.Send(selButtonA))
		st.Buttons[ButtonEast] = gcButtonVal(ext.Send(selButtonB))
		st.Buttons[ButtonWest] = gcButtonVal(ext.Send(selButtonX))
		st.Buttons[ButtonNorth] = gcButtonVal(ext.Send(selButtonY))
		st.Buttons[ButtonLB] = gcButtonVal(ext.Send(selLeftShoulder))
		st.Buttons[ButtonRB] = gcButtonVal(ext.Send(selRightShoulder))
		st.Buttons[ButtonLT] = gcButtonVal(ext.Send(selLeftTrigger))
		st.Buttons[ButtonRT] = gcButtonVal(ext.Send(selRightTrigger))
		if objc.Send[bool](ext, selRespondsToSelector, selButtonOptions) {
			st.Buttons[ButtonBack] = gcButtonVal(ext.Send(selButtonOptions))
		}
		if objc.Send[bool](ext, selRespondsToSelector, selButtonMenu) {
			st.Buttons[ButtonStart] = gcButtonVal(ext.Send(selButtonMenu))
		}
		if objc.Send[bool](ext, selRespondsToSelector, selLeftThumbstickButton) {
			st.Buttons[ButtonLeftStick] = gcButtonVal(ext.Send(selLeftThumbstickButton))
		}
		if objc.Send[bool](ext, selRespondsToSelector, selRightThumbstickButton) {
			st.Buttons[ButtonRightStick] = gcButtonVal(ext.Send(selRightThumbstickButton))
		}
		if objc.Send[bool](ext, selRespondsToSelector, selButtonHome) {
			st.Buttons[ButtonHome] = gcButtonVal(ext.Send(selButtonHome))
		}
		if dpad := ext.Send(selDPad); dpad != 0 {
			st.Buttons[ButtonDPadUp] = gcButtonVal(dpad.Send(selUp))
			st.Buttons[ButtonDPadDown] = gcButtonVal(dpad.Send(selDown))
			st.Buttons[ButtonDPadLeft] = gcButtonVal(dpad.Send(selLeft))
			st.Buttons[ButtonDPadRight] = gcButtonVal(dpad.Send(selRight))
		}
		if left := ext.Send(selLeftThumbstick); left != 0 {
			st.Axes[AxisLeftX] = gcAxisVal(left.Send(selXAxis))
			// GCController +Y is up; W3C Standard Gamepad +Y is down.
			st.Axes[AxisLeftY] = clampAxis(-gcAxisVal(left.Send(selYAxis)))
		}
		if right := ext.Send(selRightThumbstick); right != 0 {
			st.Axes[AxisRightX] = gcAxisVal(right.Send(selXAxis))
			st.Axes[AxisRightY] = clampAxis(-gcAxisVal(right.Send(selYAxis)))
		}
		return st
	}

	if micro := ctrl.Send(selMicroGamepad); micro != 0 {
		st.Buttons[ButtonSouth] = gcButtonVal(micro.Send(selButtonA))
		st.Buttons[ButtonWest] = gcButtonVal(micro.Send(selButtonX))
		if objc.Send[bool](micro, selRespondsToSelector, selButtonMenu) {
			st.Buttons[ButtonStart] = gcButtonVal(micro.Send(selButtonMenu))
		}
		if dpad := micro.Send(selDPad); dpad != 0 {
			st.Buttons[ButtonDPadUp] = gcButtonVal(dpad.Send(selUp))
			st.Buttons[ButtonDPadDown] = gcButtonVal(dpad.Send(selDown))
			st.Buttons[ButtonDPadLeft] = gcButtonVal(dpad.Send(selLeft))
			st.Buttons[ButtonDPadRight] = gcButtonVal(dpad.Send(selRight))
			st.Axes[AxisLeftX] = gcAxisVal(dpad.Send(selXAxis))
			st.Axes[AxisLeftY] = clampAxis(-gcAxisVal(dpad.Send(selYAxis)))
		}
	}
	return st
}

func gcButtonVal(btn objc.ID) float32 {
	if btn == 0 {
		return 0
	}
	return clamp01(objc.Send[float32](btn, selValue))
}

func gcAxisVal(axis objc.ID) float32 {
	if axis == 0 {
		return 0
	}
	return clampAxis(objc.Send[float32](axis, selValue))
}

func gcNSString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	ptr := objc.Send[*byte](s, selUTF8String)
	if ptr == nil {
		return ""
	}
	var n int
	for *(*byte)(unsafe.Add(unsafe.Pointer(ptr), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(ptr, n))
}
