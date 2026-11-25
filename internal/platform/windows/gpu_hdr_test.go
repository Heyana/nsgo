package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestD3DResolveSwapFormat(t *testing.T) {
	cases := []struct {
		name        string
		cfg         driver.GPUViewConfig
		wantFmt     driver.GPUTextureFormat
		wantCS      driver.GPUColorSpace
		wantDXGIFmt uint32
		wantDXGICS  uint32
	}{
		{
			name:        "default SDR",
			cfg:         driver.GPUViewConfig{},
			wantFmt:     driver.GPUFormatBGRA8Unorm,
			wantCS:      driver.GPUColorSpaceSRGB,
			wantDXGIFmt: dxgiFmtB8G8R8A8Unorm,
			wantDXGICS:  dxgiColorSpaceRGBFullG22NoneP709,
		},
		{
			name:        "scRGB extended linear sRGB",
			cfg:         driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceExtendedLinearSRGB},
			wantFmt:     driver.GPUFormatRGBA16Float,
			wantCS:      driver.GPUColorSpaceExtendedLinearSRGB,
			wantDXGIFmt: dxgiFmtR16G16B16A16Float,
			wantDXGICS:  dxgiColorSpaceRGBFullG10NoneP709,
		},
		{
			name:        "HDR10 PQ 10-bit",
			cfg:         driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10PQ},
			wantFmt:     driver.GPUFormatRGB10A2Unorm,
			wantCS:      driver.GPUColorSpaceHDR10PQ,
			wantDXGIFmt: dxgiFmtR10G10B10A2Unorm,
			wantDXGICS:  dxgiColorSpaceRGBFullG2084NoneP2020,
		},
		{
			name:        "HDR10 PQ float16",
			cfg:         driver.GPUViewConfig{ColorFormat: driver.GPUFormatRGBA16Float, ColorSpace: driver.GPUColorSpaceHDR10PQ},
			wantFmt:     driver.GPUFormatRGBA16Float,
			wantCS:      driver.GPUColorSpaceHDR10PQ,
			wantDXGIFmt: dxgiFmtR16G16B16A16Float,
			wantDXGICS:  dxgiColorSpaceRGBFullG2084NoneP2020,
		},
		{
			name:        "HLG falls back to SDR on DXGI",
			cfg:         driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10HLG},
			wantFmt:     driver.GPUFormatBGRA8Unorm,
			wantCS:      driver.GPUColorSpaceSRGB,
			wantDXGIFmt: dxgiFmtB8G8R8A8Unorm,
			wantDXGICS:  dxgiColorSpaceRGBFullG22NoneP709,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, cs, dxgiFmt, dxgiCS := d3dResolveSwapFormat(tc.cfg)
			if f != tc.wantFmt || cs != tc.wantCS || dxgiFmt != tc.wantDXGIFmt || dxgiCS != tc.wantDXGICS {
				t.Fatalf("d3dResolveSwapFormat(%+v) = (%v, %v, %d, %d), want (%v, %v, %d, %d)",
					tc.cfg, f, cs, dxgiFmt, dxgiCS, tc.wantFmt, tc.wantCS, tc.wantDXGIFmt, tc.wantDXGICS)
			}
		})
	}
}

func TestD3DTextureFormatRGB10A2Unorm(t *testing.T) {
	texFmt, srvFmt, rtvFmt, dsvFmt := d3dTextureFormats(driver.GPUFormatRGB10A2Unorm, true)
	if texFmt != dxgiFmtR10G10B10A2Unorm || srvFmt != dxgiFmtR10G10B10A2Unorm || rtvFmt != dxgiFmtR10G10B10A2Unorm || dsvFmt != dxgiFmtUnknown {
		t.Fatalf("d3dTextureFormats(RGB10A2Unorm) = (%d, %d, %d, %d)", texFmt, srvFmt, rtvFmt, dsvFmt)
	}
}
