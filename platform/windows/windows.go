//go:build windows

// Package windows is NSGo's Win32 backend: one window that draws a
// retained tree of views with GDI, text the engine measured and laid out
// with GDI fonts, presses hit tested by the engine, and inputs as native
// EDIT controls. It uses only the syscall package, so it builds without
// cgo.
//
// GDI has no alpha: colors draw opaque, views with an opacity of 0 are
// skipped and other opacities draw opaque. Shadows are not drawn, and
// there is no accessibility yet, which needs UI Automation providers.
package windows

import (
	"math"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"unicode/utf16"
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
	destroyWindow       = user32.NewProc("DestroyWindow")
	defWindowProcW      = user32.NewProc("DefWindowProcW")
	callWindowProcW     = user32.NewProc("CallWindowProcW")
	getMessageW         = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessageW    = user32.NewProc("DispatchMessageW")
	postMessageW        = user32.NewProc("PostMessageW")
	sendMessageW        = user32.NewProc("SendMessageW")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	showWindow          = user32.NewProc("ShowWindow")
	enableWindow        = user32.NewProc("EnableWindow")
	setFocus            = user32.NewProc("SetFocus")
	getFocus            = user32.NewProc("GetFocus")
	beginPaint          = user32.NewProc("BeginPaint")
	endPaint            = user32.NewProc("EndPaint")
	getDC               = user32.NewProc("GetDC")
	releaseDC           = user32.NewProc("ReleaseDC")
	invalidateRect      = user32.NewProc("InvalidateRect")
	getClientRect       = user32.NewProc("GetClientRect")
	adjustWindowRect    = user32.NewProc("AdjustWindowRect")
	screenToClient      = user32.NewProc("ScreenToClient")
	loadCursorW         = user32.NewProc("LoadCursorW")
	moveWindow          = user32.NewProc("MoveWindow")
	setWindowRgn        = user32.NewProc("SetWindowRgn")
	setWindowTextW      = user32.NewProc("SetWindowTextW")
	getWindowTextW      = user32.NewProc("GetWindowTextW")
	getWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	getWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	setWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	hideCaret           = user32.NewProc("HideCaret")
	showCaret           = user32.NewProc("ShowCaret")
	getSysColor         = user32.NewProc("GetSysColor")
	getDpiForSystem     = user32.NewProc("GetDpiForSystem")
	setDpiAwareness     = user32.NewProc("SetProcessDpiAwarenessContext")
	drawTextW           = user32.NewProc("DrawTextW")
	fillRect            = user32.NewProc("FillRect")

	createCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	deleteDC               = gdi32.NewProc("DeleteDC")
	saveDC                 = gdi32.NewProc("SaveDC")
	restoreDC              = gdi32.NewProc("RestoreDC")
	deleteObject           = gdi32.NewProc("DeleteObject")
	selectObject           = gdi32.NewProc("SelectObject")
	bitBlt                 = gdi32.NewProc("BitBlt")
	createFontW            = gdi32.NewProc("CreateFontW")
	getTextMetricsW        = gdi32.NewProc("GetTextMetricsW")
	setTextCharacterExtra  = gdi32.NewProc("SetTextCharacterExtra")
	createSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	createPen              = gdi32.NewProc("CreatePen")
	getStockObject         = gdi32.NewProc("GetStockObject")
	roundRect              = gdi32.NewProc("RoundRect")
	createRectRgn          = gdi32.NewProc("CreateRectRgn")
	createRoundRectRgn     = gdi32.NewProc("CreateRoundRectRgn")
	extSelectClipRgn       = gdi32.NewProc("ExtSelectClipRgn")
	setBkMode              = gdi32.NewProc("SetBkMode")
	setTextColor           = gdi32.NewProc("SetTextColor")
	setBkColor             = gdi32.NewProc("SetBkColor")

	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmPaint          = 0x000F
	wmEraseBkgnd     = 0x0014
	wmSetFont        = 0x0030
	wmKeyDown        = 0x0100
	wmChar           = 0x0102
	wmCommand        = 0x0111
	wmCtlColorEdit   = 0x0133
	wmCtlColorStatic = 0x0138 // what read-only and disabled EDITs send
	wmLButtonUp      = 0x0202
	wmMouseWheel     = 0x020A
	wmMouseHWheel    = 0x020E
	wmApply          = 0x8000 + 1 // WM_APP + 1: batches are queued

	wsOverlappedWindow = 0x00CF0000
	wsClipChildren     = 0x02000000
	wsChild            = 0x40000000
	esCenter           = 0x0001
	esRight            = 0x0002
	esPassword         = 0x0020
	esAutoHScroll      = 0x0080
	enSetFocus         = 0x0100
	enKillFocus        = 0x0200
	enChange           = 0x0300
	emSetSel           = 0x00B1
	emSetPasswordChar  = 0x00CC
	emSetReadOnly      = 0x00CF
	emSetMargins       = 0x00D3
	ecBothMargins      = 0x0003
	cwUseDefault       = ^uintptr(0x7FFFFFFF) // CW_USEDEFAULT as a uintptr
	gwlpWndProc        = ^uintptr(3)          // -4
	gwlStyle           = ^uintptr(15)         // -16
	swHide             = 0
	swShow             = 5
	swShowNA           = 8
	sizeMinimized      = 1
	vkReturn           = 0x0D
	maskChar           = 0x25CF // ●

	dtCenter      = 0x0001
	dtRight       = 0x0002
	dtWordBreak   = 0x0010
	dtSingleLine  = 0x0020
	dtCalcRect    = 0x0400
	dtNoPrefix    = 0x0800
	dtEditControl = 0x2000
	dtEndEllipsis = 0x8000

	transparent     = 1
	whiteBrush      = 0
	nullBrush       = 5
	nullPen         = 8
	psInsideFrame   = 6
	rgnAnd          = 1
	srcCopy         = 0x00CC0020
	clearType       = 5
	defaultCharset  = 1
	colorGrayText   = 17
	defaultFontName = "Segoe UI"
)

// wheelStep is how far one notch of a mouse wheel scrolls.
const wheelStep core.Points = 48

type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) empty() bool { return r.Right <= r.Left || r.Bottom <= r.Top }

func (r rect) intersect(o rect) rect {
	return rect{max(r.Left, o.Left), max(r.Top, o.Top), min(r.Right, o.Right), min(r.Bottom, o.Bottom)}
}

// everywhere clips nothing.
var everywhere = rect{-1 << 30, -1 << 30, 1 << 30, 1 << 30}

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
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

type textMetric struct {
	Height, Ascent, Descent, InternalLeading, ExternalLeading int32
	AveCharWidth, MaxCharWidth, Weight, Overhang              int32
	DigitizedAspectX, DigitizedAspectY                        int32
	FirstChar, LastChar, DefaultChar, BreakChar               uint16
	Italic, Underlined, StruckOut, PitchAndFamily, CharSet    uint8
}

// view is the main thread's copy of one mounted view.
type view struct {
	id       core.NodeID
	kind     core.ViewKind
	parent   *view
	children []*view
	frame    core.Bounds
	style    core.ViewStyle
	events   core.EventMask
	deleted  bool

	// A text view's layout and where it goes in the view.
	text    *textLayout
	content core.Bounds

	// A scroll view's configuration, content size and offset.
	scroll core.ScrollConfig
	size   core.Size
	offset core.Point

	// An input's EDIT control and what the window keeps for it.
	edit       uintptr
	editProc   uintptr
	input      core.InputConfig
	font       *font
	brush      uintptr // the background WM_CTLCOLOREDIT returns
	brushColor core.Color
	edits      uint32 // user edits so far
	setting    bool   // the window is setting the text, not the user
	shown      bool
	placed     uint32 // the last placeEdits pass that showed the EDIT
	editRect   rect   // in the window's client area
	editClip   rect   // the visible part, in the EDIT's own coordinates
}

// detach takes v out of its parent's children.
func (v *view) detach() {
	if p := v.parent; p != nil {
		if i := slices.Index(p.children, v); i >= 0 {
			p.children = slices.Delete(p.children, i, i+1)
		}
		v.parent = nil
	}
}

type window struct {
	hwnd    uintptr
	engine  *engine.Engine
	batches queue.Batches
	scale   float64 // pixels per point
	fonts   fontCache

	// The main thread's.
	views []*view // index is NodeID
	root  core.NodeID
	edits map[uintptr]*view
	pass  uint32

	measureDC uintptr // the engine thread's
}

// Window procedures are process-wide callbacks; they find their window
// here. Only the main thread uses these maps.
var (
	windows     = map[uintptr]*window{}
	editWindows = map[uintptr]*window{}
)

var wndProcCallback = syscall.NewCallback(wndProc)
var editProcCallback = syscall.NewCallback(editProc)

func wide(s string) *uint16 {
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
	w := &window{scale: 1, edits: map[uintptr]*view{}}
	if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
		w.scale = float64(dpi) / 96
	}
	w.fonts = fontCache{scale: w.scale, fonts: map[fontKey]*font{}}

	instance, _, _ := getModuleHandleW.Call(0)
	const idcArrow = 32512
	cursor, _, _ := loadCursorW.Call(0, idcArrow)
	className := wide("NsgoWindow")
	wc := wndClassEx{
		WndProc:   wndProcCallback,
		Instance:  instance,
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Size the window so its client area is the app's size.
	const style = wsOverlappedWindow | wsClipChildren
	frame := rect{Right: w.px(opts.Width), Bottom: w.px(opts.Height)}
	adjustWindowRect.Call(uintptr(unsafe.Pointer(&frame)), style, 0)
	hwnd, _, _ := createWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(wide(opts.Title))), style,
		cwUseDefault, cwUseDefault, uintptr(frame.Right-frame.Left), uintptr(frame.Bottom-frame.Top),
		0, 0, instance, 0)
	w.hwnd = hwnd
	windows[hwnd] = w
	// The engine starts once the window exists, so Mount can post to it.
	w.engine = engine.Start(w, app, engine.Options{Size: core.Size{Width: opts.Width, Height: opts.Height}})
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

// fontKey is what a GDI font is made from.
type fontKey struct {
	font       core.Font
	decoration core.TextDecoration
}

// font is a GDI font shared by text layouts and EDIT controls, and freed
// when the last of them releases it.
type font struct {
	key    fontKey
	handle uintptr
	height int32 // of a line, in pixels
	refs   int
}

// fontCache holds the fonts the engine thread measures with and the main
// thread draws and edits with.
type fontCache struct {
	mu    sync.Mutex
	scale float64
	fonts map[fontKey]*font
}

func (c *fontCache) acquire(key fontKey) *font {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f := c.fonts[key]; f != nil {
		f.refs++
		return f
	}
	family := key.font.Family
	if family == "" {
		family = defaultFontName
	}
	var italic, underline, strike uintptr
	if key.font.Italic {
		italic = 1
	}
	if key.decoration&core.TextDecorationUnderline != 0 {
		underline = 1
	}
	if key.decoration&core.TextDecorationLineThrough != 0 {
		strike = 1
	}
	height := -int32(float64(key.font.Size)*c.scale + 0.5)
	h, _, _ := createFontW.Call(uintptr(height), 0, 0, 0, uintptr(key.font.Weight), italic, underline, strike,
		defaultCharset, 0, 0, clearType, 0, uintptr(unsafe.Pointer(wide(family))))
	dc, _, _ := createCompatibleDC.Call(0)
	old, _, _ := selectObject.Call(dc, h)
	var tm textMetric
	getTextMetricsW.Call(dc, uintptr(unsafe.Pointer(&tm)))
	selectObject.Call(dc, old)
	deleteDC.Call(dc)
	f := &font{key: key, handle: h, height: max(tm.Height, 1), refs: 1}
	c.fonts[key] = f
	return f
}

func (c *fontCache) retain(f *font) {
	c.mu.Lock()
	f.refs++
	c.mu.Unlock()
}

func (c *fontCache) release(f *font) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if f.refs--; f.refs == 0 {
		delete(c.fonts, f.key)
		deleteObject.Call(f.handle)
	}
}

// preparedText is text with its font, measured on the engine thread.
type preparedText struct {
	w        *window
	text     []uint16
	font     *font
	color    core.Color
	flags    uintptr // DrawTextW's, for measuring and drawing
	extra    int32   // letter spacing, in pixels
	step     int32   // line height, in pixels
	maxLines int
}

// Prepare implements core.TextEngine on the engine thread.
func (w *window) Prepare(t *core.AttributedText) core.PreparedText {
	p := &preparedText{
		w:        w,
		text:     utf16.Encode([]rune(t.Text)),
		font:     w.fonts.acquire(fontKey{t.Style.Font, t.Style.Decoration}),
		color:    t.Style.Color,
		flags:    dtNoPrefix,
		extra:    w.px(t.Style.LetterSpacing),
		maxLines: t.MaxLines,
	}
	p.step = p.font.height
	if t.Style.LineHeight > 0 {
		p.step = max(w.px(t.Style.LineHeight), 1)
	}
	switch t.Style.Align {
	case core.TextAlignCenter:
		p.flags |= dtCenter
	case core.TextAlignEnd:
		p.flags |= dtRight
	}
	switch {
	case t.MaxLines == 1:
		p.flags |= dtSingleLine
	case t.MaxLines > 1:
		// Hide the lines past the box and end the last shown with "…".
		p.flags |= dtWordBreak | dtEditControl
	default:
		p.flags |= dtWordBreak
	}
	return p
}

// lines returns the width in pixels of the text wrapped at maxWidth and
// its number of lines, at most maxLines.
func (p *preparedText) lines(maxWidth core.Points) (int32, int32) {
	if len(p.text) == 0 {
		return 0, 1
	}
	if p.w.measureDC == 0 {
		p.w.measureDC, _, _ = createCompatibleDC.Call(0)
	}
	dc := p.w.measureDC
	old, _, _ := selectObject.Call(dc, p.font.handle)
	setTextCharacterExtra.Call(dc, uintptr(p.extra))
	r := rect{Right: 1 << 24}
	if !maxWidth.IsAuto() {
		r.Right = int32(float64(maxWidth) * p.w.scale)
	}
	limit := r.Right
	drawTextW.Call(dc, uintptr(unsafe.Pointer(&p.text[0])), uintptr(len(p.text)),
		uintptr(unsafe.Pointer(&r)), p.flags|dtCalcRect)
	setTextCharacterExtra.Call(dc, 0)
	selectObject.Call(dc, old)
	n := max((r.Bottom-r.Top+p.font.height/2)/p.font.height, 1)
	if p.maxLines > 0 {
		n = min(n, int32(p.maxLines))
	}
	return min(r.Right-r.Left, limit), n
}

func (p *preparedText) Measure(maxWidth core.Points) core.Size {
	width, n := p.lines(maxWidth)
	return core.Size{
		Width:  core.Points(float64(width) / p.w.scale),
		Height: core.Points(float64(n*p.step) / p.w.scale),
	}
}

func (p *preparedText) Layout(size core.Size) core.TextLayout {
	p.w.fonts.retain(p.font)
	width := int32(math.Ceil(float64(size.Width) * p.w.scale))
	height := int32(math.Ceil(float64(size.Height) * p.w.scale))
	// DrawTextW spaces lines by the font's height: a custom line height
	// only centers the first line in its own.
	n := max((height+p.step/2)/p.step, 1)
	if p.maxLines > 0 {
		n = min(n, int32(p.maxLines))
	}
	top := (p.step - p.font.height) / 2
	flags := p.flags
	if p.maxLines > 0 {
		flags |= dtEndEllipsis
	}
	return &textLayout{
		fonts: &p.w.fonts,
		text:  p.text,
		font:  p.font,
		color: p.color,
		flags: flags,
		extra: p.extra,
		box:   rect{0, top, width, top + n*p.font.height},
	}
}

func (p *preparedText) Release() { p.w.fonts.release(p.font) }

// textLayout is what painting needs to draw text without measuring it.
type textLayout struct {
	fonts *fontCache
	text  []uint16
	font  *font
	color core.Color
	flags uintptr
	extra int32
	box   rect // relative to the content's origin, in pixels
}

// Release implements core.TextLayout on the main thread.
func (t *textLayout) Release() { t.fonts.release(t.font) }

func (t *textLayout) draw(dc uintptr, x, y int32) {
	if len(t.text) == 0 || t.color&0xff == 0 {
		return
	}
	r := rect{x + t.box.Left, y + t.box.Top, x + t.box.Right, y + t.box.Bottom}
	old, _, _ := selectObject.Call(dc, t.font.handle)
	setTextColor.Call(dc, colorRef(t.color))
	setTextCharacterExtra.Call(dc, uintptr(t.extra))
	drawTextW.Call(dc, uintptr(unsafe.Pointer(&t.text[0])), uintptr(len(t.text)),
		uintptr(unsafe.Pointer(&r)), t.flags)
	setTextCharacterExtra.Call(dc, 0)
	selectObject.Call(dc, old)
}

// Mount implements engine.Platform.
func (w *window) Mount(b *core.MountBatch) {
	if w.batches.Push(b) {
		postMessageW.Call(w.hwnd, wmApply, 0, 0)
	}
}

func (w *window) px(p core.Points) int32 { return int32(math.Round(float64(p) * w.scale)) }

func (w *window) rect(x, y core.Points, size core.Size) rect {
	return rect{w.px(x), w.px(y), w.px(x + size.Width), w.px(y + size.Height)}
}

func (w *window) point(x, y int32) core.Point {
	return core.Point{X: core.Points(float64(x) / w.scale), Y: core.Points(float64(y) / w.scale)}
}

func (w *window) view(id core.NodeID) *view {
	if int(id) < len(w.views) {
		return w.views[id]
	}
	return nil
}

func (w *window) apply(b *core.MountBatch) {
	var rescroll []*view
	type focusChange struct {
		v     *view
		focus bool
	}
	var focus []focusChange
	for i := range b.Items {
		it := &b.Items[i]
		if it.Kind == core.ItemCreate {
			w.create(it.ID, it.View)
			continue
		}
		v := w.view(it.ID)
		if v == nil {
			continue
		}
		switch it.Kind {
		case core.ItemInsert:
			if p := w.view(it.Parent); p != nil {
				v.detach()
				at := len(p.children)
				if before := w.view(it.Before); before != nil {
					if j := slices.Index(p.children, before); j >= 0 {
						at = j
					}
				}
				p.children = slices.Insert(p.children, at, v)
				v.parent = p
			}
		case core.ItemRemove:
			v.detach()
		case core.ItemDelete:
			w.delete(v)
		case core.ItemFrame:
			v.frame = it.Frame
			if v.kind == core.KindScroll {
				rescroll = append(rescroll, v)
			}
		case core.ItemViewStyle:
			v.style = *it.ViewStyle
			if v.edit != 0 {
				invalidateRect.Call(v.edit, 0, 1) // its background may have changed
			}
		case core.ItemText:
			if v.text != nil {
				v.text.Release()
			}
			v.text, _ = it.Text.Layout.(*textLayout)
			v.content = it.Text.Content
		case core.ItemInput:
			w.configure(v, it.Input)
		case core.ItemInputValue:
			w.setValue(v, it.Value, it.Count)
		case core.ItemEvents:
			v.events = it.Events
		case core.ItemScrollConfig:
			v.scroll = *it.Scroll
		case core.ItemScrollContent:
			v.size = it.Size
			rescroll = append(rescroll, v)
		case core.ItemFocus, core.ItemBlur:
			focus = append(focus, focusChange{v, it.Kind == core.ItemFocus})
		}
	}
	w.root = b.Root
	for _, v := range rescroll {
		if !v.deleted {
			w.scrollTo(v, v.offset)
		}
	}
	w.placeEdits()
	// Focus once the EDITs are shown.
	for _, f := range focus {
		switch {
		case f.v.deleted || f.v.edit == 0:
		case f.focus:
			setFocus.Call(f.v.edit)
		default:
			if current, _, _ := getFocus.Call(); current == f.v.edit {
				setFocus.Call(w.hwnd)
			}
		}
	}
	invalidateRect.Call(w.hwnd, 0, 0)
}

func (w *window) create(id core.NodeID, kind core.ViewKind) {
	for len(w.views) <= int(id) {
		w.views = append(w.views, nil)
	}
	w.views[id] = &view{id: id, kind: kind, style: core.ViewStyle{Opacity: 1}}
}

// delete destroys v; its children get their own deletes.
func (w *window) delete(v *view) {
	v.detach()
	for _, c := range v.children {
		c.parent = nil
	}
	v.children = nil
	if v.text != nil {
		v.text.Release()
		v.text = nil
	}
	if v.edit != 0 {
		// Unsubclass first so the EDIT reports nothing while it goes.
		setWindowLongPtrW.Call(v.edit, gwlpWndProc, v.editProc)
		delete(w.edits, v.edit)
		delete(editWindows, v.edit)
		destroyWindow.Call(v.edit)
		v.edit = 0
	}
	if v.font != nil {
		w.fonts.release(v.font)
		v.font = nil
	}
	if v.brush != 0 {
		deleteObject.Call(v.brush)
		v.brush = 0
	}
	v.deleted = true
	w.views[v.id] = nil
}

// configure applies an input's look and behavior to its EDIT, made on
// the first configuration so it starts with its alignment and masking.
func (w *window) configure(v *view, in *core.InputConfig) {
	old := v.input
	v.input = *in
	if v.kind != core.KindInput {
		return
	}
	var align uintptr
	switch in.Style.Align {
	case core.TextAlignCenter:
		align = esCenter
	case core.TextAlignEnd:
		align = esRight
	}
	if v.edit == 0 {
		style := wsChild | esAutoHScroll | align
		if in.Masked {
			style |= esPassword
		}
		instance, _, _ := getModuleHandleW.Call(0)
		v.edit, _, _ = createWindowExW.Call(0, uintptr(unsafe.Pointer(wide("EDIT"))), 0,
			style, 0, 0, 0, 0, w.hwnd, uintptr(v.id), instance, 0)
		if v.edit == 0 {
			return
		}
		w.edits[v.edit] = v
		editWindows[v.edit] = w
		v.editProc, _, _ = setWindowLongPtrW.Call(v.edit, gwlpWndProc, editProcCallback)
	} else if in.Style.Align != old.Style.Align {
		// Best effort: not every EDIT realigns after creation.
		style, _, _ := getWindowLongPtrW.Call(v.edit, gwlStyle)
		setWindowLongPtrW.Call(v.edit, gwlStyle, style&^(esCenter|esRight)|align)
	}
	if key := (fontKey{in.Style.Font, in.Style.Decoration}); v.font == nil || v.font.key != key {
		f := w.fonts.acquire(key)
		sendMessageW.Call(v.edit, wmSetFont, f.handle, 1)
		sendMessageW.Call(v.edit, emSetMargins, ecBothMargins, 0)
		if v.font != nil {
			w.fonts.release(v.font)
		}
		v.font = f
	}
	var mask uintptr
	if in.Masked {
		mask = maskChar
	}
	sendMessageW.Call(v.edit, emSetPasswordChar, mask, 0)
	sendMessageW.Call(v.edit, emSetReadOnly, boolArg(in.Readonly), 0)
	enableWindow.Call(v.edit, boolArg(!in.Disabled))
	invalidateRect.Call(v.edit, 0, 1)
}

// setValue sets an input's text unless the user edited it after edit
// number count.
func (w *window) setValue(v *view, value string, count uint32) {
	if v.edit == 0 || v.edits > count || editText(v.edit) == value {
		return
	}
	v.setting = true
	setWindowTextW.Call(v.edit, uintptr(unsafe.Pointer(wide(value))))
	v.setting = false
	end := uintptr(len(utf16.Encode([]rune(value))))
	sendMessageW.Call(v.edit, emSetSel, end, end)
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// placeEdits moves each EDIT to its input's content, centered on a line
// and clipped by scroll views and clipping ancestors, and hides the ones
// of inputs that are hidden, detached or clipped away.
func (w *window) placeEdits() {
	if len(w.edits) == 0 {
		return
	}
	w.pass++
	if root := w.view(w.root); root != nil {
		w.placeIn(root, 0, 0, everywhere)
	}
	for edit, v := range w.edits {
		if v.placed != w.pass && v.shown {
			showWindow.Call(edit, swHide)
			v.shown = false
		}
	}
}

func (w *window) placeIn(v *view, x, y core.Points, clip rect) {
	if v.style.Hidden || v.style.Opacity <= 0 {
		return
	}
	x, y = x+v.frame.Origin.X, y+v.frame.Origin.Y
	if v.edit != 0 {
		w.placeEdit(v, x, y, clip)
	}
	if v.style.Clip || v.kind == core.KindScroll {
		clip = clip.intersect(w.rect(x, y, v.frame.Size))
	}
	for _, c := range v.children {
		w.placeIn(c, x-v.offset.X, y-v.offset.Y, clip)
	}
}

func (w *window) placeEdit(v *view, x, y core.Points, clip rect) {
	c := v.input.Content
	r := w.rect(x+c.Origin.X, y+c.Origin.Y, c.Size)
	if v.font != nil {
		if line := v.font.height; r.Bottom-r.Top > line {
			r.Top += (r.Bottom - r.Top - line) / 2
			r.Bottom = r.Top + line
		}
	}
	visible := r.intersect(clip)
	if visible.empty() {
		return
	}
	v.placed = w.pass
	if r != v.editRect {
		moveWindow.Call(v.edit, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 1)
		v.editRect = r
	}
	visible = rect{visible.Left - r.Left, visible.Top - r.Top, visible.Right - r.Left, visible.Bottom - r.Top}
	if visible != v.editClip {
		var rgn uintptr
		if visible != (rect{0, 0, r.Right - r.Left, r.Bottom - r.Top}) {
			// The EDIT owns the region from now on.
			rgn, _, _ = createRectRgn.Call(uintptr(visible.Left), uintptr(visible.Top), uintptr(visible.Right), uintptr(visible.Bottom))
		}
		setWindowRgn.Call(v.edit, rgn, 1)
		v.editClip = visible
	}
	if !v.shown {
		showWindow.Call(v.edit, swShowNA)
		v.shown = true
	}
}

// scrollTo moves v's content to offset, clamped to the content, and
// reports whether it moved. The engine hit tests with the offset, so a
// move is always reported, whatever v's events.
func (w *window) scrollTo(v *view, offset core.Point) bool {
	offset.X = min(max(offset.X, 0), max(v.size.Width-v.frame.Size.Width, 0))
	offset.Y = min(max(offset.Y, 0), max(v.size.Height-v.frame.Size.Height, 0))
	if offset == v.offset {
		return false
	}
	v.offset = offset
	w.engine.Dispatch(core.Event{Kind: core.EventScroll, ID: v.id, Point: offset})
	return true
}

// wheel scrolls the innermost scroll view under at that can still move
// in the wheel's direction. Tilting scrolls horizontal views only.
func (w *window) wheel(at core.Point, notches core.Points, tilt bool) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	under := scrollsAt(root, at, nil)
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		offset := v.offset
		switch {
		case tilt && v.scroll.Horizontal: // a tilt right is positive
			offset.X += notches * wheelStep
		case tilt:
			continue
		case v.scroll.Horizontal: // a turn away from the user is positive
			offset.X -= notches * wheelStep
		default:
			offset.Y -= notches * wheelStep
		}
		if w.scrollTo(v, offset) {
			w.placeEdits()
			invalidateRect.Call(w.hwnd, 0, 0)
			return
		}
	}
}

// scrollsAt appends the scroll views under p, outermost first; p is in
// the coordinates of v's parent's content.
func scrollsAt(v *view, p core.Point, out []*view) []*view {
	if v.style.Hidden || v.style.Opacity <= 0 || !v.frame.Contains(p) {
		return out
	}
	if v.kind == core.KindScroll {
		out = append(out, v)
	}
	p = core.Point{X: p.X - v.frame.Origin.X + v.offset.X, Y: p.Y - v.frame.Origin.Y + v.offset.Y}
	for i := len(v.children) - 1; i >= 0; i-- {
		if c := v.children[i]; !c.style.Hidden && c.frame.Contains(p) {
			return scrollsAt(c, p, out)
		}
	}
	return out
}

func colorRef(c core.Color) uintptr {
	return uintptr(c>>24&0xff | (c>>16&0xff)<<8 | (c>>8&0xff)<<16)
}

// background is the color behind v: its own or its nearest ancestor's.
func background(v *view) core.Color {
	for ; v != nil; v = v.parent {
		if v.style.Background&0xff != 0 {
			return v.style.Background
		}
	}
	return core.RGB(0xffffff)
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
	white, _, _ := getStockObject.Call(whiteBrush)
	fillRect.Call(mem, uintptr(unsafe.Pointer(&client)), white)
	setBkMode.Call(mem, transparent)
	if root := w.view(w.root); root != nil {
		w.draw(mem, root, 0, 0)
	}
	bitBlt.Call(hdc, 0, 0, uintptr(client.Right), uintptr(client.Bottom), mem, 0, 0, srcCopy)
	selectObject.Call(mem, old)
	deleteObject.Call(bmp)
	deleteDC.Call(mem)
}

// draw paints v and its children; x and y are the origin of v's parent's
// content in the window.
func (w *window) draw(dc uintptr, v *view, x, y core.Points) {
	s := &v.style
	if s.Hidden || s.Opacity <= 0 {
		return
	}
	x, y = x+v.frame.Origin.X, y+v.frame.Origin.Y
	r := w.rect(x, y, v.frame.Size)
	diameter := min(2*w.px(s.CornerRadius), r.Right-r.Left, r.Bottom-r.Top)
	w.fill(dc, r, diameter, s)
	if v.text != nil {
		v.text.draw(dc, w.px(x+v.content.Origin.X), w.px(y+v.content.Origin.Y))
	}
	if len(v.children) == 0 {
		return
	}
	if s.Clip || v.kind == core.KindScroll {
		saved, _, _ := saveDC.Call(dc)
		defer restoreDC.Call(dc, saved)
		var rgn uintptr
		if diameter > 0 {
			rgn, _, _ = createRoundRectRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1),
				uintptr(diameter), uintptr(diameter))
		} else {
			rgn, _, _ = createRectRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
		}
		extSelectClipRgn.Call(dc, rgn, rgnAnd) // copies the region
		deleteObject.Call(rgn)
	}
	for _, c := range v.children {
		w.draw(dc, c, x-v.offset.X, y-v.offset.Y)
	}
	if v.kind == core.KindScroll && !v.scroll.HideIndicator {
		w.drawIndicator(dc, v, x, y)
	}
}

// fill paints a view's background and border, a border drawn inside r.
func (w *window) fill(dc uintptr, r rect, diameter int32, s *core.ViewStyle) {
	border := s.BorderWidth > 0 && s.BorderColor&0xff != 0
	if s.Background&0xff == 0 && !border {
		return
	}
	brush, _, _ := getStockObject.Call(nullBrush)
	if s.Background&0xff != 0 {
		brush, _, _ = createSolidBrush.Call(colorRef(s.Background))
		defer deleteObject.Call(brush)
	}
	pen, _, _ := getStockObject.Call(nullPen)
	right, bottom := r.Right, r.Bottom
	if border {
		pen, _, _ = createPen.Call(psInsideFrame, uintptr(max(w.px(s.BorderWidth), 1)), colorRef(s.BorderColor))
		defer deleteObject.Call(pen)
	} else {
		// A null pen leaves out the right and bottom edges.
		right, bottom = right+1, bottom+1
	}
	oldBrush, _, _ := selectObject.Call(dc, brush)
	oldPen, _, _ := selectObject.Call(dc, pen)
	roundRect.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(right), uintptr(bottom), uintptr(diameter), uintptr(diameter))
	selectObject.Call(dc, oldBrush)
	selectObject.Call(dc, oldPen)
}

// drawIndicator paints a scroll bar's thumb along v's scrolling edge;
// x and y are v's origin in the window.
func (w *window) drawIndicator(dc uintptr, v *view, x, y core.Points) {
	const thickness, inset, minLength = 3, 2, 20
	frame, content, offset := v.frame.Size.Height, v.size.Height, v.offset.Y
	if v.scroll.Horizontal {
		frame, content, offset = v.frame.Size.Width, v.size.Width, v.offset.X
	}
	if content <= frame || frame <= 0 {
		return
	}
	length := min(max(frame*frame/content, minLength), frame)
	at := offset / (content - frame) * (frame - length)
	var bar rect
	if v.scroll.Horizontal {
		bar = w.rect(x+at, y+v.frame.Size.Height-inset-thickness, core.Size{Width: length, Height: thickness})
	} else {
		bar = w.rect(x+v.frame.Size.Width-inset-thickness, y+at, core.Size{Width: thickness, Height: length})
	}
	brush, _, _ := createSolidBrush.Call(colorRef(core.RGB(0x8c8c8c)))
	fillRect.Call(dc, uintptr(unsafe.Pointer(&bar)), brush)
	deleteObject.Call(brush)
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
		if wParam != sizeMinimized {
			width, height := int32(lParam&0xffff), int32(lParam>>16&0xffff)
			size := w.point(width, height)
			w.engine.Resize(core.Size{Width: size.X, Height: size.Y})
		}
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		w.paint()
		return 0
	case wmLButtonUp:
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		w.engine.PressAt(w.point(int32(x), int32(y)))
		return 0
	case wmMouseWheel, wmMouseHWheel:
		// Wheel messages carry screen coordinates.
		at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
		screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&at)))
		notches := core.Points(int16(wParam>>16&0xffff)) / 120
		w.wheel(w.point(at.X, at.Y), notches, message == wmMouseHWheel)
		return 0
	case wmCommand:
		if v := w.edits[lParam]; v != nil {
			w.editCommand(v, wParam>>16&0xffff)
		}
		return 0
	case wmCtlColorEdit, wmCtlColorStatic:
		if v := w.edits[lParam]; v != nil {
			bg := background(v)
			if v.brush == 0 || v.brushColor != bg {
				if v.brush != 0 {
					deleteObject.Call(v.brush)
				}
				v.brush, _, _ = createSolidBrush.Call(colorRef(bg))
				v.brushColor = bg
			}
			setTextColor.Call(wParam, colorRef(v.input.Style.Color))
			setBkColor.Call(wParam, colorRef(bg))
			return v.brush
		}
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	}
	r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

// editCommand handles an EDIT's notification.
func (w *window) editCommand(v *view, code uintptr) {
	switch code {
	case enChange:
		if v.input.Placeholder != "" {
			invalidateRect.Call(v.edit, 0, 1)
		}
		if !v.setting {
			v.edits++
			w.engine.Dispatch(core.Event{Kind: core.EventChangeText, ID: v.id, Text: editText(v.edit), Count: v.edits})
		}
	case enSetFocus:
		if v.events.Has(core.EventFocus) {
			w.engine.Dispatch(core.Event{Kind: core.EventFocus, ID: v.id})
		}
	case enKillFocus:
		if v.events.Has(core.EventBlur) {
			w.engine.Dispatch(core.Event{Kind: core.EventBlur, ID: v.id})
		}
	}
}

// editProc subclasses EDIT controls to report Return as a submit and to
// draw placeholders, which plain EDITs lack.
func editProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	w := editWindows[hwnd]
	if w == nil {
		r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	v := w.edits[hwnd]
	if v == nil {
		r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	if wParam == vkReturn && (message == wmKeyDown || message == wmChar) {
		if message == wmKeyDown && v.events.Has(core.EventSubmitEditing) {
			w.engine.Dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.id, Text: editText(hwnd)})
		}
		return 0 // no beep
	}
	r, _, _ := callWindowProcW.Call(v.editProc, hwnd, uintptr(message), wParam, lParam)
	if message == wmPaint && v.input.Placeholder != "" {
		if n, _, _ := getWindowTextLength.Call(hwnd); n == 0 {
			drawPlaceholder(v)
		}
	}
	return r
}

func drawPlaceholder(v *view) {
	hideCaret.Call(v.edit)
	defer showCaret.Call(v.edit)
	dc, _, _ := getDC.Call(v.edit)
	defer releaseDC.Call(v.edit, dc)
	var r rect
	getClientRect.Call(v.edit, uintptr(unsafe.Pointer(&r)))
	if v.font != nil {
		old, _, _ := selectObject.Call(dc, v.font.handle)
		defer selectObject.Call(dc, old)
	}
	gray, _, _ := getSysColor.Call(colorGrayText)
	setBkMode.Call(dc, transparent)
	setTextColor.Call(dc, gray)
	flags := uintptr(dtSingleLine | dtNoPrefix | dtEndEllipsis)
	switch v.input.Style.Align {
	case core.TextAlignCenter:
		flags |= dtCenter
	case core.TextAlignEnd:
		flags |= dtRight
	}
	drawTextW.Call(dc, uintptr(unsafe.Pointer(wide(v.input.Placeholder))), ^uintptr(0),
		uintptr(unsafe.Pointer(&r)), flags)
}
