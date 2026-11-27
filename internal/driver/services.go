package driver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
)

// Update is what a platform's updater with no windows of its own found
// (EventUpdate): the app's name and running version, and a newer version
// with the page and verified download that gets it, Version empty when the
// running one is the newest, or why it could not check.
type Update struct {
	// Restart identifies a prepared platform update applied by restarting, including platforms whose updater exposes no version label.
	Restart                          bool
	App, Current                     string
	Version, URL, Notes              string
	Download, Signature, SHA256, Key string
	Size                             int64
	Managed                          UpdateManaged
	Store                            string
	Err                              string
}

// UpdateDownloadProgress reports bytes received by a platform update
// downloader.
type UpdateDownloadProgress struct{ Loaded, Total int64 }

// UpdateDownload stages a verified update through the platform's HTTP
// client. The download runs independently of the app's surfaces.
var UpdateDownload func(context.Context, Update, string, func(UpdateDownloadProgress)) (string, error)

// AlertConfig is a native alert: a title, a message and its buttons.
type AlertConfig struct {
	Title   string
	Message string
	Actions []AlertAction
	// Sheet presents the actions as an action sheet from the bottom
	// (UIAlertController's; a list dialog on Android) instead of an alert.
	Sheet bool
	// Cancelable lets Escape, back and a press outside close it without
	// an action, answered with -1.
	Cancelable bool
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
// The window starts with nsgo.WindowMaterial's and changes it with
// WindowSetMaterial; Windows shows one only in a window started with
// one. Env.WindowMaterial reports the one the window actually shows.
// What the app draws over the material decides how much shows: a view
// with an opaque background hides it, and one with no background, or a
// translucent one, lets it through.
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
	// WindowSetFrame moves and sizes the window (Frame).
	WindowSetFrame
	// WindowSetLevel puts the window at Level.
	WindowSetLevel
	// WindowOrderFront brings the window to the front of its level, showing it, and activates the app; a panel without the focus (SurfaceConfig.NoActivate) comes to the front alone.
	WindowOrderFront
	// WindowSetMenuBar says when Windows and Linux show the menu bar
	// (MenuBar).
	WindowSetMenuBar
	// WindowSetControls places or hides the window controls of a hidden
	// title bar (Controls).
	WindowSetControls
	// WindowSetMaterial puts Material behind the window's content.
	WindowSetMaterial
	// WindowSetConstraints limits the window's size and what the user may
	// do to it (Constraints).
	WindowSetConstraints
	// WindowShow shows a hidden window without activating the app; a panel shows beside AnchorBounds when Placement is set, else under tray icon Anchor when that is set, and takes the keyboard unless SurfaceConfig.NoActivate keeps it from it.
	WindowShow
	// WindowHide hides the window, keeping it and its views.
	WindowHide
	// WindowDestroy closes an opened surface's window for good: its tree
	// is gone.
	WindowDestroy
	// WindowSetAnchor places a panel beside AnchorBounds on Placement's side, moving it at once if it shows, and keeps the placement for later shows; it changes neither whether it shows nor the focus.
	WindowSetAnchor
	// WindowSetWorkspaces puts the window on the workspaces Workspaces says.
	WindowSetWorkspaces
	// WindowSetPointerEvents says where the window takes the pointer (PointerEvents).
	WindowSetPointerEvents
	// WindowSetTransparent makes a window transparent when On, without a background or frame, or gives it back both (SurfaceConfig.Transparent).
	WindowSetTransparent
	// WindowSetShadow gives a window the system's shadow when On, or takes it away (SurfaceConfig.NoShadow).
	WindowSetShadow
	// WindowSetSkipTaskbar keeps a window off the taskbar when On, or puts it back (SurfaceConfig.SkipTaskbar).
	WindowSetSkipTaskbar
	// WindowFitSize sizes a panel's content to Frame's Width and Height, the size its content takes (core.PanelOptions.FitContent), keeping the edge it is placed by: one that shows stays beside the rectangle or under the tray icon it was placed at, else keeps its top edge. It changes neither whether it shows nor the focus.
	WindowFitSize
	// WindowSetFullScreen enters or leaves full screen (On, DisplayID).
	WindowSetFullScreen
)

// SurfaceKind is what an opened surface shows in.
type SurfaceKind uint8

const (
	// SurfaceWindow is a window of the platform's own chrome.
	SurfaceWindow SurfaceKind = iota
	// SurfacePanel is a borderless window that floats over the others, which takes the keyboard without activating the app where the platform can: a tray icon's or a launcher's; or, with NoActivate, never takes it.
	SurfacePanel
	// SurfaceTrayIcon is the view a tray icon draws itself with
	// (TrayConfig.View), in the macOS menu bar or rasterized on Windows.
	SurfaceTrayIcon
	// SurfaceOverlay is a window over other apps on Android
	// (mobile.OpenOverlay): it takes touches but not the focus unless
	// Focusable, none when Passive, and hides from screen readers unless
	// Accessible. The platform reports the frame it applied in its
	// environment's WindowFrame, and ends it with EventWindowEnded.
	SurfaceOverlay
	// SurfaceScene is a scene the platform connected beyond the app's first, as iPadOS connects one for each window the user opens (EventOpenScene): it shows the app's main view, and the platform ends it with EventWindowEnded as the system discards the scene.
	SurfaceScene
)

// SurfaceConfig is a window, a panel or a tray icon's view the app opens
// (ItemOpenSurface).
type SurfaceConfig struct {
	// ID numbers the surface's tree, which the platform attaches.
	ID   uint32
	Kind SurfaceKind
	// Parent is the ID of the window this is a child of, or zero.
	Parent uint32
	// Title is a window's; empty is the app's name.
	Title string
	// Width and Height are the content's size in points; zero is the
	// platform's default.
	Width, Height Points
	// Frame, when it moves, places a window instead of centering it,
	// kept on a screen (WindowFrame.OnScreen).
	Frame WindowFrame
	// Hidden opens it without showing it, as panels open.
	Hidden bool
	// Focusable, Passive and Accessible are a SurfaceOverlay's input modes: it takes the focus as well as touches, lets touches through to the apps below instead, or shows its content to screen readers.
	Focusable, Passive, Accessible bool
	// Material is the material behind the content (WindowSetMaterial).
	Material WindowMaterial
	// Constraints limit a window (WindowSetConstraints).
	Constraints WindowConstraints
	// HideOnBlur hides a panel as it loses the focus, reported with
	// EventWindowHidden.
	HideOnBlur bool
	// NoActivate keeps a panel from ever taking the keyboard or activating the app, shown, focused or clicked.
	NoActivate bool
	// Dock attaches a panel to a display's usable edge instead of an anchor rectangle.
	Dock PanelDock
	// PointerEvents is where a window or a panel takes the pointer (WindowSetPointerEvents).
	PointerEvents WindowPointerEvents
	// Transparent opens a window without a background or frame, showing what is behind it wherever the app draws nothing (WindowSetTransparent), or has a panel draw nothing of its own, no background, frame or shadow, so only what the app draws shows; the platform reports whether either is so in Environment.WindowTransparent.
	Transparent bool
	// NoShadow opens a window without the system's shadow (WindowSetShadow).
	NoShadow bool
	// SkipTaskbar keeps a window off the Windows taskbar and Alt+Tab, the X11 task list, and macOS's Window menu and Cmd+` (WindowSetSkipTaskbar).
	SkipTaskbar bool
	// Tray is the tray icon a SurfaceTrayIcon draws.
	Tray uint32
	// Scene is the platform's number for the scene a SurfaceScene shows in, EventOpenScene's Count.
	Scene uint32
	// Workspaces are the workspaces a window shows on (WindowSetWorkspaces).
	Workspaces WindowWorkspaces
	// Main is set for the main window, which SetMainView opens: the window a test host's model follows.
	Main bool
}

// TrayConfig is a tray icon (ItemTray).
type TrayConfig struct {
	ID uint32
	// Icon is the icon as PNG, drawn in the menu bar's colors when
	// Template, on macOS.
	Icon     []byte
	Template bool
	// Title is text beside the icon, on macOS.
	Title   string
	Tooltip string
	// View, when set, is the SurfaceTrayIcon the icon draws itself with.
	View uint32
	// Removed takes the icon away.
	Removed bool
}

// WindowCommand is an ItemWindow's request.
type WindowCommand struct {
	Action   WindowAction
	TitleBar TitleBarStyle
	// Height is the custom title bar's height, in which the platform
	// centers its window controls; 0 is the platform's.
	Height   Points
	Frame    WindowFrame
	Level    WindowLevel
	MenuBar  MenuBarMode
	Controls WindowControls
	Material WindowMaterial
	// Constraints are what WindowSetConstraints sets.
	Constraints WindowConstraints
	// Anchor is the tray icon WindowShow shows a panel under.
	Anchor uint32
	// AnchorBounds is the rectangle, in the screen coordinates of DisplayInfo, that WindowShow and WindowSetAnchor place a panel beside, on Placement's side; WindowPlaceDefault places none.
	AnchorBounds Bounds
	Placement    WindowPlacement
	// Workspaces are what WindowSetWorkspaces sets.
	Workspaces WindowWorkspaces
	// PointerEvents is what WindowSetPointerEvents sets.
	PointerEvents WindowPointerEvents
	// DisplayID is the desktop display WindowSetFullScreen enters on. Empty uses the window's current display.
	DisplayID string
	// On is what WindowSetTransparent, WindowSetShadow, WindowSetSkipTaskbar and WindowSetFullScreen set.
	On bool
}

// WindowControlsStyle chooses the appearance of caption buttons a desktop backend draws in a hidden title bar.
type WindowControlsStyle uint8

const (
	// WindowControlsDefault keeps NSGo's compact caption buttons.
	WindowControlsDefault WindowControlsStyle = iota
	// WindowControlsSystem uses the host desktop's full-width caption buttons.
	WindowControlsSystem
)

// WindowControls is how a hidden title bar shows the system's window
// controls: macOS's traffic lights and the caption buttons NSGo draws on
// Windows. Linux draws none.
type WindowControls struct {
	// Hidden takes the controls away for the app to draw its own, which
	// call MinimizeWindow, ToggleMaximize and CloseWindow; Env.TitleBar
	// then reports no width at its ends.
	Hidden bool
	// Style chooses Windows' caption-button geometry and colors. It is ignored on macOS and Linux.
	Style WindowControlsStyle
	// TrafficLights puts the top left corner of macOS's close button
	// this far from the window's top left corner, in points, the other
	// two after it. A zero X keeps them 16 points in, and a zero Y
	// centers them in the title bar.
	TrafficLights Point
	// Offset moves the traffic lights this far from where TrafficLights
	// puts them, so they follow what moves beside them without the app
	// knowing their size.
	Offset Point
}

// WindowFrame is where a window moves and how large it becomes, in
// points. X and Y place its top left corner from the main screen's top
// left, y growing down, on every platform; without Move the corner stays
// where it is, so a window that changes height keeps its top edge.
// Width and Height are its outer size, borders and title bar included
// (the default size GTK keeps on Linux); 0 keeps the current one.
type WindowFrame struct {
	X, Y          Points
	Move          bool
	Width, Height Points
	// Animated eases frames of visible panels on macOS and Windows, unless
	// reduced motion is on. Other macOS windows use AppKit's resize animation.
	Animated bool
}

// The part of a window's top edge a screen must show for the user to
// reach it and drag it back: its title bar's height, and this much wide.
const (
	reachableHeight Points = 32
	reachableWidth  Points = 100
)

// OnScreen returns f when one of screens, the usable areas of the
// connected screens in f's coordinates, shows enough of its top edge to
// drag it, as a screen that went away does not. Else f shrinks to fit
// the first screen and centers on it.
func (f WindowFrame) OnScreen(screens []Bounds) WindowFrame {
	if len(screens) == 0 {
		return f
	}
	for _, s := range screens {
		wide := min(f.X+f.Width, s.Origin.X+s.Size.Width) - max(f.X, s.Origin.X)
		tall := min(f.Y+reachableHeight, s.Origin.Y+s.Size.Height) - max(f.Y, s.Origin.Y)
		if wide >= min(reachableWidth, f.Width) && tall >= reachableHeight {
			return f
		}
	}
	s := screens[0]
	f.Width, f.Height = min(f.Width, s.Size.Width), min(f.Height, s.Size.Height)
	f.X = s.Origin.X + (s.Size.Width-f.Width)/2
	f.Y = s.Origin.Y + (s.Size.Height-f.Height)/2
	return f
}

// Within returns f kept wholly in area, as an overlay keeps to the screen's usable area: no larger than it, and moved in where it pokes out; one that does not move centers on it.
func (f WindowFrame) Within(area Bounds) WindowFrame {
	f.Width, f.Height = min(max(f.Width, 0), area.Size.Width), min(max(f.Height, 0), area.Size.Height)
	if !f.Move {
		f.X = area.Origin.X + (area.Size.Width-f.Width)/2
		f.Y = area.Origin.Y + (area.Size.Height-f.Height)/2
		return f
	}
	f.X = min(max(f.X, area.Origin.X), area.Origin.X+area.Size.Width-f.Width)
	f.Y = min(max(f.Y, area.Origin.Y), area.Origin.Y+area.Size.Height-f.Height)
	return f
}

// WindowConstraints limit a window's content size, in points, and what
// the user may do to it; the zero value limits nothing. A size of 0 has
// no limit, and a maximum under the minimum takes the minimum. Full
// screen goes past the sizes.
type WindowConstraints struct {
	MinWidth, MinHeight Points
	MaxWidth, MaxHeight Points
	// NoResize keeps the user from resizing the window, and so from
	// maximizing it; WindowSetFrame still sizes it.
	NoResize bool
	// NoMinimize keeps the window from minimizing.
	NoMinimize bool
	// NoMaximize keeps the window from maximizing, and on macOS, where
	// the same button goes full screen, from full screen.
	NoMaximize bool
}

// Normalized returns c with its negative sizes 0, its maximums at least
// its minimums and NoResize implying NoMaximize.
func (c WindowConstraints) Normalized() WindowConstraints {
	c.NoMaximize = c.NoMaximize || c.NoResize
	c.MinWidth, c.MinHeight = max(c.MinWidth, 0), max(c.MinHeight, 0)
	if c.MaxWidth = max(c.MaxWidth, 0); c.MaxWidth > 0 {
		c.MaxWidth = max(c.MaxWidth, c.MinWidth)
	}
	if c.MaxHeight = max(c.MaxHeight, 0); c.MaxHeight > 0 {
		c.MaxHeight = max(c.MaxHeight, c.MinHeight)
	}
	return c
}

// Clamp returns size within c's sizes, which c.Normalized returned.
func (c WindowConstraints) Clamp(size Size) Size {
	size.Width, size.Height = max(size.Width, c.MinWidth), max(size.Height, c.MinHeight)
	if c.MaxWidth > 0 {
		size.Width = min(size.Width, c.MaxWidth)
	}
	if c.MaxHeight > 0 {
		size.Height = min(size.Height, c.MaxHeight)
	}
	return size
}

// WindowLevel is where a window stacks among the others.
type WindowLevel uint8

const (
	// WindowLevelNormal is the level of ordinary windows.
	WindowLevelNormal WindowLevel = iota
	// WindowLevelFloating stays above normal windows, as a palette does.
	WindowLevelFloating
	// WindowLevelDesktop sits with the desktop's icons on macOS, on every space and out of Mission Control; Windows puts the window at the bottom of the others, where activating it raises it again, and X11 below the others (_NET_WM_STATE_BELOW).
	WindowLevelDesktop
)

// WindowWorkspaces are the workspaces a window shows on: macOS's Spaces, the virtual desktops of Windows and Linux.
type WindowWorkspaces uint8

const (
	// WindowWorkspacesCurrent keeps the window on the workspace it opened on, as windows are.
	WindowWorkspacesCurrent WindowWorkspaces = iota
	// WindowWorkspacesAll shows it on every workspace, and over other apps' full screen spaces on macOS.
	WindowWorkspacesAll
)

// WindowPointerEvents is where a desktop window takes the pointer, by CSS's pointer-events names: everywhere, nowhere, or only where it draws something, the pointer going through to what is below it elsewhere.
type WindowPointerEvents uint8

const (
	// WindowPointerAuto takes the pointer everywhere in the window.
	WindowPointerAuto WindowPointerEvents = iota
	// WindowPointerNone takes it nowhere: presses, scrolls and hovering go through to what is below.
	WindowPointerNone
	// WindowPointerPainted takes it only where a view under it is Painted.
	WindowPointerPainted
)

// pointerEvents are the events of a view that hears the pointer.
const pointerEvents = 1<<EventPress | 1<<EventPressDown | 1<<EventPressUp | 1<<EventLongPress | 1<<EventMouseEnter |
	1<<EventMouseLeave | 1<<EventMouseMove | 1<<EventContextMenu | 1<<EventPan | 1<<EventPinch | 1<<EventRotation |
	1<<EventWheel | 1<<EventTouch | 1<<EventFileDragEnter | 1<<EventFileDragLeave | 1<<EventFileDrop

// Painted reports whether a view takes the pointer in a window that takes it only where it draws something (WindowPointerPainted): a view that hears the pointer or drags the window, of a kind that draws its whole frame, such as a text, an image or an input, or a container whose style fills or edges it. Shadows and outlines, drawn around the frame, do not count. macOS and Windows ask it of the innermost view the pointer reaches, by its pointer events, and of the views that view is in; Linux, which gives its surface an input region rather than following the pointer, of every view, by its bounds within what clips it.
func Painted(kind ViewKind, style *ViewStyle, events EventMask, drag WindowDrag) bool {
	if events&pointerEvents != 0 || drag == WindowDragOn {
		return true
	}
	if style.Hidden || style.Opacity <= 0 {
		return false
	}
	switch kind {
	case KindView, KindModal, KindScroll:
	default:
		return true
	}
	if style.Background&0xff != 0 || style.Glass != GlassNone {
		return true
	}
	if style.Borders == (Edges{}) {
		if style.BorderWidth > 0 && style.BorderColor&0xff != 0 && style.BorderStyle != BorderNone && style.BorderStyle != BorderHidden {
			return true
		}
	} else if style.BorderStyle != BorderNone && style.BorderStyle != BorderHidden {
		b, c := style.Borders, style.BorderColors
		if b.Top > 0 && c.Top&0xff != 0 || b.Right > 0 && c.Right&0xff != 0 || b.Bottom > 0 && c.Bottom&0xff != 0 || b.Left > 0 && c.Left&0xff != 0 {
			return true
		}
	}
	e := style.Effects
	return e != nil && (len(e.Gradients) > 0 || len(e.BackdropFilters) > 0)
}

// MenuBarMode says when Windows and Linux show the menu bar the window
// draws for the application menu. macOS's menu bar is the system's and
// always shows.
type MenuBarMode uint8

const (
	// MenuBarShown always shows it, above the app.
	MenuBarShown MenuBarMode = iota
	// MenuBarAutoHide shows it once Alt is pressed and released alone,
	// until Alt alone, a press elsewhere or a chosen item hides it again,
	// as Windows and GNOME apps do. F10 and Alt with a menu's mnemonic
	// still open a menu, and the bar shows while it is open.
	MenuBarAutoHide
)

// Appearance is the light or dark appearance an app asks for, or the
// system's.
type Appearance uint8

const (
	// AppearanceSystem follows the system's appearance.
	AppearanceSystem Appearance = iota
	AppearanceLight
	AppearanceDark
)

// ClipboardRead is what an ItemReadClipboard asks of the clipboard.
type ClipboardRead uint8

const (
	// ClipboardReadText reads its plain text, "" when it has none.
	ClipboardReadText ClipboardRead = iota
	// ClipboardReadHasText reports whether it holds text, without reading it,
	// so iOS shows no paste notice.
	ClipboardReadHasText
	// ClipboardReadImage reads its image as a PNG.
	ClipboardReadImage
	// ClipboardReadHasImage reports whether it holds an image, without
	// reading it.
	ClipboardReadHasImage
	// ClipboardReadFiles reads the file: URLs of the local files it holds,
	// such as files copied in a file manager.
	ClipboardReadFiles
	// ClipboardReadItems reads the clipboard's native byte representations.
	ClipboardReadItems
	// ClipboardReadChangeCount reads the platform's clipboard sequence number.
	ClipboardReadChangeCount
	// ClipboardReadFormats reads native format names without copying payloads.
	ClipboardReadFormats
)

// MaxClipboardImage is the most bytes of a PNG a clipboard read returns:
// a larger image is left out, since the app's memory may not hold it.
const MaxClipboardImage = 48 << 20

// ClipboardImage is an image read from the clipboard, or why the one it
// holds was not.
type ClipboardImage struct {
	// PNG is the image encoded as a PNG, which an ImageSource's Data
	// shows.
	PNG []byte
	// Size is its size in pixels.
	Size Size
	// TooLarge is the bytes of a PNG over MaxClipboardImage, left out of
	// PNG.
	TooLarge int
	// Err says why the image the clipboard holds could not be read.
	Err string
}

// ReadClipboardImage is a platform's answer to a ClipboardReadImage from
// what converting the clipboard's image to a PNG gave: nil when there was
// nothing to convert.
func ReadClipboardImage(png []byte, size Size, err error) *ClipboardImage {
	switch {
	case err != nil:
		return &ClipboardImage{Err: err.Error()}
	case len(png) > MaxClipboardImage:
		return &ClipboardImage{Size: size, TooLarge: len(png)}
	case len(png) == 0:
		return nil
	}
	return &ClipboardImage{PNG: png, Size: size}
}

// ClipboardPNG is ReadClipboardImage of a PNG whose size it reads, which
// fails for data that is no PNG.
func ClipboardPNG(data []byte) *ClipboardImage {
	if len(data) == 0 {
		return ReadClipboardImage(nil, Size{}, errors.New("no PNG"))
	}
	size, err := pngSize(data)
	return ReadClipboardImage(data, size, err)
}

// pngSize reads a PNG's size from its header, so reading one links no
// decoder.
func pngSize(data []byte) (Size, error) {
	if len(data) < 24 || SniffImage(data) != "image/png" || string(data[12:16]) != "IHDR" {
		return Size{}, errors.New("no PNG")
	}
	w, h := binary.BigEndian.Uint32(data[16:]), binary.BigEndian.Uint32(data[20:])
	return Size{Width: Points(w), Height: Points(h)}, nil
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
	// MenuPasteAndMatchStyle pastes the clipboard's text without its style, and MenuDelete removes the selection, in the focused text field or web view.
	MenuPasteAndMatchStyle
	MenuDelete
	// MenuStartSpeaking reads the focused text field's selection aloud and MenuStopSpeaking stops it, on macOS and, where they speak, Windows and Linux. MenuBringAllToFront is macOS's.
	MenuStartSpeaking
	MenuStopSpeaking
	MenuBringAllToFront
	// MenuServices, MenuWindowMenu and MenuHelpMenu mark an item holding a submenu as macOS's Services menu, which AppKit fills, its Window menu, which lists the app's windows, and its Help menu, which gets the help search field, whatever their titles. MenuServices is macOS's; elsewhere the other two hold ordinary submenus, and are left out with no items.
	MenuServices
	MenuWindowMenu
	MenuHelpMenu
)

var menuRoleLabels = [...]string{"", "About", "Hide", "Hide Others", "Show All", "Quit", "Undo", "Redo",
	"Cut", "Copy", "Paste", "Select All", "Minimize", "Zoom", "Toggle Full Screen", "Close",
	"Paste and Match Style", "Delete", "Start Speaking", "Stop Speaking", "Bring All to Front", "Services", "Window", "Help"}

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
	case MenuPasteAndMatchStyle:
		return pick("alt+shift+meta+v", "")
	}
	return ""
}

// EditsText reports whether r is an editing action whose key the focused text field handles itself, which it then leaves to the field. MenuPasteAndMatchStyle is not: no field handles its key, so the menu does.
func (r MenuRole) EditsText() bool {
	return r >= MenuUndo && r <= MenuSelectAll || r == MenuDelete
}

// HoldsMenu reports whether r marks an item holding a submenu (MenuServices, MenuWindowMenu, MenuHelpMenu), which it is even with no items.
func (r MenuRole) HoldsMenu() bool {
	return r == MenuServices || r == MenuWindowMenu || r == MenuHelpMenu
}

// MenuIcon is an icon shown beside a native menu item on macOS, Windows and Linux. Data holds encoded image bytes (PNG, JPEG, GIF, WebP or SVG), as TrayConfig.Icon takes; Source holds any ImageSource (asset, file or data URI, bytes or Pixels); SVG holds a parsed vector icon such as lucide.Folder(). When more than one is set, SVG takes precedence over Data, and Data over Source. Template draws a monochrome icon in the menu's text color, adapting to light and dark appearance (and selection on macOS); a monochrome SVG icon is treated as a template by default.
type MenuIcon struct {
	// Data is encoded image bytes (PNG, JPEG, GIF, WebP or SVG).
	Data []byte
	// Source is an ImageSource (asset, file or data URI, bytes or Pixels).
	Source ImageSource
	// SVG is a parsed vector icon, such as lucide.Folder().
	SVG *SVG
	// Template draws a monochrome icon in the menu's text color.
	Template bool
}

// Empty reports whether m names no icon.
func (m MenuIcon) Empty() bool {
	return m.SVG == nil && len(m.Data) == 0 && m.Source.Empty()
}

// IsTemplate reports whether m should be tinted with the menu's text color: Template is set, or SVG uses currentColor exclusively.
func (m MenuIcon) IsTemplate() bool {
	return m.Template || (m.SVG != nil && m.SVG.Monochrome())
}

// Equal reports whether m and o specify the same menu icon.
func (m MenuIcon) Equal(o MenuIcon) bool {
	if m.Template != o.Template || m.SVG != o.SVG || !bytes.Equal(m.Data, o.Data) {
		return false
	}
	return cursorSourceEqual(m.Source, o.Source)
}

// Key returns a cache key identifying m's image and template flag, or "" when empty.
func (m MenuIcon) Key() string {
	switch {
	case m.SVG != nil:
		return fmt.Sprintf("svg:%p:t:%v", m.SVG, m.IsTemplate())
	case len(m.Data) > 0:
		return fmt.Sprintf("data:%x:t:%v", Sum256(m.Data), m.Template)
	case !m.Source.Empty():
		return fmt.Sprintf("src:%s:t:%v", m.Source.Key(), m.Template)
	default:
		return ""
	}
}

// Clone returns a copy of m whose byte slices, pixel buffer and variant map do not alias m's.
func (m MenuIcon) Clone() MenuIcon {
	out := m
	if len(m.Data) > 0 {
		out.Data = bytes.Clone(m.Data)
	}
	if len(m.Source.Data) > 0 {
		out.Source.Data = bytes.Clone(m.Source.Data)
	}
	if m.Source.Pixels != nil {
		out.Source.Pixels = CopyPixels(m.Source.Pixels)
	}
	if len(m.Source.Variants) > 0 {
		out.Source.Variants = maps.Clone(m.Source.Variants)
	}
	return out
}

// MenuEntry is a MenuItem as an ItemMenu carries it: the item's handler
// replaced by an ID the platform reports with EventMenu (0: none), its
// shortcut normalized and its role's label filled in. A role item without
// a shortcut takes its role's, which the platform fills in.
type MenuEntry struct {
	ID        uint32
	Label     string
	Icon      MenuIcon
	Shortcut  string
	Role      MenuRole
	Disabled  bool
	Checkable bool
	Checked   bool
	Radio     bool
	Separator bool
	Items     []MenuEntry
	// NotInText leaves the shortcut to a text input or a web view with
	// the keyboard focus: the key goes to it rather than choosing the
	// item.
	NotInText bool
}

// KeepAwake is a platform power assertion requested by ItemKeepAwake.
type KeepAwake struct {
	Reason  string
	Display bool
}

// TaskProgressState is the state of an app task's platform progress bar.
type TaskProgressState uint8

const (
	TaskProgressNone TaskProgressState = iota
	TaskProgressNormal
	TaskProgressIndeterminate
	TaskProgressPaused
	TaskProgressError
)

// TaskProgress is the progress shown on an app icon or taskbar button.
type TaskProgress struct {
	State TaskProgressState
	Value float64
	// Call numbers the SetTaskProgress call that sent it, from 1 in the order the app made the calls. A platform that shows one progress for several surfaces, as the Dock tile, a Linux launcher entry and the progress Windows windows show from surfaces without a button, ignores a call older than the one it shows: the surfaces commit in their own order, not in the order of the calls.
	Call uint64
}

// BarFraction is the fraction a bar drawn for p fills where the platform has no indeterminate or cleared state of its own, as the Dock tile and Unity's LauncherEntry: none for no progress, half for an indeterminate task, which a snapshot cannot animate, and Value otherwise.
func (p TaskProgress) BarFraction() float64 {
	switch p.State {
	case TaskProgressNone:
		return 0
	case TaskProgressIndeterminate:
		return .5
	}
	return p.Value
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

// MemoryClipboard is a clipboard of a test's own: it takes the clipboard
// items an app sends and answers its reads, so the machine's clipboard
// is never touched.
type MemoryClipboard struct {
	Text        string
	Image       []byte // PNG
	Files       []string
	Items       []ClipboardItem
	ChangeCount int64
}

// Take applies it when it is a clipboard item and reports whether it is:
// a write changes the clipboard, and a read returns the event that
// answers it, with answer true.
func (c *MemoryClipboard) Take(it *MountItem) (ev Event, answer, taken bool) {
	switch it.Kind {
	case ItemSetClipboard:
		c.Text, c.Image, c.Files = it.Value, nil, nil
		c.Items = []ClipboardItem{{Representations: []ClipboardRepresentation{{Type: "public.utf8-plain-text", Data: []byte(it.Value)}}}}
		c.ChangeCount++
	case ItemSetClipboardImage:
		data, _ := it.Data.([]byte)
		c.Text, c.Image, c.Files = "", data, nil
		c.Items = nil
		c.ChangeCount++
	case ItemSetClipboardItems:
		items, _ := it.Data.([]ClipboardItem)
		c.setItems(items)
		if it.Count != 0 {
			return Event{Kind: EventClipboard, Count: it.Count, Data: ClipboardResult{}}, true, true
		}
	case ItemReadClipboard:
		ev = Event{Kind: EventClipboard, Count: it.Count}
		switch it.Clipboard {
		case ClipboardReadText:
			ev.Text = c.Text
		case ClipboardReadHasText:
			ev.Finished = c.Text != ""
		case ClipboardReadImage:
			if len(c.Image) > 0 {
				ev.Data = ClipboardPNG(c.Image)
			}
		case ClipboardReadHasImage:
			ev.Finished = len(c.Image) > 0
		case ClipboardReadFiles:
			ev.URIs = append([]string(nil), c.Files...)
		case ClipboardReadItems:
			items := c.snapshot()
			err := ValidateClipboardItems(items)
			if err == nil {
				items = CloneClipboardItems(items)
			} else {
				items = nil
			}
			ev.Data = ClipboardResult{Items: items, Err: err}
		case ClipboardReadChangeCount:
			ev.Data = ClipboardResult{ChangeCount: c.ChangeCount}
		case ClipboardReadFormats:
			var formats []string
			for _, item := range c.snapshot() {
				for _, rep := range item.Representations {
					formats = append(formats, rep.Type)
				}
			}
			ev.Data = ClipboardResult{Formats: formats}
		}
		return ev, true, true
	default:
		return Event{}, false, false
	}
	return Event{}, false, true
}
