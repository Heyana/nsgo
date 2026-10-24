package shader

import (
	"bytes"
	"fmt"

	"github.com/oneo-me/nsgo/internal/shaderir"
)

// Entry is the name of a shader's entry point, which returns the color
// of a pixel, premultiplied:
//
//	func Fragment(dst vec4, pos vec2) vec4
//
// dst is the pixel's center in the shader's view, in its pixels from the
// top left corner, and pos the same point in points. It may take dst
// alone, or nothing.
const Entry = "Fragment"

// The uniform variables the bridge declares, which platforms set, before
// the shader's own: the size of the view the shader fills in points, and
// the pixels a point.
const (
	RectSizeUniform = "__rectSize"
	ScaleUniform    = "__scale"
)

// BridgeLanes is how many lanes the bridge's uniforms take, before the
// shader's own.
const BridgeLanes = 3

// bridge is the source appended to a shader: the uniform variables that
// only platforms set, the functions that read them, and the vertex entry
// point, which only gives the fragment entry point its arguments:
// platforms draw with vertex stages of their own.
var bridge = fmt.Sprintf(`
%s

var %s vec2
var %s float

// rectSize returns the size of the view the shader fills, in points.
func rectSize() vec2 {
	return %[2]s
}

// pixelScale returns how many pixels of the view make a point.
func pixelScale() float {
	return %[3]s
}

func %s(dst vec2, pos vec2) (vec4, vec2) {
	return vec4(dst, 0, 1), pos
}
%s
`, InternalRegionBegin, RectSizeUniform, ScaleUniform, vertexEntry, InternalRegionEnd)

const vertexEntry = "__vertex"

// CompileFragment compiles an NSGo shader: Go's syntax in package main,
// with uniform variables (exported, package level) that the app sets and
// the entry point Fragment, which may call functions of its own.
func CompileFragment(src []byte) (*shaderir.Program, error) {
	var b bytes.Buffer
	b.Write(src)
	b.WriteString(bridge)
	p, err := Compile(b.Bytes(), vertexEntry, Entry, 0)
	if err != nil {
		return nil, err
	}
	if p.FragmentFunc.Block == nil {
		return nil, fmt.Errorf("shader: no entry point %s", Entry)
	}
	if len(p.UniformNames) < 2 || p.UniformNames[0] != RectSizeUniform || p.UniformNames[1] != ScaleUniform {
		return nil, fmt.Errorf("shader: the uniforms start with %v, not the bridge's", p.UniformNames)
	}
	p.FragmentSource = bytes.Clone(src)
	return p, nil
}
