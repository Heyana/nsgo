//go:build darwin && !ios

package macos

import (
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// The window's chrome, as NSH's macOS chrome.rs does it. A hidden title
// bar gives the window a full-size content view under a transparent,
// untitled title bar, with the traffic lights centered in the app's bar,
// where Onevra puts them: 16 points in from the left edge. Env.TitleBar
// reports the bar's height and the traffic lights' width, nothing in
// full screen. A left press on a window drag handle moves the window
// with AppKit's own drag, and a double click does what the user chose in
// System Settings. The application menu is an NSMenu bar: role items
// send AppKit's standard actions down the responder chain, the others
// report their ID with EventMenu.

// chrome is a surface's window state.
type chrome struct {
	titleBar    driver.TitleBarStyle
	titleHeight driver.Points

	appName     string
	defaultMenu objc.ID // the menu bar the backend builds
	menu        objc.ID // the app's menu bar
	menuTarget  objc.ID
	popup       objc.ID // the context menu shown last
	tracking    bool    // whether a context menu tracks
	nextPopups  []func()
}

// The title bar's height and the traffic lights' width when AppKit
// cannot tell, and where the traffic lights start in the app's bar.
const titleBarHeight, trafficLightsWidth = 28, 78
const trafficLightsLeft = 16

const (
	maskFullScreen = 1 << 14
	maskFullSize   = 1 << 15
	zoomButton     = 2
	titleHidden    = 1
	titleVisible   = 0
)

var (
	selStyleMask           = sel("styleMask")
	selStandardButton      = sel("standardWindowButton:")
	selSetFrameOrigin      = sel("setFrameOrigin:")
	selContentRectForFrame = sel("contentRectForFrameRect:styleMask:")
)

// rawHit makes the views' hitTest: return the deepest view, for the
// window drag's decision.
var rawHit bool

var selMenuChosen = sel("nsgoMenuChosen:")

// chromeItem applies an item of the window's own: its title, a window
// command or the menu bar.
func (s *surface) chromeItem(it *driver.MountItem) {
	switch it.Kind {
	case driver.ItemTitle:
		s.window.Send(sel("setTitle:"), apple.NSString(it.Value))
		s.layoutTrafficLights()
	case driver.ItemWindow:
		s.command(it.Window)
	case driver.ItemMenu:
		s.setMenu(it.Menu)
	}
}

func (s *surface) command(c *driver.WindowCommand) {
	w := s.window
	switch c.Action {
	case driver.WindowSetTitleBar:
		s.titleBar, s.titleHeight = c.TitleBar, c.Height
		s.applyTitleBar()
	case driver.WindowMinimize:
		w.Send(sel("miniaturize:"), objc.ID(0))
	case driver.WindowToggleMaximize:
		w.Send(sel("zoom:"), objc.ID(0))
	case driver.WindowToggleFullScreen:
		w.Send(sel("toggleFullScreen:"), objc.ID(0))
	case driver.WindowClose:
		w.Send(sel("performClose:"), objc.ID(0))
	case driver.WindowSetFrame:
		s.setWindowFrame(c.Frame)
	case driver.WindowSetLevel:
		s.setLevel(c.Level)
	case driver.WindowOrderFront:
		s.orderFront()
	}
}

func (s *surface) applyTitleBar() {
	w := s.window
	hidden := s.titleBar == driver.TitleBarHidden
	mask := objc.Send[uint](w, sel("styleMask"))
	if hidden {
		mask |= maskFullSize
	} else {
		mask &^= maskFullSize
	}
	w.Send(sel("setStyleMask:"), mask)
	w.Send(sel("setTitlebarAppearsTransparent:"), hidden)
	visibility := titleVisible
	if hidden {
		visibility = titleHidden
	}
	w.Send(sel("setTitleVisibility:"), visibility)
	s.layoutTrafficLights()
	s.refresh()
	// AppKit can restore the old content size after setStyleMask:.
	apple.OnMain(func() {
		content := w.Send(sel("contentView"))
		size := objc.Send[apple.CGRect](w, sel("contentRectForFrameRect:"), objc.Send[apple.CGRect](w, sel("frame"))).Size
		if objc.Send[apple.CGRect](content, sel("frame")).Size != size {
			content.Send(selSetFrameSize, size)
		}
		s.layoutTrafficLights()
		s.refresh()
	})
}

// layoutTrafficLights centers the traffic lights in the app's bar, 16
// points in from the window's left edge as Onevra places them. AppKit
// lays the title bar out again when the window resizes, its title or
// appearance changes, or it leaves full screen, so this runs after each.
func (s *surface) layoutTrafficLights() {
	w := s.window
	// An embedding host's toolbar lays the title bar out itself.
	if w == 0 || s.titleBar != driver.TitleBarHidden || s.titleHeight <= 0 || w.Send(sel("toolbar")) != 0 {
		return
	}
	if objc.Send[uint](w, selStyleMask)&maskFullScreen != 0 {
		return
	}
	close := w.Send(selStandardButton, 0)
	if close == 0 {
		return
	}
	container := close.Send(selSuperview).Send(selSuperview)
	if container == 0 {
		return
	}
	button := objc.Send[apple.CGRect](close, sel("frame"))
	height := float64(s.titleHeight)
	top := max((height-button.Size.Height)/2, 0)
	frame := objc.Send[apple.CGRect](container, sel("frame"))
	frame.Size.Height = height
	frame.Origin.Y = objc.Send[apple.CGRect](w, sel("frame")).Size.Height - height
	container.Send(selSetFrame, frame)
	mini := w.Send(selStandardButton, 1)
	gap := objc.Send[apple.CGRect](mini, sel("frame")).Origin.X - button.Origin.X
	for i := 0; i < 3; i++ {
		b := w.Send(selStandardButton, i)
		if b != 0 {
			b.Send(selSetFrameOrigin, apple.CGPoint{X: trafficLightsLeft + float64(i)*gap, Y: top})
		}
	}
}

// titleBarArea is what a hidden title bar covers: its height on top and
// the traffic lights on the left.
func (s *surface) titleBarArea() driver.Edges {
	w := s.window
	if w == 0 || s.titleBar != driver.TitleBarHidden {
		return driver.Edges{}
	}
	mask := objc.Send[uint](w, selStyleMask)
	if mask&maskFullScreen != 0 {
		return driver.Edges{}
	}
	height := float64(s.titleHeight)
	if height <= 0 || w.Send(sel("toolbar")) != 0 {
		// Measure the default bar, or the toolbar an embedding host put
		// there, as AppKit laid it out.
		frame := objc.Send[apple.CGRect](w, sel("frame"))
		content := objc.Send[apple.CGRect](objc.ID(objc.GetClass("NSWindow")), selContentRectForFrame,
			frame, mask&^maskFullSize)
		height = frame.Size.Height - content.Size.Height
		if w.Send(sel("toolbar")) != 0 {
			layout := objc.Send[apple.CGRect](w, sel("contentLayoutRect"))
			if h := frame.Size.Height - (layout.Origin.Y + layout.Size.Height); h > 0 {
				height = h
			}
		}
	}
	if height <= 0 {
		height = titleBarHeight
	}
	left := float64(trafficLightsWidth)
	if zoom := w.Send(selStandardButton, zoomButton); zoom != 0 {
		f := objc.Send[apple.CGRect](zoom, sel("frame"))
		if r := f.Origin.X + f.Size.Width + 8; r > 8 {
			left = r
		}
	}
	return driver.Edges{Top: driver.Points(height), Left: driver.Points(left)}
}

// takePress moves the window from a drag handle, or starts or extends a
// selection, at a left press the view has been given; it reports whether
// it took the press from the view. AppKit has already activated the
// window, so the press reaches the views it hit.
func (s *surface) takePress(event objc.ID) bool {
	return s.dragWindow(event) || s.selectPress(event)
}

// dragWindow moves the window, or zooms it on a double click, when a
// left mouse down lands on a window drag handle; it reports whether it
// took the press. The nearest view from the one hit up that sets
// WindowDrag decides; an input, a scroll view or a view with press or
// gesture handlers nearer than it keeps the press.
func (s *surface) dragWindow(event objc.ID) bool {
	loc := objc.Send[apple.CGPoint](event, selLocationInWindow)
	up := s.native.Send(selSuperview)
	if up == 0 {
		return false
	}
	p := objc.Send[apple.CGPoint](up, selConvertPoint, loc, objc.ID(0))
	rawHit = true
	hit := s.native.Send(sel("hitTest:"), p)
	rawHit = false
	if !dragsWindow(hit, up) {
		return false
	}
	if objc.Send[int](event, sel("clickCount")) == 2 {
		defaults := objc.ID(objc.GetClass("NSUserDefaults")).Send(sel("standardUserDefaults"))
		switch apple.GoString(defaults.Send(sel("stringForKey:"), apple.NSString("AppleActionOnDoubleClick"))) {
		case "Minimize":
			s.window.Send(sel("miniaturize:"), objc.ID(0))
		case "None":
		default:
			s.window.Send(sel("zoom:"), objc.ID(0))
		}
		return true
	}
	s.window.Send(sel("performWindowDragWithEvent:"), event)
	return true
}

func dragsWindow(hit, stop objc.ID) bool {
	for n := hit; n != 0 && n != stop; n = n.Send(selSuperview) {
		v := views[n]
		if v == nil {
			continue
		}
		if v.native != n {
			return false // an input's field or a scroll view's clip view
		}
		switch v.windowDrag {
		case driver.WindowDragOn:
			return true
		case driver.WindowDragOff:
			return false
		}
		if v.takesPointer() {
			return false
		}
	}
	return false
}

// takesPointer reports whether presses on v are the app's.
func (v *view) takesPointer() bool {
	return v.kind == driver.KindInput || v.kind == driver.KindScroll || v.kind == driver.KindWeb || v.pressable() ||
		v.events.Has(driver.EventPan) || v.events.Has(driver.EventPinch) || v.events.Has(driver.EventRotation) ||
		v.events.Has(driver.EventContextMenu)
}

// setMenu makes entries the menu bar, or restores the default one.
func (s *surface) setMenu(entries []driver.MenuEntry) {
	nsApp := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	if s.menu != 0 {
		defer s.menu.Send(selRelease)
		s.menu = 0
	}
	if entries == nil {
		nsApp.Send(sel("setMainMenu:"), s.defaultMenu)
		return
	}
	s.ensureMenuTarget()
	bar := objc.ID(objc.GetClass("NSMenu")).Send(selNew)
	for _, e := range entries {
		if e.Separator {
			continue
		}
		items := e.Items
		if len(items) == 0 {
			// A top-level action gets a menu of its own.
			items = []driver.MenuEntry{e}
		}
		holder := s.holder(e.Label, e.Disabled, items)
		bar.Send(sel("addItem:"), holder)
		holder.Send(selRelease)
	}
	nsApp.Send(sel("setMainMenu:"), bar)
	s.menu = bar
}

// The action of disabled items: nothing answers it, so AppKit disables
// them as it validates the menu.
var selMenuDisabled = sel("nsgoMenuItemDisabled:")

// holder returns a new item holding a submenu of items.
func (s *surface) holder(label string, disabled bool, items []driver.MenuEntry) objc.ID {
	var action objc.SEL
	if disabled {
		action = selMenuDisabled
	}
	it := objc.ID(objc.GetClass("NSMenuItem")).Send(selAlloc).Send(sel("initWithTitle:action:keyEquivalent:"),
		apple.NSString(label), action, apple.NSString(""))
	sub := s.submenu(label, items)
	it.Send(sel("setSubmenu:"), sub)
	sub.Send(selRelease)
	return it
}

// submenu returns a new menu of items.
func (s *surface) submenu(title string, items []driver.MenuEntry) objc.ID {
	m := objc.ID(objc.GetClass("NSMenu")).Send(selAlloc).Send(sel("initWithTitle:"), apple.NSString(title))
	lastSeparator := true
	for _, e := range items {
		switch {
		case e.Separator:
			if !lastSeparator {
				m.Send(sel("addItem:"), objc.ID(objc.GetClass("NSMenuItem")).Send(sel("separatorItem")))
				lastSeparator = true
			}
			continue
		case len(e.Items) > 0:
			holder := s.holder(e.Label, e.Disabled, e.Items)
			m.Send(sel("addItem:"), holder)
			holder.Send(selRelease)
		default:
			it := s.item(e)
			m.Send(sel("addItem:"), it)
			it.Send(selRelease)
		}
		lastSeparator = false
	}
	if n := objc.Send[int](m, sel("numberOfItems")); n > 0 && lastSeparator {
		m.Send(sel("removeItemAtIndex:"), n-1)
	}
	return m
}

// The standard actions of the menu roles, sent down the responder chain.
var roleActions = map[driver.MenuRole]string{
	driver.MenuAbout: "orderFrontStandardAboutPanel:", driver.MenuHide: "hide:",
	driver.MenuHideOthers: "hideOtherApplications:", driver.MenuShowAll: "unhideAllApplications:",
	driver.MenuQuit: "terminate:", driver.MenuUndo: "undo:", driver.MenuRedo: "redo:", driver.MenuCut: "cut:",
	driver.MenuCopy: "copy:", driver.MenuPaste: "paste:", driver.MenuSelectAll: "selectAll:",
	driver.MenuMinimize: "performMiniaturize:", driver.MenuZoom: "performZoom:",
	driver.MenuToggleFullScreen: "toggleFullScreen:", driver.MenuClose: "performClose:",
}

// item returns a new menu item of e.
func (s *surface) item(e driver.MenuEntry) objc.ID {
	title := e.Label
	switch e.Role {
	case driver.MenuAbout, driver.MenuHide, driver.MenuQuit:
		// macOS names the app in these.
		if title == e.Role.Label() && s.name() != "" {
			title += " " + s.name()
		}
	case driver.MenuToggleFullScreen:
		if title == e.Role.Label() {
			title = "Enter Full Screen" // AppKit retitles it as the window changes
		}
	}
	shortcut := e.Shortcut
	if shortcut == "" && e.Role != driver.MenuRoleNone {
		shortcut = driver.NormalizeKey(e.Role.Shortcut(true))
	}
	key, mods := keyEquivalent(shortcut)
	var action objc.SEL
	switch {
	case e.Disabled:
		action = selMenuDisabled
	case e.Role != driver.MenuRoleNone:
		action = sel(roleActions[e.Role])
	case e.ID != 0:
		action = selMenuChosen
	}
	it := objc.ID(objc.GetClass("NSMenuItem")).Send(selAlloc).Send(sel("initWithTitle:action:keyEquivalent:"),
		apple.NSString(title), action, apple.NSString(key))
	if key != "" {
		it.Send(sel("setKeyEquivalentModifierMask:"), mods)
	}
	if action == selMenuChosen {
		it.Send(selSetTarget, s.menuTarget)
		it.Send(sel("setTag:"), int(e.ID))
	}
	if e.Checkable {
		state := 0
		if e.Checked {
			state = 1
		}
		it.Send(sel("setState:"), state)
	}
	return it
}

// name is the app's name, for the app menu's items.
func (s *surface) name() string {
	if s.appName == "" {
		s.appName = apple.GoString(objc.ID(objc.GetClass("NSRunningApplication")).
			Send(sel("currentApplication")).Send(sel("localizedName")))
	}
	return s.appName
}

// AppKit's function key characters of the keys core.NormalizeKey names.
var keyEquivalents = map[string]rune{
	"enter": '\r', "escape": 0x1b, "tab": '\t', "space": ' ', "backspace": 0x08, "delete": 0xF728,
	"up": 0xF700, "down": 0xF701, "left": 0xF702, "right": 0xF703, "insert": 0xF727,
	"home": 0xF729, "end": 0xF72B, "pageup": 0xF72C, "pagedown": 0xF72D,
	"plus": '=', "minus": '-',
}

// keyEquivalent returns a normalized shortcut's key equivalent and its
// modifier mask; mod is Command.
func keyEquivalent(shortcut string) (string, uint) {
	if shortcut == "" {
		return "", 0
	}
	parts := strings.Split(shortcut, "+")
	name := parts[len(parts)-1]
	if name == "" && len(parts) > 1 {
		name = "+" // "mod++"
	}
	const shift, control, option, command = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	var mods uint
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "mod", "meta":
			mods |= command
		case "ctrl":
			mods |= control
		case "alt":
			mods |= option
		case "shift":
			mods |= shift
		}
	}
	key := name
	if r, ok := keyEquivalents[name]; ok {
		key = string(r)
	} else if len(name) > 1 && name[0] == 'f' {
		n := 0
		for _, c := range name[1:] {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n >= 1 && n <= 35 {
			key = string(rune(0xF704 + n - 1))
		}
	}
	return key, mods
}

// menuTargets are the surfaces of the menu items' targets.
var menuTargets = map[objc.ID]*surface{}

// ensureMenuTarget makes the target of the surface's menu items.
func (s *surface) ensureMenuTarget() {
	if s.menuTarget == 0 {
		s.menuTarget = objc.ID(classMenuTarget).Send(selNew)
		menuTargets[s.menuTarget] = s
	}
}

// menuChosen reports a chosen menu item's ID to its surface.
func menuChosen(self objc.ID, _ objc.SEL, item objc.ID) {
	s := menuTargets[self]
	if id := objc.Send[int](item, sel("tag")); id > 0 && s != nil && s.engine != nil && !s.closed {
		s.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: uint32(id)})
	}
}
