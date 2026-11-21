//go:build linux && !android

// Package linux is NSGo's GTK 4 backend, on NSH's Linux design: every
// view is a widget of its own (NsgoView, a GtkWidget subclass) that its
// parent places at the core's frame and that draws itself with GtkSnapshot
// and Cairo, text the engine measured and laid out with Pango included.
// Inputs, web views, videos and the app's native components are native
// GtkEntry, WebKitWebView, GtkPicture and app widgets hosted in their
// view's widget, so they stack, clip, fade and transform with the views
// around them. GTK delivers the pointer, wheel and keyboard events and
// file drops to the surface; the backend hit tests them and the engine
// keeps the interaction state. Transitions, animated values, layout
// animations and animated images run on one frame clock that ticks only
// while something animates.
//
// The backend is the one the roadmap asks for: GTK 4 through purego, no
// cgo and no hand-written C. It loads its libraries at run time, so an
// app built on macOS needs only Go and Xcode, and an app built for Linux
// needs GTK 4.14 or newer, Pango and Cairo where it runs. Web views also
// need WebKitGTK 6.0, which loads on the first one; without it a web view
// shows a placeholder, as NSH's does. Videos play through GTK's
// GStreamer media backend.
//
// The application menu is a GTK menu bar above the surface. The backend
// does not select text, expose accessibility, draw window controls of its
// own or animate modals.
package linux

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/finger"
	"github.com/nsgo-dev/nsgo/internal/platform/hover"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/loopback"
	"github.com/nsgo-dev/nsgo/internal/platform/queue"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
	"github.com/nsgo-dev/nsgo/internal/platform/speechd"
	"github.com/nsgo-dev/nsgo/internal/platform/textselect"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Options configure the window.
type Options struct {
	Title         string
	Width, Height driver.Points
	// Hidden keeps the backend's root window out of the screen while it hosts app-level services such as tray icons.
	Hidden bool
	// MainHidden opens the main window the app's function opens without showing it (nsgo.StartHidden).
	MainHidden bool
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
	// Name is the app's name, nsgo.yaml's (core.App().Name), "" where the tool set none; unlike Title, a surface's windows keep it.
	Name string
	// InstanceKey scopes the single-instance lock; empty keeps one lock per ID.
	InstanceKey string
	// Components are the native components Native elements name.
	Components driver.NativeComponents
	// Fonts are font files under the app's resources, registered before
	// the first view.
	Fonts []string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
	// Constraints limit the window (nsgo.WindowConstraints).
	Constraints driver.WindowConstraints
	// Transparent opens the window undecorated and clear wherever the app draws nothing, NoShadow without the shadow GTK draws, and SkipTaskbar off the X11 task list (look.go).
	Transparent, NoShadow, SkipTaskbar bool
	// Perf shows the performance monitor over the app (perf.go).
	Perf bool
	// Updates is how the app updates (updates.go).
	Updates updates.Config
	// LaunchedAtLogin says the login item started this process.
	LaunchedAtLogin bool
	// Host is what an embedding host gives the view (NewView).
	Host engine.HostOptions
	// test is what a test host opens the app with.
	test *testOpen
}

// testOpen is what a test host opens the app with: the test's options and
// the environment it pins.
type testOpen struct {
	options testhost.Options
	pin     engine.TestPin
	windows map[uint32]*window // opened surfaces, on the GTK thread
}

// window is one GTK window and the tree it shows. The engine's goroutine
// reaches its main-thread half only through Mount and post.
type window struct {
	testID  uint32  // the opened test surface, 0 for a regular window
	batch   uint64  // batches applied
	win     uintptr // GtkWindow, 0 for an embedded surface
	surface uintptr // the NsgoView holding the root view
	// primary is the backend's own surface. Opened surfaces share the GTK main loop, but closing one must not end the app.
	primary bool
	// opener is the app's own window for a surface the app opened, which goes when that window stops (stop).
	opener     *window
	kind       driver.SurfaceKind
	hideOnBlur bool
	visible    bool
	focused    bool
	// noActivate keeps a panel from the focus (SurfaceConfig.NoActivate), which X11 does by leaving it unmanaged: no window manager places, decorates or focuses it. anchorBounds and panelSide are where core.Window.ShowAt last placed it, panelSide WindowPlaceDefault for nowhere.
	noActivate   bool
	unmanaged    bool
	anchorBounds driver.Bounds
	panelSide    driver.WindowPlacement
	// level is the level SetWindowLevel last asked for, which X11 asks the window manager for again as the window maps once levelWatched.
	level        driver.WindowLevel
	levelWatched bool
	// workspaces are the workspaces the app put the window on, and onAllDesktops whether it last asked the window manager for every desktop (workspaces.go).
	workspaces    driver.WindowWorkspaces
	onAllDesktops bool
	// input is where the window takes the pointer (windowpointer.go).
	input windowInput
	// currentEvent is the GDK event of the press being dispatched on the main thread, valid only during its callback, for actions that need it, such as showing the window manager's title bar menu.
	currentEvent uintptr
	// settingsHandlers are the handlers setupSettings connects to GTK's settings, which outlive the window.
	settingsHandlers []uint64
	// contrastHandler is the handler on GNOME's accessibility settings, which are not GTK's.
	contrastHandler uint64
	// compositedHandler is the handler on the display's composited property, which outlives the window.
	compositedHandler uint64
	// embedded is set for a host's widget (NewView): the host owns the
	// window, its chrome and the main loop. app and opts start its engine
	// once GTK first allocates it.
	embedded bool
	// watchedRoot is the window whose focus the surface follows, once
	// mapped in one.
	watchedRoot uintptr
	// language is what the menus say (driver.ItemDirection).
	language string
	// loopback are the auth sessions the default browser shows.
	loopback loopback.Sessions
	// dir is the app's direction (driver.ItemDirection) as GTK's, or 0
	// before the app has one.
	dir     int32
	app     engine.App
	opts    Options
	box     uintptr // the window content: menu bar and surface
	menuBar uintptr
	loop    uintptr // GMainLoop

	env driver.Environment

	engine  *engine.Engine
	batches queue.Batches
	chrome  chrome

	// The engine goroutine hands these to the main thread.
	mu    sync.Mutex
	tasks []func()

	// The main thread's. views is indexed by NodeID.
	views []*view
	root  driver.NodeID
	// effects are the views with an effect, a transition or a navigation
	// shader (effect.go).
	effects     map[*view]bool
	entries     map[uintptr]*view
	natives     map[uintptr]*view // by the objects of hosted widgets
	appID       string
	keys        []driver.KeyBinding
	hotKeys     *globalHotKeys
	held        map[uint32]heldKey                  // the keys down, by keycode (keyPressedCallback)
	heldKeys    bool                                // the app follows every key going down and up (driver.MountItem.HeldKeys)
	heldMouse   bool                                // the app follows every mouse button going down and up (driver.MountItem.HeldMouseButtons)
	mouseHeld   map[driver.MouseButton]heldMouseBtn // mouse buttons reported going down and not yet up
	mouseClicks map[driver.MouseButton]buttonClicks
	animating   []*view
	ticking     bool
	// tickID is the surface's tick callback and watchID the timer watching it while the clock runs; ticked is when the frame clock last ticked.
	tickID, watchID uint32
	ticked          time.Time
	// frameWanted is set while the app waits for a display frame, by the
	// engine goroutine; clockOn mirrors ticking for it, as a running clock
	// serves the frame without a hop to the main loop.
	frameWanted, clockOn atomic.Bool
	animator             *driver.Animator
	// canvasImages are the images canvases draw, and canvasWaiting the
	// canvases to draw again when one loads.
	canvasImages   canvasImages
	canvasKept     map[string]int
	canvasWaiting  map[string][]*view
	canvasImageUse uint64
	// canvasPass is canvasImageUse when the last frame ended: images used since then are the current frame's, which eviction keeps; canvasPassPosted says an endCanvasPass is posted.
	canvasPass       uint64
	canvasPassPosted bool
	canvasClosed     bool
	// canvasFrames are the pictures of the pixels canvases drew last (canvasFrame).
	canvasFrames      []*imageSlot
	canvasFramePixels int // the pixels of canvasFrames together
	// hiddenCanvases wait out of sight to be drawn (raster.go), and
	// revealDue says a layout since the last frame may have shown some.
	hiddenCanvases []*view
	revealDue      bool
	// hud is the performance panel, when the app shows one (perf.go).
	hud *hud
	// stickyBound are the scroll views with sticky cells whose insets
	// follow animated values.
	stickyBound map[*view]bool
	epoch       time.Time // the animator's clock starts here
	layouts     []*view   // views a layout animation moves, shows or retires
	playing     []*view   // image views whose animated images play
	fading      []*view   // image views whose sources fade in

	components driver.NativeComponents
	drop       fileDrop

	// surfaceCursor is the cursor last set on the surface, once surfaceCursorSet: the style's over the view the hit test finds.
	surfaceCursor    driver.Cursor
	surfaceCursorKey string
	surfaceCursorSet bool
	customCursors    *customCursorCache

	lastPointer  driver.Point
	cursorText   *view         // the text whose widget shows a span's cursor
	down         driver.Event  // the last press: modifiers and click count
	lastClick    *view         // the view the last left press hit, for click counting
	clickSkip    int           // how many of GTK's count of the last left press came before lastClick's first
	paste        *pasting      // the paste whose clipboard is being read
	pointerIn    bool          // the pointer is over the surface, at lastPointer
	hovered      hover.Tracker // the views under it that follow hovers
	pointerState uint32        // its modifiers and buttons at its last move
	shielding    bool          // a press kept from a hosted control goes on
	panning      *pan
	bars         []*view  // the scroll views under the pointer, which show their bars
	barDrag      *barDrag // the bar the pointer drags
	pinching     *twoFinger
	// wheelTaker is the view with OnWheel a touchpad scroll goes to.
	wheelTaker    *view
	rotating      *twoFinger
	closing       bool
	lastWidth     driver.Points
	lastHeight    driver.Points
	started       bool
	handle        uintptr
	measureMap    [2]uintptr // the engine thread's Pango contexts, left to right and right to left
	hitMap        [2]uintptr // hit testing's, used under hitMu from either thread
	hitMu         sync.Mutex
	extents       map[fontKey]driver.FontExtent // the engine thread's font metrics
	capHeights    map[fontKey]driver.Points     // and capital heights
	lineMetrics   map[fontKey]lineMetrics       // and decoration lines
	pressConsumed bool
	// windowMove is a press on a window drag region that has not moved the window yet (input.go).
	windowMove *windowMove
	// selecting is the selection of an area, and selected the texts
	// that highlight a part of it (selection.go).
	selecting *textselect.Selection
	selected  []*view
	// forced is the appearance an embedded view's app forces, scoped the
	// one its controls show in, by schemeProvider under the surface's
	// name (scheme.go).
	forced, scoped driver.Appearance
	schemeProvider uintptr
	surfaceName    string
	// fingers are the views' touches: the mouse as finger 0, a touchscreen's from 1 (touch.go).
	fingers finger.Fingers
	// touching is the touch that may drag a scroll view (touchscroll.go).
	touching *touchScroll
	// blockNative is set while the touch responder keeps the scroll views from the touches (ItemBlockNative).
	blockNative bool
	// wheeled is the scroll view the last wheel turn moved, wheelMotion
	// its glide after the touchpad's fingers lifted.
	wheeled     *view
	wheelMotion *scrollbar.Motion
	// roles are the accessible roles of the views the batch being applied
	// creates (access.go).
	roles        map[driver.NodeID]int32
	longPress    uint64 // counts long presses waited for, to cancel the last
	focus        *view  // the entry with keyboard focus
	controlFocus *view  // the ordinary widget with keyboard focus
	// covered are the fields and stops modals covered while they had the keyboard focus, oldest first (focus.go).
	covered      []*view
	pinchTimeout uint32
	alerts       []driver.AlertConfig
	alerting     bool
	alertingID   uint32
	alertShown   driver.AlertConfig // the alert showing, for a test host
	menuCounter  uint32
	actionGroup  uintptr
	actionRefs   map[uintptr]menuRef
	// building names the action group, prefix and refs the menu being
	// built registers its actions in, when not the menu bar's.
	building menuBuild
	// popover is the context menu shown, popoverID its request and
	// popupRefs its items.
	popover   uintptr
	popoverID uint32
	popupRefs map[uintptr]menuRef
	trays     map[uint32]*linuxTray
	// transparent is set while the window or panel is undecorated and clear wherever the app draws nothing (look.go).
	transparent bool
	// appearance is the one the app forces.
	appearance driver.Appearance
}

// menuBuild is where a menu's actions go.
type menuBuild struct {
	group  uintptr
	prefix string
	refs   map[uintptr]menuRef
}

// updateConfig is how the app Run runs updates. Linux has no updater of its own: a check reads the app's appcast, and package updater hands a verified package to the package manager that installed the app, or offers the release's page where none did (selfUpdatable).
var updateConfig updates.Config

// InstalledBy names the sandbox whose store installed the app and updates
// it, Flatpak or Snap, as they mark their apps' environments; "" for any
// other install, a package manager's included, which nsgo.yaml's
// updates.channel tells apart.
func InstalledBy() string {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		return "Flatpak"
	}
	if os.Getenv("SNAP") != "" {
		return "Snap"
	}
	return ""
}

// Run opens a window showing app and runs GTK's main loop. Call it from
// main; it returns when the window closes.
func Run(app engine.App, opts Options) {
	// One copy runs per app ID; an app without one, as a test's window, runs as many as it is started.
	var instance *instance
	if opts.ID != "" && opts.test == nil {
		var alreadyRunning bool
		if instance, alreadyRunning = claimInstance(opts.ID, opts.InstanceKey, engine.Launched()); alreadyRunning {
			return
		}
		defer instance.close()
	}
	updateConfig = opts.Updates
	w, err := start(app, opts)
	if err != nil {
		panic("nsgo: " + err.Error())
	}
	if instance != nil {
		instance.setDelivery(func(args engine.Launch) { w.post(func() { w.launched(args) }) })
	}
	// The core keeps them until the app's first handlers register.
	if paths := engine.LaunchFiles(); len(paths) > 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: paths})
	}
	if links := engine.LaunchLinks(); len(links) > 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: links})
	}
	w.run()
	w.stop()
}

// launched hands the running app what a second launch asked of it: a launch to open links alone brings the app's window to the front, as opening a link does on macOS, any other launch reopens the app, and the files and links it names open.
func (w *window) launched(args engine.Launch) {
	if len(args.Links) > 0 && len(args.Files) == 0 {
		// A window the app hid stays hidden: the app's handler shows what the link asks for.
		if gtkWidgetGetVisible(w.win) != 0 {
			w.windowCommand(&driver.WindowCommand{Action: driver.WindowOrderFront})
		}
	} else {
		w.engine.Dispatch(driver.Event{Kind: driver.EventReopen})
	}
	if len(args.Files) > 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: args.Files})
	}
	if len(args.Links) > 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: args.Links})
	}
}

// start opens the window and starts the engine, without running the main
// loop yet. Run continues with it; tests drive the window from their own
// GTK thread.
func start(app engine.App, opts Options) (*window, error) {
	if opts.Width == 0 {
		opts.Width, opts.Height = 480, 640
	}
	if err := openLibs(); err != nil {
		return nil, fmt.Errorf("the Linux backend needs GTK 4, Pango and Cairo: %w", err)
	}
	setProgramIdentity(opts.ID)
	runtime.LockOSThread()
	registerFonts(opts.Fonts)
	preferGLOnNvidia()
	if gtkInitCheck() == 0 {
		return nil, errors.New("no display; run the app in a desktop session or with xvfb-run")
	}
	if err := registerViewType(); err != nil {
		return nil, fmt.Errorf("the Linux backend needs GTK 4: %w", err)
	}
	startUIFont()
	w := newWindow(opts, false)
	w.primary, w.app, w.opts = true, app, opts
	w.createWindow(opts)
	if !opts.Hidden {
		w.applyLook(opts)
	}
	w.env = w.environment()

	w.setAppearance(opts.Appearance)
	w.startEngine(app, opts, driver.Size{Width: opts.Width, Height: opts.Height})
	w.loop = gMainLoopNew(0, 0)
	w.setupSettings()
	return w, nil
}

// newWindow returns the state of a window, or of a host's widget when
// embedded.
func newWindow(opts Options, embedded bool) *window {
	w := &window{
		entries: map[uintptr]*view{},
		natives: map[uintptr]*view{},
		trays:   map[uint32]*linuxTray{},
		appID:   opts.ID,

		actionRefs: map[uintptr]menuRef{},
		env:        driver.Environment{OS: driver.OSLinux, Idiom: driver.IdiomDesktop, CustomCursor: true},
		components: opts.Components,
		epoch:      time.Now(),
		embedded:   embedded,
	}
	w.animator = driver.NewAnimator(w.animatorReport)
	w.handle = registerWindow(w)
	w.chrome.keys = map[string]menuRef{}
	return w
}

// startEngine starts app's engine at size.
func (w *window) startEngine(app engine.App, opts Options, size driver.Size) {
	eopts := engine.Options{
		Size:        size,
		Environment: w.env,
		Appearance:  opts.Appearance,
		Host:        opts.Host,
		// The app's own window takes its global shortcuts; a host's widget leaves them to the host.
		HotKeys: !w.embedded,
		Tray:    !w.embedded,
		Secrets: secretService{},
	}
	if opts.Hidden {
		eopts.Windows = &engine.WindowOptions{Title: opts.Title, Width: opts.Width, Height: opts.Height,
			Constraints: opts.Constraints, Transparent: opts.Transparent, NoShadow: opts.NoShadow, SkipTaskbar: opts.SkipTaskbar, Hidden: opts.MainHidden}
	}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	if opts.test != nil {
		eopts.UseTest(opts.test.options, &opts.test.pin)
	}
	imageload.UseDiskCache(eopts.CacheDir)
	if opts.Perf && opts.test == nil || opts.test != nil && opts.test.options.FPS {
		eopts.Perf = w.startPerf()
	}
	w.engine = engine.Start(w, app, eopts)
	w.reportFrameWindow(w.env)
}

// openSurface creates a window on GTK's existing main loop and attaches its tree to the root engine. Panels use a borderless GTK window and can hide on blur; one that never takes the focus is an unmanaged X11 window on X11 (panel.go), and tray anchoring remains separate work.
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
	opts.Hidden = cfg.Hidden
	opts.Constraints = cfg.Constraints
	// Only a window is transparent, shadowless or off the task list of its own choice.
	normalWindow := cfg.Kind == driver.SurfaceWindow
	opts.Transparent, opts.NoShadow, opts.SkipTaskbar = cfg.Transparent && normalWindow, cfg.NoShadow && normalWindow, cfg.SkipTaskbar && normalWindow
	if cfg.Kind == driver.SurfaceTrayIcon {
		// The status item is still registered, but Linux does not yet draw a
		// surface into it. Keep the attached tree out of sight until that
		// slice is implemented.
		opts.Hidden = true
	}
	child := newWindow(opts, false)
	child.opener = w
	if w.opener != nil {
		child.opener = w.opener
	}
	child.kind, child.hideOnBlur, child.noActivate, child.opts = cfg.Kind, cfg.HideOnBlur, cfg.NoActivate, opts
	// Applied as the window maps (windowMappedCallback).
	child.workspaces = cfg.Workspaces
	child.createWindow(opts)
	child.applyLook(opts)
	child.setupSettings()
	child.visible = !opts.Hidden
	if cfg.Kind == driver.SurfacePanel {
		// A borderless window that takes its own keyboard focus, GTK having no panel that does so without activating.
		gtkWindowSetDecorated(child.win, 0)
		if cfg.NoActivate {
			child.unmanaged = child.leaveUnmanaged()
		}
		// A transparent panel keeps its background where nothing would show through it (SurfaceConfig.Transparent).
		if cfg.Transparent && displayComposites() {
			child.setTransparent(true)
		}
	}
	if cfg.Frame.Width > 0 || cfg.Frame.Height > 0 {
		child.setFrame(cfg.Frame)
	}
	child.setPointerEvents(cfg.PointerEvents)
	child.env = child.environment()
	child.engine = w.engine.Attach(cfg.ID, child)
	if opts.test != nil {
		if opts.test.windows == nil {
			opts.test.windows = map[uint32]*window{}
		}
		child.testID = cfg.ID
		opts.test.windows[cfg.ID] = child
	}
	size := driver.Size{Width: opts.Width, Height: opts.Height}
	child.engine.Resize(size, driver.Geometry{})
	child.engine.SetEnvironment(child.env)
	child.reportFrameWindow(child.env)
	if !opts.Hidden {
		gtkWindowPresent(child.win)
	}
}

// View is an app in a GtkWidget of an embedding host's window, which the
// host's own main loop runs (NewView).
type View struct{ w *window }

// preferGLOnNvidia has GSK draw with its GL renderer instead of its
// Vulkan one (GTK's default since 4.16) under NVIDIA's driver, as NSH
// does, unless GSK_RENDERER chooses: there GSK's Vulkan frames cost the
// main thread several times as much (NSH measured ~9.5 ms of paint per
// frame at 75 fps against ~1 ms at 120 fps with GL). A host embedding a
// view chooses for itself.
func preferGLOnNvidia() {
	if _, set := os.LookupEnv("GSK_RENDERER"); set {
		return
	}
	if _, err := os.Stat("/proc/driver/nvidia/version"); err == nil {
		os.Setenv("GSK_RENDERER", "ngl")
	}
}

// NewView makes a GtkWidget showing app, for a host that has initialized GTK and runs its main loop itself. Its engine starts once GTK first allocates the widget, so settings made before that are in the app's first frame. Call it on GTK's thread; several views may show at once. It leaves the program name, which docks match the host's windows by, to the host.
func NewView(app engine.App, opts Options) (*View, error) {
	if err := openLibs(); err != nil {
		return nil, fmt.Errorf("the Linux backend needs GTK 4, Pango and Cairo: %w", err)
	}
	runtime.LockOSThread()
	registerFonts(opts.Fonts)
	if gtkInitCheck() == 0 {
		return nil, errors.New("no display")
	}
	if err := registerViewType(); err != nil {
		return nil, fmt.Errorf("the Linux backend needs GTK 4: %w", err)
	}
	startUIFont()
	if opts.Host.Settings == nil {
		opts.Host.Settings = &engine.HostSettings{}
	}
	w := newWindow(opts, true)
	w.app, w.opts = app, opts
	w.surface = w.newViewWidget(nil)
	gtkWidgetSetHexpand(w.surface, 1)
	gtkWidgetSetVexpand(w.surface, 1)
	// Keys reach the surface while the focus is in it, which a press
	// gives it.
	gtkWidgetSetFocusable(w.surface, 1)
	w.setupInputs()
	w.setupSettings()
	w.forced = opts.Appearance
	w.scopeAppearance(w.shownAppearance())
	return &View{w}, nil
}

// Handle returns the GtkWidget, which the host adds to its widgets.
func (v *View) Handle() uintptr { return v.w.surface }

// SetHostEnvironment sets what the host sets over the session's
// environment: its locale, text size and appearance.
func (v *View) SetHostEnvironment(env engine.HostEnvironment) {
	v.w.opts.Host.Settings.SetEnvironment(env)
	v.w.scopeAppearance(v.w.shownAppearance())
}

// SetTokens sets the values the host themes the app with.
func (v *View) SetTokens(t engine.Tokens) { v.w.opts.Host.Settings.SetTokens(t) }

// Close stops the app, destroys its native components and lets the
// widget go, which the host removes from its parent.
func (v *View) Close() {
	w := v.w
	if w.surface == 0 {
		return
	}
	w.scopeAppearance(driver.AppearanceSystem)
	w.close()
	w.releaseSurface()
}

// closeNatives destroys the native components still showing, once the
// app has stopped.
func (w *window) closeNatives() {
	for _, x := range w.views {
		if x != nil && x.native != nil {
			w.closeNative(x)
		}
	}
}

// closeEditors releases the editors of a stopped app window and the CSS providers they added to the whole display. The app's own window keeps its views for the callbacks GTK may still make, so releaseSurface, which an opened window goes through, does not run for it.
func (w *window) closeEditors() {
	for _, v := range w.views {
		if v != nil {
			w.releaseEditor(v)
		}
	}
}

// toplevel is the window the surface is in: its own, or the host's.
func (w *window) toplevel() uintptr {
	if w.win != 0 {
		return w.win
	}
	return gtkWidgetGetRoot(w.surface)
}

// run presents the window and runs GTK's main loop until it closes.
func (w *window) run() {
	if !w.opts.Hidden {
		gtkWindowPresent(w.win)
	}
	gMainLoopRun(w.loop)
}

// stop ends the engine, destroys the native components still showing
// and frees the loop after run returns.
func (w *window) stop() {
	w.engine.Close()
	w.closeNatives()
	w.closeEditors()
	// The windows the app opened that are still open go with it as WindowDestroy takes them, which a stopped engine no longer sends: left, their editors' CSS providers stay on the whole display.
	windowHandles.Range(func(_, value any) bool {
		if o := value.(*window); o.opener == w {
			o.destroy()
		}
		return true
	})
	engine.SyncStorage()
	w.closeCustomCursors()
	w.releaseTextContexts()
	if w.loop != 0 {
		gMainLoopUnref(w.loop)
		w.loop = 0
	}
	// The handle stays registered: GTK may still call a callback of a
	// window whose loop has stopped as its widgets are finalized.
}

// createWindow makes the window, its menu bar box and the surface.
func (w *window) createWindow(opts Options) {
	w.win = gtkWindowNew()
	title := opts.Title
	gtkWindowSetTitle(w.win, cstr(title))
	gtkWindowSetDefaultSize(w.win, int32(opts.Width), int32(opts.Height))
	w.lastWidth, w.lastHeight = opts.Width, opts.Height
	if opts.test != nil {
		// A fixed size has tiling compositors float a test's window at the
		// test's size instead of resizing it as another opens.
		gtkWindowSetResizable(w.win, 0)
	}

	w.box = gtkBoxNew(1, 0) // vertical
	w.surface = w.newViewWidget(nil)
	gtkWidgetSetHexpand(w.surface, 1)
	gtkWidgetSetVexpand(w.surface, 1)
	gtkBoxAppend(w.box, w.surface)
	gtkWindowSetChild(w.win, w.box)
	if opts.Constraints != (driver.WindowConstraints{}) {
		w.setConstraints(opts.Constraints)
	}
	w.setupInputs()
	gSignalConnect(w.win, "close-request", cbCloseRequest, w.handle)
	gSignalConnect(w.win, "map", cbWindowMapped, w.handle)
	for _, name := range []string{"maximized", "fullscreen", "default-width", "default-height"} {
		gSignalConnect(w.win, "notify::"+name, cbWindowState, w.handle)
	}
}

// setupSettings rereads the environment whenever GTK's settings or the
// app's lifecycle change.
func (w *window) setupSettings() {
	gSignalConnect(w.surface, "map", cbMapped, w.handle)
	gSignalConnect(w.surface, "unmap", cbMapped, w.handle)
	if settings := gtkSettingsGetDefault(); settings != 0 {
		names := settingsToWatch(colorScheme())
		for _, name := range names {
			w.settingsHandlers = append(w.settingsHandlers, gSignalConnect(settings, "notify::"+name, cbSettingsChanged, w.handle))
		}
	}
	if settings := gnomeA11ySettingsForWatch(); settings != 0 {
		w.contrastHandler = gSignalConnect(settings, "changed::high-contrast", cbSettingsChanged, w.handle)
	}
	// An X11 compositing manager starting or stopping changes whether a transparent window or panel shows through (CanMakeWindowsTransparent, WindowTransparent).
	if display := gdkDisplayGetDefault(); display != 0 && (w.kind == driver.SurfaceWindow || w.kind == driver.SurfacePanel) {
		w.compositedHandler = gSignalConnect(display, "notify::composited", cbSettingsChanged, w.handle)
	}
}

func settingsToWatch(colorScheme bool) []string {
	names := []string{"gtk-theme-name", "gtk-font-name", "gtk-application-prefer-dark-theme", "gtk-xft-dpi", "gtk-enable-animations"}
	if colorScheme {
		names = append(names, "gtk-interface-color-scheme", "gtk-interface-contrast")
	}
	return names
}

// Mount implements engine.Platform: it queues the batch and wakes the
// main loop once.
func (w *window) Mount(b *driver.MountBatch) {
	if w.batches.Push(b) {
		gIdleApply(cbApply, w.handle)
	}
}

// RequestFrame implements engine.Platform: the app waits for the next
// display frame. Called from the engine goroutine, it hops to the main
// loop, where the frame clock runs, only to start the clock: an app asking
// every frame keeps it running, and its next tick serves the frame.
func (w *window) RequestFrame() {
	w.frameWanted.Store(true)
	if !w.clockOn.Load() {
		w.post(w.startClock)
	}
}

// post runs fn on the main thread; any goroutine may call it.
func (w *window) post(fn func()) {
	w.mu.Lock()
	wake := len(w.tasks) == 0
	w.tasks = append(w.tasks, fn)
	w.mu.Unlock()
	if wake {
		gIdleApply(cbRun, w.handle)
	}
}

// gIdleApply schedules cb(handle) on the main loop; the callback
// returns G_SOURCE_REMOVE.
func gIdleApply(cb uintptr, handle uintptr) {
	gIdleAdd(cb, handle)
}

func runCallback(data uintptr) int32 {
	if w := windowOf(data); w != nil {
		w.runTasks()
	}
	return 0
}

func applyCallback(data uintptr) int32 {
	w := windowOf(data)
	if w == nil {
		return 0
	}
	done := w.measuring(true)
	for _, b := range w.batches.Drain() {
		if w.hud != nil && !b.Sent.IsZero() {
			w.hud.monitor.RecordLatency(time.Since(b.Sent))
		}
		driver.Apply(b, w.apply)
	}
	done()
	return 0
}

func closeRequestCallback(win uintptr, data uintptr) int32 {
	if w := windowOf(data); w != nil {
		if w.primary || w.embedded {
			w.close()
			return 1
		}
		// An opened window's close is the app's to decide (core.Window.OnClosing): it stays until WindowDestroy.
		if w.engine != nil && !w.closing {
			w.engine.Dispatch(driver.Event{Kind: driver.EventWindowClose})
		}
		return 1
	}
	return 1
}

// runTasks runs what other goroutines posted, in order.
func (w *window) runTasks() {
	w.mu.Lock()
	fns := w.tasks
	w.tasks = nil
	w.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// chromeItem applies an item of the window's own: its title, a window
// command or the menu bar.
func (w *window) chromeItem(it *driver.MountItem) {
	switch it.Kind {
	case driver.ItemTitle:
		gtkWindowSetTitle(w.win, cstr(it.Value))
	case driver.ItemWindow:
		if it.Window != nil {
			w.windowCommand(it.Window)
		}
	case driver.ItemMenu:
		w.setMenu(it.Menu)
	}
}

// close ends the app: the engine stops, and the main loop returns.
func (w *window) close() {
	if w.closing {
		return
	}
	w.closing = true
	w.closeCanvasImages()
	w.closeHotKeys()
	if w.loop != 0 {
		// The app ends with this window: every hold goes, sent before the main loop returns.
		endAwake(awakeEndTimeout)
	} else {
		w.releaseAllAwake()
	}
	w.loopback.Close()
	w.visible = false
	w.removeTrays()
	if w.ticking {
		gtkWidgetRemoveTickCallback(w.surface, w.tickID)
		gSourceRemove(w.watchID)
		w.tickID, w.watchID = 0, 0
		w.ticking = false
		stopYielding()
	}
	if w.engine != nil {
		w.engine.Close()
	}
	w.stopPerf()
	if w.loop != 0 {
		gMainLoopQuit(w.loop)
	}
}

// destroy ends an opened surface once its tree is gone (WindowDestroy): its views, its window and the handle its callbacks find it by go with it.
func (w *window) destroy() {
	if w.testID != 0 {
		delete(w.opts.test.windows, w.testID)
	}
	if w.primary || w.embedded || w.win == 0 {
		return
	}
	w.close()
	w.releaseSurface()
}

// releaseSurface releases a stopped surface's tree, process-wide registrations and native references. An embedded surface leaves its parent widget and the host's window to the host.
func (w *window) releaseSurface() {
	for _, v := range w.views {
		if v != nil {
			w.delete(v)
		}
	}
	if settings := gtkSettingsGetDefault(); settings != 0 {
		for _, id := range w.settingsHandlers {
			gSignalHandlerDisconnect(settings, id)
		}
	}
	w.settingsHandlers = nil
	if w.contrastHandler != 0 {
		gSignalHandlerDisconnect(gnomeA11ySettingsForWatch(), w.contrastHandler)
		w.contrastHandler = 0
	}
	if w.compositedHandler != 0 {
		gSignalHandlerDisconnect(gdkDisplayGetDefault(), w.compositedHandler)
		w.compositedHandler = 0
	}
	w.closeCustomCursors()
	w.releaseTextContexts()
	if w.win != 0 {
		gtkWindowDestroy(w.win)
	}
	forgetWidget(w.surface)
	gObjectUnref(w.surface)
	w.win, w.surface = 0, 0
	windowHandles.Delete(w.handle)
	updateAppSurface()
}

func (w *window) view(id driver.NodeID) *view {
	if id > 0 && int(id) < len(w.views) {
		return w.views[id]
	}
	return nil
}

// apply applies one commit's items.
func (w *window) apply(b *driver.MountBatch) {
	w.batch++
	var rescroll, loads, labels []*view
	type focusChange struct {
		v     *view
		focus bool
	}
	var focus []focusChange
	var selections map[*view]bool
	lb := w.newLayoutBatch(b)
	w.prepareRoles(b)
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate:
			w.create(it.ID, it.View)
			if lb != nil {
				lb.created[it.ID] = true
			}
			continue
		case driver.ItemAnimation:
			w.animations(it.Animation)
			continue
		case driver.ItemAlert:
			w.showAlert(it.Alert)
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
		case driver.ItemSetClipboardImage:
			if data, ok := it.Data.([]byte); ok {
				setClipboardImage(data)
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
			speechd.Handle(it.Count, *it.Speak, w.engine.Dispatch)
			continue
		case driver.ItemWebCookies:
			w.webCookies(it.Count, *it.Cookies)
			continue
		case driver.ItemNotificationPermission, driver.ItemShowNotification, driver.ItemCloseNotification, driver.ItemBadge, driver.ItemTaskProgress:
			w.notify(it)
			continue
		case driver.ItemHotKeys:
			w.applyHotKeys(it.Keys)
			continue
		case driver.ItemPermission:
			// Nothing to ask, no settings page and nothing to guide.
			driver.AnswerPermission(it.Count, *it.Permission, linuxPermission, w.engine.Dispatch)
			continue
		case driver.ItemKeepAwake:
			if it.Awake != nil {
				w.keepAwake(it.Count, *it.Awake)
			}
			continue
		case driver.ItemReleaseAwake:
			w.releaseAwake(it.Count)
			continue
		case driver.ItemCheckForUpdates:
			w.checkForUpdates(it.Count)
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
		case driver.ItemOpenFile:
			w.openFile(*it.Open, it.Count)
			continue
		case driver.ItemRevealFile:
			w.revealFile(*it.Open, it.Count)
			continue
		case driver.ItemTrashFile:
			w.trashFile(*it.Open, it.Count)
			continue
		case driver.ItemShare:
			w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoShareSheet})
			continue
		case driver.ItemSaveFile:
			w.saveFile(*it.Save, it.Count)
			continue
		case driver.ItemPopupMenu:
			w.popupMenu(it.Menu, it.Point, it.Count)
			continue
		case driver.ItemTray:
			w.applyTray(*it.Tray)
			continue
		case driver.ItemTrayMenu:
			tr := (*linuxTray)(nil)
			if it.Tray != nil {
				tr = w.trays[it.Tray.ID]
			}
			if tr == nil || tr.menuID == 0 {
				// A host without a DBus menu closes this request here.
				w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: it.Count})
				continue
			}
			w.trayMenuShown(tr, it.Menu, it.Count)
			continue
		case driver.ItemLaunchAtLogin:
			w.engine.Dispatch(driver.Event{Kind: driver.EventLaunchAtLogin, Count: it.Count, Finished: launchesAtLogin(w.opts.ID, w.opts.Name)})
			continue
		case driver.ItemSetLaunchAtLogin:
			setLaunchAtLogin(it.On, w.opts.ID, w.opts.Name)
			continue
		case driver.ItemOpenSurface:
			if it.Surface != nil {
				w.openSurface(*it.Surface)
			}
			continue
		case driver.ItemAppearance:
			// The settings are the process's, which a host's widget
			// leaves to the host: its controls follow in its scope.
			if w.embedded {
				w.forced = it.Appearance
				w.scopeAppearance(w.shownAppearance())
			} else {
				w.setAppearance(it.Appearance)
			}
			continue
		case driver.ItemDirection:
			w.setDirection(it.RTL)
			if it.Language != w.language {
				w.language = it.Language
				if !w.embedded {
					w.showMenuBar()
				}
			}
			continue
		case driver.ItemQuit:
			if !w.embedded {
				w.close()
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
		case driver.ItemKeymap:
			w.setKeymap(it.Keys)
			w.heldKeys, w.heldMouse = it.HeldKeys, it.HeldMouseButtons
			continue
		case driver.ItemTitle, driver.ItemWindow, driver.ItemMenu:
			if it.Kind == driver.ItemWindow && it.Window != nil && it.Window.Action == driver.WindowDestroy {
				// Nothing follows the surface's last item.
				w.destroy()
				return
			}
			// An embedding host owns its window and menus.
			if !w.embedded {
				w.chromeItem(it)
			}
			continue
		case driver.ItemAnnounce:
			w.announce(it.Value, false)
			continue
		case driver.ItemInputSource:
			setInputSource(it.Value)
			continue
		case driver.ItemAccessibilityLabel:
			if v := w.view(it.ID); v != nil {
				v.props.Label = it.Value
				w.accessChanged(v)
			}
			continue
		case driver.ItemAccessibilityFocus:
			if v := w.view(it.ID); v != nil && v.widget != 0 {
				w.focusView(v)
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
		case driver.ItemInsert:
			if p := w.view(it.Parent); p != nil {
				v.detach()
				at := len(p.children)
				before := w.view(it.Before)
				if before != nil {
					if j := slices.Index(p.children, before); j >= 0 {
						at = j
					} else {
						before = nil
					}
				}
				p.children = slices.Insert(p.children, at, v)
				v.parent = p
				w.insertWidget(p, v, before)
				w.inserted(v, p, lb)
				if v.kind == driver.KindModal && !v.presented {
					v.presented = true
					w.present(v)
				}
			}
		case driver.ItemRemove:
			v.detach()
			w.unparentWidget(v.widget)
		case driver.ItemDelete:
			w.animator.Forget(v.id)
			switch {
			case v.parent != nil && v.parent.dying:
				w.retireInner(v)
			case w.retiring(v, lb):
				w.retire(v, lb)
			default:
				w.delete(v)
			}
		case driver.ItemOverflow:
			v.overflow, v.transformedKids = it.Frame, it.Transformed
		case driver.ItemMatch:
			w.match(v, w.view(it.Before), lb)
		case driver.ItemFrame:
			old := v.shownFrame().Size
			w.framed(v, it, lb)
			resized := v.shownFrame().Size != old
			w.moved(v)
			if resized && v.widget != 0 {
				gtkWidgetQueueAllocate(v.widget)
			}
			if v.kind == driver.KindScroll {
				rescroll = append(rescroll, v)
			}
		case driver.ItemViewStyle:
			hidden := v.style.Hidden
			w.setStyle(v, it.ViewStyle)
			if v.style.Hidden != hidden {
				w.accessChanged(v)
			}
			w.showWidget(v)
			w.moved(v)
			if v.entry != 0 {
				w.styleEntry(v)
			}
			w.webBackground(v)
			if v.kind == driver.KindImage {
				w.playImage(v) // again once it is no longer tinted
			}
		case driver.ItemText:
			if v.text != nil {
				v.text.Release()
			}
			v.text = it.Text.Layout
			v.content = it.Text.Content
			v.plain, v.spans = it.Text.Text, it.Text.Spans
			w.accessChanged(v)
			w.changed(v)
		case driver.ItemInput:
			w.configure(v, it.Input)
		case driver.ItemInputValue:
			w.setValue(v, it.Value, it.Count)
		case driver.ItemSelection:
			w.selectRange(v, it.Selection)
			if selections == nil {
				selections = make(map[*view]bool)
			}
			selections[v] = true
		case driver.ItemEvents:
			w.setEvents(v, it.Events)
			w.accessChanged(v)
		case driver.ItemViewProps:
			if it.ViewProps != nil {
				if v.props.LabelledBy != it.ViewProps.LabelledBy {
					labels = append(labels, v)
				}
				v.props = *it.ViewProps
				if v.props.Disabled && w.drop.over == v {
					w.setDropOver(nil)
				}
				w.accessChanged(v)
			}
		case driver.ItemScrollConfig:
			old, inset := scrollbar.RestInset(&v.scroll, v.refreshing), scrollbar.RestInset(it.Scroll, v.refreshing)
			to := v.offset
			if old != inset {
				to = scrollbar.KeepStart(old, inset, to)
			}
			v.scroll = *it.Scroll
			if to != v.offset {
				w.moveOffset(v, to)
			}
			w.refreshChanged(v)
			rescroll = append(rescroll, v) // insets change the offsets
		case driver.ItemScrollContent:
			v.size = it.Size
			w.changed(v)
			rescroll = append(rescroll, v)
		case driver.ItemFocus, driver.ItemBlur:
			focus = append(focus, focusChange{v, it.Kind == driver.ItemFocus})
		case driver.ItemImage:
			if w.setImage(v, it.Image) {
				loads = append(loads, v)
			}
			w.changed(v)
		case driver.ItemModal:
			// A modal is a view: it shows without a separate window, and
			// comes in as it is first inserted (present).
			v.modal = it.Modal.Animation
		case driver.ItemCanvas:
			w.setCanvas(v, it.Drawing)
			w.setCanvasParts(v, it.Drawing.Nodes)
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
				w.redrawEffect(v)
			}
		case driver.ItemWeb:
			w.configureWeb(v, it.Web)
		case driver.ItemWebCommand:
			w.webCommand(v, it.WebCommand, it.Value, it.Data)
		case driver.ItemVideo:
			w.configureVideo(v, it.Video)
		case driver.ItemVideoSeek:
			if v.video != nil {
				v.video.seek(it.Time)
			}
		case driver.ItemNative:
			w.configureNative(v, it.Native)
		case driver.ItemNativeCommand:
			if v.native != nil && v.native.comp != nil {
				v.native.comp.Command(it.Value, it.Data)
			}
		case driver.ItemScrollBy:
			v.applied++
			w.scrollBy(v, it.Point)
		case driver.ItemStickyCells:
			w.setSticky(v, it.Sticky)
		case driver.ItemScrollTo:
			v.applied++
			w.stopWheelGlide()
			w.interruptScroll(v)
			if it.Animated {
				w.scrollAnimated(v, it.Point)
			} else {
				w.scrollTo(v, it.Point)
			}
		}
	}
	w.setRoot(b.Root)
	for _, v := range labels {
		w.updateLabelRelation(v)
	}
	// Bindings change with the batch, and its views exist now.
	w.showAnimated()
	w.updateDropTarget()
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
	w.coverFocus()
	for _, f := range focus {
		switch {
		case f.v == nil:
			if w.focusedView() != nil {
				w.focusEntry(nil)
			}
		case f.v.deleted:
		case f.focus:
			w.focusCommand(f.v, selections[f.v])
		default:
			if w.focusedView() == f.v {
				w.focusEntry(nil)
			}
		}
	}
	w.checkFocus()
	// Views that moved, appeared or went under a still pointer.
	w.hoverViews()
	w.updatePointerCursor()
	w.refreshSelection()
}

func (w *window) create(id driver.NodeID, kind driver.ViewKind) {
	for len(w.views) <= int(id) {
		w.views = append(w.views, nil)
	}
	// A new view has the default style and no props or events: the core
	// sends them only when they differ.
	v := &view{id: id, kind: kind, style: driver.DefaultViewStyle, born: w.batch}
	v.widget = w.newViewWidget(v)
	w.views[id] = v
}

// setRoot makes the view id the surface's child.
func (w *window) setRoot(id driver.NodeID) {
	if old := w.view(w.root); old != nil && w.root != id && old.widget != 0 && gtkWidgetGetParent(old.widget) == w.surface {
		w.unparentWidget(old.widget)
	}
	w.root = id
	if root := w.view(id); root != nil && root.widget != 0 && gtkWidgetGetParent(root.widget) != w.surface {
		if gtkWidgetGetParent(root.widget) != 0 {
			gtkWidgetUnparent(root.widget)
		}
		gtkWidgetSetParent(root.widget, w.surface)
		gtkWidgetQueueAllocate(w.surface)
	}
	gtkWidgetQueueDraw(w.surface)
}

// delete drops v and its subtree. The engine deletes the children too,
// so they are detached but their own items release what they hold.
func (w *window) delete(v *view) {
	w.endEffect(v)
	w.forgetDestination(v)
	w.hovered.Forget(v.id)
	if w.cursorText == v {
		w.cursorText = nil
	}
	v.detach()
	for _, c := range v.children {
		c.parent = nil
	}
	v.children = nil
	v.releaseCanvasParts()
	if v.text != nil {
		v.text.Release()
		v.text = nil
	}
	w.releaseEditor(v)
	if v.web != nil {
		w.closeWeb(v)
	}
	if v.video != nil {
		w.closeVideo(v)
	}
	if v.native != nil {
		w.closeNative(v)
	}
	if v.shader != nil {
		w.unhost(v, &v.shader.hosted)
		v.shader = nil
	}
	v.source.reset(driver.ImageSource{})
	v.placeholder.reset(driver.ImageSource{})
	w.dropWidget(v)
	v.canvas.raster.release()
	v.canvas.raster = nil
	v.deleted, v.dying, v.layout, v.animated = true, false, nil, nil
	if w.views[v.id] == v {
		w.views[v.id] = nil
	}
}

// windowCommand carries out a Window item.
func (w *window) windowCommand(c *driver.WindowCommand) {
	switch c.Action {
	case driver.WindowSetTitleBar:
		w.chrome.titleBar = c.TitleBar
		w.chrome.height = c.Height
		gtkWindowSetDecorated(w.win, boolInt(c.TitleBar == driver.TitleBarDefault && !w.transparent))
		w.showMenuBar()
		w.updateEnvironment()
	case driver.WindowMinimize:
		w.minimize()
	case driver.WindowToggleMaximize:
		w.toggleMaximize()
	case driver.WindowToggleFullScreen:
		if w.chrome.fullScreen {
			gtkWindowUnfullscreen(w.win)
			w.chrome.fullScreen = false
		} else {
			gtkWindowFullscreen(w.win)
			w.chrome.fullScreen = true
		}
		w.showMenuBar()
		w.updateEnvironment()
	case driver.WindowClose:
		if w.primary || w.embedded {
			w.close()
		} else if !w.closing {
			// As the title bar's close button does; GTK's own close does nothing for a window never shown.
			w.engine.Dispatch(driver.Event{Kind: driver.EventWindowClose})
		}
	case driver.WindowShow:
		w.focused = false
		if w.kind == driver.SurfacePanel {
			w.showPanel(c.AnchorBounds, c.Placement)
			break
		}
		w.visible = true
		gtkWidgetSetVisible(w.win, 1)
		gtkWindowPresent(w.win)
	case driver.WindowHide:
		w.visible = false
		gtkWidgetSetVisible(w.win, 0)
	case driver.WindowSetFrame:
		w.setFrame(c.Frame)
	case driver.WindowOrderFront:
		w.focused = false
		if w.kind == driver.SurfacePanel {
			w.showPanel(w.anchorBounds, w.panelSide)
			break
		}
		w.visible = true
		gtkWindowPresent(w.win)
	case driver.WindowFitSize:
		if w.kind == driver.SurfacePanel {
			w.fitPanel(c.Frame.Width, c.Frame.Height)
		}
	case driver.WindowSetAnchor:
		if w.kind == driver.SurfacePanel {
			w.anchorBounds, w.panelSide = c.AnchorBounds, c.Placement
			if w.visible && w.panelSide != driver.WindowPlaceDefault && !w.placePanel() {
				w.placementFailed()
			}
		}
	case driver.WindowSetMenuBar:
		w.chrome.menuBar, w.chrome.revealed = c.MenuBar, false
		w.showMenuBar()
	case driver.WindowSetConstraints:
		w.setConstraints(c.Constraints)
	case driver.WindowSetPointerEvents:
		w.setPointerEvents(c.PointerEvents)
	case driver.WindowSetTransparent:
		if w.kind == driver.SurfaceWindow {
			w.setTransparent(c.On)
		}
	case driver.WindowSetShadow:
		if w.kind == driver.SurfaceWindow {
			w.setShadow(c.On)
		}
	case driver.WindowSetSkipTaskbar:
		if w.kind == driver.SurfaceWindow {
			w.setSkipTaskbar(c.On)
		}
	case driver.WindowSetLevel:
		w.setLevel(c.Level)
	case driver.WindowSetWorkspaces:
		w.setWorkspaces(c.Workspaces)
	}
}

func boolInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// scrollTo moves v's content to offset, clamped to its content, and
// reports the move.
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
	if v.widget != 0 {
		gtkWidgetQueueAllocate(v.widget)
		gtkWidgetQueueDraw(v.widget)
	}
	if w.animator.Scroll(v.id, offset) {
		w.showAnimated()
	}
	if w.animator.Pulls(v.id) && w.animator.Pull(v.id, scrollbar.Pulled(&v.scroll, offset)) {
		w.showAnimated()
	}
	w.pinSticky(v)
	// The engine sees the offset within the content and its insets, as a
	// touch pulls it past.
	w.reportScroll(driver.Event{Kind: driver.EventScroll, ID: v.id, Point: scrollbar.Clamp(&v.scroll, v.frame.Size, v.size, offset), Count: v.applied})
}

// wheel scrolls the innermost scroll view under at that can move in the
// wheel's direction. dx and dy are GTK's scroll deltas in points.
func (w *window) wheel(at driver.Point, dx, dy float64) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	w.stopWheelGlide()
	under := scrollsAt(root, at, nil)
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		if v.scroll.Behavior.NoScroll {
			continue
		}
		offset := v.offset
		if v.scroll.Horizontal {
			if dx == 0 {
				continue
			}
			offset.X += driver.Points(dx)
		} else {
			if dy == 0 {
				continue
			}
			offset.Y += driver.Points(dy)
		}
		from := v.offset
		w.interruptScroll(v) // the wheel takes over
		if w.scrollTo(v, offset) {
			w.wheeled = v
			w.wheelTurned(v, v.offset.X-from.X+v.offset.Y-from.Y)
			return
		}
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

// frameAt returns v's origin in the window's points.
func (w *window) frameAt(v *view) driver.Point {
	var x, y driver.Points
	for p := v; p != nil; p = p.parent {
		x += p.frame.Origin.X - p.offset.X
		y += p.frame.Origin.Y - p.offset.Y
	}
	return driver.Point{X: x, Y: y}
}

// surfaceBounds returns v's frame in the surface and whether v is in the
// root's tree.
func (w *window) surfaceBounds(v *view) (driver.Bounds, bool) {
	b, top := v.frame, v
	for p := v.parent; p != nil; p = p.parent {
		b.Origin.X += p.frame.Origin.X - p.offset.X
		b.Origin.Y += p.frame.Origin.Y - p.offset.Y
		top = p
	}
	return b, top == w.view(w.root)
}

// back reports Escape as the back event when nothing focused took it.
func (w *window) back() {
	w.engine.Dispatch(driver.Event{Kind: driver.EventBack})
}

// setKeymap replaces the app's keyboard shortcuts.
func (w *window) setKeymap(keys []driver.KeyBinding) { w.keys = keys }
