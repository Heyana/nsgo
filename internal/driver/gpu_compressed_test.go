package driver

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func encodeRGB565(r, g, b byte) uint16 {
	return (uint16(r>>3) << 11) | (uint16(g>>2) << 5) | uint16(b>>3)
}

func makeBC1SolidBlock(r, g, b byte) []byte {
	c0 := encodeRGB565(r, g, b)
	c1 := uint16(0)
	if c0 == 0 {
		c1 = 0xFFFF
		// Put c0 > c1 by using idx 1 for black
		out := make([]byte, 8)
		binary.LittleEndian.PutUint16(out[0:2], 0xFFFF)
		binary.LittleEndian.PutUint16(out[2:4], 0x0000)
		binary.LittleEndian.PutUint32(out[4:8], 0x55555555) // all idx 1
		return out
	}
	out := make([]byte, 8)
	binary.LittleEndian.PutUint16(out[0:2], c0)
	binary.LittleEndian.PutUint16(out[2:4], c1)
	binary.LittleEndian.PutUint32(out[4:8], 0x00000000) // all idx 0 -> c0
	return out
}

func writeBitsLE(buf []byte, bitPos, count int, val uint32) {
	for i := range count {
		if (val>>i)&1 != 0 {
			bp := bitPos + i
			buf[bp>>3] |= 1 << (bp & 7)
		}
	}
}

func makeBC7Mode6SolidBlock(r, g, b, a byte) []byte {
	out := make([]byte, 16)
	// Mode 6: bits 0..5 = 0, bit 6 = 1
	writeBitsLE(out, 6, 1, 1)
	pos := 7
	// Both endpoints equal; use LSB of R as P-bit so channels round-trip closely.
	p := uint32(r & 1)
	for _, ch := range []byte{r, r, g, g, b, b, a, a} {
		writeBitsLE(out, pos, 7, uint32(ch>>1))
		pos += 7
	}
	writeBitsLE(out, pos, 1, p)
	pos++
	writeBitsLE(out, pos, 1, p)
	// All pixel indices remain 0 -> endpoint 0.
	return out
}

func makeETC2IndividualBlock(r0, g0, b0, r1, g1, b1 byte, flip bool) []byte {
	// 4-bit per channel, table 0 (modifiers +2,+8,-2,-8), all pixel indices 0 (+2)
	var hi uint32
	hi |= uint32(r0>>4) << 28
	hi |= uint32(r1>>4) << 24
	hi |= uint32(g0>>4) << 20
	hi |= uint32(g1>>4) << 16
	hi |= uint32(b0>>4) << 12
	hi |= uint32(b1>>4) << 8
	// t0=0, t1=0, diff=0
	if flip {
		hi |= 1
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], hi)
	binary.BigEndian.PutUint32(out[4:8], 0) // all idx 0 (+2)
	return out
}

func makeASTC4x4VoidExtentBlock(r, g, b, a byte) []byte {
	out := make([]byte, 16)
	out[0] = 0xFC
	out[1] = 0xFD
	out[2] = 0xFF
	out[3] = 0xFF
	out[4] = 0xFF
	out[5] = 0xFF
	out[6] = 0xFF
	out[7] = 0xFF
	binary.LittleEndian.PutUint16(out[8:10], uint16(r)<<8|uint16(r))
	binary.LittleEndian.PutUint16(out[10:12], uint16(g)<<8|uint16(g))
	binary.LittleEndian.PutUint16(out[12:14], uint16(b)<<8|uint16(b))
	binary.LittleEndian.PutUint16(out[14:16], uint16(a)<<8|uint16(a))
	return out
}

func TestGPUTextureFormatMetadata(t *testing.T) {
	cases := []struct {
		fm         GPUTextureFormat
		compressed bool
		srgb       bool
		bw, bh     uint32
		bpb        uint32
		size8x4    uint64
	}{
		{GPUFormatUndefined, false, false, 0, 0, 0, 0},
		{GPUFormatRGBA8Unorm, false, false, 1, 1, 4, 128},
		{GPUFormatRGBA8UnormSRGB, false, true, 1, 1, 4, 128},
		{GPUFormatDepth24Plus, false, false, 1, 1, 0, 0},
		{GPUFormatBC1RGBAUnorm, true, false, 4, 4, 8, 16},
		{GPUFormatBC1RGBAUnormSRGB, true, true, 4, 4, 8, 16},
		{GPUFormatBC3RGBAUnorm, true, false, 4, 4, 16, 32},
		{GPUFormatBC3RGBAUnormSRGB, true, true, 4, 4, 16, 32},
		{GPUFormatBC7RGBAUnorm, true, false, 4, 4, 16, 32},
		{GPUFormatBC7RGBAUnormSRGB, true, true, 4, 4, 16, 32},
		{GPUFormatETC2RGB8Unorm, true, false, 4, 4, 8, 16},
		{GPUFormatETC2RGBA8Unorm, true, false, 4, 4, 16, 32},
		{GPUFormatASTC4x4Unorm, true, false, 4, 4, 16, 32},
		{GPUFormatASTC4x4UnormSRGB, true, true, 4, 4, 16, 32},
	}
	for _, tc := range cases {
		if got := tc.fm.IsCompressed(); got != tc.compressed {
			t.Errorf("%s.IsCompressed() = %v, want %v", tc.fm, got, tc.compressed)
		}
		if got := tc.fm.IsSRGB(); got != tc.srgb {
			t.Errorf("%s.IsSRGB() = %v, want %v", tc.fm, got, tc.srgb)
		}
		bw, bh := tc.fm.BlockSize()
		if bw != tc.bw || bh != tc.bh {
			t.Errorf("%s.BlockSize() = (%d,%d), want (%d,%d)", tc.fm, bw, bh, tc.bw, tc.bh)
		}
		if got := tc.fm.BytesPerBlock(); got != tc.bpb {
			t.Errorf("%s.BytesPerBlock() = %d, want %d", tc.fm, got, tc.bpb)
		}
		if got := tc.fm.ByteSize(8, 4); got != tc.size8x4 {
			t.Errorf("%s.ByteSize(8,4) = %d, want %d", tc.fm, got, tc.size8x4)
		}
	}
}

func TestCompressedBlockDecoders(t *testing.T) {
	t.Run("BC1_OpaqueAndPunchThrough", func(t *testing.T) {
		// Opaque mode: c0 = pure red (0xF800) > c1 = pure blue (0x001F). Pixel 0=idx0 (red), pixel 1=idx1 (blue), pixel 2=idx2 (2/3 red + 1/3 blue), pixel 3=idx3 (1/3 red + 2/3 blue).
		blk := make([]byte, 8)
		binary.LittleEndian.PutUint16(blk[0:2], 0xF800)
		binary.LittleEndian.PutUint16(blk[2:4], 0x001F)
		binary.LittleEndian.PutUint32(blk[4:8], 0|(1<<2)|(2<<4)|(3<<6))
		var out [64]byte
		decodeBC1Block(blk, &out, true)
		if out[0] != 255 || out[1] != 0 || out[2] != 0 || out[3] != 255 {
			t.Errorf("BC1 idx0 = %v, want [255 0 0 255]", out[0:4])
		}
		if out[4] != 0 || out[5] != 0 || out[6] != 255 || out[7] != 255 {
			t.Errorf("BC1 idx1 = %v, want [0 0 255 255]", out[4:8])
		}
		if out[8] != 170 || out[10] != 85 || out[11] != 255 {
			t.Errorf("BC1 idx2 = %v, want [170 0 85 255]", out[8:12])
		}
		if out[12] != 85 || out[14] != 170 || out[15] != 255 {
			t.Errorf("BC1 idx3 = %v, want [85 0 170 255]", out[12:16])
		}

		// Punch-through alpha mode: c0 <= c1 -> idx3 is transparent black (0,0,0,0).
		binary.LittleEndian.PutUint16(blk[0:2], 0x001F)
		binary.LittleEndian.PutUint16(blk[2:4], 0xF800)
		binary.LittleEndian.PutUint32(blk[4:8], (2<<0)|(3<<2))
		decodeBC1Block(blk, &out, true)
		if out[0] != 127 || out[2] != 127 || out[3] != 255 {
			t.Errorf("BC1 punch-through idx2 = %v, want [127 0 127 255]", out[0:4])
		}
		if out[4] != 0 || out[5] != 0 || out[6] != 0 || out[7] != 0 {
			t.Errorf("BC1 punch-through idx3 = %v, want [0 0 0 0]", out[4:8])
		}
	})

	t.Run("BC3_AlphaAndColor", func(t *testing.T) {
		blk := make([]byte, 16)
		// Alpha: a0=200 > a1=60; pixel 0=idx0 (200), pixel 1=idx1 (60), pixel 2=idx2 (180)
		blk[0] = 200
		blk[1] = 60
		abits := uint64(0) | (uint64(1) << 3) | (uint64(2) << 6)
		for i := range 6 {
			blk[2+i] = byte(abits >> (8 * i))
		}
		// Color: solid green
		copy(blk[8:16], makeBC1SolidBlock(0, 255, 0))
		var out [64]byte
		decodeBC3Block(blk, &out)
		if out[0] != 0 || out[1] != 255 || out[2] != 0 || out[3] != 200 {
			t.Errorf("BC3 px0 = %v, want [0 255 0 200]", out[0:4])
		}
		if out[7] != 60 {
			t.Errorf("BC3 px1 alpha = %d, want 60", out[7])
		}
		if out[11] != 180 {
			t.Errorf("BC3 px2 alpha = %d, want 180", out[11])
		}
	})

	t.Run("BC7_Mode6", func(t *testing.T) {
		blk := makeBC7Mode6SolidBlock(255, 129, 65, 201)
		var out [64]byte
		decodeBC7Block(blk, &out)
		if out[0] != 255 || out[1] != 129 || out[2] != 65 || out[3] != 201 {
			t.Errorf("BC7 mode6 px0 = %v, want [255 129 65 201]", out[0:4])
		}
		if out[60] != 255 || out[61] != 129 || out[62] != 65 || out[63] != 201 {
			t.Errorf("BC7 mode6 px15 = %v, want [255 129 65 201]", out[60:64])
		}
	})

	t.Run("BC7_AllModesAndReserved", func(t *testing.T) {
		encodeBC7 := func(mode, part, rot, idxSel int, ep [6][4]int, pbits []int, idx1, idx2 [16]int) []byte {
			blk := make([]byte, 16)
			m := bc7Modes[mode]
			writeBitsLE(blk, mode, 1, 1)
			pos := mode + 1
			if m.partBits > 0 {
				writeBitsLE(blk, pos, m.partBits, uint32(part))
				pos += m.partBits
			}
			if m.rotBits > 0 {
				writeBitsLE(blk, pos, m.rotBits, uint32(rot))
				pos += m.rotBits
			}
			if m.idxSelBits > 0 {
				writeBitsLE(blk, pos, m.idxSelBits, uint32(idxSel))
				pos += m.idxSelBits
			}
			numEP := 2 * m.subsets
			for ch := range 3 {
				for e := range numEP {
					writeBitsLE(blk, pos, m.colorBits, uint32(ep[e][ch]))
					pos += m.colorBits
				}
			}
			if m.alphaBits > 0 {
				for e := range numEP {
					writeBitsLE(blk, pos, m.alphaBits, uint32(ep[e][3]))
					pos += m.alphaBits
				}
			}
			for _, p := range pbits {
				writeBitsLE(blk, pos, 1, uint32(p))
				pos++
			}
			anchor1, anchor2 := -1, -1
			if m.subsets == 2 {
				anchor1 = int(bc7AnchorIndex2[part])
			} else if m.subsets == 3 {
				anchor1 = int(bc7AnchorIndex3_1[part])
				anchor2 = int(bc7AnchorIndex3_2[part])
			}
			for i := range 16 {
				bits := m.idxBits
				if i == 0 || i == anchor1 || i == anchor2 {
					bits--
				}
				writeBitsLE(blk, pos, bits, uint32(idx1[i]))
				pos += bits
			}
			if m.idx2Bits > 0 {
				for i := range 16 {
					bits := m.idx2Bits
					if i == 0 {
						bits--
					}
					writeBitsLE(blk, pos, bits, uint32(idx2[i]))
					pos += bits
				}
			}
			if pos != 128 {
				t.Fatalf("mode %d encoded bit count = %d, want 128", mode, pos)
			}
			return blk
		}

		// Reference verifier for a BC7 block given decoded/expanded parameters.
		verifyBC7 := func(t *testing.T, name string, mode, part, rot, idxSel int, ep [6][4]int, pbits []int, idx1, idx2 [16]int) {
			t.Helper()
			blk := encodeBC7(mode, part, rot, idxSel, ep, pbits, idx1, idx2)
			var out [64]byte
			decodeBC7Block(blk, &out)
			m := bc7Modes[mode]
			cBits, aBits := m.colorBits, m.alphaBits
			var expEP [6][4]int
			for e := range 2 * m.subsets {
				p := 0
				hasP := false
				if m.epPBit {
					p = pbits[e]
					hasP = true
				} else if m.sharedPBit {
					p = pbits[e/2]
					hasP = true
				}
				cb := cBits
				if hasP {
					cb++
				}
				ab := aBits
				for ch := range 3 {
					v := ep[e][ch]
					if hasP {
						v = (v << 1) | p
					}
					expEP[e][ch] = bc7ExpandEndpoint(v, cb)
				}
				if ab > 0 {
					v := ep[e][3]
					if m.epPBit {
						v = (v << 1) | p
						ab++
					}
					expEP[e][3] = bc7ExpandEndpoint(v, ab)
				} else {
					expEP[e][3] = 255
				}
			}
			for i := range 16 {
				sub := 0
				if m.subsets == 2 {
					sub = int((bc7Partition2[part] >> i) & 1)
				} else if m.subsets == 3 {
					sub = int((bc7Partition3[part] >> (2 * i)) & 3)
				}
				var cw, aw int
				if m.idx2Bits == 0 {
					w := bc7Weight(m.idxBits, uint32(idx1[i]))
					cw, aw = w, w
				} else if idxSel == 0 {
					cw = bc7Weight(m.idxBits, uint32(idx1[i]))
					aw = bc7Weight(m.idx2Bits, uint32(idx2[i]))
				} else {
					cw = bc7Weight(m.idx2Bits, uint32(idx2[i]))
					aw = bc7Weight(m.idxBits, uint32(idx1[i]))
				}
				e0, e1 := expEP[2*sub], expEP[2*sub+1]
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
				got := out[i*4 : i*4+4]
				want := []byte{r, g, b, a}
				if !bytes.Equal(got, want) {
					t.Fatalf("%s px%d = %v, want %v", name, i, got, want)
				}
			}
		}

		// Mode 0: 3 subsets, 4-bit part=5, 4-bit RGB + 6 unique P-bits, 3-bit indices.
		verifyBC7(t, "Mode0", 0, 5, 0, 0,
			[6][4]int{{15, 0, 0}, {15, 15, 0}, {0, 15, 0}, {0, 15, 15}, {0, 0, 15}, {15, 15, 15}},
			[]int{1, 0, 1, 1, 0, 1},
			[16]int{1, 5, 2, 3, 7, 4, 6, 0, 2, 5, 7, 1, 4, 6, 3, 2}, [16]int{})

		// Mode 1: 2 subsets, 6-bit part=17 (anchor1=2), 6-bit RGB + 2 shared P-bits, 3-bit indices.
		verifyBC7(t, "Mode1", 1, 17, 0, 0,
			[6][4]int{{60, 10, 5}, {12, 55, 40}, {5, 20, 62}, {48, 60, 10}},
			[]int{0, 1},
			[16]int{2, 6, 1, 7, 3, 5, 4, 0, 6, 2, 7, 1, 5, 3, 4, 7}, [16]int{})

		// Mode 2: 3 subsets, 6-bit part=26 (anchor1=8, anchor2=6), 5-bit RGB, 2-bit indices.
		verifyBC7(t, "Mode2", 2, 26, 0, 0,
			[6][4]int{{31, 0, 8}, {0, 31, 16}, {10, 20, 30}, {30, 10, 5}, {4, 4, 28}, {28, 28, 4}},
			nil,
			[16]int{1, 3, 2, 0, 3, 1, 1, 2, 0, 3, 2, 1, 0, 2, 3, 1}, [16]int{})

		// Mode 3: 2 subsets, 6-bit part=34 (checkerboard, anchor1=6), 7-bit RGB + 4 P-bits, 2-bit indices.
		verifyBC7(t, "Mode3", 3, 34, 0, 0,
			[6][4]int{{127, 10, 20}, {10, 127, 30}, {20, 30, 127}, {120, 110, 40}},
			[]int{1, 0, 1, 1},
			[16]int{1, 2, 3, 0, 2, 3, 1, 0, 3, 1, 2, 0, 1, 3, 2, 0}, [16]int{})

		// Mode 4: 1 subset, all rotations (0..3) and both idxSel (0, 1).
		for rot := range 4 {
			for idxSel := range 2 {
				verifyBC7(t, "Mode4", 4, 0, rot, idxSel,
					[6][4]int{{31, 4, 12, 60}, {2, 29, 24, 10}},
					nil,
					[16]int{1, 3, 2, 0, 3, 1, 0, 2, 2, 3, 1, 0, 1, 2, 3, 0},
					[16]int{2, 7, 5, 1, 6, 4, 3, 0, 7, 2, 6, 1, 5, 4, 3, 2})
			}
		}

		// Mode 5: 1 subset, all rotations (0..3), 7-bit RGB, 8-bit A, 2-bit color & alpha indices.
		for rot := range 4 {
			verifyBC7(t, "Mode5", 5, 0, rot, 0,
				[6][4]int{{120, 20, 10, 240}, {10, 110, 127, 30}},
				nil,
				[16]int{1, 3, 2, 0, 3, 1, 0, 2, 2, 3, 1, 0, 1, 2, 3, 0},
				[16]int{0, 2, 1, 3, 2, 0, 3, 1, 1, 3, 2, 0, 3, 1, 0, 2})
		}

		// Mode 6: gradient across all 4-bit indices 0..15.
		verifyBC7(t, "Mode6_Gradient", 6, 0, 0, 0,
			[6][4]int{{0, 127, 16, 127}, {127, 0, 112, 32}},
			[]int{0, 1},
			[16]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, [16]int{})

		// Mode 7: 2 subsets, 6-bit part=47 (anchor1=6), 5-bit RGBA + 4 P-bits, 2-bit indices.
		verifyBC7(t, "Mode7", 7, 47, 0, 0,
			[6][4]int{{31, 0, 16, 28}, {4, 30, 8, 12}, {2, 12, 31, 20}, {28, 24, 4, 31}},
			[]int{1, 0, 0, 1},
			[16]int{1, 3, 2, 0, 2, 1, 0, 3, 3, 0, 2, 1, 0, 2, 1, 3}, [16]int{})

		// Reserved mode (byte 0 == 0x00) -> opaque black.
		var reservedOut [64]byte
		decodeBC7Block(make([]byte, 16), &reservedOut)
		for i := range 16 {
			if !bytes.Equal(reservedOut[i*4:i*4+4], []byte{0, 0, 0, 255}) {
				t.Fatalf("BC7 reserved px%d = %v, want [0 0 0 255]", i, reservedOut[i*4:i*4+4])
			}
		}

		// Hand-built literal Mode 3 block (part=0: left 2 cols subset 0, right 2 cols subset 1;
		// subset 0: ep0=(0,0,0,255), ep1=(254,0,0,255); subset 1: ep2=(0,254,0,255), ep3=(0,0,254,255);
		// all indices 0 on left top/bottom -> red=0, index 3 on col 1 -> red=254; index 1 on col 2 -> (0+254)/3=85? w2[1]=21 -> (21*254+32)>>6=83).
		litMode3 := encodeBC7(3, 0, 0, 0,
			[6][4]int{{0, 0, 0}, {127, 0, 0}, {0, 127, 0}, {0, 0, 127}},
			[]int{0, 0, 0, 0},
			[16]int{0, 3, 1, 2, 0, 3, 1, 2, 0, 3, 1, 2, 0, 3, 1, 2}, [16]int{})
		var litOut [64]byte
		decodeBC7Block(litMode3, &litOut)
		if !bytes.Equal(litOut[0:4], []byte{0, 0, 0, 255}) ||
			!bytes.Equal(litOut[4:8], []byte{254, 0, 0, 255}) ||
			!bytes.Equal(litOut[8:12], []byte{0, 171, 83, 255}) {
			t.Fatalf("BC7 literal Mode3 row0 = %v, want [0 0 0 255] [254 0 0 255] [0 171 83 255]", litOut[0:12])
		}
	})

	t.Run("ETC2_RGB8_And_RGBA8", func(t *testing.T) {
		// Individual mode, flip=false: left 2x4 red (255,0,0), right 2x4 blue (0,0,255), mod +2
		rgbBlk := makeETC2IndividualBlock(0xF0, 0x00, 0x00, 0x00, 0x00, 0xF0, false)
		var out [64]byte
		decodeETC2RGB8Block(rgbBlk, &out)
		// (x=0,y=0) -> left subblock: (255, 2, 2, 255)
		if out[0] != 255 || out[1] != 2 || out[2] != 2 || out[3] != 255 {
			t.Errorf("ETC2 RGB8 (0,0) = %v, want [255 2 2 255]", out[0:4])
		}
		// (x=2,y=0) -> right subblock: (2, 2, 255, 255)
		if out[8] != 2 || out[9] != 2 || out[10] != 255 || out[11] != 255 {
			t.Errorf("ETC2 RGB8 (2,0) = %v, want [2 2 255 255]", out[8:12])
		}

		// ETC2 RGBA8: constant alpha 128 (base=128, mul=0, tbl=0) + rgbBlk
		rgbaBlk := make([]byte, 16)
		rgbaBlk[0] = 128
		rgbaBlk[1] = 0x00
		copy(rgbaBlk[8:16], rgbBlk)
		decodeETC2RGBA8Block(rgbaBlk, &out)
		if out[0] != 255 || out[3] != 128 {
			t.Errorf("ETC2 RGBA8 (0,0) = %v, want R=255 A=128", out[0:4])
		}
		if out[10] != 255 || out[11] != 128 {
			t.Errorf("ETC2 RGBA8 (2,0) = %v, want B=255 A=128", out[8:12])
		}
	})

	t.Run("ASTC4x4_VoidExtent", func(t *testing.T) {
		blk := makeASTC4x4VoidExtentBlock(32, 160, 224, 192)
		var out [64]byte
		decodeASTC4x4Block(blk, &out)
		for i := range 16 {
			if out[i*4+0] != 32 || out[i*4+1] != 160 || out[i*4+2] != 224 || out[i*4+3] != 192 {
				t.Fatalf("ASTC4x4 px%d = %v, want [32 160 224 192]", i, out[i*4:i*4+4])
			}
		}
	})

	t.Run("ASTC4x4_LDR_And_ErrorBlocks", func(t *testing.T) {
		writeBitsRev := func(buf []byte, bitPos, count int, val int) {
			for i := range count {
				if (val>>i)&1 != 0 {
					bp := 127 - (bitPos + i)
					buf[bp>>3] |= 1 << (bp & 7)
				}
			}
		}
		// Build lookup tables for inverse trit/quint block encoding from the decoder.
		var tritEnc [3][3][3][3][3]byte
		for T := range 256 {
			tv := astcDecodeTritBlock(T)
			tritEnc[tv[0]][tv[1]][tv[2]][tv[3]][tv[4]] = byte(T)
		}
		var quintEnc [5][5][5]byte
		for Q := range 128 {
			qv := astcDecodeQuintBlock(Q)
			quintEnc[qv[0]][qv[1]][qv[2]] = byte(Q)
		}
		packBISE := func(buf []byte, startBit int, level int, vals []int, reverse bool) {
			write := func(pos, n, v int) {
				if reverse {
					writeBitsRev(buf, pos, n, v)
				} else {
					writeBitsLE(buf, pos, n, uint32(v))
				}
			}
			q := astcQuantLevels[level]
			pos := startBit
			if q.trits == 0 && q.quints == 0 {
				for _, v := range vals {
					write(pos, q.bits, v)
					pos += q.bits
				}
				return
			}
			if q.trits > 0 {
				tBitWidths := [5]int{2, 2, 1, 2, 1}
				tShifts := [5]int{0, 2, 4, 5, 7}
				for base := 0; base < len(vals); base += 5 {
					rem := min(5, len(vals)-base)
					var tv [5]int
					var mv [5]int
					for j := range rem {
						tv[j] = vals[base+j] >> q.bits
						mv[j] = vals[base+j] & ((1 << q.bits) - 1)
					}
					T := int(tritEnc[tv[0]][tv[1]][tv[2]][tv[3]][tv[4]])
					for j := range rem {
						if q.bits > 0 {
							write(pos, q.bits, mv[j])
							pos += q.bits
						}
						write(pos, tBitWidths[j], (T>>tShifts[j])&((1<<tBitWidths[j])-1))
						pos += tBitWidths[j]
					}
				}
				return
			}
			qBitWidths := [3]int{3, 2, 2}
			qShifts := [3]int{0, 3, 5}
			for base := 0; base < len(vals); base += 3 {
				rem := min(3, len(vals)-base)
				var qv [3]int
				var mv [3]int
				for j := range rem {
					qv[j] = vals[base+j] >> q.bits
					mv[j] = vals[base+j] & ((1 << q.bits) - 1)
				}
				Q := int(quintEnc[qv[0]][qv[1]][qv[2]])
				for j := range rem {
					if q.bits > 0 {
						write(pos, q.bits, mv[j])
						pos += q.bits
					}
					write(pos, qBitWidths[j], (Q>>qShifts[j])&((1<<qBitWidths[j])-1))
					pos += qBitWidths[j]
				}
			}
		}

		// Encode 11-bit block mode for gridW, gridH in 2..4.
		makeBM := func(gridW, gridH, weightLevel int, dualPlane bool) uint32 {
			hBit := weightLevel / 6
			r := (weightLevel % 6) + 2
			var bm uint32
			bm |= uint32(r>>1) & 3
			bm |= uint32(r&1) << 4
			bm |= uint32(hBit) << 9
			if dualPlane {
				bm |= 1 << 10
			}
			if gridW == 4 {
				// case 0: b = gridW-4 = 0, a = gridH-2
				bm |= uint32(gridH-2) << 5
			} else {
				// case 3 with bit 8 = 1: gridW = (b&1)+2, gridH = a+2
				bm |= 3 << 2
				bm |= 1 << 8
				bm |= uint32(gridW-2) << 7
				bm |= uint32(gridH-2) << 5
			}
			return bm
		}

		// Helper mapping desired unquantized value -> closest raw BISE index for a given level.
		quantizeWeight := func(level, target int) int {
			q := astcQuantLevels[level]
			maxV := 1
			if q.trits > 0 {
				maxV = 3
			} else if q.quints > 0 {
				maxV = 5
			}
			maxV = maxV*(1<<q.bits) - 1
			bestV, bestDiff := 0, 1<<30
			for v := 0; v <= maxV; v++ {
				d := astcUnquantizeWeight(level, v) - target
				if d < 0 {
					d = -d
				}
				if d < bestDiff {
					bestDiff = d
					bestV = v
				}
			}
			return bestV
		}

		encodeASTC := func(gridW, gridH, weightLevel int, dualPlane bool, planeSel, partCount, partSeed int, cems []int, epVals []int, weights []int) []byte {
			blk := make([]byte, 16)
			bm := makeBM(gridW, gridH, weightLevel, dualPlane)
			writeBitsLE(blk, 0, 11, bm)
			writeBitsLE(blk, 11, 2, uint32(partCount-1))
			numWeights := len(weights)
			weightBits := astcBISEBits(numWeights, weightLevel)
			packBISE(blk, 0, weightLevel, weights, true)

			var endpointStart, extraCEMBits int
			if partCount == 1 {
				writeBitsLE(blk, 13, 4, uint32(cems[0]))
				endpointStart = 17
			} else {
				writeBitsLE(blk, 13, 10, uint32(partSeed))
				allSame := true
				for _, c := range cems {
					if c != cems[0] {
						allSame = false
						break
					}
				}
				if allSame {
					writeBitsLE(blk, 23, 6, uint32(cems[0]<<2))
				} else {
					minClass, maxClass := 3, 0
					for _, c := range cems {
						cl := c >> 2
						if cl < minClass {
							minClass = cl
						}
						if cl > maxClass {
							maxClass = cl
						}
					}
					baseClass := max(minClass+1, maxClass)
					if maxClass-minClass > 1 {
						t.Fatalf("incompatible multi-partition CEM classes %v", cems)
					}
					combined := 0
					for p, c := range cems {
						m := (c >> 2) - (baseClass - 1)
						l := c & 3
						combined |= m << p
						combined |= l << (partCount + 2*p)
					}
					cemField := baseClass | ((combined & 0xF) << 2)
					writeBitsLE(blk, 23, 6, uint32(cemField))
					extraCEMBits = 3*partCount - 4
					numTop := extraCEMBits
					if dualPlane {
						numTop += 2
					}
					writeBitsLE(blk, 128-weightBits-numTop, extraCEMBits, uint32(combined>>4))
				}
				endpointStart = 29
			}
			endpointEnd := 128 - weightBits - extraCEMBits
			if dualPlane {
				endpointEnd -= 2
				writeBitsLE(blk, 128-weightBits-2, 2, uint32(planeSel))
			}
			availBits := endpointEnd - endpointStart
			epLevel := -1
			for lv := 20; lv >= 4; lv-- {
				if astcBISEBits(len(epVals), lv) <= availBits {
					epLevel = lv
					break
				}
			}
			if epLevel < 0 {
				t.Fatalf("not enough endpoint bits (%d) for %d values", availBits, len(epVals))
			}
			packBISE(blk, endpointStart, epLevel, epVals, false)
			return blk
		}

		// 1. Test weight quantization levels (binary with dual-plane so weightBits >= 24, trit, quint, trit+bits, quint+bits, 4-bit) with 4x4 grid.
		rawW0 := make([]int, 32)
		for i := range 32 {
			rawW0[i] = i & 1
		}
		blkW0 := encodeASTC(4, 4, 0, true, 3, 1, 0, []int{4}, []int{0, 255, 0, 255}, rawW0)
		var outW0 [64]byte
		decodeASTC4x4Block(blkW0, &outW0)
		for i := range 16 {
			wantRGB := byte(0)
			if rawW0[2*i] != 0 {
				wantRGB = 255
			}
			wantA := byte(0)
			if rawW0[2*i+1] != 0 {
				wantA = 255
			}
			if !bytes.Equal(outW0[i*4:i*4+4], []byte{wantRGB, wantRGB, wantRGB, wantA}) {
				t.Fatalf("ASTC weightLevel=0 dual-plane px%d = %v, want [%d %d %d %d]", i, outW0[i*4:i*4+4], wantRGB, wantRGB, wantRGB, wantA)
			}
		}

		for _, lv := range []int{1, 3, 4, 6, 7, 8, 9, 10} {
			// CEM 8: RGB direct, epLevel=20 (8-bit exact: 84 bits avail -> 6*8=48 bits <= 84? wait: weightBits up to 16*4=64; 128-64-17=47 < 48 for lv=8!).
			// Use CEM 0 (2 values) or CEM 8 with quantize to epLevel!
			// With CEM 0 (2 values -> at most 16 bits, always epLevel 20 = 8-bit exact):
			rawW := make([]int, 16)
			for i := range 16 {
				rawW[i] = quantizeWeight(lv, (i*64)/15)
			}
			blk := encodeASTC(4, 4, lv, false, 0, 1, 0, []int{0}, []int{0, 255}, rawW)
			var out [64]byte
			decodeASTC4x4Block(blk, &out)
			for i := range 16 {
				w := astcUnquantizeWeight(lv, rawW[i])
				c := (65535*w + 32) >> 6
				want := byte((c*255 + 32767) / 65535)
				if out[i*4] != want || out[i*4+1] != want || out[i*4+2] != want || out[i*4+3] != 255 {
					t.Fatalf("ASTC weightLevel=%d px%d = %v, want [%d %d %d 255]", lv, i, out[i*4:i*4+4], want, want, want)
				}
			}
		}

		// 2. Weight grid infill (< 4x4): 2x2, 3x3, 4x2, 2x4.
		for _, gs := range [][2]int{{2, 2}, {3, 3}, {4, 2}, {2, 4}} {
			gw, gh := gs[0], gs[1]
			lv := 8 // 4-bit weights (even 4x2*4=32 bits >= 24; 2x2*4=16 < 24 so use lv=11 (5b -> 20) or lv=10? wait 2x2 needs >= 24 bits!)
			if gw*gh*4 < 24 {
				lv = 11 // 5-bit: 2x2=20? Wait: 2x2*5=20 < 24! For 2x2, dual-plane or 3*8? Wait: 2x2=4 weights max level 11 = 20 bits < 24, so 2x2 single-plane is illegal unless weightBits >= 24!
				// Dual-plane 2x2 at lv=7 (1t+2b: 4*2=8 weights -> (64+4)/5 + 16 = 29 bits >= 24)!
			}
			if gw == 2 && gh == 2 {
				rawW := []int{
					quantizeWeight(7, 0), quantizeWeight(7, 0),
					quantizeWeight(7, 64), quantizeWeight(7, 64),
					quantizeWeight(7, 32), quantizeWeight(7, 32),
					quantizeWeight(7, 64), quantizeWeight(7, 0),
				}
				blk := encodeASTC(2, 2, 7, true, 3, 1, 0, []int{12}, []int{0, 255, 0, 255, 0, 255, 0, 255}, rawW)
				var out [64]byte
				decodeASTC4x4Block(blk, &out)
				var uq [8]int
				for i, v := range rawW {
					uq[i] = astcUnquantizeWeight(7, v)
				}
				var w0, w1 [16]int
				astcInfillWeights4x4(uq[:], 2, 2, 2, 0, &w0)
				astcInfillWeights4x4(uq[:], 2, 2, 2, 1, &w1)
				for i := range 16 {
					rgb := byte((((65535*w0[i]+32)>>6)*255 + 32767) / 65535)
					a := byte((((65535*w1[i]+32)>>6)*255 + 32767) / 65535)
					if !bytes.Equal(out[i*4:i*4+4], []byte{rgb, rgb, rgb, a}) {
						t.Fatalf("ASTC 2x2 dual-plane infill px%d = %v, want [%d %d %d %d]", i, out[i*4:i*4+4], rgb, rgb, rgb, a)
					}
				}
				continue
			}
			rawW := make([]int, gw*gh)
			for y := range gh {
				for x := range gw {
					rawW[y*gw+x] = quantizeWeight(lv, ((x+y)*64)/(gw+gh-2))
				}
			}
			blk := encodeASTC(gw, gh, lv, false, 0, 1, 0, []int{8}, []int{0, 255, 16, 240, 255, 0}, rawW)
			var out [64]byte
			decodeASTC4x4Block(blk, &out)
			var uq [16]int
			for i, v := range rawW {
				uq[i] = astcUnquantizeWeight(lv, v)
			}
			var w0 [16]int
			astcInfillWeights4x4(uq[:gw*gh], gw, gh, 1, 0, &w0)
			// Verify endpoints unquantize & infill smoothly across rows/cols.
			if out[0] != 0 || out[15*4] != 255 {
				t.Fatalf("ASTC %dx%d infill corners = (%v, %v), want R=0 at (0,0) and R=255 at (3,3)", gw, gh, out[0:4], out[15*4:15*4+4])
			}
			if w0[5] == 0 || w0[5] == 64 {
				t.Fatalf("expected non-trivial interpolated weight at (1,1) for %dx%d, got %d", gw, gh, w0[5])
			}
		}

		// 3. All 10 LDR Color Endpoint Modes (0, 1, 4, 5, 6, 8, 9, 10, 12, 13), including blue-contraction.
		// Using 4x2 grid at level 2 (8 weights * 2 bits = 16? No, >=24 bits -> level 5 (3b): 8*3 = 24 bits! Leaves 128-17-24 = 87 bits -> epLevel=20 for up to 8 endpoint values!).
		w8 := []int{0, 2, 5, 7, 1, 3, 6, 4}
		cemCases := []struct {
			name string
			cem  int
			vals []int
		}{
			{"CEM0_LumDirect", 0, []int{20, 220}},
			{"CEM1_LumBaseOffset", 1, []int{160, 0x20}}, // l0=(160>>2)|0=40, l1=40+32=72
			{"CEM4_LumAlphaDirect", 4, []int{30, 200, 40, 240}},
			{"CEM5_LumAlphaBaseOffsetNeg", 5, []int{200, 0x3E, 180, 0xBE}}, // signed negative offset
			{"CEM6_RGBBaseScale", 6, []int{200, 100, 50, 128}},
			{"CEM8_RGBDirect", 8, []int{10, 240, 20, 200, 30, 180}},
			{"CEM8_BlueContract", 8, []int{240, 10, 200, 20, 180, 30}},
			{"CEM9_RGBBaseOffset", 9, []int{100, 0x40, 120, 0x20, 80, 0x10}},
			{"CEM9_BlueContractNeg", 9, []int{100, 0xFC, 120, 0xFC, 80, 0xFC}},
			{"CEM10_RGBBaseScale2A", 10, []int{220, 180, 90, 64, 32, 240}},
			{"CEM12_RGBADirect", 12, []int{12, 230, 34, 210, 56, 190, 78, 250}},
			{"CEM12_BlueContract", 12, []int{230, 12, 210, 34, 190, 56, 250, 78}},
			{"CEM13_RGBABaseOffset", 13, []int{120, 0x10, 140, 0x12, 160, 0x14, 180, 0x16}},
			{"CEM13_BlueContractNeg", 13, []int{120, 0xF0, 140, 0xEE, 160, 0xEC, 180, 0xEA}},
		}
		for _, tc := range cemCases {
			blk := encodeASTC(4, 2, 5, false, 0, 1, 0, []int{tc.cem}, tc.vals, w8)
			var out [64]byte
			decodeASTC4x4Block(blk, &out)
			e0, e1, hdr := astcDecodeLDREndpoints(tc.cem, tc.vals)
			if hdr {
				t.Fatalf("%s unexpectedly marked HDR", tc.name)
			}
			// Texel (0,0) has weight index 0 -> w=0 -> e0!
			if !bytes.Equal(out[0:4], e0[:]) {
				t.Fatalf("%s px(0,0) = %v, want e0=%v", tc.name, out[0:4], e0)
			}
			// Texel (3,0) has grid index 3 -> w=64 -> e1!
			if !bytes.Equal(out[3*4:3*4+4], e1[:]) {
				t.Fatalf("%s px(3,0) = %v, want e1=%v", tc.name, out[3*4:3*4+4], e1)
			}
		}

		// 4. Multi-partition (2, 3, and 4 partitions), shared CEM and per-partition CEMs with trits/quints endpoint quantization.
		w16 := []int{0, 1, 2, 3, 1, 2, 3, 0, 2, 3, 0, 1, 3, 0, 1, 2} // 16 * 2b = 32 bits (level 2)
		// 2 partitions, mixed CEMs (CEM 0 and CEM 4 -> baseClass 1), seed=42.
		// avail = 128 - 32 - 29 - 2 = 65 bits for 6 values -> epLevel 19 (1t+6b: (8*6+4)/5+36=46) or 20 (48 bits)!
		blk2p := encodeASTC(4, 4, 2, false, 0, 2, 42, []int{0, 4}, []int{20, 220, 10, 240, 50, 200}, w16)
		var out2p [64]byte
		decodeASTC4x4Block(blk2p, &out2p)
		for y := range 4 {
			for x := range 4 {
				i := y*4 + x
				p := astcSelectPartition(2, 42, x, y)
				w := astcUnquantizeWeight(2, w16[i])
				var e0, e1 [4]byte
				if p == 0 {
					e0, e1, _ = astcDecodeLDREndpoints(0, []int{20, 220})
				} else {
					e0, e1, _ = astcDecodeLDREndpoints(4, []int{10, 240, 50, 200})
				}
				for ch := range 4 {
					c0 := (int(e0[ch]) << 8) | int(e0[ch])
					c1 := (int(e1[ch]) << 8) | int(e1[ch])
					want := byte((((c0*(64-w)+c1*w+32)>>6)*255 + 32767) / 65535)
					if out2p[i*4+ch] != want {
						t.Fatalf("ASTC 2-part px(%d,%d) ch%d = %d, want %d", x, y, ch, out2p[i*4+ch], want)
					}
				}
			}
		}

		// 3 partitions, mixed CEMs (CEM 0, CEM 4, CEM 6 -> baseClass 1, extraCEMBits=5), dual-plane (planeSel=1 -> Green), seed=123.
		// Using 4x2 grid at level 5 (24 weight bits): avail = 128 - 24 - 29 - 5 - 2 = 68 bits for 10 values -> epLevel 14 (6b = 60 bits) or 15 (1q+4b: 64 bits)!
		// Let's use epLevel 15 (quint + 4 bits, range 0..79) to exercise quint endpoint unpacking!
		wDual := []int{0, 7, 2, 5, 4, 3, 6, 1, 7, 0, 5, 2, 3, 4, 1, 6} // 16 weights * 3b = 48 bits!
		// avail with 48 weight bits: 128 - 48 - 29 - 5 - 2 = 44 bits for 10 values -> epLevel 9 (1q+2b: (7*10+2)/3+20 = 44 bits exact!).
		ep3p := []int{1, 18, 2, 17, 3, 16, 4, 15, 5, 14} // quint+2b values in 0..19
		blk3pDual := encodeASTC(4, 2, 5, true, 1, 3, 123, []int{0, 4, 6}, ep3p, wDual)
		var out3pDual [64]byte
		decodeASTC4x4Block(blk3pDual, &out3pDual)
		var uqEp [10]int
		for i, v := range ep3p {
			uqEp[i] = astcUnquantizeColor(9, v)
		}
		var uqW [16]int
		for i, v := range wDual {
			uqW[i] = astcUnquantizeWeight(5, v)
		}
		var dw0, dw1 [16]int
		astcInfillWeights4x4(uqW[:], 4, 2, 2, 0, &dw0)
		astcInfillWeights4x4(uqW[:], 4, 2, 2, 1, &dw1)
		e3p := [3][2][4]byte{}
		e3p[0][0], e3p[0][1], _ = astcDecodeLDREndpoints(0, uqEp[0:2])
		e3p[1][0], e3p[1][1], _ = astcDecodeLDREndpoints(4, uqEp[2:6])
		e3p[2][0], e3p[2][1], _ = astcDecodeLDREndpoints(6, uqEp[6:10])
		for y := range 4 {
			for x := range 4 {
				i := y*4 + x
				p := astcSelectPartition(3, 123, x, y)
				for ch := range 4 {
					w := dw0[i]
					if ch == 1 {
						w = dw1[i]
					}
					c0 := (int(e3p[p][0][ch]) << 8) | int(e3p[p][0][ch])
					c1 := (int(e3p[p][1][ch]) << 8) | int(e3p[p][1][ch])
					want := byte((((c0*(64-w)+c1*w+32)>>6)*255 + 32767) / 65535)
					if out3pDual[i*4+ch] != want {
						t.Fatalf("ASTC 3-part dual-plane px(%d,%d) ch%d = %d, want %d", x, y, ch, out3pDual[i*4+ch], want)
					}
				}
			}
		}

		// 4 partitions, shared CEM 0, seed=2 (produces all 4 partitions on 4x4).
		blk4p := encodeASTC(4, 4, 2, false, 0, 4, 2, []int{0, 0, 0, 0}, []int{0, 64, 64, 128, 128, 192, 192, 255}, w16)
		var out4p [64]byte
		decodeASTC4x4Block(blk4p, &out4p)
		seenParts := map[int]bool{}
		for y := range 4 {
			for x := range 4 {
				seenParts[astcSelectPartition(4, 2, x, y)] = true
			}
		}
		if len(seenParts) != 4 {
			t.Fatalf("expected 4 active partitions for seed 2, got %v", seenParts)
		}
		if out4p[0] == 255 && out4p[1] == 0 && out4p[2] == 255 {
			t.Fatalf("ASTC 4-partition block unexpectedly decoded to error color")
		}

		// 5. ASTC sRGB vs UNORM: interpolated texels match spec rounding, while non-0x0101 void-extent uses top 8 bits in sRGB.
		srgbTestBlk := encodeASTC(4, 2, 5, false, 0, 1, 0, []int{8}, []int{0, 255, 0, 255, 0, 255}, []int{3, 3, 3, 3, 3, 3, 3, 3})
		var unormOut, srgbOut [64]byte
		decodeASTC4x4Block(srgbTestBlk, &unormOut)
		decodeASTC4x4BlockMode(srgbTestBlk, &srgbOut, true)
		if unormOut[0] != 108 || srgbOut[0] != 108 {
			t.Fatalf("ASTC UNORM vs sRGB interpolated = (%d, %d), want (108, 108)", unormOut[0], srgbOut[0])
		}
		veDiff := makeASTC4x4VoidExtentBlock(0, 0, 0, 255)
		binary.LittleEndian.PutUint16(veDiff[8:10], 0x00FF)
		decodeASTC4x4Block(veDiff, &unormOut)
		decodeASTC4x4BlockMode(veDiff, &srgbOut, true)
		if unormOut[0] != 1 || srgbOut[0] != 0 {
			t.Fatalf("ASTC void-extent 0x00FF UNORM vs sRGB = (%d, %d), want (1, 0)", unormOut[0], srgbOut[0])
		}

		// 6. Error color (magenta RGBA 255,0,255,255) for HDR and illegal encodings.
		checkMagenta := func(name string, blk []byte) {
			t.Helper()
			var out [64]byte
			decodeASTC4x4Block(blk, &out)
			for i := range 16 {
				if !bytes.Equal(out[i*4:i*4+4], []byte{255, 0, 255, 255}) {
					t.Fatalf("%s px%d = %v, want error magenta [255 0 255 255]", name, i, out[i*4:i*4+4])
				}
			}
		}

		// Reserved block mode (bm[3:0] == 0).
		checkMagenta("ReservedBlockMode", make([]byte, 16))
		// 3D void-extent (bit 9 = 1).
		ve3d := makeASTC4x4VoidExtentBlock(10, 20, 30, 40)
		ve3d[1] |= 0x02
		checkMagenta("VoidExtent_3D", ve3d)
		// HDR void-extent (bits 11:10 = 10 -> 0b10 instead of 0b11).
		veHDR := makeASTC4x4VoidExtentBlock(10, 20, 30, 40)
		veHDR[1] = (veHDR[1] &^ 0x0C) | 0x08
		checkMagenta("VoidExtent_HDR", veHDR)
		// Invalid void-extent bounds (minS=100, maxS=50).
		veBadBounds := makeASTC4x4VoidExtentBlock(10, 20, 30, 40)
		veBadBounds[1] &= 0x0F // clear bits 12..15
		for b := 2; b < 8; b++ {
			veBadBounds[b] = 0
		}
		writeBitsLE(veBadBounds, 12, 13, 100)
		writeBitsLE(veBadBounds, 25, 13, 50)
		writeBitsLE(veBadBounds, 38, 13, 0)
		writeBitsLE(veBadBounds, 51, 13, 100)
		checkMagenta("VoidExtent_BadBounds", veBadBounds)
		// Oversized grid (e.g. case 1 -> gridW = 8 > 4).
		bigGrid := make([]byte, 16)
		writeBitsLE(bigGrid, 0, 11, (1<<2)|1) // bm[1:0]=1 (R=2), bm[3:2]=1 -> gridW=8
		checkMagenta("OversizedGrid", bigGrid)
		// 4 partitions + dual-plane (illegal).
		part4Dual := make([]byte, 16)
		writeBitsLE(part4Dual, 0, 11, makeBM(4, 2, 5, true))
		writeBitsLE(part4Dual, 11, 2, 3) // 4 partitions
		checkMagenta("FourPartitionsDualPlane", part4Dual)
		// weightBits < 24 (2x2 single plane at level 0 -> 4 bits < 24).
		lowWeightBits := make([]byte, 16)
		writeBitsLE(lowWeightBits, 0, 11, makeBM(2, 2, 0, false))
		checkMagenta("WeightBitsTooSmall", lowWeightBits)
		// weightBits > 96 (4x4 dual-plane at level 8 -> 32*4 = 128 bits > 96).
		highWeightBits := make([]byte, 16)
		writeBitsLE(highWeightBits, 0, 11, makeBM(4, 4, 8, true))
		checkMagenta("WeightBitsTooLarge", highWeightBits)
		// numCEMValues > 18 (4 partitions with CEM 12 -> 32 values).
		tooManyCEM := make([]byte, 16)
		writeBitsLE(tooManyCEM, 0, 11, makeBM(4, 2, 5, false))
		writeBitsLE(tooManyCEM, 11, 2, 3)     // 4 partitions
		writeBitsLE(tooManyCEM, 23, 6, 12<<2) // shared CEM 12 -> 32 values
		checkMagenta("TooManyCEMValues", tooManyCEM)
		// Too few endpoint bits (1 partition, CEM 12 (8 values -> min 21 bits), 4x4 dual-plane at lv=5 (96 weight bits) -> avail = 128-96-2-17 = 13 < 21).
		tooFewEpBits := make([]byte, 16)
		writeBitsLE(tooFewEpBits, 0, 11, makeBM(4, 4, 5, true))
		writeBitsLE(tooFewEpBits, 11, 2, 0)  // 1 partition
		writeBitsLE(tooFewEpBits, 13, 4, 12) // CEM 12
		checkMagenta("TooFewEndpointBits", tooFewEpBits)
		// All 6 HDR CEMs (2, 3, 7, 11, 14, 15).
		for _, hdrCEM := range []int{2, 3, 7, 11, 14, 15} {
			hdrBlk := make([]byte, 16)
			writeBitsLE(hdrBlk, 0, 11, makeBM(4, 2, 5, false))
			writeBitsLE(hdrBlk, 11, 2, 0)
			writeBitsLE(hdrBlk, 13, 4, uint32(hdrCEM))
			checkMagenta("HDR_CEM", hdrBlk)
		}
	})
}

func TestCompressedUploadValidationAndPaddedStride(t *testing.T) {
	// Misaligned origin
	if err := ValidateCompressedTextureUpload(GPUFormatBC1RGBAUnorm, 4, 4, 2, 0, GPUDataLayout{}, 8); err == nil {
		t.Errorf("expected error for misaligned compressed origin (2,0)")
	}
	// Truncated buffer
	if err := ValidateCompressedTextureUpload(GPUFormatBC1RGBAUnorm, 4, 4, 0, 0, GPUDataLayout{}, 4); err == nil {
		t.Errorf("expected error for truncated compressed upload (4 < 8)")
	}
	// Too-small BytesPerRow
	if err := ValidateCompressedTextureUpload(GPUFormatBC3RGBAUnorm, 8, 4, 0, 0, GPUDataLayout{BytesPerRow: 16}, 64); err == nil {
		t.Errorf("expected error for BytesPerRow < minRowBytes")
	}

	// Padded BytesPerRow (32 bytes per block row for 1 block wide x 2 blocks tall BC1 = 4x8 texture)
	dst := make([]byte, 4*8*4)
	src := make([]byte, 32+8) // row 0 padded to 32 bytes, row 1 tight 8 bytes
	copy(src[0:8], makeBC1SolidBlock(255, 0, 0))
	copy(src[32:40], makeBC1SolidBlock(0, 255, 0))
	if err := decodeCompressedToRGBA(GPUFormatBC1RGBAUnorm, dst, 4, 8, 0, 0, 4, 8, GPUDataLayout{BytesPerRow: 32}, src); err != nil {
		t.Fatalf("decodeCompressedToRGBA padded stride: %v", err)
	}
	// Top block pixel (0,0) = red, bottom block pixel (0,4) = green.
	if dst[0] != 255 || dst[1] != 0 || dst[2] != 0 {
		t.Errorf("top block pixel = %v, want red", dst[0:4])
	}
	bot := (4 * 4) * 4
	if dst[bot] != 0 || dst[bot+1] != 255 || dst[bot+2] != 0 {
		t.Errorf("bottom block pixel = %v, want green", dst[bot:bot+4])
	}
}

func TestSoftwareDeviceTimingAndDebugRecording(t *testing.T) {
	dev := NewGPUSoftwareDevice(16, 16, GPUViewConfig{ColorFormat: GPUFormatRGBA8Unorm})
	frame := GPUFrame{
		Ops: []GPUResourceOp{
			{Kind: GPUOpCreateBuffer, ID: 1, Buffer: &GPUBufferDesc{Label: "vb", Size: 16, Usage: GPUBufferVertex}},
			{Kind: GPUOpCreateSampler, ID: 2, Sampler: &GPUSamplerDesc{Label: "linear"}},
		},
		DebugOps: []GPUDebugOp{
			{Kind: GPUDebugPushGroup, Label: "Frame", PassIndex: 0},
			{Kind: GPUDebugInsertMarker, Label: "BeforePass", PassIndex: 0},
			{Kind: GPUDebugPopGroup, PassIndex: 1},
		},
		Passes: []GPUPass{
			{
				Label:            "MainPass",
				TimingLabel:      "main-timing",
				TimestampQueryID: 99,
				ColorAttachments: []GPUColorAttachment{{LoadOp: GPULoadClear, StoreOp: GPUStoreStore, ClearColor: RGB(0x112233)}},
				Commands: []GPUCommand{
					{Kind: GPUCmdPushDebugGroup, Label: "Geometry"},
					{Kind: GPUCmdInsertDebugMarker, Label: "ClearOnly"},
					{Kind: GPUCmdPopDebugGroup},
				},
			},
		},
		Present: true,
	}
	if _, err := dev.Submit(frame); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	timings := dev.LastPassTimings()
	if len(timings) != 1 || timings[0].Label != "main-timing" || timings[0].QueryID != 99 || timings[0].PassIndex != 0 {
		t.Fatalf("LastPassTimings = %+v", timings)
	}
	if dev.ResourceLabel(1) != "vb" || dev.ResourceLabel(2) != "linear" {
		t.Errorf("ResourceLabels = (%q, %q), want (vb, linear)", dev.ResourceLabel(1), dev.ResourceLabel(2))
	}
	events := dev.DebugEvents()
	wantKinds := []string{"buffer", "sampler", "push_group", "marker", "pass", "pass_push_group", "pass_marker", "pass_pop_group", "pop_group"}
	if len(events) != len(wantKinds) {
		t.Fatalf("DebugEvents len = %d (%+v), want %d", len(events), events, len(wantKinds))
	}
	for i, wk := range wantKinds {
		if events[i].Kind != wk {
			t.Errorf("DebugEvents[%d].Kind = %q, want %q", i, events[i].Kind, wk)
		}
	}
}
