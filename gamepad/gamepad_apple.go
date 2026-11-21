//go:build darwin && !(ios && nsgowatch)

package gamepad

import (
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

const (
	gameControllerFramework = "/System/Library/Frameworks/GameController.framework/GameController"
	coreHapticsFramework    = "/System/Library/Frameworks/CoreHaptics.framework/CoreHaptics"
)

var (
	gcOnce            sync.Once
	gcLib             uintptr
	gcClass           objc.Class
	gcClassXbox       objc.Class
	gcClassDualShock  objc.Class
	gcClassDualSense  objc.Class
	gcNoteConnect     objc.ID
	gcNoteDisconn     objc.ID
	gcLocalityDefault objc.ID
	gcLocalityLeft    objc.ID
	gcLocalityRight   objc.ID
	gcReady           bool
	gcAppleDefault    = &appleSource{ids: map[objc.ID]int{}, haptics: map[objc.ID]*appleHaptics{}}

	chOnce            sync.Once
	chClassParam      objc.Class
	chClassEvent      objc.Class
	chClassPattern    objc.Class
	nsClassArray      objc.Class
	nsClassPool       objc.Class
	chEventContinuous objc.ID
	chParamIntensity  objc.ID
	chParamSharpness  objc.ID
	chReady           bool

	selControllers              objc.SEL
	selCount                    objc.SEL
	selObjectAtIndex            objc.SEL
	selContainsObject           objc.SEL
	selVendorName               objc.SEL
	selProductCategory          objc.SEL
	selUTF8String               objc.SEL
	selExtendedGamepad          objc.SEL
	selMicroGamepad             objc.SEL
	selRespondsToSelector       objc.SEL
	selIsKindOfClass            objc.SEL
	selHaptics                  objc.SEL
	selSupportedLocalities      objc.SEL
	selCreateEngineWithLocality objc.SEL
	selButtonA                  objc.SEL
	selButtonB                  objc.SEL
	selButtonX                  objc.SEL
	selButtonY                  objc.SEL
	selLeftShoulder             objc.SEL
	selRightShoulder            objc.SEL
	selLeftTrigger              objc.SEL
	selRightTrigger             objc.SEL
	selButtonOptions            objc.SEL
	selButtonMenu               objc.SEL
	selButtonHome               objc.SEL
	selLeftThumbstickButton     objc.SEL
	selRightThumbstickButton    objc.SEL
	selDPad                     objc.SEL
	selUp                       objc.SEL
	selDown                     objc.SEL
	selLeft                     objc.SEL
	selRight                    objc.SEL
	selLeftThumbstick           objc.SEL
	selRightThumbstick          objc.SEL
	selXAxis                    objc.SEL
	selYAxis                    objc.SEL
	selValue                    objc.SEL
	selDefaultCenter            objc.SEL
	selAddObserver              objc.SEL
	selRemoveObserver           objc.SEL
	selAlloc                    objc.SEL
	selInit                     objc.SEL
	selDrain                    objc.SEL
	selRetain                   objc.SEL
	selRelease                  objc.SEL
	selArrayWithObjectsCount    objc.SEL
	selArray                    objc.SEL
	selInitParamIDValue         objc.SEL
	selInitEventTypeDur         objc.SEL
	selInitPatternEvents        objc.SEL
	selStartAndReturnError      objc.SEL
	selStopWithCompletion       objc.SEL
	selCreatePlayerWithPattern  objc.SEL
	selStartAtTimeError         objc.SEL
	selStopAtTimeError          objc.SEL
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
		gcClassXbox = objc.GetClass("GCXboxGamepad")
		gcClassDualShock = objc.GetClass("GCDualShockGamepad")
		gcClassDualSense = objc.GetClass("GCDualSenseGamepad")
		gcNoteConnect = frameworkGlobalID(lib, "GCControllerDidConnectNotification")
		gcNoteDisconn = frameworkGlobalID(lib, "GCControllerDidDisconnectNotification")
		gcLocalityDefault = frameworkGlobalID(lib, "GCHapticsLocalityDefault")
		gcLocalityLeft = frameworkGlobalID(lib, "GCHapticsLocalityLeftHandle")
		gcLocalityRight = frameworkGlobalID(lib, "GCHapticsLocalityRightHandle")

		selControllers = objc.RegisterName("controllers")
		selCount = objc.RegisterName("count")
		selObjectAtIndex = objc.RegisterName("objectAtIndex:")
		selContainsObject = objc.RegisterName("containsObject:")
		selVendorName = objc.RegisterName("vendorName")
		selProductCategory = objc.RegisterName("productCategory")
		selUTF8String = objc.RegisterName("UTF8String")
		selExtendedGamepad = objc.RegisterName("extendedGamepad")
		selMicroGamepad = objc.RegisterName("microGamepad")
		selRespondsToSelector = objc.RegisterName("respondsToSelector:")
		selIsKindOfClass = objc.RegisterName("isKindOfClass:")
		selHaptics = objc.RegisterName("haptics")
		selSupportedLocalities = objc.RegisterName("supportedLocalities")
		selCreateEngineWithLocality = objc.RegisterName("createEngineWithLocality:")
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
		selAlloc = objc.RegisterName("alloc")
		selInit = objc.RegisterName("init")
		selDrain = objc.RegisterName("drain")
		selRetain = objc.RegisterName("retain")
		selRelease = objc.RegisterName("release")
		gcReady = true
	})
	return gcReady
}

func loadCoreHaptics() bool {
	if !loadGameController() {
		return false
	}
	chOnce.Do(func() {
		lib, err := purego.Dlopen(coreHapticsFramework, purego.RTLD_NOW)
		if err != nil {
			return
		}
		chClassParam = objc.GetClass("CHHapticEventParameter")
		chClassEvent = objc.GetClass("CHHapticEvent")
		chClassPattern = objc.GetClass("CHHapticPattern")
		nsClassArray = objc.GetClass("NSArray")
		nsClassPool = objc.GetClass("NSAutoreleasePool")
		if chClassParam == 0 || chClassEvent == 0 || chClassPattern == 0 || nsClassArray == 0 {
			return
		}
		chEventContinuous = frameworkGlobalID(lib, "CHHapticEventTypeHapticContinuous")
		chParamIntensity = frameworkGlobalID(lib, "CHHapticEventParameterIDHapticIntensity")
		chParamSharpness = frameworkGlobalID(lib, "CHHapticEventParameterIDHapticSharpness")
		if chEventContinuous == 0 || chParamIntensity == 0 || chParamSharpness == 0 || gcLocalityDefault == 0 {
			return
		}
		selArrayWithObjectsCount = objc.RegisterName("arrayWithObjects:count:")
		selArray = objc.RegisterName("array")
		selInitParamIDValue = objc.RegisterName("initWithParameterID:value:")
		selInitEventTypeDur = objc.RegisterName("initWithEventType:parameters:relativeTime:duration:")
		selInitPatternEvents = objc.RegisterName("initWithEvents:parameters:error:")
		selStartAndReturnError = objc.RegisterName("startAndReturnError:")
		selStopWithCompletion = objc.RegisterName("stopWithCompletionHandler:")
		selCreatePlayerWithPattern = objc.RegisterName("createPlayerWithPattern:error:")
		selStartAtTimeError = objc.RegisterName("startAtTime:error:")
		selStopAtTimeError = objc.RegisterName("stopAtTime:error:")
		chReady = true
	})
	return chReady
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

type appleHaptics struct {
	dual      bool
	leftEng   objc.ID
	rightEng  objc.ID
	leftPlay  objc.ID
	rightPlay objc.ID
}

func (h *appleHaptics) stopPlayers() {
	var errID objc.ID
	if h.leftPlay != 0 {
		objc.Send[bool](h.leftPlay, selStopAtTimeError, float64(0), unsafe.Pointer(&errID))
		h.leftPlay.Send(selRelease)
		h.leftPlay = 0
	}
	if h.rightPlay != 0 {
		objc.Send[bool](h.rightPlay, selStopAtTimeError, float64(0), unsafe.Pointer(&errID))
		h.rightPlay.Send(selRelease)
		h.rightPlay = 0
	}
}

func (h *appleHaptics) destroy() {
	h.stopPlayers()
	if h.leftEng != 0 {
		h.leftEng.Send(selStopWithCompletion, objc.ID(0))
		h.leftEng.Send(selRelease)
		h.leftEng = 0
	}
	if h.rightEng != 0 {
		h.rightEng.Send(selStopWithCompletion, objc.ID(0))
		h.rightEng.Send(selRelease)
		h.rightEng = 0
	}
}

type appleSource struct {
	mu      sync.Mutex
	ids     map[objc.ID]int
	haptics map[objc.ID]*appleHaptics
}

func (a *appleSource) Poll() []driver.GamepadState {
	if !loadGameController() {
		return nil
	}
	arr := objc.ID(gcClass).Send(selControllers)
	if arr == 0 {
		a.mu.Lock()
		for ptr, h := range a.haptics {
			h.destroy()
			delete(a.haptics, ptr)
		}
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
			if h, ok := a.haptics[ptr]; ok {
				h.destroy()
				delete(a.haptics, ptr)
			}
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

func (a *appleSource) findCtrlLocked(id int) objc.ID {
	for ptr, cid := range a.ids {
		if cid == id {
			return ptr
		}
	}
	return 0
}

func (a *appleSource) Rumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	if !loadCoreHaptics() {
		return
	}
	lowFreq = clamp01(lowFreq)
	highFreq = clamp01(highFreq)
	if duration <= 0 || (lowFreq == 0 && highFreq == 0) {
		a.StopRumble(id)
		return
	}

	var pool objc.ID
	if nsClassPool != 0 {
		pool = objc.ID(nsClassPool).Send(selAlloc).Send(selInit)
		defer pool.Send(selDrain)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	ctrl := a.findCtrlLocked(id)
	if ctrl == 0 {
		return
	}
	h := a.ensureHapticsLocked(ctrl)
	if h == nil {
		return
	}
	h.stopPlayers()

	durSec := duration.Seconds()
	if h.dual {
		if lowFreq > 0 {
			h.leftPlay = startContinuousHaptic(h.leftEng, lowFreq, 0.2, durSec)
		}
		if highFreq > 0 {
			h.rightPlay = startContinuousHaptic(h.rightEng, highFreq, 0.8, durSec)
		}
		return
	}
	intensity := lowFreq
	if highFreq > intensity {
		intensity = highFreq
	}
	sharpness := float32(0.5)
	if sum := lowFreq + highFreq; sum > 0 {
		sharpness = highFreq / sum
	}
	h.leftPlay = startContinuousHaptic(h.leftEng, intensity, sharpness, durSec)
}

func (a *appleSource) StopRumble(id int) {
	if !loadCoreHaptics() {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	ctrl := a.findCtrlLocked(id)
	if ctrl == 0 {
		return
	}
	if h, ok := a.haptics[ctrl]; ok {
		h.stopPlayers()
	}
}

func (a *appleSource) ensureHapticsLocked(ctrl objc.ID) *appleHaptics {
	if h, ok := a.haptics[ctrl]; ok {
		return h
	}
	if !objc.Send[bool](ctrl, selRespondsToSelector, selHaptics) {
		return nil
	}
	devHaptics := ctrl.Send(selHaptics)
	if devHaptics == 0 {
		return nil
	}
	dual := false
	if gcLocalityLeft != 0 && gcLocalityRight != 0 && objc.Send[bool](devHaptics, selRespondsToSelector, selSupportedLocalities) {
		locs := devHaptics.Send(selSupportedLocalities)
		if locs != 0 &&
			objc.Send[bool](locs, selContainsObject, gcLocalityLeft) &&
			objc.Send[bool](locs, selContainsObject, gcLocalityRight) {
			dual = true
		}
	}
	h := &appleHaptics{dual: dual}
	if dual {
		left := devHaptics.Send(selCreateEngineWithLocality, gcLocalityLeft)
		right := devHaptics.Send(selCreateEngineWithLocality, gcLocalityRight)
		if left != 0 && right != 0 {
			h.leftEng = left.Send(selRetain)
			h.rightEng = right.Send(selRetain)
			a.haptics[ctrl] = h
			return h
		}
		h.dual = false
	}
	eng := devHaptics.Send(selCreateEngineWithLocality, gcLocalityDefault)
	if eng == 0 {
		return nil
	}
	h.leftEng = eng.Send(selRetain)
	a.haptics[ctrl] = h
	return h
}

func startContinuousHaptic(eng objc.ID, intensity, sharpness float32, durSec float64) objc.ID {
	if eng == 0 {
		return 0
	}
	var errID objc.ID
	if !objc.Send[bool](eng, selStartAndReturnError, unsafe.Pointer(&errID)) {
		return 0
	}
	pIntensity := objc.ID(chClassParam).Send(selAlloc).Send(selInitParamIDValue, chParamIntensity, intensity)
	if pIntensity == 0 {
		return 0
	}
	defer pIntensity.Send(selRelease)
	pSharpness := objc.ID(chClassParam).Send(selAlloc).Send(selInitParamIDValue, chParamSharpness, sharpness)
	if pSharpness == 0 {
		return 0
	}
	defer pSharpness.Send(selRelease)

	params := [2]objc.ID{pIntensity, pSharpness}
	paramArr := objc.ID(nsClassArray).Send(selArrayWithObjectsCount, unsafe.Pointer(&params[0]), uintptr(2))
	ev := objc.ID(chClassEvent).Send(selAlloc).Send(selInitEventTypeDur, chEventContinuous, paramArr, float64(0), durSec)
	if ev == 0 {
		return 0
	}
	defer ev.Send(selRelease)

	events := [1]objc.ID{ev}
	evArr := objc.ID(nsClassArray).Send(selArrayWithObjectsCount, unsafe.Pointer(&events[0]), uintptr(1))
	emptyArr := objc.ID(nsClassArray).Send(selArray)
	errID = 0
	pattern := objc.ID(chClassPattern).Send(selAlloc).Send(selInitPatternEvents, evArr, emptyArr, unsafe.Pointer(&errID))
	if pattern == 0 {
		return 0
	}
	defer pattern.Send(selRelease)

	errID = 0
	player := eng.Send(selCreatePlayerWithPattern, pattern, unsafe.Pointer(&errID))
	if player == 0 {
		return 0
	}
	errID = 0
	if !objc.Send[bool](player, selStartAtTimeError, float64(0), unsafe.Pointer(&errID)) {
		return 0
	}
	return player.Send(selRetain)
}

func readGCController(id int, ctrl objc.ID) driver.GamepadState {
	vendorName := gcNSString(ctrl.Send(selVendorName))
	var productCategory string
	if objc.Send[bool](ctrl, selRespondsToSelector, selProductCategory) {
		productCategory = gcNSString(ctrl.Send(selProductCategory))
	}
	name := vendorName
	if name == "" {
		name = productCategory
	}
	if name == "" {
		name = "Gamepad"
	}

	ext := ctrl.Send(selExtendedGamepad)
	family := detectAppleFamily(ext, productCategory, vendorName)
	canRumble := gcCanRumble(ctrl)

	st := driver.GamepadState{
		ID:        id,
		Name:      name,
		Family:    driver.GamepadFamily(family),
		CanRumble: canRumble,
	}

	if ext != 0 {
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

func detectAppleFamily(ext objc.ID, productCategory, vendorName string) Family {
	if ext != 0 {
		if gcClassDualSense != 0 && objc.Send[bool](ext, selIsKindOfClass, gcClassDualSense) {
			return FamilyPlayStation
		}
		if gcClassDualShock != 0 && objc.Send[bool](ext, selIsKindOfClass, gcClassDualShock) {
			return FamilyPlayStation
		}
		if gcClassXbox != 0 && objc.Send[bool](ext, selIsKindOfClass, gcClassXbox) {
			return FamilyXbox
		}
	}
	if f := DetectFamily(0, 0, productCategory); f != FamilyGeneric {
		return f
	}
	return DetectFamily(0, 0, vendorName)
}

func gcCanRumble(ctrl objc.ID) bool {
	if !loadCoreHaptics() {
		return false
	}
	if !objc.Send[bool](ctrl, selRespondsToSelector, selHaptics) {
		return false
	}
	haptics := ctrl.Send(selHaptics)
	if haptics == 0 {
		return false
	}
	if objc.Send[bool](haptics, selRespondsToSelector, selSupportedLocalities) {
		locs := haptics.Send(selSupportedLocalities)
		return locs != 0 && objc.Send[uint](locs, selCount) > 0
	}
	return true
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
