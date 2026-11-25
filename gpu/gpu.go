// Package gpu provides a portable WebGPU-shaped 3D rendering API built on
// core's KindGPU surface hook (docs/architecture/gpu.md).
//
// Persistent GPU resources (Buffer, Texture, Sampler, RenderTarget,
// BindGroupLayout, BindGroup, Pipeline) are scoped to a *core.Scope and
// released automatically on disposal. Per-frame command recording submits
// directly to the platform render thread with zero UI view-tree commits.
package gpu

import (
	"errors"
	"fmt"
	"image"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// BufferUsage is a bitmask of permitted uses for a Buffer.
type BufferUsage uint32

const (
	UsageVertex   BufferUsage = BufferUsage(driver.GPUBufferVertex)
	UsageIndex    BufferUsage = BufferUsage(driver.GPUBufferIndex)
	UsageUniform  BufferUsage = BufferUsage(driver.GPUBufferUniform)
	UsageStorage  BufferUsage = BufferUsage(driver.GPUBufferStorage)
	UsageCopySrc  BufferUsage = BufferUsage(driver.GPUBufferCopySrc)
	UsageCopyDst  BufferUsage = BufferUsage(driver.GPUBufferCopyDst)
	UsageIndirect BufferUsage = BufferUsage(driver.GPUBufferIndirect)

	// BufferUsageStorage is an alias for UsageStorage for storage buffers (var<storage, read> and var<storage, read_write>).
	BufferUsageStorage = UsageStorage
	// BufferUsageIndirect is an alias for UsageIndirect for indirect dispatch buffers.
	BufferUsageIndirect = UsageIndirect
)

// Frames-in-flight bounds for per-frame ring-buffer pipelining.
const (
	DefaultFramesInFlight = driver.GPUDefaultFramesInFlight
	MinFramesInFlight     = driver.GPUMinFramesInFlight
	MaxFramesInFlight     = driver.GPUMaxFramesInFlight
)

// BufferDesc configures Device.CreateBuffer.
type BufferDesc struct {
	Label    string
	Size     uint64
	Usage    BufferUsage
	Data     []byte // optional initial contents; copied before CreateBuffer returns
	Retained bool   // opt in to automatic recreation with latest data after device loss
}

// RingBufferDesc configures Device.CreateRingBuffer.
type RingBufferDesc struct {
	Label string
	// SlotSize is the byte capacity available per frame-in-flight slot.
	// Total underlying GPU buffer capacity is SlotSize (rounded up to Alignment)
	// multiplied by Device.FramesInFlight().
	SlotSize uint64
	// Usage selects how the ring buffer is bound (e.g. UsageUniform, UsageVertex,
	// UsageIndex, or a combination). UsageCopyDst is always included.
	Usage BufferUsage
	// Alignment overrides the per-allocation byte alignment. When zero, uniform
	// ring buffers align to Device.Limits().MinUniformBufferAlignment (256 bytes)
	// and non-uniform ring buffers align to 16 bytes.
	Alignment uint64
}

// BufferSlice describes one aligned sub-allocation within a RingBuffer's active
// frame-in-flight slot.
type BufferSlice struct {
	Buffer *Buffer // underlying retained GPU buffer
	Offset uint64  // absolute byte offset within Buffer (aligned for dynamic offsets / SetVertexBuffer)
	Size   uint64  // requested byte length
	Slot   int     // frame-in-flight slot index in [0, FramesInFlight)
	Data   []byte  // writable CPU slice of length Size backing this sub-allocation
}

// TextureDimension is the dimensionality of a Texture.
type TextureDimension uint8

const (
	Dim2D      TextureDimension = TextureDimension(driver.GPUDim2D)
	DimCube    TextureDimension = TextureDimension(driver.GPUDimCube)
	Dim2DArray TextureDimension = TextureDimension(driver.GPUDim2DArray)
)

// CubeFace identifies one face (ArrayLayer 0..5) of a DimCube Texture in canonical WebGPU / Metal / Direct3D / OpenGL order.
type CubeFace = uint32

const (
	CubeFacePosX CubeFace = driver.GPUCubeFacePosX
	CubeFaceNegX CubeFace = driver.GPUCubeFaceNegX
	CubeFacePosY CubeFace = driver.GPUCubeFacePosY
	CubeFaceNegY CubeFace = driver.GPUCubeFaceNegY
	CubeFacePosZ CubeFace = driver.GPUCubeFacePosZ
	CubeFaceNegZ CubeFace = driver.GPUCubeFaceNegZ
)

// MaxMipLevels returns the number of mipmap levels in a full chain from width x height down to 1x1.
func MaxMipLevels(width, height uint32) uint32 {
	return driver.GPUMipLevels(width, height)
}

// MipSize returns the width and height of mipLevel for a base texture of width x height (clamped to a minimum of 1x1).
func MipSize(width, height, mipLevel uint32) (uint32, uint32) {
	return driver.GPUMipSize(width, height, mipLevel)
}

// DownsampleRGBA2x produces the next mipmap level from src using a 2x2 box filter.
func DownsampleRGBA2x(src *image.RGBA) *image.RGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	dw := max(sw/2, 1)
	dh := max(sh/2, 1)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for dy := 0; dy < dh; dy++ {
		sy0 := min(dy*2, sh-1)
		sy1 := min(sy0+1, sh-1)
		for dx := 0; dx < dw; dx++ {
			sx0 := min(dx*2, sw-1)
			sx1 := min(sx0+1, sw-1)
			i00 := src.PixOffset(b.Min.X+sx0, b.Min.Y+sy0)
			i10 := src.PixOffset(b.Min.X+sx1, b.Min.Y+sy0)
			i01 := src.PixOffset(b.Min.X+sx0, b.Min.Y+sy1)
			i11 := src.PixOffset(b.Min.X+sx1, b.Min.Y+sy1)
			di := dst.PixOffset(dx, dy)
			for ch := 0; ch < 4; ch++ {
				sum := int(src.Pix[i00+ch]) + int(src.Pix[i10+ch]) + int(src.Pix[i01+ch]) + int(src.Pix[i11+ch])
				dst.Pix[di+ch] = byte((sum + 2) / 4)
			}
		}
	}
	return dst
}

// GenerateMipChainRGBA generates up to maxLevels mipmap images starting from base (level 0) using a 2x2 box filter. When maxLevels is 0, the full chain down to 1x1 is returned.
func GenerateMipChainRGBA(base *image.RGBA, maxLevels uint32) []*image.RGBA {
	if base == nil {
		return nil
	}
	b := base.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil
	}
	full := MaxMipLevels(uint32(b.Dx()), uint32(b.Dy()))
	if maxLevels == 0 || maxLevels > full {
		maxLevels = full
	}
	chain := make([]*image.RGBA, 0, maxLevels)
	cur := base
	chain = append(chain, cur)
	for lvl := uint32(1); lvl < maxLevels; lvl++ {
		cur = DownsampleRGBA2x(cur)
		chain = append(chain, cur)
	}
	return chain
}

// TextureFormat identifies the pixel or depth/stencil layout of a Texture.
type TextureFormat uint8

const (
	FormatUndefined           TextureFormat = TextureFormat(driver.GPUFormatUndefined)
	FormatRGBA8Unorm          TextureFormat = TextureFormat(driver.GPUFormatRGBA8Unorm)
	FormatRGBA8UnormSRGB      TextureFormat = TextureFormat(driver.GPUFormatRGBA8UnormSRGB)
	FormatBGRA8Unorm          TextureFormat = TextureFormat(driver.GPUFormatBGRA8Unorm)
	FormatRG8Unorm            TextureFormat = TextureFormat(driver.GPUFormatRG8Unorm)
	FormatR8Unorm             TextureFormat = TextureFormat(driver.GPUFormatR8Unorm)
	FormatRGBA16Float         TextureFormat = TextureFormat(driver.GPUFormatRGBA16Float)
	FormatDepth24Plus         TextureFormat = TextureFormat(driver.GPUFormatDepth24Plus)
	FormatDepth24PlusStencil8 TextureFormat = TextureFormat(driver.GPUFormatDepth24PlusStencil8)
	FormatDepth32Float        TextureFormat = TextureFormat(driver.GPUFormatDepth32Float)
	FormatBC1RGBAUnorm        TextureFormat = TextureFormat(driver.GPUFormatBC1RGBAUnorm)
	FormatBC1RGBAUnormSRGB    TextureFormat = TextureFormat(driver.GPUFormatBC1RGBAUnormSRGB)
	FormatBC3RGBAUnorm        TextureFormat = TextureFormat(driver.GPUFormatBC3RGBAUnorm)
	FormatBC3RGBAUnormSRGB    TextureFormat = TextureFormat(driver.GPUFormatBC3RGBAUnormSRGB)
	FormatBC7RGBAUnorm        TextureFormat = TextureFormat(driver.GPUFormatBC7RGBAUnorm)
	FormatBC7RGBAUnormSRGB    TextureFormat = TextureFormat(driver.GPUFormatBC7RGBAUnormSRGB)
	FormatETC2RGB8Unorm       TextureFormat = TextureFormat(driver.GPUFormatETC2RGB8Unorm)
	FormatETC2RGBA8Unorm      TextureFormat = TextureFormat(driver.GPUFormatETC2RGBA8Unorm)
	FormatASTC4x4Unorm        TextureFormat = TextureFormat(driver.GPUFormatASTC4x4Unorm)
	FormatASTC4x4UnormSRGB    TextureFormat = TextureFormat(driver.GPUFormatASTC4x4UnormSRGB)

	// Extended formats: accepted only where Features.ExtendedFormats is set (Device.SupportsFormat).
	FormatBGRA8UnormSRGB TextureFormat = TextureFormat(driver.GPUFormatBGRA8UnormSRGB)
	FormatRG11B10Float   TextureFormat = TextureFormat(driver.GPUFormatRG11B10Float)
	FormatR32Float       TextureFormat = TextureFormat(driver.GPUFormatR32Float)
	FormatRGBA32Float    TextureFormat = TextureFormat(driver.GPUFormatRGBA32Float)
	FormatStencil8       TextureFormat = TextureFormat(driver.GPUFormatStencil8)

	// Import-only formats: textures from Device.ImportTexture, such as the planes of a P010 frame, have them; Device.CreateTexture does not take them.
	FormatR16Unorm  TextureFormat = TextureFormat(driver.GPUFormatR16Unorm)
	FormatRG16Unorm TextureFormat = TextureFormat(driver.GPUFormatRG16Unorm)

	// FormatRGB10A2Unorm is a 10-bit-per-channel RGB + 2-bit alpha normalized format used for HDR10 (PQ/HLG) swapchains and render targets.
	FormatRGB10A2Unorm TextureFormat = TextureFormat(driver.GPUFormatRGB10A2Unorm)

	// Short HDR float format aliases matching WGSL/shader shorthand.
	FormatRGBA16F  TextureFormat = FormatRGBA16Float
	FormatRG11B10F TextureFormat = FormatRG11B10Float
	FormatR32F     TextureFormat = FormatR32Float
	FormatRGBA32F  TextureFormat = FormatRGBA32Float
)

// ColorSpace identifies the color primaries and transfer function of a gpu.View swapchain.
type ColorSpace uint8

const (
	// ColorSpaceDefault selects the platform's default SDR swapchain color space (sRGB).
	ColorSpaceDefault ColorSpace = ColorSpace(driver.GPUColorSpaceDefault)
	// ColorSpaceSRGB is standard sRGB (Rec. 709 primaries, sRGB transfer, [0, 1] range).
	ColorSpaceSRGB ColorSpace = ColorSpace(driver.GPUColorSpaceSRGB)
	// ColorSpaceExtendedLinearSRGB is linear-light extended-range sRGB / scRGB
	// (Rec. 709 primaries, linear transfer, 1.0 = SDR reference white, values > 1.0 for HDR).
	ColorSpaceExtendedLinearSRGB ColorSpace = ColorSpace(driver.GPUColorSpaceExtendedLinearSRGB)
	// ColorSpaceHDR10PQ is BT.2020 primaries with SMPTE ST 2084 (PQ) transfer.
	ColorSpaceHDR10PQ ColorSpace = ColorSpace(driver.GPUColorSpaceHDR10PQ)
	// ColorSpaceHDR10HLG is BT.2020 primaries with Hybrid Log-Gamma (HLG) transfer.
	ColorSpaceHDR10HLG ColorSpace = ColorSpace(driver.GPUColorSpaceHDR10HLG)
	// ColorSpaceDisplayP3 is Display P3 primaries with sRGB transfer.
	ColorSpaceDisplayP3 ColorSpace = ColorSpace(driver.GPUColorSpaceDisplayP3)
)

// IsHDR reports whether c is an HDR / extended dynamic range color space.
func (c ColorSpace) IsHDR() bool {
	return driver.GPUColorSpace(c).IsHDR()
}

// String returns the color space's canonical name.
func (c ColorSpace) String() string {
	return driver.GPUColorSpace(c).String()
}

// IsDepth reports whether f carries a depth aspect.
func (f TextureFormat) IsDepth() bool {
	return driver.GPUTextureFormat(f).IsDepth()
}

// IsCompressed reports whether f is a block-compressed texture format (BCn, ETC2, ASTC).
func (f TextureFormat) IsCompressed() bool {
	return driver.GPUTextureFormat(f).IsCompressed()
}

// IsSRGB reports whether f decodes sRGB RGB channels to linear on texture sampling.
func (f TextureFormat) IsSRGB() bool {
	return driver.GPUTextureFormat(f).IsSRGB()
}

// BlockSize returns the compression block dimensions (width, height) in texels. Uncompressed formats return (1, 1); FormatUndefined returns (0, 0).
func (f TextureFormat) BlockSize() (width, height uint32) {
	return driver.GPUTextureFormat(f).BlockSize()
}

// BytesPerBlock returns the byte size of one texel block (or one texel for uncompressed color formats; 0 for depth or undefined formats).
func (f TextureFormat) BytesPerBlock() uint32 {
	return driver.GPUTextureFormat(f).BytesPerBlock()
}

// ByteSize returns the tightly-packed byte size of a 2D texture level of width x height texels in format f.
func (f TextureFormat) ByteSize(width, height uint32) uint64 {
	return driver.GPUTextureFormat(f).ByteSize(width, height)
}

// String returns the format's canonical name.
func (f TextureFormat) String() string {
	return driver.GPUTextureFormat(f).String()
}

// HasStencil reports whether f carries a stencil aspect.
func (f TextureFormat) HasStencil() bool {
	return driver.GPUTextureFormat(f).HasStencil()
}

// IsDepthStencil reports whether f is a depth, stencil, or depth-stencil format.
func (f TextureFormat) IsDepthStencil() bool {
	return driver.GPUTextureFormat(f).IsDepthStencil()
}

// IsFloat reports whether f is a floating-point color format (HDR/unclamped).
func (f TextureFormat) IsFloat() bool {
	return driver.GPUTextureFormat(f).IsFloat()
}

// TextureUsage is a bitmask of permitted uses for a Texture.
type TextureUsage uint32

const (
	TextureSampled          TextureUsage = TextureUsage(driver.GPUTextureSampled)
	TextureRenderAttachment TextureUsage = TextureUsage(driver.GPUTextureRenderAttachment)
	TextureCopySrc          TextureUsage = TextureUsage(driver.GPUTextureCopySrc)
	TextureCopyDst          TextureUsage = TextureUsage(driver.GPUTextureCopyDst)
	TextureStorageBinding   TextureUsage = TextureUsage(driver.GPUTextureStorage)
)

// TextureDesc configures Device.CreateTexture.
type TextureDesc struct {
	Label       string
	Dimension   TextureDimension
	Width       uint32
	Height      uint32
	Layers      uint32 // 0 or 1 for 2D, 6 for Cube, N for 2DArray
	MipLevels   uint32 // 0 defaults to 1
	SampleCount int    // 0 or 1; 4 for MSAA render attachments
	Format      TextureFormat
	Usage       TextureUsage
	Retained    bool // opt in to automatic recreation with latest writes after device loss
}

// RenderTargetDesc configures Device.CreateRenderTarget.
//
// Set ColorFormat for a color-only offscreen target, both ColorFormat and
// DepthFormat for a color+depth target, or DepthFormat alone (with
// ColorFormat == FormatUndefined) for a depth-only target such as a shadow map.
// When both formats are FormatUndefined, ColorFormat defaults to
// FormatRGBA8Unorm. All created attachments include TextureRenderAttachment,
// TextureSampled, and TextureCopySrc so subsequent passes can sample or read
// them back.
type RenderTargetDesc struct {
	Label       string
	Width       uint32
	Height      uint32
	ColorFormat TextureFormat
	DepthFormat TextureFormat
	SampleCount int  // 0 or 1; 4 for a multisampled target with a Resolve texture
	Retained    bool // opt in to automatic recreation after device loss
}

// FilterMode selects texture minification, magnification, or mipmap filtering.
type FilterMode uint8

const (
	FilterNearest FilterMode = FilterMode(driver.GPUFilterNearest)
	FilterLinear  FilterMode = FilterMode(driver.GPUFilterLinear)
)

// AddressMode selects texture coordinate wrapping behavior.
type AddressMode uint8

const (
	AddressClampToEdge  AddressMode = AddressMode(driver.GPUAddressClampToEdge)
	AddressRepeat       AddressMode = AddressMode(driver.GPUAddressRepeat)
	AddressMirrorRepeat AddressMode = AddressMode(driver.GPUAddressMirrorRepeat)
)

// CompareFunc selects a depth, stencil, or comparison-sampler test.
type CompareFunc uint8

const (
	CompareUndefined    CompareFunc = CompareFunc(driver.GPUCompareUndefined)
	CompareNever        CompareFunc = CompareFunc(driver.GPUCompareNever)
	CompareLess         CompareFunc = CompareFunc(driver.GPUCompareLess)
	CompareEqual        CompareFunc = CompareFunc(driver.GPUCompareEqual)
	CompareLessEqual    CompareFunc = CompareFunc(driver.GPUCompareLessEqual)
	CompareGreater      CompareFunc = CompareFunc(driver.GPUCompareGreater)
	CompareNotEqual     CompareFunc = CompareFunc(driver.GPUCompareNotEqual)
	CompareGreaterEqual CompareFunc = CompareFunc(driver.GPUCompareGreaterEqual)
	CompareAlways       CompareFunc = CompareFunc(driver.GPUCompareAlways)
)

// SamplerDesc configures Device.CreateSampler.
type SamplerDesc struct {
	Label         string
	MinFilter     FilterMode
	MagFilter     FilterMode
	MipmapFilter  FilterMode
	AddressU      AddressMode
	AddressV      AddressMode
	AddressW      AddressMode
	MinLOD        float32 // honoured by the software device; native backends do not clamp LOD yet
	MaxLOD        float32 // 0 defaults to 32.0 (unbounded); honoured by the software device only for now
	Compare       CompareFunc
	MaxAnisotropy uint16
	Retained      bool // opt in to automatic recreation after device loss
}

// ShaderDesc configures CompileShader.
type ShaderDesc struct {
	Label  string
	Source string
}

// BindingType classifies an entry in a BindGroupLayout.
type BindingType uint8

const (
	BindingUniform           BindingType = BindingType(driver.GPUBindingUniform)
	BindingStorageRead       BindingType = BindingType(driver.GPUBindingStorageRead)
	BindingStorage           BindingType = BindingType(driver.GPUBindingStorage)
	BindingTexture           BindingType = BindingType(driver.GPUBindingTexture)
	BindingStorageTexture    BindingType = BindingType(driver.GPUBindingStorageTexture)
	BindingSampler           BindingType = BindingType(driver.GPUBindingSampler)
	BindingComparisonSampler BindingType = BindingType(driver.GPUBindingComparisonSampler)

	// Descriptive aliases for compute and storage bind group layout entries.
	BindingTypeStorageBuffer         = BindingStorage
	BindingTypeReadOnlyStorageBuffer = BindingStorageRead
	BindingTypeStorageTexture        = BindingStorageTexture
)

// ShaderStageFlags specifies which shader stages see a binding.
type ShaderStageFlags uint8

const (
	StageVertex   ShaderStageFlags = ShaderStageFlags(driver.GPUStageVertex)
	StageFragment ShaderStageFlags = ShaderStageFlags(driver.GPUStageFragment)
	StageCompute  ShaderStageFlags = ShaderStageFlags(driver.GPUStageCompute)
	StageAll      ShaderStageFlags = ShaderStageFlags(driver.GPUStageAll)
)

// BindGroupLayoutEntry describes one binding slot in a BindGroupLayout.
type BindGroupLayoutEntry struct {
	Binding          uint32
	Visibility       ShaderStageFlags
	Type             BindingType
	HasDynamicOffset bool
}

// BindGroupLayoutDesc configures Device.CreateBindGroupLayoutDesc.
type BindGroupLayoutDesc struct {
	Label    string
	Entries  []BindGroupLayoutEntry
	Retained bool // opt in to automatic recreation after device loss
}

// BindGroupEntry binds a Buffer, Texture, or Sampler to a slot in a BindGroup.
type BindGroupEntry struct {
	Binding uint32
	Buffer  *Buffer
	Offset  uint64
	Size    uint64 // 0 binds to the end of Buffer
	Texture *Texture
	Sampler *Sampler
}

// BindGroupDesc configures Device.CreateBindGroupDesc.
type BindGroupDesc struct {
	Label   string
	Layout  *BindGroupLayout
	Entries []BindGroupEntry
}

// VertexFormat specifies the format of one vertex attribute.
type VertexFormat uint8

const (
	VertexFloat32   VertexFormat = VertexFormat(driver.GPUVertexFloat32)
	VertexFloat32x2 VertexFormat = VertexFormat(driver.GPUVertexFloat32x2)
	VertexFloat32x3 VertexFormat = VertexFormat(driver.GPUVertexFloat32x3)
	VertexFloat32x4 VertexFormat = VertexFormat(driver.GPUVertexFloat32x4)
	VertexSint32    VertexFormat = VertexFormat(driver.GPUVertexSint32)
	VertexSint32x2  VertexFormat = VertexFormat(driver.GPUVertexSint32x2)
	VertexSint32x3  VertexFormat = VertexFormat(driver.GPUVertexSint32x3)
	VertexSint32x4  VertexFormat = VertexFormat(driver.GPUVertexSint32x4)
	VertexUint32    VertexFormat = VertexFormat(driver.GPUVertexUint32)
	VertexUint32x2  VertexFormat = VertexFormat(driver.GPUVertexUint32x2)
	VertexUint32x3  VertexFormat = VertexFormat(driver.GPUVertexUint32x3)
	VertexUint32x4  VertexFormat = VertexFormat(driver.GPUVertexUint32x4)
	VertexUnorm8x4  VertexFormat = VertexFormat(driver.GPUVertexUnorm8x4)
)

// VertexStepMode selects whether a vertex buffer advances per vertex or per instance.
type VertexStepMode uint8

const (
	StepVertex         VertexStepMode = VertexStepMode(driver.GPUStepVertex)
	StepInstance       VertexStepMode = VertexStepMode(driver.GPUStepInstance)
	VertexStepVertex   VertexStepMode = StepVertex
	VertexStepInstance VertexStepMode = StepInstance
)

// VertexAttribute describes one attribute inside a VertexBufferLayout.
type VertexAttribute struct {
	Format   VertexFormat
	Offset   uint64
	Location uint32
}

// VertexBufferLayout describes one vertex buffer slot of a Pipeline.
type VertexBufferLayout struct {
	Stride     uint64
	StepMode   VertexStepMode
	StepRate   uint32 // 0 defaults to 1 when StepMode is StepInstance / VertexStepInstance
	Attributes []VertexAttribute
}

// IndexFormat specifies the element width of an index buffer.
type IndexFormat uint8

const (
	IndexUint16 IndexFormat = IndexFormat(driver.GPUIndexUint16)
	IndexUint32 IndexFormat = IndexFormat(driver.GPUIndexUint32)
)

// PrimitiveTopology selects how vertices are assembled into primitives.
type PrimitiveTopology uint8

const (
	TopologyTriangleList  PrimitiveTopology = PrimitiveTopology(driver.GPUTopologyTriangleList)
	TopologyTriangleStrip PrimitiveTopology = PrimitiveTopology(driver.GPUTopologyTriangleStrip)
	TopologyLineList      PrimitiveTopology = PrimitiveTopology(driver.GPUTopologyLineList)
	TopologyLineStrip     PrimitiveTopology = PrimitiveTopology(driver.GPUTopologyLineStrip)
	TopologyPointList     PrimitiveTopology = PrimitiveTopology(driver.GPUTopologyPointList)
)

// FrontFace selects the winding order considered front-facing.
type FrontFace uint8

const (
	FrontCCW     FrontFace = FrontFace(driver.GPUFrontCCW)
	FrontCW      FrontFace = FrontFace(driver.GPUFrontCW)
	FrontFaceCCW FrontFace = FrontCCW
	FrontFaceCW  FrontFace = FrontCW
)

// CullMode selects which faces are culled.
type CullMode uint8

const (
	CullNone  CullMode = CullMode(driver.GPUCullNone)
	CullFront CullMode = CullMode(driver.GPUCullFront)
	CullBack  CullMode = CullMode(driver.GPUCullBack)
)

// BlendFactor selects a source or destination blend multiplier.
type BlendFactor uint8

const (
	BlendZero             BlendFactor = BlendFactor(driver.GPUBlendZero)
	BlendOne              BlendFactor = BlendFactor(driver.GPUBlendOne)
	BlendSrc              BlendFactor = BlendFactor(driver.GPUBlendSrc)
	BlendOneMinusSrc      BlendFactor = BlendFactor(driver.GPUBlendOneMinusSrc)
	BlendSrcAlpha         BlendFactor = BlendFactor(driver.GPUBlendSrcAlpha)
	BlendOneMinusSrcAlpha BlendFactor = BlendFactor(driver.GPUBlendOneMinusSrcAlpha)
	BlendDst              BlendFactor = BlendFactor(driver.GPUBlendDst)
	BlendOneMinusDst      BlendFactor = BlendFactor(driver.GPUBlendOneMinusDst)
	BlendDstAlpha         BlendFactor = BlendFactor(driver.GPUBlendDstAlpha)
	BlendOneMinusDstAlpha BlendFactor = BlendFactor(driver.GPUBlendOneMinusDstAlpha)
	BlendConstant         BlendFactor = BlendFactor(driver.GPUBlendConstant)
	BlendOneMinusConstant BlendFactor = BlendFactor(driver.GPUBlendOneMinusConstant)
)

// BlendOperation selects how blended terms combine.
type BlendOperation uint8

// BlendOp is an alias for BlendOperation.
type BlendOp = BlendOperation

const (
	BlendAdd             BlendOperation = BlendOperation(driver.GPUBlendAdd)
	BlendSubtract        BlendOperation = BlendOperation(driver.GPUBlendSubtract)
	BlendReverseSubtract BlendOperation = BlendOperation(driver.GPUBlendReverseSubtract)
	BlendMin             BlendOperation = BlendOperation(driver.GPUBlendMin)
	BlendMax             BlendOperation = BlendOperation(driver.GPUBlendMax)
)

// ColorWriteMask selects which channels a pipeline writes. Zero defaults to ColorWriteAll; use ColorWriteNone to disable all color writes.
type ColorWriteMask uint8

const (
	ColorWriteRed   ColorWriteMask = ColorWriteMask(driver.GPUColorWriteRed)
	ColorWriteGreen ColorWriteMask = ColorWriteMask(driver.GPUColorWriteGreen)
	ColorWriteBlue  ColorWriteMask = ColorWriteMask(driver.GPUColorWriteBlue)
	ColorWriteAlpha ColorWriteMask = ColorWriteMask(driver.GPUColorWriteAlpha)
	ColorWriteNone  ColorWriteMask = ColorWriteMask(driver.GPUColorWriteNone)
	ColorWriteAll   ColorWriteMask = ColorWriteMask(driver.GPUColorWriteAll)
)

// BlendComponent describes one channel's blend equation.
type BlendComponent struct {
	SrcFactor BlendFactor
	DstFactor BlendFactor
	Operation BlendOperation
}

// BlendState configures color and alpha blending.
type BlendState struct {
	Color BlendComponent
	Alpha BlendComponent
}

// BlendOpaque returns opaque replacement blending (src * 1 + dst * 0).
func BlendOpaque() *BlendState {
	return &BlendState{
		Color: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendZero, Operation: BlendAdd},
		Alpha: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendZero, Operation: BlendAdd},
	}
}

// BlendAlpha returns standard non-premultiplied alpha blending (srcAlpha / 1-srcAlpha).
func BlendAlpha() *BlendState {
	return &BlendState{
		Color: BlendComponent{SrcFactor: BlendSrcAlpha, DstFactor: BlendOneMinusSrcAlpha, Operation: BlendAdd},
		Alpha: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendOneMinusSrcAlpha, Operation: BlendAdd},
	}
}

// BlendPremultipliedAlpha returns premultiplied alpha blending (1 / 1-srcAlpha).
func BlendPremultipliedAlpha() *BlendState {
	return &BlendState{
		Color: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendOneMinusSrcAlpha, Operation: BlendAdd},
		Alpha: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendOneMinusSrcAlpha, Operation: BlendAdd},
	}
}

// BlendAdditive returns additive blending (src * srcAlpha + dst).
func BlendAdditive() *BlendState {
	return &BlendState{
		Color: BlendComponent{SrcFactor: BlendSrcAlpha, DstFactor: BlendOne, Operation: BlendAdd},
		Alpha: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendOne, Operation: BlendAdd},
	}
}

// BlendMultiply returns multiplicative blending (src * dst).
func BlendMultiply() *BlendState {
	return &BlendState{
		Color: BlendComponent{SrcFactor: BlendDst, DstFactor: BlendZero, Operation: BlendAdd},
		Alpha: BlendComponent{SrcFactor: BlendDstAlpha, DstFactor: BlendZero, Operation: BlendAdd},
	}
}

// StencilOperation specifies how a stencil buffer value updates after stencil/depth testing.
type StencilOperation uint8

const (
	StencilKeep           StencilOperation = StencilOperation(driver.GPUStencilKeep)
	StencilZero           StencilOperation = StencilOperation(driver.GPUStencilZero)
	StencilReplace        StencilOperation = StencilOperation(driver.GPUStencilReplace)
	StencilInvert         StencilOperation = StencilOperation(driver.GPUStencilInvert)
	StencilIncrementClamp StencilOperation = StencilOperation(driver.GPUStencilIncrementClamp)
	StencilDecrementClamp StencilOperation = StencilOperation(driver.GPUStencilDecrementClamp)
	StencilIncrementWrap  StencilOperation = StencilOperation(driver.GPUStencilIncrementWrap)
	StencilDecrementWrap  StencilOperation = StencilOperation(driver.GPUStencilDecrementWrap)
)

// StencilFaceState configures stencil comparison and update operations for one winding face.
type StencilFaceState struct {
	Compare     CompareFunc      // CompareUndefined defaults to CompareAlways
	FailOp      StencilOperation // applied when the stencil test fails
	DepthFailOp StencilOperation // applied when stencil passes and depth fails
	PassOp      StencilOperation // applied when both stencil and depth pass
}

// PrimitiveState configures primitive assembly and culling.
type PrimitiveState struct {
	Topology  PrimitiveTopology
	FrontFace FrontFace
	CullMode  CullMode
}

// DepthStencilState configures depth testing, stencil testing, writing, and depth bias.
type DepthStencilState struct {
	Format              TextureFormat
	DepthWrite          bool
	DepthCompare        CompareFunc
	StencilFront        StencilFaceState
	StencilBack         StencilFaceState
	StencilReadMask     uint32 // 0 defaults to 0xFF
	StencilWriteMask    uint32 // 0 defaults to 0xFF; use StencilKeep ops to disable writes
	DepthBias           int32
	DepthBiasSlopeScale float32
	DepthBiasClamp      float32
}

// usesStencilTest reports whether s configures stencil testing or stencil writes beyond the pass-through default (always pass, keep, default masks).
func (s *DepthStencilState) usesStencilTest() bool {
	for _, f := range [2]StencilFaceState{s.StencilFront, s.StencilBack} {
		if (f.Compare != CompareUndefined && f.Compare != CompareAlways) || f.FailOp != StencilKeep || f.DepthFailOp != StencilKeep || f.PassOp != StencilKeep {
			return true
		}
	}
	return (s.StencilReadMask != 0 && s.StencilReadMask != 0xFF) || (s.StencilWriteMask != 0 && s.StencilWriteMask != 0xFF)
}

// ColorTargetState configures one color output of a Pipeline.
type ColorTargetState struct {
	Format    TextureFormat
	Blend     *BlendState
	WriteMask ColorWriteMask
}

// ShaderStage names a compiled ShaderModule and its entry point function.
type ShaderStage struct {
	Module     *ShaderModule
	EntryPoint string
}

// PipelineDesc configures Device.CreatePipeline and Device.CreatePipelineAsync.
type PipelineDesc struct {
	Label    string
	Vertex   ShaderStage
	Fragment ShaderStage
	// Layouts holds the bind group layouts indexed by @group(N). A nil entry leaves that slot empty (an empty bind group layout) when the shaders declare no bindings in that group, and SetBindGroup is not required for it.
	Layouts       []*BindGroupLayout
	VertexBuffers []VertexBufferLayout
	Primitive     PrimitiveState
	DepthStencil  *DepthStencilState
	Targets       []ColorTargetState
	SampleCount   int
	Retained      bool // opt in to automatic recreation after device loss
}

// ComputePipelineDesc configures Device.CreateComputePipeline.
type ComputePipelineDesc struct {
	Label   string
	Compute ShaderStage
	// Layouts holds the bind group layouts indexed by @group(N). A nil entry leaves that slot empty (an empty bind group layout) when the compute shader declares no bindings in that group, and SetBindGroup is not required for it.
	Layouts []*BindGroupLayout
}

// LoadOp selects how an attachment is initialized at the start of a RenderPass.
type LoadOp uint8

const (
	LoadClear LoadOp = LoadOp(driver.GPULoadClear)
	LoadLoad  LoadOp = LoadOp(driver.GPULoadLoad)
)

// StoreOp selects whether an attachment's contents are stored at the end of a RenderPass.
type StoreOp uint8

const (
	StoreStore   StoreOp = StoreOp(driver.GPUStoreStore)
	StoreDiscard StoreOp = StoreOp(driver.GPUStoreDiscard)
)

// ColorAttachment configures a color target of a RenderPass.
// Texture == nil targets the view's current swapchain drawable.
type ColorAttachment struct {
	Texture       *Texture
	ResolveTarget *Texture
	MipLevel      uint32
	ArrayLayer    uint32
	LoadOp        LoadOp
	StoreOp       StoreOp
	ClearColor    core.Color
}

// DepthStencilAttachment configures the depth/stencil target of a RenderPass.
// Texture == nil targets the view's automatic swapchain depth buffer.
type DepthStencilAttachment struct {
	Texture        *Texture
	DepthLoadOp    LoadOp
	DepthStoreOp   StoreOp
	ClearDepth     float32
	StencilLoadOp  LoadOp
	StencilStoreOp StoreOp
	ClearStencil   uint32
}

// RenderPassDesc configures Encoder.BeginRenderPass and Frame.BeginRenderPass.
type RenderPassDesc struct {
	// Label names the render pass in GPU capture tools (stripped in release builds).
	Label string
	// TimingLabel opts this pass into per-pass GPU timing queries when Device.Capabilities().TimestampQueries is true.
	TimingLabel string
	// TimestampQuery optionally binds a TimestampQuery resource to receive this pass's measured GPU execution duration.
	TimestampQuery   *TimestampQuery
	ColorAttachments []ColorAttachment
	DepthStencil     *DepthStencilAttachment
}

// PassTiming reports the measured GPU execution duration of one render pass.
type PassTiming struct {
	Label     string
	PassIndex int
	Duration  time.Duration
}

// DebugEvent records a GPU debug label, group, or marker captured by SoftwareRenderer in nsgodev or nsgotest builds.
type DebugEvent struct {
	Kind  string
	Label string
	Depth int
}

// BufferCopy identifies a buffer region for copy commands.
type BufferCopy struct {
	Buffer *Buffer
	Layout DataLayout
}

// TextureCopy identifies a texture subresource and origin for writes and copies.
type TextureCopy struct {
	Texture    *Texture
	MipLevel   uint32
	ArrayLayer uint32
	OriginX    uint32
	OriginY    uint32
}

// TextureWrite identifies a texture subresource (mip level, array layer or cubemap face, and optional sub-rectangle) for Device.WriteTextureRegion.
type TextureWrite struct {
	Texture    *Texture
	MipLevel   uint32
	ArrayLayer uint32
	OriginX    uint32
	OriginY    uint32
	Width      uint32 // 0 defaults to mipWidth - OriginX
	Height     uint32 // 0 defaults to mipHeight - OriginY
}

// DataLayout describes linear pixel data layout in a Go byte slice or Buffer.
type DataLayout struct {
	Offset       uint64
	BytesPerRow  uint32
	RowsPerImage uint32
}

// TextureRegion selects a rectangular region for Device.ReadTexture.
type TextureRegion struct {
	MipLevel   uint32
	ArrayLayer uint32
	X, Y       uint32
	Width      uint32
	Height     uint32
}

// Limits reports the active GPU device's capability limits.
type Limits struct {
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
	MaxColorAttachments           uint32
	MaxTextureArrayLayers         uint32
}

func limitsFromDriver(l driver.GPULimits) Limits {
	if l.MaxTextureDimension2D == 0 {
		l = driver.DefaultGPULimits()
	}
	def := driver.DefaultGPULimits()
	if l.MaxStorageBufferBindingSize == 0 {
		l.MaxStorageBufferBindingSize = def.MaxStorageBufferBindingSize
	}
	if l.MaxComputeWorkgroupSizeX == 0 {
		l.MaxComputeWorkgroupSizeX = def.MaxComputeWorkgroupSizeX
	}
	if l.MaxComputeWorkgroupSizeY == 0 {
		l.MaxComputeWorkgroupSizeY = def.MaxComputeWorkgroupSizeY
	}
	if l.MaxComputeWorkgroupSizeZ == 0 {
		l.MaxComputeWorkgroupSizeZ = def.MaxComputeWorkgroupSizeZ
	}
	if l.MaxComputeInvocationsPerGroup == 0 {
		l.MaxComputeInvocationsPerGroup = def.MaxComputeInvocationsPerGroup
	}
	if l.MaxComputeWorkgroupsPerDim == 0 {
		l.MaxComputeWorkgroupsPerDim = def.MaxComputeWorkgroupsPerDim
	}
	if l.MaxColorAttachments == 0 {
		// Backends that do not report MRT support render to a single color target.
		l.MaxColorAttachments = 1
	}
	if l.MaxTextureArrayLayers == 0 {
		l.MaxTextureArrayLayers = def.MaxTextureArrayLayers
	}
	return Limits{
		MaxTextureDimension2D:         l.MaxTextureDimension2D,
		MaxBufferSize:                 l.MaxBufferSize,
		MaxUniformBufferSize:          l.MaxUniformBufferSize,
		MaxStorageBufferBindingSize:   l.MaxStorageBufferBindingSize,
		MaxBindGroups:                 l.MaxBindGroups,
		MaxVertexBuffers:              l.MaxVertexBuffers,
		MaxVertexAttributes:           l.MaxVertexAttributes,
		MinUniformBufferAlignment:     l.MinUniformBufferAlignment,
		MaxComputeWorkgroupSizeX:      l.MaxComputeWorkgroupSizeX,
		MaxComputeWorkgroupSizeY:      l.MaxComputeWorkgroupSizeY,
		MaxComputeWorkgroupSizeZ:      l.MaxComputeWorkgroupSizeZ,
		MaxComputeInvocationsPerGroup: l.MaxComputeInvocationsPerGroup,
		MaxComputeWorkgroupsPerDim:    l.MaxComputeWorkgroupsPerDim,
		MaxColorAttachments:           l.MaxColorAttachments,
		MaxTextureArrayLayers:         l.MaxTextureArrayLayers,
	}
}

// Features reports optional capabilities supported by the active GPU device.
type Features struct {
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
	// ExtendedFormats reports support for FormatBGRA8UnormSRGB, FormatRG11B10Float, FormatR32Float, FormatRGBA32Float and FormatStencil8 textures and render targets.
	ExtendedFormats bool
	// StencilTest reports support for pipeline stencil testing (DepthStencilState.StencilFront/StencilBack and the stencil read/write masks).
	StencilTest bool
	// MipmapGeneration reports support for Device.GenerateMipmaps. Where it is false, build the chain on the CPU (GenerateMipChainRGBA) and upload each level with WriteTextureMip.
	MipmapGeneration bool
	// ImportD3D11Shared and ImportDMABuf report support for Device.ImportTexture of ExternalD3D11Shared and ExternalDMABuf textures (Device.SupportsExternalTexture).
	ImportD3D11Shared bool
	ImportDMABuf      bool
}

// DeviceCapabilities summarizes the active GPU device's limits, optional features, timing query support, and supported compressed texture formats. Targets without native compute support (WebGL 2, OpenGL ES 3.0) report Features.Compute false; check Device.SupportsCompute and use a CPU or render-pass fallback there.
type DeviceCapabilities struct {
	Limits            Limits
	Features          Features
	TimestampQueries  bool
	CompressedFormats []TextureFormat
}

func featuresFromDriver(f driver.GPUFeatures) Features {
	return Features{
		StorageBuffers:         f.StorageBuffers,
		StorageTextures:        f.StorageTextures,
		Compute:                f.Compute,
		MSAA4x:                 f.MSAA4x,
		Depth32Float:           f.Depth32Float,
		Anisotropy:             f.Anisotropy,
		TimestampQueries:       f.TimestampQueries,
		TextureCompressionBC:   f.TextureCompressionBC,
		TextureCompressionETC2: f.TextureCompressionETC2,
		TextureCompressionASTC: f.TextureCompressionASTC,
		ExtendedFormats:        f.ExtendedFormats,
		StencilTest:            f.StencilTest,
		MipmapGeneration:       f.MipmapGeneration,
		ImportD3D11Shared:      f.ImportD3D11Shared,
		ImportDMABuf:           f.ImportDMABuf,
	}
}

func compressedFormatsFromDriver(f driver.GPUFeatures) []TextureFormat {
	raw := f.SupportedCompressedFormats()
	if len(raw) == 0 {
		return nil
	}
	out := make([]TextureFormat, len(raw))
	for i, fm := range raw {
		out[i] = TextureFormat(fm)
	}
	return out
}

func compressedFormatsFromFeatures(f Features) []TextureFormat {
	return compressedFormatsFromDriver(driver.GPUFeatures{
		TextureCompressionBC:   f.TextureCompressionBC,
		TextureCompressionETC2: f.TextureCompressionETC2,
		TextureCompressionASTC: f.TextureCompressionASTC,
	})
}

// DeviceLostReason explains why a GPU device or context was lost.
type DeviceLostReason uint8

const (
	LostUnknown          DeviceLostReason = DeviceLostReason(driver.GPULostUnknown)
	LostReset            DeviceLostReason = DeviceLostReason(driver.GPULostReset)
	LostDestroyed        DeviceLostReason = DeviceLostReason(driver.GPULostDestroyed)
	LostOutOfMemory      DeviceLostReason = DeviceLostReason(driver.GPULostOutOfMemory)
	LostDeviceRemoved    DeviceLostReason = DeviceLostReason(driver.GPULostDeviceRemoved)
	LostContextLost      DeviceLostReason = DeviceLostReason(driver.GPULostContextLost)
	LostSurfaceDestroyed DeviceLostReason = DeviceLostReason(driver.GPULostSurfaceDestroyed)

	ReasonUnknown          = LostUnknown
	ReasonReset            = LostReset
	ReasonDestroyed        = LostDestroyed
	ReasonOutOfMemory      = LostOutOfMemory
	ReasonDeviceRemoved    = LostDeviceRemoved
	ReasonContextLost      = LostContextLost
	ReasonSurfaceDestroyed = LostSurfaceDestroyed
)

// String returns a human-readable name for r.
func (r DeviceLostReason) String() string {
	return driver.GPUDeviceLostReason(r).String()
}

// DeviceLostEvent is delivered to ViewProps.OnDeviceLost.
type DeviceLostEvent struct {
	Reason  DeviceLostReason
	Message string
}

// ErrorCode classifies a GPU validation or runtime failure.
type ErrorCode uint8

const (
	ErrUnsupported       ErrorCode = ErrorCode(driver.GPUErrorUnsupported)
	ErrInvalidDescriptor ErrorCode = ErrorCode(driver.GPUErrorInvalidDescriptor)
	ErrShaderCompile     ErrorCode = ErrorCode(driver.GPUErrorShaderCompile)
	ErrOutOfMemory       ErrorCode = ErrorCode(driver.GPUErrorOutOfMemory)
	ErrDeviceLost        ErrorCode = ErrorCode(driver.GPUErrorDeviceLost)
	ErrFailed            ErrorCode = ErrorCode(driver.GPUErrorFailed)
)

func (c ErrorCode) Error() string {
	switch c {
	case ErrUnsupported:
		return "gpu: unsupported"
	case ErrInvalidDescriptor:
		return "gpu: invalid descriptor"
	case ErrShaderCompile:
		return "gpu: shader compile failed"
	case ErrOutOfMemory:
		return "gpu: out of memory"
	case ErrDeviceLost:
		return "gpu: device lost"
	default:
		return "gpu: failed"
	}
}

// Error is a structured GPU validation or asynchronous backend error.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code.Error(), e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code.Error(), e.Cause)
	}
	return e.Code.Error()
}

func (e Error) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Code
}

func (e Error) Is(target error) bool {
	if tc, ok := target.(ErrorCode); ok {
		return e.Code == tc
	}
	var te Error
	if errors.As(target, &te) {
		return e.Code == te.Code
	}
	return false
}

// Keep unused stdlib imports cleanly referenced across files.
var (
	_ *image.RGBA
	_ time.Time
)
