//go:build windows

package windows

import (
	"runtime"
	"unicode"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/menutext"
)

// The window's chrome, as NSH's Windows backend has it:
//
//   - The hidden title bar: WM_NCCALCSIZE gives the client area the
//     window's whole height (the side and bottom resize borders stay
//     non-client), WM_NCHITTEST answers the top resize border, the caption
//     buttons (HTMINBUTTON, HTMAXBUTTON, HTCLOSE, so Windows 11 shows snap
//     layouts) and the views that drag the window (HTCAPTION: the system
//     moves the window, maximizes it on a double click and opens its menu
//     on a right click; with the system's title bar too). The caption
//     buttons are drawn over the app in Windows 11's style, and their
//     non-client clicks handled here, never by DefWindowProc, which would
//     draw Windows 95's.
//   - Full screen: a borderless window the size of its monitor.
//   - The menu bar: a Win32 menu (SetMenu), not shown while the title bar
//     is hidden or in full screen; its shortcuts are matched on key downs
//     after the app's own, the editing roles excepted, which the focused
//     field handles itself.

var (
	setWindowPos             = user32.NewProc("SetWindowPos")
	isZoomed                 = user32.NewProc("IsZoomed")
	getDpiForWindow          = user32.NewProc("GetDpiForWindow")
	getSystemMetricsForDpi   = user32.NewProc("GetSystemMetricsForDpi")
	trackMouseEvent          = user32.NewProc("TrackMouseEvent")
	getWindowPlacement       = user32.NewProc("GetWindowPlacement")
	setWindowPlacement       = user32.NewProc("SetWindowPlacement")
	monitorFromWindow        = user32.NewProc("MonitorFromWindow")
	monitorFromRect          = user32.NewProc("MonitorFromRect")
	getMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	createMenu               = user32.NewProc("CreateMenu")
	createPopupMenu          = user32.NewProc("CreatePopupMenu")
	insertMenuItemW          = user32.NewProc("InsertMenuItemW")
	setMenu                  = user32.NewProc("SetMenu")
	destroyMenu              = user32.NewProc("DestroyMenu")
	deleteMenu               = user32.NewProc("DeleteMenu")
	adjustWindowRectEx       = user32.NewProc("AdjustWindowRectEx")
	adjustWindowRectExForDpi = user32.NewProc("AdjustWindowRectExForDpi")
	drawMenuBar              = user32.NewProc("DrawMenuBar")
	dwmGetWindowAttribute    = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	wmClose           = 0x0010
	wmGetMinMaxInfo   = 0x0024
	wmNCCalcSize      = 0x0083
	wmNCHitTest       = 0x0084
	wmNCActivate      = 0x0086
	wmNCMouseMove     = 0x00A0
	wmNCLButtonDown   = 0x00A1
	wmNCLButtonUp     = 0x00A2
	wmNCLButtonDblClk = 0x00A3
	wmNCMouseLeave    = 0x02A2

	htClient      = 1
	htCaption     = 2
	htMinButton   = 8
	htMaxButton   = 9
	htTop         = 12
	htTopLeft     = 13
	htLeft        = 10
	htRight       = 11
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
	htClose       = 20

	smCyFrame                        = 33
	smCxPaddedBorder                 = 92
	dwmwaVisibleFrameBorderThickness = 37

	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpFrameChanged  = 0x0020
	swpNoOwnerZOrder = 0x0200

	swMaximize = 3
	swMinimize = 6
	swRestore  = 9
	swShowNA   = 8 // SW_SHOWNA: shown in its state, not activated

	wsPopup            = 0x80000000
	monitorDefaultNear = 2
	tmeLeave           = 0x02
	tmeNonClient       = 0x10

	mfGrayed     = 0x0001
	mfChecked    = 0x0008
	mfPopup      = 0x0010
	mfByPosition = 0x0400

	miimState   = 0x0001
	miimID      = 0x0002
	miimSubMenu = 0x0004
	miimString  = 0x0040
	miimFType   = 0x0100
)

// winMenuItemInfo is Win32's MENUITEMINFOW, used with InsertMenuItemW so
// MFT_RIGHTORDER and MFT_RADIOCHECK are passed through fType officially.
type winMenuItemInfo struct {
	cbSize, fMask, fType, fState, wID    uint32
	hSubMenu, hbmpChecked, hbmpUnchecked uintptr
	dwItemData, dwTypeData               uintptr
	cch                                  uint32
	hbmpItem                             uintptr
}

// The caption buttons: 32 point squares 4 apart, their
// glyphs 10, as Windows' own are at 96 DPI, centered in the app's bar
// and 8 points in from the right edge; the bar is 32 tall unless the app
// says otherwise.
const (
	defaultCaptionHeight driver.Points = 32
	captionButtonSize    driver.Points = 32
	captionButtonGap     driver.Points = 4
	captionMargin        driver.Points = 8
	captionGlyphSize     driver.Points = 10
	// captionRight is how far the buttons reach in from the right edge.
	captionRight = captionMargin + 3*captionButtonSize + 2*captionButtonGap
	// systemCaptionButtonWidth is the width of Windows 11's caption buttons, full-height rectangles at the window's edge.
	systemCaptionButtonWidth driver.Points = 46
)

type ncCalcSizeParams struct {
	Rects [3]rect
	Pos   uintptr
}

type trackMouse struct {
	Size      uint32
	Flags     uint32
	Track     uintptr
	HoverTime uint32
}

// minMaxInfo is MINMAXINFO.
type minMaxInfo struct {
	Reserved     point
	MaxSize      point
	MaxPosition  point
	MinTrackSize point
	MaxTrackSize point
}

type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	MinPosition            point
	MaxPosition            point
	NormalPosition         rect
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

// chrome is what the window keeps of its chrome.
type chrome struct {
	titleBar driver.TitleBarStyle
	height   driver.Points // the app's title bar height; 0 for the default
	controls driver.WindowControlsStyle
	// noButtons is set while the app draws its own caption buttons.
	noButtons bool
	hover     uint32 // the caption button under the mouse (HT*), 0 for none
	pressed   uint32
	tracking  bool // the mouse is tracked for WM_NCMOUSELEAVE
	inactive  bool

	// constraints are the app's limits on the window.
	constraints driver.WindowConstraints

	fullScreen        bool
	fullScreenMonitor uintptr
	savedStyle        uintptr
	savedPlace        windowPlacement

	menu    []driver.MenuEntry // nil: no menu bar, Windows' default
	bar     uintptr
	tree    []winMenuItem      // the items in bar, which own its HBITMAPs, for updating it in place
	entries []menuRef          // the bar's commands; a command's id is its index + 1
	keys    map[string]menuRef // the menu's shortcuts
	// menuBar says when the bar shows; with MenuBarAutoHide, revealed is
	// set while Alt alone showed it, menuOpen while a key opened a menu,
	// and altAlone while Alt is down with no other key.
	menuBar                      driver.MenuBarMode
	revealed, menuOpen, altAlone bool
	// barExtra is how many pixels the window grew for the menu bar, so its content keeps its height (fitMenuBar).
	barExtra int32
}

// hidden reports whether the title bar is hidden now: full screen has
// none.
func (w *window) hidden() bool {
	return w.chrome.titleBar == driver.TitleBarHidden && !w.chrome.fullScreen && !w.transparent
}

// captionHeight is the app's bar height, and at least Windows' own.
func (w *window) captionHeight() driver.Points {
	return max(w.chrome.height, defaultCaptionHeight)
}

// titleBarArea is what the hidden title bar's window controls cover.
func (w *window) titleBarArea() driver.Edges {
	if !w.hidden() {
		return driver.Edges{}
	}
	if w.chrome.noButtons {
		return driver.Edges{Top: w.captionHeight()}
	}
	return driver.Edges{Top: w.captionHeight(), Right: w.captionRight()}
}

// captionRight is how far the caption buttons reach in from the right edge in the window's style.
func (w *window) captionRight() driver.Points {
	if w.chrome.controls == driver.WindowControlsSystem {
		return 3 * systemCaptionButtonWidth
	}
	return captionRight
}

// buttons reports whether the caption buttons show: in a hidden title
// bar the app did not take them from.
func (w *window) buttons() bool { return w.hidden() && !w.chrome.noButtons }

func (w *window) zoomed() bool {
	r, _, _ := isZoomed.Call(w.hwnd)
	return r != 0
}

// frameThickness is the resize border in pixels, also what a maximized
// window hangs off its monitor by.
func (w *window) frameThickness() int32 {
	dpi, _, _ := getDpiForWindow.Call(w.hwnd)
	if dpi == 0 {
		dpi = uintptr(w.scale * 96)
	}
	frame, _, _ := getSystemMetricsForDpi.Call(smCyFrame, dpi)
	padded, _, _ := getSystemMetricsForDpi.Call(smCxPaddedBorder, dpi)
	return int32(frame) + int32(padded)
}

// topBorder is the visible border Windows 11 draws along a window's top
// edge, in pixels.
func (w *window) topBorder() int32 {
	if dwmGetWindowAttribute.Find() != nil {
		return 0
	}
	var thickness uint32
	if r, _, _ := dwmGetWindowAttribute.Call(w.hwnd, dwmwaVisibleFrameBorderThickness,
		uintptr(unsafe.Pointer(&thickness)), 4); r != 0 {
		return 0
	}
	return int32(min(thickness, 8))
}

// frameChanged has the window compute its client area again.
func (w *window) frameChanged() {
	// Whether the window is shadowless follows its title bar, full screen and transparency.
	w.applyBare()
	setWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0, swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate)
}

// chromeChanged follows a change of the title bar or full screen: the
// menu bar, the area reported to the app and the drawing.
func (w *window) chromeChanged() {
	w.showMenuBar()
	w.updateEnvironment()
	w.redrawAll()
}

// windowCommand carries out an ItemWindow.
func (w *window) windowCommand(c *driver.WindowCommand) {
	w.showFirst.command(c.Action)
	switch c.Action {
	case driver.WindowDestroy:
		if w.testID != 0 {
			delete(w.opts.test.windows, w.testID)
		}
		untrackLiveWindow(w.surfaceID, w)
		w.destroying = true
		w.visible = false
		destroyWindow.Call(w.hwnd)
	case driver.WindowSetTitleBar:
		if c.TitleBar != w.chrome.titleBar || c.Height != w.chrome.height {
			w.chrome.titleBar, w.chrome.height = c.TitleBar, c.Height
			w.frameChanged()
			w.chromeChanged()
		}
	case driver.WindowMinimize:
		w.minimize()
	case driver.WindowToggleMaximize:
		w.toggleMaximize()
	case driver.WindowToggleFullScreen:
		w.toggleFullScreen()
	case driver.WindowSetFullScreen:
		w.setFullScreen(c.On, c.DisplayID)
	case driver.WindowClose:
		w.destroying = true
		w.visible = false
		postMessageW.Call(w.hwnd, wmClose, 0, 0)
	case driver.WindowShow:
		w.visible, w.focused = true, false
		if !w.parentVisible() {
			break
		}
		if w.kind == driver.SurfacePanel {
			w.anchor, w.anchorBounds, w.placement = c.Anchor, c.AnchorBounds, c.Placement
			if w.dock.Edge != driver.WindowPlaceDefault {
				if !w.placeDockedPanel() {
					break
				}
			} else {
				w.placePanel(w.anchor, w.anchorBounds, w.placement)
			}
			w.orderFront()
		} else {
			// Without taking the focus from another app (core.Window.Show).
			showWindow.Call(w.hwnd, swShowNA)
		}
	case driver.WindowHide:
		w.visible = false
		showWindow.Call(w.hwnd, swHide)
	case driver.WindowSetFrame:
		w.setWindowFrame(c.Frame)
	case driver.WindowSetLevel:
		w.setLevel(c.Level)
	case driver.WindowSetWorkspaces:
		// Windows has no public API that shows a window on every virtual desktop: IVirtualDesktopManager only moves one to another, so it stays on its own, and the environment leaves CanSetWindowWorkspaces false.
	case driver.WindowOrderFront:
		w.visible, w.focused = true, false
		if !w.parentVisible() {
			break
		}
		if w.kind == driver.SurfacePanel {
			if w.dock.Edge != driver.WindowPlaceDefault {
				if !w.placeDockedPanel() {
					break
				}
			} else {
				w.placePanel(w.anchor, w.anchorBounds, w.placement)
			}
		}
		w.orderFront()
	case driver.WindowSetMenuBar:
		w.setMenuBar(c.MenuBar)
	case driver.WindowSetControls:
		if c.Controls.Hidden != w.chrome.noButtons || c.Controls.Style != w.chrome.controls {
			w.chrome.noButtons = c.Controls.Hidden
			w.chrome.controls = c.Controls.Style
			w.chrome.hover, w.chrome.pressed = 0, 0
			w.chromeChanged()
		}
	case driver.WindowSetMaterial:
		w.setMaterial(c.Material)
	case driver.WindowSetConstraints:
		w.setConstraints(c.Constraints)
	case driver.WindowSetPointerEvents:
		w.setPointerEvents(c.PointerEvents)
	case driver.WindowSetTransparent:
		if w.kind == driver.SurfaceWindow {
			w.setTransparent(c.On)
			w.showMenuBar()
		}
	case driver.WindowSetSkipTaskbar:
		if w.kind == driver.SurfaceWindow && !(w.primary && w.opts.Hidden) {
			w.setSkipTaskbar(c.On)
		}
	case driver.WindowSetShadow:
		if !w.embedded && w.kind == driver.SurfaceWindow && !(w.primary && w.opts.Hidden) {
			w.setShadow(c.On)
		}
	case driver.WindowFitSize:
		if w.kind == driver.SurfacePanel {
			w.fitPanel(c.Frame.Width, c.Frame.Height)
		}
	case driver.WindowSetAnchor:
		if w.kind == driver.SurfacePanel {
			w.anchorBounds, w.placement = c.AnchorBounds, c.Placement
			// Moving a panel that shows changes neither the z-order nor the focus.
			if w.visible && w.placement != driver.WindowPlaceDefault {
				w.placeBeside(w.anchorBounds, w.placement)
			}
		}
	}
}

// constrained returns style with the frame and the buttons the window's constraints leave: a transparent window's popup has no frame to resize by (frameStyle).
func (w *window) constrained(style uintptr) uintptr {
	if w.transparent {
		return w.frameStyle(style)
	}
	return constrainedStyle(style, w.chrome.constraints)
}

// constrainedStyle returns style with the frame and the buttons c leaves.
func constrainedStyle(style uintptr, c driver.WindowConstraints) uintptr {
	style |= wsThickFrame | wsMinimizeBox | wsMaximizeBox
	if c.NoResize {
		style &^= wsThickFrame
	}
	if c.NoMinimize {
		style &^= wsMinimizeBox
	}
	if c.NoMaximize {
		style &^= wsMaximizeBox
	}
	return style
}

// setConstraints limits the window and moves it into its new sizes; in
// full screen they wait for the style it goes back to.
func (w *window) setConstraints(c driver.WindowConstraints) {
	w.chrome.constraints = c
	w.chrome.hover, w.chrome.pressed = 0, 0
	if w.chrome.fullScreen {
		w.chrome.savedStyle = w.constrained(w.chrome.savedStyle)
		return
	}
	style, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlStyle)
	setWindowLongPtrW.Call(w.hwnd, gwlStyle, w.constrained(style))
	w.frameChanged()
	w.redrawAll()
	if w.zoomed() || isIconicWindow(w.hwnd) {
		return
	}
	var outer, client rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	shown := w.point(client.Right, client.Bottom)
	size := driver.Size{Width: shown.X, Height: shown.Y}
	if fit := c.Clamp(size); fit != size {
		setWindowPos.Call(w.hwnd, 0, 0, 0,
			uintptr(outer.Right-outer.Left-client.Right+w.px(fit.Width)),
			uintptr(outer.Bottom-outer.Top-client.Bottom+w.px(fit.Height)),
			swpNoMove|swpNoZOrder|swpNoActivate)
	}
}

// minMaxInfo answers WM_GETMINMAXINFO: the constraints' content sizes
// with what the frame takes. Windows also holds SetWindowPos to these
// sizes, so full screen goes without them, and a minimized window has
// no client area to measure.
func (w *window) minMaxInfo(lParam uintptr) {
	c := &w.chrome.constraints
	if w.chrome.fullScreen || isIconicWindow(w.hwnd) {
		return
	}
	info := *(**minMaxInfo)(unsafe.Pointer(&lParam)) // the message's, valid while it is handled
	var outer, client rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	frameX, frameY := outer.Right-outer.Left-client.Right, outer.Bottom-outer.Top-client.Bottom
	if c.MinWidth > 0 {
		info.MinTrackSize.X = w.px(c.MinWidth) + frameX
	}
	if c.MinHeight > 0 {
		info.MinTrackSize.Y = w.px(c.MinHeight) + frameY
	}
	if c.MaxWidth > 0 {
		info.MaxTrackSize.X = w.px(c.MaxWidth) + frameX
		info.MaxSize.X = min(info.MaxSize.X, info.MaxTrackSize.X)
	}
	if c.MaxHeight > 0 {
		info.MaxTrackSize.Y = w.px(c.MaxHeight) + frameY
		info.MaxSize.Y = min(info.MaxSize.Y, info.MaxTrackSize.Y)
	}
}

func (w *window) minimize() {
	if !w.chrome.constraints.NoMinimize {
		w.showFirst.cancel()
		showWindow.Call(w.hwnd, swMinimize)
	}
}

func (w *window) toggleMaximize() {
	if w.chrome.fullScreen {
		return
	}
	if w.zoomed() {
		w.showFirst.cancel()
		w.visible = true
		showWindow.Call(w.hwnd, swRestore)
	} else if !w.chrome.constraints.NoMaximize {
		w.showFirst.cancel()
		w.visible = true
		showWindow.Call(w.hwnd, swMaximize)
	}
}

// toggleFullScreen makes the window a borderless one covering its
// monitor, or puts it back as it was.
func (w *window) toggleFullScreen() {
	w.setFullScreen(!w.chrome.fullScreen, "")
}

func (w *window) setFullScreen(on bool, displayID string) {
	c := &w.chrome
	if on {
		monitor, _, _ := monitorFromWindow.Call(w.hwnd, monitorDefaultNear)
		if displayID != "" {
			monitor = 0
			for _, display := range monitorLayout() {
				if display.display.ID == displayID {
					monitor = display.monitor
					break
				}
			}
		}
		if monitor == 0 || c.fullScreen && (displayID == "" || c.fullScreenMonitor == monitor) {
			return
		}
		var mi monitorInfo
		mi.Size = uint32(unsafe.Sizeof(mi))
		if r, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); r == 0 {
			return
		}
		if !c.fullScreen {
			c.savedPlace = windowPlacement{}
			c.savedPlace.Length = uint32(unsafe.Sizeof(c.savedPlace))
			if ok, _, _ := getWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&c.savedPlace))); ok == 0 {
				return
			}
			c.savedStyle, _, _ = getWindowLongPtrW.Call(w.hwnd, gwlStyle)
			c.fullScreen = true
			w.showMenuBar()
			setWindowLongPtrW.Call(w.hwnd, gwlStyle, c.savedStyle&^wsOverlappedWindow|wsPopup)
		}
		c.fullScreenMonitor = monitor
		m := mi.Monitor
		setWindowPos.Call(w.hwnd, 0, uintptr(m.Left), uintptr(m.Top), uintptr(m.Right-m.Left), uintptr(m.Bottom-m.Top),
			swpNoOwnerZOrder|swpFrameChanged)
	} else {
		if !c.fullScreen {
			return
		}
		c.fullScreen, c.fullScreenMonitor = false, 0
		setWindowLongPtrW.Call(w.hwnd, gwlStyle, c.savedStyle)
		w.showMenuBar()
		setWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&c.savedPlace)))
		setWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0,
			swpNoMove|swpNoSize|swpNoZOrder|swpNoOwnerZOrder|swpNoActivate|swpFrameChanged)
	}
	if w.engine != nil {
		w.chromeChanged()
	}
}

// ncCalcSize answers WM_NCCALCSIZE with the title bar hidden: the client
// area reaches the window's top.
func (w *window) ncCalcSize(wParam, lParam uintptr) (uintptr, bool) {
	if wParam == 0 || !w.hidden() {
		return 0, false
	}
	if w.shadowless() && !w.zoomed() {
		// The proposed window rectangle is the client area: no non-client frame for DefWindowProc to paint once DWM no longer does.
		return 0, true
	}
	params := *(**ncCalcSizeParams)(unsafe.Pointer(&lParam)) // the message's, valid while it is handled
	top := params.Rects[0].Top
	defWindowProcW.Call(w.hwnd, wmNCCalcSize, wParam, lParam)
	// A maximized window hangs off its monitor by its resize border;
	// another one's top row would be under Windows 11's border.
	if w.zoomed() {
		params.Rects[0].Top = top + w.frameThickness()
	} else {
		params.Rects[0].Top = top + w.topBorder()
	}
	return 0, true
}

// captionAt returns the caption button at p (client points), as its hit
// test code, or 0.
func (w *window) captionAt(p driver.Point) uint32 {
	height := w.captionHeight()
	buttonHeight := captionButtonSize
	top := (height - buttonHeight) / 2
	width := captionButtonSize
	gap := captionButtonGap
	margin := captionMargin
	if w.chrome.controls == driver.WindowControlsSystem {
		buttonHeight, width, gap, margin = height, systemCaptionButtonWidth, 0, 0
		top = 0
	}
	if !w.buttons() || p.Y < top || p.Y >= top+buttonHeight {
		return 0
	}
	var client rect
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	clientWidth := w.point(client.Right, 0).X
	for i, code := range []uint32{htClose, htMaxButton, htMinButton} {
		right := clientWidth - margin - driver.Points(i)*(width+gap)
		if p.X >= right-width && p.X < right {
			if !w.captionEnabled(code) {
				return 0
			}
			return code
		}
	}
	return 0
}

// captionEnabled reports whether the app's constraints leave the caption
// button code.
func (w *window) captionEnabled(code uint32) bool {
	c := &w.chrome.constraints
	return !(code == htMinButton && c.NoMinimize || code == htMaxButton && c.NoMaximize)
}

// ncHitTest answers WM_NCHITTEST: the hidden title bar's resize border
// and caption buttons, and the views that drag the window.
func (w *window) ncHitTest(lParam uintptr) uintptr {
	def, _, _ := defWindowProcW.Call(w.hwnd, wmNCHitTest, 0, lParam)
	if def != htClient {
		return def
	}
	at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
	screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	p := w.point(at.X, at.Y)
	if w.hidden() {
		border := w.frameThickness()
		resizable := !w.zoomed() && !w.chrome.constraints.NoResize
		if resizable && at.Y < border {
			var client rect
			getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
			switch {
			case at.X < border*2:
				return htTopLeft
			case at.X >= client.Right-border*2:
				return htTopRight
			}
			return htTop
		}
		// A shadowless window's client covers its side and bottom resize borders too.
		if resizable && w.shadowless() {
			var client rect
			getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
			if code := edgeHit(at, client, border); code != 0 {
				return code
			}
		}
		if code := w.captionAt(p); code != 0 {
			return uintptr(code)
		}
	}
	if w.windowDragAt(p) {
		return htCaption
	}
	return htClient
}

// edgeHit returns the side or bottom resize edge a client point at is within border pixels of, in a client area of size client, or 0.
func edgeHit(at point, client rect, border int32) uintptr {
	left, right, bottom := at.X < border, at.X >= client.Right-border, at.Y >= client.Bottom-border
	switch {
	case bottom && at.X < border*2:
		return htBottomLeft
	case bottom && at.X >= client.Right-border*2:
		return htBottomRight
	case bottom:
		return htBottom
	case left:
		return htLeft
	case right:
		return htRight
	}
	return 0
}

func isButton(code uint32) bool { return code == htMinButton || code == htMaxButton || code == htClose }

func (w *window) setHover(code uint32) {
	if w.chrome.hover != code {
		w.chrome.hover = code
		w.redrawAll()
	}
}

// ncMouse handles the non-client mouse messages over the caption
// buttons, reporting whether it took the message.
func (w *window) ncMouse(message uint32, wParam uintptr) bool {
	c := &w.chrome
	code := uint32(wParam)
	switch message {
	case wmNCMouseMove:
		if !c.tracking {
			c.tracking = true
			t := trackMouse{Flags: tmeLeave | tmeNonClient, Track: w.hwnd}
			t.Size = uint32(unsafe.Sizeof(t))
			trackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
		}
		if isButton(code) {
			w.setHover(code)
		} else {
			w.setHover(0)
		}
	case wmNCMouseLeave:
		c.tracking = false
		c.pressed = 0
		w.setHover(0)
	case wmNCLButtonDown, wmNCLButtonDblClk:
		if isButton(code) && w.hidden() {
			c.pressed = code
			w.redrawAll()
			return true
		}
	case wmNCLButtonUp:
		if isButton(code) && w.hidden() {
			pressed := c.pressed
			c.pressed = 0
			if pressed == code {
				switch code {
				case htMinButton:
					w.setHover(0)
					showWindow.Call(w.hwnd, swMinimize)
				case htMaxButton:
					w.toggleMaximize()
				default:
					postMessageW.Call(w.hwnd, wmClose, 0, 0)
				}
			}
			w.redrawAll()
			return true
		}
	}
	return false
}

// ncActivate handles WM_NCACTIVATE: the caption buttons dim in an
// inactive window, and with the title bar hidden the default handling
// must not paint the old caption over the app (lParam -1).
func (w *window) ncActivate(wParam, lParam uintptr) uintptr {
	w.chrome.inactive = wParam == 0
	w.redrawAll()
	if w.hidden() {
		lParam = ^uintptr(0)
	}
	r, _, _ := defWindowProcW.Call(w.hwnd, wmNCActivate, wParam, lParam)
	return r
}

// drawCaptionButtons draws the caption buttons over the app, with the title bar hidden, at the right of a client area width pixels wide: NSGo's rounded squares in the middle of the bar, or with WindowControlsSystem Windows 11's full-height rectangles flush with the edge, the close one red under the pointer. It draws in pixels, so the glyphs' lines fall on whole ones.
func (p *painter) drawCaptionButtons(width int32) {
	w := p.w
	if !w.buttons() {
		return
	}
	// In the topmost layer, above web views.
	p.realize()
	p.setTransform(scaling(1/w.scale, 1/w.scale))
	c := &w.chrome
	dark := w.shownDark()
	button, gap, margin := w.px(captionButtonSize), w.px(captionButtonGap), w.px(captionMargin)
	height := w.px(w.captionHeight())
	buttonHeight := button
	top := (height - buttonHeight) / 2
	system := w.chrome.controls == driver.WindowControlsSystem
	if system {
		button, buttonHeight, gap, margin = w.px(systemCaptionButtonWidth), height, 0, 0
		top = 0
	}
	glyph := w.px(captionGlyphSize)
	stroke := float32(max(w.px(1), 1))
	radius := float32(0)
	if !system {
		radius = float32(w.px(8))
	}
	for i, code := range []uint32{htClose, htMaxButton, htMinButton} {
		right := width - margin - int32(i)*(button+gap)
		r := rect{right - button, top, right, top + buttonHeight}
		hovered := c.hover == code
		pressed := hovered && c.pressed == code
		var bg driver.Color
		switch {
		case !hovered:
		case code == htClose && pressed:
			bg = 0xc42b1cff
		case code == htClose:
			bg = 0xe81123ff
		case system && pressed && !dark:
			bg = 0x00000024
		case system && pressed:
			bg = 0xffffff24
		case system && !dark:
			bg = 0x00000014
		case system:
			bg = 0xffffff14
		case !dark && pressed:
			bg = 0x33333324
		case !dark:
			bg = 0x33333314
		case pressed:
			bg = 0xeaeaea24
		default:
			bg = 0xeaeaea14
		}
		p.fillRounded(rectF{float32(r.Left), float32(r.Top), float32(r.Right), float32(r.Bottom)}, radius, bg)
		fg := driver.Color(0x717171ff)
		if dark {
			fg = 0xb7b7b7ff
		}
		switch {
		case !w.captionEnabled(code):
			fg = fg&^0xff | 0x48
		case code == htClose && hovered:
			fg = 0xffffffff
		case c.inactive && !hovered:
			fg = fg&^0xff | 0x70
		}
		left, glyphTop := float32(r.Left+(button-glyph)/2), float32(top+(buttonHeight-glyph)/2)
		if code == htMinButton {
			// The minus sits a little low, as its glyph reads.
			glyphTop += float32(w.px(2))
		}
		size := float32(glyph)
		// box strokes a square's outline inside the pixels from x, y.
		box := func(x, y, side float32) {
			line := rectF{x + stroke/2, y + stroke/2, x + side - stroke/2, y + side - stroke/2}
			p.dc.callF(rtDrawRectangle, 0b100, uintptr(unsafe.Pointer(&line)), p.solid(fg).ptr(), f32(stroke), 0)
		}
		switch {
		case code == htMinButton:
			mid := glyphTop + size/2
			p.fillRect(rectF{left, mid, left + size, mid + stroke}, fg)
		case code == htMaxButton && w.zoomed():
			// Restore: a square with another behind it.
			o := float32(max(w.px(2), 2))
			box(left, glyphTop+o, size-o)
			p.fillRect(rectF{left + o, glyphTop, left + size, glyphTop + stroke}, fg)
			p.fillRect(rectF{left + size - stroke, glyphTop, left + size, glyphTop + size - o}, fg)
			p.fillRect(rectF{left + o, glyphTop, left + o + stroke, glyphTop + o}, fg)
			p.fillRect(rectF{left + size - o, glyphTop + size - o - stroke, left + size, glyphTop + size - o}, fg)
		case code == htMaxButton:
			box(left, glyphTop, size)
		default:
			cross := p.pathGeometry(d2dFillAlternate, func(sink *comObject) {
				for _, l := range [2][2]pointF{{{left, glyphTop}, {left + size, glyphTop + size}}, {{left + size, glyphTop}, {left, glyphTop + size}}} {
					sink.callPoint(sinkBeginFigure, l[0].x, l[0].y, d2dFigureHollow)
					sink.call(sinkAddLines, uintptr(unsafe.Pointer(&l[1])), 1)
					sink.call(sinkEndFigure, d2dFigureOpen)
				}
			})
			if cross != nil {
				p.dc.callF(rtDrawGeometry, 0b100, cross.ptr(), p.solid(fg).ptr(), f32(stroke), 0)
				cross.release()
			}
		}
	}
}

// windowDragAt reports whether a press at p (client points) drags the
// window, as core.Tree.WindowDragAt decides: the nearest view around it
// that sets WindowDrag, unless a view nearer than it takes presses.
func (w *window) windowDragAt(p driver.Point) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	var path []*view
	for v := root; v != nil; {
		self, children := v.reach(p)
		if v.style.Hidden || !self && !children {
			break
		}
		if self {
			path = append(path, v)
		}
		if !children {
			break
		}
		p = v.local(p)
		p.X += v.offset.X
		p.Y += v.offset.Y
		var next *view
		for i := len(v.children) - 1; i >= 0; i-- {
			if c := v.children[i]; c.under(p) {
				next = c
				break
			}
		}
		v = next
	}
	for i := len(path) - 1; i >= 0; i-- {
		v := path[i]
		switch v.props.WindowDrag {
		case driver.WindowDragOn:
			return true
		case driver.WindowDragOff:
			return false
		}
		if takesPointer(v) {
			return false
		}
	}
	return false
}

// takesPointer reports whether presses on v are the app's.
func takesPointer(v *view) bool {
	if v.kind == driver.KindInput || v.kind == driver.KindScroll {
		return true
	}
	for _, k := range []driver.EventKind{driver.EventPress, driver.EventPressDown, driver.EventLongPress,
		driver.EventPan, driver.EventPinch, driver.EventRotation, driver.EventContextMenu} {
		if v.events.Has(k) {
			return true
		}
	}
	return false
}

// setMenu replaces the application menu; nil is Windows' default, none.
func (w *window) setMenu(entries []driver.MenuEntry) {
	w.chrome.menu = entries
	w.chrome.keys = map[string]menuRef{}
	w.menuKeys(entries)
	w.showMenuBar()
}

// shortcutOf returns e's shortcut, its role's when it has none.
func shortcutOf(e *driver.MenuEntry) string {
	return driver.MenuShortcutKey(*e, false)
}

// menuKeys gathers the shortcuts of entries' items, but the editing
// roles': the focused field handles those keys itself.
func (w *window) menuKeys(entries []driver.MenuEntry) {
	for i := range entries {
		e := &entries[i]
		switch {
		case e.Separator, e.Disabled, driver.MenuRoleUnsupported(e.Role, driver.OSWindows, speakAvailable()):
		case len(e.Items) > 0:
			w.menuKeys(e.Items)
		default:
			if k := shortcutOf(e); k != "" && !e.Role.EditsText() {
				if _, taken := w.chrome.keys[k]; !taken {
					w.chrome.keys[k] = menuRef{e.ID, e.Role, e.NotInText}
				}
			}
		}
	}
}

// menuPopups reports whether the window shows its application menu as a popup, having one and a hidden title bar, which Windows draws no menu bar in, full screen or not. A hidden root offers no menu, as in showMenuBar.
func (w *window) menuPopups() bool {
	c := &w.chrome
	return c.menu != nil && c.titleBar == driver.TitleBarHidden && !(w.primary && w.opts.Hidden)
}

// popupApplicationMenu shows entries of the application menu as a popup from the window's top leading corner, where a menu bar's menus drop from, with its first item selected, as the keys that open it open a bar's menu. It replaces a context menu tracking, as popupMenu does.
func (w *window) popupApplicationMenu(entries []driver.MenuEntry) {
	var at driver.Point
	if w.rtl {
		var client rect
		getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
		at = w.point(client.Right, 0)
	}
	if w.popupTracking {
		endMenu.Call()
	}
	w.popups = append(w.popups, popupRequest{entries: entries, at: at, keyboard: true})
	postMessageW.Call(w.hwnd, wmPopup, 0, 0)
}

// menuChar takes the character of Alt with a key (WM_SYSCHAR) in a window whose menu pops up: the top-level menu it is the mnemonic of opens, or a command there is chosen, as a menu bar's would. It reports whether one was; DefWindowProc keeps the others, such as Alt+Space's window menu. WM_SYSCHAR is the keyboard layout's character, so a label in any script matches.
func (w *window) menuChar(char uintptr) bool {
	if !w.menuPopups() {
		return false
	}
	key := unicode.ToLower(rune(char))
	for i := range w.chrome.menu {
		e := &w.chrome.menu[i]
		if e.Separator || e.Disabled || driver.MenuRoleUnsupported(e.Role, driver.OSWindows, speakAvailable()) || menutext.Mnemonic(menutext.Label(w.language, e, "")) != key {
			continue
		}
		if len(e.Items) > 0 {
			w.popupApplicationMenu(e.Items)
		} else {
			w.chooseMenu(menuRef{e.ID, e.Role, e.NotInText})
		}
		return true
	}
	return false
}

// showMenuBar shows the application menu as the window's menu bar, unless the title bar is hidden, the window is in full screen or transparent, the bar hides until Alt shows it or the window is a hidden root. A bar already shown is updated in place, so setting the menu again (such as checking an item) neither flickers the bar nor closes a menu the user has open.
func (w *window) showMenuBar() {
	defer w.fitMenuBar()
	c := &w.chrome
	shown := c.menuBar == driver.MenuBarShown || c.revealed || c.menuOpen
	// A hidden root only ever shows as an app-modal alert's window (showAlertWindow), which must not offer the app's menus while the alert waits.
	hiddenRoot := w.primary && w.opts.Hidden
	if c.menu == nil || c.titleBar != driver.TitleBarDefault || c.fullScreen || !shown || hiddenRoot || w.transparent {
		c.entries = nil
		if c.bar == 0 {
			return
		}
		setMenu.Call(w.hwnd, 0)
		destroyMenu.Call(c.bar)
		w.freeMenuBitmaps()
		c.bar = 0
		drawMenuBar.Call(w.hwnd)
		return
	}
	var refs []menuRef
	next := buildWinMenuTree(c.menu, w.language, w.menuTypeFlags(), speakAvailable(), w.menuIconLook(), &refs)
	ops := win32MenuOps{w}
	if c.bar != 0 {
		c.entries = refs
		changed := syncWinMenu(ops, c.bar, c.tree, next)
		c.tree = next
		if changed {
			drawMenuBar.Call(w.hwnd)
		}
		return
	}
	bar, _, _ := createMenu.Call()
	if bar == 0 {
		c.entries = nil
		return
	}
	syncWinMenu(ops, bar, nil, next)
	c.bar, c.tree, c.entries = bar, next, refs
	setMenu.Call(w.hwnd, bar)
	drawMenuBar.Call(w.hwnd)
}

// fitMenuBar keeps the content the size the app asked for as the menu bar attaches, changes height or goes: the window grows by the bar's height and shrinks back without it, its top left staying put. A bar Alt reveals with MenuBarAutoHide shows over the content instead, as it soon hides again. A maximized, minimized or full-screen window keeps its size, and so does the extra it already has.
func (w *window) fitMenuBar() {
	c := &w.chrome
	target := int32(0)
	if c.bar != 0 && c.menuBar == driver.MenuBarShown {
		target = w.menuBarHeight()
	}
	if target == c.barExtra || c.fullScreen || w.zoomed() || isIconicWindow(w.hwnd) {
		return
	}
	var outer rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
	height := outer.Bottom - outer.Top + target - c.barExtra
	c.barExtra = target
	setWindowPos.Call(w.hwnd, 0, 0, 0, uintptr(outer.Right-outer.Left), uintptr(height), swpNoMove|swpNoZOrder|swpNoActivate)
}

// menuBarHeight is how many pixels the menu bar takes from the client area, all its rows: how much taller the window's frame is than the frame its style has without a menu.
func (w *window) menuBarHeight() int32 {
	var outer, client rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	style, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlStyle)
	ex, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlExStyle)
	var frame rect
	if adjustWindowRectExForDpi.Find() == nil {
		dpi, _, _ := getDpiForWindow.Call(w.hwnd)
		adjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&frame)), style, 0, ex, dpi)
	} else {
		adjustWindowRectEx.Call(uintptr(unsafe.Pointer(&frame)), style, 0, ex)
	}
	return max(outer.Bottom-outer.Top-client.Bottom-(frame.Bottom-frame.Top), 0)
}

// menuIconLook is what the menu's icons' pixels depend on besides the icons: their size at the window's DPI and the appearance a template icon is tinted for.
func (w *window) menuIconLook() winMenuIconLook {
	return winMenuIconLook{size: w.menuIconSize(), dark: w.shownDark()}
}

// win32MenuOps applies menu sync operations to a window's HMENUs.
type win32MenuOps struct{ w *window }

func (win32MenuOps) createPopup() uintptr {
	sub, _, _ := createPopupMenu.Call()
	return sub
}

func (win32MenuOps) insertItem(menu uintptr, pos uint32, it *winMenuItem) {
	mi := winMenuItemInfo{
		cbSize: uint32(unsafe.Sizeof(winMenuItemInfo{})),
		fMask:  miimFType,
		fType:  it.fType,
	}
	var title *uint16
	if it.kind != winMenuSeparator {
		title = wide(it.label)
		mi.fMask |= miimState | miimString
		mi.fState = it.fState
		mi.dwTypeData = uintptr(unsafe.Pointer(title))
		if it.kind == winMenuSubmenu {
			mi.fMask |= miimSubMenu
			mi.hSubMenu = it.handle
		} else {
			mi.fMask |= miimID
			mi.wID = it.commandID
		}
		if it.bitmap != 0 {
			mi.fMask |= miimBitmap
			mi.hbmpItem = it.bitmap
		}
	}
	insertMenuItemW.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
	runtime.KeepAlive(title)
}

func (win32MenuOps) updateItem(menu uintptr, pos uint32, next *winMenuItem, diff winMenuDiff) {
	mi := winMenuItemInfo{cbSize: uint32(unsafe.Sizeof(winMenuItemInfo{}))}
	if diff.fTypeChanged {
		mi.fMask |= miimFType
		mi.fType = next.fType
	}
	if diff.fStateChanged {
		mi.fMask |= miimState
		mi.fState = next.fState
	}
	if diff.idChanged {
		mi.fMask |= miimID
		mi.wID = next.commandID
	}
	var title *uint16
	if diff.labelChanged {
		title = wide(next.label)
		mi.fMask |= miimString
		mi.dwTypeData = uintptr(unsafe.Pointer(title))
	}
	if diff.iconChanged {
		mi.fMask |= miimBitmap
		mi.hbmpItem = next.bitmap
	}
	if mi.fMask != 0 {
		setMenuItemInfoW.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
		runtime.KeepAlive(title)
	}
}

func (win32MenuOps) deleteItem(menu uintptr, pos uint32) {
	deleteMenu.Call(menu, uintptr(pos), mfByPosition)
}

func (o win32MenuOps) iconBitmap(icon driver.MenuIcon) uintptr { return o.w.menuIconBitmap(icon) }

func (win32MenuOps) freeBitmap(bitmap uintptr) { deleteObject.Call(bitmap) }

// menuTypeFlags are the MFT_* flags of every item of a menu: right to left in
// an app laid out so, whatever the system's direction.
func (w *window) menuTypeFlags() uint32 {
	if w.rtl {
		return mfString | mfRightOrder
	}
	return mfString
}

// insertSeparator appends a separator to menu at position pos.
func insertSeparator(menu uintptr, pos uint32) {
	mi := winMenuItemInfo{
		cbSize: uint32(unsafe.Sizeof(winMenuItemInfo{})),
		fMask:  miimFType,
		fType:  mfSeparator,
	}
	insertMenuItemW.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
}

// insertMenuItem appends a labeled item or submenu to menu at position pos.
func insertMenuItem(menu uintptr, pos uint32, fType, fState, id uint32, sub uintptr, label string) {
	title := wide(label)
	mi := winMenuItemInfo{
		cbSize:     uint32(unsafe.Sizeof(winMenuItemInfo{})),
		fMask:      miimFType | miimState | miimString,
		fType:      fType,
		fState:     fState,
		dwTypeData: uintptr(unsafe.Pointer(title)),
	}
	if sub != 0 {
		mi.fMask |= miimSubMenu
		mi.hSubMenu = sub
	} else {
		mi.fMask |= miimID
		mi.wID = id
	}
	insertMenuItemW.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
	runtime.KeepAlive(title)
}

// popupFlags are TrackPopupMenuEx's flags of a context menu, which opens
// leftward in an app laid out right to left.
func (w *window) popupFlags() uintptr {
	if w.rtl {
		return tpmReturnCmd | tpmRightButton | tpmLayoutRTL | tpmRightAlign
	}
	return tpmReturnCmd | tpmRightButton
}

// appendMenuTo appends entries to menu, numbering their commands by their index in refs plus one, records the HBITMAPs it creates in bitmaps, and returns how many items it appended. A separator goes between items only, so the roles Windows leaves out leave no separator at either end, and a submenu left with no items is left out too (buildWinMenuTree).
func (w *window) appendMenuTo(menu uintptr, entries []driver.MenuEntry, refs *[]menuRef, bitmaps *[]uintptr) int {
	tree := buildWinMenuTree(entries, w.language, w.menuTypeFlags(), speakAvailable(), w.menuIconLook(), refs)
	syncWinMenu(win32MenuOps{w}, menu, nil, tree)
	if bitmaps != nil {
		*bitmaps = appendWinMenuBitmaps(*bitmaps, tree)
	}
	return len(tree)
}

// appendWinMenuBitmaps appends the HBITMAPs items and their submenus hold to bitmaps.
func appendWinMenuBitmaps(bitmaps []uintptr, items []winMenuItem) []uintptr {
	for i := range items {
		if items[i].bitmap != 0 {
			bitmaps = append(bitmaps, items[i].bitmap)
		}
		bitmaps = appendWinMenuBitmaps(bitmaps, items[i].children)
	}
	return bitmaps
}

// menuCommand handles a WM_COMMAND from the menu bar.
func (w *window) menuCommand(command uint16) {
	if i := int(command) - 1; i >= 0 && i < len(w.chrome.entries) {
		w.chooseMenu(w.chrome.entries[i])
	}
}

// menuShortcut chooses the item with shortcut key, reporting whether
// there is one.
func (w *window) menuShortcut(key string) bool {
	ref, ok := w.chrome.keys[key]
	if ok && ref.notInText && w.editing() {
		return false // the field's or the page's key
	}
	if ok {
		w.chooseMenu(ref)
	}
	return ok
}

// chooseMenu carries out an item's role and tells the app about it. An editing role tells the app only when neither the focused field, the focused page nor the text selection takes it.
func (w *window) chooseMenu(ref menuRef) {
	if ref.id != 0 && !ref.role.EditsText() && ref.role != driver.MenuPasteAndMatchStyle {
		w.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: ref.id})
		return
	}
	edited := false
	switch ref.role {
	case driver.MenuQuit:
		w.requestQuit()
	case driver.MenuClose:
		postMessageW.Call(w.hwnd, wmClose, 0, 0)
	case driver.MenuMinimize:
		w.minimize()
	case driver.MenuZoom:
		w.toggleMaximize()
	case driver.MenuToggleFullScreen:
		w.toggleFullScreen()
	case driver.MenuUndo, driver.MenuRedo, driver.MenuCut, driver.MenuCopy, driver.MenuPaste, driver.MenuPasteAndMatchStyle, driver.MenuDelete, driver.MenuSelectAll:
		edited = w.menuEdit(ref.role)
	case driver.MenuStartSpeaking:
		w.speakMenu(true)
	case driver.MenuStopSpeaking:
		w.speakMenu(false)
	}
	if ref.id != 0 && !edited {
		w.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: ref.id})
	}
}

// menuEdit carries out an editing role with the focused field's own keys, else the focused page's editing command, else on the text selection (Copy and Select All), reporting whether one of them took it. A focused field takes every editing role, even one it has nothing to do for.
func (w *window) menuEdit(role driver.MenuRole) bool {
	in := w.focusedField()
	switch {
	case in == nil:
		return w.webEdit(role) || w.selectionEdit(role)
	case role == driver.MenuPasteAndMatchStyle:
		w.paste(in)
	case role == driver.MenuDelete:
		// Delete removes the selection alone, as the field's context menu does; with none it would remove the character after the caret.
		if in.model.hasSelection() {
			w.editKey(vkDelete, false, false, false)
		}
	default:
		vk := map[driver.MenuRole]uintptr{driver.MenuUndo: 'Z', driver.MenuRedo: 'Y', driver.MenuCut: 'X',
			driver.MenuCopy: 'C', driver.MenuPaste: 'V', driver.MenuSelectAll: 'A'}[role]
		w.editKey(vk, true, false, false)
	}
	return true
}
