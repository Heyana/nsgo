package driver

import (
	"image/color"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// buildClipColorProgram builds a minimal shaderir.Program taking clip-space
// position vec4 (@location(0)) and varying color vec4 (@location(1)).
func buildClipColorProgram(interp shaderir.Interpolation) *shaderir.Program {
	vec4T := shaderir.Type{Main: shaderir.Vec4}
	return &shaderir.Program{
		VertexEntry:   "vs_main",
		FragmentEntry: "fs_main",
		Attributes:    []shaderir.Type{vec4T, vec4T},
		AttributeMeta: []shaderir.StageIO{
			{Name: "pos", Location: 0},
			{Name: "color", Location: 1},
		},
		Varyings:    []shaderir.Type{vec4T},
		VaryingMeta: []shaderir.StageIO{{Name: "color", Location: 0, Interpolation: interp}},
		VertexFunc: shaderir.VertexFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 4,
				Stmts: []shaderir.Stmt{
					{Type: shaderir.Assign, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 2}, {Type: shaderir.LocalVariable, Index: 0}}},
					{Type: shaderir.Assign, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 3}, {Type: shaderir.LocalVariable, Index: 1}}},
					{Type: shaderir.Return},
				},
			},
		},
		FragmentFunc: shaderir.FragmentFunc{
			Block: &shaderir.Block{
				LocalVarIndexOffset: 2,
				Stmts: []shaderir.Stmt{
					{Type: shaderir.Return, Exprs: []shaderir.Expr{{Type: shaderir.LocalVariable, Index: 1}}},
				},
			},
		},
	}
}

func runClipColorDraw(t *testing.T, w, h int, verts []float32, interp shaderir.Interpolation, ds *GPUDepthStencilState, cmdsBeforeDraw []GPUCommand) (*GPUSoftwareDevice, color.RGBA) {
	t.Helper()
	return runClipColorDrawBlend(t, w, h, verts, interp, ds, nil, cmdsBeforeDraw)
}

func runClipColorDrawBlend(t *testing.T, w, h int, verts []float32, interp shaderir.Interpolation, ds *GPUDepthStencilState, blend *GPUBlendState, cmdsBeforeDraw []GPUCommand) (*GPUSoftwareDevice, color.RGBA) {
	t.Helper()
	clearCol := color.RGBA{10, 20, 30, 255}
	dev := NewGPUSoftwareDevice(w, h, GPUViewConfig{
		ColorFormat: GPUFormatRGBA8Unorm,
		DepthFormat: GPUFormatDepth24Plus,
	})
	prog := buildClipColorProgram(interp)
	vbData := packF32s(verts...)
	var cmds []GPUCommand
	cmds = append(cmds, GPUCommand{Kind: GPUCmdSetPipeline, ID: 2})
	cmds = append(cmds, cmdsBeforeDraw...)
	cmds = append(cmds,
		GPUCommand{Kind: GPUCmdSetVertexBuffer, Slot: 0, ID: 1},
		GPUCommand{Kind: GPUCmdDraw, U0: uint32(len(verts) / 8)},
	)
	_, err := dev.Submit(GPUFrame{
		Ops: []GPUResourceOp{
			{Kind: GPUOpCreateBuffer, ID: 1, Buffer: &GPUBufferDesc{Size: uint64(len(vbData)), Usage: GPUBufferVertex, Data: vbData}},
			{Kind: GPUOpCreatePipeline, ID: 2, Pipeline: &GPUPipelineDesc{
				Vertex:   GPUShaderStage{IR: prog, EntryPoint: "vs_main"},
				Fragment: GPUShaderStage{IR: prog, EntryPoint: "fs_main"},
				VertexBuffers: []GPUVertexBufferLayout{{
					Stride:   32,
					StepMode: GPUStepVertex,
					Attributes: []GPUVertexAttribute{
						{Format: GPUVertexFloat32x4, Offset: 0, Location: 0},
						{Format: GPUVertexFloat32x4, Offset: 16, Location: 1},
					},
				}},
				DepthStencil: ds,
				Targets:      []GPUColorTargetState{{Format: GPUFormatRGBA8Unorm, Blend: blend}},
			}},
		},
		Passes: []GPUPass{{
			ColorAttachments: []GPUColorAttachment{{
				LoadOp:     GPULoadClear,
				StoreOp:    GPUStoreStore,
				ClearColor: RGB(0x0a141e),
			}},
			DepthStencil: &GPUDepthStencilAttachment{
				DepthLoadOp:  GPULoadClear,
				DepthStoreOp: GPUStoreStore,
				ClearDepth:   1.0,
			},
			Commands: cmds,
		}},
	})
	if err != nil {
		t.Fatalf("Submit(%dx%d): %v", w, h, err)
	}
	return dev, clearCol
}

func TestGPUSoftwareFarPlaneSkyboxCoverage(t *testing.T) {
	sizes := [][2]int{{13, 17}, {32, 32}, {64, 64}, {97, 53}}
	// Use varied non-power-of-two w values with z == w to trigger 1/w rounding if depth is not handled accurately.
	w0, w1, w2, w3 := float32(0.3), float32(7.0), float32(49.0), float32(13.0)
	quad := []float32{
		-w0, -w0, w0, w0, 0, 1, 0, 1,
		w1, -w1, w1, w1, 0, 1, 0, 1,
		w2, w2, w2, w2, 0, 1, 0, 1,
		-w0, -w0, w0, w0, 0, 1, 0, 1,
		w2, w2, w2, w2, 0, 1, 0, 1,
		-w3, w3, w3, w3, 0, 1, 0, 1,
	}
	fullscreenTri := []float32{
		-w0, -w0, w0, w0, 0, 1, 0, 1,
		3 * w1, -w1, w1, w1, 0, 1, 0, 1,
		-w2, 3 * w2, w2, w2, 0, 1, 0, 1,
	}
	ds := &GPUDepthStencilState{
		Format:       GPUFormatDepth24Plus,
		DepthWrite:   true,
		DepthCompare: GPUCompareLessEqual,
	}

	for _, name := range []string{"quad", "fullscreen_tri"} {
		verts := quad
		if name == "fullscreen_tri" {
			verts = fullscreenTri
		}
		for _, sz := range sizes {
			dev, _ := runClipColorDraw(t, sz[0], sz[1], verts, shaderir.InterpPerspective, ds, nil)
			img := dev.SurfaceImage()
			want := color.RGBA{0, 255, 0, 255}
			missed := 0
			for y := 0; y < sz[1]; y++ {
				for x := 0; x < sz[0]; x++ {
					if got := img.RGBAAt(x, y); got != want {
						if missed < 5 {
							t.Errorf("%s %dx%d pixel(%d,%d) = %+v, want %+v", name, sz[0], sz[1], x, y, got, want)
						}
						missed++
					}
				}
			}
			if missed > 0 {
				t.Errorf("%s %dx%d missed %d pixels at z == w with CompareLessEqual", name, sz[0], sz[1], missed)
			}
		}
	}
}

func TestGPUSoftwareNearPlaneClippingAndVaryings(t *testing.T) {
	// Triangle with v0 behind the eye (w < 0, z < 0) and v1, v2 in front.
	v0p := [4]float32{0.0, -1.5, -1.0, -1.0}
	v0c := [4]float32{1, 0, 0, 1}
	v1p := [4]float32{1.0, 1.0, 1.0, 2.0}
	v1c := [4]float32{0, 1, 0, 1}
	v2p := [4]float32{-1.0, 1.0, 1.0, 2.0}
	v2c := [4]float32{0, 0, 1, 1}

	// Exact pre-clipped intersection against z == 0 plane.
	clipAtZ0 := func(inP, inC, outP, outC [4]float32) ([4]float32, [4]float32) {
		t := inP[2] / (inP[2] - outP[2])
		var p, c [4]float32
		for i := range 4 {
			p[i] = inP[i] + t*(outP[i]-inP[i])
			c[i] = inC[i] + t*(outC[i]-inC[i])
		}
		p[2] = 0
		return p, c
	}
	c01p, c01c := clipAtZ0(v1p, v1c, v0p, v0c)
	c02p, c02c := clipAtZ0(v2p, v2c, v0p, v0c)

	unclipped := []float32{
		v0p[0], v0p[1], v0p[2], v0p[3], v0c[0], v0c[1], v0c[2], v0c[3],
		v1p[0], v1p[1], v1p[2], v1p[3], v1c[0], v1c[1], v1c[2], v1c[3],
		v2p[0], v2p[1], v2p[2], v2p[3], v2c[0], v2c[1], v2c[2], v2c[3],
	}
	preClipped := []float32{
		c01p[0], c01p[1], c01p[2], c01p[3], c01c[0], c01c[1], c01c[2], c01c[3],
		v1p[0], v1p[1], v1p[2], v1p[3], v1c[0], v1c[1], v1c[2], v1c[3],
		v2p[0], v2p[1], v2p[2], v2p[3], v2c[0], v2c[1], v2c[2], v2c[3],

		c01p[0], c01p[1], c01p[2], c01p[3], c01c[0], c01c[1], c01c[2], c01c[3],
		v2p[0], v2p[1], v2p[2], v2p[3], v2c[0], v2c[1], v2c[2], v2c[3],
		c02p[0], c02p[1], c02p[2], c02p[3], c02c[0], c02c[1], c02c[2], c02c[3],
	}

	const w, h = 48, 48
	ds := &GPUDepthStencilState{
		Format:       GPUFormatDepth24Plus,
		DepthWrite:   true,
		DepthCompare: GPUCompareLessEqual,
	}
	gotDev, clearCol := runClipColorDraw(t, w, h, unclipped, shaderir.InterpPerspective, ds, nil)
	refDev, _ := runClipColorDraw(t, w, h, preClipped, shaderir.InterpPerspective, ds, nil)
	gotImg := gotDev.SurfaceImage()
	refImg := refDev.SurfaceImage()

	drawn := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := gotImg.RGBAAt(x, y)
			r := refImg.RGBAAt(x, y)
			if r != clearCol {
				drawn++
			}
			dr := math.Abs(float64(int(g.R) - int(r.R)))
			dg := math.Abs(float64(int(g.G) - int(r.G)))
			db := math.Abs(float64(int(g.B) - int(r.B)))
			if dr > 2 || dg > 2 || db > 2 || g.A != r.A {
				t.Fatalf("near-clipped pixel(%d,%d) = %+v, pre-clipped ref = %+v", x, y, g, r)
			}
		}
	}
	if drawn < 100 {
		t.Fatalf("expected substantial clipped visible triangle area, got %d pixels", drawn)
	}

	// Verify provoking vertex v0 is preserved for InterpFlat even when v0 is behind the eye.
	flatDev, _ := runClipColorDraw(t, w, h, unclipped, shaderir.InterpFlat, ds, nil)
	flatImg := flatDev.SurfaceImage()
	flatCol := color.RGBA{255, 0, 0, 255}
	flatDrawn := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if refImg.RGBAAt(x, y) != clearCol {
				if g := flatImg.RGBAAt(x, y); g != flatCol {
					t.Fatalf("flat varying pixel(%d,%d) = %+v, want provoking v0 color %+v", x, y, g, flatCol)
				}
				flatDrawn++
			}
		}
	}
	if flatDrawn == 0 {
		t.Fatalf("expected flat-shaded near-clipped pixels")
	}

	// Also verify a triangle whose edge crosses z == 0 with w == 0 at z == 0 still clips against X/Y planes.
	edgeOriginVerts := []float32{
		0.0, 0.0, -0.5, -1.0, 0, 1, 1, 1,
		1.0, 1.0, 1.0, 2.0, 0, 1, 1, 1,
		-1.0, 1.0, 1.0, 2.0, 0, 1, 1, 1,
	}
	originDev, _ := runClipColorDraw(t, w, h, edgeOriginVerts, shaderir.InterpPerspective, ds, nil)
	originImg := originDev.SurfaceImage()
	originDrawn := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if originImg.RGBAAt(x, y) != clearCol {
				originDrawn++
			}
		}
	}
	if originDrawn == 0 {
		t.Errorf("expected visible area for triangle crossing w=0 at z=0")
	}
}

func TestGPUSoftwareViewportDepthRangeAndClamping(t *testing.T) {
	const w, h = 32, 32
	// A fullscreen quad at w = 1 whose depth ramps from z = 0 at x = -1 to z = 2 at x = +1, so the half with x > 0 lies beyond the far plane (z > w) and must be clipped away.
	verts := []float32{
		-1, -1, 0.0, 1, 0, 1, 0, 1,
		1, -1, 2.0, 1, 0, 1, 0, 1,
		1, 1, 2.0, 1, 0, 1, 0, 1,
		-1, -1, 0.0, 1, 0, 1, 0, 1,
		1, 1, 2.0, 1, 0, 1, 0, 1,
		-1, 1, 0.0, 1, 0, 1, 0, 1,
	}
	ds := &GPUDepthStencilState{
		Format:       GPUFormatDepth24Plus,
		DepthWrite:   true,
		DepthCompare: GPUCompareLessEqual,
	}
	vpCmd := []GPUCommand{{
		Kind: GPUCmdSetViewport,
		F0:   0, F1: 0, F2: w, F3: h, F4: 0.0, F5: 0.5,
	}}
	dev, clearCol := runClipColorDraw(t, w, h, verts, shaderir.InterpPerspective, ds, vpCmd)
	img := dev.SurfaceImage()
	green := color.RGBA{0, 255, 0, 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := dev.depthBuf[y*w+x]
			got := img.RGBAAt(x, y)
			if x < w/2-1 {
				if got != green {
					t.Fatalf("left half pixel(%d,%d) = %+v, want %+v", x, y, got, green)
				}
				if d < 0 || d > 0.5 {
					t.Fatalf("left half depth(%d,%d) = %v, want in [0, 0.5]", x, y, d)
				}
			} else if x >= w/2+1 {
				if got != clearCol {
					t.Fatalf("far-clipped pixel(%d,%d) = %+v, want clear %+v", x, y, got, clearCol)
				}
				if d != 1.0 {
					t.Fatalf("far-clipped depth(%d,%d) = %v, want untouched clear 1.0", x, y, d)
				}
			}
		}
	}

	// Verify positive and negative depth bias clamp strictly to [minDepth, maxDepth] = [0.25, 0.75].
	quadMax := []float32{
		-1, -1, 1, 1, 1, 0, 0, 1,
		1, -1, 1, 1, 1, 0, 0, 1,
		1, 1, 1, 1, 1, 0, 0, 1,
		-1, -1, 1, 1, 1, 0, 0, 1,
		1, 1, 1, 1, 1, 0, 0, 1,
		-1, 1, 1, 1, 1, 0, 0, 1,
	}
	dsPosBias := &GPUDepthStencilState{
		Format:         GPUFormatDepth24Plus,
		DepthWrite:     true,
		DepthCompare:   GPUCompareLessEqual,
		DepthBias:      100000,
		DepthBiasClamp: 0.2,
	}
	vpRange := []GPUCommand{{
		Kind: GPUCmdSetViewport,
		F0:   0, F1: 0, F2: w, F3: h, F4: 0.25, F5: 0.75,
	}}
	posDev, _ := runClipColorDraw(t, w, h, quadMax, shaderir.InterpPerspective, dsPosBias, vpRange)
	for i, d := range posDev.depthBuf {
		if d != 0.75 {
			t.Fatalf("positive-biased depth[%d] = %v, want clamped maxDepth 0.75", i, d)
		}
	}

	quadMin := []float32{
		-1, -1, 0, 1, 0, 0, 1, 1,
		1, -1, 0, 1, 0, 0, 1, 1,
		1, 1, 0, 1, 0, 0, 1, 1,
		-1, -1, 0, 1, 0, 0, 1, 1,
		1, 1, 0, 1, 0, 0, 1, 1,
		-1, 1, 0, 1, 0, 0, 1, 1,
	}
	dsNegBias := &GPUDepthStencilState{
		Format:         GPUFormatDepth24Plus,
		DepthWrite:     true,
		DepthCompare:   GPUCompareLessEqual,
		DepthBias:      -100000,
		DepthBiasClamp: -0.2,
	}
	negDev, _ := runClipColorDraw(t, w, h, quadMin, shaderir.InterpPerspective, dsNegBias, vpRange)
	for i, d := range negDev.depthBuf {
		if d != 0.25 {
			t.Fatalf("negative-biased depth[%d] = %v, want clamped minDepth 0.25", i, d)
		}
	}
}

// TestGPUSoftwareClippedFanBlendsEachPixelOnce draws additive-blend geometry that the clipper splits into a fan of triangles (one larger than the guard band, one crossing the near plane) and a two-triangle quad whose diagonal passes through pixel centers, and checks that no pixel is blended twice along a shared edge.
func TestGPUSoftwareClippedFanBlendsEachPixelOnce(t *testing.T) {
	additive := &GPUBlendState{
		Color: GPUBlendComponent{SrcFactor: GPUBlendOne, DstFactor: GPUBlendOne, Operation: GPUBlendAdd},
		Alpha: GPUBlendComponent{SrcFactor: GPUBlendOne, DstFactor: GPUBlendOne, Operation: GPUBlendAdd},
	}
	const red = 0.2
	cases := []struct {
		name      string
		verts     []float32
		fullCover bool
	}{
		{"beyond_guard_band", []float32{
			-30, -30, 0.5, 1, red, 0, 0, 0,
			30, -30, 0.5, 1, red, 0, 0, 0,
			0, 30, 0.5, 1, red, 0, 0, 0,
		}, true},
		{"rotated_beyond_guard_band", []float32{
			-40, 25, 0.5, 1, red, 0, 0, 0,
			-10, -45, 0.5, 1, red, 0, 0, 0,
			45, 20, 0.5, 1, red, 0, 0, 0,
		}, true},
		{"crosses_near_plane", []float32{
			-4, -4, -0.5, 1, red, 0, 0, 0,
			4, -4, 0.5, 1, red, 0, 0, 0,
			0, 4, 0.5, 1, red, 0, 0, 0,
		}, false},
		{"quad_diagonal", []float32{
			-1, -1, 0.5, 1, red, 0, 0, 0,
			1, -1, 0.5, 1, red, 0, 0, 0,
			1, 1, 0.5, 1, red, 0, 0, 0,
			-1, -1, 0.5, 1, red, 0, 0, 0,
			1, 1, 0.5, 1, red, 0, 0, 0,
			-1, 1, 0.5, 1, red, 0, 0, 0,
		}, true},
	}
	for _, tc := range cases {
		for _, sz := range [][2]int{{64, 64}, {97, 53}} {
			dev, clearCol := runClipColorDrawBlend(t, sz[0], sz[1], tc.verts, shaderir.InterpPerspective, nil, additive, nil)
			img := dev.SurfaceImage()
			once := clearCol.R + floatToByte(red)
			drawn, doubled := 0, 0
			for y := 0; y < sz[1]; y++ {
				for x := 0; x < sz[0]; x++ {
					switch r := img.RGBAAt(x, y).R; r {
					case once:
						drawn++
					case clearCol.R:
					default:
						if doubled < 5 {
							t.Errorf("%s %dx%d pixel(%d,%d) R = %d, want %d (blended once) or %d (uncovered)", tc.name, sz[0], sz[1], x, y, r, once, clearCol.R)
						}
						doubled++
					}
				}
			}
			if doubled > 0 {
				t.Errorf("%s %dx%d: %d pixels blended more than once", tc.name, sz[0], sz[1], doubled)
			}
			if tc.fullCover && drawn != sz[0]*sz[1] {
				t.Errorf("%s %dx%d: %d of %d pixels covered, want all", tc.name, sz[0], sz[1], drawn, sz[0]*sz[1])
			}
			if !tc.fullCover && drawn < sz[0]*sz[1]/4 {
				t.Errorf("%s %dx%d: only %d pixels covered", tc.name, sz[0], sz[1], drawn)
			}
		}
	}
}

// TestGPUSoftwareLinearVaryingIsScreenSpace checks that @interpolate(linear) varyings ignore w both inside a triangle and at vertices the clipper creates.
func TestGPUSoftwareLinearVaryingIsScreenSpace(t *testing.T) {
	const w, h = 40, 30
	// NDC corners scaled by different w; the colors are chosen so the screen-space interpolation gives R = (ndcX+1)/2 and G = (ndcY+1)/2.
	tri := func(far, w0, w1, w2 float32) []float32 {
		c := (far + 1) / 2
		return []float32{
			-1 * w0, -1 * w0, 0.5 * w0, w0, 0, 0, 0, 1,
			far * w1, -1 * w1, 0.5 * w1, w1, c, 0, 0, 1,
			-1 * w2, far * w2, 0.5 * w2, w2, 0, c, 0, 1,
		}
	}
	cases := []struct {
		name  string
		verts []float32
	}{
		{"unclipped", tri(3, 1, 4, 2)},
		{"clipped_by_guard_band", tri(40, 1, 4, 2)},
	}
	for _, tc := range cases {
		dev, _ := runClipColorDraw(t, w, h, tc.verts, shaderir.InterpLinear, nil, nil)
		img := dev.SurfaceImage()
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				got := img.RGBAAt(x, y)
				wantR := (float64(x) + 0.5) / w * 255
				wantG := (1 - (float64(y)+0.5)/h) * 255
				if math.Abs(float64(got.R)-wantR) > 2 || math.Abs(float64(got.G)-wantG) > 2 {
					t.Fatalf("%s pixel(%d,%d) = %+v, want R ~ %.1f G ~ %.1f", tc.name, x, y, got, wantR, wantG)
				}
			}
		}
	}
}
