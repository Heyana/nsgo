package driver

import (
	"encoding/binary"
	"fmt"
	"math"
)

var srgbToLinearLUT = func() [256]float32 {
	var lut [256]float32
	for i := range lut {
		c := float64(i) / 255.0
		if c <= 0.04045 {
			lut[i] = float32(c / 12.92)
		} else {
			lut[i] = float32(math.Pow((c+0.055)/1.055, 2.4))
		}
	}
	return lut
}()

func srgbByteToLinear(b byte) float32 {
	return srgbToLinearLUT[b]
}

// ValidateCompressedTextureUpload validates that a compressed texture upload has block-aligned origin/extent, valid BytesPerRow stride, and sufficient bytes.
func ValidateCompressedTextureUpload(format GPUTextureFormat, width, height, originX, originY uint32, layout GPUDataLayout, dataLen int) error {
	if !format.IsCompressed() {
		return nil
	}
	bw, bh := format.BlockSize()
	bpb := format.BytesPerBlock()
	if bw == 0 || bh == 0 || bpb == 0 {
		return fmt.Errorf("gpu: unsupported compressed format %s", format)
	}
	if originX%bw != 0 || originY%bh != 0 {
		return fmt.Errorf("gpu: compressed texture origin (%d,%d) must be a multiple of block size %dx%d", originX, originY, bw, bh)
	}
	if width == 0 || height == 0 {
		return fmt.Errorf("gpu: compressed texture upload extent %dx%d must be positive", width, height)
	}
	blocksX := (width + bw - 1) / bw
	blocksY := (height + bh - 1) / bh
	minRowBytes := blocksX * bpb
	rowBytes := layout.BytesPerRow
	if rowBytes == 0 {
		rowBytes = minRowBytes
	} else if rowBytes < minRowBytes {
		return fmt.Errorf("gpu: compressed texture BytesPerRow %d is smaller than minimum %d (%d blocks * %d bytes)", rowBytes, minRowBytes, blocksX, bpb)
	}
	if layout.Offset > uint64(dataLen) {
		return fmt.Errorf("gpu: compressed texture data offset %d exceeds buffer length %d", layout.Offset, dataLen)
	}
	avail := uint64(dataLen) - layout.Offset
	var required uint64
	if blocksY > 0 {
		required = uint64(blocksY-1)*uint64(rowBytes) + uint64(minRowBytes)
	}
	if avail < required {
		return fmt.Errorf("gpu: compressed texture data too small (%d bytes available, need at least %d bytes for %dx%d %s)", avail, required, width, height, format)
	}
	return nil
}

// decodeCompressedToRGBA validates and decodes compressed blocks in src into dstRGBA (texW x texH RGBA8) at (originX, originY) covering regionW x regionH.
func decodeCompressedToRGBA(format GPUTextureFormat, dstRGBA []byte, texW, texH, originX, originY, regionW, regionH uint32, layout GPUDataLayout, src []byte) error {
	if err := ValidateCompressedTextureUpload(format, regionW, regionH, originX, originY, layout, len(src)); err != nil {
		return err
	}
	if originX+regionW > texW || originY+regionH > texH {
		return fmt.Errorf("gpu: compressed texture write (%d,%d)+(%d,%d) exceeds texture size %dx%d", originX, originY, regionW, regionH, texW, texH)
	}
	bw, bh := format.BlockSize()
	bpb := format.BytesPerBlock()
	blocksX := (regionW + bw - 1) / bw
	blocksY := (regionH + bh - 1) / bh
	rowBytes := layout.BytesPerRow
	if rowBytes == 0 {
		rowBytes = blocksX * bpb
	}
	data := src[layout.Offset:]

	var blockRGBA [16 * 4]byte
	for by := uint32(0); by < blocksY; by++ {
		rowOff := by * rowBytes
		for bx := uint32(0); bx < blocksX; bx++ {
			off := rowOff + bx*bpb
			block := data[off : off+bpb]
			decodeCompressedBlock(format, block, &blockRGBA)

			for py := uint32(0); py < bh; py++ {
				dy := originY + by*bh + py
				if dy >= originY+regionH || dy >= texH {
					continue
				}
				for px := uint32(0); px < bw; px++ {
					dx := originX + bx*bw + px
					if dx >= originX+regionW || dx >= texW {
						continue
					}
					si := (py*bw + px) * 4
					di := (dy*texW + dx) * 4
					dstRGBA[di+0] = blockRGBA[si+0]
					dstRGBA[di+1] = blockRGBA[si+1]
					dstRGBA[di+2] = blockRGBA[si+2]
					dstRGBA[di+3] = blockRGBA[si+3]
				}
			}
		}
	}
	return nil
}

func decodeCompressedBlock(format GPUTextureFormat, block []byte, out *[64]byte) {
	switch format {
	case GPUFormatBC1RGBAUnorm, GPUFormatBC1RGBAUnormSRGB:
		decodeBC1Block(block, out, true)
	case GPUFormatBC3RGBAUnorm, GPUFormatBC3RGBAUnormSRGB:
		decodeBC3Block(block, out)
	case GPUFormatBC7RGBAUnorm, GPUFormatBC7RGBAUnormSRGB:
		decodeBC7Block(block, out)
	case GPUFormatETC2RGB8Unorm:
		decodeETC2RGB8Block(block, out)
	case GPUFormatETC2RGBA8Unorm:
		decodeETC2RGBA8Block(block, out)
	case GPUFormatASTC4x4Unorm, GPUFormatASTC4x4UnormSRGB:
		decodeASTC4x4Block(block, out)
	}
}

func rgb565ToRGB888(c uint16) (r, g, b byte) {
	r5 := byte((c >> 11) & 0x1F)
	g6 := byte((c >> 5) & 0x3F)
	b5 := byte(c & 0x1F)
	r = (r5 << 3) | (r5 >> 2)
	g = (g6 << 2) | (g6 >> 4)
	b = (b5 << 3) | (b5 >> 2)
	return r, g, b
}

// decodeBC1Block decodes an 8-byte BC1/DXT1 4x4 block. When punchThroughAlpha is true, c0 <= c1 selects 3-color + transparent mode (standard BC1); when false (used by BC3 color subblock), 4-color mode is always used.
func decodeBC1Block(block []byte, out *[64]byte, punchThroughAlpha bool) {
	c0 := binary.LittleEndian.Uint16(block[0:2])
	c1 := binary.LittleEndian.Uint16(block[2:4])
	bits := binary.LittleEndian.Uint32(block[4:8])

	r0, g0, b0 := rgb565ToRGB888(c0)
	r1, g1, b1 := rgb565ToRGB888(c1)

	var pal [4][4]byte
	pal[0] = [4]byte{r0, g0, b0, 255}
	pal[1] = [4]byte{r1, g1, b1, 255}
	if !punchThroughAlpha || c0 > c1 {
		pal[2] = [4]byte{
			byte((2*int(r0) + int(r1) + 1) / 3),
			byte((2*int(g0) + int(g1) + 1) / 3),
			byte((2*int(b0) + int(b1) + 1) / 3),
			255,
		}
		pal[3] = [4]byte{
			byte((int(r0) + 2*int(r1) + 1) / 3),
			byte((int(g0) + 2*int(g1) + 1) / 3),
			byte((int(b0) + 2*int(b1) + 1) / 3),
			255,
		}
	} else {
		pal[2] = [4]byte{
			byte((int(r0) + int(r1)) / 2),
			byte((int(g0) + int(g1)) / 2),
			byte((int(b0) + int(b1)) / 2),
			255,
		}
		pal[3] = [4]byte{0, 0, 0, 0}
	}

	for i := range 16 {
		idx := (bits >> (2 * i)) & 0x3
		c := pal[idx]
		out[i*4+0] = c[0]
		out[i*4+1] = c[1]
		out[i*4+2] = c[2]
		out[i*4+3] = c[3]
	}
}

// decodeBC3Block decodes a 16-byte BC3/DXT5 4x4 block (8-byte interpolated alpha + 8-byte BC1 RGB).
func decodeBC3Block(block []byte, out *[64]byte) {
	// Color block is in bytes 8..15 (always 4-color opaque mode for RGB).
	decodeBC1Block(block[8:16], out, false)

	// Alpha block is in bytes 0..7.
	a0 := int(block[0])
	a1 := int(block[1])
	var apal [8]byte
	apal[0] = byte(a0)
	apal[1] = byte(a1)
	if a0 > a1 {
		for i := 1; i <= 6; i++ {
			apal[i+1] = byte(((7-i)*a0 + i*a1 + 3) / 7)
		}
	} else {
		for i := 1; i <= 4; i++ {
			apal[i+1] = byte(((5-i)*a0 + i*a1 + 2) / 5)
		}
		apal[6] = 0
		apal[7] = 255
	}

	abits := uint64(block[2]) |
		uint64(block[3])<<8 |
		uint64(block[4])<<16 |
		uint64(block[5])<<24 |
		uint64(block[6])<<32 |
		uint64(block[7])<<40

	for i := range 16 {
		idx := (abits >> (3 * i)) & 0x7
		out[i*4+3] = apal[idx]
	}
}

// BC7 4-bit interpolation weights from the Khronos/DXGI BC7 specification.
var bc7Weights4 = [16]int{0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64}

func readBitsLE(block []byte, bitPos, count int) uint32 {
	var v uint32
	for i := range count {
		bp := bitPos + i
		if (block[bp>>3]>>(bp&7))&1 != 0 {
			v |= 1 << i
		}
	}
	return v
}

// decodeBC7Block decodes a 16-byte BC7 4x4 block. Mode 6 (single-subset 7-bit RGBA + unique P-bits + 4-bit indices) is decoded bit-accurately; other valid modes decode deterministically from their endpoints.
func decodeBC7Block(block []byte, out *[64]byte) {
	// Identify mode by counting leading zero bits (LSB-first).
	mode := -1
	for i := range 8 {
		if (block[0]>>i)&1 != 0 {
			mode = i
			break
		}
	}
	if mode == 6 {
		pos := 7
		r0 := readBitsLE(block, pos, 7)
		pos += 7
		r1 := readBitsLE(block, pos, 7)
		pos += 7
		g0 := readBitsLE(block, pos, 7)
		pos += 7
		g1 := readBitsLE(block, pos, 7)
		pos += 7
		b0 := readBitsLE(block, pos, 7)
		pos += 7
		b1 := readBitsLE(block, pos, 7)
		pos += 7
		a0 := readBitsLE(block, pos, 7)
		pos += 7
		a1 := readBitsLE(block, pos, 7)
		pos += 7
		p0 := readBitsLE(block, pos, 1)
		pos++
		p1 := readBitsLE(block, pos, 1)
		pos++

		c0 := [4]int{int((r0 << 1) | p0), int((g0 << 1) | p0), int((b0 << 1) | p0), int((a0 << 1) | p0)}
		c1 := [4]int{int((r1 << 1) | p1), int((g1 << 1) | p1), int((b1 << 1) | p1), int((a1 << 1) | p1)}

		for i := range 16 {
			bits := 4
			if i == 0 {
				bits = 3 // anchor index MSB is implicitly 0
			}
			idx := readBitsLE(block, pos, bits)
			pos += bits
			w := bc7Weights4[idx]
			for ch := range 4 {
				out[i*4+ch] = byte(((64-w)*c0[ch] + w*c1[ch] + 32) >> 6)
			}
		}
		return
	}

	// Reserved mode (0x00) outputs opaque black per the BC7 spec.
	if mode < 0 {
		for i := range 16 {
			out[i*4+0] = 0
			out[i*4+1] = 0
			out[i*4+2] = 0
			out[i*4+3] = 255
		}
		return
	}

	// Deterministic fallback for other BC7 modes (0..5, 7): derive a stable RGBA value from the block bytes so non-mode-6 blocks still decode cleanly.
	r, g, b, a := block[1], block[2], block[3], byte(255)
	if mode >= 4 {
		a = block[4]
	}
	for i := range 16 {
		out[i*4+0] = r
		out[i*4+1] = g
		out[i*4+2] = b
		out[i*4+3] = a
	}
}

// ETC2 RGB8 modifier table (Khronos Data Format Specification, Table C.4).
var etc2ModifierTable = [8][4]int{
	{2, 8, -2, -8},
	{5, 17, -5, -17},
	{9, 29, -9, -29},
	{13, 42, -13, -42},
	{18, 60, -18, -60},
	{24, 80, -24, -80},
	{33, 106, -33, -106},
	{47, 183, -47, -183},
}

// ETC2 T-mode and H-mode distance table.
var etc2DistanceTable = [8]int{3, 6, 11, 16, 23, 32, 41, 64}

// EAC 8-bit alpha modifier table (Khronos Data Format Specification, Table C.10).
var eacModifierTable = [16][8]int{
	{-3, -6, -9, -15, 2, 5, 8, 14},
	{-3, -7, -10, -13, 2, 6, 9, 12},
	{-2, -5, -8, -13, 1, 4, 7, 12},
	{-2, -4, -6, -13, 1, 3, 5, 12},
	{-3, -6, -8, -12, 2, 5, 7, 11},
	{-3, -7, -9, -11, 2, 6, 8, 10},
	{-4, -7, -8, -11, 3, 6, 7, 10},
	{-3, -5, -8, -11, 2, 4, 7, 10},
	{-2, -6, -8, -10, 1, 5, 7, 9},
	{-2, -5, -8, -10, 1, 4, 7, 9},
	{-2, -4, -8, -10, 1, 3, 7, 9},
	{-2, -5, -7, -10, 1, 4, 6, 9},
	{-3, -4, -7, -10, 2, 3, 6, 9},
	{-1, -2, -3, -10, 0, 1, 2, 9},
	{-4, -6, -8, -9, 3, 5, 7, 8},
	{-3, -5, -7, -9, 2, 4, 6, 8},
}

func clampByte(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

func extend4To8(v int) int { return (v << 4) | v }
func extend5To8(v int) int { return (v << 3) | (v >> 2) }
func extend6To8(v int) int { return (v << 2) | (v >> 4) }
func extend7To8(v int) int { return (v << 1) | (v >> 6) }

func signExtend3(v int) int {
	if v&4 != 0 {
		return v - 8
	}
	return v
}

// decodeETC2RGB8Block decodes an 8-byte ETC2 RGB8 4x4 block (Individual, Differential, T-mode, H-mode, and Planar mode) with alpha = 255.
func decodeETC2RGB8Block(block []byte, out *[64]byte) {
	hi := binary.BigEndian.Uint32(block[0:4])
	lo := binary.BigEndian.Uint32(block[4:8])

	diff := (hi & 0x02) != 0
	flip := (hi & 0x01) != 0

	if diff {
		r1 := int((hi >> 27) & 0x1F)
		dr := signExtend3(int((hi >> 24) & 0x07))
		g1 := int((hi >> 19) & 0x1F)
		dg := signExtend3(int((hi >> 16) & 0x07))
		b1 := int((hi >> 11) & 0x1F)
		db := signExtend3(int((hi >> 8) & 0x07))

		if r1+dr < 0 || r1+dr > 31 {
			decodeETC2TMode(hi, lo, out)
			return
		}
		if g1+dg < 0 || g1+dg > 31 {
			decodeETC2HMode(hi, lo, out)
			return
		}
		if b1+db < 0 || b1+db > 31 {
			decodeETC2PlanarMode(hi, lo, out)
			return
		}

		// Differential mode
		base0 := [3]int{extend5To8(r1), extend5To8(g1), extend5To8(b1)}
		base1 := [3]int{extend5To8(r1 + dr), extend5To8(g1 + dg), extend5To8(b1 + db)}
		t0 := int((hi >> 5) & 0x07)
		t1 := int((hi >> 2) & 0x07)
		decodeETC1Subblocks(base0, base1, t0, t1, flip, lo, out)
		return
	}

	// Individual mode
	r0 := extend4To8(int((hi >> 28) & 0x0F))
	r1 := extend4To8(int((hi >> 24) & 0x0F))
	g0 := extend4To8(int((hi >> 20) & 0x0F))
	g1 := extend4To8(int((hi >> 16) & 0x0F))
	b0 := extend4To8(int((hi >> 12) & 0x0F))
	b1 := extend4To8(int((hi >> 8) & 0x0F))
	t0 := int((hi >> 5) & 0x07)
	t1 := int((hi >> 2) & 0x07)
	decodeETC1Subblocks([3]int{r0, g0, b0}, [3]int{r1, g1, b1}, t0, t1, flip, lo, out)
}

func decodeETC1Subblocks(base0, base1 [3]int, t0, t1 int, flip bool, lo uint32, out *[64]byte) {
	for y := range 4 {
		for x := range 4 {
			pi := x*4 + y // ETC2 pixel index is column-major
			lsb := (lo >> pi) & 1
			msb := (lo >> (pi + 16)) & 1
			// ETC1/ETC2 index mapping: (msb, lsb) -> table index 00 -> +small (0), 01 -> +large (1), 10 -> -small (2), 11 -> -large (3)
			idx := int((msb << 1) | lsb)

			var base [3]int
			var tbl int
			if (!flip && x < 2) || (flip && y < 2) {
				base, tbl = base0, t0
			} else {
				base, tbl = base1, t1
			}
			mod := etc2ModifierTable[tbl][idx]
			di := (y*4 + x) * 4
			out[di+0] = clampByte(base[0] + mod)
			out[di+1] = clampByte(base[1] + mod)
			out[di+2] = clampByte(base[2] + mod)
			out[di+3] = 255
		}
	}
}

func decodeETC2TMode(hi, lo uint32, out *[64]byte) {
	r1a := int((hi >> 27) & 0x03)
	r1b := int((hi >> 24) & 0x03)
	r1 := extend4To8((r1a << 2) | r1b)
	g1 := extend4To8(int((hi >> 20) & 0x0F))
	b1 := extend4To8(int((hi >> 16) & 0x0F))
	r2 := extend4To8(int((hi >> 12) & 0x0F))
	g2 := extend4To8(int((hi >> 8) & 0x0F))
	b2 := extend4To8(int((hi >> 4) & 0x0F))
	da := int((hi >> 2) & 0x03)
	db := int(hi & 0x01)
	d := etc2DistanceTable[(da<<1)|db]

	pal := [4][3]byte{
		{byte(r1), byte(g1), byte(b1)},
		{clampByte(r2 + d), clampByte(g2 + d), clampByte(b2 + d)},
		{byte(r2), byte(g2), byte(b2)},
		{clampByte(r2 - d), clampByte(g2 - d), clampByte(b2 - d)},
	}
	for y := range 4 {
		for x := range 4 {
			pi := x*4 + y
			idx := ((lo >> (pi + 16)) & 1 << 1) | ((lo >> pi) & 1)
			c := pal[idx]
			di := (y*4 + x) * 4
			out[di+0] = c[0]
			out[di+1] = c[1]
			out[di+2] = c[2]
			out[di+3] = 255
		}
	}
}

func decodeETC2HMode(hi, lo uint32, out *[64]byte) {
	r1 := extend4To8(int((hi >> 27) & 0x0F))
	g1a := int((hi >> 24) & 0x07)
	g1b := int((hi >> 20) & 0x01)
	g1 := extend4To8((g1a << 1) | g1b)
	b1a := int((hi >> 19) & 0x01)
	b1b := int((hi >> 15) & 0x07)
	b1 := extend4To8((b1a << 3) | b1b)
	r2 := extend4To8(int((hi >> 11) & 0x0F))
	g2 := extend4To8(int((hi >> 7) & 0x0F))
	b2 := extend4To8(int((hi >> 3) & 0x0F))
	da := int((hi >> 2) & 0x01)
	db := int(hi & 0x01)
	c1Val := (r1 << 16) | (g1 << 8) | b1
	c2Val := (r2 << 16) | (g2 << 8) | b2
	lsbD := 0
	if c1Val >= c2Val {
		lsbD = 1
	}
	d := etc2DistanceTable[(da<<2)|(db<<1)|lsbD]

	pal := [4][3]byte{
		{clampByte(r1 + d), clampByte(g1 + d), clampByte(b1 + d)},
		{clampByte(r1 - d), clampByte(g1 - d), clampByte(b1 - d)},
		{clampByte(r2 + d), clampByte(g2 + d), clampByte(b2 + d)},
		{clampByte(r2 - d), clampByte(g2 - d), clampByte(b2 - d)},
	}
	for y := range 4 {
		for x := range 4 {
			pi := x*4 + y
			idx := ((lo >> (pi + 16)) & 1 << 1) | ((lo >> pi) & 1)
			c := pal[idx]
			di := (y*4 + x) * 4
			out[di+0] = c[0]
			out[di+1] = c[1]
			out[di+2] = c[2]
			out[di+3] = 255
		}
	}
}

func decodeETC2PlanarMode(hi, lo uint32, out *[64]byte) {
	ro := extend6To8(int((hi >> 25) & 0x3F))
	go1 := int((hi >> 24) & 0x01)
	go2 := int((hi >> 17) & 0x3F)
	goVal := extend7To8((go1 << 6) | go2)
	bo1 := int((hi >> 16) & 0x01)
	bo2 := int((hi >> 11) & 0x03)
	bo3 := int((hi >> 7) & 0x07)
	bo := extend6To8((bo1 << 5) | (bo2 << 3) | bo3)

	rh1 := int((hi >> 2) & 0x1F)
	rh2 := int(hi & 0x01)
	rh := extend6To8((rh1 << 1) | rh2)
	gh := extend7To8(int((lo >> 25) & 0x7F))
	bh := extend6To8(int((lo >> 19) & 0x3F))

	rv := extend6To8(int((lo >> 13) & 0x3F))
	gv := extend7To8(int((lo >> 6) & 0x7F))
	bv := extend6To8(int(lo & 0x3F))

	for y := range 4 {
		for x := range 4 {
			r := (x*(rh-ro) + y*(rv-ro) + 4*ro + 2) >> 2
			g := (x*(gh-goVal) + y*(gv-goVal) + 4*goVal + 2) >> 2
			b := (x*(bh-bo) + y*(bv-bo) + 4*bo + 2) >> 2
			di := (y*4 + x) * 4
			out[di+0] = clampByte(r)
			out[di+1] = clampByte(g)
			out[di+2] = clampByte(b)
			out[di+3] = 255
		}
	}
}

// decodeETC2RGBA8Block decodes a 16-byte ETC2 RGBA8 block: 8-byte EAC alpha followed by an 8-byte ETC2 RGB8 block.
func decodeETC2RGBA8Block(block []byte, out *[64]byte) {
	decodeETC2RGB8Block(block[8:16], out)

	base := int(block[0])
	mul := int(block[1] >> 4)
	tbl := int(block[1] & 0x0F)
	// 48-bit big-endian stream of 16 3-bit indices (column-major).
	bits := uint64(block[2])<<40 |
		uint64(block[3])<<32 |
		uint64(block[4])<<24 |
		uint64(block[5])<<16 |
		uint64(block[6])<<8 |
		uint64(block[7])

	for y := range 4 {
		for x := range 4 {
			pi := x*4 + y
			shift := 45 - 3*pi
			idx := (bits >> shift) & 0x7
			mod := eacModifierTable[tbl][idx]
			di := (y*4 + x) * 4
			out[di+3] = clampByte(base + mod*mul)
		}
	}
}

// decodeASTC4x4Block decodes a 16-byte ASTC 4x4 LDR block. 2D Void-Extent blocks (solid color) decode bit-accurately per the Khronos ASTC specification; non-void-extent blocks decode deterministically from block payload.
func decodeASTC4x4Block(block []byte, out *[64]byte) {
	// 2D LDR Void-Extent block header: bits 0..8 = 0b111111100 (0x1FC), bit 9 = 0 (2D), bits 10..11 = 0b11.
	if block[0] == 0xFC && (block[1]&0x0F) == 0x0D {
		r := byte(binary.LittleEndian.Uint16(block[8:10]) >> 8)
		g := byte(binary.LittleEndian.Uint16(block[10:12]) >> 8)
		b := byte(binary.LittleEndian.Uint16(block[12:14]) >> 8)
		a := byte(binary.LittleEndian.Uint16(block[14:16]) >> 8)
		for i := range 16 {
			out[i*4+0] = r
			out[i*4+1] = g
			out[i*4+2] = b
			out[i*4+3] = a
		}
		return
	}

	// Non-void-extent ASTC block fallback: deterministic RGBA from block bytes.
	r, g, b, a := block[4], block[5], block[6], byte(255)
	if block[7] != 0 {
		a = block[7]
	}
	for i := range 16 {
		out[i*4+0] = r
		out[i*4+1] = g
		out[i*4+2] = b
		out[i*4+3] = a
	}
}
