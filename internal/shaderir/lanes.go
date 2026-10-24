package shaderir

import (
	"encoding/binary"
	"math"
)

// LaneTypes returns the scalar type of each lane of p's uniforms, in the
// order of the backends' UniformLanes: a vector's components, a matrix's
// columns and an array's elements in turn, Type.DwordCount for each.
func (p *Program) LaneTypes() []BasicType {
	var out []BasicType
	var walk func(t *Type)
	walk = func(t *Type) {
		switch {
		case t.Main == Array:
			for range t.Length {
				walk(&t.Sub[0])
			}
		case t.IsMatrix():
			for range t.MatrixSize() * t.MatrixSize() {
				out = append(out, Float)
			}
		default:
			for range max(t.VectorElementCount(), 1) {
				out = append(out, t.Scalar())
			}
		}
	}
	for i := range p.Uniforms {
		walk(&p.Uniforms[i])
	}
	return out
}

// PutLanes writes values into buf, a uniform buffer: each at its offset
// in bytes (a backend's UniformLanes) as a little-endian 32-bit number of
// its type (LaneTypes), an int, uint or bool from the value's integer
// part, and a bool in boolSize bytes (1 in Metal, 4 elsewhere).
func PutLanes(buf []byte, offsets []int, types []BasicType, values []float64, boolSize int) {
	for i, v := range values {
		if i >= len(offsets) || i >= len(types) {
			return
		}
		at := offsets[i]
		var x uint32
		switch types[i] {
		case Float:
			x = math.Float32bits(float32(v))
		case Int:
			x = uint32(int32(v))
		case Uint:
			x = uint32(int64(v))
		case Bool:
			if v != 0 {
				x = 1
			}
			if boolSize == 1 {
				buf[at] = byte(x)
				continue
			}
		}
		binary.LittleEndian.PutUint32(buf[at:], x)
	}
}
