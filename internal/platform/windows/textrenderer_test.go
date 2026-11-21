//go:build windows

package windows

import (
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHiddenSpanTextRendererSkipsHiddenEffect(t *testing.T) {
	if !shouldSkipGlyphRun(hiddenEffectObj.ptr()) {
		t.Fatal("shouldSkipGlyphRun did not match hiddenEffectObj")
	}
	if shouldSkipGlyphRun(0) {
		t.Fatal("shouldSkipGlyphRun matched 0")
	}
	var other int
	if shouldSkipGlyphRun(uintptr(unsafe.Pointer(&other))) {
		t.Fatal("shouldSkipGlyphRun matched non-hidden effect")
	}
	// Calling DrawGlyphRun with hiddenEffectObj must return S_OK immediately without dereferencing ctx.
	if hr := textDrawGlyphRunWords(&textRendererObj, nil, 0, 0, 0, 0, 0, (*comObject)(unsafe.Pointer(&hiddenEffectObj))); hr != sOK {
		t.Fatalf("textDrawGlyphRunWords(hidden) = %#x, want S_OK", hr)
	}
	// Verify QueryInterface answers IUnknown, IDWritePixelSnapping and IDWriteTextRenderer.
	for _, iid := range []*guid{&iidUnknown, &iidDWritePixelSnap, &iidDWriteTextRenderer} {
		var out unsafe.Pointer
		if hr := textRendererQueryInterface(&textRendererObj, iid, &out); hr != sOK || out != unsafe.Pointer(&textRendererObj) {
			t.Fatalf("QueryInterface(%v) = %#x, %p", *iid, hr, out)
		}
	}
	var out unsafe.Pointer
	if hr := textRendererQueryInterface(&textRendererObj, &iidDWriteInline, &out); hr != eNoInterface || out != nil {
		t.Fatalf("QueryInterface(unexpected) = %#x, %p", hr, out)
	}
	var hOut unsafe.Pointer
	if hr := hiddenEffectQueryInterface(&hiddenEffectObj, &iidUnknown, &hOut); hr != sOK || hOut != unsafe.Pointer(&hiddenEffectObj) {
		t.Fatalf("hiddenEffect QueryInterface(IUnknown) = %#x, %p", hr, hOut)
	}
	var disabled int32 = 1
	textRendererPixelSnappingDisabled(&textRendererObj, 0, &disabled)
	if disabled != 0 {
		t.Fatalf("pixel snapping disabled = %d, want 0", disabled)
	}
	var m matrix3x2
	textRendererCurrentTransform(&textRendererObj, 0, &m)
	if m != (matrix3x2{m11: 1, m22: 1}) {
		t.Fatalf("transform = %+v, want identity", m)
	}
	ctx := textDrawContext{scale: 2}
	var dpi float32
	textRendererPixelsPerDip(&textRendererObj, &ctx, &dpi)
	if dpi != 2 {
		t.Fatalf("pixelsPerDip = %v, want 2", dpi)
	}
}

func TestHiddenSpansKeepMetricsAndSkipDecorations(t *testing.T) {
	w := testWindow()
	black := driver.RGB(0x000000)
	red := driver.RGB(0xff0000)
	base := driver.TextStyle{Font: driver.Font{Family: "Segoe UI", Size: 16, Weight: 400}, Color: black}
	decorated := base
	decorated.Decoration = driver.TextDecorationUnderline | driver.TextDecorationLineThrough
	decorated.DecorationColor = red
	text := "Hello world end"
	visible := &driver.AttributedText{
		Text:  text,
		Style: base,
		Spans: []driver.TextSpan{
			{Start: 0, End: 6, Style: decorated},
			{Start: 6, End: 11, Style: decorated},
			{Start: 11, End: len(text), Style: base},
		},
	}
	hidden := &driver.AttributedText{
		Text:  text,
		Style: base,
		Spans: []driver.TextSpan{
			{Start: 0, End: 6, Style: decorated},
			{Start: 6, End: 11, Style: decorated, Hidden: true, Background: driver.TextBackground{Color: red}},
			{Start: 11, End: len(text), Style: base},
		},
	}
	pv := w.Prepare(visible)
	defer pv.Release()
	ph := w.Prepare(hidden)
	defer ph.Release()
	sv, sh := pv.Measure(driver.Auto), ph.Measure(driver.Auto)
	if sv != sh {
		t.Fatalf("measured size changed: visible %v, hidden %v", sv, sh)
	}
	lv := pv.Layout(sv).(*textLayout)
	defer lv.Release()
	lh := ph.Layout(sh).(*textLayout)
	defer lh.Release()
	if !lh.hasHidden || len(lh.hidden) != 1 || lh.hidden[0] != (textRange{start: 6, length: 5}) {
		t.Fatalf("hidden ranges = %+v (hasHidden=%v), want [{6 5}]", lh.hidden, lh.hasHidden)
	}
	if len(lh.backgrounds) != 0 {
		t.Errorf("hidden span added %d backgrounds", len(lh.backgrounds))
	}
	// Only the visible first span's underline + strikethrough (2 decorations), not the hidden span's.
	if len(lh.decorations) != 2 {
		t.Errorf("decorations = %d, want 2 (visible span only)", len(lh.decorations))
	}
	cv, _ := lv.CaretRect(driver.Caret{Offset: 11})
	ch, _ := lh.CaretRect(driver.Caret{Offset: 11})
	if cv != ch {
		t.Errorf("trailing span caret changed: visible %+v, hidden %+v", cv, ch)
	}
}
