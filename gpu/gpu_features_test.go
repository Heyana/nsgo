package gpu_test

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func packRGB565(r, g, b byte) uint16 {
	return (uint16(r>>3) << 11) | (uint16(g>>2) << 5) | uint16(b>>3)
}

func makeBC1Block(r, g, b byte) []byte {
	c0 := packRGB565(r, g, b)
	out := make([]byte, 8)
	if c0 == 0 {
		binary.LittleEndian.PutUint16(out[0:2], 0xFFFF)
		binary.LittleEndian.PutUint16(out[2:4], 0x0000)
		binary.LittleEndian.PutUint32(out[4:8], 0x55555555)
		return out
	}
	binary.LittleEndian.PutUint16(out[0:2], c0)
	binary.LittleEndian.PutUint16(out[2:4], 0x0000)
	binary.LittleEndian.PutUint32(out[4:8], 0x00000000)
	return out
}

func makeBC3Block(r, g, b, a byte) []byte {
	out := make([]byte, 16)
	out[0] = a
	out[1] = a
	// all 3-bit alpha indices 0 -> a
	copy(out[8:16], makeBC1Block(r, g, b))
	return out
}

func makeETC2RGB8SolidBlock(r, g, b byte) []byte {
	// Individual mode, both subblocks identical 4-bit color, modifier table 0 (+2)
	var hi uint32
	hi |= uint32(r>>4) << 28
	hi |= uint32(r>>4) << 24
	hi |= uint32(g>>4) << 20
	hi |= uint32(g>>4) << 16
	hi |= uint32(b>>4) << 12
	hi |= uint32(b>>4) << 8
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], hi)
	binary.BigEndian.PutUint32(out[4:8], 0)
	return out
}

func makeETC2RGBA8SolidBlock(r, g, b, a byte) []byte {
	out := make([]byte, 16)
	out[0] = a
	out[1] = 0x00 // multiplier=0 -> constant alpha = base
	copy(out[8:16], makeETC2RGB8SolidBlock(r, g, b))
	return out
}

func makeASTC4x4SolidBlock(r, g, b, a byte) []byte {
	out := make([]byte, 16)
	out[0] = 0xFC
	out[1] = 0xFD
	out[2], out[3], out[4], out[5], out[6], out[7] = 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
	binary.LittleEndian.PutUint16(out[8:10], uint16(r)<<8|uint16(r))
	binary.LittleEndian.PutUint16(out[10:12], uint16(g)<<8|uint16(g))
	binary.LittleEndian.PutUint16(out[12:14], uint16(b)<<8|uint16(b))
	binary.LittleEndian.PutUint16(out[14:16], uint16(a)<<8|uint16(a))
	return out
}

func setBitsLE(buf []byte, bitPos, count int, val uint32) {
	for i := range count {
		if (val>>i)&1 != 0 {
			bp := bitPos + i
			buf[bp>>3] |= 1 << (bp & 7)
		}
	}
}

func makeBC7SolidBlock(r, g, b, a byte) []byte {
	out := make([]byte, 16)
	setBitsLE(out, 6, 1, 1) // Mode 6
	pos := 7
	p := uint32(r & 1)
	for _, ch := range []byte{r, r, g, g, b, b, a, a} {
		setBitsLE(out, pos, 7, uint32(ch>>1))
		pos += 7
	}
	setBitsLE(out, pos, 1, p)
	pos++
	setBitsLE(out, pos, 1, p)
	return out
}

func TestCompressedFormatMetadataAndCapabilities(t *testing.T) {
	allCompressed := []gpu.TextureFormat{
		gpu.FormatBC1RGBAUnorm,
		gpu.FormatBC1RGBAUnormSRGB,
		gpu.FormatBC3RGBAUnorm,
		gpu.FormatBC3RGBAUnormSRGB,
		gpu.FormatBC7RGBAUnorm,
		gpu.FormatBC7RGBAUnormSRGB,
		gpu.FormatETC2RGB8Unorm,
		gpu.FormatETC2RGBA8Unorm,
		gpu.FormatASTC4x4Unorm,
		gpu.FormatASTC4x4UnormSRGB,
	}

	sr := gpu.NewSoftwareRenderer(16, 16, gpu.FormatUndefined)
	dev := sr.Device()
	caps := dev.Capabilities()
	if !caps.TimestampQueries {
		t.Fatalf("expected SoftwareRenderer Capabilities().TimestampQueries == true")
	}
	for _, fm := range allCompressed {
		if !fm.IsCompressed() {
			t.Errorf("%s.IsCompressed() = false, want true", fm)
		}
		bw, bh := fm.BlockSize()
		if bw != 4 || bh != 4 {
			t.Errorf("%s.BlockSize() = (%d,%d), want (4,4)", fm, bw, bh)
		}
		if fm.BytesPerBlock() != 8 && fm.BytesPerBlock() != 16 {
			t.Errorf("%s.BytesPerBlock() = %d, want 8 or 16", fm, fm.BytesPerBlock())
		}
		if !dev.SupportsCompressedFormat(fm) {
			t.Errorf("SoftwareRenderer SupportsCompressedFormat(%s) = false, want true", fm)
		}
		if !slices.Contains(dev.CompressedFormats(), fm) {
			t.Errorf("SoftwareRenderer CompressedFormats() missing %s", fm)
		}
		if !slices.Contains(caps.CompressedFormats, fm) {
			t.Errorf("SoftwareRenderer Capabilities().CompressedFormats missing %s", fm)
		}
	}

	// Block-size & usage validation on CreateTexture.
	if _, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Width: 6, Height: 4, Format: gpu.FormatBC1RGBAUnorm, Usage: gpu.TextureSampled,
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Errorf("non-multiple-of-4 BC1 texture got %v, want ErrInvalidDescriptor", err)
	}
	if _, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Width: 4, Height: 4, Format: gpu.FormatBC1RGBAUnorm, Usage: gpu.TextureRenderAttachment,
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Errorf("compressed render attachment got %v, want ErrInvalidDescriptor", err)
	}
	if _, err := dev.CreateTexture(nil, gpu.TextureDesc{
		Width: 4, Height: 4, SampleCount: 4, Format: gpu.FormatBC3RGBAUnorm, Usage: gpu.TextureSampled,
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Errorf("multisampled compressed texture got %v, want ErrInvalidDescriptor", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if _, err := dev.CreateTextureFromImage(nil, core.ImageSource{Pixels: img}, gpu.TextureDesc{
		Format: gpu.FormatBC1RGBAUnorm,
	}); !errors.Is(err, gpu.ErrInvalidDescriptor) {
		t.Errorf("CreateTextureFromImage compressed got %v, want ErrInvalidDescriptor", err)
	}

	// Device advertising only BC formats rejects ETC2 and ASTC with ErrUnsupported.
	bcOnly := gpu.NewSoftwareRendererWithFeatures(16, 16, gpu.FormatUndefined, gpu.Features{
		TextureCompressionBC: true,
	}).Device()
	if !bcOnly.SupportsCompressedFormat(gpu.FormatBC1RGBAUnorm) {
		t.Errorf("bcOnly should support BC1")
	}
	if bcOnly.SupportsCompressedFormat(gpu.FormatETC2RGB8Unorm) {
		t.Errorf("bcOnly should not support ETC2")
	}
	if bcOnly.SupportsCompressedFormat(gpu.FormatASTC4x4Unorm) {
		t.Errorf("bcOnly should not support ASTC")
	}
	if _, err := bcOnly.CreateTexture(nil, gpu.TextureDesc{
		Width: 4, Height: 4, Format: gpu.FormatETC2RGB8Unorm, Usage: gpu.TextureSampled,
	}); !errors.Is(err, gpu.ErrUnsupported) {
		t.Errorf("bcOnly CreateTexture(ETC2) got %v, want ErrUnsupported", err)
	}
}

func TestHeadlessCompressedTextureDraw(t *testing.T) {
	texMod := gpu.MustCompileWGSL(texturedWGSL)
	quadVerts := packFloat32s(
		-1, -1, 0.5, 0, 1,
		1, -1, 0.5, 1, 1,
		1, 1, 0.5, 1, 0,
		-1, -1, 0.5, 0, 1,
		1, 1, 0.5, 1, 0,
		-1, 1, 0.5, 0, 0,
	)

	drawSampled := func(t *testing.T, format gpu.TextureFormat, w, h uint32, blocks []byte, blend *gpu.BlendState, clearColor core.Color, viaCopyBuffer bool) *image.RGBA {
		t.Helper()
		sr := gpu.NewSoftwareRenderer(32, 32, gpu.FormatUndefined)
		dev := sr.Device()

		tex, err := dev.CreateTexture(nil, gpu.TextureDesc{
			Label:  "compressed",
			Width:  w,
			Height: h,
			Format: format,
			Usage:  gpu.TextureSampled | gpu.TextureCopyDst,
		})
		if err != nil {
			t.Fatalf("CreateTexture(%s): %v", format, err)
		}
		var stageBuf *gpu.Buffer
		if viaCopyBuffer {
			stageBuf, err = dev.CreateBuffer(nil, gpu.BufferDesc{
				Usage: gpu.UsageCopySrc,
				Data:  blocks,
			})
			if err != nil {
				t.Fatalf("CreateBuffer staging: %v", err)
			}
		} else {
			dev.WriteTexture(tex, gpu.TextureCopy{Texture: tex}, blocks, gpu.DataLayout{})
		}

		samp, _ := dev.CreateSampler(nil, gpu.SamplerDesc{
			MinFilter: gpu.FilterNearest,
			MagFilter: gpu.FilterNearest,
			AddressU:  gpu.AddressClampToEdge,
			AddressV:  gpu.AddressClampToEdge,
		})
		bgl, _ := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gpu.StageFragment, Type: gpu.BindingTexture},
			{Binding: 1, Visibility: gpu.StageFragment, Type: gpu.BindingSampler},
		})
		bg, _ := dev.CreateBindGroup(nil, bgl, []gpu.BindGroupEntry{
			{Binding: 0, Texture: tex},
			{Binding: 1, Sampler: samp},
		})
		vb, _ := dev.CreateBuffer(nil, gpu.BufferDesc{Usage: gpu.UsageVertex, Data: quadVerts})
		pipe, err := dev.CreatePipeline(nil, gpu.PipelineDesc{
			Vertex:   gpu.ShaderStage{Module: texMod, EntryPoint: "vs_main"},
			Fragment: gpu.ShaderStage{Module: texMod, EntryPoint: "fs_main"},
			Layouts:  []*gpu.BindGroupLayout{bgl},
			VertexBuffers: []gpu.VertexBufferLayout{{
				Stride: 5 * 4,
				Attributes: []gpu.VertexAttribute{
					{Location: 0, Offset: 0, Format: gpu.VertexFloat32x3},
					{Location: 1, Offset: 3 * 4, Format: gpu.VertexFloat32x2},
				},
			}},
			Targets: []gpu.ColorTargetState{{Format: gpu.FormatRGBA8Unorm, Blend: blend}},
		})
		if err != nil {
			t.Fatalf("CreatePipeline: %v", err)
		}

		enc := dev.BeginEncoder()
		if viaCopyBuffer {
			enc.CopyBufferToTexture(
				gpu.BufferCopy{Buffer: stageBuf},
				gpu.TextureCopy{Texture: tex},
				w, h, 1,
			)
		}
		pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
		pass.ClearColor(clearColor)
		pass.SetPipeline(pipe)
		pass.SetBindGroup(0, bg)
		pass.SetVertexBuffer(0, vb, 0)
		pass.Draw(6, 0)
		pass.End()
		dev.Submit(enc.Finish())
		if sr.LastError() != nil {
			t.Fatalf("SoftwareRenderer error (%s): %v", format, sr.LastError())
		}
		return sr.Image()
	}

	t.Run("BC1_TwoBlocks_WriteTexture", func(t *testing.T) {
		// 8x4 BC1 texture: left 4x4 block red, right 4x4 block green.
		data := append(makeBC1Block(255, 0, 0), makeBC1Block(0, 255, 0)...)
		img := drawSampled(t, gpu.FormatBC1RGBAUnorm, 8, 4, data, nil, core.RGB(0x000000), false)
		left := img.RGBAAt(6, 16)
		if left.R < 250 || left.G > 5 || left.B > 5 || left.A != 255 {
			t.Errorf("BC1 left pixel = %+v, want red", left)
		}
		right := img.RGBAAt(26, 16)
		if right.R > 5 || right.G < 250 || right.B > 5 || right.A != 255 {
			t.Errorf("BC1 right pixel = %+v, want green", right)
		}
	})

	t.Run("BC1_SRGB_DecodesToLinear", func(t *testing.T) {
		// Mid-gray ~132 in sRGB should decode linearly to ~58/255 (~0.23).
		midGray := makeBC1Block(132, 132, 132)
		linImg := drawSampled(t, gpu.FormatBC1RGBAUnorm, 4, 4, midGray, nil, core.RGB(0x000000), false)
		srgbImg := drawSampled(t, gpu.FormatBC1RGBAUnormSRGB, 4, 4, midGray, nil, core.RGB(0x000000), false)
		linPx := linImg.RGBAAt(16, 16)
		srgbPx := srgbImg.RGBAAt(16, 16)
		if linPx.R < 120 || srgbPx.R < 40 || srgbPx.R > 80 {
			t.Errorf("linear=%+v srgb=%+v; expected sRGB mid-gray to linearize ~55", linPx, srgbPx)
		}
	})

	t.Run("BC3_AlphaBlended_CopyBufferToTexture", func(t *testing.T) {
		// 4x4 BC3 block: blue (0,0,255) at ~50% alpha (128) blended over red clear color -> (~128, 0, ~128, 255)
		data := makeBC3Block(0, 0, 255, 128)
		img := drawSampled(t, gpu.FormatBC3RGBAUnorm, 4, 4, data, gpu.BlendAlpha(), core.RGB(0xff0000), true)
		px := img.RGBAAt(16, 16)
		if px.R < 118 || px.R > 138 || px.G > 5 || px.B < 118 || px.B > 138 {
			t.Errorf("BC3 blended pixel = %+v, want ~{128 0 128 255}", px)
		}
	})

	t.Run("ETC2_RGB8_And_RGBA8", func(t *testing.T) {
		rgbImg := drawSampled(t, gpu.FormatETC2RGB8Unorm, 4, 4, makeETC2RGB8SolidBlock(0, 240, 240), nil, core.RGB(0x000000), false)
		px := rgbImg.RGBAAt(16, 16)
		if px.R > 10 || px.G < 235 || px.B < 235 || px.A != 255 {
			t.Errorf("ETC2 RGB8 pixel = %+v, want cyan", px)
		}

		// ETC2 RGBA8: yellow (255,255,0) with 50% alpha (128) blended over blue (0,0,255) -> (~128, ~128, ~128)
		rgbaImg := drawSampled(t, gpu.FormatETC2RGBA8Unorm, 4, 4, makeETC2RGBA8SolidBlock(240, 240, 0, 128), gpu.BlendAlpha(), core.RGB(0x0000ff), false)
		bpx := rgbaImg.RGBAAt(16, 16)
		if bpx.R < 115 || bpx.R > 140 || bpx.G < 115 || bpx.G > 140 || bpx.B < 115 || bpx.B > 140 {
			t.Errorf("ETC2 RGBA8 blended pixel = %+v, want ~{128 128 128 255}", bpx)
		}
	})

	t.Run("BC7_Mode6", func(t *testing.T) {
		img := drawSampled(t, gpu.FormatBC7RGBAUnorm, 4, 4, makeBC7SolidBlock(255, 129, 0, 255), nil, core.RGB(0x000000), false)
		px := img.RGBAAt(16, 16)
		if px.R < 250 || px.G < 124 || px.G > 134 || px.B > 5 || px.A != 255 {
			t.Errorf("BC7 pixel = %+v, want ~{255 129 0 255}", px)
		}
	})

	t.Run("ASTC4x4_VoidExtent", func(t *testing.T) {
		img := drawSampled(t, gpu.FormatASTC4x4Unorm, 4, 4, makeASTC4x4SolidBlock(40, 200, 120, 255), nil, core.RGB(0x000000), false)
		px := img.RGBAAt(16, 16)
		if px != (color.RGBA{R: 40, G: 200, B: 120, A: 255}) {
			t.Errorf("ASTC4x4 pixel = %+v, want {40 200 120 255}", px)
		}
	})

	t.Run("TruncatedCompressedUploadErrors", func(t *testing.T) {
		sr := gpu.NewSoftwareRenderer(16, 16, gpu.FormatUndefined)
		dev := sr.Device()
		tex, err := dev.CreateTexture(nil, gpu.TextureDesc{
			Width: 8, Height: 8, Format: gpu.FormatBC1RGBAUnorm, Usage: gpu.TextureSampled | gpu.TextureCopyDst,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		// 8x8 BC1 needs 4 blocks * 8 = 32 bytes; upload only 8 bytes.
		dev.WriteTexture(tex, gpu.TextureCopy{Texture: tex}, make([]byte, 8), gpu.DataLayout{})
		enc := dev.BeginEncoder()
		p := enc.BeginRenderPass(gpu.RenderPassDesc{})
		p.ClearColor(core.RGB(0x000000))
		p.End()
		dev.Submit(enc.Finish())
		if sr.LastError() == nil {
			t.Errorf("expected error for truncated BC1 upload (8 < 32 bytes)")
		}
	})
}

func TestGPUTimingQueriesSoftwareAndHeadlessView(t *testing.T) {
	t.Run("SoftwareRendererTimingQueries", func(t *testing.T) {
		sr := gpu.NewSoftwareRenderer(64, 64, gpu.FormatDepth24Plus)
		dev := sr.Device()
		if !dev.Capabilities().TimestampQueries || !dev.SupportsTimestampQueries() {
			t.Fatalf("expected TimestampQueries capability on SoftwareRenderer")
		}

		q, err := dev.CreateTimestampQuery(nil, "query-pass")
		if err != nil {
			t.Fatalf("CreateTimestampQuery: %v", err)
		}
		if q.Label() != "query-pass" || q.ID() == 0 {
			t.Fatalf("unexpected TimestampQuery metadata: id=%d label=%q", q.ID(), q.Label())
		}

		frame := dev.BeginFrame(time.Now())
		p0 := frame.BeginRenderPass(gpu.RenderPassDesc{TimingLabel: "clear-pass"})
		p0.ClearColor(core.RGB(0x102030))
		p0.End()

		// Untimed pass (must not appear in PassTimings).
		p1 := frame.BeginRenderPass(gpu.RenderPassDesc{})
		p1.End()

		// Timed pass using TimestampQuery (defaults label to q.Label()).
		p2 := frame.BeginRenderPass(gpu.RenderPassDesc{TimestampQuery: q})
		p2.ClearColor(core.RGB(0x405060))
		p2.End()

		frame.Submit()

		timings := frame.PassTimings()
		if len(timings) != 2 {
			t.Fatalf("frame.PassTimings() len = %d (%+v), want 2", len(timings), timings)
		}
		if timings[0].Label != "clear-pass" || timings[0].PassIndex != 0 || timings[0].Duration < 0 {
			t.Errorf("timings[0] = %+v", timings[0])
		}
		if timings[1].Label != "query-pass" || timings[1].PassIndex != 2 || timings[1].Duration < 0 {
			t.Errorf("timings[1] = %+v", timings[1])
		}
		if len(q.Timings()) != 1 || q.Timings()[0].Label != "query-pass" {
			t.Errorf("q.Timings() = %+v, want 1 entry for query-pass", q.Timings())
		}
		if q.LastDuration() != timings[1].Duration {
			t.Errorf("q.LastDuration() = %v, want %v", q.LastDuration(), timings[1].Duration)
		}
		q.Reset()
		if len(q.Timings()) != 0 {
			t.Errorf("q.Reset() left %d timings", len(q.Timings()))
		}
		_ = q.Close()
	})

	t.Run("UnsupportedTimestampQueries", func(t *testing.T) {
		sr := gpu.NewSoftwareRendererWithFeatures(16, 16, gpu.FormatUndefined, gpu.Features{
			TimestampQueries: false,
		})
		dev := sr.Device()
		if dev.Capabilities().TimestampQueries || dev.SupportsTimestampQueries() {
			t.Fatalf("expected TimestampQueries == false")
		}
		if _, err := dev.CreateTimestampQuery(nil, "nope"); !errors.Is(err, gpu.ErrUnsupported) {
			t.Fatalf("CreateTimestampQuery got %v, want ErrUnsupported", err)
		}
		// TimingLabel degrades cleanly when unsupported.
		frame := dev.BeginFrame(time.Now())
		p := frame.BeginRenderPass(gpu.RenderPassDesc{TimingLabel: "ignored"})
		p.ClearColor(core.RGB(0x000000))
		p.End()
		frame.Submit()
		if len(frame.PassTimings()) != 0 {
			t.Errorf("expected no timings when TimestampQueries unsupported, got %+v", frame.PassTimings())
		}
	})

	t.Run("HeadlessViewGPUEventPassTimingsCallback", func(t *testing.T) {
		var (
			callbackBatches [][]gpu.PassTiming
			frame2Timings   []gpu.PassTiming
			frameNum        int
		)
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return gpu.View(cx, gpu.ViewProps{
				TestID: "timed-view",
				Style:  core.Style{Width: core.Pt(40), Height: core.Pt(40)},
				OnReady: func(dev *gpu.Device) {
					if !dev.Capabilities().TimestampQueries {
						t.Errorf("expected headless gpu.View TimestampQueries == true")
					}
				},
				OnPassTimings: func(timings []gpu.PassTiming) {
					callbackBatches = append(callbackBatches, timings)
				},
				OnFrame: func(frame *gpu.Frame) {
					frameNum++
					if frameNum == 2 {
						frame2Timings = frame.PassTimings()
					}
					p := frame.BeginRenderPass(gpu.RenderPassDesc{TimingLabel: "scene-pass"})
					p.ClearColor(core.RGB(0x223344))
					p.End()
					frame.Submit()
				},
			})
		}, nsgotest.Size(40, 40), nsgotest.ManualFrames())

		app.Advance(16 * time.Millisecond)
		app.Advance(16 * time.Millisecond)

		if len(callbackBatches) < 1 {
			t.Fatalf("expected OnPassTimings callback to fire via GPUEventPassTimings")
		}
		if len(callbackBatches[0]) != 1 || callbackBatches[0][0].Label != "scene-pass" {
			t.Errorf("callbackBatches[0] = %+v, want [scene-pass]", callbackBatches[0])
		}
		if len(frame2Timings) != 1 || frame2Timings[0].Label != "scene-pass" {
			t.Errorf("frame 2 Frame.PassTimings() = %+v, want [scene-pass]", frame2Timings)
		}
	})
}
