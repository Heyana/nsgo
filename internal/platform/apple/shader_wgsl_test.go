//go:build darwin

package apple

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
	"github.com/nsgo-dev/nsgo/internal/shaderir"
	"github.com/nsgo-dev/nsgo/internal/shaderir/msl"
)

// TestWGSLCompilesForMetal compiles the WGSL compiler's test shaders with
// Metal, which checks the MSL they become.
func TestWGSLCompilesForMetal(t *testing.T) {
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	files, _ := filepath.Glob(filepath.Join("..", "..", "shader", "wgsl", "testdata", "*.wgsl"))
	if len(files) == 0 {
		t.Fatal("no shaders")
	}
	srcs := map[string]string{"crossfade": wgsl.Crossfade}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		srcs[filepath.Base(f)] = string(b)
	}
	for name, src := range srcs {
		p, err := wgsl.Compile(src)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if state, msg := metalPipelineState(msl.Fragment(p.IR, "nsgoShader") + metalStage(p.IR.TextureCount)); state == 0 {
			t.Errorf("%s: %s", name, msg)
		}
	}
}

// drawMetal draws ir into a w by h target with lanes and textures, and
// returns its pixels, BGRA premultiplied.
func drawMetal(t *testing.T, ir *shaderir.Program, lanes []float64, textures []objc.ID, w, h int) []byte {
	t.Helper()
	state, msg := metalPipelineState(msl.Fragment(ir, "nsgoShader") + metalStage(ir.TextureCount))
	if state == 0 {
		t.Fatal(msg)
	}
	defer state.Send(selRelease)
	desc := objc.ID(objc.GetClass("MTLTextureDescriptor")).Send(
		sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"), uint(mtlPixelFormatBGRA8Unorm), uint(w), uint(h), false)
	const usageRenderTarget, usageShaderRead = 4, 1
	desc.Send(sel("setUsage:"), uint(usageRenderTarget|usageShaderRead))
	target := mtlDevice.Send(sel("newTextureWithDescriptor:"), desc)
	if target == 0 {
		t.Fatal("no render target")
	}
	defer target.Send(selRelease)
	offsets, size := msl.UniformLanes(ir)
	buf := make([]byte, max(size, 16))
	shaderir.PutLanes(buf, offsets, ir.LaneTypes(), lanes, 1)
	pass := objc.ID(objc.GetClass("MTLRenderPassDescriptor")).Send(sel("renderPassDescriptor"))
	att := pass.Send(selColorAttachments).Send(selObjectAtIndexedSubscript, uint(0))
	att.Send(sel("setTexture:"), target)
	att.Send(sel("setLoadAction:"), uint(mtlLoadActionClear))
	att.Send(sel("setStoreAction:"), uint(mtlStoreActionStore))
	cb := mtlQueue.Send(sel("commandBuffer"))
	enc := cb.Send(sel("renderCommandEncoderWithDescriptor:"), pass)
	enc.Send(sel("setRenderPipelineState:"), state)
	enc.Send(sel("setFragmentBytes:length:atIndex:"), unsafe.Pointer(&buf[0]), uint(len(buf)), uint(0))
	for i, tex := range textures {
		enc.Send(sel("setFragmentTexture:atIndex:"), tex, uint(i))
	}
	enc.Send(sel("drawPrimitives:vertexStart:vertexCount:"), uint(mtlPrimitiveTypeTriangle), uint(0), uint(3))
	enc.Send(sel("endEncoding"))
	// Intel textures default to managed storage, whose CPU copy remains stale after a GPU write until a blit synchronizes it. Apple silicon uses shared storage, so only waiting for completion is needed there.
	const mtlStorageModeManaged = 1
	if objc.Send[uint](target, sel("storageMode")) == mtlStorageModeManaged {
		blit := cb.Send(sel("blitCommandEncoder"))
		blit.Send(sel("synchronizeResource:"), target)
		blit.Send(sel("endEncoding"))
	}
	cb.Send(sel("commit"))
	cb.Send(sel("waitUntilCompleted"))
	const mtlCommandBufferStatusCompleted = 4
	if status := objc.Send[uint](cb, sel("status")); status != mtlCommandBufferStatusCompleted {
		t.Fatalf("Metal command status %d: %s", status, GoString(cb.Send(sel("error")).Send(sel("localizedDescription"))))
	}
	pix := make([]byte, w*h*4)
	target.Send(sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:"), unsafe.Pointer(&pix[0]), uint(w*4),
		mtlRegion{W: uint(w), H: uint(h), D: 1}, uint(0))
	return pix
}

// TestWGSLDraws draws WGSL programs: shade's straight color comes out
// premultiplied, uv runs over the box, and sample_from and sample_to read
// pictures between their pixels, mixed by nsh.progress.
func TestWGSLDraws(t *testing.T) {
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	p, err := wgsl.Compile(`struct Uniforms { alpha: f32 }
fn shade(uv: vec2f) -> vec4f { return vec4f(uv.x, 0.0, 1.0, uniforms.alpha); }`)
	if err != nil {
		t.Fatal(err)
	}
	// The bridge's lanes, the built-ins' (time, progress, overflow), then alpha.
	pix := drawMetal(t, p.IR, []float64{8, 4, 1, 0, 0, 0, 0.5}, nil, 8, 4)
	right := pix[(1*8+7)*4:]
	if right[0] < 120 || right[0] > 135 || right[2] < 100 || right[3] < 120 || right[3] > 135 {
		t.Errorf("right pixel %v, want half blue and red (BGRA), premultiplied", right[:4])
	}

	cross, err := wgsl.Compile(wgsl.Crossfade)
	if err != nil {
		t.Fatal(err)
	}
	solid := func(b, g, r, a byte) objc.ID {
		desc := objc.ID(objc.GetClass("MTLTextureDescriptor")).Send(
			sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"), uint(mtlPixelFormatBGRA8Unorm), uint(2), uint(2), false)
		tex := mtlDevice.Send(sel("newTextureWithDescriptor:"), desc)
		px := []byte{b, g, r, a, b, g, r, a, b, g, r, a, b, g, r, a}
		tex.Send(sel("replaceRegion:mipmapLevel:withBytes:bytesPerRow:"), mtlRegion{W: 2, H: 2, D: 1}, uint(0), unsafe.Pointer(&px[0]), uint(8))
		return tex
	}
	from, to := solid(0, 0, 255, 255), solid(255, 0, 0, 255) // red, blue
	// Four textures: content and backdrop unused, from and to.
	lanes := []float64{8, 4, 1, 0, 0, 0, 0, 2, 2, 2, 2, 0, 0.25, 0}
	pix = drawMetal(t, cross.IR, lanes, []objc.ID{blankTexture(), blankTexture(), from, to}, 8, 4)
	px := pix[(2*8+4)*4:]
	if px[2] < 180 || px[2] > 200 || px[0] < 55 || px[0] > 75 || px[3] != 255 {
		t.Errorf("a quarter through, pixel %v, want three quarters red (BGRA)", px[:4])
	}
}

// TestWGSLRestDraws executes var<private>, user structs, non-square matrices,
// bit-counting, packing/unpacking, modf and frexp on Metal and checks the
// computed values against exact expected results.
func TestWGSLRestDraws(t *testing.T) {
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	src := `
struct Uniforms {
	bits: u32,
	sval: i32,
	fval: f32,
}
struct Accum {
	sum: f32,
	ok: bool,
}
var<private> g_state: Accum = Accum(0.25, true);
fn bump(delta: f32) {
	g_state.sum += delta;
}
fn shade(uv: vec2f) -> vec4f {
	// 1. var<private> + struct: 0.25 + 0.25 = 0.5
	bump(0.25);
	var r = 0.0;
	if (g_state.ok && abs(g_state.sum - 0.5) < 1e-4) {
		r += 0.25;
	}
	// 2. Non-square matrices: mat2x3 * vec2 -> vec3, transpose(mat2x3) -> mat3x2
	let m23 = mat2x3f(vec3f(1.0, 2.0, 3.0), vec3f(4.0, 5.0, 6.0));
	let v3 = m23 * vec2f(2.0, 3.0); // (14, 19, 24)
	let m32 = transpose(m23);
	let v2 = m32 * vec3f(1.0, 0.0, 1.0); // (4, 10)
	if (abs(v3.x - 14.0) < 1e-3 && abs(v3.y - 19.0) < 1e-3 && abs(v3.z - 24.0) < 1e-3 &&
		abs(v2.x - 4.0) < 1e-3 && abs(v2.y - 10.0) < 1e-3) {
		r += 0.25;
	}
	// 3. Bit-counting (runtime uniforms so not constant-folded):
	// bits = 0x00F01230u: clz=8, popcnt=8, ctz=4, flb=23, ftb=4, rev=0x0C480F00u
	// extractBits(bits, 4u, 8u) = 0x23u, insertBits(bits, 0xABu, 8u, 8u) = 0x00F0AB30u
	// sval = -12i (0xFFFFFFF4): flb(-12i)=3i, extractBits(-12i, 0u, 4u)=4i, extractBits(-12i, 2u, 4u)=-3i
	let u = uniforms.bits;
	let s = uniforms.sval;
	var g = 0.0;
	if (countLeadingZeros(u) == 8u && countOneBits(u) == 8u && countTrailingZeros(u) == 4u &&
		firstLeadingBit(u) == 23u && firstTrailingBit(u) == 4u && reverseBits(u) == 0x0C480F00u &&
		extractBits(u, 4u, 8u) == 0x23u && insertBits(u, 0xABu, 8u, 8u) == 0x00F0AB30u &&
		firstLeadingBit(s) == 3i && extractBits(s, 0u, 4u) == 4i && extractBits(s, 2u, 4u) == -3i) {
		g = 1.0;
	}
	// 4. Packing & unpacking round-trips + modf & frexp (runtime uniform fval = -3.25)
	let p8u = unpack4x8unorm(pack4x8unorm(vec4f(0.0, 1.0, 0.0, 1.0)));
	let p8s = unpack4x8snorm(pack4x8snorm(vec4f(-1.0, 0.0, 1.0, -1.0)));
	let p16u = unpack2x16unorm(pack2x16unorm(vec2f(0.0, 1.0)));
	let p16s = unpack2x16snorm(pack2x16snorm(vec2f(-1.0, 1.0)));
	let p16f = unpack2x16float(pack2x16float(vec2f(uniforms.fval, 1.5)));
	let mf = modf(uniforms.fval);
	let fx = frexp(uniforms.fval);
	var b = 0.0;
	if (abs(p8u.y - 1.0) < 1e-3 && abs(p8s.x + 1.0) < 1e-3 && abs(p8s.z - 1.0) < 1e-3 &&
		abs(p16u.y - 1.0) < 1e-3 && abs(p16s.x + 1.0) < 1e-3 &&
		abs(p16f.x - uniforms.fval) < 1e-3 && abs(p16f.y - 1.5) < 1e-3 &&
		abs(mf.whole + 3.0) < 1e-4 && abs(mf.fract + 0.25) < 1e-4 &&
		abs(fx.fract + 0.8125) < 1e-4 && fx.exp == 2i) {
		b = 1.0;
	}
	return vec4f(r * 2.0, g, b, 1.0);
}
`
	p, err := wgsl.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	// Bridge lanes (rectSize.x, rectSize.y, scale), built-ins (time, progress, overflow), then uniforms (bits, sval, fval).
	lanes := []float64{4, 4, 1, 0, 0, 0, float64(0x00F01230), -12, -3.25}
	pix := drawMetal(t, p.IR, lanes, nil, 4, 4)
	px := pix[(1*4+1)*4:]
	// BGRA: expect B=255 (pack/unpack/modf/frexp), G=255 (bit-counting), R=255 (privates+structs+matrices), A=255.
	if px[0] != 255 || px[1] != 255 || px[2] != 255 || px[3] != 255 {
		t.Errorf("pixel BGRA = %v, want [255 255 255 255]", px[:4])
	}
}

// TestPictureTakesLayers pictures a layer tree as it shows, top row
// first, with the overflow around it and its mask lifted.
func TestPictureTakesLayers(t *testing.T) {
	if !loadMetal() {
		t.Skip("no Metal device")
	}
	root := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	root.Send(sel("setGeometryFlipped:"), true)
	root.Send(sel("setFrame:"), Rect(0, 0, 20, 10))
	top := objc.ID(objc.GetClass("CALayer")).Send(selNew)
	top.Send(sel("setFrame:"), Rect(0, 0, 20, 5))
	top.Send(sel("setBackgroundColor:"), CGColor(driver.RGB(0xff0000)))
	root.Send(sel("addSublayer:"), top)
	root.Send(sel("setMask:"), objc.ID(objc.GetClass("CALayer")).Send(selNew))
	var p Picture
	defer p.Release()
	if !p.Take(root, -2, -2, 24, 14, 2, 0) {
		t.Fatal("no picture")
	}
	if p.w != 48 || p.h != 28 {
		t.Fatalf("picture %dx%d", p.w, p.h)
	}
	data := cgBitmapContextGetData(p.ctx)
	px := unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), p.w*p.h*4)
	at := func(x, y int) []byte { return px[(y*p.w+x)*4 : (y*p.w+x)*4+4] }
	if c := at(24, 8); c[2] != 255 || c[3] != 255 {
		t.Errorf("the top half is %v, want red (BGRA)", c)
	}
	if c := at(24, 22); c[3] != 0 {
		t.Errorf("the bottom half is %v, want clear", c)
	}
	if c := at(1, 8); c[3] != 0 {
		t.Errorf("the overflow is %v, want clear", c)
	}
	if root.Send(sel("mask")) == 0 {
		t.Error("the mask was not put back")
	}
}
