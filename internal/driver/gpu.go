package driver

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// GPUID uniquely identifies a retained GPU resource (buffer, texture, sampler,
// bind group layout, bind group, or pipeline) within a GPU surface.
type GPUID uint64

// GPUTextureFormat identifies the pixel or depth/stencil layout of a GPU texture.
type GPUTextureFormat uint8

const (
	GPUFormatUndefined GPUTextureFormat = iota
	GPUFormatRGBA8Unorm
	GPUFormatRGBA8UnormSRGB
	GPUFormatBGRA8Unorm
	GPUFormatRG8Unorm
	GPUFormatR8Unorm
	GPUFormatRGBA16Float
	GPUFormatDepth24Plus
	GPUFormatDepth24PlusStencil8
	GPUFormatDepth32Float
	GPUFormatBC1RGBAUnorm
	GPUFormatBC1RGBAUnormSRGB
	GPUFormatBC3RGBAUnorm
	GPUFormatBC3RGBAUnormSRGB
	GPUFormatBC7RGBAUnorm
	GPUFormatBC7RGBAUnormSRGB
	GPUFormatETC2RGB8Unorm
	GPUFormatETC2RGBA8Unorm
	GPUFormatASTC4x4Unorm
	GPUFormatASTC4x4UnormSRGB
	// The extended formats below are appended so the numeric values above, which the Android backend sends over the wire, keep their meaning. Devices accept them only when GPUFeatures.ExtendedFormats is set.
	GPUFormatBGRA8UnormSRGB
	GPUFormatRG11B10Float
	GPUFormatR32Float
	GPUFormatRGBA32Float
	GPUFormatStencil8
	// The 16-bit normalized formats below exist only for imported textures, such as the planes of a P010 video frame; no device creates them (IsImportOnly).
	GPUFormatR16Unorm
	GPUFormatRG16Unorm
	// GPUFormatRGB10A2Unorm is a 10-bit-per-channel RGB + 2-bit alpha normalized format used for HDR10 (PQ/HLG) swapchains and render targets.
	GPUFormatRGB10A2Unorm
)

// GPUColorSpace selects the color space and transfer function of a GPU view's swapchain.
type GPUColorSpace uint8

const (
	// GPUColorSpaceDefault selects the platform's default SDR swapchain color space (sRGB).
	GPUColorSpaceDefault GPUColorSpace = iota
	// GPUColorSpaceSRGB is standard sRGB (Rec. 709 primaries, sRGB transfer, [0, 1] range).
	GPUColorSpaceSRGB
	// GPUColorSpaceExtendedLinearSRGB is linear-light extended-range sRGB / scRGB
	// (Rec. 709 primaries, linear transfer, 1.0 = SDR reference white, values > 1.0 for HDR).
	GPUColorSpaceExtendedLinearSRGB
	// GPUColorSpaceHDR10PQ is BT.2020 primaries with SMPTE ST 2084 (PQ) transfer.
	GPUColorSpaceHDR10PQ
	// GPUColorSpaceHDR10HLG is BT.2020 primaries with Hybrid Log-Gamma (HLG) transfer.
	GPUColorSpaceHDR10HLG
	// GPUColorSpaceDisplayP3 is Display P3 primaries with sRGB transfer.
	GPUColorSpaceDisplayP3
)

// IsHDR reports whether c is an HDR / extended dynamic range color space.
func (c GPUColorSpace) IsHDR() bool {
	return c == GPUColorSpaceExtendedLinearSRGB || c == GPUColorSpaceHDR10PQ || c == GPUColorSpaceHDR10HLG
}

// String returns a human-readable identifier for c.
func (c GPUColorSpace) String() string {
	switch c {
	case GPUColorSpaceDefault:
		return "Default"
	case GPUColorSpaceSRGB:
		return "sRGB"
	case GPUColorSpaceExtendedLinearSRGB:
		return "ExtendedLinearSRGB"
	case GPUColorSpaceHDR10PQ:
		return "HDR10PQ"
	case GPUColorSpaceHDR10HLG:
		return "HDR10HLG"
	case GPUColorSpaceDisplayP3:
		return "DisplayP3"
	default:
		return "Unknown"
	}
}

// IsDepth reports whether f carries a depth aspect.
func (f GPUTextureFormat) IsDepth() bool {
	return f == GPUFormatDepth24Plus || f == GPUFormatDepth24PlusStencil8 || f == GPUFormatDepth32Float
}

// HasStencil reports whether f carries a stencil aspect.
func (f GPUTextureFormat) HasStencil() bool {
	return f == GPUFormatDepth24PlusStencil8 || f == GPUFormatStencil8
}

// IsDepthStencil reports whether f is a depth, stencil, or combined depth-stencil format.
func (f GPUTextureFormat) IsDepthStencil() bool {
	return f.IsDepth() || f.HasStencil()
}

// IsFloat reports whether f is a floating-point color format (HDR/unclamped).
func (f GPUTextureFormat) IsFloat() bool {
	return f == GPUFormatRGBA16Float || f == GPUFormatRG11B10Float || f == GPUFormatR32Float || f == GPUFormatRGBA32Float
}

// IsExtended reports whether f is one of the extended formats gated by GPUFeatures.ExtendedFormats.
func (f GPUTextureFormat) IsExtended() bool {
	switch f {
	case GPUFormatBGRA8UnormSRGB, GPUFormatRG11B10Float, GPUFormatR32Float, GPUFormatRGBA32Float, GPUFormatStencil8:
		return true
	default:
		return false
	}
}

// IsImportOnly reports whether f is a format only imported textures have (GPUOpImportTexture).
func (f GPUTextureFormat) IsImportOnly() bool {
	return f == GPUFormatR16Unorm || f == GPUFormatRG16Unorm
}

// IsCompressed reports whether f is a block-compressed GPU texture format.
func (f GPUTextureFormat) IsCompressed() bool {
	switch f {
	case GPUFormatBC1RGBAUnorm, GPUFormatBC1RGBAUnormSRGB,
		GPUFormatBC3RGBAUnorm, GPUFormatBC3RGBAUnormSRGB,
		GPUFormatBC7RGBAUnorm, GPUFormatBC7RGBAUnormSRGB,
		GPUFormatETC2RGB8Unorm, GPUFormatETC2RGBA8Unorm,
		GPUFormatASTC4x4Unorm, GPUFormatASTC4x4UnormSRGB:
		return true
	default:
		return false
	}
}

// IsSRGB reports whether f stores sRGB-encoded RGB channels decoded to linear on sample.
func (f GPUTextureFormat) IsSRGB() bool {
	switch f {
	case GPUFormatRGBA8UnormSRGB, GPUFormatBGRA8UnormSRGB,
		GPUFormatBC1RGBAUnormSRGB,
		GPUFormatBC3RGBAUnormSRGB,
		GPUFormatBC7RGBAUnormSRGB,
		GPUFormatASTC4x4UnormSRGB:
		return true
	default:
		return false
	}
}

// BlockSize returns the compression block dimensions (width, height) in texels. Uncompressed non-undefined formats return (1, 1); undefined/unknown returns (0, 0).
func (f GPUTextureFormat) BlockSize() (width, height uint32) {
	switch f {
	case GPUFormatBC1RGBAUnorm, GPUFormatBC1RGBAUnormSRGB,
		GPUFormatBC3RGBAUnorm, GPUFormatBC3RGBAUnormSRGB,
		GPUFormatBC7RGBAUnorm, GPUFormatBC7RGBAUnormSRGB,
		GPUFormatETC2RGB8Unorm, GPUFormatETC2RGBA8Unorm,
		GPUFormatASTC4x4Unorm, GPUFormatASTC4x4UnormSRGB:
		return 4, 4
	case GPUFormatRGBA8Unorm, GPUFormatRGBA8UnormSRGB, GPUFormatBGRA8Unorm,
		GPUFormatRG8Unorm, GPUFormatR8Unorm, GPUFormatRGBA16Float,
		GPUFormatDepth24Plus, GPUFormatDepth24PlusStencil8, GPUFormatDepth32Float,
		GPUFormatBGRA8UnormSRGB, GPUFormatRG11B10Float, GPUFormatR32Float, GPUFormatRGBA32Float, GPUFormatStencil8,
		GPUFormatR16Unorm, GPUFormatRG16Unorm, GPUFormatRGB10A2Unorm:
		return 1, 1
	default:
		return 0, 0
	}
}

// BytesPerBlock returns the encoded byte size of one texel block. For uncompressed color formats it equals BytesPerPixel; for depth/undefined it is 0.
func (f GPUTextureFormat) BytesPerBlock() uint32 {
	switch f {
	case GPUFormatBC1RGBAUnorm, GPUFormatBC1RGBAUnormSRGB, GPUFormatETC2RGB8Unorm:
		return 8
	case GPUFormatBC3RGBAUnorm, GPUFormatBC3RGBAUnormSRGB,
		GPUFormatBC7RGBAUnorm, GPUFormatBC7RGBAUnormSRGB,
		GPUFormatETC2RGBA8Unorm,
		GPUFormatASTC4x4Unorm, GPUFormatASTC4x4UnormSRGB:
		return 16
	default:
		return f.BytesPerPixel()
	}
}

// ByteSize returns the tightly-packed byte size required for a 2D image of width x height texels in format f (0 for depth or undefined formats).
func (f GPUTextureFormat) ByteSize(width, height uint32) uint64 {
	bw, bh := f.BlockSize()
	bpb := f.BytesPerBlock()
	if bw == 0 || bh == 0 || bpb == 0 || width == 0 || height == 0 {
		return 0
	}
	blocksX := (uint64(width) + uint64(bw) - 1) / uint64(bw)
	blocksY := (uint64(height) + uint64(bh) - 1) / uint64(bh)
	return blocksX * blocksY * uint64(bpb)
}

// BytesPerPixel returns the texel stride in bytes for uncompressed color formats (0 for compressed, depth, stencil, or undefined formats).
func (f GPUTextureFormat) BytesPerPixel() uint32 {
	switch f {
	case GPUFormatR8Unorm:
		return 1
	case GPUFormatRG8Unorm, GPUFormatR16Unorm:
		return 2
	case GPUFormatRGBA8Unorm, GPUFormatRGBA8UnormSRGB, GPUFormatBGRA8Unorm, GPUFormatBGRA8UnormSRGB, GPUFormatRG11B10Float, GPUFormatR32Float, GPUFormatRG16Unorm, GPUFormatRGB10A2Unorm:
		return 4
	case GPUFormatRGBA16Float:
		return 8
	case GPUFormatRGBA32Float:
		return 16
	default:
		return 0
	}
}

// String returns a human-readable identifier for f.
func (f GPUTextureFormat) String() string {
	switch f {
	case GPUFormatUndefined:
		return "Undefined"
	case GPUFormatRGBA8Unorm:
		return "RGBA8Unorm"
	case GPUFormatRGBA8UnormSRGB:
		return "RGBA8UnormSRGB"
	case GPUFormatBGRA8Unorm:
		return "BGRA8Unorm"
	case GPUFormatRG8Unorm:
		return "RG8Unorm"
	case GPUFormatR8Unorm:
		return "R8Unorm"
	case GPUFormatRGBA16Float:
		return "RGBA16Float"
	case GPUFormatDepth24Plus:
		return "Depth24Plus"
	case GPUFormatDepth24PlusStencil8:
		return "Depth24PlusStencil8"
	case GPUFormatDepth32Float:
		return "Depth32Float"
	case GPUFormatBC1RGBAUnorm:
		return "BC1RGBAUnorm"
	case GPUFormatBC1RGBAUnormSRGB:
		return "BC1RGBAUnormSRGB"
	case GPUFormatBC3RGBAUnorm:
		return "BC3RGBAUnorm"
	case GPUFormatBC3RGBAUnormSRGB:
		return "BC3RGBAUnormSRGB"
	case GPUFormatBC7RGBAUnorm:
		return "BC7RGBAUnorm"
	case GPUFormatBC7RGBAUnormSRGB:
		return "BC7RGBAUnormSRGB"
	case GPUFormatETC2RGB8Unorm:
		return "ETC2RGB8Unorm"
	case GPUFormatETC2RGBA8Unorm:
		return "ETC2RGBA8Unorm"
	case GPUFormatASTC4x4Unorm:
		return "ASTC4x4Unorm"
	case GPUFormatASTC4x4UnormSRGB:
		return "ASTC4x4UnormSRGB"
	case GPUFormatBGRA8UnormSRGB:
		return "BGRA8UnormSRGB"
	case GPUFormatRG11B10Float:
		return "RG11B10Float"
	case GPUFormatR32Float:
		return "R32Float"
	case GPUFormatRGBA32Float:
		return "RGBA32Float"
	case GPUFormatStencil8:
		return "Stencil8"
	case GPUFormatR16Unorm:
		return "R16Unorm"
	case GPUFormatRG16Unorm:
		return "RG16Unorm"
	case GPUFormatRGB10A2Unorm:
		return "RGB10A2Unorm"
	default:
		return "Unknown"
	}
}

// GPUTextureDimension is the dimensionality of a GPU texture.
type GPUTextureDimension uint8

const (
	GPUDim2D GPUTextureDimension = iota
	GPUDimCube
	GPUDim2DArray
)

// Canonical cubemap face layer indices (ArrayLayer 0..5), matching WebGPU, Metal, Direct3D, and OpenGL.
const (
	GPUCubeFacePosX uint32 = 0
	GPUCubeFaceNegX uint32 = 1
	GPUCubeFacePosY uint32 = 2
	GPUCubeFaceNegY uint32 = 3
	GPUCubeFacePosZ uint32 = 4
	GPUCubeFaceNegZ uint32 = 5
)

// GPUMipLevels returns the number of mipmap levels in a full chain from width x height down to 1x1.
func GPUMipLevels(width, height uint32) uint32 {
	m := max(width, height)
	if m == 0 {
		return 0
	}
	var levels uint32 = 1
	for m > 1 {
		m >>= 1
		levels++
	}
	return levels
}

// GPUMipSize returns the dimensions of mipLevel for a base texture of width x height (clamped to a minimum of 1x1).
func GPUMipSize(width, height, mipLevel uint32) (uint32, uint32) {
	w := width >> mipLevel
	h := height >> mipLevel
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// GPUTextureUsage is a bitmask of permitted uses for a GPU texture.
type GPUTextureUsage uint32

const (
	GPUTextureSampled GPUTextureUsage = 1 << iota
	GPUTextureRenderAttachment
	GPUTextureCopySrc
	GPUTextureCopyDst
	GPUTextureStorage
)

// GPUBufferUsage is a bitmask of permitted uses for a GPU buffer.
type GPUBufferUsage uint32

const (
	GPUBufferVertex GPUBufferUsage = 1 << iota
	GPUBufferIndex
	GPUBufferUniform
	GPUBufferStorage
	GPUBufferCopySrc
	GPUBufferCopyDst
	GPUBufferIndirect
)

// GPUFilterMode selects texture minification, magnification, or mipmap filtering.
type GPUFilterMode uint8

const (
	GPUFilterNearest GPUFilterMode = iota
	GPUFilterLinear
)

// GPUAddressMode selects texture coordinate wrapping behavior.
type GPUAddressMode uint8

const (
	GPUAddressClampToEdge GPUAddressMode = iota
	GPUAddressRepeat
	GPUAddressMirrorRepeat
)

// GPUCompareFunc selects a depth, stencil, or shadow-sampler comparison function.
type GPUCompareFunc uint8

const (
	GPUCompareUndefined GPUCompareFunc = iota
	GPUCompareNever
	GPUCompareLess
	GPUCompareEqual
	GPUCompareLessEqual
	GPUCompareGreater
	GPUCompareNotEqual
	GPUCompareGreaterEqual
	GPUCompareAlways
)

// GPUVertexFormat specifies the component layout of a vertex attribute.
type GPUVertexFormat uint8

const (
	GPUVertexFloat32 GPUVertexFormat = iota + 1
	GPUVertexFloat32x2
	GPUVertexFloat32x3
	GPUVertexFloat32x4
	GPUVertexSint32
	GPUVertexSint32x2
	GPUVertexSint32x3
	GPUVertexSint32x4
	GPUVertexUint32
	GPUVertexUint32x2
	GPUVertexUint32x3
	GPUVertexUint32x4
	GPUVertexUnorm8x4
)

// ByteSize returns the size of one attribute element in bytes.
func (f GPUVertexFormat) ByteSize() uint64 {
	switch f {
	case GPUVertexFloat32, GPUVertexSint32, GPUVertexUint32, GPUVertexUnorm8x4:
		return 4
	case GPUVertexFloat32x2, GPUVertexSint32x2, GPUVertexUint32x2:
		return 8
	case GPUVertexFloat32x3, GPUVertexSint32x3, GPUVertexUint32x3:
		return 12
	case GPUVertexFloat32x4, GPUVertexSint32x4, GPUVertexUint32x4:
		return 16
	default:
		return 0
	}
}

// GPUVertexStepMode specifies whether a vertex buffer advances per vertex or per instance.
type GPUVertexStepMode uint8

const (
	GPUStepVertex GPUVertexStepMode = iota
	GPUStepInstance
)

// GPUIndexFormat specifies the element width of an index buffer.
type GPUIndexFormat uint8

const (
	GPUIndexUint16 GPUIndexFormat = iota
	GPUIndexUint32
)

// GPUPrimitiveTopology specifies how vertices are assembled into primitives.
type GPUPrimitiveTopology uint8

const (
	GPUTopologyTriangleList GPUPrimitiveTopology = iota
	GPUTopologyTriangleStrip
	GPUTopologyLineList
	GPUTopologyLineStrip
	GPUTopologyPointList
)

// GPUFrontFace specifies the winding order considered front-facing.
type GPUFrontFace uint8

const (
	GPUFrontCCW GPUFrontFace = iota
	GPUFrontCW
)

// GPUCullMode specifies which faces are culled during rasterization.
type GPUCullMode uint8

const (
	GPUCullNone GPUCullMode = iota
	GPUCullFront
	GPUCullBack
)

// GPUBlendFactor specifies a source or destination blend multiplier.
type GPUBlendFactor uint8

const (
	GPUBlendZero GPUBlendFactor = iota
	GPUBlendOne
	GPUBlendSrc
	GPUBlendOneMinusSrc
	GPUBlendSrcAlpha
	GPUBlendOneMinusSrcAlpha
	GPUBlendDst
	GPUBlendOneMinusDst
	GPUBlendDstAlpha
	GPUBlendOneMinusDstAlpha
	GPUBlendConstant
	GPUBlendOneMinusConstant
)

// GPUBlendOperation specifies how blended source and destination terms combine.
type GPUBlendOperation uint8

const (
	GPUBlendAdd GPUBlendOperation = iota
	GPUBlendSubtract
	GPUBlendReverseSubtract
	GPUBlendMin
	GPUBlendMax
)

// GPUColorWriteMask selects which color channels a render pipeline writes.
type GPUColorWriteMask uint8

const (
	GPUColorWriteRed GPUColorWriteMask = 1 << iota
	GPUColorWriteGreen
	GPUColorWriteBlue
	GPUColorWriteAlpha
	// GPUColorWriteNone disables all color writes. It is a separate bit because a zero mask means GPUColorWriteAll; backends only test the four channel bits.
	GPUColorWriteNone
	GPUColorWriteAll = GPUColorWriteRed | GPUColorWriteGreen | GPUColorWriteBlue | GPUColorWriteAlpha
)

// GPUStencilOperation specifies how a stencil buffer value updates after stencil/depth testing.
type GPUStencilOperation uint8

const (
	GPUStencilKeep GPUStencilOperation = iota
	GPUStencilZero
	GPUStencilReplace
	GPUStencilInvert
	GPUStencilIncrementClamp
	GPUStencilDecrementClamp
	GPUStencilIncrementWrap
	GPUStencilDecrementWrap
)

// GPUStencilFaceState configures stencil comparison and update operations for one winding face.
type GPUStencilFaceState struct {
	Compare     GPUCompareFunc
	FailOp      GPUStencilOperation
	DepthFailOp GPUStencilOperation
	PassOp      GPUStencilOperation
}

// GPULoadOp specifies what happens to an attachment at the start of a render pass.
type GPULoadOp uint8

const (
	GPULoadClear GPULoadOp = iota
	GPULoadLoad
)

// GPUStoreOp specifies what happens to an attachment at the end of a render pass.
type GPUStoreOp uint8

const (
	GPUStoreStore GPULoadStore = iota
	GPUStoreDiscard
)

// GPULoadStore is an alias for GPUStoreOp to keep constant iota clean.
type GPULoadStore = GPUStoreOp

// GPUBindingType classifies an entry in a bind group layout.
type GPUBindingType uint8

const (
	GPUBindingUniform GPUBindingType = iota
	GPUBindingStorageRead
	GPUBindingTexture
	GPUBindingSampler
	GPUBindingComparisonSampler
	GPUBindingStorage
	GPUBindingStorageTexture
)

// GPUShaderStageFlags specifies which shader stages can see a binding.
type GPUShaderStageFlags uint8

const (
	GPUStageVertex GPUShaderStageFlags = 1 << iota
	GPUStageFragment
	GPUStageCompute
	GPUStageAll = GPUStageVertex | GPUStageFragment | GPUStageCompute
)

// GPUDeviceLostReason describes why a GPU device was lost.
type GPUDeviceLostReason uint8

const (
	GPULostUnknown GPUDeviceLostReason = iota
	GPULostReset
	GPULostDestroyed
	GPULostOutOfMemory
	GPULostDeviceRemoved
	GPULostContextLost
	GPULostSurfaceDestroyed
)

// String returns a human-readable name for r.
func (r GPUDeviceLostReason) String() string {
	switch r {
	case GPULostReset:
		return "reset"
	case GPULostDestroyed:
		return "destroyed"
	case GPULostOutOfMemory:
		return "out_of_memory"
	case GPULostDeviceRemoved:
		return "device_removed"
	case GPULostContextLost:
		return "context_lost"
	case GPULostSurfaceDestroyed:
		return "surface_destroyed"
	default:
		return "unknown"
	}
}

// GPUErrorCode classifies a GPU validation or runtime error.
type GPUErrorCode uint8

const (
	GPUErrorNone GPUErrorCode = iota
	GPUErrorUnsupported
	GPUErrorInvalidDescriptor
	GPUErrorShaderCompile
	GPUErrorOutOfMemory
	GPUErrorDeviceLost
	GPUErrorFailed
)

// Frame-in-flight bounds for per-frame ring-buffer pipelining.
const (
	GPUDefaultFramesInFlight = 2
	GPUMinFramesInFlight     = 2
	GPUMaxFramesInFlight     = 3
)

// NormalizeFramesInFlight clamps n to [GPUMinFramesInFlight, GPUMaxFramesInFlight],
// defaulting zero or negative values to GPUDefaultFramesInFlight.
func NormalizeFramesInFlight(n int) int {
	if n <= 0 {
		return GPUDefaultFramesInFlight
	}
	if n < GPUMinFramesInFlight {
		return GPUMinFramesInFlight
	}
	if n > GPUMaxFramesInFlight {
		return GPUMaxFramesInFlight
	}
	return n
}

// GPUViewConfig is the mount configuration for a KindGPU view (ItemGPU).
type GPUViewConfig struct {
	ColorFormat    GPUTextureFormat
	ColorSpace     GPUColorSpace
	DepthFormat    GPUTextureFormat
	SampleCount    int
	FramesInFlight int
}

// GPULimits reports device capability limits.
type GPULimits struct {
	MaxTextureDimension2D         uint32
	MaxBufferSize                 uint64
	MaxUniformBufferSize          uint64
	MaxStorageBufferBindingSize   uint64
	MaxBindGroups                 uint32
	MaxVertexBuffers              uint32
	MaxVertexAttributes           uint32
	MinUniformBufferAlignment     uint32
	MaxComputeWorkgroupSizeX      uint32
	MaxComputeWorkgroupSizeY      uint32
	MaxComputeWorkgroupSizeZ      uint32
	MaxComputeInvocationsPerGroup uint32
	MaxComputeWorkgroupsPerDim    uint32
	// MaxColorAttachments is the number of color targets a pipeline and render pass may use (multiple render targets). 0 means 1.
	MaxColorAttachments   uint32
	MaxTextureArrayLayers uint32
}

// DefaultGPULimits returns conservative portable GPU limits.
func DefaultGPULimits() GPULimits {
	return GPULimits{
		MaxTextureDimension2D:         4096,
		MaxBufferSize:                 256 << 20,
		MaxUniformBufferSize:          64 << 10,
		MaxStorageBufferBindingSize:   128 << 20,
		MaxBindGroups:                 4,
		MaxVertexBuffers:              8,
		MaxVertexAttributes:           16,
		MinUniformBufferAlignment:     256,
		MaxComputeWorkgroupSizeX:      256,
		MaxComputeWorkgroupSizeY:      256,
		MaxComputeWorkgroupSizeZ:      64,
		MaxComputeInvocationsPerGroup: 256,
		MaxComputeWorkgroupsPerDim:    65535,
		MaxColorAttachments:           1,
		MaxTextureArrayLayers:         256,
	}
}

// GPUFeatures reports optional GPU capabilities supported by the active device.
type GPUFeatures struct {
	StorageBuffers         bool
	StorageTextures        bool
	Compute                bool
	MSAA4x                 bool
	Depth32Float           bool
	Anisotropy             bool
	TimestampQueries       bool
	TextureCompressionBC   bool
	TextureCompressionETC2 bool
	TextureCompressionASTC bool
	// CompressedFormats optionally enumerates the exact compressed formats supported by the device. When empty, supported formats are derived from TextureCompressionBC, TextureCompressionETC2, and TextureCompressionASTC.
	CompressedFormats []GPUTextureFormat
	// ExtendedFormats reports support for GPUFormatBGRA8UnormSRGB, GPUFormatRG11B10Float, GPUFormatR32Float, GPUFormatRGBA32Float and GPUFormatStencil8.
	ExtendedFormats bool
	// StencilTest reports support for pipeline stencil testing (GPUDepthStencilState stencil faces and masks).
	StencilTest bool
	// MipmapGeneration reports support for GPUOpGenerateMipmaps (box-filtered mip chains generated on the device).
	MipmapGeneration bool
	// ImportD3D11Shared and ImportDMABuf report support for GPUOpImportTexture of each GPUExternalTextureKind.
	ImportD3D11Shared bool
	ImportDMABuf      bool
}

// SupportsImport reports whether f supports GPUOpImportTexture of kind.
func (f GPUFeatures) SupportsImport(kind GPUExternalTextureKind) bool {
	switch kind {
	case GPUExternalD3D11Shared:
		return f.ImportD3D11Shared
	case GPUExternalDMABuf:
		return f.ImportDMABuf
	default:
		return false
	}
}

// DefaultSoftwareGPUFeatures returns the capabilities supported by the pure-Go headless software GPU device.
func DefaultSoftwareGPUFeatures() GPUFeatures {
	return GPUFeatures{
		StorageBuffers:         true,
		StorageTextures:        true,
		Compute:                true,
		MSAA4x:                 true,
		Depth32Float:           true,
		TimestampQueries:       true,
		TextureCompressionBC:   true,
		TextureCompressionETC2: true,
		TextureCompressionASTC: true,
		ExtendedFormats:        true,
		StencilTest:            true,
		MipmapGeneration:       true,
	}
}

// DefaultSoftwareGPULimits returns the limits of the pure-Go headless software GPU device, which renders up to four color targets.
func DefaultSoftwareGPULimits() GPULimits {
	l := DefaultGPULimits()
	l.MaxColorAttachments = 4
	return l
}

// SupportedCompressedFormats returns the deduplicated list of compressed texture formats supported by f.
func (f GPUFeatures) SupportedCompressedFormats() []GPUTextureFormat {
	if len(f.CompressedFormats) > 0 {
		out := make([]GPUTextureFormat, 0, len(f.CompressedFormats))
		for _, fm := range f.CompressedFormats {
			if fm.IsCompressed() {
				out = append(out, fm)
			}
		}
		return out
	}
	var out []GPUTextureFormat
	if f.TextureCompressionBC {
		out = append(out,
			GPUFormatBC1RGBAUnorm,
			GPUFormatBC1RGBAUnormSRGB,
			GPUFormatBC3RGBAUnorm,
			GPUFormatBC3RGBAUnormSRGB,
			GPUFormatBC7RGBAUnorm,
			GPUFormatBC7RGBAUnormSRGB,
		)
	}
	if f.TextureCompressionETC2 {
		out = append(out,
			GPUFormatETC2RGB8Unorm,
			GPUFormatETC2RGBA8Unorm,
		)
	}
	if f.TextureCompressionASTC {
		out = append(out,
			GPUFormatASTC4x4Unorm,
			GPUFormatASTC4x4UnormSRGB,
		)
	}
	return out
}

// SupportsCompressedFormat reports whether f supports the compressed format fm.
func (f GPUFeatures) SupportsCompressedFormat(fm GPUTextureFormat) bool {
	if !fm.IsCompressed() {
		return false
	}
	if len(f.CompressedFormats) > 0 {
		for _, candidate := range f.CompressedFormats {
			if candidate == fm {
				return true
			}
		}
		return false
	}
	switch fm {
	case GPUFormatBC1RGBAUnorm, GPUFormatBC1RGBAUnormSRGB,
		GPUFormatBC3RGBAUnorm, GPUFormatBC3RGBAUnormSRGB,
		GPUFormatBC7RGBAUnorm, GPUFormatBC7RGBAUnormSRGB:
		return f.TextureCompressionBC
	case GPUFormatETC2RGB8Unorm, GPUFormatETC2RGBA8Unorm:
		return f.TextureCompressionETC2
	case GPUFormatASTC4x4Unorm, GPUFormatASTC4x4UnormSRGB:
		return f.TextureCompressionASTC
	default:
		return false
	}
}

// GPUBufferDesc describes a GPU buffer allocation.
type GPUBufferDesc struct {
	Label  string
	Size   uint64
	Usage  GPUBufferUsage
	Data   []byte
	Shared []byte // optional in-process shared backing slice for ring buffers
}

// GPUTextureDesc describes a GPU texture allocation.
type GPUTextureDesc struct {
	Label       string
	Dimension   GPUTextureDimension
	Width       uint32
	Height      uint32
	Layers      uint32
	MipLevels   uint32
	SampleCount int
	Format      GPUTextureFormat
	Usage       GPUTextureUsage
}

// GPUExternalTextureKind identifies the GPU memory a GPUOpImportTexture opens.
type GPUExternalTextureKind uint8

const (
	// GPUExternalD3D11Shared is a Direct3D 11 texture shared through an NT handle (IDXGIResource1::CreateSharedHandle).
	GPUExternalD3D11Shared GPUExternalTextureKind = iota + 1
	// GPUExternalDMABuf is a Linux DMA-BUF imported through EGL_EXT_image_dma_buf_import.
	GPUExternalDMABuf
)

// GPUDMABufPlane is one plane of a DMA-BUF: its file descriptor, byte offset and row pitch.
type GPUDMABufPlane struct {
	FD     int
	Offset uint32
	Pitch  uint32
}

// GPUDRMFormatModInvalid is DRM_FORMAT_MOD_INVALID: the DMA-BUF's layout is implied by its driver rather than named.
const GPUDRMFormatModInvalid uint64 = 0x00ffffffffffffff

// GPUExternalTextureDesc describes a texture imported from GPU memory another API or device owns. The imported texture is a sampled 2D texture of Width x Height texels in Format.
type GPUExternalTextureDesc struct {
	Label  string
	Kind   GPUExternalTextureKind
	Width  uint32
	Height uint32
	Format GPUTextureFormat

	// D3D11: the shared NT handle, the plane of a planar texture (0 luma, 1 chroma for NV12 and P010), and the keyed mutex to hold while reading it.
	Handle     uintptr
	Plane      uint32
	KeyedMutex bool
	AcquireKey uint64
	ReleaseKey uint64

	// DMA-BUF: the plane's single-plane DRM fourcc, its format modifier (GPUDRMFormatModInvalid for none) and its one plane.
	FourCC   uint32
	Modifier uint64
	Planes   []GPUDMABufPlane
}

// GPUSamplerDesc describes a GPU texture sampler.
type GPUSamplerDesc struct {
	Label         string
	MinFilter     GPUFilterMode
	MagFilter     GPUFilterMode
	MipmapFilter  GPUFilterMode
	AddressU      GPUAddressMode
	AddressV      GPUAddressMode
	AddressW      GPUAddressMode
	MinLOD        float32
	MaxLOD        float32
	Compare       GPUCompareFunc
	MaxAnisotropy uint16
}

// GPUBindGroupLayoutEntry describes one binding slot in a bind group layout.
type GPUBindGroupLayoutEntry struct {
	Binding          uint32
	Visibility       GPUShaderStageFlags
	Type             GPUBindingType
	HasDynamicOffset bool
}

// GPUBindGroupLayoutDesc describes a bind group layout.
type GPUBindGroupLayoutDesc struct {
	Label   string
	Entries []GPUBindGroupLayoutEntry
}

// GPUBindGroupEntry binds a buffer, texture, or sampler to a slot.
type GPUBindGroupEntry struct {
	Binding uint32
	Buffer  GPUID
	Offset  uint64
	Size    uint64
	Texture GPUID
	Sampler GPUID
}

// GPUBindGroupDesc describes a bind group instance.
type GPUBindGroupDesc struct {
	Label   string
	Layout  GPUID
	Entries []GPUBindGroupEntry
}

// GPUVertexAttribute describes one vertex attribute within a vertex buffer.
type GPUVertexAttribute struct {
	Format   GPUVertexFormat
	Offset   uint64
	Location uint32
}

// GPUVertexBufferLayout describes one vertex buffer slot in a render pipeline.
type GPUVertexBufferLayout struct {
	Stride     uint64
	StepMode   GPUVertexStepMode
	StepRate   uint32 // 0 defaults to 1 for GPUStepInstance (advance once per StepRate instances)
	Attributes []GPUVertexAttribute
}

// GPUShaderStage pairs a compiled shaderir.Program with an entry point name.
type GPUShaderStage struct {
	Label      string
	IR         *shaderir.Program
	EntryPoint string
}

// GPUPrimitiveState configures primitive assembly and face culling.
type GPUPrimitiveState struct {
	Topology  GPUPrimitiveTopology
	FrontFace GPUFrontFace
	CullMode  GPUCullMode
}

// GPUDepthStencilState configures depth and stencil testing, writing, and depth bias.
type GPUDepthStencilState struct {
	Format              GPUTextureFormat
	DepthWrite          bool
	DepthCompare        GPUCompareFunc
	StencilFront        GPUStencilFaceState
	StencilBack         GPUStencilFaceState
	StencilReadMask     uint32
	StencilWriteMask    uint32
	DepthBias           int32
	DepthBiasSlopeScale float32
	DepthBiasClamp      float32
}

// GPUBlendComponent describes one channel (color or alpha) blend equation.
type GPUBlendComponent struct {
	SrcFactor GPUBlendFactor
	DstFactor GPUBlendFactor
	Operation GPUBlendOperation
}

// GPUBlendState configures color and alpha blending for a color target.
type GPUBlendState struct {
	Color GPUBlendComponent
	Alpha GPUBlendComponent
}

// GPUColorTargetState describes one color attachment output of a render pipeline.
type GPUColorTargetState struct {
	Format    GPUTextureFormat
	Blend     *GPUBlendState
	WriteMask GPUColorWriteMask
}

// GPUPipelineBindingSlot records a compiled target shader resource binding slot (Metal buffer/texture/sampler slot or HLSL b/t/s register).
type GPUPipelineBindingSlot struct {
	Group   uint32 `json:"group"`
	Binding uint32 `json:"binding"`
	Kind    uint8  `json:"kind"`
	Slot    uint32 `json:"slot"`
}

// GPUPipelineUniformBlock records a compiled GLSL uniform block slot.
type GPUPipelineUniformBlock struct {
	Group     uint32 `json:"group"`
	Binding   uint32 `json:"binding"`
	BlockName string `json:"block_name"`
	Slot      uint32 `json:"slot"`
}

// GPUPipelineSamplerPair records a synthesized GLSL combined sampler pair.
type GPUPipelineSamplerPair struct {
	Name           string `json:"name"`
	Unit           uint32 `json:"unit"`
	TextureGroup   uint32 `json:"texture_group"`
	TextureBinding uint32 `json:"texture_binding"`
	SamplerGroup   uint32 `json:"sampler_group"`
	SamplerBinding uint32 `json:"sampler_binding"`
}

// GPUPipelineArtifacts holds precompiled portable target shader sources and binding tables persisted by gpu.PipelineCache so backends skip recompilation across launches and avoid first-draw hitches.
type GPUPipelineArtifacts struct {
	MSLSource          string                    `json:"msl_source,omitempty"`
	MSLVertexEntry     string                    `json:"msl_vertex_entry,omitempty"`
	MSLFragmentEntry   string                    `json:"msl_fragment_entry,omitempty"`
	MSLSlots           []GPUPipelineBindingSlot  `json:"msl_slots,omitempty"`
	HLSLVertexSource   string                    `json:"hlsl_vertex_source,omitempty"`
	HLSLPixelSource    string                    `json:"hlsl_pixel_source,omitempty"`
	HLSLVertexEntry    string                    `json:"hlsl_vertex_entry,omitempty"`
	HLSLPixelEntry     string                    `json:"hlsl_pixel_entry,omitempty"`
	HLSLRegisters      []GPUPipelineBindingSlot  `json:"hlsl_registers,omitempty"`
	GLSLVertexSource   string                    `json:"glsl_vertex_source,omitempty"`
	GLSLFragmentSource string                    `json:"glsl_fragment_source,omitempty"`
	GLSLUniformBlocks  []GPUPipelineUniformBlock `json:"glsl_uniform_blocks,omitempty"`
	GLSLSamplerPairs   []GPUPipelineSamplerPair  `json:"glsl_sampler_pairs,omitempty"`
	GLESVertexSource   string                    `json:"gles_vertex_source,omitempty"`
	GLESFragmentSource string                    `json:"gles_fragment_source,omitempty"`
	GLESUniformBlocks  []GPUPipelineUniformBlock `json:"gles_uniform_blocks,omitempty"`
	GLESSamplerPairs   []GPUPipelineSamplerPair  `json:"gles_sampler_pairs,omitempty"`
	WGSLSource         string                    `json:"wgsl_source,omitempty"`
}

// GPUPipelineDesc describes an immutable render pipeline.
type GPUPipelineDesc struct {
	Label           string
	Vertex          GPUShaderStage
	Fragment        GPUShaderStage
	Layouts         []GPUID
	VertexBuffers   []GPUVertexBufferLayout
	Primitive       GPUPrimitiveState
	DepthStencil    *GPUDepthStencilState
	Targets         []GPUColorTargetState
	SampleCount     int
	CacheKey        string
	CachedArtifacts *GPUPipelineArtifacts
}

// GPUComputePipelineDesc describes an immutable compute pipeline.
type GPUComputePipelineDesc struct {
	Label   string
	Compute GPUShaderStage
	Layouts []GPUID
}

// GPUTextureCopy identifies a subresource and origin within a GPU texture.
type GPUTextureCopy struct {
	Texture    GPUID
	MipLevel   uint32
	ArrayLayer uint32
	OriginX    uint32
	OriginY    uint32
}

// GPUDataLayout describes linear pixel data layout in a Go byte slice or buffer.
type GPUDataLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

// GPUTextureRegion selects a rectangular region of a texture for CPU readback.
type GPUTextureRegion struct {
	MipLevel   uint32
	ArrayLayer uint32
	X, Y       uint32
	Width      uint32
	Height     uint32
}

// GPUResourceOpKind identifies a retained GPU resource lifecycle or upload op.
type GPUResourceOpKind uint8

const (
	GPUOpCreateBuffer GPUResourceOpKind = iota + 1
	GPUOpWriteBuffer
	GPUOpDestroyBuffer
	GPUOpCreateTexture
	GPUOpWriteTexture
	GPUOpDestroyTexture
	GPUOpCreateSampler
	GPUOpDestroySampler
	GPUOpCreateBindGroupLayout
	GPUOpDestroyBindGroupLayout
	GPUOpCreateBindGroup
	GPUOpDestroyBindGroup
	GPUOpCreatePipeline
	GPUOpDestroyPipeline
	GPUOpReadTexture
	// Compute ops are appended after GPUOpReadTexture so the numeric op kinds already on the Android wire (internal/platform/android/gpu.go) keep their values.
	GPUOpCreateComputePipeline
	GPUOpDestroyComputePipeline
	GPUOpReadBuffer
	GPUOpGenerateMipmaps
	// GPUOpImportTexture opens External as the texture ID; GPUOpDestroyTexture releases it.
	GPUOpImportTexture
)

// GPUResourceOp is one resource creation, update, destruction, or readback op.
type GPUResourceOp struct {
	Kind            GPUResourceOpKind
	ID              GPUID
	Offset          uint64
	Data            []byte
	Buffer          *GPUBufferDesc
	Texture         *GPUTextureDesc
	Sampler         *GPUSamplerDesc
	BindGroupLayout *GPUBindGroupLayoutDesc
	BindGroup       *GPUBindGroupDesc
	Pipeline        *GPUPipelineDesc
	ComputePipeline *GPUComputePipelineDesc
	External        *GPUExternalTextureDesc
	TextureDst      GPUTextureCopy
	DataLayout      GPUDataLayout
	ReadbackID      uint64
	ReadbackSize    uint64
	ReadbackRegion  GPUTextureRegion
	ReadbackSurface bool
}

// GPUCommandKind identifies a render-pass or compute-pass command.
type GPUCommandKind uint8

const (
	GPUCmdSetViewport GPUCommandKind = iota + 1
	GPUCmdSetScissor
	GPUCmdSetPipeline
	GPUCmdSetBindGroup
	GPUCmdSetVertexBuffer
	GPUCmdSetIndexBuffer
	GPUCmdSetStencilReference
	GPUCmdSetBlendConstant
	GPUCmdDraw
	GPUCmdDrawIndexed
	GPUCmdDrawInstanced
	GPUCmdDrawIndexedInstanced
	GPUCmdPushDebugGroup
	GPUCmdPopDebugGroup
	GPUCmdInsertDebugMarker
	GPUCmdSetComputePipeline
	GPUCmdDispatch
	GPUCmdDispatchIndirect
)

// GPUCommand is one encoded command inside a GPUPass.
type GPUCommand struct {
	Kind           GPUCommandKind
	ID             GPUID
	Slot           uint32
	Offset         uint64
	IndexFormat    GPUIndexFormat
	DynamicOffsets []uint32
	Label          string
	// Viewport / BlendConstant floats
	F0, F1, F2, F3, F4, F5 float32
	// Scissor / BaseVertex signed ints
	I0, I1, I2, I3 int32
	// Draw counts / first indices / stencil ref
	U0, U1, U2, U3 uint32
}

// GPUColorAttachment configures one color target of a render pass.
// Texture == 0 targets the view's swapchain drawable.
type GPUColorAttachment struct {
	Texture       GPUID
	ResolveTarget GPUID
	MipLevel      uint32
	ArrayLayer    uint32
	LoadOp        GPULoadOp
	StoreOp       GPUStoreOp
	ClearColor    Color
}

// GPUDepthStencilAttachment configures the depth/stencil target of a render pass.
// Texture == 0 targets the view's automatic swapchain depth buffer.
type GPUDepthStencilAttachment struct {
	Texture        GPUID
	DepthLoadOp    GPULoadOp
	DepthStoreOp   GPUStoreOp
	ClearDepth     float32
	StencilLoadOp  GPULoadOp
	StencilStoreOp GPUStoreOp
	ClearStencil   uint32
}

// GPUPass is one render pass or compute pass recorded in a GPUFrame.
type GPUPass struct {
	Label            string
	TimingLabel      string
	TimestampQueryID GPUID
	IsCompute        bool
	ColorAttachments []GPUColorAttachment
	DepthStencil     *GPUDepthStencilAttachment
	Commands         []GPUCommand
}

// GPUDebugOpKind identifies an encoder-level debug group or marker command.
type GPUDebugOpKind uint8

const (
	GPUDebugPushGroup GPUDebugOpKind = iota + 1
	GPUDebugPopGroup
	GPUDebugInsertMarker
)

// GPUDebugOp records an encoder-level debug group boundary or marker positioned relative to the frame's Passes slice (PassIndex is the pass index at which the op was recorded).
type GPUDebugOp struct {
	Kind      GPUDebugOpKind
	Label     string
	PassIndex int
}

// GPUPassTiming reports the measured GPU execution duration of one render pass.
type GPUPassTiming struct {
	Label     string
	PassIndex int
	QueryID   GPUID
	Duration  time.Duration
}

// GPUCopyOpKind identifies a GPU copy command outside a render pass.
type GPUCopyOpKind uint8

const (
	GPUCopyBufferToBuffer GPUCopyOpKind = iota + 1
	GPUCopyBufferToTexture
	GPUCopyTextureToTexture
)

// GPUCopyOp describes a buffer/texture copy recorded on an encoder.
type GPUCopyOp struct {
	Kind      GPUCopyOpKind
	SrcID     GPUID
	DstID     GPUID
	SrcOffset uint64
	DstOffset uint64
	Size      uint64
	SrcLayout GPUDataLayout
	SrcTex    GPUTextureCopy
	DstTex    GPUTextureCopy
	Width     uint32
	Height    uint32
	Layers    uint32
}

// GPUFrame is one off-commit batch of GPU resource operations, copy commands,
// and render passes submitted directly to a platform backend without triggering
// a UI view-tree commit.
type GPUFrame struct {
	SurfaceID  uint32
	ViewID     NodeID
	Generation uint32
	FrameIndex uint64
	FrameSlot  int
	FenceID    uint64
	OnComplete func() // optional callback invoked once the backend finishes this frame
	Ops        []GPUResourceOp
	Copies     []GPUCopyOp
	Passes     []GPUPass
	DebugOps   []GPUDebugOp
	Present    bool
}

// GPUEventKind identifies an asynchronous report from a GPU backend to core.
type GPUEventKind uint8

const (
	GPUEventDeviceReady GPUEventKind = iota + 1
	GPUEventDeviceLost
	GPUEventResize
	GPUEventError
	GPUEventReadback
	GPUEventPassTimings
	GPUEventFrameComplete
)

// GPUEvent is the payload carried in Event.Data for EventGPU.
type GPUEvent struct {
	Kind              GPUEventKind
	Generation        uint32
	Width             int
	Height            int
	SurfaceFormat     GPUTextureFormat
	SurfaceColorSpace GPUColorSpace
	FramesInFlight    int
	Limits            GPULimits
	Features          GPUFeatures
	LostReason        GPUDeviceLostReason
	ErrorCode         GPUErrorCode
	Message           string
	ReadbackID        uint64
	Pixels            []byte
	PassTimings       []GPUPassTiming
	FrameIndex        uint64
	FrameSlot         int
	FenceID           uint64
}
