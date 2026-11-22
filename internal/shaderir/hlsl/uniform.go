// Copyright 2022 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Modified by the MyGo contributors: uint and uvecN.

package hlsl

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/shaderir"
)

const UniformVariableBoundaryInDwords = 4

// UniformVariableOffsetsInDwords returns the offsets of the uniform variables in DWORD units in the HLSL layout.
func UniformVariableOffsetsInDwords(program *shaderir.Program) []int {
	// https://docs.microsoft.com/en-us/windows/win32/direct3dhlsl/dx-graphics-hlsl-packing-rules
	// https://github.com/microsoft/DirectXShaderCompiler/wiki/Buffer-Packing

	align := func(x int) int {
		if x == 0 {
			return 0
		}
		return ((x-1)/UniformVariableBoundaryInDwords + 1) * UniformVariableBoundaryInDwords
	}

	var offsetsInDwords []int
	var headInDwords int

	// TODO: Reorder the variables with packoffset.
	// See https://docs.microsoft.com/en-us/windows/win32/direct3dhlsl/dx-graphics-hlsl-variable-packoffset
	for _, u := range program.Uniforms {
		switch u.Main {
		case shaderir.Bool:
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += 1
		case shaderir.Float:
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += 1
		case shaderir.Int, shaderir.Uint:
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += 1
		case shaderir.Vec2, shaderir.IVec2, shaderir.UVec2:
			if headInDwords%UniformVariableBoundaryInDwords >= 3 {
				headInDwords = align(headInDwords)
			}
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += 2
		case shaderir.Vec3, shaderir.IVec3, shaderir.UVec3:
			if headInDwords%UniformVariableBoundaryInDwords >= 2 {
				headInDwords = align(headInDwords)
			}
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += 3
		case shaderir.Vec4, shaderir.IVec4, shaderir.UVec4:
			if headInDwords%UniformVariableBoundaryInDwords >= 1 {
				headInDwords = align(headInDwords)
			}
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += 4
		case shaderir.Mat2, shaderir.Mat3, shaderir.Mat4,
			shaderir.Mat2x3, shaderir.Mat2x4, shaderir.Mat3x2, shaderir.Mat3x4, shaderir.Mat4x2, shaderir.Mat4x3:
			// In HLSL cbuffers (column_major by default), a floatCxR has R columns of C dwords each, each column starting on a 4-dword register boundary.
			headInDwords = align(headInDwords)
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			headInDwords += (u.MatrixRows()-1)*UniformVariableBoundaryInDwords + u.MatrixCols()
		case shaderir.Array:
			// Each array is 16-byte aligned.
			// TODO: What if the array has 2 or more dimensions?
			headInDwords = align(headInDwords)
			offsetsInDwords = append(offsetsInDwords, headInDwords)
			n := u.Sub[0].DwordCount()
			if u.Sub[0].IsMatrix() {
				n = (u.Sub[0].MatrixRows()-1)*UniformVariableBoundaryInDwords + u.Sub[0].MatrixCols()
			}
			// Kage rejects a zero-length array, but even if one reaches here, the head must not
			// move backwards.
			if u.Length > 1 {
				headInDwords += (u.Length - 1) * align(n)
			}
			// The last element is not followed by padding.
			headInDwords += n
		case shaderir.Struct:
			// TODO: Implement this
			panic("hlsl: offset for a struct is not implemented yet")
		default:
			panic(fmt.Sprintf("hlsl: unexpected type: %s", u.String()))
		}
	}

	return offsetsInDwords
}

// UniformLanes returns, for each lane of p's uniforms (in order,
// columns of matrices, elements of arrays: Type.DwordCount of each),
// its offset in bytes in the cbuffer of Fragment, and the cbuffer's
// size, a multiple of 16: an array's elements and a matrix's rows start
// at a register each, and the backend keeps matrices transposed (rows
// of HLSL are columns of GLSL).
func UniformLanes(p *shaderir.Program) (lanes []int, size int) {
	offsets := UniformVariableOffsetsInDwords(p)
	var walk func(t *shaderir.Type, dword int)
	walk = func(t *shaderir.Type, dword int) {
		switch {
		case t.Main == shaderir.Array:
			stride := t.Sub[0].DwordCount()
			if t.Sub[0].IsMatrix() {
				stride = t.Sub[0].MatrixRows() * 4
			}
			stride = (stride + 3) / 4 * 4
			for i := range t.Length {
				walk(&t.Sub[0], dword+i*stride)
			}
		case t.IsMatrix():
			cols, rows := t.MatrixCols(), t.MatrixRows()
			for col := range cols {
				for row := range rows {
					lanes = append(lanes, 4*(dword+row*4+col))
				}
			}
		default:
			for i := range t.DwordCount() {
				lanes = append(lanes, 4*(dword+i))
			}
		}
	}
	for i := range p.Uniforms {
		walk(&p.Uniforms[i], offsets[i])
	}
	for _, l := range lanes {
		size = max(size, l+4)
	}
	return lanes, (size + 15) / 16 * 16
}
