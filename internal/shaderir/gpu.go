package shaderir

// StageBuiltin identifies a shader stage built-in input or output.
type StageBuiltin uint8

const (
	BuiltinNone StageBuiltin = iota
	BuiltinPosition
	BuiltinVertexIndex
	BuiltinInstanceIndex
	BuiltinFrontFacing
	BuiltinFragDepth
	BuiltinGlobalInvocationID
	BuiltinLocalInvocationID
	BuiltinWorkgroupID
	BuiltinNumWorkgroups
	BuiltinLocalInvocationIndex
)

func (b StageBuiltin) String() string {
	switch b {
	case BuiltinPosition:
		return "position"
	case BuiltinVertexIndex:
		return "vertex_index"
	case BuiltinInstanceIndex:
		return "instance_index"
	case BuiltinFrontFacing:
		return "front_facing"
	case BuiltinFragDepth:
		return "frag_depth"
	case BuiltinGlobalInvocationID:
		return "global_invocation_id"
	case BuiltinLocalInvocationID:
		return "local_invocation_id"
	case BuiltinWorkgroupID:
		return "workgroup_id"
	case BuiltinNumWorkgroups:
		return "num_workgroups"
	case BuiltinLocalInvocationIndex:
		return "local_invocation_index"
	default:
		return ""
	}
}

// Interpolation specifies how a varying is interpolated across a primitive.
type Interpolation uint8

const (
	InterpPerspective Interpolation = iota
	InterpFlat
	InterpLinear
)

func (i Interpolation) String() string {
	switch i {
	case InterpFlat:
		return "flat"
	case InterpLinear:
		return "linear"
	default:
		return "perspective"
	}
}

// StageIO carries optional location, built-in, name, and interpolation metadata
// for a vertex attribute or inter-stage varying.
type StageIO struct {
	Name          string
	Location      uint32
	Builtin       StageBuiltin
	Interpolation Interpolation
}

// BindingKind classifies an explicit @group(G) @binding(B) resource binding.
type BindingKind uint8

const (
	BindingUniformBuffer BindingKind = iota
	BindingStorageBufferRead
	BindingTexture
	BindingSampler
	BindingStorageBufferReadWrite
	BindingStorageTexture
)

// StorageTextureFormat identifies the texel format of a storage texture binding.
type StorageTextureFormat uint8

const (
	StorageFormatUndefined StorageTextureFormat = iota
	StorageFormatRGBA8Unorm
	StorageFormatRGBA16Float
	StorageFormatRGBA32Float
	StorageFormatR32Float
	StorageFormatR32Uint
	StorageFormatR32Sint
)

func (f StorageTextureFormat) WGSLName() string {
	switch f {
	case StorageFormatRGBA8Unorm:
		return "rgba8unorm"
	case StorageFormatRGBA16Float:
		return "rgba16float"
	case StorageFormatRGBA32Float:
		return "rgba32float"
	case StorageFormatR32Float:
		return "r32float"
	case StorageFormatR32Uint:
		return "r32uint"
	case StorageFormatR32Sint:
		return "r32sint"
	default:
		return "rgba8unorm"
	}
}

func (f StorageTextureFormat) GLSLName() string {
	switch f {
	case StorageFormatRGBA8Unorm:
		return "rgba8"
	case StorageFormatRGBA16Float:
		return "rgba16f"
	case StorageFormatRGBA32Float:
		return "rgba32f"
	case StorageFormatR32Float:
		return "r32f"
	case StorageFormatR32Uint:
		return "r32ui"
	case StorageFormatR32Sint:
		return "r32i"
	default:
		return "rgba8"
	}
}

// StorageAccess identifies the access mode of a storage buffer or storage texture.
type StorageAccess uint8

const (
	StorageAccessRead StorageAccess = iota
	StorageAccessWrite
	StorageAccessReadWrite
)

func (a StorageAccess) WGSLName() string {
	switch a {
	case StorageAccessRead:
		return "read"
	case StorageAccessReadWrite:
		return "read_write"
	default:
		return "write"
	}
}

// Binding describes a resource declared with @group(G) @binding(B).
type Binding struct {
	Group         uint32
	Binding       uint32
	Name          string
	Kind          BindingKind
	Type          Type
	StorageFormat StorageTextureFormat
	StorageAccess StorageAccess
}

// IsPipeline reports whether p uses explicit GPU pipeline stages or bindings
// rather than the legacy fragment-only UI shader convention.
func (p *Program) IsPipeline() bool {
	return len(p.Bindings) > 0 || len(p.AttributeMeta) > 0 || len(p.VaryingMeta) > 0 || len(p.FragmentOutputs) > 0 || p.VertexEntry != "" || p.FragmentEntry != "" || p.ComputeEntry != "" || p.ComputeFunc.Block != nil
}

// AlignBytes returns the WGSL/std140 uniform alignment of t in bytes.
func (t Type) AlignBytes() int {
	switch t.Main {
	case Bool, Int, Uint, Float:
		return 4
	case Vec2, IVec2, UVec2:
		return 8
	case Vec3, Vec4, IVec3, IVec4, UVec3, UVec4:
		return 16
	case Mat2, Mat3x2, Mat4x2:
		return 8
	case Mat2x3, Mat2x4, Mat3, Mat3x4, Mat4x3, Mat4:
		return 16
	case Array:
		if len(t.Sub) == 0 {
			return 16
		}
		a := t.Sub[0].AlignBytes()
		if a < 16 {
			return 16
		}
		return a
	case Struct:
		maxAlign := 4
		for _, f := range t.Sub {
			if a := f.AlignBytes(); a > maxAlign {
				maxAlign = a
			}
		}
		return roundUp(maxAlign, 16)
	default:
		return 4
	}
}

// SizeBytes returns the WGSL/std140 byte size of t.
func (t Type) SizeBytes() int {
	switch t.Main {
	case Bool, Int, Uint, Float:
		return 4
	case Vec2, IVec2, UVec2:
		return 8
	case Vec3, IVec3, UVec3:
		return 12
	case Vec4, IVec4, UVec4:
		return 16
	case Mat2:
		return 16
	case Mat2x3:
		return 32
	case Mat2x4:
		return 32
	case Mat3x2:
		return 24
	case Mat3:
		return 48
	case Mat3x4:
		return 48
	case Mat4x2:
		return 32
	case Mat4x3:
		return 64
	case Mat4:
		return 64
	case Array:
		if len(t.Sub) == 0 {
			return 0
		}
		a := t.Sub[0].AlignBytes()
		s := t.Sub[0].SizeBytes()
		stride := roundUp(s, max(a, 16))
		return stride * t.Length
	case Struct:
		offset := 0
		maxAlign := 4
		for _, f := range t.Sub {
			a := f.AlignBytes()
			if a > maxAlign {
				maxAlign = a
			}
			offset = roundUp(offset, a) + f.SizeBytes()
		}
		return roundUp(offset, maxAlign)
	default:
		return 0
	}
}

// StructFieldOffset returns the byte offset of field idx in a Struct type.
func (t Type) StructFieldOffset(idx int) int {
	if t.Main != Struct || idx < 0 || idx >= len(t.Sub) {
		return 0
	}
	offset := 0
	for i := 0; i <= idx; i++ {
		f := t.Sub[i]
		offset = roundUp(offset, f.AlignBytes())
		if i == idx {
			return offset
		}
		offset += f.SizeBytes()
	}
	return offset
}

// StorageAlignBytes returns the WGSL storage / GLSL std430 alignment of t in bytes.
func (t Type) StorageAlignBytes() int {
	switch t.Main {
	case Array:
		if len(t.Sub) == 0 {
			return 4
		}
		return t.Sub[0].StorageAlignBytes()
	case Struct:
		maxAlign := 4
		for _, f := range t.Sub {
			if a := f.StorageAlignBytes(); a > maxAlign {
				maxAlign = a
			}
		}
		return maxAlign
	default:
		return t.AlignBytes()
	}
}

// StorageStride returns the WGSL storage / GLSL std430 element stride in bytes for an element type inside a storage buffer array. Unlike std140 uniform arrays, storage array elements are aligned to their own alignment rather than rounded up to 16 bytes.
func (t Type) StorageStride() int {
	return roundUp(t.StorageSizeBytes(), t.StorageAlignBytes())
}

// StorageSizeBytes returns the WGSL storage / GLSL std430 byte size of t. For runtime-sized arrays (Length == 0) it returns 0.
func (t Type) StorageSizeBytes() int {
	switch t.Main {
	case Array:
		if len(t.Sub) == 0 || t.Length == 0 {
			return 0
		}
		return t.Sub[0].StorageStride() * t.Length
	case Struct:
		offset := 0
		maxAlign := 4
		for _, f := range t.Sub {
			a := f.StorageAlignBytes()
			if a > maxAlign {
				maxAlign = a
			}
			offset = roundUp(offset, a) + f.StorageSizeBytes()
		}
		return roundUp(offset, maxAlign)
	default:
		return t.SizeBytes()
	}
}

// StorageStructFieldOffset returns the std430 / WGSL storage byte offset of field idx in a Struct type.
func (t Type) StorageStructFieldOffset(idx int) int {
	if t.Main != Struct || idx < 0 || idx >= len(t.Sub) {
		return 0
	}
	offset := 0
	for i := 0; i <= idx; i++ {
		f := t.Sub[i]
		offset = roundUp(offset, f.StorageAlignBytes())
		if i == idx {
			return offset
		}
		offset += f.StorageSizeBytes()
	}
	return offset
}

func roundUp(n, align int) int {
	if align <= 1 {
		return n
	}
	return (n + align - 1) / align * align
}
