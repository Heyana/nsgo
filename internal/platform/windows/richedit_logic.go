package windows

import (
	"math"
	"strings"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Windowless RichEdit (msftedit.dll) COM GUID strings from textserv.h.
const (
	iidTextServicesStr  = "8d33f740-cf58-11ce-a89d-00aa006cadc5"
	iidTextServices2Str = "8d33f741-cf58-11ce-a89d-00aa006cadc5"
	iidTextHostStr      = "13e670f4-1a5a-11cf-abeb-00aa00b65ea1"
	iidTextHost2Str     = "13e670f5-1a5a-11cf-abeb-00aa00b65ea1"
)

// Vtable slot indexes for ITextServices / ITextServices2 (textserv.h).
const (
	tsTxSendMessage          = 3
	tsTxDraw                 = 4
	tsTxGetHScroll           = 5
	tsTxGetVScroll           = 6
	tsOnTxSetCursor          = 7
	tsTxQueryHitPoint        = 8
	tsOnTxInPlaceActivate    = 9
	tsOnTxInPlaceDeactivate  = 10
	tsOnTxUIActivate         = 11
	tsOnTxUIDeactivate       = 12
	tsTxGetText              = 13
	tsTxSetText              = 14
	tsTxGetCurTargetX        = 15
	tsTxGetBaseLinePos       = 16
	tsTxGetNaturalSize       = 17
	tsTxGetDropTarget        = 18
	tsOnTxPropertyBitsChange = 19
	tsTxGetCachedSize        = 20
	tsVtblSize               = 21

	// ITextServices2 extensions (textserv.h).
	ts2TxGetNaturalSize2 = 21
	ts2TxDrawD2D         = 22
	ts2VtblSize          = 23
)

// Vtable slot indexes for ITextHost / ITextHost2 (textserv.h).
const (
	thQueryInterface         = 0
	thAddRef                 = 1
	thRelease                = 2
	thTxGetDC                = 3
	thTxReleaseDC            = 4
	thTxShowScrollBar        = 5
	thTxEnableScrollBar      = 6
	thTxSetScrollRange       = 7
	thTxSetScrollPos         = 8
	thTxInvalidateRect       = 9
	thTxViewChange           = 10
	thTxCreateCaret          = 11
	thTxShowCaret            = 12
	thTxSetCaretPos          = 13
	thTxSetTimer             = 14
	thTxKillTimer            = 15
	thTxScrollWindowEx       = 16
	thTxSetCapture           = 17
	thTxSetFocus             = 18
	thTxSetCursor            = 19
	thTxScreenToClient       = 20
	thTxClientToScreen       = 21
	thTxActivate             = 22
	thTxDeactivate           = 23
	thTxGetClientRect        = 24
	thTxGetViewInset         = 25
	thTxGetCharFormat        = 26
	thTxGetParaFormat        = 27
	thTxGetSysColor          = 28
	thTxGetBackStyle         = 29
	thTxGetMaxLength         = 30
	thTxGetScrollBars        = 31
	thTxGetPasswordChar      = 32
	thTxGetAcceleratorPos    = 33
	thTxGetExtent            = 34
	thOnTxCharFormatChange   = 35
	thOnTxParaFormatChange   = 36
	thTxGetPropertyBits      = 37
	thTxNotify               = 38
	thTxImmGetContext        = 39
	thTxImmReleaseContext    = 40
	thTxGetSelectionBarWidth = 41
	thVtblSize               = 42

	// ITextHost2 extensions (textserv.h).
	th2TxIsDoubleClickPending         = 42
	th2TxGetWindow                    = 43
	th2TxSetForegroundWindow          = 44
	th2TxGetPalette                   = 45
	th2TxGetEastAsianFlags            = 46
	th2TxSetCursor2                   = 47
	th2TxFreeTextServicesNotification = 48
	th2TxGetEditStyle                 = 49
	th2TxGetWindowStyles              = 50
	th2TxShowDropCaret                = 51
	th2TxDestroyCaret                 = 52
	th2TxGetHorzExtent                = 53
	th2VtblSize                       = 54
)

// textserv.h property bits (TXTBIT_*), enums, and caret flags.
const (
	txtBitRichText         uint32 = 0x00000001
	txtBitMultiline        uint32 = 0x00000002
	txtBitReadOnly         uint32 = 0x00000004
	txtBitShowAccelerator  uint32 = 0x00000008
	txtBitUsePassword      uint32 = 0x00000010
	txtBitHideSelection    uint32 = 0x00000020
	txtBitSaveSelection    uint32 = 0x00000040
	txtBitAutoWordSel      uint32 = 0x00000080
	txtBitVertical         uint32 = 0x00000100
	txtBitSelBarChange     uint32 = 0x00000200
	txtBitWordWrap         uint32 = 0x00000400
	txtBitAllowBeep        uint32 = 0x00000800
	txtBitDisableDrag      uint32 = 0x00001000
	txtBitViewInsetChange  uint32 = 0x00002000
	txtBitBackStyleChange  uint32 = 0x00004000
	txtBitMaxLengthChange  uint32 = 0x00008000
	txtBitScrollBarChange  uint32 = 0x00010000
	txtBitCharFormatChange uint32 = 0x00020000
	txtBitParaFormatChange uint32 = 0x00040000
	txtBitExtentChange     uint32 = 0x00080000
	txtBitClientRectChange uint32 = 0x00100000
	txtBitUseCurrentBkg    uint32 = 0x00200000
	txtBitNoThreadRefCount uint32 = 0x00400000
	txtBitShowPassword     uint32 = 0x00800000
	txtBitD2DDWrite        uint32 = 0x01000000
	txtBitD2DSimpleTypo    uint32 = 0x02000000
	txtBitD2DPixelSnapped  uint32 = 0x04000000
	txtBitD2DSubpixelLines uint32 = 0x08000000

	txtBackTransparent uint32 = 0
	txtBackOpaque      uint32 = 1

	txtViewActive   int32 = 0
	txtViewInactive int32 = -1

	txtNSFitToContent uint32 = 1

	caretFlagNone     uintptr = 0
	caretFlagCustom   uintptr = 1
	caretFlagRTL      uintptr = 2
	caretFlagItalic   uintptr = 32
	caretFlagNull     uintptr = 64
	caretFlagRotate90 uintptr = 128

	// Default password masking bullet U+2022.
	richEditBullet uint16 = 0x2022

	// RichEdit unlimited text cap (EM_EXLIMITTEXT treats 0 as 64KB, so
	// pass MaxInt32-1 for unlimited fields).
	richEditUnlimitedLength uint32 = 0x7ffffffe
)

// RichEdit window messages, edit styles, language options, notifications,
// and format flags from Richedit.h / WinUser.h.
const (
	wmUser          uint32 = 0x0400
	wmLButtonDblClk uint32 = 0x0203
	wmCut           uint32 = 0x0300
	wmCopy          uint32 = 0x0301
	wmPaste         uint32 = 0x0302
	wmClear         uint32 = 0x0303
	wmUndo          uint32 = 0x0304
	wmImeNotify     uint32 = 0x0282
	wmImeChar       uint32 = 0x0286
	wmImeRequest    uint32 = 0x0288

	// Standard edit control messages handled by RichEdit.
	emGetSel          uint32 = 0x00B0
	emSetSel          uint32 = 0x00B1
	emGetLineCount    uint32 = 0x00BA
	emLineIndex       uint32 = 0x00BB
	emReplaceSel      uint32 = 0x00C2
	emCanUndo         uint32 = 0x00C6
	emUndo            uint32 = 0x00C7
	emSetPasswordChar uint32 = 0x00CC
	emEmptyUndoBuffer uint32 = 0x00CD
	emSetReadOnly     uint32 = 0x00CF
	emGetPasswordChar uint32 = 0x00D2

	// RichEdit WM_USER+ messages (Richedit.h).
	emPosFromChar     uint32 = wmUser + 38
	emCharFromPos     uint32 = wmUser + 39
	emScrollCaret     uint32 = wmUser + 49
	emExGetSel        uint32 = wmUser + 52
	emExLimitText     uint32 = wmUser + 53
	emExSetSel        uint32 = wmUser + 55
	emGetCharFormat   uint32 = wmUser + 58
	emGetEventMask    uint32 = wmUser + 59
	emGetParaFormat   uint32 = wmUser + 61
	emSetBkgndColor   uint32 = wmUser + 67
	emSetCharFormat   uint32 = wmUser + 68
	emSetEventMask    uint32 = wmUser + 69
	emSetParaFormat   uint32 = wmUser + 71
	emRedo            uint32 = wmUser + 84
	emCanRedo         uint32 = wmUser + 85
	emGetTextEx       uint32 = wmUser + 94
	emGetTextLengthEx uint32 = wmUser + 95
	emSetTextEx       uint32 = wmUser + 97
	emSetLangOptions  uint32 = wmUser + 120
	emGetLangOptions  uint32 = wmUser + 121
	emSetEditStyle    uint32 = wmUser + 204
	emGetEditStyle    uint32 = wmUser + 205
	emGetScrollPos    uint32 = wmUser + 221
	emSetScrollPos    uint32 = wmUser + 222
	emSetEditStyleEx  uint32 = wmUser + 275
	emGetEditStyleEx  uint32 = wmUser + 276

	// RichEdit notifications (EN_* / ENM_*).
	enChange        uint32 = 0x0300
	enUpdate        uint32 = 0x0400
	enHScroll       uint32 = 0x0601
	enVScroll       uint32 = 0x0602
	enRequestResize uint32 = 0x0701
	enSelChange     uint32 = 0x0702

	enmChange        uint32 = 0x00000001
	enmUpdate        uint32 = 0x00000002
	enmScroll        uint32 = 0x00000004
	enmRequestResize uint32 = 0x00040000
	enmSelChange     uint32 = 0x00080000

	// Edit styles (EM_SETEDITSTYLE / SES_*).
	sesExtendBackColor  uint32 = 0x00000004
	sesNoIME            uint32 = 0x00000080
	sesBidi             uint32 = 0x00001000
	sesUseCTF           uint32 = 0x00010000
	sesCTFAllowEmbed    uint32 = 0x00200000
	sesCTFAllowSmartTag uint32 = 0x00400000
	sesCTFAllowProofing uint32 = 0x00800000
	sesLogicalCaret     uint32 = 0x01000000

	// Extended edit styles (EM_SETEDITSTYLEEX / SES_EX_*).
	sesExNoTable        uint32 = 0x00000004
	sesExNoMath         uint32 = 0x00000040
	sesExUseSingleLine  uint32 = 0x00200000
	sesExMultiTouch     uint32 = 0x08000000
	sesExUseMouseWParam uint32 = 0x20000000

	// Language options (EM_SETLANGOPTIONS / IMF_*).
	imfAutoFont          uint32 = 0x0002
	imfIMECancelComplete uint32 = 0x0004
	imfSpellChecking     uint32 = 0x0800
	imfIMEUIIntegration  uint32 = 0x2000

	// CHARFORMAT2W masks and effects (CFM_* / CFE_* / SCF_*).
	cfmBold      uint32 = 0x00000001
	cfmItalic    uint32 = 0x00000002
	cfmUnderline uint32 = 0x00000004
	cfmStrikeOut uint32 = 0x00000008
	cfmSpacing   uint32 = 0x00200000
	cfmWeight    uint32 = 0x00400000
	cfmCharSet   uint32 = 0x08000000
	cfmFace      uint32 = 0x20000000
	cfmColor     uint32 = 0x40000000
	cfmSize      uint32 = 0x80000000

	cfeBold      uint32 = 0x00000001
	cfeItalic    uint32 = 0x00000002
	cfeUnderline uint32 = 0x00000004
	cfeStrikeOut uint32 = 0x00000008
	cfeAutoColor uint32 = 0x40000000

	scfDefault   uintptr = 0x0000
	scfSelection uintptr = 0x0001
	scfAll       uintptr = 0x0004

	// PARAFORMAT2 masks, effects, alignment, and line spacing rules.
	pfmAlignment   uint32 = 0x00000008
	pfmLineSpacing uint32 = 0x00000100
	pfmRTLPara     uint32 = 0x00010000
	pfeRTLPara     uint16 = 0x0001

	pfaLeft    uint16 = 1
	pfaRight   uint16 = 2
	pfaCenter  uint16 = 3
	pfaJustify uint16 = 4

	// Exact line spacing in twips (bLineSpacingRule = 4).
	pfLineSpacingExact byte = 4

	// EM_SETTEXTEX / EM_GETTEXTEX / EM_GETTEXTLENGTHEX constants.
	stDefault   uint32 = 0
	stKeepUndo  uint32 = 1
	stSelection uint32 = 2
	stUnicode   uint32 = 8
	gtDefault   uint32 = 0
	gtlPrecise  uint32 = 2
	gtlNumChars uint32 = 8
	cpUnicode   uint32 = 1200

	// Scroll bar & window styles returned by TxGetScrollBars / TxGetWindowStyles.
	esLeft        uint32 = 0x0000
	esCenter      uint32 = 0x0001
	esRight       uint32 = 0x0002
	esMultiline   uint32 = 0x0004
	esPassword    uint32 = 0x0020
	esAutoVScroll uint32 = 0x0040
	esAutoHScroll uint32 = 0x0080
	esReadOnly    uint32 = 0x0800
	esWantReturn  uint32 = 0x1000
	wsVScroll     uint32 = 0x00200000
	wsHScroll     uint32 = 0x00100000
	wsExRight     uint32 = 0x00001000
	wsExRTLRead   uint32 = 0x00002000

	sbHorz int32 = 0
	sbVert int32 = 1

	// System color indices passed to TxGetSysColor.
	sysColorWindow        int32 = 5
	sysColorWindowText    int32 = 8
	sysColorHighlight     int32 = 13
	sysColorHighlightText int32 = 14
	sysColorGrayText      int32 = 17

	// Default character set (DEFAULT_CHARSET).
	defaultCharSet byte = 1
)

// charFormatW is Win32 CHARFORMATW (92 bytes under pshpack4).
type charFormatW struct {
	cbSize         uint32
	mask           uint32
	effects        uint32
	yHeight        int32
	yOffset        int32
	textColor      uint32
	charSet        byte
	pitchAndFamily byte
	faceName       [32]uint16
}

// charFormat2W is Win32 C++ CHARFORMAT2W (116 bytes under pshpack4).
// Embedding charFormatW aligns wWeight at offset 92 to match MSVC's C++
// inheritance layout (`struct CHARFORMAT2W : _charformatw`) that msftedit.dll
// expects.
type charFormat2W struct {
	charFormatW
	weight         uint16
	spacing        int16
	backColor      uint32
	lcid           uint32
	reserved       uint32
	style          int16
	kerning        uint16
	underlineType  byte
	animation      byte
	revAuthor      byte
	underlineColor byte
}

// paraFormat2 is Win32 PARAFORMAT2 (188 bytes under pshpack4).
type paraFormat2 struct {
	cbSize          uint32
	mask            uint32
	numbering       uint16
	effects         uint16
	startIndent     int32
	rightIndent     int32
	offset          int32
	alignment       uint16
	tabCount        int16
	tabs            [32]int32
	spaceBefore     int32
	spaceAfter      int32
	lineSpacing     int32
	style           int16
	lineSpacingRule byte
	outlineLevel    byte
	shadingWeight   uint16
	shadingStyle    uint16
	numberingStart  uint16
	numberingStyle  uint16
	numberingTab    uint16
	borderSpace     uint16
	borderWidth     uint16
	borders         uint16
}

// charRange is Win32 CHARRANGE (8 bytes).
type charRange struct {
	cpMin int32
	cpMax int32
}

// setTextEx is Win32 SETTEXTEX (8 bytes).
type setTextEx struct {
	flags    uint32
	codepage uint32
}

// getTextLengthEx is Win32 GETTEXTLENGTHEX (8 bytes).
type getTextLengthEx struct {
	flags    uint32
	codepage uint32
}

// nmhdr is Win32 NMHDR.
type nmhdr struct {
	hwndFrom uintptr
	idFrom   uintptr
	code     uint32
}

// selChange is Win32 SELCHANGE passed to TxNotify(EN_SELCHANGE, pv).
type selChange struct {
	hdr    nmhdr
	chrg   charRange
	seltyp uint16
}

// reqResize is Win32 REQRESIZE passed to TxNotify(EN_REQUESTRESIZE, pv).
type reqResize struct {
	hdr nmhdr
	rc  struct{ Left, Top, Right, Bottom int32 }
}

// sizeL is Win32 SIZEL (8 bytes).
type sizeL struct {
	cx, cy int32
}

// colorToColorRef converts an NSGo 0xRRGGBBAA color to a Win32 COLORREF
// (0x00BBGGRR).
func colorToColorRef(c driver.Color) uint32 {
	r := uint32(c>>24) & 0xff
	g := uint32(c>>16) & 0xff
	b := uint32(c>>8) & 0xff
	return r | g<<8 | b<<16
}

// dipsToTwips converts DIPs (1/96 inch) to twips (1/1440 inch).
// 1440 / 96 = 15 twips per DIP.
func dipsToTwips(d float64) int32 {
	return int32(math.Round(d * 15))
}

// dipsToHimetric converts DIPs (1/96 inch) to HIMETRIC (1/2540 inch).
func dipsToHimetric(d float64) int32 {
	if d <= 0 {
		return 0
	}
	return int32(math.Round(d * 2540 / 96))
}

// richEditSingleLineYOffset returns the vertical centering offset in DIPs for
// a single-line RichEdit whose line height is smaller than its content box.
func richEditSingleLineYOffset(boxHeight, lineHeight float32) float32 {
	if boxHeight <= 0 || lineHeight <= 0 || lineHeight >= boxHeight {
		return 0
	}
	return (boxHeight - lineHeight) / 2
}

// richEditMasked reports whether cfg masks text with password bullets.
// Multiline fields ignore Masked, matching the other NSGo backends.
func richEditMasked(cfg driver.InputConfig) bool {
	return cfg.Masked && !cfg.Multiline
}

// richEditEditable reports whether cfg accepts user edits.
func richEditEditable(cfg driver.InputConfig) bool {
	return !cfg.Readonly && !cfg.Disabled
}

// richEditPropertyBits computes the ITextHost::TxGetPropertyBits value for cfg.
func richEditPropertyBits(cfg driver.InputConfig) uint32 {
	// Plain-text mode (TXTBIT_RICHTEXT omitted) keeps editing plain text while
	// enabling Direct2D/DirectWrite rendering, selection hiding when inactive,
	// and saving the selection across focus transitions.
	bits := txtBitD2DDWrite | txtBitHideSelection | txtBitSaveSelection | txtBitAllowBeep
	if cfg.Multiline {
		bits |= txtBitMultiline | txtBitWordWrap
	}
	if !richEditEditable(cfg) {
		bits |= txtBitReadOnly
	}
	if richEditMasked(cfg) {
		bits |= txtBitUsePassword
	}
	return bits
}

// richEditEditStyle returns the EM_SETEDITSTYLE (value, mask) pair for cfg:
// TSF integration, logical caret delegation for Direct2D, proofing when spell
// checking is active, RTL bidi support, and IME disabled for masked/readonly
// fields.
func richEditEditStyle(cfg driver.InputConfig) (style, mask uint32) {
	mask = sesUseCTF | sesCTFAllowEmbed | sesCTFAllowSmartTag | sesCTFAllowProofing |
		sesLogicalCaret | sesNoIME | sesBidi | sesExtendBackColor
	style = sesLogicalCaret | sesExtendBackColor
	if cfg.Style.RTL {
		style |= sesBidi
	}
	if richEditEditable(cfg) && !richEditMasked(cfg) {
		style |= sesUseCTF | sesCTFAllowEmbed | sesCTFAllowSmartTag
		if !cfg.NoAutoCorrect {
			style |= sesCTFAllowProofing
		}
	} else {
		style |= sesNoIME
	}
	return style, mask
}

// richEditEditStyleEx returns the EM_SETEDITSTYLEEX (value, mask) pair for cfg.
func richEditEditStyleEx(cfg driver.InputConfig) (style, mask uint32) {
	mask = sesExNoTable | sesExNoMath | sesExUseSingleLine | sesExMultiTouch | sesExUseMouseWParam
	style = sesExNoTable | sesExNoMath | sesExMultiTouch | sesExUseMouseWParam
	if !cfg.Multiline {
		style |= sesExUseSingleLine
	}
	return style, mask
}

// richEditLangOptions returns the EM_SETLANGOPTIONS flags for cfg.
// Spell checking (IMF_SPELLCHECKING) is enabled on editable, unmasked fields
// unless NoAutoCorrect is set.
func richEditLangOptions(cfg driver.InputConfig) uint32 {
	opts := imfAutoFont | imfIMECancelComplete | imfIMEUIIntegration
	if richEditEditable(cfg) && !richEditMasked(cfg) && !cfg.NoAutoCorrect {
		opts |= imfSpellChecking
	}
	return opts
}

// richEditScrollBars returns the ITextHost::TxGetScrollBars flags for cfg.
func richEditScrollBars(cfg driver.InputConfig) uint32 {
	if cfg.Multiline {
		return wsVScroll | esAutoVScroll
	}
	return esAutoHScroll
}

// richEditWindowStyles returns the ITextHost2::TxGetWindowStyles (style, exStyle)
// pair for cfg.
func richEditWindowStyles(cfg driver.InputConfig) (style, exStyle uint32) {
	switch cfg.Style.Align {
	case driver.TextAlignCenter:
		style |= esCenter
	case driver.TextAlignEnd:
		if cfg.Style.RTL {
			style |= esLeft
		} else {
			style |= esRight
		}
	default:
		if cfg.Style.RTL {
			style |= esRight
		} else {
			style |= esLeft
		}
	}
	if cfg.Multiline {
		style |= esMultiline | esAutoVScroll | esWantReturn | wsVScroll
	} else {
		style |= esAutoHScroll
	}
	if !richEditEditable(cfg) {
		style |= esReadOnly
	}
	if richEditMasked(cfg) {
		style |= esPassword
	}
	if cfg.Style.RTL {
		exStyle |= wsExRTLRead | wsExRight
	}
	return style, exStyle
}

// richEditMaxLength returns the RichEdit character limit for cfg.
func richEditMaxLength(cfg driver.InputConfig) uint32 {
	if cfg.MaxLength > 0 {
		return uint32(cfg.MaxLength)
	}
	return richEditUnlimitedLength
}

// richEditPasswordChar returns U+2022 when cfg is masked, else 0.
func richEditPasswordChar(cfg driver.InputConfig) uint16 {
	if richEditMasked(cfg) {
		return richEditBullet
	}
	return 0
}

// buildCharFormat2W builds the default CHARFORMAT2W from cfg and the resolved
// Windows font family name.
func buildCharFormat2W(cfg driver.InputConfig, faceName string) charFormat2W {
	s := cfg.Style
	size := max(float64(s.Font.Size), 1)
	weight := uint16(400)
	if s.Font.Weight != 0 {
		weight = uint16(s.Font.Weight)
	}
	var effects uint32
	if weight >= 600 {
		effects |= cfeBold
	}
	if s.Font.Italic {
		effects |= cfeItalic
	}
	if s.Decoration&driver.TextDecorationUnderline != 0 {
		effects |= cfeUnderline
	}
	if s.Decoration&driver.TextDecorationLineThrough != 0 {
		effects |= cfeStrikeOut
	}
	cf := charFormat2W{
		charFormatW: charFormatW{
			cbSize:    116, // sizeof(CHARFORMAT2W)
			mask:      cfmFace | cfmSize | cfmWeight | cfmBold | cfmItalic | cfmUnderline | cfmStrikeOut | cfmColor | cfmCharSet | cfmSpacing,
			effects:   effects,
			yHeight:   dipsToTwips(size),
			textColor: colorToColorRef(s.Color),
			charSet:   defaultCharSet,
		},
		weight:  weight,
		spacing: int16(dipsToTwips(float64(s.LetterSpacing))),
	}
	if faceName != "" {
		u := utf16.Encode([]rune(faceName))
		if len(u) >= len(cf.faceName) {
			u = u[:len(cf.faceName)-1]
		}
		copy(cf.faceName[:], u)
	}
	return cf
}

// buildParaFormat2 builds the default PARAFORMAT2 from cfg.
func buildParaFormat2(cfg driver.InputConfig) paraFormat2 {
	s := cfg.Style
	pf := paraFormat2{
		cbSize: 188, // sizeof(PARAFORMAT2)
		mask:   pfmAlignment | pfmRTLPara,
	}
	switch s.Align {
	case driver.TextAlignCenter:
		pf.alignment = pfaCenter
	case driver.TextAlignEnd:
		if s.RTL {
			pf.alignment = pfaLeft
		} else {
			pf.alignment = pfaRight
		}
	default:
		if s.RTL {
			pf.alignment = pfaRight
		} else {
			pf.alignment = pfaLeft
		}
	}
	if s.RTL {
		pf.effects |= pfeRTLPara
	}
	if s.LineHeight > 0 {
		pf.mask |= pfmLineSpacing
		pf.lineSpacing = dipsToTwips(float64(s.LineHeight))
		pf.lineSpacingRule = pfLineSpacingExact
	}
	return pf
}

// richEditToNSGoText normalizes RichEdit's paragraph breaks (\r or \r\n) to
// NSGo's Unix line breaks (\n). With GT_DEFAULT / TxGetText, RichEdit uses a
// single \r per paragraph, preserving a 1:1 UTF-16 offset correspondence with
// NSGo's \n.
func richEditToNSGoText(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// nsgoToRichEditText converts NSGo's line breaks (\n or \r\n) to RichEdit's
// internal paragraph break (\r), preserving 1:1 UTF-16 code-unit offsets.
func nsgoToRichEditText(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\r")
	return strings.ReplaceAll(s, "\n", "\r")
}

// richEditCaretState tracks the logical caret delegated by RichEdit in Direct2D
// mode via TxCreateCaret / TxShowCaret / TxSetCaretPos / TxDestroyCaret.
type richEditCaretState struct {
	created bool
	visible bool
	hasPos  bool
	flags   uintptr
	x, y    int32
	w, h    int32
}

func (c *richEditCaretState) create(flags uintptr, w, h int32) {
	c.created = true
	c.flags = flags
	c.w = max(w, 1)
	c.h = max(h, 1)
}

func (c *richEditCaretState) show(visible bool) {
	c.visible = visible
}

func (c *richEditCaretState) setPos(x, y int32) {
	c.x, c.y = x, y
	c.hasPos = true
}

func (c *richEditCaretState) destroy() {
	c.created = false
	c.visible = false
	c.hasPos = false
}

// drawable reports whether the host should paint the caret beam.
func (c *richEditCaretState) drawable() bool {
	return c.created && c.visible && c.hasPos && c.flags&caretFlagNull == 0
}

// richEditScrollState tracks scroll bar metrics reported by RichEdit via
// TxShowScrollBar / TxSetScrollRange / TxSetScrollPos.
type richEditScrollState struct {
	vShown bool
	vMin   int32
	vMax   int32
	vPos   int32
	hShown bool
	hMin   int32
	hMax   int32
	hPos   int32
}

func (s *richEditScrollState) showBar(bar int32, show bool) {
	switch bar {
	case sbVert:
		s.vShown = show
	case sbHorz:
		s.hShown = show
	}
}

func (s *richEditScrollState) setRange(bar, minPos, maxPos int32) {
	switch bar {
	case sbVert:
		s.vMin, s.vMax = minPos, maxPos
	case sbHorz:
		s.hMin, s.hMax = minPos, maxPos
	}
}

func (s *richEditScrollState) setPos(bar, pos int32) {
	switch bar {
	case sbVert:
		s.vPos = pos
	case sbHorz:
		s.hPos = pos
	}
}
