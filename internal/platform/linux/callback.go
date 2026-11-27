//go:build linux && !android

package linux

// purego keeps every NewCallback trampoline for the process's life, so
// the backend makes one per callback shared by every window, view and
// menu item, rather than one per connection. Menu items and entries are
// rebuilt as an app changes sections, and per-connection trampolines
// would exhaust purego's limit.
var (
	cbCloseRequest uintptr
	cbApply        uintptr
	cbRun          uintptr
	cbWindowState  uintptr
	cbMapped       uintptr
	cbLevelMapped  uintptr
	cbWindowMapped uintptr
	cbActivity     uintptr

	cbPreviewSelection  uintptr
	cbPreviewProperties uintptr
	cbPreviewOwner      uintptr
	cbWaylandExported   uintptr

	cbClickPressed     uintptr
	cbClickReleased    uintptr
	cbMotion           uintptr
	cbLeave            uintptr
	cbDragBegin        uintptr
	cbDragUpdate       uintptr
	cbDragEnd          uintptr
	cbZoomChanged      uintptr
	cbRotateChanged    uintptr
	cbGestureEnd       uintptr
	cbScroll           uintptr
	cbScrollBegin      uintptr
	cbScrollDecelerate uintptr
	cbKeyPressed       uintptr
	cbKeyReleased      uintptr
	cbModifiers        uintptr
	cbPinchTimeout     uintptr

	cbEntryChanged    uintptr
	cbEntryActivate   uintptr
	cbEntryFocusEnter uintptr
	cbEntryFocusLeave uintptr
	cbControlFocus    uintptr
	cbAlertChosen     uintptr

	cbWebLoadChanged         uintptr
	cbWebLoadFailed          uintptr
	cbWebNavigationChanged   uintptr
	cbWebHistoryChanged      uintptr
	cbWebResourceStarted     uintptr
	cbWebPolicy              uintptr
	cbWebCreate              uintptr
	cbWebPermission          uintptr
	cbWebMessage             uintptr
	cbWebMap                 uintptr
	cbWebFocusIn             uintptr
	cbWebFocusOut            uintptr
	cbWebKey                 uintptr
	cbWebButton              uintptr
	cbWebMotion              uintptr
	cbWebScroll              uintptr
	cbWebScrollBegin         uintptr
	cbWebScrollDecelerate    uintptr
	cbWebDownloadStarted     uintptr
	cbWebDownloadDestination uintptr
	cbWebDownloadReceived    uintptr
	cbWebDownloadFinished    uintptr
	cbWebDownloadFailed      uintptr
	cbWebScheme              uintptr

	cbShield               uintptr
	cbTouch                uintptr
	cbViewClassInit        uintptr
	cbViewDispose          uintptr
	cbViewSizeAllocate     uintptr
	cbViewFocus            uintptr
	cbViewSnapshot         uintptr
	cbViewContains         uintptr
	cbRangeIfaceInit       uintptr
	cbRangeSetCurrentValue uintptr
	cbVideoPrepared        uintptr
	cbVideoPlaying         uintptr
	cbVideoSize            uintptr
	cbVideoEnded           uintptr
	cbVideoError           uintptr
	cbVideoProgress        uintptr

	cbSettingsChanged       uintptr
	cbDisplaysChanged       uintptr
	cbCurrentDisplayChanged uintptr
	cbMenuActivate          uintptr
	cbMenuIconVisible       uintptr
	cbTick                  uintptr
	cbTickWatch             uintptr
	cbYield                 uintptr
	cbRevealLayout          uintptr

	cbNotificationAction    uintptr
	cbNotificationClosed    uintptr
	cbGlobalHotKeyResponse  uintptr
	cbGlobalHotKeyActivated uintptr
	cbColorPickResponse     uintptr
	cbX11HotKeyEvent        uintptr
	cbFrontmostEvent        uintptr
	cbSecretPrompt          uintptr

	cbShaderRender    uintptr
	cbShaderUnrealize uintptr

	cbGPURealize   uintptr
	cbGPUResize    uintptr
	cbGPURender    uintptr
	cbGPUUnrealize uintptr
	cbGPUFlush     uintptr

	cbDropMotion uintptr
	cbDropLeave  uintptr
	cbDrop       uintptr
)

func init() {
	cbCloseRequest = newCallback(closeRequestCallback)
	cbMenuIconVisible = newCallback(menuIconVisibleCallback)
	cbApply = newCallback(applyCallback)
	cbRun = newCallback(runCallback)
	cbWindowState = newCallback(windowStateCallback)
	cbMapped = newCallback(mappedCallback)
	cbLevelMapped = newCallback(levelMappedCallback)
	cbWindowMapped = newCallback(windowMappedCallback)
	cbActivity = newCallback(activityCallback)

	cbClickPressed = newCallback(clickPressedCallback)
	cbClickReleased = newCallback(clickReleasedCallback)
	cbMotion = newCallback(motionCallback)
	cbLeave = newCallback(leaveCallback)
	cbDragBegin = newCallback(dragBeginCallback)
	cbDragUpdate = newCallback(dragUpdateCallback)
	cbDragEnd = newCallback(dragEndCallback)
	cbZoomChanged = newCallback(zoomChangedCallback)
	cbRotateChanged = newCallback(rotateChangedCallback)
	cbGestureEnd = newCallback(gestureEndCallback)
	cbScroll = newCallback(scrollCallback)
	cbScrollBegin = newCallback(scrollBeginCallback)
	cbScrollDecelerate = newCallback(scrollDecelerateCallback)
	cbKeyPressed = newCallback(keyPressedCallback)
	cbKeyReleased = newCallback(keyReleasedCallback)
	cbModifiers = newCallback(modifiersCallback)
	cbPinchTimeout = newCallback(pinchTimeoutCallback)

	cbEntryChanged = newCallback(entryChangedCallback)
	cbEntryActivate = newCallback(entryActivateCallback)
	cbEntryFocusEnter = newCallback(entryFocusEnterCallback)
	cbEntryFocusLeave = newCallback(entryFocusLeaveCallback)
	cbControlFocus = newCallback(controlFocusCallback)
	cbAlertChosen = newCallback(alertChosenCallback)

	cbShield = newCallback(shieldCallback)
	cbTouch = newCallback(touchCallback)
	cbViewClassInit = newCallback(viewClassInitCallback)
	cbViewDispose = newCallback(viewDisposeCallback)
	cbViewSizeAllocate = newCallback(viewSizeAllocateCallback)
	cbViewFocus = newCallback(viewFocusCallback)
	cbViewSnapshot = newCallback(viewSnapshotCallback)
	cbViewContains = newCallback(viewContainsCallback)
	cbRangeIfaceInit = newCallback(rangeIfaceInitCallback)
	cbRangeSetCurrentValue = newCallback(rangeSetCurrentValueCallback)

	cbSettingsChanged = newCallback(settingsChangedCallback)
	cbDisplaysChanged = newCallback(displaysChangedCallback)
	cbCurrentDisplayChanged = newCallback(currentDisplayChangedCallback)
	cbMenuActivate = newCallback(menuActivateCallback)
	cbTick = newCallback(tickCallback)
	cbTickWatch = newCallback(tickWatchCallback)
	cbYield = newCallback(yieldCallback)
	cbRevealLayout = newCallback(revealLayoutCallback)
	cbPreviewSelection = newCallback(previewSelectionCallback)
	cbPreviewProperties = newCallback(previewPropertiesCallback)
	cbPreviewOwner = newCallback(previewOwnerCallback)
	cbWaylandExported = newCallback(waylandExportedCallback)
	cbNotificationAction = newCallback(notificationActionCallback)
	cbNotificationClosed = newCallback(notificationClosedCallback)
	cbGlobalHotKeyResponse = newCallback(globalHotKeyResponse)
	cbGlobalHotKeyActivated = newCallback(globalHotKeyActivated)
	cbColorPickResponse = newCallback(colorPickResponse)
	cbX11HotKeyEvent = newCallback(x11HotKeyEvent)
	cbFrontmostEvent = newCallback(frontmostEvent)
	cbSecretPrompt = newCallback(secretPromptCompleted)

	cbDropMotion = newCallback(dropMotionCallback)
	cbDropLeave = newCallback(dropLeaveCallback)
	cbDrop = newCallback(dropCallback)
}
