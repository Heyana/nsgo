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

	cbEntryChanged  uintptr
	cbEntryActivate uintptr
	cbEntryFocus    uintptr
	cbControlFocus  uintptr
	cbAlertChosen   uintptr

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
	cbWebDownloadStarted     uintptr
	cbWebDownloadDestination uintptr
	cbWebDownloadReceived    uintptr
	cbWebDownloadFinished    uintptr
	cbWebDownloadFailed      uintptr
	cbWebScheme              uintptr

	cbShield           uintptr
	cbTouch            uintptr
	cbViewClassInit    uintptr
	cbViewDispose      uintptr
	cbViewSizeAllocate uintptr
	cbViewFocus        uintptr
	cbViewSnapshot     uintptr
	cbViewContains     uintptr
	cbVideoPrepared    uintptr
	cbVideoSize        uintptr
	cbVideoEnded       uintptr
	cbVideoError       uintptr
	cbVideoProgress    uintptr

	cbSettingsChanged uintptr
	cbDisplaysChanged uintptr
	cbMenuActivate    uintptr
	cbTick            uintptr
	cbTickWatch       uintptr
	cbYield           uintptr

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

	cbDropMotion uintptr
	cbDropLeave  uintptr
	cbDrop       uintptr
)

func init() {
	cbCloseRequest = newCallback(closeRequestCallback)
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
	cbEntryFocus = newCallback(entryFocusCallback)
	cbControlFocus = newCallback(controlFocusCallback)
	cbAlertChosen = newCallback(alertChosenCallback)

	cbWebLoadChanged = newCallback(webLoadChangedCallback)
	cbWebLoadFailed = newCallback(webLoadFailedCallback)
	cbWebNavigationChanged = newCallback(webNavigationChangedCallback)
	cbWebHistoryChanged = newCallback(webHistoryChangedCallback)
	cbWebResourceStarted = newCallback(webResourceStartedCallback)
	cbWebPolicy = newCallback(webPolicyCallback)
	cbWebCreate = newCallback(webCreateCallback)
	cbWebPermission = newCallback(webPermissionCallback)
	cbWebMessage = newCallback(webMessageCallback)
	cbWebMap = newCallback(webMapCallback)
	cbWebFocusIn = newCallback(webFocusInCallback)
	cbWebFocusOut = newCallback(webFocusOutCallback)
	cbWebKey = newCallback(webKeyCallback)
	cbWebButton = newCallback(webButtonCallback)
	cbWebMotion = newCallback(webMotionCallback)
	cbWebScroll = newCallback(webScrollCallback)
	cbWebDownloadStarted = newCallback(webDownloadStartedCallback)
	cbWebDownloadDestination = newCallback(webDownloadDestinationCallback)
	cbWebDownloadReceived = newCallback(webDownloadReceivedCallback)
	cbWebDownloadFinished = newCallback(webDownloadFinishedCallback)
	cbWebDownloadFailed = newCallback(webDownloadFailedCallback)
	cbWebScheme = newCallback(webSchemeCallback)

	cbShield = newCallback(shieldCallback)
	cbTouch = newCallback(touchCallback)
	cbViewClassInit = newCallback(viewClassInitCallback)
	cbViewDispose = newCallback(viewDisposeCallback)
	cbViewSizeAllocate = newCallback(viewSizeAllocateCallback)
	cbViewFocus = newCallback(viewFocusCallback)
	cbViewSnapshot = newCallback(viewSnapshotCallback)
	cbViewContains = newCallback(viewContainsCallback)
	cbVideoPrepared = newCallback(videoPreparedCallback)
	cbVideoSize = newCallback(videoSizeCallback)
	cbVideoEnded = newCallback(videoEndedCallback)
	cbVideoError = newCallback(videoErrorCallback)
	cbVideoProgress = newCallback(videoProgressCallback)

	cbSettingsChanged = newCallback(settingsChangedCallback)
	cbDisplaysChanged = newCallback(displaysChangedCallback)
	cbMenuActivate = newCallback(menuActivateCallback)
	cbTick = newCallback(tickCallback)
	cbTickWatch = newCallback(tickWatchCallback)
	cbYield = newCallback(yieldCallback)
	cbNotificationAction = newCallback(notificationActionCallback)
	cbNotificationClosed = newCallback(notificationClosedCallback)
	cbGlobalHotKeyResponse = newCallback(globalHotKeyResponse)
	cbGlobalHotKeyActivated = newCallback(globalHotKeyActivated)
	cbColorPickResponse = newCallback(colorPickResponse)
	cbX11HotKeyEvent = newCallback(x11HotKeyEvent)
	cbFrontmostEvent = newCallback(frontmostEvent)
	cbSecretPrompt = newCallback(secretPromptCompleted)

	cbShaderRender = newCallback(shaderRenderCallback)
	cbShaderUnrealize = newCallback(shaderUnrealizeCallback)

	cbDropMotion = newCallback(dropMotionCallback)
	cbDropLeave = newCallback(dropLeaveCallback)
	cbDrop = newCallback(dropCallback)
}
