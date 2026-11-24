package gpu

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestImportTextureUnsupported(t *testing.T) {
	dev := NewSoftwareRenderer(8, 8, FormatUndefined).Device()
	for _, kind := range []ExternalTextureKind{ExternalD3D11Shared, ExternalDMABuf} {
		if dev.SupportsExternalTexture(kind) {
			t.Errorf("software device supports importing kind %d", kind)
		}
		if _, err := dev.ImportTexture(nil, ExternalTextureDesc{Kind: kind, Width: 4, Height: 4, Format: FormatR8Unorm, Handle: 1, FourCC: 1, Planes: []DMABufPlane{{}}}); !errors.Is(err, ErrUnsupported) {
			t.Errorf("ImportTexture of kind %d = %v, want ErrUnsupported", kind, err)
		}
	}
	if dev.SupportsFormat(FormatR16Unorm) || dev.SupportsFormat(FormatRG16Unorm) {
		t.Error("import-only formats are reported as creatable")
	}
	if _, err := dev.CreateTexture(nil, TextureDesc{Width: 4, Height: 4, Format: FormatR16Unorm, Usage: TextureSampled}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("CreateTexture(FormatR16Unorm) = %v, want ErrUnsupported", err)
	}
}

func TestImportTextureEnqueuesAndReleases(t *testing.T) {
	dev := NewSoftwareRenderer(8, 8, FormatUndefined).Device()
	dev.features.ImportD3D11Shared = true
	dev.features.ImportDMABuf = true
	var frames []driver.GPUFrame
	dev.submitFn = func(f driver.GPUFrame) {
		frames = append(frames, f)
		if f.OnComplete != nil {
			f.OnComplete()
		}
	}

	for name, bad := range map[string]ExternalTextureDesc{
		"no size":        {Kind: ExternalD3D11Shared, Format: FormatR8Unorm, Handle: 1},
		"bad format":     {Kind: ExternalD3D11Shared, Width: 4, Height: 4, Format: FormatRGBA16Float, Handle: 1},
		"no handle":      {Kind: ExternalD3D11Shared, Width: 4, Height: 4, Format: FormatR8Unorm},
		"third plane":    {Kind: ExternalD3D11Shared, Width: 4, Height: 4, Format: FormatR8Unorm, Handle: 1, Plane: 2},
		"no fourcc":      {Kind: ExternalDMABuf, Width: 4, Height: 4, Format: FormatR8Unorm, Planes: []DMABufPlane{{FD: 3}}},
		"no planes":      {Kind: ExternalDMABuf, Width: 4, Height: 4, Format: FormatR8Unorm, FourCC: 1},
		"negative fd":    {Kind: ExternalDMABuf, Width: 4, Height: 4, Format: FormatR8Unorm, FourCC: 1, Planes: []DMABufPlane{{FD: -1}}},
		"over the limit": {Kind: ExternalD3D11Shared, Width: 1 << 20, Height: 4, Format: FormatR8Unorm, Handle: 1},
	} {
		if _, err := dev.ImportTexture(nil, bad); !errors.Is(err, ErrInvalidDescriptor) {
			t.Errorf("%s: ImportTexture = %v, want ErrInvalidDescriptor", name, err)
		}
	}

	tex, err := dev.ImportTexture(nil, ExternalTextureDesc{Label: "luma", Kind: ExternalD3D11Shared, Width: 16, Height: 8, Format: FormatRG16Unorm, Handle: 0x44, Plane: 1, KeyedMutex: true, AcquireKey: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !tex.Imported() || tex.Desc().Format != FormatRG16Unorm || tex.Desc().Usage != TextureSampled || tex.Desc().MipLevels != 1 {
		t.Fatalf("imported texture desc = %+v", tex.Desc())
	}
	plane, err := dev.ImportTexture(nil, ExternalTextureDesc{Kind: ExternalDMABuf, Width: 8, Height: 4, Format: FormatR8Unorm, FourCC: 0x20203852, Modifier: DRMFormatModInvalid, Planes: []DMABufPlane{{FD: 9, Offset: 16, Pitch: 64}}})
	if err != nil {
		t.Fatal(err)
	}

	// Imported textures are only sampled.
	dev.WriteTexture(tex, TextureCopy{Texture: tex}, []byte{1, 2, 3, 4}, DataLayout{})
	var readErr error
	dev.ReadTexture(tex, TextureRegion{Width: 1, Height: 1}, func(_ []byte, err error) { readErr = err })
	if !errors.Is(readErr, ErrInvalidDescriptor) {
		t.Errorf("ReadTexture of an imported texture = %v", readErr)
	}
	enc := dev.BeginEncoder()
	enc.CopyTextureToTexture(TextureCopy{Texture: tex}, TextureCopy{Texture: plane}, 1, 1, 1)
	enc.BeginRenderPass(RenderPassDesc{ColorAttachments: []ColorAttachment{{Texture: tex}}}).End()
	cb := enc.Finish()
	if len(cb.copies) != 0 {
		t.Errorf("a copy to or from an imported texture was recorded: %+v", cb.copies)
	}
	dev.Submit()

	var imports []*driver.GPUExternalTextureDesc
	for _, f := range frames {
		for _, op := range f.Ops {
			switch op.Kind {
			case driver.GPUOpImportTexture:
				imports = append(imports, op.External)
			case driver.GPUOpWriteTexture:
				t.Errorf("a write to imported texture %d was sent", op.ID)
			}
		}
	}
	if len(imports) != 2 {
		t.Fatalf("imports sent = %d, want 2", len(imports))
	}
	if d := imports[0]; d.Kind != driver.GPUExternalD3D11Shared || d.Handle != 0x44 || d.Plane != 1 || !d.KeyedMutex || d.AcquireKey != 1 || d.Format != driver.GPUFormatRG16Unorm {
		t.Errorf("D3D11 import = %+v", d)
	}
	if d := imports[1]; d.Kind != driver.GPUExternalDMABuf || d.FourCC != 0x20203852 || d.Modifier != driver.GPUDRMFormatModInvalid || len(d.Planes) != 1 || d.Planes[0] != (driver.GPUDMABufPlane{FD: 9, Offset: 16, Pitch: 64}) {
		t.Errorf("DMA-BUF import = %+v", d)
	}

	frames = nil
	_ = tex.Close()
	dev.Submit()
	var destroyed bool
	for _, f := range frames {
		for _, op := range f.Ops {
			destroyed = destroyed || (op.Kind == driver.GPUOpDestroyTexture && op.ID == tex.id)
		}
	}
	if !destroyed {
		t.Error("Close sent no GPUOpDestroyTexture")
	}
}
