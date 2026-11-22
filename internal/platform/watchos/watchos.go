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
	s         *surface
	id        driver.NodeID
	kind      driver.ViewKind
	parent    *view
	children  []*view
	deleted   bool
	presented bool

	frame    driver.Bounds
	style    driver.ViewStyle
	styled   bool
	born     uint64
	props    driver.ViewProps
	events   driver.EventMask
	overflow driver.Bounds

	// transformedKids is ItemOverflow's Transformed: a child may be drawn transformed, outside overflow.
	transformedKids bool

	animated *driver.AnimatedValues
	moving   *motion     // its frame mid layout animation
	trans    *transition // its style mid transition

	// A text's layout and where it goes; a canvas's drawing; an image's
	// loads; a field's state, config, value and its own layout.
	layout  *apple.Layout
	plain   string
	box     driver.Bounds
	drawing *driver.Drawing
	image   *apple.ImageView
	field   *field.State
	value   string
	shown   *apple.Layout

	// A scroll view's config, content size, offset, the app's scrolls
	// applied and its phase (Host.swift's codes).
	scroll  driver.ScrollConfig
	content driver.Size
	offset  driver.Point
	applied uint32
	phase   uint8

	// What the host was last sent: the look, the ink, accessibility state
	// and the ops the look draws over and under the content.
	sentLook   look
	sentInk    driver.Bounds
	sentAccess watchAccess
	draws      bool
	culled     bool
	back       []driver.DrawOp
	front      []driver.DrawOp
	boxFor     boxKey
}

// look is what a node's SwiftUI modifiers show (Host.swift's look op).
type look struct {
	opacity   float64
	hidden    bool
	clips     bool
	radii     driver.Corners
	transform [6]float64
}

// boxKey is the style and size boxOps last drew.
type boxKey struct {
	style driver.ViewStyle
	w, h  float32
}

type surface struct {
	sensors apple.Sensors
	auths   authSessions

	opts        Options
	test        *testOpen
	inputGate   testInputGate
	app         engine.App
	engine      *engine.Engine
	batches     queue.Batches
	batch       uint64
	devGeometry driver.GeometryObserver
	views       []*view
	root        driver.NodeID
	env         driver.Environment
	envPin      func(driver.Environment) driver.Environment
	started     bool
	size        driver.Size
	epoch       time.Time

	w writer
	// dirty are the views whose look, ink or drawing changed since the
	// host last heard.
	dirty  map[*view]bool
	redraw map[*view]bool
	// recull says the frames or offsets that decide which views are
	// culled changed (cull.go).
	recull bool

	animator *driver.Animator
	// frameWanted is set while the app waits for a display frame, by the
	// engine goroutine; ticks mirrors ticking for it, as the host's frames
	// serve the frame without a hop to the main thread.
	frameWanted atomic.Bool
	ticks       atomic.Bool
	ticking     bool
	motions     map[*view]bool
	alerts      map[uint32][]driver.AlertAction
	// focused is the field the system's text input edits.
	focused *view

	touch   touchState
	fingers responderTouches
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
	s := &surface{opts: opts, app: root, epoch: time.Now(), dirty: map[*view]bool{}, redraw: map[*view]bool{},
		motions: map[*view]bool{}, alerts: map[uint32][]driver.AlertAction{}}
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

// Mount implements engine.Platform.
func (s *surface) Mount(b *driver.MountBatch) {
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
		if vw := s.view(id); vw != nil {
			a := v
			vw.animated = &a
			s.touched(vw)
		}
	})
	s.animator.ChangedCanvases(func(id driver.NodeID) {
		if vw := s.view(id); vw != nil {
			s.redraw[vw] = true
		}
	})
}

func (s *surface) view(id driver.NodeID) *view {
	if int(id) < len(s.views) {
		return s.views[id]
	}
	return nil
}

// touched marks v's look and ink to be sent again.
func (s *surface) touched(v *view) {
	s.dirty[v] = true
	s.recull = true
}

func (s *surface) apply(b *driver.MountBatch) {
	s.devGeometry.Begin(b.GeometryVersion)
	defer func() { s.devGeometry.End(); s.reportGeometry() }()
	s.batch++
	var lb *layoutBatch
	if b.LayoutAnimation != nil && !s.motionReduced() {
		lb = &layoutBatch{anim: b.LayoutAnimation, created: map[driver.NodeID]bool{}}
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
			for _, v := range s.views {
				if v != nil && v.field != nil {
					s.w.op(opBlur, v.id)
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
			// The SwiftUI host asks nothing yet (docs/roadmap.md): what a watch keeps answers unknown.
			driver.AnswerPermission(it.Count, *it.Permission, watchPermission, s.engine.Dispatch)
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
		case driver.ItemReadClipboard, driver.ItemPickImages, driver.ItemTakePhoto, driver.ItemPickFiles,
			driver.ItemPickDirectories, driver.ItemOpenFile, driver.ItemRevealFile, driver.ItemTrashFile, driver.ItemShare, driver.ItemSaveFile:
			s.unsupported(it)
			continue
		}
		v := s.view(it.ID)
		if v == nil {
			continue
		}
		switch it.Kind {
		case driver.ItemDelete:
			isModal := v.kind == driver.KindModal
			id := v.id
			s.delete(v)
			if isModal {
				s.engine.Dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: id})
			}
		case driver.ItemRemove:
			s.detach(v)
			s.w.op(opRemove, v.id)
		case driver.ItemInsert:
			p := s.view(it.Parent)
			if p == nil {
				break
			}
			s.detach(v)
			at := len(p.children)
			if before := s.view(it.Before); before != nil && it.Before != 0 {
				for i, c := range p.children {
					if c == before {
						at = i
						break
					}
				}
			}
			p.children = append(p.children, nil)
			copy(p.children[at+1:], p.children[at:])
			p.children[at] = v
			v.parent = p
			s.recull = true
			s.w.op(opInsert, v.id, p.id, it.Before)
			if v.kind == driver.KindModal && !v.presented {
				v.presented = true
				s.engine.Dispatch(driver.Event{Kind: driver.EventModalShown, ID: v.id})
			}
			if lb != nil && lb.created[v.id] {
				s.appear(v, lb)
			}
		case driver.ItemOverflow:
			v.overflow, v.transformedKids = it.Frame, it.Transformed
		case driver.ItemMatch:
			s.match(v, s.view(it.Before), lb)
		case driver.ItemFrame:
			s.setFrame(v, it.Frame, lb)
		case driver.ItemViewStyle:
			s.setStyle(v, it.ViewStyle)
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
			s.redraw[v] = true
		case driver.ItemCanvas:
			v.drawing = it.Drawing
			s.animator.BindCanvas(v.id, it.Drawing.Slots)
			s.touched(v)
			s.redraw[v] = true
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
				s.w.op(opFocus, v.id)
			}
		case driver.ItemBlur:
			if v.field != nil {
				s.w.op(opBlur, v.id)
			}
		case driver.ItemScrollConfig:
			v.scroll = *it.Scroll
			s.sendScroll(v)
		case driver.ItemScrollContent:
			v.content = it.Size
			s.sendScroll(v)
		case driver.ItemScrollTo:
			v.applied++
			to := clampOffset(v, it.Point)
			s.inputGate.expectScroll(uint32(v.id))
			s.w.op(opScrollTo, v.id)
			s.w.f64(float64(to.X), float64(to.Y))
			s.w.flag(it.Animated && !s.motionReduced())
		case driver.ItemScrollBy:
			v.applied++
			to := clampOffset(v, driver.Point{X: v.offset.X + it.Point.X, Y: v.offset.Y + it.Point.Y})
			s.inputGate.expectScroll(uint32(v.id))
			s.w.op(opScrollTo, v.id)
			s.w.f64(float64(to.X), float64(to.Y))
			s.w.flag(false)
		}
	}
	if b.Root != s.root {
		s.root = b.Root
		s.w.op(opRoot, b.Root)
	}
	for _, v := range s.views {
		if v != nil && v.image != nil {
			v.image.Start(float64(s.env.PixelRatio))
		}
	}
	if s.animator.Running() || len(s.motions) > 0 {
		s.tick(true)
	}
}

func (s *surface) create(kind driver.ViewKind, id driver.NodeID) *view {
	v := &view{s: s, id: id, kind: kind, style: driver.DefaultViewStyle, born: s.batch}
	switch kind {
	case driver.KindImage:
		v.image = apple.NewDrawnImage(func() {
			s.touched(v)
			s.redraw[v] = true
			s.flushSoon()
		})
		v.image.Dispatch = func(e driver.Event) { v.report(e) }
	case driver.KindInput:
		v.field = &field.State{ID: id, Dispatch: func(e driver.Event) { s.engine.Dispatch(e) }}
	case driver.KindWeb, driver.KindVideo, driver.KindNative, driver.KindShader:
		// watchOS has no web views, players the app places, components or
		// Metal: they show as empty boxes.
	}
	// The host's kinds are driver.ViewKind's up to canvases; the rest are
	// plain nodes there.
	k := kind
	if k > driver.KindCanvas {
		k = driver.KindView
	}
	s.w.op(opCreate, id)
	s.w.byte(byte(k))
	s.touched(v)
	return v
}

func (s *surface) detach(v *view) {
	if p := v.parent; p != nil {
		for i, c := range p.children {
			if c == v {
				p.children = append(p.children[:i], p.children[i+1:]...)
				break
			}
		}
		v.parent = nil
	}
}

func (s *surface) delete(v *view) {
	s.detach(v)
	v.deleted = true
	v.layout.Release()
	v.shown.Release()
	if v.image != nil {
		v.image.Close()
	}
	s.animator.Forget(v.id)
	delete(s.dirty, v)
	delete(s.redraw, v)
	delete(s.motions, v)
	s.views[v.id] = nil
	if s.touch.pressed == v {
		s.touch.pressed = nil
	}
	s.touch.locked = dropView(s.touch.locked, v)
	s.blocked = dropView(s.blocked, v)
	if s.focused == v {
		s.focused = nil
	}
	s.inputGate.forgetScroll(uint32(v.id))
	s.w.op(opDelete, v.id)
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

// shownFrame is where v shows now: mid layout animation or its own.
func (v *view) shownFrame() driver.Bounds {
	if v.moving != nil && v.moving.move != nil {
		return v.moving.move.Shown()
	}
	return v.frame
}

// shownStyle is the style v shows now: mid transition or its own, with
// the values the animator binds over it.
func (v *view) shownStyle() driver.ViewStyle {
	st := v.style
	if v.trans != nil {
		st = v.trans.shown
	}
	if v.animated != nil {
		st = v.animated.Apply(st)
	}
	return st
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
	for v := range s.dirty {
		if !v.deleted {
			s.sendLook(v)
		}
		delete(s.dirty, v)
	}
	for v := range s.redraw {
		if !v.deleted {
			s.w.op(opRedraw, v.id)
		}
		delete(s.redraw, v)
	}
	if len(s.w.buf) == 0 {
		return
	}
	purego.SyscallN(applyFn, uintptr(unsafe.Pointer(&s.w.buf[0])), uintptr(len(s.w.buf)))
	s.w.buf = s.w.buf[:0]
}

// sendLook sends v's frame, look and ink where they changed.
func (s *surface) sendLook(v *view) {
	f := v.shownFrame()
	st := v.shownStyle()
	w, h := float32(f.Size.Width), float32(f.Size.Height)
	s.w.op(opFrame, v.id)
	s.w.f64(float64(f.Origin.X), float64(f.Origin.Y), float64(f.Size.Width), float64(f.Size.Height))
	k, _ := v.keyframe()
	part, scale := v.layoutPart()
	l := look{opacity: float64(st.Opacity) * float64(k.Opacity) * part, hidden: st.Hidden, transform: [6]float64{1, 0, 0, 1, 0, 0}}
	l.clips = st.Clip || v.kind == driver.KindImage || v.kind == driver.KindInput || v.kind == driver.KindScroll
	if l.clips {
		l.radii = st.Radii(f.Size)
	}
	m := k.Matrix(f.Size).Mul(st.Matrix(f.Size))
	if scale != 1 {
		center := driver.Point{X: f.Size.Width / 2, Y: f.Size.Height / 2}
		m = driver.Transform{Scale: float32(scale)}.Matrix(center).Mul(m)
	}
	if a, b, c, d, e, ff, ok := m.Affine(); ok {
		l.transform = [6]float64{a, b, c, d, e, ff}
	}
	if l != v.sentLook {
		v.sentLook = l
		s.w.op(opLook, v.id)
		s.w.f64(l.opacity)
		s.w.flag(l.hidden)
		s.w.flag(l.clips)
		r := l.radii
		// SwiftUI's order: top leading, bottom leading, bottom trailing,
		// top trailing.
		s.w.f64(float64(r.TopLeft), float64(r.BottomLeft), float64(r.BottomRight), float64(r.TopRight))
		s.w.f64(l.transform[:]...)
	}
	key := boxKey{style: st, w: w, h: h}
	if key != v.boxFor {
		v.boxFor = key
		v.back, v.front = boxOps(&st, w, h)
		s.redraw[v] = true
	}
	draws := len(v.back) > 0 || len(v.front) > 0 || v.layout != nil || v.drawing != nil || v.image != nil || v.field != nil
	ink := inkOf(&st, w, h)
	if v.layout != nil {
		cf := v.layout.ContentFrame(float64(w), float64(h), apple.Rect(float64(v.box.Origin.X), float64(v.box.Origin.Y),
			float64(v.box.Size.Width), float64(v.box.Size.Height)))
		ink = union(ink, driver.Bounds{Origin: driver.Point{X: driver.Points(cf.Origin.X), Y: driver.Points(cf.Origin.Y)},
			Size: driver.Size{Width: driver.Points(cf.Size.Width), Height: driver.Points(cf.Size.Height)}})
	}
	if draws != v.draws || ink != v.sentInk {
		v.draws, v.sentInk = draws, ink
		s.w.op(opInk, v.id)
		s.w.flag(draws)
		s.w.f64(float64(ink.Origin.X), float64(ink.Origin.Y), float64(ink.Size.Width), float64(ink.Size.Height))
		delete(s.redraw, v) // a new ink draws again
	}
}

func union(a, b driver.Bounds) driver.Bounds {
	x0, y0 := min(a.Origin.X, b.Origin.X), min(a.Origin.Y, b.Origin.Y)
	x1 := max(a.Origin.X+a.Size.Width, b.Origin.X+b.Size.Width)
	y1 := max(a.Origin.Y+a.Size.Height, b.Origin.Y+b.Size.Height)
	return driver.Bounds{Origin: driver.Point{X: x0, Y: y0}, Size: driver.Size{Width: x1 - x0, Height: y1 - y0}}
}

// hostDraw draws node id's look and content into ctx, the node's ink
// rectangle w by h, its y axis down.
func hostDraw(id uint32, ctx uintptr, w, h float64) {
	v := s.view(driver.NodeID(id))
	if v == nil || ctx == 0 {
		return
	}
	f := v.shownFrame()
	ink := v.sentInk
	apple.DrawTopDown(ctx, h, false, func(baseUp bool) {
		apple.TranslateCTM(ctx, -float64(ink.Origin.X), -float64(ink.Origin.Y))
		host := apple.CanvasHost{Ratio: float64(s.env.PixelRatio), Owner: v, BaseUp: baseUp,
			Redraw: func() { s.redraw[v] = true; s.flushSoon() }}
		if len(v.back) > 0 {
			apple.Play(ctx, v.back, host)
		}
		switch {
		case v.layout != nil:
			box := v.box
			r := apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), float64(box.Size.Width), float64(box.Size.Height))
			v.layout.DrawBackgrounds(ctx, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y), 0, 0))
			v.layout.Draw(ctx, float64(f.Size.Height), r, baseUp)
		case v.drawing != nil:
			apple.Play(ctx, s.animator.CanvasOps(v.id, v.drawing), host)
		case v.image != nil:
			v.image.Draw(ctx)
		case v.field != nil:
			s.drawField(v, ctx, baseUp)
		}
		if len(v.front) > 0 {
			apple.Play(ctx, v.front, host)
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
	s.w.op(opScroll, v.id)
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
	s.w.access(v.id, a)
}

func hostScroll(id uint32, x, y float64, phase uint8) {
	if s == nil || !s.inputGate.allowScroll(s.test != nil, id, phase) {
		return
	}
	v := s.view(driver.NodeID(id))
	if v == nil {
		return
	}
	if phase == phaseOffset {
		v.offset = driver.Point{X: driver.Points(x), Y: driver.Points(y)}
		if s.animator.Scroll(v.id, v.offset) {
			s.tick(true)
		}
		s.recull = true
		v.scrollEvent(driver.EventScroll)
		s.flush()
		return
	}
	was := v.phase
	v.phase = phase
	switch {
	case phase == phaseDragging && was != phaseDragging:
		// A drag that scrolls is no press, and cancels touches unless a responder blocked it.
		s.cancelPress()
		s.touchesTaken()
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

// openURL answers that url did not open: watchOS opens no URL for an
// app but through SwiftUI's openURL, which takes only the system's own.
func (s *surface) openURL(_ string, request uint32) {
	s.engine.Dispatch(driver.Event{Kind: driver.EventOpenURL, Count: request})
}

// watchPermission is whether watchOS keeps the permission name: a watch has no camera, photo library, media library, or wired notifications.
func watchPermission(name string) int {
	switch name {
	case driver.PermissionCamera, driver.PermissionPhotos, driver.PermissionMediaLibrary, driver.PermissionNotifications:
		return driver.PermissionUnavailable
	}
	return driver.PhonePermission(name)
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
