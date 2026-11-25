package gpu_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/gpu"
)

const comparisonWGSL = `
@group(0) @binding(0) var<storage, read> input: array<f32>;
@group(0) @binding(1) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn compare() {
  let n = input[0];
  var bits: u32 = 0u;
  if (n == n) { bits = bits | 1u; }
  if (n < 1.0) { bits = bits | 2u; }
  if (n <= 1.0) { bits = bits | 4u; }
  if (n > 1.0) { bits = bits | 8u; }
  if (n >= 1.0) { bits = bits | 16u; }
  if (n != n) { bits = bits | 32u; }
  let a = 1.0 < 2.0;
  let b = 2.0 < 1.0;
  if (a == b) { bits = bits | 64u; }
  if (a != b) { bits = bits | 128u; }
  if (a == a) { bits = bits | 256u; }
  out[0] = bits;
}
`

// The software device compares as IEEE 754 and WGSL do: with a NaN no ordered comparison holds and != does, and a bool compares its value (#2335).
func TestSoftwareDeviceComparesNaNAndBools(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(4, 4, gpu.FormatUndefined)
	dev := sr.Device()
	mod := gpu.MustCompileWGSL(comparisonWGSL)

	in, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "in", Usage: gpu.BufferUsageStorage, Data: packFloat32s(float32(math.NaN()))})
	if err != nil {
		t.Fatalf("CreateBuffer in: %v", err)
	}
	out, err := dev.CreateBuffer(nil, gpu.BufferDesc{Label: "out", Usage: gpu.BufferUsageStorage | gpu.UsageCopySrc, Data: packUint32s(0)})
	if err != nil {
		t.Fatalf("CreateBuffer out: %v", err)
	}
	bgl, err := dev.CreateBindGroupLayout(nil, []gpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gpu.StageCompute, Type: gpu.BindingTypeReadOnlyStorageBuffer},
		{Binding: 1, Visibility: gpu.StageCompute, Type: gpu.BindingTypeStorageBuffer},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	bg, err := dev.CreateBindGroup(nil, bgl, []gpu.BindGroupEntry{
		{Binding: 0, Buffer: in},
		{Binding: 1, Buffer: out},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	pipe, err := dev.CreateComputePipeline(nil, gpu.ComputePipelineDesc{
		Label:   "compare",
		Compute: gpu.ShaderStage{Module: mod, EntryPoint: "compare"},
		Layouts: []*gpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	enc := dev.BeginEncoder()
	cp := enc.BeginComputePass()
	cp.SetPipeline(pipe)
	cp.SetBindGroup(0, bg)
	if err := cp.Dispatch(1, 1, 1); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	cp.End()
	dev.Submit(enc.Finish())
	if err := sr.LastError(); err != nil {
		t.Fatalf("SoftwareRenderer error: %v", err)
	}

	var got []byte
	dev.ReadBuffer(out, 0, 4, func(data []byte, err error) {
		if err != nil {
			t.Fatalf("ReadBuffer: %v", err)
		}
		got = data
	})
	if len(got) < 4 {
		t.Fatalf("read %d bytes, want 4", len(got))
	}
	if bits := binary.LittleEndian.Uint32(got); bits != 416 {
		t.Errorf("comparison bits = %d, want 416 (NaN is ordered by nothing and unequal to itself, a != b, a == a)", bits)
	}
}
