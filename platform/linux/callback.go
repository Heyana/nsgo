//go:build linux && !android

package linux

// purego keeps every NewCallback trampoline for the process's life, so
// the backend makes one per callback shared by every window, view and
// menu item, rather than one per connection. Menu items and entries are
// rebuilt as an app changes sections, and per-connection trampolines
// would exhaust purego's limit.
var (
	cbDraw         uintptr
	cbCloseRequest uintptr
	cbApply        uintptr
	cbRun          uintptr

	cbClickPressed  uintptr
	cbClickReleased uintptr
	cbMotion        uintptr
	cbLeave         uintptr
	cbDragBegin     uintptr
	cbDragUpdate    uintptr
	cbDragEnd       uintptr
	cbLongPress     uintptr
	cbZoomChanged   uintptr
	cbRotateChanged uintptr
	cbGestureEnd    uintptr
	cbScroll        uintptr
	cbKeyPressed    uintptr
	cbPinchTimeout  uintptr

	cbEntryChanged  uintptr
	cbEntryActivate uintptr
	cbEntryFocus    uintptr
	cbClipboardText uintptr
	cbAlertChosen   uintptr

	cbWebLoadChanged   uintptr
	cbWebLoadFailed    uintptr
	cbWebMessage       uintptr
	cbWebMap           uintptr
	cbWebChildPosition uintptr

	cbSettingsChanged uintptr
	cbMenuActivate    uintptr
	cbTick            uintptr
)

func init() {
	cbDraw = newCallback(drawCallback)
	cbCloseRequest = newCallback(closeRequestCallback)
	cbApply = newCallback(applyCallback)
	cbRun = newCallback(runCallback)

	cbClickPressed = newCallback(clickPressedCallback)
	cbClickReleased = newCallback(clickReleasedCallback)
	cbMotion = newCallback(motionCallback)
	cbLeave = newCallback(leaveCallback)
	cbDragBegin = newCallback(dragBeginCallback)
	cbDragUpdate = newCallback(dragUpdateCallback)
	cbDragEnd = newCallback(dragEndCallback)
	cbLongPress = newCallback(longPressCallback)
	cbZoomChanged = newCallback(zoomChangedCallback)
	cbRotateChanged = newCallback(rotateChangedCallback)
	cbGestureEnd = newCallback(gestureEndCallback)
	cbScroll = newCallback(scrollCallback)
	cbKeyPressed = newCallback(keyPressedCallback)
	cbPinchTimeout = newCallback(pinchTimeoutCallback)

	cbEntryChanged = newCallback(entryChangedCallback)
	cbEntryActivate = newCallback(entryActivateCallback)
	cbEntryFocus = newCallback(entryFocusCallback)
	cbClipboardText = newCallback(clipboardTextCallback)
	cbAlertChosen = newCallback(alertChosenCallback)

	cbWebLoadChanged = newCallback(webLoadChangedCallback)
	cbWebLoadFailed = newCallback(webLoadFailedCallback)
	cbWebMessage = newCallback(webMessageCallback)
	cbWebMap = newCallback(webMapCallback)
	cbWebChildPosition = newCallback(webChildPositionCallback)

	cbSettingsChanged = newCallback(settingsChangedCallback)
	cbMenuActivate = newCallback(menuActivateCallback)
	cbTick = newCallback(tickCallback)
}
