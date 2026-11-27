//go:build js && wasm

package web

import (
	goimage "image"
	"log"
	"strconv"
	"syscall/js"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvas2d"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasshade"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"
)

// A canvas op that shades with a program (canvasshade) renders its pass
// with WebGL 2 into a <canvas> every canvas view shares, one context for
// the page, and draws that canvas into the view's 2D context at the
// pass's bounds in its pixels, under the clip, the alpha, the operator
// and the shadow the context keeps. The browser copies between the two on
// the GPU.

// shadeCanvas runs a shaded op, set by EnableCanvasShaders: nil leaves
// DrawShader out and draws shaded images plain.
var shadeCanvas func(s *surface, v *view, p *canvas2d.Player, op *driver.DrawOp) bool

// EnableCanvasShaders implements driver.CanvasShaderEnabler.
func (*surface) EnableCanvasShaders() {
	if shadeCanvas == nil {
		shadeCanvas = (*surface).shadeCanvas
	}
}

// canvasGL is the WebGL 2 context canvas passes share.
type canvasGL struct {
	canvas js.Value
	gl     js.Value // null where the browser has no WebGL 2
	// programs are the programs linked so far, by the program; one that
	// failed is kept, its prog null.
	programs map[*driver.ShaderProgram]*canvasProgram
	vbo, ubo js.Value
	vbytes   js.Value // a Uint8Array the vertices go through
	// textures are the loaded images' textures, by ImageSource.Key, and
	// pixels those of images of pixels, by the core's copy, kept for the
	// pass that drew them last and the one after.
	textures boundedImageCache[glTexture]
	pixels   map[*goimage.RGBA]*shadePixels
	pass     uint64
}

type shadePixels struct {
	glTexture
	used uint64
}

type canvasProgram struct {
	prog     js.Value
	offsets  []int
	types    []shaderir.BasicType
	textures int
	buf      []byte
	bytes    js.Value
}

const canvasGLVertex = `#version 300 es
in vec2 aPos;
in vec2 aDst;
in vec2 aUV;
in vec4 aColor;
out vec2 vDst;
out vec2 vUV;
out vec4 vColor;

void main() {
	vDst = aDst;
	vUV = aUV;
	vColor = aColor;
	gl_Position = vec4(aPos, 0.0, 1.0);
}
`

// canvasGLMain is the fragment stage around the program's function: dst,
// pos and the color the vertices carry.
const canvasGLMain = `
in vec2 vDst;
in vec2 vUV;
in vec4 vColor;
out vec4 nsgoColor;

void main() {
	nsgoColor = nsgoShader(vec4(vDst, 0.0, 1.0), vUV) * vColor;
}
`

func (s *surface) canvasContext() *canvasGL {
	if s.canvasGL == nil {
		c := js.Global().Get("document").Call("createElement", "canvas")
		opts := map[string]any{"alpha": true, "premultipliedAlpha": true, "antialias": false, "depth": false, "stencil": false, "preserveDrawingBuffer": true}
		s.canvasGL = &canvasGL{canvas: c, gl: c.Call("getContext", "webgl2", opts),
			programs: map[*driver.ShaderProgram]*canvasProgram{}, pixels: map[*goimage.RGBA]*shadePixels{}}
		if g := s.canvasGL; !g.gl.IsNull() {
			gl := g.gl
			g.vbo, g.ubo = gl.Call("createBuffer"), gl.Call("createBuffer")
			g.textures.onEvict = func(t glTexture) { gl.Call("deleteTexture", t.tex) }
			g.textures.externalKept = s.canvasImages.isKept
		}
	}
	return s.canvasGL
}

func (s *surface) shadeCanvas(v *view, p *canvas2d.Player, op *driver.DrawOp) bool {
	if !canvasshade.Shades(p.Drawing, op) {
		return false
	}
	g := s.canvasContext()
	gl := g.gl
	if gl.IsNull() {
		return false
	}
	sh := driver.PaintShader(op.Shader)
	prog := g.programs[sh.Program]
	if prog == nil {
		prog = g.link(sh.Program)
		g.programs[sh.Program] = prog
	}
	if prog.prog.IsNull() {
		return false
	}
	g.pass++
	var size canvasshade.Image
	var first glTexture
	if op.Kind != driver.DrawShader {
		imgSrc := p.Drawing.Image(op)
		if imgSrc == nil {
			return false
		}
		t, density, ok := s.canvasTexture(g, v, *imgSrc)
		if !ok {
			return true
		}
		first = t
		size = canvasshade.Image{W: t.w / density, H: t.h / density, PixelsW: t.w, PixelsH: t.h}
	}
	m := p.Ctx.Call("getTransform")
	at := canvasshade.Affine{A: m.Get("a").Float(), B: m.Get("b").Float(), C: m.Get("c").Float(),
		D: m.Get("d").Float(), E: m.Get("e").Float(), F: m.Get("f").Float()}
	target := p.Ctx.Get("canvas")
	clip := goimage.Rect(0, 0, target.Get("width").Int(), target.Get("height").Int())
	pass, ok := canvasshade.Plan(p.Drawing, op, at, clip, size)
	if !ok {
		return true
	}
	textures := make([]glTexture, prog.textures)
	for i := range textures {
		switch {
		case i == 0 && op.Kind != driver.DrawShader:
			textures[i] = first
		case i < len(pass.Images):
			if t, _, ok := s.canvasTexture(g, v, pass.Images[i]); ok {
				textures[i] = t
			}
		}
	}
	w, h := pass.Bounds.Dx(), pass.Bounds.Dy()
	cw, ch := g.canvas.Get("width").Int(), g.canvas.Get("height").Int()
	if cw < w || ch < h {
		g.canvas.Set("width", max(cw, w))
		g.canvas.Set("height", max(ch, h))
		ch = max(ch, h)
	}
	lanes := pass.Lanes(prog.textures, func(i int) (float64, float64) { return textures[i].w, textures[i].h })
	clear(prog.buf)
	shaderir.PutLanes(prog.buf, prog.offsets, prog.types, lanes, 4)
	js.CopyBytesToJS(prog.bytes, prog.buf)
	verts := pass.Pack(nil, false)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(verts))), len(verts)*4)
	if g.vbytes.IsUndefined() || g.vbytes.Length() < len(raw) {
		g.vbytes = js.Global().Get("Uint8Array").New(max(len(raw), 4096))
	}
	js.CopyBytesToJS(g.vbytes, raw)

	gl.Call("viewport", 0, 0, w, h)
	gl.Call("disable", gl.Get("SCISSOR_TEST"))
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", gl.Get("COLOR_BUFFER_BIT"))
	gl.Call("useProgram", prog.prog)
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("ONE"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	gl.Call("bindBuffer", gl.Get("ARRAY_BUFFER"), g.vbo)
	gl.Call("bufferData", gl.Get("ARRAY_BUFFER"), g.vbytes, gl.Get("STREAM_DRAW"), 0, len(raw))
	const stride = canvasshade.Floats * 4
	for i, n := range [4]int{2, 2, 2, 4} {
		gl.Call("enableVertexAttribArray", i)
		gl.Call("vertexAttribPointer", i, n, gl.Get("FLOAT"), false, stride, 8*i)
	}
	gl.Call("bindBufferBase", gl.Get("UNIFORM_BUFFER"), 0, g.ubo)
	gl.Call("bufferData", gl.Get("UNIFORM_BUFFER"), prog.bytes, gl.Get("DYNAMIC_DRAW"))
	for i, t := range textures {
		gl.Call("activeTexture", gl.Get("TEXTURE0").Int()+i)
		if t.tex.IsUndefined() {
			gl.Call("bindTexture", gl.Get("TEXTURE_2D"), js.Null())
		} else {
			gl.Call("bindTexture", gl.Get("TEXTURE_2D"), t.tex)
		}
	}
	gl.Call("drawArrays", gl.Get("TRIANGLES"), 0, len(pass.Vertices))
	g.dropPixels()

	ctx := p.Ctx
	ctx.Call("save")
	ctx.Call("setTransform", 1, 0, 0, 1, 0, 0)
	ctx.Set("imageSmoothingEnabled", false)
	// The viewport is the canvas's bottom left corner.
	ctx.Call("drawImage", g.canvas, 0, ch-h, w, h, pass.Bounds.Min.X, pass.Bounds.Min.Y, w, h)
	ctx.Call("restore")
	return true
}

// canvasTexture returns src's texture in g and its pixels a point, false
// while it loads, which draws v again once it has.
func (s *surface) canvasTexture(g *canvasGL, v *view, src driver.ImageSource) (glTexture, float64, bool) {
	gl := g.gl
	if px := src.Pixels; px != nil {
		density := 1.0
		if src.Size.Width > 0 {
			density = float64(px.Rect.Dx()) / float64(src.Size.Width)
		}
		if t := g.pixels[px]; t != nil {
			t.used = g.pass
			return t.glTexture, density, true
		}
		w, h := px.Rect.Dx(), px.Rect.Dy()
		if w <= 0 || h <= 0 {
			return glTexture{}, 0, false
		}
		n := 4 * w * h
		bytes := js.Global().Get("Uint8Array").New(n)
		js.CopyBytesToJS(bytes, px.Pix[:n])
		t := glTexture{tex: gl.Call("createTexture"), w: float64(w), h: float64(h)}
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), t.tex)
		gl.Call("pixelStorei", gl.Get("UNPACK_PREMULTIPLY_ALPHA_WEBGL"), false)
		gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), w, h, 0, gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), bytes)
		nearest(gl)
		g.pixels[px] = &shadePixels{glTexture: t, used: g.pass}
		return t, density, true
	}
	im := s.loadImage(src, v)
	if !im.loaded {
		return glTexture{}, 0, false
	}
	key := src.Key()
	if t, ok := g.textures.get(key); ok && src.Offscreen == nil {
		return t, im.density, true
	}
	img := im.img
	nw, nh := img.Get("width").Float(), img.Get("height").Float()
	if n := img.Get("naturalWidth"); n.Type() == js.TypeNumber {
		nw, nh = n.Float(), img.Get("naturalHeight").Float()
	}
	if nw <= 0 || nh <= 0 {
		return glTexture{}, 0, false
	}
	t, ok := g.textures.get(key)
	if !ok {
		t = glTexture{tex: gl.Call("createTexture")}
	}
	t.w, t.h = nw, nh
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), t.tex)
	// Programs read premultiplied colors, the image's top row first.
	gl.Call("pixelStorei", gl.Get("UNPACK_PREMULTIPLY_ALPHA_WEBGL"), true)
	gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"), gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), img)
	nearest(gl)
	// An offscreen image's canvas changes in place, so its texture is
	// written again on each pass that reads it.
	g.textures.put(key, t, int64(nw)*int64(nh)*4)
	return t, im.density, true
}

func nearest(gl js.Value) {
	for _, p := range []string{"TEXTURE_MIN_FILTER", "TEXTURE_MAG_FILTER"} {
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get(p), gl.Get("NEAREST"))
	}
	for _, p := range []string{"TEXTURE_WRAP_S", "TEXTURE_WRAP_T"} {
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get(p), gl.Get("CLAMP_TO_EDGE"))
	}
}

// dropPixels lets the textures of pixels no pass read since the one
// before go: each version is a copy of its own.
func (g *canvasGL) dropPixels() {
	for px, t := range g.pixels {
		if t.used+1 < g.pass && len(g.pixels) > 4 {
			g.gl.Call("deleteTexture", t.tex)
			delete(g.pixels, px)
		}
	}
	g.textures.endPass()
}

// link translates p for canvas passes and links it; a program that fails
// has a null prog.
func (g *canvasGL) link(p *driver.ShaderProgram) *canvasProgram {
	gl := g.gl
	cp := &canvasProgram{prog: js.Null()}
	ir := p.IR
	src := "#version 300 es\nprecision highp float;\nprecision highp int;\n\n" +
		glsl.Fragment(ir, "nsgoShader", glsl.Options{FlipY: "true", Block: "NsgoUniforms"}) + canvasGLMain
	compile := func(kind, src string) js.Value {
		sh := gl.Call("createShader", gl.Get(kind))
		gl.Call("shaderSource", sh, src)
		gl.Call("compileShader", sh)
		if !gl.Call("getShaderParameter", sh, gl.Get("COMPILE_STATUS")).Truthy() {
			log.Printf("nsgo: canvas shader: WebGL: %s", gl.Call("getShaderInfoLog", sh).String())
			gl.Call("deleteShader", sh)
			return js.Null()
		}
		return sh
	}
	vs, fs := compile("VERTEX_SHADER", canvasGLVertex), compile("FRAGMENT_SHADER", src)
	if vs.IsNull() || fs.IsNull() {
		return cp
	}
	prog := gl.Call("createProgram")
	gl.Call("attachShader", prog, vs)
	gl.Call("attachShader", prog, fs)
	for i, name := range []string{"aPos", "aDst", "aUV", "aColor"} {
		gl.Call("bindAttribLocation", prog, i, name)
	}
	gl.Call("linkProgram", prog)
	gl.Call("deleteShader", vs)
	gl.Call("deleteShader", fs)
	if !gl.Call("getProgramParameter", prog, gl.Get("LINK_STATUS")).Truthy() {
		log.Printf("nsgo: canvas shader: WebGL: %s", gl.Call("getProgramInfoLog", prog).String())
		gl.Call("deleteProgram", prog)
		return cp
	}
	gl.Call("useProgram", prog)
	for i := range ir.TextureCount {
		gl.Call("uniform1i", gl.Call("getUniformLocation", prog, "T"+strconv.Itoa(i)), i)
	}
	gl.Call("uniformBlockBinding", prog, gl.Call("getUniformBlockIndex", prog, "NsgoUniforms"), 0)
	var size int
	cp.offsets, size = glsl.UniformLanes(ir)
	cp.types = ir.LaneTypes()
	cp.textures = ir.TextureCount
	cp.buf = make([]byte, size)
	cp.bytes = js.Global().Get("Uint8Array").New(size)
	cp.prog = prog
	return cp
}
