package core

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const testShader = `package main

var Time float
var Tint vec3
var Spots [2]vec4
var Count int

func Fragment(dst vec4, pos vec2) vec4 {
	a := 0.5 + 0.5*sin(Time+pos.x/rectSize().x)
	for i := 0; i < Count; i++ {
		a += Spots[i].w
	}
	return vec4(Tint*a, a)
}
`

func shaderItem(b *driver.MountBatch) *driver.Shader {
	for _, it := range b.Items {
		if it.Kind == driver.ItemShader {
			return it.Shader
		}
	}
	return nil
}

func TestShaderSendsLanesAndNodes(t *testing.T) {
	prog := MustCompileShader(testShader)
	var count *Signal[int]
	var clock *AnimatedValue
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		count = NewSignal(cx, 1)
		clock = NewAnimatedValue(cx, 0)
		return Shader{Program: prog, Style: Style{Height: Pt(40)},
			Uniforms: Uniforms{"Tint": RGB(0xff8000), "Time": clock},
			UniformsFunc: func() Uniforms {
				return Uniforms{"Count": count.Get(), "Spots": []any{1, 2, clock, 0.5}}
			}}
	})
	sh := shaderItem(h.commit())
	if sh == nil || sh.Program != prog.p {
		t.Fatalf("no shader item: %+v", sh)
	}
	// Time, Tint (3 lanes), Spots (8), Count.
	want := []float64{0, 1, 128.0 / 255, 0, 1, 2, 0, 0.5, 0, 0, 0, 0, 1}
	if !slices.Equal(sh.Lanes, want) {
		t.Errorf("lanes = %v, want %v", sh.Lanes, want)
	}
	if nodes := []driver.ShaderNode{{Lane: 0, Node: clock.id}, {Lane: 6, Node: clock.id}}; !slices.Equal(sh.Nodes, nodes) {
		t.Errorf("nodes = %v, want %v", sh.Nodes, nodes)
	}
	if sh := shaderItem(h.commit()); sh != nil {
		t.Error("an unchanged shader was sent again")
	}
	count.Set(2)
	if sh := shaderItem(h.commit()); sh == nil || sh.Lanes[12] != 2 {
		t.Errorf("a change sent %+v", sh)
	}
}

func TestShaderRejectsUnknownUniformsAndBadSources(t *testing.T) {
	if _, err := CompileShader("package main\n\nfunc Fragment(dst vec4, pos vec2) vec4 {\n\treturn undefined\n}\n"); err == nil ||
		!strings.Contains(err.Error(), "4:") {
		t.Errorf("err = %v, want one at line 4", err)
	}
	prog := MustCompileShader(testShader)
	for _, u := range []Uniforms{{"Missing": 1}, {"Time": []float64{1, 2}}, {"Tint": "red"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%v did not panic", u)
				}
			}()
			prog.values(u, nil)
		}()
	}
}

// A uniform UniformsFunc sets replaces the static value whole, its node
// binding too.
func TestShaderUniformsFuncReplacesAUniform(t *testing.T) {
	prog := MustCompileShader(testShader)
	var clock *AnimatedValue
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		clock = NewAnimatedValue(cx, 0)
		return Shader{Program: prog, Uniforms: Uniforms{"Time": clock, "Spots": []any{clock, clock}},
			UniformsFunc: func() Uniforms { return Uniforms{"Time": 3, "Spots": []float64{1}} }}
	})
	sh := shaderItem(h.commit())
	if sh == nil || sh.Lanes[0] != 3 || sh.Lanes[4] != 1 || sh.Lanes[5] != 0 || len(sh.Nodes) != 0 {
		t.Fatalf("shader = %+v, want Time 3 and Spots 1 with no nodes", sh)
	}
}

// UniformLanes says where a uniform lies in a shader's lanes, so a test can read the value without knowing the program's declaration order (#2251).
func TestUniformLanesFollowTheDeclarations(t *testing.T) {
	prog := MustCompileShader(testShader)
	for name, want := range map[string]struct{ lane, n int }{
		"Time":  {0, 1},
		"Tint":  {1, 3},
		"Spots": {4, 8},
		"Count": {12, 1},
	} {
		lane, n, ok := prog.UniformLanes(name)
		if !ok || lane != want.lane || n != want.n {
			t.Errorf("UniformLanes(%q) = %d, %d, %v; want %d, %d", name, lane, n, ok, want.lane, want.n)
		}
	}
	if _, _, ok := prog.UniformLanes("Missing"); ok {
		t.Error("UniformLanes named a uniform the program has not")
	}
}

// A shader's images reach the platform with each new pixel version, and an unchanged one sends no shader item (#2251).
func TestShaderSendsImagesByVersion(t *testing.T) {
	prog := MustCompileShader(testShader)
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var version *Signal[int]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		version = NewSignal(cx, 1)
		return Shader{Program: prog, Style: Style{Height: Pt(40)}, ImagesFunc: func() []ImageSource {
			return []ImageSource{{URI: "frame", Pixels: pixels, Version: uint64(version.Get())}}
		}}
	})
	sh := shaderItem(h.commit())
	if sh == nil || len(sh.Images) != 1 || sh.Images[0].Version != 1 || sh.Images[0].Pixels == nil {
		t.Fatalf("the first commit sent %+v", sh)
	}
	if sh := shaderItem(h.commit()); sh != nil {
		t.Errorf("an unchanged image sent %+v", sh)
	}
	version.Set(2)
	if sh := shaderItem(h.commit()); sh == nil || len(sh.Images) != 1 || sh.Images[0].Version != 2 {
		t.Errorf("a new version sent %+v", sh)
	}
}

func canvasDrawing(b *driver.MountBatch) *driver.Drawing {
	for _, it := range b.Items {
		if it.Kind == driver.ItemCanvas {
			return it.Drawing
		}
	}
	return nil
}

// A canvas shades a rectangle and a run of images with programs, their
// animated uniforms bound as the drawing's slots and the paint's pixels
// copied as an image op's are (#2260).
func TestCanvasShadesWithPaints(t *testing.T) {
	prog := MustCompileShader(testShader)
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	atlas := ImageSource{URI: "atlas.png"}
	var clock *AnimatedValue
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		clock = NewAnimatedValue(cx, 0)
		return Canvas{Style: Style{Width: Pt(100), Height: Pt(50)}, Draw: func(d *Drawing, _ Size) {
			red := prog.Paint(Uniforms{"Tint": RGB(0xff0000), "Time": clock}, ImageSource{Pixels: pixels, Version: 1})
			d.DrawShader(red, 1, 2, 30, 40)
			d.Save()
			d.SetShader(prog.Paint(Uniforms{"Count": 2}))
			d.DrawAtlas(atlas, []Bounds{{Size: Size{Width: 4, Height: 4}}}, []RSXform{{SCos: 1}}, nil)
			d.Restore()
			d.DrawImage(atlas, 0, 0)
		}}
	})
	d := canvasDrawing(h.commit())
	if d == nil || len(d.Ops) != 5 {
		t.Fatalf("drew %+v", d)
	}
	rect := driver.PaintShader(d.Ops[0].Shader)
	if d.Ops[0].Kind != driver.DrawShader || rect == nil || rect.Program != prog.p || d.Ops[0].W != 30 {
		t.Fatalf("the rectangle is %+v", d.Ops[0])
	}
	if rect.Lanes[1] != 1 || len(rect.Images) != 1 || rect.Images[0].Pixels == pixels || rect.Images[0].Pixels == nil {
		t.Errorf("the rectangle shades with lanes %v and images %+v, not a copy of the app's pixels", rect.Lanes, rect.Images)
	}
	if len(d.Slots) != 1 || d.Slots[0].Op != 0 || d.Slots[0].Arg != driver.ArgLane || d.Slots[0].Node != clock.id {
		t.Errorf("slots %+v, want Time's lane bound to the clock", d.Slots)
	}
	bound := d.Slots[0].Set(d.Ops[0], 2.5, 0)
	if lanes := driver.PaintShader(bound.Shader).Lanes; lanes[0] != 2.5 || rect.Lanes[0] != 0 {
		t.Errorf("binding the clock set lanes %v over %v", lanes, rect.Lanes)
	}
	if sprites := driver.PaintShader(d.Ops[2].Shader); d.Ops[2].Kind != driver.DrawAtlas || sprites == nil || sprites.Lanes[12] != 2 {
		t.Errorf("the atlas shades with %+v", sprites)
	}
	if last := d.Ops[4]; last.Kind != driver.DrawImage || driver.PaintShader(last.Shader) != nil {
		t.Errorf("Restore kept the shader for %+v", last)
	}
	if !d.Shaded() {
		t.Error("the drawing does not report it shades")
	}
}
