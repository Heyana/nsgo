package linux

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

type mockAttribCall struct {
	vao        uint32
	vbo        uint32
	index      uint32
	size       int32
	typ        uint32
	normalized bool
	integer    bool
	stride     int32
	offset     uintptr
	divisor    uint32
}

type mockDrawCall struct {
	kind          string
	mode          uint32
	first         int32
	count         int32
	instanceCount int32
	typ           uint32
	offset        uintptr
	baseVertex    int32
	vao           uint32
	ibo           uint32
	prog          uint32
	fbo           uint32
}

type mockUBOBind struct {
	slot   uint32
	buf    uint32
	offset int
	size   int
	ranged bool
}

type mockSamplerBind struct {
	unit    uint32
	target  uint32
	tex     uint32
	sampler uint32
}

type mockTexUpload struct {
	target uint32
	level  int32
	x, y   int32
	z      int32
	w, h   int32
	d      int32
	rowLen int32
	data   []byte
	pboOff uintptr
	isPBO  bool
}

type mockGL struct {
	nextID uint32

	liveBuffers  map[uint32][]byte
	liveTextures map[uint32]uint32 // glID -> target
	liveSamplers map[uint32]map[uint32]int32
	liveVAOs     map[uint32]bool
	liveShaders  map[uint32]string
	livePrograms map[uint32]bool
	liveFBOs     map[uint32]bool

	failShaderKind uint32
	failLink       bool
	fboStatus      uint32

	boundBuffers   map[uint32]uint32
	boundVAO       uint32
	boundProg      uint32
	boundFBO       uint32
	boundReadFBO   uint32
	boundDrawFBO   uint32
	activeTexUnit  uint32
	boundTextures  map[uint32]uint32 // unit -> glID
	boundSamplers  map[uint32]uint32 // unit -> glID
	unpackRowLen   int32
	enabledCaps    map[uint32]bool
	viewport       [4]int32
	depthRange     [2]float32
	scissor        [4]int32
	cullFace       uint32
	frontFace      uint32
	depthMask      bool
	depthFunc      uint32
	polyOffset     [2]float32
	colorMask      [4]bool
	blendFuncs     [4]uint32
	blendOps       [2]uint32
	blendColor     [4]float32
	clearColor     [4]float32
	clearDepth     float32
	clearMasks     []uint32
	drawBufs       []uint32
	fboAttachments map[[2]uint32]uint32 // (fbo, attach) -> texID

	shaderSources    []string
	ubBlockBindings  map[string]uint32
	samplerUniforms  map[string]int32
	attribCalls      []mockAttribCall
	uboBinds         []mockUBOBind
	samplerBinds     []mockSamplerBind
	texUploads       []mockTexUpload
	drawCalls        []mockDrawCall
	copyBufCalls     int
	blitCalls        int
	readPixelsCalls  [][4]int32
	readPixelsSource []byte
	eglImageTargets  []uintptr
}

func newMockGL() *mockGL {
	return &mockGL{
		liveBuffers:     map[uint32][]byte{},
		liveTextures:    map[uint32]uint32{},
		liveSamplers:    map[uint32]map[uint32]int32{},
		liveVAOs:        map[uint32]bool{},
		liveShaders:     map[uint32]string{},
		livePrograms:    map[uint32]bool{},
		liveFBOs:        map[uint32]bool{},
		fboStatus:       gpuGLFramebufferComplete,
		boundBuffers:    map[uint32]uint32{},
		boundTextures:   map[uint32]uint32{},
		boundSamplers:   map[uint32]uint32{},
		enabledCaps:     map[uint32]bool{},
		fboAttachments:  map[[2]uint32]uint32{},
		ubBlockBindings: map[string]uint32{},
		samplerUniforms: map[string]int32{},
	}
}

func (m *mockGL) alloc() uint32 {
	m.nextID++
	return m.nextID
}

func (m *mockGL) CreateShader(kind uint32) uint32 {
	id := m.alloc()
	m.liveShaders[id] = ""
	if kind == m.failShaderKind {
		m.liveShaders[id] = "FAIL"
	}
	return id
}
func (m *mockGL) ShaderSource(shader uint32, src string) {
	m.shaderSources = append(m.shaderSources, src)
	if m.liveShaders[shader] != "FAIL" {
		m.liveShaders[shader] = src
	}
}
func (m *mockGL) CompileShader(uint32) {}
func (m *mockGL) GetShaderiv(shader, pname uint32) int32 {
	if pname == gpuGLCompileStatus && m.liveShaders[shader] == "FAIL" {
		return 0
	}
	return 1
}
func (m *mockGL) GetShaderInfoLog(uint32) string { return "mock shader error" }
func (m *mockGL) DeleteShader(shader uint32)     { delete(m.liveShaders, shader) }
func (m *mockGL) CreateProgram() uint32 {
	id := m.alloc()
	m.livePrograms[id] = true
	return id
}
func (m *mockGL) AttachShader(uint32, uint32) {}
func (m *mockGL) LinkProgram(uint32)          {}
func (m *mockGL) GetProgramiv(prog, pname uint32) int32 {
	if pname == gpuGLLinkStatus && m.failLink {
		return 0
	}
	return 1
}
func (m *mockGL) GetProgramInfoLog(uint32) string { return "mock link error" }
func (m *mockGL) DeleteProgram(prog uint32)       { delete(m.livePrograms, prog) }
func (m *mockGL) UseProgram(prog uint32)          { m.boundProg = prog }
func (m *mockGL) GetUniformLocation(prog uint32, name string) int32 {
	// Return deterministic non-negative location for each sampler name.
	m.samplerUniforms[name] = -1
	return int32(len(m.samplerUniforms))
}
func (m *mockGL) Uniform1i(loc, v int32) {
	for k, cur := range m.samplerUniforms {
		if cur == -1 {
			m.samplerUniforms[k] = v
			return
		}
	}
}
func (m *mockGL) GetUniformBlockIndex(prog uint32, name string) uint32 {
	m.ubBlockBindings[name] = gpuGLInvalidIndex
	return uint32(len(m.ubBlockBindings))
}
func (m *mockGL) UniformBlockBinding(prog, index, binding uint32) {
	for k, cur := range m.ubBlockBindings {
		if cur == gpuGLInvalidIndex {
			m.ubBlockBindings[k] = binding
			return
		}
	}
}

func (m *mockGL) GenBuffer() uint32 {
	id := m.alloc()
	m.liveBuffers[id] = nil
	return id
}
func (m *mockGL) DeleteBuffer(id uint32)       { delete(m.liveBuffers, id) }
func (m *mockGL) BindBuffer(target, id uint32) { m.boundBuffers[target] = id }
func (m *mockGL) BufferData(target uint32, size int, data []byte, usage uint32) {
	id := m.boundBuffers[target]
	buf := make([]byte, size)
	copy(buf, data)
	m.liveBuffers[id] = buf
}
func (m *mockGL) BufferSubData(target uint32, offset int, data []byte) {
	id := m.boundBuffers[target]
	copy(m.liveBuffers[id][offset:], data)
}
func (m *mockGL) BindBufferBase(target, index, id uint32) {
	m.uboBinds = append(m.uboBinds, mockUBOBind{slot: index, buf: id, size: len(m.liveBuffers[id])})
}
func (m *mockGL) BindBufferRange(target, index, id uint32, offset, size int) {
	m.uboBinds = append(m.uboBinds, mockUBOBind{slot: index, buf: id, offset: offset, size: size, ranged: true})
}
func (m *mockGL) CopyBufferSubData(readTarget, writeTarget uint32, readOffset, writeOffset, size int) {
	src := m.liveBuffers[m.boundBuffers[readTarget]]
	dst := m.liveBuffers[m.boundBuffers[writeTarget]]
	copy(dst[writeOffset:writeOffset+size], src[readOffset:readOffset+size])
	m.copyBufCalls++
}

func (m *mockGL) GenVertexArray() uint32 {
	id := m.alloc()
	m.liveVAOs[id] = true
	return id
}
func (m *mockGL) DeleteVertexArray(id uint32) { delete(m.liveVAOs, id) }
func (m *mockGL) BindVertexArray(id uint32)   { m.boundVAO = id }
func (m *mockGL) EnableVertexAttribArray(uint32) {
}
func (m *mockGL) DisableVertexAttribArray(uint32) {
}
func (m *mockGL) VertexAttribPointer(index uint32, size int32, typ uint32, normalized bool, stride int32, offset uintptr) {
	m.attribCalls = append(m.attribCalls, mockAttribCall{
		vao: m.boundVAO, vbo: m.boundBuffers[gpuGLArrayBuffer],
		index: index, size: size, typ: typ, normalized: normalized, stride: stride, offset: offset,
	})
}
func (m *mockGL) VertexAttribIPointer(index uint32, size int32, typ uint32, stride int32, offset uintptr) {
	m.attribCalls = append(m.attribCalls, mockAttribCall{
		vao: m.boundVAO, vbo: m.boundBuffers[gpuGLArrayBuffer],
		index: index, size: size, typ: typ, integer: true, stride: stride, offset: offset,
	})
}
func (m *mockGL) VertexAttribDivisor(index, divisor uint32) {
	for i := len(m.attribCalls) - 1; i >= 0; i-- {
		if m.attribCalls[i].index == index {
			m.attribCalls[i].divisor = divisor
			return
		}
	}
}

func (m *mockGL) GenTexture() uint32 {
	id := m.alloc()
	m.liveTextures[id] = 0
	return id
}
func (m *mockGL) DeleteTexture(id uint32)   { delete(m.liveTextures, id) }
func (m *mockGL) ActiveTexture(unit uint32) { m.activeTexUnit = unit - gpuGLTexture0 }
func (m *mockGL) BindTexture(target, id uint32) {
	m.boundTextures[m.activeTexUnit] = id
	if id != 0 {
		m.liveTextures[id] = target
	}
}
func (m *mockGL) PixelStorei(pname uint32, param int32) {
	if pname == gpuGLUnpackRowLength {
		m.unpackRowLen = param
	}
}
func (m *mockGL) TexImage2D(target uint32, level, internalFormat, width, height, border int32, format, typ uint32, data []byte) {
}
func (m *mockGL) TexSubImage2D(target uint32, level, x, y, width, height int32, format, typ uint32, data []byte) {
	m.texUploads = append(m.texUploads, mockTexUpload{
		target: target, level: level, x: x, y: y, w: width, h: height, rowLen: m.unpackRowLen,
		data: append([]byte(nil), data...),
	})
}
func (m *mockGL) TexSubImage2DOffset(target uint32, level, x, y, width, height int32, format, typ uint32, offset uintptr) {
	m.texUploads = append(m.texUploads, mockTexUpload{
		target: target, level: level, x: x, y: y, w: width, h: height, rowLen: m.unpackRowLen,
		pboOff: offset, isPBO: true,
	})
}
func (m *mockGL) TexImage3D(target uint32, level, internalFormat, width, height, depth, border int32, format, typ uint32, data []byte) {
}
func (m *mockGL) TexSubImage3D(target uint32, level, x, y, z, width, height, depth int32, format, typ uint32, data []byte) {
	m.texUploads = append(m.texUploads, mockTexUpload{
		target: target, level: level, x: x, y: y, z: z, w: width, h: height, d: depth, rowLen: m.unpackRowLen,
		data: append([]byte(nil), data...),
	})
}
func (m *mockGL) TexSubImage3DOffset(target uint32, level, x, y, z, width, height, depth int32, format, typ uint32, offset uintptr) {
	m.texUploads = append(m.texUploads, mockTexUpload{
		target: target, level: level, x: x, y: y, z: z, w: width, h: height, d: depth, rowLen: m.unpackRowLen,
		pboOff: offset, isPBO: true,
	})
}
func (m *mockGL) TexParameteri(target, pname uint32, param int32) {}
func (m *mockGL) EGLImageTargetTexture2D(target uint32, image uintptr) {
	m.eglImageTargets = append(m.eglImageTargets, image)
}

func (m *mockGL) GenSampler() uint32 {
	id := m.alloc()
	m.liveSamplers[id] = map[uint32]int32{}
	return id
}
func (m *mockGL) DeleteSampler(id uint32) { delete(m.liveSamplers, id) }
func (m *mockGL) BindSampler(unit, sampler uint32) {
	m.boundSamplers[unit] = sampler
	m.samplerBinds = append(m.samplerBinds, mockSamplerBind{
		unit:    unit,
		tex:     m.boundTextures[unit],
		target:  m.liveTextures[m.boundTextures[unit]],
		sampler: sampler,
	})
}
func (m *mockGL) SamplerParameteri(sampler, pname uint32, param int32) {
	if s := m.liveSamplers[sampler]; s != nil {
		s[pname] = param
	}
}
func (m *mockGL) SamplerParameterf(sampler, pname uint32, param float32) {}

func (m *mockGL) GenFramebuffer() uint32 {
	id := m.alloc()
	m.liveFBOs[id] = true
	return id
}
func (m *mockGL) DeleteFramebuffer(id uint32) { delete(m.liveFBOs, id) }
func (m *mockGL) BindFramebuffer(target, id uint32) {
	switch target {
	case gpuGLReadFramebuffer:
		m.boundReadFBO = id
	case gpuGLDrawFramebuffer:
		m.boundDrawFBO = id
	default:
		m.boundFBO = id
		m.boundReadFBO = id
		m.boundDrawFBO = id
	}
}
func (m *mockGL) FramebufferTexture2D(target, attachment, texTarget, texture uint32, level int32) {
	fbo := m.boundFBO
	if target == gpuGLReadFramebuffer {
		fbo = m.boundReadFBO
	} else if target == gpuGLDrawFramebuffer {
		fbo = m.boundDrawFBO
	}
	m.fboAttachments[[2]uint32{fbo, attachment}] = texture
}
func (m *mockGL) FramebufferTextureLayer(target, attachment, texture uint32, level, layer int32) {
	m.FramebufferTexture2D(target, attachment, gpuGLTexture2DArray, texture, level)
}
func (m *mockGL) DrawBuffers(bufs []uint32)            { m.drawBufs = append([]uint32(nil), bufs...) }
func (m *mockGL) CheckFramebufferStatus(uint32) uint32 { return m.fboStatus }
func (m *mockGL) BlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32) {
	m.blitCalls++
}
func (m *mockGL) ReadBuffer(uint32) {}
func (m *mockGL) ReadPixels(x, y, width, height int32, format, typ uint32, dst []byte) {
	m.readPixelsCalls = append(m.readPixelsCalls, [4]int32{x, y, width, height})
	if len(m.readPixelsSource) >= len(dst) {
		copy(dst, m.readPixelsSource[:len(dst)])
	}
}

func (m *mockGL) Viewport(x, y, width, height int32) { m.viewport = [4]int32{x, y, width, height} }
func (m *mockGL) DepthRangef(near, far float32)      { m.depthRange = [2]float32{near, far} }
func (m *mockGL) Enable(cap uint32)                  { m.enabledCaps[cap] = true }
func (m *mockGL) Disable(cap uint32)                 { m.enabledCaps[cap] = false }
func (m *mockGL) Scissor(x, y, width, height int32)  { m.scissor = [4]int32{x, y, width, height} }
func (m *mockGL) CullFace(mode uint32)               { m.cullFace = mode }
func (m *mockGL) FrontFace(mode uint32)              { m.frontFace = mode }
func (m *mockGL) DepthMask(flag bool)                { m.depthMask = flag }
func (m *mockGL) DepthFunc(fn uint32)                { m.depthFunc = fn }
func (m *mockGL) PolygonOffset(factor, units float32) {
	m.polyOffset = [2]float32{factor, units}
}
func (m *mockGL) ColorMask(r, g, b, a bool) { m.colorMask = [4]bool{r, g, b, a} }
func (m *mockGL) BlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha uint32) {
	m.blendFuncs = [4]uint32{srcRGB, dstRGB, srcAlpha, dstAlpha}
}
func (m *mockGL) BlendEquationSeparate(modeRGB, modeAlpha uint32) {
	m.blendOps = [2]uint32{modeRGB, modeAlpha}
}
func (m *mockGL) BlendColor(r, g, b, a float32) { m.blendColor = [4]float32{r, g, b, a} }
func (m *mockGL) ClearColor(r, g, b, a float32) { m.clearColor = [4]float32{r, g, b, a} }
func (m *mockGL) ClearDepthf(d float32)         { m.clearDepth = d }
func (m *mockGL) ClearStencil(int32)            {}
func (m *mockGL) Clear(mask uint32)             { m.clearMasks = append(m.clearMasks, mask) }

func (m *mockGL) DrawArrays(mode uint32, first, count int32) {
	m.drawCalls = append(m.drawCalls, mockDrawCall{
		kind: "arrays", mode: mode, first: first, count: count,
		vao: m.boundVAO, prog: m.boundProg, fbo: m.boundFBO,
	})
}
func (m *mockGL) DrawArraysInstanced(mode uint32, first, count, instanceCount int32) {
	m.drawCalls = append(m.drawCalls, mockDrawCall{
		kind: "arraysInstanced", mode: mode, first: first, count: count, instanceCount: instanceCount,
		vao: m.boundVAO, prog: m.boundProg, fbo: m.boundFBO,
	})
}
func (m *mockGL) DrawElements(mode uint32, count int32, typ uint32, offset uintptr) {
	m.drawCalls = append(m.drawCalls, mockDrawCall{
		kind: "elements", mode: mode, count: count, typ: typ, offset: offset,
		vao: m.boundVAO, ibo: m.boundBuffers[gpuGLElementArrayBuffer], prog: m.boundProg, fbo: m.boundFBO,
	})
}
func (m *mockGL) DrawElementsInstanced(mode uint32, count int32, typ uint32, offset uintptr, instanceCount int32) {
	m.drawCalls = append(m.drawCalls, mockDrawCall{
		kind: "elementsInstanced", mode: mode, count: count, typ: typ, offset: offset, instanceCount: instanceCount,
		vao: m.boundVAO, ibo: m.boundBuffers[gpuGLElementArrayBuffer], prog: m.boundProg, fbo: m.boundFBO,
	})
}
func (m *mockGL) DrawElementsBaseVertex(mode uint32, count int32, typ uint32, offset uintptr, baseVertex int32) {
	m.drawCalls = append(m.drawCalls, mockDrawCall{
		kind: "elementsBaseVertex", mode: mode, count: count, typ: typ, offset: offset, baseVertex: baseVertex,
		vao: m.boundVAO, ibo: m.boundBuffers[gpuGLElementArrayBuffer], prog: m.boundProg, fbo: m.boundFBO,
	})
}
func (m *mockGL) DrawElementsInstancedBaseVertex(mode uint32, count int32, typ uint32, offset uintptr, instanceCount, baseVertex int32) {
	m.drawCalls = append(m.drawCalls, mockDrawCall{
		kind: "elementsInstancedBaseVertex", mode: mode, count: count, typ: typ, offset: offset, instanceCount: instanceCount, baseVertex: baseVertex,
		vao: m.boundVAO, ibo: m.boundBuffers[gpuGLElementArrayBuffer], prog: m.boundProg, fbo: m.boundFBO,
	})
}

func (m *mockGL) GetIntegerv(uint32) int32 { return 0 }
func (m *mockGL) GetError() uint32         { return 0 }

const testTexturedLitWGSL = `
struct Frame {
	mvp: mat4x4<f32>,
	tint: vec4<f32>,
}

@group(0) @binding(0) var<uniform> frame: Frame;
@group(0) @binding(1) var albedoTex: texture_2d<f32>;
@group(0) @binding(2) var albedoSamp: sampler;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) uv: vec2<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) uv: vec2<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = frame.mvp * vec4<f32>(in.pos, 1.0);
	out.uv = in.uv;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	return textureSample(albedoTex, albedoSamp, in.uv) * frame.tint;
}
`

func compileTestWGSL(t *testing.T, src string) *shaderir.Program {
	t.Helper()
	p, err := wgsl.CompilePipeline(src)
	if err != nil {
		t.Fatalf("wgsl.CompilePipeline: %v", err)
	}
	return p.IR
}

func TestGLGPUUniformBlocksAndCombinedSamplerPairs(t *testing.T) {
	ir := compileTestWGSL(t, testTexturedLitWGSL)

	for _, tc := range []struct {
		name        string
		es          bool
		wantVersion string
	}{
		{"OpenGL330Core", false, "#version 330 core"},
		{"OpenGLES300", true, "#version 300 es"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gl := newMockGL()
			ctx := newGLGPUContext(gl, tc.es, 1, driver.DefaultGPULimits(), driver.GPUFeatures{Depth32Float: true})

			uboData := make([]byte, 512) // room for dynamic offset 256
			frame := driver.GPUFrame{
				Generation: 1,
				Ops: []driver.GPUResourceOp{
					{Kind: driver.GPUOpCreateBuffer, ID: 1, Buffer: &driver.GPUBufferDesc{Size: 512, Usage: driver.GPUBufferUniform | driver.GPUBufferCopyDst, Data: uboData}},
					{Kind: driver.GPUOpCreateTexture, ID: 2, Texture: &driver.GPUTextureDesc{Width: 4, Height: 4, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled}},
					{Kind: driver.GPUOpCreateSampler, ID: 3, Sampler: &driver.GPUSamplerDesc{MinFilter: driver.GPUFilterLinear, MagFilter: driver.GPUFilterLinear, AddressU: driver.GPUAddressRepeat, AddressV: driver.GPUAddressMirrorRepeat}},
					{Kind: driver.GPUOpCreateBindGroupLayout, ID: 4, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{
						Entries: []driver.GPUBindGroupLayoutEntry{
							{Binding: 0, Visibility: driver.GPUStageAll, Type: driver.GPUBindingUniform, HasDynamicOffset: true},
							{Binding: 1, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingTexture},
							{Binding: 2, Visibility: driver.GPUStageFragment, Type: driver.GPUBindingSampler},
						},
					}},
					{Kind: driver.GPUOpCreateBindGroup, ID: 5, BindGroup: &driver.GPUBindGroupDesc{
						Layout: 4,
						Entries: []driver.GPUBindGroupEntry{
							{Binding: 0, Buffer: 1, Offset: 0, Size: 80},
							{Binding: 1, Texture: 2},
							{Binding: 2, Sampler: 3},
						},
					}},
					{Kind: driver.GPUOpCreateBuffer, ID: 6, Buffer: &driver.GPUBufferDesc{Size: 60, Usage: driver.GPUBufferVertex}},
					{Kind: driver.GPUOpCreatePipeline, ID: 7, Pipeline: &driver.GPUPipelineDesc{
						Vertex:   driver.GPUShaderStage{IR: ir, EntryPoint: "vs_main"},
						Fragment: driver.GPUShaderStage{IR: ir, EntryPoint: "fs_main"},
						Layouts:  []driver.GPUID{4},
						VertexBuffers: []driver.GPUVertexBufferLayout{{
							Stride: 20,
							Attributes: []driver.GPUVertexAttribute{
								{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x3},
								{Location: 1, Offset: 12, Format: driver.GPUVertexFloat32x2},
							},
						}},
						Targets: []driver.GPUColorTargetState{{Format: driver.GPUFormatRGBA8Unorm}},
					}},
				},
				Passes: []driver.GPUPass{{
					ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, StoreOp: driver.GPUStoreStore, ClearColor: driver.RGB(0x102030)}},
					Commands: []driver.GPUCommand{
						{Kind: driver.GPUCmdSetPipeline, ID: 7},
						{Kind: driver.GPUCmdSetBindGroup, Slot: 0, ID: 5, DynamicOffsets: []uint32{256}},
						{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 6, Offset: 0},
						{Kind: driver.GPUCmdDraw, U0: 3, U1: 0},
					},
				}},
			}

			_, errs := ctx.executeFrame(frame, 42, 320, 240)
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %+v", errs)
			}

			if len(gl.shaderSources) < 2 || !strings.HasPrefix(gl.shaderSources[0], tc.wantVersion) || !strings.HasPrefix(gl.shaderSources[1], tc.wantVersion) {
				t.Fatalf("shader sources did not start with %q: %+v", tc.wantVersion, gl.shaderSources)
			}
			// Verify clip-space Z remap is present in vertex shader.
			if !strings.Contains(gl.shaderSources[0], "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;") {
				t.Errorf("vertex shader missing OpenGL clip-space Z remap:\n%s", gl.shaderSources[0])
			}
			// Uniform block UB0 must be bound to slot 0.
			if slot, ok := gl.ubBlockBindings["UB0"]; !ok || slot != 0 {
				t.Errorf("UB0 block binding = (%d, %v), want (0, true)", slot, ok)
			}
			// Synthesized combined sampler uniform must be assigned unit 0.
			if len(gl.samplerUniforms) != 1 {
				t.Fatalf("expected 1 combined sampler uniform, got %+v", gl.samplerUniforms)
			}
			for name, unit := range gl.samplerUniforms {
				if unit != 0 {
					t.Errorf("sampler uniform %q bound to unit %d, want 0", name, unit)
				}
			}
			// UBO bound with dynamic offset 256 and size 80 via glBindBufferRange.
			if len(gl.uboBinds) != 1 || !gl.uboBinds[0].ranged || gl.uboBinds[0].offset != 256 || gl.uboBinds[0].size != 80 {
				t.Errorf("uboBinds = %+v, want ranged offset=256 size=80", gl.uboBinds)
			}
			// Texture + sampler paired on unit 0.
			if len(gl.samplerBinds) != 1 || gl.samplerBinds[0].unit != 0 || gl.samplerBinds[0].tex != ctx.textures[2].glID || gl.samplerBinds[0].sampler != ctx.samplers[3].glID {
				t.Errorf("samplerBinds = %+v, want unit=0 tex=%d samp=%d", gl.samplerBinds, ctx.textures[2].glID, ctx.samplers[3].glID)
			}
			// Sampler parameters configured properly.
			sampParams := gl.liveSamplers[ctx.samplers[3].glID]
			if sampParams[gpuGLTextureWrapS] != gpuGLRepeat || sampParams[gpuGLTextureWrapT] != gpuGLMirroredRepeat {
				t.Errorf("sampler wrap params = %+v", sampParams)
			}
			// Draw executed against GtkGLArea's defaultFBO (42).
			if len(gl.drawCalls) != 1 || gl.drawCalls[0].fbo != 42 || gl.drawCalls[0].count != 3 {
				t.Errorf("drawCalls = %+v", gl.drawCalls)
			}
		})
	}
}

func TestGLGPUVertexAttributeLayoutFloatIntUnormAndInstancing(t *testing.T) {
	ir := compileTestWGSL(t, testTexturedLitWGSL)

	for _, es := range []bool{false, true} {
		gl := newMockGL()
		ctx := newGLGPUContext(gl, es, 1, driver.DefaultGPULimits(), driver.GPUFeatures{})

		frame := driver.GPUFrame{
			Generation: 1,
			Ops: []driver.GPUResourceOp{
				{Kind: driver.GPUOpCreateBuffer, ID: 10, Buffer: &driver.GPUBufferDesc{Size: 1024, Usage: driver.GPUBufferVertex}},
				{Kind: driver.GPUOpCreateBuffer, ID: 11, Buffer: &driver.GPUBufferDesc{Size: 1024, Usage: driver.GPUBufferVertex}},
				{Kind: driver.GPUOpCreateBuffer, ID: 12, Buffer: &driver.GPUBufferDesc{Size: 256, Usage: driver.GPUBufferIndex}},
				{Kind: driver.GPUOpCreatePipeline, ID: 13, Pipeline: &driver.GPUPipelineDesc{
					Vertex:   driver.GPUShaderStage{IR: ir, EntryPoint: "vs_main"},
					Fragment: driver.GPUShaderStage{IR: ir, EntryPoint: "fs_main"},
					VertexBuffers: []driver.GPUVertexBufferLayout{
						{
							Stride:   36,
							StepMode: driver.GPUStepVertex,
							Attributes: []driver.GPUVertexAttribute{
								{Location: 0, Offset: 0, Format: driver.GPUVertexFloat32x3},
								{Location: 1, Offset: 12, Format: driver.GPUVertexUnorm8x4},
								{Location: 2, Offset: 16, Format: driver.GPUVertexSint32x2},
								{Location: 3, Offset: 24, Format: driver.GPUVertexUint32x3},
							},
						},
						{
							Stride:   16,
							StepMode: driver.GPUStepInstance,
							Attributes: []driver.GPUVertexAttribute{
								{Location: 4, Offset: 0, Format: driver.GPUVertexFloat32x4},
							},
						},
					},
				}},
			},
			Passes: []driver.GPUPass{{
				ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadLoad}},
				Commands: []driver.GPUCommand{
					{Kind: driver.GPUCmdSetPipeline, ID: 13},
					{Kind: driver.GPUCmdSetVertexBuffer, Slot: 0, ID: 10, Offset: 8},
					{Kind: driver.GPUCmdSetVertexBuffer, Slot: 1, ID: 11, Offset: 32},
					{Kind: driver.GPUCmdSetIndexBuffer, ID: 12, IndexFormat: driver.GPUIndexUint32, Offset: 16},
					// indexCount=6, instanceCount=4, firstIndex=2, firstInstance=3, baseVertex=5
					{Kind: driver.GPUCmdDrawIndexedInstanced, U0: 6, U1: 4, U2: 2, U3: 3, I0: 5},
				},
			}},
		}

		_, errs := ctx.executeFrame(frame, 1, 100, 100)
		if len(errs) != 0 {
			t.Fatalf("es=%v unexpected errs: %+v", es, errs)
		}
		if len(gl.attribCalls) != 5 {
			t.Fatalf("es=%v attribCalls len = %d, want 5", es, len(gl.attribCalls))
		}
		// Base vertex emulation adds baseVertex*stride (5*36=180) only on ES 3.0.
		wantBaseOff := uintptr(8)
		if es {
			wantBaseOff += 5 * 36
		}
		// Location 0: float32x3
		if a := gl.attribCalls[0]; a.index != 0 || a.integer || a.normalized || a.typ != gpuGLFloat || a.size != 3 || a.offset != wantBaseOff || a.divisor != 0 {
			t.Errorf("es=%v loc 0 = %+v", es, a)
		}
		// Location 1: unorm8x4 (normalized float pointer)
		if a := gl.attribCalls[1]; a.index != 1 || a.integer || !a.normalized || a.typ != gpuGLUnsignedByte || a.size != 4 || a.offset != wantBaseOff+12 {
			t.Errorf("es=%v loc 1 = %+v", es, a)
		}
		// Location 2: sint32x2 (integer pointer!)
		if a := gl.attribCalls[2]; a.index != 2 || !a.integer || a.typ != gpuGLInt || a.size != 2 || a.offset != wantBaseOff+16 {
			t.Errorf("es=%v loc 2 = %+v", es, a)
		}
		// Location 3: uint32x3 (integer pointer!)
		if a := gl.attribCalls[3]; a.index != 3 || !a.integer || a.typ != gpuGLUnsignedInt || a.size != 3 || a.offset != wantBaseOff+24 {
			t.Errorf("es=%v loc 3 = %+v", es, a)
		}
		// Location 4: instanced float32x4 with divisor=1 and offset=32 + firstInstance(3)*16 = 80
		if a := gl.attribCalls[4]; a.index != 4 || a.divisor != 1 || a.offset != 32+3*16 {
			t.Errorf("es=%v loc 4 = %+v, want divisor=1 offset=80", es, a)
		}
		// Index byte offset = 16 + firstIndex(2)*4 = 24.
		dc := gl.drawCalls[0]
		if dc.offset != 24 || dc.typ != gpuGLUnsignedInt || dc.instanceCount != 4 {
			t.Errorf("es=%v drawCall = %+v", es, dc)
		}
		if !es && (dc.kind != "elementsInstancedBaseVertex" || dc.baseVertex != 5) {
			t.Errorf("GL 3.3 expected elementsInstancedBaseVertex baseVertex=5, got %+v", dc)
		}
		if es && dc.kind != "elementsInstanced" {
			t.Errorf("GLES 3.0 expected elementsInstanced with emulated baseVertex, got %+v", dc)
		}
	}
}

func TestGLGPURenderStateDepthCullBlendViewportScissor(t *testing.T) {
	ir := compileTestWGSL(t, testTexturedLitWGSL)
	gl := newMockGL()
	ctx := newGLGPUContext(gl, false, 1, driver.DefaultGPULimits(), driver.GPUFeatures{})

	frame := driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreatePipeline, ID: 1, Pipeline: &driver.GPUPipelineDesc{
				Vertex:   driver.GPUShaderStage{IR: ir},
				Fragment: driver.GPUShaderStage{IR: ir},
				Primitive: driver.GPUPrimitiveState{
					Topology:  driver.GPUTopologyTriangleStrip,
					FrontFace: driver.GPUFrontCW,
					CullMode:  driver.GPUCullBack,
				},
				DepthStencil: &driver.GPUDepthStencilState{
					Format:              driver.GPUFormatDepth24Plus,
					DepthWrite:          true,
					DepthCompare:        driver.GPUCompareLessEqual,
					DepthBias:           2,
					DepthBiasSlopeScale: 1.5,
				},
				Targets: []driver.GPUColorTargetState{{
					Format:    driver.GPUFormatRGBA8Unorm,
					WriteMask: driver.GPUColorWriteRed | driver.GPUColorWriteBlue,
					Blend: &driver.GPUBlendState{
						Color: driver.GPUBlendComponent{SrcFactor: driver.GPUBlendSrcAlpha, DstFactor: driver.GPUBlendOneMinusSrcAlpha, Operation: driver.GPUBlendAdd},
						Alpha: driver.GPUBlendComponent{SrcFactor: driver.GPUBlendOne, DstFactor: driver.GPUBlendZero, Operation: driver.GPUBlendMax},
					},
				}},
			}},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear, ClearColor: driver.RGB(0x010203)}},
			DepthStencil:     &driver.GPUDepthStencilAttachment{DepthLoadOp: driver.GPULoadClear, ClearDepth: 1.0},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetViewport, F0: 10, F1: 20, F2: 100, F3: 60, F4: 0.1, F5: 0.9},
				{Kind: driver.GPUCmdSetScissor, I0: 12, I1: 24, I2: 50, I3: 40},
				{Kind: driver.GPUCmdSetBlendConstant, F0: 0.2, F1: 0.4, F2: 0.6, F3: 0.8},
				{Kind: driver.GPUCmdSetPipeline, ID: 1},
				{Kind: driver.GPUCmdDraw, U0: 4, U1: 0},
			},
		}},
	}

	_, errs := ctx.executeFrame(frame, 9, 200, 150)
	if len(errs) != 0 {
		t.Fatalf("errs: %+v", errs)
	}
	// Viewport Y flipped from top-left (y=20, h=60 in 150px surface -> glY = 150 - 80 = 70).
	if gl.viewport != [4]int32{10, 70, 100, 60} {
		t.Errorf("viewport = %v, want [10 70 100 60]", gl.viewport)
	}
	if gl.depthRange != [2]float32{0.1, 0.9} {
		t.Errorf("depthRange = %v, want [0.1 0.9]", gl.depthRange)
	}
	// Scissor Y flipped (y=24, h=40 in 150px surface -> glY = 150 - 64 = 86).
	if gl.scissor != [4]int32{12, 86, 50, 40} {
		t.Errorf("scissor = %v, want [12 86 50 40]", gl.scissor)
	}
	if gl.frontFace != gpuGLCW || gl.cullFace != gpuGLBack || !gl.enabledCaps[gpuGLCullFace] {
		t.Errorf("cull/frontFace state mismatch: front=%x cull=%x enabled=%v", gl.frontFace, gl.cullFace, gl.enabledCaps[gpuGLCullFace])
	}
	if !gl.enabledCaps[gpuGLDepthTest] || gl.depthFunc != gpuGLLEqual || !gl.enabledCaps[gpuGLPolygonOffsetFill] || gl.polyOffset != [2]float32{1.5, 2} {
		t.Errorf("depth state mismatch: test=%v func=%x poly=%v offset=%v", gl.enabledCaps[gpuGLDepthTest], gl.depthFunc, gl.enabledCaps[gpuGLPolygonOffsetFill], gl.polyOffset)
	}
	if !gl.enabledCaps[gpuGLBlend] || gl.blendFuncs != [4]uint32{gpuGLSrcAlpha, gpuGLOneMinusSrcAlpha, gpuGLOne, gpuGLZero} || gl.blendOps != [2]uint32{gpuGLFuncAdd, gpuGLMax} {
		t.Errorf("blend state mismatch: enabled=%v funcs=%x ops=%x", gl.enabledCaps[gpuGLBlend], gl.blendFuncs, gl.blendOps)
	}
	if gl.blendColor != [4]float32{0.2, 0.4, 0.6, 0.8} {
		t.Errorf("blendColor = %v", gl.blendColor)
	}
	// After executeFrame finishes, clean GTK state is restored: defaultFBO, VAO=0, prog=0, scissor off, masks enabled.
	if gl.boundFBO != 9 || gl.boundVAO != 0 || gl.boundProg != 0 || gl.enabledCaps[gpuGLScissorTest] || gl.colorMask != [4]bool{true, true, true, true} || !gl.depthMask {
		t.Errorf("post-frame state not restored cleanly: fbo=%d vao=%d prog=%d scissor=%v colorMask=%v depthMask=%v", gl.boundFBO, gl.boundVAO, gl.boundProg, gl.enabledCaps[gpuGLScissorTest], gl.colorMask, gl.depthMask)
	}
}

func TestGLGPUOffscreenFBOAndReadback(t *testing.T) {
	gl := newMockGL()
	// 2x2 RGBA pixels returned by mock glReadPixels (OpenGL bottom-to-top): bottom row = blue (0,0,255,255), top row = red (255,0,0,255)
	gl.readPixelsSource = []byte{
		0, 0, 255, 255, 0, 0, 255, 255,
		255, 0, 0, 255, 255, 0, 0, 255,
	}
	ctx := newGLGPUContext(gl, false, 1, driver.DefaultGPULimits(), driver.GPUFeatures{})

	frame := driver.GPUFrame{
		Generation: 1,
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateTexture, ID: 1, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureRenderAttachment | driver.GPUTextureSampled}},
			{Kind: driver.GPUOpCreateTexture, ID: 2, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatDepth24Plus, Usage: driver.GPUTextureRenderAttachment}},
			{Kind: driver.GPUOpCreateTexture, ID: 3, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled | driver.GPUTextureCopyDst}},
			{Kind: driver.GPUOpWriteTexture, ID: 3, Data: make([]byte, 16), TextureDst: driver.GPUTextureCopy{Texture: 3}},
			{Kind: driver.GPUOpReadTexture, ID: 1, ReadbackID: 101, ReadbackRegion: driver.GPUTextureRegion{Width: 2, Height: 2}},
			{Kind: driver.GPUOpReadTexture, ID: 3, ReadbackID: 102, ReadbackRegion: driver.GPUTextureRegion{Width: 2, Height: 2}},
			{Kind: driver.GPUOpReadTexture, ReadbackID: 103, ReadbackSurface: true, ReadbackRegion: driver.GPUTextureRegion{Width: 2, Height: 2}},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{Texture: 1, LoadOp: driver.GPULoadClear, ClearColor: driver.RGB(0xff0000)}},
			DepthStencil:     &driver.GPUDepthStencilAttachment{Texture: 2, DepthLoadOp: driver.GPULoadClear, ClearDepth: 1.0},
		}},
	}

	rbs, errs := ctx.executeFrame(frame, 77, 2, 2)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}
	if len(rbs) != 3 {
		t.Fatalf("readbacks len = %d, want 3", len(rbs))
	}
	// Offscreen color texture (ID 1) was rendered to -> flipY reverses bottom-to-top rows so top row is red!
	if !bytes.Equal(rbs[0].pixels[:4], []byte{255, 0, 0, 255}) || !bytes.Equal(rbs[0].pixels[8:12], []byte{0, 0, 255, 255}) {
		t.Errorf("rendered texture readback not flipped to top-row-first: %v", rbs[0].pixels)
	}
	// Uploaded-only texture (ID 3) kept top-row-first layout -> no flip!
	if !bytes.Equal(rbs[1].pixels[:4], []byte{0, 0, 255, 255}) {
		t.Errorf("uploaded texture readback unexpectedly flipped: %v", rbs[1].pixels)
	}
	// Surface readback (ID 103) reads from defaultFBO with flipY!
	if !bytes.Equal(rbs[2].pixels[:4], []byte{255, 0, 0, 255}) {
		t.Errorf("surface readback not flipped to top-row-first: %v", rbs[2].pixels)
	}
	// Verify FBO attached both color texture 1 and depth texture 2.
	if gl.fboAttachments[[2]uint32{ctx.scratchFBO, gpuGLColorAttachment0}] != ctx.textures[1].glID {
		t.Errorf("scratchFBO color attachment mismatch")
	}
	if gl.fboAttachments[[2]uint32{ctx.scratchFBO, gpuGLDepthAttachment}] != ctx.textures[2].glID {
		t.Errorf("scratchFBO depth attachment mismatch")
	}
}

func TestGLGPUTextureUploadFormatsBGRAPaddingCubeAndArray(t *testing.T) {
	gl := newMockGL()
	ctx := newGLGPUContext(gl, true, 1, driver.DefaultGPULimits(), driver.GPUFeatures{})

	// 2x2 BGRA8Unorm with padded BytesPerRow=12 (8 bytes pixels + 4 bytes padding per row)
	bgraPadded := []byte{
		10, 20, 30, 255, 40, 50, 60, 255, 0, 0, 0, 0,
		70, 80, 90, 255, 100, 110, 120, 255, 0, 0, 0, 0,
	}
	// 2x2 RGBA8Unorm with padded BytesPerRow=16 (4 pixels per row stride -> UNPACK_ROW_LENGTH=4)
	rgbaPadded := make([]byte, 32)

	frame := driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateTexture, ID: 1, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatBGRA8Unorm, Usage: driver.GPUTextureSampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 1, Data: bgraPadded, TextureDst: driver.GPUTextureCopy{Texture: 1}, DataLayout: driver.GPUDataLayout{BytesPerRow: 12, RowsPerImage: 2}},
			{Kind: driver.GPUOpCreateTexture, ID: 2, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 2, Data: rgbaPadded, TextureDst: driver.GPUTextureCopy{Texture: 2}, DataLayout: driver.GPUDataLayout{BytesPerRow: 16, RowsPerImage: 2}},
			{Kind: driver.GPUOpCreateTexture, ID: 3, Texture: &driver.GPUTextureDesc{Dimension: driver.GPUDimCube, Width: 4, Height: 4, Layers: 6, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 3, Data: make([]byte, 4*4*4), TextureDst: driver.GPUTextureCopy{Texture: 3, ArrayLayer: 2}},
			{Kind: driver.GPUOpCreateTexture, ID: 4, Texture: &driver.GPUTextureDesc{Dimension: driver.GPUDim2DArray, Width: 4, Height: 4, Layers: 3, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 4, Data: make([]byte, 4*4*4), TextureDst: driver.GPUTextureCopy{Texture: 4, ArrayLayer: 1}},
			{Kind: driver.GPUOpCreateTexture, ID: 5, Texture: &driver.GPUTextureDesc{Dimension: driver.GPUDim3D, Width: 2, Height: 2, Depth: 4, MipLevels: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureSampled}},
			{Kind: driver.GPUOpWriteTexture, ID: 5, Data: make([]byte, 2*2*2*4), TextureDst: driver.GPUTextureCopy{Texture: 5, OriginZ: 1}, DataLayout: driver.GPUDataLayout{BytesPerRow: 8, RowsPerImage: 2}},
		},
	}

	_, errs := ctx.executeFrame(frame, 1, 16, 16)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}
	if len(gl.texUploads) != 5 {
		t.Fatalf("texUploads = %d, want 5", len(gl.texUploads))
	}
	// Upload 0: BGRA swizzled to RGBA and repacked tightly.
	wantSwizzled := []byte{
		30, 20, 10, 255, 60, 50, 40, 255,
		90, 80, 70, 255, 120, 110, 100, 255,
	}
	if !bytes.Equal(gl.texUploads[0].data, wantSwizzled) || gl.texUploads[0].rowLen != 0 {
		t.Errorf("BGRA upload = %v rowLen=%d, want %v rowLen=0", gl.texUploads[0].data, gl.texUploads[0].rowLen, wantSwizzled)
	}
	// Upload 1: RGBA padded row length = 4 pixels.
	if gl.texUploads[1].rowLen != 4 {
		t.Errorf("RGBA padded rowLen = %d, want 4", gl.texUploads[1].rowLen)
	}
	// Upload 2: Cube face +2.
	if gl.texUploads[2].target != gpuGLTextureCubeMapPosX+2 {
		t.Errorf("cube upload target = 0x%x, want 0x%x", gl.texUploads[2].target, gpuGLTextureCubeMapPosX+2)
	}
	// Upload 3: 2D array layer 1.
	if gl.texUploads[3].target != gpuGLTexture2DArray || gl.texUploads[3].z != 1 || gl.texUploads[3].d != 1 {
		t.Errorf("2D array upload = %+v", gl.texUploads[3])
	}
	// Upload 4: 3D volume texture starting at OriginZ=1 with depth 2.
	if gl.texUploads[4].target != gpuGLTexture3D || gl.texUploads[4].z != 1 || gl.texUploads[4].d != 2 {
		t.Errorf("3D upload = %+v, want target=GL_TEXTURE_3D z=1 d=2", gl.texUploads[4])
	}
}

func TestGLGPUCopyOpsErrorHandlingAndDestroyAllCleanup(t *testing.T) {
	ir := compileTestWGSL(t, testTexturedLitWGSL)
	gl := newMockGL()
	ctx := newGLGPUContext(gl, false, 1, driver.DefaultGPULimits(), driver.GPUFeatures{})

	srcData := make([]byte, 16)
	binary.LittleEndian.PutUint32(srcData[0:], math.Float32bits(3.5))

	frame := driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 1, Buffer: &driver.GPUBufferDesc{Size: 16, Usage: driver.GPUBufferCopySrc, Data: srcData}},
			{Kind: driver.GPUOpCreateBuffer, ID: 2, Buffer: &driver.GPUBufferDesc{Size: 16, Usage: driver.GPUBufferCopyDst | driver.GPUBufferVertex}},
			{Kind: driver.GPUOpCreateTexture, ID: 3, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureCopyDst | driver.GPUTextureCopySrc}},
			{Kind: driver.GPUOpCreateTexture, ID: 4, Texture: &driver.GPUTextureDesc{Width: 2, Height: 2, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureCopyDst}},
			{Kind: driver.GPUOpCreateSampler, ID: 5, Sampler: &driver.GPUSamplerDesc{Compare: driver.GPUCompareLess}},
			{Kind: driver.GPUOpCreatePipeline, ID: 6, Pipeline: &driver.GPUPipelineDesc{Vertex: driver.GPUShaderStage{IR: ir}, Fragment: driver.GPUShaderStage{IR: ir}}},
		},
		Copies: []driver.GPUCopyOp{
			{Kind: driver.GPUCopyBufferToBuffer, SrcID: 1, DstID: 2, SrcOffset: 0, DstOffset: 0, Size: 16},
			{Kind: driver.GPUCopyBufferToTexture, SrcID: 1, DstID: 3, Width: 2, Height: 2, Layers: 1},
			{Kind: driver.GPUCopyTextureToTexture, SrcID: 3, DstID: 4, Width: 2, Height: 2, Layers: 1},
		},
	}

	_, errs := ctx.executeFrame(frame, 1, 16, 16)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %+v", errs)
	}
	if gl.copyBufCalls != 1 || gl.blitCalls != 1 {
		t.Errorf("copyBufCalls=%d blitCalls=%d, want 1, 1", gl.copyBufCalls, gl.blitCalls)
	}
	if !bytes.Equal(gl.liveBuffers[ctx.buffers[2].glID], srcData) {
		t.Errorf("CopyBufferToBuffer data mismatch")
	}

	// Simulate shader compilation failure: must report GPUErrorShaderCompile and leak 0 shaders.
	gl.failShaderKind = gpuGLFragmentShader
	_, errs = ctx.executeFrame(driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreatePipeline, ID: 7, Pipeline: &driver.GPUPipelineDesc{Vertex: driver.GPUShaderStage{IR: ir}, Fragment: driver.GPUShaderStage{IR: ir}}},
		},
	}, 1, 16, 16)
	if len(errs) != 1 || errs[0].code != driver.GPUErrorShaderCompile {
		t.Fatalf("shader compile fail errs = %+v, want GPUErrorShaderCompile", errs)
	}
	if len(gl.liveShaders) != 0 {
		t.Errorf("leaked shaders after compile failure: %d", len(gl.liveShaders))
	}

	// Simulate GtkGLArea unrealize: destroyAll MUST free every single GL object!
	ctx.destroyAll()
	if len(gl.liveBuffers) != 0 || len(gl.liveTextures) != 0 || len(gl.liveSamplers) != 0 ||
		len(gl.liveVAOs) != 0 || len(gl.livePrograms) != 0 || len(gl.liveFBOs) != 0 {
		t.Errorf("destroyAll leaked GL objects: bufs=%d texs=%d samps=%d vaos=%d progs=%d fbos=%d",
			len(gl.liveBuffers), len(gl.liveTextures), len(gl.liveSamplers),
			len(gl.liveVAOs), len(gl.livePrograms), len(gl.liveFBOs))
	}
}

func TestMergeGPULimitsAndPipelineSeparateModules(t *testing.T) {
	lim := mergeGPULimits(8192, 65536, 256, 16)
	if lim.MaxTextureDimension2D != 8192 || lim.MaxUniformBufferSize != 65536 || lim.MinUniformBufferAlignment != 256 {
		t.Errorf("mergeGPULimits = %+v", lim)
	}
	ir1 := compileTestWGSL(t, testTexturedLitWGSL)
	ir2 := compileTestWGSL(t, testTexturedLitWGSL)
	src, err := compilePipelineGLSL(&driver.GPUPipelineDesc{
		Vertex:   driver.GPUShaderStage{IR: ir1},
		Fragment: driver.GPUShaderStage{IR: ir2},
	}, glsl.GLSLVersionDefault)
	if err != nil {
		t.Fatalf("compilePipelineGLSL separate modules: %v", err)
	}
	if src.VertexSource == "" || src.FragmentSource == "" || len(src.UniformBlocks) != 1 || len(src.SamplerPairs) != 1 {
		t.Errorf("separate module compile result unexpected: ub=%+v sp=%+v", src.UniformBlocks, src.SamplerPairs)
	}
}

func TestGLGPUDestroyOpsDepthOnlyPassAndErrorPaths(t *testing.T) {
	ir := compileTestWGSL(t, testTexturedLitWGSL)
	gl := newMockGL()
	ctx := newGLGPUContext(gl, false, 1, driver.DefaultGPULimits(), driver.GPUFeatures{Depth32Float: true})

	// passIsOffscreen checks.
	if passIsOffscreen(driver.GPUPass{ColorAttachments: []driver.GPUColorAttachment{{LoadOp: driver.GPULoadClear}}}) {
		t.Error("swapchain pass misclassified as offscreen")
	}
	if !passIsOffscreen(driver.GPUPass{DepthStencil: &driver.GPUDepthStencilAttachment{Texture: 2}}) {
		t.Error("depth-only offscreen pass not detected as offscreen")
	}

	// Create every resource kind, run a depth-only offscreen pass, then destroy each resource individually.
	_, errs := ctx.executeFrame(driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateBuffer, ID: 1, Buffer: &driver.GPUBufferDesc{Size: 64, Usage: driver.GPUBufferVertex}},
			{Kind: driver.GPUOpCreateTexture, ID: 2, Texture: &driver.GPUTextureDesc{Width: 16, Height: 16, Format: driver.GPUFormatDepth32Float, Usage: driver.GPUTextureRenderAttachment}},
			{Kind: driver.GPUOpCreateSampler, ID: 3, Sampler: &driver.GPUSamplerDesc{Compare: driver.GPUCompareLessEqual}},
			{Kind: driver.GPUOpCreateBindGroupLayout, ID: 4, BindGroupLayout: &driver.GPUBindGroupLayoutDesc{Entries: []driver.GPUBindGroupLayoutEntry{{Binding: 0, Type: driver.GPUBindingSampler}}}},
			{Kind: driver.GPUOpCreateBindGroup, ID: 5, BindGroup: &driver.GPUBindGroupDesc{Layout: 4, Entries: []driver.GPUBindGroupEntry{{Binding: 0, Sampler: 3}}}},
			{Kind: driver.GPUOpCreatePipeline, ID: 6, Pipeline: &driver.GPUPipelineDesc{
				Vertex:       driver.GPUShaderStage{IR: ir, EntryPoint: "vs_main"},
				DepthStencil: &driver.GPUDepthStencilState{Format: driver.GPUFormatDepth32Float, DepthWrite: true, DepthCompare: driver.GPUCompareLess},
			}},
		},
		Passes: []driver.GPUPass{{
			DepthStencil: &driver.GPUDepthStencilAttachment{Texture: 2, DepthLoadOp: driver.GPULoadClear, ClearDepth: 1.0},
			Commands: []driver.GPUCommand{
				{Kind: driver.GPUCmdSetPipeline, ID: 6},
				{Kind: driver.GPUCmdDraw, U0: 3, U1: 0},
			},
		}},
	}, 5, 64, 64)
	if len(errs) != 0 {
		t.Fatalf("depth-only pass errs: %+v", errs)
	}
	if len(gl.drawBufs) != 1 || gl.drawBufs[0] != gpuGLNone {
		t.Errorf("depth-only pass drawBuffers = %v, want [GL_NONE]", gl.drawBufs)
	}

	// Individual destroy ops release GL objects without leaving anything behind.
	_, errs = ctx.executeFrame(driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpDestroyBuffer, ID: 1},
			{Kind: driver.GPUOpDestroyTexture, ID: 2},
			{Kind: driver.GPUOpDestroySampler, ID: 3},
			{Kind: driver.GPUOpDestroyBindGroupLayout, ID: 4},
			{Kind: driver.GPUOpDestroyBindGroup, ID: 5},
			{Kind: driver.GPUOpDestroyPipeline, ID: 6},
		},
	}, 5, 64, 64)
	if len(errs) != 0 {
		t.Fatalf("destroy ops errs: %+v", errs)
	}
	if len(gl.liveBuffers) != 0 || len(gl.liveTextures) != 0 || len(gl.liveSamplers) != 0 || len(gl.liveVAOs) != 0 || len(gl.livePrograms) != 0 {
		t.Errorf("individual destroy ops leaked resources: bufs=%d texs=%d samps=%d vaos=%d progs=%d",
			len(gl.liveBuffers), len(gl.liveTextures), len(gl.liveSamplers), len(gl.liveVAOs), len(gl.livePrograms))
	}

	// Program link failure reports GPUErrorShaderCompile and leaks 0 programs/shaders.
	gl.failLink = true
	_, errs = ctx.executeFrame(driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreatePipeline, ID: 7, Pipeline: &driver.GPUPipelineDesc{Vertex: driver.GPUShaderStage{IR: ir}, Fragment: driver.GPUShaderStage{IR: ir}}},
		},
	}, 5, 64, 64)
	gl.failLink = false
	if len(errs) != 1 || errs[0].code != driver.GPUErrorShaderCompile {
		t.Fatalf("link failure errs = %+v, want GPUErrorShaderCompile", errs)
	}
	if len(gl.livePrograms) != 0 || len(gl.liveShaders) != 0 {
		t.Errorf("leaked program/shader on link failure: progs=%d shaders=%d", len(gl.livePrograms), len(gl.liveShaders))
	}

	// Incomplete offscreen FBO reports GPUErrorFailed and restores defaultFBO.
	gl.fboStatus = 0x8CD6 // GL_FRAMEBUFFER_INCOMPLETE_ATTACHMENT
	_, errs = ctx.executeFrame(driver.GPUFrame{
		Ops: []driver.GPUResourceOp{
			{Kind: driver.GPUOpCreateTexture, ID: 8, Texture: &driver.GPUTextureDesc{Width: 8, Height: 8, Format: driver.GPUFormatRGBA8Unorm, Usage: driver.GPUTextureRenderAttachment}},
		},
		Passes: []driver.GPUPass{{
			ColorAttachments: []driver.GPUColorAttachment{{Texture: 8, LoadOp: driver.GPULoadClear}},
		}},
	}, 5, 64, 64)
	if len(errs) != 1 || errs[0].code != driver.GPUErrorFailed || gl.boundFBO != 5 {
		t.Fatalf("incomplete FBO errs=%+v boundFBO=%d, want GPUErrorFailed and boundFBO=5", errs, gl.boundFBO)
	}
	ctx.destroyAll()
}

func TestGLGPULitTexturedShaderGLSLCodegen(t *testing.T) {
	const litWGSL = `
struct Uniforms {
	mvp: mat4x4<f32>,
	lightDir: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var tex: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

struct VertexIn {
	@location(0) pos: vec3<f32>,
	@location(1) normal: vec3<f32>,
	@location(2) uv: vec2<f32>,
	@location(3) tint: vec4<f32>,
}

struct VertexOut {
	@builtin(position) pos: vec4<f32>,
	@location(0) normal: vec3<f32>,
	@location(1) uv: vec2<f32>,
	@location(2) tint: vec4<f32>,
}

@vertex
fn vs_main(in: VertexIn) -> VertexOut {
	var out: VertexOut;
	out.pos = u.mvp * vec4<f32>(in.pos, 1.0);
	out.normal = in.normal;
	out.uv = in.uv;
	out.tint = in.tint;
	return out;
}

@fragment
fn fs_main(in: VertexOut) -> @location(0) vec4<f32> {
	let ndotl = max(dot(normalize(in.normal), normalize(u.lightDir.xyz)), 0.25);
	let base = textureSample(tex, samp, in.uv) * in.tint;
	return vec4<f32>(base.rgb * ndotl, base.a);
}
`
	ir := compileTestWGSL(t, litWGSL)
	for _, ver := range []glsl.GLSLVersion{glsl.GLSLVersionDefault, glsl.GLSLVersionES300} {
		src, err := compilePipelineGLSL(&driver.GPUPipelineDesc{
			Vertex:   driver.GPUShaderStage{IR: ir, EntryPoint: "vs_main"},
			Fragment: driver.GPUShaderStage{IR: ir, EntryPoint: "fs_main"},
		}, ver)
		if err != nil {
			t.Fatalf("version %v compilePipelineGLSL: %v", ver, err)
		}
		if !strings.Contains(src.VertexSource, "gl_Position.z = gl_Position.z * 2.0 - gl_Position.w;") {
			t.Errorf("version %v vertex shader missing Z remap:\n%s", ver, src.VertexSource)
		}
		if !strings.Contains(src.FragmentSource, "texture(") || !strings.Contains(src.FragmentSource, "normalize(") {
			t.Errorf("version %v fragment shader missing expected texture/lighting ops:\n%s", ver, src.FragmentSource)
		}
	}
}
