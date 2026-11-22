package flatpak

import (
	"encoding/binary"
	"slices"
)

// A value is a little-endian GVariant, not a D-Bus wire value. Containers align their children and encode framing offsets as GLib's serialization specification requires. Only the types needed by OSTree are constructed.
type value struct {
	typ   string
	align int
	fixed bool
	data  []byte
}

func byteValue(n byte) value { return value{"y", 1, true, []byte{n}} }
func uint32Value(n uint32) value {
	return value{"u", 4, true, binary.LittleEndian.AppendUint32(nil, n)}
}
func uint64Value(n uint64) value {
	return value{"t", 8, true, binary.LittleEndian.AppendUint64(nil, n)}
}
func bytesValue(b []byte) value  { return value{"ay", 1, false, b} }
func stringValue(s string) value { return value{"s", 1, false, append([]byte(s), 0)} }
func variant(v value) value {
	return value{"v", 8, false, append(append(slices.Clone(v.data), 0), v.typ...)}
}
func dictionaryEntry(key string, v value) value {
	r := tuple(stringValue(key), variant(v))
	r.typ = "{sv}"
	return r
}
func dictionary(entries ...value) value { return array(dictionaryEntry("", byteValue(0)), entries...) }

func alignBytes(b []byte, n int) []byte {
	return append(b, make([]byte, (-len(b))&(n-1))...)
}

func framed(data []byte, offsets []int) []byte {
	if len(offsets) == 0 {
		return data
	}
	width := 1
	for uint64(len(data))+uint64(len(offsets))*uint64(width) > uint64(1)<<(width*8)-1 {
		width *= 2
	}
	for _, offset := range offsets {
		for i := range width {
			data = append(data, byte(uint64(offset)>>(8*i)))
		}
	}
	return data
}

func tuple(children ...value) value {
	r := value{typ: "(", align: 1, fixed: true}
	var offsets []int
	for i, child := range children {
		r.typ += child.typ
		r.align = max(r.align, child.align)
		r.fixed = r.fixed && child.fixed
		r.data = append(alignBytes(r.data, child.align), child.data...)
		if !child.fixed && i < len(children)-1 {
			offsets = append(offsets, len(r.data))
		}
	}
	r.typ += ")"
	if r.fixed {
		r.data = alignBytes(r.data, r.align)
	}
	slices.Reverse(offsets)
	r.data = framed(r.data, offsets)
	return r
}

// prototype supplies the element's type and alignment even for an empty array.
func array(prototype value, children ...value) value {
	r := value{typ: "a" + prototype.typ, align: prototype.align}
	var offsets []int
	for _, child := range children {
		r.data = append(alignBytes(r.data, child.align), child.data...)
		if !prototype.fixed {
			offsets = append(offsets, len(r.data))
		}
	}
	r.data = framed(r.data, offsets)
	return r
}
