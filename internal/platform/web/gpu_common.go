package web

import (
	"fmt"
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir/wgsl"
)

// gpuBackendKind identifies which browser GPU API backs a KindGPU view.
type gpuBackendKind uint8

const (
	gpuBackendNone gpuBackendKind = iota
	gpuBackendWebGPU
	gpuBackendWebGL2
)

func (k gpuBackendKind) String() string {
	switch k {
	case gpuBackendWebGPU:
		return "webgpu"
	case gpuBackendWebGL2:
		return "webgl2"
	default:
		return "none"
	}
}

// gpuProbeCaps describes browser GPU capabilities probed during initialization.
type gpuProbeCaps struct {
	HasNavigatorGPU bool
	WebGPUAdapterOK bool
	WebGPUDeviceOK  bool
	HasWebGL2       bool
	ForceWebGL2     bool
}

// selectGPUBackend chooses WebGPU when available and initialized, falling back
// automatically to WebGL 2 when WebGPU is absent, forced off, or fails init.
func selectGPUBackend(caps gpuProbeCaps) gpuBackendKind {
	if !caps.ForceWebGL2 && caps.HasNavigatorGPU && caps.WebGPUAdapterOK && caps.WebGPUDeviceOK {
		return gpuBackendWebGPU
	}
	if caps.HasWebGL2 {
		return gpuBackendWebGL2
	}
	return gpuBackendNone
}

// gpuEnvSupported reports Env.GPU for the web backend: true whenever either
// WebGPU or WebGL 2 is available in the browser.
func gpuEnvSupported(hasNavigatorGPU, hasWebGL2 bool) bool {
	return hasNavigatorGPU || hasWebGL2
}

// --- WebGPU constants and descriptor mappings ---

const (
	webgpuBufMapRead  uint32 = 0x0001
	webgpuBufMapWrite uint32 = 0x0002
	webgpuBufCopySrc  uint32 = 0x0004
	webgpuBufCopyDst  uint32 = 0x0008
	webgpuBufIndex    uint32 = 0x0010
	webgpuBufVertex   uint32 = 0x0020
	webgpuBufUniform  uint32 = 0x0040
	webgpuBufStorage  uint32 = 0x0080

	webgpuTexCopySrc          uint32 = 0x01
	webgpuTexCopyDst          uint32 = 0x02
	webgpuTexBinding          uint32 = 0x04
	webgpuTexStorageBinding   uint32 = 0x08
	webgpuTexRenderAttachment uint32 = 0x10

	webgpuStageVertex   uint32 = 0x1
	webgpuStageFragment uint32 = 0x2

	webgpuMapRead uint32 = 0x0001
)

func webgpuBufferUsage(u driver.GPUBufferUsage) uint32 {
	var out uint32
	if u&driver.GPUBufferVertex != 0 {
		out |= webgpuBufVertex
	}
	if u&driver.GPUBufferIndex != 0 {
		out |= webgpuBufIndex
	}
	if u&driver.GPUBufferUniform != 0 {
		out |= webgpuBufUniform
	}
	if u&driver.GPUBufferStorage != 0 {
		out |= webgpuBufStorage
	}
	if u&driver.GPUBufferCopySrc != 0 {
		out |= webgpuBufCopySrc
	}
	if u&driver.GPUBufferCopyDst != 0 {
		out |= webgpuBufCopyDst
	}
	// Always include COPY_DST so WriteBuffer works even if a caller omitted UsageCopyDst.
	out |= webgpuBufCopyDst
	return out
}

func webgpuTextureUsage(u driver.GPUTextureUsage, sampleCount int, isDepth bool) uint32 {
	var out uint32
	if u&driver.GPUTextureSampled != 0 {
		out |= webgpuTexBinding
	}
	if u&driver.GPUTextureRenderAttachment != 0 {
		out |= webgpuTexRenderAttachment
	}
	if u&driver.GPUTextureCopySrc != 0 {
		out |= webgpuTexCopySrc
	}
	if u&driver.GPUTextureCopyDst != 0 {
		out |= webgpuTexCopyDst
	}
	if sampleCount <= 1 && !isDepth {
		// Allow WriteTexture and ReadTexture on single-sample color textures.
		out |= webgpuTexCopyDst | webgpuTexCopySrc
	}
	return out
}

func wgslTextureFormat(f driver.GPUTextureFormat) string {
	switch f {
	case driver.GPUFormatRGBA8Unorm:
		return "rgba8unorm"
	case driver.GPUFormatRGBA8UnormSRGB:
		return "rgba8unorm-srgb"
	case driver.GPUFormatBGRA8Unorm:
		return "bgra8unorm"
	case driver.GPUFormatRG8Unorm:
		return "rg8unorm"
	case driver.GPUFormatR8Unorm:
		return "r8unorm"
	case driver.GPUFormatRGBA16Float:
		return "rgba16float"
	case driver.GPUFormatDepth24Plus:
		return "depth24plus"
	case driver.GPUFormatDepth24PlusStencil8:
		return "depth24plus-stencil8"
	case driver.GPUFormatDepth32Float:
		return "depth32float"
	default:
		return "rgba8unorm"
	}
}

func driverFormatFromWebGPU(s string) driver.GPUTextureFormat {
	switch s {
	case "bgra8unorm":
		return driver.GPUFormatBGRA8Unorm
	case "rgba8unorm-srgb":
		return driver.GPUFormatRGBA8UnormSRGB
	case "rgba16float":
		return driver.GPUFormatRGBA16Float
	case "rg8unorm":
		return driver.GPUFormatRG8Unorm
	case "r8unorm":
		return driver.GPUFormatR8Unorm
	case "depth24plus":
		return driver.GPUFormatDepth24Plus
	case "depth24plus-stencil8":
		return driver.GPUFormatDepth24PlusStencil8
	case "depth32float":
		return driver.GPUFormatDepth32Float
	default:
		return driver.GPUFormatRGBA8Unorm
	}
}

func webgpuTextureViewDimension(d driver.GPUTextureDimension) string {
	switch d {
	case driver.GPUDimCube:
		return "cube"
	case driver.GPUDim2DArray:
		return "2d-array"
	default:
		return "2d"
	}
}

func webgpuFilterMode(m driver.GPUFilterMode) string {
	if m == driver.GPUFilterLinear {
		return "linear"
	}
	return "nearest"
}

func webgpuAddressMode(m driver.GPUAddressMode) string {
	switch m {
	case driver.GPUAddressRepeat:
		return "repeat"
	case driver.GPUAddressMirrorRepeat:
		return "mirror-repeat"
	default:
		return "clamp-to-edge"
	}
}

func webgpuCompareFunc(c driver.GPUCompareFunc) string {
	switch c {
	case driver.GPUCompareNever:
		return "never"
	case driver.GPUCompareLess:
		return "less"
	case driver.GPUCompareEqual:
		return "equal"
	case driver.GPUCompareLessEqual:
		return "less-equal"
	case driver.GPUCompareGreater:
		return "greater"
	case driver.GPUCompareNotEqual:
		return "not-equal"
	case driver.GPUCompareGreaterEqual:
		return "greater-equal"
	case driver.GPUCompareAlways:
		return "always"
	default:
		return "less"
	}
}

func webgpuVertexFormat(f driver.GPUVertexFormat) string {
	switch f {
	case driver.GPUVertexFloat32:
		return "float32"
	case driver.GPUVertexFloat32x2:
		return "float32x2"
	case driver.GPUVertexFloat32x3:
		return "float32x3"
	case driver.GPUVertexFloat32x4:
		return "float32x4"
	case driver.GPUVertexSint32:
		return "sint32"
	case driver.GPUVertexSint32x2:
		return "sint32x2"
	case driver.GPUVertexSint32x3:
		return "sint32x3"
	case driver.GPUVertexSint32x4:
		return "sint32x4"
	case driver.GPUVertexUint32:
		return "uint32"
	case driver.GPUVertexUint32x2:
		return "uint32x2"
	case driver.GPUVertexUint32x3:
		return "uint32x3"
	case driver.GPUVertexUint32x4:
		return "uint32x4"
	case driver.GPUVertexUnorm8x4:
		return "unorm8x4"
	default:
		return "float32"
	}
}

func webgpuVertexStepMode(m driver.GPUVertexStepMode) string {
	if m == driver.GPUStepInstance {
		return "instance"
	}
	return "vertex"
}

func webgpuIndexFormat(f driver.GPUIndexFormat) string {
	if f == driver.GPUIndexUint32 {
		return "uint32"
	}
	return "uint16"
}

func webgpuTopology(t driver.GPUPrimitiveTopology) string {
	switch t {
	case driver.GPUTopologyTriangleStrip:
		return "triangle-strip"
	case driver.GPUTopologyLineList:
		return "line-list"
	case driver.GPUTopologyLineStrip:
		return "line-strip"
	case driver.GPUTopologyPointList:
		return "point-list"
	default:
		return "triangle-list"
	}
}

func webgpuFrontFace(f driver.GPUFrontFace) string {
	if f == driver.GPUFrontCW {
		return "cw"
	}
	return "ccw"
}

func webgpuCullMode(c driver.GPUCullMode) string {
	switch c {
	case driver.GPUCullFront:
		return "front"
	case driver.GPUCullBack:
		return "back"
	default:
		return "none"
	}
}

func webgpuBlendFactor(f driver.GPUBlendFactor) string {
	switch f {
	case driver.GPUBlendZero:
		return "zero"
	case driver.GPUBlendOne:
		return "one"
	case driver.GPUBlendSrc:
		return "src"
	case driver.GPUBlendOneMinusSrc:
		return "one-minus-src"
	case driver.GPUBlendSrcAlpha:
		return "src-alpha"
	case driver.GPUBlendOneMinusSrcAlpha:
		return "one-minus-src-alpha"
	case driver.GPUBlendDst:
		return "dst"
	case driver.GPUBlendOneMinusDst:
		return "one-minus-dst"
	case driver.GPUBlendDstAlpha:
		return "dst-alpha"
	case driver.GPUBlendOneMinusDstAlpha:
		return "one-minus-dst-alpha"
	case driver.GPUBlendConstant:
		return "constant"
	case driver.GPUBlendOneMinusConstant:
		return "one-minus-constant"
	default:
		return "one"
	}
}

func webgpuBlendOp(op driver.GPUBlendOperation) string {
	switch op {
	case driver.GPUBlendSubtract:
		return "subtract"
	case driver.GPUBlendReverseSubtract:
		return "reverse-subtract"
	case driver.GPUBlendMin:
		return "min"
	case driver.GPUBlendMax:
		return "max"
	default:
		return "add"
	}
}

func webgpuLoadOp(op driver.GPULoadOp) string {
	if op == driver.GPULoadLoad {
		return "load"
	}
	return "clear"
}

func webgpuStoreOp(op driver.GPUStoreOp) string {
	if op == driver.GPUStoreDiscard {
		return "discard"
	}
	return "store"
}

func webgpuVisibility(v driver.GPUShaderStageFlags) uint32 {
	var out uint32
	if v&driver.GPUStageVertex != 0 {
		out |= webgpuStageVertex
	}
	if v&driver.GPUStageFragment != 0 {
		out |= webgpuStageFragment
	}
	if out == 0 {
		out = webgpuStageVertex | webgpuStageFragment
	}
	return out
}

func webgpuLostReason(reason string) driver.GPUDeviceLostReason {
	switch reason {
	case "destroyed":
		return driver.GPULostDestroyed
	default:
		return driver.GPULostReset
	}
}

func alignUp(v, align uint64) uint64 {
	if align <= 1 {
		return v
	}
	return (v + align - 1) &^ (align - 1)
}

func webgpuReadbackBytesPerRow(width, bytesPerPixel uint32) uint32 {
	raw := width * bytesPerPixel
	return uint32(alignUp(uint64(raw), 256))
}

// --- WebGL 2 constants and descriptor mappings ---

const (
	glNone                  uint32 = 0
	glTexture2D             uint32 = 0x0DE1
	glTextureCubeMap        uint32 = 0x8513
	glTextureCubeMapPosX    uint32 = 0x8515
	glTexture2DArray        uint32 = 0x8C1A
	glArrayBuffer           uint32 = 0x8892
	glElementArrayBuffer    uint32 = 0x8893
	glUniformBuffer         uint32 = 0x8A11
	glCopyReadBuffer        uint32 = 0x8F36
	glCopyWriteBuffer       uint32 = 0x8F37
	glPixelUnpackBuffer     uint32 = 0x88EC
	glStaticDraw            uint32 = 0x88E4
	glDynamicDraw           uint32 = 0x88E8
	glFramebuffer           uint32 = 0x8D40
	glReadFramebuffer       uint32 = 0x8CA8
	glDrawFramebuffer       uint32 = 0x8CA9
	glRenderbuffer          uint32 = 0x8D41
	glColorAttachment0      uint32 = 0x8CE0
	glDepthAttachment       uint32 = 0x8D00
	glDepthStencilAttach    uint32 = 0x821A
	glColorBufferBit        uint32 = 0x4000
	glDepthBufferBit        uint32 = 0x0100
	glStencilBufferBit      uint32 = 0x0400
	glColorEnum             uint32 = 0x1800
	glDepthStencilEnum      uint32 = 0x84F9
	glInvalidIndex          uint32 = 0xFFFFFFFF
	glTexture0              uint32 = 0x84C0
	glUnpackAlignment       uint32 = 0x0CF5
	glUnpackRowLength       uint32 = 0x0CF2
	glUnpackFlipYWebGL      uint32 = 0x9240
	glUnpackPremultWebGL    uint32 = 0x9241
	glTextureMinFilter      uint32 = 0x2801
	glTextureMagFilter      uint32 = 0x2800
	glTextureWrapS          uint32 = 0x2802
	glTextureWrapT          uint32 = 0x2803
	glTextureWrapR          uint32 = 0x8072
	glTextureCompareMode    uint32 = 0x884C
	glTextureCompareFunc    uint32 = 0x884D
	glCompareRefToTexture   uint32 = 0x884E
	glNearest               uint32 = 0x2600
	glLinear                uint32 = 0x2601
	glNearestMipmapNearest  uint32 = 0x2700
	glLinearMipmapNearest   uint32 = 0x2701
	glNearestMipmapLinear   uint32 = 0x2702
	glLinearMipmapLinear    uint32 = 0x2703
	glClampToEdge           uint32 = 0x812F
	glRepeat                uint32 = 0x2901
	glMirroredRepeat        uint32 = 0x8370
	glPoints                uint32 = 0x0000
	glLines                 uint32 = 0x0001
	glLineStrip             uint32 = 0x0003
	glTriangles             uint32 = 0x0004
	glTriangleStrip         uint32 = 0x0005
	glCW                    uint32 = 0x0900
	glCCW                   uint32 = 0x0901
	glFront                 uint32 = 0x0404
	glBack                  uint32 = 0x0405
	glCullFaceEnable        uint32 = 0x0B44
	glDepthTest             uint32 = 0x0B71
	glScissorTest           uint32 = 0x0C11
	glBlendEnable           uint32 = 0x0BE2
	glPolygonOffsetFill     uint32 = 0x8037
	glNever                 uint32 = 0x0200
	glLess                  uint32 = 0x0201
	glEqual                 uint32 = 0x0202
	glLequal                uint32 = 0x0203
	glGreater               uint32 = 0x0204
	glNotequal              uint32 = 0x0205
	glGequal                uint32 = 0x0206
	glAlways                uint32 = 0x0207
	glZero                  uint32 = 0
	glOne                   uint32 = 1
	glSrcColor              uint32 = 0x0300
	glOneMinusSrcColor      uint32 = 0x0301
	glSrcAlpha              uint32 = 0x0302
	glOneMinusSrcAlpha      uint32 = 0x0303
	glDstAlpha              uint32 = 0x0304
	glOneMinusDstAlpha      uint32 = 0x0305
	glDstColor              uint32 = 0x0306
	glOneMinusDstColor      uint32 = 0x0307
	glConstantColor         uint32 = 0x8001
	glOneMinusConstantColor uint32 = 0x8002
	glFuncAdd               uint32 = 0x8006
	glFuncSubtract          uint32 = 0x800A
	glFuncReverseSubtract   uint32 = 0x800B
	glMin                   uint32 = 0x8007
	glMax                   uint32 = 0x8008
	glUnsignedByte          uint32 = 0x1401
	glUnsignedShort         uint32 = 0x1403
	glInt                   uint32 = 0x1404
	glUnsignedInt           uint32 = 0x1405
	glFloat                 uint32 = 0x1406
	glHalfFloat             uint32 = 0x140B
	glUnsignedInt248        uint32 = 0x84FA
	glRed                   uint32 = 0x1903
	glRG                    uint32 = 0x8227
	glRGBA                  uint32 = 0x1908
	glDepthComponent        uint32 = 0x1902
	glR8                    uint32 = 0x8229
	glRG8                   uint32 = 0x822B
	glRGBA8                 uint32 = 0x8058
	glSRGB8Alpha8           uint32 = 0x8C43
	glRGBA16F               uint32 = 0x881A
	glDepthComponent24      uint32 = 0x81A6
	glDepth24Stencil8       uint32 = 0x88F0
	glDepthComponent32F     uint32 = 0x8CAC
	glVertexShader          uint32 = 0x8B31
	glFragmentShader        uint32 = 0x8B30
	glCompileStatus         uint32 = 0x8B81
	glLinkStatus            uint32 = 0x8B82
	glMaxTextureSize        uint32 = 0x0D33
	glMaxUniformBlockSize   uint32 = 0x8A30
	glMaxVertexAttribs      uint32 = 0x8869
	glUniformBufOffsetAlign uint32 = 0x8A34
)

type glTextureFormatInfo struct {
	InternalFormat uint32
	Format         uint32
	Type           uint32
	BytesPerPixel  uint32
	IsDepth        bool
	HasStencil     bool
}

func glFormatInfo(f driver.GPUTextureFormat) glTextureFormatInfo {
	switch f {
	case driver.GPUFormatRGBA8UnormSRGB:
		return glTextureFormatInfo{glSRGB8Alpha8, glRGBA, glUnsignedByte, 4, false, false}
	case driver.GPUFormatRG8Unorm:
		return glTextureFormatInfo{glRG8, glRG, glUnsignedByte, 2, false, false}
	case driver.GPUFormatR8Unorm:
		return glTextureFormatInfo{glR8, glRed, glUnsignedByte, 1, false, false}
	case driver.GPUFormatRGBA16Float:
		return glTextureFormatInfo{glRGBA16F, glRGBA, glHalfFloat, 8, false, false}
	case driver.GPUFormatDepth24Plus:
		return glTextureFormatInfo{glDepthComponent24, glDepthComponent, glUnsignedInt, 4, true, false}
	case driver.GPUFormatDepth24PlusStencil8:
		return glTextureFormatInfo{glDepth24Stencil8, glDepthStencilEnum, glUnsignedInt248, 4, true, true}
	case driver.GPUFormatDepth32Float:
		return glTextureFormatInfo{glDepthComponent32F, glDepthComponent, glFloat, 4, true, false}
	default:
		// RGBA8Unorm and BGRA8Unorm both map to RGBA8 in WebGL 2.
		return glTextureFormatInfo{glRGBA8, glRGBA, glUnsignedByte, 4, false, false}
	}
}

func glTextureTarget(d driver.GPUTextureDimension) uint32 {
	switch d {
	case driver.GPUDimCube:
		return glTextureCubeMap
	case driver.GPUDim2DArray:
		return glTexture2DArray
	default:
		return glTexture2D
	}
}

func glBufferTarget(u driver.GPUBufferUsage) uint32 {
	switch {
	case u&driver.GPUBufferUniform != 0:
		return glUniformBuffer
	case u&driver.GPUBufferIndex != 0 && u&driver.GPUBufferVertex == 0:
		return glElementArrayBuffer
	default:
		return glArrayBuffer
	}
}

type glVertexAttr struct {
	Components int
	GLType     uint32
	Normalized bool
	Integer    bool
}

func glVertexAttrInfo(f driver.GPUVertexFormat) glVertexAttr {
	switch f {
	case driver.GPUVertexFloat32:
		return glVertexAttr{1, glFloat, false, false}
	case driver.GPUVertexFloat32x2:
		return glVertexAttr{2, glFloat, false, false}
	case driver.GPUVertexFloat32x3:
		return glVertexAttr{3, glFloat, false, false}
	case driver.GPUVertexFloat32x4:
		return glVertexAttr{4, glFloat, false, false}
	case driver.GPUVertexSint32:
		return glVertexAttr{1, glInt, false, true}
	case driver.GPUVertexSint32x2:
		return glVertexAttr{2, glInt, false, true}
	case driver.GPUVertexSint32x3:
		return glVertexAttr{3, glInt, false, true}
	case driver.GPUVertexSint32x4:
		return glVertexAttr{4, glInt, false, true}
	case driver.GPUVertexUint32:
		return glVertexAttr{1, glUnsignedInt, false, true}
	case driver.GPUVertexUint32x2:
		return glVertexAttr{2, glUnsignedInt, false, true}
	case driver.GPUVertexUint32x3:
		return glVertexAttr{3, glUnsignedInt, false, true}
	case driver.GPUVertexUint32x4:
		return glVertexAttr{4, glUnsignedInt, false, true}
	case driver.GPUVertexUnorm8x4:
		return glVertexAttr{4, glUnsignedByte, true, false}
	default:
		return glVertexAttr{4, glFloat, false, false}
	}
}

func glPrimitiveMode(t driver.GPUPrimitiveTopology) uint32 {
	switch t {
	case driver.GPUTopologyTriangleStrip:
		return glTriangleStrip
	case driver.GPUTopologyLineList:
		return glLines
	case driver.GPUTopologyLineStrip:
		return glLineStrip
	case driver.GPUTopologyPointList:
		return glPoints
	default:
		return glTriangles
	}
}

func glFrontFace(f driver.GPUFrontFace) uint32 {
	if f == driver.GPUFrontCW {
		return glCW
	}
	return glCCW
}

func glCullFace(c driver.GPUCullMode) (enabled bool, face uint32) {
	switch c {
	case driver.GPUCullFront:
		return true, glFront
	case driver.GPUCullBack:
		return true, glBack
	default:
		return false, glBack
	}
}

func glCompareFunc(c driver.GPUCompareFunc) uint32 {
	switch c {
	case driver.GPUCompareNever:
		return glNever
	case driver.GPUCompareLess:
		return glLess
	case driver.GPUCompareEqual:
		return glEqual
	case driver.GPUCompareLessEqual:
		return glLequal
	case driver.GPUCompareGreater:
		return glGreater
	case driver.GPUCompareNotEqual:
		return glNotequal
	case driver.GPUCompareGreaterEqual:
		return glGequal
	case driver.GPUCompareAlways:
		return glAlways
	default:
		return glLess
	}
}

func glBlendFactor(f driver.GPUBlendFactor) uint32 {
	switch f {
	case driver.GPUBlendZero:
		return glZero
	case driver.GPUBlendOne:
		return glOne
	case driver.GPUBlendSrc:
		return glSrcColor
	case driver.GPUBlendOneMinusSrc:
		return glOneMinusSrcColor
	case driver.GPUBlendSrcAlpha:
		return glSrcAlpha
	case driver.GPUBlendOneMinusSrcAlpha:
		return glOneMinusSrcAlpha
	case driver.GPUBlendDst:
		return glDstColor
	case driver.GPUBlendOneMinusDst:
		return glOneMinusDstColor
	case driver.GPUBlendDstAlpha:
		return glDstAlpha
	case driver.GPUBlendOneMinusDstAlpha:
		return glOneMinusDstAlpha
	case driver.GPUBlendConstant:
		return glConstantColor
	case driver.GPUBlendOneMinusConstant:
		return glOneMinusConstantColor
	default:
		return glOne
	}
}

func glBlendEq(op driver.GPUBlendOperation) uint32 {
	switch op {
	case driver.GPUBlendSubtract:
		return glFuncSubtract
	case driver.GPUBlendReverseSubtract:
		return glFuncReverseSubtract
	case driver.GPUBlendMin:
		return glMin
	case driver.GPUBlendMax:
		return glMax
	default:
		return glFuncAdd
	}
}

func glWrapMode(m driver.GPUAddressMode) uint32 {
	switch m {
	case driver.GPUAddressRepeat:
		return glRepeat
	case driver.GPUAddressMirrorRepeat:
		return glMirroredRepeat
	default:
		return glClampToEdge
	}
}

func glMinFilter(minF, mipF driver.GPUFilterMode, hasMips bool) uint32 {
	if !hasMips {
		if minF == driver.GPUFilterLinear {
			return glLinear
		}
		return glNearest
	}
	if minF == driver.GPUFilterLinear {
		if mipF == driver.GPUFilterLinear {
			return glLinearMipmapLinear
		}
		return glLinearMipmapNearest
	}
	if mipF == driver.GPUFilterLinear {
		return glNearestMipmapLinear
	}
	return glNearestMipmapNearest
}

func glMagFilter(magF driver.GPUFilterMode) uint32 {
	if magF == driver.GPUFilterLinear {
		return glLinear
	}
	return glNearest
}

func glIndexType(f driver.GPUIndexFormat) (glType uint32, elemBytes uint64) {
	if f == driver.GPUIndexUint32 {
		return glUnsignedInt, 4
	}
	return glUnsignedShort, 2
}

// glViewportY converts a top-left-origin viewport Y + height into OpenGL's
// bottom-left-origin viewport Y.
func glViewportY(y, height float32, targetH int) int {
	return targetH - int(math.Round(float64(y+height)))
}

// glScissorY converts a top-left-origin scissor Y + height into OpenGL's
// bottom-left-origin scissor Y.
func glScissorY(y, height int32, targetH int) int {
	return targetH - int(y+height)
}

// glFlipRowsRGBA flips an RGBA image vertically in place (converting bottom-up
// WebGL readPixels output into top-down image.RGBA order).
func glFlipRowsRGBA(pix []byte, width, height int) {
	rowBytes := width * 4
	if rowBytes <= 0 || height <= 1 || len(pix) < rowBytes*height {
		return
	}
	tmp := make([]byte, rowBytes)
	for top, bot := 0, height-1; top < bot; top, bot = top+1, bot-1 {
		topOff := top * rowBytes
		botOff := bot * rowBytes
		copy(tmp, pix[topOff:topOff+rowBytes])
		copy(pix[topOff:topOff+rowBytes], pix[botOff:botOff+rowBytes])
		copy(pix[botOff:botOff+rowBytes], tmp)
	}
}

// stripRowPadding copies width*height pixels of bytesPerPixel bytes out of a
// padded WebGPU readback buffer (bytesPerRow aligned to 256) into a tightly
// packed byte slice, swizzling BGRA -> RGBA when swizzleBGRA is true and
// bytesPerPixel == 4.
func stripRowPadding(src []byte, width, height, bytesPerPixel, bytesPerRow int, swizzleBGRA bool) []byte {
	if width <= 0 || height <= 0 || bytesPerPixel <= 0 {
		return nil
	}
	dstRow := width * bytesPerPixel
	out := make([]byte, dstRow*height)
	for y := 0; y < height; y++ {
		sOff := y * bytesPerRow
		dOff := y * dstRow
		if sOff+dstRow > len(src) {
			break
		}
		copy(out[dOff:dOff+dstRow], src[sOff:sOff+dstRow])
		if swizzleBGRA && bytesPerPixel == 4 {
			for x := 0; x < dstRow; x += 4 {
				out[dOff+x], out[dOff+x+2] = out[dOff+x+2], out[dOff+x]
			}
		}
	}
	return out
}

// stripRowPaddingRGBA is stripRowPadding specialized for 4-byte RGBA/BGRA formats.
func stripRowPaddingRGBA(src []byte, width, height, bytesPerRow int, swizzleBGRA bool) []byte {
	return stripRowPadding(src, width, height, 4, bytesPerRow, swizzleBGRA)
}

// mipDim returns the dimension of mip level m for a base texture size, clamped
// to at least 1 per the WebGPU and WebGL 2 specifications.
func mipDim(base, mip uint32) uint32 {
	if base == 0 {
		return 1
	}
	d := base >> mip
	if d == 0 {
		return 1
	}
	return d
}

// isSwapchainPass reports whether pass targets the default canvas swapchain
// rather than offscreen render-target textures (including depth-only passes).
func isSwapchainPass(pass driver.GPUPass) bool {
	if len(pass.ColorAttachments) > 0 {
		return pass.ColorAttachments[0].Texture == 0
	}
	return pass.DepthStencil == nil || pass.DepthStencil.Texture == 0
}

// clampScissorRect clamps signed scissor coordinates to non-negative unsigned
// values required by WebGPU's setScissorRect.
func clampScissorRect(x, y, w, h int32) (uint32, uint32, uint32, uint32) {
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return uint32(x), uint32(y), uint32(w), uint32(h)
}

// --- Shader pipeline compilation helpers ---

type compiledWebGPUShaders struct {
	VertexWGSL    string
	VertexEntry   string
	FragmentWGSL  string
	FragmentEntry string
	SharedModule  bool
}

func compileWebGPUShaders(vs, fs driver.GPUShaderStage) (compiledWebGPUShaders, error) {
	if vs.IR == nil {
		return compiledWebGPUShaders{}, fmt.Errorf("webgpu: vertex shader IR is nil")
	}
	vsEntry := vs.IR.VertexEntry
	if vsEntry == "" {
		vsEntry = "vs_main"
	}
	if fs.IR == nil {
		code, err := wgsl.CompilePipeline(vs.IR)
		if err != nil {
			return compiledWebGPUShaders{}, err
		}
		return compiledWebGPUShaders{
			VertexWGSL:  code,
			VertexEntry: vsEntry,
		}, nil
	}
	fsEntry := fs.IR.FragmentEntry
	if fsEntry == "" {
		fsEntry = "fs_main"
	}
	if vs.IR == fs.IR {
		code, err := wgsl.CompilePipeline(vs.IR)
		if err != nil {
			return compiledWebGPUShaders{}, err
		}
		return compiledWebGPUShaders{
			VertexWGSL:    code,
			VertexEntry:   vsEntry,
			FragmentWGSL:  code,
			FragmentEntry: fsEntry,
			SharedModule:  true,
		}, nil
	}
	vsCode, err := wgsl.CompilePipeline(vs.IR)
	if err != nil {
		return compiledWebGPUShaders{}, fmt.Errorf("webgpu vertex: %w", err)
	}
	fsCode, err := wgsl.CompilePipeline(fs.IR)
	if err != nil {
		return compiledWebGPUShaders{}, fmt.Errorf("webgpu fragment: %w", err)
	}
	return compiledWebGPUShaders{
		VertexWGSL:    vsCode,
		VertexEntry:   vsEntry,
		FragmentWGSL:  fsCode,
		FragmentEntry: fsEntry,
		SharedModule:  false,
	}, nil
}

const webgl2EmptyFragSource = "#version 300 es\nprecision highp float;\nvoid main() {}\n"

func compileWebGL2Shaders(vs, fs driver.GPUShaderStage) (glsl.PipelineSource, error) {
	if vs.IR == nil {
		return glsl.PipelineSource{}, fmt.Errorf("webgl2: vertex shader IR is nil")
	}
	if fs.IR == nil {
		src, err := glsl.CompilePipeline(vs.IR, glsl.GLSLVersionES300)
		if err != nil {
			return glsl.PipelineSource{}, err
		}
		if src.FragmentSource == "" {
			src.FragmentSource = webgl2EmptyFragSource
		}
		return src, nil
	}
	if vs.IR == fs.IR {
		src, err := glsl.CompilePipeline(vs.IR, glsl.GLSLVersionES300)
		if err != nil {
			return glsl.PipelineSource{}, err
		}
		if src.FragmentSource == "" {
			src.FragmentSource = webgl2EmptyFragSource
		}
		return src, nil
	}
	vSrc, err := glsl.CompilePipeline(vs.IR, glsl.GLSLVersionES300)
	if err != nil {
		return glsl.PipelineSource{}, fmt.Errorf("webgl2 vertex: %w", err)
	}
	fSrc, err := glsl.CompilePipeline(fs.IR, glsl.GLSLVersionES300)
	if err != nil {
		return glsl.PipelineSource{}, fmt.Errorf("webgl2 fragment: %w", err)
	}
	// Merge uniform block slots and sampler pairs from both stages.
	out := glsl.PipelineSource{
		VertexSource:   vSrc.VertexSource,
		FragmentSource: fSrc.FragmentSource,
	}
	if out.FragmentSource == "" {
		out.FragmentSource = webgl2EmptyFragSource
	}
	seenUB := map[[2]uint32]bool{}
	var nextSlot uint32
	for _, ub := range append(vSrc.UniformBlocks, fSrc.UniformBlocks...) {
		k := [2]uint32{ub.Group, ub.Binding}
		if seenUB[k] {
			continue
		}
		seenUB[k] = true
		ub.Slot = nextSlot
		nextSlot++
		out.UniformBlocks = append(out.UniformBlocks, ub)
	}
	seenSP := map[string]bool{}
	var nextUnit uint32
	for _, sp := range append(vSrc.SamplerPairs, fSrc.SamplerPairs...) {
		if seenSP[sp.Name] {
			continue
		}
		seenSP[sp.Name] = true
		sp.Unit = nextUnit
		nextUnit++
		out.SamplerPairs = append(out.SamplerPairs, sp)
	}
	return out, nil
}

// --- Pure-Go GPU view lifecycle state machine ---

type gpuViewState uint8

const (
	gpuStateUninitialized gpuViewState = iota
	gpuStateInitializing
	gpuStateReady
	gpuStateLost
	gpuStateDestroyed
)

type gpuResourceCounts struct {
	Buffers          int
	Textures         int
	Samplers         int
	BindGroupLayouts int
	BindGroups       int
	Pipelines        int
}

// gpuViewController manages the portable lifecycle, pixel sizing, generation
// tracking, and resource accounting for one mounted KindGPU view.
type gpuViewController struct {
	viewID        driver.NodeID
	cfg           driver.GPUViewConfig
	configured    bool
	state         gpuViewState
	backend       gpuBackendKind
	generation    uint32
	surfaceFormat driver.GPUTextureFormat
	limits        driver.GPULimits
	features      driver.GPUFeatures
	width         int
	height        int
	cssWidth      driver.Points
	cssHeight     driver.Points
	pixelRatio    float32

	buffers    map[driver.GPUID]struct{}
	textures   map[driver.GPUID]struct{}
	samplers   map[driver.GPUID]struct{}
	layouts    map[driver.GPUID]struct{}
	bindGroups map[driver.GPUID]struct{}
	pipelines  map[driver.GPUID]struct{}
}

func newGPUViewController(id driver.NodeID) *gpuViewController {
	return &gpuViewController{
		viewID:     id,
		width:      1,
		height:     1,
		pixelRatio: 1,
		buffers:    map[driver.GPUID]struct{}{},
		textures:   map[driver.GPUID]struct{}{},
		samplers:   map[driver.GPUID]struct{}{},
		layouts:    map[driver.GPUID]struct{}{},
		bindGroups: map[driver.GPUID]struct{}{},
		pipelines:  map[driver.GPUID]struct{}{},
	}
}

func (c *gpuViewController) configure(cfg driver.GPUViewConfig) {
	c.cfg = cfg
	c.configured = true
}

func gpuPixelDimensions(cssW, cssH driver.Points, pixelRatio float32) (int, int) {
	ratio := float64(pixelRatio)
	if ratio <= 0 || math.IsNaN(ratio) {
		ratio = 1
	}
	w := int(math.Round(float64(cssW) * ratio))
	h := int(math.Round(float64(cssH) * ratio))
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return w, h
}

// resize updates the controller's physical pixel dimensions and returns a
// GPUEventResize when the view is already ready and the size changed.
func (c *gpuViewController) resize(cssW, cssH driver.Points, pixelRatio float32) (w, h int, changed bool, ev *driver.GPUEvent) {
	c.cssWidth, c.cssHeight = cssW, cssH
	if pixelRatio > 0 {
		c.pixelRatio = pixelRatio
	}
	nw, nh := gpuPixelDimensions(cssW, cssH, c.pixelRatio)
	if nw == c.width && nh == c.height {
		return nw, nh, false, nil
	}
	c.width, c.height = nw, nh
	if c.state == gpuStateReady {
		ev = &driver.GPUEvent{
			Kind:       driver.GPUEventResize,
			Generation: c.generation,
			Width:      nw,
			Height:     nh,
		}
	}
	return nw, nh, true, ev
}

func (c *gpuViewController) beginInit() bool {
	if c.state == gpuStateDestroyed || c.state == gpuStateInitializing || c.state == gpuStateReady {
		return false
	}
	c.state = gpuStateInitializing
	return true
}

// markReady transitions to Ready, bumps the device generation, and returns the
// GPUEventDeviceReady payload (nil if already destroyed).
func (c *gpuViewController) markReady(backend gpuBackendKind, surfaceFmt driver.GPUTextureFormat, limits driver.GPULimits, features driver.GPUFeatures) *driver.GPUEvent {
	if c.state == gpuStateDestroyed {
		return nil
	}
	c.clearTrackedResources()
	c.backend = backend
	c.state = gpuStateReady
	c.generation++
	if surfaceFmt == driver.GPUFormatUndefined {
		surfaceFmt = driver.GPUFormatRGBA8Unorm
	}
	c.surfaceFormat = surfaceFmt
	c.limits = limits
	c.features = features
	return &driver.GPUEvent{
		Kind:          driver.GPUEventDeviceReady,
		Generation:    c.generation,
		Width:         c.width,
		Height:        c.height,
		SurfaceFormat: c.surfaceFormat,
		Limits:        c.limits,
		Features:      c.features,
	}
}

// markLost transitions to Lost, clears tracked live resources, and returns the
// GPUEventDeviceLost payload (nil if not currently Ready or already Destroyed).
func (c *gpuViewController) markLost(reason driver.GPUDeviceLostReason, message string) *driver.GPUEvent {
	if c.state == gpuStateDestroyed || c.state == gpuStateLost {
		return nil
	}
	gen := c.generation
	c.state = gpuStateLost
	c.clearTrackedResources()
	return &driver.GPUEvent{
		Kind:       driver.GPUEventDeviceLost,
		Generation: gen,
		LostReason: reason,
		Message:    message,
	}
}

func (c *gpuViewController) makeError(code driver.GPUErrorCode, message string) *driver.GPUEvent {
	if c.state == gpuStateDestroyed {
		return nil
	}
	return &driver.GPUEvent{
		Kind:       driver.GPUEventError,
		Generation: c.generation,
		ErrorCode:  code,
		Message:    message,
	}
}

// acceptFrame checks whether frame targets the live generation and records
// resource create/destroy ops for leak accounting.
func (c *gpuViewController) acceptFrame(frame driver.GPUFrame) bool {
	if c.state != gpuStateReady {
		return false
	}
	if frame.Generation != 0 && frame.Generation != c.generation {
		return false
	}
	for _, op := range frame.Ops {
		switch op.Kind {
		case driver.GPUOpCreateBuffer:
			c.buffers[op.ID] = struct{}{}
		case driver.GPUOpDestroyBuffer:
			delete(c.buffers, op.ID)
		case driver.GPUOpCreateTexture:
			c.textures[op.ID] = struct{}{}
		case driver.GPUOpDestroyTexture:
			delete(c.textures, op.ID)
		case driver.GPUOpCreateSampler:
			c.samplers[op.ID] = struct{}{}
		case driver.GPUOpDestroySampler:
			delete(c.samplers, op.ID)
		case driver.GPUOpCreateBindGroupLayout:
			c.layouts[op.ID] = struct{}{}
		case driver.GPUOpDestroyBindGroupLayout:
			delete(c.layouts, op.ID)
		case driver.GPUOpCreateBindGroup:
			c.bindGroups[op.ID] = struct{}{}
		case driver.GPUOpDestroyBindGroup:
			delete(c.bindGroups, op.ID)
		case driver.GPUOpCreatePipeline:
			c.pipelines[op.ID] = struct{}{}
		case driver.GPUOpDestroyPipeline:
			delete(c.pipelines, op.ID)
		}
	}
	return true
}

func (c *gpuViewController) resourceCounts() gpuResourceCounts {
	return gpuResourceCounts{
		Buffers:          len(c.buffers),
		Textures:         len(c.textures),
		Samplers:         len(c.samplers),
		BindGroupLayouts: len(c.layouts),
		BindGroups:       len(c.bindGroups),
		Pipelines:        len(c.pipelines),
	}
}

func (c *gpuViewController) clearTrackedResources() {
	clear(c.buffers)
	clear(c.textures)
	clear(c.samplers)
	clear(c.layouts)
	clear(c.bindGroups)
	clear(c.pipelines)
}

func (c *gpuViewController) destroy() {
	c.state = gpuStateDestroyed
	c.backend = gpuBackendNone
	c.clearTrackedResources()
}

// resolveDynamicOffsets returns the ordered dynamic offsets for a bind group
// layout's entries that declare HasDynamicOffset.
func resolveDynamicOffsets(layout []driver.GPUBindGroupLayoutEntry, dynamicOffsets []uint32) map[uint32]uint32 {
	if len(dynamicOffsets) == 0 || len(layout) == 0 {
		return nil
	}
	out := make(map[uint32]uint32, len(dynamicOffsets))
	idx := 0
	for _, le := range layout {
		if le.HasDynamicOffset && idx < len(dynamicOffsets) {
			out[le.Binding] = dynamicOffsets[idx]
			idx++
		}
	}
	return out
}

// clampGPULimits fills zero fields with conservative Stage 1 defaults.
func clampGPULimits(l driver.GPULimits) driver.GPULimits {
	def := driver.DefaultGPULimits()
	if l.MaxTextureDimension2D == 0 {
		l.MaxTextureDimension2D = def.MaxTextureDimension2D
	}
	if l.MaxBufferSize == 0 {
		l.MaxBufferSize = def.MaxBufferSize
	}
	if l.MaxUniformBufferSize == 0 {
		l.MaxUniformBufferSize = def.MaxUniformBufferSize
	}
	if l.MaxBindGroups == 0 {
		l.MaxBindGroups = def.MaxBindGroups
	}
	if l.MaxVertexBuffers == 0 {
		l.MaxVertexBuffers = def.MaxVertexBuffers
	}
	if l.MaxVertexAttributes == 0 {
		l.MaxVertexAttributes = def.MaxVertexAttributes
	}
	if l.MinUniformBufferAlignment == 0 {
		l.MinUniformBufferAlignment = def.MinUniformBufferAlignment
	}
	return l
}
