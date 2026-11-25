//go:build darwin

package apple

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAppleGPUHDRLayerConfig(t *testing.T) {
	if !GPUSupported() {
		t.Skip("Metal unavailable")
	}
	cases := []struct {
		name       string
		cfg        driver.GPUViewConfig
		wantFormat driver.GPUTextureFormat
		wantCS     driver.GPUColorSpace
		wantEDR    bool
	}{
		{
			name:       "default SDR",
			cfg:        driver.GPUViewConfig{},
			wantFormat: driver.GPUFormatBGRA8Unorm,
			wantCS:     driver.GPUColorSpaceSRGB,
			wantEDR:    false,
		},
		{
			name:       "scRGB extended linear sRGB",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceExtendedLinearSRGB},
			wantFormat: driver.GPUFormatRGBA16Float,
			wantCS:     driver.GPUColorSpaceExtendedLinearSRGB,
			wantEDR:    true,
		},
		{
			name:       "HDR10 PQ 10-bit",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10PQ},
			wantFormat: driver.GPUFormatRGB10A2Unorm,
			wantCS:     driver.GPUColorSpaceHDR10PQ,
			wantEDR:    true,
		},
		{
			name:       "HDR10 PQ float16",
			cfg:        driver.GPUViewConfig{ColorFormat: driver.GPUFormatRGBA16Float, ColorSpace: driver.GPUColorSpaceHDR10PQ},
			wantFormat: driver.GPUFormatRGBA16Float,
			wantCS:     driver.GPUColorSpaceHDR10PQ,
			wantEDR:    true,
		},
		{
			name:       "HDR10 HLG",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10HLG},
			wantFormat: driver.GPUFormatRGB10A2Unorm,
			wantCS:     driver.GPUColorSpaceHDR10HLG,
			wantEDR:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := make(chan *driver.GPUEvent, 8)
			layer := NewGPULayer(tc.cfg, func(ev *driver.GPUEvent) {
				events <- ev
			})
			if layer == nil {
				t.Fatal("NewGPULayer returned nil")
			}
			defer func() {
				layer.Release()
				select {
				case <-layer.render.done:
				case <-time.After(5 * time.Second):
					t.Error("render goroutine did not stop")
				}
			}()
			layer.Resize(8, 8, 1)

			var ready *driver.GPUEvent
			select {
			case ready = <-events:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for GPUEventDeviceReady")
			}
			if ready.Kind != driver.GPUEventDeviceReady {
				t.Fatalf("event kind = %v, want GPUEventDeviceReady", ready.Kind)
			}
			if ready.SurfaceFormat != tc.wantFormat {
				t.Fatalf("SurfaceFormat = %v, want %v", ready.SurfaceFormat, tc.wantFormat)
			}
			if ready.SurfaceColorSpace != tc.wantCS {
				t.Fatalf("SurfaceColorSpace = %v, want %v", ready.SurfaceColorSpace, tc.wantCS)
			}
			if objc.Send[bool](layer.Layer, sel("respondsToSelector:"), sel("wantsExtendedDynamicRangeContent")) {
				gotEDR := objc.Send[bool](layer.Layer, sel("wantsExtendedDynamicRangeContent"))
				if gotEDR != tc.wantEDR {
					t.Fatalf("wantsExtendedDynamicRangeContent = %v, want %v", gotEDR, tc.wantEDR)
				}
			}
		})
	}
}
