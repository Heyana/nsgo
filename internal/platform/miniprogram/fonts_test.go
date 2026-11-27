package miniprogram

import (
	"encoding/binary"
	"strings"
	"testing"
)

// minimalFont builds a minimal TrueType or OpenType font with OS/2 and name
// tables readable by fontfile.Describe.
func minimalFont(sfntTag, family string, weight uint16, italic bool) []byte {
	os2 := make([]byte, 64)
	binary.BigEndian.PutUint16(os2[4:], weight)
	if italic {
		binary.BigEndian.PutUint16(os2[62:], 1)
	}
	// Windows Unicode BMP English name record (platform 3, encoding 1, lang 0x0409, nameID 1).
	utf16Bytes := make([]byte, len(family)*2)
	for i := 0; i < len(family); i++ {
		binary.BigEndian.PutUint16(utf16Bytes[2*i:], uint16(family[i]))
	}
	nameTbl := make([]byte, 6+12+len(utf16Bytes))
	binary.BigEndian.PutUint16(nameTbl[2:], 1)    // count
	binary.BigEndian.PutUint16(nameTbl[4:], 6+12) // stringOffset
	rec := nameTbl[6:]
	binary.BigEndian.PutUint16(rec[0:], 3)
	binary.BigEndian.PutUint16(rec[2:], 1)
	binary.BigEndian.PutUint16(rec[4:], 0x0409)
	binary.BigEndian.PutUint16(rec[6:], 1)
	binary.BigEndian.PutUint16(rec[8:], uint16(len(utf16Bytes)))
	binary.BigEndian.PutUint16(rec[10:], 0)
	copy(nameTbl[6+12:], utf16Bytes)

	const numTables = 2
	headerLen := 12 + 16*numTables
	os2Off := headerLen
	nameOff := os2Off + len(os2)
	out := make([]byte, nameOff+len(nameTbl))
	copy(out[:4], sfntTag)
	binary.BigEndian.PutUint16(out[4:], numTables)

	writeRec := func(i int, tag string, off, length int) {
		r := out[12+16*i:]
		copy(r[:4], tag)
		binary.BigEndian.PutUint32(r[8:], uint32(off))
		binary.BigEndian.PutUint32(r[12:], uint32(length))
	}
	writeRec(0, "OS/2", os2Off, len(os2))
	writeRec(1, "name", nameOff, len(nameTbl))
	copy(out[os2Off:], os2)
	copy(out[nameOff:], nameTbl)
	return out
}

func TestBuildFontFaceSpec(t *testing.T) {
	ttf := minimalFont("\x00\x01\x00\x00", "Demo Sans", 600, false)
	spec, err := buildFontFaceSpec(ttf)
	if err != nil {
		t.Fatalf("buildFontFaceSpec(ttf): %v", err)
	}
	if spec.Family != "Demo Sans" || spec.Weight != "600" || spec.Style != "normal" {
		t.Fatalf("spec = %+v", spec)
	}
	if !strings.HasPrefix(spec.Source, `url("data:font/ttf;base64,`) || !strings.HasSuffix(spec.Source, `")`) {
		t.Fatalf("ttf source = %q", spec.Source)
	}

	otf := minimalFont("OTTO", "Demo Mono", 700, true)
	spec, err = buildFontFaceSpec(otf)
	if err != nil {
		t.Fatalf("buildFontFaceSpec(otf): %v", err)
	}
	if spec.Family != "Demo Mono" || spec.Weight != "700" || spec.Style != "italic" {
		t.Fatalf("otf spec = %+v", spec)
	}
	if !strings.HasPrefix(spec.Source, `url("data:font/otf;base64,`) {
		t.Fatalf("otf source = %q", spec.Source)
	}

	if _, err := buildFontFaceSpec([]byte("bad")); err == nil {
		t.Fatal("expected error for invalid font data")
	}
}

func TestResourceFileCandidates(t *testing.T) {
	for _, in := range []string{"fonts/Inter.ttf", "/fonts/Inter.ttf", "asset:fonts/Inter.ttf", "resources/fonts/Inter.ttf", "/resources/fonts/Inter.ttf"} {
		got := resourceFileCandidates(in)
		if len(got) != 2 || got[0] != "resources/fonts/Inter.ttf" || got[1] != "/resources/fonts/Inter.ttf" {
			t.Errorf("resourceFileCandidates(%q) = %v", in, got)
		}
	}
	if got := resourceFileCandidates(""); len(got) != 0 {
		t.Errorf("empty resourceFileCandidates = %v", got)
	}
}
