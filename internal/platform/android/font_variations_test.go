package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWriteStyleEncodesAxesAndFeaturesWithBackwardsCompat(t *testing.T) {
	readU16 := func(r *reader) uint16 { return uint16(r.byte())<<8 | uint16(r.byte()) }

	// Plain style (no Axes, no Features): bits 5 and 6 stay 0 and no trailing strings are written.
	var plain writer
	writeStyle(&plain, &driver.TextStyle{
		Font: driver.Font{Family: "Inter", Language: "en", Size: 16, Weight: 400, Variant: driver.FontTabularNums},
	}, false)
	pr := reader{plain.buf}
	_ = pr.str()     // family
	_ = pr.str()     // language
	_ = pr.float()   // size
	_ = readU16(&pr) // weight
	_ = pr.u32()     // color
	_ = pr.float()   // lineHeight
	_ = pr.float()   // letterSpacing
	_ = pr.byte()    // align
	flags := pr.byte()
	if flags&(1<<5|1<<6) != 0 {
		t.Fatalf("plain style set unexpected axes/features flags: %08b", flags)
	}
	if variant := pr.byte(); variant != byte(driver.FontTabularNums) {
		t.Fatalf("variant = %d, want %d", variant, driver.FontTabularNums)
	}
	if len(pr.buf) != 0 {
		t.Fatalf("plain style wrote %d unexpected trailing bytes", len(pr.buf))
	}

	// Styled with Axes and Features (overriding Variant's tnum to 0).
	var rich writer
	writeStyle(&rich, &driver.TextStyle{
		Font: driver.Font{
			Family:   "Inter",
			Size:     18,
			Weight:   500,
			Variant:  driver.FontTabularNums | driver.FontSmallCaps,
			Axes:     map[string]float32{"wdth": 75, "opsz": 18},
			Features: map[string]int{"tnum": 0, "ss01": 1, "zero": 1},
		},
	}, false)
	rr := reader{rich.buf}
	_ = rr.str()
	_ = rr.str()
	_ = rr.float()
	_ = readU16(&rr)
	_ = rr.u32()
	_ = rr.float()
	_ = rr.float()
	_ = rr.byte()
	rflags := rr.byte()
	if rflags&(1<<5) == 0 || rflags&(1<<6) == 0 {
		t.Fatalf("rich style flags = %08b, want bits 5 and 6 set", rflags)
	}
	_ = rr.byte() // variant
	if got, want := rr.str(), `'opsz' 18, 'wdth' 75`; got != want {
		t.Errorf("variations = %q, want %q", got, want)
	}
	if got, want := rr.str(), `'smcp' 1, 'ss01' 1, 'tnum' 0, 'zero' 1`; got != want {
		t.Errorf("features = %q, want %q", got, want)
	}
	if len(rr.buf) != 0 {
		t.Fatalf("rich style left %d unread bytes", len(rr.buf))
	}
}

func TestWriteDrawingEncodesFontAxesAndFeatures(t *testing.T) {
	var dw writer
	writeDrawing(&dw, &driver.Drawing{
		Ops: []driver.DrawOp{{
			Kind: driver.DrawFillText,
			Text: "0123",
			Font: driver.Font{
				Family:   "Roboto Flex",
				Size:     20,
				Weight:   600,
				Italic:   true,
				Axes:     map[string]float32{"wdth": 85},
				Features: map[string]int{"zero": 1, "ss01": 1},
			},
		}},
	})
	dr := reader{dw.buf}
	_ = dr.u32() // op bytes length
	if kind := dr.byte(); kind != byte(driver.DrawFillText) {
		t.Fatalf("kind = %v, want DrawFillText", kind)
	}
	_ = dr.str()                                 // text
	_ = dr.float()                               // x
	_ = dr.float()                               // y
	_ = dr.str()                                 // family
	_ = dr.float()                               // size
	_ = uint16(dr.byte())<<8 | uint16(dr.byte()) // weight
	flags := dr.byte()
	if flags != 0b111 {
		t.Fatalf("drawing font flags = %08b, want 00000111", flags)
	}
	_ = dr.byte() // variant
	if got, want := dr.str(), `'wdth' 85`; got != want {
		t.Errorf("drawing variations = %q, want %q", got, want)
	}
	if got, want := dr.str(), `'ss01' 1, 'zero' 1`; got != want {
		t.Errorf("drawing features = %q, want %q", got, want)
	}
}
