package gpu_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestHDRSwapchainNegotiation(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("checks headless software HDR swapchain negotiation")
	}
	cases := []struct {
		name       string
		reqFormat  gpu.TextureFormat
		reqCS      gpu.ColorSpace
		wantFormat gpu.TextureFormat
		wantCS     gpu.ColorSpace
		wantHDR    bool
	}{
		{
			name:       "default SDR",
			wantFormat: gpu.FormatRGBA8Unorm,
			wantCS:     gpu.ColorSpaceSRGB,
			wantHDR:    false,
		},
		{
			name:       "scRGB extended linear sRGB auto format",
			reqCS:      gpu.ColorSpaceExtendedLinearSRGB,
			wantFormat: gpu.FormatRGBA16Float,
			wantCS:     gpu.ColorSpaceExtendedLinearSRGB,
			wantHDR:    true,
		},
		{
			name:       "HDR10 PQ auto format",
			reqCS:      gpu.ColorSpaceHDR10PQ,
			wantFormat: gpu.FormatRGB10A2Unorm,
			wantCS:     gpu.ColorSpaceHDR10PQ,
			wantHDR:    true,
		},
		{
			name:       "HDR10 PQ explicit float16",
			reqFormat:  gpu.FormatRGBA16Float,
			reqCS:      gpu.ColorSpaceHDR10PQ,
			wantFormat: gpu.FormatRGBA16Float,
			wantCS:     gpu.ColorSpaceHDR10PQ,
			wantHDR:    true,
		},
		{
			name:       "HDR10 HLG auto format",
			reqCS:      gpu.ColorSpaceHDR10HLG,
			wantFormat: gpu.FormatRGB10A2Unorm,
			wantCS:     gpu.ColorSpaceHDR10HLG,
			wantHDR:    true,
		},
		{
			name:       "Display P3",
			reqCS:      gpu.ColorSpaceDisplayP3,
			wantFormat: gpu.FormatRGBA8Unorm,
			wantCS:     gpu.ColorSpaceDisplayP3,
			wantHDR:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotDev *gpu.Device
			var readBytes []byte
			var readErr error
			nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return gpu.View(cx, gpu.ViewProps{
					TestID:      "hdr-view",
					ColorFormat: tc.reqFormat,
					ColorSpace:  tc.reqCS,
					Style:       core.Style{FlexGrow: 1},
					OnReady: func(dev *gpu.Device) {
						gotDev = dev
						enc := dev.BeginEncoder()
						pass := enc.BeginRenderPass(gpu.RenderPassDesc{})
						pass.ClearColor(core.RGB(0x804020))
						pass.End()
						dev.Submit(enc.Finish())
						dev.ReadSurface(func(px []byte, err error) {
							readBytes = px
							readErr = err
						})
					},
				})
			}, nsgotest.Size(16, 16))

			if gotDev == nil {
				t.Fatal("device never became ready")
			}
			if gotDev.SurfaceFormat() != tc.wantFormat {
				t.Fatalf("SurfaceFormat() = %v, want %v", gotDev.SurfaceFormat(), tc.wantFormat)
			}
			if gotDev.SurfaceColorSpace() != tc.wantCS {
				t.Fatalf("SurfaceColorSpace() = %v, want %v", gotDev.SurfaceColorSpace(), tc.wantCS)
			}
			if gotDev.SurfaceColorSpace().IsHDR() != tc.wantHDR {
				t.Fatalf("SurfaceColorSpace().IsHDR() = %v, want %v", gotDev.SurfaceColorSpace().IsHDR(), tc.wantHDR)
			}
			if readErr != nil {
				t.Fatalf("ReadSurface error: %v", readErr)
			}
			wantBytes := 16 * 16 * 4
			if len(readBytes) != wantBytes {
				t.Fatalf("ReadSurface len = %d, want %d", len(readBytes), wantBytes)
			}
		})
	}
}

func TestDisplayInfoHDRReactive(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("checks reactive DisplayInfo HDR updates in headless test host")
	}
	var observed []core.DisplayInfo
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() {
			observed = append(observed, cx.Env().Display())
		})
		return core.View{TestID: "root"}
	}, nsgotest.Environment(core.Environment{
		OS:        core.OSMacOS,
		Idiom:     core.IdiomDesktop,
		DisplayID: "disp-1",
		Displays: []core.DisplayInfo{{
			ID:           "disp-1",
			Main:         true,
			Scale:        2,
			HDR:          false,
			HDRSupported: true,
			HDRHeadroom:  1.0,
		}},
	}))

	if len(observed) != 1 || observed[0].HDR || !observed[0].HDRSupported || observed[0].HDRHeadroom != 1.0 {
		t.Fatalf("initial display = %+v", observed)
	}

	app.SetEnvironment(core.Environment{
		OS:        core.OSMacOS,
		Idiom:     core.IdiomDesktop,
		DisplayID: "disp-1",
		Displays: []core.DisplayInfo{{
			ID:            "disp-1",
			Main:          true,
			Scale:         2,
			HDR:           true,
			HDRSupported:  true,
			HDRHeadroom:   4.0,
			SDRWhiteLevel: 200,
			MaxLuminance:  800,
		}},
	})

	if len(observed) != 2 {
		t.Fatalf("expected reactive update on HDR toggle, got %d observations", len(observed))
	}
	got := observed[1]
	if !got.HDR || !got.HDRSupported || got.HDRHeadroom != 4.0 || got.SDRWhiteLevel != 200 || got.MaxLuminance != 800 {
		t.Fatalf("updated display = %+v", got)
	}
}
