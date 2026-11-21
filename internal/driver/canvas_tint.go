package driver

// TintImagePixels multiplies src's channels by tint and writes dst in
// the same layout. Both slices hold four-byte pixels, and may be the same
// slice. Premultiplied colors also take tint's alpha.
func TintImagePixels(dst, src []byte, tint Color, bgra, premultiplied bool) {
	r, g, b, a := uint32(tint>>24), uint32(tint>>16&0xff), uint32(tint>>8&0xff), uint32(tint&0xff)
	if bgra {
		r, b = b, r
	}
	divisor := uint32(255)
	if premultiplied {
		r, g, b = r*a, g*a, b*a
		divisor *= 255
	}
	half := divisor / 2
	for i := 0; i+3 < len(src) && i+3 < len(dst); i += 4 {
		dst[i] = byte((uint32(src[i])*r + half) / divisor)
		dst[i+1] = byte((uint32(src[i+1])*g + half) / divisor)
		dst[i+2] = byte((uint32(src[i+2])*b + half) / divisor)
		dst[i+3] = byte((uint32(src[i+3])*a + 127) / 255)
	}
}
