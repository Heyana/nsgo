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
	case GPUFormatASTC4x4Unorm:
		decodeASTC4x4Block(block, out)
	case GPUFormatASTC4x4UnormSRGB:
		decodeASTC4x4BlockMode(block, out, true)
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

// BC7 interpolation weight tables (2-, 3-, and 4-bit) from the Khronos/DXGI BC7 specification.
var (
	bc7Weights2 = [4]int{0, 21, 43, 64}
	bc7Weights3 = [8]int{0, 9, 18, 27, 37, 46, 55, 64}
	bc7Weights4 = [16]int{0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64}
)

type bc7ModeDesc struct {
	subsets    int
	partBits   int
	rotBits    int
	idxSelBits int
	colorBits  int
	alphaBits  int
	epPBit     bool
	sharedPBit bool
	idxBits    int
	idx2Bits   int
}

var bc7Modes = [8]bc7ModeDesc{
	{subsets: 3, partBits: 4, colorBits: 4, epPBit: true, idxBits: 3},                            // Mode 0
	{subsets: 2, partBits: 6, colorBits: 6, sharedPBit: true, idxBits: 3},                        // Mode 1
	{subsets: 3, partBits: 6, colorBits: 5, idxBits: 2},                                          // Mode 2
	{subsets: 2, partBits: 6, colorBits: 7, epPBit: true, idxBits: 2},                            // Mode 3
	{subsets: 1, rotBits: 2, idxSelBits: 1, colorBits: 5, alphaBits: 6, idxBits: 2, idx2Bits: 3}, // Mode 4
	{subsets: 1, rotBits: 2, colorBits: 7, alphaBits: 8, idxBits: 2, idx2Bits: 2},                // Mode 5
	{subsets: 1, colorBits: 7, alphaBits: 7, epPBit: true, idxBits: 4},                           // Mode 6
	{subsets: 2, partBits: 6, colorBits: 5, alphaBits: 5, epPBit: true, idxBits: 2},              // Mode 7
}

// BC7 2-subset partition table (64 patterns, 1 bit per texel, bit i = texel i subset).
var bc7Partition2 = [64]uint16{
	0xCCCC, 0x8888, 0xEEEE, 0xECC8, 0xC880, 0xFEEC, 0xFEC8, 0xEC80,
	0xC800, 0xFFEC, 0xFE80, 0xE800, 0xFFE8, 0xFF00, 0xFFF0, 0xF000,
	0xF710, 0x008E, 0x7100, 0x08CE, 0x008C, 0x7310, 0x3100, 0x8CCE,
	0x088C, 0x3110, 0x6666, 0x366C, 0x17E8, 0x0FF0, 0x718E, 0x399C,
	0xAAAA, 0xF0F0, 0x5A5A, 0x33CC, 0x3C3C, 0x55AA, 0x9696, 0xA55A,
	0x73CE, 0x13C8, 0x324C, 0x3BDC, 0x6996, 0xC33C, 0x9966, 0x0660,
	0x0272, 0x04E4, 0x4E40, 0x2720, 0xC936, 0x936C, 0x39C6, 0x639C,
	0x9336, 0x9CC6, 0x817E, 0xE718, 0xCCF0, 0x0FCC, 0x7744, 0xEE22,
}

// BC7 3-subset partition table (64 patterns, 2 bits per texel, bits 2*i..2*i+1 = texel i subset).
var bc7Partition3 = [64]uint32{
	0xAA685050, 0x6A5A5040, 0x5A5A4200, 0x5450A0A8, 0xA5A50000, 0xA0A05050, 0x5555A0A0, 0x5A5A5050,
	0xAA550000, 0xAA555500, 0xAAAA5500, 0x90909090, 0x94949494, 0xA4A4A4A4, 0xA9A59450, 0x2A0A4250,
	0xA5945040, 0x0A425054, 0xA5A5A500, 0x55A0A0A0, 0xA8A85454, 0x6A6A4040, 0xA4A45000, 0x1A1A0500,
	0x0050A4A4, 0xAAA59090, 0x14696914, 0x69691400, 0xA08585A0, 0xAA821414, 0x50A4A450, 0x6A5A0200,
	0xA9A58000, 0x5090A0A8, 0xA8A09050, 0x24242424, 0x00AA5500, 0x24924924, 0x24499224, 0x50A50A50,
	0x500AA550, 0xAAAA4444, 0x66660000, 0xA5A0A5A0, 0x50A050A0, 0x69286928, 0x44AAAA44, 0x66666600,
	0xAA444444, 0x54A854A8, 0x95809580, 0x96969600, 0xA85454A8, 0x80959580, 0xAA141414, 0x96960000,
	0xAAAA1414, 0xA05050A0, 0xA0A5A5A0, 0x96000000, 0x40804080, 0xA9A8A9A8, 0xAAAAAA44, 0x2A4A5254,
}

var bc7AnchorIndex2 = [64]uint8{
	15, 15, 15, 15, 15, 15, 15, 15,
	15, 15, 15, 15, 15, 15, 15, 15,
	15, 2, 8, 2, 2, 8, 8, 15,
	2, 8, 2, 2, 8, 8, 2, 2,
	15, 15, 6, 8, 2, 8, 15, 15,
	2, 8, 2, 2, 2, 15, 15, 6,
	6, 2, 6, 8, 15, 15, 2, 2,
	15, 15, 15, 15, 15, 2, 2, 15,
}

var bc7AnchorIndex3_1 = [64]uint8{
	3, 3, 15, 15, 8, 3, 15, 15,
	8, 8, 6, 6, 6, 5, 3, 3,
	3, 3, 8, 15, 3, 3, 6, 10,
	5, 8, 8, 6, 8, 5, 15, 15,
	8, 15, 3, 5, 6, 10, 8, 15,
	15, 3, 15, 5, 15, 15, 15, 15,
	3, 15, 5, 5, 5, 8, 5, 10,
	5, 10, 8, 13, 15, 12, 3, 3,
}

var bc7AnchorIndex3_2 = [64]uint8{
	15, 8, 8, 3, 15, 15, 3, 8,
	15, 15, 15, 15, 15, 15, 15, 8,
	15, 8, 15, 3, 15, 8, 15, 8,
	3, 15, 6, 10, 15, 15, 10, 8,
	15, 3, 15, 10, 10, 8, 9, 10,
	6, 15, 8, 15, 3, 6, 6, 8,
	15, 3, 15, 15, 15, 15, 15, 15,
	15, 15, 15, 15, 3, 15, 15, 8,
}

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

func bc7Weight(bits int, idx uint32) int {
	switch bits {
	case 2:
		return bc7Weights2[idx&3]
	case 3:
		return bc7Weights3[idx&7]
	default:
		return bc7Weights4[idx&15]
	}
}

func bc7ExpandEndpoint(val, bits int) int {
	return (val << (8 - bits)) | (val >> (2*bits - 8))
}

// decodeBC7Block decodes a 16-byte BC7 4x4 block across all eight modes (0–7) per the Khronos/DXGI BC7 specification, including 2- and 3-subset partitions, anchor indices, unique/shared P-bits, rotation, and index selection. Reserved mode (0x00) outputs opaque black.
func decodeBC7Block(block []byte, out *[64]byte) {
	// Identify mode by counting leading zero bits (LSB-first).
	mode := -1
	for i := range 8 {
		if (block[0]>>i)&1 != 0 {
			mode = i
			break
		}
	}
	if mode < 0 {
		for i := range 16 {
			out[i*4+0] = 0
			out[i*4+1] = 0
			out[i*4+2] = 0
			out[i*4+3] = 255
		}
		return
	}

	m := bc7Modes[mode]
	pos := mode + 1

	part := 0
	if m.partBits > 0 {
		part = int(readBitsLE(block, pos, m.partBits))
		pos += m.partBits
	}
	rot := 0
	if m.rotBits > 0 {
		rot = int(readBitsLE(block, pos, m.rotBits))
		pos += m.rotBits
	}
	idxSel := 0
	if m.idxSelBits > 0 {
		idxSel = int(readBitsLE(block, pos, m.idxSelBits))
		pos += m.idxSelBits
	}

	numEP := 2 * m.subsets
	var ep [6][4]int
	for ch := range 3 {
		for e := range numEP {
			ep[e][ch] = int(readBitsLE(block, pos, m.colorBits))
			pos += m.colorBits
		}
	}
	if m.alphaBits > 0 {
		for e := range numEP {
			ep[e][3] = int(readBitsLE(block, pos, m.alphaBits))
			pos += m.alphaBits
		}
	}

	cBits := m.colorBits
	aBits := m.alphaBits
	if m.epPBit {
		for e := range numEP {
			p := int(readBitsLE(block, pos, 1))
			pos++
			for ch := range 3 {
				ep[e][ch] = (ep[e][ch] << 1) | p
			}
			if aBits > 0 {
				ep[e][3] = (ep[e][3] << 1) | p
			}
		}
		cBits++
		if aBits > 0 {
			aBits++
		}
	} else if m.sharedPBit {
		for s := range m.subsets {
			p := int(readBitsLE(block, pos, 1))
			pos++
			for k := range 2 {
				e := 2*s + k
				for ch := range 3 {
					ep[e][ch] = (ep[e][ch] << 1) | p
				}
			}
		}
		cBits++
	}

	for e := range numEP {
		for ch := range 3 {
			ep[e][ch] = bc7ExpandEndpoint(ep[e][ch], cBits)
		}
		if aBits > 0 {
			ep[e][3] = bc7ExpandEndpoint(ep[e][3], aBits)
		} else {
			ep[e][3] = 255
		}
	}

	anchor1, anchor2 := -1, -1
	if m.subsets == 2 {
		anchor1 = int(bc7AnchorIndex2[part])
	} else if m.subsets == 3 {
		anchor1 = int(bc7AnchorIndex3_1[part])
		anchor2 = int(bc7AnchorIndex3_2[part])
	}

	var idx1 [16]uint32
	for i := range 16 {
		bits := m.idxBits
		if i == 0 || i == anchor1 || i == anchor2 {
			bits--
		}
		idx1[i] = readBitsLE(block, pos, bits)
		pos += bits
	}

	var idx2 [16]uint32
	if m.idx2Bits > 0 {
		for i := range 16 {
			bits := m.idx2Bits
			if i == 0 {
				bits--
			}
			idx2[i] = readBitsLE(block, pos, bits)
			pos += bits
		}
	}

	for i := range 16 {
		subset := 0
		if m.subsets == 2 {
			subset = int((bc7Partition2[part] >> i) & 1)
		} else if m.subsets == 3 {
			subset = int((bc7Partition3[part] >> (2 * i)) & 3)
		}
		e0 := ep[2*subset]
		e1 := ep[2*subset+1]

		var cw, aw int
		if m.idx2Bits == 0 {
			w := bc7Weight(m.idxBits, idx1[i])
			cw, aw = w, w
		} else if idxSel == 0 {
			cw = bc7Weight(m.idxBits, idx1[i])
			aw = bc7Weight(m.idx2Bits, idx2[i])
		} else {
			cw = bc7Weight(m.idx2Bits, idx2[i])
			aw = bc7Weight(m.idxBits, idx1[i])
		}

		r := byte(((64-cw)*e0[0] + cw*e1[0] + 32) >> 6)
		g := byte(((64-cw)*e0[1] + cw*e1[1] + 32) >> 6)
		b := byte(((64-cw)*e0[2] + cw*e1[2] + 32) >> 6)
		a := byte(((64-aw)*e0[3] + aw*e1[3] + 32) >> 6)

		switch rot {
		case 1:
			r, a = a, r
		case 2:
			g, a = a, g
		case 3:
			b, a = a, b
		}

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

type astcQuantSpec struct {
	trits  int
	quints int
	bits   int
}

var astcQuantLevels = [21]astcQuantSpec{
	{0, 0, 1}, // 0: max 1
	{1, 0, 0}, // 1: max 2
	{0, 0, 2}, // 2: max 3
	{0, 1, 0}, // 3: max 4
	{1, 0, 1}, // 4: max 5
	{0, 0, 3}, // 5: max 7
	{0, 1, 1}, // 6: max 9
	{1, 0, 2}, // 7: max 11
	{0, 0, 4}, // 8: max 15
	{0, 1, 2}, // 9: max 19
	{1, 0, 3}, // 10: max 23
	{0, 0, 5}, // 11: max 31
	{0, 1, 3}, // 12: max 39
	{1, 0, 4}, // 13: max 47
	{0, 0, 6}, // 14: max 63
	{0, 1, 4}, // 15: max 79
	{1, 0, 5}, // 16: max 95
	{0, 0, 7}, // 17: max 127
	{0, 1, 5}, // 18: max 159
	{1, 0, 6}, // 19: max 191
	{0, 0, 8}, // 20: max 255
}

func astcBISEBits(n, level int) int {
	q := astcQuantLevels[level]
	if q.trits > 0 {
		return (8*n+4)/5 + n*q.bits
	}
	if q.quints > 0 {
		return (7*n+2)/3 + n*q.bits
	}
	return n * q.bits
}

func readBitsRevLE(block []byte, bitPos, count int) int {
	var v int
	for i := range count {
		bp := 127 - (bitPos + i)
		if (block[bp>>3]>>(bp&7))&1 != 0 {
			v |= 1 << i
		}
	}
	return v
}

func astcDecodeTritBlock(T int) [5]int {
	var C int
	var t [5]int
	if (T & 0x1C) == 0x1C {
		C = ((T & 0xE0) >> 3) | (T & 0x03)
		t[4] = 2
		t[3] = 2
	} else {
		C = T & 0x1F
		if (T & 0x60) == 0x60 {
			t[4] = 2
			t[3] = (T >> 7) & 1
		} else {
			t[4] = (T >> 7) & 1
			t[3] = (T >> 5) & 3
		}
	}
	if (C & 0x03) == 0x03 {
		t[2] = 2
		t[1] = (C >> 4) & 1
		t[0] = (C >> 2) & 3
		if t[0] == 3 {
			t[0] = 2
		}
	} else if (C & 0x0C) == 0x0C {
		t[2] = 2
		t[1] = 2
		t[0] = C & 3
	} else {
		t[2] = (C >> 4) & 1
		t[1] = (C >> 2) & 3
		t[0] = C & 3
	}
	return t
}

func astcDecodeQuintBlock(Q int) [3]int {
	if (Q & 0x07) == 5 {
		return [3]int{(Q >> 3) & 3, 4, (Q >> 5) & 3}
	}
	if (Q & 0x06) == 6 {
		switch (Q >> 5) & 3 {
		case 0:
			if Q&1 != 0 {
				return [3]int{4, 4, 4}
			}
			return [3]int{4, 4, (Q >> 3) & 3}
		case 1:
			if Q&1 != 0 {
				return [3]int{(Q >> 3) & 3, 4, 4}
			}
			return [3]int{4, (Q >> 3) & 3, 4}
		default:
			q0 := (((^Q >> 5) & 1) << 1) | (Q & 1)
			return [3]int{q0, (Q >> 3) & 3, 4}
		}
	}
	return [3]int{Q & 7, (Q >> 3) & 3, (Q >> 5) & 3}
}

func astcUnpackBISE(block []byte, startBit, count, level int, reverse bool, out []int) {
	read := func(pos, n int) int {
		if reverse {
			return readBitsRevLE(block, pos, n)
		}
		return int(readBitsLE(block, pos, n))
	}
	q := astcQuantLevels[level]
	if q.trits == 0 && q.quints == 0 {
		pos := startBit
		for i := range count {
			out[i] = read(pos, q.bits)
			pos += q.bits
		}
		return
	}

	pos := startBit
	if q.trits > 0 {
		tBitWidths := [5]int{2, 2, 1, 2, 1}
		tShifts := [5]int{0, 2, 4, 5, 7}
		for base := 0; base < count; base += 5 {
			rem := min(5, count-base)
			var m [5]int
			T := 0
			for j := range rem {
				if q.bits > 0 {
					m[j] = read(pos, q.bits)
					pos += q.bits
				}
				tb := read(pos, tBitWidths[j])
				pos += tBitWidths[j]
				T |= tb << tShifts[j]
			}
			t := astcDecodeTritBlock(T)
			for j := range rem {
				out[base+j] = (t[j] << q.bits) | m[j]
			}
		}
		return
	}

	qBitWidths := [3]int{3, 2, 2}
	qShifts := [3]int{0, 3, 5}
	for base := 0; base < count; base += 3 {
		rem := min(3, count-base)
		var m [3]int
		Q := 0
		for j := range rem {
			if q.bits > 0 {
				m[j] = read(pos, q.bits)
				pos += q.bits
			}
			qb := read(pos, qBitWidths[j])
			pos += qBitWidths[j]
			Q |= qb << qShifts[j]
		}
		qv := astcDecodeQuintBlock(Q)
		for j := range rem {
			out[base+j] = (qv[j] << q.bits) | m[j]
		}
	}
}

func astcUnquantizeWeight(level, val int) int {
	q := astcQuantLevels[level]
	var u int
	if q.trits == 0 && q.quints == 0 {
		switch q.bits {
		case 1:
			if val != 0 {
				u = 63
			}
		case 2:
			u = (val << 4) | (val << 2) | val
		case 3:
			u = (val << 3) | val
		case 4:
			u = (val << 2) | (val >> 2)
		case 5:
			u = (val << 1) | (val >> 4)
		}
	} else if q.bits == 0 {
		if q.trits > 0 {
			u = [3]int{0, 32, 63}[val]
		} else {
			u = [5]int{0, 16, 32, 47, 63}[val]
		}
	} else {
		var C int
		switch level {
		case 4:
			C = 50
		case 6:
			C = 28
		case 7:
			C = 23
		case 9:
			C = 13
		case 10:
			C = 11
		}
		a := val & 1
		A := 0
		if a != 0 {
			A = 0x7F
		}
		D := val >> q.bits
		x := (val >> 1) & ((1 << (q.bits - 1)) - 1)
		var B int
		switch level {
		case 7: // 1t+2b, x is 1 bit
			B = (x << 6) | (x << 2) | x
		case 9: // 1q+2b, x is 1 bit
			B = (x << 6) | (x << 1)
		case 10: // 1t+3b, x is 2 bits
			B = (x << 5) | x
		}
		T := (D*C + B) ^ A
		u = (A & 0x20) | (T >> 2)
	}
	if u > 32 {
		u++
	}
	return u
}

func astcUnquantizeColor(level, val int) int {
	q := astcQuantLevels[level]
	if q.trits == 0 && q.quints == 0 {
		switch q.bits {
		case 3:
			return (val << 5) | (val << 2) | (val >> 1)
		case 4:
			return (val << 4) | val
		case 5:
			return (val << 3) | (val >> 2)
		case 6:
			return (val << 2) | (val >> 4)
		case 7:
			return (val << 1) | (val >> 6)
		default:
			return val
		}
	}
	var C int
	switch level {
	case 4:
		C = 204
	case 6:
		C = 113
	case 7:
		C = 93
	case 9:
		C = 54
	case 10:
		C = 44
	case 12:
		C = 26
	case 13:
		C = 22
	case 15:
		C = 13
	case 16:
		C = 11
	case 18:
		C = 6
	case 19:
		C = 5
	}
	a := val & 1
	A := 0
	if a != 0 {
		A = 0x1FF
	}
	D := val >> q.bits
	b := (val >> 1) & ((1 << (q.bits - 1)) - 1)
	b0 := b & 1
	var B int
	switch level {
	case 7:
		B = (b0 << 8) | (b0 << 4) | (b0 << 2) | (b0 << 1)
	case 9:
		B = (b0 << 8) | (b0 << 3) | (b0 << 2)
	case 10:
		B = (b << 7) | (b << 2)
	case 12:
		B = (b << 7) | (b << 1)
	case 13:
		B = (b << 6) | b
	case 15:
		B = (b << 6) | (b >> 1)
	case 16:
		B = (b << 5) | (b >> 2)
	case 18:
		B = (b << 5) | (b >> 3)
	case 19:
		B = (b << 4) | (b >> 4)
	}
	T := (D*C + B) ^ A
	return (A & 0x80) | (T >> 2)
}

func astcHash52(p uint32) uint32 {
	p ^= p >> 15
	p *= 0xEEDE0891
	p ^= p >> 5
	p += p << 16
	p ^= p >> 7
	p ^= p >> 3
	p ^= p << 6
	p ^= p >> 17
	return p
}

// astcSelectPartition maps a 4x4 texel (x, y) to its partition index (0..partCount-1) using the ASTC small-block hash52 rule.
func astcSelectPartition(partCount, seed, x, y int) int {
	// 4x4 has 16 < 31 texels, so small-block coordinate doubling always applies.
	sx := uint32(x << 1)
	sy := uint32(y << 1)
	fullSeed := uint32(seed + (partCount-1)*1024)
	rnum := astcHash52(fullSeed)

	seed1 := rnum & 0xF
	seed2 := (rnum >> 4) & 0xF
	seed3 := (rnum >> 8) & 0xF
	seed4 := (rnum >> 12) & 0xF
	seed5 := (rnum >> 16) & 0xF
	seed6 := (rnum >> 20) & 0xF
	seed7 := (rnum >> 24) & 0xF
	seed8 := (rnum >> 28) & 0xF

	seed1 *= seed1
	seed2 *= seed2
	seed3 *= seed3
	seed4 *= seed4
	seed5 *= seed5
	seed6 *= seed6
	seed7 *= seed7
	seed8 *= seed8

	var sh1, sh2 uint
	if fullSeed&1 != 0 {
		if fullSeed&2 != 0 {
			sh1 = 4
		} else {
			sh1 = 5
		}
		if partCount == 3 {
			sh2 = 6
		} else {
			sh2 = 5
		}
	} else {
		if partCount == 3 {
			sh1 = 6
		} else {
			sh1 = 5
		}
		if fullSeed&2 != 0 {
			sh2 = 4
		} else {
			sh2 = 5
		}
	}

	seed1 >>= sh1
	seed2 >>= sh2
	seed3 >>= sh1
	seed4 >>= sh2
	seed5 >>= sh1
	seed6 >>= sh2
	seed7 >>= sh1
	seed8 >>= sh2

	a := (seed1*sx + seed2*sy + (rnum >> 14)) & 0x3F
	b := (seed3*sx + seed4*sy + (rnum >> 10)) & 0x3F
	c := (seed5*sx + seed6*sy + (rnum >> 6)) & 0x3F
	d := (seed7*sx + seed8*sy + (rnum >> 2)) & 0x3F

	if partCount < 4 {
		d = 0
	}
	if partCount < 3 {
		c = 0
	}
	if partCount < 2 {
		b = 0
	}

	if a >= b && a >= c && a >= d {
		return 0
	}
	if b >= c && b >= d {
		return 1
	}
	if c >= d {
		return 2
	}
	return 3
}

func astcBitTransferSigned(offset, base int) (int, int) {
	base >>= 1
	base |= offset & 0x80
	offset >>= 1
	offset &= 0x3F
	if offset&0x20 != 0 {
		offset -= 0x40
	}
	return offset, base
}

func astcDecodeLDREndpoints(cem int, v []int) (e0, e1 [4]byte, hdr bool) {
	clampPair := func(a, b, c, d int) [4]byte {
		return [4]byte{clampByte(a), clampByte(b), clampByte(c), clampByte(d)}
	}
	blueContract := func(r0, g0, b0, a0, r1, g1, b1, a1 int) ([4]byte, [4]byte) {
		return clampPair((r1+b1)>>1, (g1+b1)>>1, b1, a1),
			clampPair((r0+b0)>>1, (g0+b0)>>1, b0, a0)
	}

	switch cem {
	case 0: // LDR Luminance, direct
		e0 = clampPair(v[0], v[0], v[0], 255)
		e1 = clampPair(v[1], v[1], v[1], 255)
	case 1: // LDR Luminance, base+offset
		l0 := (v[0] >> 2) | (v[1] & 0xC0)
		l1 := min(255, l0+(v[1]&0x3F))
		e0 = clampPair(l0, l0, l0, 255)
		e1 = clampPair(l1, l1, l1, 255)
	case 4: // LDR Luminance+Alpha, direct
		e0 = clampPair(v[0], v[0], v[0], v[2])
		e1 = clampPair(v[1], v[1], v[1], v[3])
	case 5: // LDR Luminance+Alpha, base+offset
		dl, l0 := astcBitTransferSigned(v[1], v[0])
		da, a0 := astcBitTransferSigned(v[3], v[2])
		e0 = clampPair(l0, l0, l0, a0)
		e1 = clampPair(l0+dl, l0+dl, l0+dl, a0+da)
	case 6: // LDR RGB, base+scale
		e0 = clampPair((v[0]*v[3])>>8, (v[1]*v[3])>>8, (v[2]*v[3])>>8, 255)
		e1 = clampPair(v[0], v[1], v[2], 255)
	case 8: // LDR RGB, direct
		if v[1]+v[3]+v[5] >= v[0]+v[2]+v[4] {
			e0 = clampPair(v[0], v[2], v[4], 255)
			e1 = clampPair(v[1], v[3], v[5], 255)
		} else {
			e0, e1 = blueContract(v[0], v[2], v[4], 255, v[1], v[3], v[5], 255)
		}
	case 9: // LDR RGB, base+offset
		dr, r0 := astcBitTransferSigned(v[1], v[0])
		dg, g0 := astcBitTransferSigned(v[3], v[2])
		db, b0 := astcBitTransferSigned(v[5], v[4])
		if dr+dg+db >= 0 {
			e0 = clampPair(r0, g0, b0, 255)
			e1 = clampPair(r0+dr, g0+dg, b0+db, 255)
		} else {
			e0, e1 = blueContract(r0, g0, b0, 255, r0+dr, g0+dg, b0+db, 255)
		}
	case 10: // LDR RGB, base+scale plus two A
		e0 = clampPair((v[0]*v[3])>>8, (v[1]*v[3])>>8, (v[2]*v[3])>>8, v[4])
		e1 = clampPair(v[0], v[1], v[2], v[5])
	case 12: // LDR RGBA, direct
		if v[1]+v[3]+v[5] >= v[0]+v[2]+v[4] {
			e0 = clampPair(v[0], v[2], v[4], v[6])
			e1 = clampPair(v[1], v[3], v[5], v[7])
		} else {
			e0, e1 = blueContract(v[0], v[2], v[4], v[6], v[1], v[3], v[5], v[7])
		}
	case 13: // LDR RGBA, base+offset
		dr, r0 := astcBitTransferSigned(v[1], v[0])
		dg, g0 := astcBitTransferSigned(v[3], v[2])
		db, b0 := astcBitTransferSigned(v[5], v[4])
		da, a0 := astcBitTransferSigned(v[7], v[6])
		if dr+dg+db >= 0 {
			e0 = clampPair(r0, g0, b0, a0)
			e1 = clampPair(r0+dr, g0+dg, b0+db, a0+da)
		} else {
			e0, e1 = blueContract(r0, g0, b0, a0, r0+dr, g0+dg, b0+db, a0+da)
		}
	default: // HDR endpoint modes (2, 3, 7, 11, 14, 15)
		hdr = true
	}
	return e0, e1, hdr
}

func astcDecodeBlockMode4x4(bm uint32) (gridW, gridH, weightLevel int, dualPlane, ok bool) {
	if (bm & 0x0F) == 0 {
		return 0, 0, 0, false, false
	}
	var r, hBit int
	if (bm & 0x03) != 0 {
		r = int(((bm & 3) << 1) | ((bm >> 4) & 1))
		hBit = int((bm >> 9) & 1)
		dualPlane = (bm & (1 << 10)) != 0
		a := int((bm >> 5) & 3)
		b := int((bm >> 7) & 3)
		switch (bm >> 2) & 3 {
		case 0:
			gridW, gridH = b+4, a+2
		case 1:
			gridW, gridH = b+8, a+2
		case 2:
			gridW, gridH = a+2, b+8
		case 3:
			if (bm & (1 << 8)) == 0 {
				gridW, gridH = a+2, (b&1)+6
			} else {
				gridW, gridH = (b&1)+2, a+2
			}
		}
	} else {
		r = int(((bm >> 1) & 6) | ((bm >> 4) & 1))
		a := int((bm >> 5) & 3)
		switch (bm >> 7) & 3 {
		case 0:
			gridW, gridH = 12, a+2
			hBit = int((bm >> 9) & 1)
			dualPlane = (bm & (1 << 10)) != 0
		case 1:
			gridW, gridH = a+2, 12
			hBit = int((bm >> 9) & 1)
			dualPlane = (bm & (1 << 10)) != 0
		case 2:
			b := int((bm >> 9) & 3)
			gridW, gridH = a+6, b+6
		case 3:
			switch a {
			case 0:
				gridW, gridH = 6, 10
			case 1:
				gridW, gridH = 10, 6
			default:
				return 0, 0, 0, false, false
			}
			hBit = int((bm >> 9) & 1)
			dualPlane = (bm & (1 << 10)) != 0
		}
	}
	if gridW > 4 || gridH > 4 {
		return 0, 0, 0, false, false
	}
	weightLevel = hBit*6 + (r - 2)
	return gridW, gridH, weightLevel, dualPlane, true
}

func astcFillErrorColor(out *[64]byte) {
	for i := range 16 {
		out[i*4+0] = 255
		out[i*4+1] = 0
		out[i*4+2] = 255
		out[i*4+3] = 255
	}
}

func astcInfillWeights4x4(grid []int, gridW, gridH int, stride, offset int, out *[16]int) {
	if gridW == 4 && gridH == 4 {
		for i := range 16 {
			out[i] = grid[i*stride+offset]
		}
		return
	}
	// ASTC 4x4 step: ds = dt = (1024 + 4/2) / (4 - 1) = 342.
	const step = 342
	for y := range 4 {
		ct := step * y
		gt := (ct*(gridH-1) + 32) >> 6
		jt := gt >> 4
		ft := gt & 0xF
		for x := range 4 {
			cs := step * x
			gs := (cs*(gridW-1) + 32) >> 6
			js := gs >> 4
			fs := gs & 0xF

			v0 := jt*gridW + js
			p00 := grid[v0*stride+offset]
			var p01, p10, p11 int
			if js+1 < gridW {
				p01 = grid[(v0+1)*stride+offset]
			}
			if jt+1 < gridH {
				p10 = grid[(v0+gridW)*stride+offset]
				if js+1 < gridW {
					p11 = grid[(v0+gridW+1)*stride+offset]
				}
			}
			w11 := (fs*ft + 8) >> 4
			w10 := ft - w11
			w01 := fs - w11
			w00 := 16 - fs - ft + w11
			out[y*4+x] = (p00*w00 + p01*w01 + p10*w10 + p11*w11 + 8) >> 4
		}
	}
}

// decodeASTC4x4Block decodes a 16-byte ASTC 4x4 LDR UNORM block (void-extent, 1–4 partitions, dual-plane, weight grid infill up to 4x4, BISE trits/quints/bits, and all LDR color endpoint modes) per the Khronos ASTC specification. HDR endpoint modes and illegal encodings output the spec's error color (magenta, RGBA 255,0,255,255).
func decodeASTC4x4Block(block []byte, out *[64]byte) {
	decodeASTC4x4BlockMode(block, out, false)
}

func decodeASTC4x4BlockMode(block []byte, out *[64]byte, srgb bool) {
	bm := readBitsLE(block, 0, 11)
	// Void-extent block: bits 0..8 == 0x1FC.
	if (bm & 0x1FF) == 0x1FC {
		// 3D flag (bit 9) or non-LDR format (bits 10..11 != 0b11, including HDR void-extent 0b10) is an error in 2D LDR mode.
		if (bm&0x200) != 0 || readBitsLE(block, 10, 2) != 3 {
			astcFillErrorColor(out)
			return
		}
		minS := readBitsLE(block, 12, 13)
		maxS := readBitsLE(block, 25, 13)
		minT := readBitsLE(block, 38, 13)
		maxT := readBitsLE(block, 51, 13)
		if !(minS == 0x1FFF && maxS == 0x1FFF && minT == 0x1FFF && maxT == 0x1FFF) && (minS >= maxS || minT >= maxT) {
			astcFillErrorColor(out)
			return
		}
		var r, g, b byte
		if srgb {
			r, g, b = block[9], block[11], block[13]
		} else {
			r = byte((uint32(binary.LittleEndian.Uint16(block[8:10]))*255 + 32767) / 65535)
			g = byte((uint32(binary.LittleEndian.Uint16(block[10:12]))*255 + 32767) / 65535)
			b = byte((uint32(binary.LittleEndian.Uint16(block[12:14]))*255 + 32767) / 65535)
		}
		a := byte((uint32(binary.LittleEndian.Uint16(block[14:16]))*255 + 32767) / 65535)
		for i := range 16 {
			out[i*4+0] = r
			out[i*4+1] = g
			out[i*4+2] = b
			out[i*4+3] = a
		}
		return
	}

	gridW, gridH, weightLevel, dualPlane, ok := astcDecodeBlockMode4x4(bm)
	if !ok {
		astcFillErrorColor(out)
		return
	}
	planeMult := 1
	if dualPlane {
		planeMult = 2
	}
	numWeights := gridW * gridH * planeMult
	if numWeights > 64 {
		astcFillErrorColor(out)
		return
	}
	weightBits := astcBISEBits(numWeights, weightLevel)
	if weightBits < 24 || weightBits > 96 {
		astcFillErrorColor(out)
		return
	}

	partCount := int(readBitsLE(block, 11, 2)) + 1
	if partCount == 4 && dualPlane {
		astcFillErrorColor(out)
		return
	}

	var partSeed int
	var cems [4]int
	var endpointStart, extraCEMBits int
	if partCount == 1 {
		cems[0] = int(readBitsLE(block, 13, 4))
		endpointStart = 17
	} else {
		partSeed = int(readBitsLE(block, 13, 10))
		cemField := int(readBitsLE(block, 23, 6))
		baseClass := cemField & 3
		if baseClass == 0 {
			shared := (cemField >> 2) & 0xF
			for p := range partCount {
				cems[p] = shared
			}
		} else {
			extraCEMBits = 3*partCount - 4
			numTopBits := extraCEMBits
			if dualPlane {
				numTopBits += 2
			}
			if 29+numTopBits+weightBits > 128 {
				astcFillErrorColor(out)
				return
			}
			extraVal := int(readBitsLE(block, 128-weightBits-numTopBits, extraCEMBits))
			combined := (extraVal << 4) | ((cemField >> 2) & 0xF)
			for p := range partCount {
				m := (combined >> p) & 1
				l := (combined >> (partCount + 2*p)) & 3
				cems[p] = ((baseClass - 1 + m) << 2) | l
			}
		}
		endpointStart = 29
	}

	numCEMValues := 0
	for p := range partCount {
		numCEMValues += 2 * ((cems[p] >> 2) + 1)
	}
	if numCEMValues > 18 {
		astcFillErrorColor(out)
		return
	}

	endpointEnd := 128 - weightBits - extraCEMBits
	planeSelector := -1
	if dualPlane {
		endpointEnd -= 2
		planeSelector = int(readBitsLE(block, 128-weightBits-2, 2))
	}
	availBits := endpointEnd - endpointStart
	if availBits <= 0 {
		astcFillErrorColor(out)
		return
	}

	epLevel := -1
	for lv := 20; lv >= 4; lv-- {
		if astcBISEBits(numCEMValues, lv) <= availBits {
			epLevel = lv
			break
		}
	}
	if epLevel < 0 {
		astcFillErrorColor(out)
		return
	}

	var rawEp [18]int
	astcUnpackBISE(block, endpointStart, numCEMValues, epLevel, false, rawEp[:numCEMValues])
	for i := range numCEMValues {
		rawEp[i] = astcUnquantizeColor(epLevel, rawEp[i])
	}

	var endpoints [4][2][4]byte
	epOff := 0
	for p := range partCount {
		cnt := 2 * ((cems[p] >> 2) + 1)
		e0, e1, hdr := astcDecodeLDREndpoints(cems[p], rawEp[epOff:epOff+cnt])
		if hdr {
			astcFillErrorColor(out)
			return
		}
		endpoints[p][0] = e0
		endpoints[p][1] = e1
		epOff += cnt
	}

	var rawWeights [64]int
	astcUnpackBISE(block, 0, numWeights, weightLevel, true, rawWeights[:numWeights])
	for i := range numWeights {
		rawWeights[i] = astcUnquantizeWeight(weightLevel, rawWeights[i])
	}

	var w0, w1 [16]int
	astcInfillWeights4x4(rawWeights[:numWeights], gridW, gridH, planeMult, 0, &w0)
	if dualPlane {
		astcInfillWeights4x4(rawWeights[:numWeights], gridW, gridH, planeMult, 1, &w1)
	}

	for y := range 4 {
		for x := range 4 {
			i := y*4 + x
			part := 0
			if partCount > 1 {
				part = astcSelectPartition(partCount, partSeed, x, y)
			}
			e0 := endpoints[part][0]
			e1 := endpoints[part][1]
			for ch := range 4 {
				w := w0[i]
				if dualPlane && ch == planeSelector {
					w = w1[i]
				}
				if srgb && ch < 3 {
					c0 := (int(e0[ch]) << 8) | 0x80
					c1 := (int(e1[ch]) << 8) | 0x80
					c := (c0*(64-w) + c1*w + 32) >> 6
					out[i*4+ch] = byte(c >> 8)
				} else {
					c0 := (int(e0[ch]) << 8) | int(e0[ch])
					c1 := (int(e1[ch]) << 8) | int(e1[ch])
					c := (c0*(64-w) + c1*w + 32) >> 6
					out[i*4+ch] = byte((c*255 + 32767) / 65535)
				}
			}
		}
	}
}
