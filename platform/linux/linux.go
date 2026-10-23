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
	"runtime"
	"slices"
	"sync"
	"time"

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
	// Components are the native components Native elements name.
	Components core.NativeComponents
}

// window is one GTK window and the tree it shows. The engine's goroutine
// reaches its main-thread half only through Mount and post.
type window struct {
	win     uintptr // GtkWindow
	surface uintptr // the NsgoView holding the root view
	box     uintptr // the window content: menu bar and surface
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
	natives   map[uintptr]*view // by the objects of hosted widgets
	appID     string
	keys      map[string]bool
	animating []*view
	ticking   bool
	tickID    uint32
	animator  *core.Animator
	epoch     time.Time // the animator's clock starts here
	layouts   []*view   // views a layout animation moves, shows or retires
	playing   []*view   // image views whose animated images play

	components core.NativeComponents
	drop       fileDrop

	lastPointer    core.Point
	shielding      bool // a press kept from a hosted control goes on
	panning        *pan
	bars           []*view  // the scroll views under the pointer, which show their bars
	barDrag        *barDrag // the bar the pointer drags
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
	if err := registerViewType(); err != nil {
		return nil, fmt.Errorf("the Linux backend needs GTK 4: %w", err)
	}
	w := &window{
		entries:    map[uintptr]*view{},
		natives:    map[uintptr]*view{},
		appID:      opts.ID,
		keys:       map[string]bool{},
		actionRefs: map[uintptr]menuRef{},
		env:        core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop},
		components: opts.Components,
		epoch:      time.Now(),
	}
	w.animator = core.NewAnimator(w.animatorReport)
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

// createWindow makes the window, its menu bar box and the surface.
func (w *window) createWindow(opts Options) {
	w.win = gtkWindowNew()
	title := opts.Title
	gtkWindowSetTitle(w.win, cstr(title))
	gtkWindowSetDefaultSize(w.win, int32(opts.Width), int32(opts.Height))
	w.lastWidth, w.lastHeight = opts.Width, opts.Height

	w.box = gtkBoxNew(1, 0) // vertical
	w.surface = w.newViewWidget(nil)
	gtkWidgetSetHexpand(w.surface, 1)
	gtkWidgetSetVexpand(w.surface, 1)
	gtkBoxAppend(w.box, w.surface)
	gtkWindowSetChild(w.win, w.box)
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
	return 0
}

func closeRequestCallback(win uintptr, data uintptr) int32 {
	if w := windowOf(data); w != nil {
		w.close()
	}
	return 1 // the window closes; Run returns after the loop ends
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

// apply applies one commit's items.
func (w *window) apply(b *core.MountBatch) {
	var rescroll, loads []*view
	type focusChange struct {
		v     *view
		focus bool
	}
	var focus []focusChange
	lb := w.newLayoutBatch(b)
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			w.create(it.ID, it.View)
			if lb != nil {
				lb.created[it.ID] = true
			}
			continue
		case core.ItemAnimation:
			w.animations(it.Animation)
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
			}
		case core.ItemRemove:
			v.detach()
			w.unparentWidget(v.widget)
		case core.ItemDelete:
			w.animator.Forget(v.id)
			switch {
			case v.parent != nil && v.parent.dying:
				w.retireInner(v)
			case w.retiring(v, lb):
				w.retire(v, lb)
			default:
				w.delete(v)
			}
		case core.ItemFrame:
			old := v.shownFrame().Size
			w.framed(v, it.Frame, lb)
			resized := v.shownFrame().Size != old
			w.moved(v)
			if resized && v.widget != 0 {
				gtkWidgetQueueAllocate(v.widget)
			}
			if v.kind == core.KindScroll {
				rescroll = append(rescroll, v)
			}
		case core.ItemViewStyle:
			w.setStyle(v, it.ViewStyle)
			w.showWidget(v)
			w.moved(v)
			if v.entry != 0 {
				w.styleEntry(v)
			}
			w.webBackground(v)
			if v.kind == core.KindImage {
				w.playImage(v) // again once it is no longer tinted
			}
		case core.ItemText:
			if v.text != nil {
				v.text.Release()
			}
			v.text = it.Text.Layout
			v.content = it.Text.Content
			w.changed(v)
		case core.ItemInput:
			w.configure(v, it.Input)
		case core.ItemInputValue:
			w.setValue(v, it.Value, it.Count)
		case core.ItemEvents:
			w.setEvents(v, it.Events)
		case core.ItemViewProps:
			if it.ViewProps != nil {
				v.props = *it.ViewProps
				if v.props.Disabled && w.drop.over == v {
					w.setDropOver(nil)
				}
			}
		case core.ItemScrollConfig:
			v.scroll = *it.Scroll
		case core.ItemScrollContent:
			v.size = it.Size
			w.changed(v)
			rescroll = append(rescroll, v)
		case core.ItemFocus, core.ItemBlur:
			focus = append(focus, focusChange{v, it.Kind == core.ItemFocus})
		case core.ItemImage:
			if w.setImage(v, it.Image) {
				loads = append(loads, v)
			}
			w.changed(v)
		case core.ItemModal:
			// A modal is a view: it shows without a separate window.
		case core.ItemCanvas:
			v.drawing = it.Drawing
			w.changed(v)
		case core.ItemWeb:
			w.configureWeb(v, it.Web)
		case core.ItemWebCommand:
			w.webCommand(v, it.WebCommand, it.Value)
		case core.ItemVideo:
			w.configureVideo(v, it.Video)
		case core.ItemVideoSeek:
			if v.video != nil {
				v.video.seek(it.Time)
			}
		case core.ItemNative:
			w.configureNative(v, it.Native)
		case core.ItemNativeCommand:
			if v.native != nil && v.native.comp != nil {
				v.native.comp.Command(it.Value, it.Data)
			}
		case core.ItemScrollTo:
			v.scrolling = nil
			if it.Animated {
				w.scrollAnimated(v, it.Point)
			} else {
				w.scrollTo(v, it.Point)
			}
		}
	}
	w.setRoot(b.Root)
	// Bindings change with the batch, and its views exist now.
	w.showAnimated()
	w.updateDropTarget()
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
}

func (w *window) create(id core.NodeID, kind core.ViewKind) {
	for len(w.views) <= int(id) {
		w.views = append(w.views, nil)
	}
	v := &view{id: id, kind: kind, style: core.ViewStyle{Opacity: 1}}
	v.widget = w.newViewWidget(v)
	w.views[id] = v
}

// setRoot makes the view id the surface's child.
func (w *window) setRoot(id core.NodeID) {
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
	w.forgetDestination(v)
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
		w.unparentWidget(v.entry)
		gObjectUnref(v.entry)
		v.entry = 0
	}
	if v.web != nil {
		w.closeWeb(v)
	}
	if v.video != nil {
		w.closeVideo(v)
	}
	if v.native != nil {
		w.closeNative(v)
	}
	v.source.reset(core.ImageSource{})
	v.placeholder.reset(core.ImageSource{})
	w.dropWidget(v)
	v.deleted, v.dying, v.layout, v.animated = true, false, nil, nil
	if w.views[v.id] == v {
		w.views[v.id] = nil
	}
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
	if v.widget != 0 {
		gtkWidgetQueueAllocate(v.widget)
		gtkWidgetQueueDraw(v.widget)
	}
	if w.animator.Scroll(v.id, offset) {
		w.showAnimated()
	}
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
