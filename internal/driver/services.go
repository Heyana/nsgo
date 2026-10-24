package driver

// AlertConfig is a native alert: a title, a message and its buttons.
type AlertConfig struct {
	Title   string
	Message string
	Actions []AlertAction
	// ID numbers the alert; the engine sets it.
	ID uint32
}

// AlertAction is one button of an alert.
type AlertAction struct {
	Label string
	Role  AlertRole
}

// AlertRole says what an alert's button does.
type AlertRole uint8

const (
	AlertDefault AlertRole = iota
	// AlertCancel is the button Escape and back choose.
	AlertCancel
	// AlertDestructive is drawn as dangerous.
	AlertDestructive
)

// TitleBarStyle is whether the system draws the window's title bar.
type TitleBarStyle uint8

const (
	// TitleBarDefault is the system's title bar, above the app.
	TitleBarDefault TitleBarStyle = iota
	// TitleBarHidden lets the app fill the window up to its top edge and
	// draw its own title bar. The system's window controls stay, in the
	// area Env.TitleBar reports.
	TitleBarHidden
)

// WindowDrag marks a view as a handle that moves the desktop window, or
// keeps a view inside such a handle clickable. The nearest view that
// sets it decides; inputs and views with press or gesture handlers
// nearer than it stay clickable either way.
type WindowDrag uint8

const (
	// WindowDragInherit follows the views around it.
	WindowDragInherit WindowDrag = iota
	// WindowDragOn drags the window, and a double click zooms it as the
	// system's title bar does.
	WindowDragOn
	// WindowDragOff keeps presses on the view for the app.
	WindowDragOff
)

// WindowMaterial is a translucent material behind a desktop window's
// content, where the app draws nothing opaque. macOS shows a system
// vibrancy material; Windows 11 22H2 and later shows Mica, Acrylic or Mica
// Alt, choosing the closest backdrop for the other names. Other platforms
// keep the window opaque.
//
// The app chooses a material when it starts (nsgo.WindowMaterial) and the
// window keeps it for its life; Env.WindowMaterial reports the one the
// window actually shows. What it draws over the material decides how much
// shows: a view with an opaque background hides it, and one with no
// background, or a translucent one, lets it through.
type WindowMaterial uint8

const (
	// WindowMaterialNone keeps the window opaque.
	WindowMaterialNone WindowMaterial = iota
	// The materials of window areas.
	WindowMaterialWindow
	WindowMaterialContent
	WindowMaterialSidebar
	WindowMaterialHeader
	WindowMaterialTitlebar
	WindowMaterialUnderWindow
	WindowMaterialUnderPage
	// The materials of transient surfaces: menus, popovers, HUD windows,
	// sheets, tooltips, selections and full-screen UI.
	WindowMaterialMenu
	WindowMaterialPopover
	WindowMaterialHUD
	WindowMaterialSheet
	WindowMaterialTooltip
	WindowMaterialSelection
	WindowMaterialFullScreenUI
	// Windows 11's system backdrops. Mica and Tabbed use the under-window
	// material on macOS; Acrylic uses the HUD one.
	WindowMaterialMica
	WindowMaterialAcrylic
	WindowMaterialTabbed
)

// WindowAction is what an ItemWindow asks of the window.
type WindowAction uint8

const (
	// WindowSetTitleBar shows or hides the title bar (TitleBar, Height).
	WindowSetTitleBar WindowAction = iota + 1
	WindowMinimize
	// WindowToggleMaximize maximizes the window, or restores it.
	WindowToggleMaximize
	WindowToggleFullScreen
	// WindowClose closes the window as its close button does.
	WindowClose
)

// WindowCommand is an ItemWindow's request.
type WindowCommand struct {
	Action   WindowAction
	TitleBar TitleBarStyle
	// Height is the custom title bar's height, in which the platform
	// centers its window controls; 0 is the platform's.
	Height Points
}

// MenuRole is a standard menu item the platform carries out, labelling it
// and giving it its usual shortcut when the item has none of its own. The
// app is not told when it is chosen.
type MenuRole uint8

const (
	MenuRoleNone MenuRole = iota
	// MenuAbout, MenuHide, MenuHideOthers and MenuShowAll are macOS's.
	MenuAbout
	MenuHide
	MenuHideOthers
	MenuShowAll
	MenuQuit
	// The edit roles act on the focused text field.
	MenuUndo
	MenuRedo
	MenuCut
	MenuCopy
	MenuPaste
	MenuSelectAll
	MenuMinimize
	MenuZoom
	MenuToggleFullScreen
	MenuClose
)

var menuRoleLabels = [...]string{"", "About", "Hide", "Hide Others", "Show All", "Quit", "Undo", "Redo",
	"Cut", "Copy", "Paste", "Select All", "Minimize", "Zoom", "Toggle Full Screen", "Close"}

// Label returns the label of an item with role r and no label of its own.
func (r MenuRole) Label() string {
	if int(r) < len(menuRoleLabels) {
		return menuRoleLabels[r]
	}
	return ""
}

// Shortcut returns the shortcut of an item with role r and no shortcut of
// its own, in OnKey's form, on a Mac (mac) or elsewhere; "" for none.
func (r MenuRole) Shortcut(mac bool) string {
	pick := func(onMac, other string) string {
		if mac {
			return onMac
		}
		return other
	}
	switch r {
	case MenuHide:
		return pick("meta+h", "")
	case MenuHideOthers:
		return pick("alt+meta+h", "")
	case MenuQuit:
		return pick("meta+q", "alt+f4")
	case MenuUndo:
		return "mod+z"
	case MenuRedo:
		return pick("shift+meta+z", "ctrl+y")
	case MenuCut:
		return "mod+x"
	case MenuCopy:
		return "mod+c"
	case MenuPaste:
		return "mod+v"
	case MenuSelectAll:
		return "mod+a"
	case MenuMinimize:
		return pick("meta+m", "")
	case MenuToggleFullScreen:
		return pick("ctrl+meta+f", "f11")
	case MenuClose:
		return "mod+w"
	}
	return ""
}

// EditsText reports whether r acts on the focused text field, whose own
// shortcut it then is.
func (r MenuRole) EditsText() bool { return r >= MenuUndo && r <= MenuSelectAll }

// MenuEntry is a MenuItem as an ItemMenu carries it: the item's handler
// replaced by an ID the platform reports with EventMenu (0: none), its
// shortcut normalized and its role's label filled in. A role item without
// a shortcut takes its role's, which the platform fills in.
type MenuEntry struct {
	ID        uint32
	Label     string
	Shortcut  string
	Role      MenuRole
	Disabled  bool
	Checkable bool
	Checked   bool
	Separator bool
	Items     []MenuEntry
}

// StorageOp says what a StorageChange does.
type StorageOp uint8

const (
	// StorageSet sets Key to Value.
	StorageSet StorageOp = iota
	// StorageRemove removes Key.
	StorageRemove
	// StorageClear removes every entry.
	StorageClear
)

// StorageChange is a change to the entries, for the backend to keep.
type StorageChange struct {
	Op         StorageOp
	Key, Value string
}

// StorageBackend is where a platform keeps an app's entries: a file in
// the app's data directory, the browser's localStorage.
type StorageBackend interface {
	// Load returns the entries kept, read once when the app first uses
	// its storage. An error leaves the app with none, logged.
	Load() (map[string]string, error)
	// Write keeps change. It is called in order, on the goroutine of the
	// graph that made the change; a backend that blocks writes elsewhere.
	Write(change StorageChange)
}
