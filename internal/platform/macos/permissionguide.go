//go:build darwin && !ios

package macos

import (
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// The permission guide (desktop.GuidePermission) is an NSPanel that never takes the focus, so System Settings stays the active app while the user drags from it. A goroutine follows System Settings through the window server (CGWindowListCopyWindowInfo), which needs no permission, 30 times a second, and has the main thread move the panel under its window as it moves, or hide it while another app is in front. The card holding the app is a view of a class of ours, the drag's source, whose item writes the app to the pasteboard as Finder does. Its look, its place and what it writes follow PermissionFlow through MyGo's port (NOTICE); its pointer stays the arrow.

const (
	nsWindowStyleMaskNonactivatingPanel = 1 << 7
	nsBackingStoreBuffered              = 2
	nsFloatingWindowLevel               = 3
	nsCollectionCanJoinAllSpaces        = 1 << 0
	nsCollectionFullScreenAuxiliary     = 1 << 8
	nsVisualEffectMaterialPopover       = 6
	nsVisualEffectBlendingBehindWindow  = 0
	nsVisualEffectStateActive           = 1
	nsBoxCustom                         = 4
	nsNoTitle                           = 0
	nsDragOperationCopy                 = 1
	nsDraggingFormationNone             = 1
	nsImageOnly                         = 1
	nsImageResizingModeStretch          = 1
	kCGWindowListOptionOnScreenOnly     = 1 << 0
	kCGWindowListExcludeDesktopElements = 1 << 4
	kCGEventSourceStateCombined         = 0
	kCGMouseButtonLeft                  = 0

	settingsBundleID = "com.apple.systempreferences"

	// The panel's layout, in points.
	guidePadding      = 12
	guideRadius       = 16
	guideCardHeight   = 48
	guideIconSize     = 32
	guideCloseSize    = 18
	guideDragIconSize = 56

	// guideTick is how often the guide follows System Settings, and guideStatusTicks how many ticks pass between looks at the status, about 250 ms.
	guideTick        = time.Second / 30
	guideStatusTicks = 7
	// guideMisses is how many ticks in a row System Settings must be missing for the guide to end: it is missing for a moment as it opens or changes panes.
	guideMisses = 12
	// guideSettingsWait is how long System Settings may take to open.
	guideSettingsWait = 10 * time.Second
)

// permissionGuide is one guide, which its surface's request started and its engine hears the end of.
type permissionGuide struct {
	s *surface
	// dispatch hears the guide's end: its surface's engine's.
	dispatch         func(driver.Event)
	id               uint32
	name, app, label string
	lang             string
	draggable        bool
	// stop closes as the guide ends; quiet says it ends unanswered, its surface gone.
	stop     chan struct{}
	stopOnce sync.Once
	quiet    atomic.Bool

	// The panel's state, the main thread's.
	panel      objc.ID
	views      guideViews
	labelH     float64
	shown      bool
	dragOrigin apple.CGPoint
	dragBegan  bool
}

// guideViews are the panel's views, which its content view keeps.
type guideViews struct {
	border, label, close, cardBox, icon, name, hint, hand, card objc.ID
}

var (
	// guideMu guards currentGuide, the guide running, which a new one ends, and liveGuides, each surface's guides yet to answer, which it silences as it closes.
	guideMu      sync.Mutex
	currentGuide *permissionGuide
	liveGuides   = map[*surface]map[*permissionGuide]struct{}{}
	// activeGuide is the guide whose panel shows, which the panel's classes reach: the main thread's.
	activeGuide *permissionGuide
	// guideDragging is set while the app is dragged from the panel, which lets the mouse through until the tracker sees the button up.
	guideDragging atomic.Bool

	guideOnce                  sync.Once
	guideLoaded                bool
	windowListOnce             sync.Once
	windowListLoaded           bool
	cgWindowListCopyWindowInfo func(option, relativeTo uint32) objc.ID
	cgEventSourceButtonState   func(state int32, button uint32) bool
	cfRelease                  func(objc.ID)
	objcPoolPush               func() uintptr
	objcPoolPop                func(uintptr)
	msgSendFloat               func(objc.ID, objc.SEL) float64
)

// loadWindowList registers the window-server calls used by the permission guide.
func loadWindowList() bool {
	windowListOnce.Do(func() {
		cg, err1 := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		cf, err2 := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		lib, err3 := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		purego.RegisterLibFunc(&cgWindowListCopyWindowInfo, cg, "CGWindowListCopyWindowInfo")
		purego.RegisterLibFunc(&cgEventSourceButtonState, cg, "CGEventSourceButtonState")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&objcPoolPush, lib, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&objcPoolPop, lib, "objc_autoreleasePoolPop")
		purego.RegisterLibFunc(&msgSendFloat, lib, "objc_msgSend")
		windowListLoaded = true
	})
	return windowListLoaded
}

// loadGuide registers what the guide calls and its classes, once, on the main thread.
func loadGuide() bool {
	guideOnce.Do(func() {
		if !loadWindowList() {
			return
		}
		registerGuideClasses()
		guideLoaded = true
	})
	return guideLoaded
}

func registerGuideClasses() {
	var source, writing []*objc.Protocol
	if p := objc.GetProtocol("NSDraggingSource"); p != nil {
		source = append(source, p)
	}
	if p := objc.GetProtocol("NSPasteboardWriting"); p != nil {
		writing = append(writing, p)
	}
	register("NsgoPermissionDragView", apple.Class("NSView"), source, []objc.MethodDef{
		{Cmd: sel("mouseDown:"), Fn: guideCardDown},
		{Cmd: sel("mouseDragged:"), Fn: guideCardDragged},
		{Cmd: sel("mouseUp:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			if g := activeGuide; g != nil {
				g.dragBegan = false
			}
		}},
		{Cmd: sel("acceptsFirstMouse:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool { return true }},
		{Cmd: sel("mouseDownCanMoveWindow"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("draggingSession:sourceOperationMaskForDraggingContext:"), Fn: func(objc.ID, objc.SEL, objc.ID, int) uint { return nsDragOperationCopy }},
		{Cmd: sel("ignoreModifierKeysForDraggingSession:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool { return true }},
	})
	register("NsgoPermissionAppWriter", apple.Class("NSObject"), writing, []objc.MethodDef{
		{Cmd: sel("writableTypesForPasteboard:"), Fn: guideWriterTypes},
		{Cmd: sel("pasteboardPropertyListForType:"), Fn: guideWriterPropertyList},
	})
	register("NsgoPermissionGuideTarget", apple.Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("close:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			if g := activeGuide; g != nil {
				g.end()
			}
		}},
	})
}

// guideDefaultApp is the app the guide holds out when the app names none: its bundle, else its executable, never the app that started it.
func guideDefaultApp() string {
	path := apple.GoString(objc.ID(apple.Class("NSBundle")).Send(sel("mainBundle")).Send(sel("bundlePath")))
	if strings.HasSuffix(path, ".app") {
		return path
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return path
}

func notificationPermissionStatus(np int) int {
	switch np {
	case driver.NotificationsNotAsked:
		return driver.PermissionNotAsked
	case driver.NotificationsGranted:
		return driver.PermissionGranted
	case driver.NotificationsDenied:
		return driver.PermissionDenied
	}
	return driver.PermissionUnavailable
}

func guidePermissionAllowed(st int) bool {
	return st == driver.PermissionGranted || st == driver.PermissionLimited
}

// pollGuideStatus reads name's status on the guide tracker's background goroutine.
func pollGuideStatus(name string) int {
	if name != driver.PermissionNotifications {
		return permissionStatus(name)
	}
	ch := make(chan int, 1)
	apple.NotificationPermission(false, func(np int) { ch <- notificationPermissionStatus(np) })
	select {
	case st := <-ch:
		return st
	case <-time.After(time.Second):
		return driver.PermissionUnknown
	}
}

// guidePermission opens System Settings at the pane of r's permission and follows it until the app has the permission, the user closes the panel or System Settings, or the guide is stopped; the engine hears the status then. Nothing to give answers at once.
func (s *surface) guidePermission(id uint32, r driver.PermissionRequest) {
	if r.Name == driver.PermissionNotifications {
		apple.NotificationPermission(false, func(np int) {
			apple.OnMain(func() { s.startGuide(id, r, notificationPermissionStatus(np)) })
		})
		return
	}
	s.startGuide(id, r, permissionStatus(r.Name))
}

func (s *surface) startGuide(id uint32, r driver.PermissionRequest, st int) {
	u := privacySettingsURL(r.Name)
	if guidePermissionAllowed(st) || st == driver.PermissionUnavailable || u == "" || !loadGuide() || !openSystemURL(u) {
		s.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: st})
		return
	}
	g := &permissionGuide{s: s, dispatch: s.engine.Dispatch, id: id, name: r.Name, app: r.App, label: r.AppName, lang: s.language,
		draggable: takesDrag(r.Name), stop: make(chan struct{})}
	if g.app == "" {
		g.app = guideDefaultApp()
	}
	if g.label == "" {
		g.label = appDisplayName(g.app)
	}
	s.adoptGuide(g)
	go g.track()
}

// adoptGuide makes g the guide running, ending the one before, and has its surface end it unanswered as it closes.
func (s *surface) adoptGuide(g *permissionGuide) {
	guideMu.Lock()
	old := currentGuide
	currentGuide = g
	live := liveGuides[s]
	first := live == nil
	if first {
		live = map[*permissionGuide]struct{}{}
		liveGuides[s] = live
	}
	live[g] = struct{}{}
	guideMu.Unlock()
	if old != nil {
		old.end()
	}
	// One stop for all of the surface's guides, which leaves nothing behind as each answers.
	if first {
		s.stops = append(s.stops, func() { endSurfaceGuides(s) })
	}
}

// endSurfaceGuides ends surface s's guides unanswered, as s closes.
func endSurfaceGuides(s *surface) {
	guideMu.Lock()
	live := liveGuides[s]
	delete(liveGuides, s)
	for g := range live {
		g.quiet.Store(true)
	}
	guideMu.Unlock()
	for g := range live {
		g.end()
	}
}

// stopGuide ends surface s's guide numbered id, which still answers.
func stopGuide(s *surface, id uint32) {
	guideMu.Lock()
	g := currentGuide
	guideMu.Unlock()
	if g != nil && g.s == s && g.id == id {
		g.end()
	}
}

func (g *permissionGuide) end() { g.stopOnce.Do(func() { close(g.stop) }) }

// withPool runs fn in an autorelease pool on one thread, for the tracker's goroutine.
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objcPoolPush()
	defer objcPoolPop(pool)
	fn()
}

// track follows System Settings until the guide ends, then answers once and closes the panel.
func (g *permissionGuide) track() {
	ticker := time.NewTicker(guideTick)
	defer ticker.Stop()
	var (
		started = time.Now()
		seen    bool // System Settings showed
		misses  int  // ticks in a row it did not
		last    guideRect
		front   bool
	)
	for tick := 0; ; tick++ {
		if tick%guideStatusTicks == 0 && guidePermissionAllowed(pollGuideStatus(g.name)) {
			break
		}
		pid, frontmost := settingsProcess()
		if pid != 0 {
			seen, misses = true, 0
		} else if seen {
			if misses++; misses >= guideMisses {
				break
			}
		} else if time.Since(started) > guideSettingsWait {
			break
		}
		if guideDragging.Load() && !cgEventSourceButtonState(kCGEventSourceStateCombined, kCGMouseButtonLeft) {
			apple.OnMain(g.endDrag)
		}
		if g.draggable && pid != 0 {
			// While a drag is on, System Settings is in front before the window server says so.
			show := frontmost || guideDragging.Load()
			if frame, ok := settingsFrame(pid); ok && (frame != last || show != front) {
				last, front = frame, show
				apple.OnMain(func() { g.place(frame, show) })
			}
		}
		select {
		case <-g.stop:
		case <-ticker.C:
			continue
		}
		break
	}
	g.finish(pollGuideStatus(g.name))
}

// finish ends the guide with status, answering its request unless its surface is gone, and has the main thread close its panel.
func (g *permissionGuide) finish(status int) {
	g.end()
	guideMu.Lock()
	if currentGuide == g {
		currentGuide = nil
	}
	delete(liveGuides[g.s], g)
	guideMu.Unlock()
	if !g.quiet.Load() {
		g.dispatch(driver.Event{Kind: driver.EventPermission, Count: g.id, Index: status})
	}
	apple.OnMain(g.destroy)
}

// settingsProcess is System Settings' process, 0 when it does not run, and whether it is the app in front.
func settingsProcess() (pid int32, frontmost bool) {
	withPool(func() {
		apps := objc.ID(apple.Class("NSRunningApplication")).Send(sel("runningApplicationsWithBundleIdentifier:"), apple.NSString(settingsBundleID))
		if apple.Msg(apps, sel("count")) == 0 {
			return
		}
		pid = int32(apple.Msg(apps.Send(sel("firstObject")), sel("processIdentifier")))
		front := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace")).Send(sel("frontmostApplication"))
		frontmost = front != 0 && int32(apple.Msg(front, sel("processIdentifier"))) == pid
	})
	return pid, frontmost
}

// settingsFrame is the bounds of System Settings' window in the window server's coordinates.
func settingsFrame(pid int32) (r guideRect, ok bool) {
	withPool(func() {
		list := cgWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly|kCGWindowListExcludeDesktopElements, 0)
		if list == 0 {
			return
		}
		defer cfRelease(list)
		kPID, kLayer, kAlpha, kBounds := apple.NSString("kCGWindowOwnerPID"), apple.NSString("kCGWindowLayer"), apple.NSString("kCGWindowAlpha"), apple.NSString("kCGWindowBounds")
		number := func(d, key objc.ID) (float64, bool) {
			v := objc.ID(apple.Msg(d, sel("objectForKey:"), uintptr(key)))
			if v == 0 {
				return 0, false
			}
			return msgSendFloat(v, sel("doubleValue")), true
		}
		var windows []settingsWindow
		for i := range apple.Msg(list, sel("count")) {
			w := objc.ID(apple.Msg(list, sel("objectAtIndex:"), i))
			if owner, ok := number(w, kPID); !ok || int32(owner) != pid {
				continue
			}
			b := objc.ID(apple.Msg(w, sel("objectForKey:"), uintptr(kBounds)))
			if b == 0 {
				continue
			}
			sw := settingsWindow{alpha: 1}
			if v, ok := number(w, kLayer); ok {
				sw.layer = int(v)
			}
			if v, ok := number(w, kAlpha); ok {
				sw.alpha = v
			}
			x, _ := number(b, apple.NSString("X"))
			y, _ := number(b, apple.NSString("Y"))
			width, _ := number(b, apple.NSString("Width"))
			height, _ := number(b, apple.NSString("Height"))
			sw.bounds = guideRect{x, y, width, height}
			windows = append(windows, sw)
		}
		r, ok = mainSettingsWindow(windows)
	})
	return r, ok
}

func cgRect(r guideRect) apple.CGRect { return apple.Rect(r.X, r.Y, r.W, r.H) }

func fromCG(r apple.CGRect) guideRect {
	return guideRect{r.Origin.X, r.Origin.Y, r.Size.Width, r.Size.Height}
}

// place shows the panel under System Settings' window at frame, in the window server's coordinates, or hides it. Main thread.
func (g *permissionGuide) place(frame guideRect, show bool) {
	select {
	case <-g.stop:
		return
	default:
	}
	if !show {
		if g.panel != 0 && g.shown {
			g.panel.Send(sel("orderOut:"), objc.ID(0))
			g.shown = false
		}
		return
	}
	screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
	n := objc.Send[uint](screens, sel("count"))
	if n == 0 {
		return
	}
	mainHeight := objc.Send[apple.CGRect](screens.Send(sel("objectAtIndex:"), uint(0)), sel("frame")).Size.Height
	settings := flipRect(frame, mainHeight)
	var visible []guideRect
	for i := range n {
		visible = append(visible, fromCG(objc.Send[apple.CGRect](screens.Send(sel("objectAtIndex:"), i), sel("visibleFrame"))))
	}
	screen := screenOf(settings, visible)
	w := guideWidth(settings, screen)
	if g.panel == 0 {
		g.build(w)
	}
	g.layout(w)
	target := guideFrame(settings, screen, guidePadding+g.labelH+8+guideCardHeight+guidePadding)
	g.panel.Send(sel("setFrame:display:"), cgRect(target), true)
	g.panel.Send(sel("invalidateShadow"))
	if !g.shown {
		g.panel.Send(sel("orderFrontRegardless"))
		g.shown = true
	}
	activeGuide = g
}

// paneTitle is the title of the permission's pane as System Settings on macOS major shows it, "" for one that takes no drag.
func paneTitle(msgs locales.Messages, name string, major int) string {
	switch name {
	case driver.PermissionAccessibility:
		if major >= 27 {
			return msgs.PermissionDeviceControl()
		}
		return msgs.PermissionAccessibility()
	case driver.PermissionScreenRecording:
		if major >= 15 {
			return msgs.PermissionScreenAudioRecording()
		}
		return msgs.PermissionScreenRecording()
	case driver.PermissionInputMonitoring:
		return msgs.PermissionInputMonitoring()
	case driver.PermissionFullDiskAccess:
		return msgs.PermissionFullDiskAccess()
	case driver.PermissionAppManagement:
		return msgs.PermissionAppManagement()
	case driver.PermissionDeveloperTools:
		return msgs.PermissionDeveloperTools()
	case driver.PermissionBluetooth:
		return msgs.PermissionBluetooth()
	case driver.PermissionMediaLibrary:
		return msgs.PermissionMediaLibrary()
	}
	return ""
}

// systemColor is NSColor's color called name, such as labelColor.
func systemColor(name string) objc.ID { return objc.ID(apple.Class("NSColor")).Send(sel(name)) }

// build makes the panel and its views. Main thread.
func (g *permissionGuide) build(w float64) {
	msgs := locales.In(g.lang)
	panel := objc.ID(apple.Class("NSPanel")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(0, 0, w, 100), uint(nsWindowStyleMaskNonactivatingPanel), uint(nsBackingStoreBuffered), false)
	panel.Send(sel("setLevel:"), nsFloatingWindowLevel)
	panel.Send(sel("setReleasedWhenClosed:"), false)
	panel.Send(sel("setOpaque:"), false)
	panel.Send(sel("setBackgroundColor:"), systemColor("clearColor"))
	panel.Send(sel("setHasShadow:"), true)
	panel.Send(sel("setHidesOnDeactivate:"), false)
	panel.Send(sel("setCollectionBehavior:"), uint(nsCollectionCanJoinAllSpaces|nsCollectionFullScreenAuxiliary))
	panel.Send(sel("setMovableByWindowBackground:"), false)

	effect := objc.ID(apple.Class("NSVisualEffectView")).Send(selAlloc).Send(sel("initWithFrame:"), apple.CGRect{})
	effect.Send(sel("setMaterial:"), nsVisualEffectMaterialPopover)
	effect.Send(sel("setBlendingMode:"), nsVisualEffectBlendingBehindWindow)
	effect.Send(sel("setState:"), nsVisualEffectStateActive)
	// A layer's corner radius does not clip what the view blurs from behind the window: a mask does, and the shadow follows it.
	mask := roundedMask(guideRadius)
	effect.Send(sel("setMaskImage:"), mask)
	mask.Send(selRelease)
	panel.Send(sel("setContentView:"), effect)
	effect.Send(selRelease)

	v := &g.views
	v.border = guideBox(systemColor("clearColor"), guideRadius)
	effect.Send(sel("addSubview:"), v.border)
	v.border.Send(selRelease)

	text := objc.ID(apple.Class("NSMutableAttributedString")).Send(selAlloc).Send(sel("init"))
	runs := splitRuns(msgs.GuideDrag(guideAppMarker, guidePermissionMarker),
		runMarker{guideAppMarker, g.label}, runMarker{guidePermissionMarker, paneTitle(msgs, g.name, osVersion().Major)})
	for _, r := range runs {
		font := objc.ID(apple.Class("NSFont")).Send(sel("systemFontOfSize:"), 13.0)
		if r.bold {
			font = objc.ID(apple.Class("NSFont")).Send(sel("boldSystemFontOfSize:"), 13.0)
		}
		attrs := objc.ID(apple.Class("NSMutableDictionary")).Send(sel("dictionary"))
		attrs.Send(sel("setObject:forKey:"), font, apple.NSString("NSFont"))
		attrs.Send(sel("setObject:forKey:"), systemColor("labelColor"), apple.NSString("NSColor"))
		part := objc.ID(apple.Class("NSAttributedString")).Send(selAlloc).Send(sel("initWithString:attributes:"), apple.NSString(r.text), attrs)
		text.Send(sel("appendAttributedString:"), part)
		part.Send(selRelease)
	}
	label := objc.ID(apple.Class("NSTextField")).Send(sel("wrappingLabelWithString:"), apple.NSString(""))
	label.Send(sel("setAttributedStringValue:"), text)
	text.Send(selRelease)
	effect.Send(sel("addSubview:"), label)
	v.label = label

	closeButton := objc.ID(apple.Class("NSButton")).Send(selAlloc).Send(sel("initWithFrame:"), apple.CGRect{})
	closeButton.Send(sel("setBordered:"), false)
	closeButton.Send(sel("setImagePosition:"), nsImageOnly)
	if img := guideSymbol("xmark.circle.fill", msgs.Close()); img != 0 {
		closeButton.Send(sel("setImage:"), img)
		closeButton.Send(sel("setContentTintColor:"), systemColor("secondaryLabelColor"))
	} else {
		closeButton.Send(sel("setTitle:"), apple.NSString(msgs.Close()))
	}
	// The target lives as long as the process: buttons do not retain theirs.
	closeButton.Send(sel("setTarget:"), objc.ID(apple.Class("NsgoPermissionGuideTarget")).Send(selAlloc).Send(sel("init")))
	closeButton.Send(sel("setAction:"), sel("close:"))
	effect.Send(sel("addSubview:"), closeButton)
	closeButton.Send(selRelease)
	v.close = closeButton

	// The card: a box, the app's icon and name, the hint, and over them the view the drag starts from, which gets every click on the card.
	v.cardBox = guideBox(systemColor("controlBackgroundColor").Send(sel("colorWithAlphaComponent:"), 0.65), 10)
	effect.Send(sel("addSubview:"), v.cardBox)
	v.cardBox.Send(selRelease)

	icon := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace")).Send(sel("iconForFile:"), apple.NSString(g.app))
	v.icon = objc.ID(apple.Class("NSImageView")).Send(sel("imageViewWithImage:"), icon)
	effect.Send(sel("addSubview:"), v.icon)

	v.name = objc.ID(apple.Class("NSTextField")).Send(sel("labelWithString:"), apple.NSString(g.label))
	v.name.Send(sel("setFont:"), objc.ID(apple.Class("NSFont")).Send(sel("systemFontOfSize:"), 15.0))
	effect.Send(sel("addSubview:"), v.name)

	v.hint = objc.ID(apple.Class("NSTextField")).Send(sel("labelWithString:"), apple.NSString(msgs.GuideDragHint()))
	v.hint.Send(sel("setFont:"), objc.ID(apple.Class("NSFont")).Send(sel("systemFontOfSize:"), 9.0))
	v.hint.Send(sel("setTextColor:"), systemColor("secondaryLabelColor"))
	v.hint.Send(sel("setAlignment:"), nsAlignCenter)
	effect.Send(sel("addSubview:"), v.hint)

	if img := guideSymbol("hand.draw", msgs.GuideDragHint()); img != 0 {
		v.hand = objc.ID(apple.Class("NSImageView")).Send(sel("imageViewWithImage:"), img)
		v.hand.Send(sel("setContentTintColor:"), systemColor("secondaryLabelColor"))
		effect.Send(sel("addSubview:"), v.hand)
	}

	v.card = objc.ID(apple.Class("NsgoPermissionDragView")).Send(selAlloc).Send(sel("initWithFrame:"), apple.CGRect{})
	effect.Send(sel("addSubview:"), v.card)
	v.card.Send(selRelease)
	g.panel = panel
}

// roundedMask is an image of a rounded rectangle that stretches to any size keeping its corners, for NSVisualEffectView's maskImage.
func roundedMask(radius float64) objc.ID {
	side := 2*radius + 1
	img := objc.ID(apple.Class("NSImage")).Send(selAlloc).Send(sel("initWithSize:"), apple.CGSize{Width: side, Height: side})
	img.Send(sel("lockFocus"))
	systemColor("blackColor").Send(sel("set"))
	objc.ID(apple.Class("NSBezierPath")).Send(sel("bezierPathWithRoundedRect:xRadius:yRadius:"), apple.Rect(0, 0, side, side), radius, radius).Send(sel("fill"))
	img.Send(sel("unlockFocus"))
	type edgeInsets struct{ Top, Left, Bottom, Right float64 }
	img.Send(sel("setCapInsets:"), edgeInsets{radius, radius, radius, radius})
	img.Send(sel("setResizingMode:"), nsImageResizingModeStretch)
	return img
}

// guideBox is a rounded box filled with fill and bordered by the separator's color.
func guideBox(fill objc.ID, radius float64) objc.ID {
	box := objc.ID(apple.Class("NSBox")).Send(selAlloc).Send(sel("initWithFrame:"), apple.CGRect{})
	box.Send(sel("setBoxType:"), nsBoxCustom)
	box.Send(sel("setTitlePosition:"), nsNoTitle)
	box.Send(sel("setBorderWidth:"), 1.0)
	box.Send(sel("setCornerRadius:"), radius)
	box.Send(sel("setBorderColor:"), systemColor("separatorColor"))
	box.Send(sel("setFillColor:"), fill)
	return box
}

// guideSymbol is the SF Symbol called name, described for VoiceOver, or 0.
func guideSymbol(name, description string) objc.ID {
	return objc.ID(apple.Class("NSImage")).Send(sel("imageWithSystemSymbolName:accessibilityDescription:"), apple.NSString(name), apple.NSString(description))
}

// layout lays the panel out at width w, measuring its text. Main thread.
func (g *permissionGuide) layout(w float64) {
	v := &g.views
	textW := w - 2*guidePadding - guideCloseSize - 6
	v.label.Send(sel("setPreferredMaxLayoutWidth:"), textW)
	g.labelH = math.Ceil(objc.Send[apple.CGSize](v.label, sel("fittingSize")).Height)
	h := guidePadding + g.labelH + 8 + guideCardHeight + guidePadding
	set := func(view objc.ID, r guideRect) {
		if view != 0 {
			view.Send(sel("setFrame:"), cgRect(r))
		}
	}
	card := guideRect{guidePadding, guidePadding, w - 2*guidePadding, guideCardHeight}
	set(v.border, guideRect{0, 0, w, h})
	set(v.label, guideRect{guidePadding, h - guidePadding - g.labelH, textW, g.labelH})
	set(v.close, guideRect{w - guidePadding - guideCloseSize, h - guidePadding - guideCloseSize + 2, guideCloseSize, guideCloseSize})
	set(v.cardBox, card)
	set(v.icon, guideRect{card.X + 8, card.Y + (guideCardHeight-guideIconSize)/2, guideIconSize, guideIconSize})
	set(v.name, guideRect{card.X + 8 + guideIconSize + 8, card.Y + (guideCardHeight-20)/2, card.W - guideIconSize - 70, 20})
	set(v.hand, guideRect{card.maxX() - 38, card.Y + 20, 24, 18})
	set(v.hint, guideRect{card.maxX() - 44, card.Y + 6, 36, 12})
	set(v.card, card)
}

// destroy closes the panel. Main thread.
func (g *permissionGuide) destroy() {
	if activeGuide == g {
		activeGuide = nil
		guideDragging.Store(false)
	}
	if g.panel == 0 {
		return
	}
	g.panel.Send(sel("orderOut:"), objc.ID(0))
	g.panel.Send(sel("close"))
	g.panel.Send(selRelease)
	g.panel, g.views = 0, guideViews{}
}

// endDrag gives the panel the mouse back after a drag. Main thread.
func (g *permissionGuide) endDrag() {
	guideDragging.Store(false)
	g.dragBegan = false
	if g.panel != 0 {
		g.panel.Send(sel("setIgnoresMouseEvents:"), false)
		g.panel.Send(sel("setAlphaValue:"), 1.0)
	}
}

// The pasteboard types System Settings takes an app dragged from Finder in.
var guideAppTypes = []string{"public.file-url", "public.url", "NSFilenamesPboardType", "com.apple.pasteboard.promised-file-url", "public.utf8-plain-text"}

func guideWriterTypes(objc.ID, objc.SEL, objc.ID) objc.ID {
	types := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	for _, t := range guideAppTypes {
		types.Send(sel("addObject:"), apple.NSString(t))
	}
	return types
}

func guideWriterPropertyList(_ objc.ID, _ objc.SEL, typ objc.ID) objc.ID {
	g := activeGuide
	if g == nil {
		return 0
	}
	switch apple.GoString(typ) {
	case "public.file-url", "public.url", "com.apple.pasteboard.promised-file-url":
		return objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(g.app)).Send(sel("absoluteString"))
	case "NSFilenamesPboardType":
		return objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), apple.NSString(g.app))
	case "public.utf8-plain-text":
		return apple.NSString(g.app)
	}
	return 0
}

func guideCardDown(_ objc.ID, _ objc.SEL, event objc.ID) {
	if g := activeGuide; g != nil {
		g.dragOrigin = objc.Send[apple.CGPoint](event, sel("locationInWindow"))
		g.dragBegan = false
	}
}

// guideCardDragged starts dragging the app once the pointer has moved 4 points, letting the mouse through the panel to System Settings' list under it.
func guideCardDragged(self objc.ID, _ objc.SEL, event objc.ID) {
	g := activeGuide
	if g == nil || g.dragBegan {
		return
	}
	at := objc.Send[apple.CGPoint](event, sel("locationInWindow"))
	if math.Hypot(at.X-g.dragOrigin.X, at.Y-g.dragOrigin.Y) <= 4 {
		return
	}
	g.dragBegan = true
	writer := objc.ID(apple.Class("NsgoPermissionAppWriter")).Send(selAlloc).Send(sel("init"))
	item := objc.ID(apple.Class("NSDraggingItem")).Send(selAlloc).Send(sel("initWithPasteboardWriter:"), writer)
	icon := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace")).Send(sel("iconForFile:"), apple.NSString(g.app))
	icon.Send(sel("setSize:"), apple.CGSize{Width: guideDragIconSize, Height: guideDragIconSize})
	p := objc.Send[apple.CGPoint](self, sel("convertPoint:fromView:"), at, objc.ID(0))
	item.Send(sel("setDraggingFrame:contents:"), apple.Rect(p.X-guideDragIconSize/2, p.Y-guideDragIconSize/2, guideDragIconSize, guideDragIconSize), icon)
	session := self.Send(sel("beginDraggingSessionWithItems:event:source:"), objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), item), event, self)
	session.Send(sel("setAnimatesToStartingPositionsOnCancelOrFail:"), true)
	session.Send(sel("setDraggingFormation:"), nsDraggingFormationNone)
	item.Send(selRelease)
	writer.Send(selRelease)
	guideDragging.Store(true)
	g.panel.Send(sel("setIgnoresMouseEvents:"), true)
	g.panel.Send(sel("setAlphaValue:"), 0.72)
}
