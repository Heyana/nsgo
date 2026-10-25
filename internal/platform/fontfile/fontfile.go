// Package fontfile reads what the backends need to register an app's
// font files: the family, weight and slant of each face in a TrueType,
// OpenType or collection file, from its name and OS/2 tables. Backends
// whose systems read font files themselves (CoreText, DirectWrite,
// fontconfig) need none of it; the web's FontFace and Android's Typeface
// take the family from the app.
package fontfile

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

// Face is one face of a font file.
type Face struct {
	// Family is the face's typographic family, else its family.
	Family string
	// Weight is its OS/2 weight class, 100 to 900; 400 without one.
	Weight int
	Italic bool
}

// Describe returns the faces of a font file.
func Describe(data []byte) ([]Face, error) {
	if len(data) < 12 {
		return nil, errors.New("fontfile: too short")
	}
	if string(data[:4]) != "ttcf" {
		f, err := face(data, 0)
		if err != nil {
			return nil, err
		}
		return []Face{f}, nil
	}
	n := int(binary.BigEndian.Uint32(data[8:]))
	if n <= 0 || 12+4*n > len(data) {
		return nil, errors.New("fontfile: bad collection")
	}
	var out []Face
	for i := range n {
		f, err := face(data, int(binary.BigEndian.Uint32(data[12+4*i:])))
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// face reads the face whose table directory is at offset.
func face(data []byte, offset int) (Face, error) {
	if offset < 0 || offset+12 > len(data) {
		return Face{}, errors.New("fontfile: bad offset")
	}
	switch string(data[offset : offset+4]) {
	case "\x00\x01\x00\x00", "OTTO", "true":
	default:
		return Face{}, errors.New("fontfile: not a TrueType or OpenType font")
	}
	tables := map[string][]byte{}
	n := int(binary.BigEndian.Uint16(data[offset+4:]))
	for i := range n {
		rec := offset + 12 + 16*i
		if rec+16 > len(data) {
			return Face{}, errors.New("fontfile: bad table directory")
		}
		at, size := int(binary.BigEndian.Uint32(data[rec+8:])), int(binary.BigEndian.Uint32(data[rec+12:]))
		if at < 0 || size < 0 || at+size > len(data) {
			continue
		}
		tables[string(data[rec:rec+4])] = data[at : at+size]
	}
	f := Face{Weight: 400}
	if os2 := tables["OS/2"]; len(os2) >= 64 {
		if w := int(binary.BigEndian.Uint16(os2[4:])); w >= 1 && w <= 1000 {
			f.Weight = w
		}
		f.Italic = binary.BigEndian.Uint16(os2[62:])&1 != 0
	}
	names := tables["name"]
	family, typographic := name(names, 1), name(names, 16)
	f.Family = typographic
	if f.Family == "" {
		f.Family = family
	}
	if f.Family == "" {
		return Face{}, errors.New("fontfile: no family name")
	}
	return f, nil
}

// name returns name id's English string, preferring Windows' Unicode
// records, else Macintosh Roman's ASCII.
func name(t []byte, id uint16) string {
	if len(t) < 6 {
		return ""
	}
	count := int(binary.BigEndian.Uint16(t[2:]))
	strings := int(binary.BigEndian.Uint16(t[4:]))
	var mac string
	for i := range count {
		rec := 6 + 12*i
		if rec+12 > len(t) {
			break
		}
		platform, encoding := binary.BigEndian.Uint16(t[rec:]), binary.BigEndian.Uint16(t[rec+2:])
		language, nameID := binary.BigEndian.Uint16(t[rec+4:]), binary.BigEndian.Uint16(t[rec+6:])
		size, at := int(binary.BigEndian.Uint16(t[rec+8:])), strings+int(binary.BigEndian.Uint16(t[rec+10:]))
		if nameID != id || at+size > len(t) {
			continue
		}
		raw := t[at : at+size]
		switch {
		case platform == 3 && (encoding == 1 || encoding == 10) && (language == 0x409 || language&0xff == 0x09):
			units := make([]uint16, len(raw)/2)
			for k := range units {
				units[k] = binary.BigEndian.Uint16(raw[2*k:])
			}
			return string(utf16.Decode(units))
		case platform == 0:
			units := make([]uint16, len(raw)/2)
			for k := range units {
				units[k] = binary.BigEndian.Uint16(raw[2*k:])
			}
			if mac == "" {
				mac = string(utf16.Decode(units))
			}
		case platform == 1 && encoding == 0 && mac == "":
			mac = string(raw)
		}
	}
	return mac
}
