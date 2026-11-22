package linux

import (
	"fmt"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

// OpenGL 3.3 Core / OpenGL ES 3.0 constants used by the Linux GPU backend. Prefixed with gpuGL to stay independent of the build-tagged 2D shader path.
const (
	gpuGLFalse = 0
	gpuGLTrue  = 1

	// Shaders and programs.
	gpuGLFragmentShader = 0x8B30
	gpuGLVertexShader   = 0x8B31
	gpuGLCompileStatus  = 0x8B81
	gpuGLLinkStatus     = 0x8B82
	gpuGLInvalidIndex   = 0xFFFFFFFF

	// Buffers.
	gpuGLArrayBuffer        = 0x8892
	gpuGLElementArrayBuffer = 0x8893
	gpuGLUniformBuffer      = 0x8A11
	gpuGLCopyReadBuffer     = 0x8F36
	gpuGLCopyWriteBuffer    = 0x8F37
	gpuGLPixelUnpackBuffer  = 0x88EC
	gpuGLStaticDraw         = 0x88E4
	gpuGLDynamicDraw        = 0x88E8

	// Data types.
	gpuGLByte           = 0x1400
	gpuGLUnsignedByte   = 0x1401
	gpuGLShort          = 0x1402
	gpuGLUnsignedShort  = 0x1403
	gpuGLInt            = 0x1404
	gpuGLUnsignedInt    = 0x1405
	gpuGLFloat          = 0x1406
	gpuGLHalfFloat      = 0x140B
	gpuGLUnsignedInt248 = 0x84FA

	// Textures and pixel storage.
	gpuGLTexture2D            = 0x0DE1
	gpuGLTextureCubeMap       = 0x8513
	gpuGLTextureCubeMapPosX   = 0x8515
	gpuGLTexture2DArray       = 0x8C1A
	gpuGLTexture0             = 0x84C0
	gpuGLPackRowLength        = 0x0D02
	gpuGLPackAlignment        = 0x0D05
	gpuGLUnpackRowLength      = 0x0CF2
	gpuGLUnpackAlignment      = 0x0CF5
	gpuGLTextureBaseLevel     = 0x813C
	gpuGLTextureMaxLevel      = 0x813D
	gpuGLTextureMagFilter     = 0x2800
	gpuGLTextureMinFilter     = 0x2801
	gpuGLTextureWrapS         = 0x2802
	gpuGLTextureWrapT         = 0x2803
	gpuGLTextureWrapR         = 0x8072
	gpuGLTextureCompareMode   = 0x884C
	gpuGLTextureCompareFunc   = 0x884D
	gpuGLCompareRefToTexture  = 0x884E
	gpuGLNone                 = 0
	gpuGLTextureMaxAnisotropy = 0x84FE
	gpuGLNearest              = 0x2600
	gpuGLLinear               = 0x2601
	gpuGLNearestMipmapNearest = 0x2700
	gpuGLLinearMipmapNearest  = 0x2701
	gpuGLNearestMipmapLinear  = 0x2702
	gpuGLLinearMipmapLinear   = 0x2703
	gpuGLRepeat               = 0x2901
	gpuGLClampToEdge          = 0x812F
	gpuGLMirroredRepeat       = 0x8370

	// Pixel and sized internal formats.
	gpuGLRed               = 0x1903
	gpuGLRG                = 0x8227
	gpuGLRGBA              = 0x1908
	gpuGLDepthComponent    = 0x1902
	gpuGLDepthStencil      = 0x84F9
	gpuGLR8                = 0x8229
	gpuGLRG8               = 0x822B
	gpuGLRGBA8             = 0x8058
	gpuGLSRGB8Alpha8       = 0x8C43
	gpuGLRGBA16F           = 0x881A
	gpuGLDepthComponent24  = 0x81A6
	gpuGLDepth24Stencil8   = 0x88F0
	gpuGLDepthComponent32F = 0x8CAC

	// Framebuffers.
	gpuGLFramebuffer         = 0x8D40
	gpuGLReadFramebuffer     = 0x8CA8
	gpuGLDrawFramebuffer     = 0x8CA9
	gpuGLFramebufferBinding  = 0x8CA6
	gpuGLColorAttachment0    = 0x8CE0
	gpuGLDepthAttachment     = 0x8D00
	gpuGLStencilAttachment   = 0x8D20
	gpuGLDepthStencilAttach  = 0x821A
	gpuGLFramebufferComplete = 0x8CD5
	gpuGLColorBufferBit      = 0x4000
	gpuGLDepthBufferBit      = 0x0100
	gpuGLStencilBufferBit    = 0x0400

	// Primitives.
	gpuGLPoints        = 0x0000
	gpuGLLines         = 0x0001
	gpuGLLineStrip     = 0x0003
	gpuGLTriangles     = 0x0004
	gpuGLTriangleStrip = 0x0005

	// Rasterizer, depth, stencil, scissor, and blending.
	gpuGLCullFace          = 0x0B44
	gpuGLDepthTest         = 0x0B71
	gpuGLBlend             = 0x0BE2
	gpuGLScissorTest       = 0x0C11
	gpuGLPolygonOffsetFill = 0x8037
	gpuGLFront             = 0x0404
	gpuGLBack              = 0x0405
	gpuGLCW                = 0x0900
	gpuGLCCW               = 0x0901

	// Compare functions.
	gpuGLNever    = 0x0200
	gpuGLLess     = 0x0201
	gpuGLEqual    = 0x0202
	gpuGLLEqual   = 0x0203
	gpuGLGreater  = 0x0204
	gpuGLNotEqual = 0x0205
	gpuGLGEqual   = 0x0206
	gpuGLAlways   = 0x0207

	// Blend factors and equations.
	gpuGLZero                  = 0
	gpuGLOne                   = 1
	gpuGLSrcColor              = 0x0300
	gpuGLOneMinusSrcColor      = 0x0301
	gpuGLSrcAlpha              = 0x0302
	gpuGLOneMinusSrcAlpha      = 0x0303
	gpuGLDstAlpha              = 0x0304
	gpuGLOneMinusDstAlpha      = 0x0305
	gpuGLDstColor              = 0x0306
	gpuGLOneMinusDstColor      = 0x0307
	gpuGLConstantColor         = 0x8001
	gpuGLOneMinusConstantColor = 0x8002
	gpuGLFuncAdd               = 0x8006
	gpuGLMin                   = 0x8007
	gpuGLMax                   = 0x8008
	gpuGLFuncSubtract          = 0x800A
	gpuGLFuncReverseSubtract   = 0x800B

	// Queries.
	gpuGLMaxTextureSize           = 0x0D33
	gpuGLMaxVertexAttribs         = 0x8869
	gpuGLMaxUniformBlockSize      = 0x8A30
	gpuGLUniformBufferOffsetAlign = 0x8A34
)

// glAPI abstracts OpenGL 3.3 Core / OpenGL ES 3.0 entry points so the full GPU command interpreter, resource manager, and state machine are unit-testable on any host while gpu.go wires it directly to libepoxy through purego on Linux.
type glAPI interface {
	// Shaders and programs.
	CreateShader(kind uint32) uint32
	ShaderSource(shader uint32, src string)
	CompileShader(shader uint32)
	GetShaderiv(shader, pname uint32) int32
	GetShaderInfoLog(shader uint32) string
	DeleteShader(shader uint32)
	CreateProgram() uint32
	AttachShader(prog, shader uint32)
	LinkProgram(prog uint32)
	GetProgramiv(prog, pname uint32) int32
	GetProgramInfoLog(prog uint32) string
	DeleteProgram(prog uint32)
	UseProgram(prog uint32)
	GetUniformLocation(prog uint32, name string) int32
	Uniform1i(loc, v int32)
	GetUniformBlockIndex(prog uint32, name string) uint32
	UniformBlockBinding(prog, index, binding uint32)

	// Buffers.
	GenBuffer() uint32
	DeleteBuffer(id uint32)
	BindBuffer(target, id uint32)
	BufferData(target uint32, size int, data []byte, usage uint32)
	BufferSubData(target uint32, offset int, data []byte)
	BindBufferBase(target, index, id uint32)
	BindBufferRange(target, index, id uint32, offset, size int)
	CopyBufferSubData(readTarget, writeTarget uint32, readOffset, writeOffset, size int)

	// Vertex arrays.
	GenVertexArray() uint32
	DeleteVertexArray(id uint32)
	BindVertexArray(id uint32)
	EnableVertexAttribArray(index uint32)
	DisableVertexAttribArray(index uint32)
	VertexAttribPointer(index uint32, size int32, typ uint32, normalized bool, stride int32, offset uintptr)
	VertexAttribIPointer(index uint32, size int32, typ uint32, stride int32, offset uintptr)
	VertexAttribDivisor(index, divisor uint32)

	// Textures.
	GenTexture() uint32
	DeleteTexture(id uint32)
	ActiveTexture(unit uint32)
	BindTexture(target, id uint32)
	PixelStorei(pname uint32, param int32)
	TexImage2D(target uint32, level, internalFormat, width, height, border int32, format, typ uint32, data []byte)
	TexSubImage2D(target uint32, level, x, y, width, height int32, format, typ uint32, data []byte)
	TexSubImage2DOffset(target uint32, level, x, y, width, height int32, format, typ uint32, offset uintptr)
	TexImage3D(target uint32, level, internalFormat, width, height, depth, border int32, format, typ uint32, data []byte)
	TexSubImage3D(target uint32, level, x, y, z, width, height, depth int32, format, typ uint32, data []byte)
	TexSubImage3DOffset(target uint32, level, x, y, z, width, height, depth int32, format, typ uint32, offset uintptr)
	TexParameteri(target, pname uint32, param int32)

	// Samplers.
	GenSampler() uint32
	DeleteSampler(id uint32)
	BindSampler(unit, sampler uint32)
	SamplerParameteri(sampler, pname uint32, param int32)
	SamplerParameterf(sampler, pname uint32, param float32)

	// Framebuffers and readback.
	GenFramebuffer() uint32
	DeleteFramebuffer(id uint32)
	BindFramebuffer(target, id uint32)
	FramebufferTexture2D(target, attachment, texTarget, texture uint32, level int32)
	FramebufferTextureLayer(target, attachment, texture uint32, level, layer int32)
	DrawBuffers(bufs []uint32)
	CheckFramebufferStatus(target uint32) uint32
	BlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32)
	ReadBuffer(src uint32)
	ReadPixels(x, y, width, height int32, format, typ uint32, dst []byte)

	// State and clears.
	Viewport(x, y, width, height int32)
	DepthRangef(near, far float32)
	Enable(cap uint32)
	Disable(cap uint32)
	Scissor(x, y, width, height int32)
	CullFace(mode uint32)
	FrontFace(mode uint32)
	DepthMask(flag bool)
	DepthFunc(fn uint32)
	PolygonOffset(factor, units float32)
	ColorMask(r, g, b, a bool)
	BlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha uint32)
	BlendEquationSeparate(modeRGB, modeAlpha uint32)
	BlendColor(r, g, b, a float32)
	ClearColor(r, g, b, a float32)
	ClearDepthf(d float32)
	ClearStencil(s int32)
	Clear(mask uint32)

	// Draw calls.
	DrawArrays(mode uint32, first, count int32)
	DrawArraysInstanced(mode uint32, first, count, instanceCount int32)
	DrawElements(mode uint32, count int32, typ uint32, offset uintptr)
	DrawElementsInstanced(mode uint32, count int32, typ uint32, offset uintptr, instanceCount int32)
	DrawElementsBaseVertex(mode uint32, count int32, typ uint32, offset uintptr, baseVertex int32)
	DrawElementsInstancedBaseVertex(mode uint32, count int32, typ uint32, offset uintptr, instanceCount, baseVertex int32)

	// Queries.
	GetIntegerv(pname uint32) int32
	GetError() uint32
}

type glGPUBuffer struct {
	glID   uint32
	size   uint64
	usage  driver.GPUBufferUsage
	target uint32
}

type glGPUTexture struct {
	glID      uint32
	target    uint32
	dim       driver.GPUTextureDimension
	format    driver.GPUTextureFormat
	width     uint32
	height    uint32
	layers    uint32
	mipLevels uint32
	// rendered records whether the texture's current contents came from an FBO render pass (OpenGL Y-up) rather than a top-row-first WriteTexture upload.
	rendered bool
}

type glGPUSampler struct {
	glID uint32
	desc driver.GPUSamplerDesc
}

type glGPUBindGroup struct {
	layout  driver.GPUID
	entries map[uint32]driver.GPUBindGroupEntry
}

type glGPUPipeline struct {
	prog          uint32
	vao           uint32
	desc          driver.GPUPipelineDesc
	uniformBlocks []glsl.UniformBlockSlot
	samplerPairs  []glsl.SamplerPair
}

type glGPUError struct {
	code    driver.GPUErrorCode
	message string
}

type glGPUReadback struct {
	id     uint64
	pixels []byte
	errMsg string
}

// glGPUContext owns all retained OpenGL objects and executes driver.GPUFrame batches within one GtkGLArea's GL context.
type glGPUContext struct {
	gl            glAPI
	es            bool
	hasBaseVertex bool
	generation    uint32
	limits        driver.GPULimits
	features      driver.GPUFeatures

	buffers    map[driver.GPUID]*glGPUBuffer
	textures   map[driver.GPUID]*glGPUTexture
	samplers   map[driver.GPUID]*glGPUSampler
	layouts    map[driver.GPUID]*driver.GPUBindGroupLayoutDesc
	bindGroups map[driver.GPUID]*glGPUBindGroup
	pipelines  map[driver.GPUID]*glGPUPipeline

	scratchFBO          uint32
	readFBO             uint32
	drawFBO             uint32
	lastOffscreenColors int
}

func newGLGPUContext(gl glAPI, es bool, generation uint32, limits driver.GPULimits, features driver.GPUFeatures) *glGPUContext {
	return &glGPUContext{
		gl:            gl,
		es:            es,
		hasBaseVertex: !es,
		generation:    generation,
		limits:        limits,
		features:      features,
		buffers:       map[driver.GPUID]*glGPUBuffer{},
		textures:      map[driver.GPUID]*glGPUTexture{},
		samplers:      map[driver.GPUID]*glGPUSampler{},
		layouts:       map[driver.GPUID]*driver.GPUBindGroupLayoutDesc{},
		bindGroups:    map[driver.GPUID]*glGPUBindGroup{},
		pipelines:     map[driver.GPUID]*glGPUPipeline{},
	}
}

// mergeGPULimits combines queried GL driver limits with conservative defaults.
func mergeGPULimits(maxTex, maxUBO, minUBOAlign, maxAttribs int32) driver.GPULimits {
	lim := driver.DefaultGPULimits()
	if maxTex > 0 {
		lim.MaxTextureDimension2D = uint32(maxTex)
	}
	if maxUBO > 0 {
		lim.MaxUniformBufferSize = uint64(maxUBO)
	}
	if minUBOAlign > 0 {
		lim.MinUniformBufferAlignment = uint32(minUBOAlign)
	}
	if maxAttribs > 0 && uint32(maxAttribs) < lim.MaxVertexAttributes {
		lim.MaxVertexAttributes = uint32(maxAttribs)
	}
	return lim
}

// destroyAll deletes every OpenGL object owned by c. Caller must have made the owning GL context current.
func (c *glGPUContext) destroyAll() {
	if c == nil || c.gl == nil {
		return
	}
	for id, b := range c.buffers {
		if b.glID != 0 {
			c.gl.DeleteBuffer(b.glID)
		}
		delete(c.buffers, id)
	}
	for id, t := range c.textures {
		if t.glID != 0 {
			c.gl.DeleteTexture(t.glID)
		}
		delete(c.textures, id)
	}
	for id, s := range c.samplers {
		if s.glID != 0 {
			c.gl.DeleteSampler(s.glID)
		}
		delete(c.samplers, id)
	}
	for id, p := range c.pipelines {
		if p.vao != 0 {
			c.gl.DeleteVertexArray(p.vao)
		}
		if p.prog != 0 {
			c.gl.DeleteProgram(p.prog)
		}
		delete(c.pipelines, id)
	}
	clear(c.layouts)
	clear(c.bindGroups)
	if c.scratchFBO != 0 {
		c.gl.DeleteFramebuffer(c.scratchFBO)
		c.scratchFBO = 0
	}
	if c.readFBO != 0 {
		c.gl.DeleteFramebuffer(c.readFBO)
		c.readFBO = 0
	}
	if c.drawFBO != 0 {
		c.gl.DeleteFramebuffer(c.drawFBO)
		c.drawFBO = 0
	}
	c.lastOffscreenColors = 0
}

// executeFrame runs one submitted driver.GPUFrame against the active GL context: non-readback resource ops -> copy ops -> render passes -> readbacks.
func (c *glGPUContext) executeFrame(frame driver.GPUFrame, defaultFBO uint32, surfaceW, surfaceH int32) ([]glGPUReadback, []glGPUError) {
	var (
		readbacks   []glGPUReadback
		errs        []glGPUError
		pendingRead []driver.GPUResourceOp
	)
	for _, op := range frame.Ops {
		if op.Kind == driver.GPUOpReadTexture {
			pendingRead = append(pendingRead, op)
			continue
		}
		if err := c.applyOp(op); err != nil {
			errs = append(errs, *err)
		}
	}
	for _, cp := range frame.Copies {
		if err := c.execCopy(cp, defaultFBO); err != nil {
			errs = append(errs, *err)
		}
	}
	for _, pass := range frame.Passes {
		if err := c.execPass(pass, defaultFBO, surfaceW, surfaceH); err != nil {
			errs = append(errs, *err)
		}
	}
	for _, rop := range pendingRead {
		readbacks = append(readbacks, c.execReadback(rop, defaultFBO, surfaceW, surfaceH))
	}
	// Restore clean default framebuffer, VAO/program, and mask/scissor state for GTK.
	c.gl.BindVertexArray(0)
	c.gl.UseProgram(0)
	c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
	c.gl.Disable(gpuGLScissorTest)
	c.gl.ColorMask(true, true, true, true)
	c.gl.DepthMask(true)
	return readbacks, errs
}

func (c *glGPUContext) applyOp(op driver.GPUResourceOp) *glGPUError {
	switch op.Kind {
	case driver.GPUOpCreateBuffer:
		return c.createBuffer(op.ID, op.Buffer)
	case driver.GPUOpWriteBuffer:
		return c.writeBuffer(op.ID, op.Offset, op.Data)
	case driver.GPUOpDestroyBuffer:
		if b, ok := c.buffers[op.ID]; ok {
			if b.glID != 0 {
				c.gl.DeleteBuffer(b.glID)
			}
			delete(c.buffers, op.ID)
		}
	case driver.GPUOpCreateTexture:
		return c.createTexture(op.ID, op.Texture)
	case driver.GPUOpWriteTexture:
		return c.writeTexture(op.ID, op.TextureDst, op.Data, op.DataLayout)
	case driver.GPUOpDestroyTexture:
		if t, ok := c.textures[op.ID]; ok {
			if t.glID != 0 {
				c.gl.DeleteTexture(t.glID)
			}
			delete(c.textures, op.ID)
		}
	case driver.GPUOpCreateSampler:
		return c.createSampler(op.ID, op.Sampler)
	case driver.GPUOpDestroySampler:
		if s, ok := c.samplers[op.ID]; ok {
			if s.glID != 0 {
				c.gl.DeleteSampler(s.glID)
			}
			delete(c.samplers, op.ID)
		}
	case driver.GPUOpCreateBindGroupLayout:
		if op.BindGroupLayout != nil {
			cp := *op.BindGroupLayout
			cp.Entries = slices.Clone(op.BindGroupLayout.Entries)
			c.layouts[op.ID] = &cp
		}
	case driver.GPUOpDestroyBindGroupLayout:
		delete(c.layouts, op.ID)
	case driver.GPUOpCreateBindGroup:
		if op.BindGroup != nil {
			entries := make(map[uint32]driver.GPUBindGroupEntry, len(op.BindGroup.Entries))
			for _, e := range op.BindGroup.Entries {
				entries[e.Binding] = e
			}
			c.bindGroups[op.ID] = &glGPUBindGroup{
				layout:  op.BindGroup.Layout,
				entries: entries,
			}
		}
	case driver.GPUOpDestroyBindGroup:
		delete(c.bindGroups, op.ID)
	case driver.GPUOpCreatePipeline:
		return c.createPipeline(op.ID, op.Pipeline)
	case driver.GPUOpDestroyPipeline:
		if p, ok := c.pipelines[op.ID]; ok {
			if p.vao != 0 {
				c.gl.DeleteVertexArray(p.vao)
			}
			if p.prog != 0 {
				c.gl.DeleteProgram(p.prog)
			}
			delete(c.pipelines, op.ID)
		}
	}
	return nil
}

func glBufferTarget(usage driver.GPUBufferUsage) uint32 {
	switch {
	case usage&driver.GPUBufferUniform != 0:
		return gpuGLUniformBuffer
	case usage&driver.GPUBufferIndex != 0:
		return gpuGLElementArrayBuffer
	default:
		return gpuGLArrayBuffer
	}
}

func (c *glGPUContext) createBuffer(id driver.GPUID, desc *driver.GPUBufferDesc) *glGPUError {
	if desc == nil {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "nil buffer descriptor"}
	}
	if old, ok := c.buffers[id]; ok && old.glID != 0 {
		c.gl.DeleteBuffer(old.glID)
	}
	glID := c.gl.GenBuffer()
	target := glBufferTarget(desc.Usage)
	usageHint := uint32(gpuGLStaticDraw)
	if desc.Usage&(driver.GPUBufferCopyDst|driver.GPUBufferUniform) != 0 {
		usageHint = gpuGLDynamicDraw
	}
	c.gl.BindBuffer(target, glID)
	size := int(desc.Size)
	if size == 0 {
		size = len(desc.Data)
	}
	if len(desc.Data) == size {
		c.gl.BufferData(target, size, desc.Data, usageHint)
	} else {
		c.gl.BufferData(target, size, nil, usageHint)
		if len(desc.Data) > 0 {
			c.gl.BufferSubData(target, 0, desc.Data)
		}
	}
	c.gl.BindBuffer(target, 0)
	c.buffers[id] = &glGPUBuffer{
		glID:   glID,
		size:   uint64(size),
		usage:  desc.Usage,
		target: target,
	}
	return nil
}

func (c *glGPUContext) writeBuffer(id driver.GPUID, offset uint64, data []byte) *glGPUError {
	b, ok := c.buffers[id]
	if !ok {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("unknown buffer %d", id)}
	}
	if len(data) == 0 {
		return nil
	}
	if offset+uint64(len(data)) > b.size {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("buffer %d write out of range", id)}
	}
	c.gl.BindBuffer(b.target, b.glID)
	c.gl.BufferSubData(b.target, int(offset), data)
	c.gl.BindBuffer(b.target, 0)
	return nil
}

func glTextureTarget(dim driver.GPUTextureDimension) uint32 {
	switch dim {
	case driver.GPUDimCube:
		return gpuGLTextureCubeMap
	case driver.GPUDim2DArray:
		return gpuGLTexture2DArray
	default:
		return gpuGLTexture2D
	}
}

func glTextureFormatInfo(f driver.GPUTextureFormat) (internalFmt int32, pixelFmt, pixelType uint32, bgraSwizzle, ok bool) {
	switch f {
	case driver.GPUFormatRGBA8Unorm:
		return gpuGLRGBA8, gpuGLRGBA, gpuGLUnsignedByte, false, true
	case driver.GPUFormatRGBA8UnormSRGB:
		return gpuGLSRGB8Alpha8, gpuGLRGBA, gpuGLUnsignedByte, false, true
	case driver.GPUFormatBGRA8Unorm:
		// Swizzle BGRA->RGBA on CPU upload so OpenGL 3.3 Core and ES 3.0 behave identically.
		return gpuGLRGBA8, gpuGLRGBA, gpuGLUnsignedByte, true, true
	case driver.GPUFormatRG8Unorm:
		return gpuGLRG8, gpuGLRG, gpuGLUnsignedByte, false, true
	case driver.GPUFormatR8Unorm:
		return gpuGLR8, gpuGLRed, gpuGLUnsignedByte, false, true
	case driver.GPUFormatRGBA16Float:
		return gpuGLRGBA16F, gpuGLRGBA, gpuGLHalfFloat, false, true
	case driver.GPUFormatDepth24Plus:
		return gpuGLDepthComponent24, gpuGLDepthComponent, gpuGLUnsignedInt, false, true
	case driver.GPUFormatDepth24PlusStencil8:
		return gpuGLDepth24Stencil8, gpuGLDepthStencil, gpuGLUnsignedInt248, false, true
	case driver.GPUFormatDepth32Float:
		return gpuGLDepthComponent32F, gpuGLDepthComponent, gpuGLFloat, false, true
	default:
		return 0, 0, 0, false, false
	}
}

func (c *glGPUContext) createTexture(id driver.GPUID, desc *driver.GPUTextureDesc) *glGPUError {
	if desc == nil {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "nil texture descriptor"}
	}
	internalFmt, pixelFmt, pixelType, _, ok := glTextureFormatInfo(desc.Format)
	if !ok {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("unsupported texture format %d", desc.Format)}
	}
	if old, exists := c.textures[id]; exists && old.glID != 0 {
		c.gl.DeleteTexture(old.glID)
	}
	glID := c.gl.GenTexture()
	target := glTextureTarget(desc.Dimension)
	mipLevels := max(desc.MipLevels, 1)
	layers := desc.Layers
	if layers == 0 {
		if desc.Dimension == driver.GPUDimCube {
			layers = 6
		} else {
			layers = 1
		}
	}
	c.gl.BindTexture(target, glID)
	c.gl.TexParameteri(target, gpuGLTextureBaseLevel, 0)
	c.gl.TexParameteri(target, gpuGLTextureMaxLevel, int32(mipLevels-1))
	c.gl.TexParameteri(target, gpuGLTextureMinFilter, gpuGLNearest)
	c.gl.TexParameteri(target, gpuGLTextureMagFilter, gpuGLNearest)
	c.gl.TexParameteri(target, gpuGLTextureWrapS, gpuGLClampToEdge)
	c.gl.TexParameteri(target, gpuGLTextureWrapT, gpuGLClampToEdge)
	if desc.Dimension == driver.GPUDimCube {
		c.gl.TexParameteri(target, gpuGLTextureWrapR, gpuGLClampToEdge)
	}
	for level := uint32(0); level < mipLevels; level++ {
		w := max(desc.Width>>level, 1)
		h := max(desc.Height>>level, 1)
		switch desc.Dimension {
		case driver.GPUDimCube:
			for face := uint32(0); face < 6; face++ {
				c.gl.TexImage2D(gpuGLTextureCubeMapPosX+face, int32(level), internalFmt, int32(w), int32(h), 0, pixelFmt, pixelType, nil)
			}
		case driver.GPUDim2DArray:
			c.gl.TexImage3D(gpuGLTexture2DArray, int32(level), internalFmt, int32(w), int32(h), int32(layers), 0, pixelFmt, pixelType, nil)
		default:
			c.gl.TexImage2D(gpuGLTexture2D, int32(level), internalFmt, int32(w), int32(h), 0, pixelFmt, pixelType, nil)
		}
	}
	c.gl.BindTexture(target, 0)
	c.textures[id] = &glGPUTexture{
		glID:      glID,
		target:    target,
		dim:       desc.Dimension,
		format:    desc.Format,
		width:     desc.Width,
		height:    desc.Height,
		layers:    layers,
		mipLevels: mipLevels,
	}
	return nil
}

func prepareTextureUpload(data []byte, format driver.GPUTextureFormat, width, height uint32, layout driver.GPUDataLayout) ([]byte, int32) {
	if layout.Offset >= uint64(len(data)) {
		return nil, 0
	}
	src := data[layout.Offset:]
	bpp := format.BytesPerPixel()
	if bpp == 0 || width == 0 || height == 0 {
		return src, 0
	}
	tightRow := width * bpp
	rowStride := layout.BytesPerRow
	if rowStride == 0 {
		rowStride = tightRow
	}
	needsSwizzle := format == driver.GPUFormatBGRA8Unorm
	if needsSwizzle || (rowStride != tightRow && rowStride%bpp != 0) {
		out := make([]byte, int(tightRow*height))
		for y := uint32(0); y < height; y++ {
			sOff := int(y * rowStride)
			if sOff >= len(src) {
				break
			}
			n := min(int(tightRow), len(src)-sOff)
			dOff := int(y * tightRow)
			copy(out[dOff:dOff+n], src[sOff:sOff+n])
			if needsSwizzle {
				for x := 0; x+3 < n; x += 4 {
					out[dOff+x], out[dOff+x+2] = out[dOff+x+2], out[dOff+x]
				}
			}
		}
		return out, 0
	}
	if rowStride != tightRow {
		return src, int32(rowStride / bpp)
	}
	return src, 0
}

func (c *glGPUContext) writeTexture(id driver.GPUID, dst driver.GPUTextureCopy, data []byte, layout driver.GPUDataLayout) *glGPUError {
	t, ok := c.textures[id]
	if !ok {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("unknown texture %d", id)}
	}
	_, pixelFmt, pixelType, _, ok := glTextureFormatInfo(t.format)
	if !ok {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("unsupported texture format %d", t.format)}
	}
	mipW := max(t.width>>dst.MipLevel, 1)
	mipH := max(t.height>>dst.MipLevel, 1)
	if dst.OriginX >= mipW || dst.OriginY >= mipH {
		return nil
	}
	uploadW := mipW - dst.OriginX
	uploadH := mipH - dst.OriginY
	if layout.RowsPerImage > 0 && layout.RowsPerImage < uploadH {
		uploadH = layout.RowsPerImage
	}
	pixels, rowLen := prepareTextureUpload(data, t.format, uploadW, uploadH, layout)
	if len(pixels) == 0 {
		return nil
	}
	c.gl.BindTexture(t.target, t.glID)
	c.gl.PixelStorei(gpuGLUnpackAlignment, 1)
	c.gl.PixelStorei(gpuGLUnpackRowLength, rowLen)
	switch t.dim {
	case driver.GPUDimCube:
		face := dst.ArrayLayer % 6
		c.gl.TexSubImage2D(gpuGLTextureCubeMapPosX+face, int32(dst.MipLevel), int32(dst.OriginX), int32(dst.OriginY), int32(uploadW), int32(uploadH), pixelFmt, pixelType, pixels)
	case driver.GPUDim2DArray:
		c.gl.TexSubImage3D(gpuGLTexture2DArray, int32(dst.MipLevel), int32(dst.OriginX), int32(dst.OriginY), int32(dst.ArrayLayer), int32(uploadW), int32(uploadH), 1, pixelFmt, pixelType, pixels)
	default:
		c.gl.TexSubImage2D(gpuGLTexture2D, int32(dst.MipLevel), int32(dst.OriginX), int32(dst.OriginY), int32(uploadW), int32(uploadH), pixelFmt, pixelType, pixels)
	}
	if rowLen != 0 {
		c.gl.PixelStorei(gpuGLUnpackRowLength, 0)
	}
	c.gl.BindTexture(t.target, 0)
	t.rendered = false
	return nil
}

func gpuGLMinFilterMode(minF, mipF driver.GPUFilterMode) int32 {
	if minF == driver.GPUFilterLinear {
		if mipF == driver.GPUFilterLinear {
			return gpuGLLinearMipmapLinear
		}
		return gpuGLLinearMipmapNearest
	}
	if mipF == driver.GPUFilterLinear {
		return gpuGLNearestMipmapLinear
	}
	return gpuGLNearestMipmapNearest
}

func gpuGLMagFilterMode(magF driver.GPUFilterMode) int32 {
	if magF == driver.GPUFilterLinear {
		return gpuGLLinear
	}
	return gpuGLNearest
}

func glAddressMode(a driver.GPUAddressMode) int32 {
	switch a {
	case driver.GPUAddressRepeat:
		return gpuGLRepeat
	case driver.GPUAddressMirrorRepeat:
		return gpuGLMirroredRepeat
	default:
		return gpuGLClampToEdge
	}
}

func glCompareFunc(f driver.GPUCompareFunc) uint32 {
	switch f {
	case driver.GPUCompareNever:
		return gpuGLNever
	case driver.GPUCompareLess:
		return gpuGLLess
	case driver.GPUCompareEqual:
		return gpuGLEqual
	case driver.GPUCompareLessEqual:
		return gpuGLLEqual
	case driver.GPUCompareGreater:
		return gpuGLGreater
	case driver.GPUCompareNotEqual:
		return gpuGLNotEqual
	case driver.GPUCompareGreaterEqual:
		return gpuGLGEqual
	default:
		return gpuGLAlways
	}
}

func (c *glGPUContext) createSampler(id driver.GPUID, desc *driver.GPUSamplerDesc) *glGPUError {
	if desc == nil {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "nil sampler descriptor"}
	}
	if old, ok := c.samplers[id]; ok && old.glID != 0 {
		c.gl.DeleteSampler(old.glID)
	}
	glID := c.gl.GenSampler()
	c.gl.SamplerParameteri(glID, gpuGLTextureMinFilter, gpuGLMinFilterMode(desc.MinFilter, desc.MipmapFilter))
	c.gl.SamplerParameteri(glID, gpuGLTextureMagFilter, gpuGLMagFilterMode(desc.MagFilter))
	c.gl.SamplerParameteri(glID, gpuGLTextureWrapS, glAddressMode(desc.AddressU))
	c.gl.SamplerParameteri(glID, gpuGLTextureWrapT, glAddressMode(desc.AddressV))
	c.gl.SamplerParameteri(glID, gpuGLTextureWrapR, glAddressMode(desc.AddressW))
	if desc.Compare != driver.GPUCompareUndefined {
		c.gl.SamplerParameteri(glID, gpuGLTextureCompareMode, gpuGLCompareRefToTexture)
		c.gl.SamplerParameteri(glID, gpuGLTextureCompareFunc, int32(glCompareFunc(desc.Compare)))
	} else {
		c.gl.SamplerParameteri(glID, gpuGLTextureCompareMode, gpuGLNone)
	}
	if desc.MaxAnisotropy > 1 && c.features.Anisotropy {
		c.gl.SamplerParameterf(glID, gpuGLTextureMaxAnisotropy, float32(desc.MaxAnisotropy))
	}
	c.samplers[id] = &glGPUSampler{glID: glID, desc: *desc}
	return nil
}

// compilePipelineGLSL emits GLSL 3.30 Core or GLSL ES 3.00 sources and binding tables from desc, handling both single-module and separate VS/FS modules.
func compilePipelineGLSL(desc *driver.GPUPipelineDesc, version glsl.GLSLVersion) (glsl.PipelineSource, error) {
	if desc == nil || desc.Vertex.IR == nil {
		return glsl.PipelineSource{}, fmt.Errorf("pipeline missing vertex shader IR")
	}
	if desc.Fragment.IR == nil || desc.Fragment.IR == desc.Vertex.IR {
		return glsl.CompilePipeline(desc.Vertex.IR, version)
	}
	vs, err := glsl.CompilePipeline(desc.Vertex.IR, version)
	if err != nil {
		return glsl.PipelineSource{}, err
	}
	fs, err := glsl.CompilePipeline(desc.Fragment.IR, version)
	if err != nil {
		return glsl.PipelineSource{}, err
	}
	out := glsl.PipelineSource{
		VertexSource:   vs.VertexSource,
		FragmentSource: fs.FragmentSource,
	}
	seenUB := map[string]bool{}
	var nextSlot uint32
	for _, ub := range append(slices.Clone(vs.UniformBlocks), fs.UniformBlocks...) {
		if seenUB[ub.BlockName] {
			continue
		}
		seenUB[ub.BlockName] = true
		ub.Slot = nextSlot
		nextSlot++
		out.UniformBlocks = append(out.UniformBlocks, ub)
	}
	seenSP := map[string]bool{}
	var nextUnit uint32
	for _, sp := range append(slices.Clone(vs.SamplerPairs), fs.SamplerPairs...) {
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

func (c *glGPUContext) createPipeline(id driver.GPUID, desc *driver.GPUPipelineDesc) *glGPUError {
	if desc == nil {
		return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "nil pipeline descriptor"}
	}
	ver := glsl.GLSLVersionDefault
	if c.es {
		ver = glsl.GLSLVersionES300
	}
	compiled, err := compilePipelineGLSL(desc, ver)
	if err != nil {
		return &glGPUError{code: driver.GPUErrorShaderCompile, message: err.Error()}
	}
	if compiled.VertexSource == "" {
		return &glGPUError{code: driver.GPUErrorShaderCompile, message: "empty vertex shader source"}
	}
	fragSrc := compiled.FragmentSource
	if fragSrc == "" {
		// Depth-only pass fallback fragment shader.
		if c.es {
			fragSrc = "#version 300 es\nprecision highp float;\nvoid main() {}\n"
		} else {
			fragSrc = "#version 330 core\nvoid main() {}\n"
		}
	}

	compileStage := func(kind uint32, src string) (uint32, string) {
		sh := c.gl.CreateShader(kind)
		c.gl.ShaderSource(sh, src)
		c.gl.CompileShader(sh)
		if c.gl.GetShaderiv(sh, gpuGLCompileStatus) == 0 {
			info := c.gl.GetShaderInfoLog(sh)
			c.gl.DeleteShader(sh)
			return 0, info
		}
		return sh, ""
	}

	vs, vsErr := compileStage(gpuGLVertexShader, compiled.VertexSource)
	if vs == 0 {
		return &glGPUError{code: driver.GPUErrorShaderCompile, message: fmt.Sprintf("vertex shader compile failed: %s", vsErr)}
	}
	fs, fsErr := compileStage(gpuGLFragmentShader, fragSrc)
	if fs == 0 {
		c.gl.DeleteShader(vs)
		return &glGPUError{code: driver.GPUErrorShaderCompile, message: fmt.Sprintf("fragment shader compile failed: %s", fsErr)}
	}

	prog := c.gl.CreateProgram()
	c.gl.AttachShader(prog, vs)
	c.gl.AttachShader(prog, fs)
	c.gl.LinkProgram(prog)
	c.gl.DeleteShader(vs)
	c.gl.DeleteShader(fs)
	if c.gl.GetProgramiv(prog, gpuGLLinkStatus) == 0 {
		info := c.gl.GetProgramInfoLog(prog)
		c.gl.DeleteProgram(prog)
		return &glGPUError{code: driver.GPUErrorShaderCompile, message: fmt.Sprintf("program link failed: %s", info)}
	}

	// Bind uniform block indices to flattened slots and combined sampler uniforms to texture units.
	c.gl.UseProgram(prog)
	for _, ub := range compiled.UniformBlocks {
		if idx := c.gl.GetUniformBlockIndex(prog, ub.BlockName); idx != gpuGLInvalidIndex {
			c.gl.UniformBlockBinding(prog, idx, ub.Slot)
		}
	}
	for _, sp := range compiled.SamplerPairs {
		if loc := c.gl.GetUniformLocation(prog, sp.Name); loc >= 0 {
			c.gl.Uniform1i(loc, int32(sp.Unit))
		}
	}
	c.gl.UseProgram(0)

	if old, exists := c.pipelines[id]; exists {
		if old.vao != 0 {
			c.gl.DeleteVertexArray(old.vao)
		}
		if old.prog != 0 {
			c.gl.DeleteProgram(old.prog)
		}
	}

	vao := c.gl.GenVertexArray()
	c.pipelines[id] = &glGPUPipeline{
		prog:          prog,
		vao:           vao,
		desc:          *desc,
		uniformBlocks: compiled.UniformBlocks,
		samplerPairs:  compiled.SamplerPairs,
	}
	return nil
}

func (c *glGPUContext) ensureFBO(fbo *uint32) uint32 {
	if *fbo == 0 {
		*fbo = c.gl.GenFramebuffer()
	}
	return *fbo
}

func (c *glGPUContext) attachTextureToFBO(target, attachment uint32, t *glGPUTexture, mipLevel, arrayLayer uint32) {
	switch t.dim {
	case driver.GPUDimCube:
		c.gl.FramebufferTexture2D(target, attachment, gpuGLTextureCubeMapPosX+(arrayLayer%6), t.glID, int32(mipLevel))
	case driver.GPUDim2DArray:
		c.gl.FramebufferTextureLayer(target, attachment, t.glID, int32(mipLevel), int32(arrayLayer))
	default:
		c.gl.FramebufferTexture2D(target, attachment, gpuGLTexture2D, t.glID, int32(mipLevel))
	}
}

func (c *glGPUContext) execCopy(op driver.GPUCopyOp, defaultFBO uint32) *glGPUError {
	switch op.Kind {
	case driver.GPUCopyBufferToBuffer:
		src, okS := c.buffers[op.SrcID]
		dst, okD := c.buffers[op.DstID]
		if !okS || !okD {
			return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "copy buffer: unknown buffer"}
		}
		if op.Size == 0 || op.SrcOffset+op.Size > src.size || op.DstOffset+op.Size > dst.size {
			return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "copy buffer: range out of bounds"}
		}
		c.gl.BindBuffer(gpuGLCopyReadBuffer, src.glID)
		c.gl.BindBuffer(gpuGLCopyWriteBuffer, dst.glID)
		c.gl.CopyBufferSubData(gpuGLCopyReadBuffer, gpuGLCopyWriteBuffer, int(op.SrcOffset), int(op.DstOffset), int(op.Size))
		c.gl.BindBuffer(gpuGLCopyReadBuffer, 0)
		c.gl.BindBuffer(gpuGLCopyWriteBuffer, 0)

	case driver.GPUCopyBufferToTexture:
		src, okS := c.buffers[op.SrcID]
		dst, okD := c.textures[op.DstID]
		if !okS || !okD {
			return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "copy buffer to texture: unknown resource"}
		}
		_, pixelFmt, pixelType, _, ok := glTextureFormatInfo(dst.format)
		if !ok {
			return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "copy buffer to texture: unsupported format"}
		}
		bpp := dst.format.BytesPerPixel()
		var rowLen int32
		if bpp > 0 && op.SrcLayout.BytesPerRow > 0 {
			rowLen = int32(op.SrcLayout.BytesPerRow / bpp)
		}
		c.gl.BindBuffer(gpuGLPixelUnpackBuffer, src.glID)
		c.gl.BindTexture(dst.target, dst.glID)
		c.gl.PixelStorei(gpuGLUnpackAlignment, 1)
		c.gl.PixelStorei(gpuGLUnpackRowLength, rowLen)
		switch dst.dim {
		case driver.GPUDimCube:
			face := op.DstTex.ArrayLayer % 6
			c.gl.TexSubImage2DOffset(gpuGLTextureCubeMapPosX+face, int32(op.DstTex.MipLevel), int32(op.DstTex.OriginX), int32(op.DstTex.OriginY), int32(op.Width), int32(op.Height), pixelFmt, pixelType, uintptr(op.SrcLayout.Offset))
		case driver.GPUDim2DArray:
			c.gl.TexSubImage3DOffset(gpuGLTexture2DArray, int32(op.DstTex.MipLevel), int32(op.DstTex.OriginX), int32(op.DstTex.OriginY), int32(op.DstTex.ArrayLayer), int32(op.Width), int32(op.Height), int32(max(op.Layers, 1)), pixelFmt, pixelType, uintptr(op.SrcLayout.Offset))
		default:
			c.gl.TexSubImage2DOffset(gpuGLTexture2D, int32(op.DstTex.MipLevel), int32(op.DstTex.OriginX), int32(op.DstTex.OriginY), int32(op.Width), int32(op.Height), pixelFmt, pixelType, uintptr(op.SrcLayout.Offset))
		}
		if rowLen != 0 {
			c.gl.PixelStorei(gpuGLUnpackRowLength, 0)
		}
		c.gl.BindTexture(dst.target, 0)
		c.gl.BindBuffer(gpuGLPixelUnpackBuffer, 0)
		dst.rendered = false

	case driver.GPUCopyTextureToTexture:
		src, okS := c.textures[op.SrcID]
		dst, okD := c.textures[op.DstID]
		if !okS || !okD {
			return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: "copy texture to texture: unknown texture"}
		}
		rFBO := c.ensureFBO(&c.readFBO)
		dFBO := c.ensureFBO(&c.drawFBO)
		c.gl.BindFramebuffer(gpuGLReadFramebuffer, rFBO)
		c.gl.BindFramebuffer(gpuGLDrawFramebuffer, dFBO)
		layers := max(op.Layers, 1)
		for l := uint32(0); l < layers; l++ {
			c.attachTextureToFBO(gpuGLReadFramebuffer, gpuGLColorAttachment0, src, op.SrcTex.MipLevel, op.SrcTex.ArrayLayer+l)
			c.attachTextureToFBO(gpuGLDrawFramebuffer, gpuGLColorAttachment0, dst, op.DstTex.MipLevel, op.DstTex.ArrayLayer+l)
			c.gl.BlitFramebuffer(
				int32(op.SrcTex.OriginX), int32(op.SrcTex.OriginY), int32(op.SrcTex.OriginX+op.Width), int32(op.SrcTex.OriginY+op.Height),
				int32(op.DstTex.OriginX), int32(op.DstTex.OriginY), int32(op.DstTex.OriginX+op.Width), int32(op.DstTex.OriginY+op.Height),
				gpuGLColorBufferBit, gpuGLNearest,
			)
		}
		dst.rendered = src.rendered
		c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
	}
	return nil
}

type glVertexFormatSpec struct {
	components int32
	glType     uint32
	normalized bool
	integer    bool
}

func glVertexFormatInfo(f driver.GPUVertexFormat) (glVertexFormatSpec, bool) {
	switch f {
	case driver.GPUVertexFloat32:
		return glVertexFormatSpec{1, gpuGLFloat, false, false}, true
	case driver.GPUVertexFloat32x2:
		return glVertexFormatSpec{2, gpuGLFloat, false, false}, true
	case driver.GPUVertexFloat32x3:
		return glVertexFormatSpec{3, gpuGLFloat, false, false}, true
	case driver.GPUVertexFloat32x4:
		return glVertexFormatSpec{4, gpuGLFloat, false, false}, true
	case driver.GPUVertexSint32:
		return glVertexFormatSpec{1, gpuGLInt, false, true}, true
	case driver.GPUVertexSint32x2:
		return glVertexFormatSpec{2, gpuGLInt, false, true}, true
	case driver.GPUVertexSint32x3:
		return glVertexFormatSpec{3, gpuGLInt, false, true}, true
	case driver.GPUVertexSint32x4:
		return glVertexFormatSpec{4, gpuGLInt, false, true}, true
	case driver.GPUVertexUint32:
		return glVertexFormatSpec{1, gpuGLUnsignedInt, false, true}, true
	case driver.GPUVertexUint32x2:
		return glVertexFormatSpec{2, gpuGLUnsignedInt, false, true}, true
	case driver.GPUVertexUint32x3:
		return glVertexFormatSpec{3, gpuGLUnsignedInt, false, true}, true
	case driver.GPUVertexUint32x4:
		return glVertexFormatSpec{4, gpuGLUnsignedInt, false, true}, true
	case driver.GPUVertexUnorm8x4:
		return glVertexFormatSpec{4, gpuGLUnsignedByte, true, false}, true
	default:
		return glVertexFormatSpec{}, false
	}
}

func glPrimitiveTopology(t driver.GPUPrimitiveTopology) uint32 {
	switch t {
	case driver.GPUTopologyTriangleStrip:
		return gpuGLTriangleStrip
	case driver.GPUTopologyLineList:
		return gpuGLLines
	case driver.GPUTopologyLineStrip:
		return gpuGLLineStrip
	case driver.GPUTopologyPointList:
		return gpuGLPoints
	default:
		return gpuGLTriangles
	}
}

func glBlendFactor(f driver.GPUBlendFactor) uint32 {
	switch f {
	case driver.GPUBlendZero:
		return gpuGLZero
	case driver.GPUBlendOne:
		return gpuGLOne
	case driver.GPUBlendSrc:
		return gpuGLSrcColor
	case driver.GPUBlendOneMinusSrc:
		return gpuGLOneMinusSrcColor
	case driver.GPUBlendSrcAlpha:
		return gpuGLSrcAlpha
	case driver.GPUBlendOneMinusSrcAlpha:
		return gpuGLOneMinusSrcAlpha
	case driver.GPUBlendDst:
		return gpuGLDstColor
	case driver.GPUBlendOneMinusDst:
		return gpuGLOneMinusDstColor
	case driver.GPUBlendDstAlpha:
		return gpuGLDstAlpha
	case driver.GPUBlendOneMinusDstAlpha:
		return gpuGLOneMinusDstAlpha
	case driver.GPUBlendConstant:
		return gpuGLConstantColor
	case driver.GPUBlendOneMinusConstant:
		return gpuGLOneMinusConstantColor
	default:
		return gpuGLOne
	}
}

func glBlendOperation(op driver.GPUBlendOperation) uint32 {
	switch op {
	case driver.GPUBlendSubtract:
		return gpuGLFuncSubtract
	case driver.GPUBlendReverseSubtract:
		return gpuGLFuncReverseSubtract
	case driver.GPUBlendMin:
		return gpuGLMin
	case driver.GPUBlendMax:
		return gpuGLMax
	default:
		return gpuGLFuncAdd
	}
}

// glFlipYRect converts a WebGPU top-left rect (y, h) inside a target of height targetH into an OpenGL bottom-left Y coordinate.
func glFlipYRect(y, h, targetH int32) int32 {
	return targetH - (y + h)
}

type boundBuffer struct {
	id     driver.GPUID
	offset uint64
}

type glPassState struct {
	pipe           *glGPUPipeline
	bindGroups     map[uint32]driver.GPUID
	dynamicOffsets map[uint32][]uint32
	vertexBuffers  map[uint32]boundBuffer
	indexBuffer    boundBuffer
	indexFormat    driver.GPUIndexFormat
}

// passIsOffscreen reports whether pass targets offscreen texture attachments rather than the view's swapchain framebuffer.
func passIsOffscreen(pass driver.GPUPass) bool {
	for _, ca := range pass.ColorAttachments {
		if ca.Texture != 0 {
			return true
		}
	}
	return pass.DepthStencil != nil && pass.DepthStencil.Texture != 0
}

func (c *glGPUContext) execPass(pass driver.GPUPass, defaultFBO uint32, surfaceW, surfaceH int32) *glGPUError {
	targetW, targetH := surfaceW, surfaceH
	isOffscreen := passIsOffscreen(pass)

	if !isOffscreen {
		c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
	} else {
		fbo := c.ensureFBO(&c.scratchFBO)
		c.gl.BindFramebuffer(gpuGLFramebuffer, fbo)
		drawBufs := make([]uint32, 0, len(pass.ColorAttachments))
		var firstSize bool
		for i, ca := range pass.ColorAttachments {
			if ca.Texture == 0 {
				continue
			}
			t, ok := c.textures[ca.Texture]
			if !ok {
				c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
				return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("render pass unknown color texture %d", ca.Texture)}
			}
			attach := gpuGLColorAttachment0 + uint32(i)
			c.attachTextureToFBO(gpuGLFramebuffer, attach, t, ca.MipLevel, ca.ArrayLayer)
			t.rendered = true
			drawBufs = append(drawBufs, attach)
			if !firstSize {
				targetW = int32(max(t.width>>ca.MipLevel, 1))
				targetH = int32(max(t.height>>ca.MipLevel, 1))
				firstSize = true
			}
		}
		// Unbind any extra color attachments from a previous MRT pass.
		for i := len(drawBufs); i < c.lastOffscreenColors; i++ {
			c.gl.FramebufferTexture2D(gpuGLFramebuffer, gpuGLColorAttachment0+uint32(i), gpuGLTexture2D, 0, 0)
		}
		c.lastOffscreenColors = len(drawBufs)
		if len(drawBufs) > 0 {
			c.gl.DrawBuffers(drawBufs)
		} else {
			c.gl.DrawBuffers([]uint32{gpuGLNone})
		}

		// Detach previous depth/stencil before attaching this pass's depth target.
		c.gl.FramebufferTexture2D(gpuGLFramebuffer, gpuGLDepthStencilAttach, gpuGLTexture2D, 0, 0)
		if pass.DepthStencil != nil && pass.DepthStencil.Texture != 0 {
			dt, ok := c.textures[pass.DepthStencil.Texture]
			if !ok {
				c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
				return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("render pass unknown depth texture %d", pass.DepthStencil.Texture)}
			}
			attach := uint32(gpuGLDepthAttachment)
			if dt.format == driver.GPUFormatDepth24PlusStencil8 {
				attach = gpuGLDepthStencilAttach
			}
			c.attachTextureToFBO(gpuGLFramebuffer, attach, dt, 0, 0)
			if !firstSize {
				targetW = int32(max(dt.width, 1))
				targetH = int32(max(dt.height, 1))
			}
		}
		if status := c.gl.CheckFramebufferStatus(gpuGLFramebuffer); status != gpuGLFramebufferComplete {
			c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
			return &glGPUError{code: driver.GPUErrorFailed, message: fmt.Sprintf("incomplete offscreen framebuffer: 0x%x", status)}
		}
	}

	// Default full-target viewport and unclipped clears.
	c.gl.Disable(gpuGLScissorTest)
	c.gl.Viewport(0, 0, targetW, targetH)
	c.gl.DepthRangef(0, 1)
	c.gl.ColorMask(true, true, true, true)
	c.gl.DepthMask(true)

	var clearMask uint32
	if len(pass.ColorAttachments) > 0 && pass.ColorAttachments[0].LoadOp == driver.GPULoadClear {
		r, g, b, a := pass.ColorAttachments[0].ClearColor.Components()
		c.gl.ClearColor(float32(r), float32(g), float32(b), float32(a))
		clearMask |= gpuGLColorBufferBit
	}
	if pass.DepthStencil != nil {
		if pass.DepthStencil.DepthLoadOp == driver.GPULoadClear {
			c.gl.ClearDepthf(pass.DepthStencil.ClearDepth)
			clearMask |= gpuGLDepthBufferBit
		}
		if pass.DepthStencil.StencilLoadOp == driver.GPULoadClear {
			c.gl.ClearStencil(int32(pass.DepthStencil.ClearStencil))
			clearMask |= gpuGLStencilBufferBit
		}
	}
	if clearMask != 0 {
		c.gl.Clear(clearMask)
	}

	state := glPassState{
		bindGroups:     map[uint32]driver.GPUID{},
		dynamicOffsets: map[uint32][]uint32{},
		vertexBuffers:  map[uint32]boundBuffer{},
	}

	for _, cmd := range pass.Commands {
		switch cmd.Kind {
		case driver.GPUCmdSetViewport:
			vx, vy, vw, vh := int32(cmd.F0), int32(cmd.F1), int32(cmd.F2), int32(cmd.F3)
			c.gl.Viewport(vx, glFlipYRect(vy, vh, targetH), vw, vh)
			c.gl.DepthRangef(cmd.F4, cmd.F5)

		case driver.GPUCmdSetScissor:
			c.gl.Enable(gpuGLScissorTest)
			c.gl.Scissor(cmd.I0, glFlipYRect(cmd.I1, cmd.I3, targetH), cmd.I2, cmd.I3)

		case driver.GPUCmdSetPipeline:
			p, ok := c.pipelines[cmd.ID]
			if !ok {
				return &glGPUError{code: driver.GPUErrorInvalidDescriptor, message: fmt.Sprintf("unknown pipeline %d", cmd.ID)}
			}
			state.pipe = p
			c.applyPipelineState(p)

		case driver.GPUCmdSetBindGroup:
			state.bindGroups[cmd.Slot] = cmd.ID
			state.dynamicOffsets[cmd.Slot] = cmd.DynamicOffsets

		case driver.GPUCmdSetVertexBuffer:
			state.vertexBuffers[cmd.Slot] = boundBuffer{id: cmd.ID, offset: cmd.Offset}

		case driver.GPUCmdSetIndexBuffer:
			state.indexBuffer = boundBuffer{id: cmd.ID, offset: cmd.Offset}
			state.indexFormat = cmd.IndexFormat

		case driver.GPUCmdSetBlendConstant:
			c.gl.BlendColor(cmd.F0, cmd.F1, cmd.F2, cmd.F3)

		case driver.GPUCmdSetStencilReference:
			// Reserved for stencil test follow-ups.

		case driver.GPUCmdDraw:
			if state.pipe == nil {
				continue
			}
			c.bindResourcesAndVAO(&state, 0, 0, false)
			mode := glPrimitiveTopology(state.pipe.desc.Primitive.Topology)
			c.gl.DrawArrays(mode, int32(cmd.U1), int32(cmd.U0))

		case driver.GPUCmdDrawIndexed:
			if state.pipe == nil {
				continue
			}
			emulateBase := cmd.I0 != 0 && !c.hasBaseVertex
			c.bindResourcesAndVAO(&state, cmd.I0, 0, true)
			mode := glPrimitiveTopology(state.pipe.desc.Primitive.Topology)
			idxType, idxBytes := glIndexType(state.indexFormat)
			byteOff := uintptr(state.indexBuffer.offset + uint64(cmd.U1)*idxBytes)
			if cmd.I0 == 0 || emulateBase {
				c.gl.DrawElements(mode, int32(cmd.U0), idxType, byteOff)
			} else {
				c.gl.DrawElementsBaseVertex(mode, int32(cmd.U0), idxType, byteOff, cmd.I0)
			}

		case driver.GPUCmdDrawInstanced:
			if state.pipe == nil {
				continue
			}
			c.bindResourcesAndVAO(&state, 0, cmd.U3, false)
			mode := glPrimitiveTopology(state.pipe.desc.Primitive.Topology)
			c.gl.DrawArraysInstanced(mode, int32(cmd.U2), int32(cmd.U0), int32(cmd.U1))

		case driver.GPUCmdDrawIndexedInstanced:
			if state.pipe == nil {
				continue
			}
			emulateBase := cmd.I0 != 0 && !c.hasBaseVertex
			c.bindResourcesAndVAO(&state, cmd.I0, cmd.U3, true)
			mode := glPrimitiveTopology(state.pipe.desc.Primitive.Topology)
			idxType, idxBytes := glIndexType(state.indexFormat)
			byteOff := uintptr(state.indexBuffer.offset + uint64(cmd.U2)*idxBytes)
			if cmd.I0 == 0 || emulateBase {
				c.gl.DrawElementsInstanced(mode, int32(cmd.U0), idxType, byteOff, int32(cmd.U1))
			} else {
				c.gl.DrawElementsInstancedBaseVertex(mode, int32(cmd.U0), idxType, byteOff, int32(cmd.U1), cmd.I0)
			}
		}
	}

	if isOffscreen {
		c.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
	}
	return nil
}

func glIndexType(f driver.GPUIndexFormat) (glType uint32, elemBytes uint64) {
	if f == driver.GPUIndexUint32 {
		return gpuGLUnsignedInt, 4
	}
	return gpuGLUnsignedShort, 2
}

func (c *glGPUContext) applyPipelineState(p *glGPUPipeline) {
	c.gl.UseProgram(p.prog)

	// Primitive winding & culling.
	if p.desc.Primitive.FrontFace == driver.GPUFrontCW {
		c.gl.FrontFace(gpuGLCW)
	} else {
		c.gl.FrontFace(gpuGLCCW)
	}
	switch p.desc.Primitive.CullMode {
	case driver.GPUCullFront:
		c.gl.Enable(gpuGLCullFace)
		c.gl.CullFace(gpuGLFront)
	case driver.GPUCullBack:
		c.gl.Enable(gpuGLCullFace)
		c.gl.CullFace(gpuGLBack)
	default:
		c.gl.Disable(gpuGLCullFace)
	}

	// Depth state.
	if ds := p.desc.DepthStencil; ds != nil {
		c.gl.Enable(gpuGLDepthTest)
		c.gl.DepthMask(ds.DepthWrite)
		c.gl.DepthFunc(glCompareFunc(ds.DepthCompare))
		if ds.DepthBias != 0 || ds.DepthBiasSlopeScale != 0 {
			c.gl.Enable(gpuGLPolygonOffsetFill)
			c.gl.PolygonOffset(ds.DepthBiasSlopeScale, float32(ds.DepthBias))
		} else {
			c.gl.Disable(gpuGLPolygonOffsetFill)
		}
	} else {
		c.gl.Disable(gpuGLDepthTest)
		c.gl.DepthMask(false)
		c.gl.Disable(gpuGLPolygonOffsetFill)
	}

	// Color write mask and blend state (attachment 0).
	mask := driver.GPUColorWriteAll
	var blend *driver.GPUBlendState
	if len(p.desc.Targets) > 0 {
		if p.desc.Targets[0].WriteMask != 0 {
			mask = p.desc.Targets[0].WriteMask
		}
		blend = p.desc.Targets[0].Blend
	}
	c.gl.ColorMask(
		mask&driver.GPUColorWriteRed != 0,
		mask&driver.GPUColorWriteGreen != 0,
		mask&driver.GPUColorWriteBlue != 0,
		mask&driver.GPUColorWriteAlpha != 0,
	)
	if blend != nil {
		c.gl.Enable(gpuGLBlend)
		c.gl.BlendFuncSeparate(
			glBlendFactor(blend.Color.SrcFactor),
			glBlendFactor(blend.Color.DstFactor),
			glBlendFactor(blend.Alpha.SrcFactor),
			glBlendFactor(blend.Alpha.DstFactor),
		)
		c.gl.BlendEquationSeparate(
			glBlendOperation(blend.Color.Operation),
			glBlendOperation(blend.Alpha.Operation),
		)
	} else {
		c.gl.Disable(gpuGLBlend)
	}
}

func (c *glGPUContext) dynamicOffsetFor(bg *glGPUBindGroup, binding uint32, offsets []uint32) uint64 {
	if bg == nil || len(offsets) == 0 || bg.layout == 0 {
		return 0
	}
	layout, ok := c.layouts[bg.layout]
	if !ok {
		return 0
	}
	// WebGPU orders dynamic offsets by ascending binding number in the layout.
	var dynIdx int
	var hasDyn bool
	for _, le := range layout.Entries {
		if !le.HasDynamicOffset {
			continue
		}
		if le.Binding == binding {
			hasDyn = true
		} else if le.Binding < binding {
			dynIdx++
		}
	}
	if !hasDyn || dynIdx >= len(offsets) {
		return 0
	}
	return uint64(offsets[dynIdx])
}

func (c *glGPUContext) bindResourcesAndVAO(state *glPassState, baseVertex int32, firstInstance uint32, indexed bool) {
	p := state.pipe

	// 1. Bind Uniform Buffer Objects (std140) per flattened uniform block slot.
	for _, ub := range p.uniformBlocks {
		bgID, ok := state.bindGroups[ub.Group]
		if !ok {
			continue
		}
		bg, ok := c.bindGroups[bgID]
		if !ok {
			continue
		}
		entry, ok := bg.entries[ub.Binding]
		if !ok || entry.Buffer == 0 {
			continue
		}
		buf, ok := c.buffers[entry.Buffer]
		if !ok {
			continue
		}
		dynOff := c.dynamicOffsetFor(bg, ub.Binding, state.dynamicOffsets[ub.Group])
		offset := entry.Offset + dynOff
		size := entry.Size
		if size == 0 && offset < buf.size {
			size = buf.size - offset
		}
		if offset == 0 && size == buf.size {
			c.gl.BindBufferBase(gpuGLUniformBuffer, ub.Slot, buf.glID)
		} else if offset < buf.size && size > 0 {
			if offset+size > buf.size {
				size = buf.size - offset
			}
			c.gl.BindBufferRange(gpuGLUniformBuffer, ub.Slot, buf.glID, int(offset), int(size))
		}
	}

	// 2. Bind synthesized combined sampler2D (texture, sampler) pairs.
	for _, sp := range p.samplerPairs {
		c.gl.ActiveTexture(gpuGLTexture0 + sp.Unit)
		var tex *glGPUTexture
		if bgID, ok := state.bindGroups[sp.TextureGroup]; ok {
			if bg, ok := c.bindGroups[bgID]; ok {
				if entry, ok := bg.entries[sp.TextureBinding]; ok && entry.Texture != 0 {
					tex = c.textures[entry.Texture]
				}
			}
		}
		if tex != nil {
			c.gl.BindTexture(tex.target, tex.glID)
		} else {
			c.gl.BindTexture(gpuGLTexture2D, 0)
		}
		var samp *glGPUSampler
		if bgID, ok := state.bindGroups[sp.SamplerGroup]; ok {
			if bg, ok := c.bindGroups[bgID]; ok {
				if entry, ok := bg.entries[sp.SamplerBinding]; ok && entry.Sampler != 0 {
					samp = c.samplers[entry.Sampler]
				}
			}
		}
		if samp != nil {
			c.gl.BindSampler(sp.Unit, samp.glID)
		} else {
			c.gl.BindSampler(sp.Unit, 0)
		}
	}

	// 3. Configure pipeline VAO vertex attributes and optional index buffer.
	c.gl.BindVertexArray(p.vao)
	emulateBase := baseVertex != 0 && !c.hasBaseVertex
	for slot, vbLayout := range p.desc.VertexBuffers {
		bound, ok := state.vertexBuffers[uint32(slot)]
		if !ok || bound.id == 0 {
			continue
		}
		buf, ok := c.buffers[bound.id]
		if !ok {
			continue
		}
		c.gl.BindBuffer(gpuGLArrayBuffer, buf.glID)
		var extra int64
		var divisor uint32
		if vbLayout.StepMode == driver.GPUStepInstance {
			divisor = 1
			extra = int64(uint64(firstInstance) * vbLayout.Stride)
		} else if emulateBase {
			extra = int64(baseVertex) * int64(vbLayout.Stride)
		}
		for _, attr := range vbLayout.Attributes {
			spec, ok := glVertexFormatInfo(attr.Format)
			if !ok {
				continue
			}
			c.gl.EnableVertexAttribArray(attr.Location)
			signedOff := max(int64(bound.offset+attr.Offset)+extra, 0)
			off := uintptr(signedOff)
			if spec.integer {
				c.gl.VertexAttribIPointer(attr.Location, spec.components, spec.glType, int32(vbLayout.Stride), off)
			} else {
				c.gl.VertexAttribPointer(attr.Location, spec.components, spec.glType, spec.normalized, int32(vbLayout.Stride), off)
			}
			c.gl.VertexAttribDivisor(attr.Location, divisor)
		}
	}
	if indexed && state.indexBuffer.id != 0 {
		if ibuf, ok := c.buffers[state.indexBuffer.id]; ok {
			c.gl.BindBuffer(gpuGLElementArrayBuffer, ibuf.glID)
		}
	}
}

func flipRowsRGBA(pixels []byte, width, height int) {
	stride := width * 4
	if stride <= 0 || height <= 1 || len(pixels) < stride*height {
		return
	}
	tmp := make([]byte, stride)
	for y := 0; y < height/2; y++ {
		top := y * stride
		bot := (height - 1 - y) * stride
		copy(tmp, pixels[top:top+stride])
		copy(pixels[top:top+stride], pixels[bot:bot+stride])
		copy(pixels[bot:bot+stride], tmp)
	}
}

func (c *glGPUContext) execReadback(op driver.GPUResourceOp, defaultFBO uint32, surfaceW, surfaceH int32) glGPUReadback {
	reg := op.ReadbackRegion
	var targetW, targetH uint32
	flipY := true

	if op.ReadbackSurface {
		targetW = uint32(max(surfaceW, 0))
		targetH = uint32(max(surfaceH, 0))
		c.gl.BindFramebuffer(gpuGLReadFramebuffer, defaultFBO)
	} else {
		t, ok := c.textures[op.ID]
		if !ok {
			return glGPUReadback{id: op.ReadbackID, errMsg: fmt.Sprintf("unknown texture %d", op.ID)}
		}
		targetW = max(t.width>>reg.MipLevel, 1)
		targetH = max(t.height>>reg.MipLevel, 1)
		rFBO := c.ensureFBO(&c.readFBO)
		c.gl.BindFramebuffer(gpuGLReadFramebuffer, rFBO)
		c.attachTextureToFBO(gpuGLReadFramebuffer, gpuGLColorAttachment0, t, reg.MipLevel, reg.ArrayLayer)
		c.gl.ReadBuffer(gpuGLColorAttachment0)
		flipY = t.rendered
	}

	if reg.X >= targetW || reg.Y >= targetH {
		c.gl.BindFramebuffer(gpuGLReadFramebuffer, defaultFBO)
		return glGPUReadback{id: op.ReadbackID}
	}
	w := reg.Width
	if w == 0 || reg.X+w > targetW {
		w = targetW - reg.X
	}
	h := reg.Height
	if h == 0 || reg.Y+h > targetH {
		h = targetH - reg.Y
	}
	if w == 0 || h == 0 {
		c.gl.BindFramebuffer(gpuGLReadFramebuffer, defaultFBO)
		return glGPUReadback{id: op.ReadbackID}
	}

	pixels := make([]byte, int(w*h*4))
	c.gl.PixelStorei(gpuGLPackAlignment, 1)
	c.gl.PixelStorei(gpuGLPackRowLength, 0)
	glY := int32(reg.Y)
	if flipY {
		glY = glFlipYRect(int32(reg.Y), int32(h), int32(targetH))
	}
	c.gl.ReadPixels(int32(reg.X), glY, int32(w), int32(h), gpuGLRGBA, gpuGLUnsignedByte, pixels)
	c.gl.BindFramebuffer(gpuGLReadFramebuffer, defaultFBO)
	if flipY {
		flipRowsRGBA(pixels, int(w), int(h))
	}
	return glGPUReadback{id: op.ReadbackID, pixels: pixels}
}
