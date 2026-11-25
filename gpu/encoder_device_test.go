package gpu_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/gpu"
)

// An encoder and its passes report the device they record for, so a helper that takes only an encoder can notice a restored device (#2345).
func TestEncoderReportsItsDevice(t *testing.T) {
	sr := gpu.NewSoftwareRenderer(4, 4, gpu.FormatUndefined)
	dev := sr.Device()
	enc := dev.BeginEncoder()
	if got := enc.Device(); got != dev {
		t.Fatalf("encoder device = %p, want %p", got, dev)
	}
	rp := enc.BeginRenderPass(gpu.RenderPassDesc{})
	if got := rp.Device(); got != dev {
		t.Fatalf("render pass device = %p, want %p", got, dev)
	}
	rp.End()
	cp := enc.BeginComputePass()
	if got := cp.Device(); got != dev {
		t.Fatalf("compute pass device = %p, want %p", got, dev)
	}
	cp.End()
	dev.Submit(enc.Finish())

	var nilEncoder *gpu.Encoder
	if nilEncoder.Device() != nil {
		t.Fatal("a nil encoder reported a device")
	}
	var nilPass *gpu.RenderPass
	if nilPass.Device() != nil {
		t.Fatal("a nil render pass reported a device")
	}
}
