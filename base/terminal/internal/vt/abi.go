package vt

import "encoding/binary"

func abiSize(m manifest, name string) uintptr {
	switch name {
	case "pointer":
		return uintptr(m.ABI.PointerSize)
	case "usize", "isize":
		return uintptr(m.ABI.UsizeSize)
	case "bool", "u8", "i8":
		return 1
	case "u16", "i16":
		return 2
	case "u32", "i32", "f32":
		return 4
	case "u64", "i64", "f64":
		return 8
	}
	return m.Types[name].Size
}

// convertABI copies values, never addresses Go memory from guest pointers. A pointer remains an opaque guest address until an explicit memory access.
func convertABI(dst, src []byte, name string, to, from manifest) {
	t, ok := from.Types[name]
	if !ok || t.Kind != "struct" {
		if ok && t.Kind == "alias" {
			convertABI(dst, src, t.Type, to, from)
			return
		}
		clear(dst)
		copy(dst, src)
		return
	}
	for field, f := range t.Fields {
		d := to.Types[name].Fields[field]
		in, out := src[f.Offset:f.Offset+f.Size], dst[d.Offset:d.Offset+d.Size]
		if field == "size" && unsignedABI(in) != 0 {
			putUnsignedABI(out, uint64(len(dst)))
			continue
		}
		if f.Type == "array" {
			a, b := abiSize(from, f.Elem), abiSize(to, d.Elem)
			for i := range f.Count {
				convertABI(out[i*b:(i+1)*b], in[i*a:(i+1)*a], f.Elem, to, from)
			}
		} else {
			convertABI(out, in, f.Type, to, from)
		}
	}
}

func unsignedABI(b []byte) uint64 {
	var buf [8]byte
	copy(buf[:], b)
	return binary.LittleEndian.Uint64(buf[:])
}

func putUnsignedABI(b []byte, value uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	copy(b, buf[:])
}
