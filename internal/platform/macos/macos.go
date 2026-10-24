//go:build darwin && !ios

// Package macos is NSGo's AppKit backend: one flipped, layer-backed NSView
// per node, styled through its CALayer, with text the engine laid out
// with CoreText drawn on the main thread and images decoded off it, and
// an optional NSVisualEffectView material behind the window's content.
// Alerts are sheets on the window; Escape, Cmd-[, Option-Left and the
// mouse's back button go back. It calls AppKit through purego, so it
// builds without cgo.
package macos

import (
	"math"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/engine"
	"github.com/oneo-me/nsgo/internal/platform/apple"
	"github.com/oneo-me/nsgo/internal/platform/finger"
	"github.com/oneo-me/nsgo/internal/platform/queue"
)

func init() {
	// AppKit runs on the process's main thread, which main starts on.
	runtime.LockOSThread()
}

// Options configure the window.
type Options struct {
	Title         string
	Width, Height driver.Points
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
	// Components are the native components Native elements name.
	Components driver.NativeComponents
	// Material is the material behind the app's views, where they draw
	// nothing opaque (nsgo.WindowMaterial).
	Material driver.WindowMaterial
	// Perf shows the performance monitor over the app: frame rate, frame
	// and commit costs, CPU, GPU and memory.
	Perf bool
}

// longPressDelay is how long a press is held before it is a long press.
const longPressDelay = 500 * time.Millisecond

// view is the Go side of one native view.
type view struct {
	surface *surface
	id      driver.NodeID
	kind    driver.ViewKind
	native  objc.ID
	content objc.ID // where children go: native, or a scroll view's document
	events  driver.EventMask

	pressing  atomic.Bool
	longFired atomic.Bool
	pressAt   driver.Event  // where the press began, and how
	hover     objc.ID       // the tracking area of a view reporting hovers
	moves     bool          // whether it reports moves
	overflow  driver.Bounds // where its children reach outside it (ItemOverflow)
	// focusBefore is what had the keyboard focus as a modal came.
	focusBefore objc.ID

	layout *apple.Layout // a text view's
	box    driver.Bounds

	sc    *scrollState // a scroll view's
	input *input       // an input's editor
	clip  objc.ID      // a scroll view's NSClipView
	wheel wheelOwner
	inset driver.Bounds

	// contentLayer is what a text or canvas draws into (content.go);
	// stale says its content changed while it could not be seen.
	contentLayer objc.ID
	stale        bool
	// up is the view the batches inserted v in, nil while v is out of
	// the tree.
	up *view

	image     *apple.ImageView
	animation driver.ModalAnimation // a modal's
	presented bool                  // whether a modal was inserted yet
	transform driver.Transform      // AppKit drops it when the frame changes
	drawing   *driver.Drawing       // a canvas's
	shader    *apple.ShaderLayer    // a shader view's (shader.go)
	web       *apple.Web            // a web view's
	video     *apple.Video          // a video's

	// frame is where the view shows now; style its own style, which its
	// animated values and a layout animation draw over.
	frame    driver.Bounds
	framed   bool
	shift    driver.Point // a sticky cell's, from frame
	style    driver.ViewStyle
	animated driver.AnimatedValues

	component     driver.NativeComponent // a native view's
	componentView objc.ID
	disabled      bool

	// props are the view's last props; the rest what access.go last
	// gave AppKit.
	props                driver.ViewProps
	propsSet, textSet    bool
	axElement, axGrouped bool
	axCustom             bool

	windowDrag driver.WindowDrag
	activation driver.PanActivation

	// A text's content and selection, and whether a view is an area.
	plain            string
	selectable       driver.Selectable
	selColor         driver.Color
	selStart, selEnd int // UTF-16
	area             bool
}

type surface struct {
	native  objc.ID
	window  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []*view            // index is NodeID
	movers  map[*view]struct{} // views following mouse moves
	root    driver.NodeID      // the root view's, which the batches name
	// mouse is finger 0 of the views' touches; touched is the last
	// NSEvent reported, which AppKit passes up the views.
	mouse   finger.Mouse
	touched struct {
		event objc.ID
		kind  int
		time  float64
	}
	env driver.Environment
	// material is the material behind the app's views.
	material driver.WindowMaterial
	keys     []driver.KeyBinding // the keymap's bindings

	// What a batch touched, finished once its frames are set.
	images, presenting []*view
	modals             []*view // the modals shown, oldest first, for the keyboard focus
	fieldEditor        objc.ID // NSGo's field editor of the window, made as a field first edits
	// stale are the views whose content changed out of sight.
	stale []*view
	// shading are the shader views to draw once the batch or the frame
	// is applied.
	shading []*view

	// The frame clock and what it animates.
	animator *driver.Animator
	epoch    time.Time
	link     objc.ID // the frame clock's CADisplayLink, made once
	timer    objc.ID // or its NSTimer while ticking, before macOS 14
	ticking  bool
	// frameWanted is set while the app waits for a display frame.
	frameWanted bool
	motions     map[*view]*motion
	ghosts      []*ghost
	components  driver.NativeComponents
	unknown     map[string]bool // component names warned about
	dragTarget  *view           // the view files are dragged over
	dragFiles   bool            // whether the drag carries files
	applying    bool            // whether batches are applying

	// The gestures in progress.
	pan             *panTrack
	pinch, rotation *gestureTrack
	selecting       *selection

	// The performance monitor and the panel that shows it.
	perf *engine.PerfMonitor
	hud  *hud

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
	selApply                        = sel("nsgoApply:")
	selPerformOnMain                = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selWindow, selMakeFirstResponer = sel("window"), sel("makeFirstResponder:")
	selDocumentView                 = sel("documentView")
	selSetAccessibilityElement      = sel("setAccessibilityElement:")
	selSetAccessibilityRole         = sel("setAccessibilityRole:")
	selSetAccessibilitySubrole      = sel("setAccessibilitySubrole:")
	selSetAccessibilityEnabled      = sel("setAccessibilityEnabled:")
	selSetAccessibilitySelected     = sel("setAccessibilitySelected:")
	selSetAccessibilityValue        = sel("setAccessibilityValue:")
	selSetAccessibilityIdentifier   = sel("setAccessibilityIdentifier:")
	selIntrinsicContentSize         = sel("intrinsicContentSize")
	selCurrentContext, selCGContext = sel("currentContext"), sel("CGContext")
	selIsEqualToString              = sel("isEqualToString:")

	classView, classSurface, classInput objc.Class
	classScroll                         objc.Class
	classObserver, classMenuTarget      objc.Class
	classMaterial                       objc.Class
)

func loadAppKit() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	nsView := objc.GetClass("NSView")
	common := []objc.MethodDef{
		{Cmd: sel("makeBackingLayer"), Fn: makeBackingLayer},
		{Cmd: sel("isFlipped"), Fn: func(self objc.ID, _ objc.SEL) bool { return true }},
		{Cmd: sel("hitTest:"), Fn: hitTest},
		{Cmd: sel("mouseDown:"), Fn: mouseDown},
		{Cmd: sel("mouseUp:"), Fn: mouseUp},
		{Cmd: sel("mouseDragged:"), Fn: mouseDragged},
		{Cmd: sel("magnifyWithEvent:"), Fn: magnify},
		{Cmd: sel("rotateWithEvent:"), Fn: rotate},
		{Cmd: sel("rightMouseDown:"), Fn: rightMouseDown},
		{Cmd: sel("mouseEntered:"), Fn: mouseEntered},
		{Cmd: sel("mouseMoved:"), Fn: mouseMoved},
		{Cmd: sel("mouseExited:"), Fn: mouseExited},
		{Cmd: sel("resetCursorRects"), Fn: resetCursorRects},
		// A click on an inactive window is the app's, as NSH's views take
		// it: AppKit activates the window, then delivers the press.
		{Cmd: sel("acceptsFirstMouse:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool { return true }},
		// The surface moves the window from its drag handles, not AppKit.
		{Cmd: sel("mouseDownCanMoveWindow"), Fn: func(objc.ID, objc.SEL) bool { return false }},
	}
	common = append(common, accessMethods()...)
	classView = register("NsgoView", nsView, nil, append(common, focusMethods()...))
	// The material sits behind the app's views and takes no press.
	classMaterial = register("NsgoMaterialView", objc.GetClass("NSVisualEffectView"), nil, []objc.MethodDef{{
		Cmd: sel("hitTest:"),
		Fn:  func(objc.ID, objc.SEL, apple.CGPoint) objc.ID { return 0 },
	}})
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
		// It holds selections, but Tab passes it by.
		objc.MethodDef{Cmd: sel("canBecomeKeyView"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		objc.MethodDef{Cmd: sel("resignFirstResponder"), Fn: surfaceResign},
		// Keys reach the surface only while it holds a selection; it
		// takes them, as a text view would, rather than beep.
		objc.MethodDef{Cmd: sel("keyDown:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			// Tab still moves the keyboard focus. The surface is in no key
			// view loop, so the window starts it from its first key view.
			const tab, shift = 48, 1 << 17
			if v := views[self]; v != nil && objc.Send[uint16](event, sel("keyCode")) == tab {
				w := v.surface.window
				w.Send(sel("makeFirstResponder:"), objc.ID(0))

				if objc.Send[uint](event, sel("modifierFlags"))&shift != 0 {
					w.Send(sel("selectPreviousKeyView:"), objc.ID(0))
				} else {
					w.Send(sel("selectNextKeyView:"), objc.ID(0))
				}
			}
		}},
		objc.MethodDef{Cmd: sel("copy:"), Fn: surfaceCopy},
		objc.MethodDef{Cmd: sel("selectAll:"), Fn: surfaceSelectAll},
		objc.MethodDef{Cmd: selTick, Fn: surfaceTick},
		objc.MethodDef{Cmd: sel("draggingEntered:"), Fn: draggingEntered},
		objc.MethodDef{Cmd: sel("draggingUpdated:"), Fn: draggingUpdated},
		objc.MethodDef{Cmd: sel("draggingExited:"), Fn: draggingExited},
		objc.MethodDef{Cmd: sel("draggingEnded:"), Fn: draggingExited},
		objc.MethodDef{Cmd: sel("prepareForDragOperation:"), Fn: prepareForDrag},
		objc.MethodDef{Cmd: sel("performDragOperation:"), Fn: performDrag},
	))
	classObserver = register("NsgoObserver", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("observeValueForKeyPath:ofObject:change:context:"),
		Fn: func(self objc.ID, _ objc.SEL, path, object, change objc.ID, ctx uintptr) {
			apple.OnMain(func() { active.refresh() })
		},
	}})
	// A scroll view shows its scroll bars while the pointer is over it,
	// so they can be dragged, and reports hovers itself.
	classScroll = register("NsgoScrollView", objc.GetClass("NSScrollView"), nil, []objc.MethodDef{
		{Cmd: sel("mouseEntered:"), Fn: scrollEntered},
		{Cmd: sel("mouseMoved:"), Fn: func(self objc.ID, cmd objc.SEL, event objc.ID) {
			self.Send(sel("flashScrollers"))
			mouseMoved(self, cmd, event)
		}},
		{Cmd: sel("mouseExited:"), Fn: mouseExited},
		{Cmd: sel("scrollWheel:"), Fn: scrollWheel},
		{Cmd: sel("resetCursorRects"), Fn: resetCursorRects},
	})
	classInput = register("NsgoInput", nsView, nil, append(common,
		objc.MethodDef{Cmd: sel("layout"), Fn: inputLayout},
	))
	registerInput()
	registerContent()
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
	// Tab goes through the fields and the views that take presses.
	window.Send(sel("setAutorecalculatesKeyViewLoop:"), true)

	s := &surface{window: window, epoch: time.Now(), motions: map[*view]*motion{}, movers: map[*view]struct{}{},
		components: opts.Components}
	s.defaultMenu, s.appName = defaultMenu, opts.Title
	active = s
	s.setWindowDelegate()
	s.native = objc.ID(classSurface).Send(selAlloc).Send(sel("initWithFrame:"),
		apple.Rect(0, 0, float64(opts.Width), float64(opts.Height)))
	s.native.Send(selSetWantsLayer, true)
	views[s.native] = &view{surface: s}
	window.Send(sel("setContentView:"), s.native)
	s.applyMaterial(opts.Material)
	s.animator = driver.NewAnimator(func(e driver.Event) { s.engine.Dispatch(e) })
	s.acceptFiles()
	s.env = s.environment()
	var monitor *engine.PerfMonitor
	if opts.Perf {
		monitor = engine.NewPerfMonitor()
		s.perf = monitor
	}
	eopts := engine.Options{Size: driver.Size{Width: opts.Width, Height: opts.Height}, Environment: s.env, Perf: monitor,
		Thread: apple.RaiseThread}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	s.engine = engine.Start(s, app, eopts)
	s.observe()
	if monitor != nil {
		s.hud = newHud(s, monitor)
		s.startTicking()
	}
	window.Send(sel("center"))
	window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	// Activate once the run loop runs: a request before it can be lost,
	// and launch activation settles after the app has finished starting.
	apple.OnMain(func() {
		activateApp()
		window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	})
	nsApp.Send(sel("run"))
}

// activateApp makes the app active. activateIgnoringOtherApps: is
// deprecated but still the request the system honors soonest, and
// macOS 14's replacement, activate, is asynchronous.
func activateApp() {
	app := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	if objc.Send[bool](app, sel("respondsToSelector:"), sel("activateIgnoringOtherApps:")) {
		app.Send(sel("activateIgnoringOtherApps:"), true)
	} else {
		app.Send(sel("activate"))
	}
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
func (s *surface) environment() driver.Environment {
	ws := objc.ID(objc.GetClass("NSWorkspace")).Send(sel("sharedWorkspace"))
	locale, rtl := apple.Locale()
	appearance := apple.GoString(s.native.Send(sel("effectiveAppearance")).Send(sel("name")))
	return driver.Environment{
		Dark:           strings.Contains(appearance, "Dark"),
		FontScale:      1,
		ReduceMotion:   objc.Send[bool](ws, sel("accessibilityDisplayShouldReduceMotion")),
		ScreenReader:   objc.Send[bool](ws, sel("isVoiceOverEnabled")),
		RTL:            rtl,
		PixelRatio:     float32(objc.Send[float64](s.window, sel("backingScaleFactor"))),
		Locale:         locale,
		OS:             driver.OSMacOS,
		Idiom:          driver.IdiomDesktop,
		TitleBar:       s.titleBarArea(),
		WindowMaterial: s.material,
		Maximized:      objc.Send[bool](s.window, sel("isZoomed")),
	}
}

// refresh lays the title bar out again and reports the environment to the
// engine if it changed.
func (s *surface) refresh() {
	if s == nil || s.engine == nil {
		return
	}
	s.layoutTrafficLights()
	if env := s.environment(); env != s.env {
		rescale := env.PixelRatio != s.env.PixelRatio
		s.env = env
		s.engine.SetEnvironment(env)
		if rescale {
			s.rescale()
		}
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
	apple.Observe(center, apple.Global(appKit, "NSScrollViewWillStartLiveScrollNotification"),
		func(note objc.ID) { liveScroll(note, true) })
	apple.Observe(center, apple.Global(appKit, "NSScrollViewDidEndLiveScrollNotification"),
		func(note objc.ID) { liveScroll(note, false) })
	for _, name := range []string{"NSWindowDidResizeNotification", "NSWindowDidEnterFullScreenNotification",
		"NSWindowDidExitFullScreenNotification"} {
		apple.Observe(center, apple.Global(appKit, name), func(note objc.ID) {
			if note.Send(sel("object")) != s.window {
				return
			}
			s.refresh()
			if name != "NSWindowDidResizeNotification" {
				// AppKit lays the bar out again as it leaves full screen,
				// after the notification.
				apple.OnMain(func() { s.layoutTrafficLights(); s.refresh() })
			}
		})
	}
	observer := objc.ID(classObserver).Send(selNew)
	ws.Send(sel("addObserver:forKeyPath:options:context:"), observer, apple.NSString("voiceOverEnabled"), uint(0), uintptr(0))
	// The window's appearance lays its title bar out again too.
	s.window.Send(sel("addObserver:forKeyPath:options:context:"), observer,
		apple.NSString("effectiveAppearance"), uint(0), uintptr(0))

	const keyDown, leftDown, leftUp, leftDragged, rightDown, otherDown = 1 << 10, 1 << 1, 1 << 2, 1 << 6, 1 << 3, 1 << 25
	handler := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID { return s.filter(event) })
	objc.ID(objc.GetClass("NSEvent")).Send(sel("addLocalMonitorForEventsMatchingMask:handler:"),
		uint64(keyDown|leftDown|leftUp|leftDragged|rightDown|otherDown), handler)
	handler.Release()
}

// filter sees the app's key and mouse events before their views do, and
// measures them when a performance monitor does. It turns back keys and the
// back button into EventBack, which it takes, reports presses outside to the
// views that listen, and follows a selection's drag. Down presses are never
// taken: AppKit activates the window and hands them to the views that hit,
// which move the window or select text before their own handling.
func (s *surface) filter(event objc.ID) objc.ID {
	if s.perf == nil || event.Send(selWindow) != s.window {
		return s.filterEvent(event)
	}
	start := time.Now()
	out := s.filterEvent(event)
	const typeLeftDragged = 6
	dirty := objc.Send[uint](event, sel("type")) == typeLeftDragged
	s.perf.RecordWork(time.Since(start), dirty)
	return out
}

func (s *surface) filterEvent(event objc.ID) objc.ID {
	if event.Send(selWindow) != s.window {
		return event
	}
	const typeLeftDown, typeLeftUp, typeRightDown, typeLeftDragged, typeKeyDown, typeOtherDown = 1, 2, 3, 6, 10, 25
	switch objc.Send[uint](event, sel("type")) {
	case typeKeyDown:
		if binding, consumed := s.binding(event); consumed {
			if binding != "" {
				s.engine.Dispatch(driver.Event{Kind: driver.EventKey, Text: binding})
			}
			return 0
		}
		if s.selectKey(event) {
			return 0
		}
		if s.isBack(event) {
			s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
			return 0
		}
	case typeLeftDown:
		s.pressOutside(event)
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
			s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
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

// binding returns the keymap's binding a key event presses, as
// core.FindKey decides, and whether the event is consumed. It names the
// press both with "mod", Command here, and with "meta". Keys go to an
// input method composing text, as arrows choose its candidates.
func (s *surface) binding(event objc.ID) (string, bool) {
	if len(s.keys) == 0 || s.composing() || activates(event) && s.focusedPressable() {
		return "", false
	}
	name, ok := keyNames[objc.Send[uint16](event, sel("keyCode"))]
	if !ok {
		// The key's character without Shift or Option, in the current layout.
		name = strings.ToLower(apple.GoString(event.Send(sel("charactersByApplyingModifiers:"), uint(0))))
		if name == "" {
			return "", false
		}
	}
	return driver.FindKey(s.keys, apple.KeyNames(name, objc.Send[uint](event, sel("modifierFlags"))), s.editing(),
		objc.Send[bool](event, sel("isARepeat")))
}

// isBack reports whether a key event is Escape, Cmd-[ or Option-Left.
// Escape and Option-Left belong to a text field being edited.
func (s *surface) isBack(event objc.ID) bool {
	const shift, control, option, command = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	const escape, leftArrow = 53, 123
	mods := objc.Send[uint](event, sel("modifierFlags")) & (shift | control | option | command)
	editing := s.editing()
	switch code := objc.Send[uint16](event, sel("keyCode")); {
	case code == escape && mods == 0, code == leftArrow && mods == option:
		return !editing
	case mods == command:
		return apple.GoString(event.Send(sel("charactersIgnoringModifiers"))) == "["
	}
	return false
}

// composing reports whether an input method is composing text in the
// focused text field.
func (s *surface) composing() bool {
	first := s.window.Send(sel("firstResponder"))
	return first != 0 && objc.Send[bool](first, sel("respondsToSelector:"), sel("hasMarkedText")) &&
		objc.Send[bool](first, sel("hasMarkedText"))
}

// editing reports whether keys go to a text field or a web page.
func (s *surface) editing() bool {
	first := s.window.Send(sel("firstResponder"))
	if objc.Send[bool](first, sel("isKindOfClass:"), objc.GetClass("NSText")) {
		return true
	}
	for n := first; n != 0 && objc.Send[bool](n, sel("isKindOfClass:"), objc.GetClass("NSView")); n = n.Send(selSuperview) {
		if v := views[n]; v != nil && v.kind == driver.KindWeb {
			return true
		}
	}
	return false
}

// pressOutside reports a mouse down to each view listening for presses
// outside it that neither holds the view hit nor the point.
func (s *surface) pressOutside(event objc.ID) {
	loc := objc.Send[apple.CGPoint](event, selLocationInWindow)
	var hit objc.ID
	at := driver.Point{}
	for _, v := range s.views {
		if v == nil || !v.events.Has(driver.EventPressOutside) || v.native.Send(selWindow) == 0 {
			continue
		}
		if hit == 0 {
			p := objc.Send[apple.CGPoint](s.native, selConvertPoint, loc, objc.ID(0))
			at = driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
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
		s.engine.Dispatch(driver.Event{Kind: driver.EventPressOutside, ID: v.id, Point: at})
	}
}

// alert shows a as a sheet on the window. The Cancel action answers
// Escape; the sheet's answer goes to the engine as EventAlert.
func (s *surface) alert(a *driver.AlertConfig) {
	alert := objc.ID(objc.GetClass("NSAlert")).Send(selNew)
	alert.Send(sel("setMessageText:"), apple.NSString(a.Title))
	alert.Send(sel("setInformativeText:"), apple.NSString(a.Message))
	for _, act := range a.Actions {
		button := alert.Send(sel("addButtonWithTitle:"), apple.NSString(act.Label))
		switch act.Role {
		case driver.AlertCancel:
			button.Send(sel("setKeyEquivalent:"), apple.NSString("\x1b"))
		case driver.AlertDestructive:
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
		s.engine.Dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: index})
		alert.Send(selRelease)
	})
	alert.Send(sel("beginSheetModalForWindow:completionHandler:"), s.window, done)
	done.Release()
}

// pasteboardText is NSPasteboardTypeString.
const pasteboardText = "public.utf8-plain-text"

// scrollTo scrolls a scroll view's content to p, clamped to what it can
// show; the clip view reports the offset as it moves.
func (v *view) scrollTo(p driver.Point, animated bool) {
	if v.clip == 0 {
		return
	}
	lo, hi := v.limits()
	to := apple.CGPoint{X: float64(min(max(p.X, lo.X), hi.X)), Y: float64(min(max(p.Y, lo.Y), hi.Y))}
	clip := v.clip
	if animated {
		clip = clip.Send(sel("animator"))
	}
	clip.Send(sel("setBoundsOrigin:"), to)
	v.native.Send(sel("reflectScrolledClipView:"), v.clip)
}

// Prepare implements core.TextEngine with CoreText.
func (s *surface) Prepare(t *driver.AttributedText) driver.PreparedText {
	return apple.TextEngine{}.Prepare(t)
}

// Mount implements engine.Platform.
func (s *surface) Mount(b *driver.MountBatch) {
	if s.batches.Push(b) {
		s.native.Send(selPerformOnMain, selApply, objc.ID(0), false)
	}
}

// RequestFrame implements engine.Platform: the app waits for the next
// display frame. Called from the engine goroutine, so it hops to the main
// thread, where the display link lives.
func (s *surface) RequestFrame() {
	apple.OnMain(func() {
		s.frameWanted = true
		s.startTicking()
	})
}

func (s *surface) apply(b *driver.MountBatch) {
	var lb *layoutBatch
	if b.LayoutAnimation != nil && !s.env.ReduceMotion {
		lb = &layoutBatch{anim: b.LayoutAnimation, created: map[driver.NodeID]bool{}, deleted: map[objc.ID]bool{}}
	}
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate:
			for len(s.views) <= int(it.ID) {
				s.views = append(s.views, nil)
			}
			s.views[it.ID] = s.create(it.View, it.ID)
			if lb != nil {
				lb.created[it.ID] = true
			}
			continue
		case driver.ItemAnimation:
			s.animator.Apply(it.Animation)
			continue
		case driver.ItemAlert:
			s.alert(it.Alert)
			continue
		case driver.ItemDismissKeyboard:
			s.window.Send(selMakeFirstResponer, objc.ID(0))
			continue
		case driver.ItemSetClipboard:
			pb := objc.ID(objc.GetClass("NSPasteboard")).Send(sel("generalPasteboard"))
			pb.Send(sel("clearContents"))
			pb.Send(sel("setString:forType:"), apple.NSString(it.Value), apple.NSString(pasteboardText))
			continue
		case driver.ItemReadClipboard:
			pb := objc.ID(objc.GetClass("NSPasteboard")).Send(sel("generalPasteboard"))
			text := apple.GoString(pb.Send(sel("stringForType:"), apple.NSString(pasteboardText)))
			s.engine.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: it.Count, Text: text})
			continue
		case driver.ItemImageInfo:
			apple.ImageInfo(it, s.engine.Dispatch)
			continue
		case driver.ItemAnnounce:
			announce(it.Value)
			continue
		case driver.ItemKeymap:
			s.keys = it.Keys
			continue
		case driver.ItemTitle:
			s.window.Send(sel("setTitle:"), apple.NSString(it.Value))
			s.layoutTrafficLights()
			continue
		case driver.ItemWindow:
			s.command(it.Window)
			continue
		case driver.ItemMenu:
			s.setMenu(it.Menu)
			continue
		case driver.ItemStatusBar, driver.ItemBackUnhandled:
			continue
		}
		v := s.views[it.ID]
		switch it.Kind {
		case driver.ItemDelete:
			v.up = nil
			kept := s.vanish(v, lb)
			s.delete(v, kept)
			if v.kind == driver.KindModal {
				s.modalGone(v)
			}
		case driver.ItemRemove:
			v.up = nil
			v.native.Send(selRemoveFromSuperview)
		case driver.ItemInsert:
			v.up = s.views[it.Parent]
			parent := s.views[it.Parent].content
			if it.Before == 0 {
				parent.Send(selAddSubview, v.native)
			} else {
				const below = -1
				parent.Send(selAddSubviewPositioned, v.native, below, s.views[it.Before].native)
			}
			if v.style.Cursor != driver.CursorAuto {
				invalidateCursor(v)
			}
			if v.kind == driver.KindModal && !v.presented {
				v.presented = true
				s.presenting = append(s.presenting, v)
				s.modalCame(v)
			}
			s.appear(v, lb, it.Parent)
		case driver.ItemFrame:
			s.setFrame(v, it.Frame, lb)
		case driver.ItemOverflow:
			v.overflow = it.Frame
		case driver.ItemViewStyle:
			cursor := v.style.Cursor
			v.style = *it.ViewStyle
			if cursor != v.style.Cursor {
				invalidateCursor(v)
			}
			if v.animatesStyle() {
				v.restyle()
			} else {
				apple.StyleLayer(v.native.Send(selLayer), it.ViewStyle, true)
				v.transform = it.ViewStyle.Transform
			}
			v.native.Send(selSetHidden, it.ViewStyle.Hidden)
			if v.image != nil {
				v.image.SetStyle(it.ViewStyle)
			}
		case driver.ItemImage:
			v.image.Configure(it.Image)
			s.images = append(s.images, v)
		case driver.ItemModal:
			v.animation = it.Modal.Animation
		case driver.ItemWeb:
			delete(views, v.web.View())
			v.web.Configure(it.Web)
			views[v.web.View()] = v
		case driver.ItemWebCommand:
			v.web.Command(it.WebCommand, it.Value)
		case driver.ItemVideo:
			delete(views, v.video.View())
			v.video.Configure(it.Video)
			if shown := v.video.View(); shown != 0 {
				views[shown] = v
			}
		case driver.ItemVideoSeek:
			v.video.Seek(it.Time)
		case driver.ItemNative:
			s.configureNative(v, it.Native)
		case driver.ItemNativeCommand:
			if v.component != nil {
				v.component.Command(it.Value, it.Data)
			}
		case driver.ItemCanvas:
			v.drawing = it.Drawing
			v.redraw()
		case driver.ItemShader:
			s.setShader(v, it.Shader)
		case driver.ItemScrollTo:
			if v.sc != nil {
				v.sc.applied++
				v.interruptSnap()
				v.scrollTo(it.Point, it.Animated)
			}
		case driver.ItemScrollBy:
			if v.sc != nil {
				v.sc.applied++
				v.scrollBy(it.Point)
			}
		case driver.ItemStickyCells:
			if v.sc != nil {
				v.setSticky(it.Sticky)
			}
		case driver.ItemViewProps:
			v.setProps(it.ViewProps)
			p := it.ViewProps
			v.windowDrag, v.activation, v.area, v.selectable = p.WindowDrag, p.PanActivation, p.SelectionArea, p.Selectable
			v.disabled = p.Disabled
			if v.selColor != p.SelectionColor {
				v.selColor = p.SelectionColor
				v.redraw()
			}
		case driver.ItemEvents:
			v.setEvents(it.Events)
			v.updateAccessibility()
		case driver.ItemAccessibilityLabel:
			v.setLabel(it.Value)
		case driver.ItemAccessibilityFocus:
			v.focusAccessibility()
		case driver.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.box = it.Text.Content
			v.textChanged(it.Text.Text)
			v.redraw()
		case driver.ItemInput:
			v.configure(it.Input)
		case driver.ItemInputValue:
			v.input.value(it.Value, it.Count)
		case driver.ItemSelection:
			if v.input != nil {
				v.input.setSelection(it.Selection)
			}
		case driver.ItemScrollConfig:
			v.configureScroll(it.Scroll)
		case driver.ItemScrollContent:
			v.content.Send(selSetFrameSize, apple.CGSize{Width: float64(it.Size.Width), Height: float64(it.Size.Height)})
		case driver.ItemFocus:
			if v.input != nil {
				v.input.focus()
			}
		case driver.ItemBlur:
			if w := v.native.Send(selWindow); w != 0 {
				w.Send(selMakeFirstResponer, objc.ID(0))
			}
		}
	}
	s.root = b.Root
	if root := s.views[b.Root]; root.native.Send(selSuperview) != s.native {
		s.native.Send(selAddSubview, root.native)
	}
	s.finish()
	s.revisit()
	s.refreshSelection()
	apple.WithoutAnimation(func() {
		if lb != nil {
			s.stepMotions(s.clock())
		}
		s.applyAnimated()
	})
	s.drawShaders()
	if s.animator.Running() {
		s.startTicking()
	}
}

// finish starts what a batch asked for once its frames are set: image
// loads at the views' sizes and modals' entrances.
func (s *surface) finish() {
	s.startImages()
	for _, v := range s.presenting {
		if s.views[v.id] == v && v.native.Send(selSuperview) != 0 {
			h := objc.Send[apple.CGRect](v.native.Send(selSuperview), selBounds).Size.Height
			apple.AnimateIn(v.native.Send(selLayer), v.animation, h, s.env.ReduceMotion)
		}
	}
	clear(s.presenting)
	s.presenting = s.presenting[:0]
}

// startImages starts the image loads a batch or a frame's layout asked
// for, at the views' new sizes.
func (s *surface) startImages() {
	for _, v := range s.images {
		if s.views[v.id] == v {
			v.image.Start(float64(s.env.PixelRatio))
		}
	}
	clear(s.images)
	s.images = s.images[:0]
}

func (s *surface) create(kind driver.ViewKind, id driver.NodeID) *view {
	v := &view{surface: s, id: id, kind: kind}
	switch kind {
	case driver.KindScroll:
		v.native = objc.ID(classScroll).Send(selNew)
		v.native.Send(sel("setDrawsBackground:"), false)
		v.native.Send(sel("setAutohidesScrollers:"), true)
		v.native.Send(sel("setAutomaticallyAdjustsContentInsets:"), false)
		v.sc = &scrollState{}
		v.hover = trackingArea(v.native, true)
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
	case driver.KindInput:
		v.native = objc.ID(classInput).Send(selNew)
		v.newInput()
	default:
		v.native = objc.ID(classView).Send(selNew)
	}
	v.native.Send(selSetWantsLayer, true)
	if v.content == 0 {
		v.content = v.native
	}
	switch kind {
	case driver.KindImage:
		v.image = apple.NewImageView(v.native.Send(selLayer))
		v.image.Dispatch = v.report
	case driver.KindWeb:
		v.web = apple.NewWeb(v.native)
		v.web.Dispatch = v.report
	case driver.KindVideo:
		v.video = apple.NewVideo(v.native)
		v.video.Dispatch = v.report
	case driver.KindShader:
		v.newShader()
	}
	views[v.native] = v
	return v
}

// delete releases v; kept leaves its native where it is, going away.
func (s *surface) delete(v *view, kept bool) {
	if !kept && !s.inGhost(v.native) {
		v.native.Send(selRemoveFromSuperview)
	}
	s.animator.Forget(v.id)
	delete(s.motions, v)
	if s.dragTarget == v {
		s.dragTarget = nil
	}
	if v.component != nil {
		v.component.Close()
		v.component = nil
	}
	apple.ForgetTransform(v.native.Send(selLayer))
	apple.ForgetBorders(v.native.Send(selLayer))
	v.forgetContent()
	if v.shader != nil {
		v.shader.Release()
	}
	v.layout.Release()
	if v.image != nil {
		v.image.Close()
	}
	if v.web != nil {
		delete(views, v.web.View())
		v.web.Close()
	}
	if v.video != nil {
		delete(views, v.video.View())
		v.video.Close()
	}
	if v.hover != 0 {
		v.hover.Send(selRelease)
	}
	delete(views, v.native)
	if in := v.input; in != nil {
		delete(views, in.field)
		delete(views, in.host)
	}
	delete(views, v.clip)
	v.native.Send(selRelease)
	delete(s.movers, v)
	s.views[v.id] = nil
}

func (v *view) setEvents(m driver.EventMask) {
	if v.input != nil {
		v.input.fs.Events = m
	}
	v.events = m
	moves := m.Has(driver.EventHoverMove) || m.Has(driver.EventMouseMove)
	wantHover := moves || m.Has(driver.EventHoverIn) || m.Has(driver.EventHoverOut)
	if moves {
		v.surface.movers[v] = struct{}{}
	} else {
		delete(v.surface.movers, v)
	}
	if v.kind == driver.KindScroll { // its tracking area reports hovers and moves
		return
	}
	if v.hover != 0 && (!wantHover || v.moves != moves) {
		v.native.Send(sel("removeTrackingArea:"), v.hover)
		v.hover.Send(selRelease)
		v.hover = 0
	}
	if wantHover && v.hover == 0 {
		v.hover, v.moves = trackingArea(v.native, moves), moves
	}
}

// trackingArea adds a tracking area over view's visible rect, reporting
// the pointer entering and leaving it and, if moves, moving within it.
func trackingArea(view objc.ID, moves bool) objc.ID {
	const enteredAndExited, mouseMoved, activeInActiveApp, inVisibleRect = 0x01, 0x02, 0x40, 0x200
	options := uint(enteredAndExited | activeInActiveApp | inVisibleRect)
	if moves {
		options |= mouseMoved
	}
	area := objc.ID(objc.GetClass("NSTrackingArea")).Send(selAlloc).Send(
		sel("initWithRect:options:owner:userInfo:"), apple.Rect(0, 0, 0, 0), options, view, objc.ID(0))
	view.Send(sel("addTrackingArea:"), area)
	return area
}

func (v *view) dispatch(kind driver.EventKind) {
	v.report(driver.Event{Kind: kind})
}

// pointer returns event's position in v and in its root view, and the
// modifier keys held, for an event of kind.
func (v *view) pointer(kind driver.EventKind, event objc.ID) driver.Event {
	loc := objc.Send[apple.CGPoint](event, selLocationInWindow)
	p := objc.Send[apple.CGPoint](v.native, selConvertPoint, loc, objc.ID(0))
	return driver.Event{Kind: kind, Point: driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}, Page: v.page(loc),
		Modifiers: apple.Modifiers(objc.Send[uint](event, sel("modifierFlags")))}
}

// page converts a window point to the root view v is in: the modal
// around it, or the surface.
func (v *view) page(loc apple.CGPoint) driver.Point {
	root := v.surface.native
	for n := v.native; n != 0 && n != root; n = n.Send(selSuperview) {
		if m := views[n]; m != nil && m.native == n && m.kind == driver.KindModal {
			root = n
			break
		}
	}
	p := objc.Send[apple.CGPoint](root, selConvertPoint, loc, objc.ID(0))
	return driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
}

// pressed reports kind about the press v follows, with where it began.
func (v *view) pressed(kind driver.EventKind) {
	e := v.pressAt
	e.Kind = kind
	v.report(e)
}

// report sends e about v if v listens for its kind.
func (v *view) report(e driver.Event) {
	if v.events.Has(e.Kind) {
		e.ID = v.id
		// Values bound to the gesture move before it is reported, so they
		// are drawn in the frame that shows the move.
		if s := v.surface; s.animator.Gesture(&e) {
			apple.WithoutAnimation(s.applyAnimated)
		}
		v.surface.engine.Dispatch(e)
	}
}

func nsColor(c driver.Color) objc.ID {
	r, g, b, a := c.Components()
	return objc.ID(objc.GetClass("NSColor")).Send(sel("colorWithSRGBRed:green:blue:alpha:"), r, g, b, a)
}

// hitTest lets presses on text and plain containers through to the
// nearest pressable ancestor; text fields keep theirs.
func hitTest(self objc.ID, cmd objc.SEL, p apple.CGPoint) objc.ID {
	if rawHit {
		return apple.SendSuper[objc.ID](self, cmd, p)
	}
	hit := reach(self, cmd, p)
	stop := self.Send(selSuperview)
	for n := hit; n != 0 && n != stop; n = n.Send(selSuperview) {
		if v := views[n]; v != nil {
			if v.isEditor(n) || v.kind == driver.KindScroll || v.kind == driver.KindWeb {
				return hit
			}
			if v.kind == driver.KindVideo && v.native != n {
				return hit // the player's controls
			}
			if (v.pressable() || v.events.Has(driver.EventPan)) && v.native == n {
				return n
			}
		}
	}
	return hit
}

// reach is AppKit's hit test of self at p, in its superview's
// coordinates, by its pointer events and hit slop, and looking in its
// children where they reach outside it, as core.Reach says.
func reach(self objc.ID, cmd objc.SEL, p apple.CGPoint) objc.ID {
	v := views[self]
	if v == nil || v.native != self {
		return apple.SendSuper[objc.ID](self, cmd, p)
	}
	f := objc.Send[apple.CGRect](self, sel("frame"))
	frame := driver.Bounds{Origin: driver.Point{X: driver.Points(f.Origin.X), Y: driver.Points(f.Origin.Y)},
		Size: driver.Size{Width: driver.Points(f.Size.Width), Height: driver.Points(f.Size.Height)}}
	at := driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
	own, children := driver.Reach(frame, &v.props, v.overflow, at)
	if !frame.Contains(at) {
		if children {
			local := objc.Send[apple.CGPoint](self, selConvertPoint, p, self.Send(selSuperview))
			subs := self.Send(sel("subviews"))
			for i := objc.Send[int](subs, sel("count")) - 1; i >= 0; i-- {
				if hit := subs.Send(sel("objectAtIndex:"), i).Send(cmd, local); hit != 0 {
					return hit
				}
			}
		}
		if own {
			return self
		}
		return 0
	}
	hit := apple.SendSuper[objc.ID](self, cmd, p)
	switch {
	case !own && !children:
		return 0
	case !children && hit != 0: // box-only
		return self
	case !own && hit == self: // box-none
		return 0
	}
	return hit
}

// within reports whether p, in v's coordinates, is on v of bounds b or
// in its hit slop, where a press ends as a press.
func (v *view) within(p apple.CGPoint, b apple.CGRect) bool {
	s := v.props.HitSlop
	return p.X >= -float64(s.Left) && p.Y >= -float64(s.Top) && p.X < b.Size.Width+float64(s.Right) &&
		p.Y < b.Size.Height+float64(s.Bottom)
}

// longPressDelay is how long a press on v is held to be a long press.
func (v *view) longPressDelay() time.Duration {
	if d := v.props.DelayLongPress; d > 0 {
		return d
	}
	return longPressDelay
}

// touch reports event, a mouse event at a point of the window, as a
// finger with report, once however many views it reaches.
func (s *surface) touch(event objc.ID, report func(*surface, driver.Point) (driver.Event, bool)) {
	seen := s.touched
	seen.event, seen.kind, seen.time = event, objc.Send[int](event, sel("type")), objc.Send[float64](event, selTimestamp)
	if seen == s.touched {
		return
	}
	s.touched = seen
	loc := objc.Send[apple.CGPoint](event, selLocationInWindow)
	p := objc.Send[apple.CGPoint](s.native, selConvertPoint, loc, objc.ID(0))
	if ev, ok := report(s, driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}); ok {
		s.engine.Dispatch(ev)
	}
}

// listensTouches reports whether a view listens to touches.
func (s *surface) listensTouches() bool {
	for _, v := range s.views {
		if v != nil && finger.Listens(v.events) {
			return true
		}
	}
	return false
}

func (v *view) pressable() bool {
	return v.events.Has(driver.EventPress) || v.events.Has(driver.EventLongPress) ||
		v.events.Has(driver.EventPressDown) || v.events.Has(driver.EventPressUp)
}

func mouseDown(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	// Control-click is a secondary click.
	const control = 1 << 18
	if v != nil && v.events.Has(driver.EventContextMenu) && objc.Send[uint](event, sel("modifierFlags"))&control != 0 {
		rightMouseDown(self, cmd, event)
		return
	}
	if v != nil {
		v.surface.touch(event, func(s *surface, p driver.Point) (driver.Event, bool) {
			return s.mouse.Down(s.listensTouches(), p)
		})
	}
	// AppKit has activated the window and handed the press to the view it
	// hit: a drag handle moves the window, and a selection area starts or
	// extends its selection, before the view sees it.
	if v != nil && v.surface.takePress(event) {
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
	v.pressAt = v.pointer(driver.EventPressDown, event)
	v.pressAt.Count = uint32(max(objc.Send[int](event, sel("clickCount")), 1))
	v.pressed(driver.EventPressDown)
	if v.events.Has(driver.EventLongPress) {
		e, long := v.surface.engine, v.pressAt
		long.Kind, long.ID = driver.EventLongPress, v.id
		time.AfterFunc(v.longPressDelay(), func() {
			if v.pressing.Load() && v.longFired.CompareAndSwap(false, true) {
				e.Dispatch(long)
			}
		})
	}
}

func mouseUp(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	if v != nil {
		v.surface.touch(event, func(s *surface, p driver.Point) (driver.Event, bool) { return s.mouse.Up(p) })
	}
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
	v.pressed(driver.EventPressUp)
	if pressed && !v.longFired.Load() && v.within(p, b) {
		e := v.pressAt
		e.Kind, e.Index = driver.EventPress, v.indexAt(p)
		v.report(e)
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
	if v == nil || !v.events.Has(driver.EventContextMenu) {
		apple.Super(self, cmd, event)
		return
	}
	v.report(v.pointer(driver.EventContextMenu, event))
}

// resetCursorRects gives the view its style's cursor, or none to inherit
// its parent's.
func resetCursorRects(self objc.ID, cmd objc.SEL) {
	apple.Super(self, cmd)
	v := views[self]
	if v == nil || v.style.Cursor == driver.CursorAuto {
		return
	}
	self.Send(sel("addCursorRect:cursor:"), objc.Send[apple.CGRect](self, selBounds), nsCursor(v.style.Cursor))
}

// invalidateCursor makes the window ask for the view's cursor rects again.
func invalidateCursor(v *view) {
	if w := v.native.Send(selWindow); w != 0 {
		w.Send(sel("invalidateCursorRectsForView:"), v.native)
	}
}

// nsCursor is the AppKit cursor of c. AppKit has no public wait, progress,
// help or zoom cursors: those show the arrow.
func nsCursor(c driver.Cursor) objc.ID {
	class := objc.ID(objc.GetClass("NSCursor"))
	pick := func(name string) objc.ID { return class.Send(sel(name)) }
	switch c {
	case driver.CursorPointer:
		return pick("pointingHandCursor")
	case driver.CursorText:
		return pick("IBeamCursor")
	case driver.CursorCrosshair:
		return pick("crosshairCursor")
	case driver.CursorMove, driver.CursorGrab:
		return pick("openHandCursor")
	case driver.CursorGrabbing:
		return pick("closedHandCursor")
	case driver.CursorNotAllowed:
		return pick("operationNotAllowedCursor")
	case driver.CursorColResize:
		return pick("resizeLeftRightCursor")
	case driver.CursorRowResize:
		return pick("resizeUpDownCursor")
	case driver.CursorNone:
		return invisibleCursor()
	}
	return pick("arrowCursor")
}

var invisibleNS objc.ID

// invisibleCursor is a cursor that draws nothing.
func invisibleCursor() objc.ID {
	if invisibleNS == 0 {
		image := objc.ID(objc.GetClass("NSImage")).Send(sel("alloc"))
		image = image.Send(sel("initWithSize:"), apple.CGSize{Width: 1, Height: 1})
		invisibleNS = objc.ID(objc.GetClass("NSCursor")).Send(sel("alloc"))
		invisibleNS = invisibleNS.Send(sel("initWithImage:hotSpot:"), image, apple.CGPoint{})
		image.Send(selRelease)
	}
	return invisibleNS
}

func mouseEntered(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		defer v.surface.measuring(false)()
		v.dispatch(driver.EventHoverIn)
	}
}

// wheelOwner is who takes a scroll view's wheel events: the view itself
// or the scroll views around it.
type wheelOwner uint8

const (
	wheelUndecided wheelOwner = iota
	wheelOwn
	wheelPass
)

// scrollWheel scrolls the view unless it is at its edge in the wheel's
// direction, when the scroll views around it scroll instead. A trackpad
// gesture and its momentum keep the choice made as it began; a mouse
// wheel makes it on each turn.
func scrollWheel(self objc.ID, cmd objc.SEL, event objc.ID) {
	const began, mayBegin = 1, 32
	v := views[self]
	if v == nil {
		apple.Super(self, cmd, event)
		return
	}
	defer v.surface.measuring(true)()
	phase, momentum := objc.Send[uint](event, sel("phase")), objc.Send[uint](event, sel("momentumPhase"))
	if phase&(began|mayBegin) != 0 || phase == 0 && momentum == 0 {
		v.wheel = wheelUndecided
	}
	dx, dy := objc.Send[float64](event, sel("scrollingDeltaX")), objc.Send[float64](event, sel("scrollingDeltaY"))
	if v.wheel == wheelUndecided && (dx != 0 || dy != 0) {
		v.wheel = wheelPass
		if v.canScroll(dx, dy) {
			v.wheel = wheelOwn
		}
	}
	if next := self.Send(sel("nextResponder")); v.wheel == wheelPass && next != 0 {
		next.Send(cmd, event)
		return
	}
	if v.scrollWheelOwn(event, phase, momentum, dx, dy) {
		return
	}
	apple.Super(self, cmd, event)
}

// canScroll reports whether a scroll view's content can move by a wheel's
// deltas along their main axis: positive ones show what is above or left.
func (v *view) canScroll(dx, dy float64) bool {
	if v.sc.config.Behavior.NoScroll {
		return false
	}
	at := v.offset()
	lo, hi := v.limits()
	if math.Abs(dx) > math.Abs(dy) {
		return canScrollAxis(dx, float64(at.X), float64(lo.X), float64(hi.X))
	}
	return canScrollAxis(dy, float64(at.Y), float64(lo.Y), float64(hi.Y))
}

// scrollEntered shows a scroll view's scroll bars as the pointer enters.
func scrollEntered(self objc.ID, cmd objc.SEL, event objc.ID) {
	self.Send(sel("flashScrollers"))
	mouseEntered(self, cmd, event)
}

// mouseMoved reports a move with no button down to the view whose
// tracking area it crossed. It goes no further up the responder chain,
// since the views around follow their own areas.
func mouseMoved(self objc.ID, _ objc.SEL, event objc.ID) {
	v := views[self]
	if v == nil || v.native != self {
		return
	}
	defer v.surface.measuring(false)()
	if v.events.Has(driver.EventHoverMove) {
		v.report(v.pointer(driver.EventHoverMove, event))
	}
	if v.events.Has(driver.EventMouseMove) {
		v.report(v.pointer(driver.EventMouseMove, event))
	}
}

// dragMoved reports a move with a button down, which AppKit sends only to
// the view the press began in, to every view under the pointer that
// follows mouse moves, innermost first.
func (s *surface) dragMoved(event objc.ID) {
	if len(s.movers) == 0 {
		return
	}
	loc := objc.Send[apple.CGPoint](event, selLocationInWindow)
	type under struct {
		v     *view
		depth int
		at    apple.CGPoint
	}
	var hit []under
	for v := range s.movers {
		if !v.events.Has(driver.EventMouseMove) || v.native.Send(selWindow) == 0 ||
			objc.Send[bool](v.native, sel("isHiddenOrHasHiddenAncestor")) {
			continue
		}
		p := objc.Send[apple.CGPoint](v.native, selConvertPoint, loc, objc.ID(0))
		b := objc.Send[apple.CGRect](v.native, selBounds)
		if p.X < 0 || p.Y < 0 || p.X >= b.Size.Width || p.Y >= b.Size.Height {
			continue
		}
		depth := 0
		for n := v.native; n != 0; n = n.Send(selSuperview) {
			depth++
		}
		hit = append(hit, under{v, depth, p})
	}
	slices.SortFunc(hit, func(a, b under) int { return b.depth - a.depth })
	for _, u := range hit {
		u.v.report(driver.Event{Kind: driver.EventMouseMove, Point: driver.Point{X: driver.Points(u.at.X), Y: driver.Points(u.at.Y)}})
	}
}

func mouseExited(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		defer v.surface.measuring(false)()
		v.dispatch(driver.EventHoverOut)
	}
}

func surfaceSetFrameSize(self objc.ID, cmd objc.SEL, size apple.CGSize) {
	v := views[self]
	if v != nil && v.surface != nil {
		// An animated resize, such as zooming from the title bar, steps the
		// frame in a loop that runs no run loop sources until it ends, so
		// nsgoApply: waits. Each step applies the commits that came in
		// since the last one instead: the content trails a step behind.
		v.surface.applyQueued()
	}
	if v != nil && v.surface != nil && v.surface.perf != nil {
		defer v.surface.measuring(false)()
	}
	apple.Super(self, cmd, size)
	if v != nil && v.surface != nil && v.surface.engine != nil {
		v.surface.engine.Resize(driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)})
	}
	if v != nil && v.surface != nil && v.surface.hud != nil {
		v.surface.hud.layout()
	}
}

func surfaceApply(self objc.ID, _ objc.SEL, _ objc.ID) {
	views[self].surface.applyQueued()
}

// applyQueued applies the batches the engine has sent, unless a batch is
// being applied: one can resize the surface, and batches apply in order.
func (s *surface) applyQueued() {
	if s.applying {
		return
	}
	batches := s.batches.Drain()
	if len(batches) == 0 {
		return
	}
	s.applying = true
	defer func() { s.applying = false }()
	if s.perf == nil {
		for _, b := range batches {
			s.apply(b)
		}
	} else {
		start := time.Now()
		for _, b := range batches {
			if !b.Sent.IsZero() {
				s.perf.RecordLatency(time.Since(b.Sent))
			}
			s.apply(b)
		}
		s.perf.RecordWork(time.Since(start), true)
		if s.hud != nil {
			s.hud.raise()
		}
	}
	// A frame requested before the view was in a window starts the clock
	// here: a display link needs the view mounted.
	if s.frameWanted && !s.ticking {
		s.startTicking()
	}
}

// clipScrolled reports a scroll view's offset when its clip view moves.
func clipScrolled(note objc.ID) {
	clip := note.Send(sel("object"))
	v := views[clip]
	if v == nil || v.clip != clip {
		return
	}
	o := objc.Send[apple.CGRect](clip, selBounds).Origin
	offset := driver.Point{X: driver.Points(o.X), Y: driver.Points(o.Y)}
	v.sc.shown = offset
	if v.surface.animator.Scroll(v.id, offset) {
		apple.WithoutAnimation(v.surface.applyAnimated)
	}
	if len(v.sc.sticky) > 0 {
		v.pinSticky()
	}
	v.surface.revisit()
	if v.events.Has(driver.EventScroll) {
		v.surface.engine.Dispatch(driver.Event{Kind: driver.EventScroll, ID: v.id, Point: offset, Count: v.sc.applied})
	}
}
