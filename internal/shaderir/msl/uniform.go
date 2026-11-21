package msl

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

// UniformLayout returns where each of p's uniform variables lies in the
// struct Uniforms of Fragment, in bytes, and the struct's size, as Metal
// lays it out: a vector of 3 takes the room of 4, a matrix is its
// columns, and an array's elements are as far apart as their alignment
// makes them.
func UniformLayout(p *shaderir.Program) (offsets []int, size int) {
	align := 1
	for i := range p.Uniforms {
		s, a := sizeAlign(&p.Uniforms[i])
		size = roundUp(size, a)
		offsets = append(offsets, size)
		size += s
		align = max(align, a)
	}
	return offsets, roundUp(size, align)
}

// UniformLanes returns, for each lane of p's uniforms (in order,
// columns of matrices, elements of arrays: Type.DwordCount of each),
// its offset in bytes in the struct Uniforms of Fragment, and the
// struct's size.
func UniformLanes(p *shaderir.Program) (lanes []int, size int) {
	offsets, size := UniformLayout(p)
	for i := range p.Uniforms {
		for _, at := range typeLanes(&p.Uniforms[i]) {
			lanes = append(lanes, offsets[i]+at)
		}
	}
	return lanes, size
}

// typeLanes returns the offset in bytes of each lane of a value of type t
// in its Metal layout.
func typeLanes(t *shaderir.Type) []int {
	var out []int
	var walk func(t *shaderir.Type, base int)
	walk = func(t *shaderir.Type, base int) {
		switch {
		case t.Main == shaderir.Array:
			s, a := sizeAlign(&t.Sub[0])
			stride := roundUp(s, a)
			for i := range t.Length {
				walk(&t.Sub[0], base+i*stride)
			}
		case t.IsMatrix():
			cols, rows := t.MatrixCols(), t.MatrixRows()
			col := shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, rows)}
			s, a := sizeAlign(&col)
			stride := roundUp(s, a)
			for c := range cols {
				walk(&col, base+c*stride)
			}
		case t.Main == shaderir.Bool:
			out = append(out, base)
		default:
			n := max(t.VectorElementCount(), 1)
			for i := range n {
				out = append(out, base+4*i)
			}
		}
	}
	walk(t, 0)
	return out
}

// sizeAlign returns the size and the alignment of t in Metal, in bytes.
func sizeAlign(t *shaderir.Type) (size, align int) {
	switch t.Main {
	case shaderir.Bool:
		return 1, 1
	case shaderir.Int, shaderir.Uint, shaderir.Float:
		return 4, 4
	case shaderir.Vec2, shaderir.IVec2, shaderir.UVec2:
		return 8, 8
	case shaderir.Vec3, shaderir.IVec3, shaderir.UVec3, shaderir.Vec4, shaderir.IVec4, shaderir.UVec4:
		return 16, 16
	case shaderir.Mat2:
		return 16, 8
	case shaderir.Mat3:
		return 48, 16
	case shaderir.Mat4:
		return 64, 16
	case shaderir.Mat2x3, shaderir.Mat2x4:
		return 32, 16
	case shaderir.Mat3x2:
		return 24, 8
	case shaderir.Mat3x4:
		return 48, 16
	case shaderir.Mat4x2:
		return 32, 8
	case shaderir.Mat4x3:
		return 64, 16
	case shaderir.Array:
		s, a := sizeAlign(&t.Sub[0])
		return t.Length * roundUp(s, a), a
	}
	panic(fmt.Sprintf("msl: no layout for %s", t))
}

func roundUp(x, a int) int { return (x + a - 1) / a * a }
