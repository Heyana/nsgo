package driver

import (
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
		// Opaque mode: c0 = pure red (0xF800) > c1 = pure blue (0x001F).
		// Pixel 0=idx0 (red), pixel 1=idx1 (blue), pixel 2=idx2 (2/3 red + 1/3 blue), pixel 3=idx3 (1/3 red + 2/3 blue).
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
