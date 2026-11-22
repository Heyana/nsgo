//go:build ios

// Package ios is NSGo's UIKit backend: one UIView per node, styled through
// its CALayer, with text the engine laid out with CoreText drawn on the
// main thread and images decoded off it. Alerts are UIAlertControllers.
// It calls UIKit through purego; Go itself needs cgo to link for iOS.
package ios

import (
	"runtime"
	"slices"
	"strings"
	"structs"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/queue"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/internal/testhost"
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
	id      driver.NodeID
	kind    driver.ViewKind
	native  objc.ID
	events  driver.EventMask
	// contentLayer draws a text's or a canvas's content.
	contentLayer objc.ID
	// laidFrame is the frame contentLayer was last given, and needFrame
	// the frame its content last needed (contentFrame), which a text's
	// may be smaller than.
	laidFrame, needFrame apple.CGRect
	// rasterStep is how many times its pixels the content inside v draws
	// at, as v's scale has it (rasterscale.go); 0 is 1.
	rasterStep float64
	// axNodes are a canvas's named parts, its accessibility elements.
	axNodes []objc.ID
	// backdrop is the view's backdrop: a UIVisualEffectView for backdropMaterial
	// or backdropGlass, or a holder UIView of backdropLayer (a CABackdropLayer)
	// and tint for backdropFiltered.
	backdrop, backdropLayer, tint objc.ID
	backdropKind                  backdropMode

	pressing  atomic.Bool
	longFired atomic.Bool
	pressAt   driver.Event  // where the press began, and how
	overflow  driver.Bounds // where its children reach outside it (ItemOverflow)

	// transformedKids is ItemOverflow's Transformed: a child may be drawn transformed, outside overflow.
	transformedKids bool

	layout *apple.Layout // a text view's
	box    driver.Bounds

	sc    *scrollState // a scroll view's
	input *input       // an input's editor
	inset driver.Bounds

	image     *apple.ImageView
	modal     driver.ModalConfig // a modal's
	presented bool               // whether a modal was inserted yet
	// focusBefore is what had the focus as a modal came (focus.go).
	focusBefore *view
	sheet       *sheet  // a modal presented as a sheet
	bar         *navBar // a screen's system header
	// headerWrap holds a header view in its screen's bar.
	headerWrap objc.ID
	drawing    *driver.Drawing    // a canvas's
	played     []driver.DrawOp    // the ops it last drew, its bound arguments' values in place
	shader     *apple.ShaderLayer // a shader view's (shader.go)
	gpu        *apple.GPULayer    // a GPU view's (gpu.go)
	container  objc.ID            // what holds its children under an effect or for its clip, once it has one
	clipHeld   bool               // whether container clips the children in place of the view's own layer, which keeps its shadow and outline (clipsInHolder)
	web        *apple.Web         // a web view's
	pip        *apple.PictureInPicture
	video      *apple.Video // a video's

	activation           driver.PanActivation
	handoff              bool    // its pan hands off from the scroll views inside it
	pan, pinch, rotation objc.ID // the recognizers of the gestures it follows
	hover                objc.ID // its UIHoverGestureRecognizer while it follows hovers
	// scrolls and notches are its scroll-only pans while it takes the
	// wheel: a trackpad's two fingers and a mouse wheel's turns.
	scrolls, notches objc.ID

	// A text's content, spans and selection, whether a view is an area,
	// and the long press that selects in it.
	plain            string
	spans            []driver.TextSpan
	selectable       driver.Selectable
	selColor         driver.Color
	selStart, selEnd int // UTF-16
	area             bool
	longPress        objc.ID
	selTap           objc.ID        // an area's tap (selection.go)
	tsel             *textSelection // a selectable text's own selection (textinput.go)

	// props are the view's last props; the rest what access.go last
	// gave UIKit.
	props               driver.ViewProps
	propsSet, textSet   bool
	axElement, axCustom bool

	// frame is where the view shows now; style its own style, which its
	// animated values and a layout animation draw over.
	frame    driver.Bounds
	framed   bool
	shift    driver.Point // a sticky cell's, from frame
	style    driver.ViewStyle
	animated driver.AnimatedValues
	// drawn is the style its layer was last drawn with, once styled, and
	// the default style before. born is the batch that created it, whose
	// style shows at once.
	drawn  driver.ViewStyle
	styled bool
	born   uint64

	component driver.NativeComponent // a native view's
	// componentView is the component's view, which fills the box: views
	// such as UITabBar size themselves, so it is placed at each frame
	// rather than autoresized.
	componentView  objc.ID
	componentTried bool
	accessory      driver.NodeID // an input's InputAccessoryView
}

type surface struct {
	// gpuLayers are the GPU views' layers by view, which the engine goroutine submits frames to.
	gpuLayers sync.Map
	sensors   apple.Sensors
	discovery apple.Discovery

	batch uint64 // batches applied
	// hinge is the latest a UIHingeInteraction reported (watchHinge).
	hinge hingeReading
	// regionClock follows the sizes the reserved regions are read at (geometry).
	regionClock regionClock
	// perf measures the surface while its panel, hud, shows; perfOn says
	// the app asked for them.
	perf   *engine.PerfMonitor
	hud    *hud
	perfOn bool
	// embedded is set for a host's NSGoView (embed.go): the host owns the
	// window and its view controllers. host is what it gives the app, and
	// embed the settings its view holds; stops end the surface's
	// observations as it closes.
	embedded     bool
	host         engine.HostOptions
	embed        *embedState
	stops        []func()
	closed       bool
	gestureEdges driver.SystemGestureEdge
	// A standalone app's scene (sceneWillConnect): window is the UIWindow it shows in, 0 while its scene is disconnected, and controller its root view controller. session is the persistent identifier of the scene session it shows, "" once the system discarded it. scene numbers a scene beyond the first for the tree the engine opens it (EventOpenScene), and tree is that tree's ID once attached; ending says the engine destroyed the tree.
	window, controller objc.ID
	session            string
	scene, tree        uint32
	discarded, ending  bool
	// alerts are the alerts shown, by ID, kept for a test host or nsgo dev's
	// automation to answer them.
	alerts map[uint32]shownAlert
	// opening is the document interaction showing the apps that open a
	// file, kept while it shows.
	opening objc.ID
	// appearance is the one the app forces, or the system's.
	appearance driver.Appearance
	// rtl is the app's direction (driver.ItemDirection), which its
	// navigation slides and swipes back in and its UIKit views lay out in
	// once directed.
	rtl, directed bool
	// stickyBound are the scroll views with sticky cells whose insets
	// follow animated values.
	stickyBound map[*view]bool
	app         engine.App
	id          string
	native      objc.ID
	engine      *engine.Engine
	audio       *apple.Audio // the engine's sounds and players, made with its first ItemAudio and closed with the surface
	batches     queue.Batches
	views       []*view // index is NodeID
	pips        []*view // the views of mobile.PictureInPicture trees, which draw into their sessions' frames
	fingers     fingers // the touches reported
	env         driver.Environment

	// resizedScrolls are checked after the current batch or animation step.
	resizedScrolls []*view

	keyboard driver.Points
	// keyboardScroll is the scroll view keeping the focused input clear
	// of the keyboard (keyboard.go).
	keyboardScroll   *view
	keyboardDuration time.Duration
	// keyboardStart is when the keyboard last started to move, on clock.
	keyboardStart time.Duration

	keys []driver.KeyBinding // the keymap's bindings
	// keysHeld are the hardware keys whose down was reported, by UIKeyboardHIDUsage: the view the down went to, 0 for none, and the name it reported, which the key's up reports wherever the focus has gone since.
	keysHeld map[int]heldKey
	heldKeys bool // ItemKeymap's HeldKeys: report every hardware key going down and up, not only a focused view's
	// commandMenuKeys are the commands UIKit's main menu was last asked to build.
	commandMenuKeys []driver.KeyBinding
	// held is the ID of the binding a held key fires again, on a timer of its own: UIKit reports a held key's press once.
	held     uint32
	heldStop chan struct{}
	// menuHeld: the remote's Menu button is down and went back.
	menuHeld    bool
	statusBar   int // the UIStatusBarStyle the controller asks for
	hideBar     bool
	barAnimated bool
	// headers are the navigation bar's heights; oriented the modals
	// that limit the orientations, in the order they were presented.
	headers  [2]driver.Points
	oriented []*view
	// The active scope's orientation set and UIKit's orientation before it.
	requestedOrientation driver.Orientation
	previousOrientation  uint
	// tops is the screen each stack shows, as the core last said, slides
	// the stacks whose screens move, and changes the stacks a batch
	// changes the screens of (nav.go).
	tops    map[*view]*view
	slides  map[*view]*slide
	changes []stackChange
	// history is how many entries back goes through (ItemHistory).
	history int
	// On an Apple TV (focus.go): modals are those shown, oldest first;
	// focused is the view with the focus, focusBack the one to give it
	// back to as a modal goes.
	modals             []*view
	focused, focusBack *view

	// What a batch touched, finished once its frames are set.
	images, presenting []*view
	// shading are the shader views to draw once the batch or the frame
	// is applied.
	shading []*view
	// effects are the views with an effect, a transition or a navigation
	// shader (effect.go); root is the root view's, which screen shaders
	// draw over.
	effects apple.Effects[*view]
	root    driver.NodeID

	selecting *selection

	// The frame clock and what it animates.
	animator *driver.Animator
	epoch    time.Time
	link     objc.ID // the CADisplayLink
	ticking  bool
	// pace is the rate the link is slowed to, 0 for the screen's highest.
	pace float64
	// frameWanted is set while the app waits for a display frame, by the
	// engine goroutine; fullRate while the clock ticks at the display's
	// full rate, which serves it without a hop to the main thread.
	frameWanted atomic.Bool
	fullRate    atomic.Bool
	motions     map[*view]*motion
	ghosts      []*ghost
	components  driver.NativeComponents
	unknown     map[string]bool
	// accessories are the inputs each InputAccessoryView rides above.
	accessories map[driver.NodeID]*view
	// ownTexts are the selectable texts, which select on their own
	// outside areas (textinput.go).
	ownTexts map[driver.NodeID]*view
}

// edgeInsets is UIEdgeInsets.
type edgeInsets struct {
	_                        structs.HostLayout
	Top, Left, Bottom, Right float64
}

// views maps native objects to their Go side. Only the main thread uses it.
var views = map[objc.ID]*view{}

// runner is the surface a standalone app's first scene shows (Run), the engine's own; UIKit makes the scene's delegate, so it cannot be handed one. Embedded surfaces are their hosts' views (NewView).
var runner *surface

// scenes are a standalone app's surfaces, one for each scene session it shows, runner first: the windows of one engine, each further scene a tree of its own (driver.SurfaceScene), as a desktop app's windows are.
var scenes []*surface

var (
	sel = apple.Sel

	selAlloc, selNew                 = sel("alloc"), sel("new")
	selRelease                       = sel("release")
	selLayer                         = sel("layer")
	selSetFrame, selBounds           = sel("setFrame:"), sel("bounds")
	selAddSubview, selSuperview      = sel("addSubview:"), sel("superview")
	selInsertBelow                   = sel("insertSubview:belowSubview:")
	selRemoveFromSuperview           = sel("removeFromSuperview")
	selSetHidden                     = sel("setHidden:")
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
	selSetOpaque                     = sel("setOpaque:")
	selInitWithFrame, selSetDeleg    = sel("initWithFrame:"), sel("setDelegate:")
	selContentOffset                 = sel("contentOffset")
	selSetContentSize                = sel("setContentSize:")

	classView, classSurface, classInput, classScroll, classWatcher, classController, classWindow objc.Class
	// surfaceMethods are the surface's, NsgoSurface's and NSGoView's.
	surfaceMethods []objc.MethodDef

	uiApplicationMain                           func(argc int, argv unsafe.Pointer, principal, delegate uintptr) int
	uiReduceMotion, uiVoiceOver, uiDarkerColors func() bool
	uiAccessibilityPost                         func(notification uint32, argument objc.ID)
	announcement                                uint32 // UIAccessibilityAnnouncementNotification
	traits                                      = map[string]uint64{}
)

const uikitPath = "/System/Library/Frameworks/UIKit.framework/UIKit"

func loadUIKit() {
	uikit, err := purego.Dlopen(uikitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&uiApplicationMain, uikit, "UIApplicationMain")
	purego.RegisterLibFunc(&uiReduceMotion, uikit, "UIAccessibilityIsReduceMotionEnabled")
	purego.RegisterLibFunc(&uiVoiceOver, uikit, "UIAccessibilityIsVoiceOverRunning")
	purego.RegisterLibFunc(&uiDarkerColors, uikit, "UIAccessibilityDarkerSystemColorsEnabled")
	purego.RegisterLibFunc(&uiAccessibilityPost, uikit, "UIAccessibilityPostNotification")
	loadAccessibility(uikit)

	uiView := apple.Class("UIView")
	common := []objc.MethodDef{
		{Cmd: sel("hitTest:withEvent:"), Fn: hitTest},
		{Cmd: sel("touchesBegan:withEvent:"), Fn: touchesBegan},
		{Cmd: sel("touchesEnded:withEvent:"), Fn: touchesEnded},
		{Cmd: sel("touchesCancelled:withEvent:"), Fn: touchesCancelled},
	}
	common = append(common, accessMethods()...)
	common = append(common, focusMethods()...)
	common = append(common, gestureMethods()...)
	common = append(common, navMethods()...)
	registerGestures()
	registerTouches()
	registerNav()
	registerSelection()
	registerContent()
	registerCanvasNodes()
	classView = register("NsgoView", uiView, nil, slices.Concat(common, pressMethods()))
	registerTextInput(slices.Concat(common, pressMethods()))
	surfaceMethods = append(common,
		objc.MethodDef{Cmd: sel("layoutSubviews"), Fn: surfaceLayout},
		objc.MethodDef{Cmd: selApply, Fn: surfaceApply},
		objc.MethodDef{Cmd: selTick, Fn: surfaceTick},
		objc.MethodDef{Cmd: sel("safeAreaInsetsDidChange"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			if v := views[self]; v != nil {
				v.surface.layoutChanged()
			}
		}},
		objc.MethodDef{Cmd: sel("traitCollectionDidChange:"), Fn: func(self objc.ID, cmd objc.SEL, previous objc.ID) {
			apple.Super(self, cmd, previous)
			if v := views[self]; v != nil {
				v.surface.layoutChanged()
			}
		}},
	)
	classSurface = register("NsgoSurface", uiView, nil, surfaceMethods)
	classWindow = register("NsgoWindow", apple.Class("UIWindow"), nil, []objc.MethodDef{
		{Cmd: sel("nsgoKeyboardWindow:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) { keyboardWindow = self }},
	})
	classController = register("NsgoViewController", apple.Class("UIViewController"), nil, append(keyCommandMethods(), []objc.MethodDef{
		{Cmd: sel("preferredScreenEdgesDeferringSystemGestures"), Fn: func(self objc.ID, _ objc.SEL) uint {
			if s := controllerSurface(self); s != nil && s.controller == self {
				return uint(s.gestureEdges)
			}
			return 0
		}},
		{Cmd: sel("preferredStatusBarStyle"), Fn: func(self objc.ID, _ objc.SEL) int { return controllerSurface(self).statusBar }},
		{Cmd: sel("prefersStatusBarHidden"), Fn: func(self objc.ID, _ objc.SEL) bool { return controllerSurface(self).hideBar }},
		{Cmd: sel("preferredStatusBarUpdateAnimation"), Fn: func(self objc.ID, _ objc.SEL) int { return controllerSurface(self).barAnimation() }},
		{Cmd: sel("supportedInterfaceOrientations"), Fn: func(self objc.ID, cmd objc.SEL) uint {
			if m := controllerSurface(self).orientations(); m != 0 {
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
		{Cmd: sel("pressesEnded:withEvent:"), Fn: pressesEnded},
		{Cmd: sel("pressesCancelled:withEvent:"), Fn: pressesEnded},
		{Cmd: sel("preferredFocusEnvironments"), Fn: controllerPreferredFocus},
		{Cmd: sel("copy:"), Fn: controllerCopy},
		{Cmd: sel("selectAll:"), Fn: controllerSelectAll},
		{Cmd: sel("canPerformAction:withSender:"), Fn: controllerCanPerform},
	}...))
	// The watcher sees every touch going down in the surface, before its
	// views do, and lets it through.
	classWatcher = register("NsgoTouchWatcher", apple.Class("UIGestureRecognizer"), nil, []objc.MethodDef{
		{Cmd: sel("touchesBegan:withEvent:"), Fn: watcherBegan},
	})
	classInput = register("NsgoInput", uiView, []*objc.Protocol{objc.GetProtocol("UITextFieldDelegate"),
		objc.GetProtocol("UITextViewDelegate")}, append(common, inputMethods()...))
	registerInput()
	classScroll = register("NsgoScrollView", apple.Class("UIScrollView"),
		[]*objc.Protocol{objc.GetProtocol("UIScrollViewDelegate")},
		append(append(scrollMethods(), behaviorMethods()...), objc.MethodDef{Cmd: sel("scrollViewDidScroll:"), Fn: scrolled}))
	register("NsgoAppDelegate", apple.Class("UIResponder"),
		[]*objc.Protocol{objc.GetProtocol("UIApplicationDelegate")},
		[]objc.MethodDef{{Cmd: sel("application:didFinishLaunchingWithOptions:"),
			Fn: func(self objc.ID, _ objc.SEL, application, options objc.ID) bool {
				// Before it returns, so a press that launched the app is
				// heard.
				apple.WatchNotifications()
				return true
			}},
			// iOS ends the app, as the user swipes it away while it runs:
			// its OnQuit handlers run, and its storage is written.
			{Cmd: sel("applicationWillTerminate:"), Fn: func(self objc.ID, _ objc.SEL, application objc.ID) {
				if runner != nil && runner.engine != nil {
					runner.engine.Close()
				}
				engine.SyncStorage()
			}},
			{Cmd: sel("application:didDiscardSceneSessions:"), Fn: func(self objc.ID, _ objc.SEL, application, sessions objc.ID) {
				apple.ForEach(sessions, func(session objc.ID) { discardScene(sessionID(session)) })
			}},
			{Cmd: sel("buildMenuWithBuilder:"), Fn: buildKeyMenus},
		})
	// The SDK requires the scene life cycle; Info.plist names this class.
	register("NsgoSceneDelegate", apple.Class("UIResponder"),
		[]*objc.Protocol{objc.GetProtocol("UIWindowSceneDelegate")},
		[]objc.MethodDef{{Cmd: sel("scene:willConnectToSession:options:"), Fn: sceneWillConnect},
			{Cmd: sel("sceneDidDisconnect:"), Fn: sceneDidDisconnect},
			{Cmd: sel("sceneDidBecomeActive:"), Fn: rebuildKeyMenus},
			{Cmd: sel("scene:continueUserActivity:"), Fn: sceneContinueActivity},
			{Cmd: sel("scene:openURLContexts:"), Fn: sceneOpenURLContexts}})
}

func register(name string, super objc.Class, protocols []*objc.Protocol, methods []objc.MethodDef) objc.Class {
	return apple.RegisterClass(name, super, protocols, methods)
}

// Options configure Run.
type Options struct {
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
	// Components are the native components Native elements name.
	Components driver.NativeComponents
	// Fonts are font files under the app's resources, registered before
	// the first view.
	Fonts []string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
	// Perf shows the performance monitor over the app (perf.go).
	Perf bool
	// Updates is how the app updates (updates.go).
	Updates updates.Config
}

// Run shows app full screen and runs UIKit's event loop. Call it from
// main; it does not return.
func Run(root engine.App, opts Options) {
	updateConfig = opts.Updates
	runner = newSurface(root, opts)
	scenes = []*surface{runner}
	runUIKit(opts)
}

// newSurface is the surface the scene shows root in; a test host's has no
// root until the test opens one.
func newSurface(root engine.App, opts Options) *surface {
	s := &surface{app: root, id: opts.ID, epoch: time.Now(), motions: map[*view]*motion{}, tops: map[*view]*view{}, slides: map[*view]*slide{}, appearance: opts.Appearance,
		components: opts.Components, unknown: map[string]bool{}, accessories: map[driver.NodeID]*view{}, ownTexts: map[driver.NodeID]*view{},
		perfOn: opts.Perf}
	if automationBuild {
		// nsgo dev's automation answers alerts as a test does.
		s.alerts = map[uint32]shownAlert{}
	}
	s.animator = driver.NewAnimator(func(e driver.Event) { s.engine.Dispatch(e) })
	return s
}

// runUIKit runs UIKit's loop, which connects the scene, on this thread.
func runUIKit(opts Options) {
	loadUIKit()
	apple.RegisterFonts(opts.Fonts)
	argv := []*byte{&[]byte("nsgo\x00")[0], nil}
	uiApplicationMain(1, unsafe.Pointer(&argv[0]), 0, apple.CFString("NsgoAppDelegate"))
}

// sceneWillConnect shows a scene UIKit connects in a surface of its own: the first in runner, which starts the engine; one that connects again after its disconnect (sceneDidDisconnect) in the surface it showed, its tree kept; and a further one, as iPadOS connects for each window the user opens, in a new surface whose tree the engine opens to show the main view (openScene). An app that declares no multiple scenes (UIApplicationSupportsMultipleScenes) has runner alone, which moves to a scene that connects while it shows in another.
func sceneWillConnect(self objc.ID, _ objc.SEL, scene, session, options objc.ID) {
	takeConnectionActivities(options)
	takeConnectionLinks(options)
	id := sessionID(session)
	s := sceneSurface(id)
	switch {
	case s != nil:
	case runner.native == 0 || !multipleScenes() || runner.discarded && runner.window == 0:
		s = runner
	default:
		s = newSurface(runner.app, Options{ID: runner.id, Components: runner.components, Appearance: runner.appearance})
		scenes = append(scenes, s)
	}
	s.session, s.discarded = id, false
	if s.native != 0 {
		s.reconnect(scene)
		return
	}
	s.show(scene)
	s.watch()
	s.env = s.environment()
	if s != runner {
		sceneCount++
		s.scene = sceneCount
		if runner.engine != nil {
			runner.engine.Dispatch(driver.Event{Kind: driver.EventOpenScene, Count: s.scene})
		}
		return
	}
	if s.app == nil {
		// A test host's: the test opens its app.
		close(sceneReady)
		return
	}
	s.startEngine(nil)
}

// sceneCount numbers the scenes beyond the first (surface.scene).
var sceneCount uint32

// sessionID is a UISceneSession's persistent identifier.
func sessionID(session objc.ID) string {
	return apple.GoString(session.Send(sel("persistentIdentifier")))
}

// sceneSurface is the surface that shows the scene session id, nil for none.
func sceneSurface(id string) *surface {
	for _, s := range scenes {
		if s.session == id && id != "" {
			return s
		}
	}
	return nil
}

// multipleScenes reports whether the app declares multiple scenes (UIApplicationSupportsMultipleScenes, apple.multiple_scenes).
func multipleScenes() bool {
	return objc.Send[bool](objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication")), sel("supportsMultipleScenes"))
}

// controllerSurface is the surface whose root view controller is controller, runner for one it does not know.
func controllerSurface(controller objc.ID) *surface {
	for _, s := range scenes {
		if s.controller == controller {
			return s
		}
	}
	return runner
}

// show makes the surface's view, at first, and shows it in a new key window of scene.
func (s *surface) show(scene objc.ID) {
	s.window = objc.ID(classWindow).Send(selAlloc).Send(sel("initWithWindowScene:"), scene)
	bounds := objc.Send[apple.CGRect](s.window, selBounds)
	if s.native == 0 {
		s.controller = objc.ID(classController).Send(selNew)
		s.native = objc.ID(classSurface).Send(selAlloc).Send(selInitWithFrame, bounds)
		views[s.native] = &view{surface: s}
		s.controller.Send(sel("setView:"), s.native)
	} else {
		s.native.Send(selSetFrame, bounds)
	}
	s.window.Send(sel("setRootViewController:"), s.controller)
	s.window.Send(sel("setOverrideUserInterfaceStyle:"), uiStyle(s.appearance))
	s.window.Send(sel("makeKeyAndVisible"))
}

// attachScene attaches the surface of the scene cfg names to the tree the engine opened for it (openScene), as macOS attaches a window: at its view's size, in its environment.
func attachScene(cfg driver.SurfaceConfig) {
	i := slices.IndexFunc(scenes, func(s *surface) bool { return s.scene == cfg.Scene && s.engine == nil })
	if i < 0 {
		return
	}
	s := scenes[i]
	s.tree = cfg.ID
	s.engine = runner.engine.Attach(cfg.ID, s)
	if s.discarded {
		// The system discarded the scene before its tree came.
		s.engine.Dispatch(driver.Event{Kind: driver.EventWindowEnded})
		return
	}
	bounds := objc.Send[apple.CGRect](s.native, selBounds)
	size := driver.Size{Width: driver.Points(bounds.Size.Width), Height: driver.Points(bounds.Size.Height)}
	s.env = s.environment()
	s.engine.ResizeWithEnvironment(size, s.geometry(size), s.env)
	s.observe()
	if s.frameWanted.Load() || s.animator.Running() {
		s.startTicking()
	}
}

// sceneDidDisconnect lets go of the window of the scene UIKit releases, as it may a scene in the background while the process lives, and of the display link, which holds the surface view. The engine, the scene's tree, its surface view and its controller stay for the scene to connect again (reconnect); the engine closes only as the app terminates, and a further scene's tree as the system discards its session (discardScene).
func sceneDidDisconnect(self objc.ID, _ objc.SEL, scene objc.ID) {
	if s := sceneSurface(sessionID(scene.Send(sel("session")))); s != nil {
		s.detach()
	}
}

// discardScene follows the system discarding scene session id, as the user closes one of the app's windows: a further scene's tree ends, and runner's waits, windowless, for the next scene that connects, as the engine's own tree outlives the app's windows.
func discardScene(id string) {
	s := sceneSurface(id)
	if s == nil {
		return
	}
	s.detach()
	s.discarded = true
	if s != runner && s.engine != nil {
		s.engine.Dispatch(driver.Event{Kind: driver.EventWindowEnded})
	}
}

// endScene lets go of a further scene's surface once the engine destroyed its tree.
func (s *surface) endScene() {
	s.detach()
	s.close()
	delete(views, s.native)
	s.controller.Send(selRelease)
	s.native.Send(selRelease)
	s.native, s.controller = 0, 0
	scenes = slices.DeleteFunc(scenes, func(o *surface) bool { return o == s })
}

// detach takes the surface view out of its scene's window and lets the window go.
func (s *surface) detach() {
	if s.window == 0 {
		return
	}
	s.dropLink()
	w := s.window
	s.window = 0
	s.setKeyboard(0, 0)
	w.Send(selSetHidden, true)
	w.Send(sel("setRootViewController:"), objc.ID(0))
	w.Send(selRelease)
}

// reconnect shows a surface that shows already, or did until its scene disconnected (sceneDidDisconnect), in the window of a scene that connects, rather than starting a second engine or tree over the first. The new window lays the view out at its size, which reaches the engine as a resize with the environment read there; the links and the activity the scene connects with go to the running app.
func (s *surface) reconnect(scene objc.ID) {
	// A single-scene app's scene that connects while the window still shows in another moves the app to it.
	s.detach()
	s.show(scene)
	if s.engine == nil {
		// A test host's before a test opened its app, or a further scene's before its tree attached.
		return
	}
	s.refresh()
	if s.requestedOrientation != 0 || s.previousOrientation != 0 {
		s.updateOrientations()
	}
	if runner.engine != nil {
		runner.sendLaunchLinks()
	}
	if s.perf != nil || s.frameWanted.Load() || s.animator.Running() || len(s.motions) > 0 {
		s.startTicking()
	}
}

// watch adds what follows the surface's touches: the watcher of presses
// outside views and the tracker of raw touches.
func (s *surface) watch() {
	watcher := objc.ID(classWatcher).Send(selAlloc).Send(sel("initWithTarget:action:"), objc.ID(0), objc.SEL(0))
	watcher.Send(sel("setCancelsTouchesInView:"), false)
	watcher.Send(sel("setDelaysTouchesEnded:"), false)
	s.native.Send(sel("addGestureRecognizer:"), watcher)
	watcher.Send(selRelease)
	s.addTracker()
	s.watchHinge()
	s.watchVerticalBarEdge()
	s.headers = headerHeights()
}

// sceneReady is closed once a test host's scene has its window.
var sceneReady chan struct{}

// startEngine starts s.app's engine at the surface's size; test, when
// set, is what a test host opens it with.
func (s *surface) startEngine(test *testOpen) {
	bounds := objc.Send[apple.CGRect](s.native, selBounds)
	opts := engine.Options{
		Size:           driver.Size{Width: driver.Points(bounds.Size.Width), Height: driver.Points(bounds.Size.Height)},
		Environment:    s.env,
		NetworkWatcher: apple.NetworkWatcher{},
		Thread:         apple.RaiseThread,
		Appearance:     s.appearance,
		Host:           s.host,
		Secrets:        apple.Keychain{},
	}
	opts.SetDirs(engine.AppDirs(s.id))
	if test != nil {
		size := opts.Size
		opts.UseTest(test.options, &test.pin)
		// A phone's screen is what it is.
		opts.Size = size
		s.alerts = map[uint32]shownAlert{}
	}
	// The first commit is laid out with the regions read at the size it starts at, as layoutSubviews reports them.
	opts.Geometry = s.geometry(opts.Size)
	imageload.UseDiskCache(opts.CacheDir)
	if s.perfOn && test == nil || test != nil && test.options.FPS {
		s.perf = engine.NewPerfMonitor()
		opts.Perf = s.perf
		s.hud = newHud(s, s.perf)
		s.startTicking()
	}
	s.engine = engine.Start(s, s.app, opts)
	e := s.engine
	apple.SetNotificationPress(func(id string) { e.Dispatch(driver.Event{Kind: driver.EventNotificationPress, Text: id}) })
	apple.SetPurchaseUpdates(e.Dispatch)
	s.sendLaunchLinks()
	s.observe()
	// Scenes that connected before the engine started, as iPadOS restores a test host's, open their trees now.
	for _, o := range scenes {
		if o.scene != 0 && o.engine == nil {
			e.Dispatch(driver.Event{Kind: driver.EventOpenScene, Count: o.scene})
		}
	}
}

// testOpen is what a test host opens the app with: the test's options and
// the environment it pins.
type testOpen struct {
	options testhost.Options
	pin     engine.TestPin
}

// shownAlert is an alert a test may answer: its controller and actions.
type shownAlert struct {
	ctrl    objc.ID
	actions []driver.AlertAction
}

// environment returns what UIKit tells about where the app runs.
func (s *surface) environment() driver.Environment {
	in := objc.Send[edgeInsets](s.native, sel("safeAreaInsets"))
	traits := s.native.Send(sel("traitCollection"))
	scale := objc.Send[float64](traits, sel("displayScale"))
	if scale <= 0 {
		scale = objc.Send[float64](objc.ID(apple.Class("UIScreen")).Send(sel("mainScreen")), sel("scale"))
	}
	// The body text style is 17 points at the default text size.
	body := objc.ID(apple.Class("UIFont")).Send(sel("preferredFontForTextStyle:"),
		apple.Global(uikitPath, "UIFontTextStyleBody"))
	idiom := driver.IdiomPhone
	const pad, tv, mac = 1, 2, 5
	switch objc.Send[int](objc.ID(apple.Class("UIDevice")).Send(sel("currentDevice")), sel("userInterfaceIdiom")) {
	case pad:
		idiom = driver.IdiomTablet
	case tv:
		idiom = driver.IdiomTV
	case mac:
		idiom = driver.IdiomMac
	}
	if onMac() {
		idiom = driver.IdiomMac
	}
	const dark = 2
	locale, rtl := apple.Locale()
	deviceName, deviceModel := deviceIdentity()
	thermal, lowPower := apple.ProcessPower()
	env := driver.Environment{
		DeviceName:  deviceName,
		DeviceModel: deviceModel,
		SafeArea: driver.Edges{Top: driver.Points(in.Top), Right: driver.Points(in.Right),
			Bottom: driver.Points(in.Bottom), Left: driver.Points(in.Left)},
		KeyboardHeight:   s.keyboard,
		KeyboardDuration: s.keyboardDuration,
		Dark:             objc.Send[int](traits, sel("userInterfaceStyle")) == dark,
		FontScale:        float32(objc.Send[float64](body, sel("pointSize")) / 17),
		ReduceMotion:     uiReduceMotion(),
		IncreaseContrast: uiDarkerColors(),
		ScreenReader:     uiVoiceOver(),
		RTL:              rtl,
		PixelRatio:       float32(scale),
		Locale:           locale,
		LocaleSettings:   apple.LocaleSettings(),
		OS:               driver.OSIOS,
		OSVersion:        osVersion(),
		Idiom:            idiom,
		DiscoverServices: idiom != driver.IdiomTV,
		Speech:           apple.SpeechAvailable(),
		Speak:            apple.SpeakAvailable(),
		Audio:            idiom != driver.IdiomTV && apple.AudioAvailable(), // none on tvOS in the first stages (docs/architecture/audio.md)
		Gamepads:         true,
		GPU:              idiom != driver.IdiomTV && apple.GPUSupported(), // tvOS waits for its own verification (docs/architecture/gpu.md)
		AudioSchedule:    idiom != driver.IdiomTV && apple.AudioAvailable(),
		Camera:           cameraAvailable(),
		ScanCode:         cameraAvailable() && idiom != driver.IdiomTV,
		CanSendInput:     idiom != driver.IdiomTV,
		HasWebView:       idiom != driver.IdiomTV,
		SignInWithApple:  apple.AppleSignInAvailable(),
		Purchases:        apple.PurchasesAvailable(),
		CanRequestReview: !onTV() && apple.ReviewAvailable(),
		VerticalBarEdge:  verticalBarEdge(traits),
		Lifecycle:        appLifecycle(),
		ThermalState:     thermal,
		LowPowerMode:     lowPower,
		AppClip:          appClip(),
		OpenedURL:        openedURL,

		// tvOS's center shows badges only.
		ScheduledNotifications: idiom != driver.IdiomTV && apple.NotificationsAvailable(),
	}
	env.HeaderHeight, env.LargeHeaderHeight = s.headers[0], s.headers[1]
	return env
}

// lifecycleNote is the state each of UIApplication's notifications moves
// to, which applicationState may not say yet as it is posted.
var lifecycleNote = map[string]driver.Lifecycle{
	"UIApplicationDidBecomeActiveNotification":     driver.LifecycleActive,
	"UIApplicationWillResignActiveNotification":    driver.LifecycleInactive,
	"UIApplicationDidEnterBackgroundNotification":  driver.LifecycleBackground,
	"UIApplicationWillEnterForegroundNotification": driver.LifecycleInactive,
}

// lifecycleNow is the state the last notification moved to, 255 before
// one.
var lifecycleNow driver.Lifecycle = 255

// appLifecycle is UIApplication's applicationState: inactive under an
// incoming call, the app switcher or Control Center.
func appLifecycle() driver.Lifecycle {
	if lifecycleNow != 255 {
		return lifecycleNow
	}
	const inactive, background = 1, 2
	switch objc.Send[int](objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication")), sel("applicationState")) {
	case inactive:
		return driver.LifecycleInactive
	case background:
		return driver.LifecycleBackground
	}
	return driver.LifecycleActive
}

// layoutChanged follows a change of the surface view's safe area or traits. UIKit mostly makes them as it rotates the view or moves it to another display, after it set the new bounds and before it lays the view out at them, so for a view in a window the environment goes with the size from the layout that follows (surfaceLayout): reported now, it would reach the app at the previous size. Where UIKit lays the view out between the two, as docs/architecture/foldable.md (Updates) records, the app sees them in two batches.
func (s *surface) layoutChanged() {
	if s.engine != nil && s.uiWindow() != 0 {
		s.native.Send(selSetNeedsLayout)
		return
	}
	s.refresh()
}

// refresh reports the environment to the engine if it changed.
func (s *surface) refresh() {
	if s == nil || s.engine == nil {
		return
	}
	if env := s.environment(); !env.Equal(s.env) {
		s.env = env
		s.engine.SetEnvironment(env)
	}
}

// observe watches the notifications parts of the environment change with,
// until the surface closes.
func (s *surface) observe() {
	center := objc.ID(apple.Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	refresh := func(objc.ID) { s.refresh() }
	s.stops = append(s.stops, apple.ObserveProcessPower(s.refresh))
	on := func(name objc.ID, fn func(objc.ID)) { s.stops = append(s.stops, apple.Observe(center, name, fn)) }
	for _, name := range []string{"UIContentSizeCategoryDidChangeNotification",
		"UIAccessibilityReduceMotionStatusDidChangeNotification", "UIAccessibilityDarkerSystemColorsStatusDidChangeNotification",
		"UIAccessibilityVoiceOverStatusDidChangeNotification"} {
		on(apple.Global(uikitPath, name), refresh)
	}
	for name, state := range lifecycleNote {
		on(apple.Global(uikitPath, name), func(objc.ID) {
			lifecycleNow = state
			if name == "UIApplicationWillResignActiveNotification" {
				// The keys held come up for the app: UIKit may end their presses elsewhere, or not at all.
				s.releaseKeys()
			}
			// Players pause as the app leaves the foreground, as NSH's do.
			if state == driver.LifecycleBackground && s.audio != nil {
				s.audio.PausePlayers()
			}
			s.refresh()
		})
	}
	on(apple.Global("/System/Library/Frameworks/Foundation.framework/Foundation", "NSCurrentLocaleDidChangeNotification"), refresh)
	on(apple.Global(uikitPath, "UIKeyboardWillChangeFrameNotification"), func(note objc.ID) { s.keyboardMoved(note, false) })
	on(apple.Global(uikitPath, "UIKeyboardWillHideNotification"), func(note objc.ID) { s.keyboardMoved(note, true) })
}

// uiWindow is the window the surface shows in, 0 while it is in none.
func (s *surface) uiWindow() objc.ID { return s.native.Send(sel("window")) }

// activeWindowScene is the scene in which this surface can show a review prompt.
func (s *surface) activeWindowScene() objc.ID {
	const foregroundActive = 0 // UISceneActivationStateForegroundActive
	if window := s.uiWindow(); window != 0 && objc.Send[bool](window, sel("respondsToSelector:"), sel("windowScene")) {
		if scene := window.Send(sel("windowScene")); scene != 0 && objc.Send[int](scene, sel("activationState")) == foregroundActive {
			return scene
		}
	}
	return 0
}

// viewController is the view controller the surface's view belongs to:
// a standalone app's own, or the host's nearest above an embedded view.
func (s *surface) viewController() objc.ID {
	if !s.embedded {
		return s.controller
	}
	for r := s.native.Send(sel("nextResponder")); r != 0; r = r.Send(sel("nextResponder")) {
		if objc.Send[bool](r, sel("isKindOfClass:"), apple.Class("UIViewController")) {
			return r
		}
	}
	return 0
}

// topController returns the view controller on top of the surface's
// window's.
func (s *surface) topController() objc.ID {
	top := s.uiWindow().Send(sel("rootViewController"))
	for p := top.Send(sel("presentedViewController")); p != 0; p = top.Send(sel("presentedViewController")) {
		top = p
	}
	return top
}

// alert presents a over whatever the window shows; the action chosen goes
// to the engine as EventAlert.
func (s *surface) alert(a *driver.AlertConfig) {
	const styleSheet, styleAlert = 0, 1
	style := styleAlert
	if a.Sheet {
		style = styleSheet
	}
	ctrl := objc.ID(apple.Class("UIAlertController")).Send(sel("alertControllerWithTitle:message:preferredStyle:"),
		apple.NSString(a.Title), apple.NSString(a.Message), style)
	s.directTree(ctrl.Send(sel("view")))
	id := a.ID
	add := func(label string, style, index int) {
		handler := objcblock.New(func(_ objc.Block, _ objc.ID) {
			if s.alerts != nil {
				delete(s.alerts, id)
			}
			s.engine.Dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: index})
		})
		action := objc.ID(apple.Class("UIAlertAction")).Send(sel("actionWithTitle:style:handler:"),
			apple.NSString(label), style, handler)
		handler.Release()
		ctrl.Send(sel("addAction:"), action)
	}
	const actionDefault, actionCancel, actionDestructive = 0, 1, 2
	cancel := false
	for i, act := range a.Actions {
		style := actionDefault
		switch act.Role {
		case driver.AlertCancel:
			// UIKit allows one cancel action.
			if !cancel {
				style, cancel = actionCancel, true
			}
		case driver.AlertDestructive:
			style = actionDestructive
		}
		add(act.Label, style, i)
	}
	if len(a.Actions) == 0 {
		add("OK", actionCancel, -1)
	}
	if pop := ctrl.Send(sel("popoverPresentationController")); pop != 0 {
		// iPad: an action sheet is a popover, anchored at the bottom
		// center without an arrow. It hides its cancel action and runs it
		// on a tap outside, so one that is never shown closes it.
		if !cancel {
			handler := objcblock.New(func(_ objc.Block, _ objc.ID) {
				if s.alerts != nil {
					delete(s.alerts, id)
				}
				s.engine.Dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: -1})
			})
			action := objc.ID(apple.Class("UIAlertAction")).Send(sel("actionWithTitle:style:handler:"),
				objc.ID(0), actionCancel, handler)
			handler.Release()
			ctrl.Send(sel("addAction:"), action)
		}
		b := objc.Send[apple.CGRect](s.native, selBounds)
		pop.Send(sel("setSourceView:"), s.native)
		pop.Send(sel("setSourceRect:"), apple.Rect(b.Size.Width/2, b.Size.Height, 0, 0))
		pop.Send(sel("setPermittedArrowDirections:"), 0)
	}
	top := s.topController()
	if s.alerts != nil {
		s.alerts[id] = shownAlert{ctrl, a.Actions}
	}
	top.Send(sel("presentViewController:animated:completion:"), ctrl, true, objc.ID(0))
}

// pressOutside reports a touch going down to each view listening for
// presses outside it that holds neither the view touched nor the point.
func (s *surface) pressOutside(touch objc.ID) {
	hit := touch.Send(sel("view"))
	var at driver.Point
	found := false
	for _, v := range s.views {
		if v == nil || !v.events.Has(driver.EventPressOutside) || v.native.Send(sel("window")) == 0 {
			continue
		}
		if !found {
			p := objc.Send[apple.CGPoint](touch, selLocationInView, s.native)
			at, found = driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}, true
		}
		p := objc.Send[apple.CGPoint](touch, selLocationInView, v.native)
		b := objc.Send[apple.CGRect](v.native, selBounds)
		inside := p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height
		if inside || (hit != 0 && objc.Send[bool](hit, sel("isDescendantOfView:"), v.native)) {
			continue
		}
		s.engine.Dispatch(driver.Event{Kind: driver.EventPressOutside, ID: v.id, Point: at})
	}
}

func watcherBegan(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	apple.Super(self, cmd, touches, event)
	if v := views[self.Send(sel("view"))]; v != nil && v.surface.engine != nil {
		s := v.surface
		s.engine.InputModality(driver.ModalityPointer)
		apple.ForEach(touches, s.pressOutside)
		apple.ForEach(touches, s.clearTouched)
		apple.ForEach(touches, s.resignTouched)
	}
	// Failing at once keeps the watcher out of every other gesture.
	const failed = 5
	self.Send(sel("setState:"), failed)
}

// Prepare implements core.TextEngine with CoreText.
// RegisterFont implements driver.FontRegistrar: CoreText registers it for
// the process.
func (s *surface) RegisterFont(data []byte) error { return apple.RegisterFont(data) }

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
// display frame. Called from the engine goroutine, it hops to the main
// thread, where the display link lives, only to start the clock or bring
// it to the full rate: an app asking every frame keeps the clock running,
// and its next tick serves the frame.
func (s *surface) RequestFrame() {
	s.frameWanted.Store(true)
	if !s.fullRate.Load() {
		apple.OnMain(s.frameClock)
	}
}

// frameClock starts the clock that serves a frame the engine asked s for: s's own, or, for a standalone app's scene that shows in no foreground window, that of one that does, since the engine asks one of its surfaces for the frames of all their trees.
func (s *surface) frameClock() {
	if !s.embedded && !s.foreground() {
		for _, o := range scenes {
			if o.foreground() && o.engine != nil {
				s.frameWanted.Store(false)
				o.frameWanted.Store(true)
				o.startTicking()
				return
			}
		}
	}
	s.startTicking()
}

// foreground reports whether the surface shows in the window of a scene in the foreground, whose display link ticks.
func (s *surface) foreground() bool {
	const background = 2 // UISceneActivationStateBackground; unattached is -1
	if s.window == 0 {
		return false
	}
	state := objc.Send[int](s.window.Send(sel("windowScene")), sel("activationState"))
	return state >= 0 && state < background
}

func (s *surface) apply(b *driver.MountBatch) {
	var restored []func()
	keysChanged := false
	defer func() {
		for _, done := range restored {
			done()
		}
	}()
	s.batch++
	var lb *layoutBatch
	if b.LayoutAnimation != nil && !s.env.ReduceMotion {
		lb = &layoutBatch{anim: b.LayoutAnimation, created: map[driver.NodeID]bool{}, deleted: map[objc.ID]bool{}}
	}
	s.coverScreens(b.Items)
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
		case driver.ItemPictureInPictureCommand:
			if it.Count == driver.PictureInPictureRestored {
				restored = append(restored, apple.PictureInPictureRestoreCompletion(uint32(it.Before)))
				continue
			}
			if int(it.ID) < len(s.views) {
				if v := s.views[it.ID]; v != nil {
					if v.pip != nil {
						v.pip.Command(it.Count)
					} else if v.video != nil {
						v.video.PictureInPictureCommand(it.Count)
					}
				}
			}
			continue
		case driver.ItemAnimation:
			s.animator.Apply(it.Animation)
			continue
		case driver.ItemAlert:
			s.alert(it.Alert)
			continue
		case driver.ItemDismissKeyboard:
			s.native.Send(sel("endEditing:"), true)
			continue
		case driver.ItemSensor:
			s.sensors.Handle(it, s.engine.Dispatch, !onTV(), !onTV())
			continue
		case driver.ItemDiscovery:
			s.discovery.Handle(it, s.engine.Dispatch)
			continue
		case driver.ItemSpeech:
			apple.HandleSpeech(it.Count, *it.Speech, s.engine.Dispatch)
			continue
		case driver.ItemAuthSession:
			apple.HandleAuthSession(it.Count, *it.Auth, s.uiWindow, s.engine.Dispatch)
			continue
		case driver.ItemSpeak:
			apple.HandleSpeak(it.Count, *it.Speak, s.engine.Dispatch)
			continue
		case driver.ItemWebCookies:
			apple.WebCookies(it.Count, *it.Cookies, s.engine.Dispatch)
			continue
		case driver.ItemAudio:
			if s.audio == nil {
				s.audio = apple.NewAudio(s.engine.Dispatch)
			}
			s.audio.Handle(it.Count, it.Audio)
			continue
		case driver.ItemSetClipboard:
			objc.ID(apple.Class("UIPasteboard")).Send(sel("generalPasteboard")).Send(sel("setString:"), apple.NSString(it.Value))
			continue
		case driver.ItemReadClipboard:
			s.readClipboard(it)
			continue
		case driver.ItemDiskSpace:
			go apple.DiskSpace(it.Value, it.Count, !onTV(), s.engine.Dispatch)
			continue
		case driver.ItemSetClipboardImage:
			if data, ok := it.Data.([]byte); ok {
				setClipboardImage(data)
			}
			continue
		case driver.ItemOpenURL:
			s.openURL(it.Value, it.Count)
			continue
		case driver.ItemHaptic:
			haptic(it.Count)
			continue
		case driver.ItemSystemGestures:
			if !s.closed {
				s.setSystemGestureEdges(it.SystemGestures.IOSEdges)
			}
			continue
		case driver.ItemKeepAwake:
			if it.Awake != nil && !s.closed {
				s.keepAwake(it.Count, *it.Awake)
			}
			continue
		case driver.ItemReleaseAwake:
			s.releaseAwake(it.Count)
			continue
		case driver.ItemReleaseOffscreen:
			apple.ReleaseOffscreen(it.OffscreenID)
			continue
		case driver.ItemNotificationPermission, driver.ItemShowNotification, driver.ItemCloseNotification, driver.ItemBadge, driver.ItemTaskProgress,
			driver.ItemBiometricAvailability, driver.ItemBiometricAuthenticate:
			s.notify(it)
			continue
		case driver.ItemPermission:
			s.permission(it.Count, *it.Permission)
			continue
		case driver.ItemSignInWithApple, driver.ItemAppleCredentialState:
			apple.HandleAppleSignIn(it, s.native.Send(sel("window")), s.engine.Dispatch)
			continue
		case driver.ItemPurchase:
			apple.HandlePurchase(it, s.engine.Dispatch)
			continue
		case driver.ItemRequestReview:
			if !onTV() {
				apple.RequestReview(s.activeWindowScene())
			}
			continue
		case driver.ItemCheckForUpdates:
			s.checkForUpdates(it.Count)
			continue
		case driver.ItemPickImages:
			s.pickImages(*it.Pick, it.Count)
			continue
		case driver.ItemTakePhoto:
			s.takePhoto(*it.Pick, it.Count)
			continue
		case driver.ItemScanCode:
			s.scanCode(*it.Scan, it.Count)
			continue
		case driver.ItemPickFiles:
			s.pickFiles(*it.Files, it.Count)
			continue
		case driver.ItemPickDirectories:
			s.pickDirectories(*it.Directories, it.Count)
			continue
		case driver.ItemOpenFile:
			s.openFile(*it.Open, it.Count)
			continue
		case driver.ItemRevealFile, driver.ItemTrashFile:
			s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoFileRevealer})
			continue
		case driver.ItemInstallPackage:
			s.engine.Dispatch(driver.NoPackageInstaller(it.Count))
			continue
		case driver.ItemShare:
			s.share(*it.Share, it.Count)
			continue
		case driver.ItemSaveFile:
			s.saveFile(*it.Save, it.Count)
			continue
		case driver.ItemDirection:
			s.rtl, s.directed = it.RTL, true
			s.applyDirection()
			// An embedding host owns its languages.
			if !s.embedded {
				pinned := ""
				if it.Forced {
					pinned = it.Language
				}
				apple.PinLanguage(pinned)
			}
			continue
		case driver.ItemAppearance:
			s.appearance = it.Appearance
			if s.embedded {
				s.embedStyle()
			} else {
				setAppearance(it.Appearance)
			}
			continue
		case driver.ItemQuit, driver.ItemPopupMenu:
			// iOS has no toasts, apps do not quit themselves, and the core
			// shows phones no native menus.
			continue
		case driver.ItemImageInfo:
			apple.ImageInfo(it, s.engine.Dispatch)
			continue
		case driver.ItemKeepImage:
			if !s.closed {
				apple.KeepCanvasImage(s, it.Value)
			}
			continue
		case driver.ItemReleaseImage:
			apple.ReleaseCanvasImage(s, it.Value)
			continue
		case driver.ItemAnnounce:
			uiAccessibilityPost(announcement, apple.NSString(it.Value))
			continue
		case driver.ItemBlockNative:
			var v *view
			if int(it.ID) < len(s.views) {
				v = s.views[it.ID]
			}
			s.blockNative(v)
			continue
		case driver.ItemKeymap:
			s.keys, s.heldKeys = it.Keys, it.HeldKeys
			keysChanged = true
			continue
		case driver.ItemOrientation:
			s.setOrientation(driver.Orientation(it.Count))
			continue
		case driver.ItemStatusBar:
			s.setStatusBar(it.StatusBar)
			continue
		case driver.ItemHistory:
			s.history = int(it.Count)
			continue
		case driver.ItemOpenSurface:
			if it.Surface.Kind == driver.SurfaceScene {
				attachScene(*it.Surface)
			}
			continue
		case driver.ItemWindow:
			if it.Window.Action == driver.WindowDestroy && s.scene != 0 {
				// A further scene's tree ended (discardScene): the batch says no more, and surfaceApply lets the surface go.
				s.ending = true
				return
			}
			continue
		case driver.ItemTitle, driver.ItemBackUnhandled, driver.ItemBackPreview, driver.ItemMenu:
			continue
		}
		if int(it.ID) >= len(s.views) || s.views[it.ID] == nil {
			continue
		}
		v := s.views[it.ID]
		switch it.Kind {
		case driver.ItemDelete:
			if v.bar != nil {
				s.touchStack(s.stackOf(v))
			}
			s.delete(v, s.keepScreen(v) || s.vanish(v, lb))
		case driver.ItemRemove:
			if v.bar != nil {
				s.touchStack(s.stackOf(v))
			}
			if v.sheet != nil {
				s.dismissSheet(v)
				v.presented = false
				s.updateOrientations()
			}
			v.native.Send(selRemoveFromSuperview)
		case driver.ItemInsert:
			if v.kind == driver.KindModal && v.isSheet() {
				// A sheet holds the modal's view, wherever the core puts it.
				if v.sheet == nil {
					v.presented = true
					s.presentSheet(v)
					s.updateOrientations()
				}
				break
			}
			if v.bar != nil {
				s.touchStack(s.stackOf(v))
				s.touchStack(s.views[it.Parent])
			}
			parent := s.views[it.Parent].children()
			if it.Before == 0 {
				parent.Send(selAddSubview, v.native)
			} else {
				parent.Send(selInsertBelow, v.native, s.views[it.Before].native)
			}
			if v.kind == driver.KindModal && !v.presented {
				v.presented = true
				s.presenting = append(s.presenting, v)
				s.updateOrientations()
			}
			if p := s.views[it.Parent]; p.bar != nil {
				// The header stays over the screen's content.
				parent.Send(sel("bringSubviewToFront:"), p.bar.bar)
			}
			s.appear(v, lb, it.Parent)
		case driver.ItemOverflow:
			v.overflow, v.transformedKids = it.Frame, it.Transformed
		case driver.ItemMatch:
			s.match(v, s.views[it.Before], lb)
		case driver.ItemFrame:
			if it.Frame.Size == (driver.Size{}) && s.covered(v) {
				break
			}
			s.setFrame(v, it, lb)
			if v.bar != nil {
				s.layoutBar(v)
			}
			if v.headerWrap != 0 {
				s.sizeHeaderWrap(v)
			}
		case driver.ItemScreen:
			s.configureScreen(v, it.Screen)
		case driver.ItemViewStyle:
			if v.bar != nil && it.ViewStyle.Hidden != v.style.Hidden {
				s.touchStack(s.stackOf(v))
			}
			v.style = *it.ViewStyle
			v.setBackdrop(it.ViewStyle)
			if v.animatesStyle() {
				v.restyle()
			} else {
				v.styleLayer(it.ViewStyle)
			}
			if v.layout != nil {
				// A rounded text's layer takes its exact frame (sizeContent).
				v.sizeContent()
			}
			v.native.Send(selSetHidden, it.ViewStyle.Hidden)
			if v.kind == driver.KindScroll || v.kind == driver.KindVideo {
				// A scroll view shows only what it scrolls to, and a video
				// only its frame.
				v.native.Send(sel("setClipsToBounds:"), true)
			}
			if v.image != nil {
				v.image.SetStyle(it.ViewStyle)
			}
		case driver.ItemImage:
			v.image.Configure(it.Image)
			s.images = append(s.images, v)
		case driver.ItemModal:
			v.modal = *it.Modal
			if v.modal.Orientations != 0 {
				s.oriented = append(s.oriented, v)
			}
		case driver.ItemWeb:
			v.web.Configure(it.Web)
		case driver.ItemWebCommand:
			if in, ok := it.Data.(*driver.WebInput); ok && it.WebCommand == driver.WebSendInput {
				if web := v.web.View(); web != 0 {
					sendWebInput(web, *in)
				}
				break
			}
			v.web.Command(it.WebCommand, it.Value, it.Data)
		case driver.ItemPictureInPicture:
			if v.pip == nil {
				v.pip = apple.NewPictureInPicture(0, v.report)
				s.pips = append(s.pips, v)
				if layer := v.pip.Layer(); layer != 0 {
					// AVKit needs an inline source attached to a window. The content
					// tree stays detached; a one-point layer is its system anchor.
					layer.Send(selSetFrame, apple.Rect(0, 0, 1, 1))
					s.native.Send(selLayer).Send(sel("addSublayer:"), layer)
				}
			}
			v.pip.Configure(*it.PictureInPicture)
		case driver.ItemVideo:
			v.video.Configure(it.Video)
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
			v.played = v.played[:0] // drawn whole first
			s.animator.BindCanvas(v.id, it.Drawing.Slots)
			v.setCanvasNodes(it.Drawing.Nodes)
			v.redraw()
		case driver.ItemGPU:
			if v.kind == driver.KindGPU && it.GPU != nil && s.env.GPU {
				v.newGPU(*it.GPU)
			}
		case driver.ItemShader:
			if v.kind == driver.KindShader && it.Shader != nil && it.Shader.Role == driver.ShaderFill {
				s.setShader(v, it.Shader)
			} else {
				s.setEffect(v, it.Shader)
			}
		case driver.ItemShaderTransition:
			s.startTransition(v)
		case driver.ItemShaderInput:
			if fx := s.effects.Get(v); fx != nil {
				fx.Invalidate()
				s.shadeLater(v)
			}
		case driver.ItemScrollTo:
			start, end := scrollLimits(v.native)
			to := apple.CGPoint{
				X: min(max(float64(it.Point.X), start.X), end.X),
				Y: min(max(float64(it.Point.Y), start.Y), end.Y),
			}
			if v.sc != nil {
				v.sc.applied++
			}
			v.native.Send(sel("setContentOffset:animated:"), to, it.Animated)
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
			v.activation, v.handoff, v.area, v.selectable = p.PanActivation, p.PanScrollHandoff, p.SelectionArea, p.Selectable
			if v.selColor != p.SelectionColor {
				v.selColor = p.SelectionColor
				v.redraw()
			}
			v.setSelectable()
			if v.kind == driver.KindText && v.selectable == driver.SelectableOn {
				s.ownTexts[v.id] = v
			} else {
				delete(s.ownTexts, v.id)
			}
			v.updateOwnSelection()
		case driver.ItemEvents:
			v.events = it.Events
			if v.input != nil {
				v.input.fs.Events = it.Events
			}
			v.setGestures(it.Events)
			v.updateAccessibility()
		case driver.ItemAccessibilityLabel:
			v.setLabel(it.Value)
		case driver.ItemAccessibilityFocus:
			v.focusAccessibility()
		case driver.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.box = it.Text.Content
			v.sizeContent()
			if it.Text.Text != v.plain {
				v.textReplaced()
			}
			v.spans = it.Text.Spans
			v.textChanged(it.Text.Text)
			v.redraw()
		case driver.ItemInput:
			v.configure(it.Input)
			s.setAccessory(v, it.Input.Accessory)
		case driver.ItemInputValue:
			v.input.value(it.Value, it.Count)
		case driver.ItemSelection:
			if v.input != nil {
				v.input.setSelection(it.Selection)
			}
		case driver.ItemScrollConfig:
			if v.sc != nil {
				v.configureScroll(it.Scroll)
			}
		case driver.ItemScrollContent:
			if v.sc != nil {
				v.setContent(it.Size)
			}
		case driver.ItemZoomTo:
			v.zoomTo(it.Zoom, it.Animated)
		case driver.ItemFocus:
			if v.input != nil {
				v.input.field.Send(selBecomeFirst)
			} else if v.focusable() {
				s.focusView(v)
			}
		case driver.ItemBlur:
			if v.input != nil {
				v.input.field.Send(selResign)
			}
		}
	}
	s.root = b.Root
	if root := s.views[b.Root]; root.native.Send(selSuperview) != s.native {
		s.native.Send(selAddSubview, root.native)
	}
	s.slideStacks()
	s.finish()
	if keysChanged {
		s.refreshKeyCommands()
	}
	s.refreshSelection()
	for _, v := range s.ownTexts {
		v.updateOwnSelection()
	}
	apple.WithoutAnimation(func() {
		if lb != nil {
			s.stepMotions(s.clock())
		}
		s.pullBackResizedScrolls()
		s.applyAnimated()
	})
	s.drawShaders()
	s.dirtyPictureInPictures()
	if s.animator.Running() {
		s.startTicking()
	}
}

// finish starts what a batch asked for once its frames are set: image
// loads at the views' sizes and modals' entrances.
func (s *surface) finish() {
	s.startImages()
	s.updateModals()
	for _, v := range s.presenting {
		if s.views[v.id] == v && v.native.Send(selSuperview) != 0 && !s.navigating(s.views[s.root]) {
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

func (s *surface) create(kind driver.ViewKind, id driver.NodeID) *view {
	// A new view has the default style and no props or events: the core
	// sends them only when they differ.
	v := &view{surface: s, id: id, kind: kind, style: driver.DefaultViewStyle,
		drawn: driver.DefaultViewStyle, born: s.batch}
	switch kind {
	case driver.KindScroll:
		v.native = objc.ID(classScroll).Send(selNew)
		newScroll(v)
		// It shows only what it scrolls to.
		v.native.Send(sel("setClipsToBounds:"), true)
	case driver.KindInput:
		v.newInput()
	case driver.KindText:
		v.native = objc.ID(classText).Send(selNew)
	default:
		v.native = objc.ID(classView).Send(selNew)
	}
	v.native.Send(selSetOpaque, false)
	s.direct(v.native)
	switch kind {
	case driver.KindText, driver.KindCanvas:
		v.newContent()
	case driver.KindImage:
		v.image = apple.NewImageView(v.native.Send(selLayer))
		v.image.Dispatch = v.report
		v.image.Changed = s.dirtyPictureInPictures
	case driver.KindWeb:
		v.web = apple.NewWeb(v.native)
		v.web.Dispatch = v.report
	case driver.KindVideo:
		v.video = apple.NewVideo(v.native)
		v.video.Dispatch = v.report
		v.video.Parent = s.viewController()
		v.native.Send(sel("setClipsToBounds:"), true)
	case driver.KindShader:
		v.newShader()
	}
	if kind == driver.KindText {
		v.updateAccessibility() // a text is an element without props
	}
	views[v.native] = v
	return v
}

// delete releases v; kept leaves its native where it is, going away.
func (s *surface) delete(v *view, kept bool) {
	v.releaseCanvasNodes()
	if v.tsel != nil {
		v.resignText()
		v.tsel = nil
	}
	delete(s.ownTexts, v.id)
	if v.sc != nil {
		for i := range v.sc.edges {
			if ev := &v.sc.edges[i]; ev.holder != 0 {
				ev.paint.Release()
			}
		}
	}
	if v.sheet != nil {
		s.dismissSheet(v)
		defer s.updateOrientations()
	}
	if sl := s.slides[v]; sl != nil {
		s.endSlide(sl)
	}
	delete(s.tops, v)
	if b := v.bar; b != nil {
		delete(bars, b.bar)
		b.bar.Send(selRelease)
		b.item.Send(selRelease)
		b.back.Send(selRelease)
		if b.system != 0 {
			b.system.Send(selRelease)
		}
	}
	if v.headerWrap != 0 {
		v.headerWrap.Send(selRemoveFromSuperview)
		v.headerWrap.Send(selRelease)
	}
	if !kept && !s.inGhost(v.native) && !s.inGone(v.native) {
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
	apple.ForgetTransform(v.native.Send(selLayer))
	apple.ForgetDecor(v.native.Send(selLayer))
	v.forgetContent()
	forgetGate(v.pan)
	v.layout.Release()
	if v.image != nil {
		v.image.Close()
	}
	if v.web != nil {
		v.web.Close()
	}
	if v.pip != nil {
		v.pip.Close(nil)
		v.pip = nil
		s.pips = slices.DeleteFunc(s.pips, func(p *view) bool { return p == v })
	}
	if v.video != nil {
		v.video.Close()
	}
	s.endEffect(v)
	if v.container != 0 {
		apple.ForgetHolder(v.container.Send(selLayer))
		delete(views, v.container)
	}
	if v.shader != nil {
		v.shader.Release()
	}
	if v.gpu != nil {
		s.releaseGPU(v)
	}
	delete(views, v.native)
	if v.input != nil {
		delete(views, v.input.field)
	}
	v.native.Send(selRelease)
	s.views[v.id] = nil
}

func (v *view) dispatch(kind driver.EventKind) {
	v.report(driver.Event{Kind: kind})
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

// pointer returns where touch is in v and in v's root view, the modal
// around it or the surface, and the modifier keys event holds.
func (v *view) pointer(kind driver.EventKind, touch, event objc.ID) driver.Event {
	root := v.surface.native
	for n := v.native; n != 0 && n != root; n = n.Send(selSuperview) {
		if m := views[n]; m != nil && m.native == n && m.kind == driver.KindModal {
			root = n
			break
		}
	}
	p := objc.Send[apple.CGPoint](touch, selLocationInView, v.native)
	page := objc.Send[apple.CGPoint](touch, selLocationInView, root)
	return driver.Event{Kind: kind, Point: driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)},
		Page:      driver.Point{X: driver.Points(page.X), Y: driver.Points(page.Y)},
		Modifiers: apple.Modifiers(objc.Send[uint](event, sel("modifierFlags")))}
}

// pressed reports kind about the press v follows, with where it began.
func (v *view) pressed(kind driver.EventKind) {
	e := v.pressAt
	e.Kind = kind
	v.report(e)
}

// hitTest is UIKit's hit test of self at p, in its own coordinates, by
// its pointer events and hit slop, and looking in its children where
// they reach outside it, as core.Reach says.
func hitTest(self objc.ID, cmd objc.SEL, p apple.CGPoint, event objc.ID) objc.ID {
	v := views[self]
	if v == nil || v.native != self {
		return apple.SendSuper[objc.ID](self, cmd, p, event)
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	frame := driver.Bounds{Size: driver.Size{Width: driver.Points(b.Size.Width), Height: driver.Points(b.Size.Height)}}
	at := driver.Point{X: driver.Points(p.X - b.Origin.X), Y: driver.Points(p.Y - b.Origin.Y)}
	own, children := driver.ReachView(frame.Size, &v.props, v.overflow, v.transformedKids, at)
	if !frame.Contains(at) {
		if objc.Send[bool](self, sel("isHidden")) || !objc.Send[bool](self, sel("isUserInteractionEnabled")) {
			return 0
		}
		// A selection's knobs stand out of its text or its area.
		if v.reachesEnds(p) {
			return self
		}
		if sl := v.surface.selecting; sl != nil && sl.area == v {
			for _, h := range sl.handles {
				if h == 0 {
					continue
				}
				local := objc.Send[apple.CGPoint](self, sel("convertPoint:toView:"), p, h)
				if hit := h.Send(cmd, local, event); hit != 0 {
					return hit
				}
			}
		}
		if children {
			subs := self.Send(sel("subviews"))
			for i := objc.Send[int](subs, sel("count")) - 1; i >= 0; i-- {
				sub := subs.Send(sel("objectAtIndex:"), i)
				local := objc.Send[apple.CGPoint](self, sel("convertPoint:toView:"), p, sub)
				if hit := sub.Send(cmd, local, event); hit != 0 {
					return hit
				}
			}
		}
		if own {
			return self
		}
		return 0
	}
	hit := apple.SendSuper[objc.ID](self, cmd, p, event)
	if hit != 0 && hit != self && hit == v.container {
		// The holder of v's children, hit where no child is, is v itself; an input's editor, also v's, takes its own clicks.
		hit = self
	}
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

// longPressDelay is how long a touch on v is held to be a long press.
func (v *view) longPressDelay() time.Duration {
	if d := v.props.DelayLongPress; d > 0 {
		return d
	}
	return longPressDelay
}

func (v *view) pressable() bool {
	return v.events.Has(driver.EventPress) || v.events.Has(driver.EventLongPress) ||
		v.events.Has(driver.EventPressDown) || v.events.Has(driver.EventPressUp)
}

// Touches a view does not handle go up the responder chain to a pressable
// ancestor.
func touchesBegan(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	// A secondary click of a pointer opens a context menu.
	const secondary = 1 << 1
	if v != nil && v.events.Has(driver.EventContextMenu) && objc.Send[uint](event, sel("buttonMask"))&secondary != 0 {
		v.report(v.pointer(driver.EventContextMenu, touches.Send(selAnyObject), event))
		return
	}
	if v == nil || !v.pressable() {
		apple.Super(self, cmd, touches, event)
		return
	}
	v.pressing.Store(true)
	v.longFired.Store(false)
	touch := touches.Send(selAnyObject)
	v.pressAt = v.pointer(driver.EventPressDown, touch, event)
	v.pressAt.Count = uint32(max(objc.Send[int](touch, sel("tapCount")), 1))
	// A long press on a text's span is the span's.
	v.pressAt.Index = v.indexAt(objc.Send[apple.CGPoint](touch, selLocationInView, self))
	v.beginPress()
}

func touchesCancelled(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		apple.Super(self, cmd, touches, event)
		return
	}
	if v.pressing.Swap(false) {
		v.pressed(driver.EventPressUp)
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
	v.pressed(driver.EventPressUp)
	if pressed && !v.longFired.Load() && v.within(p, b) {
		e := v.pressAt
		e.Kind, e.Index = driver.EventPress, v.indexAt(p)
		v.report(e)
	}
}

// indexAt returns the byte offset of the character at p in a text view,
// for presses on its links, or -1.
func (v *view) indexAt(p apple.CGPoint) int {
	if v.layout == nil {
		return -1
	}
	box := v.box
	return v.layout.IndexAt(p, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
		float64(box.Size.Width), float64(box.Size.Height)))
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
	app := pressSurface(self)
	if app != nil && typesKey(presses) {
		app.engine.InputModality(driver.ModalityKeyboard)
	}

	if app != nil && app.engine != nil && app.menuGoesBack(presses) {
		// The remote's Menu button: back, rather than out of the app.
		app.engine.Dispatch(driver.Event{Kind: driver.EventBack})
		app.menuHeld = true
		return
	}
	if app != nil && app.engine != nil && app.pressKeys(presses) {
		// The focused view consumed the key: the keymap and UIKit do not see it.
		return
	}
	var id uint32
	consumed := false
	if app != nil && app.engine != nil && len(app.keys) > 0 {
		focus := app.focusKeyUse()
		apple.ForEach(presses, func(press objc.ID) {
			key := press.Send(sel("key"))
			if key == 0 || consumed {
				return
			}
			name, ok := keyNames[objc.Send[int](key, sel("keyCode"))]
			if !ok {
				name = strings.ToLower(apple.GoString(key.Send(sel("charactersIgnoringModifiers"))))
			}
			if name != "" {
				id, consumed = driver.FindKey(app.keys, apple.KeyNames(name, objc.Send[uint](key, sel("modifierFlags"))),
					focus, false)
			}
		})
	}
	if !consumed {
		apple.Super(self, cmd, presses, event)
		return
	}
	app.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
	if i := slices.IndexFunc(app.keys, func(b driver.KeyBinding) bool { return b.ID == id }); i >= 0 && app.keys[i].Repeat {
		app.repeatKey(id)
	}
}

// modifierUsages are the UIKeyboardHIDUsage codes of the modifier keys, by the name core.NormalizeKey gives a modifier pressed alone. The keymap leaves them out, as no binding takes a modifier alone.
var modifierUsages = map[int]string{
	224: "ctrl", 228: "ctrl", 225: "shift", 229: "shift", 226: "alt", 230: "alt", 227: "meta", 231: "meta",
}

// heldKey is a hardware key whose down was reported: the view it went to, 0 for none, and the name it reported.
type heldKey struct {
	id   driver.NodeID
	name string
}

// pressesKey names a UIKey as driver.NormalizeKey does, without modifiers, a modifier pressed alone included, or "" for none.
func pressesKey(key objc.ID) string {
	code := objc.Send[int](key, sel("keyCode"))
	if name, ok := modifierUsages[code]; ok {
		return name
	}
	if name, ok := keyNames[code]; ok {
		return name
	}
	return strings.ToLower(apple.GoString(key.Send(sel("charactersIgnoringModifiers"))))
}

// withModifier returns m with the modifier a key named name is, if it is one, held or not.
func withModifier(m driver.Modifiers, name string, held bool) driver.Modifiers {
	switch name {
	case "shift":
		m.Shift = held
	case "ctrl":
		m.Control = held
	case "alt":
		m.Alt = held
	case "meta":
		m.Platform = held
	}
	return m
}

// pressKeys reports the hardware keys going down in presses, before the keymap: to the focused view that hears keys (View.OnKeyDown), which only an Apple TV has, or with ID 0 where none does and the surface asked for held keys. A press with no key, as the remote's buttons, and keys typed while a text input has the focus are left out. It records each key it reports, so its up reaches the same view, and reports whether the focused view consumed one. UIKit reports a held key once, so no down is a repeat.
func (s *surface) pressKeys(presses objc.ID) bool {
	if s.editing() {
		return false
	}
	v := s.focused
	if v != nil && (s.views[v.id] != v || !v.events.Has(driver.EventKeyDown)) {
		v = nil
	}
	if v == nil && !s.heldKeys {
		return false
	}
	consumed := false
	apple.ForEach(presses, func(press objc.ID) {
		key := press.Send(sel("key"))
		if key == 0 {
			return
		}
		name := pressesKey(key)
		if name == "" {
			return
		}
		code := objc.Send[int](key, sel("keyCode"))
		mods := s.heldModifiers(apple.Modifiers(objc.Send[uint](key, sel("modifierFlags"))))
		if held, ok := s.keysHeld[code]; ok {
			// Its up never came: report it before the key goes down again.
			s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: driver.USBKeyCode(uint16(code)), Modifiers: withModifier(mods, held.name, false)})
		}
		mods = withModifier(mods, name, true)
		if s.keysHeld == nil {
			s.keysHeld = map[int]heldKey{}
		}
		if v == nil {
			s.keysHeld[code] = heldKey{0, name}
			s.engine.Dispatch(driver.Event{Kind: driver.EventKeyDown, Text: name, KeyCode: driver.USBKeyCode(uint16(code)), Modifiers: mods})
			return
		}
		s.keysHeld[code] = heldKey{v.id, name}
		if s.engine.DispatchKeyDown(driver.Event{Kind: driver.EventKeyDown, ID: v.id, Text: name, KeyCode: driver.USBKeyCode(uint16(code)), Modifiers: mods}) {
			consumed = true
		}
	})
	return consumed
}

// releasePresses reports the up of each hardware key in presses whose down was reported, to the view its down went to, with the modifiers still held, the released one not among them while no other key of its name is held.
func (s *surface) releasePresses(presses, event objc.ID) {
	if len(s.keysHeld) == 0 {
		return
	}
	apple.ForEach(presses, func(press objc.ID) {
		key := press.Send(sel("key"))
		if key == 0 {
			return
		}
		code := objc.Send[int](key, sel("keyCode"))
		held, ok := s.keysHeld[code]
		if !ok {
			return
		}
		delete(s.keysHeld, code)
		// The event's flags are the keys held now; a press's are those of its own down.
		flags := objc.Send[uint](key, sel("modifierFlags"))
		if event != 0 && objc.Send[bool](event, sel("respondsToSelector:"), sel("modifierFlags")) {
			flags = objc.Send[uint](event, sel("modifierFlags"))
		}
		mods := withModifier(s.heldModifiers(apple.Modifiers(flags)), held.name, s.holds(held.name))
		s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: driver.USBKeyCode(uint16(code)), Modifiers: mods})
	})
}

// heldModifiers returns m with the modifier keys whose down the surface reported and that are still held: UIKit's flags can miss one, as they miss the first Shift a simulator's synthesized keyboard holds in a newly launched app.
func (s *surface) heldModifiers(m driver.Modifiers) driver.Modifiers {
	for _, held := range s.keysHeld {
		// withModifier leaves m as it is for a key that is no modifier.
		m = withModifier(m, held.name, true)
	}
	return m
}

// holds reports whether a key named name is still held, as the other Shift.
func (s *surface) holds(name string) bool {
	for _, held := range s.keysHeld {
		if held.name == name {
			return true
		}
	}
	return false
}

// releaseKeys reports the up of every hardware key whose down was reported, with no modifiers held, as the app stops being active.
func (s *surface) releaseKeys() {
	if s.engine == nil {
		return
	}
	for code, held := range s.keysHeld {
		delete(s.keysHeld, code)
		s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: driver.USBKeyCode(uint16(code))})
	}
}

// typesKey reports whether presses hold a hardware key other than a
// modifier (UIKeyboardHIDUsage's Caps Lock, and the controls, shifts,
// options and commands from 224 on).
func typesKey(presses objc.ID) bool {
	typed := false
	apple.ForEach(presses, func(press objc.ID) {
		if key := press.Send(sel("key")); key != 0 {
			if code := objc.Send[int](key, sel("keyCode")); code != 57 && (code < 224 || code > 231) {
				typed = true
			}
		}
	})
	return typed
}

// pressSurface is the surface hardware keys pressed in self go to: an
// embedded view's, or a standalone app's scene's from its controller.
func pressSurface(self objc.ID) *surface {
	if v := views[self]; v != nil {
		return v.surface
	}
	if v := sheets[self]; v != nil {
		return v.surface
	}
	return controllerSurface(self)
}

// pressesEnded reports the held keys' ups and stops repeating a held key's binding.
func pressesEnded(self objc.ID, cmd objc.SEL, presses, event objc.ID) {
	app := pressSurface(self)
	if app != nil && app.engine != nil {
		app.releasePresses(presses, event)
	}
	if app != nil && app.heldStop != nil {
		close(app.heldStop)
		app.heldStop, app.held = nil, 0
	}
	if app != nil && app.menuHeld && hasPress(presses, pressMenu) {
		// The Menu press that went back ends here too, or UIKit would
		// leave the app at its end.
		app.menuHeld = false
		return
	}
	apple.Super(self, cmd, presses, event)
}

// repeatKey fires the binding with ID id again while its key is held, as a hardware keyboard's auto-repeat would: after 500 ms, then 30 times a second.
func (s *surface) repeatKey(id uint32) {
	if s.heldStop != nil {
		close(s.heldStop)
	}
	stop := make(chan struct{})
	s.held, s.heldStop = id, stop
	e := s.engine
	go func() {
		t := time.NewTimer(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				e.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
				t.Reset(33 * time.Millisecond)
			}
		}
	}()
}

// editing reports whether a text input has the keyboard focus.
func (s *surface) editing() bool {
	for _, v := range s.views {
		if v != nil && v.input != nil && v.input.focused {
			return true
		}
	}
	return false
}

func surfaceLayout(self objc.ID, cmd objc.SEL) {
	apple.Super(self, cmd)
	if v := views[self]; v != nil && v.surface.engine != nil {
		b := objc.Send[apple.CGRect](self, selBounds)
		// The geometry is read at the size reported now, so it goes with it, as does the environment that changed with the size, such as the safe area.
		s := v.surface
		size := driver.Size{Width: driver.Points(b.Size.Width), Height: driver.Points(b.Size.Height)}
		g := s.geometry(size)
		if env := s.environment(); !env.Equal(s.env) {
			s.env = env
			s.engine.ResizeWithEnvironment(size, g, env)
		} else {
			s.engine.Resize(size, g)
		}
		if h := v.surface.hud; h != nil {
			h.layout()
		}
	}
}

func surfaceApply(self objc.ID, _ objc.SEL, _ objc.ID) {
	// The message outlives a surface closed while it waited.
	v := views[self]
	if v == nil {
		return
	}
	s := v.surface
	done := s.measuring(true)
	for _, b := range s.batches.Drain() {
		if s.perf != nil && !b.Sent.IsZero() {
			s.perf.RecordLatency(time.Since(b.Sent))
		}
		driver.Apply(b, s.apply)
	}
	done()
	if s.ending {
		s.endScene()
		return
	}
	if s.hud != nil {
		s.native.Send(sel("bringSubviewToFront:"), s.hud.panel)
	}
	// A frame requested before the view was in a window starts the clock
	// here: a display link needs the view mounted.
	if s.frameWanted.Load() && !s.ticking {
		s.startTicking()
	}
}

func scrolled(self objc.ID, _ objc.SEL, _ objc.ID) {
	v := views[self]
	if v == nil {
		return
	}
	apple.FollowBounds(self.Send(selLayer))
	// Pinning it scrolls it again, which reports the pinned offset.
	if at := objc.Send[apple.CGPoint](self, selContentOffset); pinTop(self) && objc.Send[apple.CGPoint](self, selContentOffset) != at {
		return
	}
	o := objc.Send[apple.CGPoint](self, selContentOffset)
	offset := driver.Point{X: driver.Points(o.X), Y: driver.Points(o.Y)}
	scrolled := v.surface.animator.Scroll(v.id, offset)
	if v.sc != nil && v.surface.animator.Pulls(v.id) && v.surface.animator.Pull(v.id, v.pulled()) {
		scrolled = true
	}
	if scrolled {
		apple.WithoutAnimation(v.surface.applyAnimated)
	}
	if v.sc != nil && len(v.sc.sticky) > 0 {
		v.pinSticky()
	}
	if v.sc != nil && v.hasEdges() {
		v.followEdges()
	}
	v.surface.followLargeTitle(v, offset.Y)
	v.scrollEvent(driver.EventScroll)
}

// dirtyPictureInPictures schedules snapshots after tree changes, animation steps and image loads, including ones that need no engine commit.
func (s *surface) dirtyPictureInPictures() {
	for _, v := range s.pips {
		v.pip.Dirty()
		if v.pip.Layer() != 0 {
			s.startTicking()
		}
	}
}

// drawPictureInPictures retains one pending frame under back-pressure.
func (s *surface) drawPictureInPictures() bool {
	pending := false
	for _, v := range s.pips {
		if v.pip.DrawFrame(v.native.Send(selLayer), float64(s.env.PixelRatio)) {
			pending = true
		}
	}
	return pending
}
