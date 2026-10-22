//go:build windows

// Package windows is NSGo's Win32 backend: one window that draws a
// retained tree of views with GDI into a 32-bit bitmap, text the engine
// measured and laid out with GDI fonts, presses hit tested by the engine,
// inputs as native EDIT controls, images decoded by Go, canvases drawn
// antialiased by GDI+, and alerts as task dialogs. It uses only the
// syscall package, so it builds without cgo.
//
// GDI has no alpha and cannot blend a fractional edge. Translucent colors
// fill their clipped shape with AlphaBlend, translucent views draw into a
// layer blended back at their opacity, and the painter measures rounded
// corners itself, blending them pixel by pixel. Canvases, whose paths are
// arbitrary, are drawn by GDI+ into a bitmap the painter blends back.
// Transforms are GDI world transforms. Style transitions tick on a timer.
// EDIT controls ignore transforms and opacity; shadows, image blur and
// modal animations are not drawn, and there is no accessibility yet, which
// needs UI Automation providers.
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
	// ID names the app's directories and storage (engine.AppDirs).
	ID string
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
	wmSettingChange  = 0x001A
	wmSetFont        = 0x0030
	wmKeyDown        = 0x0100
	wmChar           = 0x0102
	wmSysKeyDown     = 0x0104
	wmCommand        = 0x0111
	wmCtlColorEdit   = 0x0133
	wmCtlColorStatic = 0x0138 // what read-only and disabled EDITs send
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmRButtonDown    = 0x0204
	wmMouseWheel     = 0x020A
	wmMouseHWheel    = 0x020E
	wmAppCommand     = 0x0319     // what DefWindowProc makes of XBUTTON1 and back keys
	wmApply          = 0x8000 + 1 // WM_APP + 1: batches are queued
	wmRun            = 0x8000 + 2 // functions are queued
	wmAlert          = 0x8000 + 3 // alerts are queued

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
	vkEscape           = 0x1B
	vkLeft             = 0x25
	appCommandBack     = 1      // APPCOMMAND_BROWSER_BACKWARD
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

// fontName returns the GDI face of a family name.
func fontName(family string) string {
	switch family {
	case "":
		return defaultFontName
	case core.FontMonospace:
		return "Consolas"
	}
	return family
}

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
	props    core.ViewProps
	deleted  bool

	// A text view's layout and where it goes in the view.
	text    drawnText
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

	// An image view's configuration, source and placeholder.
	image               core.ImageConfig
	source, placeholder imageSlot

	// A canvas's drawing.
	drawing *core.Drawing

	// What animates: the style, after the first, and the scroll offset.
	styled     bool
	transition *transition
	scrolling  *scrolling
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
	title   string
	engine  *engine.Engine
	batches queue.Batches
	scale   float64 // pixels per point
	fonts   fontCache

	// What other goroutines hand the main thread.
	mu    sync.Mutex
	tasks []func()

	// The main thread's.
	views     []*view // index is NodeID
	root      core.NodeID
	edits     map[uintptr]*view
	pass      uint32
	env       core.Environment // the last reported
	alerts    []core.AlertConfig
	alerting  bool
	keys      map[string]bool // the keyboard shortcuts
	animating []*view
	ticking   bool
	chrome    chrome

	// The gestures in progress.
	pan                            *pan
	pinch, rotation, wheelPinching *twoFinger

	measureDC uintptr // the engine thread's
	aa        *smoothing
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
	w := &window{title: opts.Title, scale: 1, edits: map[uintptr]*view{}}
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
	w.configureGestures()
	w.env = w.environment()
	w.darkTitleBar(w.env.Dark)
	// The engine starts once the window exists, so Mount can post to it.
	eopts := engine.Options{
		Size:        core.Size{Width: opts.Width, Height: opts.Height},
		Environment: w.env,
	}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	w.engine = engine.Start(w, app, eopts)
	showWindow.Call(hwnd, swShow)

	var m msg
	for {
		r, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			w.engine.Close()
			engine.SyncStorage()
			return
		}
		// Shortcuts take key presses before they become characters.
		if (m.Message == wmKeyDown || m.Message == wmSysKeyDown) && (m.Hwnd == hwnd || editWindows[m.Hwnd] == w) &&
			w.shortcut(m.WParam) {
			continue
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
	ascent int32
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
	family := fontName(key.font.Family)
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
	f := &font{key: key, handle: h, height: max(tm.Height, 1), ascent: tm.Ascent, refs: 1}
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
	if len(t.Spans) > 0 {
		return w.prepareSpans(t)
	}
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

// post runs fn on the main thread; any goroutine may call it.
func (w *window) post(fn func()) {
	w.mu.Lock()
	wake := len(w.tasks) == 0
	w.tasks = append(w.tasks, fn)
	w.mu.Unlock()
	if wake {
		postMessageW.Call(w.hwnd, wmRun, 0, 0)
	}
}

func (w *window) runTasks() {
	w.mu.Lock()
	fns := w.tasks
	w.tasks = nil
	w.mu.Unlock()
	for _, fn := range fns {
		fn()
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
	var rescroll, loads []*view
	type focusChange struct {
		v     *view // nil dismisses the keyboard
		focus bool
	}
	var focus []focusChange
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			w.create(it.ID, it.View)
			continue
		case core.ItemAlert:
			// Shown from a message of its own: the dialog's message loop
			// must not run inside this one's batch.
			w.alerts = append(w.alerts, *it.Alert)
			postMessageW.Call(w.hwnd, wmAlert, 0, 0)
			continue
		case core.ItemDismissKeyboard:
			focus = append(focus, focusChange{})
			continue
		case core.ItemSetClipboard:
			w.setClipboard(it.Value)
			continue
		case core.ItemReadClipboard:
			w.engine.Dispatch(core.Event{Kind: core.EventClipboard, Count: it.Count, Text: w.clipboard()})
			continue
		case core.ItemKeymap:
			w.setKeymap(it.Keys)
			continue
		case core.ItemTitle:
			w.title = it.Value
			setWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(wide(it.Value))))
			continue
		case core.ItemWindow:
			if it.Window != nil {
				w.windowCommand(it.Window)
			}
			continue
		case core.ItemMenu:
			w.setMenu(it.Menu)
			continue
		case core.ItemAnnounce, core.ItemStatusBar, core.ItemBackUnhandled:
			// No announcements without UI Automation; no status bar.
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
			w.setStyle(v, it.ViewStyle)
			if v.edit != 0 {
				invalidateRect.Call(v.edit, 0, 1) // its background may have changed
			}
		case core.ItemText:
			if v.text != nil {
				v.text.Release()
			}
			v.text, _ = it.Text.Layout.(drawnText)
			v.content = it.Text.Content
		case core.ItemInput:
			w.configure(v, it.Input)
		case core.ItemInputValue:
			w.setValue(v, it.Value, it.Count)
		case core.ItemEvents:
			v.events = it.Events
		case core.ItemViewProps:
			if it.ViewProps != nil {
				v.props = *it.ViewProps
			}
		case core.ItemScrollConfig:
			v.scroll = *it.Scroll
		case core.ItemScrollContent:
			v.size = it.Size
			rescroll = append(rescroll, v)
		case core.ItemFocus, core.ItemBlur:
			focus = append(focus, focusChange{v, it.Kind == core.ItemFocus})
		case core.ItemImage:
			if w.setImage(v, it.Image) {
				loads = append(loads, v)
			}
		case core.ItemModal:
			// A modal is a view; it appears without animation.
		case core.ItemCanvas:
			v.drawing = it.Drawing
		case core.ItemScrollTo:
			v.scrolling = nil
			if it.Animated {
				w.scrollAnimated(v, it.Point)
			} else {
				w.scrollTo(v, it.Point)
			}
		}
	}
	w.root = b.Root
	for _, v := range rescroll {
		if !v.deleted {
			w.scrollTo(v, v.offset)
		}
	}
	for _, v := range loads {
		if !v.deleted {
			w.load(v, &v.placeholder, false)
			w.load(v, &v.source, true)
		}
	}
	w.placeEdits()
	// Focus once the EDITs are shown.
	for _, f := range focus {
		switch {
		case f.v == nil:
			if current, _, _ := getFocus.Call(); w.edits[current] != nil {
				setFocus.Call(w.hwnd)
			}
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
	v.source.reset(core.ImageSource{})
	v.placeholder.reset(core.ImageSource{})
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
	if v.kind == core.KindModal {
		w.cover(w.rect(x, y, v.frame.Size).intersect(clip))
	}
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

// cover hides the EDITs placed so far in this pass that a modal drawn
// over them covers, in r, and takes the focus from them.
func (w *window) cover(r rect) {
	for edit, v := range w.edits {
		if v.placed == w.pass && !v.editRect.intersect(r).empty() {
			v.placed = w.pass - 1
			if current, _, _ := getFocus.Call(); current == edit {
				setFocus.Call(w.hwnd)
			}
		}
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
		v.scrolling = nil // the wheel takes over
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

// pressOutside reports a press at x, y in the client area to each view
// that asks for presses outside it and does not contain the point. It
// does not take the press: the caller handles it as before.
func (w *window) pressOutside(x, y int32) {
	at := w.point(x, y)
	for _, v := range w.views {
		if v == nil || !v.events.Has(core.EventPressOutside) {
			continue
		}
		if b, shown := w.surfaceBounds(v); shown && !b.Contains(at) {
			w.engine.Dispatch(core.Event{Kind: core.EventPressOutside, ID: v.id, Point: at})
		}
	}
}

// surfaceBounds returns v's frame in the surface, through its ancestors'
// frames and scroll offsets, and whether v is in the root's tree.
func (w *window) surfaceBounds(v *view) (core.Bounds, bool) {
	b, top := v.frame, v
	for p := v.parent; p != nil; p = p.parent {
		b.Origin.X += p.frame.Origin.X - p.offset.X
		b.Origin.Y += p.frame.Origin.Y - p.offset.Y
		top = p
	}
	return b, top == w.view(w.root)
}

func (w *window) back() { w.engine.Dispatch(core.Event{Kind: core.EventBack}) }

// repeated reports whether a key message's key was already down.
func repeated(lParam uintptr) bool { return lParam&(1<<30) != 0 }

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
	case wmRun:
		w.runTasks()
		return 0
	case wmAlert:
		w.showAlert()
		return 0
	case wmSettingChange:
		// Theme ("ImmersiveColorSet"), locale and accessibility changes.
		w.updateEnvironment()
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
	case wmLButtonDown, wmRButtonDown:
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		w.pressOutside(int32(x), int32(y))
		if message == wmLButtonDown {
			w.panDown(w.point(int32(x), int32(y)))
		}
	case wmMouseMove:
		w.setHover(0) // off the caption buttons
		if wParam&mkLButton != 0 && w.pan != nil {
			x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
			w.panMove(w.point(int32(x), int32(y)))
		}
		return 0
	case wmLButtonUp:
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		if at := w.point(int32(x), int32(y)); !w.panUp(at) {
			w.press(at)
		}
		return 0
	case wmCaptureChanged:
		w.panLost()
		return 0
	case wmGesture:
		if w.gesture(lParam) {
			return 0
		}
	case wmNCCalcSize:
		if r, ok := w.ncCalcSize(wParam, lParam); ok {
			return r
		}
	case wmNCHitTest:
		return w.ncHitTest(lParam)
	case wmNCActivate:
		return w.ncActivate(wParam, lParam)
	case wmNCMouseMove, wmNCMouseLeave, wmNCLButtonDown, wmNCLButtonUp, wmNCLButtonDblClk:
		if w.ncMouse(message, wParam) {
			return 0
		}
	case wmContextMenu:
		if lParam&0xffffffff != contextFromKey&0xffffffff {
			at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
			screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&at)))
			w.contextMenu(w.point(at.X, at.Y))
		}
		return 0
	case wmTimer:
		switch wParam {
		case animationTimer:
			w.tick()
		case pinchTimer:
			w.wheelPinchEnded()
		}
		return 0
	case wmKeyDown:
		// The window has the keyboard only when no EDIT does.
		if wParam == vkEscape && !repeated(lParam) {
			w.back()
			return 0
		}
	case wmSysKeyDown:
		// Alt+Left; DefWindowProc still sees it, or releasing Alt would
		// open the window menu.
		if wParam == vkLeft && !repeated(lParam) {
			w.back()
		}
	case wmAppCommand:
		if lParam>>16&0x0fff == appCommandBack {
			w.back()
			return 1
		}
	case wmMouseWheel, wmMouseHWheel:
		// Wheel messages carry screen coordinates.
		at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
		screenToClient.Call(hwnd, uintptr(unsafe.Pointer(&at)))
		delta := int16(wParam >> 16 & 0xffff)
		// A precision touchpad's pinch comes as turns with Control held.
		if message == wmMouseWheel && wParam&mkControl != 0 && w.wheelPinch(w.point(at.X, at.Y), delta) {
			return 0
		}
		w.wheel(w.point(at.X, at.Y), core.Points(delta)/120, message == wmMouseHWheel)
		return 0
	case wmCommand:
		if v := w.edits[lParam]; v != nil {
			w.editCommand(v, wParam>>16&0xffff)
		} else if lParam == 0 && wParam>>16&0xffff == 0 {
			w.menuCommand(uint16(wParam))
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
	switch {
	case message == wmSysKeyDown && wParam == vkLeft && !repeated(lParam):
		w.back()
	case message == wmLButtonDown || message == wmRButtonDown:
		// The EDIT has no border: its client area starts at editRect.
		x, y := int16(lParam&0xffff), int16(lParam>>16&0xffff)
		w.pressOutside(v.editRect.Left+int32(x), v.editRect.Top+int32(y))
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
