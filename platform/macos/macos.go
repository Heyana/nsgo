//go:build darwin && !ios

// Package macos is NSGo's AppKit backend: one flipped, layer-backed NSView
// per node, styled through its CALayer, with text the engine laid out
// with CoreText drawn on the main thread and images decoded off it.
// Alerts are sheets on the window; Escape, Cmd-[, Option-Left and the
// mouse's back button go back. It calls AppKit through purego, so it
// builds without cgo.
package macos

import (
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
	"github.com/oneo-me/nsgo/platform/internal/apple"
	"github.com/oneo-me/nsgo/platform/internal/queue"
)

func init() {
	// AppKit runs on the process's main thread, which main starts on.
	runtime.LockOSThread()
}

// Options configure the window.
type Options struct {
	Title         string
	Width, Height core.Points
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
}

// longPressDelay is how long a press is held before it is a long press.
const longPressDelay = 500 * time.Millisecond

// view is the Go side of one native view.
type view struct {
	surface *surface
	id      core.NodeID
	kind    core.ViewKind
	native  objc.ID
	content objc.ID // where children go: native, or a scroll view's document
	events  core.EventMask

	pressing  atomic.Bool
	longFired atomic.Bool
	hover     objc.ID // the tracking area of a view reporting hovers

	layout *apple.Layout // a text view's
	box    core.Bounds

	field  objc.ID // an input's NSTextField
	clip   objc.ID // a scroll view's NSClipView
	masked bool
	inset  core.Bounds
	edits  uint32

	image     *apple.ImageView
	animation core.ModalAnimation // a modal's
	presented bool                // whether a modal was inserted yet
	transform core.Transform      // AppKit drops it when the frame changes
	drawing   *core.Drawing       // a canvas's

	windowDrag core.WindowDrag
	activation core.PanActivation

	// A text's content and selection, and whether a view is an area.
	plain            string
	selectable       core.Selectable
	selColor         core.Color
	selStart, selEnd int // UTF-16
	area             bool
}

type surface struct {
	native  objc.ID
	window  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []*view // index is NodeID
	env     core.Environment
	keys    map[string]bool // the keymap's bindings

	// What a batch touched, finished once its frames are set.
	images, presenting []*view

	// The gestures in progress.
	pan             *panTrack
	pinch, rotation *gestureTrack
	selecting       *selection

	chrome
}

// active is the surface observers report to.
var active *surface

// views maps native objects (views, fields, clip views) to their Go side.
// Only the main thread uses it.
var views = map[objc.ID]*view{}

var (
	sel = objc.RegisterName

	selAlloc, selInit, selNew       = sel("alloc"), sel("init"), sel("new")
	selRelease                      = sel("release")
	selSetWantsLayer, selLayer      = sel("setWantsLayer:"), sel("layer")
	selSetFrame, selBounds          = sel("setFrame:"), sel("bounds")
	selSetFrameSize                 = sel("setFrameSize:")
	selAddSubview, selSuperview     = sel("addSubview:"), sel("superview")
	selAddSubviewPositioned         = sel("addSubview:positioned:relativeTo:")
	selRemoveFromSuperview          = sel("removeFromSuperview")
	selSetHidden                    = sel("setHidden:")
	selSetNeedsDisplay              = sel("setNeedsDisplay:")
	selSetNeedsLayout               = sel("setNeedsLayout:")
	selConvertPoint                 = sel("convertPoint:fromView:")
	selLocationInWindow             = sel("locationInWindow")
	selStringValue, selSetString    = sel("stringValue"), sel("setStringValue:")
	selSetFont, selSetTextColor     = sel("setFont:"), sel("setTextColor:")
	selSetAlignment                 = sel("setAlignment:")
	selSetPlaceholder               = sel("setPlaceholderString:")
	selSetEditable, selSetEnabled   = sel("setEditable:"), sel("setEnabled:")
	selSetDelegate                  = sel("setDelegate:")
	selSetTarget, selSetAction      = sel("setTarget:"), sel("setAction:")
	selApply, selSubmit             = sel("nsgoApply:"), sel("nsgoSubmit:")
	selPerformOnMain                = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selWindow, selMakeFirstResponer = sel("window"), sel("makeFirstResponder:")
	selDocumentView                 = sel("documentView")
	selSetAccessibilityElement      = sel("setAccessibilityElement:")
	selSetAccessibilityRole         = sel("setAccessibilityRole:")
	selSetAccessibilitySubrole      = sel("setAccessibilitySubrole:")
	selSetAccessibilityLabel        = sel("setAccessibilityLabel:")
	selSetAccessibilityEnabled      = sel("setAccessibilityEnabled:")
	selSetAccessibilitySelected     = sel("setAccessibilitySelected:")
	selSetAccessibilityValue        = sel("setAccessibilityValue:")
	selSetAccessibilityIdentifier   = sel("setAccessibilityIdentifier:")
	selIntrinsicContentSize         = sel("intrinsicContentSize")
	selCurrentContext, selCGContext = sel("currentContext"), sel("CGContext")
	selIsEqualToString              = sel("isEqualToString:")

	classView, classSurface, classInput, classField, classSecureField objc.Class
	classObserver, classMenuTarget                                    objc.Class
)

func loadAppKit() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	nsView := objc.GetClass("NSView")
	common := []objc.MethodDef{
		{Cmd: sel("makeBackingLayer"), Fn: makeBackingLayer},
		{Cmd: sel("isFlipped"), Fn: func(self objc.ID, _ objc.SEL) bool { return true }},
		{Cmd: sel("drawRect:"), Fn: drawRect},
		{Cmd: sel("hitTest:"), Fn: hitTest},
		{Cmd: sel("mouseDown:"), Fn: mouseDown},
		{Cmd: sel("mouseUp:"), Fn: mouseUp},
		{Cmd: sel("mouseDragged:"), Fn: mouseDragged},
		{Cmd: sel("magnifyWithEvent:"), Fn: magnify},
		{Cmd: sel("rotateWithEvent:"), Fn: rotate},
		{Cmd: sel("rightMouseDown:"), Fn: rightMouseDown},
		{Cmd: sel("mouseEntered:"), Fn: mouseEntered},
		{Cmd: sel("mouseExited:"), Fn: mouseExited},
		{Cmd: sel("accessibilityPerformPress"), Fn: accessibilityPress},
	}
	classView = register("NsgoView", nsView, nil, common)
	refresh := func(self objc.ID, cmd objc.SEL) {
		apple.Super(self, cmd)
		if v := views[self]; v != nil {
			v.surface.refresh()
		}
	}
	classSurface = register("NsgoSurface", nsView, nil, append(common,
		objc.MethodDef{Cmd: sel("setFrameSize:"), Fn: surfaceSetFrameSize},
		objc.MethodDef{Cmd: selApply, Fn: surfaceApply},
		objc.MethodDef{Cmd: sel("viewDidChangeEffectiveAppearance"), Fn: refresh},
		objc.MethodDef{Cmd: sel("viewDidChangeBackingProperties"), Fn: refresh},
		objc.MethodDef{Cmd: sel("acceptsFirstResponder"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		objc.MethodDef{Cmd: sel("resignFirstResponder"), Fn: surfaceResign},
		// Keys reach the surface only while it holds a selection; it
		// takes them, as a text view would, rather than beep.
		objc.MethodDef{Cmd: sel("keyDown:"), Fn: func(objc.ID, objc.SEL, objc.ID) {}},
		objc.MethodDef{Cmd: sel("copy:"), Fn: surfaceCopy},
		objc.MethodDef{Cmd: sel("selectAll:"), Fn: surfaceSelectAll},
	))
	classObserver = register("NsgoObserver", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("observeValueForKeyPath:ofObject:change:context:"),
		Fn: func(self objc.ID, _ objc.SEL, path, object, change objc.ID, ctx uintptr) {
			apple.OnMain(func() { active.refresh() })
		},
	}})
	classInput = register("NsgoInput", nsView, nil, append(common,
		objc.MethodDef{Cmd: sel("layout"), Fn: inputLayout},
	))
	fieldMethods := []objc.MethodDef{
		{Cmd: sel("controlTextDidChange:"), Fn: fieldChanged},
		{Cmd: sel("controlTextDidEndEditing:"), Fn: fieldEnded},
		{Cmd: sel("becomeFirstResponder"), Fn: fieldFocused},
		{Cmd: selSubmit, Fn: fieldSubmit},
	}
	delegate := []*objc.Protocol{objc.GetProtocol("NSTextFieldDelegate")}
	classField = register("NsgoField", objc.GetClass("NSTextField"), delegate, fieldMethods)
	classSecureField = register("NsgoSecureField", objc.GetClass("NSSecureTextField"), delegate, fieldMethods)
	classMenuTarget = register("NsgoMenuTarget", objc.GetClass("NSObject"), nil, []objc.MethodDef{
		{Cmd: selMenuChosen, Fn: menuChosen},
	})
}

var (
	objectSetClass func(obj objc.ID, class objc.Class) objc.Class
	layerBase      objc.Class // the class of AppKit's backing layers
	classLayer     objc.Class // layerBase, holding transforms
)

// makeBackingLayer gives a view AppKit's own backing layer, made a
// subclass that keeps the transform a style gives it.
func makeBackingLayer(self objc.ID, cmd objc.SEL) objc.ID {
	layer := apple.SendSuper[objc.ID](self, cmd)
	if classLayer == 0 {
		libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&objectSetClass, libobjc, "object_setClass")
		layerBase = layer.Class()
		classLayer = register("NsgoLayer", layerBase, nil, apple.HeldTransformMethods())
	}
	if layer.Class() == layerBase {
		objectSetClass(layer, classLayer)
	}
	return layer
}

func register(name string, super objc.Class, protocols []*objc.Protocol, methods []objc.MethodDef) objc.Class {
	return apple.RegisterClass(name, super, protocols, methods)
}

// Run opens a window showing app and runs the event loop. Call it from
// main; it does not return.
func Run(app engine.App, opts Options) {
	if opts.Width == 0 {
		opts.Width, opts.Height = 480, 640
	}
	loadAppKit()
	nsApp := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	nsApp.Send(sel("setActivationPolicy:"), 0)
	defaultMenu := mainMenu()
	nsApp.Send(sel("setMainMenu:"), defaultMenu)
	delegate := objc.ID(register("NsgoAppDelegate", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("applicationShouldTerminateAfterLastWindowClosed:"),
		Fn:  func(self objc.ID, _ objc.SEL, app objc.ID) bool { return true },
	}, {
		Cmd: sel("applicationWillTerminate:"),
		Fn:  func(self objc.ID, _ objc.SEL, note objc.ID) { engine.SyncStorage() },
	}})).Send(selNew)
	nsApp.Send(sel("setDelegate:"), delegate)

	const titled, closable, miniaturizable, resizable = 1, 2, 4, 8
	window := objc.ID(objc.GetClass("NSWindow")).Send(selAlloc).Send(
		sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(0, 0, float64(opts.Width), float64(opts.Height)),
		uint(titled|closable|miniaturizable|resizable), uint(2), false)
	window.Send(sel("setTitle:"), apple.NSString(opts.Title))
	window.Send(sel("setReleasedWhenClosed:"), false)

	s := &surface{window: window}
	s.defaultMenu, s.appName = defaultMenu, opts.Title
	active = s
	s.native = objc.ID(classSurface).Send(selAlloc).Send(sel("initWithFrame:"),
		apple.Rect(0, 0, float64(opts.Width), float64(opts.Height)))
	s.native.Send(selSetWantsLayer, true)
	views[s.native] = &view{surface: s}
	window.Send(sel("setContentView:"), s.native)
	s.env = s.environment()
	eopts := engine.Options{Size: core.Size{Width: opts.Width, Height: opts.Height}, Environment: s.env}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	s.engine = engine.Start(s, app, eopts)
	s.observe()
	window.Send(sel("center"))
	window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	nsApp.Send(sel("activateIgnoringOtherApps:"), true)
	nsApp.Send(sel("run"))
}

func mainMenu() objc.ID {
	menu := func(title string) objc.ID {
		return objc.ID(objc.GetClass("NSMenu")).Send(selAlloc).Send(sel("initWithTitle:"), apple.NSString(title))
	}
	item := func(m objc.ID, title, action, key string) {
		m.Send(sel("addItemWithTitle:action:keyEquivalent:"), apple.NSString(title), sel(action), apple.NSString(key))
	}
	submenu := func(bar, m objc.ID) {
		it := objc.ID(objc.GetClass("NSMenuItem")).Send(selNew)
		it.Send(sel("setSubmenu:"), m)
		bar.Send(sel("addItem:"), it)
	}
	bar := menu("")
	app := menu("")
	item(app, "Quit", "terminate:", "q")
	submenu(bar, app)
	edit := menu("Edit")
	item(edit, "Undo", "undo:", "z")
	item(edit, "Redo", "redo:", "Z")
	item(edit, "Cut", "cut:", "x")
	item(edit, "Copy", "copy:", "c")
	item(edit, "Paste", "paste:", "v")
	item(edit, "Select All", "selectAll:", "a")
	submenu(bar, edit)
	return bar
}

// environment returns what AppKit tells about where the app runs.
func (s *surface) environment() core.Environment {
	ws := objc.ID(objc.GetClass("NSWorkspace")).Send(sel("sharedWorkspace"))
	locale, rtl := apple.Locale()
	appearance := apple.GoString(s.native.Send(sel("effectiveAppearance")).Send(sel("name")))
	return core.Environment{
		Dark:         strings.Contains(appearance, "Dark"),
		FontScale:    1,
		ReduceMotion: objc.Send[bool](ws, sel("accessibilityDisplayShouldReduceMotion")),
		ScreenReader: objc.Send[bool](ws, sel("isVoiceOverEnabled")),
		RTL:          rtl,
		PixelRatio:   float32(objc.Send[float64](s.window, sel("backingScaleFactor"))),
		Locale:       locale,
		OS:           core.OSMacOS,
		Idiom:        core.IdiomDesktop,
		TitleBar:     s.titleBarArea(),
	}
}

// refresh reports the environment to the engine if it changed.
func (s *surface) refresh() {
	if s == nil || s.engine == nil {
		return
	}
	if env := s.environment(); env != s.env {
		s.env = env
		s.engine.SetEnvironment(env)
	}
}

// observe watches what the environment and the back and press-outside
// events come from: accessibility settings, the locale, and the app's
// key and mouse events.
func (s *surface) observe() {
	const appKit = "/System/Library/Frameworks/AppKit.framework/AppKit"
	const foundation = "/System/Library/Frameworks/Foundation.framework/Foundation"
	ws := objc.ID(objc.GetClass("NSWorkspace")).Send(sel("sharedWorkspace"))
	refresh := func(objc.ID) { s.refresh() }
	apple.Observe(ws.Send(sel("notificationCenter")),
		apple.Global(appKit, "NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification"), refresh)
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(sel("defaultCenter"))
	apple.Observe(center, apple.Global(foundation, "NSCurrentLocaleDidChangeNotification"), refresh)
	apple.Observe(center, apple.Global(appKit, "NSViewBoundsDidChangeNotification"), clipScrolled)
	for _, name := range []string{"NSWindowDidResizeNotification", "NSWindowDidEnterFullScreenNotification",
		"NSWindowDidExitFullScreenNotification"} {
		apple.Observe(center, apple.Global(appKit, name), func(note objc.ID) {
			if note.Send(sel("object")) == s.window {
				s.refresh()
			}
		})
	}
	// Full screen keeps a toolbar over the app: the hidden title bar's
	// hides with the rest of the bar.
	for name, visible := range map[string]bool{"NSWindowWillEnterFullScreenNotification": false,
		"NSWindowDidExitFullScreenNotification": true} {
		apple.Observe(center, apple.Global(appKit, name), func(note objc.ID) {
			if note.Send(sel("object")) == s.window && s.toolbar != 0 {
				s.toolbar.Send(sel("setVisible:"), visible)
				s.refresh()
			}
		})
	}
	observer := objc.ID(classObserver).Send(selNew)
	ws.Send(sel("addObserver:forKeyPath:options:context:"), observer, apple.NSString("voiceOverEnabled"), uint(0), uintptr(0))

	const keyDown, leftDown, leftUp, leftDragged, rightDown, otherDown = 1 << 10, 1 << 1, 1 << 2, 1 << 6, 1 << 3, 1 << 25
	handler := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID { return s.filter(event) })
	objc.ID(objc.GetClass("NSEvent")).Send(sel("addLocalMonitorForEventsMatchingMask:handler:"),
		uint64(keyDown|leftDown|leftUp|leftDragged|rightDown|otherDown), handler)
	handler.Release()
}

// filter sees the app's key and mouse events before their views do. It
// turns back keys and the back button into EventBack, which it takes,
// reports presses outside to the views that listen and runs selections.
func (s *surface) filter(event objc.ID) objc.ID {
	if event.Send(selWindow) != s.window {
		return event
	}
	const typeLeftDown, typeLeftUp, typeRightDown, typeLeftDragged, typeKeyDown, typeOtherDown = 1, 2, 3, 6, 10, 25
	switch objc.Send[uint](event, sel("type")) {
	case typeKeyDown:
		if binding := s.binding(event); binding != "" {
			s.engine.Dispatch(core.Event{Kind: core.EventKey, Text: binding})
			return 0
		}
		if s.selectKey(event) {
			return 0
		}
		if s.isBack(event) {
			s.engine.Dispatch(core.Event{Kind: core.EventBack})
			return 0
		}
	case typeLeftDown:
		s.pressOutside(event)
		if s.dragWindow(event) || s.selectPress(event) {
			return 0
		}
	case typeLeftDragged:
		if s.selectDrag(event) {
			return 0
		}
	case typeLeftUp:
		if s.selectUp() {
			return 0
		}
	case typeRightDown:
		s.pressOutside(event)
	case typeOtherDown:
		const backButton = 3
		if objc.Send[int](event, sel("buttonNumber")) == backButton {
			s.engine.Dispatch(core.Event{Kind: core.EventBack})
			return 0
		}
	}
	return event
}

// AppKit key codes of the keys core.NormalizeKey names.
var keyNames = map[uint16]string{
	36: "enter", 76: "enter", 53: "escape", 48: "tab", 49: "space", 51: "backspace", 117: "delete",
	126: "up", 125: "down", 123: "left", 124: "right", 115: "home", 119: "end", 116: "pageup", 121: "pagedown",
	122: "f1", 120: "f2", 99: "f3", 118: "f4", 96: "f5", 97: "f6", 98: "f7", 100: "f8", 101: "f9",
	109: "f10", 103: "f11", 111: "f12",
}

// binding returns the keymap's binding a key event presses, or "". It
// names the press both with "mod", Command here, and with "meta".
func (s *surface) binding(event objc.ID) string {
	if len(s.keys) == 0 {
		return ""
	}
	name, ok := keyNames[objc.Send[uint16](event, sel("keyCode"))]
	if !ok {
		// The key's character without Shift or Option, in the current layout.
		name = strings.ToLower(apple.GoString(event.Send(sel("charactersByApplyingModifiers:"), uint(0))))
		if name == "" {
			return ""
		}
	}
	for _, k := range apple.KeyNames(name, objc.Send[uint](event, sel("modifierFlags"))) {
		if s.keys[k] {
			return k
		}
	}
	return ""
}

// isBack reports whether a key event is Escape, Cmd-[ or Option-Left.
// Escape and Option-Left belong to a text field being edited.
func (s *surface) isBack(event objc.ID) bool {
	const shift, control, option, command = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	const escape, leftArrow = 53, 123
	mods := objc.Send[uint](event, sel("modifierFlags")) & (shift | control | option | command)
	editing := objc.Send[bool](s.window.Send(sel("firstResponder")), sel("isKindOfClass:"), objc.GetClass("NSText"))
	switch code := objc.Send[uint16](event, sel("keyCode")); {
	case code == escape && mods == 0, code == leftArrow && mods == option:
		return !editing
	case mods == command:
		return apple.GoString(event.Send(sel("charactersIgnoringModifiers"))) == "["
	}
	return false
}

// pressOutside reports a mouse down to each view listening for presses
// outside it that neither holds the view hit nor the point.
func (s *surface) pressOutside(event objc.ID) {
	loc := objc.Send[apple.CGPoint](event, selLocationInWindow)
	var hit objc.ID
	at := core.Point{}
	for _, v := range s.views {
		if v == nil || !v.events.Has(core.EventPressOutside) || v.native.Send(selWindow) == 0 {
			continue
		}
		if hit == 0 {
			p := objc.Send[apple.CGPoint](s.native, selConvertPoint, loc, objc.ID(0))
			at = core.Point{X: core.Points(p.X), Y: core.Points(p.Y)}
			up := objc.Send[apple.CGPoint](s.native.Send(selSuperview), selConvertPoint, loc, objc.ID(0))
			if hit = s.native.Send(sel("hitTest:"), up); hit == 0 {
				hit = s.native
			}
		}
		p := objc.Send[apple.CGPoint](v.native, selConvertPoint, loc, objc.ID(0))
		b := objc.Send[apple.CGRect](v.native, selBounds)
		inside := p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height
		if inside || objc.Send[bool](hit, sel("isDescendantOf:"), v.native) {
			continue
		}
		s.engine.Dispatch(core.Event{Kind: core.EventPressOutside, ID: v.id, Point: at})
	}
}

// alert shows a as a sheet on the window. The Cancel action answers
// Escape; the sheet's answer goes to the engine as EventAlert.
func (s *surface) alert(a *core.AlertConfig) {
	alert := objc.ID(objc.GetClass("NSAlert")).Send(selNew)
	alert.Send(sel("setMessageText:"), apple.NSString(a.Title))
	alert.Send(sel("setInformativeText:"), apple.NSString(a.Message))
	for _, act := range a.Actions {
		button := alert.Send(sel("addButtonWithTitle:"), apple.NSString(act.Label))
		switch act.Role {
		case core.AlertCancel:
			button.Send(sel("setKeyEquivalent:"), apple.NSString("\x1b"))
		case core.AlertDestructive:
			button.Send(sel("setHasDestructiveAction:"), true)
		}
	}
	id, n := a.ID, len(a.Actions)
	done := objc.NewBlock(func(_ objc.Block, response int) {
		const firstButton = 1000
		index := response - firstButton
		if index < 0 || index >= n {
			index = -1
		}
		s.engine.Dispatch(core.Event{Kind: core.EventAlert, Count: id, Index: index})
		alert.Send(selRelease)
	})
	alert.Send(sel("beginSheetModalForWindow:completionHandler:"), s.window, done)
	done.Release()
}

// pasteboardText is NSPasteboardTypeString.
const pasteboardText = "public.utf8-plain-text"

// announce has VoiceOver say text.
func announce(text string) {
	const appKit = "/System/Library/Frameworks/AppKit.framework/AppKit"
	lib, err := purego.Dlopen(appKit, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	var post func(element, name, info objc.ID)
	purego.RegisterLibFunc(&post, lib, "NSAccessibilityPostNotificationWithUserInfo")
	const high = 90 // NSAccessibilityPriorityHigh
	keys := []objc.ID{apple.Global(appKit, "NSAccessibilityAnnouncementKey"), apple.Global(appKit, "NSAccessibilityPriorityKey")}
	values := []objc.ID{apple.NSString(text), objc.ID(objc.GetClass("NSNumber")).Send(sel("numberWithInt:"), high)}
	info := objc.ID(objc.GetClass("NSDictionary")).Send(sel("dictionaryWithObjects:forKeys:count:"),
		unsafe.Pointer(&values[0]), unsafe.Pointer(&keys[0]), uint(len(keys)))
	app := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	post(app, apple.Global(appKit, "NSAccessibilityAnnouncementRequestedNotification"), info)
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
}

// scrollTo scrolls a scroll view's content to p, clamped to what it can
// show; the clip view reports the offset as it moves.
func (v *view) scrollTo(p core.Point, animated bool) {
	if v.clip == 0 {
		return
	}
	doc := objc.Send[apple.CGRect](v.content, selBounds).Size
	seen := objc.Send[apple.CGRect](v.clip, selBounds).Size
	to := apple.CGPoint{X: min(max(float64(p.X), 0), max(doc.Width-seen.Width, 0)),
		Y: min(max(float64(p.Y), 0), max(doc.Height-seen.Height, 0))}
	clip := v.clip
	if animated {
		clip = clip.Send(sel("animator"))
	}
	clip.Send(sel("setBoundsOrigin:"), to)
	v.native.Send(sel("reflectScrolledClipView:"), v.clip)
}

// Prepare implements core.TextEngine with CoreText.
func (s *surface) Prepare(t *core.AttributedText) core.PreparedText {
	return apple.TextEngine{}.Prepare(t)
}

// Mount implements engine.Platform.
func (s *surface) Mount(b *core.MountBatch) {
	if s.batches.Push(b) {
		s.native.Send(selPerformOnMain, selApply, objc.ID(0), false)
	}
}

func (s *surface) apply(b *core.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			for len(s.views) <= int(it.ID) {
				s.views = append(s.views, nil)
			}
			s.views[it.ID] = s.create(it.View, it.ID)
			continue
		case core.ItemAlert:
			s.alert(it.Alert)
			continue
		case core.ItemDismissKeyboard:
			s.window.Send(selMakeFirstResponer, objc.ID(0))
			continue
		case core.ItemSetClipboard:
			pb := objc.ID(objc.GetClass("NSPasteboard")).Send(sel("generalPasteboard"))
			pb.Send(sel("clearContents"))
			pb.Send(sel("setString:forType:"), apple.NSString(it.Value), apple.NSString(pasteboardText))
			continue
		case core.ItemReadClipboard:
			pb := objc.ID(objc.GetClass("NSPasteboard")).Send(sel("generalPasteboard"))
			text := apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardText)))
			s.engine.Dispatch(core.Event{Kind: core.EventClipboard, Count: it.Count, Text: text})
			continue
		case core.ItemAnnounce:
			announce(it.Value)
			continue
		case core.ItemKeymap:
			s.keys = map[string]bool{}
			for _, k := range it.Keys {
				s.keys[k] = true
			}
			continue
		case core.ItemTitle:
			s.window.Send(sel("setTitle:"), apple.NSString(it.Value))
			continue
		case core.ItemWindow:
			s.command(it.Window)
			continue
		case core.ItemMenu:
			s.setMenu(it.Menu)
			continue
		case core.ItemStatusBar, core.ItemBackUnhandled:
			continue
		}
		v := s.views[it.ID]
		switch it.Kind {
		case core.ItemDelete:
			s.delete(v)
		case core.ItemRemove:
			v.native.Send(selRemoveFromSuperview)
		case core.ItemInsert:
			parent := s.views[it.Parent].content
			if it.Before == 0 {
				parent.Send(selAddSubview, v.native)
			} else {
				const below = -1
				parent.Send(selAddSubviewPositioned, v.native, below, s.views[it.Before].native)
			}
			if v.kind == core.KindModal && !v.presented {
				v.presented = true
				s.presenting = append(s.presenting, v)
			}
		case core.ItemFrame:
			f := it.Frame
			v.native.Send(selSetFrame, apple.Rect(float64(f.Origin.X), float64(f.Origin.Y),
				float64(f.Size.Width), float64(f.Size.Height)))
			if v.transform != (core.Transform{}) {
				apple.SetTransform(v.native.Send(selLayer), v.transform)
			}
			apple.LayoutBorders(v.native.Send(selLayer))
			if v.drawing != nil {
				v.native.Send(selSetNeedsDisplay, true)
			}
			if v.image != nil {
				v.image.Layout(apple.CGSize{Width: float64(f.Size.Width), Height: float64(f.Size.Height)})
				s.images = append(s.images, v)
			}
		case core.ItemViewStyle:
			apple.StyleLayer(v.native.Send(selLayer), it.ViewStyle, true)
			v.native.Send(selSetHidden, it.ViewStyle.Hidden)
			v.transform = it.ViewStyle.Transform
			if v.image != nil {
				v.image.SetStyle(it.ViewStyle)
			}
		case core.ItemImage:
			v.image.Configure(it.Image)
			s.images = append(s.images, v)
		case core.ItemModal:
			v.animation = it.Modal.Animation
		case core.ItemCanvas:
			v.drawing = it.Drawing
			v.native.Send(selSetNeedsDisplay, true)
		case core.ItemScrollTo:
			v.scrollTo(it.Point, it.Animated)
		case core.ItemViewProps:
			setAccessibility(v.native, it.ViewProps)
			p := it.ViewProps
			v.windowDrag, v.activation, v.area, v.selectable = p.WindowDrag, p.PanActivation, p.SelectionArea, p.Selectable
			if v.selColor != p.SelectionColor {
				v.selColor = p.SelectionColor
				v.native.Send(selSetNeedsDisplay, true)
			}
		case core.ItemEvents:
			v.setEvents(it.Events)
		case core.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.box, v.plain = it.Text.Content, it.Text.Text
			v.native.Send(selSetAccessibilityLabel, apple.NSString(it.Text.Text))
			v.native.Send(selSetNeedsDisplay, true)
		case core.ItemInput:
			v.configure(it.Input)
		case core.ItemInputValue:
			if v.edits <= it.Count {
				value := apple.NSString(it.Value)
				// Setting the text the field shows would move the caret.
				if !objc.Send[bool](v.field.Send(selStringValue), selIsEqualToString, value) {
					v.field.Send(selSetString, value)
				}
			}
		case core.ItemScrollConfig:
			v.native.Send(sel("setHasVerticalScroller:"), !it.Scroll.Horizontal && !it.Scroll.HideIndicator)
			v.native.Send(sel("setHasHorizontalScroller:"), it.Scroll.Horizontal && !it.Scroll.HideIndicator)
		case core.ItemScrollContent:
			v.content.Send(selSetFrameSize, apple.CGSize{Width: float64(it.Size.Width), Height: float64(it.Size.Height)})
		case core.ItemFocus:
			if w := v.native.Send(selWindow); w != 0 && v.field != 0 {
				w.Send(selMakeFirstResponer, v.field)
			}
		case core.ItemBlur:
			if w := v.native.Send(selWindow); w != 0 {
				w.Send(selMakeFirstResponer, objc.ID(0))
			}
		}
	}
	if root := s.views[b.Root]; root.native.Send(selSuperview) != s.native {
		s.native.Send(selAddSubview, root.native)
	}
	s.finish()
	s.refreshSelection()
}

// finish starts what a batch asked for once its frames are set: image
// loads at the views' sizes and modals' entrances.
func (s *surface) finish() {
	for _, v := range s.images {
		if s.views[v.id] == v {
			v.image.Start(float64(s.env.PixelRatio))
		}
	}
	for _, v := range s.presenting {
		if s.views[v.id] == v && v.native.Send(selSuperview) != 0 {
			h := objc.Send[apple.CGRect](v.native.Send(selSuperview), selBounds).Size.Height
			apple.AnimateIn(v.native.Send(selLayer), v.animation, h, s.env.ReduceMotion)
		}
	}
	clear(s.images)
	clear(s.presenting)
	s.images, s.presenting = s.images[:0], s.presenting[:0]
}

func (s *surface) create(kind core.ViewKind, id core.NodeID) *view {
	v := &view{surface: s, id: id, kind: kind}
	switch kind {
	case core.KindScroll:
		v.native = objc.ID(objc.GetClass("NSScrollView")).Send(selNew)
		v.native.Send(sel("setDrawsBackground:"), false)
		v.native.Send(sel("setAutohidesScrollers:"), true)
		// The clip view posts its scrolling, which observe follows; an
		// override would recurse once animator's KVO subclasses it.
		clip := objc.ID(objc.GetClass("NSClipView")).Send(selNew)
		clip.Send(sel("setPostsBoundsChangedNotifications:"), true)
		clip.Send(sel("setDrawsBackground:"), false)
		v.native.Send(sel("setContentView:"), clip)
		clip.Send(selRelease)
		v.content = objc.ID(classView).Send(selNew)
		v.content.Send(selSetWantsLayer, true)
		v.native.Send(sel("setDocumentView:"), v.content)
		v.content.Send(selRelease)
		v.clip = clip
		views[clip] = v
	case core.KindInput:
		v.native = objc.ID(classInput).Send(selNew)
		v.setField(false)
	default:
		v.native = objc.ID(classView).Send(selNew)
	}
	v.native.Send(selSetWantsLayer, true)
	if v.content == 0 {
		v.content = v.native
	}
	if kind == core.KindImage {
		v.image = apple.NewImageView(v.native.Send(selLayer))
		v.image.Dispatch = v.report
	}
	views[v.native] = v
	return v
}

func (s *surface) delete(v *view) {
	v.native.Send(selRemoveFromSuperview)
	apple.ForgetTransform(v.native.Send(selLayer))
	apple.ForgetBorders(v.native.Send(selLayer))
	v.layout.Release()
	if v.image != nil {
		v.image.Close()
	}
	if v.hover != 0 {
		v.hover.Send(selRelease)
	}
	delete(views, v.native)
	delete(views, v.field)
	delete(views, v.clip)
	v.native.Send(selRelease)
	s.views[v.id] = nil
}

// setField replaces an input's text field, keeping its text.
func (v *view) setField(masked bool) {
	text := apple.NSString("")
	if v.field != 0 {
		text = v.field.Send(selStringValue)
		v.field.Send(selRemoveFromSuperview)
		delete(views, v.field)
	}
	class := classField
	if masked {
		class = classSecureField
	}
	field := objc.ID(class).Send(selNew)
	field.Send(sel("setBezeled:"), false)
	field.Send(sel("setBordered:"), false)
	field.Send(sel("setDrawsBackground:"), false)
	field.Send(sel("setFocusRingType:"), 1)
	field.Send(selSetDelegate, field)
	field.Send(selSetTarget, field)
	field.Send(selSetAction, selSubmit)
	field.Send(selSetString, text)
	cell := field.Send(sel("cell"))
	cell.Send(sel("setScrollable:"), true)
	cell.Send(sel("setUsesSingleLineMode:"), true)
	v.native.Send(selAddSubview, field)
	field.Send(selRelease)
	v.field, v.masked = field, masked
	views[field] = v
}

// AppKit's text alignments.
const nsAlignLeft, nsAlignRight, nsAlignCenter = 0, 1, 2

func (v *view) configure(c *core.InputConfig) {
	if c.Masked != v.masked {
		v.setField(c.Masked)
	}
	font := apple.Font(c.Style.Font)
	v.field.Send(selSetFont, objc.ID(font))
	apple.Release(font)
	v.field.Send(selSetTextColor, nsColor(c.Style.Color))
	align := nsAlignLeft
	switch c.Style.Align {
	case core.TextAlignCenter:
		align = nsAlignCenter
	case core.TextAlignEnd:
		align = nsAlignRight
	}
	v.field.Send(selSetAlignment, align)
	v.field.Send(selSetPlaceholder, apple.NSString(c.Placeholder))
	v.field.Send(selSetEditable, !c.Readonly)
	v.field.Send(selSetEnabled, !c.Disabled)
	v.inset = c.Content
	v.native.Send(selSetNeedsLayout, true)
}

func (v *view) setEvents(m core.EventMask) {
	v.events = m
	wantHover := m.Has(core.EventHoverIn) || m.Has(core.EventHoverOut)
	switch {
	case wantHover && v.hover == 0:
		const enteredAndExited, activeInActiveApp, inVisibleRect = 0x01, 0x40, 0x200
		v.hover = objc.ID(objc.GetClass("NSTrackingArea")).Send(selAlloc).Send(
			sel("initWithRect:options:owner:userInfo:"), apple.Rect(0, 0, 0, 0),
			uint(enteredAndExited|activeInActiveApp|inVisibleRect), v.native, objc.ID(0))
		v.native.Send(sel("addTrackingArea:"), v.hover)
	case !wantHover && v.hover != 0:
		v.native.Send(sel("removeTrackingArea:"), v.hover)
		v.hover.Send(selRelease)
		v.hover = 0
	}
}

func (v *view) dispatch(kind core.EventKind) {
	v.report(core.Event{Kind: kind})
}

// report sends e about v if v listens for its kind.
func (v *view) report(e core.Event) {
	if v.events.Has(e.Kind) {
		e.ID = v.id
		v.surface.engine.Dispatch(e)
	}
}

// AppKit accessibility roles of core roles: role and subrole.
var roles = map[core.Role][2]string{
	core.RoleButton:     {"AXButton", ""},
	core.RoleLink:       {"AXLink", ""},
	core.RoleHeader:     {"AXStaticText", ""},
	core.RoleImage:      {"AXImage", ""},
	core.RoleCheckbox:   {"AXCheckBox", ""},
	core.RoleSwitch:     {"AXCheckBox", "AXSwitch"},
	core.RoleRadio:      {"AXRadioButton", ""},
	core.RoleTab:        {"AXRadioButton", "AXTabButton"},
	core.RoleText:       {"AXStaticText", ""},
	core.RoleRadioGroup: {"AXRadioGroup", ""},
	core.RoleTabList:    {"AXTabGroup", ""},
	core.RoleMenu:       {"AXMenu", ""},
	core.RoleMenuItem:   {"AXMenuItem", ""},
	// WebKit's subroles for ARIA's dialog and alert.
	core.RoleDialog: {"AXGroup", "AXApplicationDialog"},
	core.RoleAlert:  {"AXGroup", "AXApplicationAlert"},
}

func setAccessibility(native objc.ID, p *core.ViewProps) {
	if p.TestID != "" {
		native.Send(selSetAccessibilityIdentifier, apple.NSString(p.TestID))
	}
	r, ok := roles[p.Role]
	native.Send(selSetAccessibilityElement, ok)
	if !ok {
		return
	}
	native.Send(selSetAccessibilityRole, apple.NSString(r[0]))
	if r[1] != "" {
		native.Send(selSetAccessibilitySubrole, apple.NSString(r[1]))
	}
	if p.Label != "" {
		native.Send(selSetAccessibilityLabel, apple.NSString(p.Label))
	}
	native.Send(selSetAccessibilityEnabled, !p.Disabled)
	native.Send(selSetAccessibilitySelected, p.Selected)
	if p.Role == core.RoleCheckbox || p.Role == core.RoleSwitch || p.Role == core.RoleRadio {
		n := 0
		switch {
		case p.Indeterminate:
			n = 2 // mixed
		case p.Checked || p.Selected:
			n = 1
		}
		native.Send(selSetAccessibilityValue, objc.ID(objc.GetClass("NSNumber")).Send(sel("numberWithInt:"), n))
	}
}

func nsColor(c core.Color) objc.ID {
	r, g, b, a := c.Components()
	return objc.ID(objc.GetClass("NSColor")).Send(sel("colorWithSRGBRed:green:blue:alpha:"), r, g, b, a)
}

func drawRect(self objc.ID, _ objc.SEL, dirty apple.CGRect) {
	v := views[self]
	if v == nil || (v.layout == nil && v.drawing == nil) {
		return
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	ctx := objc.Send[uintptr](objc.ID(objc.GetClass("NSGraphicsContext")).Send(selCurrentContext), selCGContext)
	if v.drawing != nil {
		apple.Play(ctx, v.drawing)
		return
	}
	box := v.box
	v.layout.DrawBackgrounds(ctx, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
	v.drawSelection(ctx)
	v.layout.Draw(ctx, b.Size.Height, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
		float64(box.Size.Width), float64(box.Size.Height)))
}

// hitTest lets presses on text and plain containers through to the
// nearest pressable ancestor; text fields keep theirs.
func hitTest(self objc.ID, cmd objc.SEL, p apple.CGPoint) objc.ID {
	hit := apple.SendSuper[objc.ID](self, cmd, p)
	if rawHit {
		return hit
	}
	stop := self.Send(selSuperview)
	for n := hit; n != 0 && n != stop; n = n.Send(selSuperview) {
		if v := views[n]; v != nil {
			if v.field == n || v.kind == core.KindScroll {
				return hit
			}
			if (v.pressable() || v.events.Has(core.EventPan)) && v.native == n {
				return n
			}
		}
	}
	return hit
}

func (v *view) pressable() bool {
	return v.events.Has(core.EventPress) || v.events.Has(core.EventLongPress) ||
		v.events.Has(core.EventPressDown) || v.events.Has(core.EventPressUp)
}

func mouseDown(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	// Control-click is a secondary click.
	const control = 1 << 18
	if v != nil && v.events.Has(core.EventContextMenu) && objc.Send[uint](event, sel("modifierFlags"))&control != 0 {
		rightMouseDown(self, cmd, event)
		return
	}
	if v != nil {
		v.surface.startPan(self, event)
	}
	if v == nil || !v.pressable() {
		if v == nil || v.surface.pan == nil {
			apple.Super(self, cmd, event)
		}
		return
	}
	v.pressing.Store(true)
	v.longFired.Store(false)
	v.dispatch(core.EventPressDown)
	if v.events.Has(core.EventLongPress) {
		e := v.surface.engine
		time.AfterFunc(longPressDelay, func() {
			if v.pressing.Load() && v.longFired.CompareAndSwap(false, true) {
				e.Dispatch(core.Event{Kind: core.EventLongPress, ID: v.id})
			}
		})
	}
}

func mouseUp(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	panned := v != nil && v.surface.endPan(event)
	if v == nil || !v.pressable() {
		if !panned {
			apple.Super(self, cmd, event)
		}
		return
	}
	p := objc.Send[apple.CGPoint](self, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	b := objc.Send[apple.CGRect](self, selBounds)
	pressed := v.pressing.Swap(false)
	if !pressed {
		return // a pan took the press
	}
	v.dispatch(core.EventPressUp)
	if pressed && !v.longFired.Load() && p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height {
		v.report(core.Event{Kind: core.EventPress, Index: v.indexAt(p)})
	}
}

// indexAt returns the byte offset of the character at p in a text view,
// for presses on its links.
func (v *view) indexAt(p apple.CGPoint) int {
	if v.layout == nil {
		return -1
	}
	box := v.box
	return v.layout.IndexAt(p, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
		float64(box.Size.Width), float64(box.Size.Height)))
}

// rightMouseDown reports a secondary click to a view listening for
// context menus, and passes it on otherwise.
func rightMouseDown(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	if v == nil || !v.events.Has(core.EventContextMenu) {
		apple.Super(self, cmd, event)
		return
	}
	p := objc.Send[apple.CGPoint](self, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	v.report(core.Event{Kind: core.EventContextMenu, Point: core.Point{X: core.Points(p.X), Y: core.Points(p.Y)}})
}

func mouseEntered(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventHoverIn)
	}
}

func mouseExited(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventHoverOut)
	}
}

func accessibilityPress(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	if v == nil || !v.events.Has(core.EventPress) {
		return false
	}
	v.report(core.Event{Kind: core.EventPress, Index: -1})
	return true
}

func surfaceSetFrameSize(self objc.ID, cmd objc.SEL, size apple.CGSize) {
	apple.Super(self, cmd, size)
	if v := views[self]; v != nil && v.surface.engine != nil {
		v.surface.engine.Resize(core.Size{Width: core.Points(size.Width), Height: core.Points(size.Height)})
	}
}

func surfaceApply(self objc.ID, _ objc.SEL, _ objc.ID) {
	s := views[self].surface
	for _, b := range s.batches.Drain() {
		s.apply(b)
	}
}

func inputLayout(self objc.ID, cmd objc.SEL) {
	apple.Super(self, cmd)
	v := views[self]
	h := objc.Send[apple.CGSize](v.field, selIntrinsicContentSize).Height
	in := v.inset
	y := float64(in.Origin.Y) + (float64(in.Size.Height)-h)/2
	v.field.Send(selSetFrame, apple.Rect(float64(in.Origin.X), y, float64(in.Size.Width), h))
}

func fieldChanged(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.edits++
		v.surface.engine.Dispatch(core.Event{Kind: core.EventChangeText, ID: v.id,
			Text: apple.GoString(self.Send(selStringValue)), Count: v.edits})
	}
}

func fieldSubmit(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.surface.engine.Dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.id,
			Text: apple.GoString(self.Send(selStringValue))})
	}
}

func fieldFocused(self objc.ID, cmd objc.SEL) bool {
	ok := apple.SendSuper[bool](self, cmd)
	if v := views[self]; v != nil && ok {
		v.dispatch(core.EventFocus)
	}
	return ok
}

func fieldEnded(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventBlur)
	}
}

// clipScrolled reports a scroll view's offset when its clip view moves.
func clipScrolled(note objc.ID) {
	clip := note.Send(sel("object"))
	if v := views[clip]; v != nil && v.clip == clip && v.events.Has(core.EventScroll) {
		o := objc.Send[apple.CGRect](clip, selBounds).Origin
		v.surface.engine.Dispatch(core.Event{Kind: core.EventScroll, ID: v.id,
			Point: core.Point{X: core.Points(o.X), Y: core.Points(o.Y)}})
	}
}
