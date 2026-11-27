//go:build darwin

package apple

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestVariableFontWidthAxisChangesMeasurement proves that CoreText applies
// Font.Axes ("wdth") when measuring text with a variable font (macOS's Skia).
func TestVariableFontWidthAxisChangesMeasurement(t *testing.T) {
	measure := func(wdth float32) driver.Points {
		p := TextEngine{}.Prepare(&driver.AttributedText{
			Text: "HAMBURGEFONTS",
			Style: driver.TextStyle{
				Font: driver.Font{
					Family: "Skia",
					Size:   24,
					Weight: driver.FontWeightNormal,
					Axes:   map[string]float32{"wdth": wdth},
				},
				Color: driver.RGB(0),
			},
		})
		defer p.Release()
		return p.Measure(driver.Auto).Width
	}
	condensed := measure(0.62)
	expanded := measure(1.3)
	if expanded <= condensed*1.2 {
		t.Fatalf("Skia width at wdth=1.3 (%v) should be substantially wider than wdth=0.62 (%v)", expanded, condensed)
	}
}

// TestOpenTypeFeaturesChangeRenderedGlyphs proves that CoreText applies
// arbitrary OpenType features (ss01, zero) and changes the rendered glyphs
// on both macOS's system font (San Francisco) and a registered OpenType font.
func TestOpenTypeFeaturesChangeRenderedGlyphs(t *testing.T) {
	box := driver.Size{Width: 160, Height: 64}
	draw := func(family, text string, features map[string]int) []byte {
		img, layout := render(t, &driver.AttributedText{
			Text: text,
			Style: driver.TextStyle{
				Font: driver.Font{
					Family:   family,
					Size:     42,
					Weight:   driver.FontWeightNormal,
					Features: features,
				},
				Color: driver.RGB(0x000000),
			},
		}, box)
		layout.Release()
		return img.Pix
	}

	// San Francisco's ss01 draws straight-sided 6 and 9.
	plain69 := draw("", "69", map[string]int{"ss01": 0})
	alt69 := draw("", "69", map[string]int{"ss01": 1})
	if bytes.Equal(plain69, alt69) {
		t.Fatal("ss01=1 rendered identical pixels to ss01=0 for \"69\" in system font")
	}

	// Register a minimal OpenType font with explicit GSUB ss01 and zero lookups.
	const testFamily = "NSGoOTFeatureTest"
	if err := RegisterFont(buildOpenTypeFeatureTestFont(testFamily)); err != nil {
		t.Fatalf("RegisterFont(%s): %v", testFamily, err)
	}

	plainA := draw(testFamily, "A", map[string]int{"ss01": 0})
	altA := draw(testFamily, "A", map[string]int{"ss01": 1})
	if bytes.Equal(plainA, altA) {
		t.Fatalf("ss01=1 rendered identical pixels to ss01=0 for \"A\" in %s", testFamily)
	}

	plainZero := draw(testFamily, "0", map[string]int{"zero": 0})
	slashedZero := draw(testFamily, "0", map[string]int{"zero": 1})
	if bytes.Equal(plainZero, slashedZero) {
		t.Fatalf("zero=1 rendered identical pixels to zero=0 for \"0\" in %s", testFamily)
	}
}

// TestFontCacheKeysOnAxesAndFeatures ensures fonts and attribute dictionaries
// with distinct Axes or Features do not collide in CoreText's caches.
func TestFontCacheKeysOnAxesAndFeatures(t *testing.T) {
	base := driver.Font{Family: "Skia", Size: 16, Weight: driver.FontWeightNormal}
	others := map[string]driver.Font{
		"wdth-narrow": {Family: "Skia", Size: 16, Weight: driver.FontWeightNormal, Axes: map[string]float32{"wdth": 0.65}},
		"wdth-wide":   {Family: "Skia", Size: 16, Weight: driver.FontWeightNormal, Axes: map[string]float32{"wdth": 1.25}},
		"ss01":        {Family: "Skia", Size: 16, Weight: driver.FontWeightNormal, Features: map[string]int{"ss01": 1}},
		"zero":        {Family: "Skia", Size: 16, Weight: driver.FontWeightNormal, Features: map[string]int{"zero": 1}},
	}
	a := Font(base)
	defer Release(a)
	for name, f := range others {
		b := Font(f)
		if b == a {
			t.Errorf("%s: collided with base font in fontCache", name)
		}
		Release(b)
	}

	stBase := driver.TextStyle{Font: base, Color: driver.RGB(0x111111)}
	stVar := driver.TextStyle{Font: others["wdth-wide"], Color: driver.RGB(0x111111)}
	da, db := textAttributes(stBase, false), textAttributes(stVar, false)
	defer Release(da)
	defer Release(db)
	if da == db {
		t.Fatal("textAttributes collided for different Font.Axes")
	}
}

// buildOpenTypeFeatureTestFont constructs a minimal valid TrueType/OpenType
// font containing glyphs for '0' and 'A' and GSUB single-substitution lookups
// for the "ss01" ('A' -> wide box) and "zero" ('0' -> wide box) feature tags.
func buildOpenTypeFeatureTestFont(family string) []byte {
	be16 := func(b []byte, v uint16) []byte { return binary.BigEndian.AppendUint16(b, v) }
	be32 := func(b []byte, v uint32) []byte { return binary.BigEndian.AppendUint32(b, v) }
	i16 := func(v int16) uint16 { return uint16(v) }

	// Simple rectangular TrueType glyph outline.
	rectGlyph := func(xMin, yMin, xMax, yMax int16) []byte {
		var b []byte
		b = be16(b, 1) // numberOfContours
		b = be16(b, uint16(xMin))
		b = be16(b, uint16(yMin))
		b = be16(b, uint16(xMax))
		b = be16(b, uint16(yMax))
		b = be16(b, 3) // endPtsOfContours[0] = 3 (4 points)
		b = be16(b, 0) // instructionLength = 0
		// Flags: on-curve (0x01) for all 4 points, 16-bit signed deltas.
		b = append(b, 1, 1, 1, 1)
		// X deltas: (xMin), (xMax-xMin), (0), (xMin-xMax)
		b = be16(b, uint16(xMin))
		b = be16(b, uint16(xMax-xMin))
		b = be16(b, 0)
		b = be16(b, uint16(xMin-xMax))
		// Y deltas: (yMin), (0), (yMax-yMin), (0)
		b = be16(b, uint16(yMin))
		b = be16(b, 0)
		b = be16(b, uint16(yMax-yMin))
		b = be16(b, 0)
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		return b
	}

	// Glyphs: 0=.notdef, 1='0' (narrow), 2='0.zero' (wide), 3='A' (narrow), 4='A.ss01' (wide).
	glyphs := [][]byte{
		nil,
		rectGlyph(100, 0, 400, 700),
		rectGlyph(50, 0, 850, 700),
		rectGlyph(100, 0, 400, 700),
		rectGlyph(50, 0, 850, 700),
	}
	var glyf, loca []byte
	for _, g := range glyphs {
		loca = be32(loca, uint32(len(glyf)))
		glyf = append(glyf, g...)
	}
	loca = be32(loca, uint32(len(glyf)))

	// hmtx: advanceWidth, lsb per glyph.
	advances := []uint16{500, 500, 900, 500, 900}
	var hmtx []byte
	for _, a := range advances {
		hmtx = be16(hmtx, a)
		hmtx = be16(hmtx, 50)
	}

	// head (54 bytes), indexToLocFormat = 1 (long offsets).
	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head[0:], 0x00010000)
	binary.BigEndian.PutUint32(head[12:], 0x5F0F3CF5) // magicNumber
	binary.BigEndian.PutUint16(head[16:], 0x000B)     // flags
	binary.BigEndian.PutUint16(head[18:], 1000)       // unitsPerEm
	binary.BigEndian.PutUint16(head[36:], 0)          // xMin
	binary.BigEndian.PutUint16(head[38:], 0)          // yMin
	binary.BigEndian.PutUint16(head[40:], 900)        // xMax
	binary.BigEndian.PutUint16(head[42:], 700)        // yMax
	binary.BigEndian.PutUint16(head[46:], 8)          // lowestRecPPEM
	binary.BigEndian.PutUint16(head[48:], 2)          // fontDirectionHint
	binary.BigEndian.PutUint16(head[50:], 1)          // indexToLocFormat = long

	// hhea (36 bytes).
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint32(hhea[0:], 0x00010000)
	binary.BigEndian.PutUint16(hhea[4:], 800)       // ascender
	binary.BigEndian.PutUint16(hhea[6:], i16(-200)) // descender
	binary.BigEndian.PutUint16(hhea[10:], 900)      // advanceWidthMax
	binary.BigEndian.PutUint16(hhea[18:], 1)        // caretSlopeRise
	binary.BigEndian.PutUint16(hhea[34:], uint16(len(glyphs)))

	// maxp v1.0 (32 bytes).
	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:], uint16(len(glyphs)))
	binary.BigEndian.PutUint16(maxp[6:], 4) // maxPoints
	binary.BigEndian.PutUint16(maxp[8:], 1) // maxContours

	// OS/2 v4 (96 bytes).
	os2 := make([]byte, 96)
	binary.BigEndian.PutUint16(os2[0:], 4)
	binary.BigEndian.PutUint16(os2[2:], 600)     // xAvgCharWidth
	binary.BigEndian.PutUint16(os2[4:], 400)     // usWeightClass
	binary.BigEndian.PutUint16(os2[6:], 5)       // usWidthClass
	binary.BigEndian.PutUint16(os2[62:], 0x0040) // fsSelection = REGULAR
	binary.BigEndian.PutUint16(os2[64:], '0')
	binary.BigEndian.PutUint16(os2[66:], 'A')
	binary.BigEndian.PutUint16(os2[68:], 800)
	binary.BigEndian.PutUint16(os2[70:], i16(-200))
	binary.BigEndian.PutUint16(os2[74:], 800)
	binary.BigEndian.PutUint16(os2[76:], 200)
	binary.BigEndian.PutUint16(os2[86:], 500) // sxHeight
	binary.BigEndian.PutUint16(os2[88:], 700) // sCapHeight

	// post v3.0 (32 bytes).
	post := make([]byte, 32)
	binary.BigEndian.PutUint32(post[0:], 0x00030000)

	// cmap format 4 mapping '0' (0x30) -> 1, 'A' (0x41) -> 3.
	var cmapSub []byte
	cmapSub = be16(cmapSub, 4)  // format
	cmapSub = be16(cmapSub, 38) // length
	cmapSub = be16(cmapSub, 0)  // language
	cmapSub = be16(cmapSub, 6)  // segCountX2 (3 segments: '0', 'A', 0xFFFF)
	cmapSub = be16(cmapSub, 4)  // searchRange
	cmapSub = be16(cmapSub, 1)  // entrySelector
	cmapSub = be16(cmapSub, 2)  // rangeShift
	// endCode
	cmapSub = be16(cmapSub, '0')
	cmapSub = be16(cmapSub, 'A')
	cmapSub = be16(cmapSub, 0xFFFF)
	cmapSub = be16(cmapSub, 0) // reservedPad
	// startCode
	cmapSub = be16(cmapSub, '0')
	cmapSub = be16(cmapSub, 'A')
	cmapSub = be16(cmapSub, 0xFFFF)
	// idDelta: (glyph - code) mod 65536
	cmapSub = be16(cmapSub, i16(1-'0'))
	cmapSub = be16(cmapSub, i16(3-'A'))
	cmapSub = be16(cmapSub, 1)
	// idRangeOffset
	cmapSub = be16(cmapSub, 0)
	cmapSub = be16(cmapSub, 0)
	cmapSub = be16(cmapSub, 0)
	var cmap []byte
	cmap = be16(cmap, 0)  // version
	cmap = be16(cmap, 1)  // numTables
	cmap = be16(cmap, 3)  // platformID (Windows)
	cmap = be16(cmap, 1)  // encodingID (Unicode BMP)
	cmap = be32(cmap, 12) // offset
	cmap = append(cmap, cmapSub...)

	// name table: family (1), subfamily (2), full name (4), PostScript name (6).
	utf16BE := func(s string) []byte {
		out := make([]byte, 0, len(s)*2)
		for _, r := range s {
			out = be16(out, uint16(r))
		}
		return out
	}
	nameEntries := []struct {
		id  uint16
		val string
	}{
		{1, family},
		{2, "Regular"},
		{4, family},
		{6, family},
	}
	var nameStrs []byte
	var nameRecs []byte
	for _, e := range nameEntries {
		enc := utf16BE(e.val)
		nameRecs = be16(nameRecs, 3)      // platformID Windows
		nameRecs = be16(nameRecs, 1)      // encodingID Unicode BMP
		nameRecs = be16(nameRecs, 0x0409) // en-US
		nameRecs = be16(nameRecs, e.id)
		nameRecs = be16(nameRecs, uint16(len(enc)))
		nameRecs = be16(nameRecs, uint16(len(nameStrs)))
		nameStrs = append(nameStrs, enc...)
	}
	var nameTab []byte
	nameTab = be16(nameTab, 0)
	nameTab = be16(nameTab, uint16(len(nameEntries)))
	nameTab = be16(nameTab, uint16(6+len(nameRecs)))
	nameTab = append(nameTab, nameRecs...)
	nameTab = append(nameTab, nameStrs...)

	// GSUB v1.0 with scripts DFLT + latn, features ss01 (lookup 0: GID 3->4) and zero (lookup 1: GID 1->2).
	// Header (10 bytes): major=1, minor=0, scriptListOff, featureListOff, lookupListOff.
	// ScriptList: count=2 (DFLT, latn) sharing one ScriptTable -> DefaultLangSys (features 0 and 1).
	var scriptList []byte
	scriptList = be16(scriptList, 2) // scriptCount
	scriptList = append(scriptList, "DFLT"...)
	scriptList = be16(scriptList, 14) // offset to ScriptTable
	scriptList = append(scriptList, "latn"...)
	scriptList = be16(scriptList, 14) // offset to ScriptTable
	// ScriptTable at offset 14: defaultLangSys=4, langSysCount=0
	scriptList = be16(scriptList, 4)
	scriptList = be16(scriptList, 0)
	// LangSysTable at offset 18: lookupOrder=0, reqFeatureIndex=0xFFFF, featureIndexCount=2, indices=[0, 1]
	scriptList = be16(scriptList, 0)
	scriptList = be16(scriptList, 0xFFFF)
	scriptList = be16(scriptList, 2)
	scriptList = be16(scriptList, 0)
	scriptList = be16(scriptList, 1)

	// FeatureList: count=2 ("ss01" -> lookup 0, "zero" -> lookup 1).
	var featureList []byte
	featureList = be16(featureList, 2)
	featureList = append(featureList, "ss01"...)
	featureList = be16(featureList, 14)
	featureList = append(featureList, "zero"...)
	featureList = be16(featureList, 20)
	// FeatureTable 0 (6 bytes) at offset 14
	featureList = be16(featureList, 0) // featureParamsOffset
	featureList = be16(featureList, 1) // lookupIndexCount
	featureList = be16(featureList, 0) // lookupListIndices[0]
	// FeatureTable 1 (6 bytes) at offset 20
	featureList = be16(featureList, 0)
	featureList = be16(featureList, 1)
	featureList = be16(featureList, 1)

	// SingleSubst lookup helper (Lookup + SingleSubstFormat2 + CoverageFormat1).
	makeSingleSubst := func(fromGID, toGID uint16) []byte {
		var l []byte
		l = be16(l, 1) // lookupType = Single Substitution
		l = be16(l, 0) // lookupFlag
		l = be16(l, 1) // subTableCount
		l = be16(l, 8) // subtableOffsets[0] = 8
		// SingleSubstFormat2 at offset 8 (8 bytes): format=2, coverageOffset=8, glyphCount=1, substituteGlyphIDs=[toGID]
		l = be16(l, 2)
		l = be16(l, 8)
		l = be16(l, 1)
		l = be16(l, toGID)
		// CoverageFormat1 at offset 16 (6 bytes): format=1, glyphCount=1, glyphArray=[fromGID]
		l = be16(l, 1)
		l = be16(l, 1)
		l = be16(l, fromGID)
		return l
	}
	l0 := makeSingleSubst(3, 4) // 'A' -> 'A.ss01'
	l1 := makeSingleSubst(1, 2) // '0' -> '0.zero'
	var lookupList []byte
	lookupList = be16(lookupList, 2)
	lookupList = be16(lookupList, 6)
	lookupList = be16(lookupList, uint16(6+len(l0)))
	lookupList = append(lookupList, l0...)
	lookupList = append(lookupList, l1...)

	scriptOff := 10
	featureOff := scriptOff + len(scriptList)
	lookupOff := featureOff + len(featureList)
	var gsub []byte
	gsub = be16(gsub, 1)
	gsub = be16(gsub, 0)
	gsub = be16(gsub, uint16(scriptOff))
	gsub = be16(gsub, uint16(featureOff))
	gsub = be16(gsub, uint16(lookupOff))
	gsub = append(gsub, scriptList...)
	gsub = append(gsub, featureList...)
	gsub = append(gsub, lookupList...)

	// Assemble SFNT container.
	type sfntTable struct {
		tag  string
		data []byte
	}
	tables := []sfntTable{
		{"GSUB", gsub},
		{"OS/2", os2},
		{"cmap", cmap},
		{"glyf", glyf},
		{"head", head},
		{"hhea", hhea},
		{"hmtx", hmtx},
		{"loca", loca},
		{"maxp", maxp},
		{"name", nameTab},
		{"post", post},
	}
	slices.SortFunc(tables, func(a, b sfntTable) int {
		if a.tag < b.tag {
			return -1
		}
		if a.tag > b.tag {
			return 1
		}
		return 0
	})
	numTables := uint16(len(tables))
	var out []byte
	out = be32(out, 0x00010000) // sfntVersion
	out = be16(out, numTables)
	out = be16(out, 128) // searchRange
	out = be16(out, 3)   // entrySelector
	out = be16(out, numTables*16-128)

	offset := uint32(12 + int(numTables)*16)
	var body []byte
	checksum := func(d []byte) uint32 {
		var sum uint32
		for i := 0; i < len(d); i += 4 {
			var v uint32
			for j := range 4 {
				v <<= 8
				if i+j < len(d) {
					v |= uint32(d[i+j])
				}
			}
			sum += v
		}
		return sum
	}
	headOffset := 0
	for _, tb := range tables {
		if tb.tag == "head" {
			headOffset = int(offset)
		}
		out = append(out, tb.tag...)
		out = be32(out, checksum(tb.data))
		out = be32(out, offset)
		out = be32(out, uint32(len(tb.data)))
		body = append(body, tb.data...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		offset = uint32(12+int(numTables)*16) + uint32(len(body))
	}
	out = append(out, body...)
	adj := uint32(0xB1B0AFBA) - checksum(out)
	binary.BigEndian.PutUint32(out[headOffset+8:], adj)
	return out
}
