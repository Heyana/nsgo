//go:build js && wasm

package web

import (
	"log"
	"math"
	"slices"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// Liquid Glass (driver.ViewStyle.Glass) on the web uses Chrome's
// HTML-in-Canvas (<canvas content="drawable">, drawElementImage,
// requestPaint) together with a shared WebGL 2 glass shader matching
// Windows' glass.go (MyGo's glass / macOS 27's NSGlassEffectView).
//
// While at least one eligible glass view is mounted and the browser
// exposes HTML-in-Canvas + WebGL 2, s.root.el moves inside a stage
// <canvas content="drawable"> with attribute drawable, each eligible
// glass view and any following sibling above it in paint order becomes
// a flat (depth-1) drawable layer, and the stage's paint event:
//   1. Draws s.root.el (which excludes nested drawables) and composites
//      its visible descendant <canvas> elements (icons, charts, pixel
//      images, shaders), which Chrome's drawElementImage omits or draws
//      only in part.
//   2. For each glass view in paint order: snapshots the stage behind
//      it (with blur padding), runs the 2-pass separable Gaussian blur
//      and glass refraction/tone/tint/rim shader in the shared WebGL 2
//      context, draws the glass onto the stage, then draws the glass
//      view's own DOM borders/children and descendant <canvas> elements
//      on top.
//   3. Draws any overlay sibling layer above the glass view in DOM
//      order.
//
// Views under unsafe ancestors (blend mode, mask, translucent backdrop
// filter, or nested glass — where Chrome hangs on nested drawables) or
// non-upright transforms, and all glass views in browsers without
// HTML-in-Canvas (Firefox, Safari, older Chrome), cleanly fall back to
// CSS backdrop-filter blur + tint + inset rim without mounting the
// stage canvas.

const glassStageCSS = `.nsgo-glass-stage { position: absolute; inset: 0; width: 100%; height: 100%; display: block; }`

// glassSupportedFunc is overridden in tests to simulate browsers
// without HTML-in-Canvas support.
var glassSupportedFunc = htmlInCanvasSupported

// htmlInCanvasSupported reports whether the browser exposes
// HTML-in-Canvas (content="drawable", requestPaint, drawElementImage)
// and WebGL 2 for the glass shader.
func htmlInCanvasSupported() bool {
	g := js.Global()
	canvasCtor := g.Get("HTMLCanvasElement")
	ctx2dCtor := g.Get("CanvasRenderingContext2D")
	doc := g.Get("document")
	if !canvasCtor.Truthy() || !ctx2dCtor.Truthy() || !doc.Truthy() {
		return false
	}
	cp := canvasCtor.Get("prototype")
	p2d := ctx2dCtor.Get("prototype")
	if !cp.Truthy() || !p2d.Truthy() {
		return false
	}
	if !hasProp(cp, "content") || !hasProp(cp, "requestPaint") || !hasProp(cp, "getElementImageDefaultSize") {
		return false
	}
	if !hasProp(p2d, "drawElementImage") {
		return false
	}
	// Verify WebGL 2 is available without leaking a context.
	probe := doc.Call("createElement", "canvas")
	gl := probe.Call("getContext", "webgl2")
	if !gl.Truthy() {
		return false
	}
	if lose := gl.Call("getExtension", "WEBGL_lose_context"); lose.Truthy() {
		lose.Call("loseContext")
	}
	return true
}

func hasProp(obj js.Value, name string) bool {
	if !obj.Truthy() {
		return false
	}
	return js.Global().Get("Reflect").Call("has", obj, name).Bool()
}

// glassStage owns a surface's HTML-in-Canvas stage and shared WebGL 2
// glass renderer.
type glassStage struct {
	stage      js.Value // <canvas content="drawable" class="nsgo-glass-stage">
	ctx        js.Value // 2D context of stage
	cropCanvas js.Value // offscreen 2D canvas for backdrop snapshots
	cropCtx    js.Value
	gl         *glassGL
	paintFn    js.Func
	views      map[*view]bool // all mounted views with Glass != GlassNone
	layers     []glassLayer   // ordered drawables inside root: glass views + overlays
	drawables  []js.Value     // elements currently carrying the drawable attribute
	active     bool
	styled     bool
	painting   bool
	dirty      bool
	paintCount int
}

type glassLayer struct {
	el   js.Value
	view *view // non-nil for glass views, nil for overlay siblings
}

// glassGL is the surface's single shared WebGL 2 context for blurring
// backdrop snapshots and running the Liquid Glass shader.
type glassGL struct {
	canvas   js.Value
	gl       js.Value
	blurProg js.Value
	prog     js.Value
	fbo      js.Value
	srcTex   js.Value
	tempTex  js.Value
	blurTex  js.Value
	texW     int
	texH     int
	failed   bool

	// Blur uniform locations.
	uBlurSrc   js.Value
	uBlurDir   js.Value
	uBlurSigma js.Value

	// Glass uniform locations.
	uBackdrop js.Value
	uRect     js.Value
	uRadii    js.Value
	uPad      js.Value
	uP0       js.Value
	uP1       js.Value
	uP2       js.Value
	uP3       js.Value
}

const glassVertGLSL = `#version 300 es
out vec2 vUV;
void main() {
	vec2 uv = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
	vUV = uv;
	gl_Position = vec4(uv * 2.0 - 1.0, 0.0, 1.0);
}
`

const glassBlurFragGLSL = `#version 300 es
precision highp float;
uniform sampler2D uSrc;
uniform vec2 uDir;
uniform float uSigma;
in vec2 vUV;
out vec4 fragColor;

void main() {
	float sigma = max(uSigma, 0.001);
	float span = sigma * 2.5;
	int radius = min(24, max(1, int(ceil(span))));
	float stride = max(1.0, span / float(radius));
	float invTwoSigma2 = 0.5 / (sigma * sigma);
	vec4 sum = texture(uSrc, vUV);
	float weight = 1.0;
	for (int i = 1; i <= 24; i++) {
		if (i > radius) {
			break;
		}
		float off = float(i) * stride;
		float w = exp(-off * off * invTwoSigma2);
		vec2 d = uDir * off;
		sum += (texture(uSrc, clamp(vUV + d, 0.0, 1.0)) + texture(uSrc, clamp(vUV - d, 0.0, 1.0))) * w;
		weight += 2.0 * w;
	}
	fragColor = sum / weight;
}
`

// glassFragGLSL is the GLSL ES 3.00 port of Windows' glassHLSL
// (internal/platform/windows/glass.go).
const glassFragGLSL = `#version 300 es
precision highp float;
uniform sampler2D uBackdrop;
uniform vec4 uRect;  // x, y, width, height in pixels
uniform vec4 uRadii; // top-left, top-right, bottom-right, bottom-left in pixels
uniform vec4 uPad;   // padX, padY, backdropW, backdropH in pixels
uniform vec4 uP0;    // bezel, refraction, rim, rim width
uniform vec4 uP1;    // tint, straight
uniform vec4 uP2;    // low, high, curve, saturation
uniform vec4 uP3;    // light direction
out vec4 fragColor;

float sdRoundRect(vec2 p, vec4 rect, vec4 radii) {
	vec2 h = rect.zw * 0.5;
	vec2 q = p - rect.xy - h;
	float r = q.x < 0.0 ? (q.y < 0.0 ? radii.x : radii.w) : (q.y < 0.0 ? radii.y : radii.z);
	vec2 a = abs(q) - h + r;
	return length(max(a, 0.0)) + min(max(a.x, a.y), 0.0) - r;
}

vec3 sampleBackdrop(vec2 q) {
	vec2 sz = max(uPad.zw, vec2(1.0));
	vec2 halfTexel = vec2(0.5) / sz;
	vec2 uv = clamp((q + uPad.xy) / sz, halfTexel, vec2(1.0) - halfTexel);
	return texture(uBackdrop, uv).rgb;
}

float glassLens(float t, float bezel) {
	if (bezel <= 0.0 || t >= bezel) {
		return 0.0;
	}
	float u = 1.0 - max(t, 0.0) / bezel;
	float v = max(1.0 - u * u * u * u, 0.0);
	float a = sqrt(v * sqrt(v));
	float b = u * u * u;
	float n = sqrt(a * a + b * b);
	if (n <= 0.0) {
		return 0.0;
	}
	float s = b / n;
	float c = a / n;
	float st = s / 1.5;
	float ct = sqrt(max(1.0 - st * st, 0.0));
	return (s * ct - c * st) / (c * ct + s * st) / 1.1180340;
}

vec2 glassNormal(vec2 p) {
	vec2 h = uRect.zw * 0.5;
	vec2 q = p - uRect.xy - h;
	float r = q.x < 0.0 ? (q.y < 0.0 ? uRadii.x : uRadii.w) : (q.y < 0.0 ? uRadii.y : uRadii.z);
	r = min(r * 1.5, min(h.x, h.y));
	vec2 a = abs(q) - (h - r);
	vec2 sq = sign(q);
	if (a.x > 0.0 || a.y > 0.0) {
		vec2 m = max(a, 0.0);
		float len = length(m);
		if (len > 0.0) {
			return sq * m / len;
		}
		return vec2(0.0, 1.0);
	}
	return a.x > a.y ? vec2(sq.x, 0.0) : vec2(0.0, sq.y);
}

vec3 glassTone(vec3 c) {
	float l = 0.2126 * c.r + 0.7152 * c.g + 0.0722 * c.b;
	float t = uP2.x + (uP2.y - uP2.x) * (1.0 - pow(max(1.0 - l, 0.0), uP2.z));
	float k = (uP2.y - uP2.x) * uP2.w;
	return clamp(vec3(t) + (c - vec3(l)) * k, 0.0, 1.0);
}

void main() {
	vec2 p = vec2(gl_FragCoord.x, uRect.w - gl_FragCoord.y);
	float d = sdRoundRect(p, uRect, uRadii);
	float alpha = clamp(0.5 - d, 0.0, 1.0);
	if (alpha <= 0.0) {
		fragColor = vec4(0.0);
		return;
	}
	float t = max(-d, 0.0);
	float bezel = uP0.x;
	float rimWidth = uP0.w;
	vec2 n = vec2(0.0);
	if (t < bezel || t < 3.0 * rimWidth) {
		n = glassNormal(p);
	}
	vec2 q = p;
	if (t < bezel) {
		q -= n * (uP0.y * glassLens(t, bezel));
	}
	vec3 c = glassTone(sampleBackdrop(q));
	c += (uP1.rgb - c) * uP1.a;
	if (uP0.z > 0.0 && t < 3.0 * rimWidth) {
		float r = t / rimWidth;
		float a = uP0.z * exp(-r * r);
		float l = abs(n.x * uP3.x + n.y * uP3.y);
		c += (vec3(l) - c) * a;
	}
	fragColor = vec4(c * alpha, alpha);
}
`

func newGlassGL(doc js.Value) *glassGL {
	c := doc.Call("createElement", "canvas")
	opts := map[string]any{"alpha": true, "premultipliedAlpha": true, "antialias": false, "depth": false, "stencil": false}
	gl := c.Call("getContext", "webgl2", opts)
	g := &glassGL{canvas: c, gl: gl}
	if !gl.Truthy() {
		g.failed = true
		return g
	}
	compile := func(kind, src string) js.Value {
		sh := gl.Call("createShader", gl.Get(kind))
		gl.Call("shaderSource", sh, src)
		gl.Call("compileShader", sh)
		if !gl.Call("getShaderParameter", sh, gl.Get("COMPILE_STATUS")).Truthy() {
			log.Printf("nsgo: glass: WebGL: %s", gl.Call("getShaderInfoLog", sh).String())
			gl.Call("deleteShader", sh)
			return js.Null()
		}
		return sh
	}
	link := func(vs, fs js.Value) js.Value {
		if vs.IsNull() || fs.IsNull() {
			return js.Null()
		}
		p := gl.Call("createProgram")
		gl.Call("attachShader", p, vs)
		gl.Call("attachShader", p, fs)
		gl.Call("linkProgram", p)
		if !gl.Call("getProgramParameter", p, gl.Get("LINK_STATUS")).Truthy() {
			log.Printf("nsgo: glass: WebGL: %s", gl.Call("getProgramInfoLog", p).String())
			gl.Call("deleteProgram", p)
			return js.Null()
		}
		return p
	}
	vs := compile("VERTEX_SHADER", glassVertGLSL)
	blurFS := compile("FRAGMENT_SHADER", glassBlurFragGLSL)
	glassFS := compile("FRAGMENT_SHADER", glassFragGLSL)
	g.blurProg = link(vs, blurFS)
	g.prog = link(vs, glassFS)
	if !vs.IsNull() {
		gl.Call("deleteShader", vs)
	}
	if !blurFS.IsNull() {
		gl.Call("deleteShader", blurFS)
	}
	if !glassFS.IsNull() {
		gl.Call("deleteShader", glassFS)
	}
	if g.blurProg.IsNull() || g.prog.IsNull() {
		g.failed = true
		g.release()
		return g
	}
	g.uBlurSrc = gl.Call("getUniformLocation", g.blurProg, "uSrc")
	g.uBlurDir = gl.Call("getUniformLocation", g.blurProg, "uDir")
	g.uBlurSigma = gl.Call("getUniformLocation", g.blurProg, "uSigma")
	g.uBackdrop = gl.Call("getUniformLocation", g.prog, "uBackdrop")
	g.uRect = gl.Call("getUniformLocation", g.prog, "uRect")
	g.uRadii = gl.Call("getUniformLocation", g.prog, "uRadii")
	g.uPad = gl.Call("getUniformLocation", g.prog, "uPad")
	g.uP0 = gl.Call("getUniformLocation", g.prog, "uP0")
	g.uP1 = gl.Call("getUniformLocation", g.prog, "uP1")
	g.uP2 = gl.Call("getUniformLocation", g.prog, "uP2")
	g.uP3 = gl.Call("getUniformLocation", g.prog, "uP3")

	g.fbo = gl.Call("createFramebuffer")
	makeTex := func() js.Value {
		tex := gl.Call("createTexture")
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), tex)
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
		return tex
	}
	g.srcTex = makeTex()
	g.tempTex = makeTex()
	g.blurTex = makeTex()
	return g
}

func (g *glassGL) ensureTextures(w, h int) {
	if g.texW == w && g.texH == h {
		return
	}
	gl := g.gl
	target, rgba, ub := gl.Get("TEXTURE_2D"), gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE")
	for _, tex := range []js.Value{g.srcTex, g.tempTex, g.blurTex} {
		gl.Call("bindTexture", target, tex)
		gl.Call("texImage2D", target, 0, rgba, w, h, 0, rgba, ub, js.Null())
	}
	g.texW, g.texH = w, h
}

// render blurs src (bw by bh pixels) by sigma pixels and draws the
// Liquid Glass shader for u into g.canvas.
func (g *glassGL) render(src js.Value, bw, bh int, padX, padY float64, u webstyle.GlassUniforms, sigma float64) bool {
	if g == nil || g.failed || !g.gl.Truthy() || bw <= 0 || bh <= 0 {
		return false
	}
	pw, ph := int(u.Rect[2]), int(u.Rect[3])
	if pw <= 0 || ph <= 0 {
		return false
	}
	gl := g.gl
	if g.canvas.Get("width").Int() != pw || g.canvas.Get("height").Int() != ph {
		g.canvas.Set("width", pw)
		g.canvas.Set("height", ph)
	}
	g.ensureTextures(bw, bh)

	target := gl.Get("TEXTURE_2D")
	rgba := gl.Get("RGBA")
	ub := gl.Get("UNSIGNED_BYTE")
	fbTarget := gl.Get("FRAMEBUFFER")
	colorAttach := gl.Get("COLOR_ATTACHMENT0")
	triangles := gl.Get("TRIANGLES")

	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	gl.Call("bindTexture", target, g.srcTex)
	gl.Call("pixelStorei", gl.Get("UNPACK_FLIP_Y_WEBGL"), false)
	gl.Call("pixelStorei", gl.Get("UNPACK_PREMULTIPLY_ALPHA_WEBGL"), false)
	gl.Call("texSubImage2D", target, 0, 0, 0, rgba, ub, src)

	// Pass 1: horizontal Gaussian blur (srcTex -> tempTex).
	gl.Call("bindFramebuffer", fbTarget, g.fbo)
	gl.Call("framebufferTexture2D", fbTarget, colorAttach, target, g.tempTex, 0)
	gl.Call("viewport", 0, 0, bw, bh)
	gl.Call("useProgram", g.blurProg)
	gl.Call("uniform1i", g.uBlurSrc, 0)
	gl.Call("uniform2f", g.uBlurDir, 1.0/float64(bw), 0.0)
	gl.Call("uniform1f", g.uBlurSigma, sigma)
	gl.Call("drawArrays", triangles, 0, 3)

	// Pass 2: vertical Gaussian blur (tempTex -> blurTex).
	gl.Call("framebufferTexture2D", fbTarget, colorAttach, target, g.blurTex, 0)
	gl.Call("bindTexture", target, g.tempTex)
	gl.Call("uniform2f", g.uBlurDir, 0.0, 1.0/float64(bh))
	gl.Call("drawArrays", triangles, 0, 3)

	// Pass 3: Liquid Glass shader (blurTex -> default drawing buffer).
	gl.Call("bindFramebuffer", fbTarget, js.Null())
	gl.Call("viewport", 0, 0, pw, ph)
	gl.Call("useProgram", g.prog)
	gl.Call("bindTexture", target, g.blurTex)
	gl.Call("uniform1i", g.uBackdrop, 0)
	gl.Call("uniform4f", g.uRect, u.Rect[0], u.Rect[1], u.Rect[2], u.Rect[3])
	gl.Call("uniform4f", g.uRadii, u.Radii[0], u.Radii[1], u.Radii[2], u.Radii[3])
	gl.Call("uniform4f", g.uPad, padX, padY, float64(bw), float64(bh))
	gl.Call("uniform4f", g.uP0, u.P0[0], u.P0[1], u.P0[2], u.P0[3])
	gl.Call("uniform4f", g.uP1, u.P1[0], u.P1[1], u.P1[2], u.P1[3])
	gl.Call("uniform4f", g.uP2, u.P2[0], u.P2[1], u.P2[2], u.P2[3])
	gl.Call("uniform4f", g.uP3, u.P3[0], u.P3[1], u.P3[2], u.P3[3])
	gl.Call("clearColor", 0, 0, 0, 0)
	gl.Call("clear", gl.Get("COLOR_BUFFER_BIT"))
	gl.Call("drawArrays", triangles, 0, 3)
	return true
}

func (g *glassGL) release() {
	if g == nil || !g.gl.Truthy() {
		return
	}
	gl := g.gl
	if g.blurProg.Truthy() {
		gl.Call("deleteProgram", g.blurProg)
	}
	if g.prog.Truthy() {
		gl.Call("deleteProgram", g.prog)
	}
	if g.fbo.Truthy() {
		gl.Call("deleteFramebuffer", g.fbo)
	}
	for _, tex := range []js.Value{g.srcTex, g.tempTex, g.blurTex} {
		if tex.Truthy() {
			gl.Call("deleteTexture", tex)
		}
	}
	if lose := gl.Call("getExtension", "WEBGL_lose_context"); lose.Truthy() {
		lose.Call("loseContext")
	}
	g.gl = js.Null()
	g.blurProg, g.prog, g.fbo = js.Undefined(), js.Undefined(), js.Undefined()
	g.srcTex, g.tempTex, g.blurTex = js.Undefined(), js.Undefined(), js.Undefined()
	g.texW, g.texH = 0, 0
}

// shownDark reports whether the surface currently renders in dark mode,
// matching setAppearance's precedence (forced > hosted > system).
func (s *surface) shownDark() bool {
	a := s.forced
	if a == driver.AppearanceSystem {
		a = s.hosted
	}
	switch a {
	case driver.AppearanceDark:
		return true
	case driver.AppearanceLight:
		return false
	default:
		return s.env.Dark
	}
}

func (s *surface) pixelRatio() float64 {
	if s.env.PixelRatio > 0 {
		return float64(s.env.PixelRatio)
	}
	if dpr := js.Global().Get("devicePixelRatio"); dpr.Type() == js.TypeNumber && dpr.Float() > 0 {
		return dpr.Float()
	}
	return 1
}

// syncGlassView registers or unregisters v when its ViewStyle changes
// and marks the glass stage dirty for the end of the mount batch.
func (s *surface) syncGlassView(v *view) {
	want := v.style.Glass != driver.GlassNone
	if !want && (s.glass == nil || !s.glass.views[v]) {
		// An ancestor of a glass view may have changed blend/mask/opacity.
		if s.glass != nil && len(s.glass.views) > 0 {
			s.glass.dirty = true
		}
		return
	}
	if s.glass == nil {
		s.glass = &glassStage{views: map[*view]bool{}}
	}
	if want {
		s.glass.views[v] = true
	} else {
		delete(s.glass.views, v)
		if v.glassActive {
			v.glassActive = false
			v.el.Call("removeAttribute", "drawable")
		}
	}
	s.glass.dirty = true
}

// removeGlassView drops v from the glass stage on ItemDelete.
func (s *surface) removeGlassView(v *view) {
	if s.glass == nil {
		return
	}
	if s.glass.views[v] {
		delete(s.glass.views, v)
		v.glassActive = false
		s.glass.dirty = true
	} else if len(s.glass.views) > 0 {
		s.glass.dirty = true
	}
}

// markGlassDirty notes that DOM structure or styling changed while
// glass views exist.
func (s *surface) markGlassDirty() {
	if s.glass != nil && len(s.glass.views) > 0 {
		s.glass.dirty = true
	}
}

// refreshGlassAppearance updates fallback CSS and schedules a shader
// repaint when light/dark mode or pixel ratio changes.
func (s *surface) refreshGlassAppearance() {
	if s.glass == nil || len(s.glass.views) == 0 {
		return
	}
	for v := range s.glass.views {
		if s.view(v.id) == v && !v.glassActive {
			s.setLook(v)
		}
	}
	if s.glass.active {
		s.requestGlassPaint()
	}
}

// glassEligible reports whether v can render through the HTML-in-Canvas
// glass shader without risking a Chrome hang or self-capture.
func (s *surface) glassEligible(v *view) bool {
	if v == nil || s.root == nil || v == s.root || s.view(v.id) != v {
		return false
	}
	if v.style.Glass == driver.GlassNone || v.style.Hidden || v.style.Opacity <= 0 {
		return false
	}
	if !v.el.Get("isConnected").Bool() || !s.root.el.Call("contains", v.el).Bool() {
		return false
	}
	vs := v.style
	if v.animated.Set != 0 {
		vs = v.animated.Apply(vs)
	}
	if _, _, _, _, ok := webstyle.GlassBoxTransform(0, 0, 1, 1, &vs); !ok {
		return false
	}
	// Walk ancestors between v and s.root: refuse any ancestor that
	// triggers Chrome's nested-drawable hang (blend/mask/translucent
	// backdrop) or nested glass.
	for el := v.el.Get("parentElement"); el.Truthy() && !el.Equal(s.root.el); el = el.Get("parentElement") {
		if anc := s.viewOf(el); anc != nil {
			as := anc.style
			if anc.animated.Set != 0 {
				as = anc.animated.Apply(as)
			}
			if webstyle.GlassUnsafeAncestor(&as) {
				return false
			}
		}
	}
	return true
}

// syncGlass reconciles the HTML-in-Canvas stage, layer drawables, and
// each glass view's active/fallback CSS.
func (s *surface) syncGlass() {
	g := s.glass
	if g == nil {
		return
	}
	g.dirty = false
	if len(g.views) == 0 || s.root == nil || !glassSupportedFunc() {
		for v := range g.views {
			if s.view(v.id) == v && v.glassActive {
				v.glassActive = false
				s.setLook(v)
			}
		}
		s.teardownGlass()
		return
	}

	eligible := make(map[*view]bool, len(g.views))
	for v := range g.views {
		if s.view(v.id) != v {
			delete(g.views, v)
			continue
		}
		if s.glassEligible(v) {
			eligible[v] = true
		}
	}
	if len(eligible) == 0 {
		for v := range g.views {
			if v.glassActive {
				v.glassActive = false
				s.setLook(v)
			}
		}
		s.teardownGlass()
		return
	}

	if !s.activateGlassStage() {
		for v := range g.views {
			if v.glassActive {
				v.glassActive = false
				s.setLook(v)
			}
		}
		return
	}

	// Update each glass view's active state and CSS look.
	for v := range g.views {
		on := eligible[v]
		if v.glassActive != on {
			v.glassActive = on
			s.setLook(v)
		}
	}

	// Collect all depth-1 drawable layers inside s.root.el: every
	// eligible glass view plus any following sibling of an ancestor
	// along the path to s.root.el (overlays/modals/popups above glass)
	// that does not contain an eligible glass view.
	layerMap := make([]glassLayer, 0, len(eligible)*2)
	hasLayer := func(el js.Value) bool {
		for _, l := range layerMap {
			if l.el.Equal(el) {
				return true
			}
		}
		return false
	}
	containsGlass := func(el js.Value) bool {
		for gv := range eligible {
			if el.Equal(gv.el) || el.Call("contains", gv.el).Bool() {
				return true
			}
		}
		return false
	}
	for gv := range eligible {
		layerMap = append(layerMap, glassLayer{el: gv.el, view: gv})
	}
	for gv := range eligible {
		for cur := gv.el; cur.Truthy() && !cur.Equal(s.root.el); cur = cur.Get("parentElement") {
			for sib := cur.Get("nextElementSibling"); sib.Truthy(); sib = sib.Get("nextElementSibling") {
				if hasLayer(sib) || containsGlass(sib) {
					continue
				}
				layerMap = append(layerMap, glassLayer{el: sib})
			}
		}
	}

	// Sort layers in DOM document order (exact paint order).
	slices.SortFunc(layerMap, func(a, b glassLayer) int {
		if a.el.Equal(b.el) {
			return 0
		}
		pos := a.el.Call("compareDocumentPosition", b.el).Int()
		if pos&4 != 0 { // DOCUMENT_POSITION_FOLLOWING: b follows a
			return -1
		}
		if pos&2 != 0 { // DOCUMENT_POSITION_PRECEDING: b precedes a
			return 1
		}
		return 0
	})

	// Update drawable attributes cleanly without touching unchanged ones.
	wanted := make([]js.Value, 0, len(layerMap)+1)
	wanted = append(wanted, s.root.el)
	for _, l := range layerMap {
		wanted = append(wanted, l.el)
	}
	inWanted := func(el js.Value) bool {
		for _, w := range wanted {
			if w.Equal(el) {
				return true
			}
		}
		return false
	}
	for _, old := range g.drawables {
		if old.Truthy() && !inWanted(old) {
			old.Call("removeAttribute", "drawable")
		}
	}
	for _, w := range wanted {
		if !w.Call("hasAttribute", "drawable").Bool() {
			w.Call("setAttribute", "drawable", "")
		}
	}
	g.drawables = wanted
	g.layers = layerMap
	s.requestGlassPaint()
}

// activateGlassStage creates the HTML-in-Canvas stage and shared WebGL 2
// renderer and moves s.root.el inside it.
func (s *surface) activateGlassStage() bool {
	g := s.glass
	if g.active {
		return true
	}
	if s.root == nil || !s.host.Truthy() {
		return false
	}
	if g.gl == nil {
		g.gl = newGlassGL(s.doc)
	}
	if g.gl.failed {
		return false
	}
	if !g.styled && s.doc.Call("getElementById", "nsgo-glass-style").IsNull() {
		st := s.doc.Call("createElement", "style")
		st.Set("id", "nsgo-glass-style")
		st.Set("textContent", glassStageCSS)
		s.doc.Get("head").Call("appendChild", st)
	}
	g.styled = true

	stage := s.doc.Call("createElement", "canvas")
	stage.Set("className", "nsgo-glass-stage")
	stage.Call("setAttribute", "content", "drawable")
	ctx := stage.Call("getContext", "2d")
	if !ctx.Truthy() {
		return false
	}
	crop := s.doc.Call("createElement", "canvas")
	cropCtx := crop.Call("getContext", "2d")
	if !cropCtx.Truthy() {
		return false
	}

	g.stage, g.ctx = stage, ctx
	g.cropCanvas, g.cropCtx = crop, cropCtx
	g.paintFn = js.FuncOf(func(js.Value, []js.Value) any {
		s.paintGlassStage()
		return nil
	})
	stage.Call("addEventListener", "paint", g.paintFn)

	// Place stage where s.root.el sits, then move s.root.el inside it
	// (preserving scroll offsets and focus via moveBefore when available).
	s.host.Call("insertBefore", stage, s.root.el)
	if canMove() && s.root.el.Get("isConnected").Bool() {
		stage.Call("moveBefore", s.root.el, js.Null())
	} else {
		stage.Call("appendChild", s.root.el)
	}
	s.root.el.Call("setAttribute", "drawable", "")
	g.drawables = []js.Value{s.root.el}
	g.active = true
	return true
}

// teardownGlass removes the HTML-in-Canvas stage, restores s.root.el
// directly under s.host, strips all drawable attributes, and releases
// listeners and WebGL 2 resources.
func (s *surface) teardownGlass() {
	g := s.glass
	if g == nil {
		return
	}
	for _, el := range g.drawables {
		if el.Truthy() {
			el.Call("removeAttribute", "drawable")
		}
	}
	g.drawables = nil
	g.layers = nil
	if g.active {
		g.active = false
		if g.stage.Truthy() {
			if !g.paintFn.IsUndefined() {
				g.stage.Call("removeEventListener", "paint", g.paintFn)
				g.paintFn.Release()
				g.paintFn = js.Func{}
			}
			if s.root != nil && s.root.el.Truthy() && s.host.Truthy() {
				s.root.el.Call("removeAttribute", "drawable")
				if s.root.el.Get("parentNode").Equal(g.stage) {
					if canMove() && s.root.el.Get("isConnected").Bool() {
						s.host.Call("moveBefore", s.root.el, g.stage)
					} else {
						s.host.Call("insertBefore", s.root.el, g.stage)
					}
				}
			}
			g.stage.Call("remove")
		}
		g.stage, g.ctx = js.Undefined(), js.Undefined()
		g.cropCanvas, g.cropCtx = js.Undefined(), js.Undefined()
	}
	if g.gl != nil {
		g.gl.release()
		g.gl = nil
	}
}

// syncGlassStageSize sizes the stage bitmap to match its CSS box at dpr.
func (s *surface) syncGlassStageSize() (pw, ph float64, ok bool) {
	g := s.glass
	if g == nil || !g.active || !g.stage.Truthy() {
		return 0, 0, false
	}
	dpr := s.pixelRatio()
	sr := g.stage.Call("getBoundingClientRect")
	cw, ch := sr.Get("width").Float(), sr.Get("height").Float()
	if cw <= 0 || ch <= 0 {
		return 0, 0, false
	}
	pw, ph = math.Ceil(cw*dpr), math.Ceil(ch*dpr)
	if g.stage.Get("width").Float() != pw || g.stage.Get("height").Float() != ph {
		g.stage.Set("width", pw)
		g.stage.Set("height", ph)
	}
	return pw, ph, true
}

// requestGlassPaint schedules a paint event on the HTML-in-Canvas stage.
func (s *surface) requestGlassPaint() {
	g := s.glass
	if g == nil || !g.active || g.painting || !g.stage.Truthy() {
		return
	}
	if _, _, ok := s.syncGlassStageSize(); !ok {
		return
	}
	safeCall(func() { g.stage.Call("requestPaint") })
}

// paintGlassStage renders the app tree, nested <canvas> elements,
// Liquid Glass shader surfaces, and overlays onto g.stage.
func (s *surface) paintGlassStage() {
	g := s.glass
	if g == nil || !g.active || g.painting || s.root == nil {
		return
	}
	g.painting = true
	defer func() { g.painting = false }()

	pw, ph, ok := s.syncGlassStageSize()
	if !ok {
		return
	}
	dpr := s.pixelRatio()
	sr := g.stage.Call("getBoundingClientRect")
	ctx := g.ctx
	ctx.Call("setTransform", 1, 0, 0, 1, 0, 0)
	ctx.Call("clearRect", 0, 0, pw, ph)

	// 1. Base app tree (everything not in a nested drawable layer).
	safeCall(func() { ctx.Call("drawElementImage", s.root.el, 0, 0) })
	s.compositeLayerCanvases(ctx, s.root.el, sr, dpr)

	dark := s.shownDark()

	// 2. Glass views and overlay layers in DOM paint order.
	for _, layer := range g.layers {
		if !layer.el.Truthy() || !layer.el.Get("isConnected").Bool() {
			continue
		}
		lx, ly, locOK := layerLayoutOrigin(layer.el, sr)
		if !locOK {
			continue
		}
		if layer.view == nil {
			// Overlay sibling above a glass view.
			ctx.Call("save")
			s.clipAncestorOverflows(ctx, layer.el, s.root.el, sr, dpr)
			safeCall(func() { ctx.Call("drawElementImage", layer.el, math.Round(lx*dpr), math.Round(ly*dpr)) })
			s.compositeLayerCanvases(ctx, layer.el, sr, dpr)
			ctx.Call("restore")
			continue
		}

		v := layer.view
		if s.view(v.id) != v || !v.glassActive {
			continue
		}
		vs := v.style
		if v.animated.Set != 0 {
			vs = v.animated.Apply(vs)
		}
		if vs.Hidden || vs.Opacity <= 0 {
			continue
		}
		gw := layer.el.Get("offsetWidth").Float()
		gh := layer.el.Get("offsetHeight").Float()
		gx, gy, sw, sh, boxOK := webstyle.GlassBoxTransform(lx, ly, gw, gh, &vs)
		if !boxOK || sw <= 0 || sh <= 0 {
			continue
		}
		u, uOK := webstyle.ComputeGlassUniforms(&vs, float32(sw), float32(sh), float32(dpr), dark)
		if !uOK {
			continue
		}

		ctx.Call("save")
		s.clipAncestorOverflows(ctx, layer.el, s.root.el, sr, dpr)

		// Outer box shadows (drawElementImage clips drawables to their border box).
		drawGlassOuterShadows(ctx, &vs, gx*dpr, gy*dpr, float64(u.Rect[2]), float64(u.Rect[3]), u.Radii, dpr)

		// Snapshot blurred backdrop region behind v from the stage canvas
		// as painted so far (excluding v itself and anything above v).
		sigma := float64(u.Blur) * dpr
		pad := math.Min(math.Max(math.Ceil(sigma*2.5), 4), 64)
		boxLeft, boxTop := gx*dpr, gy*dpr
		boxRight, boxBottom := boxLeft+float64(u.Rect[2]), boxTop+float64(u.Rect[3])
		sx := math.Max(0, math.Floor(boxLeft-pad))
		sy := math.Max(0, math.Floor(boxTop-pad))
		ex := math.Min(pw, math.Ceil(boxRight+pad))
		ey := math.Min(ph, math.Ceil(boxBottom+pad))
		cw, ch := int(ex-sx), int(ey-sy)
		if cw > 0 && ch > 0 {
			if g.cropCanvas.Get("width").Int() != cw || g.cropCanvas.Get("height").Int() != ch {
				g.cropCanvas.Set("width", cw)
				g.cropCanvas.Set("height", ch)
			}
			g.cropCtx.Call("setTransform", 1, 0, 0, 1, 0, 0)
			g.cropCtx.Call("clearRect", 0, 0, cw, ch)
			g.cropCtx.Call("drawImage", g.stage, sx, sy, cw, ch, 0, 0, cw, ch)

			padX, padY := boxLeft-sx, boxTop-sy
			if g.gl.render(g.cropCanvas, cw, ch, padX, padY, u, sigma) {
				alpha := effectiveAncestorAlpha(layer.el, s.root.el) * float64(vs.Opacity)
				ctx.Set("globalAlpha", math.Max(0, math.Min(1, alpha)))
				ctx.Call("drawImage", g.gl.canvas, boxLeft, boxTop, float64(u.Rect[2]), float64(u.Rect[3]))
			}
		}

		// Draw v's own DOM borders and children on top of the glass.
		ctx.Set("globalAlpha", 1)
		safeCall(func() { ctx.Call("drawElementImage", layer.el, math.Round(lx*dpr), math.Round(ly*dpr)) })
		s.compositeLayerCanvases(ctx, layer.el, sr, dpr)
		ctx.Call("restore")
	}

	g.paintCount++
}

// layerLayoutOrigin computes el's untransformed layout top-left in CSS
// pixels relative to the stage bounding rect sr using offsetParent.
func layerLayoutOrigin(el, sr js.Value) (x, y float64, ok bool) {
	op := el.Get("offsetParent")
	if !op.Truthy() {
		return 0, 0, false
	}
	opr := op.Call("getBoundingClientRect")
	x = (opr.Get("left").Float() + op.Get("clientLeft").Float() - sr.Get("left").Float()) + el.Get("offsetLeft").Float()
	y = (opr.Get("top").Float() + op.Get("clientTop").Float() - sr.Get("top").Float()) + el.Get("offsetTop").Float()
	return x, y, true
}

// compositeLayerCanvases draws visible descendant <canvas> elements
// belonging directly to layerEl (skipping nested drawable subtrees and
// the glass stage itself) onto ctx. Chrome's drawElementImage omits
// WebGL canvases and larger/scaled 2D canvases (icons, charts), so
// compositing them here preserves every <canvas> in the app both on
// screen and inside glass backdrops without recursion.
func (s *surface) compositeLayerCanvases(ctx, layerEl, sr js.Value, dpr float64) {
	canvases := layerEl.Call("querySelectorAll", "canvas")
	n := canvases.Length()
	if n == 0 {
		return
	}
	stage := s.glass.stage
	stageLeft, stageTop := sr.Get("left").Float(), sr.Get("top").Float()
	for i := range n {
		c := canvases.Index(i)
		if !c.Truthy() || c.Equal(stage) || c.Get("width").Int() <= 0 || c.Get("height").Int() <= 0 {
			continue
		}
		if insideNestedDrawable(c, layerEl) {
			continue
		}
		alpha, visible := canvasEffectiveAlpha(c, layerEl)
		if !visible || alpha <= 0 {
			continue
		}
		cr := c.Call("getBoundingClientRect")
		cw, ch := cr.Get("width").Float(), cr.Get("height").Float()
		if cw <= 0 || ch <= 0 {
			continue
		}
		dx := (cr.Get("left").Float() - stageLeft) * dpr
		dy := (cr.Get("top").Float() - stageTop) * dpr
		ctx.Call("save")
		s.clipAncestorOverflows(ctx, c, layerEl, sr, dpr)
		ctx.Set("globalAlpha", alpha)
		safeCall(func() { ctx.Call("drawImage", c, dx, dy, cw*dpr, ch*dpr) })
		ctx.Call("restore")
	}
}

// insideNestedDrawable reports whether el sits inside a drawable
// descendant of layerEl (other than layerEl itself).
func insideNestedDrawable(el, layerEl js.Value) bool {
	for p := el; p.Truthy() && !p.Equal(layerEl); p = p.Get("parentElement") {
		if p.Call("hasAttribute", "drawable").Bool() {
			return true
		}
	}
	return false
}

// canvasEffectiveAlpha returns the combined opacity of el and its
// ancestors up to (and including) layerEl, or visible=false when hidden.
func canvasEffectiveAlpha(el, layerEl js.Value) (alpha float64, visible bool) {
	alpha = 1
	g := js.Global()
	for cur := el; cur.Truthy(); cur = cur.Get("parentElement") {
		cs := g.Call("getComputedStyle", cur)
		if cs.Get("display").String() == "none" || cs.Get("visibility").String() == "hidden" {
			return 0, false
		}
		alpha *= parseNumber(cs.Get("opacity").String(), 1)
		if alpha <= 0 {
			return 0, false
		}
		if cur.Equal(layerEl) {
			break
		}
	}
	return alpha, true
}

func effectiveAncestorAlpha(el, rootEl js.Value) float64 {
	alpha := 1.0
	g := js.Global()
	for cur := el.Get("parentElement"); cur.Truthy(); cur = cur.Get("parentElement") {
		cs := g.Call("getComputedStyle", cur)
		alpha *= parseNumber(cs.Get("opacity").String(), 1)
		if cur.Equal(rootEl) {
			break
		}
	}
	return alpha
}

// clipAncestorOverflows intersects ctx's clip with every ancestor
// between el and stopEl whose overflow clips its descendants.
func (s *surface) clipAncestorOverflows(ctx, el, stopEl, sr js.Value, dpr float64) {
	g := js.Global()
	stageLeft, stageTop := sr.Get("left").Float(), sr.Get("top").Float()
	for p := el.Get("parentElement"); p.Truthy(); p = p.Get("parentElement") {
		cs := g.Call("getComputedStyle", p)
		ox := cs.Get("overflowX").String()
		oy := cs.Get("overflowY").String()
		if (ox != "visible" && ox != "") || (oy != "visible" && oy != "") {
			pr := p.Call("getBoundingClientRect")
			x := (pr.Get("left").Float() - stageLeft) * dpr
			y := (pr.Get("top").Float() - stageTop) * dpr
			w := pr.Get("width").Float() * dpr
			h := pr.Get("height").Float() * dpr
			r := parseNumber(cs.Get("borderTopLeftRadius").String(), 0) * dpr
			ctx.Call("beginPath")
			if r > 0 {
				r = math.Min(r, math.Min(w, h)/2)
				ctx.Call("roundRect", x, y, w, h, r)
			} else {
				ctx.Call("rect", x, y, w, h)
			}
			ctx.Call("clip")
		}
		if p.Equal(stopEl) {
			break
		}
	}
}

// drawGlassOuterShadows draws non-inset box shadows for a glass view
// directly onto the stage canvas before the glass surface draws, since
// drawElementImage clips a drawable element to its border box.
func drawGlassOuterShadows(ctx js.Value, vs *driver.ViewStyle, x, y, w, h float64, radii [4]float32, dpr float64) {
	type shadowSpec struct {
		ox, oy, blur, spread float64
		color                driver.Color
	}
	var specs []shadowSpec
	if e := vs.Effects; e != nil {
		for i := len(e.Shadows) - 1; i >= 0; i-- {
			sh := e.Shadows[i]
			if !sh.Inset && sh.Color != 0 {
				specs = append(specs, shadowSpec{
					ox: float64(sh.Offset.X) * dpr, oy: float64(sh.Offset.Y) * dpr,
					blur: float64(sh.Blur) * dpr, spread: float64(sh.Spread) * dpr,
					color: sh.Color,
				})
			}
		}
	}
	if vs.ShadowColor != 0 {
		specs = append(specs, shadowSpec{
			ox: float64(vs.ShadowOffset.X) * dpr, oy: float64(vs.ShadowOffset.Y) * dpr,
			blur: float64(2*vs.ShadowRadius) * dpr, color: vs.ShadowColor,
		})
	}
	if len(specs) == 0 {
		return
	}
	// Draw each shadow from an offscreen-shifted shape so the interior
	// fill never bleeds through the glass box.
	const shift = 1e5
	for _, sh := range specs {
		sx, sy := x-sh.spread, y-sh.spread
		sw, shh := w+2*sh.spread, h+2*sh.spread
		if sw <= 0 || shh <= 0 {
			continue
		}
		r := []any{
			math.Max(0, float64(radii[0])+sh.spread),
			math.Max(0, float64(radii[1])+sh.spread),
			math.Max(0, float64(radii[2])+sh.spread),
			math.Max(0, float64(radii[3])+sh.spread),
		}
		ctx.Call("save")
		// Clip out the interior of the glass box using even-odd fill rule.
		ctx.Call("beginPath")
		ctx.Call("rect", x-sh.blur*3-math.Abs(sh.ox)-math.Abs(sh.spread)-16,
			y-sh.blur*3-math.Abs(sh.oy)-math.Abs(sh.spread)-16,
			w+sh.blur*6+2*math.Abs(sh.ox)+2*math.Abs(sh.spread)+32,
			h+sh.blur*6+2*math.Abs(sh.oy)+2*math.Abs(sh.spread)+32)
		ctx.Call("roundRect", x, y, w, h, []any{float64(radii[0]), float64(radii[1]), float64(radii[2]), float64(radii[3])})
		ctx.Call("clip", "evenodd")

		ctx.Set("fillStyle", "#000")
		ctx.Set("shadowColor", webstyle.Color(sh.color))
		ctx.Set("shadowBlur", sh.blur)
		ctx.Set("shadowOffsetX", sh.ox+shift)
		ctx.Set("shadowOffsetY", sh.oy)
		ctx.Call("beginPath")
		ctx.Call("roundRect", sx-shift, sy, sw, shh, r)
		ctx.Call("fill")
		ctx.Call("restore")
	}
}

// safeCall invokes fn and recovers any js.Error thrown by browser APIs
// (such as drawElementImage on an element awaiting its first layout).
func safeCall(fn func()) {
	defer func() { _ = recover() }()
	fn()
}
