//go:build js && wasm

package web

import (
	"log"
	"math"
	"syscall/js"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/shader"
	"github.com/oneo-me/nsgo/internal/shaderir"
	"github.com/oneo-me/nsgo/internal/shaderir/glsl"
)

// A shader view draws with WebGL 2 into a <canvas> of its own, whose
// pixels follow the pixel ratio: when its uniforms, its size or the
// animated nodes its lanes follow change, in the next animation frame, so
// at most once a frame however many batches arrive. Each view has its own
// context, which compiles its program the first time it draws it; browsers
// keep a few dozen contexts at most.

// glShader is a shader view's WebGL state.
type glShader struct {
	gl      js.Value // WebGL2RenderingContext, null where the browser has none
	shader  *driver.Shader
	program *driver.ShaderProgram // what prog was linked from
	prog    js.Value
	ubo     js.Value
	height  js.Value // the uniform location of the drawable's height
	offsets []int
	types   []shaderir.BasicType
	lanes   []float64
	buf     []byte
	bytes   js.Value // a Uint8Array as large as buf
	failed  bool
}

const glVertex = `#version 300 es
void main() {
	vec2 uv = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
	gl_Position = vec4(uv * 2.0 - 1.0, 0.0, 1.0);
}
`

// glMain is the fragment stage around the program's function: the
// position counts down from the top, as the program's does.
const glMain = `
uniform float nsgoHeight;
out vec4 nsgoColor;

void main() {
	vec2 p = vec2(gl_FragCoord.x, nsgoHeight - gl_FragCoord.y);
	nsgoColor = nsgoShader(vec4(p, gl_FragCoord.z, 1.0), p / U1);
}
`

func (s *surface) createShader(v *view) {
	v.inner = s.element("canvas")
	setStyle(v.inner, map[string]string{"width": "100%", "height": "100%", "display": "block"})
	v.el.Call("appendChild", v.inner)
	opts := map[string]any{"alpha": true, "premultipliedAlpha": true, "antialias": false, "depth": false, "stencil": false}
	v.gl = &glShader{gl: v.inner.Call("getContext", "webgl2", opts)}
}

// setShader gives v what it draws and binds its animated lanes.
func (s *surface) setShader(v *view, sh *driver.Shader) {
	v.gl.shader = sh
	s.animator.BindShader(v.id, sh.Nodes)
	s.shadeLater(v)
}

// shadeLater draws v in the next animation frame.
func (s *surface) shadeLater(v *view) {
	s.queueShader(v)
	s.startTicking()
}

func (s *surface) queueShader(v *view) {
	for _, w := range s.shading {
		if w == v {
			return
		}
	}
	s.shading = append(s.shading, v)
}

// changedShaders queues the shader views whose animated lanes changed,
// for the frame to draw.
func (s *surface) changedShaders() {
	s.animator.ChangedShaders(func(id driver.NodeID) {
		if v := s.view(id); v != nil && v.gl != nil {
			s.queueShader(v)
		}
	})
}

// drawShaders draws the shader views waiting to, in an animation frame.
func (s *surface) drawShaders() {
	for _, v := range s.shading {
		if s.view(v.id) == v {
			s.drawShader(v)
		}
	}
	clear(s.shading)
	s.shading = s.shading[:0]
}

func (s *surface) drawShader(v *view) {
	g := v.gl
	gl := g.gl
	if gl.IsNull() || g.shader == nil || g.failed && g.program == g.shader.Program {
		return
	}
	if g.program != g.shader.Program && !g.link(g.shader.Program) {
		return
	}
	ratio := float64(s.env.PixelRatio)
	if ratio <= 0 {
		ratio = 1
	}
	w := math.Max(float64(v.size.Width-v.border.Left-v.border.Right), 0)
	h := math.Max(float64(v.size.Height-v.border.Top-v.border.Bottom), 0)
	pw, ph := math.Ceil(w*ratio), math.Ceil(h*ratio)
	if pw == 0 || ph == 0 {
		return
	}
	if v.inner.Get("width").Float() != pw || v.inner.Get("height").Float() != ph {
		v.inner.Set("width", pw)
		v.inner.Set("height", ph)
	}

	n := shader.BridgeLanes + len(g.shader.Lanes)
	g.lanes = append(g.lanes[:0], make([]float64, n)...)
	copy(g.lanes[shader.BridgeLanes:], g.shader.Lanes)
	s.animator.ShaderLanes(v.id, g.lanes[shader.BridgeLanes:])
	g.lanes[0], g.lanes[1], g.lanes[2] = w, h, ratio
	clear(g.buf)
	shaderir.PutLanes(g.buf, g.offsets, g.types, g.lanes, 4)
	js.CopyBytesToJS(g.bytes, g.buf)

	gl.Call("viewport", 0, 0, pw, ph)
	gl.Call("useProgram", g.prog)
	gl.Call("uniform1f", g.height, ph)
	gl.Call("bindBufferBase", gl.Get("UNIFORM_BUFFER"), 0, g.ubo)
	gl.Call("bufferSubData", gl.Get("UNIFORM_BUFFER"), 0, g.bytes)
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", gl.Get("COLOR_BUFFER_BIT"))
	gl.Call("drawArrays", gl.Get("TRIANGLES"), 0, 3)
}

// link translates p for WebGL and links it, reporting whether it can draw.
func (g *glShader) link(p *driver.ShaderProgram) bool {
	gl := g.gl
	g.program, g.failed = p, true
	if !g.prog.IsUndefined() {
		gl.Call("deleteProgram", g.prog)
		g.prog = js.Undefined()
	}
	ir, err := shader.CompileFragment([]byte(p.Source))
	if err != nil {
		log.Printf("nsgo: shader: %v", err)
		return false
	}
	src := "#version 300 es\nprecision highp float;\nprecision highp int;\n\n" +
		glsl.Fragment(ir, "nsgoShader", glsl.Options{FlipY: "true", Block: "NsgoUniforms"}) + glMain
	compile := func(kind, src string) js.Value {
		sh := gl.Call("createShader", gl.Get(kind))
		gl.Call("shaderSource", sh, src)
		gl.Call("compileShader", sh)
		if !gl.Call("getShaderParameter", sh, gl.Get("COMPILE_STATUS")).Truthy() {
			log.Printf("nsgo: shader: WebGL: %s", gl.Call("getShaderInfoLog", sh).String())
			gl.Call("deleteShader", sh)
			return js.Null()
		}
		return sh
	}
	vs, fs := compile("VERTEX_SHADER", glVertex), compile("FRAGMENT_SHADER", src)
	if vs.IsNull() || fs.IsNull() {
		return false
	}
	prog := gl.Call("createProgram")
	gl.Call("attachShader", prog, vs)
	gl.Call("attachShader", prog, fs)
	gl.Call("linkProgram", prog)
	gl.Call("deleteShader", vs)
	gl.Call("deleteShader", fs)
	if !gl.Call("getProgramParameter", prog, gl.Get("LINK_STATUS")).Truthy() {
		log.Printf("nsgo: shader: WebGL: %s", gl.Call("getProgramInfoLog", prog).String())
		gl.Call("deleteProgram", prog)
		return false
	}
	g.prog = prog
	g.height = gl.Call("getUniformLocation", prog, "nsgoHeight")
	var size int
	g.offsets, size = glsl.UniformLanes(ir)
	g.types = ir.LaneTypes()
	g.buf = make([]byte, size)
	g.bytes = js.Global().Get("Uint8Array").New(size)
	gl.Call("uniformBlockBinding", prog, gl.Call("getUniformBlockIndex", prog, "NsgoUniforms"), 0)
	if g.ubo.IsUndefined() {
		g.ubo = gl.Call("createBuffer")
	}
	gl.Call("bindBuffer", gl.Get("UNIFORM_BUFFER"), g.ubo)
	gl.Call("bufferData", gl.Get("UNIFORM_BUFFER"), size, gl.Get("DYNAMIC_DRAW"))
	g.failed = false
	return true
}

// release gives the view's context back to the browser, which keeps few.
func (g *glShader) release() {
	if g.gl.IsNull() {
		return
	}
	if lose := g.gl.Call("getExtension", "WEBGL_lose_context"); !lose.IsNull() {
		lose.Call("loseContext")
	}
}
