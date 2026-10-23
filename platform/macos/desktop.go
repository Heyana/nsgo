//go:build darwin && !ios

package macos

import (
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/apple"
)

// The window's chrome, as NSH's macOS chrome.rs does it. A hidden title
// bar gives the window a full-size content view under a transparent,
// untitled title bar; a custom height above 0 selects a unified toolbar,
// compact up to 44 points, so AppKit centers the traffic lights in it.
// Env.TitleBar reports the bar's height and the traffic lights' width,
// nothing in full screen. A left press on a window drag handle moves the
// window with AppKit's own drag, and a double click does what the user
// chose in System Settings. The application menu is an NSMenu bar: role
// items send AppKit's standard actions down the responder chain, the
// others report their ID with EventMenu.

// chrome is a surface's window state.
type chrome struct {
	titleBar    core.TitleBarStyle
	titleHeight core.Points
	toolbar     objc.ID // the toolbar a custom title bar height installed

	appName     string
	defaultMenu objc.ID // the menu bar the backend builds
	menu        objc.ID // the app's menu bar
	menuTarget  objc.ID
}

// The title bar's height and the traffic lights' width when AppKit
// cannot tell.
const titleBarHeight, trafficLightsWidth = 28, 78

const (
	maskFullScreen   = 1 << 14
	maskFullSize     = 1 << 15
	toolbarUnified   = 3
	toolbarCompact   = 4
	zoomButton       = 2
	titleHidden      = 1
	titleVisible     = 0
	toolbarAutomatic = 0
)

// rawHit makes the views' hitTest: return the deepest view, for the
// window drag's decision.
var rawHit bool

var selMenuChosen = sel("nsgoMenuChosen:")

func (s *surface) command(c *core.WindowCommand) {
	w := s.window
	switch c.Action {
	case core.WindowSetTitleBar:
		s.titleBar, s.titleHeight = c.TitleBar, c.Height
		s.applyTitleBar()
	case core.WindowMinimize:
		w.Send(sel("miniaturize:"), objc.ID(0))
	case core.WindowToggleMaximize:
		w.Send(sel("zoom:"), objc.ID(0))
	case core.WindowToggleFullScreen:
		w.Send(sel("toggleFullScreen:"), objc.ID(0))
	case core.WindowClose:
		w.Send(sel("performClose:"), objc.ID(0))
	}
}

func (s *surface) applyTitleBar() {
	w := s.window
	hidden := s.titleBar == core.TitleBarHidden
	mask := objc.Send[uint](w, sel("styleMask"))
	if hidden {
		mask |= maskFullSize
	} else {
		mask &^= maskFullSize
	}
	w.Send(sel("setStyleMask:"), mask)
	if hidden && s.titleHeight > 0 {
		style := toolbarUnified
		if s.titleHeight <= 44 {
			style = toolbarCompact
		}
		if s.toolbar == 0 && w.Send(sel("toolbar")) == 0 {
			s.toolbar = objc.ID(objc.GetClass("NSToolbar")).Send(selAlloc).Send(sel("initWithIdentifier:"),
				apple.NSString("nsgo.title-bar"))
			w.Send(sel("setToolbar:"), s.toolbar)
		}
		if s.toolbar != 0 {
			w.Send(sel("setToolbarStyle:"), style)
		}
	} else if s.toolbar != 0 {
		if w.Send(sel("toolbar")) == s.toolbar {
			w.Send(sel("setToolbar:"), objc.ID(0))
			w.Send(sel("setToolbarStyle:"), toolbarAutomatic)
		}
		s.toolbar.Send(selRelease)
		s.toolbar = 0
	}
	w.Send(sel("setTitlebarAppearsTransparent:"), hidden)
	visibility := titleVisible
	if hidden {
		visibility = titleHidden
	}
	w.Send(sel("setTitleVisibility:"), visibility)
	s.refresh()
	// AppKit can restore the old content size after setStyleMask:, and
	// the toolbar's layout settles later.
	apple.OnMain(func() {
		content := w.Send(sel("contentView"))
		size := objc.Send[apple.CGRect](w, sel("contentRectForFrameRect:"), objc.Send[apple.CGRect](w, sel("frame"))).Size
		if objc.Send[apple.CGRect](content, sel("frame")).Size != size {
			content.Send(selSetFrameSize, size)
		}
		s.refresh()
	})
}

// titleBarArea is what a hidden title bar covers: its height on top and
// the traffic lights on the left.
func (s *surface) titleBarArea() core.Edges {
	w := s.window
	if w == 0 || s.titleBar != core.TitleBarHidden {
		return core.Edges{}
	}
	mask := objc.Send[uint](w, sel("styleMask"))
	if mask&maskFullScreen != 0 {
		return core.Edges{}
	}
	frame := objc.Send[apple.CGRect](w, sel("frame"))
	content := objc.Send[apple.CGRect](objc.ID(objc.GetClass("NSWindow")), sel("contentRectForFrameRect:styleMask:"),
		frame, mask&^maskFullSize)
	height := frame.Size.Height - content.Size.Height
	if w.Send(sel("toolbar")) != 0 {
		// A toolbar's height is AppKit's, not the one that chose its style.
		layout := objc.Send[apple.CGRect](w, sel("contentLayoutRect"))
		if h := frame.Size.Height - (layout.Origin.Y + layout.Size.Height); h > 0 {
			height = h
		}
	}
	if height <= 0 {
		height = titleBarHeight
	}
	left := float64(trafficLightsWidth)
	if zoom := w.Send(sel("standardWindowButton:"), zoomButton); zoom != 0 {
		f := objc.Send[apple.CGRect](zoom, sel("frame"))
		if r := f.Origin.X + f.Size.Width + 8; r > 8 {
			left = r
		}
	}
	return core.Edges{Top: core.Points(height), Left: core.Points(left)}
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
		case core.WindowDragOn:
			return true
		case core.WindowDragOff:
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
	return v.kind == core.KindInput || v.kind == core.KindScroll || v.kind == core.KindWeb || v.pressable() ||
		v.events.Has(core.EventPan) || v.events.Has(core.EventPinch) || v.events.Has(core.EventRotation) ||
		v.events.Has(core.EventContextMenu)
}

// setMenu makes entries the menu bar, or restores the default one.
func (s *surface) setMenu(entries []core.MenuEntry) {
	nsApp := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	if s.menu != 0 {
		defer s.menu.Send(selRelease)
		s.menu = 0
	}
	if entries == nil {
		nsApp.Send(sel("setMainMenu:"), s.defaultMenu)
		return
	}
	if s.menuTarget == 0 {
		s.menuTarget = objc.ID(classMenuTarget).Send(selNew)
	}
	bar := objc.ID(objc.GetClass("NSMenu")).Send(selNew)
	for _, e := range entries {
		if e.Separator {
			continue
		}
		items := e.Items
		if len(items) == 0 {
			// A top-level action gets a menu of its own.
			items = []core.MenuEntry{e}
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
func (s *surface) holder(label string, disabled bool, items []core.MenuEntry) objc.ID {
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
func (s *surface) submenu(title string, items []core.MenuEntry) objc.ID {
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
var roleActions = map[core.MenuRole]string{
	core.MenuAbout: "orderFrontStandardAboutPanel:", core.MenuHide: "hide:",
	core.MenuHideOthers: "hideOtherApplications:", core.MenuShowAll: "unhideAllApplications:",
	core.MenuQuit: "terminate:", core.MenuUndo: "undo:", core.MenuRedo: "redo:", core.MenuCut: "cut:",
	core.MenuCopy: "copy:", core.MenuPaste: "paste:", core.MenuSelectAll: "selectAll:",
	core.MenuMinimize: "performMiniaturize:", core.MenuZoom: "performZoom:",
	core.MenuToggleFullScreen: "toggleFullScreen:", core.MenuClose: "performClose:",
}

// item returns a new menu item of e.
func (s *surface) item(e core.MenuEntry) objc.ID {
	title := e.Label
	switch e.Role {
	case core.MenuAbout, core.MenuHide, core.MenuQuit:
		// macOS names the app in these.
		if title == e.Role.Label() && s.name() != "" {
			title += " " + s.name()
		}
	case core.MenuToggleFullScreen:
		if title == e.Role.Label() {
			title = "Enter Full Screen" // AppKit retitles it as the window changes
		}
	}
	shortcut := e.Shortcut
	if shortcut == "" && e.Role != core.MenuRoleNone {
		shortcut = core.NormalizeKey(e.Role.Shortcut(true))
	}
	key, mods := keyEquivalent(shortcut)
	var action objc.SEL
	switch {
	case e.Disabled:
		action = selMenuDisabled
	case e.Role != core.MenuRoleNone:
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

// menuChosen reports a chosen menu item's ID.
func menuChosen(self objc.ID, _ objc.SEL, item objc.ID) {
	if id := objc.Send[int](item, sel("tag")); id > 0 && active != nil && active.engine != nil {
		active.engine.Dispatch(core.Event{Kind: core.EventMenu, Count: uint32(id)})
	}
}
