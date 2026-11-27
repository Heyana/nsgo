//go:build ios

// Package watchos runs an app on Apple Watch. watchOS has no public UIKit
// or Core Animation, so the views are SwiftUI's, in the host the tool
// compiles with the app (cmd/nsgo/watch/Host.swift): this package keeps
// the views' state, applies each commit to the host's tree as a batch of
// ops, draws each view's look and content with Core Graphics into its
// Canvas, hit-tests touches and runs the animations on the host's frames,
// as the Linux backend does on GTK's.
package watchos

import (
	"os"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/field"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/queue"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Options configure Start.
type Options struct {
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
	// Fonts are font files under the app's resources, registered before
	// the first view.
	Fonts []string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
}

// view is the Go side of one node of the host's tree.
type view struct {
	*watchNode
	s *surface

	// A text's layout and where it goes; a canvas's drawing; an image's
	// loads; a field's state, config, value and its own layout.
	layout    *apple.Layout
	plain     string
	box       driver.Bounds
	drawing   *driver.Drawing
	frozenOps []driver.DrawOp // canvas ops frozen while retired as a leaving ghost
	image     *apple.ImageView
	field     *field.State
	value     string
	shown     *apple.Layout
}

func viewOf(n *watchNode) *view {
	if n == nil || n.owner == nil {
		return nil
	}
	return n.owner.(*view)
}

type surface struct {
	watchTree
	sensors apple.Sensors
	auths   authSessions
	perms   watchPermissions

	opts        Options
	test        *testOpen
	inputGate   testInputGate
	app         engine.App
	engine      *engine.Engine
	batches     queue.Batches
	devGeometry driver.GeometryObserver
	env         driver.Environment
	envPin      func(driver.Environment) driver.Environment
	started     bool
	size        driver.Size

	animator *driver.Animator
	// frameWanted is set while the app waits for a display frame, by the
	// engine goroutine; ticks mirrors ticking for it, as the host's frames
	// serve the frame without a hop to the main thread.
	frameWanted atomic.Bool
	ticks       atomic.Bool
	ticking     bool
	alerts      map[uint32][]driver.AlertAction
	// focused is the field the system's text input edits.
	focused *view

	touch   touchState
	fingers responderTouches
	crown   crownRouter
	blocked []*view // the scroll views a touch responder locked (ItemBlockNative)
}

func init() {
	// SwiftUI runs on the process's main thread, which main starts on.
	runtime.LockOSThread()
	// A watch counts every page Go's heap holds: collect once it grows
	// by half rather than doubles, unless GOGC says else.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
}

var (
	s *surface
	// nsgo_watch_apply, tick and snapshot, Host.swift's.
	applyFn, tickFn, snapshotFn uintptr
	// snapshot is the PNG the host last rendered (hostSnapshot).
	snapshot []byte
	// referenceDate is Foundation's, which the host's dates count from.
	referenceDate = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
)

func hostSnapshot(data *byte, n int) {
	if n > 0 {
		snapshot = append([]byte(nil), unsafe.Slice(data, n)...)
	}
}

// testOpen is what a test host opens the app with: the test's options and
// the environment it pins.
type testOpen struct {
	options testhost.Options
	pin     engine.TestPin
}

// Run shows root full screen and runs the host's SwiftUI app, which
// reports its size to start the engine. Call it from main; it does not
// return.
func Run(root engine.App, opts Options) {
	s = newSurface(root, opts)
	runHost()
}

func newSurface(root engine.App, opts Options) *surface {
	s := &surface{opts: opts, app: root, alerts: map[uint32][]driver.AlertAction{}}
	s.initTree(time.Now())
	s.animator = driver.NewAnimator(func(e driver.Event) { s.engine.Dispatch(e) })
	return s
}

// runHost hands the host the bridge and runs its app on this thread.
func runHost() {
	apple.RegisterFonts(s.opts.Fonts)
	// Before the SwiftUI app finishes launching, so a notification press that launched it is heard.
	apple.WatchNotifications()
	apple.ObserveProcessPower(func() {
		thermal, lowPower := apple.ProcessPower()
		if thermal != s.env.ThermalState || lowPower != s.env.LowPowerMode {
			s.env.ThermalState, s.env.LowPowerMode = thermal, lowPower
			if s.engine != nil {
				s.engine.SetEnvironment(s.env)
			}
		}
	})
	sym := func(name string) uintptr {
		addr, err := purego.Dlsym(purego.RTLD_DEFAULT, name)
		if err != nil {
			panic("watchos: the host has no " + name + ": " + err.Error())
		}
		return addr
	}
	connect, main := sym("nsgo_watch_connect"), sym("nsgo_watch_main")
	applyFn, tickFn, snapshotFn = sym("nsgo_watch_apply"), sym("nsgo_watch_tick"), sym("nsgo_watch_snapshot")
	table := []uintptr{
		purego.NewCallback(hostSize),
		purego.NewCallback(hostTouch),
		purego.NewCallback(hostDraw),
		purego.NewCallback(hostScroll),
		purego.NewCallback(hostFrame),
		purego.NewCallback(hostText),
		purego.NewCallback(hostAlert),
		purego.NewCallback(hostLifecycle),
		purego.NewCallback(hostSnapshot),
		purego.NewCallback(hostAuth),
		purego.NewCallback(hostPermission),
		purego.NewCallback(hostCrown),
	}
	purego.SyscallN(connect, uintptr(unsafe.Pointer(&table[0])), uintptr(len(table)))
	// The table is copied; the callbacks live as long as the process.
	purego.SyscallN(main)
}

func hostSize(w, h, scale, top, left, bottom, right float64, flags uint32) {
	size := driver.Size{Width: driver.Points(w), Height: driver.Points(h)}
	env := s.environment(scale, driver.Edges{Top: driver.Points(top), Left: driver.Points(left),
		Bottom: driver.Points(bottom), Right: driver.Points(right)}, flags)
	if !s.started {
		s.size, s.env = size, env
		if s.app != nil {
			s.started = true
			s.start(nil)
		} else if hostReady != nil {
			// A test host's: the test opens its app.
			close(hostReady)
			hostReady = nil
		}
		return
	}
	if size != s.size {
		s.size = size
		s.engine.Resize(size, driver.Geometry{})
	}
	if !env.Equal(s.env) {
		s.env = env
		s.engine.SetEnvironment(env)
	}
}

// hostReady is closed once a test host's surface has its size.
var hostReady chan struct{}

// start starts the engine at the surface's size; test, when set, is what
// a test host opens it with.
func (s *surface) start(test *testOpen) {
	opts := engine.Options{ReadAsset: imageload.ReadAsset, Size: s.size, Environment: s.env, NetworkWatcher: apple.NetworkWatcher{}, Thread: apple.RaiseThread, Appearance: s.opts.Appearance,
		Secrets: apple.Keychain{}}
	opts.SetDirs(engine.AppDirs(s.opts.ID))
	s.test = test
	if test != nil {
		size := opts.Size
		opts.UseTest(test.options, &test.pin)
		s.envPin = opts.Pin
		// A watch's screen is what it is.
		opts.Size = size
	}
	imageload.UseDiskCache(opts.CacheDir)
	s.engine = engine.Start(s, s.app, opts)
	e := s.engine
	apple.SetNotificationPress(func(id string) { e.Dispatch(driver.Event{Kind: driver.EventNotificationPress, Text: id}) })
}

// environment is what the host told of the surface, with the system's.
func (s *surface) environment(scale float64, safe driver.Edges, flags uint32) driver.Environment {
	locale, rtl := apple.Locale()
	thermal, lowPower := apple.ProcessPower()
	env := buildEnvironment(scale, safe, flags, locale, rtl, apple.LocaleSettings(), osVersion(), s.env.Lifecycle)
	env.DeviceModel = apple.ModelIdentifier()
	env.ThermalState = thermal
	env.LowPowerMode = lowPower
	// watchOS's notification center schedules once, daily and weekly notifications natively.
	env.ScheduledNotifications = apple.NotificationsAvailable()
	env.Speak = apple.SpeakAvailable()
	env.CanRenderDrawings = true
	return env
}

// osVersion is the system's.
func osVersion() driver.OSVersion {
	type version struct{ Major, Minor, Patch int }
	v := objc.Send[version](objc.ID(apple.Class("NSProcessInfo")).Send(objc.RegisterName("processInfo")),
		objc.RegisterName("operatingSystemVersion"))
	return driver.OSVersion{Major: v.Major, Minor: v.Minor}
}

func hostLifecycle(state uint8) {
	l := driver.LifecycleActive
	switch state {
	case 1:
		l = driver.LifecycleInactive
	case 2:
		l = driver.LifecycleBackground
	}
	if l == s.env.Lifecycle {
		return
	}
	s.env.Lifecycle = l
	if s.engine != nil {
		s.engine.SetEnvironment(s.env)
		// watchOS suspends an app in the background and ends it unheard,
		// so the OnQuit handlers run as it goes there.
		if l == driver.LifecycleBackground {
			s.engine.Hide()
		}
	}
}

// notify carries out a notification's item through watchOS's UNUserNotificationCenter.
func (s *surface) notify(it *driver.MountItem) {
	dispatchWatchNotification(it, watchNotificationBackend{
		permission: apple.NotificationPermission,
		show:       apple.ShowNotification,
		close:      apple.CloseNotification,
	}, s.engine.Dispatch)
}

// RegisterFont implements the engine's font registration.
func (s *surface) RegisterFont(data []byte) error { return apple.RegisterFont(data) }

// Prepare implements driver.TextEngine with Core Text, as iOS does.
func (s *surface) Prepare(t *driver.AttributedText) driver.PreparedText {
	return apple.TextEngine{}.Prepare(t)
}

var watchRenderDrawing atomic.Pointer[func(*driver.RenderDrawingRequest, uint32, func(driver.Event))]

// EnableCanvas implements driver.CanvasEnabler: it links the offscreen decoder only into apps that draw a canvas.
func (s *surface) EnableCanvas() {
	apple.EnableCanvas()
	if watchRenderDrawing.Load() == nil {
		fn := apple.RenderDrawingAsync
		watchRenderDrawing.Store(&fn)
	}
}

// Mount implements engine.Platform.
func (s *surface) Mount(b *driver.MountBatch) {
	driver.RetainBatchDrawings(b)
	if s.batches.Push(b) {
		apple.OnMain(func() {
			for _, b := range s.batches.Drain() {
				driver.Apply(b, s.apply)
			}
			s.applyAnimated()
			s.flush()
		})
	}
}

// RequestFrame implements engine.Platform. It hops to the main thread
// only to start the host's frames: an app asking every frame keeps them
// running, and the next serves it.
func (s *surface) RequestFrame() {
	s.frameWanted.Store(true)
	if !s.ticks.Load() {
		apple.OnMain(func() { s.tick(true) })
	}
}

func (s *surface) SubmitGPU(driver.GPUFrame) {}

// tick starts or stops the host's display frames.
func (s *surface) tick(on bool) {
	if on == s.ticking {
		return
	}
	s.ticking = on
	s.ticks.Store(on)
	var v uintptr
	if on {
		v = 1
	}
	purego.SyscallN(tickFn, v)
}

// hostFrame steps a frame of the host's TimelineView, whose date, when the frame shows, is shown seconds since 2001 on the wall clock.
func hostFrame(shown float64) {
	defer s.reportGeometry()
	now := time.Since(s.epoch)
	s.animator.Step(now)
	s.applyAnimated()
	s.stepMotions()
	if s.frameWanted.Swap(false) {
		s.engine.FrameAt(driver.DisplayTime(time.Duration(shown*float64(time.Second)), time.Since(referenceDate)))
	}
	s.flush()
	apple.YieldFrame()
	if !s.frameWanted.Load() && !s.animator.Running() && len(s.motions) == 0 {
		s.tick(false)
		// The app asked for a frame as the frames stopped, seeing them
		// still running.
		if s.frameWanted.Load() {
			s.tick(true)
		}
	}
}

// applyAnimated applies bound values after a commit and on each host frame.
// A value that starts without a running animation still needs to reach the
// host before the first frame, as a checked switch's initial position does.
func (s *surface) applyAnimated() {
	s.animator.Changed(func(id driver.NodeID, v driver.AnimatedValues) {
		if n := s.node(id); n != nil {
			a := v
			n.animated = &a
			s.touchedNode(n)
		}
	})
	s.animator.ChangedCanvases(func(id driver.NodeID) {
		if n := s.node(id); n != nil {
			s.ensureMaps()
			s.redraw[n] = true
		}
	})
	if s.sticky.repinBound(func(id driver.NodeID) (driver.Point, bool, bool) {
		if vw := s.view(id); vw != nil && !vw.deleted {
			return vw.offset, vw.scroll.Horizontal, true
		}
		return driver.Point{}, false, false
	}, s.animator, &s.w) {
		s.recull = true
	}
}

func (s *surface) view(id driver.NodeID) *view {
	return viewOf(s.node(id))
}

func (s *surface) hostView(hostID driver.NodeID) *view {
	return viewOf(s.hostNode(hostID))
}

// touched marks v's look and ink to be sent again.
func (s *surface) touched(v *view) {
	s.touchedNode(v.watchNode)
}

func (s *surface) apply(b *driver.MountBatch) {
	s.devGeometry.Begin(b.GeometryVersion)
	defer func() { s.devGeometry.End(); s.reportGeometry() }()
	s.batch++
	now := time.Now()
	lb := s.newLayoutBatch(b.LayoutAnimation, now, s.motionReduced())
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate:
			v := s.create(it.View, it.ID)
			if lb != nil {
				lb.created[v.watchNode] = true
			}
			continue
		case driver.ItemAnimation:
			s.animator.Apply(it.Animation)
			continue
		case driver.ItemBlockNative:
			s.blockNative(s.view(it.ID))
			continue
		case driver.ItemAlert:
			s.alert(it.Alert)
			continue
		case driver.ItemDiskSpace:
			go apple.DiskSpace(it.Value, it.Count, false, s.engine.Dispatch)
			continue
		case driver.ItemImageInfo:
			apple.ImageInfo(it, s.engine.Dispatch)
			continue
		case driver.ItemKeepImage:
			apple.KeepCanvasImage(s, it.Value)
			continue
		case driver.ItemReleaseImage:
			apple.ReleaseCanvasImage(s, it.Value)
			continue
		case driver.ItemImageCacheLimit:
			apple.SetImageCacheLimit(it.Bytes)
			continue
		case driver.ItemOpenURL:
			s.openURL(it.Value, it.Count)
			continue
		case driver.ItemDismissKeyboard:
			for _, n := range s.views {
				if v := viewOf(n); v != nil && v.field != nil {
					s.w.op(opBlur, v.hostID)
				}
			}
			continue
		case driver.ItemSensor:
			s.sensors.Handle(it, s.engine.Dispatch, true, true)
			continue
		case driver.ItemSpeech:
			// watchOS has no Speech framework: the session reports it
			// unavailable.
			apple.HandleSpeech(it.Count, *it.Speech, s.engine.Dispatch)
			continue
		case driver.ItemSpeak:
			dispatchWatchSpeak(it, apple.HandleSpeak, s.engine.Dispatch)
			continue
		case driver.ItemBiometricAvailability, driver.ItemBiometricAuthenticate:
			// LocalAuthentication's biometric policy is not on watchOS.
			s.engine.Dispatch(driver.NoBiometric(it))
			continue
		case driver.ItemWebCookies:
			// watchOS has no WebKit.
			s.engine.Dispatch(driver.NoWebCookies(it))
			continue
		case driver.ItemAuthSession:
			if it.Auth != nil {
				s.auths.handle(it.Count, *it.Auth, &s.w, s.engine.Dispatch)
			}
			continue
		case driver.ItemPermission:
			if it.Permission != nil {
				s.perms.handle(it.Count, *it.Permission, &s.w, apple.NotificationPermission, s.engine.Dispatch)
			}
			continue
		case driver.ItemNotificationPermission, driver.ItemShowNotification, driver.ItemCloseNotification:
			s.notify(it)
			continue
		case driver.ItemBadge:
			// A watch app's icon has no badge.
			continue
		case driver.ItemReleaseOffscreen:
			apple.ReleaseOffscreen(it.OffscreenID)
			continue
		case driver.ItemInstallPackage:
			s.engine.Dispatch(driver.NoPackageInstaller(it.Count))
			continue
		case driver.ItemCapture:
			s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoCapture})
			continue
		case driver.ItemOpenDirectory, driver.ItemReleaseDirectory:
			s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoDirectoryPersistence})
			continue
		case driver.ItemRenderDrawing:
			if fn := watchRenderDrawing.Load(); fn != nil {
				(*fn)(it.RenderDrawing, it.Count, s.engine.Dispatch)
			} else {
				s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoDrawingRenderer})
			}
			continue
		case driver.ItemReadClipboard, driver.ItemPickImages, driver.ItemTakePhoto, driver.ItemPickFiles,
			driver.ItemPickDirectories, driver.ItemOpenFile, driver.ItemRevealFile, driver.ItemTrashFile, driver.ItemShare, driver.ItemSaveFile:
			s.unsupported(it)
			continue
		}
		v := s.view(it.ID)
		if v == nil {
			if it.Kind == driver.ItemCanvas {
				it.Drawing.Release()
			}
			continue
		}
		switch it.Kind {
		case driver.ItemDelete:
			isModal := v.kind == driver.KindModal
			id := v.id
			s.deleteView(v, lb)
			if isModal {
				s.engine.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: id})
			}
		case driver.ItemRemove:
			s.removeNode(v.watchNode)
		case driver.ItemInsert:
			p := s.view(it.Parent)
			if p == nil {
				break
			}
			var before *watchNode
			if it.Before != 0 {
				before = s.node(it.Before)
			}
			s.insertNode(v.watchNode, p.watchNode, before, lb)
			if v.kind == driver.KindModal && !v.presented {
				v.presented = true
				s.engine.Dispatch(driver.Event{Kind: driver.EventModalShown, ID: v.id})
			}
		case driver.ItemOverflow:
			v.overflow, v.transformedKids = it.Frame, it.Transformed
		case driver.ItemMatch:
			s.matchNode(v.watchNode, s.node(it.Before), lb)
		case driver.ItemFrame:
			s.setFrame(v, it.Frame, lb, now)
		case driver.ItemViewStyle:
			s.setNodeStyle(v.watchNode, it.ViewStyle, now, s.motionReduced())
		case driver.ItemViewProps:
			v.props = *it.ViewProps
			s.sendAccess(v)
		case driver.ItemAccessibilityLabel:
			v.props.Label = it.Value
			s.sendAccess(v)
		case driver.ItemEvents:
			v.events = it.Events
			if v.field != nil {
				v.field.Events = it.Events
			}
			s.sendAccess(v)
		case driver.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.plain = it.Text.Text
			v.box = it.Text.Content
			s.sendAccess(v)
			s.touched(v)
			s.redraw[v.watchNode] = true
		case driver.ItemCanvas:
			was := v.drawing
			v.drawing = it.Drawing
			was.Release()
			s.animator.BindCanvas(v.id, it.Drawing.Slots)
			s.touched(v)
			s.redraw[v.watchNode] = true
		case driver.ItemImage:
			v.image.Configure(it.Image)
			s.touched(v)
		case driver.ItemInput:
			s.configureInput(v, it.Input)
		case driver.ItemInputValue:
			if v.field != nil && v.field.Value(it.Value, it.Count) {
				v.value = it.Value
				s.relayoutField(v)
				s.sendInput(v)
			}
		case driver.ItemFocus:
			if v.field != nil && fieldCanFocus(v.field.Config) {
				s.w.op(opFocus, v.hostID)
			}
			if crownKindOf(v.watchNode, s.viewInputDisabled(v.watchNode)) != crownNone {
				s.crown.setFocus(v.id)
			}
		case driver.ItemBlur:
			if v.field != nil {
				s.w.op(opBlur, v.hostID)
			}
			s.crown.clearFocus(v.id)
		case driver.ItemScrollConfig:
			axisChanged := v.scroll.Horizontal != it.Scroll.Horizontal
			v.scroll = *it.Scroll
			s.sendScroll(v)
			if axisChanged && s.sticky.pin(v.id, v.offset, v.scroll.Horizontal, s.animator) {
				s.recull = true
			}
		case driver.ItemScrollContent:
			v.content = it.Size
			s.sendScroll(v)
		case driver.ItemScrollTo:
			v.applied++
			to := clampOffset(v, it.Point)
			s.inputGate.expectScroll(uint32(v.hostID))
			s.w.op(opScrollTo, v.hostID)
			s.w.f64(float64(to.X), float64(to.Y))
			s.w.flag(it.Animated && !s.motionReduced())
		case driver.ItemScrollBy:
			v.applied++
			to := clampOffset(v, driver.Point{X: v.offset.X + it.Point.X, Y: v.offset.Y + it.Point.Y})
			s.inputGate.expectScroll(uint32(v.hostID))
			s.w.op(opScrollTo, v.hostID)
			s.w.f64(float64(to.X), float64(to.Y))
			s.w.flag(false)
		case driver.ItemStickyCells:
			if s.sticky.set(v.id, it.Sticky, v.offset, v.scroll.Horizontal, s.animator, &s.w) {
				s.recull = true
			}
		}
	}
	s.setRootNode(b.Root)
	for _, n := range s.views {
		if v := viewOf(n); v != nil && v.image != nil {
			v.image.Start(float64(s.env.PixelRatio))
		}
	}
	if s.animator.Running() || len(s.motions) > 0 {
		s.tick(true)
	}
}

func (s *surface) create(kind driver.ViewKind, id driver.NodeID) *view {
	n := s.createNode(kind, id)
	v := &view{watchNode: n, s: s}
	n.owner = v
	switch kind {
	case driver.KindImage:
		v.image = apple.NewDrawnImage(func() {
			if !n.deleted || n.retired {
				s.touchedNode(n)
				s.redraw[n] = true
				s.flushSoon()
			}
		})
		v.image.Dispatch = func(e driver.Event) { v.report(e) }
	case driver.KindInput:
		v.field = &field.State{ID: id, Dispatch: func(e driver.Event) { s.engine.Dispatch(e) }}
	case driver.KindWeb, driver.KindVideo, driver.KindNative, driver.KindShader:
		// watchOS has no web views, players the app places, components or
		// Metal: they show as empty boxes.
	}
	return v
}

func (s *surface) retirePayload(n *watchNode) {
	v := viewOf(n)
	if v != nil && v.drawing != nil {
		v.frozenOps = s.animator.CanvasOps(n.id, v.drawing)
	}
	s.animator.Forget(n.id)
	if v != nil {
		if s.touch.pressed == v {
			s.touch.pressed = nil
		}
		s.touch.locked = dropView(s.touch.locked, v)
		s.blocked = dropView(s.blocked, v)
		if s.focused == v {
			s.focused = nil
			s.w.op(opBlur, v.hostID)
		}
	}
	s.crown.forgetNode(n.id)
	s.inputGate.forgetScroll(uint32(n.hostID))
	if s.sticky.delete(n.id) {
		s.recull = true
	}
}

func (s *surface) destroyPayload(n *watchNode) {
	v := viewOf(n)
	if v == nil {
		return
	}
	v.layout.Release()
	v.layout = nil
	v.shown.Release()
	v.shown = nil
	v.drawing.Release()
	v.drawing = nil
	if v.image != nil {
		v.image.Close()
		v.image = nil
	}
	v.frozenOps = nil
}

func (s *surface) deleteView(v *view, lb *layoutBatch) {
	if s.deleteNode(v.watchNode, lb, s.retirePayload, s.destroyPayload) {
		s.tick(true)
	}
}

// close reaps any active leaving ghosts on surface teardown.
func (s *surface) close() {
	s.crown.reset()
	s.closeLeaving(s.destroyPayload)
}

// report sends e about v if v listens for its kind.
func (v *view) report(e driver.Event) {
	if v.events.Has(e.Kind) && !v.deleted {
		e.ID = v.id
		if v.s.animator.Gesture(&e) {
			v.s.applyAnimated()
			v.s.tick(true)
		}
		v.s.engine.Dispatch(e)
	}
}

// flushSoon flushes on the next turn of the main thread, for what changes
// outside a batch or a frame, such as an image that loaded.
func (s *surface) flushSoon() { apple.OnMain(s.flush) }

// flush sends the host what changed: each touched view's look and ink,
// the drawings to make again, then the ops the batch wrote.
func (s *surface) flush() {
	if s.recull {
		s.recull = false
		s.cull()
	}
	now := time.Now()
	for n := range s.dirty {
		if !n.deleted || n.retired {
			s.sendLook(n, now)
		}
		delete(s.dirty, n)
	}
	for n := range s.redraw {
		if !n.deleted || n.retired {
			s.w.op(opRedraw, n.hostID)
		}
		delete(s.redraw, n)
	}
	s.syncCrown()
	if len(s.w.buf) == 0 {
		return
	}
	purego.SyscallN(applyFn, uintptr(unsafe.Pointer(&s.w.buf[0])), uintptr(len(s.w.buf)))
	s.w.buf = s.w.buf[:0]
}

func (s *surface) viewInputDisabled(n *watchNode) bool {
	if v := viewOf(n); v != nil && v.field != nil {
		return !fieldCanFocus(v.field.Config)
	}
	return false
}

func (s *surface) focusedNode() *watchNode {
	if s.focused == nil {
		return nil
	}
	return s.focused.watchNode
}

func (s *surface) syncCrown() {
	s.crown.syncMode(&s.w, s.node(s.root), s.focusedNode(), s.viewInputDisabled)
}

func (s *surface) crownCenter(n *watchNode) (local, page driver.Point) {
	f := n.shownFrame()
	local = driver.Point{X: f.Size.Width / 2, Y: f.Size.Height / 2}
	if b, ok := s.surfaceBounds(n); ok {
		page = driver.Point{X: b.Origin.X + b.Size.Width/2, Y: b.Origin.Y + b.Size.Height/2}
	} else {
		page = local
	}
	return local, page
}

func (s *surface) handleCrown(phase uint8, delta, velocity float64) {
	events := s.crown.handle(phase, delta, velocity, s.node(s.root), s.focusedNode(), s.viewInputDisabled, s.crownCenter)
	for _, ev := range events {
		if v := s.view(ev.ID); v != nil && !v.deleted {
			v.report(ev)
		}
	}
	s.flush()
}

func hostCrown(phase uint8, delta, velocity float64) {
	if s == nil || !s.inputGate.allowCrown(s.test != nil) {
		return
	}
	s.handleCrown(phase, delta, velocity)
}

// sendLook sends n's frame, look and ink where they changed.
func (s *surface) sendLook(n *watchNode, now time.Time) {
	f, st := s.writeNodeLook(n, now)
	w, h := float32(f.Size.Width), float32(f.Size.Height)
	v := viewOf(n)
	key := boxKey{style: st, w: w, h: h}
	if key != n.boxFor {
		n.boxFor = key
		n.back, n.front = boxOps(&st, w, h)
		s.redraw[n] = true
	}
	draws := len(n.back) > 0 || len(n.front) > 0 || (v != nil && (v.layout != nil || v.drawing != nil || v.image != nil || v.field != nil))
	ink := inkOf(&st, w, h)
	if v != nil && v.layout != nil {
		cf := v.layout.ContentFrame(float64(w), float64(h), apple.Rect(float64(v.box.Origin.X), float64(v.box.Origin.Y),
			float64(v.box.Size.Width), float64(v.box.Size.Height)))
		ink = union(ink, driver.Bounds{Origin: driver.Point{X: driver.Points(cf.Origin.X), Y: driver.Points(cf.Origin.Y)},
			Size: driver.Size{Width: driver.Points(cf.Size.Width), Height: driver.Points(cf.Size.Height)}})
	}
	if draws != n.draws || ink != n.sentInk {
		n.draws, n.sentInk = draws, ink
		s.w.op(opInk, n.hostID)
		s.w.flag(draws)
		s.w.f64(float64(ink.Origin.X), float64(ink.Origin.Y), float64(ink.Size.Width), float64(ink.Size.Height))
		delete(s.redraw, n) // a new ink draws again
	}
}

func union(a, b driver.Bounds) driver.Bounds {
	x0, y0 := min(a.Origin.X, b.Origin.X), min(a.Origin.Y, b.Origin.Y)
	x1 := max(a.Origin.X+a.Size.Width, b.Origin.X+b.Size.Width)
	y1 := max(a.Origin.Y+a.Size.Height, b.Origin.Y+b.Size.Height)
	return driver.Bounds{Origin: driver.Point{X: x0, Y: y0}, Size: driver.Size{Width: x1 - x0, Height: y1 - y0}}
}

// hostDraw draws host node id's look and content into ctx, the node's ink
// rectangle w by h, its y axis down. Leaving ghosts stay registered in byHost
// so SwiftUI can redraw them while they animate out.
func hostDraw(id uint32, ctx uintptr, w, h float64) {
	n := s.hostNode(driver.NodeID(id))
	if n == nil || (n.deleted && !n.retired) || ctx == 0 {
		return
	}
	v := viewOf(n)
	if v == nil {
		return
	}
	f := n.shownFrame()
	ink := n.sentInk
	apple.DrawTopDown(ctx, h, false, func(baseUp bool) {
		apple.TranslateCTM(ctx, -float64(ink.Origin.X), -float64(ink.Origin.Y))
		host := apple.CanvasHost{Ratio: float64(s.env.PixelRatio), Owner: v, BaseUp: baseUp,
			Redraw: func() {
				if !n.deleted || n.retired {
					s.redraw[n] = true
					s.flushSoon()
				}
			}}
		if len(n.back) > 0 {
			apple.Play(ctx, nil, n.back, host)
		}
		switch {
		case v.layout != nil:
			box := v.box
			r := apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), float64(box.Size.Width), float64(box.Size.Height))
			v.layout.DrawBackgrounds(ctx, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
			v.layout.Draw(ctx, float64(f.Size.Height), r, baseUp)
		case v.drawing != nil:
			s.playCanvas(ctx, v, f.Size, host)
		case v.image != nil:
			v.image.Draw(ctx)
		case v.field != nil:
			s.drawField(v, ctx, baseUp)
		}
		if len(n.front) > 0 {
			apple.Play(ctx, nil, n.front, host)
		}
	})
}

// clampOffset keeps a scroll view's offset inside its content.
func clampOffset(v *view, p driver.Point) driver.Point {
	maxX := max(v.content.Width-v.frame.Size.Width, 0)
	maxY := max(v.content.Height-v.frame.Size.Height, 0)
	return driver.Point{X: min(max(p.X, 0), maxX), Y: min(max(p.Y, 0), maxY)}
}

// sendScroll sends a scroll view's direction, indicator and content.
func (s *surface) sendScroll(v *view) {
	s.w.op(opScroll, v.hostID)
	s.w.flag(v.scroll.Horizontal)
	s.w.flag(!v.scroll.HideIndicator)
	s.w.f64(float64(v.content.Width), float64(v.content.Height))
}

// sendAccess sends v's accessibility state to the host when it changes.
func (s *surface) sendAccess(v *view) {
	inputDisabled := v.field != nil && v.field.Config.Disabled
	a := watchAccessOf(v.kind, &v.props, v.events, v.plain, inputDisabled)
	if a == v.sentAccess {
		return
	}
	v.sentAccess = a
	s.w.access(v.hostID, a)
}

func hostScroll(id uint32, x, y float64, phase uint8) {
	if s == nil || !s.inputGate.allowScroll(s.test != nil, id, phase) {
		return
	}
	v := s.hostView(driver.NodeID(id))
	if v == nil || v.deleted {
		return
	}
	if phase == phaseOffset {
		v.offset = driver.Point{X: driver.Points(x), Y: driver.Points(y)}
		if s.animator.Scroll(v.id, v.offset) {
			s.applyAnimated()
			s.tick(true)
		}
		s.sticky.pin(v.id, v.offset, v.scroll.Horizontal, s.animator)
		s.recull = true
		v.scrollEvent(driver.EventScroll)
		s.flush()
		return
	}
	was := v.phase
	v.phase = phase
	switch {
	case phase == phaseDragging && was != phaseDragging:
		// A drag that scrolls is no press, cancels touches unless a responder blocked it, and returns Digital Crown focus to ScrollViews.
		s.cancelPress()
		s.touchesTaken()
		s.crown.clearAllFocus()
		if was == phaseGliding {
			v.scrollEvent(driver.EventMomentumEnd)
		}
		v.scrollEvent(driver.EventScrollDragStart)
	case phase == phaseGliding:
		if was == phaseDragging {
			v.scrollEvent(driver.EventScrollDragEnd)
		}
		v.scrollEvent(driver.EventMomentumStart)
	case phase == phaseRest:
		switch was {
		case phaseDragging:
			v.scrollEvent(driver.EventScrollDragEnd)
		case phaseGliding:
			v.scrollEvent(driver.EventMomentumEnd)
		}
		s.animator.Settle(v.id)
		if s.animator.Running() {
			s.tick(true)
		}
		v.scrollEvent(driver.EventScrollEnd)
	}
	s.flush()
}

func (v *view) scrollEvent(kind driver.EventKind) {
	defer v.s.reportGeometry()
	v.report(driver.Event{Kind: kind, Point: v.offset, Count: v.applied})
}

// alert shows a, answered by hostAlert.
func (s *surface) alert(a *driver.AlertConfig) {
	s.alerts[a.ID] = a.Actions
	s.w.op(opAlert, driver.NodeID(a.ID))
	s.w.str(a.Title)
	s.w.str(a.Message)
	s.w.byte(byte(len(a.Actions)))
	for _, act := range a.Actions {
		s.w.str(act.Label)
		var role byte
		switch act.Role {
		case driver.AlertCancel:
			role = 1
		case driver.AlertDestructive:
			role = 2
		}
		s.w.byte(role)
	}
}

func hostAlert(id uint32, index int32) {
	if s == nil || !s.inputGate.allowDirect(s.test != nil) {
		return
	}
	if _, ok := s.alerts[id]; !ok {
		return
	}
	delete(s.alerts, id)
	s.engine.Dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: int(index)})
}

// hostAuth copies the callback or failure while the host's UTF-8 is alive.
func hostAuth(id uint32, outcome uint8, data *byte, n int) {
	value := ""
	if data != nil && n > 0 {
		value = string(unsafe.Slice(data, n))
	}
	if s != nil && s.engine != nil {
		s.auths.finished(id, outcome, value, s.engine.Dispatch)
	}
}

// hostPermission completes a permission check or prompt answered by Host.swift.
func hostPermission(id uint32, status uint8) {
	if s != nil && s.engine != nil {
		s.perms.finished(id, status, s.engine.Dispatch)
	}
}

// openURL answers that url did not open: watchOS opens no URL for an
// app but through SwiftUI's openURL, which takes only the system's own.
func (s *surface) openURL(_ string, request uint32) {
	s.engine.Dispatch(driver.Event{Kind: driver.EventOpenURL, Count: request})
}

// errUnsupported is what a service watchOS has not answers with.
const errUnsupported = "unsupported on watchOS"

// unsupported answers a request for a service watchOS has not: no
// clipboard, photos, camera, files or share sheet.
func (s *surface) unsupported(it *driver.MountItem) {
	kind := driver.EventFiles
	switch it.Kind {
	case driver.ItemReadClipboard:
		kind = driver.EventClipboard
	case driver.ItemPickImages, driver.ItemTakePhoto:
		kind = driver.EventPick
	}
	s.engine.Dispatch(driver.Event{Kind: kind, Count: it.Count, Text: errUnsupported})
}

// motionReduced uses a test's pinned preference for the renderer too; the system's environment is kept intact for future preference changes.
func (s *surface) motionReduced() bool { return driver.Pinned(s.env, s.envPin).ReduceMotion }
