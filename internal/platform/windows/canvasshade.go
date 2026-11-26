//go:build windows

package windows

import (
	"image"
	"log"
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasshade"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"
)

// A canvas op that shades with a program (canvasshade) renders its pass
// with Direct3D 11, on the device Direct2D draws with, into a texture of
// its own, and Direct2D draws the texture as a bitmap at the pass's
// pixels, under the canvas's clip, alpha, operator and shadow: no pixel
// goes through the CPU. A canvas that shades plays its drawing on each
// frame rather than from a command list, so each pass renders at the
// pixels the frame shows.

// shadeCanvas runs a shaded op, set by EnableCanvasShaders: nil leaves
// DrawShader out and draws shaded images plain.
var shadeCanvas func(c *canvasPlayer, op *driver.DrawOp) bool

// EnableCanvasShaders implements driver.CanvasShaderEnabler: it links
// the Direct3D path of canvas ops that shade with programs.
func (*window) EnableCanvasShaders() {
	if shadeCanvas == nil {
		shadeCanvas = (*canvasPlayer).shade
	}
}

// shaded runs op through its program when it shades with one, reporting
// whether it did; a false report draws an image op plain.
func (c *canvasPlayer) shaded(op *driver.DrawOp) bool {
	return shadeCanvas != nil && canvasshade.Shades(op) && shadeCanvas(c, op)
}

// Vtable indexes and values the canvas passes use besides shader.go's.
const (
	ctxIASetVertexBuffersCanvas = 18
	ctxOMSetBlendStateCanvas    = 35
	ctxRSSetStateCanvas         = 43
	d3dCreateInputLayoutCanvas  = 11
	d3dCreateBlendStateCanvas   = 20
	d3dCreateRasterizerCanvas   = 22
	d3dBindVertexBuffer         = 1
	dxgiFormatRG32Float         = 16
	dxgiFormatRGBA32Float       = 2
)

// canvasStage is the program's fragment function as the pixel stage of a
// canvas pass: the vertices' clip positions, and dst, pos and the color
// they carry, which multiplies the program's.
const canvasStage = `
struct NsgoCanvasIn {
	float2 clip : TEXCOORD0;
	float2 dst : TEXCOORD1;
	float2 pos : TEXCOORD2;
	float4 color : TEXCOORD3;
};

struct NsgoCanvasOut {
	float4 position : SV_Position;
	float2 dst : TEXCOORD0;
	float2 pos : TEXCOORD1;
	float4 color : TEXCOORD2;
};

NsgoCanvasOut nsgoCanvasVertex(NsgoCanvasIn i) {
	NsgoCanvasOut o;
	o.position = float4(i.clip, 0.0, 1.0);
	o.dst = i.dst;
	o.pos = i.pos;
	o.color = i.color;
	return o;
}

float4 nsgoCanvasPixel(NsgoCanvasOut i) : SV_Target {
	return nsgoShader(float4(i.dst, 0.0, 1.0), i.pos) * i.color;
}
`

// d3dCanvasProgram is a program compiled for canvas passes on the device
// of a generation.
type d3dCanvasProgram struct {
	vs, ps, layout, cb *comObject
	offsets            []int
	types              []shaderir.BasicType
	size               int
	textures           int
	generation         uint64
	failed             bool
}

var (
	d3dCanvasPrograms = map[*driver.ShaderProgram]*d3dCanvasProgram{}
	// canvasBlend blends premultiplied colors source over and
	// canvasRaster culls nothing, made in canvasStatesGeneration.
	canvasBlend, canvasRaster *comObject
	canvasStatesGeneration    uint64
)

// canvasProgram returns prog compiled for canvas passes, or nil.
func (g *gfx) canvasProgram(prog *driver.ShaderProgram) *d3dCanvasProgram {
	p := d3dCanvasPrograms[prog]
	if p != nil && p.generation == gfxGeneration {
		if p.failed {
			return nil
		}
		return p
	}
	if p != nil {
		releaseAll(&p.vs, &p.ps, &p.layout, &p.cb)
	}
	p = &d3dCanvasProgram{generation: gfxGeneration, failed: true}
	d3dCanvasPrograms[prog] = p
	ir := prog.IR
	src := hlsl.Fragment(ir, "nsgoShader", 0, 0) + canvasStage
	vs, ok := compileHLSL(src, "nsgoCanvasVertex", "vs_5_0")
	if !ok {
		return nil
	}
	ps, ok := compileHLSL(src, "nsgoCanvasPixel", "ps_5_0")
	if !ok {
		return nil
	}
	if failed(g.d3d.call(d3dCreateVertexShader, uintptr(unsafe.Pointer(&vs[0])), uintptr(len(vs)), 0,
		uintptr(unsafe.Pointer(&p.vs)))) ||
		failed(g.d3d.call(d3dCreatePixelShader, uintptr(unsafe.Pointer(&ps[0])), uintptr(len(ps)), 0,
			uintptr(unsafe.Pointer(&p.ps)))) {
		log.Printf("nsgo: canvas shader: Direct3D cannot make the shaders")
		return nil
	}
	elems := [4]d3d11InputElementDesc{
		{semanticName: &d3dTexcoordSemantic[0], semanticIndex: 0, format: dxgiFormatRG32Float, alignedByteOffset: 0},
		{semanticName: &d3dTexcoordSemantic[0], semanticIndex: 1, format: dxgiFormatRG32Float, alignedByteOffset: 8},
		{semanticName: &d3dTexcoordSemantic[0], semanticIndex: 2, format: dxgiFormatRG32Float, alignedByteOffset: 16},
		{semanticName: &d3dTexcoordSemantic[0], semanticIndex: 3, format: dxgiFormatRGBA32Float, alignedByteOffset: 24},
	}
	if failed(g.d3d.call(d3dCreateInputLayoutCanvas, uintptr(unsafe.Pointer(&elems[0])), uintptr(len(elems)),
		uintptr(unsafe.Pointer(&vs[0])), uintptr(len(vs)), uintptr(unsafe.Pointer(&p.layout)))) {
		log.Printf("nsgo: canvas shader: Direct3D cannot make the input layout")
		return nil
	}
	p.offsets, p.size = hlsl.UniformLanes(ir)
	p.types = ir.LaneTypes()
	p.textures = ir.TextureCount
	p.size = (max(p.size, 16) + 15) &^ 15
	desc := struct{ size, usage, bind, cpu, misc, stride uint32 }{size: uint32(p.size), bind: d3dBindConstantBuffer}
	if failed(g.d3d.call(d3dCreateBuffer, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&p.cb)))) {
		return nil
	}
	p.failed = false
	return p
}

// canvasStates makes the blend and rasterizer states canvas passes draw
// with, once per device; false when it cannot.
func (g *gfx) canvasStates() bool {
	if canvasBlend != nil && canvasStatesGeneration == gfxGeneration {
		return true
	}
	releaseAll(&canvasBlend, &canvasRaster)
	const blendOne, blendInvSrcAlpha, blendOpAdd = 2, 6, 1
	var bd d3d11BlendDesc
	bd.renderTarget[0] = d3d11RenderTargetBlendDesc{blendEnable: 1, srcBlend: blendOne, destBlend: blendInvSrcAlpha,
		blendOp: blendOpAdd, srcBlendAlpha: blendOne, destBlendAlpha: blendInvSrcAlpha, blendOpAlpha: blendOpAdd,
		renderTargetWriteMask: 0xf}
	const fillSolid, cullNone = 3, 1
	rd := d3d11RasterizerDesc{fillMode: fillSolid, cullMode: cullNone, depthClipEnable: 1}
	if failed(g.d3d.call(d3dCreateBlendStateCanvas, uintptr(unsafe.Pointer(&bd)), uintptr(unsafe.Pointer(&canvasBlend)))) ||
		failed(g.d3d.call(d3dCreateRasterizerCanvas, uintptr(unsafe.Pointer(&rd)), uintptr(unsafe.Pointer(&canvasRaster)))) {
		releaseAll(&canvasBlend, &canvasRaster)
		return false
	}
	canvasStatesGeneration = gfxGeneration
	return true
}

func (c *canvasPlayer) shade(op *driver.DrawOp) bool {
	g := c.p.g
	if g == nil || g.d3d == nil {
		return false
	}
	sh := driver.PaintShader(op.Shader)
	prog := g.canvasProgram(sh.Program)
	if prog == nil || !g.canvasStates() {
		return false
	}
	var size canvasshade.Image
	var first *d3dImage
	if op.Kind != driver.DrawShader {
		if op.Image.Offscreen != nil {
			// An offscreen bitmap is Direct2D's alone: it draws plain.
			return false
		}
		s := c.p.w.canvasImage(*op.Image)
		if s == nil || s.pic == nil {
			c.waited = c.waited || (s != nil && s.load != nil)
			return true
		}
		if first = s.shadeTexture(g); first == nil {
			return false
		}
		nat := natural(s)
		size = canvasshade.Image{W: float64(nat.Width), H: float64(nat.Height), PixelsW: first.w, PixelsH: first.h}
	}
	px := float64(c.pixels())
	toPixels := c.world.then(scaling(px, px))
	m := c.st.ctm.then(toPixels)
	bound := toPixels.bound(rect{0, 0, int32(math.Ceil(float64(c.width))), int32(math.Ceil(float64(c.height)))})
	pass, ok := canvasshade.Plan(op, canvasshade.Affine{A: m.a, B: m.b, C: m.c, D: m.d, E: m.e, F: m.f},
		image.Rect(int(bound.Left), int(bound.Top), int(bound.Right), int(bound.Bottom)), size)
	if !ok {
		return true
	}
	srvs := make([]uintptr, prog.textures)
	sizes := make([][2]float64, prog.textures)
	for i := range srvs {
		var im *d3dImage
		if i == 0 && first != nil {
			im = first
		} else if i < len(pass.Images) && pass.Images[i].Offscreen == nil {
			if s := c.p.w.canvasImage(pass.Images[i]); s != nil && s.pic != nil {
				im = s.shadeTexture(g)
			} else if s != nil && s.load != nil {
				c.waited = true
			}
		}
		if im != nil {
			srvs[i], sizes[i] = im.srv.ptr(), [2]float64{im.w, im.h}
		}
	}
	lanes := pass.Lanes(prog.textures, func(i int) (float64, float64) { return sizes[i][0], sizes[i][1] })
	buf := make([]byte, prog.size)
	shaderir.PutLanes(buf, prog.offsets, prog.types, lanes, 4)
	w, h := uint32(pass.Bounds.Dx()), uint32(pass.Bounds.Dy())
	bmp := c.renderPass(prog, w, h, pass.Pack(nil, false), len(pass.Vertices), buf, srvs, float32(px))
	if bmp == nil {
		return true
	}
	defer bmp.release()
	s := float32(px)
	b := pass.Bounds
	dst := rectF{float32(b.Min.X) / s, float32(b.Min.Y) / s, float32(b.Max.X) / s, float32(b.Max.Y) / s}
	alpha := c.st.alpha
	toView := c.world.invert()
	c.paint(func(dc *comObject, base matrix) {
		c.setTransform(dc, toView.then(base))
		dc.callF(rtDrawBitmap, 0b100, bmp.ptr(), uintptr(unsafe.Pointer(&dst)), f32(alpha), d2dInterpolationNearest, 0)
	})
	return true
}

// pixels returns the pixels a DIP of the context the player draws on
// covers.
func (c *canvasPlayer) pixels() float32 {
	if c.pixelScale > 0 {
		return c.pixelScale
	}
	return c.p.scale
}

// shadeTexture returns the slot's picture as a texture passes read,
// made once while the slot keeps its picture, or nil.
func (s *imageSlot) shadeTexture(g *gfx) *d3dImage {
	if s.shade != nil && s.shade.generation == gfxGeneration {
		return s.shade
	}
	s.releaseShade()
	pic := s.pic
	if pic == nil || pic.svg || len(pic.frames) == 0 || pic.width <= 0 || pic.height <= 0 ||
		len(pic.frames[0].Pix) < pic.width*pic.height*4 {
		return nil
	}
	desc := texture2DDesc{width: uint32(pic.width), height: uint32(pic.height), mipLevels: 1, arraySize: 1,
		format: dxgiFormatBGRA, sampleCount: 1, bindFlags: d3dBindShaderResource}
	data := struct {
		mem         uintptr
		pitch, size uint32
	}{uintptr(unsafe.Pointer(&pic.frames[0].Pix[0])), uint32(pic.width * 4), 0}
	im := &d3dImage{w: float64(pic.width), h: float64(pic.height), generation: gfxGeneration}
	if failed(g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&data)),
		uintptr(unsafe.Pointer(&im.tex)))) {
		return nil
	}
	if failed(g.d3d.call(d3dCreateShaderResourceView, im.tex.ptr(), 0, uintptr(unsafe.Pointer(&im.srv)))) {
		im.tex.release()
		return nil
	}
	s.shade = im
	return im
}

// releaseShade lets the slot's pass texture go.
func (s *imageSlot) releaseShade() {
	if s.shade != nil {
		releaseAll(&s.shade.srv, &s.shade.tex)
		s.shade = nil
	}
}

// passTarget is the texture the last canvas pass drew into, kept for the
// next of the same size: Direct2D flushes before each pass, so what it
// drew from the texture goes to the GPU before the pass draws over it.
type passTarget struct {
	tex, rtv, bmp *comObject
	dc            *comObject
	w, h          uint32
	scale         float32
	generation    uint64
}

var canvasTarget passTarget

// target returns the pass target w by h, as Direct2D's bitmap at scale
// pixels a DIP for dc, or nil.
func (t *passTarget) target(g *gfx, dc *comObject, w, h uint32, scale float32) *passTarget {
	if t.tex != nil && t.w == w && t.h == h && t.scale == scale && t.dc == dc && t.generation == gfxGeneration {
		return t
	}
	releaseAll(&t.bmp, &t.rtv, &t.tex)
	desc := texture2DDesc{width: w, height: h, mipLevels: 1, arraySize: 1, format: dxgiFormatBGRA,
		sampleCount: 1, bindFlags: d3dBindRenderTarget | d3dBindShaderResource}
	if failed(g.d3d.call(d3dCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&t.tex)))) {
		return nil
	}
	if failed(g.d3d.call(d3dCreateRenderTargetView, t.tex.ptr(), 0, uintptr(unsafe.Pointer(&t.rtv)))) {
		releaseAll(&t.tex)
		return nil
	}
	surface := t.tex.query(&iidDXGISurfaceForShader)
	if surface == nil {
		releaseAll(&t.rtv, &t.tex)
		return nil
	}
	defer surface.release()
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * scale, dpiY: 96 * scale}
	if failed(dc.call(dcCreateBitmapFromDxgiSurface, surface.ptr(), uintptr(unsafe.Pointer(&props)),
		uintptr(unsafe.Pointer(&t.bmp)))) {
		releaseAll(&t.rtv, &t.tex)
		return nil
	}
	t.dc, t.w, t.h, t.scale, t.generation = dc, w, h, scale, gfxGeneration
	return t
}

// renderPass draws count vertices with prog into the pass target, w by h
// and cleared, and returns Direct2D's bitmap of it at scale pixels a
// DIP, or nil.
func (c *canvasPlayer) renderPass(prog *d3dCanvasProgram, w, h uint32, vertices []float32, count int, uniforms []byte, srvs []uintptr, scale float32) *comObject {
	g, dc := c.p.g, c.p.dc
	var vb *comObject
	defer releaseAll(&vb)
	t := canvasTarget.target(g, dc, w, h, scale)
	if t == nil {
		return nil
	}
	vdesc := struct{ size, usage, bind, cpu, misc, stride uint32 }{size: uint32(len(vertices) * 4), bind: d3dBindVertexBuffer}
	vdata := struct {
		mem         uintptr
		pitch, size uint32
	}{uintptr(unsafe.Pointer(&vertices[0])), 0, 0}
	if failed(g.d3d.call(d3dCreateBuffer, uintptr(unsafe.Pointer(&vdesc)), uintptr(unsafe.Pointer(&vdata)),
		uintptr(unsafe.Pointer(&vb)))) {
		return nil
	}
	var ctx *comObject
	g.d3d.call(d3dGetImmediateContext, uintptr(unsafe.Pointer(&ctx)))
	if ctx == nil {
		return nil
	}
	defer ctx.release()
	// What Direct2D recorded so far draws before Direct3D's.
	dc.call(rtFlush, 0, 0)
	r := t.rtv.ptr()
	ctx.call(ctxOMSetRenderTargets, 1, uintptr(unsafe.Pointer(&r)), 0)
	vp := d3dViewport{w: float32(w), h: float32(h), maxDepth: 1}
	ctx.call(ctxRSSetViewports, 1, uintptr(unsafe.Pointer(&vp)))
	var clear [4]float32
	ctx.call(ctxClearRenderTargetView, r, uintptr(unsafe.Pointer(&clear[0])))
	var blendFactor [4]float32
	ctx.call(ctxOMSetBlendStateCanvas, canvasBlend.ptr(), uintptr(unsafe.Pointer(&blendFactor[0])), 0xffffffff)
	ctx.call(ctxRSSetStateCanvas, canvasRaster.ptr())
	ctx.call(ctxIASetInputLayout, prog.layout.ptr())
	ctx.call(ctxIASetPrimitiveTopology, d3dTriangleList)
	vbp, stride, offset := vb.ptr(), uint32(canvasshade.Floats*4), uint32(0)
	ctx.call(ctxIASetVertexBuffersCanvas, 0, 1, uintptr(unsafe.Pointer(&vbp)), uintptr(unsafe.Pointer(&stride)),
		uintptr(unsafe.Pointer(&offset)))
	ctx.call(ctxVSSetShader, prog.vs.ptr(), 0, 0)
	ctx.call(ctxPSSetShader, prog.ps.ptr(), 0, 0)
	ctx.call(ctxUpdateSubresource, prog.cb.ptr(), 0, 0, uintptr(unsafe.Pointer(&uniforms[0])), 0, 0)
	cb := prog.cb.ptr()
	ctx.call(ctxPSSetConstantBuffers, 0, 1, uintptr(unsafe.Pointer(&cb)))
	if len(srvs) > 0 {
		ctx.call(ctxPSSetShaderResources, 0, uintptr(len(srvs)), uintptr(unsafe.Pointer(&srvs[0])))
	}
	ctx.call(ctxDraw, uintptr(count), 0)
	// Unbound, the texture is Direct2D's to read, and the states go back
	// to the defaults shader views draw with.
	ctx.call(ctxOMSetRenderTargets, 0, 0, 0)
	if len(srvs) > 0 {
		none := make([]uintptr, len(srvs))
		ctx.call(ctxPSSetShaderResources, 0, uintptr(len(none)), uintptr(unsafe.Pointer(&none[0])))
	}
	var nullBuffer uintptr
	ctx.call(ctxIASetVertexBuffersCanvas, 0, 1, uintptr(unsafe.Pointer(&nullBuffer)), uintptr(unsafe.Pointer(&stride)),
		uintptr(unsafe.Pointer(&offset)))
	ctx.call(ctxIASetInputLayout, 0)
	ctx.call(ctxOMSetBlendStateCanvas, 0, 0, 0xffffffff)
	ctx.call(ctxRSSetStateCanvas, 0)
	t.bmp.addRef()
	return t.bmp
}
