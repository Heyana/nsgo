//go:build windows

// Package windows is NSGo's Win32 backend: one window whose retained node
// tree is drawn with GDI, presses hit tested by the engine, and inputs as
// native EDIT controls. It uses only the syscall package, so it builds
// without cgo.
package windows

import (
	"runtime"
	"syscall"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
	"github.com/oneo-me/nsgo/platform/internal/queue"
)

func init() {
	// The window belongs to the thread that creates it: main's.
	runtime.LockOSThread()
}

// Options configure the window.
type Options struct {
	Title         string
	Width, Height core.Points
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	registerClassExW    = user32.NewProc("RegisterClassExW")
	createWindowExW     = user32.NewProc("CreateWindowExW")
	defWindowProcW      = user32.NewProc("DefWindowProcW")
	callWindowProcW     = user32.NewProc("CallWindowProcW")
	getMessageW         = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessageW    = user32.NewProc("DispatchMessageW")
	postMessageW        = user32.NewProc("PostMessageW")
	sendMessageW        = user32.NewProc("SendMessageW")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	showWindow          = user32.NewProc("ShowWindow")
	beginPaint          = user32.NewProc("BeginPaint")
	endPaint            = user32.NewProc("EndPaint")
	invalidateRect      = user32.NewProc("InvalidateRect")
	getClientRect       = user32.NewProc("GetClientRect")
	loadCursorW         = user32.NewProc("LoadCursorW")
	moveWindow          = user32.NewProc("MoveWindow")
	setWindowTextW      = user32.NewProc("SetWindowTextW")
	getWindowTextW      = user32.NewProc("GetWindowTextW")
	getWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	setWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	getDpiForSystem     = user32.NewProc("GetDpiForSystem")
	setDpiAwareness     = user32.NewProc("SetProcessDpiAwarenessContext")
	drawTextW           = user32.NewProc("DrawTextW")
	fillRect            = user32.NewProc("FillRect")

	createCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	deleteDC               = gdi32.NewProc("DeleteDC")
	deleteObject           = gdi32.NewProc("DeleteObject")
	selectObject           = gdi32.NewProc("SelectObject")
	bitBlt                 = gdi32.NewProc("BitBlt")
	createFontW            = gdi32.NewProc("CreateFontW")
	createSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	getStockObject         = gdi32.NewProc("GetStockObject")
	roundRect              = gdi32.NewProc("RoundRect")
	setBkMode              = gdi32.NewProc("SetBkMode")
	setTextColor           = gdi32.NewProc("SetTextColor")
	setBkColor             = gdi32.NewProc("SetBkColor")

	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy     = 0x0002
	wmSize        = 0x0005
	wmPaint       = 0x000F
	wmEraseBkgnd  = 0x0014
	wmSetFont     = 0x0030
	wmKeyDown     = 0x0100
	wmChar        = 0x0102
	wmCommand     = 0x0111
	wmLButtonUp   = 0x0202
	wmCtlColorEdt = 0x0133
	wmApply       = 0x8000 + 1 // WM_APP + 1: batches are queued

	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	esAutoHScroll      = 0x0080
	enChange           = 0x0300
	cwUseDefault       = ^uintptr(0x7FFFFFFF) // CW_USEDEFAULT as a uintptr
	gwlpWndProc        = ^uintptr(3)          // -4
	vkReturn           = 0x0D

	dtWordBreak  = 0x0010
	dtCalcRect   = 0x0400
	dtNoPrefix   = 0x0800
	dtSingleLine = 0x0020
	transparent  = 1
	nullPen      = 8
	srcCopy      = 0x00CC0020
	clearType    = 5
)

type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	_       uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	_         [32]byte
}

// node is the main thread's copy of one mounted node.
type node struct {
	tag      core.Tag
	props    core.Props
	frame    core.Bounds
	children []core.NodeID
	edit     uintptr // an input's EDIT control
	font     uintptr
	brush    uintptr // an input's background brush, for WM_CTLCOLOREDIT
	editProc uintptr
}

type window struct {
	hwnd    uintptr
	engine  *engine.Engine
	batches queue.Batches
	nodes   []*node // index is NodeID
	root    core.NodeID
	scale   float64 // pixels per point
	edits   map[uintptr]core.NodeID

	measureDC    uintptr // the engine thread's
	measureFonts fonts   // the engine thread's
	paintFonts   fonts   // the main thread's
}

// Window procedures are process-wide callbacks; they find their window
// here. Only the main thread uses these maps.
var (
	windows     = map[uintptr]*window{}
	editWindows = map[uintptr]*window{}
)

var wndProcCallback = syscall.NewCallback(wndProc)
var editProcCallback = syscall.NewCallback(editProc)

func utf16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

// Run opens a window showing app and runs the message loop. Call it from
// main; it does not return until the window closes.
func Run(app engine.App, opts Options) {
	if opts.Width == 0 {
		opts.Width, opts.Height = 480, 640
	}
	const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	setDpiAwareness.Call(perMonitorAwareV2)
	w := &window{scale: 1, edits: map[uintptr]core.NodeID{}, measureFonts: fonts{}, paintFonts: fonts{}}
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		w.scale = float64(dpi) / 96
	}

	instance, _, _ := getModuleHandleW.Call(0)
	const idcArrow = 32512
	cursor, _, _ := loadCursorW.Call(0, idcArrow)
	className := utf16("NsgoWindow")
	wc := wndClassEx{
		WndProc:   wndProcCallback,
		Instance:  instance,
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	w.engine = engine.Start(w, app, core.Size{Width: opts.Width, Height: opts.Height})
	hwnd, _, _ := createWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16(opts.Title))), wsOverlappedWindow,
		cwUseDefault, cwUseDefault,
		uintptr(float64(opts.Width)*w.scale), uintptr(float64(opts.Height)*w.scale),
		0, 0, instance, 0)
	w.hwnd = hwnd
	windows[hwnd] = w
	const swShow = 5
	showWindow.Call(hwnd, swShow)

	var m msg
	for {
		r, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// fonts are GDI fonts by size and weight, in pixels. The engine thread and
// the main thread each keep their own.
type fonts map[core.Font]uintptr

func (f fonts) get(font core.Font, scale float64) uintptr {
	if h, ok := f[font]; ok {
		return h
	}
	height := -int32(float64(font.Size)*scale + 0.5)
	h, _, _ := createFontW.Call(uintptr(height), 0, 0, 0, uintptr(font.Weight), 0, 0, 0,
		1 /* DEFAULT_CHARSET */, 0, 0, clearType, 0, uintptr(unsafe.Pointer(utf16("Segoe UI"))))
	f[font] = h
	return h
}

// Measure implements core.TextEngine with GDI on the engine's thread.
func (w *window) Measure(text string, font core.Font, maxWidth core.Points) core.Size {
	if w.measureDC == 0 {
		w.measureDC, _, _ = createCompatibleDC.Call(0)
	}
	selectObject.Call(w.measureDC, w.measureFonts.get(font, w.scale))
	s := text
	if s == "" {
		s = " "
	}
	r := rect{Right: 1 << 20}
	flags := uintptr(dtCalcRect | dtNoPrefix)
	if !maxWidth.IsAuto() {
		r.Right = int32(float64(maxWidth) * w.scale)
		flags |= dtWordBreak
	} else {
		flags |= dtSingleLine
	}
	u, _ := syscall.UTF16FromString(s)
	drawTextW.Call(w.measureDC, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), flags)
	width := core.Points(float64(r.Right-r.Left) / w.scale)
	if text == "" {
		width = 0
	}
	return core.Size{Width: width + 1, Height: core.Points(float64(r.Bottom-r.Top) / w.scale)}
}

// Mount implements engine.Platform.
func (w *window) Mount(b *core.MountBatch) {
	if w.batches.Push(b) {
		postMessageW.Call(w.hwnd, wmApply, 0, 0)
	}
}

func (w *window) px(p core.Points) int32 { return int32(float64(p)*w.scale + 0.5) }

func (w *window) apply(b *core.MountBatch) {
	moved := false
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			for len(w.nodes) <= int(it.ID) {
				w.nodes = append(w.nodes, nil)
			}
			n := &node{tag: it.Tag}
			if it.Tag == core.TagInput {
				instance, _, _ := getModuleHandleW.Call(0)
				n.edit, _, _ = createWindowExW.Call(0, uintptr(unsafe.Pointer(utf16("EDIT"))), 0,
					wsChild|wsVisible|esAutoHScroll, 0, 0, 0, 0, w.hwnd, uintptr(it.ID), instance, 0)
				n.editProc, _, _ = setWindowLongPtrW.Call(n.edit, gwlpWndProc, editProcCallback)
				w.edits[n.edit] = it.ID
				editWindows[n.edit] = w
			}
			w.nodes[it.ID] = n
		case core.ItemProps:
			n := w.nodes[it.ID]
			n.props = it.Props
			if n.edit != 0 {
				n.font = w.paintFonts.get(it.Props.Font, w.scale)
				sendMessageW.Call(n.edit, wmSetFont, n.font, 1)
				if current := editText(n.edit); current != it.Props.Text {
					setWindowTextW.Call(n.edit, uintptr(unsafe.Pointer(utf16(it.Props.Text))))
				}
				if n.brush != 0 {
					deleteObject.Call(n.brush)
				}
				n.brush, _, _ = createSolidBrush.Call(colorRef(it.Props.Background))
				moved = true
			}
		case core.ItemInsert:
			p := w.nodes[it.Parent]
			idx := min(it.Index, len(p.children))
			p.children = append(p.children, 0)
			copy(p.children[idx+1:], p.children[idx:])
			p.children[idx] = it.ID
		case core.ItemFrame:
			w.nodes[it.ID].frame = it.Frame
			moved = true
		}
	}
	w.root = b.Root
	if moved {
		w.placeEdits(w.root, 0, 0)
	}
	invalidateRect.Call(w.hwnd, 0, 0)
}

// placeEdits moves EDIT controls to their nodes' frames, inset by padding
// and centered vertically on the font's line.
func (w *window) placeEdits(id core.NodeID, x, y core.Points) {
	n := w.nodes[id]
	if n == nil {
		return
	}
	x, y = x+n.frame.Origin.X, y+n.frame.Origin.Y
	if n.edit != 0 {
		p := n.props.Padding
		line := core.Points(float64(n.props.Font.Size) * 1.4)
		inner := n.frame.Size.Height - p.Top - p.Bottom
		moveWindow.Call(n.edit, uintptr(w.px(x+p.Left)), uintptr(w.px(y+p.Top+(inner-line)/2)),
			uintptr(w.px(n.frame.Size.Width-p.Left-p.Right)), uintptr(w.px(line)), 1)
	}
	for _, c := range n.children {
		w.placeEdits(c, x, y)
	}
}

func colorRef(c core.Color) uintptr {
	return uintptr(c>>24&0xff | (c>>16&0xff)<<8 | (c>>8&0xff)<<16)
}

func editText(edit uintptr) string {
	n, _, _ := getWindowTextLength.Call(edit)
	buf := make([]uint16, n+1)
	getWindowTextW.Call(edit, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func (w *window) paint() {
	var ps paintStruct
	hdc, _, _ := beginPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer endPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	var client rect
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))

	// Draw into a bitmap first so the window never shows a half-drawn frame.
	mem, _, _ := createCompatibleDC.Call(hdc)
	bmp, _, _ := createCompatibleBitmap.Call(hdc, uintptr(client.Right), uintptr(client.Bottom))
	old, _, _ := selectObject.Call(mem, bmp)
	white, _, _ := getStockObject.Call(0 /* WHITE_BRUSH */)
	fillRect.Call(mem, uintptr(unsafe.Pointer(&client)), white)
	setBkMode.Call(mem, transparent)
	if w.root != 0 {
		w.draw(mem, w.root, 0, 0)
	}
	bitBlt.Call(hdc, 0, 0, uintptr(client.Right), uintptr(client.Bottom), mem, 0, 0, srcCopy)
	selectObject.Call(mem, old)
	deleteObject.Call(bmp)
	deleteDC.Call(mem)
}

func (w *window) draw(dc uintptr, id core.NodeID, x, y core.Points) {
	n := w.nodes[id]
	if n == nil {
		return
	}
	x, y = x+n.frame.Origin.X, y+n.frame.Origin.Y
	r := rect{w.px(x), w.px(y), w.px(x + n.frame.Size.Width), w.px(y + n.frame.Size.Height)}
	p := &n.props
	if p.Background&0xff != 0 {
		brush, _, _ := createSolidBrush.Call(colorRef(p.Background))
		pen, _, _ := getStockObject.Call(nullPen)
		oldBrush, _, _ := selectObject.Call(dc, brush)
		oldPen, _, _ := selectObject.Call(dc, pen)
		d := uintptr(w.px(p.CornerRadius * 2))
		// RoundRect leaves out the right and bottom edges with a null pen.
		roundRect.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1), d, d)
		selectObject.Call(dc, oldBrush)
		selectObject.Call(dc, oldPen)
		deleteObject.Call(brush)
	}
	if n.tag == core.TagText && p.Text != "" {
		selectObject.Call(dc, w.paintFonts.get(p.Font, w.scale))
		setTextColor.Call(dc, colorRef(p.Color))
		box := rect{r.Left + w.px(p.Padding.Left), r.Top + w.px(p.Padding.Top),
			r.Right - w.px(p.Padding.Right), r.Bottom - w.px(p.Padding.Bottom)}
		u, _ := syscall.UTF16FromString(p.Text)
		drawTextW.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
			uintptr(unsafe.Pointer(&box)), dtWordBreak|dtNoPrefix)
	}
	for _, c := range n.children {
		w.draw(dc, c, x, y)
	}
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	w := windows[hwnd]
	if w == nil {
		r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	switch message {
	case wmApply:
		for _, b := range w.batches.Drain() {
			w.apply(b)
		}
		return 0
	case wmSize:
		width, height := lParam&0xffff, lParam>>16&0xffff
		w.engine.Resize(core.Size{Width: core.Points(float64(width) / w.scale), Height: core.Points(float64(height) / w.scale)})
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		w.paint()
		return 0
	case wmLButtonUp:
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		w.engine.PressAt(core.Point{X: core.Points(float64(x) / w.scale), Y: core.Points(float64(y) / w.scale)})
		return 0
	case wmCommand:
		if wParam>>16 == enChange {
			if id, ok := w.edits[lParam]; ok {
				w.engine.Change(id, editText(lParam))
			}
		}
		return 0
	case wmCtlColorEdt:
		if id, ok := w.edits[lParam]; ok {
			n := w.nodes[id]
			setTextColor.Call(wParam, colorRef(n.props.Color))
			setBkColor.Call(wParam, colorRef(n.props.Background))
			return n.brush
		}
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	}
	r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

// editProc subclasses EDIT controls to report Return as a submit.
func editProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	w := editWindows[hwnd]
	if w == nil {
		r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	id := w.edits[hwnd]
	if wParam == vkReturn && (message == wmKeyDown || message == wmChar) {
		if message == wmKeyDown {
			w.engine.Submit(id, editText(hwnd))
		}
		return 0 // no beep
	}
	r, _, _ := callWindowProcW.Call(w.nodes[id].editProc, hwnd, uintptr(message), wParam, lParam)
	return r
}
