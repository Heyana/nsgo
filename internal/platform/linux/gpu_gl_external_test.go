package linux

import (
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeEGLImporter struct {
	modifiers bool
	next      uintptr
	live      map[uintptr]bool
	attribs   [][]int32
}

func (f *fakeEGLImporter) Modifiers() bool { return f.modifiers }

func (f *fakeEGLImporter) CreateDMABufImage(attribs []int32) (uintptr, error) {
	f.next++
	if f.live == nil {
		f.live = map[uintptr]bool{}
	}
	f.live[f.next] = true
	f.attribs = append(f.attribs, slices.Clone(attribs))
	return f.next, nil
}

func (f *fakeEGLImporter) DestroyImage(image uintptr) { delete(f.live, image) }

func TestDMABufImageAttribs(t *testing.T) {
	desc := &driver.GPUExternalTextureDesc{
		Kind: driver.GPUExternalDMABuf, Width: 64, Height: 36, Format: driver.GPUFormatRG8Unorm,
		FourCC: drmFormatGR88, Modifier: 0x0100000000000002,
		Planes: []driver.GPUDMABufPlane{{FD: 7, Offset: 4096, Pitch: 128}},
	}
	got, err := dmaBufImageAttribs(desc, true)
	want := []int32{
		eglWidth, 64, eglHeight, 36, eglLinuxDRMFourCC, drmFormatGR88,
		eglDMABufPlane0FD, 7, eglDMABufPlane0Offset, 4096, eglDMABufPlane0Pitch, 128,
		eglDMABufPlane0ModLo, 2, eglDMABufPlane0ModHi, 0x01000000,
		eglNone,
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("attribs = %v, %v; want %v", got, err, want)
	}
	if _, err := dmaBufImageAttribs(desc, false); err == nil {
		t.Error("a tiled modifier without EGL_EXT_image_dma_buf_import_modifiers was accepted")
	}
	linear := *desc
	linear.Modifier = drmFormatModLinear
	if got, err := dmaBufImageAttribs(&linear, false); err != nil || got[len(got)-1] != eglNone || len(got) != 13 {
		t.Errorf("linear without modifiers = %v, %v", got, err)
	}
	implicit := *desc
	implicit.Modifier = driver.GPUDRMFormatModInvalid
	if got, err := dmaBufImageAttribs(&implicit, true); err != nil || len(got) != 13 {
		t.Errorf("DRM_FORMAT_MOD_INVALID = %v, %v; want no modifier attributes", got, err)
	}
	for name, bad := range map[string]driver.GPUExternalTextureDesc{
		"multi-plane NV12":  {Width: 4, Height: 4, Format: driver.GPUFormatR8Unorm, FourCC: 0x3231564e, Planes: desc.Planes},
		"format mismatch":   {Width: 4, Height: 4, Format: driver.GPUFormatR8Unorm, FourCC: drmFormatGR88, Planes: desc.Planes},
		"two planes for R8": {Width: 4, Height: 4, Format: driver.GPUFormatR8Unorm, FourCC: drmFormatR8, Planes: []driver.GPUDMABufPlane{{FD: 1}, {FD: 2}}},
	} {
		if _, err := dmaBufImageAttribs(&bad, true); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if f, ok := drmPlaneFormat(drmFormatR16); !ok || f != driver.GPUFormatR16Unorm {
		t.Errorf("R16 samples as %s", f)
	}
	if f, ok := drmPlaneFormat(drmFormatXRGB8888); !ok || f != driver.GPUFormatBGRA8Unorm {
		t.Errorf("XRGB8888 samples as %s", f)
	}
}

func TestGLGPUImportTextureBindsAndReleasesEGLImage(t *testing.T) {
	gl := newMockGL()
	imp := &fakeEGLImporter{modifiers: true}
	ctx := newGLGPUContext(gl, false, 1, driver.DefaultGPULimits(), driver.GPUFeatures{ImportDMABuf: true})
	ctx.importer = imp
	luma := &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalDMABuf, Width: 8, Height: 8, Format: driver.GPUFormatR8Unorm, FourCC: drmFormatR8, Modifier: driver.GPUDRMFormatModInvalid, Planes: []driver.GPUDMABufPlane{{FD: 3, Pitch: 64}}}
	chroma := &driver.GPUExternalTextureDesc{Kind: driver.GPUExternalDMABuf, Width: 4, Height: 4, Format: driver.GPUFormatRG8Unorm, FourCC: drmFormatGR88, Modifier: driver.GPUDRMFormatModInvalid, Planes: []driver.GPUDMABufPlane{{FD: 3, Offset: 512, Pitch: 64}}}
	_, errs := ctx.executeFrame(driver.GPUFrame{Ops: []driver.GPUResourceOp{
		{Kind: driver.GPUOpImportTexture, ID: 1, External: luma},
		{Kind: driver.GPUOpImportTexture, ID: 2, External: chroma},
		{Kind: driver.GPUOpWriteTexture, ID: 1, Data: []byte{1, 2, 3, 4}, DataLayout: driver.GPUDataLayout{BytesPerRow: 2}},
	}}, 0, 8, 8)
	if len(errs) != 1 || !strings.Contains(errs[0].message, "imported") {
		t.Fatalf("errors = %+v, want only the write to the imported texture rejected", errs)
	}
	if len(imp.live) != 2 || len(gl.eglImageTargets) != 2 || ctx.textures[2].eglImage == 0 || ctx.textures[2].format != driver.GPUFormatRG8Unorm {
		t.Fatalf("images %v, targets %v, textures %+v", imp.live, gl.eglImageTargets, ctx.textures)
	}
	if len(gl.texUploads) != 0 {
		t.Fatalf("an imported texture was uploaded to: %+v", gl.texUploads)
	}
	ctx.executeFrame(driver.GPUFrame{Ops: []driver.GPUResourceOp{{Kind: driver.GPUOpDestroyTexture, ID: 1}}}, 0, 8, 8)
	if len(imp.live) != 1 || len(gl.liveTextures) != 1 {
		t.Fatalf("after destroy: images %v, textures %v", imp.live, gl.liveTextures)
	}
	ctx.destroyAll()
	if len(imp.live) != 0 || len(gl.liveTextures) != 0 {
		t.Fatalf("leak after destroyAll: images %v, textures %v", imp.live, gl.liveTextures)
	}

	// Without an EGL importer, imports are unsupported.
	plain := newGLGPUContext(newMockGL(), false, 1, driver.DefaultGPULimits(), driver.GPUFeatures{})
	if err := plain.importTexture(1, luma); err == nil || err.code != driver.GPUErrorUnsupported {
		t.Fatalf("import without EGL = %+v", err)
	}
}
