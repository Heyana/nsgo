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
	UsageVertex  BufferUsage = BufferUsage(driver.GPUBufferVertex)
	UsageIndex   BufferUsage = BufferUsage(driver.GPUBufferIndex)
	UsageUniform BufferUsage = BufferUsage(driver.GPUBufferUniform)
	UsageStorage BufferUsage = BufferUsage(driver.GPUBufferStorage)
	UsageCopySrc BufferUsage = BufferUsage(driver.GPUBufferCopySrc)
	UsageCopyDst BufferUsage = BufferUsage(driver.GPUBufferCopyDst)
)

// BufferDesc configures Device.CreateBuffer.
type BufferDesc struct {
	Label string
	Size  uint64
	Usage BufferUsage
	Data  []byte // optional initial contents; copied before CreateBuffer returns
}

// TextureDimension is the dimensionality of a Texture.
type TextureDimension uint8

const (
	Dim2D      TextureDimension = TextureDimension(driver.GPUDim2D)
	DimCube    TextureDimension = TextureDimension(driver.GPUDimCube)
	Dim2DArray TextureDimension = TextureDimension(driver.GPUDim2DArray)
)

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
)

// IsDepth reports whether f is a depth or depth/stencil format.
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

// BlockSize returns the compression block dimensions (width, height) in texels.
// Uncompressed formats return (1, 1); FormatUndefined returns (0, 0).
func (f TextureFormat) BlockSize() (width, height uint32) {
	return driver.GPUTextureFormat(f).BlockSize()
}

// BytesPerBlock returns the byte size of one texel block (or one texel for
// uncompressed color formats; 0 for depth or undefined formats).
func (f TextureFormat) BytesPerBlock() uint32 {
	return driver.GPUTextureFormat(f).BytesPerBlock()
}

// ByteSize returns the tightly-packed byte size of a 2D texture level of
// width x height texels in format f.
func (f TextureFormat) ByteSize(width, height uint32) uint64 {
	return driver.GPUTextureFormat(f).ByteSize(width, height)
}

// String returns the format's canonical name.
func (f TextureFormat) String() string {
	return driver.GPUTextureFormat(f).String()
}

// TextureUsage is a bitmask of permitted uses for a Texture.
type TextureUsage uint32

const (
	TextureSampled          TextureUsage = TextureUsage(driver.GPUTextureSampled)
	TextureRenderAttachment TextureUsage = TextureUsage(driver.GPUTextureRenderAttachment)
	TextureCopySrc          TextureUsage = TextureUsage(driver.GPUTextureCopySrc)
	TextureCopyDst          TextureUsage = TextureUsage(driver.GPUTextureCopyDst)
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
	SampleCount int // 0 or 1
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
	Compare       CompareFunc
	MaxAnisotropy uint16
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
	BindingTexture           BindingType = BindingType(driver.GPUBindingTexture)
	BindingSampler           BindingType = BindingType(driver.GPUBindingSampler)
	BindingComparisonSampler BindingType = BindingType(driver.GPUBindingComparisonSampler)
)

// ShaderStageFlags specifies which shader stages see a binding.
type ShaderStageFlags uint8

const (
	StageVertex   ShaderStageFlags = ShaderStageFlags(driver.GPUStageVertex)
	StageFragment ShaderStageFlags = ShaderStageFlags(driver.GPUStageFragment)
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
	Label   string
	Entries []BindGroupLayoutEntry
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
	StepVertex   VertexStepMode = VertexStepMode(driver.GPUStepVertex)
	StepInstance VertexStepMode = VertexStepMode(driver.GPUStepInstance)
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
	FrontCCW FrontFace = FrontFace(driver.GPUFrontCCW)
	FrontCW  FrontFace = FrontFace(driver.GPUFrontCW)
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

const (
	BlendAdd             BlendOperation = BlendOperation(driver.GPUBlendAdd)
	BlendSubtract        BlendOperation = BlendOperation(driver.GPUBlendSubtract)
	BlendReverseSubtract BlendOperation = BlendOperation(driver.GPUBlendReverseSubtract)
	BlendMin             BlendOperation = BlendOperation(driver.GPUBlendMin)
	BlendMax             BlendOperation = BlendOperation(driver.GPUBlendMax)
)

// ColorWriteMask selects which channels a pipeline writes.
type ColorWriteMask uint8

const (
	ColorWriteRed   ColorWriteMask = ColorWriteMask(driver.GPUColorWriteRed)
	ColorWriteGreen ColorWriteMask = ColorWriteMask(driver.GPUColorWriteGreen)
	ColorWriteBlue  ColorWriteMask = ColorWriteMask(driver.GPUColorWriteBlue)
	ColorWriteAlpha ColorWriteMask = ColorWriteMask(driver.GPUColorWriteAlpha)
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

// BlendAlpha returns standard non-premultiplied alpha blending (srcAlpha / 1-srcAlpha).
func BlendAlpha() *BlendState {
	return &BlendState{
		Color: BlendComponent{SrcFactor: BlendSrcAlpha, DstFactor: BlendOneMinusSrcAlpha, Operation: BlendAdd},
		Alpha: BlendComponent{SrcFactor: BlendOne, DstFactor: BlendOneMinusSrcAlpha, Operation: BlendAdd},
	}
}

// PrimitiveState configures primitive assembly and culling.
type PrimitiveState struct {
	Topology  PrimitiveTopology
	FrontFace FrontFace
	CullMode  CullMode
}

// DepthStencilState configures depth testing and writing.
type DepthStencilState struct {
	Format              TextureFormat
	DepthWrite          bool
	DepthCompare        CompareFunc
	DepthBias           int32
	DepthBiasSlopeScale float32
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

// PipelineDesc configures Device.CreatePipeline.
type PipelineDesc struct {
	Label         string
	Vertex        ShaderStage
	Fragment      ShaderStage
	Layouts       []*BindGroupLayout
	VertexBuffers []VertexBufferLayout
	Primitive     PrimitiveState
	DepthStencil  *DepthStencilState
	Targets       []ColorTargetState
	SampleCount   int
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
	// TimingLabel opts this pass into per-pass GPU timing queries when
	// Device.Capabilities().TimestampQueries is true.
	TimingLabel string
	// TimestampQuery optionally binds a TimestampQuery resource to receive this
	// pass's measured GPU execution duration.
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

// DebugEvent records a GPU debug label, group, or marker captured by
// SoftwareRenderer in nsgodev or nsgotest builds.
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
	MaxTextureDimension2D     uint32
	MaxBufferSize             uint64
	MaxUniformBufferSize      uint64
	MaxBindGroups             uint32
	MaxVertexBuffers          uint32
	MaxVertexAttributes       uint32
	MinUniformBufferAlignment uint32
}

func limitsFromDriver(l driver.GPULimits) Limits {
	if l.MaxTextureDimension2D == 0 {
		l = driver.DefaultGPULimits()
	}
	return Limits{
		MaxTextureDimension2D:     l.MaxTextureDimension2D,
		MaxBufferSize:             l.MaxBufferSize,
		MaxUniformBufferSize:      l.MaxUniformBufferSize,
		MaxBindGroups:             l.MaxBindGroups,
		MaxVertexBuffers:          l.MaxVertexBuffers,
		MaxVertexAttributes:       l.MaxVertexAttributes,
		MinUniformBufferAlignment: l.MinUniformBufferAlignment,
	}
}

// Features reports optional capabilities supported by the active GPU device.
type Features struct {
	StorageBuffers         bool
	Compute                bool
	MSAA4x                 bool
	Depth32Float           bool
	Anisotropy             bool
	TimestampQueries       bool
	TextureCompressionBC   bool
	TextureCompressionETC2 bool
	TextureCompressionASTC bool
}

// DeviceCapabilities summarizes the active GPU device's limits, optional
// features, timing query support, and supported compressed texture formats.
type DeviceCapabilities struct {
	Limits            Limits
	Features          Features
	TimestampQueries  bool
	CompressedFormats []TextureFormat
}

func featuresFromDriver(f driver.GPUFeatures) Features {
	return Features{
		StorageBuffers:         f.StorageBuffers,
		Compute:                f.Compute,
		MSAA4x:                 f.MSAA4x,
		Depth32Float:           f.Depth32Float,
		Anisotropy:             f.Anisotropy,
		TimestampQueries:       f.TimestampQueries,
		TextureCompressionBC:   f.TextureCompressionBC,
		TextureCompressionETC2: f.TextureCompressionETC2,
		TextureCompressionASTC: f.TextureCompressionASTC,
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
	LostUnknown     DeviceLostReason = DeviceLostReason(driver.GPULostUnknown)
	LostReset       DeviceLostReason = DeviceLostReason(driver.GPULostReset)
	LostDestroyed   DeviceLostReason = DeviceLostReason(driver.GPULostDestroyed)
	LostOutOfMemory DeviceLostReason = DeviceLostReason(driver.GPULostOutOfMemory)
)

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
