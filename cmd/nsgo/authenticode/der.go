package authenticode

import (
	"bytes"
	"encoding/asn1"
	"math/big"
	"sort"
	"time"
)

// DER, built by hand: Authenticode's content types are not structures
// encoding/asn1 marshals, and one of SpcPeImageData's fields, its flags,
// is a BIT STRING whose bits Authenticode's writers leave undefined.

// der wraps content in the tag and length every DER value carries.
func der(tag byte, content ...[]byte) []byte {
	n := 0
	for _, c := range content {
		n += len(c)
	}
	out := make([]byte, 0, 1+5+n)
	out = append(out, tag)
	switch {
	case n < 0x80:
		out = append(out, byte(n))
	case n <= 0xff:
		out = append(out, 0x81, byte(n))
	case n <= 0xffff:
		out = append(out, 0x82, byte(n>>8), byte(n))
	case n <= 0xffffff:
		out = append(out, 0x83, byte(n>>16), byte(n>>8), byte(n))
	default:
		out = append(out, 0x84, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	for _, c := range content {
		out = append(out, c...)
	}
	return out
}

func sequence(parts ...[]byte) []byte { return der(0x30, parts...) }

// set is a SET OF, whose members DER orders by their encodings.
func set(parts ...[]byte) []byte { return der(0x31, setAttributes(parts)...) }

// setAttributes are a SET OF's members in the order DER gives them: their
// encodings', byte for byte.
func setAttributes(parts [][]byte) [][]byte {
	sorted := append([][]byte(nil), parts...)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i], sorted[j]) < 0 })
	return sorted
}

// explicit tags content with the context-specific tag n, which keeps its
// own encoding under the tag, as an ASN.1 EXPLICIT tag does.
func explicit(n int, content ...[]byte) []byte { return der(byte(0xa0|n), content...) }

// implicit encodes content, which must be a value's contents and not the
// value, as a constructed value of the context-specific tag n, as an
// IMPLICIT tag over a SEQUENCE or SET does.
func implicit(n int, content ...[]byte) []byte { return der(byte(0xa0|n), content...) }

// implicitPrimitive is implicit over a primitive value, such as a string.
func implicitPrimitive(n int, content []byte) []byte { return der(byte(0x80|n), content) }

// oid is an object identifier's encoding.
func oid(id asn1.ObjectIdentifier) []byte {
	data, err := asn1.Marshal(id)
	if err != nil {
		panic(err) // the package's own identifiers, which always marshal
	}
	return data
}

func octetString(data []byte) []byte { return der(0x04, data) }

func null() []byte { return []byte{0x05, 0x00} }

func boolean(v bool) []byte {
	if v {
		return []byte{0x01, 0x01, 0xff}
	}
	return []byte{0x01, 0x01, 0x00}
}

// integer encodes v, its leading zero bytes gone and a zero byte in front
// of one whose top bit would read as a negative number's sign.
func integer(v *big.Int) []byte {
	b := v.Bytes()
	if len(b) == 0 {
		b = []byte{0}
	} else if b[0]&0x80 != 0 {
		b = append([]byte{0}, b...)
	}
	return der(0x02, b)
}

func smallInt(v int) []byte { return integer(big.NewInt(int64(v))) }

// utf16BE is s as UTF-16's big-endian bytes, the contents of a BMPSTRING,
// which holds the Basic Multilingual Plane alone.
func utf16BE(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		if r > 0xffff {
			r = '?'
		}
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

// utcTime is a time as CMS writes it: UTCTime, or GeneralizedTime from
// 2050 on, which UTCTime cannot say.
func utcTime(t time.Time) []byte {
	t = t.UTC()
	if t.Year() < 1950 || t.Year() >= 2050 {
		return der(0x18, []byte(t.Format("20060102150405Z")))
	}
	return der(0x17, []byte(t.Format("060102150405Z")))
}

// algorithmIdentifier is an algorithm's object identifier with the
// parameters CMS and Authenticode give it, NULL.
func algorithmIdentifier(id asn1.ObjectIdentifier) []byte {
	return sequence(oid(id), null())
}
