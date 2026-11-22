//go:build darwin && !ios

package macos

import (
	"math"
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/menutext"
)

// The window's chrome, as NSH's macOS chrome.rs does it. A hidden title
// bar gives the window a full-size content view under a transparent,
// untitled title bar, with the traffic lights centered in the app's bar,
// where NSGo puts them: 16 points in from the left edge, or where the
// app's WindowControls put them, as MyGo's TrafficLightPosition does.
// Env.TitleBar reports the bar's height and the traffic lights' width,
// nothing in full screen. A left press on a window drag handle moves the
// window with AppKit's own drag, and a double click does what the user
// chose in System Settings. The application menu is an NSMenu bar: role
// items send AppKit's standard actions down the responder chain, the
// others report their ID with EventMenu.

// chrome is a surface's window state.
type chrome struct {
	titleBar    driver.TitleBarStyle
	titleHeight driver.Points
	controls    driver.WindowControls
	// placed is set while the traffic lights are where layoutTrafficLights
	// put them rather than where AppKit does.
	placed bool
	// constraints are the app's limits on the window.
	constraints driver.WindowConstraints
	// level and workspaces are where the app put the window (setLevel, setWorkspaces), which applySpaces reads with constraints.
	level      driver.WindowLevel
	workspaces driver.WindowWorkspaces

	appName     string
	menu        objc.ID // the app's menu bar
	menuEntries []driver.MenuEntry
	menuOrder   uint64
	language    string // what the menus say (driver.ItemDirection)
	menuTarget  objc.ID
	// roleFallback are the roles of the editing items with an ID, which menuRoleChosen carries out or reports.
	roleFallback map[uint32]driver.MenuRole
	// notInText are the IDs of the items whose shortcuts a field or a
	// web view with the keyboard focus keeps (MenuEntry.NotInText).
	notInText map[uint32]bool
	// editingKeys are the actions of the notInText items whose key equivalent is an editing role's, such as ⌘Z for undo:, which the focused field gets only through the menu (see validateMenu).
	editingKeys map[uint32]objc.SEL
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

	// fullScreenNone is the collection behavior of a window that never
	// goes full screen.
	fullScreenNone = 1 << 9
)

var (
	selStyleMask           = sel("styleMask")
	selStandardButton      = sel("standardWindowButton:")
	selSetFrameOrigin      = sel("setFrameOrigin:")
	selContentRectForFrame = sel("contentRectForFrameRect:styleMask:")
)

// rawHit makes the views' hitTest: return the deepest view that takes
// the pointer, by its pointer events, rather than the pressable ancestor a
// press goes to: for the window drag's decision, drops, selections and the
// scroll views under the mouse.
var rawHit bool

var selMenuChosen = sel("nsgoMenuChosen:")
var selMenuRoleChosen = sel("nsgoMenuRoleChosen:")
var selDockMenuChosen = sel("nsgoDockMenuChosen:")

// The action of the full screen role's item, in place of AppKit's
// toggleFullScreen:, whose validation titles the item in the language of
// the app's bundle.
var selMenuFullScreen = sel("nsgoMenuFullScreen:")

// fullScreenTitled is the tag of a full screen item the app did not label,
// which says what choosing it does.
const fullScreenTitled = -1

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
	case driver.ItemDockMenu:
		s.setDockMenu(it.Menu)
	case driver.ItemRecentFile:
		recentFile(it.Value, it.On)
	}
}

// recentFile notes path among the app's recent documents, which the Dock menu and File › Open Recent list for the types the app declares, or forgets them all when clearRecent.
func recentFile(path string, clearRecent bool) {
	doc := objc.ID(apple.Class("NSDocumentController")).Send(sel("sharedDocumentController"))
	if clearRecent {
		doc.Send(sel("clearRecentDocuments:"), objc.ID(0))
		return
	}
	url := objc.ID(apple.Class("NSURL")).Send(selAlloc).Send(sel("initFileURLWithPath:"), apple.NSString(path))
	if url != 0 {
		doc.Send(sel("noteNewRecentDocumentURL:"), url)
		url.Send(selRelease)
	}
}

func (s *surface) command(c *driver.WindowCommand) {
	w := s.window
	switch c.Action {
	case driver.WindowSetTitleBar:
		s.titleBar, s.titleHeight = c.TitleBar, c.Height
		s.applyTitleBar()
		// A title bar's change keeps the content's size, not the frame's.
		if s.openFrame.Move {
			s.setWindowFrame(s.openFrame)
		}
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
	case driver.WindowSetWorkspaces:
		s.setWorkspaces(c.Workspaces)
	case driver.WindowOrderFront:
		if s.kind == driver.SurfacePanel {
			s.showPanel(s.anchor, s.anchorBounds, s.placement)
		} else {
			s.orderFront()
		}
	case driver.WindowShow:
		if s.kind == driver.SurfacePanel {
			s.showPanel(c.Anchor, c.AnchorBounds, c.Placement)
		} else {
			s.show(c.Anchor)
		}
	case driver.WindowHide:
		s.showFirst = false
		if s.kind == driver.SurfacePanel {
			s.hidePanel(false)
		} else {
			s.childShown = false
			s.attachChildWindow(false)
			w.Send(sel("orderOut:"), objc.ID(0))
			s.syncChildWindows()
		}
	case driver.WindowSetControls:
		hidden := s.controls.Hidden
		s.controls = c.Controls
		// Moving them, as a transition does each frame, needs no new
		// style mask.
		if c.Controls.Hidden == hidden {
			s.layoutTrafficLights()
			s.refresh()
			break
		}
		s.applyTitleBar()
	case driver.WindowSetMaterial:
		s.setMaterial(c.Material)
		s.refresh()
	case driver.WindowSetConstraints:
		s.setConstraints(c.Constraints)
	case driver.WindowSetPointerEvents:
		s.setPointerEvents(c.PointerEvents)
	case driver.WindowSetTransparent:
		s.setTransparent(c.On)
	case driver.WindowSetShadow:
		if s.kind == driver.SurfaceWindow {
			w.Send(selSetHasShadow, c.On)
		}
	case driver.WindowSetSkipTaskbar:
		if s.kind == driver.SurfaceWindow {
			s.setSkipTaskbar(c.On)
		}
	case driver.WindowFitSize:
		if s.kind == driver.SurfacePanel {
			s.fitPanel(c.Frame.Width, c.Frame.Height)
		}
	case driver.WindowSetAnchor:
		if s.kind != driver.SurfacePanel {
			break
		}
		s.anchorBounds, s.placement = c.AnchorBounds, c.Placement
		// Moving a panel that shows leaves the key window as it is.
		if c.Placement != driver.WindowPlaceDefault && objc.Send[bool](s.window, sel("isVisible")) {
			s.window.Send(sel("setFrameOrigin:"), placePanel(c.AnchorBounds, objc.Send[apple.CGRect](s.window, sel("frame")).Size, c.Placement))
		}
	}
}

// setConstraints limits the window, a panel being the app's to size.
// miniaturize: does nothing without Miniaturizable, and zoom: nothing
// once windowShouldZoom:toFrame: declines, so the app's commands, the
// title bar's double click and the menus all follow.
func (s *surface) setConstraints(c driver.WindowConstraints) {
	w := s.window
	if w == 0 || s.kind != driver.SurfaceWindow {
		return
	}
	s.constraints = c
	w.Send(sel("setContentMinSize:"), apple.CGSize{Width: float64(c.MinWidth), Height: float64(c.MinHeight)})
	limit := func(p driver.Points) float64 {
		if p > 0 {
			return float64(p)
		}
		return math.MaxFloat32
	}
	w.Send(sel("setContentMaxSize:"), apple.CGSize{Width: limit(c.MaxWidth), Height: limit(c.MaxHeight)})
	s.applySpaces()
	mask := objc.Send[uint](w, selStyleMask)
	style := mask | styleResizable | styleMiniaturizable
	if c.NoResize {
		style &^= styleResizable
	}
	if c.NoMinimize {
		style &^= styleMiniaturizable
	}
	if style != mask {
		w.Send(sel("setStyleMask:"), style)
		s.applyTitleBar()
	}
	if zoom := w.Send(selStandardButton, zoomButton); zoom != 0 {
		zoom.Send(sel("setEnabled:"), !c.NoMaximize)
	}
	if mask&maskFullScreen != 0 {
		return
	}
	size := objc.Send[apple.CGRect](s.native, selBounds).Size
	shown := driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)}
	if fit := c.Clamp(shown); fit != shown {
		w.Send(sel("setContentSize:"), apple.CGSize{Width: float64(fit.Width), Height: float64(fit.Height)})
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
	for i := 0; i < 3; i++ {
		if b := w.Send(selStandardButton, i); b != 0 {
			b.Send(sel("setHidden:"), hidden && s.controls.Hidden)
		}
	}
	s.layoutTrafficLights()
	s.refresh()
	// AppKit can restore the old content size after setStyleMask:. The block holds the window until it has run, so it never depends on a release being queued after it, and leaves a surface that closed or changed windows meanwhile alone.
	w.Send(sel("retain"))
	apple.OnMain(func() {
		defer w.Send(selRelease)
		if s.closed || s.window != w {
			return
		}
		content := w.Send(sel("contentView"))
		size := objc.Send[apple.CGRect](w, sel("contentRectForFrameRect:"), objc.Send[apple.CGRect](w, sel("frame"))).Size
		if objc.Send[apple.CGRect](content, sel("frame")).Size != size {
			content.Send(selSetFrameSize, size)
		}
		s.layoutTrafficLights()
		s.refresh()
	})
}

// layoutTrafficLights puts the traffic lights in the app's bar: the
// close button's top left corner at the app's TrafficLights, by default
// 16 points in from the window's left edge as NSGo places them and
// centered in the bar, then moved by its Offset. Without a bar height or
// a place of the app's they stay where AppKit lays them out. AppKit puts
// them back whenever it lays the title bar out, so this runs after each
// of the window's layout passes (NsgoWindow), and changes only what moved.
func (s *surface) layoutTrafficLights() {
	w := s.window
	at := s.controls.TrafficLights
	// An embedding host's toolbar lays the title bar out itself.
	if w == 0 || s.titleBar != driver.TitleBarHidden || s.titleHeight <= 0 && at == (driver.Point{}) && s.controls.Offset == (driver.Point{}) ||
		w.Send(sel("toolbar")) != 0 {
		s.restoreTrafficLights()
		return
	}
	if objc.Send[uint](w, selStyleMask)&maskFullScreen != 0 {
		return
	}
	close := w.Send(selStandardButton, 0)
	if close == 0 {
		return
	}
	bar := close.Send(selSuperview)
	container := bar.Send(selSuperview)
	if container == 0 {
		return
	}
	button := objc.Send[apple.CGRect](close, sel("frame"))
	height := float64(s.barHeight())
	frame := objc.Send[apple.CGRect](container, sel("frame"))
	if y := objc.Send[apple.CGRect](w, sel("frame")).Size.Height - height; frame.Size.Height != height || frame.Origin.Y != y {
		frame.Size.Height, frame.Origin.Y = height, y
		container.Send(selSetFrame, frame)
	}
	left, top := float64(at.X), float64(at.Y)
	if left <= 0 {
		left = trafficLightsLeft
	}
	if top <= 0 {
		top = max((height-button.Size.Height)/2, 0)
	}
	left += float64(s.controls.Offset.X)
	top += float64(s.controls.Offset.Y)
	if !objc.Send[bool](bar, sel("isFlipped")) {
		top = objc.Send[apple.CGRect](bar, selBounds).Size.Height - top - button.Size.Height
	}
	mini := w.Send(selStandardButton, 1)
	gap := objc.Send[apple.CGRect](mini, sel("frame")).Origin.X - button.Origin.X
	for i := 0; i < 3; i++ {
		b := w.Send(selStandardButton, i)
		origin := apple.CGPoint{X: left + float64(i)*gap, Y: top}
		if b != 0 && objc.Send[apple.CGRect](b, sel("frame")).Origin != origin {
			b.Send(selSetFrameOrigin, origin)
		}
	}
	s.placed = true
}

// restoreTrafficLights gives the traffic lights back to AppKit after
// layoutTrafficLights placed them. A layout pass of the window leaves
// them where they are; hiding and showing them again makes AppKit lay
// the title bar out, as Electron's window_buttons_proxy.mm notes.
func (s *surface) restoreTrafficLights() {
	if !s.placed || s.window == 0 {
		return
	}
	s.placed = false
	for i := 0; i < 3; i++ {
		if b := s.window.Send(selStandardButton, i); b != 0 {
			hidden := objc.Send[bool](b, sel("isHidden"))
			b.Send(sel("setHidden:"), !hidden)
			b.Send(sel("setHidden:"), hidden)
		}
	}
}

// barHeight is the hidden title bar's height: the app's, or the one
// AppKit gives the window's own title bar.
func (s *surface) barHeight() driver.Points {
	if s.titleHeight > 0 {
		return s.titleHeight
	}
	w := s.window
	mask := objc.Send[uint](w, selStyleMask)
	frame := objc.Send[apple.CGRect](w, sel("frame"))
	content := objc.Send[apple.CGRect](objc.ID(apple.Class("NSWindow")), selContentRectForFrame,
		frame, mask&^maskFullSize)
	if h := frame.Size.Height - content.Size.Height; h > 0 {
		return driver.Points(h)
	}
	return titleBarHeight
}

// titleBarArea is what a hidden title bar covers: its height on top and
// the traffic lights on the left, unless the app hid them.
func (s *surface) titleBarArea() driver.Edges {
	w := s.window
	if w == 0 || s.titleBar != driver.TitleBarHidden || s.transparent {
		return driver.Edges{}
	}
	if objc.Send[uint](w, selStyleMask)&maskFullScreen != 0 {
		return driver.Edges{}
	}
	height := s.barHeight()
	if w.Send(sel("toolbar")) != 0 {
		// The toolbar an embedding host put there, as AppKit laid it out.
		frame := objc.Send[apple.CGRect](w, sel("frame"))
		layout := objc.Send[apple.CGRect](w, sel("contentLayoutRect"))
		if h := frame.Size.Height - (layout.Origin.Y + layout.Size.Height); h > 0 {
			height = driver.Points(h)
		}
	}
	if s.controls.Hidden {
		return driver.Edges{Top: height}
	}
	left := driver.Points(trafficLightsWidth)
	if zoom := w.Send(selStandardButton, zoomButton); zoom != 0 {
		f := objc.Send[apple.CGRect](zoom, sel("frame"))
		if r := f.Origin.X + f.Size.Width + 8; r > 8 {
			left = driver.Points(r)
		}
	}
	return driver.Edges{Top: height, Left: left}
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
	// A transparent window has no title bar whose double-click zooms or minimizes it, as a desktop pet's must not: it drags as for one click.
	if objc.Send[int](event, sel("clickCount")) == 2 && !s.transparent {
		defaults := objc.ID(apple.Class("NSUserDefaults")).Send(sel("standardUserDefaults"))
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
			return false // an input's field
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
	menuSequence++
	s.menuOrder = menuSequence
	mainMenuSurface = s
	nsApp := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
	s.menuEntries = entries
	if entries == nil {
		if s.menu != 0 {
			defer s.menu.Send(selRelease)
			s.menu = 0
		}
		showDefaultMenu()
		s.registerMenus(0)
		return
	}
	s.ensureMenuTarget()
	clear(s.notInText)
	clear(s.roleFallback)
	clear(s.editingKeys)
	bar := objc.ID(apple.Class("NSMenu")).Send(selNew)
	for _, e := range entries {
		if e.Separator {
			continue
		}
		items := e.Items
		if len(items) == 0 && !e.Role.HoldsMenu() {
			// A top-level action gets a menu of its own.
			items = []driver.MenuEntry{e}
		}
		holder := s.holder(e.Role, menutext.Label(s.language, &e, s.name()), e.Disabled, items)
		bar.Send(sel("addItem:"), holder)
		holder.Send(selRelease)
	}
	if s.menu != 0 && nsApp.Send(sel("mainMenu")) == s.menu {
		// setMainMenu: redraws the whole bar, which flickers as an app
		// sets its menu again, such as for an item it checks, so the bar
		// shown takes the new one's items in place.
		syncMenu(s.menu, bar)
		bar.Send(selRelease)
		s.directMenu(s.menu)
		s.registerMenus(s.menu)
		return
	}
	s.directMenu(bar)
	nsApp.Send(sel("setMainMenu:"), bar)
	s.registerMenus(bar)
	if s.menu != 0 {
		s.menu.Send(selRelease)
	}
	s.menu = bar
}

// restoreMainMenu puts the newest surviving surface's menu back on AppKit's menu bar as s, its current owner, closes, or the default menu when none remains. The surface sets its menu again rather than showing its old bar: the bars shown since took that bar's Services menu, which AppKit fills, and registered their own Window and Help menus.
func (s *surface) restoreMainMenu() {
	var next *surface
	for candidate, alive := range surfaces {
		if !alive || candidate == nil || candidate.closed || candidate.embedded || candidate.menuOrder == 0 {
			continue
		}
		if next == nil || candidate.menuOrder > next.menuOrder {
			next = candidate
		}
	}
	if next != nil {
		next.setMenu(next.menuEntries)
		return
	}
	showDefaultMenu()
	s.registerMenus(0)
	mainMenuSurface = nil
}

// dockMenu is the menu applicationDockMenu: hands AppKit, the one a surface set last (core.Scope.SetDockMenu), and that surface, whose engine hears its items.
var dockMenu struct {
	menu   objc.ID
	target *surface
}

// setDockMenu makes entries the app's Dock menu, nil none. AppKit asks for it as the Dock shows the menu, so it is built here, ahead of time.
func (s *surface) setDockMenu(entries []driver.MenuEntry) {
	clearDockMenu()
	if entries == nil {
		return
	}
	s.ensureMenuTarget()
	dockMenu.menu, dockMenu.target = s.dockSubmenu("", entries), s
}

// clearDockMenu leaves the app with the system's Dock items alone.
func clearDockMenu() {
	if dockMenu.menu != 0 {
		dockMenu.menu.Send(selRelease)
	}
	dockMenu.menu, dockMenu.target = 0, nil
}

// dockSubmenu returns a new menu of Dock items: as submenu builds the application menu's, but its actions report EventDockMenu, and roles and shortcuts, which a Dock menu has none of, are left out.
func (s *surface) dockSubmenu(title string, items []driver.MenuEntry) objc.ID {
	m := objc.ID(apple.Class("NSMenu")).Send(selAlloc).Send(sel("initWithTitle:"), apple.NSString(title))
	lastSeparator := true
	for _, e := range items {
		if e.Separator {
			if !lastSeparator {
				sep := objc.ID(apple.Class("NSMenuItem")).Send(sel("separatorItem"))
				sep.Send(selSetIdentifier, ownItem())
				m.Send(sel("addItem:"), sep)
				lastSeparator = true
			}
			continue
		}
		action := objc.SEL(0)
		switch {
		case e.Disabled:
			action = selMenuDisabled
		case e.ID != 0 && len(e.Items) == 0:
			action = selDockMenuChosen
		}
		it := objc.ID(apple.Class("NSMenuItem")).Send(selAlloc).Send(sel("initWithTitle:action:keyEquivalent:"),
			apple.NSString(e.Label), action, apple.NSString(""))
		it.Send(selSetIdentifier, ownItem())
		if action == selDockMenuChosen {
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
		if len(e.Items) > 0 {
			sub := s.dockSubmenu(e.Label, e.Items)
			it.Send(sel("setSubmenu:"), sub)
			sub.Send(selRelease)
		}
		m.Send(sel("addItem:"), it)
		it.Send(selRelease)
		lastSeparator = false
	}
	if n := objc.Send[int](m, sel("numberOfItems")); n > 0 && lastSeparator {
		m.Send(sel("removeItemAtIndex:"), n-1)
	}
	return m
}

// syncMenu makes the app's items of menu m what next is, keeping those
// still of the same kind and taking the others from next. The items
// AppKit adds itself, such as the Edit menu's Start Dictation, stay.
func syncMenu(m, next objc.ID) {
	if t := next.Send(sel("title")); apple.GoString(m.Send(sel("title"))) != apple.GoString(t) {
		m.Send(sel("setTitle:"), t)
	}
	var old []objc.ID
	for i := range objc.Send[int](m, sel("numberOfItems")) {
		if it := m.Send(selItemAtIndex, i); isOwnItem(it) {
			old = append(old, it)
		}
	}
	n := objc.Send[int](next, sel("numberOfItems"))
	items := make([]objc.ID, n)
	for i := range items {
		items[i] = next.Send(selItemAtIndex, 0).Send(sel("retain"))
		next.Send(sel("removeItemAtIndex:"), 0)
	}
	at := 0 // where the next item goes: after the one before it
	for i, it := range items {
		if i < len(old) {
			if sameMenuKind(old[i], it) {
				syncMenuItem(old[i], it)
				it.Send(selRelease)
				at = objc.Send[int](m, sel("indexOfItem:"), old[i]) + 1
				continue
			}
			m.Send(sel("removeItem:"), old[i])
		}
		m.Send(sel("insertItem:atIndex:"), it, at)
		it.Send(selRelease)
		at++
	}
	for _, it := range old[min(n, len(old)):] {
		m.Send(sel("removeItem:"), it)
	}
}

var selSetIdentifier = sel("setIdentifier:")

// ownItemID is the identifier of the menu items NSGo makes.
const ownItemID = "dev.nsgo.menu"

func ownItem() objc.ID { return apple.NSString(ownItemID) }

func isOwnItem(it objc.ID) bool { return apple.GoString(it.Send(sel("identifier"))) == ownItemID }

var selItemAtIndex = sel("itemAtIndex:")

// sameMenuKind reports whether a and b are both separators, both hold
// submenus or both are actions.
func sameMenuKind(a, b objc.ID) bool {
	sep := sel("isSeparatorItem")
	sub := a.Send(sel("submenu")) != 0
	return objc.Send[bool](a, sep) == objc.Send[bool](b, sep) && sub == (b.Send(sel("submenu")) != 0) &&
		// A holder's tag is the role that registers its menu with AppKit, which fills it, so a menu of another role is a new one.
		(!sub || objc.Send[int](a, sel("tag")) == objc.Send[int](b, sel("tag")))
}

// syncMenuItem gives item it what next has, setting only what changed.
func syncMenuItem(it, next objc.ID) {
	if objc.Send[bool](next, sel("isSeparatorItem")) {
		return
	}
	for _, p := range [][2]string{{"title", "setTitle:"}, {"keyEquivalent", "setKeyEquivalent:"}} {
		if v := next.Send(sel(p[0])); apple.GoString(it.Send(sel(p[0]))) != apple.GoString(v) {
			it.Send(sel(p[1]), v)
		}
	}
	if v := objc.Send[uint](next, sel("keyEquivalentModifierMask")); objc.Send[uint](it, sel("keyEquivalentModifierMask")) != v {
		it.Send(sel("setKeyEquivalentModifierMask:"), v)
	}
	if v := objc.Send[objc.SEL](next, sel("action")); objc.Send[objc.SEL](it, sel("action")) != v {
		it.Send(sel("setAction:"), v)
	}
	if v := next.Send(sel("target")); it.Send(sel("target")) != v {
		it.Send(selSetTarget, v)
	}
	for _, p := range [][2]string{{"tag", "setTag:"}, {"state", "setState:"}} {
		if v := objc.Send[int](next, sel(p[0])); objc.Send[int](it, sel(p[0])) != v {
			it.Send(sel(p[1]), v)
		}
	}
	if sub := next.Send(sel("submenu")); sub != 0 {
		syncMenu(it.Send(sel("submenu")), sub)
	}
}

// relabelMenus says the menu bar again in the surface's language.
func (s *surface) relabelMenus() {
	if s.language != defaultLanguage {
		rebuildDefaultMenu(s.language)
	}
	if s.menu != 0 {
		s.setMenu(s.menuEntries)
	}
}

// The action of disabled items: nothing answers it, so AppKit disables
// them as it validates the menu.
var selMenuDisabled = sel("nsgoMenuItemDisabled:")

// holder returns a new item holding a submenu of items.
func (s *surface) holder(role driver.MenuRole, label string, disabled bool, items []driver.MenuEntry) objc.ID {
	var action objc.SEL
	if disabled {
		action = selMenuDisabled
	}
	it := objc.ID(apple.Class("NSMenuItem")).Send(selAlloc).Send(sel("initWithTitle:action:keyEquivalent:"),
		apple.NSString(label), action, apple.NSString(""))
	it.Send(selSetIdentifier, ownItem())
	if role.HoldsMenu() {
		// The tag marks the role, which registerMenus finds in the bar shown; a holder's tag is otherwise unused.
		it.Send(sel("setTag:"), -int(role))
	}
	sub := s.submenu(label, items)
	it.Send(sel("setSubmenu:"), sub)
	sub.Send(selRelease)
	return it
}

// registeredHelp is the Help menu registerMenus last gave AppKit.
var registeredHelp objc.ID

// registerMenus tells AppKit which of bar's submenus are the Services, Window and Help menus, the first each role marks. It runs on the bar shown, as syncMenu keeps that bar's submenus, which AppKit fills, rather than the new bar's, and AppKit takes a Window menu only from the main menu. AppKit keeps the first Services menu it is given, ignoring any other and nil, so a later Services holder takes that menu over with its own items, and it ignores nil for the Window menu too, which then stays a menu no bar shows; a Help menu given before and no longer marked is taken back, leaving AppKit to find one by its title.
func (s *surface) registerMenus(bar objc.ID) {
	holders := map[driver.MenuRole]objc.ID{}
	var walk func(m objc.ID)
	walk = func(m objc.ID) {
		for i := range objc.Send[int](m, sel("numberOfItems")) {
			it := m.Send(selItemAtIndex, i)
			sub := it.Send(sel("submenu"))
			if sub == 0 || !isOwnItem(it) {
				continue
			}
			if tag := objc.Send[int](it, sel("tag")); tag < 0 {
				if r := driver.MenuRole(-tag); r.HoldsMenu() && holders[r] == 0 {
					holders[r] = it
				}
			}
			walk(sub)
		}
	}
	if bar != 0 {
		walk(bar)
	}
	nsApp := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
	if it := holders[driver.MenuServices]; it != 0 {
		if cur := nsApp.Send(sel("servicesMenu")); cur == 0 {
			nsApp.Send(sel("setServicesMenu:"), it.Send(sel("submenu")))
		} else if sub := it.Send(sel("submenu")); cur != sub {
			if super := cur.Send(sel("supermenu")); super != 0 {
				// A menu is in one menu at a time: the bar it leaves is being replaced or no longer shown.
				if i := objc.Send[int](super, sel("indexOfItemWithSubmenu:"), cur); i >= 0 {
					super.Send(selItemAtIndex, i).Send(sel("setSubmenu:"), objc.ID(0))
				}
			}
			syncMenu(cur, sub)
			it.Send(sel("setSubmenu:"), cur)
			s.directMenu(cur)
		}
	}
	if it := holders[driver.MenuWindowMenu]; it != 0 {
		if sub := it.Send(sel("submenu")); nsApp.Send(sel("windowsMenu")) != sub {
			nsApp.Send(sel("setWindowsMenu:"), sub)
		}
	}
	help := objc.ID(0)
	if it := holders[driver.MenuHelpMenu]; it != 0 {
		help = it.Send(sel("submenu"))
	}
	if cur := nsApp.Send(sel("helpMenu")); cur != help && (help != 0 || cur == registeredHelp) {
		nsApp.Send(sel("setHelpMenu:"), help)
	}
	registeredHelp = help
}

// submenu returns a new menu of items.
func (s *surface) submenu(title string, items []driver.MenuEntry) objc.ID {
	m := objc.ID(apple.Class("NSMenu")).Send(selAlloc).Send(sel("initWithTitle:"), apple.NSString(title))
	lastSeparator := true
	for _, e := range items {
		switch {
		case e.Separator:
			if !lastSeparator {
				sep := objc.ID(apple.Class("NSMenuItem")).Send(sel("separatorItem"))
				sep.Send(selSetIdentifier, ownItem())
				m.Send(sel("addItem:"), sep)
				lastSeparator = true
			}
			continue
		case len(e.Items) > 0 || e.Role.HoldsMenu():
			holder := s.holder(e.Role, menutext.Label(s.language, &e, s.name()), e.Disabled, e.Items)
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
	driver.MenuPasteAndMatchStyle: "pasteAsPlainText:", driver.MenuDelete: "delete:",
	driver.MenuStartSpeaking: "startSpeaking:", driver.MenuStopSpeaking: "stopSpeaking:",
	driver.MenuBringAllToFront: "arrangeInFront:", driver.MenuMinimize: "performMiniaturize:",
	driver.MenuZoom: "performZoom:", driver.MenuClose: "performClose:",
}

// item returns a new menu item of e.
func (s *surface) item(e driver.MenuEntry) objc.ID {
	title := menutext.Label(s.language, &e, s.name())
	fullScreen := e.Role == driver.MenuToggleFullScreen
	titled := fullScreen && (e.Label == "" || e.Label == e.Role.Label())
	if titled {
		title = locales.In(s.language).EnterFullScreen() // validateMenu retitles it as the window changes
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
	case fullScreen:
		action = selMenuFullScreen
	case e.Role != driver.MenuRoleNone:
		if e.ID != 0 && (e.Role.EditsText() || e.Role == driver.MenuPasteAndMatchStyle) {
			action = selMenuRoleChosen
		} else if e.ID != 0 {
			action = selMenuChosen
		} else {
			action = sel(roleActions[e.Role])
		}
	case e.ID != 0:
		action = selMenuChosen
	}
	it := objc.ID(apple.Class("NSMenuItem")).Send(selAlloc).Send(sel("initWithTitle:action:keyEquivalent:"),
		apple.NSString(title), action, apple.NSString(key))
	it.Send(selSetIdentifier, ownItem())
	if key != "" {
		it.Send(sel("setKeyEquivalentModifierMask:"), mods)
	}
	if action == selMenuFullScreen {
		it.Send(selSetTarget, s.menuTarget)
		if titled {
			it.Send(sel("setTag:"), fullScreenTitled)
		}
	}
	if action == selMenuRoleChosen {
		if s.roleFallback == nil {
			s.roleFallback = map[uint32]driver.MenuRole{}
		}
		s.roleFallback[uint32(e.ID)] = e.Role
		it.Send(selSetTarget, s.menuTarget)
		it.Send(sel("setTag:"), int(e.ID))
	}
	if action == selMenuChosen {
		it.Send(selSetTarget, s.menuTarget)
		it.Send(sel("setTag:"), int(e.ID))
		if e.NotInText && key != "" {
			if s.notInText == nil {
				s.notInText = map[uint32]bool{}
			}
			s.notInText[e.ID] = true
			if action := editingKey(key, mods); action != 0 {
				if s.editingKeys == nil {
					s.editingKeys = map[uint32]objc.SEL{}
				}
				s.editingKeys[e.ID] = action
			}
		}
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

// editingKey returns the action of the editing role whose key equivalent is key with mods, such as undo: for ⌘Z, or 0. A field editor takes these keys only through the menu: it has no binding of its own for them.
func editingKey(key string, mods uint) objc.SEL {
	for _, r := range []driver.MenuRole{driver.MenuUndo, driver.MenuRedo, driver.MenuCut, driver.MenuCopy,
		driver.MenuPaste, driver.MenuSelectAll, driver.MenuPasteAndMatchStyle} {
		if k, m := keyEquivalent(driver.NormalizeKey(r.Shortcut(true))); k == key && m == mods {
			return sel(roleActions[r])
		}
	}
	return 0
}

// name is the app's name, for the app menu's items: in the menus'
// language as the bundle says it, else the app's title, else the name
// the system gives it in the languages it launched in.
func (s *surface) name() string {
	if name := apple.BundleName(s.language); name != "" {
		return name
	}
	if s.appName == "" {
		s.appName = apple.GoString(objc.ID(apple.Class("NSRunningApplication")).
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

// validateMenu disables an item whose shortcut a field with the keyboard focus keeps while a key press looks for it, so AppKit hands the key to the field; clicked, the item is enabled. An item whose key is an editing role's, such as ⌘Z, stays enabled, and menuChosen sends the field the role's action, since a field editor has no binding of its own for those keys and takes them only from the menu.
func validateMenu(self objc.ID, _ objc.SEL, item objc.ID) bool {
	s := menuTargets[self]
	if s != nil && objc.Send[objc.SEL](item, sel("action")) == selMenuFullScreen {
		return s.validateFullScreen(item)
	}
	id := uint32(objc.Send[int](item, sel("tag")))
	if s != nil {
		if role, ok := s.roleFallback[id]; ok {
			return s.validateRoleMenuItem(item, role)
		}
	}
	if s == nil || !s.notInText[id] || s.editingKeys[id] != 0 {
		return true
	}
	return s.keyField() == 0
}

// validateRoleMenuItem asks the responder AppKit would use for an editing
// role whether it can carry out the action, in the same window that choosing
// the item uses. No editor leaves the item to the app's OnSelect fallback.
func (s *surface) validateRoleMenuItem(item objc.ID, role driver.MenuRole) bool {
	action := sel(roleActions[role])
	if action == 0 {
		return true
	}
	w := s.roleWindow()
	var target objc.ID
	if ws := windowSurface(w); ws != nil {
		if !ws.edits(action) {
			return true
		}
		target = w
	} else {
		target = nsApp().Send(sel("targetForAction:to:from:"), action, objc.ID(0), objc.ID(0))
	}
	if target == 0 {
		return true
	}
	if objc.Send[bool](target, sel("respondsToSelector:"), sel("firstResponder")) && windowEditing(target) {
		first := target.Send(sel("firstResponder"))
		switch role {
		case driver.MenuUndo, driver.MenuRedo:
			if objc.Send[bool](first, sel("respondsToSelector:"), sel("undoManager")) {
				manager := first.Send(sel("undoManager"))
				if manager == 0 {
					break
				}
				if role == driver.MenuUndo {
					return objc.Send[bool](manager, sel("canUndo"))
				}
				return objc.Send[bool](manager, sel("canRedo"))
			}
		case driver.MenuCut, driver.MenuCopy, driver.MenuDelete:
			if objc.Send[bool](first, sel("respondsToSelector:"), selSelectedRange) {
				selected := objc.Send[apple.NSRange](first, selSelectedRange)
				return selected.Length != 0
			}
		}
	}
	if objc.Send[bool](target, sel("respondsToSelector:"), sel("firstResponder")) {
		if first := target.Send(sel("firstResponder")); first != 0 {
			target = first
		}
	}
	oldAction := objc.Send[objc.SEL](item, sel("action"))
	item.Send(sel("setAction:"), action)
	defer item.Send(sel("setAction:"), oldAction)
	if objc.Send[bool](target, sel("respondsToSelector:"), sel("validateUserInterfaceItem:")) {
		return objc.Send[bool](target, sel("validateUserInterfaceItem:"), item)
	}
	if objc.Send[bool](target, sel("respondsToSelector:"), sel("validateMenuItem:")) {
		return objc.Send[bool](target, sel("validateMenuItem:"), item)
	}
	return true
}

// keyField returns the window a key press looking for a menu item's key equivalent went to when a field or a web view in it has the keyboard focus, and 0 otherwise. The menu bar is the app's whichever surface set it, so the window is the key's own, any of the app's or AppKit's: the event's, else the key window, the main window, and the menu's own window while no window has the keyboard.
func (s *surface) keyField() objc.ID {
	const keyDown = 10 // NSEventTypeKeyDown
	app := nsApp()
	e := app.Send(sel("currentEvent"))
	if e == 0 || objc.Send[uint](e, sel("type")) != keyDown {
		return 0
	}
	w := e.Send(selWindow)
	if w == 0 {
		w = app.Send(sel("keyWindow"))
	}
	if w == 0 {
		w = app.Send(sel("mainWindow"))
	}
	if w == 0 && !s.embedded {
		w = s.window
	}
	if w == 0 || !windowEditing(w) {
		return 0
	}
	return w
}

// validateFullScreen enables a full screen item while a window has the
// keyboard focus, and titles it by what choosing it does there.
func (s *surface) validateFullScreen(item objc.ID) bool {
	w := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication")).Send(sel("keyWindow"))
	if w == 0 {
		return false
	}
	if objc.Send[int](item, sel("tag")) == fullScreenTitled {
		msgs, title := locales.In(s.language), ""
		if objc.Send[uint](w, selStyleMask)&maskFullScreen != 0 {
			title = msgs.ExitFullScreen()
		} else {
			title = msgs.EnterFullScreen()
		}
		item.Send(sel("setTitle:"), apple.NSString(title))
	}
	return true
}

// menuFullScreen toggles the window with the keyboard focus.
func menuFullScreen(objc.ID, objc.SEL, objc.ID) {
	objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication")).Send(sel("keyWindow")).Send(sel("toggleFullScreen:"), objc.ID(0))
}

// menuChosen reports a chosen menu item's ID to its surface, or, for an item whose key is an editing role's chosen by that key while a field or a web view has the keyboard focus, sends the role's action to it in the item's place (validateMenu).
func menuChosen(self objc.ID, _ objc.SEL, item objc.ID) {
	s := menuTargets[self]
	id := objc.Send[int](item, sel("tag"))
	if id <= 0 || s == nil || s.engine == nil || s.closed {
		return
	}
	if action := s.editingKeys[uint32(id)]; action != 0 {
		if w := s.keyField(); w != 0 {
			w.Send(sel("firstResponder")).Send(sel("tryToPerform:with:"), action, item)
			return
		}
	}
	s.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: uint32(id)})
}

// menuRoleChosen carries out an editing role with an ID: AppKit's action goes down the responder chain when an editor takes it, otherwise the item's ID is reported.
func menuRoleChosen(self objc.ID, _ objc.SEL, item objc.ID) {
	s := menuTargets[self]
	if s == nil || s.engine == nil || s.closed {
		return
	}
	id := uint32(objc.Send[int](item, sel("tag")))
	action := sel(roleActions[s.roleFallback[id]])
	if action == 0 {
		return
	}
	app := nsApp()
	w := s.roleWindow()
	ws := windowSurface(w)
	switch {
	case ws == nil:
		// A window of AppKit's own, such as a panel, edits by its responder.
		target := app.Send(sel("targetForAction:to:from:"), action, objc.ID(0), objc.ID(0))
		if target != 0 && !objc.Send[bool](target, sel("isKindOfClass:"), apple.Class("NSWindow")) {
			app.Send(sel("sendAction:to:from:"), action, target, item)
			return
		}
	case ws.edits(action):
		w.Send(sel("firstResponder")).Send(sel("tryToPerform:with:"), action, item)
		return
	}
	s.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: id})
}

// roleWindow is the window whose editor takes a menu role, for both validation
// and dispatch. While the app is inactive, use the menu's own window.
func (s *surface) roleWindow() objc.ID {
	app := nsApp()
	w := app.Send(sel("keyWindow"))
	if w == 0 {
		w = app.Send(sel("mainWindow"))
	}
	if w == 0 && !s.embedded {
		w = s.window
	}
	return w
}

// edits reports whether an editor in the surface's window takes an editing role's action: a field or a web view with the keyboard focus, or the surface's text selection for Copy (when something is selected) and Select All. A responder for the action does not settle it, since every window answers undo: and redo: and the surface answers copy: and selectAll: with no selection.
func (s *surface) edits(action objc.SEL) bool {
	if s.editing() {
		return true
	}
	sl := s.selecting
	if sl == nil || s.window.Send(sel("firstResponder")) != s.native {
		return false
	}
	return action == sel("selectAll:") || action == sel("copy:") && !sl.model.IsEmpty()
}

// dockMenuChosen reports a chosen Dock menu item's ID to the surface that set the menu.
func dockMenuChosen(self objc.ID, _ objc.SEL, item objc.ID) {
	s := menuTargets[self]
	if id := objc.Send[int](item, sel("tag")); id > 0 && s != nil && s.engine != nil && !s.closed {
		s.engine.Dispatch(driver.Event{Kind: driver.EventDockMenu, Count: uint32(id)})
	}
}
