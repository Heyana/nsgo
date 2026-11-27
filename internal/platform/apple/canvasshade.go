//go:build darwin

package apple

import (
	"fmt"
	"image"
	"log"
	"math"
	"strings"
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasshade"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

// A canvas op that shades with a program (canvasshade) renders its pass
// with Metal into a texture of its own, which a blit copies into a buffer
// the CPU reads once the GPU finished, and draws the pixels as an image
// in the context's device space, under the clip, the alpha, the operator
// and the shadow Core Graphics keeps. Each program compiles once for the
// process on a goroutine, as a shader view's does; a canvas that drew
// before its program was ready draws again once it is.

// shadeCanvas runs a shaded op, set by EnableCanvasShaders: nil leaves
// DrawShader out and draws shaded images plain.
var shadeCanvas func(p *player, op *driver.DrawOp) bool

// EnableCanvasShaders links the Metal path of canvas ops that shade with
// programs.
func EnableCanvasShaders() {
	if shadeCanvas == nil {
		shadeCanvas = (*player).shade
	}
}

// shaded runs op through its program when it shades with one, reporting
// whether it did; a false report draws an image op plain.
func (p *player) shaded(op *driver.DrawOp) bool {
	return shadeCanvas != nil && canvasshade.Shades(p.drawing, op) && shadeCanvas(p, op)
}

var cgContextGetClipBoundingBox func(ctx uintptr) CGRect

// canvasPipeline is a program compiled for canvas passes: its pipeline
// state, 0 until ready or when it failed, and where its lanes lie.
type canvasPipeline struct {
	state    objc.ID
	ready    bool
	offsets  []int
	types    []shaderir.BasicType
	size     int
	textures int
	waiting  map[any]func()
}

var canvasPipelines = map[*driver.ShaderProgram]*canvasPipeline{}

func (p *player) shade(op *driver.DrawOp) bool {
	if !loadMetal() {
		return false
	}
	if cgContextGetClipBoundingBox == nil {
		cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		purego.RegisterLibFunc(&cgContextGetClipBoundingBox, cg, "CGContextGetClipBoundingBox")
	}
	sh := driver.PaintShader(op.Shader)
	pipe := canvasPipelines[sh.Program]
	if pipe == nil {
		pipe = compileCanvasMetal(sh.Program)
		canvasPipelines[sh.Program] = pipe
	}
	if !pipe.ready {
		if p.host.Redraw != nil {
			pipe.waiting[p.host.Owner] = p.host.Redraw
		}
		p.waited = true
		return true
	}
	if pipe.state == 0 {
		return false
	}
	var first *shadeImage
	var size canvasshade.Image
	if op.Kind != driver.DrawShader {
		imgSrc := p.drawing.Image(op)
		if imgSrc == nil {
			return false
		}
		first = p.shadeImage(*imgSrc)
		if first == nil {
			return true
		}
		defer first.done()
		size = canvasshade.Image{W: float64(first.points.Width), H: float64(first.points.Height),
			PixelsW: float64(first.w), PixelsH: float64(first.h)}
	}
	ctm := cgContextGetCTM(p.ctx)
	m := canvasshade.Affine{A: ctm.A, B: ctm.B, C: ctm.C, D: ctm.D, E: ctm.X, F: ctm.Y}
	clip := cgContextGetClipBoundingBox(p.ctx)
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		x, y := m.Apply(clip.Origin.X+c[0]*clip.Size.Width, clip.Origin.Y+c[1]*clip.Size.Height)
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	bounds := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
	pass, ok := canvasshade.Plan(p.drawing, op, m, bounds, size)
	if !ok {
		return true
	}
	textures := make([]objc.ID, pipe.textures)
	sizes := make([][2]float64, pipe.textures)
	for i := range textures {
		textures[i] = blankTexture()
		if i == 0 && first != nil {
			textures[i], sizes[i] = first.tex, [2]float64{float64(first.w), float64(first.h)}
			continue
		}
		if i < len(pass.Images) {
			if img := p.shadeImage(pass.Images[i]); img != nil {
				defer img.done()
				textures[i], sizes[i] = img.tex, [2]float64{float64(img.w), float64(img.h)}
			}
		}
	}
	lanes := pass.Lanes(pipe.textures, func(i int) (float64, float64) { return sizes[i][0], sizes[i][1] })
	buf := make([]byte, pipe.size)
	shaderir.PutLanes(buf, pipe.offsets, pipe.types, lanes, 1)
	w, h := pass.Bounds.Dx(), pass.Bounds.Dy()
	img := renderCanvasPass(pipe.state, w, h, pass.Pack(nil, false), len(pass.Vertices), buf, textures)
	if img == 0 {
		return true
	}
	defer Release(img)
	cgContextSaveGState(p.ctx)
	cgContextConcatCTM(p.ctx, ctm.invert())
	p.drawImage(img, float64(pass.Bounds.Min.X), float64(pass.Bounds.Min.Y), float64(w), float64(h), driver.ImageSamplingNearest)
	cgContextRestoreGState(p.ctx)
	return true
}

// shadeImage is an image a pass reads, as a Metal texture of w by h
// pixels and its size in points; done lets a texture made for the pass
// alone go.
type shadeImage struct {
	tex    objc.ID
	w, h   int
	points driver.Size
	owned  bool
}

func (s *shadeImage) done() {
	if s.owned {
		s.tex.Send(selRelease)
	}
}

// shadeImage returns src's texture, kept with the canvas image while it
// stays, or nil while it loads.
func (p *player) shadeImage(src driver.ImageSource) *shadeImage {
	if src.Offscreen != nil {
		img, width, height, points, waited := offscreenImage(src, p.host)
		p.waited = p.waited || waited
		if img == 0 {
			return nil
		}
		defer Release(img)
		t := textureOf(img, int(width), int(height))
		if t == 0 {
			return nil
		}
		return &shadeImage{tex: t, w: int(width), h: int(height), points: points, owned: true}
	}
	var im *loadedImage
	if p.host.image != nil {
		im = p.host.image(src, p.host.Ratio)
	} else {
		im = canvasImage(src, p.host)
	}
	if im == nil || im.image == 0 {
		p.waited = p.waited || im == nil || !im.failed
		return nil
	}
	w, h := int(cgImageGetWidth(im.image)), int(cgImageGetHeight(im.image))
	if im.metal == 0 {
		im.metal = textureOf(im.image, w, h)
		if im.metal == 0 {
			return nil
		}
	}
	return &shadeImage{tex: im.metal, w: w, h: h, points: im.points}
}

// renderCanvasPass draws count vertices with state into a transparent
// BGRA texture w by h and returns its pixels as an image, premultiplied,
// or 0.
func renderCanvasPass(state objc.ID, w, h int, vertices []float32, count int, uniforms []byte, textures []objc.ID) (img uintptr) {
	withPool(func() { img = encodeCanvasPass(state, w, h, vertices, count, uniforms, textures) })
	return img
}

func encodeCanvasPass(state objc.ID, w, h int, vertices []float32, count int, uniforms []byte, textures []objc.ID) uintptr {
	desc := objc.ID(Class("MTLTextureDescriptor")).Send(
		sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"), uint(mtlPixelFormatBGRA8Unorm), uint(w), uint(h), false)
	const usageShaderRead, usageRenderTarget, storagePrivate = 1, 4, 2
	desc.Send(sel("setUsage:"), uint(usageShaderRead|usageRenderTarget))
	desc.Send(sel("setStorageMode:"), uint(storagePrivate))
	target := mtlDevice.Send(sel("newTextureWithDescriptor:"), desc)
	if target == 0 {
		return 0
	}
	defer target.Send(selRelease)
	row := w * 4
	out := mtlDevice.Send(sel("newBufferWithLength:options:"), uint(row*h), uint(0))
	if out == 0 {
		return 0
	}
	defer out.Send(selRelease)
	pass := objc.ID(Class("MTLRenderPassDescriptor")).Send(sel("renderPassDescriptor"))
	att := pass.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(0))
	att.Send(sel("setTexture:"), target)
	att.Send(sel("setLoadAction:"), uint(mtlLoadActionClear))
	att.Send(sel("setStoreAction:"), uint(mtlStoreActionStore))
	att.Send(sel("setClearColor:"), mtlClearColor{})
	cb := mtlQueue.Send(sel("commandBuffer"))
	enc := cb.Send(sel("renderCommandEncoderWithDescriptor:"), pass)
	enc.Send(sel("setRenderPipelineState:"), state)
	vbytes := len(vertices) * 4
	var vbuf, ubuf objc.ID
	if vbytes <= mtlMaxBytes {
		enc.Send(sel("setVertexBytes:length:atIndex:"), unsafe.Pointer(&vertices[0]), uint(vbytes), uint(1))
	} else {
		vbuf = mtlDevice.Send(sel("newBufferWithBytes:length:options:"), unsafe.Pointer(&vertices[0]), uint(vbytes), uint(0))
		enc.Send(sel("setVertexBuffer:offset:atIndex:"), vbuf, uint(0), uint(1))
		defer vbuf.Send(selRelease)
	}
	if len(uniforms) <= mtlMaxBytes {
		enc.Send(sel("setFragmentBytes:length:atIndex:"), unsafe.Pointer(&uniforms[0]), uint(len(uniforms)), uint(0))
	} else {
		ubuf = mtlDevice.Send(sel("newBufferWithBytes:length:options:"), unsafe.Pointer(&uniforms[0]), uint(len(uniforms)), uint(0))
		enc.Send(sel("setFragmentBuffer:offset:atIndex:"), ubuf, uint(0), uint(0))
		defer ubuf.Send(selRelease)
	}
	for i, t := range textures {
		enc.Send(sel("setFragmentTexture:atIndex:"), t, uint(i))
	}
	enc.Send(sel("drawPrimitives:vertexStart:vertexCount:"), uint(mtlPrimitiveTypeTriangle), uint(0), uint(count))
	enc.Send(sel("endEncoding"))
	blit := cb.Send(sel("blitCommandEncoder"))
	blit.Send(sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
		target, uint(0), uint(0), mtlOrigin{}, mtlSize{W: uint(w), H: uint(h), D: 1}, out, uint(0), uint(row), uint(row*h))
	blit.Send(sel("endEncoding"))
	cb.Send(sel("commit"))
	cb.Send(sel("waitUntilCompleted"))
	data := objc.Send[unsafe.Pointer](out, sel("contents"))
	if data == nil {
		return 0
	}
	// bgraImage copies the pixels, so the buffer can go.
	return bgraImage(unsafe.Slice((*byte)(data), row*h), w, h)
}

// mtlOrigin and mtlSize are MTLOrigin and MTLSize.
type (
	mtlOrigin struct {
		_       structs.HostLayout
		X, Y, Z uint
	}
	mtlSize struct {
		_       structs.HostLayout
		W, H, D uint
	}
)

// metalCanvasMain is the stage a canvas pass runs the program's fragment
// function in: the vertices' clip positions, and dst, pos and the color
// they carry, which multiplies the program's.
const metalCanvasMain = `
struct NsgoCanvasOut {
	float4 position [[position]];
	float2 dst;
	float2 pos;
	float4 color;
};

vertex NsgoCanvasOut nsgoCanvasVertex(uint vid [[vertex_id]], constant float *v [[buffer(1)]]) {
	constant float *p = v + vid * 10;
	NsgoCanvasOut out;
	out.position = float4(p[0], p[1], 0.0, 1.0);
	out.dst = float2(p[2], p[3]);
	out.pos = float2(p[4], p[5]);
	out.color = float4(p[6], p[7], p[8], p[9]);
	return out;
}

fragment float4 nsgoCanvasFragment(NsgoCanvasOut in [[stage_in]], constant Uniforms& uniforms [[buffer(0)]]{{params}}) {
	return nsgoShader(uniforms{{args}}, float4(in.dst, 0.0, 1.0), in.pos) * in.color;
}
`

// compileCanvasMetal translates prog for canvas passes and compiles it on
// a goroutine, drawing again the canvases waiting for it once it is
// ready.
func compileCanvasMetal(prog *driver.ShaderProgram) *canvasPipeline {
	p := &canvasPipeline{waiting: map[any]func(){}}
	ir := prog.IR
	p.offsets, p.size = msl.UniformLanes(ir)
	p.size = max(p.size, 16)
	p.types = ir.LaneTypes()
	p.textures = ir.TextureCount
	var params, args strings.Builder
	for i := range ir.TextureCount {
		fmt.Fprintf(&params, ", texture2d<float> T%[1]d [[texture(%[1]d)]]", i)
		fmt.Fprintf(&args, ", T%d", i)
	}
	src := msl.Fragment(ir, "nsgoShader") +
		strings.NewReplacer("{{params}}", params.String(), "{{args}}", args.String()).Replace(metalCanvasMain)
	go withPool(func() {
		state, err := canvasPipelineState(src)
		if err != "" {
			log.Printf("nsgo: canvas shader: Metal: %s", err)
		}
		OnMain(func() {
			p.state, p.ready = state, true
			waiting := p.waiting
			p.waiting = nil
			for _, fn := range waiting {
				fn()
			}
		})
	})
	return p
}

// canvasPipelineState compiles src into a pipeline state that blends
// premultiplied colors source over, or returns Metal's error.
func canvasPipelineState(src string) (objc.ID, string) {
	var errObj objc.ID
	lib := mtlDevice.Send(sel("newLibraryWithSource:options:error:"), NSString(src), objc.ID(0), unsafe.Pointer(&errObj))
	if lib == 0 {
		return 0, errorText(errObj)
	}
	defer lib.Send(selRelease)
	vertex := lib.Send(sel("newFunctionWithName:"), NSString("nsgoCanvasVertex"))
	fragment := lib.Send(sel("newFunctionWithName:"), NSString("nsgoCanvasFragment"))
	defer vertex.Send(selRelease)
	defer fragment.Send(selRelease)
	desc := objc.ID(Class("MTLRenderPipelineDescriptor")).Send(selNew)
	defer desc.Send(selRelease)
	desc.Send(sel("setVertexFunction:"), vertex)
	desc.Send(sel("setFragmentFunction:"), fragment)
	att := desc.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(0))
	att.Send(sel("setPixelFormat:"), uint(mtlPixelFormatBGRA8Unorm))
	const one, oneMinusSourceAlpha = 1, 5
	att.Send(sel("setBlendingEnabled:"), true)
	att.Send(sel("setSourceRGBBlendFactor:"), uint(one))
	att.Send(sel("setSourceAlphaBlendFactor:"), uint(one))
	att.Send(sel("setDestinationRGBBlendFactor:"), uint(oneMinusSourceAlpha))
	att.Send(sel("setDestinationAlphaBlendFactor:"), uint(oneMinusSourceAlpha))
	errObj = 0
	state := mtlDevice.Send(sel("newRenderPipelineStateWithDescriptor:error:"), desc, unsafe.Pointer(&errObj))
	if state == 0 {
		return 0, errorText(errObj)
	}
	return state, ""
}
