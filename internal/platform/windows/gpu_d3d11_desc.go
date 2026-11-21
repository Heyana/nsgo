package windows

import (
	"errors"
	"fmt"
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
)

// Direct3D 11 and DXGI constants used by the Windows GPU view backend. This file has no OS build constraint so descriptor mapping, constant-buffer alignment, HLSL pipeline compilation, and resource-lifecycle logic can be tested on every host platform.
const (
	// DXGI_FORMAT values (dxgiformat.h).
	dxgiFmtUnknown            uint32 = 0
	dxgiFmtR32G32B32A32Float  uint32 = 2
	dxgiFmtR32G32B32A32Uint   uint32 = 3
	dxgiFmtR32G32B32A32Sint   uint32 = 4
	dxgiFmtR32G32B32Float     uint32 = 6
	dxgiFmtR32G32B32Uint      uint32 = 7
	dxgiFmtR32G32B32Sint      uint32 = 8
	dxgiFmtR16G16B16A16Float  uint32 = 10
	dxgiFmtR32G32Float        uint32 = 16
	dxgiFmtR32G32Uint         uint32 = 17
	dxgiFmtR32G32Sint         uint32 = 18
	dxgiFmtR8G8B8A8Unorm      uint32 = 28
	dxgiFmtR8G8B8A8UnormSRGB  uint32 = 29
	dxgiFmtR32Typeless        uint32 = 39
	dxgiFmtD32Float           uint32 = 40
	dxgiFmtR32Float           uint32 = 41
	dxgiFmtR32Uint            uint32 = 42
	dxgiFmtR32Sint            uint32 = 43
	dxgiFmtR24G8Typeless      uint32 = 44
	dxgiFmtD24UnormS8Uint     uint32 = 45
	dxgiFmtR24UnormX8Typeless uint32 = 46
	dxgiFmtR8G8Unorm          uint32 = 49
	dxgiFmtR16Uint            uint32 = 57
	dxgiFmtR8Unorm            uint32 = 61
	dxgiFmtB8G8R8A8Unorm      uint32 = 87

	// D3D11_USAGE (d3d11.h).
	d3d11UsageDefault   uint32 = 0
	d3d11UsageImmutable uint32 = 1
	d3d11UsageDynamic   uint32 = 2
	d3d11UsageStaging   uint32 = 3

	// D3D11_BIND_FLAG (d3d11.h).
	d3d11BindVertexBuffer   uint32 = 0x1
	d3d11BindIndexBuffer    uint32 = 0x2
	d3d11BindConstantBuffer uint32 = 0x4
	d3d11BindShaderResource uint32 = 0x8
	d3d11BindRenderTarget   uint32 = 0x20
	d3d11BindDepthStencil   uint32 = 0x40

	// D3D11_CPU_ACCESS_FLAG (d3d11.h).
	d3d11CPUAccessWrite uint32 = 0x10000
	d3d11CPUAccessRead  uint32 = 0x20000

	// D3D11_RESOURCE_MISC_FLAG (d3d11.h).
	d3d11MiscTextureCube      uint32 = 0x4
	d3d11MiscBufferStructured uint32 = 0x40

	// D3D11_MAP (d3d11.h).
	d3d11MapRead uint32 = 1

	// D3D11_INPUT_CLASSIFICATION (d3d11.h).
	d3d11InputPerVertexData   uint32 = 0
	d3d11InputPerInstanceData uint32 = 1

	// D3D11_SRV_DIMENSION (d3d11.h).
	d3d11SRVDimBuffer         uint32 = 1
	d3d11SRVDimTexture2D      uint32 = 4
	d3d11SRVDimTexture2DArray uint32 = 5
	d3d11SRVDimTexture2DMS    uint32 = 6
	d3d11SRVDimTextureCube    uint32 = 9

	// D3D11_RTV_DIMENSION (d3d11.h).
	d3d11RTVDimTexture2D      uint32 = 4
	d3d11RTVDimTexture2DArray uint32 = 5
	d3d11RTVDimTexture2DMS    uint32 = 6

	// D3D11_DSV_DIMENSION (d3d11.h).
	d3d11DSVDimTexture2D      uint32 = 3
	d3d11DSVDimTexture2DArray uint32 = 4
	d3d11DSVDimTexture2DMS    uint32 = 5

	// D3D11_TEXTURE_ADDRESS_MODE (d3d11.h).
	d3d11AddressWrap   uint32 = 1
	d3d11AddressMirror uint32 = 2
	d3d11AddressClamp  uint32 = 3

	// D3D11_COMPARISON_FUNC (d3d11.h).
	d3d11CompareNever        uint32 = 1
	d3d11CompareLess         uint32 = 2
	d3d11CompareEqual        uint32 = 3
	d3d11CompareLessEqual    uint32 = 4
	d3d11CompareGreater      uint32 = 5
	d3d11CompareNotEqual     uint32 = 6
	d3d11CompareGreaterEqual uint32 = 7
	d3d11CompareAlways       uint32 = 8

	// D3D11_FILL_MODE (d3d11.h).
	d3d11FillSolid uint32 = 3

	// D3D11_CULL_MODE (d3d11.h).
	d3d11CullNone  uint32 = 1
	d3d11CullFront uint32 = 2
	d3d11CullBack  uint32 = 3

	// D3D11_DEPTH_WRITE_MASK (d3d11.h).
	d3d11DepthWriteMaskZero uint32 = 0
	d3d11DepthWriteMaskAll  uint32 = 1

	// D3D11_STENCIL_OP (d3d11.h).
	d3d11StencilOpKeep uint32 = 1

	// D3D11_BLEND (d3d11.h).
	d3d11BlendZero           uint32 = 1
	d3d11BlendOne            uint32 = 2
	d3d11BlendSrcColor       uint32 = 3
	d3d11BlendInvSrcColor    uint32 = 4
	d3d11BlendSrcAlpha       uint32 = 5
	d3d11BlendInvSrcAlpha    uint32 = 6
	d3d11BlendDestAlpha      uint32 = 7
	d3d11BlendInvDestAlpha   uint32 = 8
	d3d11BlendDestColor      uint32 = 9
	d3d11BlendInvDestColor   uint32 = 10
	d3d11BlendBlendFactor    uint32 = 14
	d3d11BlendInvBlendFactor uint32 = 15

	// D3D11_BLEND_OP (d3d11.h).
	d3d11BlendOpAdd         uint32 = 1
	d3d11BlendOpSubtract    uint32 = 2
	d3d11BlendOpRevSubtract uint32 = 3
	d3d11BlendOpMin         uint32 = 4
	d3d11BlendOpMax         uint32 = 5

	// D3D11_PRIMITIVE_TOPOLOGY (d3dcommon.h).
	d3d11TopologyPointList     uint32 = 1
	d3d11TopologyLineList      uint32 = 2
	d3d11TopologyLineStrip     uint32 = 3
	d3d11TopologyTriangleList  uint32 = 4
	d3d11TopologyTriangleStrip uint32 = 5

	// D3D11_CLEAR_FLAG (d3d11.h).
	d3d11ClearDepth   uint32 = 0x1
	d3d11ClearStencil uint32 = 0x2

	// Constant buffer alignment and limit in Direct3D 11.
	d3d11ConstantBufferAlign   uint64 = 16
	d3d11MaxConstantBufferSize uint64 = 65536

	// Device-removed / out-of-memory HRESULT codes.
	dxgiHRErrDeviceRemoved         uint32 = 0x887A0005
	dxgiHRErrDeviceHung            uint32 = 0x887A0006
	dxgiHRErrDeviceReset           uint32 = 0x887A0007
	dxgiHRErrDriverInternalError   uint32 = 0x887A0020
	d2dHRErrRecreateTarget         uint32 = 0x8899000C
	winHRErrOutOfMemory            uint32 = 0x8007000E
	d3d11MaxSimultaneousRenderTgts        = 8
)

// d3d11BufferDesc matches D3D11_BUFFER_DESC (24 bytes).
type d3d11BufferDesc struct {
	byteWidth           uint32
	usage               uint32
	bindFlags           uint32
	cpuAccessFlags      uint32
	miscFlags           uint32
	structureByteStride uint32
}

// d3d11Texture2DDesc matches D3D11_TEXTURE2D_DESC (44 bytes).
type d3d11Texture2DDesc struct {
	width, height, mipLevels, arraySize, format uint32
	sampleCount, sampleQuality                  uint32
	usage, bindFlags, cpuAccess, misc           uint32
}

// d3d11SRVDesc matches D3D11_SHADER_RESOURCE_VIEW_DESC (24 bytes).
type d3d11SRVDesc struct {
	format         uint32
	viewDimension  uint32
	u0, u1, u2, u3 uint32
}

// d3d11RTVDesc matches D3D11_RENDER_TARGET_VIEW_DESC (20 bytes).
type d3d11RTVDesc struct {
	format        uint32
	viewDimension uint32
	u0, u1, u2    uint32
}

// d3d11DSVDesc matches D3D11_DEPTH_STENCIL_VIEW_DESC (24 bytes).
type d3d11DSVDesc struct {
	format        uint32
	viewDimension uint32
	flags         uint32
	u0, u1, u2    uint32
}

// d3d11SamplerDesc matches D3D11_SAMPLER_DESC (52 bytes).
type d3d11SamplerDesc struct {
	filter         uint32
	addressU       uint32
	addressV       uint32
	addressW       uint32
	mipLODBias     float32
	maxAnisotropy  uint32
	comparisonFunc uint32
	borderColor    [4]float32
	minLOD         float32
	maxLOD         float32
}

// d3d11RasterizerDesc matches D3D11_RASTERIZER_DESC (40 bytes).
type d3d11RasterizerDesc struct {
	fillMode              uint32
	cullMode              uint32
	frontCounterClockwise int32
	depthBias             int32
	depthBiasClamp        float32
	slopeScaledDepthBias  float32
	depthClipEnable       int32
	scissorEnable         int32
	multisampleEnable     int32
	antialiasedLineEnable int32
}

// d3d11DepthStencilOpDesc matches D3D11_DEPTH_STENCILOP_DESC (16 bytes).
type d3d11DepthStencilOpDesc struct {
	stencilFailOp      uint32
	stencilDepthFailOp uint32
	stencilPassOp      uint32
	stencilFunc        uint32
}

// d3d11DepthStencilDesc matches D3D11_DEPTH_STENCIL_DESC (52 bytes).
type d3d11DepthStencilDesc struct {
	depthEnable      int32
	depthWriteMask   uint32
	depthFunc        uint32
	stencilEnable    int32
	stencilReadMask  uint8
	stencilWriteMask uint8
	_                [2]byte
	frontFace        d3d11DepthStencilOpDesc
	backFace         d3d11DepthStencilOpDesc
}

// d3d11RenderTargetBlendDesc matches D3D11_RENDER_TARGET_BLEND_DESC (32 bytes).
type d3d11RenderTargetBlendDesc struct {
	blendEnable           int32
	srcBlend              uint32
	destBlend             uint32
	blendOp               uint32
	srcBlendAlpha         uint32
	destBlendAlpha        uint32
	blendOpAlpha          uint32
	renderTargetWriteMask uint8
	_                     [3]byte
}

// d3d11BlendDesc matches D3D11_BLEND_DESC (264 bytes).
type d3d11BlendDesc struct {
	alphaToCoverageEnable  int32
	independentBlendEnable int32
	renderTarget           [8]d3d11RenderTargetBlendDesc
}

// d3d11Box matches D3D11_BOX (24 bytes).
type d3d11Box struct {
	left, top, front, right, bottom, back uint32
}

// d3d11Viewport matches D3D11_VIEWPORT (24 bytes).
type d3d11Viewport struct {
	x, y, w, h, minDepth, maxDepth float32
}

// d3d11Rect matches D3D11_RECT / Win32 RECT (16 bytes).
type d3d11Rect struct {
	left, top, right, bottom int32
}

// d3dInputElementSpec is the portable specification of a D3D11_INPUT_ELEMENT_DESC.
type d3dInputElementSpec struct {
	semanticName         string
	semanticIndex        uint32
	format               uint32
	inputSlot            uint32
	alignedByteOffset    uint32
	inputSlotClass       uint32
	instanceDataStepRate uint32
}

// d3d11Limits returns Direct3D 11 Feature Level 11_0 device limits.
func d3d11Limits() driver.GPULimits {
	return driver.GPULimits{
		MaxTextureDimension2D:     16384,
		MaxBufferSize:             256 << 20,
		MaxUniformBufferSize:      d3d11MaxConstantBufferSize,
		MaxBindGroups:             4,
		MaxVertexBuffers:          8,
		MaxVertexAttributes:       16,
		MinUniformBufferAlignment: 256,
	}
}

// d3d11Features returns the Stage 1 optional GPU features enabled on D3D11.
func d3d11Features() driver.GPUFeatures {
	return driver.GPUFeatures{
		StorageBuffers: false,
		Compute:        false,
		MSAA4x:         true,
		Depth32Float:   true,
		Anisotropy:     true,
	}
}

// d3dAlignConstantBufferSize rounds size up to a 16-byte multiple (min 16), matching Direct3D 11's D3D11_BIND_CONSTANT_BUFFER ByteWidth requirement.
func d3dAlignConstantBufferSize(size uint64) uint32 {
	if size < d3d11ConstantBufferAlign {
		return uint32(d3d11ConstantBufferAlign)
	}
	aligned := (size + (d3d11ConstantBufferAlign - 1)) &^ (d3d11ConstantBufferAlign - 1)
	if aligned > d3d11MaxConstantBufferSize {
		aligned = d3d11MaxConstantBufferSize
	}
	return uint32(aligned)
}

// d3dBufferDescriptor maps a driver.GPUBufferDesc to one or two D3D11_BUFFER_DESC structs. Direct3D 11 forbids combining D3D11_BIND_CONSTANT_BUFFER with any other bind flag, so when Usage includes Uniform alongside Vertex/Index/Storage, a separate 16-byte-aligned constant-buffer descriptor is returned in cbDesc.
func d3dBufferDescriptor(desc driver.GPUBufferDesc) (mainDesc d3d11BufferDesc, hasCB bool, cbDesc d3d11BufferDesc) {
	size := desc.Size
	if size == 0 {
		size = uint64(len(desc.Data))
	}
	if size == 0 {
		size = 16
	}
	var nonCBBind uint32
	if desc.Usage&driver.GPUBufferVertex != 0 {
		nonCBBind |= d3d11BindVertexBuffer
	}
	if desc.Usage&driver.GPUBufferIndex != 0 {
		nonCBBind |= d3d11BindIndexBuffer
	}
	var misc, stride uint32
	if desc.Usage&driver.GPUBufferStorage != 0 {
		nonCBBind |= d3d11BindShaderResource
		misc |= d3d11MiscBufferStructured
		stride = 4
		size = (size + 3) &^ 3
	}
	isUniform := desc.Usage&driver.GPUBufferUniform != 0
	if isUniform {
		cbDesc = d3d11BufferDesc{
			byteWidth: d3dAlignConstantBufferSize(size),
			usage:     d3d11UsageDefault,
			bindFlags: d3d11BindConstantBuffer,
		}
		if nonCBBind == 0 {
			return cbDesc, true, cbDesc
		}
		// Combined Uniform + other bind flags: create both main and shadow CB.
		mainDesc = d3d11BufferDesc{
			byteWidth:           uint32(size),
			usage:               d3d11UsageDefault,
			bindFlags:           nonCBBind,
			miscFlags:           misc,
			structureByteStride: stride,
		}
		return mainDesc, true, cbDesc
	}
	// Copy-only buffers still need a valid bind flag or staging usage in D3D11; vertex buffer is accepted universally for D3D11_USAGE_DEFAULT copy buffers.
	if nonCBBind == 0 {
		nonCBBind = d3d11BindVertexBuffer
	}
	mainDesc = d3d11BufferDesc{
		byteWidth:           uint32(size),
		usage:               d3d11UsageDefault,
		bindFlags:           nonCBBind,
		miscFlags:           misc,
		structureByteStride: stride,
	}
	return mainDesc, false, d3d11BufferDesc{}
}

// d3dTextureFormats maps a driver.GPUTextureFormat to DXGI formats for the underlying Texture2D resource and its SRV, RTV, and DSV views. Depth formats that are also sampled use a typeless resource format so both DSV and SRV can be created on the same ID3D11Texture2D.
func d3dTextureFormats(f driver.GPUTextureFormat, sampled bool) (texFmt, srvFmt, rtvFmt, dsvFmt uint32) {
	switch f {
	case driver.GPUFormatRGBA8Unorm:
		return dxgiFmtR8G8B8A8Unorm, dxgiFmtR8G8B8A8Unorm, dxgiFmtR8G8B8A8Unorm, dxgiFmtUnknown
	case driver.GPUFormatRGBA8UnormSRGB:
		return dxgiFmtR8G8B8A8UnormSRGB, dxgiFmtR8G8B8A8UnormSRGB, dxgiFmtR8G8B8A8UnormSRGB, dxgiFmtUnknown
	case driver.GPUFormatBGRA8Unorm:
		return dxgiFmtB8G8R8A8Unorm, dxgiFmtB8G8R8A8Unorm, dxgiFmtB8G8R8A8Unorm, dxgiFmtUnknown
	case driver.GPUFormatRG8Unorm:
		return dxgiFmtR8G8Unorm, dxgiFmtR8G8Unorm, dxgiFmtR8G8Unorm, dxgiFmtUnknown
	case driver.GPUFormatR8Unorm:
		return dxgiFmtR8Unorm, dxgiFmtR8Unorm, dxgiFmtR8Unorm, dxgiFmtUnknown
	case driver.GPUFormatRGBA16Float:
		return dxgiFmtR16G16B16A16Float, dxgiFmtR16G16B16A16Float, dxgiFmtR16G16B16A16Float, dxgiFmtUnknown
	case driver.GPUFormatDepth32Float, driver.GPUFormatDepth24Plus:
		if sampled {
			return dxgiFmtR32Typeless, dxgiFmtR32Float, dxgiFmtUnknown, dxgiFmtD32Float
		}
		return dxgiFmtD32Float, dxgiFmtUnknown, dxgiFmtUnknown, dxgiFmtD32Float
	case driver.GPUFormatDepth24PlusStencil8:
		if sampled {
			return dxgiFmtR24G8Typeless, dxgiFmtR24UnormX8Typeless, dxgiFmtUnknown, dxgiFmtD24UnormS8Uint
		}
		return dxgiFmtD24UnormS8Uint, dxgiFmtUnknown, dxgiFmtUnknown, dxgiFmtD24UnormS8Uint
	default:
		return dxgiFmtR8G8B8A8Unorm, dxgiFmtR8G8B8A8Unorm, dxgiFmtR8G8B8A8Unorm, dxgiFmtUnknown
	}
}

// d3dTexture2DDescriptor builds D3D11_TEXTURE2D_DESC plus optional SRV, RTV, and DSV view descriptors for a driver.GPUTextureDesc.
func d3dTexture2DDescriptor(desc driver.GPUTextureDesc) (d3d11Texture2DDesc, d3d11SRVDesc, d3d11RTVDesc, d3d11DSVDesc) {
	mipLevels := max(desc.MipLevels, 1)
	arraySize := max(desc.Layers, 1)
	if desc.Dimension == driver.GPUDimCube && arraySize < 6 {
		arraySize = 6
	}
	samples := uint32(max(desc.SampleCount, 1))
	sampled := desc.Usage&driver.GPUTextureSampled != 0
	isDepth := desc.Format.IsDepth()

	texFmt, srvFmt, rtvFmt, dsvFmt := d3dTextureFormats(desc.Format, sampled)

	var bindFlags, misc uint32
	if sampled {
		bindFlags |= d3d11BindShaderResource
	}
	if desc.Usage&driver.GPUTextureRenderAttachment != 0 {
		if isDepth {
			bindFlags |= d3d11BindDepthStencil
		} else {
			bindFlags |= d3d11BindRenderTarget
		}
	}
	if desc.Dimension == driver.GPUDimCube {
		misc |= d3d11MiscTextureCube
	}

	td := d3d11Texture2DDesc{
		width:       max(desc.Width, 1),
		height:      max(desc.Height, 1),
		mipLevels:   mipLevels,
		arraySize:   arraySize,
		format:      texFmt,
		sampleCount: samples,
		usage:       d3d11UsageDefault,
		bindFlags:   bindFlags,
		misc:        misc,
	}

	var srv d3d11SRVDesc
	if sampled && srvFmt != dxgiFmtUnknown {
		srv.format = srvFmt
		switch {
		case samples > 1:
			srv.viewDimension = d3d11SRVDimTexture2DMS
		case desc.Dimension == driver.GPUDimCube:
			srv.viewDimension = d3d11SRVDimTextureCube
			srv.u0 = 0
			srv.u1 = mipLevels
		case desc.Dimension == driver.GPUDim2DArray || arraySize > 1:
			srv.viewDimension = d3d11SRVDimTexture2DArray
			srv.u0 = 0
			srv.u1 = mipLevels
			srv.u2 = 0
			srv.u3 = arraySize
		default:
			srv.viewDimension = d3d11SRVDimTexture2D
			srv.u0 = 0
			srv.u1 = mipLevels
		}
	}

	var rtv d3d11RTVDesc
	if desc.Usage&driver.GPUTextureRenderAttachment != 0 && !isDepth && rtvFmt != dxgiFmtUnknown {
		rtv = d3dRTVDescriptorForSlice(rtvFmt, samples, arraySize, 0, 0)
	}

	var dsv d3d11DSVDesc
	if desc.Usage&driver.GPUTextureRenderAttachment != 0 && isDepth && dsvFmt != dxgiFmtUnknown {
		dsv.format = dsvFmt
		switch {
		case samples > 1:
			dsv.viewDimension = d3d11DSVDimTexture2DMS
		case arraySize > 1:
			dsv.viewDimension = d3d11DSVDimTexture2DArray
			dsv.u0 = 0
			dsv.u1 = 0
			dsv.u2 = 1
		default:
			dsv.viewDimension = d3d11DSVDimTexture2D
			dsv.u0 = 0
		}
	}

	return td, srv, rtv, dsv
}

// d3dRTVDescriptorForSlice builds a D3D11_RENDER_TARGET_VIEW_DESC for a given mip level and array slice.
func d3dRTVDescriptorForSlice(rtvFmt, samples, arraySize, mipLevel, arrayLayer uint32) d3d11RTVDesc {
	rtv := d3d11RTVDesc{format: rtvFmt}
	switch {
	case samples > 1:
		rtv.viewDimension = d3d11RTVDimTexture2DMS
	case arraySize > 1:
		rtv.viewDimension = d3d11RTVDimTexture2DArray
		rtv.u0 = mipLevel
		rtv.u1 = arrayLayer
		rtv.u2 = 1
	default:
		rtv.viewDimension = d3d11RTVDimTexture2D
		rtv.u0 = mipLevel
	}
	return rtv
}

// d3dVertexFormatDXGI maps a GPUVertexFormat to DXGI_FORMAT.
func d3dVertexFormatDXGI(f driver.GPUVertexFormat) uint32 {
	switch f {
	case driver.GPUVertexFloat32:
		return dxgiFmtR32Float
	case driver.GPUVertexFloat32x2:
		return dxgiFmtR32G32Float
	case driver.GPUVertexFloat32x3:
		return dxgiFmtR32G32B32Float
	case driver.GPUVertexFloat32x4:
		return dxgiFmtR32G32B32A32Float
	case driver.GPUVertexSint32:
		return dxgiFmtR32Sint
	case driver.GPUVertexSint32x2:
		return dxgiFmtR32G32Sint
	case driver.GPUVertexSint32x3:
		return dxgiFmtR32G32B32Sint
	case driver.GPUVertexSint32x4:
		return dxgiFmtR32G32B32A32Sint
	case driver.GPUVertexUint32:
		return dxgiFmtR32Uint
	case driver.GPUVertexUint32x2:
		return dxgiFmtR32G32Uint
	case driver.GPUVertexUint32x3:
		return dxgiFmtR32G32B32Uint
	case driver.GPUVertexUint32x4:
		return dxgiFmtR32G32B32A32Uint
	case driver.GPUVertexUnorm8x4:
		return dxgiFmtR8G8B8A8Unorm
	default:
		return dxgiFmtUnknown
	}
}

// d3dBuildInputElements converts pipeline vertex buffer layouts into D3D11 input-element specs (matching hlsl.CompilePipeline's TEXCOORD<Location> vertex input semantics) and per-slot vertex buffer strides.
func d3dBuildInputElements(layouts []driver.GPUVertexBufferLayout) ([]d3dInputElementSpec, []uint32) {
	strides := make([]uint32, len(layouts))
	var elements []d3dInputElementSpec
	for slot, vb := range layouts {
		strides[slot] = uint32(vb.Stride)
		slotClass := d3d11InputPerVertexData
		stepRate := uint32(0)
		if vb.StepMode == driver.GPUStepInstance {
			slotClass = d3d11InputPerInstanceData
			stepRate = 1
		}
		for _, attr := range vb.Attributes {
			elements = append(elements, d3dInputElementSpec{
				semanticName:         "TEXCOORD",
				semanticIndex:        attr.Location,
				format:               d3dVertexFormatDXGI(attr.Format),
				inputSlot:            uint32(slot),
				alignedByteOffset:    uint32(attr.Offset),
				inputSlotClass:       slotClass,
				instanceDataStepRate: stepRate,
			})
		}
	}
	return elements, strides
}

// d3dAddressMode maps a GPUAddressMode to D3D11_TEXTURE_ADDRESS_MODE.
func d3dAddressMode(m driver.GPUAddressMode) uint32 {
	switch m {
	case driver.GPUAddressRepeat:
		return d3d11AddressWrap
	case driver.GPUAddressMirrorRepeat:
		return d3d11AddressMirror
	default:
		return d3d11AddressClamp
	}
}

// d3dCompareFunc maps a GPUCompareFunc to D3D11_COMPARISON_FUNC.
func d3dCompareFunc(c driver.GPUCompareFunc) uint32 {
	switch c {
	case driver.GPUCompareNever:
		return d3d11CompareNever
	case driver.GPUCompareLess:
		return d3d11CompareLess
	case driver.GPUCompareEqual:
		return d3d11CompareEqual
	case driver.GPUCompareLessEqual:
		return d3d11CompareLessEqual
	case driver.GPUCompareGreater:
		return d3d11CompareGreater
	case driver.GPUCompareNotEqual:
		return d3d11CompareNotEqual
	case driver.GPUCompareGreaterEqual:
		return d3d11CompareGreaterEqual
	case driver.GPUCompareAlways:
		return d3d11CompareAlways
	default:
		return d3d11CompareNever
	}
}

// d3dSamplerDescriptor builds a D3D11_SAMPLER_DESC from driver.GPUSamplerDesc.
func d3dSamplerDescriptor(desc driver.GPUSamplerDesc) d3d11SamplerDesc {
	var filter uint32
	maxAniso := uint32(desc.MaxAnisotropy)
	if maxAniso > 1 {
		filter = 0x55 // D3D11_FILTER_ANISOTROPIC
		if maxAniso > 16 {
			maxAniso = 16
		}
	} else {
		maxAniso = 1
		if desc.MinFilter == driver.GPUFilterLinear {
			filter |= 0x10
		}
		if desc.MagFilter == driver.GPUFilterLinear {
			filter |= 0x04
		}
		if desc.MipmapFilter == driver.GPUFilterLinear {
			filter |= 0x01
		}
	}
	cmp := d3d11CompareNever
	if desc.Compare != driver.GPUCompareUndefined {
		filter |= 0x80 // D3D11_FILTER_COMPARISON_*
		cmp = d3dCompareFunc(desc.Compare)
	}
	return d3d11SamplerDesc{
		filter:         filter,
		addressU:       d3dAddressMode(desc.AddressU),
		addressV:       d3dAddressMode(desc.AddressV),
		addressW:       d3dAddressMode(desc.AddressW),
		maxAnisotropy:  maxAniso,
		comparisonFunc: cmp,
		minLOD:         0,
		maxLOD:         math.MaxFloat32,
	}
}

// d3dRasterizerDescriptor builds a D3D11_RASTERIZER_DESC. ScissorEnable is always enabled and each render pass initializes the scissor rect to the full render target so SetScissor works without recreating rasterizer state.
func d3dRasterizerDescriptor(prim driver.GPUPrimitiveState, ds *driver.GPUDepthStencilState, sampleCount int) d3d11RasterizerDesc {
	cull := d3d11CullNone
	switch prim.CullMode {
	case driver.GPUCullFront:
		cull = d3d11CullFront
	case driver.GPUCullBack:
		cull = d3d11CullBack
	}
	frontCCW := int32(1)
	if prim.FrontFace == driver.GPUFrontCW {
		frontCCW = 0
	}
	var depthBias int32
	var slopeBias float32
	if ds != nil {
		depthBias = ds.DepthBias
		slopeBias = ds.DepthBiasSlopeScale
	}
	msaa := int32(0)
	if sampleCount > 1 {
		msaa = 1
	}
	return d3d11RasterizerDesc{
		fillMode:              d3d11FillSolid,
		cullMode:              cull,
		frontCounterClockwise: frontCCW,
		depthBias:             depthBias,
		slopeScaledDepthBias:  slopeBias,
		depthClipEnable:       1,
		scissorEnable:         1,
		multisampleEnable:     msaa,
	}
}

// d3dDepthStencilDescriptor builds a D3D11_DEPTH_STENCIL_DESC.
func d3dDepthStencilDescriptor(ds *driver.GPUDepthStencilState) d3d11DepthStencilDesc {
	op := d3d11DepthStencilOpDesc{
		stencilFailOp:      d3d11StencilOpKeep,
		stencilDepthFailOp: d3d11StencilOpKeep,
		stencilPassOp:      d3d11StencilOpKeep,
		stencilFunc:        d3d11CompareAlways,
	}
	if ds == nil {
		return d3d11DepthStencilDesc{
			depthEnable:      0,
			depthWriteMask:   d3d11DepthWriteMaskZero,
			depthFunc:        d3d11CompareAlways,
			stencilReadMask:  0xff,
			stencilWriteMask: 0xff,
			frontFace:        op,
			backFace:         op,
		}
	}
	writeMask := d3d11DepthWriteMaskZero
	if ds.DepthWrite {
		writeMask = d3d11DepthWriteMaskAll
	}
	cmp := d3dCompareFunc(ds.DepthCompare)
	if ds.DepthCompare == driver.GPUCompareUndefined {
		cmp = d3d11CompareLess
	}
	return d3d11DepthStencilDesc{
		depthEnable:      1,
		depthWriteMask:   writeMask,
		depthFunc:        cmp,
		stencilReadMask:  0xff,
		stencilWriteMask: 0xff,
		frontFace:        op,
		backFace:         op,
	}
}

// d3dBlendFactor maps a GPUBlendFactor to D3D11_BLEND. When alphaChannel is true, color-only factors are remapped to their alpha equivalents because Direct3D 11 rejects *_COLOR factors for SrcBlendAlpha/DestBlendAlpha.
func d3dBlendFactor(f driver.GPUBlendFactor, alphaChannel bool) uint32 {
	switch f {
	case driver.GPUBlendZero:
		return d3d11BlendZero
	case driver.GPUBlendOne:
		return d3d11BlendOne
	case driver.GPUBlendSrc:
		if alphaChannel {
			return d3d11BlendSrcAlpha
		}
		return d3d11BlendSrcColor
	case driver.GPUBlendOneMinusSrc:
		if alphaChannel {
			return d3d11BlendInvSrcAlpha
		}
		return d3d11BlendInvSrcColor
	case driver.GPUBlendSrcAlpha:
		return d3d11BlendSrcAlpha
	case driver.GPUBlendOneMinusSrcAlpha:
		return d3d11BlendInvSrcAlpha
	case driver.GPUBlendDst:
		if alphaChannel {
			return d3d11BlendDestAlpha
		}
		return d3d11BlendDestColor
	case driver.GPUBlendOneMinusDst:
		if alphaChannel {
			return d3d11BlendInvDestAlpha
		}
		return d3d11BlendInvDestColor
	case driver.GPUBlendDstAlpha:
		return d3d11BlendDestAlpha
	case driver.GPUBlendOneMinusDstAlpha:
		return d3d11BlendInvDestAlpha
	case driver.GPUBlendConstant:
		return d3d11BlendBlendFactor
	case driver.GPUBlendOneMinusConstant:
		return d3d11BlendInvBlendFactor
	default:
		return d3d11BlendOne
	}
}

// d3dBlendOp maps a GPUBlendOperation to D3D11_BLEND_OP.
func d3dBlendOp(op driver.GPUBlendOperation) uint32 {
	switch op {
	case driver.GPUBlendSubtract:
		return d3d11BlendOpSubtract
	case driver.GPUBlendReverseSubtract:
		return d3d11BlendOpRevSubtract
	case driver.GPUBlendMin:
		return d3d11BlendOpMin
	case driver.GPUBlendMax:
		return d3d11BlendOpMax
	default:
		return d3d11BlendOpAdd
	}
}

// d3dBlendDescriptor builds a D3D11_BLEND_DESC for pipeline color targets.
func d3dBlendDescriptor(targets []driver.GPUColorTargetState) d3d11BlendDesc {
	var bd d3d11BlendDesc
	if len(targets) > 1 {
		bd.independentBlendEnable = 1
	}
	for i := range bd.renderTarget {
		bd.renderTarget[i] = d3d11RenderTargetBlendDesc{
			srcBlend:              d3d11BlendOne,
			destBlend:             d3d11BlendZero,
			blendOp:               d3d11BlendOpAdd,
			srcBlendAlpha:         d3d11BlendOne,
			destBlendAlpha:        d3d11BlendZero,
			blendOpAlpha:          d3d11BlendOpAdd,
			renderTargetWriteMask: uint8(driver.GPUColorWriteAll),
		}
	}
	for i, t := range targets {
		if i >= len(bd.renderTarget) {
			break
		}
		mask := t.WriteMask
		if mask == 0 {
			mask = driver.GPUColorWriteAll
		}
		// GPUColorWriteNone is a bit outside the channel mask; keep only the channel bits so it disables all writes.
		mask &= driver.GPUColorWriteAll
		rt := d3d11RenderTargetBlendDesc{
			srcBlend:              d3d11BlendOne,
			destBlend:             d3d11BlendZero,
			blendOp:               d3d11BlendOpAdd,
			srcBlendAlpha:         d3d11BlendOne,
			destBlendAlpha:        d3d11BlendZero,
			blendOpAlpha:          d3d11BlendOpAdd,
			renderTargetWriteMask: uint8(mask),
		}
		if t.Blend != nil {
			rt.blendEnable = 1
			rt.srcBlend = d3dBlendFactor(t.Blend.Color.SrcFactor, false)
			rt.destBlend = d3dBlendFactor(t.Blend.Color.DstFactor, false)
			rt.blendOp = d3dBlendOp(t.Blend.Color.Operation)
			rt.srcBlendAlpha = d3dBlendFactor(t.Blend.Alpha.SrcFactor, true)
			rt.destBlendAlpha = d3dBlendFactor(t.Blend.Alpha.DstFactor, true)
			rt.blendOpAlpha = d3dBlendOp(t.Blend.Alpha.Operation)
		}
		bd.renderTarget[i] = rt
	}
	return bd
}

// d3dPrimitiveTopology maps a GPUPrimitiveTopology to D3D11_PRIMITIVE_TOPOLOGY.
func d3dPrimitiveTopology(t driver.GPUPrimitiveTopology) uint32 {
	switch t {
	case driver.GPUTopologyTriangleStrip:
		return d3d11TopologyTriangleStrip
	case driver.GPUTopologyLineList:
		return d3d11TopologyLineList
	case driver.GPUTopologyLineStrip:
		return d3d11TopologyLineStrip
	case driver.GPUTopologyPointList:
		return d3d11TopologyPointList
	default:
		return d3d11TopologyTriangleList
	}
}

// d3dIndexFormatDXGI maps a GPUIndexFormat to DXGI_FORMAT.
func d3dIndexFormatDXGI(f driver.GPUIndexFormat) uint32 {
	if f == driver.GPUIndexUint32 {
		return dxgiFmtR32Uint
	}
	return dxgiFmtR16Uint
}

// d3dCalcSubresource matches D3D11CalcSubresource(MipSlice, ArraySlice, MipLevels).
func d3dCalcSubresource(mipSlice, arraySlice, mipLevels uint32) uint32 {
	return mipSlice + arraySlice*max(mipLevels, 1)
}

// d3dSwizzleBGRAtoRGBA copies a width x height region from a BGRA8Unorm buffer with row pitch srcPitch into a tightly packed RGBA8Unorm buffer dst.
func d3dSwizzleBGRAtoRGBA(dst, src []byte, width, height, srcPitch uint32) {
	rowBytes := int(width) * 4
	for y := uint32(0); y < height; y++ {
		sOff := int(y * srcPitch)
		dOff := int(y) * rowBytes
		for x := uint32(0); x < width; x++ {
			si := sOff + int(x)*4
			di := dOff + int(x)*4
			dst[di+0] = src[si+2] // R <- B-offset+2
			dst[di+1] = src[si+1] // G
			dst[di+2] = src[si+0] // B <- B-offset+0
			dst[di+3] = src[si+3] // A
		}
	}
}

// d3dCopyPitchedRows copies a width x height region with bpp bytes per pixel from src (stride srcPitch) into tightly packed dst.
func d3dCopyPitchedRows(dst, src []byte, width, height, bpp, srcPitch uint32) {
	rowBytes := int(width * bpp)
	for y := uint32(0); y < height; y++ {
		sOff := int(y * srcPitch)
		dOff := int(y) * rowBytes
		copy(dst[dOff:dOff+rowBytes], src[sOff:sOff+rowBytes])
	}
}

// d3dIsDeviceLostHR reports whether hr indicates Direct3D device removal or reset.
func d3dIsDeviceLostHR(hr uint32) bool {
	switch hr {
	case dxgiHRErrDeviceRemoved, dxgiHRErrDeviceHung, dxgiHRErrDeviceReset,
		dxgiHRErrDriverInternalError, d2dHRErrRecreateTarget:
		return true
	}
	return false
}

// d3dLostReasonFromHR classifies a device-loss HRESULT into a GPUDeviceLostReason.
func d3dLostReasonFromHR(hr uint32) driver.GPUDeviceLostReason {
	switch hr {
	case dxgiHRErrDeviceRemoved, dxgiHRErrDeviceHung, dxgiHRErrDeviceReset,
		dxgiHRErrDriverInternalError, d2dHRErrRecreateTarget:
		return driver.GPULostReset
	case winHRErrOutOfMemory:
		return driver.GPULostOutOfMemory
	default:
		return driver.GPULostUnknown
	}
}

// d3dCompiledHLSL holds the HLSL SM 5.0 sources, entry points, and register bindings produced from a GPUPipelineDesc via hlsl.CompilePipeline.
type d3dCompiledHLSL struct {
	VertexSource string
	VertexEntry  string
	PixelSource  string
	PixelEntry   string
	VSRegisters  []hlsl.RegisterBinding
	PSRegisters  []hlsl.RegisterBinding
}

// d3dCompilePipelineHLSL compiles a pipeline's vertex and fragment shader IRs into HLSL SM 5.0 sources and per-stage register tables.
func d3dCompilePipelineHLSL(desc *driver.GPUPipelineDesc) (d3dCompiledHLSL, error) {
	if desc == nil || desc.Vertex.IR == nil {
		return d3dCompiledHLSL{}, errors.New("d3d11: pipeline missing vertex shader IR")
	}
	vsOut, err := hlsl.CompilePipeline(desc.Vertex.IR)
	if err != nil {
		return d3dCompiledHLSL{}, err
	}
	if vsOut.VertexSource == "" {
		return d3dCompiledHLSL{}, errors.New("d3d11: vertex shader IR produced empty HLSL")
	}
	out := d3dCompiledHLSL{
		VertexSource: vsOut.VertexSource,
		VertexEntry:  vsOut.VertexEntry,
		VSRegisters:  vsOut.Registers,
	}
	if desc.Vertex.EntryPoint != "" && desc.Vertex.IR.VertexEntry == desc.Vertex.EntryPoint {
		out.VertexEntry = desc.Vertex.EntryPoint
	}
	if desc.Fragment.IR == nil {
		return out, nil
	}
	if desc.Fragment.IR == desc.Vertex.IR {
		out.PixelSource = vsOut.PixelSource
		out.PixelEntry = vsOut.PixelEntry
		out.PSRegisters = vsOut.Registers
	} else {
		psOut, err := hlsl.CompilePipeline(desc.Fragment.IR)
		if err != nil {
			return d3dCompiledHLSL{}, err
		}
		out.PixelSource = psOut.PixelSource
		out.PixelEntry = psOut.PixelEntry
		out.PSRegisters = psOut.Registers
	}
	if desc.Fragment.EntryPoint != "" && desc.Fragment.IR.FragmentEntry == desc.Fragment.EntryPoint {
		out.PixelEntry = desc.Fragment.EntryPoint
	}
	return out, nil
}

// --- Portable GPU resource lifecycle and device-loss state machine ---

// d3dReleaser is implemented by every retained native D3D11 resource wrapper so the lifecycle controller can release all COM objects deterministically on destroy, device loss, or unmount.
type d3dReleaser interface {
	Release()
}

type d3dTrackedBuffer struct {
	id         driver.GPUID
	desc       driver.GPUBufferDesc
	mainDesc   d3d11BufferDesc
	hasCB      bool
	cbDesc     d3d11BufferDesc
	cbShadow   []byte
	handle     d3dReleaser
	hasInitErr bool
}

type d3dTrackedTexture struct {
	id      driver.GPUID
	desc    driver.GPUTextureDesc
	texDesc d3d11Texture2DDesc
	srvDesc d3d11SRVDesc
	rtvDesc d3d11RTVDesc
	dsvDesc d3d11DSVDesc
	handle  d3dReleaser
}

type d3dTrackedSampler struct {
	id      driver.GPUID
	desc    driver.GPUSamplerDesc
	d3dDesc d3d11SamplerDesc
	handle  d3dReleaser
}

type d3dTrackedBindGroup struct {
	id      driver.GPUID
	layout  driver.GPUID
	entries map[uint32]driver.GPUBindGroupEntry
}

type d3dTrackedPipeline struct {
	id        driver.GPUID
	desc      driver.GPUPipelineDesc
	elements  []d3dInputElementSpec
	vbStrides []uint32
	topology  uint32
	vsRegs    []hlsl.RegisterBinding
	psRegs    []hlsl.RegisterBinding
	handle    d3dReleaser
}

// d3dPassLookup exposes tracked resources to the native ExecutePass callback.
type d3dPassLookup struct {
	buffers    map[driver.GPUID]*d3dTrackedBuffer
	textures   map[driver.GPUID]*d3dTrackedTexture
	samplers   map[driver.GPUID]*d3dTrackedSampler
	bindGroups map[driver.GPUID]*d3dTrackedBindGroup
	pipelines  map[driver.GPUID]*d3dTrackedPipeline
}

// d3dNativeOps abstracts the Windows COM calls so d3dGPUController can run identically on real Direct3D 11 (gpu.go) and on pure-Go host unit tests.
type d3dNativeOps interface {
	CreateSwapTargets(w, h uint32, cfg driver.GPUViewConfig) (d3dReleaser, uint32, error)
	CreateBuffer(mainDesc d3d11BufferDesc, hasCB bool, cbDesc d3d11BufferDesc, initData, cbShadow []byte) (d3dReleaser, uint32, error)
	WriteBuffer(buf *d3dTrackedBuffer, offset uint64, data []byte) (uint32, error)
	CreateTexture(desc driver.GPUTextureDesc, td d3d11Texture2DDesc, srv d3d11SRVDesc, rtv d3d11RTVDesc, dsv d3d11DSVDesc) (d3dReleaser, uint32, error)
	WriteTexture(tex *d3dTrackedTexture, dst driver.GPUTextureCopy, data []byte, layout driver.GPUDataLayout) (uint32, error)
	CreateSampler(desc d3d11SamplerDesc) (d3dReleaser, uint32, error)
	CreatePipeline(desc *driver.GPUPipelineDesc, compiled d3dCompiledHLSL, elements []d3dInputElementSpec, rs d3d11RasterizerDesc, dss d3d11DepthStencilDesc, bs d3d11BlendDesc) (d3dReleaser, driver.GPUErrorCode, uint32, error)
	ExecuteCopy(cp driver.GPUCopyOp, lookup *d3dPassLookup) (uint32, error)
	ExecutePass(pass driver.GPUPass, swap d3dReleaser, swapW, swapH uint32, lookup *d3dPassLookup) (drewSwap bool, hr uint32, err error)
	ReadTexture(swap d3dReleaser, swapW, swapH uint32, tex *d3dTrackedTexture, region driver.GPUTextureRegion, surface bool) ([]byte, uint32, error)
	DeviceRemovedReason() uint32
}

// d3dGPUController manages one KindGPU view's swapchain, retained resources, generation tracking, device-loss transitions, and GPUEvent emission.
type d3dGPUController struct {
	ops  d3dNativeOps
	emit func(*driver.GPUEvent)

	cfg        driver.GPUViewConfig
	cfgDirty   bool
	pw, ph     uint32
	generation uint32
	ready      bool
	lost       bool
	disposed   bool

	swap       d3dReleaser
	buffers    map[driver.GPUID]*d3dTrackedBuffer
	textures   map[driver.GPUID]*d3dTrackedTexture
	samplers   map[driver.GPUID]*d3dTrackedSampler
	layouts    map[driver.GPUID]*driver.GPUBindGroupLayoutDesc
	bindGroups map[driver.GPUID]*d3dTrackedBindGroup
	pipelines  map[driver.GPUID]*d3dTrackedPipeline
}

func newD3DGPUController(ops d3dNativeOps, emit func(*driver.GPUEvent)) *d3dGPUController {
	return &d3dGPUController{
		ops:        ops,
		emit:       emit,
		buffers:    map[driver.GPUID]*d3dTrackedBuffer{},
		textures:   map[driver.GPUID]*d3dTrackedTexture{},
		samplers:   map[driver.GPUID]*d3dTrackedSampler{},
		layouts:    map[driver.GPUID]*driver.GPUBindGroupLayoutDesc{},
		bindGroups: map[driver.GPUID]*d3dTrackedBindGroup{},
		pipelines:  map[driver.GPUID]*d3dTrackedPipeline{},
	}
}

// Configure sets the view's swapchain format/depth/MSAA configuration.
func (c *d3dGPUController) Configure(cfg driver.GPUViewConfig) {
	if cfg.SampleCount <= 0 {
		cfg.SampleCount = 1
	}
	if c.ready && cfg != c.cfg {
		c.cfgDirty = true
	}
	c.cfg = cfg
}

// MountOrResize initializes the GPU view's swapchain on first mount (emitting GPUEventDeviceReady) or resizes it when physical pixel dimensions change (emitting GPUEventResize).
func (c *d3dGPUController) MountOrResize(pw, ph uint32) bool {
	if c.disposed {
		return false
	}
	pw, ph = max(pw, 1), max(ph, 1)
	if c.ready && !c.lost && !c.cfgDirty && c.pw == pw && c.ph == ph && c.swap != nil {
		return true
	}
	wasReady := c.ready && !c.lost
	if c.swap != nil {
		c.swap.Release()
		c.swap = nil
	}
	swap, hr, err := c.ops.CreateSwapTargets(pw, ph, c.cfg)
	if err != nil {
		if c.checkDeviceLost(hr, err.Error()) {
			return false
		}
		c.emitError(driver.GPUErrorFailed, err.Error())
		return false
	}
	c.swap = swap
	sizeChanged := c.pw != pw || c.ph != ph
	c.pw, c.ph = pw, ph
	c.cfgDirty = false
	if !wasReady {
		c.generation++
		c.ready = true
		c.lost = false
		c.emitEvent(&driver.GPUEvent{
			Kind:          driver.GPUEventDeviceReady,
			Generation:    c.generation,
			Width:         int(pw),
			Height:        int(ph),
			SurfaceFormat: driver.GPUFormatBGRA8Unorm,
			Limits:        d3d11Limits(),
			Features:      d3d11Features(),
		})
	} else if sizeChanged {
		c.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventResize,
			Generation: c.generation,
			Width:      int(pw),
			Height:     int(ph),
		})
	}
	return true
}

// NotifyDeviceLost releases all GPU resources, marks the controller lost, and emits GPUEventDeviceLost once for the active generation.
func (c *d3dGPUController) NotifyDeviceLost(hr uint32, msg string) {
	if c.disposed || c.lost {
		return
	}
	wasReady := c.ready
	gen := c.generation
	c.releaseResourcesOnly()
	c.ready = false
	c.lost = true
	if wasReady {
		if msg == "" {
			msg = fmt.Sprintf("Direct3D 11 device lost (HRESULT 0x%08x)", hr)
		}
		c.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventDeviceLost,
			Generation: gen,
			LostReason: d3dLostReasonFromHR(hr),
			Message:    msg,
		})
	}
}

// Recover recreates the swapchain targets after a device loss, bumping the generation and emitting GPUEventDeviceReady so the app rebuilds resources.
func (c *d3dGPUController) Recover() bool {
	if c.disposed || !c.lost {
		return false
	}
	return c.MountOrResize(c.pw, c.ph)
}

// Release releases every retained GPU resource and swapchain target on unmount or window close. Safe to call multiple times.
func (c *d3dGPUController) Release() {
	c.disposed = true
	c.ready = false
	c.releaseResourcesOnly()
}

func (c *d3dGPUController) releaseResourcesOnly() {
	for id, p := range c.pipelines {
		if p.handle != nil {
			p.handle.Release()
		}
		delete(c.pipelines, id)
	}
	for id := range c.bindGroups {
		delete(c.bindGroups, id)
	}
	for id := range c.layouts {
		delete(c.layouts, id)
	}
	for id, s := range c.samplers {
		if s.handle != nil {
			s.handle.Release()
		}
		delete(c.samplers, id)
	}
	for id, t := range c.textures {
		if t.handle != nil {
			t.handle.Release()
		}
		delete(c.textures, id)
	}
	for id, b := range c.buffers {
		if b.handle != nil {
			b.handle.Release()
		}
		delete(c.buffers, id)
	}
	if c.swap != nil {
		c.swap.Release()
		c.swap = nil
	}
}

// Submit executes one off-commit GPUFrame (resource ops, copies, render passes, readbacks, destroys) and reports whether the swapchain was modified.
func (c *d3dGPUController) Submit(frame driver.GPUFrame) (drewSwap bool) {
	if c.disposed {
		return false
	}
	if !c.ready || c.lost || (frame.Generation != 0 && frame.Generation != c.generation) {
		for _, op := range frame.Ops {
			if op.Kind == driver.GPUOpReadTexture {
				c.emitEvent(&driver.GPUEvent{
					Kind:       driver.GPUEventReadback,
					Generation: frame.Generation,
					ReadbackID: op.ReadbackID,
					Message:    "GPU device is lost or stale generation",
				})
			}
		}
		return false
	}

	var deferredReadbacks []driver.GPUResourceOp
	var deferredDestroys []driver.GPUResourceOp

	// Phase 1: creates and writes.
	for _, op := range frame.Ops {
		switch op.Kind {
		case driver.GPUOpReadTexture:
			deferredReadbacks = append(deferredReadbacks, op)
		case driver.GPUOpDestroyBuffer, driver.GPUOpDestroyTexture, driver.GPUOpDestroySampler,
			driver.GPUOpDestroyBindGroupLayout, driver.GPUOpDestroyBindGroup, driver.GPUOpDestroyPipeline:
			deferredDestroys = append(deferredDestroys, op)
		default:
			if !c.execCreateOrWrite(op) {
				if c.lost {
					c.failReadbacks(deferredReadbacks, "GPU device lost")
					return false
				}
			}
		}
	}

	lookup := &d3dPassLookup{
		buffers:    c.buffers,
		textures:   c.textures,
		samplers:   c.samplers,
		bindGroups: c.bindGroups,
		pipelines:  c.pipelines,
	}

	// Phase 2: copies.
	for _, cp := range frame.Copies {
		hr, err := c.ops.ExecuteCopy(cp, lookup)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				c.failReadbacks(deferredReadbacks, "GPU device lost")
				return false
			}
			c.emitError(driver.GPUErrorFailed, err.Error())
		}
	}

	// Phase 3: render passes.
	for _, pass := range frame.Passes {
		sw, hr, err := c.ops.ExecutePass(pass, c.swap, c.pw, c.ph, lookup)
		if sw {
			drewSwap = true
		}
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				c.failReadbacks(deferredReadbacks, "GPU device lost")
				return false
			}
			c.emitError(driver.GPUErrorFailed, err.Error())
		}
	}

	// Phase 4: readbacks (after passes so single-frame Pass+ReadTexture works).
	for i, op := range deferredReadbacks {
		var tex *d3dTrackedTexture
		if !op.ReadbackSurface {
			tex = c.textures[op.ID]
			if tex == nil {
				c.emitEvent(&driver.GPUEvent{
					Kind:       driver.GPUEventReadback,
					Generation: c.generation,
					ReadbackID: op.ReadbackID,
					Message:    fmt.Sprintf("d3d11: unknown texture %d for readback", op.ID),
				})
				continue
			}
		}
		pix, hr, err := c.ops.ReadTexture(c.swap, c.pw, c.ph, tex, op.ReadbackRegion, op.ReadbackSurface)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				c.failReadbacks(deferredReadbacks[i:], err.Error())
				return false
			}
			c.emitEvent(&driver.GPUEvent{
				Kind:       driver.GPUEventReadback,
				Generation: c.generation,
				ReadbackID: op.ReadbackID,
				Message:    err.Error(),
			})
			continue
		}
		c.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: c.generation,
			ReadbackID: op.ReadbackID,
			Pixels:     pix,
		})
	}

	// Phase 5: deferred destroys (safe because IDs are globally monotonic).
	for _, op := range deferredDestroys {
		c.execDestroy(op)
	}

	// Check for asynchronous device removal after command submission.
	if reason := c.ops.DeviceRemovedReason(); reason != 0 && d3dIsDeviceLostHR(reason) {
		c.NotifyDeviceLost(reason, fmt.Sprintf("Direct3D 11 device removed (HRESULT 0x%08x)", reason))
		return false
	}

	return drewSwap || frame.Present
}

func (c *d3dGPUController) execCreateOrWrite(op driver.GPUResourceOp) bool {
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		if op.Buffer == nil {
			return false
		}
		mainDesc, hasCB, cbDesc := d3dBufferDescriptor(*op.Buffer)
		var shadow []byte
		if hasCB {
			shadow = make([]byte, cbDesc.byteWidth)
			copy(shadow, op.Buffer.Data)
		}
		handle, hr, err := c.ops.CreateBuffer(mainDesc, hasCB, cbDesc, op.Buffer.Data, shadow)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				return false
			}
			c.emitError(d3dErrorCodeFromHR(hr, driver.GPUErrorFailed), err.Error())
			return false
		}
		if prev := c.buffers[op.ID]; prev != nil && prev.handle != nil {
			prev.handle.Release()
		}
		c.buffers[op.ID] = &d3dTrackedBuffer{
			id:       op.ID,
			desc:     *op.Buffer,
			mainDesc: mainDesc,
			hasCB:    hasCB,
			cbDesc:   cbDesc,
			cbShadow: shadow,
			handle:   handle,
		}
		return true

	case driver.GPUOpWriteBuffer:
		buf := c.buffers[op.ID]
		if buf == nil || len(op.Data) == 0 {
			return false
		}
		if buf.hasCB && len(buf.cbShadow) > 0 && op.Offset < uint64(len(buf.cbShadow)) {
			copy(buf.cbShadow[op.Offset:], op.Data)
		}
		hr, err := c.ops.WriteBuffer(buf, op.Offset, op.Data)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				return false
			}
			c.emitError(driver.GPUErrorFailed, err.Error())
			return false
		}
		return true

	case driver.GPUOpCreateTexture:
		if op.Texture == nil {
			return false
		}
		td, srv, rtv, dsv := d3dTexture2DDescriptor(*op.Texture)
		handle, hr, err := c.ops.CreateTexture(*op.Texture, td, srv, rtv, dsv)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				return false
			}
			c.emitError(d3dErrorCodeFromHR(hr, driver.GPUErrorFailed), err.Error())
			return false
		}
		if prev := c.textures[op.ID]; prev != nil && prev.handle != nil {
			prev.handle.Release()
		}
		c.textures[op.ID] = &d3dTrackedTexture{
			id:      op.ID,
			desc:    *op.Texture,
			texDesc: td,
			srvDesc: srv,
			rtvDesc: rtv,
			dsvDesc: dsv,
			handle:  handle,
		}
		return true

	case driver.GPUOpWriteTexture:
		tex := c.textures[op.ID]
		if tex == nil || len(op.Data) == 0 {
			return false
		}
		hr, err := c.ops.WriteTexture(tex, op.TextureDst, op.Data, op.DataLayout)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				return false
			}
			c.emitError(driver.GPUErrorFailed, err.Error())
			return false
		}
		return true

	case driver.GPUOpCreateSampler:
		if op.Sampler == nil {
			return false
		}
		sd := d3dSamplerDescriptor(*op.Sampler)
		handle, hr, err := c.ops.CreateSampler(sd)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				return false
			}
			c.emitError(d3dErrorCodeFromHR(hr, driver.GPUErrorFailed), err.Error())
			return false
		}
		if prev := c.samplers[op.ID]; prev != nil && prev.handle != nil {
			prev.handle.Release()
		}
		c.samplers[op.ID] = &d3dTrackedSampler{
			id:      op.ID,
			desc:    *op.Sampler,
			d3dDesc: sd,
			handle:  handle,
		}
		return true

	case driver.GPUOpCreateBindGroupLayout:
		if op.BindGroupLayout == nil {
			return false
		}
		cp := *op.BindGroupLayout
		c.layouts[op.ID] = &cp
		return true

	case driver.GPUOpCreateBindGroup:
		if op.BindGroup == nil {
			return false
		}
		entries := make(map[uint32]driver.GPUBindGroupEntry, len(op.BindGroup.Entries))
		for _, e := range op.BindGroup.Entries {
			entries[e.Binding] = e
		}
		c.bindGroups[op.ID] = &d3dTrackedBindGroup{
			id:      op.ID,
			layout:  op.BindGroup.Layout,
			entries: entries,
		}
		return true

	case driver.GPUOpCreatePipeline:
		if op.Pipeline == nil {
			return false
		}
		compiled, err := d3dCompilePipelineHLSL(op.Pipeline)
		if err != nil {
			c.emitError(driver.GPUErrorShaderCompile, err.Error())
			return false
		}
		elements, strides := d3dBuildInputElements(op.Pipeline.VertexBuffers)
		rs := d3dRasterizerDescriptor(op.Pipeline.Primitive, op.Pipeline.DepthStencil, op.Pipeline.SampleCount)
		dss := d3dDepthStencilDescriptor(op.Pipeline.DepthStencil)
		bs := d3dBlendDescriptor(op.Pipeline.Targets)
		handle, errCode, hr, err := c.ops.CreatePipeline(op.Pipeline, compiled, elements, rs, dss, bs)
		if err != nil {
			if c.checkDeviceLost(hr, err.Error()) {
				return false
			}
			if errCode == driver.GPUErrorNone {
				errCode = d3dErrorCodeFromHR(hr, driver.GPUErrorFailed)
			}
			c.emitError(errCode, err.Error())
			return false
		}
		if prev := c.pipelines[op.ID]; prev != nil && prev.handle != nil {
			prev.handle.Release()
		}
		c.pipelines[op.ID] = &d3dTrackedPipeline{
			id:        op.ID,
			desc:      *op.Pipeline,
			elements:  elements,
			vbStrides: strides,
			topology:  d3dPrimitiveTopology(op.Pipeline.Primitive.Topology),
			vsRegs:    compiled.VSRegisters,
			psRegs:    compiled.PSRegisters,
			handle:    handle,
		}
		return true
	}
	return true
}

func (c *d3dGPUController) execDestroy(op driver.GPUResourceOp) {
	switch op.Kind {
	case driver.GPUOpDestroyBuffer:
		if b, ok := c.buffers[op.ID]; ok {
			if b.handle != nil {
				b.handle.Release()
			}
			delete(c.buffers, op.ID)
		}
	case driver.GPUOpDestroyTexture:
		if t, ok := c.textures[op.ID]; ok {
			if t.handle != nil {
				t.handle.Release()
			}
			delete(c.textures, op.ID)
		}
	case driver.GPUOpDestroySampler:
		if s, ok := c.samplers[op.ID]; ok {
			if s.handle != nil {
				s.handle.Release()
			}
			delete(c.samplers, op.ID)
		}
	case driver.GPUOpDestroyBindGroupLayout:
		delete(c.layouts, op.ID)
	case driver.GPUOpDestroyBindGroup:
		delete(c.bindGroups, op.ID)
	case driver.GPUOpDestroyPipeline:
		if p, ok := c.pipelines[op.ID]; ok {
			if p.handle != nil {
				p.handle.Release()
			}
			delete(c.pipelines, op.ID)
		}
	}
}

func (c *d3dGPUController) checkDeviceLost(hr uint32, msg string) bool {
	if d3dIsDeviceLostHR(hr) {
		c.NotifyDeviceLost(hr, msg)
		return true
	}
	if reason := c.ops.DeviceRemovedReason(); reason != 0 && d3dIsDeviceLostHR(reason) {
		c.NotifyDeviceLost(reason, msg)
		return true
	}
	return false
}

func (c *d3dGPUController) failReadbacks(rbs []driver.GPUResourceOp, msg string) {
	for _, op := range rbs {
		c.emitEvent(&driver.GPUEvent{
			Kind:       driver.GPUEventReadback,
			Generation: c.generation,
			ReadbackID: op.ReadbackID,
			Message:    msg,
		})
	}
}

func (c *d3dGPUController) emitError(code driver.GPUErrorCode, msg string) {
	c.emitEvent(&driver.GPUEvent{
		Kind:       driver.GPUEventError,
		Generation: c.generation,
		ErrorCode:  code,
		Message:    msg,
	})
}

func (c *d3dGPUController) emitEvent(ev *driver.GPUEvent) {
	if c.emit != nil && ev != nil {
		c.emit(ev)
	}
}

func d3dErrorCodeFromHR(hr uint32, fallback driver.GPUErrorCode) driver.GPUErrorCode {
	if hr == winHRErrOutOfMemory {
		return driver.GPUErrorOutOfMemory
	}
	if d3dIsDeviceLostHR(hr) {
		return driver.GPUErrorDeviceLost
	}
	return fallback
}

// d3dResolveBinding maps one HLSL RegisterBinding to the matching resource entry in the currently bound bind groups.
func d3dResolveBinding(rb hlsl.RegisterBinding, bound [8]*d3dTrackedBindGroup) (driver.GPUBindGroupEntry, bool) {
	if rb.Group >= uint32(len(bound)) {
		return driver.GPUBindGroupEntry{}, false
	}
	bg := bound[rb.Group]
	if bg == nil {
		return driver.GPUBindGroupEntry{}, false
	}
	e, ok := bg.entries[rb.Binding]
	return e, ok
}

// d3dBindingStageKind is a helper alias for readability when dispatching shaderir.BindingKind in native binding loops.
func d3dIsSRVBinding(k shaderir.BindingKind) bool {
	return k == shaderir.BindingTexture || k == shaderir.BindingStorageBufferRead
}
