//go:build darwin && !ios

package macos

import (
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// The app's own surface and the surfaces it opens. Run starts the app on
// a surface in no window, own, which shows no view: the app opens its
// windows, panels and tray views (driver.ItemOpenSurface), and own makes
// each a surface of its own, attached to the engine, as a window, an
// NSPanel or a view in a status item's button.

// own is the app's own surface while Run runs; perf is the performance
// monitor its first window shows.
var (
	own     *surface
	ownPerf *engine.PerfMonitor
)

// NSApplication's activation policies.
const policyRegular, policyAccessory = 0, 1

// startOwn starts app's engine on the app's own surface.
func startOwn(app engine.App, opts Options) *surface {
	o := opts
	o.Width, o.Height = 0, 0
	s := newSurface(o, false)
	s.opts = opts
	own = s
	s.env = s.environment()
	if opts.Perf {
		ownPerf = engine.NewPerfMonitor()
	}
	eopts := engine.Options{ReadAsset: imageload.ReadAsset, Environment: s.env, NetworkWatcher: apple.NetworkWatcher{}, Perf: ownPerf, Thread: apple.RaiseThread,
		Appearance: opts.Appearance, Host: opts.Host, Secrets: apple.Keychain{},
		Windows: &engine.WindowOptions{Title: opts.Title, Width: opts.Width, Height: opts.Height, Frame: opts.Frame,
			Material: opts.Material, Constraints: opts.Constraints, Transparent: opts.Transparent, NoShadow: opts.NoShadow,
			SkipTaskbar: opts.SkipTaskbar, Hidden: opts.Hidden}}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	imageload.UseDiskCache(eopts.CacheDir)
	s.engine = engine.Start(s, app, eopts)
	s.reportFrameWindow(s.env)
	e := s.engine
	apple.SetNotificationPress(func(id string) { e.Dispatch(driver.Event{Kind: driver.EventNotificationPress, Text: id}) })
	apple.SetPurchaseUpdates(e.Dispatch)
	observeApp()
	return s
}

// windowSurface returns the surface whose window is w.
func windowSurface(w objc.ID) *surface {
	for s := range surfaces {
		if s.window == w && !s.embedded {
			return s
		}
	}
	return nil
}

// classPanel is the class of panels: borderless windows that take the keyboard without activating the app; classStillPanel that of panels that never take it (SurfaceConfig.NoActivate), so the app the user types in keeps its key window and caret.
var classPanel, classStillPanel objc.Class

// Window style masks and levels.
const (
	styleTitled, styleClosable, styleMiniaturizable, styleResizable = 1, 2, 4, 8
	styleNonactivating                                              = 1 << 7
	levelStatus                                                     = 25
	joinsAllSpaces, fullScreenAuxiliary                             = 1, 1 << 8
)

// openSurface opens the surface cfg asks for and attaches it to the
// app's engine.
func (o *surface) openSurface(cfg driver.SurfaceConfig) {
	opts := o.opts
	opts.Width, opts.Height, opts.Material = cfg.Width, cfg.Height, cfg.Material
	if cfg.Kind == driver.SurfaceTrayIcon {
		s := newSurface(opts, true)
		s.id, s.kind, s.tray = cfg.ID, cfg.Kind, cfg.Tray
		s.attach(o.engine)
		if tr := trays[cfg.Tray]; tr != nil {
			tr.show(s)
		}
		return
	}
	var window objc.ID
	if cfg.Kind == driver.SurfacePanel {
		window = newPanel(cfg)
	} else {
		style := uint(styleTitled | styleClosable | styleMiniaturizable | styleResizable)
		if cfg.Transparent {
			style = styleMiniaturizable | styleResizable
		}
		class := classWindow
		if o.test != nil {
			class = unconstrainedTestWindow()
		}
		window = objc.ID(class).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, float64(cfg.Width), float64(cfg.Height)), style, uint(2), false)
		title := cfg.Title
		if title == "" {
			title = opts.Title
		}
		window.Send(sel("setTitle:"), apple.NSString(title))
		// Tab goes through the fields and the views that take presses.
		window.Send(sel("setAutorecalculatesKeyViewLoop:"), true)
	}
	window.Send(sel("setReleasedWhenClosed:"), false)
	s := newSurface(opts, false)
	s.id, s.kind, s.hideOnBlur, s.noActivate = cfg.ID, cfg.Kind, cfg.HideOnBlur, cfg.NoActivate
	if o.test != nil {
		// A test's app: its windows take the test's input without activating it, with a pointer of their own, and keep their alerts for the test to answer.
		s.test, s.alerts, s.testPointer = o.test, map[uint32]shownAlert{}, &apple.CGPoint{X: -1e6, Y: -1e6}
	}
	// A window or a panel opened transparent (look.go, newPanel).
	s.transparent = cfg.Transparent
	s.window = window
	s.windowOwner = o
	if cfg.Parent != 0 {
		for parent := range surfaces {
			if parent.windowOwner == o && parent.id == cfg.Parent && parent.kind == driver.SurfaceWindow && parent.window != 0 {
				s.windowParent = parent
				parent.windowChildren = append(parent.windowChildren, s)
				break
			}
		}
	}
	// A panel is on every space from the start (newPanel); setConstraints applies a window's.
	s.workspaces = cfg.Workspaces
	if cfg.Kind == driver.SurfacePanel {
		s.workspaces = driver.WindowWorkspacesAll
	}
	s.setWindowDelegate()
	window.Send(sel("setContentView:"), s.native)
	if cfg.Kind == driver.SurfacePanel && !s.transparent {
		// A panel's content is clipped to its rounded corners.
		s.native.Send(selLayer).Send(sel("setCornerRadius:"), float64(12))
		s.native.Send(selLayer).Send(sel("setMasksToBounds:"), true)
	}
	s.setMaterial(cfg.Material)
	s.setPointerEvents(cfg.PointerEvents)
	if cfg.Kind == driver.SurfaceWindow {
		s.applyBackground()
		window.Send(selSetHasShadow, !cfg.NoShadow)
		s.setSkipTaskbar(cfg.SkipTaskbar)
		s.setConstraints(cfg.Constraints)
		if cfg.Main {
			cfg.Frame = s.devFrame.restore(cfg.Frame)
		}
		if cfg.Frame.Move {
			s.openFrame = cfg.Frame.OnScreen(usableScreens())
			s.setWindowFrame(s.openFrame)
		} else if s.windowParent != nil {
			s.centerOverParent()
		} else {
			window.Send(sel("center"))
		}
	}
	// Attach once the frame is set so the app's first layout sees the
	// window's opening screen and content size.
	s.attach(o.engine)
	if cfg.Main && s.test != nil {
		s.test.main.Store(s)
	}
	s.devFrame.start(s)
	// Shown before its first batch, the window would show AppKit's title
	// bar and no content until the app's title bar, frame and views come.
	s.showFirst = !cfg.Hidden
}

// newPanel returns a panel of cfg's size, above the windows of every space: titled for its rounded corners and shadow, with no title bar or controls; or, transparent, borderless, clear and without a shadow, so that only what the app draws shows, its own corners and shadow among it.
func newPanel(cfg driver.SurfaceConfig) objc.ID {
	if classPanel == 0 {
		no := func(objc.ID, objc.SEL) bool { return false }
		yes := func(objc.ID, objc.SEL) bool { return true }
		classPanel = register("NsgoPanel", apple.Class("NSPanel"), nil, []objc.MethodDef{
			{Cmd: sel("canBecomeKeyWindow"), Fn: yes},
		})
		classStillPanel = register("NsgoStillPanel", apple.Class("NSPanel"), nil, []objc.MethodDef{
			{Cmd: sel("canBecomeKeyWindow"), Fn: no},
			{Cmd: sel("canBecomeMainWindow"), Fn: no},
		})
	}
	class := classPanel
	if cfg.NoActivate {
		class = classStillPanel
	}
	style := uint(styleTitled | styleNonactivating | maskFullSize)
	if cfg.Transparent {
		style = styleNonactivating // NSWindowStyleMaskBorderless is 0
	}
	p := objc.ID(class).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(0, 0, float64(cfg.Width), float64(cfg.Height)), style, uint(2), false)
	if cfg.Transparent {
		p.Send(selSetOpaque, false)
		p.Send(selSetBackgroundColor, objc.ID(apple.Class("NSColor")).Send(selClearColor))
		p.Send(sel("setHasShadow:"), false)
	} else {
		p.Send(sel("setTitlebarAppearsTransparent:"), true)
		p.Send(sel("setTitleVisibility:"), titleHidden)
		for i := 0; i < 3; i++ {
			if b := p.Send(selStandardButton, i); b != 0 {
				b.Send(selSetHidden, true)
			}
		}
	}
	p.Send(sel("setMovable:"), false)
	p.Send(sel("setFloatingPanel:"), true)
	p.Send(sel("setHidesOnDeactivate:"), false)
	p.Send(sel("setLevel:"), levelStatus)
	p.Send(sel("setCollectionBehavior:"), uint(joinsAllSpaces|fullScreenAuxiliary))
	return p
}

// attach attaches an opened surface to the app's engine, as start starts
// one: at its view's size, in its environment.
func (s *surface) attach(app *engine.Engine) {
	s.env = s.environment()
	s.engine = app.Attach(s.id, s)
	size := objc.Send[apple.CGRect](s.native, selBounds).Size
	s.engine.Resize(driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)}, driver.Geometry{})
	s.engine.SetEnvironment(s.env)
	s.reportFrameWindow(s.env)
	if s.kind == driver.SurfaceTrayIcon {
		observeApp()
		return
	}
	s.observe()
	if s.hideOnBlur {
		center := objc.ID(apple.Class("NSNotificationCenter")).Send(sel("defaultCenter"))
		s.stops = append(s.stops, apple.Observe(center, apple.Global(appKitPath, "NSWindowDidResignKeyNotification"),
			func(note objc.ID) {
				if note.Send(sel("object")) == s.window && objc.Send[bool](s.window, sel("isVisible")) {
					s.hidePanel(true)
				}
			}))
	}
	if ownPerf != nil && s.kind == driver.SurfaceWindow && !ownHud {
		ownHud = true
		s.perf = ownPerf
		s.hud = newHud(s, ownPerf)
		s.startTicking()
	}
}

// ownHud is set once a window shows the performance monitor.
var ownHud bool

// show shows an opened surface's window: a panel under tray icon anchor, or a window without activating the app.
func (s *surface) show(anchor uint32) {
	if s.kind == driver.SurfacePanel {
		s.showPanel(anchor, driver.Bounds{}, driver.WindowPlaceDefault)
		return
	}
	if s.showChildWindow() {
		s.window.Send(sel("orderFront:"), objc.ID(0))
		s.syncChildWindows()
	}
}

// showPanel shows a panel and gives it the keyboard, unless it never takes it: beside at on side when side is set (core.Window.ShowAt), else under tray icon anchor, else in the upper middle of the screen with the mouse, as Spotlight shows.
func (s *surface) showPanel(anchor uint32, at driver.Bounds, side driver.WindowPlacement) {
	s.anchor, s.anchorBounds, s.placement = anchor, at, side
	s.movePanel(anchor, at, side)
	s.orderPanelFront()
}

// movePanel moves a panel where showPanel shows it, without showing it.
func (s *surface) movePanel(anchor uint32, at driver.Bounds, side driver.WindowPlacement) {
	w := s.window
	w.Send(sel("setFrameOrigin:"), panelOrigin(anchor, at, side, objc.Send[apple.CGRect](w, sel("frame")).Size))
}

// panelOrigin returns the AppKit origin of a panel's frame of size where showPanel shows it.
func panelOrigin(anchor uint32, at driver.Bounds, side driver.WindowPlacement, size apple.CGSize) apple.CGPoint {
	if side != driver.WindowPlaceDefault {
		return placePanel(at, size, side)
	}
	frame := apple.CGRect{Size: size}
	var screen objc.ID
	if tr := trays[anchor]; tr != nil && tr.item != 0 {
		bar := tr.item.Send(sel("button")).Send(selWindow)
		icon := objc.Send[apple.CGRect](bar, sel("frame"))
		screen = bar.Send(sel("screen"))
		frame.Origin.X = icon.Origin.X + icon.Size.Width/2 - frame.Size.Width/2
		frame.Origin.Y = icon.Origin.Y - frame.Size.Height - 6
	} else {
		mouse := objc.Send[apple.CGPoint](objc.ID(apple.Class("NSEvent")), sel("mouseLocation"))
		screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
		for i := range objc.Send[int](screens, sel("count")) {
			sc := screens.Send(sel("objectAtIndex:"), i)
			r := objc.Send[apple.CGRect](sc, sel("frame"))
			if mouse.X >= r.Origin.X && mouse.X < r.Origin.X+r.Size.Width && mouse.Y >= r.Origin.Y &&
				mouse.Y < r.Origin.Y+r.Size.Height {
				screen = sc
				break
			}
		}
		if screen == 0 {
			screen = objc.ID(apple.Class("NSScreen")).Send(sel("mainScreen"))
		}
		r := objc.Send[apple.CGRect](screen, sel("frame"))
		frame.Origin.X = r.Origin.X + (r.Size.Width-frame.Size.Width)/2
		frame.Origin.Y = r.Origin.Y + r.Size.Height*2/3 - frame.Size.Height/2
	}
	if screen != 0 {
		// Inside the screen's visible part.
		v := objc.Send[apple.CGRect](screen, sel("visibleFrame"))
		frame.Origin.X = max(v.Origin.X+8, min(frame.Origin.X, v.Origin.X+v.Size.Width-frame.Size.Width-8))
		frame.Origin.Y = max(v.Origin.Y+8, min(frame.Origin.Y, v.Origin.Y+v.Size.Height-frame.Size.Height))
	}
	return frame.Origin
}

// fitPanel sizes a panel's content to width by height, the size its content takes (driver.WindowFitSize), keeping its top edge: one that shows beside the rectangle ShowAt placed it at, or under its tray icon, takes its new size there in the same move.
func (s *surface) fitPanel(width, height driver.Points) {
	w := s.window
	frame := objc.Send[apple.CGRect](w, sel("frame"))
	content := objc.Send[apple.CGRect](w, sel("contentRectForFrameRect:"), frame).Size
	top := frame.Origin.Y + frame.Size.Height
	frame.Size.Width += float64(width) - content.Width
	frame.Size.Height += float64(height) - content.Height
	frame.Origin.Y = top - frame.Size.Height
	if tr := trays[s.anchor]; objc.Send[bool](w, sel("isVisible")) && (s.placement != driver.WindowPlaceDefault || tr != nil && tr.item != 0) {
		frame.Origin = panelOrigin(s.anchor, s.anchorBounds, s.placement, frame.Size)
	}
	w.Send(sel("setFrame:display:"), frame, true)
}

// orderPanelFront shows a panel in front, as the key window unless it never takes the keyboard: then orderFrontRegardless leaves the key window, and the app in front, as they are.
func (s *surface) orderPanelFront() {
	if s.noActivate {
		s.window.Send(sel("orderFrontRegardless"))
	} else {
		s.window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	}
}

// placePanel returns the AppKit origin of a panel of size beside at, a rectangle in the coordinates of displays, on side: inside the visible frame of the display that holds at's center, else of the primary display.
func placePanel(at driver.Bounds, size apple.CGSize, side driver.WindowPlacement) apple.CGPoint {
	infos := displays()
	if len(infos) == 0 {
		return apple.CGPoint{X: float64(at.Origin.X), Y: float64(at.Origin.Y)}
	}
	center := driver.Point{X: at.Origin.X + at.Size.Width/2, Y: at.Origin.Y + at.Size.Height/2}
	area := infos[0].VisibleFrame
	for _, d := range infos {
		if d.Frame.Contains(center) {
			area = d.VisibleFrame
			break
		}
	}
	p, _ := driver.PlaceWindow(at, driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)}, area, side)
	// displays measures from the primary screen's top left, y down; AppKit from its bottom left, y up.
	primary := screenFrames("frame")[0]
	return apple.CGPoint{X: primary.Origin.X + float64(p.X), Y: primary.Origin.Y + primary.Size.Height - float64(p.Y) - size.Height}
}

// hidePanel hides a panel; blurred says it lost the focus, which the app
// hears.
func (s *surface) hidePanel(blurred bool) {
	s.window.Send(sel("orderOut:"), objc.ID(0))
	if !blurred {
		return
	}
	if tr := trays[s.anchor]; tr != nil {
		tr.blurred = time.Now()
	}
	s.engine.Dispatch(driver.Event{Kind: driver.EventWindowHidden})
}

// destroy closes an opened surface whose tree is gone.
func (s *surface) destroy() {
	if s.closed {
		return
	}
	window := s.window
	if s.embedded {
		// The host's View holds the native view until its Close releases it, so an embedded surface whose engine ends on a fault only stops.
		s.native.Send(selRemoveFromSuperview)
		s.close()
		return
	}
	if window != 0 {
		s.closePopup()
		// The window does not retain its delegate (setWindowDelegate).
		d := window.Send(sel("delegate"))
		window.Send(selSetDelegate, objc.ID(0))
		if d != 0 {
			d.Send(selRelease)
		}
		window.Send(sel("orderOut:"), objc.ID(0))
	}
	s.close()
	delete(views, s.native)
	s.native.Send(selRelease)
	if window != 0 {
		// Closed once the batch that destroyed it has returned.
		apple.OnMain(func() {
			window.Send(sel("close"))
			window.Send(selRelease)
		})
	}
}

// setDock shows the app in the Dock and the app switcher, or not.
func setDock(on bool) {
	policy := policyAccessory
	if on {
		policy = policyRegular
	}
	app := nsApp()
	if objc.Send[int](app, sel("activationPolicy")) == policy {
		return
	}
	app.Send(sel("setActivationPolicy:"), policy)
	if on {
		// The menu bar becomes the app's once it activates again.
		activateApp()
	}
}
