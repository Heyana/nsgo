//go:build windows

package windows

// Windowless RichEdit (msftedit.dll) hosting via ITextServices2 and ITextHost2, drawn into the view's Direct2D device context with TxDrawD2D.
//
// Editing, multi-level undo/redo, native OS spell checking (IMF_SPELLCHECKING), password masking (TXTBIT_USEPASSWORD), selection, word breaks, and TSF/IMM32 input methods run through Windows' own RichEdit engine, without creating a child HWND so the input clips, transforms, and layers with the rest of the view tree. On architectures or environments where msftedit.dll or ITextServices2 is unavailable (such as 32-bit x86, where ITextHost uses __thiscall, or unit tests without a window), textInput falls back to editModel + DirectWrite.

import (
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	msftedit                 = syscall.NewLazyDLL("msftedit.dll")
	createTextServicesProc   = msftedit.NewProc("CreateTextServices")
	shutdownTextServicesProc = msftedit.NewProc("ShutdownTextServices")

	richEditGetDC       = user32.NewProc("GetDC")
	richEditReleaseDC   = user32.NewProc("ReleaseDC")
	richEditGetSysColor = user32.NewProc("GetSysColor")
	sysStringLen        = oleaut32.NewProc("SysStringLen")

	iidTextServices  = parseGUID(iidTextServicesStr)
	iidTextServices2 = parseGUID(iidTextServices2Str)
	iidTextHost      = parseGUID(iidTextHostStr)
	iidTextHost2     = parseGUID(iidTextHost2Str)
)

// richEditTimerBase is the first Win32 timer ID reserved on a window for RichEdit's TxSetTimer callbacks (window's own timers use 1..5).
const richEditTimerBase uintptr = 0x1000

// richEditTimerEntry maps a window timer ID back to a RichEdit host and its internal timer ID.
type richEditTimerEntry struct {
	in     *textInput
	richID uintptr
}

// richEditAvailable reports whether this process can host 64-bit windowless RichEdit via msftedit.dll's CreateTextServices. On 32-bit x86, ITextHost and ITextServices use __thiscall (this in ECX), so 32-bit falls back cleanly.
var richEditAvailable = sync.OnceValue(func() bool {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return false
	}
	return createTextServicesProc.Find() == nil
})

// richEditHost implements ITextHost and ITextHost2 for one textInput. Its first field MUST be vtbl so its address is a valid COM interface pointer.
type richEditHost struct {
	vtbl *[th2VtblSize]uintptr
	refs atomic.Int32
	pin  runtime.Pinner

	in  *textInput
	unk *comObject // IUnknown returned by CreateTextServices
	ts2 *comObject // ITextServices2 queried from unk

	cf charFormat2W
	pf paraFormat2

	width, height float32 // content box size in DIPs
	yOffset       float32 // single-line vertical centering offset in DIPs
	activated     bool
	composing     bool

	// suppressNotify silences re-entrant TxNotify callbacks while NSGo applies programmatic text/selection/config updates to RichEdit.
	suppressNotify int

	caret  richEditCaretState
	scroll richEditScrollState
}

var (
	richEditHostsMu sync.Mutex
	richEditHosts   = map[*richEditHost]struct{}{}
)

var richEditHostVtbl [th2VtblSize]uintptr

func init() {
	richEditHostVtbl = [th2VtblSize]uintptr{
		thQueryInterface:                  syscall.NewCallback(reHostQueryInterface),
		thAddRef:                          syscall.NewCallback(reHostAddRef),
		thRelease:                         syscall.NewCallback(reHostRelease),
		thTxGetDC:                         syscall.NewCallback(reHostTxGetDC),
		thTxReleaseDC:                     syscall.NewCallback(reHostTxReleaseDC),
		thTxShowScrollBar:                 syscall.NewCallback(reHostTxShowScrollBar),
		thTxEnableScrollBar:               syscall.NewCallback(reHostTxEnableScrollBar),
		thTxSetScrollRange:                syscall.NewCallback(reHostTxSetScrollRange),
		thTxSetScrollPos:                  syscall.NewCallback(reHostTxSetScrollPos),
		thTxInvalidateRect:                syscall.NewCallback(reHostTxInvalidateRect),
		thTxViewChange:                    syscall.NewCallback(reHostTxViewChange),
		thTxCreateCaret:                   syscall.NewCallback(reHostTxCreateCaret),
		thTxShowCaret:                     syscall.NewCallback(reHostTxShowCaret),
		thTxSetCaretPos:                   syscall.NewCallback(reHostTxSetCaretPos),
		thTxSetTimer:                      syscall.NewCallback(reHostTxSetTimer),
		thTxKillTimer:                     syscall.NewCallback(reHostTxKillTimer),
		thTxScrollWindowEx:                syscall.NewCallback(reHostTxScrollWindowEx),
		thTxSetCapture:                    syscall.NewCallback(reHostTxSetCapture),
		thTxSetFocus:                      syscall.NewCallback(reHostTxSetFocus),
		thTxSetCursor:                     syscall.NewCallback(reHostTxSetCursor),
		thTxScreenToClient:                syscall.NewCallback(reHostTxScreenToClient),
		thTxClientToScreen:                syscall.NewCallback(reHostTxClientToScreen),
		thTxActivate:                      syscall.NewCallback(reHostTxActivate),
		thTxDeactivate:                    syscall.NewCallback(reHostTxDeactivate),
		thTxGetClientRect:                 syscall.NewCallback(reHostTxGetClientRect),
		thTxGetViewInset:                  syscall.NewCallback(reHostTxGetViewInset),
		thTxGetCharFormat:                 syscall.NewCallback(reHostTxGetCharFormat),
		thTxGetParaFormat:                 syscall.NewCallback(reHostTxGetParaFormat),
		thTxGetSysColor:                   syscall.NewCallback(reHostTxGetSysColor),
		thTxGetBackStyle:                  syscall.NewCallback(reHostTxGetBackStyle),
		thTxGetMaxLength:                  syscall.NewCallback(reHostTxGetMaxLength),
		thTxGetScrollBars:                 syscall.NewCallback(reHostTxGetScrollBars),
		thTxGetPasswordChar:               syscall.NewCallback(reHostTxGetPasswordChar),
		thTxGetAcceleratorPos:             syscall.NewCallback(reHostTxGetAcceleratorPos),
		thTxGetExtent:                     syscall.NewCallback(reHostTxGetExtent),
		thOnTxCharFormatChange:            syscall.NewCallback(reHostOnTxCharFormatChange),
		thOnTxParaFormatChange:            syscall.NewCallback(reHostOnTxParaFormatChange),
		thTxGetPropertyBits:               syscall.NewCallback(reHostTxGetPropertyBits),
		thTxNotify:                        syscall.NewCallback(reHostTxNotify),
		thTxImmGetContext:                 syscall.NewCallback(reHostTxImmGetContext),
		thTxImmReleaseContext:             syscall.NewCallback(reHostTxImmReleaseContext),
		thTxGetSelectionBarWidth:          syscall.NewCallback(reHostTxGetSelectionBarWidth),
		th2TxIsDoubleClickPending:         syscall.NewCallback(reHostTxIsDoubleClickPending),
		th2TxGetWindow:                    syscall.NewCallback(reHostTxGetWindow),
		th2TxSetForegroundWindow:          syscall.NewCallback(reHostTxSetForegroundWindow),
		th2TxGetPalette:                   syscall.NewCallback(reHostTxGetPalette),
		th2TxGetEastAsianFlags:            syscall.NewCallback(reHostTxGetEastAsianFlags),
		th2TxSetCursor2:                   syscall.NewCallback(reHostTxSetCursor2),
		th2TxFreeTextServicesNotification: syscall.NewCallback(reHostTxFreeTextServicesNotification),
		th2TxGetEditStyle:                 syscall.NewCallback(reHostTxGetEditStyle),
		th2TxGetWindowStyles:              syscall.NewCallback(reHostTxGetWindowStyles),
		th2TxShowDropCaret:                syscall.NewCallback(reHostTxShowDropCaret),
		th2TxDestroyCaret:                 syscall.NewCallback(reHostTxDestroyCaret),
		th2TxGetHorzExtent:                syscall.NewCallback(reHostTxGetHorzExtent),
	}
}

// newRichEditHost creates a windowless RichEdit ITextServices2 attached to in. It returns nil if RichEdit/ITextServices2 is unavailable or in has no HWND.
func newRichEditHost(in *textInput) *richEditHost {
	if in == nil || in.w == nil || in.w.hwnd == 0 || !richEditAvailable() {
		return nil
	}
	h := &richEditHost{
		vtbl:   &richEditHostVtbl,
		in:     in,
		width:  max(in.width, 1),
		height: max(in.height, 1),
	}
	h.cf = buildCharFormat2W(in.config, fontName(in.config.Style.Font.Family))
	h.pf = buildParaFormat2(in.config)
	h.refs.Store(1)
	h.pin.Pin(h)
	h.pin.Pin(&h.cf)
	h.pin.Pin(&h.pf)
	richEditHostsMu.Lock()
	richEditHosts[h] = struct{}{}
	richEditHostsMu.Unlock()

	var unk *comObject
	hr, _, _ := createTextServicesProc.Call(0, uintptr(unsafe.Pointer(h)), uintptr(unsafe.Pointer(&unk)))
	if failed(hr) || unk == nil {
		h.in = nil
		reHostRelease(h)
		return nil
	}
	ts2 := unk.query(&iidTextServices2)
	if ts2 == nil {
		if shutdownTextServicesProc.Find() == nil {
			shutdownTextServicesProc.Call(unk.ptr())
		} else {
			unk.release()
		}
		h.in = nil
		reHostRelease(h)
		return nil
	}
	h.unk = unk
	h.ts2 = ts2

	h.suppressNotify++
	h.applyAllConfig()
	rc := h.clientRect()
	h.ts2.call(tsOnTxInPlaceActivate, uintptr(unsafe.Pointer(&rc)))
	h.activated = true
	h.suppressNotify--

	return h
}

// close shuts down the ITextServices2 instance, kills any active host timers, and releases our reference on the host COM object.
func (h *richEditHost) close() {
	if h == nil {
		return
	}
	if h.in != nil && h.in.w != nil {
		h.in.w.killRichEditTimers(h.in)
	}
	h.suppressNotify++
	if h.ts2 != nil {
		if h.activated {
			h.ts2.call(tsOnTxInPlaceDeactivate)
			h.activated = false
		}
		h.ts2.release()
		h.ts2 = nil
	}
	if h.unk != nil {
		if shutdownTextServicesProc.Find() == nil {
			shutdownTextServicesProc.Call(h.unk.ptr())
		} else {
			h.unk.release()
		}
		h.unk = nil
	}
	h.in = nil
	h.suppressNotify--
	reHostRelease(h)
}

func (h *richEditHost) scale() float64 {
	if h != nil && h.in != nil && h.in.w != nil && h.in.w.scale > 0 {
		return h.in.w.scale
	}
	return 1
}

func (h *richEditHost) hwnd() uintptr {
	if h != nil && h.in != nil && h.in.w != nil {
		return h.in.w.hwnd
	}
	return 0
}

func (h *richEditHost) clientRect() rect {
	s := h.scale()
	w := max(int32(math.Round(float64(h.width)*s)), 1)
	ht := max(int32(math.Round(float64(h.height)*s)), 1)
	return rect{Right: w, Bottom: ht}
}

func (h *richEditHost) computeYOffset() float32 {
	if h == nil || h.in == nil || h.in.multiline() {
		return 0
	}
	s := h.in.config.Style
	lineH := float32(s.LineHeight)
	if lineH <= 0 {
		m := metricsOf(s.Font)
		size := max(float32(s.Font.Size), 1)
		lineH = (m.ascent + m.descent) * size
		if lineH <= 0 {
			lineH = size * 1.25
		}
	}
	return richEditSingleLineYOffset(h.height, lineH)
}

func (h *richEditHost) sendMessage(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	if h == nil || h.ts2 == nil {
		return 0, false
	}
	var res uintptr
	hr := h.ts2.call(tsTxSendMessage, uintptr(msg), wParam, lParam, uintptr(unsafe.Pointer(&res)))
	return res, !failed(hr)
}

func (h *richEditHost) applyAllConfig() {
	if h == nil || h.ts2 == nil || h.in == nil {
		return
	}
	cfg := h.in.config
	h.cf = buildCharFormat2W(cfg, fontName(cfg.Style.Font.Family))
	h.pf = buildParaFormat2(cfg)
	h.yOffset = h.computeYOffset()

	bits := richEditPropertyBits(cfg)
	allBits := txtBitRichText | txtBitMultiline | txtBitReadOnly | txtBitUsePassword |
		txtBitHideSelection | txtBitSaveSelection | txtBitAutoWordSel | txtBitWordWrap |
		txtBitAllowBeep | txtBitD2DDWrite
	h.ts2.call(tsOnTxPropertyBitsChange, uintptr(allBits), uintptr(bits))

	es, esMask := richEditEditStyle(cfg)
	h.sendMessage(emSetEditStyle, uintptr(es), uintptr(esMask))

	esx, esxMask := richEditEditStyleEx(cfg)
	h.sendMessage(emSetEditStyleEx, uintptr(esx), uintptr(esxMask))

	h.sendMessage(emSetLangOptions, 0, uintptr(richEditLangOptions(cfg)))
	h.sendMessage(emExLimitText, 0, uintptr(richEditMaxLength(cfg)))
	h.sendMessage(emSetPasswordChar, uintptr(richEditPasswordChar(cfg)), 0)
	h.sendMessage(emSetCharFormat, scfDefault, uintptr(unsafe.Pointer(&h.cf)))
	h.sendMessage(emSetCharFormat, scfAll, uintptr(unsafe.Pointer(&h.cf)))
	h.sendMessage(emSetParaFormat, 0, uintptr(unsafe.Pointer(&h.pf)))
	h.sendMessage(emSetEventMask, 0, uintptr(enmChange|enmSelChange|enmScroll|enmRequestResize))
	h.ts2.call(tsOnTxPropertyBitsChange, uintptr(txtBitCharFormatChange|txtBitParaFormatChange|txtBitScrollBarChange|txtBitMaxLengthChange|txtBitBackStyleChange),
		uintptr(txtBitCharFormatChange|txtBitParaFormatChange|txtBitScrollBarChange|txtBitMaxLengthChange|txtBitBackStyleChange))
}

func (h *richEditHost) configure(old, cur driver.InputConfig) {
	if h == nil || h.ts2 == nil {
		return
	}
	h.suppressNotify++
	defer func() { h.suppressNotify-- }()

	if !old.Style.Equal(cur.Style) || old.Multiline != cur.Multiline || old.Masked != cur.Masked ||
		old.Readonly != cur.Readonly || old.Disabled != cur.Disabled ||
		old.MaxLength != cur.MaxLength || old.NoAutoCorrect != cur.NoAutoCorrect {
		h.applyAllConfig()
	}
	h.resize(float32(cur.Content.Size.Width), float32(cur.Content.Size.Height))
}

func (h *richEditHost) resize(width, height float32) {
	if h == nil || h.ts2 == nil {
		return
	}
	w := max(width, 1)
	ht := max(height, 1)
	if w == h.width && ht == h.height {
		return
	}
	h.width, h.height = w, ht
	h.yOffset = h.computeYOffset()
	h.suppressNotify++
	h.ts2.call(tsOnTxPropertyBitsChange, uintptr(txtBitClientRectChange|txtBitExtentChange), uintptr(txtBitClientRectChange|txtBitExtentChange))
	h.suppressNotify--
}

// getText reads the control's text via ITextServices::TxGetText and normalizes paragraph breaks (\r -> \n) so UTF-16 offsets match 1:1.
func (h *richEditHost) getText() string {
	if h == nil || h.ts2 == nil {
		return ""
	}
	var bstr *uint16
	if failed(h.ts2.call(tsTxGetText, uintptr(unsafe.Pointer(&bstr)))) || bstr == nil {
		return ""
	}
	defer sysFreeString.Call(uintptr(unsafe.Pointer(bstr)))
	n, _, _ := sysStringLen.Call(uintptr(unsafe.Pointer(bstr)))
	if n == 0 {
		return ""
	}
	units := unsafe.Slice(bstr, int(n))
	return richEditToNSGoText(string(utf16.Decode(units)))
}

// setText replaces the control's entire text programmatically without firing user edit notifications and clears the undo stack.
func (h *richEditHost) setText(text string) {
	if h == nil || h.ts2 == nil {
		return
	}
	h.suppressNotify++
	defer func() { h.suppressNotify-- }()

	st := setTextEx{flags: stDefault | stUnicode, codepage: cpUnicode}
	w := wide(nsgoToRichEditText(text))
	h.sendMessage(emSetTextEx, uintptr(unsafe.Pointer(&st)), uintptr(unsafe.Pointer(w)))
	h.sendMessage(emEmptyUndoBuffer, 0, 0)
	h.sendMessage(emSetCharFormat, scfAll, uintptr(unsafe.Pointer(&h.cf)))
	h.sendMessage(emSetParaFormat, 0, uintptr(unsafe.Pointer(&h.pf)))
}

// replaceSel replaces the current selection with text, optionally recording an undo step.
func (h *richEditHost) replaceSel(text string, canUndo bool) {
	if h == nil || h.ts2 == nil {
		return
	}
	undoFlag := uintptr(0)
	if canUndo {
		undoFlag = 1
	}
	w := wide(nsgoToRichEditText(text))
	h.sendMessage(emReplaceSel, undoFlag, uintptr(unsafe.Pointer(w)))
}

// getSel returns the current selection range in UTF-16 code units.
func (h *richEditHost) getSel() (start, end int) {
	if h == nil || h.ts2 == nil {
		return 0, 0
	}
	var cr charRange
	h.sendMessage(emExGetSel, 0, uintptr(unsafe.Pointer(&cr)))
	s, e := int(cr.cpMin), int(cr.cpMax)
	if s < 0 {
		s = 0
	}
	if e < s {
		e = s
	}
	return s, e
}

// setSel sets the selection range in UTF-16 code units without firing user notifications.
func (h *richEditHost) setSel(start, end int) {
	if h == nil || h.ts2 == nil {
		return
	}
	h.suppressNotify++
	defer func() { h.suppressNotify-- }()
	cr := charRange{cpMin: int32(start), cpMax: int32(end)}
	h.sendMessage(emExSetSel, 0, uintptr(unsafe.Pointer(&cr)))
}

func (h *richEditHost) selectAll() {
	if h == nil || h.ts2 == nil {
		return
	}
	cr := charRange{cpMin: 0, cpMax: -1}
	h.sendMessage(emExSetSel, 0, uintptr(unsafe.Pointer(&cr)))
}

func (h *richEditHost) canUndo() bool {
	r, ok := h.sendMessage(emCanUndo, 0, 0)
	return ok && r != 0
}

func (h *richEditHost) canRedo() bool {
	r, ok := h.sendMessage(emCanRedo, 0, 0)
	return ok && r != 0
}

func (h *richEditHost) undo() bool {
	r, ok := h.sendMessage(emUndo, 0, 0)
	return ok && r != 0
}

func (h *richEditHost) redo() bool {
	r, ok := h.sendMessage(emRedo, 0, 0)
	return ok && r != 0
}

func (h *richEditHost) scrollCaret() {
	h.sendMessage(emScrollCaret, 0, 0)
}

// caretRect returns the caret's x, y, height in the content box's DIPs.
func (h *richEditHost) caretRect() (x, y, height float32, ok bool) {
	if h == nil || h.ts2 == nil {
		return 0, 0, 0, false
	}
	scale := float32(h.scale())
	if h.caret.hasPos && h.caret.h > 0 {
		return float32(h.caret.x) / scale, float32(h.caret.y)/scale + h.yOffset, float32(h.caret.h) / scale, true
	}
	_, end := h.getSel()
	var pt point
	if _, sent := h.sendMessage(emPosFromChar, uintptr(unsafe.Pointer(&pt)), uintptr(end)); sent {
		lineH := float32(h.in.config.Style.LineHeight)
		if lineH <= 0 {
			lineH = max(float32(h.in.config.Style.Font.Size)*1.25, 12)
		}
		return float32(pt.X) / scale, float32(pt.Y)/scale + h.yOffset, lineH, true
	}
	return 0, h.yOffset, max(float32(h.in.config.Style.Font.Size)*1.25, 12), true
}

// vScrollMetrics returns the vertical scroll offset and total content height in DIPs. The offset is EM_GETSCROLLPOS's. Windowless RichEdit keeps no scroll range of its own unless it shows a scroll bar through the host, which NSGo draws itself, so TxGetVScroll and TxSetScrollRange report none (as on Windows 11 26300): the content height is measured from where the text ends instead, unless TxGetVScroll does report a range.
func (h *richEditHost) vScrollMetrics() (pos, contentHeight float32, ok bool) {
	if h == nil || h.ts2 == nil {
		return 0, 0, false
	}
	scale := float32(h.scale())
	var cur point
	if _, sent := h.sendMessage(emGetScrollPos, 0, uintptr(unsafe.Pointer(&cur))); !sent {
		return 0, 0, false
	}
	pos = float32(max(cur.Y, 0)) / scale
	var minV, maxV, posV, pageV, enabled int32
	hr := h.ts2.call(tsTxGetVScroll,
		uintptr(unsafe.Pointer(&minV)),
		uintptr(unsafe.Pointer(&maxV)),
		uintptr(unsafe.Pointer(&posV)),
		uintptr(unsafe.Pointer(&pageV)),
		uintptr(unsafe.Pointer(&enabled)))
	if !failed(hr) && maxV > minV {
		return pos, max(float32(maxV-minV)/scale, h.height), true
	}
	return pos, max(h.textBottom(cur.Y)/scale, h.height), true
}

// textBottom returns the bottom of the text's last line in RichEdit's unscrolled physical pixels, given the current vertical scroll position scrollY: the top of the position after the last character plus that line's height.
func (h *richEditHost) textBottom(scrollY int32) float32 {
	lines, _ := h.sendMessage(emGetLineCount, 0, 0)
	end, _ := h.sendMessage(emLineIndex, uintptr(max(int(lines)-1, 0)), 0)
	var last point
	h.sendMessage(emPosFromChar, uintptr(unsafe.Pointer(&last)), end)
	lineH := float32(0)
	if lines > 1 {
		prev, _ := h.sendMessage(emLineIndex, uintptr(lines-2), 0)
		var above point
		h.sendMessage(emPosFromChar, uintptr(unsafe.Pointer(&above)), prev)
		lineH = float32(last.Y - above.Y)
	}
	if lineH <= 0 {
		lineH = float32(h.in.config.Style.LineHeight)
		if lineH <= 0 {
			lineH = max(float32(h.in.config.Style.Font.Size)*1.25, 12)
		}
		lineH *= float32(h.scale())
	}
	return float32(last.Y+scrollY) + lineH
}

// setVScroll scrolls RichEdit vertically to y DIPs and reports whether the scroll offset changed.
func (h *richEditHost) setVScroll(y float32) bool {
	if h == nil || h.ts2 == nil {
		return false
	}
	var cur point
	h.sendMessage(emGetScrollPos, 0, uintptr(unsafe.Pointer(&cur)))
	scale := float32(h.scale())
	target := point{X: cur.X, Y: int32(math.Round(float64(max(y, 0) * scale)))}
	if target.Y == cur.Y {
		return false
	}
	h.sendMessage(emSetScrollPos, 0, uintptr(unsafe.Pointer(&target)))
	var after point
	h.sendMessage(emGetScrollPos, 0, uintptr(unsafe.Pointer(&after)))
	return after.Y != cur.Y
}

// packLocalPoint maps a client point (in window DIPs) into a packed WM_MOUSE* lParam in RichEdit's local physical pixel coordinates.
func (h *richEditHost) packLocalPoint(at driver.Point) uintptr {
	lx, ly := h.in.local(at)
	s := h.scale()
	px := int32(math.Round(float64(lx) * s))
	py := int32(math.Round(float64(ly-h.yOffset) * s))
	return uintptr(uint16(int16(px))) | uintptr(uint16(int16(py)))<<16
}

// localToScreen converts a RichEdit local physical pixel point to screen pixels.
func (h *richEditHost) localToScreen(pt *point) bool {
	if pt == nil || h == nil || h.in == nil || h.in.w == nil || h.in.w.hwnd == 0 {
		return false
	}
	in := h.in
	s := h.scale()
	c := in.config.Content.Origin
	lx := float64(pt.X)/s + float64(c.X)
	ly := float64(pt.Y)/s + float64(c.Y) + float64(h.yOffset)
	world := in.world
	if !in.drawn {
		b, _ := in.w.surfaceBounds(in.v)
		world = translation(float64(b.Origin.X), float64(b.Origin.Y))
	}
	gx, gy := world.apply(lx, ly)
	pt.X = int32(math.Round(gx * s))
	pt.Y = int32(math.Round(gy * s))
	r, _, _ := clientToScreen.Call(in.w.hwnd, uintptr(unsafe.Pointer(pt)))
	return r != 0
}

// screenToClient converts a screen pixel point to RichEdit local physical pixels.
func (h *richEditHost) screenToClient(pt *point) bool {
	if pt == nil || h == nil || h.in == nil || h.in.w == nil || h.in.w.hwnd == 0 {
		return false
	}
	in := h.in
	r, _, _ := screenToClient.Call(in.w.hwnd, uintptr(unsafe.Pointer(pt)))
	if r == 0 {
		return false
	}
	at := in.w.point(pt.X, pt.Y)
	lx, ly := in.local(at)
	s := h.scale()
	pt.X = int32(math.Round(float64(lx) * s))
	pt.Y = int32(math.Round(float64(ly-h.yOffset) * s))
	return true
}

// onSetCursor forwards WM_SETCURSOR hit testing to ITextServices::OnTxSetCursor.
func (h *richEditHost) onSetCursor(at driver.Point) bool {
	if h == nil || h.ts2 == nil || h.in == nil {
		return false
	}
	lx, ly := h.in.local(at)
	s := h.scale()
	px := int32(math.Round(float64(lx) * s))
	py := int32(math.Round(float64(ly-h.yOffset) * s))
	rc := h.clientRect()
	hr := h.ts2.call(tsOnTxSetCursor,
		dvAspectContent, 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&rc)), uintptr(px), uintptr(py))
	return !failed(hr)
}

// drawD2D draws the RichEdit control into p.dc at the content box's origin (shifted by yOffset for single-line centering).
func (h *richEditHost) drawD2D(p *painter, world matrix) bool {
	if h == nil || h.ts2 == nil || h.in == nil || p == nil || p.dc == nil {
		return false
	}
	in := h.in
	h.resize(in.width, in.height)
	h.yOffset = h.computeYOffset()
	c := in.config.Content

	p.realize()
	drawM := translation(float64(c.Origin.X), float64(c.Origin.Y)+float64(h.yOffset)).then(world)
	p.setTransform(drawM)

	bounds := h.clientRect()
	hr := h.ts2.call(ts2TxDrawD2D, p.dc.ptr(), uintptr(unsafe.Pointer(&bounds)), 0, uintptr(uint32(txtViewActive)))

	// TxDrawD2D may alter the render target's transform internally; force the painter to re-emit world on the next draw call.
	p.set = false
	p.setTransform(world)

	return !failed(hr)
}

// --- Window timer bridge for RichEdit ---------------------------------------

func (w *window) setRichEditTimer(in *textInput, richID uintptr, timeoutMs uint32) bool {
	if w == nil || w.hwnd == 0 || in == nil {
		return false
	}
	if w.reTimers == nil {
		w.reTimers = map[uintptr]richEditTimerEntry{}
		w.nextReTimer = richEditTimerBase
	}
	var winID uintptr
	for id, e := range w.reTimers {
		if e.in == in && e.richID == richID {
			winID = id
			break
		}
	}
	if winID == 0 {
		winID = w.nextReTimer
		w.nextReTimer++
		w.reTimers[winID] = richEditTimerEntry{in: in, richID: richID}
	}
	r, _, _ := setTimer.Call(w.hwnd, winID, uintptr(timeoutMs), 0)
	return r != 0
}

func (w *window) killRichEditTimer(in *textInput, richID uintptr) {
	if w == nil || w.reTimers == nil {
		return
	}
	for id, e := range w.reTimers {
		if e.in == in && e.richID == richID {
			if w.hwnd != 0 {
				killTimer.Call(w.hwnd, id)
			}
			delete(w.reTimers, id)
			return
		}
	}
}

func (w *window) killRichEditTimers(in *textInput) {
	if w == nil || w.reTimers == nil {
		return
	}
	for id, e := range w.reTimers {
		if e.in == in {
			if w.hwnd != 0 {
				killTimer.Call(w.hwnd, id)
			}
			delete(w.reTimers, id)
		}
	}
}

func (w *window) richEditTimer(winID uintptr) {
	if w == nil || w.reTimers == nil {
		return
	}
	e, ok := w.reTimers[winID]
	if !ok || e.in == nil || e.in.re == nil || e.in.v == nil || e.in.v.deleted {
		return
	}
	e.in.re.sendMessage(wmTimer, e.richID, 0)
}

// --- IUnknown / ITextHost / ITextHost2 COM callbacks ------------------------

func reHostQueryInterface(h *richEditHost, iid *guid, out *unsafe.Pointer) uintptr {
	if out == nil {
		return ePointer
	}
	if iid != nil && (*iid == iidUnknown || *iid == iidTextHost || *iid == iidTextHost2) {
		*out = unsafe.Pointer(h)
		h.refs.Add(1)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func reHostAddRef(h *richEditHost) uintptr {
	return uintptr(h.refs.Add(1))
}

func reHostRelease(h *richEditHost) uintptr {
	n := h.refs.Add(-1)
	if n == 0 {
		richEditHostsMu.Lock()
		delete(richEditHosts, h)
		richEditHostsMu.Unlock()
		h.pin.Unpin()
	}
	return uintptr(n)
}

func reHostTxGetDC(h *richEditHost) uintptr {
	hwnd := h.hwnd()
	if hwnd == 0 {
		return 0
	}
	dc, _, _ := richEditGetDC.Call(hwnd)
	return dc
}

func reHostTxReleaseDC(h *richEditHost, hdc uintptr) uintptr {
	if hdc == 0 {
		return 0
	}
	r, _, _ := richEditReleaseDC.Call(h.hwnd(), hdc)
	return r
}

func reHostTxShowScrollBar(h *richEditHost, fnBar, fShow uintptr) uintptr {
	h.scroll.showBar(int32(fnBar), fShow != 0)
	return 1
}

func reHostTxEnableScrollBar(h *richEditHost, fuSBFlags, fuArrowFlags uintptr) uintptr {
	return 1
}

func reHostTxSetScrollRange(h *richEditHost, fnBar, nMinPos, nMaxPos, fRedraw uintptr) uintptr {
	h.scroll.setRange(int32(fnBar), int32(nMinPos), int32(nMaxPos))
	if int32(fnBar) == sbVert && h.in != nil {
		s := float32(h.scale())
		h.in.contentHeight = max(float32(int32(nMaxPos)-int32(nMinPos))/s, h.height)
	}
	return 1
}

func reHostTxSetScrollPos(h *richEditHost, fnBar, nPos, fRedraw uintptr) uintptr {
	h.scroll.setPos(int32(fnBar), int32(nPos))
	if int32(fnBar) == sbVert && h.in != nil {
		s := float32(h.scale())
		old := h.in.scroll
		h.in.scroll = max(float32(int32(nPos))/s, 0)
		if h.in.multiline() && h.in.scroll != old && h.in.w != nil && h.in.v != nil {
			h.in.w.showBar(h.in.v)
		}
		if fRedraw != 0 && h.in.w != nil && h.in.v != nil {
			h.in.w.redrawView(h.in.v)
		}
	}
	return 1
}

func reHostTxInvalidateRect(h *richEditHost, prc, fMode uintptr) uintptr {
	if h.in != nil && h.in.w != nil {
		if h.in.v != nil {
			h.in.w.redrawView(h.in.v)
		} else {
			h.in.w.invalidate()
		}
	}
	return 0
}

func reHostTxViewChange(h *richEditHost, fUpdate uintptr) uintptr {
	if h.in != nil && h.in.w != nil {
		if h.in.v != nil {
			h.in.w.redrawView(h.in.v)
		} else {
			h.in.w.invalidate()
		}
	}
	return 0
}

func reHostTxCreateCaret(h *richEditHost, hbmp, xWidth, yHeight uintptr) uintptr {
	h.caret.create(hbmp, int32(xWidth), int32(yHeight))
	return 1
}

func reHostTxShowCaret(h *richEditHost, fShow uintptr) uintptr {
	h.caret.show(fShow != 0)
	if h.in != nil && h.in.w != nil && h.in.v != nil {
		h.in.w.redrawView(h.in.v)
	}
	return 1
}

func reHostTxSetCaretPos(h *richEditHost, x, y uintptr) uintptr {
	h.caret.setPos(int32(x), int32(y))
	if h.in != nil && h.in.w != nil {
		h.in.caretMoved()
		h.in.w.placeIME()
		if h.in.v != nil {
			h.in.w.redrawView(h.in.v)
		}
	}
	return 1
}

func reHostTxSetTimer(h *richEditHost, idTimer, uTimeout uintptr) uintptr {
	if h.in != nil && h.in.w != nil && h.in.w.setRichEditTimer(h.in, idTimer, uint32(uTimeout)) {
		return 1
	}
	return 0
}

func reHostTxKillTimer(h *richEditHost, idTimer uintptr) uintptr {
	if h.in != nil && h.in.w != nil {
		h.in.w.killRichEditTimer(h.in, idTimer)
	}
	return 0
}

func reHostTxScrollWindowEx(h *richEditHost, dx, dy, lprcScroll, lprcClip, hrgnUpdate, lprcUpdate, fuScroll uintptr) uintptr {
	if h.in != nil && h.in.w != nil {
		if h.in.v != nil {
			h.in.w.redrawView(h.in.v)
		} else {
			h.in.w.invalidate()
		}
	}
	return 0
}

func reHostTxSetCapture(h *richEditHost, fCapture uintptr) uintptr {
	if h.in == nil || h.in.w == nil || h.in.w.hwnd == 0 {
		return 0
	}
	if fCapture != 0 {
		h.in.w.selecting = h.in
		setCapture.Call(h.in.w.hwnd)
	} else {
		if h.in.w.selecting == h.in {
			h.in.w.selecting = nil
		}
		releaseCapture.Call()
	}
	return 0
}

func reHostTxSetFocus(h *richEditHost) uintptr {
	if h.in != nil && h.in.w != nil && h.in.focusable() {
		h.in.w.setFocus(h.in)
	}
	return 0
}

func reHostTxSetCursor(h *richEditHost, hcur, fText uintptr) uintptr {
	if hcur == 0 && fText != 0 {
		hcur, _, _ = loadCursorW.Call(0, idcIBeam)
	}
	if hcur != 0 {
		setCursor.Call(hcur)
	}
	return 0
}

func reHostTxScreenToClient(h *richEditHost, lppt *point) uintptr {
	if h.screenToClient(lppt) {
		return 1
	}
	return 0
}

func reHostTxClientToScreen(h *richEditHost, lppt *point) uintptr {
	if h.localToScreen(lppt) {
		return 1
	}
	return 0
}

func reHostTxActivate(h *richEditHost, plOldState *int32) uintptr {
	if plOldState != nil {
		*plOldState = 0
	}
	return sOK
}

func reHostTxDeactivate(h *richEditHost, lNewState uintptr) uintptr {
	return sOK
}

func reHostTxGetClientRect(h *richEditHost, prc *rect) uintptr {
	if prc == nil {
		return ePointer
	}
	*prc = h.clientRect()
	return sOK
}

func reHostTxGetViewInset(h *richEditHost, prc *rect) uintptr {
	if prc == nil {
		return ePointer
	}
	*prc = rect{}
	return sOK
}

func reHostTxGetCharFormat(h *richEditHost, ppCF **charFormatW) uintptr {
	if ppCF == nil {
		return ePointer
	}
	*ppCF = &h.cf.charFormatW
	return sOK
}

func reHostTxGetParaFormat(h *richEditHost, ppPF **paraFormat2) uintptr {
	if ppPF == nil {
		return ePointer
	}
	*ppPF = &h.pf
	return sOK
}

func reHostTxGetSysColor(h *richEditHost, nIndex uintptr) uintptr {
	idx := int32(nIndex)
	if h.in != nil {
		switch idx {
		case sysColorWindow:
			if h.in.v != nil && visible(h.in.v.style.Background) && uint32(h.in.v.style.Background&0xff) == 255 {
				return uintptr(colorToColorRef(h.in.v.style.Background))
			}
		case sysColorWindowText:
			return uintptr(colorToColorRef(h.in.config.Style.Color))
		case sysColorHighlight:
			c := h.in.selectionColor()
			// Blend translucent highlight colors over a light/dark base so the opaque COLORREF matches the configured selection tint.
			if alpha := uint32(c & 0xff); alpha < 255 {
				base := driver.RGB(0xffffff)
				if h.in.w != nil && h.in.w.shownDark() {
					base = driver.RGB(0x1e1e1e)
				}
				c = blendOverOpaque(c, base)
			}
			return uintptr(colorToColorRef(c))
		case sysColorHighlightText:
			c := h.in.selectionColor()
			if uint32(c&0xff) < 255 {
				return uintptr(colorToColorRef(h.in.config.Style.Color))
			}
		}
	}
	r, _, _ := richEditGetSysColor.Call(nIndex)
	return r
}

func blendOverOpaque(fg, bg driver.Color) driver.Color {
	a := uint32(fg & 0xff)
	ia := 255 - a
	r := ((uint32(fg>>24)&0xff)*a + (uint32(bg>>24)&0xff)*ia) / 255
	g := ((uint32(fg>>16)&0xff)*a + (uint32(bg>>16)&0xff)*ia) / 255
	b := ((uint32(fg>>8)&0xff)*a + (uint32(bg>>8)&0xff)*ia) / 255
	return driver.Color(r<<24 | g<<16 | b<<8 | 0xff)
}

func reHostTxGetBackStyle(h *richEditHost, pstyle *uint32) uintptr {
	if pstyle == nil {
		return ePointer
	}
	*pstyle = txtBackTransparent
	return sOK
}

func reHostTxGetMaxLength(h *richEditHost, plength *uint32) uintptr {
	if plength == nil {
		return ePointer
	}
	if h.in != nil {
		*plength = richEditMaxLength(h.in.config)
	} else {
		*plength = richEditUnlimitedLength
	}
	return sOK
}

func reHostTxGetScrollBars(h *richEditHost, pdwScrollBar *uint32) uintptr {
	if pdwScrollBar == nil {
		return ePointer
	}
	if h.in != nil {
		*pdwScrollBar = richEditScrollBars(h.in.config)
	} else {
		*pdwScrollBar = esAutoHScroll
	}
	return sOK
}

func reHostTxGetPasswordChar(h *richEditHost, pch *uint16) uintptr {
	if pch == nil {
		return ePointer
	}
	if h.in != nil {
		*pch = richEditPasswordChar(h.in.config)
	} else {
		*pch = 0
	}
	return sOK
}

func reHostTxGetAcceleratorPos(h *richEditHost, pcp *int32) uintptr {
	if pcp == nil {
		return ePointer
	}
	*pcp = -1
	return sOK
}

func reHostTxGetExtent(h *richEditHost, lpExtent *sizeL) uintptr {
	if lpExtent == nil {
		return ePointer
	}
	lpExtent.cx = dipsToHimetric(float64(h.width))
	lpExtent.cy = dipsToHimetric(float64(h.height))
	return sOK
}

func reHostOnTxCharFormatChange(h *richEditHost, pCF *charFormatW) uintptr {
	if pCF != nil {
		if pCF.cbSize == 116 {
			h.cf = *(*charFormat2W)(unsafe.Pointer(pCF))
		} else {
			h.cf.charFormatW = *pCF
		}
	}
	return sOK
}

func reHostOnTxParaFormatChange(h *richEditHost, pPF *paraFormat2) uintptr {
	if pPF != nil && pPF.cbSize == 188 {
		h.pf = *pPF
	}
	return sOK
}

func reHostTxGetPropertyBits(h *richEditHost, dwMask uintptr, pdwBits *uint32) uintptr {
	if pdwBits == nil {
		return ePointer
	}
	var bits uint32
	if h.in != nil {
		bits = richEditPropertyBits(h.in.config)
	} else {
		bits = txtBitD2DDWrite
	}
	*pdwBits = bits & uint32(dwMask)
	return sOK
}

func reHostTxNotify(h *richEditHost, iNotify uintptr, pv unsafe.Pointer) uintptr {
	if h.suppressNotify > 0 || h.in == nil || h.in.v == nil || h.in.v.deleted {
		return sOK
	}
	switch uint32(iNotify) {
	case enChange:
		h.in.onRichEditChange()
	case enSelChange:
		h.in.onRichEditSelChange()
	case enVScroll, enHScroll:
		h.in.onRichEditScroll()
	case enRequestResize:
		if pv != nil {
			rr := (*reqResize)(pv)
			s := float32(h.scale())
			if ht := float32(rr.rc.Bottom-rr.rc.Top) / s; ht > 0 {
				h.in.contentHeight = ht
			}
		}
	}
	return sOK
}

func reHostTxImmGetContext(h *richEditHost) uintptr {
	hwnd := h.hwnd()
	if hwnd == 0 {
		return 0
	}
	himc, _, _ := immGetContext.Call(hwnd)
	return himc
}

func reHostTxImmReleaseContext(h *richEditHost, himc uintptr) uintptr {
	if himc != 0 {
		immReleaseContext.Call(h.hwnd(), himc)
	}
	return 0
}

func reHostTxGetSelectionBarWidth(h *richEditHost, lSelBarWidth *int32) uintptr {
	if lSelBarWidth == nil {
		return ePointer
	}
	*lSelBarWidth = 0
	return sOK
}

// --- ITextHost2 callbacks ---------------------------------------------------

func reHostTxIsDoubleClickPending(h *richEditHost) uintptr {
	return 0
}

func reHostTxGetWindow(h *richEditHost, phwnd *uintptr) uintptr {
	if phwnd == nil {
		return ePointer
	}
	*phwnd = h.hwnd()
	return sOK
}

func reHostTxSetForegroundWindow(h *richEditHost) uintptr {
	if hwnd := h.hwnd(); hwnd != 0 {
		setForegroundWindow.Call(hwnd)
	}
	return sOK
}

func reHostTxGetPalette(h *richEditHost) uintptr {
	return 0
}

func reHostTxGetEastAsianFlags(h *richEditHost, pFlags *int32) uintptr {
	if pFlags != nil {
		*pFlags = 0
	}
	return sOK
}

func reHostTxSetCursor2(h *richEditHost, hcur, bText uintptr) uintptr {
	if hcur == 0 && bText != 0 {
		hcur, _, _ = loadCursorW.Call(0, idcIBeam)
	}
	if hcur != 0 {
		prev, _, _ := setCursor.Call(hcur)
		return prev
	}
	return 0
}

func reHostTxFreeTextServicesNotification(h *richEditHost) uintptr {
	return 0
}

func reHostTxGetEditStyle(h *richEditHost, dwItem uintptr, pdwData *uint32) uintptr {
	if pdwData != nil {
		*pdwData = 0
	}
	return sOK
}

func reHostTxGetWindowStyles(h *richEditHost, pdwStyle, pdwExStyle *uint32) uintptr {
	var style, exStyle uint32
	if h.in != nil {
		style, exStyle = richEditWindowStyles(h.in.config)
	}
	if pdwStyle != nil {
		*pdwStyle = style
	}
	if pdwExStyle != nil {
		*pdwExStyle = exStyle
	}
	return sOK
}

func reHostTxShowDropCaret(h *richEditHost, fShow, hdc, prc uintptr) uintptr {
	return sOK
}

func reHostTxDestroyCaret(h *richEditHost) uintptr {
	h.caret.destroy()
	if h.in != nil && h.in.w != nil && h.in.v != nil {
		h.in.w.redrawView(h.in.v)
	}
	return sOK
}

func reHostTxGetHorzExtent(h *richEditHost, plHorzExtent *int32) uintptr {
	return eNotImpl
}
