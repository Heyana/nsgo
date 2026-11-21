//go:build windows

// Package windows is NSGo's Win32 backend: one window that draws a
// retained tree of views with Direct2D into a flip-model swap chain on a
// Direct3D 11 device, text the engine laid out with DirectWrite, presses
// hit tested by the engine, inputs as editors of the backend's own drawn
// with the views (input.go), web views as WebView2 composition controllers
// in the window's DirectComposition tree, videos played by Media
// Foundation's media engine, images decoded by Go, native components'
// child windows following their views (component.go), files dropped
// through OLE (drop.go), alerts as task dialogs and updates through
// WinSparkle (updates.go). A window with a
// material behind its content draws through DirectComposition too, over
// the system backdrop DWM draws (material.go). It uses only the syscall
// package, so it builds without cgo.
//
// Transforms are Direct2D world transforms, translucent views draw in
// layers, and shapes and text are antialiased by Direct2D. Style
// transitions, animated values, layout animations and animated images
// tick on one frame clock, a timer that runs only while something
// animates (animate.go). Image blur and modal animations are not drawn,
// and there is no accessibility yet, which needs UI Automation
// providers.
package windows

import (
	"errors"
	"fmt"
	"log"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/finger"
	"github.com/nsgo-dev/nsgo/internal/platform/hover"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/loopback"
	"github.com/nsgo-dev/nsgo/internal/platform/queue"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
	"github.com/nsgo-dev/nsgo/internal/platform/textselect"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/internal/platform/viewtree"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func init() {
	// The window belongs to the thread that creates it: main's.
	runtime.LockOSThread()
}

// Options configure the window.
type Options struct {
	Title         string
	Width, Height driver.Points
	// Hidden keeps the backend's root window off the screen as the app's own surface (nsgo.Run(App)), which opens the windows the app shows and shows only as an alert's window.
	Hidden bool
	// MainHidden opens the main window the app's function opens without showing it (nsgo.StartHidden).
	MainHidden bool
	// Frame is where the window opens (nsgo.WindowFrame).
	Frame driver.WindowFrame
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
	// InstanceKey scopes the single-instance lock; empty keeps one lock per ID.
	InstanceKey string
	// Components are the native components Native elements name.
	Components driver.NativeComponents
	// Material is the material behind the window's content, where the app
	// draws nothing opaque (nsgo.WindowMaterial). It needs Windows 11
	// 22H2 or later, where DWM draws system backdrops; earlier Windows
	// keeps the window opaque.
	Material driver.WindowMaterial
	// Constraints limit the window (nsgo.WindowConstraints).
	Constraints driver.WindowConstraints
	// Transparent opens the window without a background or frame, NoShadow without DWM's shadow, which only a transparent window lacks, and SkipTaskbar off the taskbar (look.go).
	Transparent, NoShadow, SkipTaskbar bool
	// Fonts are font files under the app's resources, registered before
	// the first view.
	Fonts []string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
	// Perf shows the performance monitor over the app (perf.go).
	Perf bool
	// Updates is how the app updates, with WinSparkle (updates.go).
	Updates updates.Config
	// RegisterToast is the shortcut the installer has the app stamp with the toast activator, and UnregisterToast has the uninstaller's run remove the activator's registration, from the launch flags; either run does that alone and returns (toast_activation.go).
	RegisterToast   string
	UnregisterToast bool
	// DockTask is the label of the item the Jump List task that started the app chose, from the launch flags.
	DockTask string
	// LaunchedAtLogin says the login item started this process.
	LaunchedAtLogin bool
	// Host is what an embedding host gives the view (NewView), and Parent
	// the HWND of the window it is a child window of.
	Host   engine.HostOptions
	Parent uintptr
	// test is what a test host opens the app with.
	test *testOpen
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	registerClassExW  = user32.NewProc("RegisterClassExW")
	createWindowExW   = user32.NewProc("CreateWindowExW")
	defWindowProcW    = user32.NewProc("DefWindowProcW")
	getMessageW       = user32.NewProc("GetMessageW")
	translateMessage  = user32.NewProc("TranslateMessage")
	dispatchMessageW  = user32.NewProc("DispatchMessageW")
	postMessageW      = user32.NewProc("PostMessageW")
	postQuitMessage   = user32.NewProc("PostQuitMessage")
	showWindow        = user32.NewProc("ShowWindow")
	setFocus          = user32.NewProc("SetFocus")
	getFocus          = user32.NewProc("GetFocus")
	getAncestor       = user32.NewProc("GetAncestor")
	destroyWindow     = user32.NewProc("DestroyWindow")
	beginPaint        = user32.NewProc("BeginPaint")
	endPaint          = user32.NewProc("EndPaint")
	invalidateRect    = user32.NewProc("InvalidateRect")
	getClientRect     = user32.NewProc("GetClientRect")
	adjustWindowRect  = user32.NewProc("AdjustWindowRect")
	screenToClient    = user32.NewProc("ScreenToClient")
	loadCursorW       = user32.NewProc("LoadCursorW")
	loadIconW         = user32.NewProc("LoadIconW")
	setWindowTextW    = user32.NewProc("SetWindowTextW")
	getWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	setWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	getDpiForSystem   = user32.NewProc("GetDpiForSystem")
	setDpiAwareness   = user32.NewProc("SetProcessDpiAwarenessContext")

	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy             = 0x0002
	wmMove                = 0x0003
	wmSize                = 0x0005
	wmActivate            = 0x0006
	wmPaint               = 0x000F
	wmEraseBkgnd          = 0x0014
	wmSettingChange       = 0x001A
	wmInputLanguageChange = 0x0051
	wmDisplayChange       = 0x007E
	wmDPIChanged          = 0x02E0
	wmShowWindow          = 0x0018
	wmActivateApp         = 0x001C
	wmMouseActivate       = 0x0021
	wmKeyDown             = 0x0100
	wmKeyUp               = 0x0101
	wmChar                = 0x0102
	wmSysKeyDown          = 0x0104
	wmCommand             = 0x0111
	wmLButtonDown         = 0x0201
	wmLButtonUp           = 0x0202
	wmRButtonDown         = 0x0204
	wmMouseWheel          = 0x020A
	wmMouseHWheel         = 0x020E
	wmAppCommand          = 0x0319     // what DefWindowProc makes of XBUTTON1 and back keys
	wmApply               = 0x8000 + 1 // WM_APP + 1: batches are queued
	wmRun                 = 0x8000 + 2 // functions are queued
	wmAlert               = 0x8000 + 3 // alerts are queued
	wmFrame               = 0x8000 + 4 // the app waits for a display frame
	wmSysCommand          = 0x0112
	scClose               = 0xF060 // SC_CLOSE, in WM_SYSCOMMAND's wParam

	wsOverlappedWindow = 0x00CF0000
	wsCaption          = 0x00C00000
	wsSysMenu          = 0x00080000
	wsThickFrame       = 0x00040000
	wsMinimizeBox      = 0x00020000
	wsMaximizeBox      = 0x00010000
	wsClipChildren     = 0x02000000
	wsVisible          = 0x10000000
	wsExNoRedirect     = 0x00200000           // no redirection bitmap, for a composition surface
	wsExToolWindow     = 0x00000080           // no taskbar button or Alt+Tab entry
	wsExNoActivate     = 0x08000000           // a click does not activate the window
	wsExTopmost        = 0x00000008           // above every window that is not topmost
	maNoActivate       = 3                    // MA_NOACTIVATE: a click goes to the window without activating it
	cwUseDefault       = ^uintptr(0x7FFFFFFF) // CW_USEDEFAULT as a uintptr
	gwlStyle           = ^uintptr(15)         // -16
	swShow             = 5
	sizeMinimized      = 1
	vkReturn           = 0x0D
	vkEscape           = 0x1B
	vkLeft             = 0x25
	appCommandBack     = 1 // APPCOMMAND_BROWSER_BACKWARD

	defaultFontName = "Segoe UI"
)

// wheelStep is how far one notch of a mouse wheel scrolls.
const wheelStep driver.Points = 48

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) empty() bool { return r.Right <= r.Left || r.Bottom <= r.Top }

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	_       uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	_         [32]byte
}

// view is the main thread's copy of one mounted view.
type view struct {
	window   *window
	id       driver.NodeID
	kind     driver.ViewKind
	parent   *view
	children []*view
	frame    driver.Bounds
	style    driver.ViewStyle
	events   driver.EventMask
	props    driver.ViewProps
	overflow driver.Bounds // where its children reach outside it (ItemOverflow)
	deleted  bool
	// transformedKids is ItemOverflow's Transformed: a child may be drawn transformed, outside overflow.
	transformedKids bool

	// A text view's layout and where it goes in the view, its content
	// and the UTF-16 range of it a selection highlights.
	text             drawnText
	content          driver.Bounds
	plain            string
	spans            []driver.TextSpan
	selStart, selEnd int

	// The shadow drawn under the background, and the box shadows, by
	// what each depends on; shadowsUsed are those the last pass drew.
	shadow      *shadow
	boxShadows  map[boxShadowKey]*boxShadow
	shadowsUsed map[boxShadowKey]bool
	// keyframeStart is when the keyframe animation of style began.
	keyframeStart time.Time

	// A scroll view's configuration, content size and offset.
	scroll driver.ScrollConfig
	size   driver.Size
	offset driver.Point
	// applied counts the core's ItemScrollTo and ItemScrollBy items,
	// reported with each offset; sticky are a scroll view's sticky cells,
	// and laid and shift a sticky cell's frame from the core and how far
	// it is moved from it.
	applied uint32
	sticky  scrollbar.StickyCells
	laid    driver.Bounds
	shift   driver.Point
	wheel   scrollbar.Wheel // a snapping view's wheel turns
	// glide is how a scroll view a finger let go comes to rest,
	// refreshing whether it keeps the refresh room and spinning whether
	// its spinner turns (touchscroll.go).
	glide                *scrollbar.Motion
	refreshing, spinning bool
	// The bar: when it last showed, its opacity, and whether its fade
	// runs on the frame clock.
	barShown   time.Time
	barAlpha   float32
	barTicking bool

	// An input's editor.
	field *textInput

	// An image view's configuration, source and placeholder.
	image               driver.ImageConfig
	source, placeholder imageSlot

	// A canvas's drawing, and the command lists it plays into.
	drawing     *driver.Drawing
	canvasCache canvasCache
	// shaderView is a shader view's Direct3D state.
	shaderView *shaderView
	fx         *effect // its effect, transition or navigation shader (effect.go)
	// glass is the textures its Liquid Glass draws through.
	glass *glassView

	// A web view's WebView2 controller and portal, a video's media engine.
	web   *webView
	video *video

	// A native view's component.
	component *component

	// What animates: the style, after the first, and the scroll offset.
	styled     bool
	born       uint64 // the batch that created it
	transition *transition
	scrolling  *scrollbar.OffsetAnimation
	// animated are the values bound to the view, Set 0 when none; drawn is
	// its style with them, as styleOf last made it.
	animated driver.AnimatedValues
	drawn    driver.ViewStyle
	// What a LayoutAnimation animates: the frame drawn, moving to frame,
	// and the view appearing or going.
	moving *moving
	fading *fading
	// leaving are the deleted children a LayoutAnimation keeps drawing
	// while they go, by where they were; leftFrom is the view a leaving
	// view is drawn in. A retired view is one of those or in one: its ID
	// is free and it only draws.
	leaving  []leaving
	leftFrom *view
	retired  bool

	// area and tree are where the last frame painted the view and its
	// subtree, in the window's points; queued says the next frame draws
	// them again, below that it measures a view under it (damage.go).
	area, tree    rectF
	queued, below bool
}

// detach takes v out of its parent's children.
func (v *view) detach() {
	if p := v.parent; p != nil {
		if i := slices.Index(p.children, v); i >= 0 {
			p.children = slices.Delete(p.children, i, i+1)
		}
		v.parent = nil
	}
}

type window struct {
	testID       uint32  // the opened test surface, 0 for a regular window
	surfaceID    uint32  // the engine surface, independent of the test host
	parentWindow *window // WindowOptions.Parent, separate from the app service host
	batch        uint64  // batches applied
	hwnd         uintptr
	title        string
	saves        driver.SavePreflights
	devFrame     devWindowFrame

	powerNotification uintptr
	// primary is the backend's own surface. Opened surfaces share its engine
	// and message loop, but closing one must not end the process.
	primary       bool
	hostWindow    *window // root window for services shared by an opened surface
	kind          driver.SurfaceKind
	destroying    bool
	sessionEnding bool
	hideOnBlur    bool
	noActivate    bool
	visible       bool
	// pointerMode is where the window takes the pointer (windowpointer.go).
	pointerMode driver.WindowPointerEvents
	focused     bool
	// anchor is the tray icon a panel last showed under, 0 for none, and anchorBounds and placement where core.Window.ShowAt last placed it, placement WindowPlaceDefault for nowhere. noActivate keeps a panel from ever taking the focus.
	anchor       uint32
	anchorBounds driver.Bounds
	placement    driver.WindowPlacement
	// language is what the menus say (driver.ItemDirection).
	language string
	// loopback are the auth sessions the default browser shows.
	loopback loopback.Sessions
	// rtl is the app's direction (driver.ItemDirection), which the menus
	// lay out in.
	rtl bool
	// damage is what the next frame draws again (damage.go).
	damage damage
	// hud is the performance panel, when the app shows one (perf.go).
	hud *hud
	// embedded is set for a host's child window (NewView): the host owns
	// the top-level window, its chrome and the message loop. app and
	// opts start its engine once it first has a size. The skip flags drop
	// characters the host translated before the child consumed their keys.
	embedded    bool
	app         engine.App
	opts        Options
	skipChar    bool
	skipSysChar bool
	engine      *engine.Engine
	audio       *windowAudio
	batches     queue.Batches
	// applying is set while batches are applied; resized is the commit
	// at a size resize stopped waiting for, until it lands.
	applying bool
	resized  <-chan struct{}
	scale    float64 // pixels per point
	// placing is set while setWindowPosAt moves the window to pixels it chose (wmDPIChanged).
	placing bool

	// What other goroutines hand the main thread.
	mu    sync.Mutex
	tasks []func()

	// The main thread's.
	views []*view // index is NodeID
	root  driver.NodeID
	// effects are the views with an effect, a transition or a navigation
	// shader (effect.go).
	effects       map[*view]bool
	customCursors *customCursorCache
	colorPicker   *winColorPicker
	env           driver.Environment // the last reported
	alerts        []driver.AlertConfig
	alerting      bool
	// alertShown is the alert showing, for a test host to answer; alert
	// is how it is drawn, and alertRestore the field it took the focus
	// from (alert.go).
	alertShown   driver.AlertConfig
	alert        *shownAlert
	alertRestore *textInput
	// alertWindow is set while a hidden root window shows as an app-level alert's own window.
	alertWindow bool
	// The open and save dialogs and context menus waiting for a message
	// of their own, and whether one shows.
	dialogs  []func()
	inDialog bool
	// folderPick is the folder dialog showing, which wmFolderChange
	// corrects (system.go).
	folderPick    *comObject
	popups        []popupRequest
	popupTracking bool
	// popupChoice is the item a test chose in the menu tracking, which
	// ends its tracking (testhost.go).
	popupChoice *uint32
	// popupShown are the items of the context menu tracking.
	popupShown []driver.MenuEntry
	trays      map[uint32]*trayIcon
	// trayID links a hidden drawn-icon surface to its root's notification icon.
	trayID          uint32
	trayDrawPending bool
	// menuOwner is the hidden window that owns the tray icons' menus of a shown root window.
	menuOwner uintptr
	// ownPerf is the performance monitor of a hidden root window, which the first window it opens shows.
	ownPerf *engine.PerfMonitor
	// badge and progress are what the window's taskbar button shows, kept to show once the button is in place and again after Explorer starts again; buttonReady says the button is in place (taskbarButtonCreated). appProgress, on a root window, is the progress its surfaces without a button of their own set, which the app's windows show while they have none (setTaskProgress).
	badge                 uint32
	progress, appProgress driver.TaskProgress
	buttonReady           bool
	// appearance is the one the app forces, or the system's; host the one
	// an embedding host sets in place of the system's.
	appearance, host driver.Appearance
	keys             []driver.KeyBinding // the keyboard shortcuts
	hotKeys          map[uint32]struct{} // the registered global shortcuts
	animating        []*view
	ticking          bool
	clock            *frameClock // made as it first ticks
	// frameWanted is set while the app waits for a display frame, by the
	// engine goroutine; clockOn mirrors ticking for it, as a running clock
	// serves the frame without a message to the window.
	frameWanted, clockOn atomic.Bool
	chrome               chrome
	webs                 map[*webView]bool
	cookies              *cookieView            // the hidden web view of the cookie requests, nil for none
	cacheDir             string                 // where web views keep their profile
	frames               []func(time.Time) bool // frame clients (onFrame)
	// animator runs the animated values on the frame clock, whose times
	// count from epoch.
	animator *driver.Animator
	epoch    time.Time
	// canvasImages are the images canvases draw, by ImageSource.Key.
	canvasImages   map[string]*imageSlot
	canvasKept     map[string]int
	canvasImageUse uint64
	// canvasPass is canvasImageUse when the last paint ended: images used since then are the current paint's, which eviction keeps; canvasPassPosted says an endCanvasPass is posted.
	canvasPass       uint64
	canvasPassPosted bool
	canvasClosed     bool
	// canvasFrames are the pictures of the pixels canvases drew last (canvasFrame).
	canvasFrames      []*imageSlot
	canvasFramePixels int // the pixels of canvasFrames together
	// offscreen caches GPU Direct2D bitmaps for OffscreenCanvas images by ID.
	offscreen          map[uint64]*offscreenEntry
	offscreenRendering map[uint64]bool
	// stickyBound are the scroll views with sticky cells whose insets
	// follow animated values.
	stickyBound scrollbar.StickyBindings[*view]
	// components are the app's native components, hosts the views showing
	// them with a window of their own, and missing the unknown names
	// already logged.
	components driver.NativeComponents
	hosts      map[*component]bool
	missing    map[string]bool
	// drop is the window's file drop target while views take dropped
	// files: drops holds those views.
	drop    *dropTarget
	drops   map[*view]bool
	sharing *shareState

	// composition is the window's while it has web views or a material
	// behind its content, and composed the web views the last frame drew.
	composition *composition
	composed    []*webView
	// baseComposed draws the window's content through DirectComposition, with its alpha, for a material behind it or a transparent window or panel; backdrop is the DWM system backdrop, backdropNone when the material could not be set.
	baseComposed bool
	backdrop     int32
	// material is the material DWM draws, WindowMaterialNone when the
	// window is opaque.
	material driver.WindowMaterial
	// transparent is set while the window or panel shows what is behind it where the app draws nothing (SurfaceConfig.Transparent), and canTransparent once it opened so, which only such a window can be (look.go).
	transparent, canTransparent bool
	// The page holding the mouse while a button is held on it, the one
	// under it, those holding touch and pen contacts, and the video whose
	// control bar holds the mouse.
	rawWeb, hoverWeb *webView
	touchWebs        map[uint32]*webView
	barVideo         *video

	// The gestures in progress.
	pan                            *pan
	pinch, rotation, wheelPinching *twoFinger

	// The scroll views under the pointer, which show their bars; the bar
	// it drags; and whether it asked to hear when it leaves the window.
	bars     []*view
	barDrag  *barDrag
	tracking bool

	// The views under the pointer that follow hovers, and where the
	// pointer is while it is over the client area.
	hovered   hover.Tracker
	held      uintptr // the buttons the last mouse move held down
	pointer   driver.Point
	pointerIn bool

	// The field with the keyboard focus, the one a modal took it from,
	// the one a press is selecting in, and whether the window lacks the
	// keyboard. blinking is set while the caret's timer runs; surrogate
	// holds the first half of a character typed in two WM_CHARs.
	focus, restore, selecting    *textInput
	controlFocus, restoreControl *view
	// textSelection is the selection of an area, and selected the texts
	// that highlight a part of it (selection.go).
	textSelection *textselect.Selection
	selected      []*view
	// touching is the touch that may drag a scroll view (touchscroll.go).
	touching *touchScroll
	// blockNative is set while the touch responder keeps the scroll views from the touches (ItemBlockNative).
	blockNative bool
	// uia are the UI Automation elements made for the views (uia.go).
	uia                map[uint64]*uiaElement
	inactive, blinking bool
	surrogate          uint16
	// The last press, for counting double and triple clicks.
	clicks struct {
		time   uint32
		at     point
		count  int
		target *view
	}
	down driver.Event // the left button's last press: modifiers and count
	// fingers are the views' touches: the mouse as finger 0, a touchscreen's from 1 (touch.go).
	fingers finger.Fingers
	// pressing is the left button's press on a view, as it happens.
	pressing *pressing

	swap *swapChain

	// heldKeys is ItemKeymap's ask for every key's down and up (driver.MountItem.HeldKeys); keysHeld the keys reported going down and not yet up, by physical key (physicalKey).
	heldKeys  bool
	keyLayout uintptr
	keysHeld  map[uint32]heldKey
	// heldMouse is ItemKeymap's ask for every mouse button's down and up (driver.MountItem.HeldMouseButtons); mouseHeld the buttons reported going down and not yet up.
	heldMouse   bool
	mouseHeld   map[driver.MouseButton]heldMouseButton
	mouseClicks map[driver.MouseButton]buttonClicks
}

// Window procedures are process-wide callbacks; they find their window
// here. Only the main thread uses these maps.
var windows = map[uintptr]*window{}

var wndProcCallback = syscall.NewCallback(wndProc)

func wide(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

// launched hands the running app what a second launch asked of it (launchArguments): a Jump List task chooses its item, a launch to open links alone brings the app's window to the front, as opening a link does on macOS, any other launch reopens the app, and the files and links it names open.
func (w *window) launched(args launch) {
	switch {
	case args.Task != "":
		// As choosing a Dock menu item on macOS, it does not reopen the app.
		w.engine.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: args.Task})
	case len(args.Links) > 0 && len(args.Files) == 0:
		// A window the app hid stays hidden: the app's handler shows what the link asks for.
		if visible, _, _ := isWindowVisible.Call(w.hwnd); visible != 0 {
			w.windowCommand(&driver.WindowCommand{Action: driver.WindowOrderFront})
		}
	default:
		w.engine.Dispatch(driver.Event{Kind: driver.EventReopen})
	}
	if len(args.Files) > 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: args.Files})
	}
	if len(args.Links) > 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: args.Links})
	}
}

// Run opens a window showing app and runs the message loop. Call it from
// main; it does not return until the window closes.
func Run(app engine.App, opts Options) {
	comApartment()
	if opts.test == nil {
		setProcessAppUserModelID(opts.ID)
	}
	// The installer's and uninstaller's runs come first: handed off to a running copy, they would never stamp the shortcut or remove the registration.
	if opts.RegisterToast != "" {
		setToastActivatorShortcut(opts.RegisterToast, opts.ID)
		return
	}
	if opts.UnregisterToast {
		unregisterToast(opts.ID)
		return
	}
	// One copy runs per app ID; an app without one, as a test's window, runs as many as it is started.
	var single *instance
	if opts.ID != "" && opts.test == nil {
		var alreadyRunning bool
		if single, alreadyRunning = claimInstance(opts.ID, opts.InstanceKey, launchArguments(opts.DockTask)); alreadyRunning {
			return
		}
		defer single.close()
	}
	registerToastActivator(opts.ID)
	defer unregisterToastActivator()
	if opts.Width == 0 {
		opts.Width, opts.Height = 480, 640
	}
	const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	setDpiAwareness.Call(perMonitorAwareV2)
	registerFonts(opts.Fonts)
	w := newWindow(opts)
	w.primary, w.app, w.opts = true, app, opts
	if !opts.Hidden {
		// A hidden root shows nothing: the windows it opens take the material.
		w.composeFor(opts.Material)
		if opts.Transparent {
			w.composeTransparent()
		}
	}
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		w.scale = float64(dpi) / 96
	}

	instance, className := registerClass()

	// A first guess at the window around the app's size, which placeNew
	// corrects.
	w.chrome.constraints = opts.Constraints
	style := w.frameStyle(wsClipChildren)
	if opts.Hidden {
		// It only ever shows as an alert's window (showAlertWindow): a caption and a close button, without the main window's limits, which the window the app opens for it takes.
		w.chrome.constraints = driver.WindowConstraints{}
		style = wsCaption | wsSysMenu | wsClipChildren
	}
	var ex uintptr
	if w.baseComposed {
		ex = wsExNoRedirect
	}
	if opts.SkipTaskbar && !opts.Hidden {
		ex |= wsExToolWindow
	}
	frame := rect{Right: w.px(opts.Width), Bottom: w.px(opts.Height)}
	adjustWindowRect.Call(uintptr(unsafe.Pointer(&frame)), style, 0)
	hwnd, _, _ := createWindowExW.Call(ex, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(wide(opts.Title))), style,
		cwUseDefault, cwUseDefault, uintptr(frame.Right-frame.Left), uintptr(frame.Bottom-frame.Top),
		0, 0, instance, 0)
	w.hwnd = hwnd
	if w.transparent {
		w.applyBare()
	}
	// Placed before it is the window's, so the sizes this sends do not
	// reach the engine, which starts at the client area the window has.
	w.placeNew(opts.Width, opts.Height, opts.test == nil)
	frameToPlace, fullScreen := opts.Frame, false
	if !opts.Hidden {
		frameToPlace, fullScreen = w.devFrame.restore(frameToPlace)
	}
	if frameToPlace.Move {
		w.placeAt(frameToPlace)
	}
	if fullScreen {
		w.toggleFullScreen()
		// The restored full-screen window is still hidden. Its saved
		// regular style is the one it takes after showing.
		w.chrome.savedStyle |= wsVisible
	}
	windows[hwnd] = w
	hearTaskbarButton(hwnd)
	beginToastActivation(w)
	defer endToastActivation(w)
	// Nothing has the focus yet: typing must not open a composition.
	w.updateIME()
	w.configureGestures()
	if w.baseComposed {
		if w.applyMaterial() {
			w.material = opts.Material
		} else {
			w.backdrop = backdropNone
		}
	}
	w.env = w.environment()
	w.devFrame.start(w)
	defer w.devFrame.close()
	w.appearance = opts.Appearance
	w.darkTitleBar(w.shownDark())
	darkMenus(w.appearance)
	// The engine starts once the window exists, so Mount can post to it.
	var client rect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	size := w.point(client.Right, client.Bottom)
	w.startEngine(app, opts, driver.Size{Width: size.X, Height: size.Y})
	if single != nil {
		single.setDelivery(func(args launch) { w.post(func() { w.launched(args) }) })
	}
	attachToastActivation(w, w.engine)
	if !opts.Hidden {
		showWindow.Call(hwnd, swShow)
	}
	if opts.test != nil {
		opts.test.ready <- w
	} else if opts.DockTask != "" {
		// The core keeps it until the app sets its menu.
		w.engine.Dispatch(driver.Event{Kind: driver.EventDockMenu, Text: opts.DockTask})
	} else {
		// The core keeps them until the app's first handlers register.
		if paths := engine.LaunchFiles(); len(paths) > 0 {
			w.engine.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: paths})
		}
		if links := engine.LaunchLinks(); len(links) > 0 {
			w.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: links})
		}
	}
	if opts.test == nil {
		// Language waits for the app's mount, so WinSparkle starts in the language the app shows.
		startWinSparkle(opts.Updates, w, w.engine.Language)
	}

	var m msg
	for {
		r, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			w.closeAudio()
			unwatchFrontmost()
			stopWinSparkle()
			if !w.sessionEnding {
				w.engine.Close()
				engine.SyncStorage()
			}
			return
		}
		// Shortcuts take key presses before they become characters, in the window the keys go to: this one or a surface the app opened.
		if t := windows[m.Hwnd]; t != nil && (t == w || t.hostWindow == w) {
			t.reportModality(m.Message, m.WParam)
			if (m.Message == wmKeyDown || m.Message == wmSysKeyDown) && (t.keyDown(m.WParam, m.LParam) || t.shortcut(m.WParam, m.LParam)) {
				// A consumed system key still counts as a key with Alt held.
				// Otherwise releasing Alt would look like Alt was pressed alone
				// and toggle an auto-hiding menu bar or popup menu.
				if m.Message == wmSysKeyDown {
					t.chrome.altAlone = false
				}
				continue
			}
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// placeNew sizes a new window so its client area is width by height in
// the DPI of its monitor, measuring what its frame takes rather than
// trusting AdjustWindowRect's system DPI, and centers it in the monitor's
// work area, shrunk to fit it when fit is set.
func (w *window) placeNew(width, height driver.Points, fit bool) {
	if dpi, _, _ := getDpiForWindow.Call(w.hwnd); dpi != 0 {
		w.scale = float64(dpi) / 96
	}
	var outer, client rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	cx := outer.Right - outer.Left + w.px(width) - client.Right
	cy := outer.Bottom - outer.Top + w.px(height) - client.Bottom
	x, y := outer.Left, outer.Top
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	monitor, _, _ := monitorFromWindow.Call(w.hwnd, monitorDefaultNear)
	if r, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); r != 0 {
		work := mi.Work
		workW, workH := work.Right-work.Left, work.Bottom-work.Top
		if fit {
			cx, cy = min(cx, workW), min(cy, workH)
		}
		x, y = work.Left+max(workW-cx, 0)/2, work.Top+max(workH-cy, 0)/2
	}
	setWindowPos.Call(w.hwnd, 0, uintptr(x), uintptr(y), uintptr(cx), uintptr(cy), swpNoZOrder|swpNoActivate)
}

// placeAt moves the new window to frame f, kept on the monitor nearest it.
// That monitor's scale may be another one than the window started with.
func (w *window) placeAt(f driver.WindowFrame) {
	// Through the monitor the frame is on, as windowFrame reported it, kept in that monitor's work area.
	if placement, ok := placementForFrame(f); ok {
		f = f.OnScreen([]driver.Bounds{placement.display.VisibleFrame})
		x, y := placement.pixel(f.X, f.Y)
		w.setWindowPosAt(x, y, placement.px(f.Width), placement.px(f.Height))
		if dpi, _, _ := getDpiForWindow.Call(w.hwnd); dpi != 0 {
			w.scale = float64(dpi) / 96
		}
		return
	}
	r := rect{Left: w.px(f.X), Top: w.px(f.Y), Right: w.px(f.X + f.Width), Bottom: w.px(f.Y + f.Height)}
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&r)), monitorDefaultNear)
	if ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); ok != 0 {
		corner := w.point(mi.Work.Left, mi.Work.Top)
		size := w.point(mi.Work.Right-mi.Work.Left, mi.Work.Bottom-mi.Work.Top)
		f = f.OnScreen([]driver.Bounds{{Origin: corner, Size: driver.Size{Width: size.X, Height: size.Y}}})
	}
	setWindowPos.Call(w.hwnd, 0, uintptr(w.px(f.X)), uintptr(w.px(f.Y)), uintptr(w.px(f.Width)), uintptr(w.px(f.Height)), swpNoZOrder|swpNoActivate)
	if dpi, _, _ := getDpiForWindow.Call(w.hwnd); dpi != 0 {
		w.scale = float64(dpi) / 96
	}
}

// setWindowPosAt moves and sizes the window to a rect in the virtual desktop's pixels that it chose itself, so a DPI change the move causes keeps it (wmDPIChanged).
func (w *window) setWindowPosAt(x, y, cx, cy int32) {
	w.placing = true
	setWindowPos.Call(w.hwnd, 0, uintptr(x), uintptr(y), uintptr(cx), uintptr(cy), swpNoZOrder|swpNoActivate)
	w.placing = false
}

// composeFor has a window opening with material m draw through DirectComposition behind its backdrop, where the system has both.
func (w *window) composeFor(m driver.WindowMaterial) {
	if m == driver.WindowMaterialNone {
		return
	}
	switch {
	case !systemBackdrops():
		log.Printf("nsgo: window materials need Windows 11 22H2 or later: the window stays opaque")
	case !compositionAvailable():
		log.Printf("nsgo: window materials need DirectComposition: the window stays opaque")
	default:
		w.baseComposed, w.backdrop = true, backdropFor(m)
	}
}

// newWindow returns the state of a window showing an app.
func newWindow(opts Options) *window {
	w := &window{title: opts.Title, scale: 1, webs: map[*webView]bool{}, touchWebs: map[uint32]*webView{}, trays: map[uint32]*trayIcon{},
		epoch: time.Now(), components: opts.Components, hosts: map[*component]bool{}, missing: map[string]bool{},
		drops: map[*view]bool{}}
	w.animator = driver.NewAnimator(func(ev driver.Event) { w.engine.Dispatch(ev) })
	return w
}

// registerClass registers NSGo's window class, once.
func registerClass() (instance uintptr, className *uint16) {
	instance, _, _ = getModuleHandleW.Call(0)
	className = wide("NsgoWindow")
	if classRegistered {
		return instance, className
	}
	classRegistered = true
	cursor, _, _ := loadCursorW.Call(0, idcArrow)
	// nsgo links the app's icon as group 1; without one the system's
	// default stands. Windows picks the small icon from the same group.
	const appIcon = 1
	icon, _, _ := loadIconW.Call(instance, appIcon)
	wc := wndClassEx{
		WndProc:   wndProcCallback,
		Instance:  instance,
		Icon:      icon,
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	return instance, className
}

var classRegistered bool

// startEngine starts app's engine at size.
func (w *window) startEngine(app engine.App, opts Options, size driver.Size) {
	eopts := engine.Options{
		ReadAsset:      imageload.ReadAsset,
		Size:           size,
		Environment:    w.env,
		NetworkWatcher: networkWatcher{},
		Appearance:     opts.Appearance,
		Host:           opts.Host,
		Tray:           !w.embedded,
		Secrets:        credentials{},
	}
	if opts.Hidden {
		eopts.Windows = &engine.WindowOptions{Title: opts.Title, Width: opts.Width, Height: opts.Height,
			Frame: opts.Frame, Material: opts.Material, Constraints: opts.Constraints, Transparent: opts.Transparent,
			NoShadow: opts.NoShadow, SkipTaskbar: opts.SkipTaskbar, Hidden: opts.MainHidden}
	}
	data, cache, logs := engine.AppDirs(opts.ID)
	eopts.SetDirs(data, cache, logs)
	if opts.test != nil {
		eopts.UseTest(opts.test.options, &opts.test.pin)
		cache = eopts.CacheDir
	}
	imageload.UseDiskCache(cache)
	w.cacheDir = cache
	if opts.Perf && opts.test == nil || opts.test != nil && opts.test.options.FPS {
		if opts.Hidden {
			// Shown by the first window the app opens, as on macOS.
			w.ownPerf = engine.NewPerfMonitor()
			eopts.Perf = w.ownPerf
		} else {
			eopts.Perf = w.startPerf()
		}
	}
	w.engine = engine.Start(w, app, eopts)
	w.reportFrameWindow()
	w.watchPower()
}

// openSurface creates an opened window on the backend's message loop and
// attaches its tree to the same engine. The root window owns the loop; a
// child surface only owns its HWND and engine surface.
func (w *window) openSurface(cfg driver.SurfaceConfig) {
	opts := w.opts
	if cfg.Title != "" {
		opts.Title = cfg.Title
	}
	if cfg.Width > 0 {
		opts.Width = cfg.Width
	}
	if cfg.Height > 0 {
		opts.Height = cfg.Height
	}
	opts.Frame, opts.Material, opts.Constraints, opts.Hidden = cfg.Frame, cfg.Material, cfg.Constraints, cfg.Hidden
	// A window or a panel is transparent of its own choice, a panel drawing its own corners and shadow (applyBare), and only a window is off the taskbar of its own, as a panel never shows there.
	opts.Transparent, opts.NoShadow, opts.SkipTaskbar = cfg.Transparent && (cfg.Kind == driver.SurfaceWindow || cfg.Kind == driver.SurfacePanel), cfg.NoShadow,
		cfg.SkipTaskbar && cfg.Kind == driver.SurfaceWindow
	if cfg.Kind == driver.SurfaceTrayIcon {
		// The tree is rendered into the notification icon, never a visible window.
		opts.Hidden = true
	}
	child := newWindow(opts)
	child.surfaceID = cfg.ID
	child.parentWindow = w.surfaceWindow(cfg.Parent)
	child.opts, child.hostWindow, child.kind, child.hideOnBlur, child.noActivate = opts, w, cfg.Kind, cfg.HideOnBlur, cfg.NoActivate
	child.visible = !opts.Hidden
	child.trayID = cfg.Tray
	child.chrome.constraints = opts.Constraints
	child.composeFor(opts.Material)
	if opts.Transparent {
		child.composeTransparent()
	}
	style := child.frameStyle(wsClipChildren)
	var ex uintptr
	if child.baseComposed {
		ex = wsExNoRedirect
	}
	if opts.SkipTaskbar {
		ex |= wsExToolWindow
	}
	if cfg.Kind == driver.SurfacePanel || cfg.Kind == driver.SurfaceTrayIcon {
		style = wsPopup | wsClipChildren
	}
	if cfg.Kind != driver.SurfaceWindow {
		// Panels, as tray flyouts, have no taskbar button or Alt+Tab entry.
		ex |= wsExToolWindow
		if cfg.Kind == driver.SurfacePanel && cfg.NoActivate {
			// One that never takes the focus stays above the window it serves, which keeps it.
			ex |= wsExNoActivate | wsExTopmost
		}
	}
	frame := rect{Right: child.px(opts.Width), Bottom: child.px(opts.Height)}
	if cfg.Kind == driver.SurfaceWindow {
		adjustWindowRect.Call(uintptr(unsafe.Pointer(&frame)), style, 0)
	}
	// Run and NewView register the class before the engine can open a
	// surface. Reuse it directly here; calling registerClass from this path
	// would make wndProc's callback initialization recursive.
	instance, _, _ := getModuleHandleW.Call(0)
	className := wide("NsgoWindow")
	hwnd, _, _ := createWindowExW.Call(ex, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(wide(opts.Title))), style,
		cwUseDefault, cwUseDefault, uintptr(frame.Right-frame.Left), uintptr(frame.Bottom-frame.Top),
		child.parentHandle(), 0, instance, 0)
	if hwnd == 0 {
		return
	}
	child.hwnd = hwnd
	if child.transparent {
		child.applyBare()
	}
	if cfg.Kind == driver.SurfaceWindow {
		child.placeNew(opts.Width, opts.Height, true)
		if opts.Frame.Move {
			child.placeAt(opts.Frame)
		}
	} else {
		// At its monitor's scale, as placeNew sizes a window.
		if dpi, _, _ := getDpiForWindow.Call(hwnd); dpi != 0 {
			child.scale = float64(dpi) / 96
		}
		setWindowPos.Call(hwnd, 0, 0, 0, uintptr(child.px(opts.Width)), uintptr(child.px(opts.Height)), swpNoActivate|swpNoZOrder)
	}
	windows[hwnd] = child
	child.setPointerEvents(cfg.PointerEvents)
	hearTaskbarButton(hwnd)
	child.updateIME()
	child.configureGestures()
	if child.baseComposed {
		if child.applyMaterial() {
			child.material = opts.Material
		} else {
			child.backdrop = backdropNone
		}
	}
	if w.ownPerf != nil && cfg.Kind == driver.SurfaceWindow {
		child.showPerf(w.ownPerf)
		w.ownPerf = nil
	}
	child.env = child.environment()
	child.appearance = opts.Appearance
	child.darkTitleBar(child.shownDark())
	darkMenus(child.appearance)
	var client rect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	size := child.point(client.Right, client.Bottom)
	child.engine = w.engine.Attach(cfg.ID, child)
	if opts.test != nil {
		if opts.test.windows == nil {
			opts.test.windows = map[uint32]*window{}
		}
		child.testID = cfg.ID
		opts.test.windows[cfg.ID] = child
	}
	child.engine.Resize(driver.Size{Width: size.X, Height: size.Y}, driver.Geometry{})
	child.engine.SetEnvironment(child.env)
	child.reportFrameWindow()
	if !opts.Hidden && child.parentVisible() {
		if cfg.Kind == driver.SurfacePanel {
			child.placePanel(0, driver.Bounds{}, driver.WindowPlaceDefault)
			child.orderFront()
		} else {
			showWindow.Call(hwnd, swShow)
		}
	}
}

// View is an app in a child window of an embedding host's window, which
// the host's own message loop runs (NewView).
type View struct{ w *window }

// NewView makes a child window of opts.Parent showing app, for a host
// that runs the message loop itself on this thread, with COM initialized
// (OleInitialize, for the clipboard and drops). The host places it
// (MoveWindow); its engine starts once it first has a size, so settings
// made before that are in the app's first frame. Several views may show
// at once.
func NewView(app engine.App, opts Options) (*View, error) {
	if opts.Parent == 0 {
		return nil, errors.New("a view on Windows is a child window: give the HWND of its parent")
	}
	registerFonts(opts.Fonts)
	if opts.Host.Settings == nil {
		opts.Host.Settings = &engine.HostSettings{}
	}
	w := newWindow(opts)
	w.embedded, w.app, w.opts = true, app, opts
	if dpi, _, _ := getDpiForWindow.Call(opts.Parent); dpi != 0 {
		w.scale = float64(dpi) / 96
	}
	instance, className := registerClass()
	const style = wsChild | wsVisible | wsClipChildren | wsClipSiblings
	hwnd, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0, style, 0, 0, 0, 0,
		opts.Parent, 0, instance, 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("no child window: %v", err)
	}
	w.hwnd = hwnd
	windows[hwnd] = w
	w.updateIME()
	w.configureGestures()
	w.env = w.environment()
	w.appearance = opts.Appearance
	return &View{w}, nil
}

// Handle returns the child window's HWND, which the host places.
func (v *View) Handle() uintptr { return v.w.hwnd }

// SetHostEnvironment sets what the host sets over the system's
// environment: its locale, text size and appearance, which the fields
// NSGo draws take too.
func (v *View) SetHostEnvironment(env engine.HostEnvironment) {
	v.w.opts.Host.Settings.SetEnvironment(env)
	v.w.host = env.Appearance
	v.w.redrawAll()
}

// SetTokens sets the values the host themes the app with.
func (v *View) SetTokens(t engine.Tokens) { v.w.opts.Host.Settings.SetTokens(t) }

// Close stops the app, destroys its native components and the child
// window.
func (v *View) Close() {
	if v.w.hwnd != 0 {
		destroyWindow.Call(v.w.hwnd)
	}
}

// testOpen is what a test host opens the app with: the test's options,
// the environment it pins, and where the window goes once it exists.
type testOpen struct {
	options testhost.Options
	pin     engine.TestPin
	ready   chan *window
	windows map[uint32]*window // opened surfaces, on the window thread
}

// chromeItem applies an item of the window's own: its title, a window
// command or the menu bar.
func (w *window) chromeItem(it *driver.MountItem) {
	switch it.Kind {
	case driver.ItemTitle:
		w.title = it.Value
		setWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(wide(it.Value))))
	case driver.ItemWindow:
		if it.Window != nil {
			w.windowCommand(it.Window)
		}
	case driver.ItemMenu:
		w.setMenu(it.Menu)
	case driver.ItemDockMenu:
		w.setDockMenu(it.Menu)
	case driver.ItemRecentFile:
		w.recentFile(it.Value, it.On)
	}
}

// owner is the top-level window the app's dialogs and menus belong to:
// its own, or the host's.
func (w *window) owner() uintptr {
	if !w.embedded {
		return w.hwnd
	}
	const gaRoot = 2
	r, _, _ := getAncestor.Call(w.hwnd, gaRoot)
	return r
}

// Mount implements engine.Platform.
func (w *window) Mount(b *driver.MountBatch) {
	if w.batches.Push(b) {
		postMessageW.Call(w.hwnd, wmApply, 0, 0)
	}
}

// RequestFrame implements engine.Platform: the app waits for the next
// display frame. Called from the engine goroutine, it posts to the window,
// whose frame clock runs on the UI thread, only to start the clock: an app
// asking every frame keeps it running, and its next tick serves the frame.
func (w *window) RequestFrame() {
	w.frameWanted.Store(true)
	if !w.clockOn.Load() {
		postMessageW.Call(w.hwnd, wmFrame, 0, 0)
	}
}

func (w *window) SubmitGPU(driver.GPUFrame) {}

// applyBatches applies the batches the engine has sent, in order, unless
// a batch is being applied: one can resize the window.
func (w *window) applyBatches() {
	if w.applying {
		return
	}
	w.applying = true
	defer func() { w.applying = false }()
	done := w.measuring(true)
	for _, b := range w.batches.Drain() {
		if w.hud != nil && !b.Sent.IsZero() {
			w.hud.monitor.RecordLatency(time.Since(b.Sent))
		}
		driver.Apply(b, w.apply)
	}
	done()
}

// resizeWait is the longest the main thread waits for the layout at a new
// size, the one place it waits for the engine, as the macOS backend does:
// DWM shows the window at its new size at once, around the last frame
// otherwise.
const resizeWait = 16 * time.Millisecond

// resize tells the engine the window's new size and waits, at most
// resizeWait, for the commit laid out at it, which it applies and draws
// before the message returns. An engine that missed the deadline is
// not waited for again until that commit lands, so a busy app resizes as
// before.
func (w *window) resize(size driver.Size) {
	if w.resized != nil {
		select {
		case <-w.resized:
			w.resized = nil
		default:
		}
	}
	done := w.engine.ResizeAndWait(size, driver.Geometry{})
	if w.resized != nil || w.applying {
		return
	}
	t := time.NewTimer(resizeWait)
	defer t.Stop()
	select {
	case <-done:
		w.applyBatches()
		if visible, _, _ := isWindowVisible.Call(w.hwnd); visible != 0 {
			w.frame()
		}
	case <-t.C:
		w.resized = done
	}
}

// post runs fn on the main thread; any goroutine may call it.
func (w *window) post(fn func()) {
	w.mu.Lock()
	wake := len(w.tasks) == 0
	w.tasks = append(w.tasks, fn)
	w.mu.Unlock()
	if wake {
		postMessageW.Call(w.hwnd, wmRun, 0, 0)
	}
}

func (w *window) runTasks() {
	w.mu.Lock()
	fns := w.tasks
	w.tasks = nil
	w.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func (w *window) px(p driver.Points) int32 { return int32(math.Round(float64(p) * w.scale)) }

func (w *window) point(x, y int32) driver.Point {
	return driver.Point{X: driver.Points(float64(x) / w.scale), Y: driver.Points(float64(y) / w.scale)}
}

func (w *window) view(id driver.NodeID) *view {
	if int(id) < len(w.views) {
		return w.views[id]
	}
	return nil
}

func (w *window) apply(b *driver.MountBatch) {
	w.batch++
	var rescroll, loads []*view
	structure := false // views came or went, for UI Automation
	recursor := false  // a text's span cursors changed
	lb := w.layoutBatch(b)
	type focusChange struct {
		v     *view // nil dismisses the keyboard
		focus bool
	}
	var focus []focusChange
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate:
			structure = true
			w.create(it.ID, it.View)
			if lb != nil {
				lb.created[w.views[it.ID]] = true
			}
			continue
		case driver.ItemAnimation:
			w.applyAnimations(it.Animation)
			continue
		case driver.ItemAlert:
			// Shown from a message of its own: the dialog's message loop
			// must not run inside this one's batch.
			w.alerts = append(w.alerts, *it.Alert)
			postMessageW.Call(w.hwnd, wmAlert, 0, 0)
			continue
		case driver.ItemDismissKeyboard:
			focus = append(focus, focusChange{})
			continue
		case driver.ItemBlockNative:
			w.blockNative = it.ID != 0
			continue
		case driver.ItemSetClipboard:
			w.setClipboard(it.Value)
			continue
		case driver.ItemReadClipboard:
			w.readClipboard(it)
			continue
		case driver.ItemDiskSpace:
			go w.diskSpace(it.Value, it.Count)
			continue
		case driver.ItemSetClipboardImage:
			if data, ok := it.Data.([]byte); ok {
				w.setClipboardImage(data)
			}
			continue
		case driver.ItemOpenURL:
			w.openURL(it.Value, it.Count)
			continue
		case driver.ItemSensor:
			if it.Sensor.Command != driver.SensorStop {
				w.engine.Dispatch(driver.NoSensor(it))
			}
			continue
		case driver.ItemBiometricAvailability, driver.ItemBiometricAuthenticate:
			w.engine.Dispatch(driver.NoBiometric(it))
			continue
		case driver.ItemAuthSession:
			w.authSession(it.Count, *it.Auth)
			continue
		case driver.ItemSpeak:
			w.speak(it.Count, *it.Speak)
			continue
		case driver.ItemWebCookies:
			w.webCookies(it.Count, *it.Cookies)
			continue
		case driver.ItemOpenSurface:
			if it.Surface != nil {
				w.openSurface(*it.Surface)
			}
			continue
		case driver.ItemNotificationPermission, driver.ItemShowNotification, driver.ItemCloseNotification, driver.ItemBadge, driver.ItemTaskProgress:
			w.notify(it)
			continue
		case driver.ItemHotKeys:
			w.hotKeysApply(it.Keys)
			continue
		case driver.ItemPermission:
			w.permission(it.Count, *it.Permission)
			continue
		case driver.ItemKeepAwake:
			if it.Awake != nil {
				w.keepAwake(it.Count, *it.Awake)
			}
			continue
		case driver.ItemReleaseAwake:
			w.releaseAwake(it.Count)
			continue
		case driver.ItemReleaseOffscreen:
			w.releaseOffscreen(it.OffscreenID)
			continue
		case driver.ItemCheckForUpdates:
			w.checkForUpdates(it.On, it.Count)
			continue
		case driver.ItemPointerLock:
			w.setPointerLock(it)
			continue
		case driver.ItemPickImages:
			w.pickImages(*it.Pick, it.Count)
			continue
		case driver.ItemPickFiles:
			w.pickFiles(*it.Files, it.Count)
			continue
		case driver.ItemPickDirectories:
			w.pickDirectories(*it.Directories, it.Count)
			continue
		case driver.ItemPickScreenColor:
			w.pickScreenColor(it.Count)
			continue
		case driver.ItemOpenFile:
			w.openFile(*it.Open, it.Count)
			continue
		case driver.ItemRevealFile:
			w.revealFile(*it.Open, it.Count)
			continue
		case driver.ItemTrashFile:
			w.trashFile(*it.Open, it.Count)
			continue
		case driver.ItemInstallPackage:
			w.engine.Dispatch(driver.NoPackageInstaller(it.Count))
			continue
		case driver.ItemShare:
			w.share(*it.Share, it.Count)
			continue
		case driver.ItemSaveFile:
			w.saveFile(*it.Save, it.Count)
			continue
		case driver.ItemSavePreflightAnswer:
			w.saves.Answer(*it)
			continue
		case driver.ItemPopupMenu:
			w.popupMenu(it.Menu, it.Point, it.Count)
			continue
		case driver.ItemTray:
			w.applyTray(*it.Tray)
			continue
		case driver.ItemTrayMenu:
			w.trayMenu(it.Tray.ID, it.Menu, it.Count)
			continue
		case driver.ItemCursorPoint:
			p, ok := cursorPoint()
			w.engine.Dispatch(driver.Event{Kind: driver.EventCursorPoint, Count: it.Count, Point: p, Finished: ok})
			continue
		case driver.ItemLaunchAtLogin:
			w.engine.Dispatch(driver.Event{Kind: driver.EventLaunchAtLogin, Count: it.Count, Finished: launchesAtLogin(w.opts.ID)})
			continue
		case driver.ItemSetLaunchAtLogin:
			setLaunchAtLogin(it.On, w.opts.ID)
			continue
		case driver.ItemDock:
			// Windows has no Dock policy.
			continue
		case driver.ItemRequestReview:
			// Windows has no in-app store review prompt.
			continue
		case driver.ItemAppearance:
			w.setAppearance(it.Appearance)
			continue
		case driver.ItemDirection:
			// An embedding host owns its thread and process UI languages.
			if !w.embedded {
				forced := ""
				if it.Forced {
					forced = it.Language
				}
				setForcedUILanguage(forced)
			}
			if w.rtl != it.RTL || w.language != it.Language {
				w.rtl, w.language = it.RTL, it.Language
				if !w.embedded {
					w.showMenuBar()
				}
			}
			continue
		case driver.ItemQuit:
			w.finishQuit()
			continue
		case driver.ItemQuitResponse:
			if !answerUpdateQuit(w, it) && it.On {
				w.finishQuit()
			}
			continue
		case driver.ItemImageInfo:
			w.imageInfo(it)
			continue
		case driver.ItemKeepImage:
			w.keepCanvasImage(it.Value)
			continue
		case driver.ItemReleaseImage:
			w.releaseCanvasImage(it.Value)
			continue
		case driver.ItemAudio:
			w.audioCommand(it.Count, it.Audio)
			continue
		case driver.ItemKeymap:
			started := it.HeldKeys && !w.heldKeys
			w.setKeymap(it.Keys, it.HeldKeys, it.HeldMouseButtons)
			if started {
				w.updateEnvironment()
			}
			continue
		case driver.ItemTitle, driver.ItemWindow, driver.ItemMenu, driver.ItemDockMenu, driver.ItemRecentFile:
			// An embedding host owns its window and menu bar.
			if !w.embedded {
				w.chromeItem(it)
			}
			continue
		case driver.ItemAnnounce:
			w.uiaAnnounce(it.Value)
			continue
		case driver.ItemInputSource:
			setInputSource(it.Value)
			continue
		case driver.ItemAccessibilityLabel:
			if v := w.view(it.ID); v != nil {
				held := w.uiaHeld(v)
				v.props.Label = it.Value
				w.uiaChanged(held)
				w.accessChanged(v, true)
			}
			continue
		case driver.ItemAccessibilityFocus:
			if v := w.view(it.ID); v != nil && v.field != nil {
				w.setFocus(v.field)
			}
			continue
		case driver.ItemStatusBar, driver.ItemBackUnhandled, driver.ItemBackPreview, driver.ItemHistory:
			// No status bar; Escape is back.
			continue
		}
		v := w.view(it.ID)
		if v == nil {
			continue
		}
		switch it.Kind {
		case driver.ItemEvents, driver.ItemViewProps, driver.ItemNativeCommand, driver.ItemWebCommand,
			driver.ItemOverflow:
		default:
			// Where it was, where it goes, or what it draws.
			w.redrawView(v)
		}
		switch it.Kind {
		case driver.ItemInsert:
			if p := w.view(it.Parent); p != nil {
				v.detach()
				at := len(p.children)
				if before := w.view(it.Before); before != nil {
					if j := slices.Index(p.children, before); j >= 0 {
						at = j
					}
				}
				p.children = slices.Insert(p.children, at, v)
				v.parent = p
				if lb != nil && lb.created[v] {
					lb.inserted = append(lb.inserted, v)
				}
			}
		case driver.ItemRemove:
			v.detach()
		case driver.ItemDelete:
			w.animator.Forget(v.id)
			w.forgetElements(v)
			structure = true
			if !w.leave(v, lb) {
				w.destroy(v)
			}
		case driver.ItemOverflow:
			v.overflow, v.transformedKids = it.Frame, it.Transformed
		case driver.ItemMatch:
			w.match(v, w.view(it.Before), lb)
		case driver.ItemFrame:
			w.setFrame(v, it, lb)
			if v.kind == driver.KindScroll {
				rescroll = append(rescroll, v)
			}
		case driver.ItemViewStyle:
			if v.style.Cursor != it.ViewStyle.Cursor || !v.style.CursorImage.Equal(it.ViewStyle.CursorImage) || v.style.Hidden != it.ViewStyle.Hidden {
				recursor = true
			}
			w.setStyle(v, it.ViewStyle)
			if v.web != nil {
				v.web.restyle()
			}
			if v.kind == driver.KindImage {
				// Untinted, an animation plays again.
				w.play(v, &v.source)
				w.play(v, &v.placeholder)
			}
		case driver.ItemText:
			if v.text != nil {
				v.text.Release()
			}
			v.text, _ = it.Text.Layout.(drawnText)
			v.content = it.Text.Content
			if driver.HasSpanCursor(v.spans) || driver.HasSpanCursor(it.Text.Spans) {
				recursor = true // the cursor over a still pointer may change with the spans
			}
			v.spans = it.Text.Spans
			if v.plain != it.Text.Text {
				held := w.uiaHeld(v)
				v.plain = it.Text.Text
				w.uiaChanged(held)
				w.accessChanged(v, true)
			}
		case driver.ItemInput:
			if v.field != nil && it.Input != nil {
				v.field.configure(it.Input)
			}
		case driver.ItemInputValue:
			if v.field != nil {
				v.field.setValue(it.Value, it.Count)
			}
		case driver.ItemSelection:
			if v.field != nil {
				v.field.selectRange(it.Selection)
			}
		case driver.ItemEvents:
			v.events = it.Events
			if v.field != nil {
				v.field.fs.Events = it.Events
			}
			w.dropEvents(v)
		case driver.ItemViewProps:
			if it.ViewProps != nil {
				held := w.uiaHeld(v)
				v.props = *it.ViewProps
				w.uiaChanged(held)
			}
		case driver.ItemScrollConfig:
			old, inset := scrollbar.RestInset(&v.scroll, v.refreshing), scrollbar.RestInset(it.Scroll, v.refreshing)
			to := v.offset
			if old != inset {
				to = scrollbar.KeepStart(old, inset, to)
			}
			v.scroll = *it.Scroll
			if v.scroll.HideIndicator {
				v.barAlpha = 0
				w.redrawView(v)
			}
			if to != v.offset {
				w.moveOffset(v, to)
			}
			w.refreshChanged(v)
			rescroll = append(rescroll, v) // insets change the offsets
		case driver.ItemScrollContent:
			v.size = it.Size
			rescroll = append(rescroll, v)
		case driver.ItemFocus, driver.ItemBlur:
			focus = append(focus, focusChange{v, it.Kind == driver.ItemFocus})
		case driver.ItemImage:
			if w.setImage(v, it.Image) {
				loads = append(loads, v)
			}
		case driver.ItemModal:
			// A modal is a view; it appears without animation.
		case driver.ItemCanvas:
			held := w.uiaHeld(v)
			had := len(v.canvasNodes())
			v.drawing = it.Drawing
			w.forgetParts(v.id, len(v.canvasNodes())+1, had)
			w.uiaChanged(held)
			w.animator.BindCanvas(v.id, it.Drawing.Slots)
		case driver.ItemShader:
			if v.kind == driver.KindShader && it.Shader != nil && it.Shader.Role == driver.ShaderFill {
				w.setShader(v, it.Shader)
			} else {
				w.setEffect(v, it.Shader)
			}
		case driver.ItemShaderTransition:
			w.startTransition(v)
		case driver.ItemShaderInput:
			if v.fx != nil {
				w.redrawAll()
			}
		case driver.ItemWeb:
			if v.kind == driver.KindWeb && it.Web != nil {
				if v.web == nil {
					v.web = w.newWeb(v, it.Web)
				}
				v.web.configure(it.Web)
			}
		case driver.ItemWebCommand:
			if v.web != nil {
				v.web.command(it.WebCommand, it.Value, it.Data)
			}
		case driver.ItemVideo:
			if v.kind == driver.KindVideo && it.Video != nil {
				if v.video == nil {
					v.video = &video{w: w, v: v}
				}
				v.video.configure(it.Video)
			}
		case driver.ItemVideoSeek:
			if v.video != nil {
				v.video.seek(it.Time)
			}
		case driver.ItemNative:
			if v.kind == driver.KindNative && it.Native != nil {
				w.setNative(v, it.Native)
			}
		case driver.ItemNativeCommand:
			if c := v.component; c != nil && c.c != nil {
				c.c.Command(it.Value, it.Data)
			}
		case driver.ItemScrollBy:
			v.applied++
			w.scrollBy(v, it.Point)
		case driver.ItemStickyCells:
			w.setSticky(v, it.Sticky)
		case driver.ItemScrollTo:
			v.applied++
			w.interruptScroll(v)
			if it.Animated {
				w.scrollAnimated(v, it.Point)
			} else {
				w.scrollTo(v, it.Point)
			}
		}
	}
	if w.root != b.Root {
		w.redrawAll()
	}
	w.root = b.Root
	w.appear(lb)
	// Bindings change with the batch's views.
	w.animatedChanged()
	w.updateDrop()
	for _, v := range rescroll {
		// Late commits retain the refresh room even after its animation ends.
		if !v.deleted {
			if to := scrollbar.ClampRefreshing(&v.scroll, v.frame.Size, v.size, v.offset, v.refreshing); to != v.offset {
				w.moveOffset(v, to)
			}
		}
	}
	for _, v := range loads {
		if !v.deleted {
			w.load(v, &v.placeholder, false)
			w.load(v, &v.source, true)
		}
	}
	// The batch's web views asked for their environments' app schemes.
	startWebEnvironments()
	// Focus once the batch placed the fields.
	for _, f := range focus {
		switch {
		case f.v == nil:
			w.blurAll()
		case f.v.deleted:
		case f.focus:
			w.focusView(f.v)
		case w.focusedView() == f.v:
			w.setFocus(nil)
		}
	}
	w.checkFocus()
	w.checkControlFocus()
	w.placeIME()
	// Views that moved, appeared or went under a still pointer.
	w.hoverViews()
	if recursor {
		w.refreshCursor()
	}
	w.refreshSelection()
	if structure {
		w.structureChanged()
	}
}

func (w *window) create(id driver.NodeID, kind driver.ViewKind) {
	for len(w.views) <= int(id) {
		w.views = append(w.views, nil)
	}
	// A new view has the default style and no props or events: the core
	// sends them only when they differ.
	v := &view{window: w, id: id, kind: kind, style: driver.DefaultViewStyle, born: w.batch}
	if kind == driver.KindInput {
		v.field = newTextInput(w, v)
	}
	w.views[id] = v
}

// destroy destroys v. Its children get their own deletes, unless they
// were retired with it: those go with it.
func (w *window) destroy(v *view) {
	v.detach()
	if p := v.leftFrom; p != nil {
		p.leaving = slices.DeleteFunc(p.leaving, func(l leaving) bool { return l.v == v })
		v.leftFrom = nil
	}
	for _, l := range v.leaving {
		l.v.leftFrom = nil
		w.destroy(l.v)
	}
	v.leaving = nil
	children := v.children
	v.children = nil
	for _, c := range children {
		c.parent = nil
		if c.retired {
			w.destroy(c)
		}
	}
	w.release(v)
	if v.field != nil {
		v.field.release() // drawn since it retired, if it did
	}
	if v.text != nil {
		v.text.Release()
		v.text = nil
	}
	v.shadow.release()
	v.shadow = nil
	v.shaderView.release()
	w.endEffect(v)
	v.glass.release()
	v.shaderView = nil
	v.canvasCache.release()
	for _, sh := range v.boxShadows {
		sh.release()
	}
	v.boxShadows, v.shadowsUsed = nil, nil
	v.retired, v.moving, v.fading, v.drawing = false, nil, nil, nil
	v.source.reset(driver.ImageSource{})
	v.placeholder.reset(driver.ImageSource{})
	v.deleted = true
	w.hovered.Forget(v.id)
	if w.views[v.id] == v {
		w.views[v.id] = nil
	}
}

// retire frees v's ID, as a delete does, but keeps what it draws for a
// layout animation to draw it going: what takes input or plays goes at
// once.
func (w *window) retire(v *view) {
	w.release(v)
	v.retired, v.deleted = true, true
	v.scrolling = nil
	if w.views[v.id] == v {
		w.views[v.id] = nil
	}
}

// release lets go of what a deleted view takes input with, plays or
// hosts, once.
func (w *window) release(v *view) {
	if v.retired {
		return // it did when it retired
	}
	w.dropGone(v)
	if v.field != nil {
		if w.selecting == v.field {
			w.fieldUp()
		}
		v.field.release()
	}
	if v.web != nil {
		v.web.close()
		v.web = nil
	}
	if v.video != nil {
		v.video.close()
		v.video = nil
	}
	if v.component != nil {
		w.closeComponent(v.component)
		v.component = nil
	}
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// scrollTo moves v's content to offset, clamped to the content, and
// reports whether it moved. The engine hit tests with the offset, so a
// move is always reported, whatever v's events.
func (w *window) scrollTo(v *view, offset driver.Point) bool {
	offset = scrollbar.Clamp(&v.scroll, v.frame.Size, v.size, offset)
	if offset == v.offset {
		return false
	}
	w.moveOffset(v, offset)
	return true
}

// moveOffset puts v's content at offset, unclamped, and reports it.
func (w *window) moveOffset(v *view, offset driver.Point) {
	v.offset = offset
	if scrollMoveShowsIndicator(v) {
		w.showBar(v)
	}
	w.redrawView(v)
	// Values bound to the offset move in the frame that shows it.
	if w.animator.Scroll(v.id, offset) {
		w.animatedChanged()
	}
	if w.animator.Pulls(v.id) && w.animator.Pull(v.id, scrollbar.Pulled(&v.scroll, offset)) {
		w.animatedChanged()
	}
	w.pinSticky(v)
	// The engine sees the offset within the content and its insets, as a
	// touch pulls it past.
	w.reportScroll(driver.Event{Kind: driver.EventScroll, ID: v.id, Point: scrollbar.Clamp(&v.scroll, v.frame.Size, v.size, offset), Count: v.applied})
}

// wheelNotch is how far the content under a view taking the wheel moves
// for a notch: three lines of 33 points, as browsers scroll about 100.
const wheelNotch driver.Points = 99

// wheelTaken sends a wheel's turn of delta at p to the innermost view with
// OnWheel under it, unless a scroll view inside it is there, and reports
// whether one took it. A precision touchpad's scrolls are turns of less
// than a notch; Windows sends its momentum on as more of them.
func (w *window) wheelTaken(p driver.Point, delta int16, tilt bool) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	under, _ := viewsAt(root, p, func(v *view) bool { return v.kind == driver.KindScroll || v.events.Has(driver.EventWheel) }, nil)
	if len(under) == 0 {
		return false
	}
	v := under[len(under)-1]
	if v.kind == driver.KindScroll {
		return false
	}
	d := driver.Points(delta) / 120 * wheelNotch
	ev := driver.Event{Kind: driver.EventWheel, ID: v.id, Phase: driver.GestureChanged, Point: v.surfacePoint(p),
		Page: p, Modifiers: modifiers(), Precise: delta%120 != 0}
	// A turn away from the user shows what is above; a tilt right, what
	// is right.
	if tilt {
		ev.Delta.X = -d
	} else {
		ev.Delta.Y = d
	}
	w.engine.Dispatch(ev)
	return true
}

// wheel scrolls the innermost scroll view under at that can still move
// in the wheel's direction. A wheel goes to the views along its axis, as
// a browser's does, so a vertical wheel scrolls the page around a
// horizontal row; horizontal views take a vertical wheel only where none
// under it is vertical. Tilting scrolls horizontal views only.
func (w *window) wheel(at driver.Point, notches driver.Points, tilt bool) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	if !tilt {
		if in := w.fieldAt(at); in != nil && in.wheelScroll(notches) {
			return
		}
	}
	v, to := wheelTurn(scrollsAt(root, at, nil), notches, tilt)
	if v == nil {
		return
	}
	from := v.offset
	w.interruptScroll(v) // the wheel takes over
	if w.scrollTo(v, to) {
		w.showBar(v)
		w.wheelTurned(v, v.offset.X-from.X+v.offset.Y-from.Y)
	}
}

// wheelTurn returns the scroll view a wheel of notches at under turns,
// and where it goes: the innermost view along the wheel's axis that can
// move its way, else the innermost of any axis where none is along it.
func wheelTurn(under []*view, notches driver.Points, tilt bool) (*view, driver.Point) {
	along := false
	for _, v := range under {
		along = along || v.scroll.Horizontal == tilt
	}
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		if v.scroll.Behavior.NoScroll || along && v.scroll.Horizontal != tilt {
			continue
		}
		offset := v.offset
		switch {
		case tilt && v.scroll.Horizontal: // a tilt right is positive
			offset.X += notches * wheelStep
		case tilt:
			continue
		case v.scroll.Horizontal: // a turn away from the user is positive
			offset.X -= notches * wheelStep
		default:
			offset.Y -= notches * wheelStep
		}
		if to := scrollbar.Clamp(&v.scroll, v.frame.Size, v.size, offset); to != v.offset {
			return v, to
		}
	}
	return nil, driver.Point{}
}

// barDrag is the pointer dragging a scroll bar: origin is the scroll
// view's in the window.
type barDrag struct {
	v      *view
	origin driver.Point
	drag   scrollbar.Drag
}

// bar returns a scroll view's or multiline input's scroll bar.
func (v *view) bar() scrollbar.Bar {
	if v.kind == driver.KindInput && v.field != nil && v.field.multiline() {
		return v.field.bar()
	}
	return scrollbar.Bar{Horizontal: v.scroll.Horizontal, Frame: v.frame.Size, Content: v.size, Offset: v.offset,
		Inset: v.scroll.Behavior.ContentInset}
}

// hoverBars shows the bars of the scroll views and multiline inputs under at, the pointer.
func (w *window) hoverBars(at driver.Point) {
	var under []*view
	if root := w.view(w.root); root != nil {
		under = scrollsAt(root, at, nil)
		if v := pick(root, at); v != nil && v.field != nil && v.field.multiline() {
			under = append(under, v)
		}
	}
	if !slices.Equal(under, w.bars) {
		for _, v := range w.bars {
			if !slices.Contains(under, v) {
				w.redrawView(v)
			}
		}
		for _, v := range under {
			if !slices.Contains(w.bars, v) {
				w.redrawView(v)
			}
		}
		w.bars = under
	}
	// The pointer over a view keeps its bar up, as its move does.
	for _, v := range under {
		if !v.scroll.Behavior.NoScroll {
			w.showBar(v)
		}
	}
}

// showsBar reports whether v's bar is visible: its opacity is above zero.
func (w *window) showsBar(v *view) bool {
	return !v.scroll.HideIndicator && v.barAlpha > 0
}

// pressBar starts dragging the scroll bar under at, if there is one.
func (w *window) pressBar(at driver.Point) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	v, local := barAt(root, at)
	if v == nil {
		return false
	}
	w.barDrag = &barDrag{v: v, origin: driver.Point{X: at.X - local.X, Y: at.Y - local.Y}, drag: v.bar().PressIndicator(local)}
	w.showBar(v)
	w.interruptScroll(v)
	w.scrollEvent(v, driver.EventScrollDragStart)
	setCapture.Call(w.hwnd)
	w.dragBar(at)
	return true
}

// dragBar scrolls the dragged bar's view to follow the pointer at at.
func (w *window) dragBar(at driver.Point) {
	d := w.barDrag
	if d.v.deleted {
		w.barDrag = nil
		return
	}
	w.interruptScroll(d.v) // the bar takes over
	w.scrollTo(d.v, d.drag.Offset(driver.Point{X: at.X - d.origin.X, Y: at.Y - d.origin.Y}))
}

func barAt(v *view, p driver.Point) (*view, driver.Point) {
	return viewtree.BarAt(v, p)
}

// scrollsAt appends the scroll views under p, outermost first; p is in
// the coordinates of v's parent's content.
func scrollsAt(v *view, p driver.Point, out []*view) []*view {
	out, _ = viewsAt(v, p, func(v *view) bool { return v.kind == driver.KindScroll }, out)
	return out
}

func viewsAt(v *view, p driver.Point, match func(*view) bool, out []*view) ([]*view, bool) {
	return viewtree.ViewsAt(v, p, match, out)
}

// hoverViews reports the pointer entering and leaving the views that
// follow hovers, as NSH's views hear it from the system.
func (w *window) hoverViews() {
	var under []driver.NodeID
	if root := w.view(w.root); root != nil && w.pointerIn {
		views, _ := viewsAt(root, w.pointer, func(v *view) bool { return hover.Follows(v.events) }, nil)
		for _, v := range views {
			under = append(under, v.id)
		}
	}
	mods, button := modifiers(), hover.Pressed(uint32(w.held), mouseButtons)
	for _, e := range w.hovered.Move(under) {
		if v := w.view(e.ID); v != nil {
			e.Point = v.surfacePoint(w.pointer)
			e.Page, e.Modifiers, e.Button = w.pointer, mods, button
		}
		w.engine.Dispatch(e)
	}
}

// pointerMoved follows the mouse to client pixel x, y, asking to hear
// when it leaves the client area.
func (w *window) pointerMoved(x, y int32) {
	if !w.tracking {
		w.tracking = true
		t := trackMouse{Flags: tmeLeave, Track: w.hwnd}
		t.Size = uint32(unsafe.Sizeof(t))
		trackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
	}
	w.pointer, w.pointerIn = w.point(x, y), true
	w.hoverViews()
}

// touch reports the views' fingers, when ok.
func (w *window) touch(ev driver.Event, ok bool) {
	if ok {
		w.captureTouches(&ev)
		w.engine.Dispatch(ev)
	}
}

// listensTouches reports whether a view listens to touches.
func (w *window) listensTouches() bool {
	for _, v := range w.views {
		if v != nil && finger.Listens(v.events) {
			return true
		}
	}
	return false
}

// mouseButtons are a mouse message's button bits.
var mouseButtons = [5]uint32{mkLButton, mkRButton, mkMButton, mkXButton1, mkXButton2}

// mouseMoved reports the mouse's move to the views under it that follow
// moves, innermost first.
func (w *window) mouseMoved() {
	root := w.view(w.root)
	if root == nil || !w.pointerIn {
		return
	}
	w.reportMouseMove()
}

// reportMouseMove sends a move to where the mouse is to the views under it
// that hear moves, innermost first.
func (w *window) reportMouseMove() {
	root := w.view(w.root)
	under, _ := viewsAt(root, w.pointer, func(v *view) bool { return v.events.Has(driver.EventMouseMove) }, nil)
	if len(under) == 0 {
		return
	}
	mods, button := modifiers(), hover.Pressed(uint32(w.held), mouseButtons)
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		local := v.surfacePoint(w.pointer)
		w.engine.Dispatch(driver.Event{Kind: driver.EventMouseMove, ID: v.id, Point: local, Page: w.pointer, Modifiers: mods, Button: button})
	}
}

// modifierKey reports a modifier key pressed or released while the mouse
// rests over the window as a mouse move to where it is with the new
// modifiers, so a view hears Control come and go under a still mouse.
func (w *window) modifierKey(vk uintptr) {
	switch vk {
	case vkShift, vkControl, vkMenu, vkLWin, vkRWin:
	default:
		return
	}
	root := w.view(w.root)
	if root == nil || !w.pointerIn || keyDown(vkLButton) {
		return
	}
	w.reportMouseMove()
}

// reportModality tells the engine what the user gives input with, a key
// other than a modifier or a button going down, before the focus moves
// the message causes.
func (w *window) reportModality(message uint32, wParam uintptr) {
	if w.engine == nil {
		return
	}
	switch message {
	case wmKeyDown, wmSysKeyDown:
		switch wParam {
		case vkShift, vkControl, vkMenu, vkLWin, vkRWin:
		default:
			w.engine.InputModality(driver.ModalityKeyboard)
		}
	case wmLButtonDown, wmRButtonDown, wmMButtonDown, wmXButtonDown, wmPointerDown:
		w.engine.InputModality(driver.ModalityPointer)
	}
}

// background is the color behind v: its own or its nearest ancestor's.
func background(v *view) driver.Color {
	for ; v != nil; v = v.parent {
		if v.style.Background&0xff != 0 {
			return v.style.Background
		}
	}
	return driver.RGB(0xffffff)
}

// edgeColor is the opaque background of the topmost view that fills the
// window, the root or one of its descendants each filling its parent,
// or the frame's clear color: what a frame shows along its right and
// bottom edges.
func (w *window) edgeColor() driver.Color {
	c := driver.RGB(0xffffff)
	for v := w.view(w.root); v != nil; {
		if v.style.Background&0xff == 0xff {
			c = v.style.Background
		}
		var top *view
		for _, k := range v.children {
			if k != nil && k.frame.Origin == (driver.Point{}) && k.frame.Size == v.frame.Size {
				top = k
			}
		}
		v = top
	}
	return c
}

// pressOutside reports a press at x, y in the client area to each view
// that asks for presses outside it and does not contain the point. It
// does not take the press: the caller handles it as before.
func (w *window) pressOutside(x, y int32) {
	at := w.point(x, y)
	for _, v := range w.views {
		if v == nil || !v.events.Has(driver.EventPressOutside) {
			continue
		}
		if _, shown := w.surfaceBounds(v); shown && !(driver.Bounds{Size: v.frame.Size}).Contains(v.surfacePoint(at)) {
			w.engine.Dispatch(driver.Event{Kind: driver.EventPressOutside, ID: v.id, Point: at})
		}
	}
}

// surfaceBounds returns v's frame in the surface, through its ancestors'
// frames and scroll offsets, and whether v is in the root's tree.
func (w *window) surfaceBounds(v *view) (driver.Bounds, bool) {
	b, top := v.frame, v
	for p := v.parent; p != nil; p = p.parent {
		b.Origin.X += p.frame.Origin.X - p.offset.X
		b.Origin.Y += p.frame.Origin.Y - p.offset.Y
		top = p
	}
	return b, top == w.view(w.root)
}

func (w *window) back() { w.engine.Dispatch(driver.Event{Kind: driver.EventBack}) }

// repeated reports whether a key message's key was already down.
func repeated(lParam uintptr) bool { return lParam&(1<<30) != 0 }

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	w := windows[hwnd]
	// A host's child window has no app until it has a size.
	if w == nil || w.embedded && w.engine == nil && message != wmSize && message != wmDestroy {
		r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	if !w.embedded {
		switch message {
		case wmClose:
			if !w.destroying {
				if w.primary {
					w.requestQuit()
				} else {
					w.engine.Dispatch(driver.Event{Kind: driver.EventWindowClose})
				}
				return 0
			}
		case wmQueryEndSession:
			return 1
		case wmEndSession:
			if wParam != 0 {
				w.appWindow().endSession()
			}
			return 0
		}
	}
	// Closing an alert's window cancels the alert, if it can be, rather than ending the app.
	if message == wmSysCommand && wParam&0xfff0 == scClose && w.alertWindow {
		if s := w.alert; s != nil && s.cancelable {
			w.answerAlert(s.cancel)
		}
		return 0
	}
	if message == taskbarCreated && taskbarCreated != 0 {
		w.readdTrays()
		return 0
	}
	if message == taskbarButtonCreated && taskbarButtonCreated != 0 {
		w.taskbarButtonReady()
		return 0
	}
	if message == wmKeyDown && w.escapePointer(wParam) {
		return 0
	}
	if lockedPointer.owner == w && message == wmMouseMove {
		return 0
	}
	w.reportModality(message, wParam)
	if message == wmMouseActivate && w.kind == driver.SurfacePanel && w.noActivate {
		return maNoActivate
	}
	// An alert takes the pointer and the keys while it shows.
	if w.alert != nil {
		switch message {
		case wmMouseMove, wmLButtonDown, wmLButtonUp:
			x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
			w.alertMouse(message, w.point(int32(x), int32(y)))
			return 0
		case wmRButtonDown, wmRButtonUp, wmMButtonDown, wmMButtonUp, wmMouseWheel, wmChar:
			return 0
		case wmKeyDown:
			if w.alertKey(wParam) {
				return 0
			}
		}
	}
	if message == wmMouseMove {
		w.held = wParam
		w.pointerMoved(int32(int16(lParam&0xffff)), int32(int16(lParam>>16&0xffff)))
		w.mouseMoved()
	}
	switch message {
	case wmMouseMove, wmLButtonDown, wmLButtonUp, wmRButtonDown, wmRButtonUp, wmMButtonDown, wmMButtonUp,
		wmXButtonDown, wmXButtonUp:
		if w.hostedMouse(message, wParam, lParam) {
			return 0
		}
	case wmPointerDown, wmPointerUpdate, wmPointerUp, wmPointerLeave:
		if w.hostedPointer(message, wParam, lParam) {
			return 0
		}
		if message == wmPointerDown {
			if w.touchPointer(message, wParam, lParam) {
				return 0
			}
			w.touchFinger(message, wParam, lParam)
			break
		}
		w.touchFinger(message, wParam, lParam)
		if w.touchPointer(message, wParam, lParam) {
			return 0
		}
	case wmPointerCaptureChanged:
		w.touchFinger(message, wParam, lParam)
	case wmMouseLeave:
		w.mouseLeft()
		return 0
	case wmSetCursor:
		if lParam&0xffff == htClientArea && (w.hostedCursor() || w.fieldCursor() || w.viewCursor()) {
			return 1
		}
	case wmImeSetContext, wmImeStartComposition, wmImeComposition, wmImeEndComposition:
		if r, ok := w.ime(message, wParam, lParam); ok {
			return r
		}
	}
	switch message {
	case wmApply:
		w.applyBatches()
		return 0
	case wmRun:
		w.runTasks()
		return 0
	case wmHotKey:
		id := uint32(wParam)
		if _, registered := w.hotKeys[id]; registered && w.engine != nil {
			w.engine.Dispatch(driver.Event{Kind: driver.EventHotKey, Count: id})
		}
		return 0
	case wmFrame:
		w.startClock()
		return 0
	case wmAlert:
		w.showAlert()
		return 0
	case trayCallbackMessage:
		w.trayMessage(lParam)
		return 0
	case wmDialog:
		w.showDialog()
		return 0
	case wmFolderChange:
		w.leaveVirtualFolder()
		return 0
	case wmPopup:
		w.showPopup()
		return 0
	case wmKeyUp:
		w.keyUp(wParam, lParam)
		w.modifierKey(wParam)
	case wmSysKeyUp:
		// Before the menu bar, which takes Alt's up.
		w.keyUp(wParam, lParam)
		w.modifierKey(wParam)
		if w.menuBarKey(message, wParam, lParam) {
			return 0
		}
	case wmEnterMenuLoop, wmExitMenuLoop:
		// The menu's own loop takes the keys and mouse buttons from here on, their ups too.
		if message == wmEnterMenuLoop {
			w.releaseKeys()
			w.releaseMouseButtons()
		}
		w.menuLoop(message == wmEnterMenuLoop, wParam != 0)
	case wmSettingChange:
		// Theme ("ImmersiveColorSet"), locale and accessibility changes.
		w.updateEnvironment()
	case wmPowerBroadcast:
		if w.powerChanged(wParam, lParam) {
			return 1
		}
	case wmInputLanguageChange:
		w.updateEnvironment()
	case wmDisplayChange:
		for _, other := range windows {
			if other.engine != nil {
				other.updateEnvironment()
			}
		}
	case wmDPIChanged:
		// Under per-monitor awareness Windows leaves the window its pixels: it takes the rect Windows suggests for the new DPI, unless the move is one of the window's own (setWindowPosAt), which chose its pixels already. Either way the engine then lays out the client area in the new points, whether or not a WM_SIZE came before this.
		if dpi := uint32(wParam & 0xffff); dpi != 0 {
			w.scale = float64(dpi) / 96
		}
		if !w.placing && lParam != 0 {
			r := *(**rect)(unsafe.Pointer(&lParam)) // the message's, valid while it is handled
			setWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpNoZOrder|swpNoActivate)
		}
		if w.engine != nil {
			var client rect
			getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			if size := w.point(client.Right, client.Bottom); size.X > 0 && size.Y > 0 {
				w.engine.Resize(driver.Size{Width: size.X, Height: size.Y}, driver.Geometry{})
			}
			w.updateEnvironment()
		}
		return 0
	case wmActivate:
		if w.engine != nil {
			w.post(w.reportFrameWindow)
		}
	case wmRawInput:
		w.rawPointerMotion(lParam)
	case wmActivateApp:
		if wParam == 0 && lockedPointer.owner != nil {
			lockedPointer.owner.releasePointer(true)
		}
		// Sent to each top-level window of the thread: the embedded views
		// on it follow too.
		*appActive() = wParam != 0
		for _, other := range windows {
			if other.engine != nil {
				other.updateEnvironment()
			}
		}
	case wmShowWindow:
		if w.engine != nil {
			w.post(func() {
				w.syncOwnedVisibility()
				w.updateEnvironment()
			}) // once the window shows or hides
		}
	case wmSize:
		w.post(w.syncOwnedVisibility)
		if wParam != sizeMinimized {
			width, height := int32(lParam&0xffff), int32(lParam>>16&0xffff)
			size := w.point(width, height)
			switch {
			case w.engine != nil:
				w.resize(driver.Size{Width: size.X, Height: size.Y})
			case width > 0 && height > 0:
				w.startEngine(w.app, w.opts, driver.Size{Width: size.X, Height: size.Y})
			}
		}
		if w.engine == nil {
			return 0
		}
		// A maximize or restore changes the environment.
		w.updateEnvironment()
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		w.paint()
		return 0
	case wmLButtonDown, wmRButtonDown:
		w.held = wParam & 0xffff
		if w.embedded {
			// A child window takes the keyboard when pressed.
			if f, _, _ := getFocus.Call(); f != hwnd {
				setFocus.Call(hwnd)
			}
		}
		w.hideMenuBar()
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		at := w.point(int32(x), int32(y))
		if message == wmRButtonDown {
			w.mouseButtonDown(driver.MouseButtonRight, at, w.countButtonClicks(driver.MouseButtonRight, lParam))
		}
		if message == wmLButtonDown && w.pressBar(at) {
			return 0
		}
		if message == wmLButtonDown && !touchMouse() {
			// A touchscreen's fingers are reported by their pointers (touch.go).
			w.touch(w.fingers.Down(w.listensTouches(), at))
		}
		w.pressOutside(int32(x), int32(y))
		if message == wmLButtonDown {
			clicks := w.countClicks(lParam)
			if !touchMouse() {
				w.mouseButtonDown(driver.MouseButtonLeft, at, uint32(clicks))
			}
			w.down = driver.Event{Modifiers: modifiers(), Count: uint32(clicks)}
			if w.fieldDown(at, clicks, wParam&mkShift != 0) {
				w.clearSelection()
				return 0
			}
			if w.selectPress(at, clicks, wParam&mkShift != 0) {
				return 0
			}
			w.pressDown(at)
			w.panDown(at)
		}
	case wmRButtonUp:
		w.held = wParam & 0xffff
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		w.mouseButtonUp(driver.MouseButtonRight, w.point(int32(x), int32(y)))
	case wmMButtonDown, wmMButtonUp:
		w.held = wParam & 0xffff
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		at := w.point(int32(x), int32(y))
		if message == wmMButtonDown {
			w.mouseButtonDown(driver.MouseButtonMiddle, at, w.countButtonClicks(driver.MouseButtonMiddle, lParam))
		} else {
			w.mouseButtonUp(driver.MouseButtonMiddle, at)
		}
	case wmXButtonDown, wmXButtonUp:
		w.held = wParam & 0xffff
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		at := w.point(int32(x), int32(y))
		btn := driver.MouseButtonNone
		switch wParam >> 16 & 0xffff {
		case 1:
			btn = driver.MouseButtonBack
		case xButtonForward:
			btn = driver.MouseButtonForward
		}
		if message == wmXButtonDown {
			if w.mouseButtonDown(btn, at, w.countButtonClicks(btn, lParam)) {
				return 1
			}
		} else {
			w.mouseButtonUp(btn, at)
		}
	case wmMouseMove:
		w.setHover(0) // off the caption buttons
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		at := w.point(int32(x), int32(y))
		w.touch(w.fingers.Move(at))
		if w.barDrag != nil {
			w.dragBar(at)
			return 0
		}
		if w.selecting != nil {
			w.fieldMove(at)
			return 0
		}
		if w.selectDrag(at) {
			return 0
		}
		w.hoverBars(at)
		if wParam&mkLButton != 0 {
			w.pressMoved(at)
		}
		if wParam&mkLButton != 0 && w.pan != nil {
			w.panMove(at)
		}
		return 0
	case wmLButtonUp:
		w.held = wParam & 0xffff
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		at := w.point(int32(x), int32(y))
		w.mouseButtonUp(driver.MouseButtonLeft, at)
		w.touch(w.fingers.Up(at))
		if w.fieldUp() || w.selectUp() {
			return 0
		}
		if w.barDrag != nil {
			w.releaseBar()
			releaseCapture.Call()
			w.hoverBars(at)
			w.redrawAll()
			return 0
		}
		long := w.pressUp()
		if !w.panUp(at) && !long {
			w.press(at)
		}
		return 0
	case wmCaptureChanged:
		if lParam != hwnd && w.selecting != nil {
			w.selecting.drag = dragNone
			w.selecting = nil
		}
		if lParam != hwnd && w.textSelection != nil {
			w.textSelection.Dragging = false
		}
		w.releaseBar()
		w.captureLost(lParam)
		w.panLost()
		if lParam != hwnd {
			w.pressCancel()
		}
		if lParam != hwnd {
			w.touch(w.fingers.Cancel(w.pointer))
		}
		return 0
	case wmGesture:
		if w.gesture(lParam) {
			return 0
		}
	case wmGetMinMaxInfo:
		if !w.embedded && w.chrome.constraints != (driver.WindowConstraints{}) {
			w.minMaxInfo(lParam)
			return 0
		}
	case wmNCCalcSize:
		if r, ok := w.ncCalcSize(wParam, lParam); ok && !w.embedded {
			return r
		}
	case wmNCHitTest:
		if !w.embedded {
			return w.ncHitTest(lParam)
		}
	case wmNCActivate:
		if !w.embedded {
			return w.ncActivate(wParam, lParam)
		}
	case wmNCMouseMove, wmNCMouseLeave, wmNCLButtonDown, wmNCLButtonUp, wmNCLButtonDblClk:
		if !w.embedded && w.ncMouse(message, wParam) {
			return 0
		}
	case wmContextMenu:
		if lParam&0xffffffff == contextFromKey&0xffffffff {
			w.fieldContextMenu(nil)
		} else {
			at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
			screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&at)))
			if p := w.point(at.X, at.Y); !w.fieldContextMenu(&p) {
				w.contextMenu(p)
			}
		}
		return 0
	case wmTick:
		w.ticked()
		return 0
	case wmTimer:
		switch wParam {
		case pinchTimer:
			w.wheelPinchEnded()
		case caretTimer:
			w.blink()
		case longPressTimer:
			w.longPressed()
		case passTimer:
			w.passUnpainted()
		}
		return 0
	case wmKeyDown:
		if !repeated(lParam) {
			w.modifierKey(wParam)
		}
		// The host's loop translated the key before it came here; Run's
		// loop asks the shortcuts first.
		if w.embedded && (w.keyDown(wParam, lParam) || w.shortcut(wParam, lParam)) {
			w.skipChar = true
			return 0
		}
		// The focused field first, then going back (Escape leaves the
		// field instead), then Tab between the fields.
		if w.controlKey(wParam, lParam) || w.fieldKey(wParam) || w.selectKey(wParam) {
			return 0
		}
		if wParam == vkEscape && !repeated(lParam) {
			if w.focusedField() != nil {
				w.blurAll()
			} else {
				w.back()
			}
			return 0
		}
		if wParam == vkTab && !keyDown(vkControl) && !keyDown(vkMenu) && w.tab(keyDown(vkShift)) {
			return 0
		}
	case wmChar:
		if w.skipChar {
			w.skipChar = false
			return 0
		}
		w.fieldChar(uint16(wParam))
		return 0
	case wmSetFocus, wmKillFocus:
		if message == wmKillFocus {
			w.releasePointer(true)
		}
		w.windowFocus(message == wmSetFocus)
	case wmSysKeyDown:
		if !repeated(lParam) {
			w.modifierKey(wParam)
		}
		// A host's loop sends system keys directly to an embedded child
		// window. Give the focused view and the keymap the same chance as
		// WM_KEYDOWN before Windows handles menu mnemonics or Alt+Left.
		w.skipSysChar = w.embedded && (w.keyDown(wParam, lParam) || w.shortcut(wParam, lParam))
		if w.skipSysChar {
			// As in Run's loop, a consumed key means Alt was not pressed alone, so its release opens no menu.
			w.chrome.altAlone = false
			return 0
		}
		if w.menuBarKey(message, wParam, lParam) {
			return 0
		}
		// Alt+Left; DefWindowProc still sees it, or releasing Alt would
		// open the window menu.
		if wParam == vkLeft && !repeated(lParam) {
			w.back()
		}
	case wmSysChar:
		if w.skipSysChar {
			w.skipSysChar = false
			return 0
		}
		if w.menuChar(wParam) {
			return 0
		}
	case wmAppCommand:
		if lParam>>16&0x0fff == appCommandBack {
			w.back()
			return 1
		}
	case wmMouseWheel, wmMouseHWheel:
		// Wheel messages carry screen coordinates.
		at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
		screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&at)))
		delta := int16(wParam >> 16 & 0xffff)
		if w.hostedWheel(at.X, at.Y, delta, message == wmMouseHWheel) {
			return 0
		}
		// A precision touchpad's pinch comes as turns with Control held.
		if message == wmMouseWheel && wParam&mkControl != 0 && w.wheelPinch(w.point(at.X, at.Y), delta) {
			return 0
		}
		if w.wheelTaken(w.point(at.X, at.Y), delta, message == wmMouseHWheel) {
			return 0
		}
		w.wheel(w.point(at.X, at.Y), driver.Points(delta)/120, message == wmMouseHWheel)
		return 0
	case wmCommand:
		if lParam == 0 && wParam>>16&0xffff == 0 {
			w.menuCommand(uint16(wParam))
		}
		return 0
	case wmGetObject:
		if r, ok := w.getObject(wParam, lParam); ok {
			return r
		}
	case wmDestroy:
		w.cancelColorPicker()
		w.closeCustomCursors()
		w.stopPower()
		w.releasePointer(false)
		w.closeAudio()
		w.closeCanvasImages()
		w.removeTrays()
		w.unregisterHotKeys()
		endWindowGuide(w)
		w.releaseAllAwake()
		w.loopback.Close()
		// Clients let the window's providers go.
		if uiaReturnRawElementProvider.Find() == nil {
			uiaReturnRawElementProvider.Call(hwnd, 0, 0, 0)
		}
		for wv := range w.webs {
			wv.close()
		}
		w.closeCookieView()
		w.revokeDrop()
		w.closeShare()
		for c := range w.hosts {
			w.closeComponent(c)
		}
		if w.primary {
			postQuitMessage.Call(0)
			return 0
		}
		// The host's loop goes on: the view's app ends with its window,
		// and its timers.
		w.ticking = false
		w.clockOn.Store(false)
		if w.clock != nil {
			w.clock.close()
			w.clock = nil
		}
		if w.engine != nil {
			w.engine.Close()
		}
		w.stopPerf()
		// Its views let go of what they draw with, shaders' textures among it, and the window of its swap chain.
		for _, v := range w.views {
			if v != nil {
				w.destroy(v)
			}
		}
		if w.composition != nil {
			w.composition.release()
		}
		if w.swap != nil {
			w.swap.release()
			w.swap = nil
		}
		delete(windows, hwnd)
		w.hwnd = 0
		w.updateAppSurface()
		return 0
	case wmMove:
		for wv := range w.webs {
			wv.moved()
		}
		if f := w.windowFrame(); w.engine != nil && f != w.env.WindowFrame {
			w.env.WindowFrame = f
			w.engine.SetEnvironment(w.env)
			w.devFrame.changed(w)
		}
	}
	r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}
