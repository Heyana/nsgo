//go:build linux && !android

package linux

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The libraries NSGo needs. GTK 4 merged GDK into itself; Cairo and Pango
// are reached directly for drawing and text layout.
var (
	gtkLib        uintptr
	glibLib       uintptr
	gobjectLib    uintptr
	gioLib        uintptr
	pangoLib      uintptr
	pangocairoLib uintptr
	cairoLib      uintptr
)

var (
	canvasLibsOnce sync.Once
	canvasLibsErr  error
	libsOnce       sync.Once
	libsErr        error
)

func loadSharedLib(name string, candidates ...string) (uintptr, error) {
	var last error
	for _, candidate := range candidates {
		h, err := purego.Dlopen(candidate, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		last = err
	}
	return 0, fmt.Errorf("%s: %w", name, last)
}

// openCanvasLibs loads only GLib, GObject, Pango, PangoCairo and Cairo,
// without GTK or a display connection, for windowless text and canvas rendering.
func openCanvasLibs() error {
	canvasLibsOnce.Do(func() {
		var err error
		if glibLib, err = loadSharedLib("GLib", "libglib-2.0.so.0", "libglib-2.0.so"); err != nil {
			canvasLibsErr = err
			return
		}
		if gobjectLib, err = loadSharedLib("GObject", "libgobject-2.0.so.0", "libgobject-2.0.so"); err != nil {
			canvasLibsErr = err
			return
		}
		if pangoLib, err = loadSharedLib("Pango", "libpango-1.0.so.0", "libpango-1.0.so"); err != nil {
			canvasLibsErr = err
			return
		}
		if pangocairoLib, err = loadSharedLib("PangoCairo", "libpangocairo-1.0.so.0", "libpangocairo-1.0.so"); err != nil {
			canvasLibsErr = err
			return
		}
		if cairoLib, err = loadSharedLib("Cairo", "libcairo.so.2", "libcairo.so"); err != nil {
			canvasLibsErr = err
			return
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					canvasLibsErr = fmt.Errorf("Pango and Cairo are required: %v", r)
				}
			}()
			bindCanvasLibs()
		}()
	})
	return canvasLibsErr
}

// gtkDisplayReady checks the display as well as GTK's initialization flag: gtk_init_check may return true after its earlier display connection failed. A host that subsequently supplies GDK's default display can still start.
func gtkDisplayReady() bool { return gtkInitCheck() != 0 && gdkDisplayGetDefault() != 0 }

// openLibs loads the libraries, reporting which one is missing.
func openLibs() error {
	libsOnce.Do(func() {
		// Its Wayland shim must precede GTK's libwayland dependency.
		_ = loadLayerShell()
		var err error
		if err = openCanvasLibs(); err != nil {
			libsErr = err
			return
		}
		if gtkLib, err = loadSharedLib("GTK 4", "libgtk-4.so.1", "libgtk-4.so.0", "libgtk-4.so"); err != nil {
			libsErr = err
			return
		}
		if gioLib, err = loadSharedLib("GIO", "libgio-2.0.so.0", "libgio-2.0.so"); err != nil {
			libsErr = err
			return
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					libsErr = fmt.Errorf("GTK 4.14, Pango and Cairo are required: %v", r)
				}
			}()
			bind()
		}()
	})
	return libsErr
}

// The bound functions. Their types are the C signatures purego converts
// Go values to, so pointer arguments are uintptr and C strings are *byte.
var (
	// GLib.
	gMainContextIteration func(uintptr, int32) int32
	gMainContextWakeup    func(uintptr)
	gTimeoutAdd           func(uint32, uintptr, uintptr) uint32
	gGetMonotonicTime     func() int64
	gSourceRemove         func(uint32)
	gIdleAdd              func(uintptr, uintptr) uint32
	gFree                 func(uintptr)
	gErrorFree            func(uintptr)
	gSetPrgname           func(*byte)
	gObjectUnref          func(uintptr)
	gObjectRef            func(uintptr) uintptr
	gObjectGetData        func(uintptr, *byte) uintptr
	gObjectSetData        func(uintptr, *byte, uintptr)
	gTypeIsA              func(uintptr, uintptr) int32
	gObjectRefSink        func(uintptr) uintptr
	gSignalConnectData    func(uintptr, *byte, uintptr, uintptr, uintptr, uint32) uint64
	gSignalConnectObj     func(uintptr, *byte, uintptr, uintptr, uint32) uint64
	gSignalEmitByName     func(uintptr, *byte, ...any)
	gObjectGet            func(uintptr, ...any)
	gObjectSetProperty    func(uintptr, *byte, *gValue)
	gObjectNotify         func(uintptr, *byte)
	gStrfreev             func(uintptr)
	gVariantNewString     func(*byte) uintptr
	gVariantNewBoolean    func(int32) uintptr
	gVariantUnref         func(uintptr)

	// GTK.
	gtkInitCheck                           func() int32
	gtkWindowNew                           func() uintptr
	gtkWindowSetTitle                      func(uintptr, *byte)
	gtkWindowGetTitle                      func(uintptr) uintptr
	gtkWindowSetDefaultSize                func(uintptr, int32, int32)
	gtkWindowSetResizable                  func(uintptr, int32)
	gtkWindowSetChild                      func(uintptr, uintptr)
	gtkWindowSetFocus                      func(uintptr, uintptr)
	gtkWindowGetFocus                      func(uintptr) uintptr
	gtkWindowPresent                       func(uintptr)
	gtkWindowDestroy                       func(uintptr)
	gtkWindowClose                         func(uintptr)
	gtkWindowSetDecorated                  func(uintptr, int32)
	gtkWindowMinimize                      func(uintptr)
	gtkWindowMaximize                      func(uintptr)
	gtkWindowUnmaximize                    func(uintptr)
	gtkWindowFullscreen                    func(uintptr)
	gtkWindowFullscreenOnMonitor           func(uintptr, uintptr)
	gtkWindowUnfullscreen                  func(uintptr)
	gtkWindowSetTransientFor               func(uintptr, uintptr)
	gtkWindowIsFullscreen                  func(uintptr) int32
	gtkWindowIsMaximized                   func(uintptr) int32
	gtkWindowIsActive                      func(uintptr) int32
	gdkToplevelGetState                    func(uintptr) uint32
	gtkWidgetShow                          func(uintptr)
	gtkWidgetSetVisible                    func(uintptr, int32)
	gtkWidgetGetVisible                    func(uintptr) int32
	gtkWidgetAddController                 func(uintptr, uintptr)
	gtkWidgetQueueDraw                     func(uintptr)
	gtkWidgetAddTickCallback               func(widget, callback, data, destroy uintptr) uint32
	gtkWidgetRemoveTickCallback            func(widget uintptr, id uint32)
	gtkWidgetQueueAllocate                 func(uintptr)
	gtkWidgetGetMapped                     func(uintptr) int32
	gtkWidgetGrabFocus                     func(uintptr) int32
	gtkWidgetGetRoot                       func(uintptr) uintptr
	gtkWidgetSetFocusable                  func(uintptr, int32)
	gtkWidgetGetFocusable                  func(uintptr) int32
	gtkWidgetGetWidth                      func(uintptr) int32
	gtkWidgetGetHeight                     func(uintptr) int32
	gtkWidgetGetNative                     func(uintptr) uintptr
	gtkWidgetGetScaleFactor                func(uintptr) int32
	gtkWidgetGetStyleContext               func(uintptr) uintptr
	gtkWidgetGetParent                     func(uintptr) uintptr
	gtkWidgetHasFocus                      func(uintptr) int32
	gtkWidgetActivateAction                func(uintptr, *byte, uintptr) int32
	gtkWidgetGetDefaultDirection           func() int32
	gtkWidgetSetDefaultDirection           func(int32)
	gtkWidgetGetDirection                  func(uintptr) int32
	gtkWidgetSetDirection                  func(uintptr, int32)
	gtkWidgetSetSizeRequest                func(uintptr, int32, int32)
	gtkWidgetSetHexpand                    func(uintptr, int32)
	gtkWidgetSetVexpand                    func(uintptr, int32)
	gtkWidgetSetHalign                     func(uintptr, int32)
	gtkWidgetSetValign                     func(uintptr, int32)
	gtkWidgetSetOverflow                   func(uintptr, int32)
	gtkWidgetSetSensitive                  func(uintptr, int32)
	gtkWidgetSetCursor                     func(uintptr, uintptr)
	gtkWidgetSetCursorFromName             func(uintptr, *byte)
	gdkCursorNewFromTexture                func(uintptr, int32, int32, uintptr) uintptr
	gdkCursorNewFromName                   func(*byte, uintptr) uintptr
	gtkWidgetActivate                      func(uintptr) int32
	gtkNativeGetSurface                    func(uintptr) uintptr
	gtkBoxNew                              func(int32, int32) uintptr
	gtkBoxAppend                           func(uintptr, uintptr)
	gtkBoxPrepend                          func(uintptr, uintptr)
	gtkBoxRemove                           func(uintptr, uintptr)
	gtkEntryNew                            func() uintptr
	gtkEntryGrabFocusWithoutSelecting      func(uintptr) int32
	gtkEditableSetText                     func(uintptr, *byte)
	gtkEditableGetText                     func(uintptr) uintptr
	gtkEditableSetEditable                 func(uintptr, int32)
	gtkEditableSelectRegion                func(uintptr, int32, int32)
	gtkEditableDeleteSelection             func(uintptr) int32
	gtkEntrySetPlaceholderText             func(uintptr, *byte)
	gtkEntrySetVisibility                  func(uintptr, int32)
	gtkEntrySetAlignment                   func(uintptr, float32)
	gtkGestureClickNew                     func() uintptr
	gtkGestureSingleSetButton              func(uintptr, uint32)
	gtkGestureDragNew                      func() uintptr
	gtkGestureZoomNew                      func() uintptr
	gtkGestureRotateNew                    func() uintptr
	gtkGestureGetBoundingBoxCenter         func(uintptr, *float64, *float64) int32
	gtkGestureSingleGetCurrentButton       func(uintptr) uint32
	gtkEventControllerMotionNew            func() uintptr
	gtkEventControllerKeyNew               func() uintptr
	gtkEventControllerScrollNew            func(int32) uintptr
	gtkEventControllerScrollSetFlags       func(uintptr, int32)
	gtkEventControllerScrollGetUnit        func(uintptr) int32 // from GTK 4.8
	gtkEventControllerFocusNew             func() uintptr
	gtkEventControllerSetPropagationPhase  func(uintptr, int32)
	gtkEventControllerGetCurrentEventState func(uintptr) uint32
	gtkEventControllerGetCurrentEvent      func(uintptr) uintptr
	gtkCssProviderNew                      func() uintptr
	gtkCssProviderLoadFromString           func(uintptr, *byte)
	gtkStyleContextAddProvider             func(uintptr, uintptr, uint32)
	gtkSettingsGetDefault                  func() uintptr
	gtkSettingsResetProperty               func(uintptr, *byte)
	gtkAlertDialogNew                      func(format *byte) uintptr
	gtkAlertDialogSetMessage               func(uintptr, *byte)
	gtkAlertDialogSetDetail                func(uintptr, *byte)
	gtkAlertDialogSetButtons               func(uintptr, uintptr)
	gtkAlertDialogSetCancelButton          func(uintptr, int32)
	gtkAlertDialogSetDefaultButton         func(uintptr, int32)
	gtkAlertDialogChoose                   func(uintptr, uintptr, uintptr, uintptr, uintptr)
	gtkAlertDialogChooseFinish             func(uintptr, uintptr, uintptr) int32
	gtkPopoverMenuBarNewFromModel          func(uintptr) uintptr
	gtkWindowGetDefaultSize                func(uintptr, *int32, *int32)

	// GDK, in libgtk-4.
	gdkFrameClockGetFrameTime     func(clock uintptr) int64
	gdkDisplayGetDefault          func() uintptr
	gdkDisplayGetMonitors         func(uintptr) uintptr
	gdkDisplayGetClipboard        func(uintptr) uintptr
	gdkClipboardSetText           func(uintptr, *byte)
	gdkClipboardSetContent        func(uintptr, uintptr) int32
	gdkContentProviderNewForBytes func(*byte, uintptr) uintptr
	gdkContentProviderNewUnion    func(*uintptr, uintptr) uintptr
	gdkClipboardReadTextAsync     func(uintptr, uintptr, uintptr, uintptr)
	gdkClipboardReadTextFinish    func(uintptr, uintptr, uintptr) uintptr
	gdkSurfaceGetScale            func(uintptr) float64
	gdkMonitorGetGeometry         func(uintptr, *gdkMonitorRectangle)
	gdkMonitorGetScaleFactor      func(uintptr) int32
	gdkMonitorGetConnector        func(uintptr) uintptr
	gdkToplevelBeginMove          func(uintptr, uintptr, int32, float64, float64, uint32)
	gdkToplevelLower              func(uintptr) int32
	gdkToplevelShowWindowMenu     func(uintptr, uintptr) int32
	gtkGestureGetDevice           func(uintptr) uintptr

	gdkDisplayGetMonitorAtSurface func(uintptr, uintptr) uintptr
	gdkMonitorGetRefreshRate      func(uintptr) int32
	// Optional GDK functions, nil where GTK lacks them: the fractional monitor scale (GTK 4.14), and the work area GTK 4 has only for X11 monitors, where GTK was built with X11.
	gdkMonitorGetScale       func(uintptr) float64
	gdkX11MonitorGetType     func() uintptr
	gdkX11MonitorGetWorkarea func(uintptr, *gdkMonitorRectangle)

	// GIO.
	gMenuNew                   func() uintptr
	gMenuAppend                func(uintptr, *byte, *byte)
	gMenuAppendSubmenu         func(uintptr, *byte, uintptr)
	gMenuAppendItem            func(uintptr, uintptr)
	gMenuAppendSection         func(uintptr, *byte, uintptr)
	gMenuInsertSubmenu         func(uintptr, int32, *byte, uintptr)
	gMenuInsertItem            func(uintptr, int32, uintptr)
	gMenuInsertSection         func(uintptr, int32, *byte, uintptr)
	gMenuRemove                func(uintptr, int32)
	gMenuItemNew               func(*byte, *byte) uintptr
	gMenuItemNewSubmenu        func(*byte, uintptr) uintptr
	gMenuItemSetIcon           func(uintptr, uintptr)
	gMenuItemSetAttributeValue func(uintptr, *byte, uintptr)
	gBytesIconNew              func(uintptr) uintptr
	gBytesIconGetType          func() uintptr
	gtkImageGetType            func() uintptr
	gtkImageGetGicon           func(uintptr) uintptr
	gSimpleActionNew           func(*byte, uintptr) uintptr
	gSimpleActionNewStateful   func(name, parameterType *byte, state uintptr) uintptr
	gSimpleActionSetEnabled    func(uintptr, int32)
	gSimpleActionSetState      func(uintptr, uintptr)
	gActionMapAddAction        func(uintptr, uintptr)
	gSimpleActionGroupNew      func() uintptr
	gtkWidgetInsertActionGroup func(uintptr, *byte, uintptr)
	gActionMapRemoveAction     func(uintptr, *byte)

	// Pango.
	pangoCairoFontMapNew              func() uintptr
	pangoFontMapCreateContext         func(uintptr) uintptr
	pangoFontMapLoadFont              func(uintptr, uintptr, uintptr) uintptr
	pangoFontMapUnref                 func(uintptr)
	pangoFontDescribeWithAbsoluteSize func(uintptr) uintptr
	pangoFontDescriptionToString      func(uintptr) uintptr
	pangoGlyphStringNew               func() uintptr
	pangoGlyphStringSetSize           func(uintptr, int32)
	pangoGlyphStringFree              func(uintptr)
	pangoLayoutNew                    func(uintptr) uintptr
	pangoLayoutSetAutoDir             func(uintptr, bool)
	pangoLayoutGetContext             func(uintptr) uintptr
	pangoContextSetBaseDir            func(uintptr, int32)
	pangoLayoutSetText                func(uintptr, *byte, int32)
	pangoLayoutGetText                func(uintptr) uintptr
	pangoLayoutSetWidth               func(uintptr, int32)
	pangoLayoutSetHeight              func(uintptr, int32)
	pangoLayoutSetWrap                func(uintptr, int32)
	pangoLayoutSetEllipsize           func(uintptr, int32)
	pangoLayoutSetAlignment           func(uintptr, int32)
	pangoLayoutSetJustify             func(uintptr, bool)
	pangoLayoutSetSpacing             func(uintptr, int32)
	pangoLayoutSetFontDescription     func(uintptr, uintptr)
	pangoLayoutSetAttributes          func(uintptr, uintptr)
	pangoLayoutContextChanged         func(uintptr)
	pangoLayoutGetSize                func(uintptr, *int32, *int32)
	pangoLayoutGetLineCount           func(uintptr) int32
	pangoLayoutGetBaseline            func(uintptr) int32
	pangoLayoutXyToIndex              func(uintptr, int32, int32, *int32, *int32) int32
	pangoLayoutIndexToPos             func(uintptr, int32, *pangoRectangle)
	pangoLayoutIterNextCluster        func(uintptr) int32
	pangoLayoutIterGetIndex           func(uintptr) int32
	pangoLayoutIterGetClusterExt      func(uintptr, *pangoRectangle, *pangoRectangle)
	pangoFontDescriptionFromString    func(*byte) uintptr
	pangoFontDescriptionNew           func() uintptr
	pangoFontDescriptionFree          func(uintptr)
	pangoFontDescriptionSetFamily     func(uintptr, *byte)
	pangoFontDescriptionGetFamily     func(uintptr) uintptr
	pangoFontDescriptionSetWeight     func(uintptr, int32)
	pangoFontDescriptionSetStyle      func(uintptr, int32)
	pangoFontDescriptionSetVariations func(uintptr, *byte)
	pangoAttrListNew                  func() uintptr
	pangoAttrListInsert               func(uintptr, uintptr)
	pangoAttrListUnref                func(uintptr)
	pangoAttrFontDescNew              func(uintptr) uintptr
	pangoAttrFamilyNew                func(*byte) uintptr
	pangoLanguageFromString           func(*byte) uintptr
	pangoAttrLanguageNew              func(uintptr) uintptr
	pangoAttrAllowBreaksNew           func(int32) uintptr
	pangoAttrWeightNew                func(int32) uintptr
	pangoAttrStyleNew                 func(int32) uintptr
	pangoAttrForegroundNew            func(uint16, uint16, uint16) uintptr
	pangoAttrForegroundAlphaNew       func(uint16) uintptr
	pangoAttrLetterSpacingNew         func(int32) uintptr
	pangoAttrLineHeightNew            func(int32) uintptr
	pangoAttrLineHeightAbsoluteNew    func(int32) uintptr
	pangoAttrUnderlineNew             func(int32) uintptr
	pangoAttrStrikethroughNew         func(int32) uintptr
	pangoAttrUnderlineColorNew        func(uint16, uint16, uint16) uintptr
	pangoAttrStrikethroughColorNew    func(uint16, uint16, uint16) uintptr
	pangoAttrShapeNew                 func(*pangoRectangle, *pangoRectangle) uintptr
	pangoAttrFontFeaturesNew          func(*byte) uintptr
	pangoContextGetMetrics            func(context, desc, language uintptr) uintptr
	pangoFontMetricsUnref             func(uintptr)
	pangoFontMetricsUnderlinePos      func(uintptr) int32
	pangoFontMetricsUnderlineThick    func(uintptr) int32
	pangoFontMetricsStrikePos         func(uintptr) int32
	pangoFontMetricsStrikeThick       func(uintptr) int32
	pangoLayoutGetExtents             func(uintptr, *pangoRectangle, *pangoRectangle)
	pangoLayoutGetIter                func(uintptr) uintptr
	pangoLayoutIterFree               func(uintptr)
	pangoLayoutIterNextLine           func(uintptr) int32
	pangoLayoutIterNextRun            func(uintptr) int32
	pangoLayoutIterGetLine            func(uintptr) uintptr
	pangoLayoutIterGetRun             func(uintptr) uintptr
	pangoLayoutIterGetBaseline        func(uintptr) int32
	pangoLayoutIterGetRunExtents      func(uintptr, *pangoRectangle, *pangoRectangle)
	pangoLayoutIterGetLineExtents     func(uintptr, *pangoRectangle, *pangoRectangle)
	pangoLayoutLineGetXRanges         func(uintptr, int32, int32, *uintptr, *int32)
	pangoCairoCreateLayout            func(uintptr) uintptr
	pangoCairoShowLayout              func(uintptr, uintptr)
	pangoCairoShowGlyphItem           func(uintptr, uintptr, uintptr)
	pangoCairoUpdateLayout            func(uintptr, uintptr)
	pangoCairoUpdateContext           func(uintptr, uintptr)

	// Cairo.
	cairoImageSurfaceCreate    func(int32, int32, int32) uintptr
	cairoCreate                func(uintptr) uintptr
	cairoDestroy               func(uintptr)
	cairoSurfaceDestroy        func(uintptr)
	cairoSurfaceFlush          func(uintptr)
	cairoSurfaceMarkDirty      func(uintptr)
	cairoImageSurfaceGetData   func(uintptr) uintptr
	cairoImageSurfaceGetStride func(uintptr) int32
	cairoSetSourceRGBA         func(uintptr, float64, float64, float64, float64)
	cairoPaint                 func(uintptr)
	cairoPaintWithAlpha        func(uintptr, float64)
	cairoRectangle             func(uintptr, float64, float64, float64, float64)
	cairoArc                   func(uintptr, float64, float64, float64, float64, float64)
	cairoArcNegative           func(uintptr, float64, float64, float64, float64, float64)
	cairoSetFillRule           func(uintptr, int32)
	cairoIdentityMatrix        func(uintptr)
	cairoNewSubPath            func(uintptr)
	cairoNewPath               func(uintptr)
	cairoGetCurrentPoint       func(uintptr, *float64, *float64)
	cairoClosePath             func(uintptr)
	cairoFill                  func(uintptr)
	cairoFillPreserve          func(uintptr)
	cairoStroke                func(uintptr)
	cairoSave                  func(uintptr)
	cairoRestore               func(uintptr)
	cairoTranslate             func(uintptr, float64, float64)
	cairoScale                 func(uintptr, float64, float64)
	cairoRotate                func(uintptr, float64)
	cairoClip                  func(uintptr)
	cairoMoveTo                func(uintptr, float64, float64)
	cairoLineTo                func(uintptr, float64, float64)
	cairoCurveTo               func(uintptr, float64, float64, float64, float64, float64, float64)
	cairoSetLineWidth          func(uintptr, float64)
	cairoSetLineCap            func(uintptr, int32)
	cairoSetLineJoin           func(uintptr, int32)
	cairoPushGroup             func(uintptr)
	cairoPopGroupToSource      func(uintptr)
	cairoSetOperator           func(uintptr, int32)
	cairoSetSourceSurface      func(uintptr, uintptr, float64, float64)
	cairoGetSource             func(uintptr) uintptr
	cairoPatternSetFilter      func(uintptr, int32)
	cairoSurfaceWriteToPNG     func(uintptr, *byte) int32
)

// Pango's absolute sizes are in Pango units of a logical pixel, the units of every NSGo size, where its point sizes would scale by the context's resolution.
var (
	pangoFontDescriptionSetAbsoluteSize func(uintptr, float64)
	pangoAttrSizeNewAbsolute            func(int32) uintptr
)

type gdkMonitorRectangle struct{ X, Y, Width, Height int32 }

// newCallback returns a C function pointer for the package-level
// callback fn. purego keeps the function for the process's life.
func newCallback(fn any) uintptr { return purego.NewCallback(fn) }

// cPointer turns a C address into a Go pointer. It reads the uintptr
// through an unsafe.Pointer so the conversion is explicit to go vet.
func cPointer(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

// Windows and views travel to C callbacks as integer handles, so no Go
// pointer is handed to C.
var (
	windowHandles sync.Map // uintptr -> *window
	nextHandle    atomic.Uintptr
)

// registerWindow gives w a handle C callbacks find it by.
func registerWindow(w *window) uintptr {
	h := nextHandle.Add(1)
	windowHandles.Store(h, w)
	return h
}

// windowOf returns the window a handler's data names, or nil.
func windowOf(data uintptr) *window {
	v, ok := windowHandles.Load(data)
	if !ok {
		return nil
	}
	return v.(*window)
}

// gSignalConnect connects fn to instance's signal. The callback must be
// one of the package-level functions passed through newCallback, so
// purego keeps it for the process's life.
func gSignalConnect(instance uintptr, signal string, fn uintptr, data uintptr) uint64 {
	return gSignalConnectData(instance, cstr(signal), fn, data, 0, 0)
}

// cstr returns s as a NUL-terminated C string.
func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

// setProgramIdentity names the process after the app's ID, which its desktop entry's name and StartupWMClass are, so docks match its windows to the entry. GTK 4 has no program class of its own: X11 windows take the program name as both parts of WM_CLASS, and Wayland toplevels take it as their app ID. It runs before gtk_init_check, as GDK's X11 client leader window reads it when the display opens.
func setProgramIdentity(id string) {
	if id == "" {
		return
	}
	gSetPrgname(cstr(id))
}

// cStrings is a NULL-terminated array of C strings and the Go memory
// holding them: the array keeps only their addresses, which the collector
// does not follow, so bufs keeps the strings alive while it is reachable.
type cStrings struct {
	ptrs []uintptr
	bufs [][]byte
}

// stringArray returns ss as C strings. Callers keep the result reachable,
// with runtime.KeepAlive, while the C side reads it.
func stringArray(ss []string) *cStrings {
	c := &cStrings{ptrs: make([]uintptr, len(ss)+1), bufs: make([][]byte, len(ss))}
	for i, s := range ss {
		c.bufs[i] = append([]byte(s), 0)
		c.ptrs[i] = uintptr(unsafe.Pointer(&c.bufs[i][0]))
	}
	return c
}

// array is the address of the array's first element.
func (c *cStrings) array() uintptr { return uintptr(unsafe.Pointer(&c.ptrs[0])) }

// bind registers every symbol once the libraries are open.
func bind() {
	// GLib.
	purego.RegisterLibFunc(&gMainContextIteration, glibLib, "g_main_context_iteration")
	purego.RegisterLibFunc(&gMainContextWakeup, glibLib, "g_main_context_wakeup")
	purego.RegisterLibFunc(&gTimeoutAdd, glibLib, "g_timeout_add")
	bindSyscall(&gGetMonotonicTime, glibLib, "g_get_monotonic_time")
	purego.RegisterLibFunc(&gSourceRemove, glibLib, "g_source_remove")
	purego.RegisterLibFunc(&gIdleAdd, glibLib, "g_idle_add")
	purego.RegisterLibFunc(&gErrorFree, glibLib, "g_error_free")
	purego.RegisterLibFunc(&gSetPrgname, glibLib, "g_set_prgname")
	purego.RegisterLibFunc(&gStrfreev, glibLib, "g_strfreev")
	purego.RegisterLibFunc(&gVariantNewString, glibLib, "g_variant_new_string")
	purego.RegisterLibFunc(&gVariantNewBoolean, glibLib, "g_variant_new_boolean")
	purego.RegisterLibFunc(&gVariantUnref, glibLib, "g_variant_unref")
	purego.RegisterLibFunc(&gObjectGetData, gobjectLib, "g_object_get_data")
	purego.RegisterLibFunc(&gObjectSetData, gobjectLib, "g_object_set_data")
	purego.RegisterLibFunc(&gTypeIsA, gobjectLib, "g_type_check_instance_is_a")
	bindSyscall(&gObjectRefSink, gobjectLib, "g_object_ref_sink")
	purego.RegisterLibFunc(&gSignalConnectData, gobjectLib, "g_signal_connect_data")
	purego.RegisterLibFunc(&gSignalConnectObj, gobjectLib, "g_signal_connect_object")
	purego.RegisterLibFunc(&gSignalEmitByName, gobjectLib, "g_signal_emit_by_name")
	purego.RegisterLibFunc(&gObjectGet, gobjectLib, "g_object_get")
	purego.RegisterLibFunc(&gObjectSetProperty, gobjectLib, "g_object_set_property")
	purego.RegisterLibFunc(&gObjectNotify, gobjectLib, "g_object_notify")

	// GTK.
	purego.RegisterLibFunc(&gtkInitCheck, gtkLib, "gtk_init_check")
	purego.RegisterLibFunc(&gtkWindowNew, gtkLib, "gtk_window_new")
	purego.RegisterLibFunc(&gtkWindowSetTitle, gtkLib, "gtk_window_set_title")
	purego.RegisterLibFunc(&gtkWindowGetTitle, gtkLib, "gtk_window_get_title")
	purego.RegisterLibFunc(&gtkWindowSetDefaultSize, gtkLib, "gtk_window_set_default_size")
	purego.RegisterLibFunc(&gtkWindowSetResizable, gtkLib, "gtk_window_set_resizable")
	purego.RegisterLibFunc(&gtkWindowGetDefaultSize, gtkLib, "gtk_window_get_default_size")
	purego.RegisterLibFunc(&gtkWindowSetChild, gtkLib, "gtk_window_set_child")
	purego.RegisterLibFunc(&gtkWindowSetFocus, gtkLib, "gtk_window_set_focus")
	purego.RegisterLibFunc(&gtkWindowGetFocus, gtkLib, "gtk_window_get_focus")
	purego.RegisterLibFunc(&gtkWindowPresent, gtkLib, "gtk_window_present")
	purego.RegisterLibFunc(&gtkWindowDestroy, gtkLib, "gtk_window_destroy")
	purego.RegisterLibFunc(&gtkWindowClose, gtkLib, "gtk_window_close")
	purego.RegisterLibFunc(&gtkWindowSetDecorated, gtkLib, "gtk_window_set_decorated")
	purego.RegisterLibFunc(&gtkWindowMinimize, gtkLib, "gtk_window_minimize")
	purego.RegisterLibFunc(&gtkWindowMaximize, gtkLib, "gtk_window_maximize")
	purego.RegisterLibFunc(&gtkWindowUnmaximize, gtkLib, "gtk_window_unmaximize")
	purego.RegisterLibFunc(&gtkWindowFullscreen, gtkLib, "gtk_window_fullscreen")
	purego.RegisterLibFunc(&gtkWindowFullscreenOnMonitor, gtkLib, "gtk_window_fullscreen_on_monitor")
	purego.RegisterLibFunc(&gtkWindowUnfullscreen, gtkLib, "gtk_window_unfullscreen")
	purego.RegisterLibFunc(&gtkWindowSetTransientFor, gtkLib, "gtk_window_set_transient_for")
	purego.RegisterLibFunc(&gtkWindowIsFullscreen, gtkLib, "gtk_window_is_fullscreen")
	purego.RegisterLibFunc(&gtkWindowIsMaximized, gtkLib, "gtk_window_is_maximized")
	purego.RegisterLibFunc(&gtkWindowIsActive, gtkLib, "gtk_window_is_active")
	purego.RegisterLibFunc(&gdkToplevelGetState, gtkLib, "gdk_toplevel_get_state")
	purego.RegisterLibFunc(&gtkWidgetShow, gtkLib, "gtk_widget_show")
	purego.RegisterLibFunc(&gtkWidgetSetVisible, gtkLib, "gtk_widget_set_visible")
	purego.RegisterLibFunc(&gtkWidgetGetVisible, gtkLib, "gtk_widget_get_visible")
	purego.RegisterLibFunc(&gtkWidgetAddController, gtkLib, "gtk_widget_add_controller")
	bindSyscall(&gtkWidgetQueueDraw, gtkLib, "gtk_widget_queue_draw")
	purego.RegisterLibFunc(&gtkWidgetAddTickCallback, gtkLib, "gtk_widget_add_tick_callback")
	purego.RegisterLibFunc(&gtkWidgetRemoveTickCallback, gtkLib, "gtk_widget_remove_tick_callback")
	bindSyscall(&gtkWidgetQueueAllocate, gtkLib, "gtk_widget_queue_allocate")
	bindSyscall(&gtkWidgetGetMapped, gtkLib, "gtk_widget_get_mapped")
	purego.RegisterLibFunc(&gtkWidgetGrabFocus, gtkLib, "gtk_widget_grab_focus")
	bindSyscall(&gtkWidgetGetRoot, gtkLib, "gtk_widget_get_root")
	bindSyscall(&gtkWidgetSetFocusable, gtkLib, "gtk_widget_set_focusable")
	bindSyscall(&gtkWidgetGetFocusable, gtkLib, "gtk_widget_get_focusable")
	bindSyscall(&gtkWidgetGetWidth, gtkLib, "gtk_widget_get_width")
	bindSyscall(&gtkWidgetGetHeight, gtkLib, "gtk_widget_get_height")
	bindSyscall(&gtkWidgetGetNative, gtkLib, "gtk_widget_get_native")
	bindSyscall(&gtkWidgetGetScaleFactor, gtkLib, "gtk_widget_get_scale_factor")
	purego.RegisterLibFunc(&gtkWidgetGetStyleContext, gtkLib, "gtk_widget_get_style_context")
	bindSyscall(&gtkWidgetGetParent, gtkLib, "gtk_widget_get_parent")
	purego.RegisterLibFunc(&gtkWidgetHasFocus, gtkLib, "gtk_widget_has_focus")
	purego.RegisterLibFunc(&gtkWidgetActivateAction, gtkLib, "gtk_widget_activate_action")
	purego.RegisterLibFunc(&gtkWidgetGetDefaultDirection, gtkLib, "gtk_widget_get_default_direction")
	purego.RegisterLibFunc(&gtkWidgetSetDefaultDirection, gtkLib, "gtk_widget_set_default_direction")
	purego.RegisterLibFunc(&gtkWidgetGetDirection, gtkLib, "gtk_widget_get_direction")
	purego.RegisterLibFunc(&gtkWidgetSetDirection, gtkLib, "gtk_widget_set_direction")
	purego.RegisterLibFunc(&gtkWidgetSetSizeRequest, gtkLib, "gtk_widget_set_size_request")
	purego.RegisterLibFunc(&gtkWidgetSetHexpand, gtkLib, "gtk_widget_set_hexpand")
	purego.RegisterLibFunc(&gtkWidgetSetVexpand, gtkLib, "gtk_widget_set_vexpand")
	purego.RegisterLibFunc(&gtkWidgetSetHalign, gtkLib, "gtk_widget_set_halign")
	purego.RegisterLibFunc(&gtkWidgetSetValign, gtkLib, "gtk_widget_set_valign")
	purego.RegisterLibFunc(&gtkWidgetSetOverflow, gtkLib, "gtk_widget_set_overflow")
	purego.RegisterLibFunc(&gtkWidgetSetSensitive, gtkLib, "gtk_widget_set_sensitive")
	purego.RegisterLibFunc(&gtkWidgetSetCursor, gtkLib, "gtk_widget_set_cursor")
	purego.RegisterLibFunc(&gtkWidgetSetCursorFromName, gtkLib, "gtk_widget_set_cursor_from_name")
	purego.RegisterLibFunc(&gdkCursorNewFromTexture, gtkLib, "gdk_cursor_new_from_texture")
	purego.RegisterLibFunc(&gdkCursorNewFromName, gtkLib, "gdk_cursor_new_from_name")
	purego.RegisterLibFunc(&gtkWidgetActivate, gtkLib, "gtk_widget_activate")
	purego.RegisterLibFunc(&gtkNativeGetSurface, gtkLib, "gtk_native_get_surface")
	purego.RegisterLibFunc(&gtkBoxNew, gtkLib, "gtk_box_new")
	purego.RegisterLibFunc(&gtkBoxAppend, gtkLib, "gtk_box_append")
	purego.RegisterLibFunc(&gtkBoxPrepend, gtkLib, "gtk_box_prepend")
	purego.RegisterLibFunc(&gtkBoxRemove, gtkLib, "gtk_box_remove")
	purego.RegisterLibFunc(&gtkEntryNew, gtkLib, "gtk_entry_new")
	purego.RegisterLibFunc(&gtkEntryGrabFocusWithoutSelecting, gtkLib, "gtk_entry_grab_focus_without_selecting")
	purego.RegisterLibFunc(&gtkEditableSetText, gtkLib, "gtk_editable_set_text")
	purego.RegisterLibFunc(&gtkEditableGetText, gtkLib, "gtk_editable_get_text")
	purego.RegisterLibFunc(&gtkEditableSetEditable, gtkLib, "gtk_editable_set_editable")
	purego.RegisterLibFunc(&gtkEditableSelectRegion, gtkLib, "gtk_editable_select_region")
	purego.RegisterLibFunc(&gtkEditableDeleteSelection, gtkLib, "gtk_editable_delete_selection")
	purego.RegisterLibFunc(&gtkEntrySetPlaceholderText, gtkLib, "gtk_entry_set_placeholder_text")
	purego.RegisterLibFunc(&gtkEntrySetVisibility, gtkLib, "gtk_entry_set_visibility")
	purego.RegisterLibFunc(&gtkEntrySetAlignment, gtkLib, "gtk_entry_set_alignment")
	purego.RegisterLibFunc(&gtkGestureClickNew, gtkLib, "gtk_gesture_click_new")
	purego.RegisterLibFunc(&gtkGestureSingleSetButton, gtkLib, "gtk_gesture_single_set_button")
	purego.RegisterLibFunc(&gtkGestureDragNew, gtkLib, "gtk_gesture_drag_new")
	purego.RegisterLibFunc(&gtkGestureZoomNew, gtkLib, "gtk_gesture_zoom_new")
	purego.RegisterLibFunc(&gtkGestureRotateNew, gtkLib, "gtk_gesture_rotate_new")
	purego.RegisterLibFunc(&gtkGestureGetBoundingBoxCenter, gtkLib, "gtk_gesture_get_bounding_box_center")
	purego.RegisterLibFunc(&gtkGestureSingleGetCurrentButton, gtkLib, "gtk_gesture_single_get_current_button")
	purego.RegisterLibFunc(&gtkEventControllerMotionNew, gtkLib, "gtk_event_controller_motion_new")
	purego.RegisterLibFunc(&gtkEventControllerKeyNew, gtkLib, "gtk_event_controller_key_new")
	purego.RegisterLibFunc(&gtkEventControllerScrollNew, gtkLib, "gtk_event_controller_scroll_new")
	purego.RegisterLibFunc(&gtkEventControllerScrollSetFlags, gtkLib, "gtk_event_controller_scroll_set_flags")
	if _, err := purego.Dlsym(gtkLib, "gtk_event_controller_scroll_get_unit"); err == nil {
		purego.RegisterLibFunc(&gtkEventControllerScrollGetUnit, gtkLib, "gtk_event_controller_scroll_get_unit")
	}
	purego.RegisterLibFunc(&gtkEventControllerFocusNew, gtkLib, "gtk_event_controller_focus_new")
	purego.RegisterLibFunc(&gtkEventControllerSetPropagationPhase, gtkLib, "gtk_event_controller_set_propagation_phase")
	purego.RegisterLibFunc(&gtkEventControllerGetCurrentEventState, gtkLib, "gtk_event_controller_get_current_event_state")
	purego.RegisterLibFunc(&gtkEventControllerGetCurrentEvent, gtkLib, "gtk_event_controller_get_current_event")
	purego.RegisterLibFunc(&gtkCssProviderNew, gtkLib, "gtk_css_provider_new")
	purego.RegisterLibFunc(&gtkCssProviderLoadFromString, gtkLib, "gtk_css_provider_load_from_string")
	purego.RegisterLibFunc(&gtkStyleContextAddProvider, gtkLib, "gtk_style_context_add_provider")
	purego.RegisterLibFunc(&gtkSettingsGetDefault, gtkLib, "gtk_settings_get_default")
	purego.RegisterLibFunc(&gtkSettingsResetProperty, gtkLib, "gtk_settings_reset_property")
	purego.RegisterLibFunc(&gtkAlertDialogNew, gtkLib, "gtk_alert_dialog_new")
	purego.RegisterLibFunc(&gtkAlertDialogSetMessage, gtkLib, "gtk_alert_dialog_set_message")
	purego.RegisterLibFunc(&gtkAlertDialogSetDetail, gtkLib, "gtk_alert_dialog_set_detail")
	purego.RegisterLibFunc(&gtkAlertDialogSetButtons, gtkLib, "gtk_alert_dialog_set_buttons")
	purego.RegisterLibFunc(&gtkAlertDialogSetCancelButton, gtkLib, "gtk_alert_dialog_set_cancel_button")
	purego.RegisterLibFunc(&gtkAlertDialogSetDefaultButton, gtkLib, "gtk_alert_dialog_set_default_button")
	purego.RegisterLibFunc(&gtkAlertDialogChoose, gtkLib, "gtk_alert_dialog_choose")
	purego.RegisterLibFunc(&gtkAlertDialogChooseFinish, gtkLib, "gtk_alert_dialog_choose_finish")
	purego.RegisterLibFunc(&gtkPopoverMenuBarNewFromModel, gtkLib, "gtk_popover_menu_bar_new_from_model")
	purego.RegisterLibFunc(&gtkGestureGetDevice, gtkLib, "gtk_gesture_get_device")

	// GDK.
	bindSyscall(&gdkFrameClockGetFrameTime, gtkLib, "gdk_frame_clock_get_frame_time")
	purego.RegisterLibFunc(&gdkDisplayGetDefault, gtkLib, "gdk_display_get_default")
	purego.RegisterLibFunc(&gdkDisplayGetMonitors, gtkLib, "gdk_display_get_monitors")
	purego.RegisterLibFunc(&gdkDisplayGetMonitorAtSurface, gtkLib, "gdk_display_get_monitor_at_surface")
	purego.RegisterLibFunc(&gdkMonitorGetRefreshRate, gtkLib, "gdk_monitor_get_refresh_rate")
	purego.RegisterLibFunc(&gdkDisplayGetClipboard, gtkLib, "gdk_display_get_clipboard")
	purego.RegisterLibFunc(&gdkClipboardSetText, gtkLib, "gdk_clipboard_set_text")
	purego.RegisterLibFunc(&gdkClipboardSetContent, gtkLib, "gdk_clipboard_set_content")
	purego.RegisterLibFunc(&gdkContentProviderNewForBytes, gtkLib, "gdk_content_provider_new_for_bytes")
	purego.RegisterLibFunc(&gdkContentProviderNewUnion, gtkLib, "gdk_content_provider_new_union")
	purego.RegisterLibFunc(&gdkClipboardReadTextAsync, gtkLib, "gdk_clipboard_read_text_async")
	purego.RegisterLibFunc(&gdkClipboardReadTextFinish, gtkLib, "gdk_clipboard_read_text_finish")
	purego.RegisterLibFunc(&gdkSurfaceGetScale, gtkLib, "gdk_surface_get_scale")
	purego.RegisterLibFunc(&gdkMonitorGetGeometry, gtkLib, "gdk_monitor_get_geometry")
	purego.RegisterLibFunc(&gdkMonitorGetScaleFactor, gtkLib, "gdk_monitor_get_scale_factor")
	purego.RegisterLibFunc(&gdkMonitorGetConnector, gtkLib, "gdk_monitor_get_connector")
	if _, err := purego.Dlsym(gtkLib, "gdk_monitor_get_scale"); err == nil {
		purego.RegisterLibFunc(&gdkMonitorGetScale, gtkLib, "gdk_monitor_get_scale")
	}
	if _, err := purego.Dlsym(gtkLib, "gdk_x11_monitor_get_workarea"); err == nil {
		if _, err := purego.Dlsym(gtkLib, "gdk_x11_monitor_get_type"); err == nil {
			purego.RegisterLibFunc(&gdkX11MonitorGetType, gtkLib, "gdk_x11_monitor_get_type")
			purego.RegisterLibFunc(&gdkX11MonitorGetWorkarea, gtkLib, "gdk_x11_monitor_get_workarea")
		}
	}
	purego.RegisterLibFunc(&gdkToplevelBeginMove, gtkLib, "gdk_toplevel_begin_move")
	purego.RegisterLibFunc(&gdkToplevelLower, gtkLib, "gdk_toplevel_lower")
	purego.RegisterLibFunc(&gdkToplevelShowWindowMenu, gtkLib, "gdk_toplevel_show_window_menu")

	// GIO.
	purego.RegisterLibFunc(&gMenuNew, gioLib, "g_menu_new")
	purego.RegisterLibFunc(&gMenuAppend, gioLib, "g_menu_append")
	purego.RegisterLibFunc(&gMenuAppendSubmenu, gioLib, "g_menu_append_submenu")
	purego.RegisterLibFunc(&gMenuAppendItem, gioLib, "g_menu_append_item")
	purego.RegisterLibFunc(&gMenuAppendSection, gioLib, "g_menu_append_section")
	purego.RegisterLibFunc(&gMenuInsertSubmenu, gioLib, "g_menu_insert_submenu")
	purego.RegisterLibFunc(&gMenuInsertItem, gioLib, "g_menu_insert_item")
	purego.RegisterLibFunc(&gMenuInsertSection, gioLib, "g_menu_insert_section")
	purego.RegisterLibFunc(&gMenuRemove, gioLib, "g_menu_remove")
	purego.RegisterLibFunc(&gMenuItemNew, gioLib, "g_menu_item_new")
	purego.RegisterLibFunc(&gMenuItemNewSubmenu, gioLib, "g_menu_item_new_submenu")
	purego.RegisterLibFunc(&gMenuItemSetIcon, gioLib, "g_menu_item_set_icon")
	purego.RegisterLibFunc(&gMenuItemSetAttributeValue, gioLib, "g_menu_item_set_attribute_value")
	purego.RegisterLibFunc(&gBytesIconNew, gioLib, "g_bytes_icon_new")
	purego.RegisterLibFunc(&gBytesIconGetType, gioLib, "g_bytes_icon_get_type")
	purego.RegisterLibFunc(&gtkImageGetType, gtkLib, "gtk_image_get_type")
	purego.RegisterLibFunc(&gtkImageGetGicon, gtkLib, "gtk_image_get_gicon")
	purego.RegisterLibFunc(&gSimpleActionNew, gioLib, "g_simple_action_new")
	purego.RegisterLibFunc(&gSimpleActionNewStateful, gioLib, "g_simple_action_new_stateful")
	purego.RegisterLibFunc(&gSimpleActionSetEnabled, gioLib, "g_simple_action_set_enabled")
	purego.RegisterLibFunc(&gSimpleActionSetState, gioLib, "g_simple_action_set_state")
	purego.RegisterLibFunc(&gActionMapAddAction, gioLib, "g_action_map_add_action")
	purego.RegisterLibFunc(&gSimpleActionGroupNew, gioLib, "g_simple_action_group_new")
	purego.RegisterLibFunc(&gtkWidgetInsertActionGroup, gtkLib, "gtk_widget_insert_action_group")
	purego.RegisterLibFunc(&gActionMapRemoveAction, gioLib, "g_action_map_remove_action")

	bindNative()
	bindWidget()
	bindRasterGTK()
	bindDecor()
	bindFileDrop()
	bindPaste()
	bindSystem()
	bindEditing()
	bindAccess()
	bindTouchScroll()
	bindTouches()
	bindWindowPointer()
}

// bindCanvasLibs binds the GLib, GObject, Pango and Cairo symbols needed by
// text layout and canvas playback without GTK or a display.
func bindCanvasLibs() {
	bindSyscall(&gFree, glibLib, "g_free")
	bindSyscall(&gObjectRef, gobjectLib, "g_object_ref")
	bindSyscall(&gObjectUnref, gobjectLib, "g_object_unref")

	// Pango.
	purego.RegisterLibFunc(&pangoCairoFontMapNew, pangocairoLib, "pango_cairo_font_map_new")
	purego.RegisterLibFunc(&pangoCairoFontMapGetDefault, pangocairoLib, "pango_cairo_font_map_get_default")
	// Pango's fontconfig font maps, the default one among them.
	if ft2, err := purego.Dlopen("libpangoft2-1.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
		purego.RegisterLibFunc(&pangoFcFontMapConfigChanged, ft2, "pango_fc_font_map_config_changed")
	}
	bindSyscall(&pangoFontMapCreateContext, pangoLib, "pango_font_map_create_context")
	bindSyscall(&pangoFontMapLoadFont, pangoLib, "pango_font_map_load_font")
	bindSyscall(&pangoFontMapUnref, gobjectLib, "g_object_unref") // PangoFontMap is a GObject
	bindSyscall(&pangoFontDescribeWithAbsoluteSize, pangoLib, "pango_font_describe_with_absolute_size")
	bindSyscall(&pangoFontDescriptionToString, pangoLib, "pango_font_description_to_string")
	bindSyscall(&pangoGlyphStringNew, pangoLib, "pango_glyph_string_new")
	bindSyscall(&pangoGlyphStringSetSize, pangoLib, "pango_glyph_string_set_size")
	bindSyscall(&pangoGlyphStringFree, pangoLib, "pango_glyph_string_free")
	bindSyscall(&pangoLayoutNew, pangoLib, "pango_layout_new")
	bindSyscall(&pangoLayoutSetAutoDir, pangoLib, "pango_layout_set_auto_dir")
	bindSyscall(&pangoLayoutGetContext, pangoLib, "pango_layout_get_context")
	bindSyscall(&pangoContextSetBaseDir, pangoLib, "pango_context_set_base_dir")
	bindSyscall(&pangoLayoutSetText, pangoLib, "pango_layout_set_text")
	bindSyscall(&pangoLayoutGetText, pangoLib, "pango_layout_get_text")
	bindSyscall(&pangoLayoutSetWidth, pangoLib, "pango_layout_set_width")
	bindSyscall(&pangoLayoutSetHeight, pangoLib, "pango_layout_set_height")
	bindSyscall(&pangoLayoutSetWrap, pangoLib, "pango_layout_set_wrap")
	bindSyscall(&pangoLayoutSetEllipsize, pangoLib, "pango_layout_set_ellipsize")
	bindSyscall(&pangoLayoutSetAlignment, pangoLib, "pango_layout_set_alignment")
	bindSyscall(&pangoLayoutSetJustify, pangoLib, "pango_layout_set_justify")
	bindSyscall(&pangoLayoutSetSpacing, pangoLib, "pango_layout_set_spacing")
	bindSyscall(&pangoLayoutSetFontDescription, pangoLib, "pango_layout_set_font_description")
	bindSyscall(&pangoLayoutSetAttributes, pangoLib, "pango_layout_set_attributes")
	bindSyscall(&pangoLayoutContextChanged, pangoLib, "pango_layout_context_changed")
	bindSyscall(&pangoLayoutGetSize, pangoLib, "pango_layout_get_size")
	bindSyscall(&pangoLayoutGetLineCount, pangoLib, "pango_layout_get_line_count")
	bindSyscall(&pangoLayoutGetBaseline, pangoLib, "pango_layout_get_baseline")
	purego.RegisterLibFunc(&pangoLayoutXyToIndex, pangoLib, "pango_layout_xy_to_index")
	purego.RegisterLibFunc(&pangoLayoutIndexToPos, pangoLib, "pango_layout_index_to_pos")
	bindSyscall(&pangoLayoutIterNextCluster, pangoLib, "pango_layout_iter_next_cluster")
	bindSyscall(&pangoLayoutIterGetIndex, pangoLib, "pango_layout_iter_get_index")
	bindSyscall(&pangoLayoutIterGetClusterExt, pangoLib, "pango_layout_iter_get_cluster_extents")
	bindSyscall(&pangoFontDescriptionFromString, pangoLib, "pango_font_description_from_string")
	bindSyscall(&pangoFontDescriptionGetFamily, pangoLib, "pango_font_description_get_family")
	bindSyscall(&pangoFontDescriptionNew, pangoLib, "pango_font_description_new")
	bindSyscall(&pangoFontDescriptionFree, pangoLib, "pango_font_description_free")
	purego.RegisterLibFunc(&pangoFontDescriptionSetFamily, pangoLib, "pango_font_description_set_family")
	purego.RegisterLibFunc(&pangoFontDescriptionSetAbsoluteSize, pangoLib, "pango_font_description_set_absolute_size")
	bindSyscall(&pangoFontDescriptionSetWeight, pangoLib, "pango_font_description_set_weight")
	bindSyscall(&pangoFontDescriptionSetStyle, pangoLib, "pango_font_description_set_style")
	purego.RegisterLibFunc(&pangoFontDescriptionSetVariations, pangoLib, "pango_font_description_set_variations")
	bindSyscall(&pangoAttrListNew, pangoLib, "pango_attr_list_new")
	bindSyscall(&pangoAttrListInsert, pangoLib, "pango_attr_list_insert")
	bindSyscall(&pangoAttrListUnref, pangoLib, "pango_attr_list_unref")
	bindSyscall(&pangoAttrFontDescNew, pangoLib, "pango_attr_font_desc_new")
	bindSyscall(&pangoAttrFamilyNew, pangoLib, "pango_attr_family_new")
	bindSyscall(&pangoLanguageFromString, pangoLib, "pango_language_from_string")
	bindSyscall(&pangoAttrLanguageNew, pangoLib, "pango_attr_language_new")
	bindSyscall(&pangoAttrAllowBreaksNew, pangoLib, "pango_attr_allow_breaks_new")
	bindSyscall(&pangoAttrSizeNewAbsolute, pangoLib, "pango_attr_size_new_absolute")
	bindSyscall(&pangoAttrWeightNew, pangoLib, "pango_attr_weight_new")
	bindSyscall(&pangoAttrStyleNew, pangoLib, "pango_attr_style_new")
	bindSyscall(&pangoAttrForegroundNew, pangoLib, "pango_attr_foreground_new")
	purego.RegisterLibFunc(&pangoAttrForegroundAlphaNew, pangoLib, "pango_attr_foreground_alpha_new")
	bindSyscall(&pangoAttrLetterSpacingNew, pangoLib, "pango_attr_letter_spacing_new")
	bindSyscall(&pangoAttrLineHeightNew, pangoLib, "pango_attr_line_height_new")
	bindSyscall(&pangoAttrLineHeightAbsoluteNew, pangoLib, "pango_attr_line_height_new_absolute")
	bindSyscall(&pangoAttrUnderlineNew, pangoLib, "pango_attr_underline_new")
	bindSyscall(&pangoAttrStrikethroughNew, pangoLib, "pango_attr_strikethrough_new")
	bindSyscall(&pangoAttrUnderlineColorNew, pangoLib, "pango_attr_underline_color_new")
	bindSyscall(&pangoAttrStrikethroughColorNew, pangoLib, "pango_attr_strikethrough_color_new")
	purego.RegisterLibFunc(&pangoAttrShapeNew, pangoLib, "pango_attr_shape_new")
	bindSyscall(&pangoAttrFontFeaturesNew, pangoLib, "pango_attr_font_features_new")
	bindSyscall(&pangoContextGetMetrics, pangoLib, "pango_context_get_metrics")
	bindSyscall(&pangoFontMetricsUnref, pangoLib, "pango_font_metrics_unref")
	bindSyscall(&pangoFontMetricsUnderlinePos, pangoLib, "pango_font_metrics_get_underline_position")
	bindSyscall(&pangoFontMetricsUnderlineThick, pangoLib, "pango_font_metrics_get_underline_thickness")
	bindSyscall(&pangoFontMetricsStrikePos, pangoLib, "pango_font_metrics_get_strikethrough_position")
	bindSyscall(&pangoFontMetricsStrikeThick, pangoLib, "pango_font_metrics_get_strikethrough_thickness")
	bindSyscall(&pangoLayoutGetExtents, pangoLib, "pango_layout_get_extents")
	bindSyscall(&pangoLayoutGetIter, pangoLib, "pango_layout_get_iter")
	bindSyscall(&pangoLayoutIterFree, pangoLib, "pango_layout_iter_free")
	bindSyscall(&pangoLayoutIterNextLine, pangoLib, "pango_layout_iter_next_line")
	bindSyscall(&pangoLayoutIterNextRun, pangoLib, "pango_layout_iter_next_run")
	bindSyscall(&pangoLayoutIterGetLine, pangoLib, "pango_layout_iter_get_line_readonly")
	bindSyscall(&pangoLayoutIterGetRun, pangoLib, "pango_layout_iter_get_run_readonly")
	bindSyscall(&pangoLayoutIterGetBaseline, pangoLib, "pango_layout_iter_get_baseline")
	bindSyscall(&pangoLayoutIterGetRunExtents, pangoLib, "pango_layout_iter_get_run_extents")
	bindSyscall(&pangoLayoutIterGetLineExtents, pangoLib, "pango_layout_iter_get_line_extents")
	purego.RegisterLibFunc(&pangoLayoutLineGetXRanges, pangoLib, "pango_layout_line_get_x_ranges")
	bindSyscall(&pangoCairoCreateLayout, pangocairoLib, "pango_cairo_create_layout")
	bindSyscall(&pangoCairoShowLayout, pangocairoLib, "pango_cairo_show_layout")
	bindSyscall(&pangoCairoShowGlyphItem, pangocairoLib, "pango_cairo_show_glyph_item")
	bindSyscall(&pangoCairoUpdateLayout, pangocairoLib, "pango_cairo_update_layout")
	bindSyscall(&pangoCairoUpdateContext, pangocairoLib, "pango_cairo_update_context")

	// Cairo.
	purego.RegisterLibFunc(&cairoImageSurfaceCreate, cairoLib, "cairo_image_surface_create")
	bindSyscall(&cairoCreate, cairoLib, "cairo_create")
	bindSyscall(&cairoDestroy, cairoLib, "cairo_destroy")
	bindSyscall(&cairoSurfaceDestroy, cairoLib, "cairo_surface_destroy")
	bindSyscall(&cairoSurfaceFlush, cairoLib, "cairo_surface_flush")
	bindSyscall(&cairoSurfaceMarkDirty, cairoLib, "cairo_surface_mark_dirty")
	bindSyscall(&cairoImageSurfaceGetData, cairoLib, "cairo_image_surface_get_data")
	bindSyscall(&cairoImageSurfaceGetStride, cairoLib, "cairo_image_surface_get_stride")
	purego.RegisterLibFunc(&cairoSetSourceRGBA, cairoLib, "cairo_set_source_rgba")
	purego.RegisterLibFunc(&cairoPaint, cairoLib, "cairo_paint")
	purego.RegisterLibFunc(&cairoPaintWithAlpha, cairoLib, "cairo_paint_with_alpha")
	purego.RegisterLibFunc(&cairoRectangle, cairoLib, "cairo_rectangle")
	purego.RegisterLibFunc(&cairoArc, cairoLib, "cairo_arc")
	purego.RegisterLibFunc(&cairoArcNegative, cairoLib, "cairo_arc_negative")
	purego.RegisterLibFunc(&cairoSetFillRule, cairoLib, "cairo_set_fill_rule")
	purego.RegisterLibFunc(&cairoIdentityMatrix, cairoLib, "cairo_identity_matrix")
	purego.RegisterLibFunc(&cairoNewSubPath, cairoLib, "cairo_new_sub_path")
	purego.RegisterLibFunc(&cairoNewPath, cairoLib, "cairo_new_path")
	purego.RegisterLibFunc(&cairoGetCurrentPoint, cairoLib, "cairo_get_current_point")
	purego.RegisterLibFunc(&cairoClosePath, cairoLib, "cairo_close_path")
	purego.RegisterLibFunc(&cairoFill, cairoLib, "cairo_fill")
	purego.RegisterLibFunc(&cairoFillPreserve, cairoLib, "cairo_fill_preserve")
	purego.RegisterLibFunc(&cairoStroke, cairoLib, "cairo_stroke")
	purego.RegisterLibFunc(&cairoSave, cairoLib, "cairo_save")
	purego.RegisterLibFunc(&cairoRestore, cairoLib, "cairo_restore")
	purego.RegisterLibFunc(&cairoTranslate, cairoLib, "cairo_translate")
	purego.RegisterLibFunc(&cairoScale, cairoLib, "cairo_scale")
	purego.RegisterLibFunc(&cairoRotate, cairoLib, "cairo_rotate")
	purego.RegisterLibFunc(&cairoClip, cairoLib, "cairo_clip")
	purego.RegisterLibFunc(&cairoMoveTo, cairoLib, "cairo_move_to")
	purego.RegisterLibFunc(&cairoLineTo, cairoLib, "cairo_line_to")
	purego.RegisterLibFunc(&cairoCurveTo, cairoLib, "cairo_curve_to")
	purego.RegisterLibFunc(&cairoSetLineWidth, cairoLib, "cairo_set_line_width")
	purego.RegisterLibFunc(&cairoSetLineCap, cairoLib, "cairo_set_line_cap")
	purego.RegisterLibFunc(&cairoSetLineJoin, cairoLib, "cairo_set_line_join")
	purego.RegisterLibFunc(&cairoPushGroup, cairoLib, "cairo_push_group")
	purego.RegisterLibFunc(&cairoPopGroupToSource, cairoLib, "cairo_pop_group_to_source")
	purego.RegisterLibFunc(&cairoSetOperator, cairoLib, "cairo_set_operator")
	purego.RegisterLibFunc(&cairoSetSourceSurface, cairoLib, "cairo_set_source_surface")
	purego.RegisterLibFunc(&cairoGetSource, cairoLib, "cairo_get_source")
	purego.RegisterLibFunc(&cairoPatternSetFilter, cairoLib, "cairo_pattern_set_filter")
	purego.RegisterLibFunc(&cairoSurfaceWriteToPNG, cairoLib, "cairo_surface_write_to_png")
	bindCanvas(cairoLib, pangocairoLib)
}
