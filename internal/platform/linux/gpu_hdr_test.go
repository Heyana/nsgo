package linux

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestResolveLinuxGPUSurface(t *testing.T) {
	cases := []struct {
		name       string
		cfg        driver.GPUViewConfig
		negotiated driver.GPUColorSpace
		wantFmt    driver.GPUTextureFormat
		wantCS     driver.GPUColorSpace
	}{
		{
			name:       "default SDR",
			cfg:        driver.GPUViewConfig{},
			negotiated: driver.GPUColorSpaceDefault,
			wantFmt:    driver.GPUFormatRGBA8Unorm,
			wantCS:     driver.GPUColorSpaceSRGB,
		},
		{
			name:       "scRGB extended linear sRGB",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceExtendedLinearSRGB},
			negotiated: driver.GPUColorSpaceExtendedLinearSRGB,
			wantFmt:    driver.GPUFormatRGBA16Float,
			wantCS:     driver.GPUColorSpaceExtendedLinearSRGB,
		},
		{
			name:       "HDR10 PQ 10-bit",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10PQ},
			negotiated: driver.GPUColorSpaceHDR10PQ,
			wantFmt:    driver.GPUFormatRGB10A2Unorm,
			wantCS:     driver.GPUColorSpaceHDR10PQ,
		},
		{
			name:       "HDR10 PQ float16",
			cfg:        driver.GPUViewConfig{ColorFormat: driver.GPUFormatRGBA16Float, ColorSpace: driver.GPUColorSpaceHDR10PQ},
			negotiated: driver.GPUColorSpaceHDR10PQ,
			wantFmt:    driver.GPUFormatRGBA16Float,
			wantCS:     driver.GPUColorSpaceHDR10PQ,
		},
		{
			name:       "HDR10 HLG",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10HLG},
			negotiated: driver.GPUColorSpaceHDR10HLG,
			wantFmt:    driver.GPUFormatRGB10A2Unorm,
			wantCS:     driver.GPUColorSpaceHDR10HLG,
		},
		{
			name:       "compositor fallback to SDR",
			cfg:        driver.GPUViewConfig{ColorSpace: driver.GPUColorSpaceHDR10PQ},
			negotiated: driver.GPUColorSpaceSRGB,
			wantFmt:    driver.GPUFormatRGBA8Unorm,
			wantCS:     driver.GPUColorSpaceSRGB,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, cs := resolveLinuxGPUSurface(tc.cfg, tc.negotiated)
			if f != tc.wantFmt || cs != tc.wantCS {
				t.Fatalf("resolveLinuxGPUSurface(%+v, %v) = (%v, %v), want (%v, %v)",
					tc.cfg, tc.negotiated, f, cs, tc.wantFmt, tc.wantCS)
			}
		})
	}
}

func TestGLTextureFormatRGB10A2Unorm(t *testing.T) {
	internalFmt, pixelFmt, pixelType, swizzle, ok := glTextureFormatInfo(driver.GPUFormatRGB10A2Unorm)
	if !ok || internalFmt != gpuGLRGB10A2 || pixelFmt != gpuGLRGBA || pixelType != gpuGLUnsignedInt2101010R || swizzle {
		t.Fatalf("glTextureFormatInfo(RGB10A2Unorm) = (%x, %x, %x, %v, %v)", internalFmt, pixelFmt, pixelType, swizzle, ok)
	}
}
