//go:build ios

// Package ios is NSGo's UIKit backend: one UIView per node, styled through
// its CALayer, with text the engine laid out with CoreText drawn on the
// main thread and images decoded off it. Alerts are UIAlertControllers.
// It calls UIKit through purego; Go itself needs cgo to link for iOS.
package ios

import (
	"runtime"
	"strings"
	"structs"
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
	// UIKit runs on the process's main thread, which main starts on.
	runtime.LockOSThread()
}

// longPressDelay is how long a touch is held before it is a long press.
const longPressDelay = 500 * time.Millisecond

// view is the Go side of one native view.
type view struct {
	surface *surface
	id      core.NodeID
	kind    core.ViewKind
	native  objc.ID
	events  core.EventMask

	pressing  atomic.Bool
	longFired atomic.Bool

	layout *apple.Layout // a text view's
	box    core.Bounds

	sc    *scrollState // a scroll view's
	input *input       // an input's editor
	inset core.Bounds

	image     *apple.ImageView
	modal     core.ModalConfig // a modal's
	presented bool             // whether a modal was inserted yet
	sheet     *sheet           // a modal presented as a sheet
	bar       *navBar          // a screen's system header
	// headerWrap holds a header view in its screen's bar.
	headerWrap objc.ID
	drawing    *core.Drawing // a canvas's
	web        *apple.Web    // a web view's
	video      *apple.Video  // a video's

	activation           core.PanActivation
	pan, pinch, rotation objc.ID // the recognizers of the gestures it follows

	// A text's content and selection, whether a view is an area, and the
	// long press that selects in it.
	plain            string
	selectable       core.Selectable
	selColor         core.Color
	selStart, selEnd int // UTF-16
	area             bool
	longPress        objc.ID

	// props are the view's last props; the rest what access.go last
	// gave UIKit.
	props               core.ViewProps
	propsSet, textSet   bool
	axElement, axCustom bool

	// frame is where the view shows now; style its own style, which its
	// animated values and a layout animation draw over.
	frame    core.Bounds
	framed   bool
	shift    core.Point // a sticky cell's, from frame
	style    core.ViewStyle
	animated core.AnimatedValues

	component      core.NativeComponent // a native view's
	componentTried bool
	accessory      core.NodeID // an input's InputAccessoryView
}

type surface struct {
	app     engine.App
	id      string
	native  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []*view // index is NodeID
	env     core.Environment

	keyboard         core.Points
	keyboardDuration time.Duration

	keys        map[string]bool // the keymap's bindings
	statusBar   int             // the UIStatusBarStyle the controller asks for
	hideBar     bool
	barAnimated bool
	// headers are the navigation bar's heights; oriented the modals
	// that limit the orientations, in the order they were presented.
	headers  [2]core.Points
	oriented []*view

	// What a batch touched, finished once its frames are set.
	images, presenting []*view

	selecting *selection

	// The frame clock and what it animates.
	animator   *core.Animator
	epoch      time.Time
	link       objc.ID // the CADisplayLink
	ticking    bool
	motions    map[*view]*motion
	ghosts     []*ghost
	components core.NativeComponents
	unknown    map[string]bool
	// accessories are the inputs each InputAccessoryView rides above.
	accessories map[core.NodeID]*view
}

// edgeInsets is UIEdgeInsets.
type edgeInsets struct {
	_                        structs.HostLayout
	Top, Left, Bottom, Right float64
}

// views maps native objects to their Go side. Only the main thread uses it.
var views = map[objc.ID]*view{}

// app is the surface UIApplicationMain's delegate shows; UIKit makes the
// delegate, so it cannot be handed one.
var app *surface

var (
	sel = objc.RegisterName

	selAlloc, selNew                 = sel("alloc"), sel("new")
	selRelease                       = sel("release")
	selLayer                         = sel("layer")
	selSetFrame, selBounds           = sel("setFrame:"), sel("bounds")
	selAddSubview, selSuperview      = sel("addSubview:"), sel("superview")
	selInsertBelow                   = sel("insertSubview:belowSubview:")
	selRemoveFromSuperview           = sel("removeFromSuperview")
	selSetHidden                     = sel("setHidden:")
	selSetNeedsDisplay               = sel("setNeedsDisplay")
	selSetNeedsLayout                = sel("setNeedsLayout")
	selLocationInView                = sel("locationInView:")
	selAnyObject                     = sel("anyObject")
	selText, selSetText              = sel("text"), sel("setText:")
	selSetFont, selSetTextColor      = sel("setFont:"), sel("setTextColor:")
	selSetPlaceholder                = sel("setPlaceholder:")
	selSetTextAlignment              = sel("setTextAlignment:")
	selSetSecure                     = sel("setSecureTextEntry:")
	selSetKeyboardType               = sel("setKeyboardType:")
	selSetEnabled                    = sel("setEnabled:")
	selBecomeFirst, selResign        = sel("becomeFirstResponder"), sel("resignFirstResponder")
	selApply                         = sel("nsgoApply:")
	selChanged                       = sel("nsgoChanged:")
	selPerformOnMain                 = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selSetIsAccessibilityElement     = sel("setIsAccessibilityElement:")
	selSetAccessibilityTraits        = sel("setAccessibilityTraits:")
	selSetAccessibilityValue         = sel("setAccessibilityValue:")
	selSetAccessibilityIdentifier    = sel("setAccessibilityIdentifier:")
	selSetAccessibilityContainerType = sel("setAccessibilityContainerType:")
	selIsEqualToString               = sel("isEqualToString:")
	selSetContentMode                = sel("setContentMode:")
	selSetOpaque                     = sel("setOpaque:")
	selInitWithFrame, selSetDeleg    = sel("initWithFrame:"), sel("setDelegate:")
	selContentOffset                 = sel("contentOffset")
	selSetContentSize                = sel("setContentSize:")

	classView, classSurface, classInput, classScroll, classWatcher, classController objc.Class

	uiGraphicsGetCurrentContext func() uintptr
	uiApplicationMain           func(argc int, argv unsafe.Pointer, principal, delegate uintptr) int
	uiReduceMotion, uiVoiceOver func() bool
	uiAccessibilityPost         func(notification uint32, argument objc.ID)
	announcement                uint32 // UIAccessibilityAnnouncementNotification
	traits                      = map[string]uint64{}
)

const uikitPath = "/System/Library/Frameworks/UIKit.framework/UIKit"

func loadUIKit() {
	uikit, err := purego.Dlopen(uikitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&uiGraphicsGetCurrentContext, uikit, "UIGraphicsGetCurrentContext")
	purego.RegisterLibFunc(&uiApplicationMain, uikit, "UIApplicationMain")
	purego.RegisterLibFunc(&uiReduceMotion, uikit, "UIAccessibilityIsReduceMotionEnabled")
	purego.RegisterLibFunc(&uiVoiceOver, uikit, "UIAccessibilityIsVoiceOverRunning")
	purego.RegisterLibFunc(&uiAccessibilityPost, uikit, "UIAccessibilityPostNotification")
	loadAccessibility(uikit)

	uiView := objc.GetClass("UIView")
	common := []objc.MethodDef{
		{Cmd: sel("drawRect:"), Fn: drawRect},
		{Cmd: sel("touchesBegan:withEvent:"), Fn: touchesBegan},
		{Cmd: sel("touchesEnded:withEvent:"), Fn: touchesEnded},
		{Cmd: sel("touchesCancelled:withEvent:"), Fn: touchesCancelled},
	}
	common = append(common, accessMethods()...)
	common = append(common, gestureMethods()...)
	common = append(common, navMethods()...)
	registerGestures()
	registerNav()
	registerSelection()
	classView = register("NsgoView", uiView, nil, common)
	classSurface = register("NsgoSurface", uiView, nil, append(common,
		objc.MethodDef{Cmd: sel("layoutSubviews"), Fn: surfaceLayout},
		objc.MethodDef{Cmd: selApply, Fn: surfaceApply},
		objc.MethodDef{Cmd: selTick, Fn: surfaceTick},
		objc.MethodDef{Cmd: sel("safeAreaInsetsDidChange"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			app.refresh()
		}},
		objc.MethodDef{Cmd: sel("traitCollectionDidChange:"), Fn: func(self objc.ID, cmd objc.SEL, previous objc.ID) {
			apple.Super(self, cmd, previous)
			app.refresh()
		}},
	))
	classController = register("NsgoViewController", objc.GetClass("UIViewController"), nil, []objc.MethodDef{
		{Cmd: sel("preferredStatusBarStyle"), Fn: func(objc.ID, objc.SEL) int { return app.statusBar }},
		{Cmd: sel("prefersStatusBarHidden"), Fn: func(objc.ID, objc.SEL) bool { return app.hideBar }},
		{Cmd: sel("preferredStatusBarUpdateAnimation"), Fn: func(objc.ID, objc.SEL) int { return app.barAnimation() }},
		{Cmd: sel("supportedInterfaceOrientations"), Fn: func(self objc.ID, cmd objc.SEL) uint {
			if m := app.orientations(); m != 0 {
				return m
			}
			return apple.SendSuper[uint](self, cmd)
		}},
		{Cmd: sel("canBecomeFirstResponder"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("viewDidAppear:"), Fn: func(self objc.ID, cmd objc.SEL, animated bool) {
			apple.Super(self, cmd, animated)
			self.Send(selBecomeFirst)
		}},
		{Cmd: sel("pressesBegan:withEvent:"), Fn: pressesBegan},
		{Cmd: sel("copy:"), Fn: controllerCopy},
		{Cmd: sel("selectAll:"), Fn: controllerSelectAll},
		{Cmd: sel("canPerformAction:withSender:"), Fn: controllerCanPerform},
	})
	// The watcher sees every touch going down in the surface, before its
	// views do, and lets it through.
	classWatcher = register("NsgoTouchWatcher", objc.GetClass("UIGestureRecognizer"), nil, []objc.MethodDef{
		{Cmd: sel("touchesBegan:withEvent:"), Fn: watcherBegan},
	})
	classInput = register("NsgoInput", uiView, []*objc.Protocol{objc.GetProtocol("UITextFieldDelegate"),
		objc.GetProtocol("UITextViewDelegate")}, append(common, inputMethods()...))
	registerInput()
	classScroll = register("NsgoScrollView", objc.GetClass("UIScrollView"),
		[]*objc.Protocol{objc.GetProtocol("UIScrollViewDelegate")},
		append(append(scrollMethods(), behaviorMethods()...), objc.MethodDef{Cmd: sel("scrollViewDidScroll:"), Fn: scrolled}))
	register("NsgoAppDelegate", objc.GetClass("UIResponder"),
		[]*objc.Protocol{objc.GetProtocol("UIApplicationDelegate")},
		[]objc.MethodDef{{Cmd: sel("application:didFinishLaunchingWithOptions:"),
			Fn: func(self objc.ID, _ objc.SEL, application, options objc.ID) bool { return true }}})
	// The SDK requires the scene life cycle; Info.plist names this class.
	register("NsgoSceneDelegate", objc.GetClass("UIResponder"),
		[]*objc.Protocol{objc.GetProtocol("UIWindowSceneDelegate")},
		[]objc.MethodDef{{Cmd: sel("scene:willConnectToSession:options:"), Fn: sceneWillConnect}})
}

func register(name string, super objc.Class, protocols []*objc.Protocol, methods []objc.MethodDef) objc.Class {
	return apple.RegisterClass(name, super, protocols, methods)
}

// Options configure Run.
type Options struct {
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
	// Components are the native components Native elements name.
	Components core.NativeComponents
}

// Run shows app full screen and runs UIKit's event loop. Call it from
// main; it does not return.
func Run(root engine.App, opts Options) {
	app = &surface{app: root, id: opts.ID, epoch: time.Now(), motions: map[*view]*motion{},
		components: opts.Components, unknown: map[string]bool{}, accessories: map[core.NodeID]*view{}}
	app.animator = core.NewAnimator(func(e core.Event) { app.engine.Dispatch(e) })
	loadUIKit()
	argv := []*byte{&[]byte("nsgo\x00")[0], nil}
	uiApplicationMain(1, unsafe.Pointer(&argv[0]), 0, apple.CFString("NsgoAppDelegate"))
}

// window keeps the key window alive; controller is its root view
// controller.
var window, controller objc.ID

func sceneWillConnect(self objc.ID, _ objc.SEL, scene, session, options objc.ID) {
	window = objc.ID(objc.GetClass("UIWindow")).Send(selAlloc).Send(sel("initWithWindowScene:"), scene)
	bounds := objc.Send[apple.CGRect](window, selBounds)
	controller = objc.ID(classController).Send(selNew)
	s := app
	s.native = objc.ID(classSurface).Send(selAlloc).Send(selInitWithFrame, bounds)
	views[s.native] = &view{surface: s}
	controller.Send(sel("setView:"), s.native)
	window.Send(sel("setRootViewController:"), controller)
	window.Send(sel("makeKeyAndVisible"))
	watcher := objc.ID(classWatcher).Send(selAlloc).Send(sel("initWithTarget:action:"), objc.ID(0), objc.SEL(0))
	watcher.Send(sel("setCancelsTouchesInView:"), false)
	watcher.Send(sel("setDelaysTouchesEnded:"), false)
	s.native.Send(sel("addGestureRecognizer:"), watcher)
	watcher.Send(selRelease)
	s.headers = headerHeights()
	s.env = s.environment()
	opts := engine.Options{
		Size:        core.Size{Width: core.Points(bounds.Size.Width), Height: core.Points(bounds.Size.Height)},
		Environment: s.env,
	}
	opts.SetDirs(engine.AppDirs(s.id))
	s.engine = engine.Start(s, s.app, opts)
	s.observe()
}

// environment returns what UIKit tells about where the app runs.
func (s *surface) environment() core.Environment {
	in := objc.Send[edgeInsets](s.native, sel("safeAreaInsets"))
	traits := s.native.Send(sel("traitCollection"))
	scale := objc.Send[float64](traits, sel("displayScale"))
	if scale <= 0 {
		scale = objc.Send[float64](objc.ID(objc.GetClass("UIScreen")).Send(sel("mainScreen")), sel("scale"))
	}
	// The body text style is 17 points at the default text size.
	body := objc.ID(objc.GetClass("UIFont")).Send(sel("preferredFontForTextStyle:"),
		apple.Global(uikitPath, "UIFontTextStyleBody"))
	idiom := core.IdiomPhone
	const pad, mac = 1, 5
	switch objc.Send[int](objc.ID(objc.GetClass("UIDevice")).Send(sel("currentDevice")), sel("userInterfaceIdiom")) {
	case pad:
		idiom = core.IdiomTablet
	case mac:
		idiom = core.IdiomDesktop
	}
	const dark = 2
	locale, rtl := apple.Locale()
	env := core.Environment{
		SafeArea: core.Edges{Top: core.Points(in.Top), Right: core.Points(in.Right),
			Bottom: core.Points(in.Bottom), Left: core.Points(in.Left)},
		KeyboardHeight:   s.keyboard,
		KeyboardDuration: s.keyboardDuration,
		Dark:             objc.Send[int](traits, sel("userInterfaceStyle")) == dark,
		FontScale:        float32(objc.Send[float64](body, sel("pointSize")) / 17),
		ReduceMotion:     uiReduceMotion(),
		ScreenReader:     uiVoiceOver(),
		RTL:              rtl,
		PixelRatio:       float32(scale),
		Locale:           locale,
		OS:               core.OSIOS,
		Idiom:            idiom,
	}
	env.HeaderHeight, env.LargeHeaderHeight = s.headers[0], s.headers[1]
	return env
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

// observe watches the notifications parts of the environment change with.
func (s *surface) observe() {
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(sel("defaultCenter"))
	refresh := func(objc.ID) { s.refresh() }
	for _, name := range []string{"UIContentSizeCategoryDidChangeNotification",
		"UIAccessibilityReduceMotionStatusDidChangeNotification", "UIAccessibilityVoiceOverStatusDidChangeNotification"} {
		apple.Observe(center, apple.Global(uikitPath, name), refresh)
	}
	apple.Observe(center, apple.Global("/System/Library/Frameworks/Foundation.framework/Foundation",
		"NSCurrentLocaleDidChangeNotification"), refresh)
	apple.Observe(center, apple.Global(uikitPath, "UIKeyboardWillChangeFrameNotification"),
		func(note objc.ID) { s.keyboardMoved(note, false) })
	apple.Observe(center, apple.Global(uikitPath, "UIKeyboardWillHideNotification"),
		func(note objc.ID) { s.keyboardMoved(note, true) })
}

// keyboardMoved follows the software keyboard: how much of the surface's
// bottom it will cover, and how long it takes to get there.
func (s *surface) keyboardMoved(note objc.ID, hiding bool) {
	info := note.Send(sel("userInfo"))
	get := func(key string) objc.ID { return info.Send(sel("objectForKey:"), apple.Global(uikitPath, key)) }
	seconds := objc.Send[float64](get("UIKeyboardAnimationDurationUserInfoKey"), sel("doubleValue"))
	s.keyboardDuration = time.Duration(seconds * float64(time.Second))
	s.keyboard = 0
	if frame := get("UIKeyboardFrameEndUserInfoKey"); !hiding && frame != 0 {
		end := objc.Send[apple.CGRect](frame, sel("CGRectValue"))
		space := window.Send(sel("screen")).Send(sel("coordinateSpace"))
		local := objc.Send[apple.CGRect](s.native, sel("convertRect:fromCoordinateSpace:"), end, space)
		h := objc.Send[apple.CGRect](s.native, selBounds).Size.Height - local.Origin.Y
		s.keyboard = core.Points(max(h, 0))
	}
	s.refresh()
}

// alert presents a over whatever the window shows; the action chosen goes
// to the engine as EventAlert.
func (s *surface) alert(a *core.AlertConfig) {
	const styleAlert = 1
	ctrl := objc.ID(objc.GetClass("UIAlertController")).Send(sel("alertControllerWithTitle:message:preferredStyle:"),
		apple.NSString(a.Title), apple.NSString(a.Message), styleAlert)
	id := a.ID
	add := func(label string, style, index int) {
		handler := objc.NewBlock(func(_ objc.Block, _ objc.ID) {
			s.engine.Dispatch(core.Event{Kind: core.EventAlert, Count: id, Index: index})
		})
		action := objc.ID(objc.GetClass("UIAlertAction")).Send(sel("actionWithTitle:style:handler:"),
			apple.NSString(label), style, handler)
		handler.Release()
		ctrl.Send(sel("addAction:"), action)
	}
	const actionDefault, actionCancel, actionDestructive = 0, 1, 2
	cancel := false
	for i, act := range a.Actions {
		style := actionDefault
		switch act.Role {
		case core.AlertCancel:
			// UIKit allows one cancel action.
			if !cancel {
				style, cancel = actionCancel, true
			}
		case core.AlertDestructive:
			style = actionDestructive
		}
		add(act.Label, style, i)
	}
	if len(a.Actions) == 0 {
		add("OK", actionCancel, -1)
	}
	top := window.Send(sel("rootViewController"))
	for p := top.Send(sel("presentedViewController")); p != 0; p = top.Send(sel("presentedViewController")) {
		top = p
	}
	top.Send(sel("presentViewController:animated:completion:"), ctrl, true, objc.ID(0))
}

// pressOutside reports a touch going down to each view listening for
// presses outside it that holds neither the view touched nor the point.
func (s *surface) pressOutside(touch objc.ID) {
	hit := touch.Send(sel("view"))
	var at core.Point
	found := false
	for _, v := range s.views {
		if v == nil || !v.events.Has(core.EventPressOutside) || v.native.Send(sel("window")) == 0 {
			continue
		}
		if !found {
			p := objc.Send[apple.CGPoint](touch, selLocationInView, s.native)
			at, found = core.Point{X: core.Points(p.X), Y: core.Points(p.Y)}, true
		}
		p := objc.Send[apple.CGPoint](touch, selLocationInView, v.native)
		b := objc.Send[apple.CGRect](v.native, selBounds)
		inside := p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height
		if inside || (hit != 0 && objc.Send[bool](hit, sel("isDescendantOfView:"), v.native)) {
			continue
		}
		s.engine.Dispatch(core.Event{Kind: core.EventPressOutside, ID: v.id, Point: at})
	}
}

func watcherBegan(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	apple.Super(self, cmd, touches, event)
	if app.engine != nil {
		apple.ForEach(touches, app.pressOutside)
		apple.ForEach(touches, app.clearTouched)
	}
	// Failing at once keeps the watcher out of every other gesture.
	const failed = 5
	self.Send(sel("setState:"), failed)
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
	var lb *layoutBatch
	if b.LayoutAnimation != nil && !s.env.ReduceMotion {
		lb = &layoutBatch{anim: b.LayoutAnimation, created: map[core.NodeID]bool{}, deleted: map[objc.ID]bool{}}
	}
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			for len(s.views) <= int(it.ID) {
				s.views = append(s.views, nil)
			}
			s.views[it.ID] = s.create(it.View, it.ID)
			if lb != nil {
				lb.created[it.ID] = true
			}
			continue
		case core.ItemAnimation:
			s.animator.Apply(it.Animation)
			continue
		case core.ItemAlert:
			s.alert(it.Alert)
			continue
		case core.ItemDismissKeyboard:
			s.native.Send(sel("endEditing:"), true)
			continue
		case core.ItemSetClipboard:
			objc.ID(objc.GetClass("UIPasteboard")).Send(sel("generalPasteboard")).Send(sel("setString:"), apple.NSString(it.Value))
			continue
		case core.ItemReadClipboard:
			text := apple.GoString(objc.ID(objc.GetClass("UIPasteboard")).Send(sel("generalPasteboard")).Send(sel("string")))
			s.engine.Dispatch(core.Event{Kind: core.EventClipboard, Count: it.Count, Text: text})
			continue
		case core.ItemImageInfo:
			apple.ImageInfo(it, s.engine.Dispatch)
			continue
		case core.ItemAnnounce:
			uiAccessibilityPost(announcement, apple.NSString(it.Value))
			continue
		case core.ItemKeymap:
			s.keys = map[string]bool{}
			for _, k := range it.Keys {
				s.keys[k] = true
			}
			continue
		case core.ItemStatusBar:
			s.setStatusBar(it.StatusBar)
			continue
		case core.ItemTitle, core.ItemBackUnhandled, core.ItemWindow, core.ItemMenu:
			continue
		}
		v := s.views[it.ID]
		switch it.Kind {
		case core.ItemDelete:
			s.delete(v, s.vanish(v, lb))
		case core.ItemRemove:
			if v.sheet != nil {
				s.dismissSheet(v)
				v.presented = false
				s.updateOrientations()
			}
			v.native.Send(selRemoveFromSuperview)
		case core.ItemInsert:
			if v.kind == core.KindModal && v.isSheet() {
				// A sheet holds the modal's view, wherever the core puts it.
				if v.sheet == nil {
					v.presented = true
					s.presentSheet(v)
					s.updateOrientations()
				}
				break
			}
			parent := s.views[it.Parent].children()
			if it.Before == 0 {
				parent.Send(selAddSubview, v.native)
			} else {
				parent.Send(selInsertBelow, v.native, s.views[it.Before].native)
			}
			if v.kind == core.KindModal && !v.presented {
				v.presented = true
				s.presenting = append(s.presenting, v)
				s.updateOrientations()
			}
			if p := s.views[it.Parent]; p.bar != nil {
				// The header stays over the screen's content.
				parent.Send(sel("bringSubviewToFront:"), p.bar.bar)
			}
			s.appear(v, lb, it.Parent)
		case core.ItemFrame:
			s.setFrame(v, it.Frame, lb)
			if v.bar != nil {
				s.layoutBar(v)
			}
			if v.headerWrap != 0 {
				s.sizeHeaderWrap(v)
			}
		case core.ItemScreen:
			s.configureScreen(v, it.Screen)
		case core.ItemViewStyle:
			v.style = *it.ViewStyle
			if v.animatesStyle() {
				v.restyle()
			} else {
				apple.StyleLayer(v.native.Send(selLayer), it.ViewStyle, false)
			}
			v.native.Send(selSetHidden, it.ViewStyle.Hidden)
			if v.kind == core.KindScroll || v.kind == core.KindVideo {
				// A scroll view shows only what it scrolls to, and a video
				// only its frame.
				v.native.Send(sel("setClipsToBounds:"), true)
			}
			if v.image != nil {
				v.image.SetStyle(it.ViewStyle)
			}
		case core.ItemImage:
			v.image.Configure(it.Image)
			s.images = append(s.images, v)
		case core.ItemModal:
			v.modal = *it.Modal
			if v.modal.Orientations != 0 {
				s.oriented = append(s.oriented, v)
			}
		case core.ItemWeb:
			v.web.Configure(it.Web)
		case core.ItemWebCommand:
			v.web.Command(it.WebCommand, it.Value)
		case core.ItemVideo:
			v.video.Configure(it.Video)
		case core.ItemVideoSeek:
			v.video.Seek(it.Time)
		case core.ItemNative:
			s.configureNative(v, it.Native)
		case core.ItemNativeCommand:
			if v.component != nil {
				v.component.Command(it.Value, it.Data)
			}
		case core.ItemCanvas:
			v.drawing = it.Drawing
			v.native.Send(selSetNeedsDisplay)
		case core.ItemScrollTo:
			size := objc.Send[apple.CGSize](v.native, sel("contentSize"))
			in := objc.Send[edgeInsets](v.native, sel("adjustedContentInset"))
			seen := objc.Send[apple.CGRect](v.native, selBounds).Size
			to := apple.CGPoint{
				X: min(max(float64(it.Point.X), -in.Left), max(size.Width-seen.Width+in.Right, -in.Left)),
				Y: min(max(float64(it.Point.Y), -in.Top), max(size.Height-seen.Height+in.Bottom, -in.Top)),
			}
			if v.sc != nil {
				v.sc.applied++
			}
			v.native.Send(sel("setContentOffset:animated:"), to, it.Animated)
		case core.ItemScrollBy:
			if v.sc != nil {
				v.sc.applied++
				v.scrollBy(it.Point)
			}
		case core.ItemStickyCells:
			if v.sc != nil {
				v.setSticky(it.Sticky)
			}
		case core.ItemViewProps:
			v.setProps(it.ViewProps)
			p := it.ViewProps
			v.activation, v.area, v.selectable = p.PanActivation, p.SelectionArea, p.Selectable
			if v.selColor != p.SelectionColor {
				v.selColor = p.SelectionColor
				v.native.Send(selSetNeedsDisplay)
			}
			v.setSelectable()
		case core.ItemEvents:
			v.events = it.Events
			if v.input != nil {
				v.input.fs.Events = it.Events
			}
			v.setGestures(it.Events)
			v.updateAccessibility()
		case core.ItemAccessibilityLabel:
			v.setLabel(it.Value)
		case core.ItemAccessibilityFocus:
			v.focusAccessibility()
		case core.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.box = it.Text.Content
			v.textChanged(it.Text.Text)
			v.native.Send(selSetNeedsDisplay)
		case core.ItemInput:
			v.configure(it.Input)
			s.setAccessory(v, it.Input.Accessory)
		case core.ItemInputValue:
			v.input.value(it.Value, it.Count)
		case core.ItemSelection:
			if v.input != nil {
				v.input.setSelection(it.Selection)
			}
		case core.ItemScrollConfig:
			if v.sc != nil {
				v.configureScroll(it.Scroll)
			}
		case core.ItemScrollContent:
			if v.sc != nil {
				v.setContent(it.Size)
			}
		case core.ItemZoomTo:
			v.zoomTo(it.Zoom, it.Animated)
		case core.ItemFocus:
			if v.input != nil {
				v.input.field.Send(selBecomeFirst)
			}
		case core.ItemBlur:
			if v.input != nil {
				v.input.field.Send(selResign)
			}
		}
	}
	if root := s.views[b.Root]; root.native.Send(selSuperview) != s.native {
		s.native.Send(selAddSubview, root.native)
	}
	s.finish()
	s.refreshSelection()
	apple.WithoutAnimation(func() {
		if lb != nil {
			s.stepMotions(s.clock())
		}
		s.applyAnimated()
	})
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
			apple.AnimateIn(v.native.Send(selLayer), v.modal.Animation, h, s.env.ReduceMotion)
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

func (s *surface) create(kind core.ViewKind, id core.NodeID) *view {
	v := &view{surface: s, id: id, kind: kind}
	switch kind {
	case core.KindScroll:
		v.native = objc.ID(classScroll).Send(selNew)
		newScroll(v)
	case core.KindInput:
		v.newInput()
	default:
		v.native = objc.ID(classView).Send(selNew)
		if kind == core.KindText || kind == core.KindCanvas {
			const redraw = 3
			v.native.Send(selSetContentMode, redraw)
		}
	}
	v.native.Send(selSetOpaque, false)
	switch kind {
	case core.KindImage:
		v.image = apple.NewImageView(v.native.Send(selLayer))
		v.image.Dispatch = v.report
	case core.KindWeb:
		v.web = apple.NewWeb(v.native)
		v.web.Dispatch = v.report
	case core.KindVideo:
		v.video = apple.NewVideo(v.native)
		v.video.Dispatch = v.report
		v.video.Parent = controller
		v.native.Send(sel("setClipsToBounds:"), true)
	}
	views[v.native] = v
	return v
}

// delete releases v; kept leaves its native where it is, going away.
func (s *surface) delete(v *view, kept bool) {
	if v.sheet != nil {
		s.dismissSheet(v)
		defer s.updateOrientations()
	}
	if b := v.bar; b != nil {
		delete(bars, b.bar)
		b.bar.Send(selRelease)
		b.item.Send(selRelease)
		b.back.Send(selRelease)
	}
	if v.headerWrap != 0 {
		v.headerWrap.Send(selRemoveFromSuperview)
		v.headerWrap.Send(selRelease)
	}
	if !kept && !s.inGhost(v.native) {
		v.native.Send(selRemoveFromSuperview)
	}
	s.animator.Forget(v.id)
	delete(s.motions, v)
	if v.component != nil {
		v.component.Close()
		v.component = nil
	}
	if in := s.accessories[v.id]; in != nil {
		s.setAccessory(in, 0)
	}
	if v.accessory != 0 {
		s.setAccessory(v, 0)
	}
	apple.ForgetBorders(v.native.Send(selLayer))
	delete(gates, v.pan)
	v.layout.Release()
	if v.image != nil {
		v.image.Close()
	}
	if v.web != nil {
		v.web.Close()
	}
	if v.video != nil {
		v.video.Close()
	}
	delete(views, v.native)
	v.native.Send(selRelease)
	s.views[v.id] = nil
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

func (v *view) pressable() bool {
	return v.events.Has(core.EventPress) || v.events.Has(core.EventLongPress) ||
		v.events.Has(core.EventPressDown) || v.events.Has(core.EventPressUp)
}

func drawRect(self objc.ID, _ objc.SEL, dirty apple.CGRect) {
	v := views[self]
	if v != nil && v.drawing != nil {
		apple.Play(uiGraphicsGetCurrentContext(), v.drawing)
		return
	}
	if v == nil || v.layout == nil {
		return
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	box := v.box
	v.layout.DrawBackgrounds(uiGraphicsGetCurrentContext(), apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
	v.drawSelection(uiGraphicsGetCurrentContext())
	v.layout.Draw(uiGraphicsGetCurrentContext(), b.Size.Height, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
		float64(box.Size.Width), float64(box.Size.Height)))
}

// Touches a view does not handle go up the responder chain to a pressable
// ancestor.
func touchesBegan(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	// A secondary click of a pointer opens a context menu.
	const secondary = 1 << 1
	if v != nil && v.events.Has(core.EventContextMenu) && objc.Send[uint](event, sel("buttonMask"))&secondary != 0 {
		p := objc.Send[apple.CGPoint](touches.Send(selAnyObject), selLocationInView, self)
		v.report(core.Event{Kind: core.EventContextMenu, Point: core.Point{X: core.Points(p.X), Y: core.Points(p.Y)}})
		return
	}
	if v == nil || !v.pressable() {
		apple.Super(self, cmd, touches, event)
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

func touchesCancelled(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		apple.Super(self, cmd, touches, event)
		return
	}
	if v.pressing.Swap(false) {
		v.dispatch(core.EventPressUp)
	}
}

func touchesEnded(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		apple.Super(self, cmd, touches, event)
		return
	}
	p := objc.Send[apple.CGPoint](touches.Send(selAnyObject), selLocationInView, self)
	b := objc.Send[apple.CGRect](self, selBounds)
	pressed := v.pressing.Swap(false)
	v.dispatch(core.EventPressUp)
	if pressed && !v.longFired.Load() && p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height {
		index := -1
		if v.layout != nil {
			box := v.box
			index = v.layout.IndexAt(p, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
				float64(box.Size.Width), float64(box.Size.Height)))
		}
		v.report(core.Event{Kind: core.EventPress, Index: index})
	}
}

// UIKeyboardHIDUsage codes of the keys core.NormalizeKey names.
var keyNames = map[int]string{
	40: "enter", 88: "enter", 41: "escape", 43: "tab", 44: "space", 42: "backspace", 76: "delete",
	82: "up", 81: "down", 80: "left", 79: "right", 74: "home", 77: "end", 75: "pageup", 78: "pagedown",
	58: "f1", 59: "f2", 60: "f3", 61: "f4", 62: "f5", 63: "f6", 64: "f7", 65: "f8", 66: "f9",
	67: "f10", 68: "f11", 69: "f12",
}

// pressesBegan takes hardware key presses the keymap binds, as EventKey,
// and passes the rest on.
func pressesBegan(self objc.ID, cmd objc.SEL, presses, event objc.ID) {
	binding := ""
	if len(app.keys) > 0 {
		apple.ForEach(presses, func(press objc.ID) {
			key := press.Send(sel("key"))
			if key == 0 || binding != "" {
				return
			}
			name, ok := keyNames[objc.Send[int](key, sel("keyCode"))]
			if !ok {
				name = strings.ToLower(apple.GoString(key.Send(sel("charactersIgnoringModifiers"))))
			}
			for _, k := range apple.KeyNames(name, objc.Send[uint](key, sel("modifierFlags"))) {
				if name != "" && app.keys[k] {
					binding = k
					return
				}
			}
		})
	}
	if binding == "" {
		apple.Super(self, cmd, presses, event)
		return
	}
	app.engine.Dispatch(core.Event{Kind: core.EventKey, Text: binding})
}

func surfaceLayout(self objc.ID, cmd objc.SEL) {
	apple.Super(self, cmd)
	if v := views[self]; v != nil && v.surface.engine != nil {
		b := objc.Send[apple.CGRect](self, selBounds)
		v.surface.engine.Resize(core.Size{Width: core.Points(b.Size.Width), Height: core.Points(b.Size.Height)})
	}
}

func surfaceApply(self objc.ID, _ objc.SEL, _ objc.ID) {
	s := views[self].surface
	for _, b := range s.batches.Drain() {
		s.apply(b)
	}
}

func scrolled(self objc.ID, _ objc.SEL, _ objc.ID) {
	v := views[self]
	if v == nil {
		return
	}
	o := objc.Send[apple.CGPoint](self, selContentOffset)
	offset := core.Point{X: core.Points(o.X), Y: core.Points(o.Y)}
	if v.surface.animator.Scroll(v.id, offset) {
		apple.WithoutAnimation(v.surface.applyAnimated)
	}
	if v.sc != nil && len(v.sc.sticky) > 0 {
		v.pinSticky()
	}
	v.scrollEvent(core.EventScroll)
}
