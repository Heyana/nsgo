//go:build linux && !android

package linux

import (
	"image"
	"log"
	"math"
	"strconv"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasshade"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

// A canvas op that shades with a program (canvasshade) renders its pass
// with OpenGL 3.3 or OpenGL ES 3.0 in a context of the display's own,
// which draws to no window, into a framebuffer of the pass's size, reads
// the pixels back and paints them into the Cairo context in its device
// pixels, under the clip, the alpha, the operator and the shadow the
// player keeps. The context is the process's, made through GDK so that
// GDK knows which context is current; the one current before a pass is
// current again after it.

// shadeCanvas runs a shaded op, set by EnableCanvasShaders: nil leaves
// DrawShader out and draws shaded images plain.
var shadeCanvas func(p *canvasPlayer, op *driver.DrawOp) bool

// EnableCanvasShaders implements driver.CanvasShaderEnabler: it links the
// GL path of canvas ops that shade with programs.
func (*window) EnableCanvasShaders() {
	if shadeCanvas == nil {
		shadeCanvas = (*canvasPlayer).shade
	}
}

// shaded runs op through its program when it shades with one, reporting
// whether it did; a false report draws an image op plain.
func (p *canvasPlayer) shaded(op *driver.DrawOp) bool {
	return shadeCanvas != nil && canvasshade.Shades(op) && shadeCanvas(p, op)
}

var (
	canvasGLTried bool
	canvasGLCtx   uintptr // the GdkGLContext, 0 without one
	canvasGLES    bool
	canvasGLVAO   uint32
	canvasGLVBO   uint32
	canvasGLUBO   uint32
	canvasGLFBO   uint32
	canvasGLDraw  uint32 // the framebuffer's texture
	canvasGLW     int32  // its size
	canvasGLH     int32
	// canvasPrograms are the programs linked for passes, 0 when one failed.
	canvasPrograms = map[*driver.ShaderProgram]*canvasGLProgram{}
	// canvasTextures are the textures of the pictures passes read, by the
	// picture, and the pass that used each last.
	canvasTextures = map[*picture]*canvasGLTexture{}
	canvasPass     uint64

	gdkDisplayCreateGLContext func(display uintptr, err *uintptr) uintptr
	gdkGLContextSetAllowedAPI func(ctx uintptr, apis uint32)
	gdkGLContextRealize       func(ctx uintptr, err *uintptr) bool
	gdkGLContextMakeCurrent   func(ctx uintptr)
	gdkGLContextGetCurrent    func() uintptr
	gdkGLContextClearCurrent  func()
)

// More of GL's enumerations.
const (
	glFramebuffer         = 0x8D40
	glColorAttachment0    = 0x8CE0
	glFramebufferComplete = 0x8CD5
	glArrayBuffer         = 0x8892
	glStreamDraw          = 0x88E0
	glFloat               = 0x1406
	glBlend               = 0x0BE2
	glOne                 = 1
	glOneMinusSrcAlpha    = 0x0303
	glPackAlignment       = 0x0D05
	glScissorTest         = 0x0C11
	glDepthTest           = 0x0B71
)

type canvasGLProgram struct {
	prog    uint32
	offsets []int
	types   []shaderir.BasicType
	size    int
	count   int
}

type canvasGLTexture struct {
	id   uint32
	used uint64
}

// canvasGL makes the display's context current once it exists, making it
// the first time, and returns a function making the one current before
// current again; nil where there is none.
func canvasGL() func() {
	if !canvasGLTried {
		canvasGLTried = true
		canvasGLCtx = newCanvasGL()
	}
	if canvasGLCtx == 0 {
		return nil
	}
	prev := gdkGLContextGetCurrent()
	gdkGLContextMakeCurrent(canvasGLCtx)
	return func() {
		if prev != 0 {
			gdkGLContextMakeCurrent(prev)
		} else {
			gdkGLContextClearCurrent()
		}
	}
}

func newCanvasGL() uintptr {
	if !loadGL() || !loadGPU() || gdkDisplayGetDefault == nil {
		return 0
	}
	display := gdkDisplayGetDefault()
	if display == 0 {
		return 0
	}
	if _, err := purego.Dlsym(gtkLib, "gdk_display_create_gl_context"); err != nil {
		return 0 // GTK before 4.6
	}
	reg := purego.RegisterLibFunc
	reg(&gdkDisplayCreateGLContext, gtkLib, "gdk_display_create_gl_context")
	reg(&gdkGLContextSetAllowedAPI, gtkLib, "gdk_gl_context_set_allowed_apis")
	reg(&gdkGLContextRealize, gtkLib, "gdk_gl_context_realize")
	reg(&gdkGLContextMakeCurrent, gtkLib, "gdk_gl_context_make_current")
	reg(&gdkGLContextGetCurrent, gtkLib, "gdk_gl_context_get_current")
	reg(&gdkGLContextClearCurrent, gtkLib, "gdk_gl_context_clear_current")
	var gerr uintptr
	ctx := gdkDisplayCreateGLContext(display, &gerr)
	if ctx == 0 {
		log.Printf("nsgo: canvas shader: no GL context: %s", gErrorMessage(gerr))
		return 0
	}
	const both = 1 | 2 // GDK_GL_API_GL | GDK_GL_API_GLES
	gdkGLContextSetAllowedAPI(ctx, both)
	if !gdkGLContextRealize(ctx, &gerr) {
		log.Printf("nsgo: canvas shader: GL context: %s", gErrorMessage(gerr))
		gObjectUnref(ctx)
		return 0
	}
	prev := gdkGLContextGetCurrent()
	gdkGLContextMakeCurrent(ctx)
	canvasGLES = gdkGLContextGetUseES(ctx) != 0
	glGenVertexArrays(1, &canvasGLVAO)
	glBindVertexArray(canvasGLVAO)
	glGenBuffers(1, &canvasGLVBO)
	glBindBuffer(glArrayBuffer, canvasGLVBO)
	const stride = canvasshade.Floats * 4
	for i, n := range [4]int32{2, 2, 2, 4} {
		glEnableVertexAttribArray(uint32(i))
		glVertexAttribPointer(uint32(i), n, glFloat, 0, stride, uintptr([4]int{0, 8, 16, 24}[i]))
	}
	glGenBuffers(1, &canvasGLUBO)
	glGenFramebuffers(1, &canvasGLFBO)
	if prev != 0 {
		gdkGLContextMakeCurrent(prev)
	} else {
		gdkGLContextClearCurrent()
	}
	return ctx
}

// gErrorMessage returns a GError's message, or "".
func gErrorMessage(err uintptr) string {
	if err == 0 {
		return ""
	}
	// GError is a domain (uint32), a code (int32) and a message.
	return goString(*(*uintptr)(unsafe.Add(cPointer(err), 8)))
}

// glCanvasVertexSource passes a pass's vertices on, as canvasshade.Pack
// writes them.
const glCanvasVertexSource = `
layout(location = 0) in vec2 nsgoPosition;
layout(location = 1) in vec2 nsgoDstIn;
layout(location = 2) in vec2 nsgoPosIn;
layout(location = 3) in vec4 nsgoColorIn;
out vec2 nsgoDst;
out vec2 nsgoPos;
out vec4 nsgoTint;

void main() {
	nsgoDst = nsgoDstIn;
	nsgoPos = nsgoPosIn;
	nsgoTint = nsgoColorIn;
	gl_Position = vec4(nsgoPosition, 0.0, 1.0);
}
`

// glCanvasMainSource runs the program's function with what the vertices
// carry, multiplied by their color.
const glCanvasMainSource = `
in vec2 nsgoDst;
in vec2 nsgoPos;
in vec4 nsgoTint;
out vec4 nsgoColor;

void main() {
	nsgoColor = nsgoShader(vec4(nsgoDst, 0.0, 1.0), nsgoPos) * nsgoTint;
}
`

// canvasProgram returns p linked in the canvas context, made current, or
// nil when it does not link.
func canvasProgram(p *driver.ShaderProgram) *canvasGLProgram {
	if c, ok := canvasPrograms[p]; ok {
		return c
	}
	canvasPrograms[p] = nil
	head := "#version 330 core\n"
	if canvasGLES {
		head = "#version 300 es\nprecision highp float;\nprecision highp int;\n"
	}
	ir := p.IR
	// The vertices put the pass's first row at the framebuffer's bottom,
	// so window y and the pass's y count the same way.
	frag := head + "\n" + glsl.Fragment(ir, "nsgoShader", glsl.Options{Block: "NsgoUniforms"}) + glCanvasMainSource
	compile := func(kind uint32, src string) uint32 {
		sh := glCreateShader(kind)
		cs := cstr(src)
		glShaderSource(sh, 1, &cs, nil)
		glCompileShader(sh)
		var ok int32
		glGetShaderiv(sh, glCompileStatus, &ok)
		if ok == 0 {
			buf := make([]byte, 4096)
			var n int32
			glGetShaderInfoLog(sh, int32(len(buf)), &n, &buf[0])
			log.Printf("nsgo: canvas shader: GL: %s", buf[:max(n, 0)])
			glDeleteShader(sh)
			return 0
		}
		return sh
	}
	vs, fs := compile(glVertexShader, head+glCanvasVertexSource), compile(glFragmentShader, frag)
	if vs == 0 || fs == 0 {
		return nil
	}
	prog := glCreateProgram()
	glAttachShader(prog, vs)
	glAttachShader(prog, fs)
	glLinkProgram(prog)
	glDeleteShader(vs)
	glDeleteShader(fs)
	var ok int32
	glGetProgramiv(prog, glLinkStatus, &ok)
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		glGetProgramInfoLog(prog, int32(len(buf)), &n, &buf[0])
		log.Printf("nsgo: canvas shader: GL: %s", buf[:max(n, 0)])
		glDeleteProgram(prog)
		return nil
	}
	c := &canvasGLProgram{prog: prog, types: ir.LaneTypes(), count: ir.TextureCount}
	c.offsets, c.size = glsl.UniformLanes(ir)
	glUseProgram(prog)
	for i := range c.count {
		glUniform1i(glGetUniformLocation(prog, cstr("T"+strconv.Itoa(i))), int32(i))
	}
	if idx := glGetUniformBlockIdx(prog, cstr("NsgoUniforms")); idx != glInvalidIndex {
		glUniformBlockBind(prog, idx, 0)
	}
	canvasPrograms[p] = c
	return c
}

// shadeSource is an image a pass reads: its picture, or the pixels of an
// offscreen drawing, and its size in points.
type shadeSource struct {
	pic    *picture
	pix    []byte // BGRA, premultiplied, for an offscreen drawing
	w, h   int
	points driver.Size
}

// shadeSource returns what src shows, or nil while it loads.
func (p *canvasPlayer) shadeSource(src driver.ImageSource) *shadeSource {
	if src.Offscreen != nil {
		e := p.w.offscreenSurface(p.v, src.Offscreen)
		if e == nil || e.surface == 0 {
			return nil
		}
		cairoSurfaceFlush(e.surface)
		data, stride := cairoImageSurfaceGetData(e.surface), int(cairoImageSurfaceGetStride(e.surface))
		if data == 0 || stride < e.width*4 {
			return nil
		}
		rows := unsafe.Slice((*byte)(cPointer(data)), stride*e.height)
		pix := make([]byte, e.width*e.height*4)
		for y := range e.height {
			copy(pix[y*e.width*4:(y+1)*e.width*4], rows[y*stride:])
		}
		return &shadeSource{pix: pix, w: e.width, h: e.height, points: offscreenPoints(src, e)}
	}
	s := p.w.canvasImage(src, p.v)
	if s == nil || s.pic == nil {
		p.waited = p.waited || (s != nil && s.load != nil)
		return nil
	}
	return &shadeSource{pic: s.pic, w: s.pic.width, h: s.pic.height, points: natural(s)}
}

// texture binds s's texture to the active unit, uploading it unless the
// picture's is kept; a texture it made for this pass alone goes with
// done.
func (s *shadeSource) texture() (done func()) {
	if s.pic != nil {
		if t := canvasTextures[s.pic]; t != nil {
			t.used = canvasPass
			glBindTexture(glTexture2D, t.id)
			return func() {}
		}
	}
	pix := s.pix
	if s.pic != nil {
		pix = s.pic.pix
	}
	// Cairo's premultiplied BGRA, as GL's RGBA wants it.
	rgba := make([]byte, s.w*s.h*4)
	for i := 0; i+3 < len(rgba) && i+3 < len(pix); i += 4 {
		rgba[i], rgba[i+1], rgba[i+2], rgba[i+3] = pix[i+2], pix[i+1], pix[i], pix[i+3]
	}
	var id uint32
	glGenTextures(1, &id)
	glBindTexture(glTexture2D, id)
	glTexImage2D(glTexture2D, 0, glRGBA, int32(s.w), int32(s.h), 0, glRGBA, glUnsignedByte, unsafe.Pointer(&rgba[0]))
	glTexParameteri(glTexture2D, glMinFilter, glNearest)
	glTexParameteri(glTexture2D, glMagFilter, glNearest)
	glTexParameteri(glTexture2D, glWrapS, glClampToEdge)
	glTexParameteri(glTexture2D, glWrapT, glClampToEdge)
	if s.pic == nil {
		return func() { glDeleteTextures(1, &id) }
	}
	canvasTextures[s.pic] = &canvasGLTexture{id: id, used: canvasPass}
	return func() {}
}

// trimCanvasTextures lets go of the textures no pass read for a while,
// as their pictures may be gone.
func trimCanvasTextures() {
	for pic, t := range canvasTextures {
		if canvasPass-t.used > 120 || len(canvasTextures) > 64 && t.used != canvasPass {
			glDeleteTextures(1, &t.id)
			delete(canvasTextures, pic)
		}
	}
}

var (
	cairoSurfaceGetDeviceScale  func(surface uintptr, x, y *float64)
	cairoSurfaceGetDeviceOffset func(surface uintptr, x, y *float64)
	cairoGetTarget              func(cr uintptr) uintptr
	cairoClipExtents            func(cr uintptr, x0, y0, x1, y1 *float64)
)

func (p *canvasPlayer) shade(op *driver.DrawOp) bool {
	if cairoSurfaceGetDeviceScale == nil {
		purego.RegisterLibFunc(&cairoSurfaceGetDeviceScale, cairoLib, "cairo_surface_get_device_scale")
		purego.RegisterLibFunc(&cairoSurfaceGetDeviceOffset, cairoLib, "cairo_surface_get_device_offset")
		purego.RegisterLibFunc(&cairoGetTarget, cairoLib, "cairo_get_target")
		purego.RegisterLibFunc(&cairoClipExtents, cairoLib, "cairo_clip_extents")
	}
	restore := canvasGL()
	if restore == nil {
		return false
	}
	defer restore()
	sh := driver.PaintShader(op.Shader)
	prog := canvasProgram(sh.Program)
	if prog == nil {
		return false
	}
	canvasPass++
	var first *shadeSource
	var size canvasshade.Image
	if op.Kind != driver.DrawShader {
		if first = p.shadeSource(*op.Image); first == nil {
			return true
		}
		size = canvasshade.Image{W: float64(first.points.Width), H: float64(first.points.Height),
			PixelsW: float64(first.w), PixelsH: float64(first.h)}
	}
	// User space to the target's pixels: the matrix, then the surface's
	// device scale and offset.
	cr := p.cr
	var ctm cairoMatrix
	cairoGetMatrix(cr, &ctm)
	target := cairoGetTarget(cr)
	sx, sy, ox, oy := 1.0, 1.0, 0.0, 0.0
	cairoSurfaceGetDeviceScale(target, &sx, &sy)
	cairoSurfaceGetDeviceOffset(target, &ox, &oy)
	device := cairoMatrix{xx: sx, yy: sy, x0: ox, y0: oy}
	full := device.then(ctm)
	m := canvasshade.Affine{A: full.xx, B: full.yx, C: full.xy, D: full.yy, E: full.x0, F: full.y0}
	var x0, y0, x1, y1 float64
	cairoClipExtents(cr, &x0, &y0, &x1, &y1)
	cx0, cy0 := math.Inf(1), math.Inf(1)
	cx1, cy1 := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		x, y := m.Apply(c[0], c[1])
		cx0, cy0, cx1, cy1 = min(cx0, x), min(cy0, y), max(cx1, x), max(cy1, y)
	}
	clip := image.Rect(int(math.Floor(cx0)), int(math.Floor(cy0)), int(math.Ceil(cx1)), int(math.Ceil(cy1)))
	pass, ok := canvasshade.Plan(op, m, clip, size)
	if !ok {
		return true
	}
	w, h := int32(pass.Bounds.Dx()), int32(pass.Bounds.Dy())
	sizes := make([][2]float64, prog.count)
	for i := range prog.count {
		glActiveTexture(glTexture0 + uint32(i))
		src := first
		if i > 0 || first == nil {
			src = nil
			if i < len(pass.Images) {
				src = p.shadeSource(pass.Images[i])
			}
		}
		if src == nil {
			glBindTexture(glTexture2D, 0)
			continue
		}
		defer src.texture()()
		sizes[i] = [2]float64{float64(src.w), float64(src.h)}
	}
	lanes := pass.Lanes(prog.count, func(i int) (float64, float64) { return sizes[i][0], sizes[i][1] })
	buf := make([]byte, max(prog.size, 16))
	shaderir.PutLanes(buf, prog.offsets, prog.types, lanes, 4)
	vertices := pass.Pack(nil, true)

	glBindFramebuffer(glFramebuffer, canvasGLFBO)
	if canvasGLDraw == 0 || canvasGLW < w || canvasGLH < h {
		if canvasGLDraw != 0 {
			glDeleteTextures(1, &canvasGLDraw)
		}
		canvasGLW, canvasGLH = max(canvasGLW, w), max(canvasGLH, h)
		glActiveTexture(glTexture0 + uint32(prog.count))
		glGenTextures(1, &canvasGLDraw)
		glBindTexture(glTexture2D, canvasGLDraw)
		glTexImage2D(glTexture2D, 0, glRGBA, canvasGLW, canvasGLH, 0, glRGBA, glUnsignedByte, nil)
		glTexParameteri(glTexture2D, glMinFilter, glNearest)
		glTexParameteri(glTexture2D, glMagFilter, glNearest)
		glFramebufferTexture2D(glFramebuffer, glColorAttachment0, glTexture2D, canvasGLDraw, 0)
	}
	if glCheckFramebufferStatus(glFramebuffer) != glFramebufferComplete {
		return false
	}
	glViewport(0, 0, w, h)
	glDisable(glScissorTest)
	glDisable(glDepthTest)
	glClearColor(0, 0, 0, 0)
	glClear(glColorBuffer)
	glEnable(glBlend)
	glBlendFuncSeparate(glOne, glOneMinusSrcAlpha, glOne, glOneMinusSrcAlpha)
	glUseProgram(prog.prog)
	glBindBuffer(glUniformBuffer, canvasGLUBO)
	glBufferData(glUniformBuffer, len(buf), unsafe.Pointer(&buf[0]), glDynamicDraw)
	glBindBufferBase(glUniformBuffer, 0, canvasGLUBO)
	glBindVertexArray(canvasGLVAO)
	glBindBuffer(glArrayBuffer, canvasGLVBO)
	glBufferData(glArrayBuffer, len(vertices)*4, unsafe.Pointer(&vertices[0]), glStreamDraw)
	glDrawArrays(glTriangles, 0, int32(len(pass.Vertices)))
	glPixelStorei(glPackAlignment, 4)
	rgba := make([]byte, int(w)*int(h)*4)
	glReadPixels(0, 0, w, h, glRGBA, glUnsignedByte, unsafe.Pointer(&rgba[0]))
	glBindFramebuffer(glFramebuffer, 0)
	trimCanvasTextures()

	// The framebuffer's first row is the pass's top; Cairo's surface is
	// BGRA and its own memory, which a recording context may keep.
	const formatARGB32 = 0
	surf := cairoImageSurfaceCreate(formatARGB32, w, h)
	defer cairoSurfaceDestroy(surf)
	cairoSurfaceFlush(surf)
	data, stride := cairoImageSurfaceGetData(surf), int(cairoImageSurfaceGetStride(surf))
	if data == 0 {
		return true
	}
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*int(h))
	for y := range int(h) {
		from, to := rgba[y*int(w)*4:(y+1)*int(w)*4], rows[y*stride:]
		for i := 0; i+3 < len(from); i += 4 {
			to[i], to[i+1], to[i+2], to[i+3] = from[i+2], from[i+1], from[i], from[i+3]
		}
	}
	cairoSurfaceMarkDirty(surf)
	// Paint in the target's pixels: the inverse of its device transform.
	px := cairoMatrix{xx: 1 / sx, yy: 1 / sy, x0: -ox / sx, y0: -oy / sy}
	bx, by := float64(pass.Bounds.Min.X), float64(pass.Bounds.Min.Y)
	saved := cairoCopyPath(cr)
	cairoSave(cr)
	cairoSetMatrix(cr, &px)
	cairoNewPath(cr)
	cairoRectangle(cr, bx, by, float64(w), float64(h))
	p.shadowed(func(cr uintptr) {
		cairoSave(cr)
		cairoClip(cr)
		cairoSetSourceSurface(cr, surf, bx, by)
		const cairoFilterNearest = 3
		cairoPatternSetFilter(cairoGetSource(cr), cairoFilterNearest)
		cairoPaintWithAlpha(cr, p.st.alpha)
		cairoRestore(cr)
	})
	cairoRestore(cr)
	cairoNewPath(cr)
	cairoAppendPath(cr, saved)
	cairoPathDestroy(saved)
	return true
}
