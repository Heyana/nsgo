//go:build linux && !android

// Package linux is NSGo's GTK 4 backend. One GtkWindow shows a
// GtkDrawingArea that draws the whole tree with Cairo, text the engine
// measured and laid out with Pango, and inputs and web views as native
// GtkEntry and WebKitWebView widgets above it. GTK delivers the pointer, wheel and keyboard events;
// the engine hit tests presses and keeps the interaction state.
//
// The backend is the one the roadmap asks for: GTK 4 through purego, no
// cgo and no hand-written C. It loads its libraries at run time, so an
// app built on macOS needs only Go and Xcode, and an app built for Linux
// needs GTK 4.14 or newer, Pango and Cairo where it runs. Web views also
// need WebKitGTK 6.0, which loads on the first one; without it a web view
// stays empty and reports an error.
//
// The application menu is a GTK menu bar above the surface. The backend
// does not select text, expose accessibility, draw window controls of its
// own or animate modals. A partly clipped input is hidden, because GTK
// has no per-widget clip rectangle; a web view is clipped to a rectangle,
// without its ancestors' rounded corners. Inputs and web views stay above
// everything the surface draws, including views drawn over them.
package linux

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
	"github.com/oneo-me/nsgo/platform/internal/queue"
)

// Options configure the window.
type Options struct {
	Title         string
	Width, Height core.Points
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
}

// window is one GTK window and the tree it shows. The engine's goroutine
// reaches its main-thread half only through Mount and post.
type window struct {
	win     uintptr // GtkWindow
	area    uintptr // GtkDrawingArea
	box     uintptr // the window content: menu bar and overlay
	overlay uintptr
	fixed   uintptr // GtkFixed hosting the entries
	menuBar uintptr
	loop    uintptr // GMainLoop

	env core.Environment

	engine  *engine.Engine
	batches queue.Batches
	chrome  chrome

	// The engine goroutine hands these to the main thread.
	mu    sync.Mutex
	tasks []func()

	// The main thread's. views is indexed by NodeID.
	views     []*view
	root      core.NodeID
	entries   map[uintptr]*view
	webs      map[uintptr]*view // by page, holder and content manager
	keys      map[string]bool
	animating []*view
	ticking   bool
	tickID    uint32

	lastPointer    core.Point
	panning        *pan
	pinching       *twoFinger
	rotating       *twoFinger
	closing        bool
	lastWidth      core.Points
	lastHeight     core.Points
	started        bool
	handle         uintptr
	measureMap     uintptr                       // the engine thread's Pango font map
	extents        map[core.Font]core.FontExtent // the engine thread's font metrics
	pressConsumed  bool
	focus          *view // the entry with keyboard focus
	pinchTimeout   uint32
	clipboardCount uint32
	alerts         []core.AlertConfig
	alerting       bool
	alertingID     uint32
	menuCounter    uint32
	actionGroup    uintptr
	actionRefs     map[uintptr]menuRef
}

// Run opens a window showing app and runs GTK's main loop. Call it from
// main; it returns when the window closes.
func Run(app engine.App, opts Options) {
	w, err := start(app, opts)
	if err != nil {
		panic("nsgo: " + err.Error())
	}
	w.run()
	w.stop()
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
	runtime.LockOSThread()
	if gtkInitCheck() == 0 {
		return nil, errors.New("no display; run the app in a desktop session or with xvfb-run")
	}
	w := &window{
		entries:    map[uintptr]*view{},
		webs:       map[uintptr]*view{},
		keys:       map[string]bool{},
		actionRefs: map[uintptr]menuRef{},
		env:        core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop},
	}
	w.handle = registerWindow(w)
	w.chrome.keys = map[string]menuRef{}
	w.createWindow(opts)
	w.env = w.environment()

	eopts := engine.Options{
		Size:        core.Size{Width: opts.Width, Height: opts.Height},
		Environment: w.env,
	}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	w.engine = engine.Start(w, app, eopts)
	w.loop = gMainLoopNew(0, 0)
	w.setupSettings()
	return w, nil
}

// run presents the window and runs GTK's main loop until it closes.
func (w *window) run() {
	gtkWindowPresent(w.win)
	gMainLoopRun(w.loop)
}

// stop ends the engine and frees the loop after run returns.
func (w *window) stop() {
	w.engine.Close()
	engine.SyncStorage()
	if w.measureMap != 0 {
		gObjectUnref(w.measureMap)
		w.measureMap = 0
	}
	if w.loop != 0 {
		gMainLoopUnref(w.loop)
		w.loop = 0
	}
	// The handle stays registered: GTK may still call a callback of a
	// window whose loop has stopped as its widgets are finalized.
}

// createWindow makes the window, its menu bar box, the drawing area and
// the overlay that hosts the entries.
func (w *window) createWindow(opts Options) {
	w.win = gtkWindowNew()
	title := opts.Title
	gtkWindowSetTitle(w.win, cstr(title))
	gtkWindowSetDefaultSize(w.win, int32(opts.Width), int32(opts.Height))
	w.lastWidth, w.lastHeight = opts.Width, opts.Height

	w.box = gtkBoxNew(1, 0) // vertical
	w.overlay = gtkOverlayNew()
	w.area = gtkDrawingAreaNew()
	gtkWidgetSetHexpand(w.area, 1)
	gtkWidgetSetVexpand(w.area, 1)
	gtkOverlaySetChild(w.overlay, w.area)

	w.fixed = gtkFixedNew()
	gtkWidgetSetHexpand(w.fixed, 1)
	gtkWidgetSetVexpand(w.fixed, 1)
	gtkOverlayAddOverlay(w.overlay, w.fixed)
	gtkBoxAppend(w.box, w.overlay)
	gtkWindowSetChild(w.win, w.box)
	gtkDrawingAreaSetDrawFunc(w.area, cbDraw, w.handle, 0)
	w.setupInputs()
	gSignalConnect(w.win, "close-request", cbCloseRequest, w.handle)
}

// setupSettings rereads the environment whenever GTK's settings change.
func (w *window) setupSettings() {
	if settings := gtkSettingsGetDefault(); settings != 0 {
		for _, name := range []string{"gtk-theme-name", "gtk-application-prefer-dark-theme", "gtk-xft-dpi", "gtk-enable-animations"} {
			gSignalConnect(settings, "notify::"+name, cbSettingsChanged, w.handle)
		}
	}
}

// Mount implements engine.Platform: it queues the batch and wakes the
// main loop once.
func (w *window) Mount(b *core.MountBatch) {
	if w.batches.Push(b) {
		gIdleApply(cbApply, w.handle)
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
	for _, b := range w.batches.Drain() {
		w.apply(b)
	}
	gtkWidgetQueueDraw(w.area)
	return 0
}

func closeRequestCallback(win uintptr, data uintptr) int32 {
	if w := windowOf(data); w != nil {
		w.close()
	}
	return 1 // the window closes; Run returns after the loop ends
}

func drawCallback(area uintptr, cr uintptr, width, height int32, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	w.draw(cr, width, height)
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

// close ends the app: the engine stops, and the main loop returns.
func (w *window) close() {
	if w.closing {
		return
	}
	w.closing = true
	if w.tickID != 0 {
		gSourceRemove(w.tickID)
		w.tickID = 0
	}
	if w.engine != nil {
		w.engine.Close()
	}
	if w.loop != 0 {
		gMainLoopQuit(w.loop)
	}
}

func (w *window) view(id core.NodeID) *view {
	if id > 0 && int(id) < len(w.views) {
		return w.views[id]
	}
	return nil
}

// draw paints the tree, reports the size GTK gave the area, and tells GTK
// about the new size as the user resizes the window.
func (w *window) draw(cr uintptr, width, height int32) {
	size := core.Size{Width: core.Points(width), Height: core.Points(height)}
	if !w.started {
		// The surface exists now, so its scale and direction are known.
		w.started = true
		w.updateEnvironment()
	} else if size.Width != w.lastWidth || size.Height != w.lastHeight {
		if w.engine != nil {
			w.engine.Resize(size)
		}
	}
	w.lastWidth, w.lastHeight = size.Width, size.Height
	w.paint(cr, size)
}

// apply applies one commit's items.
func (w *window) apply(b *core.MountBatch) {
	var rescroll, loads []*view
	type focusChange struct {
		v     *view
		focus bool
	}
	var focus []focusChange
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			w.create(it.ID, it.View)
			continue
		case core.ItemAlert:
			w.showAlert(it.Alert)
			continue
		case core.ItemDismissKeyboard:
			focus = append(focus, focusChange{})
			continue
		case core.ItemSetClipboard:
			w.setClipboard(it.Value)
			continue
		case core.ItemReadClipboard:
			w.readClipboard(it.Count)
			continue
		case core.ItemKeymap:
			w.setKeymap(it.Keys)
			continue
		case core.ItemTitle:
			gtkWindowSetTitle(w.win, cstr(it.Value))
			continue
		case core.ItemWindow:
			if it.Window != nil {
				w.windowCommand(it.Window)
			}
			continue
		case core.ItemMenu:
			w.setMenu(it.Menu)
			continue
		case core.ItemAnnounce, core.ItemStatusBar, core.ItemBackUnhandled:
			continue
		}
		v := w.view(it.ID)
		if v == nil {
			continue
		}
		switch it.Kind {
		case core.ItemInsert:
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
			}
		case core.ItemRemove:
			v.detach()
		case core.ItemDelete:
			w.delete(v)
		case core.ItemFrame:
			v.frame = it.Frame
			if v.kind == core.KindScroll {
				rescroll = append(rescroll, v)
			}
		case core.ItemViewStyle:
			w.setStyle(v, it.ViewStyle)
			if v.entry != 0 {
				w.styleEntry(v)
			}
		case core.ItemText:
			if v.text != nil {
				v.text.Release()
			}
			v.text = it.Text.Layout
			v.content = it.Text.Content
		case core.ItemInput:
			w.configure(v, it.Input)
		case core.ItemInputValue:
			w.setValue(v, it.Value, it.Count)
		case core.ItemEvents:
			v.events = it.Events
		case core.ItemViewProps:
			if it.ViewProps != nil {
				v.props = *it.ViewProps
			}
		case core.ItemScrollConfig:
			v.scroll = *it.Scroll
		case core.ItemScrollContent:
			v.size = it.Size
			rescroll = append(rescroll, v)
		case core.ItemFocus, core.ItemBlur:
			focus = append(focus, focusChange{v, it.Kind == core.ItemFocus})
		case core.ItemImage:
			if w.setImage(v, it.Image) {
				loads = append(loads, v)
			}
		case core.ItemModal:
			// A modal is a view: it shows without a separate window.
		case core.ItemCanvas:
			v.drawing = it.Drawing
		case core.ItemWeb:
			w.configureWeb(v, it.Web)
		case core.ItemWebCommand:
			w.webCommand(v, it.WebCommand, it.Value)
		case core.ItemScrollTo:
			v.scrolling = nil
			if it.Animated {
				w.scrollAnimated(v, it.Point)
			} else {
				w.scrollTo(v, it.Point)
			}
		}
	}
	w.root = b.Root
	for _, v := range rescroll {
		if !v.deleted {
			w.scrollTo(v, v.offset)
		}
	}
	for _, v := range loads {
		if !v.deleted {
			w.load(v, &v.placeholder, false)
			w.load(v, &v.source, true)
		}
	}
	w.placeWidgets()
	for _, f := range focus {
		switch {
		case f.v == nil:
			if w.focusedEntry() != nil {
				w.focusEntry(nil)
			}
		case f.v.deleted || f.v.entry == 0:
		case f.focus:
			w.focusEntry(f.v)
		default:
			if w.focusedEntry() == f.v {
				w.focusEntry(nil)
			}
		}
	}
	gtkWidgetQueueDraw(w.area)
}

func (w *window) create(id core.NodeID, kind core.ViewKind) {
	for len(w.views) <= int(id) {
		w.views = append(w.views, nil)
	}
	w.views[id] = &view{id: id, kind: kind, style: core.ViewStyle{Opacity: 1}}
}

// delete drops v and its subtree. The engine deletes the children too,
// so they are detached but their own items release what they hold.
func (w *window) delete(v *view) {
	v.detach()
	for _, c := range v.children {
		c.parent = nil
	}
	v.children = nil
	if v.text != nil {
		v.text.Release()
		v.text = nil
	}
	if v.provider != 0 {
		gObjectUnref(v.provider)
		v.provider = 0
	}
	if v.entry != 0 {
		if w.focus == v {
			w.focus = nil
		}
		delete(w.entries, v.entry)
		if parent := gtkWidgetGetParent(v.entry); parent != 0 {
			// Unparenting drops GTK's only reference: the widget was
			// floating when gtk_fixed_put sank it.
			gtkFixedRemove(w.fixed, v.entry)
		}
		v.entry = 0
	}
	if v.web != nil {
		w.closeWeb(v)
	}
	v.source.reset(core.ImageSource{})
	v.placeholder.reset(core.ImageSource{})
	v.deleted = true
	w.views[v.id] = nil
}

// windowCommand carries out a Window item.
func (w *window) windowCommand(c *core.WindowCommand) {
	switch c.Action {
	case core.WindowSetTitleBar:
		w.chrome.titleBar = c.TitleBar
		w.chrome.height = c.Height
		gtkWindowSetDecorated(w.win, boolInt(c.TitleBar == core.TitleBarDefault))
		w.showMenuBar()
		w.updateEnvironment()
	case core.WindowMinimize:
		gtkWindowMinimize(w.win)
	case core.WindowToggleMaximize:
		if w.chrome.max {
			gtkWindowUnmaximize(w.win)
			w.chrome.max = false
		} else {
			gtkWindowMaximize(w.win)
			w.chrome.max = true
		}
	case core.WindowToggleFullScreen:
		if w.chrome.fullScreen {
			gtkWindowUnfullscreen(w.win)
			w.chrome.fullScreen = false
		} else {
			gtkWindowFullscreen(w.win)
			w.chrome.fullScreen = true
		}
		w.showMenuBar()
		w.updateEnvironment()
	case core.WindowClose:
		w.close()
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
func (w *window) scrollTo(v *view, offset core.Point) bool {
	offset.X = min(max(offset.X, 0), max(v.size.Width-v.frame.Size.Width, 0))
	offset.Y = min(max(offset.Y, 0), max(v.size.Height-v.frame.Size.Height, 0))
	if offset == v.offset {
		return false
	}
	v.offset = offset
	w.engine.Dispatch(core.Event{Kind: core.EventScroll, ID: v.id, Point: offset})
	return true
}

// wheel scrolls the innermost scroll view under at that can move in the
// wheel's direction. dx and dy are GTK's scroll deltas in points.
func (w *window) wheel(at core.Point, dx, dy float64) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	under := scrollsAt(root, at, nil)
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		offset := v.offset
		if v.scroll.Horizontal {
			if dx == 0 {
				continue
			}
			offset.X += core.Points(dx)
		} else {
			if dy == 0 {
				continue
			}
			offset.Y += core.Points(dy)
		}
		v.scrolling = nil
		if w.scrollTo(v, offset) {
			w.placeWidgets()
			gtkWidgetQueueDraw(w.area)
			return
		}
	}
}

// background is the color behind v: its own or its nearest ancestor's.
func background(v *view) core.Color {
	for ; v != nil; v = v.parent {
		if v.style.Background&0xff != 0 {
			return v.style.Background
		}
	}
	return core.RGB(0xffffff)
}

// frameAt returns v's origin in the window's points.
func (w *window) frameAt(v *view) core.Point {
	var x, y core.Points
	for p := v; p != nil; p = p.parent {
		x += p.frame.Origin.X - p.offset.X
		y += p.frame.Origin.Y - p.offset.Y
	}
	return core.Point{X: x, Y: y}
}

// surfaceBounds returns v's frame in the surface and whether v is in the
// root's tree.
func (w *window) surfaceBounds(v *view) (core.Bounds, bool) {
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
	w.engine.Dispatch(core.Event{Kind: core.EventBack})
}

// setKeymap replaces the app's keyboard shortcuts.
func (w *window) setKeymap(keys []string) {
	w.keys = map[string]bool{}
	for _, k := range keys {
		w.keys[k] = true
	}
}
