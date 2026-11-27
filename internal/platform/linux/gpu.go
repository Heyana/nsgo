//go:build linux && !android

package linux

import (
	"log"
	"math"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// glGPUView hosts a GtkGLArea for a driver.KindGPU view and executes off-commit driver.GPUFrame batches on its OpenGL 3.3 Core or OpenGL ES 3.0 context through libepoxy (purego, zero cgo).
type glGPUView struct {
	hosted
	area              uintptr
	cfg               driver.GPUViewConfig
	ctx               *glGPUContext
	generation        uint32
	realized          bool
	hasRendered       bool
	width             int
	height            int
	surfaceFormat     driver.GPUTextureFormat
	surfaceColorSpace driver.GPUColorSpace
	colorCleanup      func()
	hdrFBO            uint32
	hdrTex            uint32
	hdrW, hdrH        int
	frames            []driver.GPUFrame
}

// Additional OpenGL 3.3 Core / OpenGL ES 3.0 and GtkGLArea functions bound lazily through libepoxy and GTK 4.
var (
	gpuGLOnce bool
	gpuGLOK   bool

	glBindBufferRange                 func(target, index, id uint32, offset, size int)
	glCopyBufferSubData               func(readTarget, writeTarget uint32, readOffset, writeOffset, size int)
	glEnableVertexAttribArray         func(index uint32)
	glDisableVertexAttribArray        func(index uint32)
	glVertexAttribPointer             func(index uint32, size int32, typ uint32, normalized uint8, stride int32, offset uintptr)
	glVertexAttribIPointer            func(index uint32, size int32, typ uint32, stride int32, offset uintptr)
	glVertexAttribDivisor             func(index, divisor uint32)
	glPixelStorei                     func(pname uint32, param int32)
	glTexSubImage2DOffset             func(target uint32, level, x, y, w, h int32, format, typ uint32, offset uintptr)
	glTexImage3D                      func(target uint32, level, internal, w, h, d, border int32, format, typ uint32, data unsafe.Pointer)
	glTexSubImage3D                   func(target uint32, level, x, y, z, w, h, d int32, format, typ uint32, data unsafe.Pointer)
	glTexSubImage3DOffset             func(target uint32, level, x, y, z, w, h, d int32, format, typ uint32, offset uintptr)
	glGenSamplers                     func(n int32, out *uint32)
	glDeleteSamplers                  func(n int32, ids *uint32)
	glBindSampler                     func(unit, sampler uint32)
	glSamplerParameteri               func(sampler, pname uint32, param int32)
	glSamplerParameterf               func(sampler, pname uint32, param float32)
	glGenFramebuffers                 func(n int32, out *uint32)
	glDeleteFramebuffers              func(n int32, ids *uint32)
	glBindFramebuffer                 func(target, id uint32)
	glFramebufferTexture2D            func(target, attachment, texTarget, texture uint32, level int32)
	glFramebufferTextureLayer         func(target, attachment, texture uint32, level, layer int32)
	glDrawBuffers                     func(n int32, bufs *uint32)
	glCheckFramebufferStatus          func(target uint32) uint32
	glBlitFramebuffer                 func(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32)
	glReadBuffer                      func(src uint32)
	glReadPixels                      func(x, y, width, height int32, format, typ uint32, data unsafe.Pointer)
	glEnable                          func(cap uint32)
	glDisable                         func(cap uint32)
	glScissor                         func(x, y, w, h int32)
	glCullFace                        func(mode uint32)
	glFrontFace                       func(mode uint32)
	glDepthMask                       func(flag uint8)
	glDepthFunc                       func(fn uint32)
	glPolygonOffset                   func(factor, units float32)
	glColorMask                       func(r, g, b, a uint8)
	glBlendFuncSeparate               func(srcRGB, dstRGB, srcA, dstA uint32)
	glBlendEquationSeparate           func(modeRGB, modeA uint32)
	glBlendColor                      func(r, g, b, a float32)
	glClearDepth                      func(d float64)
	glClearDepthf                     func(d float32)
	glClearStencil                    func(s int32)
	glDepthRange                      func(near, far float64)
	glDepthRangef                     func(near, far float32)
	glDrawArraysInstanced             func(mode uint32, first, count, instanceCount int32)
	glDrawElements                    func(mode uint32, count int32, typ uint32, offset uintptr)
	glDrawElementsInstanced           func(mode uint32, count int32, typ uint32, offset uintptr, instanceCount int32)
	glDrawElementsBaseVertex          func(mode uint32, count int32, typ uint32, offset uintptr, baseVertex int32)
	glDrawElementsInstancedBaseVertex func(mode uint32, count int32, typ uint32, offset uintptr, instanceCount, baseVertex int32)
	glGetIntegerv                     func(pname uint32, data *int32)
	glGetError                        func() uint32

	gtkGLAreaSetHasDepthBuffer   func(uintptr, int32)
	gtkGLAreaSetHasStencilBuffer func(uintptr, int32)
	gtkGLAreaSetAutoRender       func(uintptr, int32)
	gtkGLAreaGetContext          func(uintptr) uintptr
)

// loadGPU binds the OpenGL 3.3 Core / ES 3.0 and GtkGLArea functions once and reports whether Linux GPU views are supported on this machine.
func loadGPU() bool {
	if gpuGLOnce {
		return gpuGLOK
	}
	gpuGLOnce = true
	if !loadGL() {
		return false
	}
	epoxy, err := purego.Dlopen("libepoxy.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		log.Printf("nsgo: gpu: %v", err)
		return false
	}
	ok := true
	fn := func(out any, name string) {
		addr, err := purego.Dlsym(epoxy, "epoxy_"+name)
		if err != nil {
			ok = false
			return
		}
		ptr := *(*uintptr)(cPointer(addr))
		if ptr == 0 {
			ok = false
			return
		}
		purego.RegisterFunc(out, ptr)
	}
	opt := func(out any, name string) {
		addr, err := purego.Dlsym(epoxy, "epoxy_"+name)
		if err != nil {
			return
		}
		ptr := *(*uintptr)(cPointer(addr))
		if ptr == 0 {
			return
		}
		purego.RegisterFunc(out, ptr)
	}

	fn(&glBindBufferRange, "glBindBufferRange")
	fn(&glCopyBufferSubData, "glCopyBufferSubData")
	fn(&glEnableVertexAttribArray, "glEnableVertexAttribArray")
	fn(&glDisableVertexAttribArray, "glDisableVertexAttribArray")
	fn(&glVertexAttribPointer, "glVertexAttribPointer")
	fn(&glVertexAttribIPointer, "glVertexAttribIPointer")
	fn(&glVertexAttribDivisor, "glVertexAttribDivisor")
	fn(&glPixelStorei, "glPixelStorei")
	fn(&glTexSubImage2DOffset, "glTexSubImage2D")
	fn(&glTexImage3D, "glTexImage3D")
	fn(&glTexSubImage3D, "glTexSubImage3D")
	fn(&glTexSubImage3DOffset, "glTexSubImage3D")
	fn(&glGenSamplers, "glGenSamplers")
	fn(&glDeleteSamplers, "glDeleteSamplers")
	fn(&glBindSampler, "glBindSampler")
	fn(&glSamplerParameteri, "glSamplerParameteri")
	fn(&glSamplerParameterf, "glSamplerParameterf")
	fn(&glGenFramebuffers, "glGenFramebuffers")
	fn(&glDeleteFramebuffers, "glDeleteFramebuffers")
	fn(&glBindFramebuffer, "glBindFramebuffer")
	fn(&glFramebufferTexture2D, "glFramebufferTexture2D")
	fn(&glFramebufferTextureLayer, "glFramebufferTextureLayer")
	fn(&glDrawBuffers, "glDrawBuffers")
	fn(&glCheckFramebufferStatus, "glCheckFramebufferStatus")
	fn(&glBlitFramebuffer, "glBlitFramebuffer")
	fn(&glReadBuffer, "glReadBuffer")
	fn(&glReadPixels, "glReadPixels")
	fn(&glEnable, "glEnable")
	fn(&glDisable, "glDisable")
	fn(&glScissor, "glScissor")
	fn(&glCullFace, "glCullFace")
	fn(&glFrontFace, "glFrontFace")
	fn(&glDepthMask, "glDepthMask")
	fn(&glDepthFunc, "glDepthFunc")
	fn(&glPolygonOffset, "glPolygonOffset")
	fn(&glColorMask, "glColorMask")
	fn(&glBlendFuncSeparate, "glBlendFuncSeparate")
	fn(&glBlendEquationSeparate, "glBlendEquationSeparate")
	fn(&glBlendColor, "glBlendColor")
	fn(&glClearStencil, "glClearStencil")
	fn(&glDrawArraysInstanced, "glDrawArraysInstanced")
	fn(&glDrawElements, "glDrawElements")
	fn(&glDrawElementsInstanced, "glDrawElementsInstanced")
	fn(&glGetIntegerv, "glGetIntegerv")
	fn(&glGetError, "glGetError")

	// API-specific depth range / base-vertex entry points.
	opt(&glClearDepth, "glClearDepth")
	opt(&glClearDepthf, "glClearDepthf")
	opt(&glDepthRange, "glDepthRange")
	opt(&glDepthRangef, "glDepthRangef")
	opt(&glDrawElementsBaseVertex, "glDrawElementsBaseVertex")
	opt(&glDrawElementsInstancedBaseVertex, "glDrawElementsInstancedBaseVertex")

	if !ok {
		log.Printf("nsgo: gpu: libepoxy lacks required OpenGL 3.3 / ES 3.0 functions")
		return false
	}

	purego.RegisterLibFunc(&gtkGLAreaSetHasDepthBuffer, gtkLib, "gtk_gl_area_set_has_depth_buffer")
	purego.RegisterLibFunc(&gtkGLAreaSetHasStencilBuffer, gtkLib, "gtk_gl_area_set_has_stencil_buffer")
	purego.RegisterLibFunc(&gtkGLAreaSetAutoRender, gtkLib, "gtk_gl_area_set_auto_render")
	purego.RegisterLibFunc(&gtkGLAreaGetContext, gtkLib, "gtk_gl_area_get_context")

	gpuGLOK = true
	return true
}

func slicePtr(b []byte) unsafe.Pointer {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Pointer(&b[0])
}

func boolU8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// epoxyGL adapts libepoxy's purego-bound OpenGL functions to glAPI.
type epoxyGL struct {
	es bool
}

func (e epoxyGL) CreateShader(kind uint32) uint32 { return glCreateShader(kind) }
func (e epoxyGL) ShaderSource(shader uint32, src string) {
	cs := cstr(src)
	glShaderSource(shader, 1, &cs, nil)
	runtime.KeepAlive(cs)
}
func (e epoxyGL) CompileShader(shader uint32) { glCompileShader(shader) }
func (e epoxyGL) GetShaderiv(shader, pname uint32) int32 {
	var v int32
	glGetShaderiv(shader, pname, &v)
	return v
}
func (e epoxyGL) GetShaderInfoLog(shader uint32) string {
	buf := make([]byte, 4096)
	var n int32
	glGetShaderInfoLog(shader, int32(len(buf)), &n, &buf[0])
	return string(buf[:max(n, 0)])
}
func (e epoxyGL) DeleteShader(shader uint32)       { glDeleteShader(shader) }
func (e epoxyGL) CreateProgram() uint32            { return glCreateProgram() }
func (e epoxyGL) AttachShader(prog, shader uint32) { glAttachShader(prog, shader) }
func (e epoxyGL) LinkProgram(prog uint32)          { glLinkProgram(prog) }
func (e epoxyGL) GetProgramiv(prog, pname uint32) int32 {
	var v int32
	glGetProgramiv(prog, pname, &v)
	return v
}
func (e epoxyGL) GetProgramInfoLog(prog uint32) string {
	buf := make([]byte, 4096)
	var n int32
	glGetProgramInfoLog(prog, int32(len(buf)), &n, &buf[0])
	return string(buf[:max(n, 0)])
}
func (e epoxyGL) DeleteProgram(prog uint32) { glDeleteProgram(prog) }
func (e epoxyGL) UseProgram(prog uint32)    { glUseProgram(prog) }
func (e epoxyGL) GetUniformLocation(prog uint32, name string) int32 {
	cs := cstr(name)
	loc := glGetUniformLocation(prog, cs)
	runtime.KeepAlive(cs)
	return loc
}
func (e epoxyGL) Uniform1i(loc, v int32) { glUniform1i(loc, v) }
func (e epoxyGL) GetUniformBlockIndex(prog uint32, name string) uint32 {
	cs := cstr(name)
	idx := glGetUniformBlockIdx(prog, cs)
	runtime.KeepAlive(cs)
	return idx
}
func (e epoxyGL) UniformBlockBinding(prog, index, binding uint32) {
	glUniformBlockBind(prog, index, binding)
}

func (e epoxyGL) GenBuffer() uint32 {
	var id uint32
	glGenBuffers(1, &id)
	return id
}
func (e epoxyGL) DeleteBuffer(id uint32)       { glDeleteBuffers(1, &id) }
func (e epoxyGL) BindBuffer(target, id uint32) { glBindBuffer(target, id) }
func (e epoxyGL) BufferData(target uint32, size int, data []byte, usage uint32) {
	glBufferData(target, size, slicePtr(data), usage)
	runtime.KeepAlive(data)
}
func (e epoxyGL) BufferSubData(target uint32, offset int, data []byte) {
	if len(data) == 0 {
		return
	}
	glBufferSubData(target, offset, len(data), slicePtr(data))
	runtime.KeepAlive(data)
}
func (e epoxyGL) BindBufferBase(target, index, id uint32) { glBindBufferBase(target, index, id) }
func (e epoxyGL) BindBufferRange(target, index, id uint32, offset, size int) {
	glBindBufferRange(target, index, id, offset, size)
}
func (e epoxyGL) CopyBufferSubData(readTarget, writeTarget uint32, readOffset, writeOffset, size int) {
	glCopyBufferSubData(readTarget, writeTarget, readOffset, writeOffset, size)
}

func (e epoxyGL) GenVertexArray() uint32 {
	var id uint32
	glGenVertexArrays(1, &id)
	return id
}
func (e epoxyGL) DeleteVertexArray(id uint32)       { glDeleteVertexArrays(1, &id) }
func (e epoxyGL) BindVertexArray(id uint32)         { glBindVertexArray(id) }
func (e epoxyGL) EnableVertexAttribArray(i uint32)  { glEnableVertexAttribArray(i) }
func (e epoxyGL) DisableVertexAttribArray(i uint32) { glDisableVertexAttribArray(i) }
func (e epoxyGL) VertexAttribPointer(index uint32, size int32, typ uint32, normalized bool, stride int32, offset uintptr) {
	glVertexAttribPointer(index, size, typ, boolU8(normalized), stride, offset)
}
func (e epoxyGL) VertexAttribIPointer(index uint32, size int32, typ uint32, stride int32, offset uintptr) {
	glVertexAttribIPointer(index, size, typ, stride, offset)
}
func (e epoxyGL) VertexAttribDivisor(index, divisor uint32) { glVertexAttribDivisor(index, divisor) }

func (e epoxyGL) GenTexture() uint32 {
	var id uint32
	glGenTextures(1, &id)
	return id
}
func (e epoxyGL) DeleteTexture(id uint32)           { glDeleteTextures(1, &id) }
func (e epoxyGL) ActiveTexture(unit uint32)         { glActiveTexture(unit) }
func (e epoxyGL) BindTexture(target, id uint32)     { glBindTexture(target, id) }
func (e epoxyGL) PixelStorei(pname uint32, p int32) { glPixelStorei(pname, p) }
func (e epoxyGL) TexImage2D(target uint32, level, internalFormat, width, height, border int32, format, typ uint32, data []byte) {
	glTexImage2D(target, level, internalFormat, width, height, border, format, typ, slicePtr(data))
	runtime.KeepAlive(data)
}
func (e epoxyGL) TexSubImage2D(target uint32, level, x, y, width, height int32, format, typ uint32, data []byte) {
	glTexSubImage2D(target, level, x, y, width, height, format, typ, slicePtr(data))
	runtime.KeepAlive(data)
}
func (e epoxyGL) TexSubImage2DOffset(target uint32, level, x, y, width, height int32, format, typ uint32, offset uintptr) {
	glTexSubImage2DOffset(target, level, x, y, width, height, format, typ, offset)
}
func (e epoxyGL) TexImage3D(target uint32, level, internalFormat, width, height, depth, border int32, format, typ uint32, data []byte) {
	glTexImage3D(target, level, internalFormat, width, height, depth, border, format, typ, slicePtr(data))
	runtime.KeepAlive(data)
}
func (e epoxyGL) TexSubImage3D(target uint32, level, x, y, z, width, height, depth int32, format, typ uint32, data []byte) {
	glTexSubImage3D(target, level, x, y, z, width, height, depth, format, typ, slicePtr(data))
	runtime.KeepAlive(data)
}
func (e epoxyGL) TexSubImage3DOffset(target uint32, level, x, y, z, width, height, depth int32, format, typ uint32, offset uintptr) {
	glTexSubImage3DOffset(target, level, x, y, z, width, height, depth, format, typ, offset)
}
func (e epoxyGL) TexParameteri(target, pname uint32, param int32) {
	glTexParameteri(target, pname, param)
}

func (e epoxyGL) GenSampler() uint32 {
	var id uint32
	glGenSamplers(1, &id)
	return id
}
func (e epoxyGL) DeleteSampler(id uint32)                    { glDeleteSamplers(1, &id) }
func (e epoxyGL) BindSampler(unit, sampler uint32)           { glBindSampler(unit, sampler) }
func (e epoxyGL) SamplerParameteri(s, pname uint32, p int32) { glSamplerParameteri(s, pname, p) }
func (e epoxyGL) SamplerParameterf(s, pname uint32, p float32) {
	glSamplerParameterf(s, pname, p)
}

func (e epoxyGL) GenFramebuffer() uint32 {
	var id uint32
	glGenFramebuffers(1, &id)
	return id
}
func (e epoxyGL) DeleteFramebuffer(id uint32)       { glDeleteFramebuffers(1, &id) }
func (e epoxyGL) BindFramebuffer(target, id uint32) { glBindFramebuffer(target, id) }
func (e epoxyGL) FramebufferTexture2D(target, attachment, texTarget, texture uint32, level int32) {
	glFramebufferTexture2D(target, attachment, texTarget, texture, level)
}
func (e epoxyGL) FramebufferTextureLayer(target, attachment, texture uint32, level, layer int32) {
	glFramebufferTextureLayer(target, attachment, texture, level, layer)
}
func (e epoxyGL) DrawBuffers(bufs []uint32) {
	if len(bufs) == 0 {
		return
	}
	glDrawBuffers(int32(len(bufs)), &bufs[0])
	runtime.KeepAlive(bufs)
}
func (e epoxyGL) CheckFramebufferStatus(target uint32) uint32 {
	return glCheckFramebufferStatus(target)
}
func (e epoxyGL) BlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32) {
	glBlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1, mask, filter)
}
func (e epoxyGL) ReadBuffer(src uint32) { glReadBuffer(src) }
func (e epoxyGL) ReadPixels(x, y, width, height int32, format, typ uint32, dst []byte) {
	if len(dst) == 0 {
		return
	}
	glReadPixels(x, y, width, height, format, typ, slicePtr(dst))
	runtime.KeepAlive(dst)
}

func (e epoxyGL) Viewport(x, y, width, height int32) { glViewport(x, y, width, height) }
func (e epoxyGL) DepthRangef(near, far float32) {
	if e.es && glDepthRangef != nil {
		glDepthRangef(near, far)
	} else if glDepthRange != nil {
		glDepthRange(float64(near), float64(far))
	} else if glDepthRangef != nil {
		glDepthRangef(near, far)
	}
}
func (e epoxyGL) Enable(cap uint32)                   { glEnable(cap) }
func (e epoxyGL) Disable(cap uint32)                  { glDisable(cap) }
func (e epoxyGL) Scissor(x, y, width, height int32)   { glScissor(x, y, width, height) }
func (e epoxyGL) CullFace(mode uint32)                { glCullFace(mode) }
func (e epoxyGL) FrontFace(mode uint32)               { glFrontFace(mode) }
func (e epoxyGL) DepthMask(flag bool)                 { glDepthMask(boolU8(flag)) }
func (e epoxyGL) DepthFunc(fn uint32)                 { glDepthFunc(fn) }
func (e epoxyGL) PolygonOffset(factor, units float32) { glPolygonOffset(factor, units) }
func (e epoxyGL) ColorMask(r, g, b, a bool) {
	glColorMask(boolU8(r), boolU8(g), boolU8(b), boolU8(a))
}
func (e epoxyGL) BlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha uint32) {
	glBlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha)
}
func (e epoxyGL) BlendEquationSeparate(modeRGB, modeAlpha uint32) {
	glBlendEquationSeparate(modeRGB, modeAlpha)
}
func (e epoxyGL) BlendColor(r, g, b, a float32) { glBlendColor(r, g, b, a) }
func (e epoxyGL) ClearColor(r, g, b, a float32) { glClearColor(r, g, b, a) }
func (e epoxyGL) ClearDepthf(d float32) {
	if e.es && glClearDepthf != nil {
		glClearDepthf(d)
	} else if glClearDepth != nil {
		glClearDepth(float64(d))
	} else if glClearDepthf != nil {
		glClearDepthf(d)
	}
}
func (e epoxyGL) ClearStencil(s int32) { glClearStencil(s) }
func (e epoxyGL) Clear(mask uint32)    { glClear(mask) }

func (e epoxyGL) DrawArrays(mode uint32, first, count int32) { glDrawArrays(mode, first, count) }
func (e epoxyGL) DrawArraysInstanced(mode uint32, first, count, instanceCount int32) {
	glDrawArraysInstanced(mode, first, count, instanceCount)
}
func (e epoxyGL) DrawElements(mode uint32, count int32, typ uint32, offset uintptr) {
	glDrawElements(mode, count, typ, offset)
}
func (e epoxyGL) DrawElementsInstanced(mode uint32, count int32, typ uint32, offset uintptr, instanceCount int32) {
	glDrawElementsInstanced(mode, count, typ, offset, instanceCount)
}
func (e epoxyGL) DrawElementsBaseVertex(mode uint32, count int32, typ uint32, offset uintptr, baseVertex int32) {
	if !e.es && glDrawElementsBaseVertex != nil {
		glDrawElementsBaseVertex(mode, count, typ, offset, baseVertex)
		return
	}
	glDrawElements(mode, count, typ, offset)
}
func (e epoxyGL) DrawElementsInstancedBaseVertex(mode uint32, count int32, typ uint32, offset uintptr, instanceCount, baseVertex int32) {
	if !e.es && glDrawElementsInstancedBaseVertex != nil {
		glDrawElementsInstancedBaseVertex(mode, count, typ, offset, instanceCount, baseVertex)
		return
	}
	glDrawElementsInstanced(mode, count, typ, offset, instanceCount)
}

func (e epoxyGL) GetIntegerv(pname uint32) int32 {
	var v int32
	glGetIntegerv(pname, &v)
	return v
}
func (e epoxyGL) GetError() uint32 { return glGetError() }

// setGPU configures v's GtkGLArea for 3D rendering, creating it on first mount.
func (w *window) setGPU(v *view, cfg driver.GPUViewConfig) {
	if v.gpu == nil {
		if !loadGPU() {
			w.dispatchGPUError(v.id, 0, driver.GPUErrorUnsupported, "OpenGL 3.3 / ES 3.0 unavailable via libepoxy")
			return
		}
		g := &glGPUView{
			area: gtkGLAreaNew(),
			cfg:  cfg,
		}
		v.gpu = g
		if gtkGLAreaSetAllowedAPIs != nil {
			const both = 1 | 2 // GDK_GL_API_GL | GDK_GL_API_GLES
			gtkGLAreaSetAllowedAPIs(g.area, both)
		}
		gtkGLAreaSetHasDepthBuffer(g.area, boolInt(cfg.DepthFormat != driver.GPUFormatUndefined))
		gtkGLAreaSetHasStencilBuffer(g.area, boolInt(cfg.DepthFormat == driver.GPUFormatDepth24PlusStencil8))
		// Keep the last rendered framebuffer across HUD/window repaints; SubmitGPU calls gtk_gl_area_queue_render for each new GPUFrame.
		gtkGLAreaSetAutoRender(g.area, 0)
		// Host underneath v's children so HUD overlays paint above the 3D surface and receive pointer events normally.
		w.hostUnder(v, &g.hosted, g.area, false)
		w.connect(v, g.area, "realize", cbGPURealize)
		w.connect(v, g.area, "resize", cbGPUResize)
		w.connect(v, g.area, "render", cbGPURender)
		w.connect(v, g.area, "unrealize", cbGPUUnrealize)
		return
	}
	v.gpu.cfg = cfg
	gtkGLAreaSetHasDepthBuffer(v.gpu.area, boolInt(cfg.DepthFormat != driver.GPUFormatUndefined))
	gtkGLAreaSetHasStencilBuffer(v.gpu.area, boolInt(cfg.DepthFormat == driver.GPUFormatDepth24PlusStencil8))
}

// closeGPU destroys all OpenGL resources for v and unhosts its GtkGLArea.
func (w *window) closeGPU(v *view) {
	g := v.gpu
	if g == nil {
		return
	}
	w.gpuMu.Lock()
	pending := w.gpuPending[v.id]
	delete(w.gpuPending, v.id)
	w.gpuMu.Unlock()
	completeGPUFrames(pending)
	if g.ctx != nil && g.area != 0 {
		gtkGLAreaMakeCurrent(g.area)
		if gtkGLAreaGetError(g.area) == 0 {
			g.releaseHDRTarget()
			g.ctx.destroyAll()
		}
		g.ctx = nil
	}
	if g.colorCleanup != nil {
		g.colorCleanup()
		g.colorCleanup = nil
	}
	completeGPUFrames(g.frames)
	g.frames = nil
	g.realized = false
	g.hasRendered = false
	w.unhost(v, &g.hosted)
	v.gpu = nil
}

// SubmitGPU implements engine.Platform: queues an off-commit GPUFrame from the engine goroutine and wakes the GTK main loop to schedule gtk_gl_area_queue_render without touching the UI view tree.
func (w *window) SubmitGPU(frame driver.GPUFrame) {
	w.gpuMu.Lock()
	if w.gpuPending == nil {
		w.gpuPending = map[driver.NodeID][]driver.GPUFrame{}
	}
	w.gpuPending[frame.ViewID] = append(w.gpuPending[frame.ViewID], frame)
	wake := !w.gpuPosted
	w.gpuPosted = true
	w.gpuMu.Unlock()
	if wake {
		gIdleApply(cbGPUFlush, w.handle)
	}
}

func gpuFlushCallback(data uintptr) int32 {
	if w := windowOf(data); w != nil && !w.closing {
		w.flushGPU()
	}
	return 0
}

func (w *window) flushGPU() {
	w.gpuMu.Lock()
	pending := w.gpuPending
	w.gpuPending = nil
	w.gpuPosted = false
	w.gpuMu.Unlock()
	for id, frames := range pending {
		v := w.view(id)
		if v == nil || v.deleted || v.gpu == nil || v.gpu.area == 0 {
			completeGPUFrames(frames)
			continue
		}
		v.gpu.frames = append(v.gpu.frames, frames...)
		gtkGLAreaQueueRender(v.gpu.area)
	}
}

func glAreaErrorString(area uintptr) string {
	e := gtkGLAreaGetError(area)
	if e == 0 {
		return ""
	}
	return goString((*gError)(cPointer(e)).message)
}

func (w *window) dispatchGPUError(id driver.NodeID, gen uint32, code driver.GPUErrorCode, msg string) {
	if w.engine == nil || w.closing {
		return
	}
	w.engine.Dispatch(driver.Event{
		Kind: driver.EventGPU,
		ID:   id,
		Data: &driver.GPUEvent{
			Kind:       driver.GPUEventError,
			Generation: gen,
			ErrorCode:  code,
			Message:    msg,
		},
	})
}

func gpuPhysicalSize(v *view, area uintptr) (int, int) {
	scale := float64(max(gtkWidgetGetScaleFactorG(area), 1))
	pw := int(math.Round(float64(gtkWidgetGetWidth(area)) * scale))
	ph := int(math.Round(float64(gtkWidgetGetHeight(area)) * scale))
	if pw <= 0 || ph <= 0 {
		f := v.shownFrame().Size
		pw = int(math.Ceil(float64(f.Width) * scale))
		ph = int(math.Ceil(float64(f.Height) * scale))
	}
	return max(pw, 1), max(ph, 1)
}

func (w *window) initGPUContext(v *view, g *glGPUView, context uintptr) bool {
	if msg := glAreaErrorString(g.area); msg != "" {
		w.dispatchGPUError(v.id, g.generation, driver.GPUErrorUnsupported, msg)
		return false
	}
	if context == 0 {
		context = gtkGLAreaGetContext(g.area)
	}
	es := context != 0 && gdkGLContextGetUseES(context) != 0
	api := epoxyGL{es: es}
	lim := mergeGPULimits(
		api.GetIntegerv(gpuGLMaxTextureSize),
		api.GetIntegerv(gpuGLMaxUniformBlockSize),
		api.GetIntegerv(gpuGLUniformBufferOffsetAlign),
		api.GetIntegerv(gpuGLMaxVertexAttribs),
	)
	if max3D := api.GetIntegerv(gpuGLMax3DTextureSize); max3D > 0 {
		lim.MaxTextureDimension3D = uint32(max3D)
	}
	feat := driver.GPUFeatures{
		Depth32Float: true,
		Texture3D:    true,
	}
	importer := newEGLImporter()
	feat.ImportDMABuf = importer != nil
	g.generation++
	g.ctx = newGLGPUContext(api, es, g.generation, lim, feat)
	if importer != nil {
		g.ctx.importer = importer
	}
	if g.colorCleanup != nil {
		g.colorCleanup()
		g.colorCleanup = nil
	}
	var gdkSurf uintptr
	if w.surface != 0 {
		if native := gtkWidgetGetNative(w.surface); native != 0 {
			gdkSurf = gtkNativeGetSurface(native)
		}
	}
	negotiatedCS, colorCleanup := setWaylandSurfaceColorSpace(gdkSurf, g.cfg.ColorSpace)
	surfFmt, surfCS := resolveLinuxGPUSurface(g.cfg, negotiatedCS)
	g.surfaceFormat = surfFmt
	g.surfaceColorSpace = surfCS
	g.colorCleanup = colorCleanup

	g.realized = true
	g.hasRendered = false
	pw, ph := gpuPhysicalSize(v, g.area)
	g.width, g.height = pw, ph
	if w.engine != nil && !w.closing {
		w.engine.Dispatch(driver.Event{
			Kind: driver.EventGPU,
			ID:   v.id,
			Data: &driver.GPUEvent{
				Kind:              driver.GPUEventDeviceReady,
				Generation:        g.generation,
				Width:             pw,
				Height:            ph,
				SurfaceFormat:     surfFmt,
				SurfaceColorSpace: surfCS,
				Limits:            lim,
				Features:          feat,
			},
		})
	}
	return true
}

func (g *glGPUView) releaseHDRTarget() {
	if g.ctx == nil {
		g.hdrFBO, g.hdrTex, g.hdrW, g.hdrH = 0, 0, 0, 0
		return
	}
	if g.hdrFBO != 0 {
		g.ctx.gl.DeleteFramebuffer(g.hdrFBO)
		g.hdrFBO = 0
	}
	if g.hdrTex != 0 {
		g.ctx.gl.DeleteTexture(g.hdrTex)
		g.hdrTex = 0
	}
	g.hdrW, g.hdrH = 0, 0
}

func (g *glGPUView) ensureHDRTarget(pw, ph int) uint32 {
	if g.surfaceFormat == driver.GPUFormatRGBA8Unorm || g.ctx == nil {
		return 0
	}
	if g.hdrFBO != 0 && g.hdrW == pw && g.hdrH == ph {
		return g.hdrFBO
	}
	g.releaseHDRTarget()
	internalFmt, pixelFmt, pixelType, _, ok := glTextureFormatInfo(g.surfaceFormat)
	if !ok {
		return 0
	}
	gl := g.ctx.gl
	tex := gl.GenTexture()
	gl.BindTexture(gpuGLTexture2D, tex)
	gl.TexImage2D(gpuGLTexture2D, 0, internalFmt, int32(pw), int32(ph), 0, pixelFmt, pixelType, nil)
	gl.TexParameteri(gpuGLTexture2D, gpuGLTextureMinFilter, gpuGLNearest)
	gl.TexParameteri(gpuGLTexture2D, gpuGLTextureMagFilter, gpuGLNearest)
	gl.BindTexture(gpuGLTexture2D, 0)

	fbo := gl.GenFramebuffer()
	gl.BindFramebuffer(gpuGLFramebuffer, fbo)
	gl.FramebufferTexture2D(gpuGLFramebuffer, gpuGLColorAttachment0, gpuGLTexture2D, tex, 0)
	if gl.CheckFramebufferStatus(gpuGLFramebuffer) != gpuGLFramebufferComplete {
		gl.BindFramebuffer(gpuGLFramebuffer, 0)
		gl.DeleteFramebuffer(fbo)
		gl.DeleteTexture(tex)
		return 0
	}
	g.hdrFBO = fbo
	g.hdrTex = tex
	g.hdrW, g.hdrH = pw, ph
	return fbo
}

func gpuRealizeCallback(area, data uintptr) {
	w, v := nativeOf(data, area)
	if v == nil || v.gpu == nil || v.gpu.area != area {
		return
	}
	gtkGLAreaMakeCurrent(area)
	if !v.gpu.realized {
		w.initGPUContext(v, v.gpu, 0)
	}
}

func gpuResizeCallback(area uintptr, width, height int32, data uintptr) {
	w, v := nativeOf(data, area)
	if v == nil || v.gpu == nil || v.gpu.area != area {
		return
	}
	g := v.gpu
	pw, ph := max(int(width), 1), max(int(height), 1)
	if !g.realized {
		g.width, g.height = pw, ph
		return
	}
	if pw == g.width && ph == g.height {
		return
	}
	g.width, g.height = pw, ph
	g.hasRendered = false
	if w.engine != nil && !w.closing {
		w.engine.Dispatch(driver.Event{
			Kind: driver.EventGPU,
			ID:   v.id,
			Data: &driver.GPUEvent{
				Kind:       driver.GPUEventResize,
				Generation: g.generation,
				Width:      pw,
				Height:     ph,
			},
		})
	}
}

func gpuRenderCallback(area, context, data uintptr) uintptr {
	w, v := nativeOf(data, area)
	if v == nil || v.gpu == nil || v.gpu.area != area {
		return 0
	}
	g := v.gpu
	if msg := glAreaErrorString(area); msg != "" {
		if g.realized {
			g.realized = false
			if g.ctx != nil {
				g.ctx = nil
			}
			if w.engine != nil && !w.closing {
				w.engine.Dispatch(driver.Event{
					Kind: driver.EventGPU,
					ID:   v.id,
					Data: &driver.GPUEvent{
						Kind:       driver.GPUEventDeviceLost,
						Generation: g.generation,
						LostReason: driver.GPULostReset,
						Message:    msg,
					},
				})
			}
		} else {
			w.dispatchGPUError(v.id, g.generation, driver.GPUErrorUnsupported, msg)
		}
		return 0
	}
	if !g.realized || g.ctx == nil {
		if !w.initGPUContext(v, g, context) {
			return 0
		}
	}

	// GtkGLArea binds its own internal framebuffer before emitting "render".
	defaultFBO := uint32(g.ctx.gl.GetIntegerv(gpuGLFramebufferBinding))
	pw, ph := gpuPhysicalSize(v, area)
	if pw != g.width || ph != g.height {
		g.width, g.height = pw, ph
		g.hasRendered = false
		if w.engine != nil && !w.closing {
			w.engine.Dispatch(driver.Event{
				Kind: driver.EventGPU,
				ID:   v.id,
				Data: &driver.GPUEvent{
					Kind:       driver.GPUEventResize,
					Generation: g.generation,
					Width:      pw,
					Height:     ph,
				},
			})
		}
	}

	targetFBO := defaultFBO
	if hdrFBO := g.ensureHDRTarget(pw, ph); hdrFBO != 0 {
		targetFBO = hdrFBO
	}

	frames := g.frames
	g.frames = nil
	defer completeGPUFrames(frames)
	for _, fr := range frames {
		if fr.Generation != 0 && fr.Generation != g.generation {
			continue
		}
		readbacks, errs := g.ctx.executeFrame(fr, targetFBO, int32(pw), int32(ph))
		for _, pass := range fr.Passes {
			if !passIsOffscreen(pass) {
				g.hasRendered = true
				break
			}
		}
		for _, e := range errs {
			w.dispatchGPUError(v.id, g.generation, e.code, e.message)
		}
		for _, rb := range readbacks {
			if w.engine != nil && !w.closing {
				w.engine.Dispatch(driver.Event{
					Kind: driver.EventGPU,
					ID:   v.id,
					Data: &driver.GPUEvent{
						Kind:       driver.GPUEventReadback,
						Generation: g.generation,
						ReadbackID: rb.id,
						Pixels:     rb.pixels,
						Message:    rb.errMsg,
					},
				})
			}
		}
	}
	if targetFBO != defaultFBO && g.hasRendered {
		g.ctx.gl.BindFramebuffer(gpuGLReadFramebuffer, targetFBO)
		g.ctx.gl.BindFramebuffer(gpuGLDrawFramebuffer, defaultFBO)
		g.ctx.gl.BlitFramebuffer(0, 0, int32(pw), int32(ph), 0, 0, int32(pw), int32(ph), gpuGLColorBufferBit, gpuGLNearest)
		g.ctx.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
	}
	if !g.hasRendered {
		g.ctx.gl.BindFramebuffer(gpuGLFramebuffer, defaultFBO)
		g.ctx.gl.Disable(gpuGLScissorTest)
		g.ctx.gl.ColorMask(true, true, true, true)
		g.ctx.gl.Viewport(0, 0, int32(pw), int32(ph))
		g.ctx.gl.ClearColor(0, 0, 0, 0)
		g.ctx.gl.Clear(gpuGLColorBufferBit)
	}
	return 1
}

func gpuUnrealizeCallback(area, data uintptr) {
	w, v := nativeOf(data, area)
	if v == nil || v.gpu == nil || v.gpu.area != area {
		return
	}
	g := v.gpu
	gtkGLAreaMakeCurrent(area)
	if gtkGLAreaGetError(area) == 0 && g.ctx != nil {
		g.releaseHDRTarget()
		g.ctx.destroyAll()
	}
	if g.colorCleanup != nil {
		g.colorCleanup()
		g.colorCleanup = nil
	}
	g.ctx = nil
	wasRealized := g.realized
	g.realized = false
	g.hasRendered = false
	completeGPUFrames(g.frames)
	g.frames = nil
	if wasRealized && !v.deleted && !w.closing && w.engine != nil {
		w.engine.Dispatch(driver.Event{
			Kind: driver.EventGPU,
			ID:   v.id,
			Data: &driver.GPUEvent{
				Kind:       driver.GPUEventDeviceLost,
				Generation: g.generation,
				LostReason: driver.GPULostDestroyed,
				Message:    "GL context unrealized",
			},
		})
	}
}

// completeGPUFrames reports frames done, rendered or dropped, so the FrameFences gpu.Device waits on for their ring slots are released.
func completeGPUFrames(frames []driver.GPUFrame) {
	for _, f := range frames {
		if f.OnComplete != nil {
			f.OnComplete()
		}
	}
}
