package windows

import (
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRichEditGUIDStrings(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"IID_ITextServices", iidTextServicesStr, "8d33f740-cf58-11ce-a89d-00aa006cadc5"},
		{"IID_ITextServices2", iidTextServices2Str, "8d33f741-cf58-11ce-a89d-00aa006cadc5"},
		{"IID_ITextHost", iidTextHostStr, "13e670f4-1a5a-11cf-abeb-00aa00b65ea1"},
		{"IID_ITextHost2", iidTextHost2Str, "13e670f5-1a5a-11cf-abeb-00aa00b65ea1"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestRichEditVtableLayout(t *testing.T) {
	// ITextServices (IUnknown 0..2 + 18 methods = 21 slots) and ITextServices2
	// (TxGetNaturalSize2 at 21, TxDrawD2D at 22 = 23 slots).
	if tsTxSendMessage != 3 || tsTxDraw != 4 || tsTxGetHScroll != 5 || tsTxGetVScroll != 6 ||
		tsOnTxSetCursor != 7 || tsTxQueryHitPoint != 8 || tsOnTxInPlaceActivate != 9 ||
		tsOnTxInPlaceDeactivate != 10 || tsOnTxUIActivate != 11 || tsOnTxUIDeactivate != 12 ||
		tsTxGetText != 13 || tsTxSetText != 14 || tsTxGetCurTargetX != 15 ||
		tsTxGetBaseLinePos != 16 || tsTxGetNaturalSize != 17 || tsTxGetDropTarget != 18 ||
		tsOnTxPropertyBitsChange != 19 || tsTxGetCachedSize != 20 || tsVtblSize != 21 {
		t.Fatalf("ITextServices vtable slots drifted: size=%d", tsVtblSize)
	}
	if ts2TxGetNaturalSize2 != 21 || ts2TxDrawD2D != 22 || ts2VtblSize != 23 {
		t.Fatalf("ITextServices2 vtable slots drifted: nat2=%d drawD2D=%d size=%d",
			ts2TxGetNaturalSize2, ts2TxDrawD2D, ts2VtblSize)
	}

	// ITextHost (IUnknown 0..2 + 39 methods = 42 slots) and ITextHost2
	// (12 methods 42..53 = 54 slots).
	if thQueryInterface != 0 || thAddRef != 1 || thRelease != 2 ||
		thTxGetDC != 3 || thTxReleaseDC != 4 || thTxShowScrollBar != 5 ||
		thTxCreateCaret != 11 || thTxShowCaret != 12 || thTxSetCaretPos != 13 ||
		thTxSetTimer != 14 || thTxKillTimer != 15 || thTxSetCapture != 17 ||
		thTxSetFocus != 18 || thTxGetClientRect != 24 || thTxGetCharFormat != 26 ||
		thTxGetParaFormat != 27 || thTxGetSysColor != 28 || thTxGetBackStyle != 29 ||
		thTxGetMaxLength != 30 || thTxGetScrollBars != 31 || thTxGetPasswordChar != 32 ||
		thTxGetPropertyBits != 37 || thTxNotify != 38 || thTxImmGetContext != 39 ||
		thTxImmReleaseContext != 40 || thTxGetSelectionBarWidth != 41 || thVtblSize != 42 {
		t.Fatalf("ITextHost vtable slots drifted: size=%d", thVtblSize)
	}
	if th2TxIsDoubleClickPending != 42 || th2TxGetWindow != 43 || th2TxSetForegroundWindow != 44 ||
		th2TxGetPalette != 45 || th2TxGetEastAsianFlags != 46 || th2TxSetCursor2 != 47 ||
		th2TxFreeTextServicesNotification != 48 || th2TxGetEditStyle != 49 ||
		th2TxGetWindowStyles != 50 || th2TxShowDropCaret != 51 ||
		th2TxDestroyCaret != 52 || th2TxGetHorzExtent != 53 || th2VtblSize != 54 {
		t.Fatalf("ITextHost2 vtable slots drifted: size=%d", th2VtblSize)
	}
}

func TestRichEditStructABI(t *testing.T) {
	if got := unsafe.Sizeof(charFormatW{}); got != 92 {
		t.Fatalf("sizeof(charFormatW) = %d, want 92", got)
	}
	var cf2 charFormat2W
	if got := unsafe.Sizeof(cf2); got != 116 {
		t.Fatalf("sizeof(charFormat2W) = %d, want 116", got)
	}
	if got := unsafe.Offsetof(cf2.weight); got != 92 {
		t.Fatalf("offsetof(charFormat2W.weight) = %d, want 92 (MSVC C++ CHARFORMAT2W)", got)
	}
	if got := unsafe.Offsetof(cf2.spacing); got != 94 {
		t.Fatalf("offsetof(charFormat2W.spacing) = %d, want 94", got)
	}
	if got := unsafe.Offsetof(cf2.backColor); got != 96 {
		t.Fatalf("offsetof(charFormat2W.backColor) = %d, want 96", got)
	}
	if got := unsafe.Offsetof(cf2.underlineColor); got != 115 {
		t.Fatalf("offsetof(charFormat2W.underlineColor) = %d, want 115", got)
	}

	var pf2 paraFormat2
	if got := unsafe.Sizeof(pf2); got != 188 {
		t.Fatalf("sizeof(paraFormat2) = %d, want 188", got)
	}
	if got := unsafe.Offsetof(pf2.alignment); got != 24 {
		t.Fatalf("offsetof(paraFormat2.alignment) = %d, want 24", got)
	}
	if got := unsafe.Offsetof(pf2.lineSpacing); got != 164 {
		t.Fatalf("offsetof(paraFormat2.lineSpacing) = %d, want 164", got)
	}
	if got := unsafe.Offsetof(pf2.lineSpacingRule); got != 170 {
		t.Fatalf("offsetof(paraFormat2.lineSpacingRule) = %d, want 170", got)
	}

	if got := unsafe.Sizeof(charRange{}); got != 8 {
		t.Fatalf("sizeof(charRange) = %d, want 8", got)
	}
	if got := unsafe.Sizeof(setTextEx{}); got != 8 {
		t.Fatalf("sizeof(setTextEx) = %d, want 8", got)
	}
	if got := unsafe.Sizeof(getTextLengthEx{}); got != 8 {
		t.Fatalf("sizeof(getTextLengthEx) = %d, want 8", got)
	}
	if got := unsafe.Sizeof(sizeL{}); got != 8 {
		t.Fatalf("sizeof(sizeL) = %d, want 8", got)
	}
}

func TestRichEditPropertyBitsAndStyles(t *testing.T) {
	// Default editable single-line input: D2D/DWrite, TSF + spell checking, single-line ex style.
	base := driver.InputConfig{}
	bits := richEditPropertyBits(base)
	if bits&txtBitD2DDWrite == 0 {
		t.Error("expected TXTBIT_D2DDWRITE")
	}
	if bits&(txtBitMultiline|txtBitWordWrap|txtBitReadOnly|txtBitUsePassword|txtBitRichText) != 0 {
		t.Errorf("unexpected bits on default single-line input: %#x", bits)
	}
	es, _ := richEditEditStyle(base)
	if es&(sesUseCTF|sesCTFAllowProofing|sesLogicalCaret) != (sesUseCTF | sesCTFAllowProofing | sesLogicalCaret) {
		t.Errorf("default edit style = %#x, want TSF + proofing + logical caret", es)
	}
	if es&sesNoIME != 0 {
		t.Errorf("default edit style unexpectedly disabled IME: %#x", es)
	}
	esx, _ := richEditEditStyleEx(base)
	if esx&sesExUseSingleLine == 0 || esx&sesExNoTable == 0 || esx&sesExNoMath == 0 {
		t.Errorf("default edit style ex = %#x", esx)
	}
	if opts := richEditLangOptions(base); opts&imfSpellChecking == 0 {
		t.Errorf("default lang options = %#x, want IMF_SPELLCHECKING", opts)
	}
	if sb := richEditScrollBars(base); sb != esAutoHScroll {
		t.Errorf("single-line scrollbars = %#x, want %#x", sb, esAutoHScroll)
	}
	if lim := richEditMaxLength(base); lim != richEditUnlimitedLength {
		t.Errorf("default maxLength = %#x, want %#x", lim, richEditUnlimitedLength)
	}

	// Password field: masks with bullet, disables IME and spell checking.
	pw := driver.InputConfig{Masked: true, MaxLength: 32}
	if bits := richEditPropertyBits(pw); bits&txtBitUsePassword == 0 {
		t.Errorf("password bits = %#x, want TXTBIT_USEPASSWORD", bits)
	}
	if ch := richEditPasswordChar(pw); ch != richEditBullet {
		t.Errorf("password char = %#x, want %#x", ch, richEditBullet)
	}
	if es, _ := richEditEditStyle(pw); es&sesNoIME == 0 || es&sesCTFAllowProofing != 0 {
		t.Errorf("password edit style = %#x, want SES_NOIME without proofing", es)
	}
	if opts := richEditLangOptions(pw); opts&imfSpellChecking != 0 {
		t.Errorf("password lang options = %#x, must not spellcheck", opts)
	}
	if lim := richEditMaxLength(pw); lim != 32 {
		t.Errorf("password maxLength = %d, want 32", lim)
	}

	// Multiline + Masked ignores masking (matching all other NSGo backends).
	mlMasked := driver.InputConfig{Multiline: true, Masked: true, NoAutoCorrect: true}
	if bits := richEditPropertyBits(mlMasked); bits&(txtBitMultiline|txtBitWordWrap) != (txtBitMultiline|txtBitWordWrap) || bits&txtBitUsePassword != 0 {
		t.Errorf("multiline+masked bits = %#x", bits)
	}
	if ch := richEditPasswordChar(mlMasked); ch != 0 {
		t.Errorf("multiline+masked password char = %#x, want 0", ch)
	}
	if opts := richEditLangOptions(mlMasked); opts&imfSpellChecking != 0 {
		t.Errorf("NoAutoCorrect lang options = %#x, must not spellcheck", opts)
	}
	if esx, _ := richEditEditStyleEx(mlMasked); esx&sesExUseSingleLine != 0 {
		t.Errorf("multiline edit style ex = %#x, must not set SES_EX_USESINGLELINE", esx)
	}
	if sb := richEditScrollBars(mlMasked); sb != wsVScroll|esAutoVScroll {
		t.Errorf("multiline scrollbars = %#x, want %#x", sb, wsVScroll|esAutoVScroll)
	}

	// Readonly & Disabled disable edits, IME, and spell checking.
	for _, cfg := range []driver.InputConfig{{Readonly: true}, {Disabled: true}} {
		if bits := richEditPropertyBits(cfg); bits&txtBitReadOnly == 0 {
			t.Errorf("non-editable bits = %#x, want TXTBIT_READONLY", bits)
		}
		if es, _ := richEditEditStyle(cfg); es&sesNoIME == 0 {
			t.Errorf("non-editable edit style = %#x, want SES_NOIME", es)
		}
		if opts := richEditLangOptions(cfg); opts&imfSpellChecking != 0 {
			t.Errorf("non-editable lang options = %#x, must not spellcheck", opts)
		}
	}
}

func TestRichEditFormatsAndRTL(t *testing.T) {
	cfg := driver.InputConfig{
		Multiline: true,
		Style: driver.TextStyle{
			Font: driver.Font{
				Size:   16,
				Weight: 700,
				Italic: true,
			},
			Color:         driver.RGB(0x123456),
			Align:         driver.TextAlignStart,
			LineHeight:    24,
			LetterSpacing: 2,
			Decoration:    driver.TextDecorationUnderline | driver.TextDecorationLineThrough,
			RTL:           true,
		},
	}

	cf := buildCharFormat2W(cfg, "Segoe UI")
	if cf.cbSize != 116 {
		t.Fatalf("cbSize = %d, want 116", cf.cbSize)
	}
	if cf.yHeight != 240 { // 16 DIPs * 15 twips/DIP
		t.Errorf("yHeight = %d, want 240", cf.yHeight)
	}
	if cf.weight != 700 {
		t.Errorf("weight = %d, want 700", cf.weight)
	}
	if cf.spacing != 30 { // 2 DIPs * 15 twips/DIP
		t.Errorf("spacing = %d, want 30", cf.spacing)
	}
	if wantColor := uint32(0x00563412); cf.textColor != wantColor {
		t.Errorf("textColor = %#x, want %#x", cf.textColor, wantColor)
	}
	wantEffects := cfeBold | cfeItalic | cfeUnderline | cfeStrikeOut
	if cf.effects != wantEffects {
		t.Errorf("effects = %#x, want %#x", cf.effects, wantEffects)
	}
	face := string(utf16.Decode(cf.faceName[:8]))
	if face != "Segoe UI" {
		t.Errorf("faceName = %q, want %q", face, "Segoe UI")
	}

	// Long font family name truncates safely with trailing NUL.
	longCF := buildCharFormat2W(cfg, "AVeryLongFontFamilyNameThatExceedsThirtyTwoChars")
	if longCF.faceName[31] != 0 {
		t.Errorf("long faceName not NUL-terminated: last=%#x", longCF.faceName[31])
	}

	pf := buildParaFormat2(cfg)
	if pf.cbSize != 188 {
		t.Fatalf("pf.cbSize = %d, want 188", pf.cbSize)
	}
	if pf.alignment != pfaRight {
		t.Errorf("RTL Start alignment = %d, want PFA_RIGHT (%d)", pf.alignment, pfaRight)
	}
	if pf.effects&pfeRTLPara == 0 {
		t.Errorf("RTL effects = %#x, want PFE_RTLPARA", pf.effects)
	}
	if pf.mask&pfmLineSpacing == 0 || pf.lineSpacing != 360 || pf.lineSpacingRule != pfLineSpacingExact {
		t.Errorf("line spacing = (mask=%#x, dy=%d, rule=%d), want exact 360", pf.mask, pf.lineSpacing, pf.lineSpacingRule)
	}

	// Window styles reflect RTL reading order and multiline flags.
	style, exStyle := richEditWindowStyles(cfg)
	if style&(esMultiline|esRight|wsVScroll) != (esMultiline | esRight | wsVScroll) {
		t.Errorf("window style = %#x", style)
	}
	if exStyle&(wsExRTLRead|wsExRight) != (wsExRTLRead | wsExRight) {
		t.Errorf("window exStyle = %#x", exStyle)
	}
	if es, _ := richEditEditStyle(cfg); es&sesBidi == 0 {
		t.Errorf("RTL edit style = %#x, want SES_BIDI", es)
	}
}

func TestRichEditTextNormalizationAndUnits(t *testing.T) {
	if got := richEditToNSGoText("one\rtwo\r\nthree\nfour"); got != "one\ntwo\nthree\nfour" {
		t.Errorf("richEditToNSGoText = %q", got)
	}
	if got := nsgoToRichEditText("one\ntwo\r\nthree"); got != "one\rtwo\rthree" {
		t.Errorf("nsgoToRichEditText = %q", got)
	}
	// Round-trip preserves exact UTF-16 length for multiline text with emoji.
	orig := "a😀\nb\nc"
	reText := nsgoToRichEditText(orig)
	if len(utf16.Encode([]rune(reText))) != len(utf16.Encode([]rune(orig))) {
		t.Fatalf("UTF-16 length changed across conversion: %q -> %q", orig, reText)
	}
	if back := richEditToNSGoText(reText); back != orig {
		t.Fatalf("round-trip mismatch: got %q, want %q", back, orig)
	}

	if got := dipsToHimetric(96); got != 2540 {
		t.Errorf("dipsToHimetric(96) = %d, want 2540", got)
	}
	if got := dipsToHimetric(0); got != 0 {
		t.Errorf("dipsToHimetric(0) = %d, want 0", got)
	}
	if got := richEditSingleLineYOffset(32, 20); got != 6 {
		t.Errorf("richEditSingleLineYOffset(32, 20) = %v, want 6", got)
	}
	if got := richEditSingleLineYOffset(16, 20); got != 0 {
		t.Errorf("richEditSingleLineYOffset(16, 20) = %v, want 0", got)
	}
}

func TestRichEditCaretAndScrollState(t *testing.T) {
	var c richEditCaretState
	if c.drawable() {
		t.Fatal("zero caret must not be drawable")
	}
	c.create(caretFlagNone, 1, 18)
	c.show(true)
	if c.drawable() {
		t.Fatal("caret without position must not be drawable")
	}
	c.setPos(12, 4)
	if !c.drawable() || c.x != 12 || c.y != 4 || c.w != 1 || c.h != 18 {
		t.Fatalf("caret state = %+v", c)
	}
	// CARET_NULL (non-empty selection) suppresses beam drawing.
	c.create(caretFlagNull, 1, 18)
	if c.drawable() {
		t.Fatal("CARET_NULL caret must not be drawable")
	}
	c.create(caretFlagNone, 1, 18)
	c.destroy()
	if c.drawable() || c.created || c.visible {
		t.Fatalf("destroyed caret still active: %+v", c)
	}

	var sc richEditScrollState
	sc.showBar(sbVert, true)
	sc.setRange(sbVert, 0, 420)
	sc.setPos(sbVert, 80)
	if !sc.vShown || sc.vMin != 0 || sc.vMax != 420 || sc.vPos != 80 {
		t.Fatalf("vertical scroll state = %+v", sc)
	}
	sc.showBar(sbHorz, true)
	sc.setRange(sbHorz, 0, 300)
	sc.setPos(sbHorz, 25)
	if !sc.hShown || sc.hMin != 0 || sc.hMax != 300 || sc.hPos != 25 {
		t.Fatalf("horizontal scroll state = %+v", sc)
	}
}
